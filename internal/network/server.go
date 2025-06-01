package network

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"ek-server/internal/game"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Permitir conexiones desde cualquier origen
		return true
	},
}

// Global room manager
var roomManager = game.NewRoomManager()

// Default room ID
const DefaultRoomID = "lobby"

// Initialize default room
func init() {
	// Create the default lobby room
	roomManager.CreateRoom(DefaultRoomID, 1000, "system", false, "")
}

// ActionMessage represents a game action to be broadcasted
type ActionMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// sendFullRoomState sends the complete room state to a player
func sendFullRoomState(room *game.Room, player *game.Player) {
	action := ActionMessage{
		Type:    ActionFullState,
		Payload: room.GetPublicInfo(),
	}

	actionData, err := json.Marshal(action)
	if err != nil {
		log.Println("Error serializing room state:", err)
		return
	}

	player.Conn.WriteMessage(websocket.TextMessage, actionData)
}

// HandleConnections handles each new WebSocket connection
func HandleConnections(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading to WebSocket:", err)
		return
	}

	// Get room ID from query parameters
	roomID := r.URL.Query().Get("room_id")
	if roomID == "" {
		roomID = DefaultRoomID
	}

	// Get or create room
	room := roomManager.GetRoom(roomID)
	if room == nil {
		// Create a new room if it doesn't exist (only for non-default rooms)
		if roomID != DefaultRoomID {
			room = roomManager.CreateRoom(roomID, 10, "system", false, "")
		} else {
			// If default room doesn't exist (shouldn't happen), create it
			room = roomManager.CreateRoom(DefaultRoomID, 1000, "system", false, "")
		}
	}

	// Try to reconnect if player_id is provided
	playerID := r.URL.Query().Get("player_id")
	var player *game.Player

	if playerID != "" {
		// Try to reconnect to the room
		if room.Match != nil && room.Match.ReconnectPlayer(playerID, conn) {
			for _, p := range room.Match.Players {
				if p.ID == playerID {
					player = p
					break
				}
			}
			if player != nil {
				sendPlayerIdentification(player, true)
				if room.Match != nil {
					sendFullMatchState(room.Match, player)
				}
				broadcastToRoom(room, ActionPlayerJoined, map[string]interface{}{
					"player_id":    player.ID,
					"username":     player.Username,
					"is_reconnect": true,
				})
				log.Printf("Player %s reconnected to room %s", player.Username, roomID)
			}
		}
	}

	// If no player (or failed to reconnect), create a new one
	if player == nil {
		playerID = uuid.New().String()
		player = &game.Player{
			ID:             playerID,
			Username:       "Player-" + playerID[:5],
			Conn:           conn,
			Send:           make(chan []byte),
			IsDisconnected: false,
		}

		// Add player to room
		if !room.AddPlayer(player) {
			sendError(player, "Room is full")
			conn.Close()
			return
		}

		sendPlayerIdentification(player, false)
		sendFullRoomState(room, player)
		broadcastToRoom(room, ActionPlayerJoined, map[string]interface{}{
			"player_id":    player.ID,
			"username":     player.Username,
			"is_reconnect": false,
		})
		log.Printf("New player %s connected to room %s", player.Username, roomID)
	}

	// Cleanup when player disconnects
	defer func() {
		conn.Close()
		if room.Match != nil {
			room.Match.RemovePlayer(player.ID)
			broadcastToRoom(room, ActionPlayerLeft, map[string]interface{}{
				"player_id":       player.ID,
				"can_reconnect":   true,
				"timeout_seconds": room.Match.ReconnectTimeout,
				"reason":          "disconnected",
			})
		}
		room.RemovePlayer(player.ID)
		log.Printf("Player %s disconnected from room %s", player.Username, roomID)

		// Remove room if empty
		if room.GetPlayerCount() == 0 && room.Name != "lobby" {
			roomManager.RemoveRoom(roomID)
			log.Printf("Room %s removed (empty)", roomID)
		}
	}()

	// Message reading loop
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				log.Printf("Unexpected close from %s: %v", player.Username, err)
			} else {
				log.Printf("Connection closed by player %s: %v", player.Username, err)
			}
			break
		}

		log.Printf("Message received from %s: %s", player.Username, string(msg))
		processMessage(room, player, msg)
	}
}

// processMessage processes each JSON message received from a player
func processMessage(room *game.Room, player *game.Player, msg []byte) {
	var data map[string]interface{}
	err := json.Unmarshal(msg, &data)
	if err != nil {
		log.Println("Error unmarshaling message:", err)
		return
	}

	action, exists := data["action"]
	if !exists {
		return
	}

	switch action {
	case ActionCardPlayed:
		if room.Match == nil {
			sendError(player, "No active match")
			return
		}
		if card, ok := data["card"].(string); ok {
			log.Printf("%s played card %s in match %s", player.Username, card, room.Match.ID)
			broadcastAction(room.Match, ActionCardPlayed, map[string]interface{}{
				"player_id": player.ID,
				"card":      card,
			})
		}

	case ActionEndTurn:
		if room.Match == nil {
			sendError(player, "No active match")
			return
		}
		room.Match.NextTurn()
		log.Printf("Turn passed to %s", room.Match.GetCurrentPlayer().Username)
		broadcastAction(room.Match, ActionTurnChanged, map[string]interface{}{
			"player_id": room.Match.GetCurrentPlayer().ID,
		})

	case ActionRoomList:
		// Send list of available rooms
		rooms := roomManager.ListRooms()
		action := ActionMessage{
			Type:    ActionRoomList,
			Payload: rooms,
		}
		actionData, err := json.Marshal(action)
		if err != nil {
			log.Println("Error serializing room list:", err)
			return
		}
		player.Conn.WriteMessage(websocket.TextMessage, actionData)

	case ActionRoomCreated:
		// Create a new room
		if name, ok := data["name"].(string); ok {
			isPrivate := false
			password := ""
			if priv, ok := data["is_private"].(bool); ok {
				isPrivate = priv
			}
			if pwd, ok := data["password"].(string); ok && isPrivate {
				password = pwd
			}

			room := roomManager.CreateRoom(name, 10, player.Username, isPrivate, password)
			action := ActionMessage{
				Type: ActionRoomCreated,
				Payload: map[string]interface{}{
					"room": room.GetPublicInfo(),
				},
			}
			actionData, err := json.Marshal(action)
			if err != nil {
				log.Println("Error serializing room created:", err)
				return
			}
			player.Conn.WriteMessage(websocket.TextMessage, actionData)
		}

	case "start_match":
		// Only room creator can start the match
		if room.CreatedBy != player.Username {
			sendError(player, "Only room creator can start the match")
			return
		}

		// Start the match
		if room.StartMatch() {
			// Send full match state to all players
			for _, p := range room.Players {
				sendFullMatchState(room.Match, p)
			}
			// Broadcast match started
			broadcastToRoom(room, "match_started", map[string]interface{}{
				"match_id": room.Match.ID,
			})
		} else {
			sendError(player, "Cannot start match: not enough players")
		}

	default:
		log.Printf("Unknown action received: %s", action)
	}
}

// sendFullMatchState envía el estado completo (snapshot) de la partida a un solo jugador
func sendFullMatchState(match *game.Match, player *game.Player) {
	jsonState, err := match.ToJSON()
	if err != nil {
		log.Println("Error al convertir estado a JSON:", err)
		return
	}

	action := ActionMessage{
		Type:    ActionFullState,
		Payload: json.RawMessage(jsonState),
	}

	actionData, err := json.Marshal(action)
	if err != nil {
		log.Println("Error al serializar estado:", err)
		return
	}

	player.Conn.WriteMessage(websocket.TextMessage, actionData)
}

// broadcastAction envía un "evento" (solo delta) a todos los jugadores de la partida
func broadcastAction(match *game.Match, actionType string, payload interface{}) {
	action := ActionMessage{
		Type:    actionType,
		Payload: payload,
	}

	actionData, err := json.Marshal(action)
	if err != nil {
		log.Println("Error al serializar broadcast:", err)
		return
	}

	// If no match, don't broadcast
	if match == nil {
		return
	}

	// Enviar a cada jugador activo
	for _, p := range match.Players {
		if !p.IsDisconnected {
			p.Conn.WriteMessage(websocket.TextMessage, actionData)
		}
	}
}

// broadcastToRoom sends a message to all players in a room
func broadcastToRoom(room *game.Room, actionType string, payload interface{}) {
	action := ActionMessage{
		Type:    actionType,
		Payload: payload,
	}

	actionData, err := json.Marshal(action)
	if err != nil {
		log.Println("Error serializing room broadcast:", err)
		return
	}

	room.Mu.RLock()
	defer room.Mu.RUnlock()

	for _, player := range room.Players {
		if !player.IsDisconnected {
			player.Conn.WriteMessage(websocket.TextMessage, actionData)
		}
	}
}

// sendError envía un mensaje de error a un jugador concreto
func sendError(player *game.Player, reason string) {
	action := ActionMessage{
		Type: "error",
		Payload: map[string]string{
			"message": reason,
		},
	}

	jsonErr, err := json.Marshal(action)
	if err != nil {
		log.Println("Error al serializar mensaje de error:", err)
		return
	}
	player.Conn.WriteMessage(websocket.TextMessage, jsonErr)
}

// sendPlayerIdentification sends the player their ID and connection status
func sendPlayerIdentification(player *game.Player, isReconnect bool) {
	action := ActionMessage{
		Type: ActionPlayerIdentified,
		Payload: map[string]interface{}{
			"player_id":    player.ID,
			"username":     player.Username,
			"is_reconnect": isReconnect,
		},
	}

	actionData, err := json.Marshal(action)
	if err != nil {
		log.Println("Error al serializar identificación:", err)
		return
	}
	player.Conn.WriteMessage(websocket.TextMessage, actionData)
}

// StartCleanupRoutine starts the routine that cleans up disconnected players
func StartCleanupRoutine() {
	// Start room cleanup routine
	roomManager.StartCleanupRoutine()

	// Start player cleanup routine
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			log.Println("Running cleanup of disconnected players...")
			// Get all rooms and clean up their matches
			rooms := roomManager.ListRooms()
			for _, roomInfo := range rooms {
				if roomID, ok := roomInfo["id"].(string); ok {
					if room := roomManager.GetRoom(roomID); room != nil && room.Match != nil {
						room.Match.CleanupDisconnectedPlayers()
					}
				}
			}
		}
	}()
}
