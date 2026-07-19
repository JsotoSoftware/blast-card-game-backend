package transport

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"exploding-game/server/internal/game"
	"exploding-game/server/internal/room"

	"github.com/coder/websocket"
)

const (
	cancelWindowBroadcastDelay = 10100 * time.Millisecond

	MessagePong             = "PONG"
	MessageCommandAck       = "COMMAND_ACK"
	MessageRoomCreated      = "ROOM_CREATED"
	MessageRoomJoined       = "ROOM_JOINED"
	MessageRoomView         = "ROOM_VIEW"
	MessageGameView         = "GAME_VIEW"
	MessageGameEvents       = "GAME_EVENTS"
	MessageGameStarted      = "GAME_STARTED"
	MessageHostTransferred  = "HOST_TRANSFERRED"
	MessageKickVoteStarted  = "KICK_VOTE_STARTED"
	MessageKickVoteUpdated  = "KICK_VOTE_UPDATED"
	MessageKickVoteResolved = "KICK_VOTE_RESOLVED"
	MessageKickedFromRoom   = "KICKED_FROM_ROOM"
	MessageCommandError     = "COMMAND_ERROR"
)

type WebSocketHandler struct {
	manager *room.Manager
	logger  *slog.Logger
	seq     atomic.Int64
	mu      sync.Mutex
	clients map[*websocket.Conn]*connectedClient
	rooms   map[string]map[*connectedClient]bool
}

type connectedClient struct {
	conn     *websocket.Conn
	writeMu  sync.Mutex
	roomID   string
	playerID string
	token    string
}

func NewWebSocketHandler(manager *room.Manager, logger *slog.Logger) *WebSocketHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebSocketHandler{
		manager: manager,
		logger:  logger,
		clients: map[*websocket.Conn]*connectedClient{},
		rooms:   map[string]map[*connectedClient]bool{},
	}
}

func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Warn("websocket accept failed", "error", err)
		return
	}
	client := h.registerConnection(conn)
	defer conn.Close(websocket.StatusNormalClosure, "connection closed")
	defer h.unregisterConnection(client)

	ctx := r.Context()
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure && !errors.Is(err, context.Canceled) {
				h.logger.Info("websocket read ended", "error", err)
			}
			return
		}
		if messageType != websocket.MessageText {
			if err := h.writeError(ctx, client, "", "UNSUPPORTED_MESSAGE_TYPE", "Only text JSON messages are supported."); err != nil {
				return
			}
			continue
		}

		if err := h.handleMessage(ctx, client, data); err != nil {
			h.logger.Info("websocket write failed", "error", err)
			return
		}
	}
}

func (h *WebSocketHandler) handleMessage(ctx context.Context, client *connectedClient, data []byte) error {
	var envelope ClientEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return h.writeError(ctx, client, "", "MALFORMED_JSON", "Malformed JSON message.")
	}
	if envelope.Version != ProtocolVersion {
		return h.writeError(ctx, client, envelope.RequestID, "UNSUPPORTED_VERSION", "Unsupported protocol version.")
	}

	switch envelope.Type {
	case "PING":
		return h.writeEnvelope(ctx, client, MessagePong, map[string]string{"requestId": envelope.RequestID})
	case "CREATE_ROOM":
		return h.handleCreateRoom(ctx, client, envelope)
	case "JOIN_ROOM":
		return h.handleJoinRoom(ctx, client, envelope)
	case "SET_READY":
		return h.handleSetReady(ctx, client, envelope)
	case "START_GAME":
		return h.handleStartGame(ctx, client, envelope)
	case "TRANSFER_HOST":
		return h.handleTransferHost(ctx, client, envelope)
	case "LEAVE_ROOM":
		return h.handleLeaveRoom(ctx, client, envelope)
	case "START_KICK_VOTE":
		return h.handleStartKickVote(ctx, client, envelope)
	case "CAST_KICK_VOTE":
		return h.handleCastKickVote(ctx, client, envelope)
	case "DRAW_CARD":
		return h.handleDrawCard(ctx, client, envelope)
	case "PLAY_CARD":
		return h.handlePlayCard(ctx, client, envelope)
	case "PLACE_EXPLOSIVE":
		return h.handlePlaceExplosive(ctx, client, envelope)
	case "CHOOSE_CARD_FOR_REQUEST":
		return h.handleChooseCardForRequest(ctx, client, envelope)
	case "PLAY_CANCEL":
		return h.handlePlayCancel(ctx, client, envelope)
	default:
		return h.writeError(ctx, client, envelope.RequestID, "UNKNOWN_COMMAND", "Unknown command type.")
	}
}

func (h *WebSocketHandler) handleCreateRoom(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	var payload CreateRoomPayload
	if len(envelope.Payload) > 0 {
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid CREATE_ROOM payload.")
		}
	}
	if payload.PlayerName == "" {
		payload.PlayerName = "Player"
	}

	createdRoom, host, err := h.manager.CreateRoom(payload.PlayerName)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, "CREATE_ROOM_FAILED", err.Error())
	}
	h.attachClientToRoom(client, createdRoom.ID(), host.ID, host.Token)

	if err := h.writeEnvelope(ctx, client, MessageRoomCreated, RoomSessionPayload{
		RequestID:   envelope.RequestID,
		RoomID:      createdRoom.ID(),
		PlayerID:    host.ID,
		PlayerToken: host.Token,
		IsHost:      host.IsHost,
	}); err != nil {
		return err
	}
	return h.broadcastRoomView(ctx, createdRoom.ID())
}

func (h *WebSocketHandler) handleJoinRoom(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	var payload JoinRoomPayload
	if len(envelope.Payload) > 0 {
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid JOIN_ROOM payload.")
		}
	}
	roomID := firstNonEmpty(envelope.RoomID, payload.RoomID, client.roomID)
	if roomID == "" {
		return h.writeError(ctx, client, envelope.RequestID, "ROOM_REQUIRED", "Room ID is required.")
	}
	if payload.PlayerName == "" {
		payload.PlayerName = "Player"
	}

	player, err := h.manager.JoinRoom(roomID, payload.PlayerName)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	h.attachClientToRoom(client, roomID, player.ID, player.Token)

	if err := h.writeEnvelope(ctx, client, MessageRoomJoined, RoomSessionPayload{
		RequestID:   envelope.RequestID,
		RoomID:      roomID,
		PlayerID:    player.ID,
		PlayerToken: player.Token,
		IsHost:      player.IsHost,
	}); err != nil {
		return err
	}
	return h.broadcastRoomView(ctx, roomID)
}

func (h *WebSocketHandler) handleSetReady(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	var payload SetReadyPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid SET_READY payload.")
	}
	if err := h.manager.SetReady(roomID, token, payload.Ready); err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	if err := h.writeEnvelope(ctx, client, MessageCommandAck, CommandAckPayload{RequestID: envelope.RequestID}); err != nil {
		return err
	}
	return h.broadcastRoomView(ctx, roomID)
}

func (h *WebSocketHandler) handleStartGame(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	events, err := h.manager.StartGame(roomID, token)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	if err := h.broadcastEnvelope(ctx, roomID, MessageGameStarted, GameStartedPayload{RequestID: envelope.RequestID, RoomID: roomID, Events: events}); err != nil {
		return err
	}
	if err := h.broadcastGameViews(ctx, roomID); err != nil {
		return err
	}
	return h.broadcastRoomView(ctx, roomID)
}

func (h *WebSocketHandler) handleTransferHost(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	var payload TransferHostPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil || payload.TargetPlayerID == "" {
		return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid TRANSFER_HOST payload.")
	}
	if err := h.manager.TransferHost(roomID, token, payload.TargetPlayerID); err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	if err := h.broadcastEnvelope(ctx, roomID, MessageHostTransferred, map[string]string{"requestId": envelope.RequestID, "targetPlayerId": payload.TargetPlayerID}); err != nil {
		return err
	}
	return h.broadcastRoomView(ctx, roomID)
}

func (h *WebSocketHandler) handleLeaveRoom(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, _, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	playerID := client.playerID
	if err := h.manager.LeaveRoom(roomID, playerID); err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	h.detachClientFromRoom(client)
	if err := h.writeEnvelope(ctx, client, MessageCommandAck, CommandAckPayload{RequestID: envelope.RequestID}); err != nil {
		return err
	}
	return h.broadcastRoomView(ctx, roomID)
}

func (h *WebSocketHandler) handleStartKickVote(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	var payload StartKickVotePayload
	if err := decodePayload(envelope.Payload, &payload); err != nil || payload.TargetPlayerID == "" {
		return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid START_KICK_VOTE payload.")
	}
	passed, kickedPlayerID, err := h.manager.StartKickVote(roomID, token, payload.TargetPlayerID)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	messageType := MessageKickVoteStarted
	payloadOut := map[string]any{"requestId": envelope.RequestID, "targetPlayerId": payload.TargetPlayerID}
	if passed {
		messageType = MessageKickVoteResolved
		payloadOut["kickedPlayerId"] = kickedPlayerID
		h.notifyAndDetachKickedPlayer(ctx, roomID, kickedPlayerID)
	}
	if err := h.broadcastEnvelope(ctx, roomID, messageType, payloadOut); err != nil {
		return err
	}
	return h.broadcastRoomView(ctx, roomID)
}

func (h *WebSocketHandler) handleCastKickVote(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	var payload CastKickVotePayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid CAST_KICK_VOTE payload.")
	}
	passed, kickedPlayerID, err := h.manager.CastKickVote(roomID, token, payload.Approve)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	messageType := MessageKickVoteUpdated
	payloadOut := map[string]any{"requestId": envelope.RequestID, "approve": payload.Approve}
	if passed {
		messageType = MessageKickVoteResolved
		payloadOut["kickedPlayerId"] = kickedPlayerID
		h.notifyAndDetachKickedPlayer(ctx, roomID, kickedPlayerID)
	}
	if err := h.broadcastEnvelope(ctx, roomID, messageType, payloadOut); err != nil {
		return err
	}
	return h.broadcastRoomView(ctx, roomID)
}

func (h *WebSocketHandler) handleDrawCard(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	events, err := h.manager.DrawCard(roomID, token)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	return h.ackEventsAndViews(ctx, client, roomID, envelope.RequestID, events)
}

func (h *WebSocketHandler) handlePlayCard(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	var payload PlayCardPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid PLAY_CARD payload.")
	}

	events, err := h.manager.PlayCard(roomID, token, payload.CardIDs, payload.TargetID)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	h.scheduleCancelWindowExpiration(roomID)
	return h.ackEventsAndViews(ctx, client, roomID, envelope.RequestID, events)
}

func (h *WebSocketHandler) handlePlaceExplosive(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	var payload PlaceExplosivePayload
	if err := decodePayload(envelope.Payload, &payload); err != nil || payload.Index == nil {
		return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid PLACE_EXPLOSIVE payload.")
	}

	events, err := h.manager.PlaceExplosive(roomID, token, *payload.Index)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	return h.ackEventsAndViews(ctx, client, roomID, envelope.RequestID, events)
}

func (h *WebSocketHandler) handleChooseCardForRequest(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	var payload ChooseCardForRequestPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil || payload.CardID == "" {
		return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid CHOOSE_CARD_FOR_REQUEST payload.")
	}

	events, err := h.manager.ChooseCardForRequest(roomID, token, payload.CardID)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	return h.ackEventsAndViews(ctx, client, roomID, envelope.RequestID, events)
}

func (h *WebSocketHandler) handlePlayCancel(ctx context.Context, client *connectedClient, envelope ClientEnvelope) error {
	roomID, token, err := h.sessionForCommand(client, envelope)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	var payload PlayCancelPayload
	if err := decodePayload(envelope.Payload, &payload); err != nil {
		return h.writeError(ctx, client, envelope.RequestID, "INVALID_PAYLOAD", "Invalid PLAY_CANCEL payload.")
	}

	events, err := h.manager.PlayCancel(roomID, token, payload.CardID, payload.PendingActionID)
	if err != nil {
		return h.writeError(ctx, client, envelope.RequestID, errorCode(err), err.Error())
	}
	return h.ackEventsAndViews(ctx, client, roomID, envelope.RequestID, events)
}

func (h *WebSocketHandler) ackEventsAndViews(ctx context.Context, client *connectedClient, roomID string, requestID string, events []game.Event) error {
	if err := h.writeEnvelope(ctx, client, MessageCommandAck, CommandAckPayload{RequestID: requestID}); err != nil {
		return err
	}
	if err := h.broadcastGameEvents(ctx, roomID, events); err != nil {
		return err
	}
	return h.broadcastGameViews(ctx, roomID)
}

func (h *WebSocketHandler) scheduleCancelWindowExpiration(roomID string) {
	time.AfterFunc(cancelWindowBroadcastDelay, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		events, err := h.manager.ExpireCancelWindow(roomID, time.Now())
		if err != nil {
			return
		}
		if err := h.broadcastGameEvents(ctx, roomID, events); err != nil {
			h.logger.Info("failed to broadcast expired cancel-window events", "roomId", roomID, "error", err)
			return
		}
		if err := h.broadcastGameViews(ctx, roomID); err != nil {
			h.logger.Info("failed to broadcast expired cancel-window game views", "roomId", roomID, "error", err)
		}
	})
}

func (h *WebSocketHandler) sessionForCommand(client *connectedClient, envelope ClientEnvelope) (string, string, error) {
	roomID := firstNonEmpty(envelope.RoomID, client.roomID)
	token := firstNonEmpty(envelope.PlayerToken, client.token)
	if roomID == "" {
		return "", "", room.ErrRoomNotFound
	}
	if token == "" {
		return "", "", room.ErrInvalidPlayerToken
	}
	return roomID, token, nil
}

func (h *WebSocketHandler) broadcastRoomView(ctx context.Context, roomID string) error {
	view, err := h.manager.RoomView(roomID)
	if err != nil {
		if errors.Is(err, room.ErrRoomNotFound) {
			return nil
		}
		return err
	}
	return h.broadcastEnvelope(ctx, roomID, MessageRoomView, view)
}

func (h *WebSocketHandler) broadcastGameViews(ctx context.Context, roomID string) error {
	views, err := h.manager.GameViews(roomID)
	if err != nil {
		return err
	}
	clients := h.roomClients(roomID)
	for _, client := range clients {
		view, exists := views[client.playerID]
		if !exists {
			continue
		}
		if err := h.writeEnvelope(ctx, client, MessageGameView, view); err != nil {
			return err
		}
	}
	return nil
}

func (h *WebSocketHandler) broadcastGameEvents(ctx context.Context, roomID string, events []game.Event) error {
	clients := h.roomClients(roomID)
	for _, client := range clients {
		filtered := game.FilterEventsForPlayer(events, client.playerID)
		if len(filtered) == 0 {
			continue
		}
		if err := h.writeEnvelope(ctx, client, MessageGameEvents, map[string]any{"events": filtered}); err != nil {
			return err
		}
	}
	return nil
}

func (h *WebSocketHandler) broadcastEnvelope(ctx context.Context, roomID string, messageType string, payload any) error {
	clients := h.roomClients(roomID)
	for _, client := range clients {
		if err := h.writeEnvelope(ctx, client, messageType, payload); err != nil {
			return err
		}
	}
	return nil
}

func (h *WebSocketHandler) notifyAndDetachKickedPlayer(ctx context.Context, roomID string, kickedPlayerID string) {
	clients := h.roomPlayerClients(roomID, kickedPlayerID)
	for _, client := range clients {
		if err := h.writeEnvelope(ctx, client, MessageKickedFromRoom, map[string]any{
			"roomId":         roomID,
			"kickedPlayerId": kickedPlayerID,
			"reason":         "kick_vote_passed",
		}); err != nil {
			h.logger.Info("failed to notify kicked player", "roomId", roomID, "playerId", kickedPlayerID, "error", err)
		}
	}
	h.detachRoomPlayer(roomID, kickedPlayerID)
}

func (h *WebSocketHandler) writeError(ctx context.Context, client *connectedClient, requestID string, code string, message string) error {
	return h.writeEnvelope(ctx, client, MessageCommandError, CommandErrorPayload{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func (h *WebSocketHandler) writeEnvelope(ctx context.Context, client *connectedClient, messageType string, payload any) error {
	data, err := json.Marshal(ServerEnvelope{
		Version:  ProtocolVersion,
		Type:     messageType,
		Sequence: h.seq.Add(1),
		Payload:  payload,
	})
	if err != nil {
		return err
	}

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	return client.conn.Write(writeCtx, websocket.MessageText, data)
}

func (h *WebSocketHandler) registerConnection(conn *websocket.Conn) *connectedClient {
	client := &connectedClient{conn: conn}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = client
	return client
}

func (h *WebSocketHandler) unregisterConnection(client *connectedClient) {
	roomID := client.roomID
	playerID := client.playerID
	h.detachClientFromRoom(client)
	if roomID != "" && playerID != "" {
		_ = h.manager.LeaveRoom(roomID, playerID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.broadcastRoomView(ctx, roomID)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, client.conn)
}

func (h *WebSocketHandler) attachClientToRoom(client *connectedClient, roomID string, playerID string, token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if client.roomID != "" && h.rooms[client.roomID] != nil {
		delete(h.rooms[client.roomID], client)
	}
	client.roomID = roomID
	client.playerID = playerID
	client.token = token
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = map[*connectedClient]bool{}
	}
	h.rooms[roomID][client] = true
}

func (h *WebSocketHandler) detachClientFromRoom(client *connectedClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if client.roomID != "" && h.rooms[client.roomID] != nil {
		delete(h.rooms[client.roomID], client)
		if len(h.rooms[client.roomID]) == 0 {
			delete(h.rooms, client.roomID)
		}
	}
	client.roomID = ""
	client.playerID = ""
	client.token = ""
}

func (h *WebSocketHandler) detachRoomPlayer(roomID string, playerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := h.rooms[roomID]
	for client := range clients {
		if client.playerID == playerID {
			delete(clients, client)
			client.roomID = ""
			client.playerID = ""
			client.token = ""
		}
	}
	if len(clients) == 0 {
		delete(h.rooms, roomID)
	}
}

func (h *WebSocketHandler) roomClients(roomID string) []*connectedClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := make([]*connectedClient, 0, len(h.rooms[roomID]))
	for client := range h.rooms[roomID] {
		clients = append(clients, client)
	}
	return clients
}

func (h *WebSocketHandler) roomPlayerClients(roomID string, playerID string) []*connectedClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := make([]*connectedClient, 0, 1)
	for client := range h.rooms[roomID] {
		if client.playerID == playerID {
			clients = append(clients, client)
		}
	}
	return clients
}

func decodePayload(data json.RawMessage, target any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, room.ErrRoomNotFound):
		return "ROOM_NOT_FOUND"
	case errors.Is(err, room.ErrRoomFull):
		return "ROOM_FULL"
	case errors.Is(err, room.ErrPlayerAlreadyInRoom):
		return "PLAYER_ALREADY_IN_ROOM"
	case errors.Is(err, room.ErrInvalidPlayerToken):
		return "INVALID_PLAYER_TOKEN"
	case errors.Is(err, room.ErrNotHost):
		return "NOT_HOST"
	case errors.Is(err, room.ErrPlayersNotReady):
		return "PLAYERS_NOT_READY"
	case errors.Is(err, room.ErrInvalidHostTransfer):
		return "INVALID_HOST_TRANSFER"
	case errors.Is(err, room.ErrKickVoteAlreadyActive):
		return "KICK_VOTE_ALREADY_ACTIVE"
	case errors.Is(err, room.ErrNoKickVoteActive):
		return "NO_KICK_VOTE_ACTIVE"
	case errors.Is(err, room.ErrCannotVoteKickSelf):
		return "CANNOT_VOTE_KICK_SELF"
	case errors.Is(err, room.ErrInvalidKickVoteTarget):
		return "INVALID_KICK_VOTE_TARGET"
	case errors.Is(err, room.ErrGameNotStarted):
		return "GAME_NOT_STARTED"
	case errors.Is(err, game.ErrInvalidPhase):
		return "INVALID_PHASE"
	case errors.Is(err, game.ErrNotYourTurn):
		return "NOT_YOUR_TURN"
	case errors.Is(err, game.ErrPlayerNotFound):
		return "PLAYER_NOT_FOUND"
	case errors.Is(err, game.ErrPlayerNotAlive):
		return "PLAYER_NOT_ALIVE"
	case errors.Is(err, game.ErrCardNotInHand):
		return "CARD_NOT_IN_HAND"
	case errors.Is(err, game.ErrInvalidCardPlay):
		return "INVALID_CARD_PLAY"
	case errors.Is(err, game.ErrTargetRequired):
		return "TARGET_REQUIRED"
	case errors.Is(err, game.ErrInvalidTarget):
		return "INVALID_TARGET"
	case errors.Is(err, game.ErrDrawPileEmpty):
		return "DRAW_PILE_EMPTY"
	case errors.Is(err, game.ErrNoPendingAction):
		return "NO_PENDING_ACTION"
	case errors.Is(err, game.ErrInvalidPlacement):
		return "INVALID_PLACEMENT"
	case errors.Is(err, game.ErrActionNotCancelable):
		return "ACTION_NOT_CANCELABLE"
	case errors.Is(err, game.ErrCancelWindowActive):
		return "CANCEL_WINDOW_ACTIVE"
	default:
		return "COMMAND_FAILED"
	}
}
