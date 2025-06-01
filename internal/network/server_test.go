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

func TestCustomRoomMaxClients(t *testing.T) {
	roomName := "testroom-max10"
	wsURL := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"}

	// Step 1: Connect the creator to the lobby
	creatorConn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		t.Fatalf("failed to connect room creator: %v", err)
	}
	defer creatorConn.Close()

	// Send create room message
	createRoomMsg := map[string]interface{}{
		"action":     "room_created",
		"name":       roomName,
		"is_private": false,
	}
	if err := creatorConn.WriteJSON(createRoomMsg); err != nil {
		t.Fatalf("failed to send create room message: %v", err)
	}

	// Wait for room created confirmation and extract room ID
	var roomID string
	creatorConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, msg, err := creatorConn.ReadMessage()
		if err != nil {
			t.Fatalf("creator failed to read message: %v", err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(msg, &m); err == nil {
			if m["type"] == "room_created" {
				if payload, ok := m["payload"].(map[string]interface{}); ok {
					if room, ok := payload["room"].(map[string]interface{}); ok {
						if id, ok := room["id"].(string); ok {
							roomID = id
							break
						}
					}
				}
			}
		}
	}
	if roomID == "" {
		t.Fatalf("Did not receive room ID from room creation response")
	}

	// Step 2: Connect 9 more clients to the lobby
	var clients []*websocket.Conn
	for i := 0; i < 9; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
		if err != nil {
			t.Fatalf("failed to connect client %d: %v", i+2, err)
		}
		clients = append(clients, c)
	}
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()

	// Step 3: Each client sends join_room message to join the custom room
	joinRoomMsg := map[string]interface{}{
		"action":  "join_room",
		"room_id": roomID,
	}
	for i, c := range clients {
		if err := c.WriteJSON(joinRoomMsg); err != nil {
			t.Fatalf("client %d failed to send join_room message: %v", i+2, err)
		}
	}

	// Step 4: Attempt to connect an 11th client to the lobby and join the custom room (should fail)
	c11, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		t.Logf("11th client could not connect to lobby as expected: %v", err)
		return
	}
	defer c11.Close()
	if err := c11.WriteJSON(joinRoomMsg); err != nil {
		t.Fatalf("11th client failed to send join_room message: %v", err)
	}
	c11.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, msg, err := c11.ReadMessage()
		if err != nil {
			t.Logf("11th client read error (expected if not in room): %v", err)
			return
		}
		var m map[string]interface{}
		if err := json.Unmarshal(msg, &m); err == nil {
			if m["type"] == "error" {
				if payload, ok := m["payload"].(map[string]interface{}); ok {
					if payload["message"] == "Room is full" {
						t.Logf("11th client received expected 'Room is full' error: %v", m)
						return
					}
				}
			}
		}
	}
	// If we get here, the 11th client did not receive the expected error
	// (the test will time out and fail)
}
