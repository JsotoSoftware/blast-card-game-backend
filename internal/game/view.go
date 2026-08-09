package game

import "sort"

type PlayerGameView struct {
	RoomID            string                   `json:"roomId"`
	Phase             GamePhase                `json:"phase"`
	You               PlayerPrivateView        `json:"you"`
	Players           []PlayerPublicView       `json:"players"`
	DrawPileCount     int                      `json:"drawPileCount"`
	DiscardPile       []PublicCardView         `json:"discardPile"`
	CurrentPlayerID   string                   `json:"currentPlayerId"`
	PendingAction     *PublicPendingActionView `json:"pendingAction,omitempty"`
	PublicMarkedCards []PublicMarkedCardView   `json:"publicMarkedCards"`
	AvailableActions  []CommandType            `json:"availableActions"`
}

type PlayerPrivateView struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Hand    []PrivateCardView `json:"hand"`
	Alive   bool              `json:"alive"`
	Blinded bool              `json:"blinded"`
}

type PlayerPublicView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HandCount int    `json:"handCount"`
	Alive     bool   `json:"alive"`
	Connected bool   `json:"connected"`
	Ready     bool   `json:"ready"`
	IsHost    bool   `json:"isHost"`
	Blinded   bool   `json:"blinded"`
}

type PrivateCardView struct {
	ID       string   `json:"id"`
	Code     CardCode `json:"code,omitempty"`
	IsHidden bool     `json:"isHidden"`
}

type PublicCardView struct {
	ID   string   `json:"id"`
	Code CardCode `json:"code"`
}

type PublicPendingActionView struct {
	ID              string            `json:"id"`
	SourcePlayerID  string            `json:"sourcePlayerId"`
	Type            PendingActionType `json:"type"`
	TargetPlayerID  string            `json:"targetPlayerId,omitempty"`
	ComboKind       ComboKind         `json:"comboKind,omitempty"`
	RequestedCode   CardCode          `json:"requestedCode,omitempty"`
	CancelCount     int               `json:"cancelCount"`
	ExpiresAtUnixMs int64             `json:"expiresAtUnixMs,omitempty"`
}

type PublicMarkedCardView struct {
	CardID  string   `json:"cardId"`
	OwnerID string   `json:"ownerId"`
	Code    CardCode `json:"code"`
}

func BuildViewForPlayer(state *GameState, playerID string) (PlayerGameView, error) {
	playerIndex := findPlayerIndexByID(state, playerID)
	if playerIndex < 0 {
		return PlayerGameView{}, ErrPlayerNotFound
	}

	player := state.Players[playerIndex]
	return PlayerGameView{
		RoomID:            state.RoomID,
		Phase:             state.Phase,
		You:               buildPrivatePlayerView(player),
		Players:           buildPublicPlayerViews(state.Players),
		DrawPileCount:     len(state.DrawPile),
		DiscardPile:       buildPublicCardViews(state.DiscardPile),
		CurrentPlayerID:   state.CurrentPlayerID,
		PendingAction:     buildPublicPendingActionView(state.PendingAction),
		PublicMarkedCards: buildPublicMarkedCardViews(state.MarkedCards),
		AvailableActions:  buildAvailableActions(state, player),
	}, nil
}

func FilterEventsForPlayer(events []Event, playerID string) []Event {
	filtered := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Type == EventPrivatePromptSent && event.PlayerID != playerID {
			continue
		}
		if event.Type == EventCardDrawn && event.PlayerID != playerID {
			event.CardIDs = nil
			event.Cards = nil
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func buildPrivatePlayerView(player Player) PlayerPrivateView {
	return PlayerPrivateView{
		ID:      player.ID,
		Name:    player.Name,
		Hand:    buildPrivateCardViews(player),
		Alive:   player.Alive,
		Blinded: player.Blinded,
	}
}

func buildPrivateCardViews(player Player) []PrivateCardView {
	cards := make([]PrivateCardView, len(player.Hand))
	for i, card := range player.Hand {
		cards[i] = PrivateCardView{ID: card.ID}
		if player.Blinded {
			cards[i].IsHidden = true
			continue
		}
		cards[i].Code = card.Code
	}
	return cards
}

func buildPublicPlayerViews(players []Player) []PlayerPublicView {
	views := make([]PlayerPublicView, len(players))
	for i, player := range players {
		views[i] = PlayerPublicView{
			ID:        player.ID,
			Name:      player.Name,
			HandCount: len(player.Hand),
			Alive:     player.Alive,
			Connected: player.Connected,
			Ready:     player.Ready,
			IsHost:    player.IsHost,
			Blinded:   player.Blinded,
		}
	}
	return views
}

func buildPublicCardViews(cards []Card) []PublicCardView {
	views := make([]PublicCardView, len(cards))
	for i, card := range cards {
		views[i] = PublicCardView{ID: card.ID, Code: card.Code}
	}
	return views
}

func buildPublicPendingActionView(pending *PendingAction) *PublicPendingActionView {
	if pending == nil {
		return nil
	}
	return &PublicPendingActionView{
		ID:              pending.ID,
		SourcePlayerID:  pending.SourcePlayerID,
		Type:            pending.Type,
		TargetPlayerID:  pending.TargetPlayerID,
		ComboKind:       pending.ComboKind,
		RequestedCode:   pending.RequestedCode,
		CancelCount:     pending.CancelCount,
		ExpiresAtUnixMs: pending.ExpiresAtUnixMs,
	}
}

func buildPublicMarkedCardViews(markedCards map[string]MarkedCard) []PublicMarkedCardView {
	views := make([]PublicMarkedCardView, 0, len(markedCards))
	for _, marked := range markedCards {
		views = append(views, PublicMarkedCardView{
			CardID:  marked.CardID,
			OwnerID: marked.OwnerID,
			Code:    marked.Revealed.Code,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].CardID < views[j].CardID })
	return views
}

func buildAvailableActions(state *GameState, player Player) []CommandType {
	if !player.Alive {
		return nil
	}

	actions := make([]CommandType, 0)
	switch state.Phase {
	case PhasePlayerTurn:
		if state.CurrentPlayerID == player.ID {
			actions = append(actions, CommandDrawCard)
			if hasPlayableAction(player.Hand) {
				actions = append(actions, CommandPlayCard)
			}
			if hasPotentialComboCards(player.Hand) {
				actions = append(actions, CommandPlayCombo)
			}
		}
	case PhaseCancelWindow:
		if findCardIndexByCode(player.Hand, CardCancel) >= 0 {
			actions = append(actions, CommandPlayCancel)
		}
	case PhaseWaitingExplosivePlacement:
		if state.PendingAction != nil && state.PendingAction.SourcePlayerID == player.ID {
			actions = append(actions, CommandPlaceExplosive)
		}
	case PhaseWaitingRequestCardChoice:
		if state.PendingAction != nil && state.PendingAction.TargetPlayerID == player.ID {
			actions = append(actions, CommandChooseCardForRequest)
		}
	case PhaseWaitingDiscardRecovery:
		if state.PendingAction != nil && state.PendingAction.SourcePlayerID == player.ID {
			actions = append(actions, CommandChooseCardFromDiscard)
		}
	case PhaseWaitingRecycleChoices:
		if state.PendingAction != nil && containsPlayerID(state.PendingAction.RecyclePlayerIDs, player.ID) && state.PendingAction.RecycleSelections[player.ID].ID == "" {
			actions = append(actions, CommandChooseCardForRecycle)
		}
	case PhaseWaitingMarkedCardChoice:
		if state.PendingAction != nil && state.PendingAction.SourcePlayerID == player.ID {
			actions = append(actions, CommandChooseMarkedCard)
		}
	}
	return actions
}

func containsPlayerID(playerIDs []string, playerID string) bool {
	for _, id := range playerIDs {
		if id == playerID {
			return true
		}
	}
	return false
}

func hasPlayableAction(cards []Card) bool {
	for _, card := range cards {
		if IsAction(card.Code) && !IsCancel(card.Code) && isSupportedBasicAction(card.Code) {
			return true
		}
	}
	return false
}

func hasPotentialComboCards(cards []Card) bool {
	return len(cards) >= 2
}
