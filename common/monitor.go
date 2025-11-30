package common

import (
	"encoding/json"
	"fmt"
	"net" 
	"strconv" 
	"time"
)

const (
	HeartbeatInterval = 2 * time.Second
	HeartbeatTimeout = 5 * time.Second
)
var currentHeartbeatStop *chan struct{}
func findLocalPeerInfo(myID int, peers []Peer) (Peer, error) {
	for _, peer := range peers {
		if peer.ID == myID {
			return peer, nil
		}
	}
	return Peer{}, fmt.Errorf("no se encontró información de puerto para el Nodo ID %d en la lista de peers", myID)
}


func StartHeartbeatSender(myID int, peers []Peer) {
    // 1. Crear el nuevo canal de detención para esta instancia.
    newStopCh := make(chan struct{})
    
    // 2. Intentar detener la instancia anterior y reemplazar el puntero.
    oldStopChPtr := currentHeartbeatStop
    
    // Reemplazamos la variable global por el puntero al nuevo canal.
    currentHeartbeatStop = &newStopCh 
    
    // 3. Detener la instancia anterior (si existe).
    if oldStopChPtr != nil {
        oldStopCh := *oldStopChPtr 
        
        // Usamos select con default para verificar si el canal ya está cerrado.
        select {
        case <-oldStopCh:
            // El canal ya estaba cerrado (la goroutine se detuvo).
        default:
            // El canal está abierto, lo cerramos para detener la goroutine anterior.
            close(oldStopCh)
            fmt.Printf("[Nodo %d] 🛑 Deteniendo Heartbeat Sender anterior.\n", myID)
        }
    }
    
    // 4. Iniciar el nuevo loop de Heartbeat
    ticker := time.NewTicker(HeartbeatInterval)
    
    // Pasamos el nuevo canal (newStopCh) como argumento al goroutine.
    go func(stopCh chan struct{}) { 
        fmt.Printf("[Nodo %d] ❤️ Heartbeat Sender iniciado.\n", myID)
        defer ticker.Stop() 

        for {
            select {
            case <-ticker.C:
                // El intervalo ha transcurrido, envía el latido.
                msg := Message{
                    Type:  MsgHeartbeat,
                    SenderID: myID,
                    Time:   time.Now(),
                }
                for _, peer := range peers {
                    if peer.ID != myID {
                        // **Esta línea generará los logs de actividad constante**
                        fmt.Printf("[Nodo %d] >>> Enviando Heartbeat a %d\n", myID, peer.ID) 
                        go sendMessage(peer.Host, peer.Port, msg)
                    }
                }
            case <-stopCh:
                // Se recibió la señal de detención (el canal fue cerrado)
                fmt.Printf("[Nodo %d] ❌ Heartbeat Sender detenido exitosamente.\n", myID)
                return // Sale del loop y finaliza la goroutine
            }
        }
    }(newStopCh)
}
func StartHeartbeatMonitor(myID int, peers []Peer, getPrimaryID func() int, startElection func(), setPrimaryID func(int), handleElectionRequest func(int, string, int)) {
	var lastHeartbeat = time.Now()

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

			if time.Since(lastHeartbeat) > HeartbeatTimeout {
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
				lastHeartbeat = time.Now()
				
			case MsgCoordinator:
				if msg.SenderID == myID {
					return
				}
				
				setPrimaryID(msg.SenderID) 
				fmt.Printf("[Nodo %d] Recibido COORDINATOR. Nuevo Primario: %d. Fin de espera.\n", myID, msg.SenderID)
				
			case MsgElection:
				if myID > msg.SenderID {
					handleElectionRequest(myID, host, port) 

					if getPrimaryID() != myID {
						fmt.Printf("[Nodo %d] Soy mayor, pero no primario. Inicio mi propia elección.\n", myID)
						startElection() 
					} else {
						fmt.Printf("[Nodo %d] Primario activo, respondo OK y reafirmo liderazgo a %d.\n", myID, msg.SenderID)
						AnnounceCoordinator(myID, peers) 
					}
				}
			}
		}(conn)
	}
}