package network

import (
	"ek-server/internal/game"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Allow all origins in development
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var activeMatches = make(map[string]*game.Match)

func HandleConnections(writer http.ResponseWriter, request *http.Request) {
	conn, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.Fatal("Error upgrading to websocket: ", err)
	}
	defer conn.Close()

	// Register a new player
	playerID := uuid.New().String()
	player := &game.Player{
		ID:       playerID,
		Username: "Player " + playerID[:5],
		Conn:     conn,
		Send:     make(chan []byte),
	}

	// Assign a match to the player
	matchID := "default-match"
	if _, exists := activeMatches[matchID]; !exists {
		activeMatches[matchID] = game.NewMatch(matchID)
	}

	activeMatches[matchID].AddPlayer(player)

	log.Printf("Player %s connected to match %s", player.Username, matchID)

	// Read messages from client
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message: ", err)
			break
		}

		log.Printf("Received message from %s: %s", player.Username, string(msg))

		processMessage(matchID, player, msg)
	}
}

func processMessage(matchID string, player *game.Player, msg []byte) {
	var data map[string]string
	err := json.Unmarshal(msg, &data)
	if err != nil {
		log.Printf("Error parsing message: %v", err)
		return
	}

	action, exists := data["action"]
	if !exists {
		log.Printf("No action found in message from %s", player.Username)
		return
	}

	switch action {
	case "test_message":
		message := data["message"]
		log.Printf("Received test message from %s: %s", player.Username, message)
		for _, p := range activeMatches[matchID].Players {
			p.Conn.WriteMessage(websocket.TextMessage, []byte(message+" from "+player.Username))
		}
	}
}
