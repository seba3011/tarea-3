package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const SyncEndpoint = "/sync"

// CORRECCIÓN: Ahora devuelve (*NodeState, int, error)
func RequestSync(myID int, peers []Peer) (*NodeState, int, error) {
	for _, peer := range peers {
		url := fmt.Sprintf("http://%s:%d%s?type=full", peer.Host, peer.Port+1000, SyncEndpoint)

		client := http.Client{Timeout: 1 * time.Second}
		resp, err := client.Post(url, "application/json", bytes.NewBuffer([]byte{}))
		
		// Verificación de estado HTTP: Se debe usar http.StatusOK (200)
		if err != nil || resp.StatusCode != http.StatusOK { 
			continue
		}

		defer resp.Body.Close()
		var state NodeState
		if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
			// Devolver ID desconocido (-1) si falla la decodificación
			return nil, -1, err
		}

		fmt.Printf("[Nodo %d] 🔁 Estado recibido del primario (ID %d)\n", myID, peer.ID)
		
		// DEVOLVER EL ID DEL PEER QUE RESPONDIÓ
		return &state, peer.ID, nil
	}
	// Devolver ID desconocido (-1) si no se pudo contactar a nadie
	return nil, -1, fmt.Errorf("no se pudo contactar a ningún primario")
}