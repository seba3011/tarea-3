package common

import (
    "encoding/json"
    "fmt"
    "net" 
    "strconv" 
    "time"
    "sync/atomic"
)

const (
    HeartbeatInterval = 2 * time.Second
    HeartbeatTimeout = 5 * time.Second
)

var currentHeartbeatStop *chan struct{}
var lastHeartbeat int64


// Funciones atómicas EXPORTADAS (Mayúscula)
func UpdateLastHeartbeatAtomic() {
    atomic.StoreInt64(&lastHeartbeat, time.Now().UnixNano())
}

func ReadLastHeartbeatAtomic() time.Time {
    nano := atomic.LoadInt64(&lastHeartbeat)
    return time.Unix(0, nano)
}
// FIN DE FUNCIONES ATÓMICAS


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
                        // Aquí es donde se envía el latido (a Peer.Port)
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
    // Inicialización del contador atómico
    UpdateLastHeartbeatAtomic()

    go func() {
        for {
            time.Sleep(1 * time.Second)
            primaryID := getPrimaryID()
            
            // Si soy el primario, no monitoreo
            if primaryID == myID {
                continue 
            }
            
            // Si el primario es desconocido (PrimaryID = -1 después de un sync fallido)
            if primaryID <= 0 {
                // Si PrimaryID == -1, se dispara la elección
                if primaryID != myID { 
                    fmt.Printf("[Nodo %d] Primario desconocido (ID %d). Iniciando Elección.\n", myID, primaryID)
                    startElection()
                }
                continue
            }

            // LÓGICA CRÍTICA DE TIMEOUT:
            if time.Since(ReadLastHeartbeatAtomic()) > HeartbeatTimeout {
                fmt.Printf("[Nodo %d] No se ha recibido heartbeat del Primario (%d) en más de %s. Iniciando elección.\n", 
                    myID, primaryID, HeartbeatTimeout.String())
                startElection()
            }
        }
    }()

    localInfo, err := findLocalPeerInfo(myID, peers)
    if err != nil {
        fmt.Printf("Error de configuración: %v\n", err)
        return 
    }
    
    // El listener de Heartbeats usa Peer.Port
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
                UpdateLastHeartbeatAtomic() // Actualiza el tiempo
                
            case MsgCoordinator:
                if msg.SenderID == myID {
                    return
                }
                
                setPrimaryID(msg.SenderID) 
                UpdateLastHeartbeatAtomic() // Actualiza el tiempo al aceptar un nuevo líder
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