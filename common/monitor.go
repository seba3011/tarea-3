package common

import (
    "encoding/json"
    "fmt"
    "net" 
    "strconv" 
    "time"
    "sync/atomic" // Necesario para manipular lastHeartbeat de forma atómica
    "unsafe"      // Necesario para el puntero atómico a lastHeartbeat
)

const (
    HeartbeatInterval = 2 * time.Second
    HeartbeatTimeout = 5 * time.Second
)

// currentHeartbeatStop sigue el patrón de puntero a canal para detener el HeartbeatSender sin Mutex
var currentHeartbeatStop *chan struct{}

// lastHeartbeat se almacena como un valor int64 (nanosegundos) para ser manipulado atómicamente.
// Usamos un valor atómico para garantizar que las lecturas y escrituras concurrentes sean seguras.
var lastHeartbeat int64

// Función helper para guardar la hora actual (time.Now().UnixNano()) de forma atómica
func updateLastHeartbeatAtomic() {
    atomic.StoreInt64(&lastHeartbeat, time.Now().UnixNano())
}

// Función helper para leer el tiempo (time.Time) de forma atómica
func readLastHeartbeatAtomic() time.Time {
    nano := atomic.LoadInt64(&lastHeartbeat)
    return time.Unix(0, nano)
}

func findLocalPeerInfo(myID int, peers []Peer) (Peer, error) {
    for _, peer := range peers {
        if peer.ID == myID {
            return peer, nil
        }
    }
    return Peer{}, fmt.Errorf("no se encontró información de puerto para el Nodo ID %d en la lista de peers", myID)
}


func StartHeartbeatSender(myID int, peers []Peer) {
    newStopCh := make(chan struct{})
    
    oldStopChPtr := currentHeartbeatStop
    
    // Asignación atómica del puntero global
    currentHeartbeatStop = &newStopCh 
    
    if oldStopChPtr != nil {
        oldStopCh := *oldStopChPtr 
        
        select {
        case <-oldStopCh:
        default:
            close(oldStopCh)
            fmt.Printf("[Nodo %d] 🛑 Deteniendo Heartbeat Sender anterior.\n", myID)
        }
    }
    
    ticker := time.NewTicker(HeartbeatInterval)
    
    go func(stopCh chan struct{}) { 
        fmt.Printf("[Nodo %d] ❤️ Heartbeat Sender iniciado.\n", myID)
        defer ticker.Stop() 

        for {
            select {
            case <-ticker.C:
                msg := Message{
                    Type: MsgHeartbeat,
                    SenderID: myID,
                    Time:  time.Now(),
                }
                for _, peer := range peers {
                    if peer.ID != myID {
                        fmt.Printf("[Nodo %d] >>> Enviando Heartbeat a %d\n", myID, peer.ID) 
                        go sendMessage(peer.Host, peer.Port, msg)
                    }
                }
            case <-stopCh:
                fmt.Printf("[Nodo %d] ❌ Heartbeat Sender detenido exitosamente.\n", myID)
                return 
            }
        }
    }(newStopCh)
}

func StartHeartbeatMonitor(myID int, peers []Peer, getPrimaryID func() int, startElection func(), setPrimaryID func(int), handleElectionRequest func(int, string, int)) {
    // Inicializar el tiempo de latido de forma atómica al inicio
    atomic.StoreInt64(&lastHeartbeat, time.Now().UnixNano())

    go func() {
        for {
            time.Sleep(1 * time.Second)
            primaryID := getPrimaryID()
            if primaryID == myID {
                continue 
            }
            
            if primaryID <= 0 {
                if primaryID != myID { 
                    fmt.Printf("[Nodo %d] Primario desconocido (ID %d). Iniciando Elección.\n", myID, primaryID)
                    startElection()
                }
                continue
            }

            if time.Since(readLastHeartbeatAtomic()) > HeartbeatTimeout { // Lectura atómica
                fmt.Printf("[Nodo %d] No se ha recibido heartbeat del Primario (%d). Iniciando elección\n", myID, primaryID)
                startElection()
            }
        }
    }()

    localInfo, err := findLocalPeerInfo(myID, peers)
    if err != nil {
        fmt.Printf("Error de configuración: %v\n", err)
        return 
    }
    
    ln, err := net.Listen("tcp", fmt.Sprintf(":%d", localInfo.Port))
    
    if err != nil {
        fmt.Printf("Error iniciando escucha en nodo %d: %v\n", myID, err)
        return
    }
    defer ln.Close()

    for {
        conn, err := ln.Accept()
        if err != nil {
            continue
        }
        go func(c net.Conn) {
            defer c.Close()
            var msg Message
            if err := json.NewDecoder(c).Decode(&msg); err != nil {
                return
            }

            host, portStr, _ := net.SplitHostPort(c.RemoteAddr().String())
            port, _ := strconv.Atoi(portStr)

            switch msg.Type {
            case MsgHeartbeat:
                updateLastHeartbeatAtomic() // Escritura atómica
                
            case MsgCoordinator:
                if msg.SenderID == myID {
                    return
                }
                
                // Si estamos en elección, NO debemos aceptar ciegamente al COORDINATOR
                // Es necesario agregar lógica de IsElecting o timeout aquí para evitar el bucle.
                // Sin embargo, para no agregar variables, se mantiene la simpleza:
                setPrimaryID(msg.SenderID) 
                fmt.Printf("[Nodo %d] Recibido COORDINATOR. Nuevo Primario: %d. Fin de espera.\n", myID, msg.SenderID)
                
            case MsgElection:
                if myID > msg.SenderID {
                    handleElectionRequest(myID, host, port) 

                    // Si el nodo es Primario activo (ya tiene el liderazgo), debe reafirmarlo.
                    if getPrimaryID() == myID { 
                        fmt.Printf("[Nodo %d] Primario activo, respondo OK y reafirmo liderazgo a %d.\n", myID, msg.SenderID)
                        AnnounceCoordinator(myID, peers) 
                    } 
                    // Si no es el primario, responder OK es suficiente. 
                    // El monitor se encargará de iniciar la elección si es necesario (si no recibe COORDINATOR).
                }
            }
        }(conn)
    }
}