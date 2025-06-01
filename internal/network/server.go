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

// activeMatches mantiene todas las partidas activas en memoria
var activeMatches = make(map[string]*game.Match)

// ActionMessage represents a game action to be broadcasted
type ActionMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// HandleConnections maneja cada nueva conexión WebSocket
func HandleConnections(w http.ResponseWriter, r *http.Request) {
	// 1) Actualizar a WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error al actualizar a WebSocket:", err)
		return
	}

	// 2) Determinar a qué partida (match) se va a unir
	matchID := r.URL.Query().Get("match_id")
	if matchID == "" {
		matchID = "default-match"
	}
	if _, exists := activeMatches[matchID]; !exists {
		activeMatches[matchID] = game.NewMatch(matchID)
	}
	match := activeMatches[matchID]

	// 3) Intentar reconexión si envían player_id en la URL
	playerID := r.URL.Query().Get("player_id")
	var player *game.Player

	if playerID != "" {
		// Si existe, tratamos de reconectar
		if match.ReconnectPlayer(playerID, conn) {
			// Encontrar el puntero al jugador recien reconectado
			for _, p := range match.Players {
				if p.ID == playerID {
					player = p
					break
				}
			}
			if player != nil {
				// Send identification first
				sendPlayerIdentification(player, true)

				// Then send full state
				sendFullMatchState(match, player)

				broadcastAction(match, ActionPlayerJoined, map[string]interface{}{
					"player_id":    player.ID,
					"username":     player.Username,
					"is_reconnect": true,
				})

				log.Printf("Jugador %s reconectado al match %s", player.Username, matchID)
			}
		}
	}

	// 4) Si no había player (o no se reconectó), creamos uno nuevo
	if player == nil {
		playerID = uuid.New().String()
		player = &game.Player{
			ID:             playerID,
			Username:       "Player-" + playerID[:5],
			Conn:           conn,
			Send:           make(chan []byte),
			IsDisconnected: false,
		}
		match.AddPlayer(player)

		// Send identification first
		sendPlayerIdentification(player, false)

		// Then send full state
		sendFullMatchState(match, player)

		broadcastAction(match, ActionPlayerJoined, map[string]interface{}{
			"player_id":    player.ID,
			"username":     player.Username,
			"is_reconnect": false,
		})

		log.Printf("Nuevo jugador %s conectado al match %s", player.Username, matchID)
	}

	// 5) Defer para limpiar al jugador cuando se desconecte
	defer func() {
		conn.Close()

		// Marcar como desconectado (no se elimina instantáneamente)
		match.RemovePlayer(player.ID)

		// Notificar que se desconectó (para que otros sepan que puede reconectar)
		broadcastAction(match, ActionPlayerLeft, map[string]interface{}{
			"player_id":       player.ID,
			"can_reconnect":   true,
			"timeout_seconds": match.ReconnectTimeout,
			"reason":          "disconnected",
		})
		log.Printf("Jugador %s desconectado del match %s (esperando reconexión %d s)",
			player.Username, matchID, match.ReconnectTimeout)

		// Si no quedan jugadores activos, borramos la partida completa
		if match.GetActivePlayers() == 0 {
			delete(activeMatches, matchID)
			log.Printf("Match %s eliminado (sin jugadores activos)", matchID)
		}
	}()

	// 6) Bucle de lectura de mensajes del cliente
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				log.Printf("Cierre inesperado de %s: %v", player.Username, err)
			} else {
				log.Printf("Conexión cerrada por el jugador %s: %v", player.Username, err)
			}
			break
		}

		log.Printf("Mensaje recibido de %s: %s", player.Username, string(msg))
		processMessage(match, player, msg)
	}
}

// processMessage procesa cada mensaje JSON recibido de un jugador
func processMessage(match *game.Match, player *game.Player, msg []byte) {
	// Deserializar a un mapa genérico de string→string
	var data map[string]string
	err := json.Unmarshal(msg, &data)
	if err != nil {
		log.Println("Error al desenlazar mensaje:", err)
		return
	}

	action, exists := data["action"]
	if !exists {
		return
	}

	switch action {
	case ActionCardPlayed:
		card := data["card"]
		log.Printf("%s jugó la carta %s en match %s", player.Username, card, match.ID)

		// Notificar a todos los jugadores del evento "card_played"
		broadcastAction(match, ActionCardPlayed, map[string]interface{}{
			"player_id": player.ID,
			"card":      card,
		})

	case ActionEndTurn:
		match.NextTurn()
		log.Printf("Turno pasado a %s", match.GetCurrentPlayer().Username)

		// Notificar a todos que cambió turno
		broadcastAction(match, ActionTurnChanged, map[string]interface{}{
			"player_id": match.GetCurrentPlayer().ID,
		})

	// Agrega más acciones aquí según necesites
	default:
		// Si viene una acción desconocida, podemos ignorarla o devolver un error
		log.Printf("Acción desconocida recibida: %s", action)
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

	// Enviar a cada jugador activo
	for _, p := range match.Players {
		if !p.IsDisconnected {
			p.Conn.WriteMessage(websocket.TextMessage, actionData)
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

// StartCleanupRoutine inicia la rutina que cada 10 segundos limpian jugadores desconectados
func StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			log.Println("Ejecutando limpieza de jugadores desconectados...")
			for _, match := range activeMatches {
				match.CleanupDisconnectedPlayers()
			}
		}
	}()
}
