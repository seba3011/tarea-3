package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "sync"
    "bytes"
    "net"   
    "strconv"
    "github.com/seba3011/tarea-3/common"
)

const (
    ConfigFile = "node2/config.json"
    StateFile = "node2/estado_node2.json"
)
type Node struct {
    ID     int
    IsPrimary  bool
    PrimaryID  int
    StateFile  string
    State    *common.NodeState 
    StateMutex sync.RWMutex
}

var GlobalNode *Node 

func main() {
    cfg := loadConfig(ConfigFile)
    GlobalNode = &Node{
        ID:      cfg.ID,
        StateFile:  fmt.Sprintf("node%d/estado_node%d.json", cfg.ID, cfg.ID),
        IsPrimary: cfg.IsPrimary,
        PrimaryID: -1, 
        StateMutex: sync.RWMutex{},
    }
    GlobalNode.State = initState(GlobalNode.StateFile, GlobalNode.ID)
    
    if cfg.IsPrimary {
        GlobalNode.PrimaryID = cfg.ID
        GlobalNode.IsPrimary = true
        fmt.Printf("[Nodo %d] Soy el primario inicial\n", cfg.ID)
        common.StartHeartbeatSender(cfg.ID, cfg.Peers)
        common.AnnounceCoordinator(cfg.ID, cfg.Peers) 
    } else {
        // CORRECCIÓN CLAVE: common.RequestSync debe devolver el ID del primario (primaryID)
        if syncedState, primaryID, err := common.RequestSync(cfg.ID, cfg.Peers); err == nil {
            GlobalNode.StateMutex.Lock()
            GlobalNode.State = syncedState 
            GlobalNode.PrimaryID = primaryID // <-- 1. Establece el Primario después de la sincronización
            GlobalNode.StateMutex.Unlock()
            
            // 2. Reinicia el contador de latidos del monitor inmediatamente
            common.UpdateLastHeartbeatAtomic() 
            
            saveState(GlobalNode.StateFile, GlobalNode.State)
            fmt.Printf("[Nodo %d] 🔁 Estado sincronizado correctamente con Primario %d. Última Seq: %d\n", cfg.ID, primaryID, syncedState.SequenceNumber)
        } else {
            fmt.Printf("[Nodo %d] ⚠️ No se pudo sincronizar con primario. Iniciando desde estado local.\n", cfg.ID)
            // Si la sincronización falla, el PrimaryID permanece en -1, lo que dispara la elección
        }
    }
    
    
    http.HandleFunc("/client_request", HandleClientRequest)
    http.HandleFunc("/sync", HandleSyncRequest)
    
    go http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port+1000), nil) 

    common.StartHeartbeatMonitor(
        cfg.ID,
        cfg.Peers,
        GlobalNode.getPrimaryID, 
        func() { 
            common.StartElection(cfg.ID, cfg.Peers, cfg.Port, func(newLeader int) {

                GlobalNode.setPrimaryID(newLeader) 
                
                if GlobalNode.IsPrimary { 
                    fmt.Printf("[Nodo %d] He sido elegido como nuevo primario\n", cfg.ID)
                    common.StartHeartbeatSender(cfg.ID, cfg.Peers)

                    common.AnnounceCoordinator(cfg.ID, cfg.Peers) 
                }
            })
        },
        GlobalNode.setPrimaryID, 
        common.HandleElectionRequest, 
    )

    select {} 
}


func HandleClientRequest(w http.ResponseWriter, r *http.Request) {
	var req common.ClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	GlobalNode.StateMutex.RLock()
	isPrimary := GlobalNode.IsPrimary
	currentPrimaryID := GlobalNode.PrimaryID
	GlobalNode.StateMutex.RUnlock()


	if !isPrimary {
		resp := common.ClientResponse{
			IsPrimary: false,
			PrimaryID: currentPrimaryID,
			Success:   false,
			Error:     fmt.Sprintf("Redirección: No soy el primario. Primario actual: %d", currentPrimaryID),
		}
		w.WriteHeader(http.StatusTemporaryRedirect)
		json.NewEncoder(w).Encode(resp)
		return
	}


	var resp common.ClientResponse
	
	if req.Type == common.OpReadInventory {

		GlobalNode.StateMutex.RLock()
		inventoryData := GlobalNode.State.Inventory
		GlobalNode.StateMutex.RUnlock()
		
		resp = common.ClientResponse{
			IsPrimary: true,
			Success:   true,
			Inventory: inventoryData,
		}
	} else if req.Type == common.OpSetQuantity || req.Type == common.OpSetPrice {

		GlobalNode.NodeWriteOperation(req)
		
		resp = common.ClientResponse{
			IsPrimary: true, 
			Success:   true, 
			SeqNumber: GlobalNode.State.SequenceNumber,
		}
	} else {
		resp = common.ClientResponse{IsPrimary: true, Success: false, Error: "Operación de cliente no reconocida."}
	}
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}


func HandleSyncRequest(w http.ResponseWriter, r *http.Request) {

	if r.URL.Query().Get("type") == "full" {
		GlobalNode.StateMutex.RLock()
		defer GlobalNode.StateMutex.RUnlock()
		
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(GlobalNode.State)
		return
	}
	

	if r.URL.Query().Get("type") == "event" {
		var event common.EventLog
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "Invalid event format", http.StatusBadRequest)
			return
		}
		
		GlobalNode.StateMutex.Lock()
		defer GlobalNode.StateMutex.Unlock()

		if event.Seq == GlobalNode.State.SequenceNumber + 1 {
			GlobalNode.applyInventoryChange(event)
			GlobalNode.State.EventLog = append(GlobalNode.State.EventLog, event)
			GlobalNode.State.SequenceNumber = event.Seq
			saveState(GlobalNode.StateFile, GlobalNode.State)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Event Applied"))
			return
		}

		http.Error(w, "Sequence mismatch", http.StatusConflict)
		return
	}
	
	http.Error(w, "Invalid sync request type", http.StatusBadRequest)
}



func (n *Node) NodeWriteOperation(req common.ClientRequest) {
	n.StateMutex.Lock()
	defer n.StateMutex.Unlock()
	

	n.State.SequenceNumber++
	newEvent := common.EventLog{
		Seq:   n.State.SequenceNumber,
		Op:    string(req.Type),
		Item:  req.ItemName,
		Value: req.NewValue,
	}

	n.applyInventoryChange(newEvent)

	n.State.EventLog = append(n.State.EventLog, newEvent)
	saveState(n.StateFile, n.State)
	

	n.replicateEvent(newEvent)
}

func (n *Node) applyInventoryChange(event common.EventLog) {
    item, exists := n.State.Inventory[event.Item]

    if !exists {
        item = common.Item{
            Quantity: 0, 
            Price: 0,
        }
    }

	if common.MessageType(event.Op) == common.OpSetQuantity {
		item.Quantity = event.Value
	} 
	if common.MessageType(event.Op) == common.OpSetPrice {
		item.Price = event.Value
	}
	

	n.State.Inventory[event.Item] = item 

}

func (n *Node) replicateEvent(event common.EventLog) {

	cfg := loadConfig(ConfigFile)

	for _, peer := range cfg.Peers {
		if peer.ID != n.ID {
			go func(p common.Peer) {
	
				url := fmt.Sprintf("http://%s:%d/sync?type=event", p.Host, p.Port+1000)

				data, _ := json.Marshal(event)

				resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
				if err != nil {
					fmt.Printf("[Nodo %d] Error replicando a %d: %v\n", n.ID, p.ID, err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					fmt.Printf("[Nodo %d] Error de secuencia/réplica en %d: %s\n", n.ID, p.ID, resp.Status)
				}
			}(peer)
		}
	}
}


func loadConfig(filename string) *common.Config {
	data, err := os.ReadFile(filename)
	if err != nil {

		panic(err)
	}
	var cfg common.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
	
		panic(err)
	}
	

	_, portStr, err := net.SplitHostPort(cfg.LocalAddress)
	if err != nil {
		panic(fmt.Errorf("error al parsear LocalAddress %s: %w", cfg.LocalAddress, err))
	}
	
	portInt, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Errorf("error al convertir puerto a entero: %w", err))
	}
	
	cfg.Port = portInt

	for i := range cfg.Peers {
		peer := &cfg.Peers[i]
		
		host, portStr, err := net.SplitHostPort(peer.Address)
		if err != nil {
			panic(fmt.Errorf("error al parsear dirección de peer %s: %w", peer.Address, err))
		}

		portInt, err := strconv.Atoi(portStr)
		if err != nil {
			panic(fmt.Errorf("error al convertir puerto de peer %s a entero: %w", portStr, err))
		}

		peer.Host = host
		peer.Port = portInt
	}
	
	return &cfg
}

func initState(stateFile string, id int) *common.NodeState {

	data, err := os.ReadFile(stateFile)
	if err == nil {
		var state common.NodeState
		if err := json.Unmarshal(data, &state); err == nil {
			fmt.Printf("[Nodo %d] Estado cargado exitosamente desde %s (Seq: %d)\n", id, stateFile, state.SequenceNumber)
			return &state
		}
	}
	

	fmt.Printf("[Nodo %d] No se pudo cargar el estado persistente. Inicializando inventario predefinido.\n", id)
	
	return &common.NodeState{
		SequenceNumber: 0,
		Inventory: map[string]common.Item{
			"LAPICES":      {Quantity: 100, Price: 120},
			"LIBROS":       {Quantity: 50, Price: 15500},
			"CUADERNOS":    {Quantity: 80, Price: 3500},
			"CALCULADORAS": {Quantity: 20, Price: 25000},
		},
		EventLog: []common.EventLog{},
	}
}
//a
func (n *Node) getPrimaryID() int {
	n.StateMutex.RLock()
	defer n.StateMutex.RUnlock()
	return n.PrimaryID
}

func (n *Node) setPrimaryID(id int) {
	n.StateMutex.Lock()
	defer n.StateMutex.Unlock()
	n.PrimaryID = id
	n.IsPrimary = (n.ID == id)
}

func saveState(filename string, state *common.NodeState) {

    if err := common.SaveState(filename, state); err != nil {
        fmt.Printf("[Nodo %d] Error guardando estado en %s: %v\n", GlobalNode.ID, filename, err)
    } else {
        fmt.Printf("[Nodo %d] Estado guardado correctamente en %s\n", GlobalNode.ID, filename)
    }
}