package common

import (
    "encoding/json"
    "fmt"
    "net" 
    "strconv" 
    "time"
    "sync/atomic" // Necesario para manipular lastHeartbeat de forma atómica
)

const (
    HeartbeatInterval = 2 * time.Second
    HeartbeatTimeout = 5 * time.Second
)

// currentHeartbeatStop sigue el patrón de puntero a canal para detener el HeartbeatSender sin Mutex
var currentHeartbeatStop *chan struct{}

// lastHeartbeat se almacena como un valor int64 (nanosegundos) para ser manipulado atómicamente.
var lastHeartbeat int64


// UpdateLastHeartbeatAtomic: FUNCIÓN PÚBLICA (EXPORTADA) para actualizar el tiempo del último latido.
func UpdateLastHeartbeatAtomic() {
    atomic.StoreInt64(&lastHeartbeat, time.Now().UnixNano())
}

// ReadLastHeartbeatAtomic: FUNCIÓN PÚBLICA (EXPORTADA) para leer el tiempo de forma atómica.
func ReadLastHeartbeatAtomic() time.Time {
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
    // Inicialización del contador atómico (con Mayúscula)
    UpdateLastHeartbeatAtomic()

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

            // USO DE LA FUNCIÓN CON MAYÚSCULA
            if time.Since(ReadLastHeartbeatAtomic()) > HeartbeatTimeout {
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
                // USO DE LA FUNCIÓN CON MAYÚSCULA
                UpdateLastHeartbeatAtomic()
                
            case MsgCoordinator:
                if msg.SenderID == myID {
                    return
                }
                
                setPrimaryID(msg.SenderID) 
                // USO DE LA FUNCIÓN CON MAYÚSCULA
                UpdateLastHeartbeatAtomic() 
                fmt.Printf("[Nodo %d] Recibido COORDINATOR. Nuevo Primario: %d. Fin de espera.\n", myID, msg.SenderID)
                
            case MsgElection:
                if myID > msg.SenderID {
                    handleElectionRequest(myID, host, port) 

                    if getPrimaryID() == myID { 
                        fmt.Printf("[Nodo %d] Primario activo, respondo OK y reafirmo liderazgo a %d.\n", myID, msg.SenderID)
                        AnnounceCoordinator(myID, peers) 
                    }
                }
            }
        }(conn)
    }
}