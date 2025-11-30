package common

import (
	"encoding/json"
	"os"
	"sync"
	"fmt"
)

var mutex sync.Mutex

type Item struct {
	Quantity int `json:"quantity"`
	Price    int `json:"price"`
}

type EventLog struct {
	Seq   int    `json:"seq"`
	Op    string `json:"op"`
	Item  string `json:"item"`
	Value int    `json:"value"`
}

type NodeState struct {
	SequenceNumber int          `json:"sequence_number"`
	Inventory      map[string]Item `json:"inventory"`
	EventLog       []EventLog   `json:"event_log"`
}

func LoadState(filename string) (*NodeState, error) {
	mutex.Lock()
	defer mutex.Unlock()

	file, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &NodeState{
				SequenceNumber: 0,
				Inventory:      make(map[string]Item),
				EventLog:       []EventLog{},
			}, nil
		}
		return nil, err
	}

	var state NodeState
	err = json.Unmarshal(file, &state)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func SaveState(filename string, state *NodeState) error {
    if state.Inventory == nil {
        state.Inventory = make(map[string]Item)
    }

    data, err := json.MarshalIndent(state, "", "  ") 
    if err != nil {
        return fmt.Errorf("error serializando estado: %w", err) 
    }
    return os.WriteFile(filename, data, 0644)
}