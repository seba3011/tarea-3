package common

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const (
	HeartbeatInterval = 2 * time.Second
	HeartbeatTimeout  = 5 * time.Second
)

// Asumo que esta función existe en el paquete common
func sendMessage(host string, port int, msg Message) {
	// Implementación real (omisión)
}

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

func StartHeartbeatMonitor(myID int, peers []Peer, getPrimaryID func() int, startElection func()) {
	var lastHeartbeat = time.Now()
    
	go func() {
		for {
			time.Sleep(1 * time.Second)
			primaryID := getPrimaryID()
            
			if primaryID <= 0 || primaryID == myID {
				if primaryID != myID {
					fmt.Printf("[Nodo %d] Primario desconocido (ID %d). Iniciando Elección.\n", myID, primaryID)
					startElection()
				}
				continue
			}
            
			if time.Since(lastHeartbeat) > HeartbeatTimeout {
				fmt.Printf("[Nodo %d] 💀 No se ha recibido heartbeat del Primario (%d). Iniciando elección\n", myID, primaryID)
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
			json.NewDecoder(c).Decode(&msg)

			if msg.Type == MsgHeartbeat {
				lastHeartbeat = time.Now() 
			}
		}(conn)
	}
}