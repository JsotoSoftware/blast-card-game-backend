package room

import (
	"sync"
	"time"

	"exploding-game/server/internal/game"
)

const MaxPlayers = game.MaxPlayers

type Player struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	Ready     bool   `json:"ready"`
	Connected bool   `json:"connected"`
	IsHost    bool   `json:"isHost"`
}

type PlayerView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Connected bool   `json:"connected"`
	IsHost    bool   `json:"isHost"`
}

type RoomView struct {
	ID           string        `json:"id"`
	HostPlayerID string        `json:"hostPlayerId,omitempty"`
	Players      []PlayerView  `json:"players"`
	PlayerCount  int           `json:"playerCount"`
	MaxPlayers   int           `json:"maxPlayers"`
	GameStarted  bool          `json:"gameStarted"`
	KickVote     *KickVoteView `json:"kickVote,omitempty"`
}

type KickVote struct {
	TargetPlayerID    string
	StartedByPlayerID string
	Approvals         map[string]bool
}

type KickVoteView struct {
	TargetPlayerID    string   `json:"targetPlayerId"`
	StartedByPlayerID string   `json:"startedByPlayerId"`
	Approvals         []string `json:"approvals"`
	RequiredApprovals int      `json:"requiredApprovals"`
}

type Room struct {
	mu           sync.RWMutex
	id           string
	players      map[string]*Player
	playerOrder  []string
	hostPlayerID string
	engine       *game.Engine
	state        *game.GameState
	kickVote     *KickVote
}

func newRoom(id string, host *Player, engine *game.Engine) *Room {
	host.IsHost = true
	return &Room{
		id:           id,
		players:      map[string]*Player{host.ID: host},
		playerOrder:  []string{host.ID},
		hostPlayerID: host.ID,
		engine:       engine,
	}
}

func (r *Room) ID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.id
}

func (r *Room) HostPlayerID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hostPlayerID
}

func (r *Room) PlayerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.players)
}

func (r *Room) State() *game.GameState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *Room) Players() []Player {
	r.mu.RLock()
	defer r.mu.RUnlock()

	players := make([]Player, 0, len(r.players))
	for _, player := range r.players {
		players = append(players, *player)
	}
	return players
}

func (r *Room) View() RoomView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.viewLocked()
}

func (r *Room) GameViews() (map[string]game.PlayerGameView, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.state == nil {
		return nil, nil
	}
	views := make(map[string]game.PlayerGameView, len(r.players))
	for playerID := range r.players {
		view, err := game.BuildViewForPlayer(r.state, playerID)
		if err != nil {
			return nil, err
		}
		views[playerID] = view
	}
	return views, nil
}

func (r *Room) Join(player *Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.players) >= MaxPlayers {
		return ErrRoomFull
	}
	if _, exists := r.players[player.ID]; exists {
		return ErrPlayerAlreadyInRoom
	}

	player.Connected = true
	r.players[player.ID] = player
	r.playerOrder = append(r.playerOrder, player.ID)
	return nil
}

func (r *Room) Leave(playerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.players[playerID]; !exists {
		return ErrPlayerNotInRoom
	}

	delete(r.players, playerID)
	r.removePlayerFromOrderLocked(playerID)
	if r.kickVote != nil {
		delete(r.kickVote.Approvals, playerID)
		if r.kickVote.TargetPlayerID == playerID {
			r.kickVote = nil
		}
	}
	if r.hostPlayerID == playerID {
		r.transferHostToAnyLocked()
	}
	return nil
}

func (r *Room) PlayerIDForToken(token string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	player := r.playerByTokenLocked(token)
	if player == nil {
		return "", ErrInvalidPlayerToken
	}
	return player.ID, nil
}

func (r *Room) SetReady(playerToken string, ready bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return ErrInvalidPlayerToken
	}
	player.Ready = ready
	return nil
}

func (r *Room) TransferHost(playerToken string, targetPlayerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return ErrInvalidPlayerToken
	}
	if player.ID != r.hostPlayerID {
		return ErrNotHost
	}
	target := r.players[targetPlayerID]
	if target == nil || !target.Connected {
		return ErrInvalidHostTransfer
	}
	r.setHostLocked(targetPlayerID)
	return nil
}

func (r *Room) StartGame(playerToken string) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if player.ID != r.hostPlayerID {
		return nil, ErrNotHost
	}
	if err := r.validateCanStartLocked(true); err != nil {
		return nil, err
	}
	return r.startGameLocked()
}

func (r *Room) StartGameWithoutAuth() ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.validateCanStartLocked(false); err != nil {
		return nil, err
	}
	return r.startGameLocked()
}

func (r *Room) validateCanStartLocked(requireReady bool) error {
	if len(r.players) < game.MinPlayers || len(r.players) > game.MaxPlayers {
		return game.ErrInvalidPlayerCount
	}
	if requireReady {
		for _, player := range r.players {
			if !player.Ready {
				return ErrPlayersNotReady
			}
		}
	}
	return nil
}

func (r *Room) startGameLocked() ([]game.Event, error) {
	players := make([]game.Player, 0, len(r.playerOrder))
	for _, playerID := range r.playerOrder {
		player := r.players[playerID]
		players = append(players, game.Player{
			ID:        player.ID,
			Name:      player.Name,
			Alive:     true,
			Connected: player.Connected,
			Ready:     player.Ready,
			IsHost:    player.IsHost,
		})
	}

	state, events, err := r.engine.StartGame(r.id, players)
	if err != nil {
		return nil, err
	}
	r.state = state
	return events, nil
}

func (r *Room) StartKickVote(playerToken string, targetPlayerID string) (bool, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return false, "", ErrInvalidPlayerToken
	}
	if targetPlayerID == player.ID {
		return false, "", ErrCannotVoteKickSelf
	}
	if r.kickVote != nil {
		return false, "", ErrKickVoteAlreadyActive
	}
	target := r.players[targetPlayerID]
	if target == nil || !target.Connected {
		return false, "", ErrInvalidKickVoteTarget
	}

	r.kickVote = &KickVote{
		TargetPlayerID:    targetPlayerID,
		StartedByPlayerID: player.ID,
		Approvals:         map[string]bool{player.ID: true},
	}
	return r.resolveKickVoteIfPassedLocked()
}

func (r *Room) CastKickVote(playerToken string, approve bool) (bool, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return false, "", ErrInvalidPlayerToken
	}
	if r.kickVote == nil {
		return false, "", ErrNoKickVoteActive
	}
	if player.ID == r.kickVote.TargetPlayerID {
		return false, "", ErrInvalidKickVoteTarget
	}
	if approve {
		r.kickVote.Approvals[player.ID] = true
	} else {
		delete(r.kickVote.Approvals, player.ID)
	}
	return r.resolveKickVoteIfPassedLocked()
}

func (r *Room) DrawCard(playerToken string) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.DrawCard(r.state, game.DrawCardCommand{PlayerID: player.ID})
}

func (r *Room) PlayCard(playerToken string, cardIDs []string, targetPlayerID string) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.PlayCard(r.state, game.PlayCardCommand{PlayerID: player.ID, CardIDs: cardIDs, TargetID: targetPlayerID})
}

func (r *Room) PlaceExplosive(playerToken string, index int) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.PlaceExplosive(r.state, game.PlaceExplosiveCommand{PlayerID: player.ID, Index: index})
}

func (r *Room) ChooseCardForRequest(playerToken string, cardID string) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.ChooseCardForRequest(r.state, game.ChooseCardForRequestCommand{PlayerID: player.ID, CardID: cardID})
}

func (r *Room) PlayCancel(playerToken string, cardID string, pendingActionID string) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.PlayCancel(r.state, game.PlayCancelCommand{PlayerID: player.ID, CardID: cardID, PendingActionID: pendingActionID})
}

func (r *Room) PlayCombo(playerToken string, cardIDs []string, targetPlayerID string, requestedCode game.CardCode) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.PlayCombo(r.state, game.PlayComboCommand{PlayerID: player.ID, CardIDs: cardIDs, TargetID: targetPlayerID, RequestedCode: requestedCode})
}

func (r *Room) ChooseMarkedCard(playerToken string, cardID string) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.ChooseMarkedCard(r.state, game.ChooseMarkedCardCommand{PlayerID: player.ID, CardID: cardID})
}

func (r *Room) ChooseCardForRecycle(playerToken string, cardID string) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.ChooseCardForRecycle(r.state, game.ChooseCardForRecycleCommand{PlayerID: player.ID, CardID: cardID})
}

func (r *Room) ChooseCardFromDiscard(playerToken string, cardID string) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.playerByTokenLocked(playerToken)
	if player == nil {
		return nil, ErrInvalidPlayerToken
	}
	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.ChooseCardFromDiscard(r.state, game.ChooseCardFromDiscardCommand{PlayerID: player.ID, CardID: cardID})
}

func (r *Room) ExpireCancelWindow(now time.Time) ([]game.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == nil {
		return nil, ErrGameNotStarted
	}
	return r.engine.ExpireCancelWindow(r.state, now)
}

func (r *Room) resolveKickVoteIfPassedLocked() (bool, string, error) {
	if r.kickVote == nil {
		return false, "", nil
	}
	if len(r.kickVote.Approvals) < r.requiredKickApprovalsLocked() {
		return false, "", nil
	}

	targetID := r.kickVote.TargetPlayerID
	r.kickVote = nil
	delete(r.players, targetID)
	r.removePlayerFromOrderLocked(targetID)
	if r.hostPlayerID == targetID {
		r.transferHostToAnyLocked()
	}
	return true, targetID, nil
}

func (r *Room) removePlayerFromOrderLocked(playerID string) {
	for i, id := range r.playerOrder {
		if id == playerID {
			r.playerOrder = append(r.playerOrder[:i], r.playerOrder[i+1:]...)
			return
		}
	}
}

func (r *Room) requiredKickApprovalsLocked() int {
	eligible := 0
	for _, player := range r.players {
		if player.Connected && r.kickVote != nil && player.ID != r.kickVote.TargetPlayerID {
			eligible++
		}
	}
	return eligible/2 + 1
}

func (r *Room) playerByTokenLocked(token string) *Player {
	if token == "" {
		return nil
	}
	for _, player := range r.players {
		if player.Token == token {
			return player
		}
	}
	return nil
}

func (r *Room) transferHostToAnyLocked() {
	r.hostPlayerID = ""
	for _, player := range r.players {
		if player.Connected {
			r.setHostLocked(player.ID)
			return
		}
	}
}

func (r *Room) setHostLocked(playerID string) {
	for _, player := range r.players {
		player.IsHost = player.ID == playerID
	}
	r.hostPlayerID = playerID
}

func (r *Room) viewLocked() RoomView {
	players := make([]PlayerView, 0, len(r.players))
	for _, player := range r.players {
		players = append(players, PlayerView{
			ID:        player.ID,
			Name:      player.Name,
			Ready:     player.Ready,
			Connected: player.Connected,
			IsHost:    player.IsHost,
		})
	}

	view := RoomView{
		ID:           r.id,
		HostPlayerID: r.hostPlayerID,
		Players:      players,
		PlayerCount:  len(players),
		MaxPlayers:   MaxPlayers,
		GameStarted:  r.state != nil,
	}
	if r.kickVote != nil {
		approvals := make([]string, 0, len(r.kickVote.Approvals))
		for playerID := range r.kickVote.Approvals {
			approvals = append(approvals, playerID)
		}
		view.KickVote = &KickVoteView{
			TargetPlayerID:    r.kickVote.TargetPlayerID,
			StartedByPlayerID: r.kickVote.StartedByPlayerID,
			Approvals:         approvals,
			RequiredApprovals: r.requiredKickApprovalsLocked(),
		}
	}
	return view
}
