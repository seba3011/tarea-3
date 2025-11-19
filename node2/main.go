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
)

// Estructura para contener el estado del nodo en memoria, incluyendo la lógica de coordinación.
// Esta reemplaza la necesidad de pasar el "state" del archivo persistente por todas partes,
// permitiendo que los mutex protejan el acceso.
type Node struct {
	ID          int
	IsPrimary   bool
	PrimaryID   int
	StateFile   string
	
	// Estado replicado (incluye SequenceNumber, Inventory, EventLog)
	State       *common.NodeState 
	StateMutex  sync.RWMutex
}

var GlobalNode *Node // Usamos una variable global para acceder al nodo desde los Handlers.

func main() {
	cfg := loadConfig(ConfigFile)
	
	// Inicializar el objeto Node
	GlobalNode = &Node{
		ID:  cfg.ID,
		StateFile:  fmt.Sprintf("estado_node%d.json", cfg.ID),
		IsPrimary:  cfg.IsPrimary,
		PrimaryID:  -1, // Inicialmente desconocido
		StateMutex: sync.RWMutex{},
	}
	GlobalNode.State = initState(GlobalNode.StateFile, GlobalNode.ID)
	
	// Si el nodo arranca como Primario
	if cfg.IsPrimary {
		GlobalNode.PrimaryID = cfg.ID
		GlobalNode.IsPrimary = true
		fmt.Printf("[Nodo %d] 🟢 Soy el primario inicial\n", cfg.ID)
		common.StartHeartbeatSender(cfg.ID, cfg.Peers)
	} else {
		// Intentar sincronización si me reintegro
		if syncedState, err := common.RequestSync(cfg.ID, cfg.Peers); err == nil {
			// Sobrescribe el estado persistente con el estado sincronizado
			GlobalNode.StateMutex.Lock()
			GlobalNode.State = syncedState 
			GlobalNode.StateMutex.Unlock()
			saveState(GlobalNode.StateFile, GlobalNode.State)
			fmt.Printf("[Nodo %d] 🔁 Estado sincronizado correctamente. Última Seq: %d\n", cfg.ID, syncedState.SequenceNumber)
		} else {
			fmt.Printf("[Nodo %d] ⚠️ No se pudo sincronizar con primario. Iniciando desde estado local.\n", cfg.ID)
		}
	}
	
	// ----------------------------------------------------
	// Inicialización de Servidores
	// ----------------------------------------------------
	
	http.HandleFunc("/client_request", HandleClientRequest)
	http.HandleFunc("/sync", HandleSyncRequest)
	
	go http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port+1000), nil) // Puerto de control/datos
	
	// ----------------------------------------------------
	// Lógica de Coordinación
	// ----------------------------------------------------
	
	// Monitoreo del primario
	// 💡 ¡CORRECCIÓN CRÍTICA! Se pasan los dos argumentos faltantes.
	common.StartHeartbeatMonitor(
		cfg.ID,
		cfg.Peers,
		GlobalNode.getPrimaryID, // 1. getPrimaryID
		func() { // 2. OnFailure (activar elección)
			common.StartElection(cfg.ID, cfg.Peers, cfg.Port, func(newLeader int) {
				GlobalNode.setPrimaryID(newLeader) // Usamos el nuevo método setPrimaryID
				
				if GlobalNode.IsPrimary { // Se actualiza después de setPrimaryID
					fmt.Printf("[Nodo %d] 👑 He sido elegido como nuevo primario\n", cfg.ID)
					common.StartHeartbeatSender(cfg.ID, cfg.Peers)
				}
			})
		},
		GlobalNode.setPrimaryID, // 3. setPrimaryID (Callback para MsgCoordinator)
		common.HandleElectionRequest, // 4. HandleElectionRequest (Callback para MsgElection)
	)

	select {} // Mantiene proceso corriendo
}

// -------------------------
// Handlers de Peticiones
// -------------------------

// HandleClientRequest procesa las solicitudes de inventario (lectura/escritura) del cliente.
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

	// 1. Lógica de Redirección (Si no es primario)
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

	// 2. Lógica del Primario (Solo el primario procesa y replica)
	var resp common.ClientResponse
	
	if req.Type == common.OpReadInventory {
		// Operación de LECTURA (no requiere lock de escritura ni replicación)
		GlobalNode.StateMutex.RLock()
		inventoryData := GlobalNode.State.Inventory
		GlobalNode.StateMutex.RUnlock()
		
		resp = common.ClientResponse{
			IsPrimary: true,
			Success:   true,
			Inventory: inventoryData,
		}
	} else if req.Type == common.OpSetQuantity || req.Type == common.OpSetPrice {
		// Operación de ESCRITURA (requiere lock de escritura y replicación)
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

// HandleSyncRequest se usa para la sincronización inicial y la replicación de eventos
func HandleSyncRequest(w http.ResponseWriter, r *http.Request) {
	// ⚠️ DEBES IMPLEMENTAR LA LÓGICA DE common.HandleSyncRequest y común.HandleEvent
	// Usando GlobalNode.State y GlobalNode.StateMutex
	// Por simplicidad, se deja la lógica de la Tarea 2025-1 (pero debe ser ajustada)
	// Ejemplo:
	// common.HandleSyncRequest(w, r, GlobalNode.State, &GlobalNode.StateMutex)
	
	// Si la petición es para el estado completo (sincronización inicial)
	if r.URL.Query().Get("type") == "full" {
		GlobalNode.StateMutex.RLock()
		defer GlobalNode.StateMutex.RUnlock()
		
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(GlobalNode.State)
		return
	}
	
	// Si la petición es un evento replicado (MsgEvent)
	if r.URL.Query().Get("type") == "event" {
		var event common.EventLog
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "Invalid event format", http.StatusBadRequest)
			return
		}
		
		GlobalNode.StateMutex.Lock()
		defer GlobalNode.StateMutex.Unlock()
		
		// Aplicar lógica de Secundario para eventos replicados (verificar secuencia, aplicar, guardar)
		if event.Seq == GlobalNode.State.SequenceNumber + 1 {
			GlobalNode.applyInventoryChange(event)
			GlobalNode.State.EventLog = append(GlobalNode.State.EventLog, event)
			GlobalNode.State.SequenceNumber = event.Seq
			saveState(GlobalNode.StateFile, GlobalNode.State)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Event Applied"))
			return
		}
		// Lógica de descarte o re-sincronización si la secuencia es incorrecta
		http.Error(w, "Sequence mismatch", http.StatusConflict)
		return
	}
	
	http.Error(w, "Invalid sync request type", http.StatusBadRequest)
}


// -------------------------
// Lógica de Operaciones de Inventario
// -------------------------

// NodeWriteOperation es llamada por el Primario para procesar la escritura del cliente, 
// aplicar el cambio localmente, persistir y replicar.
func (n *Node) NodeWriteOperation(req common.ClientRequest) {
	n.StateMutex.Lock()
	defer n.StateMutex.Unlock()
	
	// Asignar nuevo número de secuencia y crear EventLog
	n.State.SequenceNumber++
	newEvent := common.EventLog{
		Seq:   n.State.SequenceNumber,
		Op:    string(req.Type),
		Item:  req.ItemName,
		Value: req.NewValue,
	}
	
	// Aplicar el cambio al estado local
	n.applyInventoryChange(newEvent)

	// Añadir al EventLog local y guardar estado
	n.State.EventLog = append(n.State.EventLog, newEvent)
	saveState(n.StateFile, n.State)
	
	// Replicar el evento a los secundarios
	n.replicateEvent(newEvent)
}

// applyInventoryChange aplica el evento de log al estado del inventario.
// DEBE ser llamada dentro de un Lock/R-Lock.
func (n *Node) applyInventoryChange(event common.EventLog) {
	if item, exists := n.State.Inventory[event.Item]; exists {
		if common.MessageType(event.Op) == common.OpSetQuantity {
			item.Quantity = event.Value
		} 
		if common.MessageType(event.Op) == common.OpSetPrice {
			item.Price = event.Value
		}
		n.State.Inventory[event.Item] = item 
	}
}

func (n *Node) replicateEvent(event common.EventLog) {
	// Esta es una función PLACEHOLDER.

	// El primario tiene que tener acceso a la configuración
	// Nota: Es más eficiente usar la configuración global si está disponible, 
	// pero aquí respetamos la llamada a loadConfig(ConfigFile) de su código.
	cfg := loadConfig(ConfigFile)

	for _, peer := range cfg.Peers {
		if peer.ID != n.ID {
			go func(p common.Peer) {
				// Uso de p.Host y p.Port asumiendo que están definidos en common.Peer
				url := fmt.Sprintf("http://%s:%d/sync?type=event", p.Host, p.Port+1000)

				data, _ := json.Marshal(event)

				// 💡 LÍNEA CORREGIDA: Se usa bytes.NewBuffer para el cuerpo de la petición.
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

// -------------------------
// Utilidades auxiliares
// -------------------------

func loadConfig(filename string) *common.Config {
	data, err := os.ReadFile(filename)
	if err != nil {
		// Error al encontrar o leer el archivo config.json (ej: ruta incorrecta)
		panic(err)
	}
	var cfg common.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Error al parsear el JSON (ej: etiquetas incorrectas en el struct common.Config)
		panic(err)
	}
	
	// 1. Extraer y asignar el Port del nodo local (LocalAddress)
	_, portStr, err := net.SplitHostPort(cfg.LocalAddress)
	if err != nil {
		panic(fmt.Errorf("error al parsear LocalAddress %s: %w", cfg.LocalAddress, err))
	}
	
	portInt, err := strconv.Atoi(portStr)
	if err != nil {
		panic(fmt.Errorf("error al convertir puerto a entero: %w", err))
	}
	
	cfg.Port = portInt
	
	// 2. Iterar sobre todos los peers conocidos y extraer Host/Port de Address
	for i := range cfg.Peers {
		peer := &cfg.Peers[i] // Obtener referencia
		
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

// initState inicializa o carga el estado del nodo.
func initState(stateFile string, id int) *common.NodeState {
	state, err := common.LoadState(stateFile)
	if err != nil {
		panic(err)
	}
	
	// Aquí ya no se necesita crear un estado inicial con ID, IsPrimary, etc.
	// Si LoadState encontró un error IsNotExist, ya devolvió un NodeState con inventario vacío.
	
	return state
}

func saveState(filename string, state *common.NodeState) {
	if err := common.SaveState(filename, state); err != nil {
		fmt.Printf("Error guardando estado: %v\n", err)
	}
}
func (n *Node) getPrimaryID() int {
	n.StateMutex.RLock()
	defer n.StateMutex.RUnlock()
	return n.PrimaryID
}

// Método para actualizar el PrimaryID de forma segura
func (n *Node) setPrimaryID(id int) {
	n.StateMutex.Lock()
	defer n.StateMutex.Unlock()
	n.PrimaryID = id
	n.IsPrimary = (n.ID == id)
}