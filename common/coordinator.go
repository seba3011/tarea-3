// coordinator.go
package common

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const ElectionTimeout = 3 * time.Second

func StartElection(id int, peers []Peer, port int, onNewLeader func(int)) {
	fmt.Printf("[Nodo %d] \u26a0\ufe0f Iniciando elecci\u00f3n\n", id)
	responses := make(chan bool)

	higherPeers := []Peer{}
	for _, peer := range peers {
		if peer.ID > id {
			higherPeers = append(higherPeers, peer)
		}
	}

	for _, peer := range higherPeers {
		go func(p Peer) {
			msg := Message{Type: MsgElection, SenderID: id, Time: time.Now()}
			if sendMessage(p.Host, p.Port, msg) {
				responses <- true
			}
		}(peer)
	}

	timeout := time.After(ElectionTimeout)
	receivedOK := false
waiting:
	for {
		select {
		case <-responses:
			fmt.Printf("[Nodo %d] ✅ Recibido OK de nodo mayor\n", id)
			receivedOK = true
			break waiting
		case <-timeout:
			break waiting
		}
	}

	if receivedOK {
		fmt.Printf("[Nodo %d] 🕓 Esperando coordinador\n", id)
	} else {
		fmt.Printf("[Nodo %d] 👑 Me proclamo coordinador\n", id)
		announceCoordinator(id, peers)
		onNewLeader(id)
	}
}

func HandleElectionRequest(myID int, senderHost string, senderPort int) {
	fmt.Printf("[Nodo %d] ↩️ Respondiendo a ELECTION\n", myID)
	okMsg := Message{Type: MsgOK, SenderID: myID, Time: time.Now()}
	sendMessage(senderHost, senderPort, okMsg)
}

func announceCoordinator(id int, peers []Peer) {
	msg := Message{Type: MsgCoordinator, SenderID: id, Time: time.Now()}
	for _, peer := range peers {
		go sendMessage(peer.Host, peer.Port, msg)
	}
}

func sendMessage(host string, port int, msg Message) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 1*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	data, _ := json.Marshal(msg)
	_, err = conn.Write(data)
	return err == nil
}
