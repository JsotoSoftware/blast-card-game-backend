package game

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	extraTurnDebt        = 2
	cancelWindowDuration = 10 * time.Second
)

type Engine struct {
	deckFactory *DeckFactory
	nextPending int
}

func NewEngine(rng *rand.Rand) *Engine {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return &Engine{deckFactory: NewDeckFactory(rng)}
}

func (e *Engine) StartGame(roomID string, players []Player) (*GameState, []Event, error) {
	state, err := e.deckFactory.NewSetup(roomID, players)
	if err != nil {
		return nil, nil, err
	}

	events := []Event{
		e.nextEvent(state, EventGameStarted, "", nil, ""),
		e.nextEvent(state, EventTurnStarted, state.CurrentPlayerID, nil, ""),
	}

	return state, events, nil
}

func (e *Engine) DrawCard(state *GameState, cmd DrawCardCommand) ([]Event, error) {
	if state.Phase != PhasePlayerTurn {
		return nil, ErrInvalidPhase
	}
	if state.CurrentPlayerID != cmd.PlayerID {
		return nil, ErrNotYourTurn
	}
	playerIndex, player, err := currentAlivePlayer(state, cmd.PlayerID)
	if err != nil {
		return nil, err
	}
	if len(state.DrawPile) == 0 {
		return nil, ErrDrawPileEmpty
	}

	drawn := state.DrawPile[0]
	state.DrawPile = state.DrawPile[1:]

	events := []Event{e.nextEvent(state, EventCardDrawn, cmd.PlayerID, []string{drawn.ID}, "")}

	if IsExplosive(drawn.Code) {
		canHoldDrawnExplosive := findCardIndexByCode(player.Hand, CardExplosiveHolder) >= 0 && countCardsByCode(player.Hand, CardExplosive) == 0
		if canHoldDrawnExplosive {
			state.Players[playerIndex].Hand = append(state.Players[playerIndex].Hand, drawn)
			events = append(events, e.completeCurrentTurnUnit(state)...)
			return events, nil
		}

		shieldIndex := findCardIndexByCode(player.Hand, CardShield)
		if shieldIndex < 0 {
			state.DiscardPile = append(state.DiscardPile, drawn)
			state.DiscardPile = append(state.DiscardPile, player.Hand...)
			state.Players[playerIndex].Hand = nil
			state.Players[playerIndex].Alive = false
			events = append(events, e.nextEvent(state, EventPlayerEliminated, cmd.PlayerID, nil, ""))

			if e.setWinnerIfGameOver(state) {
				events = append(events, e.nextEvent(state, EventGameOver, state.WinnerPlayerID, nil, ""))
				return events, nil
			}

			events = append(events, e.advanceToNextAlivePlayer(state)...)
			return events, nil
		}

		shield := player.Hand[shieldIndex]
		state.Players[playerIndex].Hand = removeCardAt(player.Hand, shieldIndex)
		state.DiscardPile = append(state.DiscardPile, shield)
		state.Phase = PhaseWaitingExplosivePlacement
		state.PendingAction = &PendingAction{
			ID:             e.nextPendingID(),
			SourcePlayerID: cmd.PlayerID,
			Type:           PendingExplosivePlacement,
			CardIDs:        []string{drawn.ID},
			Cards:          []Card{drawn},
		}
		events = append(events, e.nextEvent(state, EventActionPending, cmd.PlayerID, []string{drawn.ID}, ""))
		return events, nil
	}

	state.Players[playerIndex].Hand = append(state.Players[playerIndex].Hand, drawn)
	events = append(events, e.completeCurrentTurnUnit(state)...)
	return events, nil
}

func (e *Engine) PlaceExplosive(state *GameState, cmd PlaceExplosiveCommand) ([]Event, error) {
	if state.Phase != PhaseWaitingExplosivePlacement {
		return nil, ErrInvalidPhase
	}
	if state.PendingAction == nil || state.PendingAction.Type != PendingExplosivePlacement {
		return nil, ErrNoPendingAction
	}
	if state.PendingAction.SourcePlayerID != cmd.PlayerID {
		return nil, ErrNotYourTurn
	}
	if cmd.Index < 0 || cmd.Index > len(state.DrawPile) {
		return nil, ErrInvalidPlacement
	}
	if len(state.PendingAction.Cards) != 1 {
		return nil, ErrNoPendingAction
	}

	card := state.PendingAction.Cards[0]
	state.DrawPile = insertCardAt(state.DrawPile, card, cmd.Index)
	state.PendingAction = nil
	state.Phase = PhasePlayerTurn

	events := []Event{e.nextEvent(state, EventActionResolved, cmd.PlayerID, []string{card.ID}, "")}
	events = append(events, e.completeCurrentTurnUnit(state)...)
	return events, nil
}

func (e *Engine) PlayCard(state *GameState, cmd PlayCardCommand) ([]Event, error) {
	if state.Phase != PhasePlayerTurn {
		return nil, ErrInvalidPhase
	}
	if state.CurrentPlayerID != cmd.PlayerID {
		return nil, ErrNotYourTurn
	}
	playerIndex, player, err := currentAlivePlayer(state, cmd.PlayerID)
	if err != nil {
		return nil, err
	}
	if len(cmd.CardIDs) != 1 {
		return nil, ErrInvalidCardPlay
	}

	cardIndex := findCardIndexByID(player.Hand, cmd.CardIDs[0])
	if cardIndex < 0 {
		return nil, ErrCardNotInHand
	}
	card := player.Hand[cardIndex]
	if !IsAction(card.Code) || IsCancel(card.Code) || !isSupportedBasicAction(card.Code) {
		return nil, ErrInvalidCardPlay
	}
	if RequiresTarget(card.Code) && cmd.TargetID == "" {
		return nil, ErrTargetRequired
	}
	if card.Code == CardTargetExtraTurns {
		if err := validateTarget(state, cmd.PlayerID, cmd.TargetID, false); err != nil {
			return nil, err
		}
	}
	if card.Code == CardRequestCard {
		if err := validateTarget(state, cmd.PlayerID, cmd.TargetID, true); err != nil {
			return nil, err
		}
	}

	state.Players[playerIndex].Hand = removeCardAt(player.Hand, cardIndex)
	state.DiscardPile = append(state.DiscardPile, card)
	state.Phase = PhaseCancelWindow
	state.PendingAction = &PendingAction{
		ID:              e.nextPendingID(),
		SourcePlayerID:  cmd.PlayerID,
		Type:            PendingPlayCard,
		CardIDs:         []string{card.ID},
		Cards:           []Card{card},
		TargetPlayerID:  cmd.TargetID,
		ExpiresAtUnixMs: time.Now().Add(cancelWindowDuration).UnixMilli(),
	}

	events := []Event{
		e.nextEvent(state, EventCardPlayed, cmd.PlayerID, []string{card.ID}, cmd.TargetID),
		e.nextEvent(state, EventActionPending, cmd.PlayerID, []string{card.ID}, cmd.TargetID),
	}
	return events, nil
}

func (e *Engine) PlayCancel(state *GameState, cmd PlayCancelCommand) ([]Event, error) {
	if state.PendingAction == nil {
		return nil, ErrNoPendingAction
	}
	if state.Phase != PhaseCancelWindow {
		return nil, ErrInvalidPhase
	}
	if cmd.PendingActionID != "" && cmd.PendingActionID != state.PendingAction.ID {
		return nil, ErrNoPendingAction
	}
	if !isCancelablePendingAction(state.PendingAction) {
		return nil, ErrActionNotCancelable
	}

	playerIndex, player, err := currentAlivePlayer(state, cmd.PlayerID)
	if err != nil {
		return nil, err
	}

	cardIndex := -1
	if cmd.CardID != "" {
		cardIndex = findCardIndexByID(player.Hand, cmd.CardID)
		if cardIndex >= 0 && !IsCancel(player.Hand[cardIndex].Code) {
			return nil, ErrInvalidCardPlay
		}
	} else {
		cardIndex = findCardIndexByCode(player.Hand, CardCancel)
	}
	if cardIndex < 0 {
		return nil, ErrCardNotInHand
	}

	card := player.Hand[cardIndex]
	state.Players[playerIndex].Hand = removeCardAt(player.Hand, cardIndex)
	state.DiscardPile = append(state.DiscardPile, card)
	state.PendingAction.CancelCount++

	events := []Event{
		e.nextEvent(state, EventCardPlayed, cmd.PlayerID, []string{card.ID}, state.PendingAction.SourcePlayerID),
		e.nextEvent(state, EventActionPending, state.PendingAction.SourcePlayerID, state.PendingAction.CardIDs, state.PendingAction.TargetPlayerID),
	}
	return events, nil
}

func (e *Engine) ExpireCancelWindow(state *GameState, now time.Time) ([]Event, error) {
	if state.PendingAction == nil {
		return nil, ErrNoPendingAction
	}
	if state.Phase != PhaseCancelWindow {
		return nil, ErrInvalidPhase
	}
	if !now.IsZero() && state.PendingAction.ExpiresAtUnixMs > 0 && now.UnixMilli() < state.PendingAction.ExpiresAtUnixMs {
		return nil, ErrCancelWindowActive
	}

	return e.ResolveCancelWindow(state)
}

func (e *Engine) ResolveCancelWindow(state *GameState) ([]Event, error) {
	if state.PendingAction == nil {
		return nil, ErrNoPendingAction
	}
	if state.Phase != PhaseCancelWindow {
		return nil, ErrInvalidPhase
	}
	if !isCancelablePendingAction(state.PendingAction) {
		return nil, ErrActionNotCancelable
	}

	pending := state.PendingAction
	if pending.CancelCount%2 == 1 {
		state.PendingAction = nil
		state.Phase = PhasePlayerTurn
		return []Event{e.nextEvent(state, EventActionCanceled, pending.SourcePlayerID, pending.CardIDs, pending.TargetPlayerID)}, nil
	}

	card := pending.Cards[0]
	state.PendingAction = nil
	state.Phase = PhasePlayerTurn
	return e.resolvePlayedCard(state, card, pending.SourcePlayerID, pending.TargetPlayerID)
}

func (e *Engine) resolvePlayedCard(state *GameState, card Card, sourcePlayerID string, targetPlayerID string) ([]Event, error) {
	events := []Event{e.nextEvent(state, EventActionResolved, sourcePlayerID, []string{card.ID}, targetPlayerID)}

	switch card.Code {
	case CardSkipTurn:
		events = append(events, e.completeCurrentTurnUnit(state)...)
	case CardSkipAllTurns:
		state.TurnDebt[sourcePlayerID] = 0
		events = append(events, e.advanceToNextAlivePlayer(state)...)
	case CardForceExtraTurns:
		targetID, err := nextAlivePlayerID(state, sourcePlayerID)
		if err != nil {
			return nil, err
		}
		state.TurnDebt[targetID] += extraTurnDebt
		state.TurnDebt[sourcePlayerID] = 0
		events = append(events, e.advanceToPlayer(state, targetID)...)
	case CardTargetExtraTurns:
		state.TurnDebt[targetPlayerID] += extraTurnDebt
		state.TurnDebt[sourcePlayerID] = 0
		events = append(events, e.advanceToPlayer(state, targetPlayerID)...)
	case CardShuffleDeck:
		e.deckFactory.Shuffle(state.DrawPile)
	case CardPeekDeck:
		events = append(events, e.nextEvent(state, EventPrivatePromptSent, sourcePlayerID, topCardIDs(state.DrawPile, 3), ""))
	case CardPeekDeck5:
		events = append(events, e.nextEvent(state, EventPrivatePromptSent, sourcePlayerID, topCardIDs(state.DrawPile, 5), ""))
	case CardRequestCard:
		state.Phase = PhaseWaitingRequestCardChoice
		state.PendingAction = &PendingAction{
			ID:             e.nextPendingID(),
			SourcePlayerID: sourcePlayerID,
			Type:           PendingRequestCardChoice,
			CardIDs:        []string{card.ID},
			TargetPlayerID: targetPlayerID,
		}
		events = append(events, e.nextEvent(state, EventPrivatePromptSent, targetPlayerID, []string{card.ID}, targetPlayerID))
	default:
		return nil, ErrInvalidCardPlay
	}

	return events, nil
}

func (e *Engine) completeCurrentTurnUnit(state *GameState) []Event {
	if state.CurrentPlayerID == "" {
		return nil
	}
	if state.TurnDebt == nil {
		state.TurnDebt = map[string]int{}
	}
	if state.TurnDebt[state.CurrentPlayerID] > 0 {
		state.TurnDebt[state.CurrentPlayerID]--
	}
	if state.TurnDebt[state.CurrentPlayerID] > 0 {
		return []Event{e.nextEvent(state, EventTurnStarted, state.CurrentPlayerID, nil, "")}
	}
	return e.advanceToNextAlivePlayer(state)
}

func (e *Engine) advanceToNextAlivePlayer(state *GameState) []Event {
	nextID, err := nextAlivePlayerID(state, state.CurrentPlayerID)
	if err != nil {
		return nil
	}
	return e.advanceToPlayer(state, nextID)
}

func (e *Engine) advanceToPlayer(state *GameState, playerID string) []Event {
	state.CurrentPlayerID = playerID
	state.Phase = PhasePlayerTurn
	if state.TurnDebt == nil {
		state.TurnDebt = map[string]int{}
	}
	if state.TurnDebt[playerID] <= 0 {
		state.TurnDebt[playerID] = 1
	}
	return []Event{e.nextEvent(state, EventTurnStarted, playerID, nil, "")}
}

func (e *Engine) setWinnerIfGameOver(state *GameState) bool {
	alive := alivePlayers(state)
	if len(alive) != 1 {
		return false
	}
	state.WinnerPlayerID = alive[0].ID
	state.CurrentPlayerID = ""
	state.Phase = PhaseGameOver
	return true
}

func (e *Engine) nextEvent(state *GameState, eventType EventType, playerID string, cardIDs []string, targetID string) Event {
	state.EventSeq++
	return Event{
		Seq:      state.EventSeq,
		Type:     eventType,
		PlayerID: playerID,
		CardIDs:  cardIDs,
		TargetID: targetID,
	}
}

func (e *Engine) nextPendingID() string {
	e.nextPending++
	return fmt.Sprintf("pending-%06d", e.nextPending)
}

func isSupportedBasicAction(code CardCode) bool {
	switch code {
	case CardSkipTurn,
		CardSkipAllTurns,
		CardForceExtraTurns,
		CardTargetExtraTurns,
		CardShuffleDeck,
		CardPeekDeck,
		CardPeekDeck5,
		CardRequestCard:
		return true
	default:
		return false
	}
}

func isCancelablePendingAction(pending *PendingAction) bool {
	return pending != nil && pending.Type == PendingPlayCard && len(pending.Cards) == 1 && isSupportedBasicAction(pending.Cards[0].Code)
}

func currentAlivePlayer(state *GameState, playerID string) (int, Player, error) {
	index := findPlayerIndexByID(state, playerID)
	if index < 0 {
		return -1, Player{}, ErrPlayerNotFound
	}
	player := state.Players[index]
	if !player.Alive {
		return -1, Player{}, ErrPlayerNotAlive
	}
	return index, player, nil
}

func validateTarget(state *GameState, sourceID string, targetID string, mustHaveCards bool) error {
	if targetID == "" {
		return ErrTargetRequired
	}
	if targetID == sourceID {
		return ErrInvalidTarget
	}
	index := findPlayerIndexByID(state, targetID)
	if index < 0 || !state.Players[index].Alive {
		return ErrInvalidTarget
	}
	if mustHaveCards && len(state.Players[index].Hand) == 0 {
		return ErrInvalidTarget
	}
	return nil
}

func nextAlivePlayerID(state *GameState, fromPlayerID string) (string, error) {
	if len(state.Players) == 0 {
		return "", ErrPlayerNotFound
	}
	start := findPlayerIndexByID(state, fromPlayerID)
	if start < 0 {
		return "", ErrPlayerNotFound
	}

	for step := 1; step <= len(state.Players); step++ {
		index := (start + step) % len(state.Players)
		if state.Players[index].Alive {
			return state.Players[index].ID, nil
		}
	}

	return "", ErrPlayerNotAlive
}

func alivePlayers(state *GameState) []Player {
	alive := make([]Player, 0, len(state.Players))
	for _, player := range state.Players {
		if player.Alive {
			alive = append(alive, player)
		}
	}
	return alive
}

func findPlayerIndexByID(state *GameState, playerID string) int {
	for i, player := range state.Players {
		if player.ID == playerID {
			return i
		}
	}
	return -1
}

func findCardIndexByID(cards []Card, cardID string) int {
	for i, card := range cards {
		if card.ID == cardID {
			return i
		}
	}
	return -1
}

func findCardIndexByCode(cards []Card, code CardCode) int {
	for i, card := range cards {
		if card.Code == code {
			return i
		}
	}
	return -1
}

func countCardsByCode(cards []Card, code CardCode) int {
	count := 0
	for _, card := range cards {
		if card.Code == code {
			count++
		}
	}
	return count
}

func topCardIDs(cards []Card, count int) []string {
	if count > len(cards) {
		count = len(cards)
	}
	ids := make([]string, count)
	for i := range count {
		ids[i] = cards[i].ID
	}
	return ids
}

func removeCardAt(cards []Card, index int) []Card {
	result := make([]Card, 0, len(cards)-1)
	result = append(result, cards[:index]...)
	result = append(result, cards[index+1:]...)
	return result
}

func insertCardAt(cards []Card, card Card, index int) []Card {
	cards = append(cards, Card{})
	copy(cards[index+1:], cards[index:])
	cards[index] = card
	return cards
}
