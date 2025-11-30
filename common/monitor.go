package common

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	HeartbeatInterval = 2 * time.Second
	HeartbeatTimeout  = 5 * time.Second
)

func findLocalPeerInfo(myID int, peers []Peer) (Peer, error) {
	for _, peer := range peers {
		if peer.ID == myID {
			return peer, nil
		}
	}
	return Peer{}, fmt.Errorf("no se encontró información de puerto para el Nodo ID %d en la lista de peers", myID)
}

func StartHeartbeatSender(myID int, peers []Peer) {
	ticker := time.NewTicker(HeartbeatInterval)
	go func() {
		for range ticker.C {
			msg := Message{
				Type:     MsgHeartbeat,
				SenderID: myID,
				Time:     time.Now(),
			}
			for _, peer := range peers {
				if peer.ID != myID {
					go sendMessage(peer.Host, peer.Port, msg)
				}
			}
		}
	}()
}

func StartHeartbeatMonitor(
	myID int,
	peers []Peer,
	getPrimaryID func() int,
	startElection func(),
	setPrimaryID func(int),
	handleElectionRequest func(int, string, int),
) {
	var lastHeartbeat time.Time = time.Now()
	var mu sync.Mutex

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

			mu.Lock()
			elapsed := time.Since(lastHeartbeat)
			mu.Unlock()

			if elapsed > HeartbeatTimeout {
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
				primaryID := getPrimaryID()
				if msg.SenderID == primaryID {
					mu.Lock()
					lastHeartbeat = time.Now()
					mu.Unlock()
				}

			case MsgCoordinator:
				if msg.SenderID == myID {
					return
				}

				setPrimaryID(msg.SenderID)

				mu.Lock()
				lastHeartbeat = time.Now()
				mu.Unlock()

				fmt.Printf("[Nodo %d] Recibido COORDINATOR. Nuevo Primario: %d. Fin de espera.\n",
					myID, msg.SenderID)

			case MsgElection:
				if myID > msg.SenderID {
					handleElectionRequest(myID, host, port)

					if getPrimaryID() != myID {
						fmt.Printf("[Nodo %d] Soy mayor, pero no primario. Inicio mi propia elección.\n",
							myID)
						startElection()
					} else {
						fmt.Printf("[Nodo %d] Primario activo, respondo OK y reafirmo liderazgo a %d.\n",
							myID, msg.SenderID)
						AnnounceCoordinator(myID, peers)
					}
				}
			}
		}(conn)
	}
}
