package game

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	extraTurnDebt        = 2
	cancelWindowDuration = 5 * time.Second
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
	if _, _, err := currentAlivePlayer(state, cmd.PlayerID); err != nil {
		return nil, err
	}
	if len(state.DrawPile) == 0 {
		return nil, ErrDrawPileEmpty
	}

	drawn := state.DrawPile[0]
	state.DrawPile = state.DrawPile[1:]
	return e.resolveDrawnCard(state, cmd.PlayerID, drawn)
}

func (e *Engine) resolveDrawnCard(state *GameState, playerID string, drawn Card) ([]Event, error) {
	playerIndex, player, err := currentAlivePlayer(state, playerID)
	if err != nil {
		return nil, err
	}

	events := []Event{e.nextEvent(state, EventCardDrawn, playerID, []string{drawn.ID}, "")}
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
			e.clearMarksForPlayerHand(state, playerID)
			state.Players[playerIndex].Alive = false
			events = append(events, e.nextEvent(state, EventPlayerEliminated, playerID, nil, ""))

			if e.setWinnerIfGameOver(state) {
				events = append(events, e.nextEvent(state, EventGameOver, state.WinnerPlayerID, nil, ""))
				return events, nil
			}

			events = append(events, e.advanceToNextAlivePlayer(state)...)
			return events, nil
		}

		shield := player.Hand[shieldIndex]
		state.Players[playerIndex].Hand = removeCardAt(player.Hand, shieldIndex)
		e.clearMarksForPlayerHand(state, playerID)
		state.DiscardPile = append(state.DiscardPile, shield)
		state.Phase = PhaseWaitingExplosivePlacement
		state.PendingAction = &PendingAction{
			ID:             e.nextPendingID(),
			SourcePlayerID: playerID,
			Type:           PendingExplosivePlacement,
			CardIDs:        []string{drawn.ID},
			Cards:          []Card{drawn},
		}
		events = append(events, e.nextEvent(state, EventActionPending, playerID, []string{drawn.ID}, ""))
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

	pending := state.PendingAction
	card := pending.Cards[0]
	state.DrawPile = insertCardAt(state.DrawPile, card, cmd.Index)
	state.PendingAction = nil
	state.Phase = PhasePlayerTurn

	events := []Event{e.nextEvent(state, EventActionResolved, cmd.PlayerID, []string{card.ID}, "")}
	if pending.ReactiveExplosive {
		events = append(events, e.resolveUnsafeExplosivesFrom(state, pending.OriginalCurrentPlayerID, pending.SourcePlayerID)...)
		return events, nil
	}

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
	if card.Code == CardRequestCard || card.Code == CardRevealHeldCard {
		if err := validateTarget(state, cmd.PlayerID, cmd.TargetID, true); err != nil {
			return nil, err
		}
	}
	if card.Code == CardDrawFromBottom && len(state.DrawPile) == 0 {
		return nil, ErrDrawPileEmpty
	}

	state.Players[playerIndex].Hand = removeCardAt(player.Hand, cardIndex)
	e.clearMarksForPlayerHand(state, cmd.PlayerID)
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

func (e *Engine) PlayCombo(state *GameState, cmd PlayComboCommand) ([]Event, error) {
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

	cards, err := cardsByIDsInHand(player.Hand, cmd.CardIDs)
	if err != nil {
		return nil, err
	}
	comboKind, err := validateCombo(player.Hand, cards, cmd.RequestedCode)
	if err != nil {
		return nil, err
	}

	if comboKind == ComboPair {
		if err := validateTarget(state, cmd.PlayerID, cmd.TargetID, true); err != nil {
			return nil, err
		}
	}
	if comboKind == ComboTriple {
		if cmd.RequestedCode == "" {
			return nil, ErrInvalidCardPlay
		}
		if err := validateTarget(state, cmd.PlayerID, cmd.TargetID, false); err != nil {
			return nil, err
		}
	}

	state.Players[playerIndex].Hand = removeCardsByIDs(player.Hand, cmd.CardIDs)
	e.clearMarksForPlayerHand(state, cmd.PlayerID)
	state.DiscardPile = append(state.DiscardPile, cards...)
	state.Phase = PhaseCancelWindow
	state.PendingAction = &PendingAction{
		ID:              e.nextPendingID(),
		SourcePlayerID:  cmd.PlayerID,
		Type:            PendingCardCombo,
		CardIDs:         append([]string(nil), cmd.CardIDs...),
		Cards:           cards,
		TargetPlayerID:  cmd.TargetID,
		ComboKind:       comboKind,
		RequestedCode:   cmd.RequestedCode,
		ExpiresAtUnixMs: time.Now().Add(cancelWindowDuration).UnixMilli(),
	}

	events := []Event{
		e.nextEvent(state, EventCardPlayed, cmd.PlayerID, cmd.CardIDs, cmd.TargetID),
		e.nextEvent(state, EventActionPending, cmd.PlayerID, cmd.CardIDs, cmd.TargetID),
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
	e.clearMarksForPlayerHand(state, cmd.PlayerID)
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

	state.PendingAction = nil
	state.Phase = PhasePlayerTurn
	switch pending.Type {
	case PendingPlayCard:
		card := pending.Cards[0]
		return e.resolvePlayedCard(state, card, pending.SourcePlayerID, pending.TargetPlayerID)
	case PendingCardCombo:
		return e.resolveCombo(state, pending)
	default:
		return nil, ErrActionNotCancelable
	}
}

func (e *Engine) ChooseCardForRequest(state *GameState, cmd ChooseCardForRequestCommand) ([]Event, error) {
	if state.Phase != PhaseWaitingRequestCardChoice {
		return nil, ErrInvalidPhase
	}
	if state.PendingAction == nil || state.PendingAction.Type != PendingRequestCardChoice {
		return nil, ErrNoPendingAction
	}
	if state.PendingAction.TargetPlayerID != cmd.PlayerID {
		return nil, ErrNotYourTurn
	}

	targetIndex, target, err := currentAlivePlayer(state, cmd.PlayerID)
	if err != nil {
		return nil, err
	}
	sourceIndex, _, err := currentAlivePlayer(state, state.PendingAction.SourcePlayerID)
	if err != nil {
		return nil, err
	}
	cardIndex := findCardIndexByID(target.Hand, cmd.CardID)
	if cardIndex < 0 {
		return nil, ErrCardNotInHand
	}

	card := target.Hand[cardIndex]
	state.Players[targetIndex].Hand = removeCardAt(target.Hand, cardIndex)
	e.clearMarksForPlayerHand(state, cmd.PlayerID)
	state.Players[sourceIndex].Hand = append(state.Players[sourceIndex].Hand, card)
	pending := state.PendingAction
	state.PendingAction = nil
	state.Phase = PhasePlayerTurn

	events := []Event{e.nextEvent(state, EventActionResolved, pending.SourcePlayerID, nil, pending.TargetPlayerID)}
	events = append(events, e.resolveUnsafeExplosives(state, cmd.PlayerID, pending.SourcePlayerID)...)
	return events, nil
}

func (e *Engine) ChooseMarkedCard(state *GameState, cmd ChooseMarkedCardCommand) ([]Event, error) {
	if state.Phase != PhaseWaitingMarkedCardChoice {
		return nil, ErrInvalidPhase
	}
	if state.PendingAction == nil || state.PendingAction.Type != PendingMarkedCardChoice {
		return nil, ErrNoPendingAction
	}
	if state.PendingAction.SourcePlayerID != cmd.PlayerID {
		return nil, ErrNotYourTurn
	}

	_, target, err := currentAlivePlayer(state, state.PendingAction.TargetPlayerID)
	if err != nil {
		return nil, err
	}
	cardIndex := findCardIndexByID(target.Hand, cmd.CardID)
	if cardIndex < 0 {
		return nil, ErrCardNotInHand
	}

	card := target.Hand[cardIndex]
	if state.MarkedCards == nil {
		state.MarkedCards = make(map[string]MarkedCard)
	}
	state.MarkedCards[card.ID] = MarkedCard{CardID: card.ID, OwnerID: target.ID, Revealed: card}
	pending := state.PendingAction
	state.PendingAction = nil
	state.Phase = PhasePlayerTurn
	return []Event{e.nextEvent(state, EventActionResolved, pending.SourcePlayerID, []string{card.ID}, pending.TargetPlayerID)}, nil
}

func (e *Engine) ChooseCardForRecycle(state *GameState, cmd ChooseCardForRecycleCommand) ([]Event, error) {
	if state.Phase != PhaseWaitingRecycleChoices {
		return nil, ErrInvalidPhase
	}
	pending := state.PendingAction
	if pending == nil || pending.Type != PendingRecycleChoices {
		return nil, ErrNoPendingAction
	}
	if !containsPlayerID(pending.RecyclePlayerIDs, cmd.PlayerID) {
		return nil, ErrNotYourTurn
	}
	if _, selected := pending.RecycleSelections[cmd.PlayerID]; selected {
		return nil, ErrInvalidCardPlay
	}

	playerIndex, player, err := currentAlivePlayer(state, cmd.PlayerID)
	if err != nil {
		return nil, err
	}
	cardIndex := findCardIndexByID(player.Hand, cmd.CardID)
	if cardIndex < 0 {
		return nil, ErrCardNotInHand
	}

	pending.RecycleSelections[cmd.PlayerID] = player.Hand[cardIndex]
	state.Players[playerIndex].Hand = removeCardAt(player.Hand, cardIndex)
	e.clearMarksForPlayerHand(state, cmd.PlayerID)
	if len(pending.RecycleSelections) != len(pending.RecyclePlayerIDs) {
		return nil, nil
	}

	for _, playerID := range pending.RecyclePlayerIDs {
		state.DrawPile = append(state.DrawPile, pending.RecycleSelections[playerID])
	}
	e.deckFactory.Shuffle(state.DrawPile)
	state.PendingAction = nil
	state.Phase = PhasePlayerTurn

	events := []Event{e.nextEvent(state, EventActionResolved, pending.SourcePlayerID, nil, "")}
	events = append(events, e.resolveUnsafeExplosives(state, pending.RecyclePlayerIDs...)...)
	return events, nil
}

func (e *Engine) ChooseCardFromDiscard(state *GameState, cmd ChooseCardFromDiscardCommand) ([]Event, error) {
	if state.Phase != PhaseWaitingDiscardRecovery {
		return nil, ErrInvalidPhase
	}
	if state.PendingAction == nil || state.PendingAction.Type != PendingDiscardRecovery {
		return nil, ErrNoPendingAction
	}
	if state.PendingAction.SourcePlayerID != cmd.PlayerID {
		return nil, ErrNotYourTurn
	}

	cardIndex := findCardIndexByID(state.DiscardPile, cmd.CardID)
	if cardIndex < 0 {
		return nil, ErrCardNotInHand
	}
	playerIndex, _, err := currentAlivePlayer(state, cmd.PlayerID)
	if err != nil {
		return nil, err
	}

	card := state.DiscardPile[cardIndex]
	state.DiscardPile = removeCardAt(state.DiscardPile, cardIndex)
	state.Players[playerIndex].Hand = append(state.Players[playerIndex].Hand, card)
	state.PendingAction = nil
	state.Phase = PhasePlayerTurn

	events := []Event{e.nextEvent(state, EventActionResolved, cmd.PlayerID, []string{card.ID}, "")}
	events = append(events, e.resolveUnsafeExplosives(state, cmd.PlayerID)...)
	return events, nil
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
		peeked := topCards(state.DrawPile, 3)
		event := e.nextEvent(state, EventPrivatePromptSent, sourcePlayerID, cardIDsOf(peeked), "")
		event.Cards = peeked
		events = append(events, event)
	case CardPeekDeck5:
		peeked := topCards(state.DrawPile, 5)
		event := e.nextEvent(state, EventPrivatePromptSent, sourcePlayerID, cardIDsOf(peeked), "")
		event.Cards = peeked
		events = append(events, event)
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
	case CardRevealHeldCard:
		state.Phase = PhaseWaitingMarkedCardChoice
		state.PendingAction = &PendingAction{
			ID:             e.nextPendingID(),
			SourcePlayerID: sourcePlayerID,
			Type:           PendingMarkedCardChoice,
			CardIDs:        []string{card.ID},
			TargetPlayerID: targetPlayerID,
		}
		targetIndex := findPlayerIndexByID(state, targetPlayerID)
		events = append(events, e.nextEvent(state, EventPrivatePromptSent, sourcePlayerID, cardIDsOf(state.Players[targetIndex].Hand), targetPlayerID))
	case CardDrawFromBottom:
		if len(state.DrawPile) == 0 {
			return nil, ErrDrawPileEmpty
		}
		drawn := state.DrawPile[len(state.DrawPile)-1]
		state.DrawPile = state.DrawPile[:len(state.DrawPile)-1]
		drawEvents, err := e.resolveDrawnCard(state, sourcePlayerID, drawn)
		if err != nil {
			return nil, err
		}
		events = append(events, drawEvents...)
	case CardCollectiveRecycle:
		recyclePlayerIDs := recycleEligiblePlayerIDs(state)
		if len(recyclePlayerIDs) == 0 {
			e.deckFactory.Shuffle(state.DrawPile)
			return events, nil
		}
		state.Phase = PhaseWaitingRecycleChoices
		state.PendingAction = &PendingAction{
			ID:                e.nextPendingID(),
			SourcePlayerID:    sourcePlayerID,
			Type:              PendingRecycleChoices,
			CardIDs:           []string{card.ID},
			RecyclePlayerIDs:  recyclePlayerIDs,
			RecycleSelections: make(map[string]Card, len(recyclePlayerIDs)),
		}
		for _, playerID := range recyclePlayerIDs {
			events = append(events, e.nextEvent(state, EventPrivatePromptSent, playerID, nil, ""))
		}
	default:
		return nil, ErrInvalidCardPlay
	}

	return events, nil
}

func (e *Engine) resolveCombo(state *GameState, pending *PendingAction) ([]Event, error) {
	events := []Event{e.nextEvent(state, EventActionResolved, pending.SourcePlayerID, pending.CardIDs, pending.TargetPlayerID)}

	switch pending.ComboKind {
	case ComboPair:
		if err := e.transferRandomCard(state, pending.TargetPlayerID, pending.SourcePlayerID); err != nil {
			return nil, err
		}
		events = append(events, e.resolveUnsafeExplosives(state, pending.TargetPlayerID, pending.SourcePlayerID)...)
	case ComboTriple:
		if pending.RequestedCode == "" {
			return nil, ErrInvalidCardPlay
		}
		transferred, err := e.transferFirstCardByCode(state, pending.TargetPlayerID, pending.SourcePlayerID, pending.RequestedCode)
		if err != nil {
			return nil, err
		}
		if transferred {
			events = append(events, e.resolveUnsafeExplosives(state, pending.TargetPlayerID, pending.SourcePlayerID)...)
		}
	case ComboFiveDifferent:
		state.Phase = PhaseWaitingDiscardRecovery
		state.PendingAction = &PendingAction{
			ID:             e.nextPendingID(),
			SourcePlayerID: pending.SourcePlayerID,
			Type:           PendingDiscardRecovery,
			CardIDs:        pending.CardIDs,
			Cards:          pending.Cards,
			ComboKind:      pending.ComboKind,
		}
		events = append(events, e.nextEvent(state, EventPrivatePromptSent, pending.SourcePlayerID, cardIDsOf(state.DiscardPile), ""))
	default:
		return nil, ErrInvalidCardPlay
	}

	return events, nil
}

func (e *Engine) transferRandomCard(state *GameState, fromPlayerID string, toPlayerID string) error {
	fromIndex, fromPlayer, err := currentAlivePlayer(state, fromPlayerID)
	if err != nil {
		return err
	}
	toIndex, _, err := currentAlivePlayer(state, toPlayerID)
	if err != nil {
		return err
	}
	if len(fromPlayer.Hand) == 0 {
		return ErrInvalidTarget
	}

	cardIndex := e.deckFactory.rng.Intn(len(fromPlayer.Hand))
	card := fromPlayer.Hand[cardIndex]
	state.Players[fromIndex].Hand = removeCardAt(fromPlayer.Hand, cardIndex)
	e.clearMarksForPlayerHand(state, fromPlayerID)
	state.Players[toIndex].Hand = append(state.Players[toIndex].Hand, card)
	return nil
}

func (e *Engine) transferFirstCardByCode(state *GameState, fromPlayerID string, toPlayerID string, code CardCode) (bool, error) {
	fromIndex, fromPlayer, err := currentAlivePlayer(state, fromPlayerID)
	if err != nil {
		return false, err
	}
	toIndex, _, err := currentAlivePlayer(state, toPlayerID)
	if err != nil {
		return false, err
	}

	cardIndex := findCardIndexByCode(fromPlayer.Hand, code)
	if cardIndex < 0 {
		return false, nil
	}

	card := fromPlayer.Hand[cardIndex]
	state.Players[fromIndex].Hand = removeCardAt(fromPlayer.Hand, cardIndex)
	e.clearMarksForPlayerHand(state, fromPlayerID)
	state.Players[toIndex].Hand = append(state.Players[toIndex].Hand, card)
	return true, nil
}

func (e *Engine) resolveUnsafeExplosives(state *GameState, playerIDs ...string) []Event {
	return e.resolveUnsafeExplosivesFrom(state, state.CurrentPlayerID, playerIDs...)
}

func (e *Engine) resolveUnsafeExplosivesFrom(state *GameState, originalCurrentPlayerID string, playerIDs ...string) []Event {
	events := make([]Event, 0, len(playerIDs))
	seen := make(map[string]bool, len(playerIDs))
	for _, playerID := range playerIDs {
		if seen[playerID] {
			continue
		}
		seen[playerID] = true

		playerIndex := findPlayerIndexByID(state, playerID)
		if playerIndex < 0 || !state.Players[playerIndex].Alive {
			continue
		}

		player := state.Players[playerIndex]
		unsafeExplosiveCount := countCardsByCode(player.Hand, CardExplosive)
		if findCardIndexByCode(player.Hand, CardExplosiveHolder) >= 0 && unsafeExplosiveCount > 0 {
			unsafeExplosiveCount--
		}
		if unsafeExplosiveCount == 0 {
			continue
		}
		shieldIndex := findCardIndexByCode(player.Hand, CardShield)
		if shieldIndex < 0 {
			state.DiscardPile = append(state.DiscardPile, player.Hand...)
			state.Players[playerIndex].Hand = nil
			e.clearMarksForPlayerHand(state, playerID)
			state.Players[playerIndex].Alive = false
			events = append(events, e.nextEvent(state, EventPlayerEliminated, playerID, nil, ""))
			continue
		}

		explosiveIndex := findCardIndexByCode(player.Hand, CardExplosive)
		explosive := player.Hand[explosiveIndex]
		shield := player.Hand[shieldIndex]
		hand := removeCardAt(player.Hand, shieldIndex)
		if explosiveIndex > shieldIndex {
			explosiveIndex--
		}
		state.Players[playerIndex].Hand = removeCardAt(hand, explosiveIndex)
		e.clearMarksForPlayerHand(state, playerID)
		state.DiscardPile = append(state.DiscardPile, shield)
		state.UnsafeExplosiveQueue = append(state.UnsafeExplosiveQueue, UnsafeExplosivePlacement{PlayerID: playerID, Card: explosive})
	}

	if len(state.UnsafeExplosiveQueue) > 0 {
		events = append(events, e.startNextUnsafeExplosivePlacement(state, originalCurrentPlayerID)...)
		return events
	}
	return append(events, e.finishUnsafeExplosiveResolution(state, originalCurrentPlayerID)...)
}

func (e *Engine) clearMarksForPlayerHand(state *GameState, playerID string) {
	playerIndex := findPlayerIndexByID(state, playerID)
	if playerIndex < 0 {
		return
	}
	for cardID, marked := range state.MarkedCards {
		if marked.OwnerID == playerID && findCardIndexByID(state.Players[playerIndex].Hand, cardID) < 0 {
			delete(state.MarkedCards, cardID)
		}
	}
}

func (e *Engine) startNextUnsafeExplosivePlacement(state *GameState, originalCurrentPlayerID string) []Event {
	placement := state.UnsafeExplosiveQueue[0]
	state.UnsafeExplosiveQueue = state.UnsafeExplosiveQueue[1:]
	state.Phase = PhaseWaitingExplosivePlacement
	state.PendingAction = &PendingAction{
		ID:                      e.nextPendingID(),
		SourcePlayerID:          placement.PlayerID,
		Type:                    PendingExplosivePlacement,
		CardIDs:                 []string{placement.Card.ID},
		Cards:                   []Card{placement.Card},
		ReactiveExplosive:       true,
		OriginalCurrentPlayerID: originalCurrentPlayerID,
	}
	return []Event{e.nextEvent(state, EventActionPending, placement.PlayerID, []string{placement.Card.ID}, "")}
}

func (e *Engine) finishUnsafeExplosiveResolution(state *GameState, originalCurrentPlayerID string) []Event {
	if e.setWinnerIfGameOver(state) {
		return []Event{e.nextEvent(state, EventGameOver, state.WinnerPlayerID, nil, "")}
	}

	originalIndex := findPlayerIndexByID(state, originalCurrentPlayerID)
	if originalIndex >= 0 && state.Players[originalIndex].Alive {
		state.CurrentPlayerID = originalCurrentPlayerID
		state.Phase = PhasePlayerTurn
		return nil
	}

	state.CurrentPlayerID = originalCurrentPlayerID
	return e.advanceToNextAlivePlayer(state)
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
		CardRequestCard,
		CardRevealHeldCard,
		CardDrawFromBottom,
		CardCollectiveRecycle:
		return true
	default:
		return false
	}
}

func isCancelablePendingAction(pending *PendingAction) bool {
	if pending == nil {
		return false
	}
	switch pending.Type {
	case PendingPlayCard:
		return len(pending.Cards) == 1 && isSupportedBasicAction(pending.Cards[0].Code)
	case PendingCardCombo:
		return pending.ComboKind == ComboPair || pending.ComboKind == ComboTriple || pending.ComboKind == ComboFiveDifferent
	default:
		return false
	}
}

func validateCombo(playerHand []Card, cards []Card, requestedCode CardCode) (ComboKind, error) {
	switch len(cards) {
	case 2:
		if !validMatchingCombo(cards) {
			return "", ErrInvalidCardPlay
		}
		return ComboPair, nil
	case 3:
		if !validMatchingCombo(cards) || requestedCode == "" {
			return "", ErrInvalidCardPlay
		}
		return ComboTriple, nil
	case 5:
		if !validFiveDifferentCombo(playerHand, cards) {
			return "", ErrInvalidCardPlay
		}
		return ComboFiveDifferent, nil
	default:
		return "", ErrInvalidCardPlay
	}
}

func validMatchingCombo(cards []Card) bool {
	if len(cards) != 2 && len(cards) != 3 {
		return false
	}

	reference := CardCode("")
	seenRealToken := false
	usesTokenRules := false
	for _, card := range cards {
		if IsExplosive(card.Code) {
			return false
		}
		if IsToken(card.Code) {
			usesTokenRules = true
			seenRealToken = true
			if reference == "" {
				reference = card.Code
			} else if reference != card.Code {
				return false
			}
			continue
		}
		if IsWildToken(card.Code) {
			usesTokenRules = true
			continue
		}
		if usesTokenRules {
			return false
		}
		if reference == "" {
			reference = card.Code
		} else if reference != card.Code {
			return false
		}
	}

	if usesTokenRules {
		return seenRealToken
	}
	return reference != ""
}

func validFiveDifferentCombo(playerHand []Card, cards []Card) bool {
	if len(cards) != 5 {
		return false
	}

	codes := map[CardCode]bool{}
	includesExplosive := false
	for _, card := range cards {
		if card.Code == CardExplosiveHolder {
			return false
		}
		if codes[card.Code] {
			return false
		}
		codes[card.Code] = true
		if IsExplosive(card.Code) {
			includesExplosive = true
		}
	}

	if includesExplosive && countCardsByCode(playerHand, CardExplosiveHolder) == 0 {
		return false
	}
	return true
}

func cardsByIDsInHand(hand []Card, cardIDs []string) ([]Card, error) {
	if len(cardIDs) == 0 {
		return nil, ErrInvalidCardPlay
	}

	seen := map[string]bool{}
	cards := make([]Card, 0, len(cardIDs))
	for _, cardID := range cardIDs {
		if cardID == "" || seen[cardID] {
			return nil, ErrInvalidCardPlay
		}
		seen[cardID] = true

		cardIndex := findCardIndexByID(hand, cardID)
		if cardIndex < 0 {
			return nil, ErrCardNotInHand
		}
		cards = append(cards, hand[cardIndex])
	}
	return cards, nil
}

func removeCardsByIDs(cards []Card, cardIDs []string) []Card {
	remove := map[string]bool{}
	for _, cardID := range cardIDs {
		remove[cardID] = true
	}

	result := make([]Card, 0, len(cards)-len(remove))
	for _, card := range cards {
		if !remove[card.ID] {
			result = append(result, card)
		}
	}
	return result
}

func recycleEligiblePlayerIDs(state *GameState) []string {
	playerIDs := make([]string, 0, len(state.Players))
	for _, player := range state.Players {
		if player.Alive && len(player.Hand) > 0 {
			playerIDs = append(playerIDs, player.ID)
		}
	}
	return playerIDs
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

func topCards(cards []Card, count int) []Card {
	if count > len(cards) {
		count = len(cards)
	}
	result := make([]Card, count)
	copy(result, cards[:count])
	return result
}

func cardIDsOf(cards []Card) []string {
	ids := make([]string, len(cards))
	for i, card := range cards {
		ids[i] = card.ID
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
