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
	log.Printf("Incoming connection from %s, query: %s", r.RemoteAddr, r.URL.RawQuery)
	log.Println("New connection attempt")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading to WebSocket:", err)
		return
	}

	// Always connect to lobby first
	roomID := DefaultRoomID
	room := roomManager.GetRoom(roomID)
	if room == nil {
		room = roomManager.CreateRoom(DefaultRoomID, 1000, "system", false, "")
	}

	playerID := uuid.New().String()
	player := &game.Player{
		ID:             playerID,
		Username:       "Player-" + playerID[:5],
		Conn:           conn,
		Send:           make(chan []byte),
		IsDisconnected: false,
	}

	log.Printf("Attempting to add player %s to lobby (current: %d, max: %d)", player.Username, room.GetPlayerCount(), room.MaxPlayers)
	if !room.AddPlayer(player) {
		log.Printf("Lobby is full (%d/%d). Rejecting player %s", room.GetPlayerCount(), room.MaxPlayers, player.Username)
		sendError(player, "Lobby is full")
		log.Printf("Sent 'Lobby is full' error to player %s", player.Username)
		return
	}
	log.Printf("Player %s successfully added to lobby (now: %d/%d)", player.Username, room.GetPlayerCount(), room.MaxPlayers)
	sendPlayerIdentification(player, false)
	sendFullRoomState(room, player)
	broadcastToRoom(room, ActionPlayerJoined, map[string]interface{}{
		"player_id":    player.ID,
		"username":     player.Username,
		"is_reconnect": false,
	})
	log.Printf("New player %s connected to lobby", player.Username)

	defer func() {
		conn.Close()
		room.RemovePlayer(player.ID)
		log.Printf("Player %s disconnected and removed from lobby", player.Username)
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
	if err := json.Unmarshal(msg, &data); err != nil {
		log.Println("Error unmarshaling message:", err)
		return
	}

	action, exists := data["action"]
	if !exists {
		return
	}

	switch action {
	case "join_room":
		roomID, ok := data["room_id"].(string)
		if !ok || roomID == "" {
			sendError(player, "Missing or invalid room_id")
			return
		}
		customRoom := roomManager.GetRoom(roomID)
		if customRoom == nil {
			customRoom = roomManager.CreateRoom(roomID, 10, player.Username, false, "")
		}
		if customRoom.GetPlayerCount() >= customRoom.MaxPlayers {
			sendError(player, "Room is full")
			log.Printf("Sent 'Room is full' error to player %s for room %s", player.Username, roomID)
			return
		}
		// Remove from current room (lobby)
		room.RemovePlayer(player.ID)
		customRoom.AddPlayer(player)
		log.Printf("Player %s moved to room %s", player.Username, roomID)
		sendFullRoomState(customRoom, player)
		broadcastToRoom(customRoom, ActionPlayerJoined, map[string]interface{}{
			"player_id":    player.ID,
			"username":     player.Username,
			"is_reconnect": false,
		})
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
