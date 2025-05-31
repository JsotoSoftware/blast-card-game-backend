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

// ActionMessage represents a game action to be broadcasted
type ActionMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

func sendFullMatchState(match *game.Match, player *game.Player) {
	matchData, err := match.ToJSON()
	if err != nil {
		log.Printf("Error marshaling match data: %v", err)
		return
	}

	action := ActionMessage{
		Type:    ActionFullState,
		Payload: json.RawMessage(matchData),
	}

	actionData, err := json.Marshal(action)
	if err != nil {
		log.Printf("Error marshaling action: %v", err)
		return
	}

	player.Conn.WriteMessage(websocket.TextMessage, actionData)
}

func broadcastAction(match *game.Match, actionType string, payload interface{}) {
	action := ActionMessage{
		Type:    actionType,
		Payload: payload,
	}

	actionData, err := json.Marshal(action)
	if err != nil {
		log.Printf("Error marshaling action: %v", err)
		return
	}

	for _, player := range match.Players {
		player.Conn.WriteMessage(websocket.TextMessage, actionData)
	}
}

func HandleConnections(writer http.ResponseWriter, request *http.Request) {
	conn, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.Fatal("Error upgrading to websocket: ", err)
	}
	defer conn.Close()

	// Get match ID from query parameters
	matchID := request.URL.Query().Get("match_id")
	if matchID == "" {
		matchID = "default-match"
	}

	// Get player ID from query parameters (for reconnection)
	playerID := request.URL.Query().Get("player_id")
	var player *game.Player

	if playerID != "" {
		// Reconnection attempt
		if match, exists := activeMatches[matchID]; exists {
			for _, p := range match.Players {
				if p.ID == playerID {
					player = p
					player.Conn = conn
					player.Send = make(chan []byte)
					break
				}
			}
		}
	}

	// If no existing player found, create new one
	if player == nil {
		playerID = uuid.New().String()
		player = &game.Player{
			ID:       playerID,
			Username: "Player " + playerID[:5],
			Conn:     conn,
			Send:     make(chan []byte),
		}
	}

	// Create or get match
	if _, exists := activeMatches[matchID]; !exists {
		activeMatches[matchID] = game.NewMatch(matchID)
	}

	match := activeMatches[matchID]
	match.AddPlayer(player)

	// Send full match state to the new/reconnected player
	sendFullMatchState(match, player)

	// Broadcast player joined action
	broadcastAction(match, ActionPlayerJoined, map[string]interface{}{
		"player_id":    player.ID,
		"username":     player.Username,
		"is_reconnect": playerID != "",
	})

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
	var data map[string]interface{}
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

	match := activeMatches[matchID]
	switch action {
	case "test_message":
		message := data["message"].(string)
		broadcastAction(match, ActionChatMessage, map[string]string{
			"player_id": player.ID,
			"username":  player.Username,
			"message":   message,
		})
	}
}
