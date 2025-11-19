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
	HeartbeatTimeout  = 5 * time.Second
)

// Asumo que sendMessage, Message, Peer, Msg... existen.

// ----------------------------------------------------------------------------------
// Funciones Auxiliares
// ----------------------------------------------------------------------------------

func findLocalPeerInfo(myID int, peers []Peer) (Peer, error) {
	for _, peer := range peers {
		if peer.ID == myID {
			return peer, nil
		}
	}
	return Peer{}, fmt.Errorf("no se encontró información de puerto para el Nodo ID %d en la lista de peers", myID)
}

// ----------------------------------------------------------------------------------
// Lógica de Heartbeat
// ----------------------------------------------------------------------------------

func StartHeartbeatSender(myID int, peers []Peer) {
	ticker := time.NewTicker(HeartbeatInterval)
	go func() {
		for range ticker.C {
			msg := Message{
				Type: 	  MsgHeartbeat,
				SenderID: myID,
				Time: 	  time.Now(),
			}
			for _, peer := range peers {
				if peer.ID != myID {
					go sendMessage(peer.Host, peer.Port, msg)
				}
			}
		}
	}()
}

func StartHeartbeatMonitor(myID int, peers []Peer, getPrimaryID func() int, startElection func(), setPrimaryID func(int), handleElectionRequest func(int, string, int)) {
	var lastHeartbeat = time.Now()
	
	// Goroutine 1: Monitoreo de Timeout
	go func() {
		for {
			time.Sleep(1 * time.Second)
			primaryID := getPrimaryID()
			
			// 💡 Corrección 1: Si yo soy el Primario, no necesito monitorear ni iniciar elección.
			if primaryID == myID {
				continue 
			}
			
			if primaryID <= 0 { // Primario desconocido (es decir, PrimaryID = -1)
				if primaryID != myID { // Solo si no nos hemos proclamado nosotros mismos
					fmt.Printf("[Nodo %d] Primario desconocido (ID %d). Iniciando Elección.\n", myID, primaryID)
					startElection()
				}
				continue
			}
			
			// Si hay un Primario conocido (primaryID > 0) que no soy yo, monitoreo su heartbeat
			if time.Since(lastHeartbeat) > HeartbeatTimeout {
				fmt.Printf("[Nodo %d] 💀 No se ha recibido heartbeat del Primario (%d). Iniciando elección\n", myID, primaryID)
				startElection()
			}
		}
	}()

	// Goroutine 2: Listener de mensajes entrantes
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
			
			// 💡 Corrección del error 'declared and not used'
			host, portStr, _ := net.SplitHostPort(c.RemoteAddr().String())
			port, _ := strconv.Atoi(portStr)
			
			// Manejo de los 3 tipos de mensajes clave
			switch msg.Type {
			case MsgHeartbeat:
				lastHeartbeat = time.Now()
                
			case MsgCoordinator:
				// 💡 Corrección 2: Ignorar el propio mensaje COORDINATOR si se envía a sí mismo
				if msg.SenderID == myID {
					return
				}
				
				setPrimaryID(msg.SenderID) 
				fmt.Printf("[Nodo %d] 🤝 Recibido COORDINATOR. Nuevo Primario: %d. Fin de espera.\n", myID, msg.SenderID)
                
			case MsgElection:
				if myID > msg.SenderID {
					// Respondo OK al nodo de menor ID
					handleElectionRequest(myID, host, port) 
					
					// 💡 Corrección 3: Solo inicio elección si NO soy el Primario
					if getPrimaryID() != myID {
						fmt.Printf("[Nodo %d] 👑 Soy mayor, pero no primario. Inicio mi propia elección.\n", myID)
						startElection() 
					} else {
						fmt.Printf("[Nodo %d] 🟢 Primario activo, respondo OK a %d.\n", myID, msg.SenderID)
					}
				}
			}
		}(conn)
	}
}