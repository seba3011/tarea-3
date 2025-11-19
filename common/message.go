package common

import "time"

type MessageType string

const (
	MsgElection    MessageType = "ELECTION"
	MsgOK          MessageType = "OK"
	MsgCoordinator MessageType = "COORDINATOR"
	MsgHeartbeat   MessageType = "HEARTBEAT"
	
	MsgSyncRequest MessageType = "SYNC_REQUEST"
	MsgSyncState   MessageType = "SYNC_STATE"
	MsgEvent       MessageType = "EVENT"
	
	OpReadInventory MessageType = "READ_INVENTORY"
	OpSetQuantity   MessageType = "SET_QTY"
	OpSetPrice      MessageType = "SET_PRICE"
)

type Message struct {
	Type    MessageType `json:"type"`
	SenderID int        `json:"sender_id"`
	Payload any         `json:"payload,omitempty"`
	Time    time.Time   `json:"time"`
}

type Peer struct {
	ID   int    `json:"id"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

type Config struct {
    ID          int    `json:"node_id"` 
    Peers       []Peer `json:"known_nodes"` 
    IsPrimary   bool `json:"is_primary"` 
    LocalAddress string `json:"local_address"`
	Port         int
}

type ClientRequest struct {
	Type     MessageType `json:"type"`
	ItemName string      `json:"item_name"`
	NewValue int         `json:"new_value"`
}

type ClientResponse struct {
	IsPrimary bool          `json:"is_primary"`
	PrimaryID int           `json:"primary_id"`
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
	Inventory map[string]Item `json:"inventory,omitempty"`
	SeqNumber int           `json:"seq_number"`
}