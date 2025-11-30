package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const SyncEndpoint = "/sync"

func RequestSync(myID int, peers []Peer) (*NodeState, error) {
	for _, peer := range peers {
		url := fmt.Sprintf("http://%s:%d%s?type=full", peer.Host, peer.Port+1000, SyncEndpoint)

		client := http.Client{Timeout: 1 * time.Second}
		resp, err := client.Post(url, "application/json", bytes.NewBuffer([]byte{}))
		if err != nil || resp.StatusCode != 200 {
			continue
		}

		defer resp.Body.Close()
		var state NodeState
		if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
			return nil, err
		}

		fmt.Printf("[Nodo %d] Estado recibido del primario (ID %d)\n", myID, peer.ID)
		return &state, nil
	}
	return nil, fmt.Errorf("no se pudo contactar a ningún primario")
}
