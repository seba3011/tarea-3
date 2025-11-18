package common

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Intervalos de chequeo
const (
	HeartbeatInterval = 2 * time.Second
	HeartbeatTimeout  = 5 * time.Second
)

// ✅ FUNCIÓN MOVIDA AQUÍ
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
				go sendMessage(peer.Host, peer.Port, msg)
			}
		}
	}()
}

// Monitoreo de heartbeats y detección de caída del primario
func StartHeartbeatMonitor(myID int, peers []Peer, getPrimaryID func() int, startElection func()) {
	lastHeartbeat := time.Now()

	go func() {
		for {
			time.Sleep(1 * time.Second)
			if getPrimaryID() != myID && time.Since(lastHeartbeat) > HeartbeatTimeout {
				fmt.Printf("[Nodo %d] 💀 No se ha recibido heartbeat del primario. Iniciando elección\n", myID)
				startElection()
			}
		}
	}()

	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", peers[myID-1].Port))
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
	}()
}
