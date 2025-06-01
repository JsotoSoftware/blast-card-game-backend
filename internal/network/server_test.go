package network

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type joinMsg struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

func TestAllConnectionsReceivePlayerJoined(t *testing.T) {
	wsURL := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws", RawQuery: "room_id=lobby"}

	const numClients = 10
	var clients []*websocket.Conn
	var mu sync.Mutex

	// Connect all clients first
	for i := 0; i < numClients; i++ {
		fmt.Printf("Connecting client %d...\n", i)
		c, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
		if err != nil {
			t.Fatalf("failed to connect client %d: %v", i, err)
		}
		fmt.Printf("Client %d connected.\n", i)
		mu.Lock()
		clients = append(clients, c)
		mu.Unlock()
	}
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()

	// Now, after all are connected, start reading messages
	for i, c := range clients {
		fmt.Printf("\n--- Reading messages for client %d ---\n", i)
		playerJoinedCount := 0
		otherCount := 0
		deadline := time.Now().Add(15 * time.Second)
		for msgNum := 0; msgNum < 20; msgNum++ {
			c.SetReadDeadline(deadline)
			_, msg, err := c.ReadMessage()
			if err != nil {
				fmt.Printf("client %d failed to read message: %v\n", i, err)
				break
			}
			var m joinMsg
			if err := json.Unmarshal(msg, &m); err == nil {
				if m.Type == "player_joined" {
					playerJoinedCount++
					pretty, _ := json.MarshalIndent(m, "", "  ")
					fmt.Printf("client %d received player_joined: %s\n", i, string(pretty))
					continue
				}
			}
			// Print all other messages
			fmt.Printf("client %d received other message: %s\n", i, string(msg))
			otherCount++
		}
		fmt.Printf("client %d summary: player_joined=%d, other=%d\n", i, playerJoinedCount, otherCount)
		if playerJoinedCount == 0 {
			t.Fatalf("client %d did not receive any player_joined messages", i)
		}
	}
}
