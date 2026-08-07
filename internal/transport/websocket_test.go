package transport

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"exploding-game/server/internal/game"
	"exploding-game/server/internal/room"

	"github.com/coder/websocket"
)

type testServerEnvelope struct {
	Version  int             `json:"version"`
	Type     string          `json:"type"`
	Sequence int64           `json:"sequence"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

func TestWebSocketPingPong(t *testing.T) {
	conn, closeServer := newTestWebSocket(t)
	defer closeServer()
	defer conn.Close(websocket.StatusNormalClosure, "done")

	writeClientEnvelope(t, conn, ClientEnvelope{Version: ProtocolVersion, Type: "PING", RequestID: "ping-1"})
	response := readServerEnvelope(t, conn)

	if response.Type != MessagePong {
		t.Fatalf("message type got %s, want %s", response.Type, MessagePong)
	}
	if response.Sequence != 1 {
		t.Fatalf("sequence got %d, want 1", response.Sequence)
	}
}

func TestWebSocketCreateRoom(t *testing.T) {
	conn, closeServer := newTestWebSocket(t)
	defer closeServer()
	defer conn.Close(websocket.StatusNormalClosure, "done")

	payload, err := json.Marshal(CreateRoomPayload{PlayerName: "Alice"})
	if err != nil {
		t.Fatalf("Marshal payload returned error: %v", err)
	}
	writeClientEnvelope(t, conn, ClientEnvelope{Version: ProtocolVersion, Type: "CREATE_ROOM", RequestID: "create-1", Payload: payload})
	response := readServerEnvelope(t, conn)

	if response.Type != MessageRoomCreated {
		t.Fatalf("message type got %s, want %s", response.Type, MessageRoomCreated)
	}
	var roomSession RoomSessionPayload
	if err := json.Unmarshal(response.Payload, &roomSession); err != nil {
		t.Fatalf("Unmarshal room session returned error: %v", err)
	}
	if roomSession.RequestID != "create-1" || roomSession.RoomID == "" || roomSession.PlayerID == "" || roomSession.PlayerToken == "" || !roomSession.IsHost {
		t.Fatalf("unexpected room session payload: %#v", roomSession)
	}
}

func TestWebSocketCreateAndJoinRoom(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	creator := dialTestWebSocket(t, server.URL)
	defer creator.Close(websocket.StatusNormalClosure, "done")
	joiner := dialTestWebSocket(t, server.URL)
	defer joiner.Close(websocket.StatusNormalClosure, "done")

	createPayload, err := json.Marshal(CreateRoomPayload{PlayerName: "Alice"})
	if err != nil {
		t.Fatalf("Marshal create payload returned error: %v", err)
	}
	writeClientEnvelope(t, creator, ClientEnvelope{Version: ProtocolVersion, Type: "CREATE_ROOM", RequestID: "create-1", Payload: createPayload})
	createResponse := readServerEnvelope(t, creator)
	var created RoomSessionPayload
	if err := json.Unmarshal(createResponse.Payload, &created); err != nil {
		t.Fatalf("Unmarshal created payload returned error: %v", err)
	}

	joinPayload, err := json.Marshal(JoinRoomPayload{PlayerName: "Bob"})
	if err != nil {
		t.Fatalf("Marshal join payload returned error: %v", err)
	}
	writeClientEnvelope(t, joiner, ClientEnvelope{Version: ProtocolVersion, Type: "JOIN_ROOM", RequestID: "join-1", RoomID: created.RoomID, Payload: joinPayload})
	joinResponse := readServerEnvelope(t, joiner)
	if joinResponse.Type != MessageRoomJoined {
		t.Fatalf("message type got %s, want %s", joinResponse.Type, MessageRoomJoined)
	}

	roomValue, exists := manager.GetRoom(created.RoomID)
	if !exists {
		t.Fatal("created room should exist")
	}
	if roomValue.PlayerCount() != 2 {
		t.Fatalf("room player count got %d, want 2", roomValue.PlayerCount())
	}
}

func TestWebSocketReadyStartAndRoomViewBroadcast(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	hostConn := dialTestWebSocket(t, server.URL)
	defer hostConn.Close(websocket.StatusNormalClosure, "done")
	joinerConn := dialTestWebSocket(t, server.URL)
	defer joinerConn.Close(websocket.StatusNormalClosure, "done")

	createPayload, err := json.Marshal(CreateRoomPayload{PlayerName: "Host"})
	if err != nil {
		t.Fatalf("Marshal create payload returned error: %v", err)
	}
	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "CREATE_ROOM", RequestID: "create-1", Payload: createPayload})
	createdEnvelope := readUntilServerEnvelope(t, hostConn, MessageRoomCreated)
	var hostSession RoomSessionPayload
	if err := json.Unmarshal(createdEnvelope.Payload, &hostSession); err != nil {
		t.Fatalf("Unmarshal host session returned error: %v", err)
	}
	readUntilServerEnvelope(t, hostConn, MessageRoomView)

	joinPayload, err := json.Marshal(JoinRoomPayload{PlayerName: "Joiner"})
	if err != nil {
		t.Fatalf("Marshal join payload returned error: %v", err)
	}
	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "JOIN_ROOM", RequestID: "join-1", RoomID: hostSession.RoomID, Payload: joinPayload})
	joinedEnvelope := readUntilServerEnvelope(t, joinerConn, MessageRoomJoined)
	var joinerSession RoomSessionPayload
	if err := json.Unmarshal(joinedEnvelope.Payload, &joinerSession); err != nil {
		t.Fatalf("Unmarshal joiner session returned error: %v", err)
	}
	joinerRoomViewEnvelope := readUntilServerEnvelope(t, joinerConn, MessageRoomView)
	var joinerRoomView room.RoomView
	if err := json.Unmarshal(joinerRoomViewEnvelope.Payload, &joinerRoomView); err != nil {
		t.Fatalf("Unmarshal room view returned error: %v", err)
	}
	if joinerRoomView.PlayerCount != 2 {
		t.Fatalf("room view player count got %d, want 2", joinerRoomView.PlayerCount)
	}

	readyPayload, err := json.Marshal(SetReadyPayload{Ready: true})
	if err != nil {
		t.Fatalf("Marshal ready payload returned error: %v", err)
	}
	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "SET_READY", RequestID: "ready-host", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: readyPayload})
	readUntilServerEnvelope(t, hostConn, MessageCommandAck)
	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "SET_READY", RequestID: "ready-joiner", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: readyPayload})
	readUntilServerEnvelope(t, joinerConn, MessageCommandAck)

	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "START_GAME", RequestID: "bad-start", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken})
	errorEnvelope := readUntilServerEnvelope(t, joinerConn, MessageCommandError)
	var errorPayload CommandErrorPayload
	if err := json.Unmarshal(errorEnvelope.Payload, &errorPayload); err != nil {
		t.Fatalf("Unmarshal error payload returned error: %v", err)
	}
	if errorPayload.Code != "NOT_HOST" {
		t.Fatalf("error code got %s, want NOT_HOST", errorPayload.Code)
	}

	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "START_GAME", RequestID: "start", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken})
	readUntilServerEnvelope(t, hostConn, MessageGameStarted)
	hostGameViewEnvelope := readUntilServerEnvelope(t, hostConn, MessageGameView)
	var hostGameView game.PlayerGameView
	if err := json.Unmarshal(hostGameViewEnvelope.Payload, &hostGameView); err != nil {
		t.Fatalf("Unmarshal host game view returned error: %v", err)
	}
	if hostGameView.You.ID != hostSession.PlayerID || len(hostGameView.You.Hand) == 0 || hostGameView.DrawPileCount == 0 {
		t.Fatalf("unexpected host game view: %#v", hostGameView)
	}

	joinerGameViewEnvelope := readUntilServerEnvelope(t, joinerConn, MessageGameView)
	var joinerGameView game.PlayerGameView
	if err := json.Unmarshal(joinerGameViewEnvelope.Payload, &joinerGameView); err != nil {
		t.Fatalf("Unmarshal joiner game view returned error: %v", err)
	}
	if joinerGameView.You.ID != joinerSession.PlayerID || len(joinerGameView.You.Hand) == 0 || joinerGameView.DrawPileCount == 0 {
		t.Fatalf("unexpected joiner game view: %#v", joinerGameView)
	}
}

func TestWebSocketDrawAndPlayCardCommands(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	hostConn := dialTestWebSocket(t, server.URL)
	defer hostConn.Close(websocket.StatusNormalClosure, "done")
	joinerConn := dialTestWebSocket(t, server.URL)
	defer joinerConn.Close(websocket.StatusNormalClosure, "done")
	hostSession, _ := createAndJoinRoomOverWebSocket(t, hostConn, joinerConn)

	if _, err := manager.StartGameWithoutAuth(hostSession.RoomID); err != nil {
		t.Fatalf("StartGameWithoutAuth returned error: %v", err)
	}
	roomValue, exists := manager.GetRoom(hostSession.RoomID)
	if !exists {
		t.Fatal("room should exist")
	}
	state := roomValue.State()
	hostPlayerIndex := -1
	for i, player := range state.Players {
		if player.ID == hostSession.PlayerID {
			hostPlayerIndex = i
			break
		}
	}
	if hostPlayerIndex < 0 {
		t.Fatal("host should be in game state")
	}

	state.Phase = game.PhasePlayerTurn
	state.CurrentPlayerID = hostSession.PlayerID
	state.TurnDebt[hostSession.PlayerID] = 1
	state.DrawPile = []game.Card{{ID: "draw-transport-1", Code: game.CardShield}}
	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "DRAW_CARD", RequestID: "draw", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken})
	readUntilServerEnvelope(t, hostConn, MessageCommandAck)
	drawEventsEnvelope := readUntilServerEnvelope(t, hostConn, MessageGameEvents)
	var drawEvents struct {
		Events []game.Event `json:"events"`
	}
	if err := json.Unmarshal(drawEventsEnvelope.Payload, &drawEvents); err != nil {
		t.Fatalf("Unmarshal draw events returned error: %v", err)
	}
	if len(drawEvents.Events) == 0 || drawEvents.Events[0].Type != game.EventCardDrawn {
		t.Fatalf("unexpected draw events: %#v", drawEvents.Events)
	}
	readUntilServerEnvelope(t, hostConn, MessageGameView)

	state.Phase = game.PhasePlayerTurn
	state.CurrentPlayerID = hostSession.PlayerID
	state.TurnDebt[hostSession.PlayerID] = 1
	state.Players[hostPlayerIndex].Hand = append(state.Players[hostPlayerIndex].Hand, game.Card{ID: "skip-transport-1", Code: game.CardSkipTurn})
	playPayload, err := json.Marshal(PlayCardPayload{CardIDs: []string{"skip-transport-1"}})
	if err != nil {
		t.Fatalf("Marshal play payload returned error: %v", err)
	}
	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLAY_CARD", RequestID: "play", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: playPayload})
	readUntilServerEnvelope(t, hostConn, MessageCommandAck)
	playEventsEnvelope := readUntilServerEnvelope(t, hostConn, MessageGameEvents)
	var playEvents struct {
		Events []game.Event `json:"events"`
	}
	if err := json.Unmarshal(playEventsEnvelope.Payload, &playEvents); err != nil {
		t.Fatalf("Unmarshal play events returned error: %v", err)
	}
	if len(playEvents.Events) < 2 || playEvents.Events[0].Type != game.EventCardPlayed || playEvents.Events[1].Type != game.EventActionPending {
		t.Fatalf("unexpected play events: %#v", playEvents.Events)
	}
	viewEnvelope := readUntilServerEnvelope(t, hostConn, MessageGameView)
	var view game.PlayerGameView
	if err := json.Unmarshal(viewEnvelope.Payload, &view); err != nil {
		t.Fatalf("Unmarshal game view returned error: %v", err)
	}
	if view.PendingAction == nil || view.PendingAction.Type != game.PendingPlayCard {
		t.Fatalf("pending action got %#v, want PLAY_CARD", view.PendingAction)
	}
}

func TestWebSocketExplosivePlacementIsAuthorizedAndPrivate(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	hostConn := dialTestWebSocket(t, server.URL)
	defer hostConn.Close(websocket.StatusNormalClosure, "done")
	joinerConn := dialTestWebSocket(t, server.URL)
	defer joinerConn.Close(websocket.StatusNormalClosure, "done")
	hostSession, joinerSession := createAndJoinRoomOverWebSocket(t, hostConn, joinerConn)

	if _, err := manager.StartGameWithoutAuth(hostSession.RoomID); err != nil {
		t.Fatalf("StartGameWithoutAuth returned error: %v", err)
	}
	roomValue, exists := manager.GetRoom(hostSession.RoomID)
	if !exists {
		t.Fatal("room should exist")
	}
	state := roomValue.State()
	hostIndex := -1
	for i, player := range state.Players {
		if player.ID == hostSession.PlayerID {
			hostIndex = i
			break
		}
	}
	if hostIndex < 0 {
		t.Fatal("host should be in game state")
	}

	state.Phase = game.PhasePlayerTurn
	state.CurrentPlayerID = hostSession.PlayerID
	state.TurnDebt[hostSession.PlayerID] = 1
	state.Players[hostIndex].Hand = []game.Card{{ID: "shield-transport-1", Code: game.CardShield}}
	state.DrawPile = []game.Card{
		{ID: "danger-transport-1", Code: game.CardExplosive},
		{ID: "draw-transport-1", Code: game.CardSkipTurn},
	}

	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "DRAW_CARD", RequestID: "draw-danger", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken})
	readUntilServerEnvelope(t, hostConn, MessageCommandAck)
	readUntilServerEnvelope(t, hostConn, MessageGameEvents)
	hostPlacementView := readUntilServerEnvelope(t, hostConn, MessageGameView)
	var hostView game.PlayerGameView
	if err := json.Unmarshal(hostPlacementView.Payload, &hostView); err != nil {
		t.Fatalf("Unmarshal host placement view returned error: %v", err)
	}
	if !containsCommand(hostView.AvailableActions, game.CommandPlaceExplosive) {
		t.Fatalf("host actions got %#v, want PLACE_EXPLOSIVE", hostView.AvailableActions)
	}

	joinerEvents := readUntilServerEnvelope(t, joinerConn, MessageGameEvents)
	joinerPlacementView := readUntilServerEnvelope(t, joinerConn, MessageGameView)
	if strings.Contains(string(joinerEvents.Payload), "index") || strings.Contains(string(joinerPlacementView.Payload), "index") {
		t.Fatal("explosive placement index must not be broadcast")
	}
	var joinerView game.PlayerGameView
	if err := json.Unmarshal(joinerPlacementView.Payload, &joinerView); err != nil {
		t.Fatalf("Unmarshal joiner placement view returned error: %v", err)
	}
	if containsCommand(joinerView.AvailableActions, game.CommandPlaceExplosive) {
		t.Fatalf("joiner actions got %#v, must not include PLACE_EXPLOSIVE", joinerView.AvailableActions)
	}

	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLACE_EXPLOSIVE", RequestID: "place-without-index", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: json.RawMessage(`{}`)})
	missingIndexResponse := readUntilServerEnvelope(t, hostConn, MessageCommandError)
	var missingIndexError CommandErrorPayload
	if err := json.Unmarshal(missingIndexResponse.Payload, &missingIndexError); err != nil {
		t.Fatalf("Unmarshal missing-index error returned error: %v", err)
	}
	if missingIndexError.Code != "INVALID_PAYLOAD" {
		t.Fatalf("missing-index error code got %s, want INVALID_PAYLOAD", missingIndexError.Code)
	}

	placementIndex := 1
	placementPayload, err := json.Marshal(PlaceExplosivePayload{Index: &placementIndex})
	if err != nil {
		t.Fatalf("Marshal placement payload returned error: %v", err)
	}
	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLACE_EXPLOSIVE", RequestID: "place-other-player", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: placementPayload})
	invalidResponse := readUntilServerEnvelope(t, joinerConn, MessageCommandError)
	var invalidError CommandErrorPayload
	if err := json.Unmarshal(invalidResponse.Payload, &invalidError); err != nil {
		t.Fatalf("Unmarshal placement error returned error: %v", err)
	}
	if invalidError.Code != "NOT_YOUR_TURN" {
		t.Fatalf("placement error code got %s, want NOT_YOUR_TURN", invalidError.Code)
	}

	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLACE_EXPLOSIVE", RequestID: "place-danger", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: placementPayload})
	readUntilServerEnvelope(t, hostConn, MessageCommandAck)
	readUntilServerEnvelope(t, hostConn, MessageGameEvents)
	readUntilServerEnvelope(t, hostConn, MessageGameView)
	joinerResolvedEvents := readUntilServerEnvelope(t, joinerConn, MessageGameEvents)
	joinerResolvedView := readUntilServerEnvelope(t, joinerConn, MessageGameView)
	if strings.Contains(string(joinerResolvedEvents.Payload), "index") || strings.Contains(string(joinerResolvedView.Payload), "index") {
		t.Fatal("resolved explosive placement index must not be broadcast")
	}
	if state.DrawPile[1].ID != "danger-transport-1" {
		t.Fatalf("explosive got %s, want danger-transport-1 at placement index", state.DrawPile[1].ID)
	}
}

func TestWebSocketPlayComboAndDiscardRecoveryCommands(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	hostConn := dialTestWebSocket(t, server.URL)
	defer hostConn.Close(websocket.StatusNormalClosure, "done")
	joinerConn := dialTestWebSocket(t, server.URL)
	defer joinerConn.Close(websocket.StatusNormalClosure, "done")
	hostSession, joinerSession := createAndJoinRoomOverWebSocket(t, hostConn, joinerConn)

	if _, err := manager.StartGameWithoutAuth(hostSession.RoomID); err != nil {
		t.Fatalf("StartGameWithoutAuth returned error: %v", err)
	}
	roomValue, exists := manager.GetRoom(hostSession.RoomID)
	if !exists {
		t.Fatal("room should exist")
	}
	state := roomValue.State()
	hostIndex, joinerIndex := -1, -1
	for i, player := range state.Players {
		switch player.ID {
		case hostSession.PlayerID:
			hostIndex = i
		case joinerSession.PlayerID:
			joinerIndex = i
		}
	}
	if hostIndex < 0 || joinerIndex < 0 {
		t.Fatal("host and joiner should be in game state")
	}
	state.Phase = game.PhasePlayerTurn
	state.CurrentPlayerID = hostSession.PlayerID
	state.TurnDebt[hostSession.PlayerID] = 1
	state.Players[hostIndex].Hand = []game.Card{{ID: "combo-1", Code: game.CardSkipTurn}, {ID: "combo-2", Code: game.CardSkipTurn}}
	state.Players[joinerIndex].Hand = []game.Card{{ID: "target-secret-1", Code: game.CardShuffleDeck}}

	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLAY_COMBO", RequestID: "bad-combo", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: json.RawMessage(`"bad"`)})
	badComboResponse := readUntilServerEnvelope(t, hostConn, MessageCommandError)
	var badComboError CommandErrorPayload
	if err := json.Unmarshal(badComboResponse.Payload, &badComboError); err != nil {
		t.Fatalf("Unmarshal combo payload error returned error: %v", err)
	}
	if badComboError.Code != "INVALID_PAYLOAD" {
		t.Fatalf("combo payload error code got %s, want INVALID_PAYLOAD", badComboError.Code)
	}

	comboPayload, err := json.Marshal(PlayComboPayload{CardIDs: []string{"combo-1", "combo-2"}, TargetID: joinerSession.PlayerID})
	if err != nil {
		t.Fatalf("Marshal combo payload returned error: %v", err)
	}
	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLAY_COMBO", RequestID: "play-combo", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: comboPayload})
	readUntilServerEnvelope(t, hostConn, MessageCommandAck)
	hostComboEvents := readUntilServerEnvelope(t, hostConn, MessageGameEvents)
	if strings.Contains(string(hostComboEvents.Payload), "target-secret-1") {
		t.Fatal("combo event must not reveal target hand card identity")
	}
	hostComboView := readUntilServerEnvelope(t, hostConn, MessageGameView)
	var comboView game.PlayerGameView
	if err := json.Unmarshal(hostComboView.Payload, &comboView); err != nil {
		t.Fatalf("Unmarshal combo view returned error: %v", err)
	}
	if comboView.PendingAction == nil || comboView.PendingAction.ComboKind != game.ComboPair {
		t.Fatalf("combo view pending action got %#v, want pair", comboView.PendingAction)
	}

	state.Phase = game.PhaseWaitingDiscardRecovery
	state.PendingAction = &game.PendingAction{ID: "recovery-pending-1", SourcePlayerID: hostSession.PlayerID, Type: game.PendingDiscardRecovery}
	state.DiscardPile = []game.Card{{ID: "recover-public-1", Code: game.CardPeekDeck}}

	recoveryPayload, err := json.Marshal(ChooseCardFromDiscardPayload{CardID: "recover-public-1"})
	if err != nil {
		t.Fatalf("Marshal recovery payload returned error: %v", err)
	}
	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "CHOOSE_CARD_FROM_DISCARD", RequestID: "recover-as-other", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: recoveryPayload})
	notSourceResponse := readUntilServerEnvelope(t, joinerConn, MessageCommandError)
	var notSourceError CommandErrorPayload
	if err := json.Unmarshal(notSourceResponse.Payload, &notSourceError); err != nil {
		t.Fatalf("Unmarshal recovery authorization error returned error: %v", err)
	}
	if notSourceError.Code != "NOT_YOUR_TURN" {
		t.Fatalf("recovery authorization code got %s, want NOT_YOUR_TURN", notSourceError.Code)
	}

	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "CHOOSE_CARD_FROM_DISCARD", RequestID: "recover-card", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: recoveryPayload})
	readUntilServerEnvelope(t, hostConn, MessageCommandAck)
	readUntilServerEnvelope(t, hostConn, MessageGameEvents)
	hostRecoveryView := readUntilServerEnvelope(t, hostConn, MessageGameView)
	var recoveryView game.PlayerGameView
	if err := json.Unmarshal(hostRecoveryView.Payload, &recoveryView); err != nil {
		t.Fatalf("Unmarshal recovery view returned error: %v", err)
	}
	foundRecovered := false
	for _, card := range recoveryView.You.Hand {
		if card.ID == "recover-public-1" {
			foundRecovered = true
		}
	}
	if !foundRecovered {
		t.Fatalf("source hand got %#v, want recovered discard card", recoveryView.You.Hand)
	}
}

func TestWebSocketPlayCancelUpdatesCancelWindow(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	hostConn := dialTestWebSocket(t, server.URL)
	defer hostConn.Close(websocket.StatusNormalClosure, "done")
	joinerConn := dialTestWebSocket(t, server.URL)
	defer joinerConn.Close(websocket.StatusNormalClosure, "done")
	hostSession, joinerSession := createAndJoinRoomOverWebSocket(t, hostConn, joinerConn)

	if _, err := manager.StartGameWithoutAuth(hostSession.RoomID); err != nil {
		t.Fatalf("StartGameWithoutAuth returned error: %v", err)
	}
	roomValue, exists := manager.GetRoom(hostSession.RoomID)
	if !exists {
		t.Fatal("room should exist")
	}
	state := roomValue.State()
	joinerIndex := -1
	for i, player := range state.Players {
		if player.ID == joinerSession.PlayerID {
			joinerIndex = i
			break
		}
	}
	if joinerIndex < 0 {
		t.Fatal("joiner should be in game state")
	}
	state.Phase = game.PhaseCancelWindow
	state.CurrentPlayerID = hostSession.PlayerID
	state.PendingAction = &game.PendingAction{
		ID:              "cancel-pending-1",
		SourcePlayerID:  hostSession.PlayerID,
		TargetPlayerID:  joinerSession.PlayerID,
		Type:            game.PendingPlayCard,
		Cards:           []game.Card{{ID: "skip-pending-1", Code: game.CardSkipTurn}},
		ExpiresAtUnixMs: time.Now().Add(time.Minute).UnixMilli(),
	}
	state.Players[joinerIndex].Hand = []game.Card{{ID: "cancel-transport-1", Code: game.CardCancel}}

	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLAY_CANCEL", RequestID: "bad-cancel", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: json.RawMessage(`"bad"`)})
	badPayloadResponse := readUntilServerEnvelope(t, joinerConn, MessageCommandError)
	var badPayloadError CommandErrorPayload
	if err := json.Unmarshal(badPayloadResponse.Payload, &badPayloadError); err != nil {
		t.Fatalf("Unmarshal cancel payload error returned error: %v", err)
	}
	if badPayloadError.Code != "INVALID_PAYLOAD" {
		t.Fatalf("cancel payload error code got %s, want INVALID_PAYLOAD", badPayloadError.Code)
	}

	payload, err := json.Marshal(PlayCancelPayload{CardID: "cancel-transport-1", PendingActionID: "cancel-pending-1"})
	if err != nil {
		t.Fatalf("Marshal cancel payload returned error: %v", err)
	}
	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLAY_CANCEL", RequestID: "play-cancel", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: payload})
	readUntilServerEnvelope(t, joinerConn, MessageCommandAck)
	readUntilServerEnvelope(t, joinerConn, MessageGameEvents)
	readUntilServerEnvelope(t, joinerConn, MessageGameView)
	readUntilServerEnvelope(t, hostConn, MessageGameEvents)
	hostViewEnvelope := readUntilServerEnvelope(t, hostConn, MessageGameView)
	var hostView game.PlayerGameView
	if err := json.Unmarshal(hostViewEnvelope.Payload, &hostView); err != nil {
		t.Fatalf("Unmarshal host cancel view returned error: %v", err)
	}
	if hostView.PendingAction == nil || hostView.PendingAction.CancelCount != 1 {
		t.Fatalf("cancel view got %#v, want count 1", hostView.PendingAction)
	}

	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "PLAY_CANCEL", RequestID: "duplicate-cancel", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: payload})
	duplicateResponse := readUntilServerEnvelope(t, joinerConn, MessageCommandError)
	var duplicateError CommandErrorPayload
	if err := json.Unmarshal(duplicateResponse.Payload, &duplicateError); err != nil {
		t.Fatalf("Unmarshal duplicate cancel error returned error: %v", err)
	}
	if duplicateError.Code != "CARD_NOT_IN_HAND" || state.PendingAction.CancelCount != 1 {
		t.Fatalf("duplicate cancel got code=%s count=%d, want CARD_NOT_IN_HAND and 1", duplicateError.Code, state.PendingAction.CancelCount)
	}
}

func TestWebSocketRequestCardChoiceTransfersWithoutPublicCardIdentity(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	hostConn := dialTestWebSocket(t, server.URL)
	defer hostConn.Close(websocket.StatusNormalClosure, "done")
	joinerConn := dialTestWebSocket(t, server.URL)
	defer joinerConn.Close(websocket.StatusNormalClosure, "done")
	hostSession, joinerSession := createAndJoinRoomOverWebSocket(t, hostConn, joinerConn)

	if _, err := manager.StartGameWithoutAuth(hostSession.RoomID); err != nil {
		t.Fatalf("StartGameWithoutAuth returned error: %v", err)
	}
	roomValue, exists := manager.GetRoom(hostSession.RoomID)
	if !exists {
		t.Fatal("room should exist")
	}
	state := roomValue.State()
	hostIndex, joinerIndex := -1, -1
	for i, player := range state.Players {
		switch player.ID {
		case hostSession.PlayerID:
			hostIndex = i
		case joinerSession.PlayerID:
			joinerIndex = i
		}
	}
	if hostIndex < 0 || joinerIndex < 0 {
		t.Fatal("host and joiner should be in game state")
	}
	state.Phase = game.PhaseWaitingRequestCardChoice
	state.CurrentPlayerID = hostSession.PlayerID
	state.PendingAction = &game.PendingAction{
		ID:             "request-pending-1",
		SourcePlayerID: hostSession.PlayerID,
		TargetPlayerID: joinerSession.PlayerID,
		Type:           game.PendingRequestCardChoice,
	}
	state.Players[hostIndex].Hand = nil
	state.Players[joinerIndex].Hand = []game.Card{{ID: "private-transfer-1", Code: game.CardSkipTurn}}

	payload, err := json.Marshal(ChooseCardForRequestPayload{CardID: "private-transfer-1"})
	if err != nil {
		t.Fatalf("Marshal request choice payload returned error: %v", err)
	}
	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "CHOOSE_CARD_FOR_REQUEST", RequestID: "choose-as-source", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: payload})
	notTargetResponse := readUntilServerEnvelope(t, hostConn, MessageCommandError)
	var notTargetError CommandErrorPayload
	if err := json.Unmarshal(notTargetResponse.Payload, &notTargetError); err != nil {
		t.Fatalf("Unmarshal request choice error returned error: %v", err)
	}
	if notTargetError.Code != "NOT_YOUR_TURN" {
		t.Fatalf("request choice error code got %s, want NOT_YOUR_TURN", notTargetError.Code)
	}

	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "CHOOSE_CARD_FOR_REQUEST", RequestID: "choose-without-card", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: json.RawMessage(`{}`)})
	missingCardResponse := readUntilServerEnvelope(t, joinerConn, MessageCommandError)
	var missingCardError CommandErrorPayload
	if err := json.Unmarshal(missingCardResponse.Payload, &missingCardError); err != nil {
		t.Fatalf("Unmarshal missing-card error returned error: %v", err)
	}
	if missingCardError.Code != "INVALID_PAYLOAD" {
		t.Fatalf("missing-card error code got %s, want INVALID_PAYLOAD", missingCardError.Code)
	}

	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "CHOOSE_CARD_FOR_REQUEST", RequestID: "choose-card", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: payload})
	readUntilServerEnvelope(t, joinerConn, MessageCommandAck)
	readUntilServerEnvelope(t, joinerConn, MessageGameEvents)
	readUntilServerEnvelope(t, joinerConn, MessageGameView)
	hostEvents := readUntilServerEnvelope(t, hostConn, MessageGameEvents)
	if strings.Contains(string(hostEvents.Payload), "private-transfer-1") || strings.Contains(string(hostEvents.Payload), string(game.CardSkipTurn)) {
		t.Fatal("public request-transfer event must not reveal the selected card")
	}
	hostViewEnvelope := readUntilServerEnvelope(t, hostConn, MessageGameView)
	var hostView game.PlayerGameView
	if err := json.Unmarshal(hostViewEnvelope.Payload, &hostView); err != nil {
		t.Fatalf("Unmarshal host request view returned error: %v", err)
	}
	if len(hostView.You.Hand) != 1 || hostView.You.Hand[0].ID != "private-transfer-1" {
		t.Fatalf("source private hand got %#v, want transferred card", hostView.You.Hand)
	}
}

func TestWebSocketTransferHost(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	hostConn := dialTestWebSocket(t, server.URL)
	defer hostConn.Close(websocket.StatusNormalClosure, "done")
	joinerConn := dialTestWebSocket(t, server.URL)
	defer joinerConn.Close(websocket.StatusNormalClosure, "done")
	hostSession, joinerSession := createAndJoinRoomOverWebSocket(t, hostConn, joinerConn)

	transferPayload, err := json.Marshal(TransferHostPayload{TargetPlayerID: joinerSession.PlayerID})
	if err != nil {
		t.Fatalf("Marshal transfer payload returned error: %v", err)
	}
	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "TRANSFER_HOST", RequestID: "transfer", RoomID: hostSession.RoomID, PlayerToken: hostSession.PlayerToken, Payload: transferPayload})
	readUntilServerEnvelope(t, hostConn, MessageHostTransferred)
	viewEnvelope := readUntilServerEnvelope(t, hostConn, MessageRoomView)
	var view room.RoomView
	if err := json.Unmarshal(viewEnvelope.Payload, &view); err != nil {
		t.Fatalf("Unmarshal room view returned error: %v", err)
	}
	if view.HostPlayerID != joinerSession.PlayerID {
		t.Fatalf("host got %s, want %s", view.HostPlayerID, joinerSession.PlayerID)
	}
}

func TestWebSocketKickedPlayerReceivesNotification(t *testing.T) {
	manager := room.NewManager()
	server := httptest.NewServer(NewWebSocketHandler(manager, nil))
	defer server.Close()

	hostConn := dialTestWebSocket(t, server.URL)
	defer hostConn.Close(websocket.StatusNormalClosure, "done")
	joinerConn := dialTestWebSocket(t, server.URL)
	defer joinerConn.Close(websocket.StatusNormalClosure, "done")
	hostSession, joinerSession := createAndJoinRoomOverWebSocket(t, hostConn, joinerConn)

	kickPayload, err := json.Marshal(StartKickVotePayload{TargetPlayerID: hostSession.PlayerID})
	if err != nil {
		t.Fatalf("Marshal kick payload returned error: %v", err)
	}
	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "START_KICK_VOTE", RequestID: "kick-host", RoomID: hostSession.RoomID, PlayerToken: joinerSession.PlayerToken, Payload: kickPayload})

	kickedEnvelope := readUntilServerEnvelope(t, hostConn, MessageKickedFromRoom)
	var kickedPayload map[string]any
	if err := json.Unmarshal(kickedEnvelope.Payload, &kickedPayload); err != nil {
		t.Fatalf("Unmarshal kicked payload returned error: %v", err)
	}
	if kickedPayload["kickedPlayerId"] != hostSession.PlayerID || kickedPayload["roomId"] != hostSession.RoomID {
		t.Fatalf("unexpected kicked payload: %#v", kickedPayload)
	}

	resolvedEnvelope := readUntilServerEnvelope(t, joinerConn, MessageKickVoteResolved)
	var resolvedPayload map[string]any
	if err := json.Unmarshal(resolvedEnvelope.Payload, &resolvedPayload); err != nil {
		t.Fatalf("Unmarshal resolved payload returned error: %v", err)
	}
	if resolvedPayload["kickedPlayerId"] != hostSession.PlayerID {
		t.Fatalf("unexpected resolved payload: %#v", resolvedPayload)
	}
}

func TestWebSocketMalformedJSONReturnsErrorAndConnectionStaysOpen(t *testing.T) {
	conn, closeServer := newTestWebSocket(t)
	defer closeServer()
	defer conn.Close(websocket.StatusNormalClosure, "done")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte("{")); err != nil {
		t.Fatalf("Write malformed JSON returned error: %v", err)
	}
	response := readServerEnvelope(t, conn)
	if response.Type != MessageCommandError {
		t.Fatalf("message type got %s, want %s", response.Type, MessageCommandError)
	}

	writeClientEnvelope(t, conn, ClientEnvelope{Version: ProtocolVersion, Type: "PING", RequestID: "ping-after-error"})
	response = readServerEnvelope(t, conn)
	if response.Type != MessagePong {
		t.Fatalf("connection should stay open; got message type %s", response.Type)
	}
}

func TestWebSocketUnknownCommandReturnsError(t *testing.T) {
	conn, closeServer := newTestWebSocket(t)
	defer closeServer()
	defer conn.Close(websocket.StatusNormalClosure, "done")

	writeClientEnvelope(t, conn, ClientEnvelope{Version: ProtocolVersion, Type: "NOPE", RequestID: "bad-1"})
	response := readServerEnvelope(t, conn)
	if response.Type != MessageCommandError {
		t.Fatalf("message type got %s, want %s", response.Type, MessageCommandError)
	}

	var payload CommandErrorPayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("Unmarshal error payload returned error: %v", err)
	}
	if payload.RequestID != "bad-1" || payload.Code != "UNKNOWN_COMMAND" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func containsCommand(commands []game.CommandType, want game.CommandType) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}
	return false
}

func newTestWebSocket(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()
	server := httptest.NewServer(NewWebSocketHandler(room.NewManager(), nil))
	conn := dialTestWebSocket(t, server.URL)
	return conn, server.Close
}

func dialTestWebSocket(t *testing.T, httpURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(httpURL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	return conn
}

func writeClientEnvelope(t *testing.T, conn *websocket.Conn, envelope ClientEnvelope) {
	t.Helper()
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal envelope returned error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
}

func createAndJoinRoomOverWebSocket(t *testing.T, hostConn *websocket.Conn, joinerConn *websocket.Conn) (RoomSessionPayload, RoomSessionPayload) {
	t.Helper()
	createPayload, err := json.Marshal(CreateRoomPayload{PlayerName: "Host"})
	if err != nil {
		t.Fatalf("Marshal create payload returned error: %v", err)
	}
	writeClientEnvelope(t, hostConn, ClientEnvelope{Version: ProtocolVersion, Type: "CREATE_ROOM", RequestID: "create-helper", Payload: createPayload})
	createdEnvelope := readUntilServerEnvelope(t, hostConn, MessageRoomCreated)
	var hostSession RoomSessionPayload
	if err := json.Unmarshal(createdEnvelope.Payload, &hostSession); err != nil {
		t.Fatalf("Unmarshal host session returned error: %v", err)
	}
	readUntilServerEnvelope(t, hostConn, MessageRoomView)

	joinPayload, err := json.Marshal(JoinRoomPayload{PlayerName: "Joiner"})
	if err != nil {
		t.Fatalf("Marshal join payload returned error: %v", err)
	}
	writeClientEnvelope(t, joinerConn, ClientEnvelope{Version: ProtocolVersion, Type: "JOIN_ROOM", RequestID: "join-helper", RoomID: hostSession.RoomID, Payload: joinPayload})
	joinedEnvelope := readUntilServerEnvelope(t, joinerConn, MessageRoomJoined)
	var joinerSession RoomSessionPayload
	if err := json.Unmarshal(joinedEnvelope.Payload, &joinerSession); err != nil {
		t.Fatalf("Unmarshal joiner session returned error: %v", err)
	}
	readUntilServerEnvelope(t, joinerConn, MessageRoomView)
	readUntilServerEnvelope(t, hostConn, MessageRoomView)
	return hostSession, joinerSession
}

func readUntilServerEnvelope(t *testing.T, conn *websocket.Conn, messageType string) testServerEnvelope {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		response := readServerEnvelope(t, conn)
		if response.Type == messageType {
			return response
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", messageType)
		}
	}
}

func readServerEnvelope(t *testing.T, conn *websocket.Conn) testServerEnvelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type got %v, want text", messageType)
	}
	var envelope testServerEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("Unmarshal server envelope returned error: %v; data=%s", err, string(data))
	}
	if envelope.Version != ProtocolVersion {
		t.Fatalf("version got %d, want %d", envelope.Version, ProtocolVersion)
	}
	return envelope
}
