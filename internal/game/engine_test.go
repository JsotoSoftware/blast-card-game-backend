package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStartGame(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(10)))
	state, events, err := engine.StartGame("ROOM1", testPlayers(3))
	if err != nil {
		t.Fatalf("StartGame returned error: %v", err)
	}

	if state.Phase != PhasePlayerTurn {
		t.Fatalf("phase got %s, want %s", state.Phase, PhasePlayerTurn)
	}
	if state.CurrentPlayerID != "player-A" {
		t.Fatalf("current player got %s, want player-A", state.CurrentPlayerID)
	}
	if state.TurnDebt["player-A"] != 1 {
		t.Fatalf("current player debt got %d, want 1", state.TurnDebt["player-A"])
	}
	if len(events) != 2 || events[0].Type != EventGameStarted || events[1].Type != EventTurnStarted {
		t.Fatalf("unexpected start events: %#v", events)
	}
}

func TestDrawCardRequiresCurrentPlayer(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(11)))
	state := engineTestState([]Player{engineTestPlayer("p1", nil), engineTestPlayer("p2", nil)}, []Card{engineTestCard("draw-1", CardSkipTurn)}, "p1")

	_, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p2"})
	if !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("DrawCard error got %v, want ErrNotYourTurn", err)
	}
}

func TestDrawNormalCardAddsToHandAndAdvancesTurn(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(12)))
	state := engineTestState([]Player{engineTestPlayer("p1", nil), engineTestPlayer("p2", nil)}, []Card{engineTestCard("draw-1", CardSkipTurn)}, "p1")

	events, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("DrawCard returned error: %v", err)
	}

	if len(state.Players[0].Hand) != 1 || state.Players[0].Hand[0].ID != "draw-1" {
		t.Fatalf("drawn card was not added to hand: %#v", state.Players[0].Hand)
	}
	if state.CurrentPlayerID != "p2" {
		t.Fatalf("current player got %s, want p2", state.CurrentPlayerID)
	}
	if !hasEvent(events, EventCardDrawn) || !hasEvent(events, EventTurnStarted) {
		t.Fatalf("expected draw and turn events, got %#v", events)
	}
}

func TestDrawNormalCardKeepsTurnWhenDebtRemains(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(13)))
	state := engineTestState([]Player{engineTestPlayer("p1", nil), engineTestPlayer("p2", nil)}, []Card{engineTestCard("draw-1", CardSkipTurn)}, "p1")
	state.TurnDebt["p1"] = 2

	_, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("DrawCard returned error: %v", err)
	}

	if state.CurrentPlayerID != "p1" {
		t.Fatalf("current player got %s, want p1", state.CurrentPlayerID)
	}
	if state.TurnDebt["p1"] != 1 {
		t.Fatalf("turn debt got %d, want 1", state.TurnDebt["p1"])
	}
}

func TestDrawExplosiveWithoutShieldEliminatesPlayer(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(14)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("hand-1", CardSkipTurn)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("danger-1", CardExplosive)},
		"p1",
	)

	events, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("DrawCard returned error: %v", err)
	}

	if state.Players[0].Alive {
		t.Fatal("p1 should be eliminated")
	}
	if state.Phase != PhaseGameOver {
		t.Fatalf("phase got %s, want %s", state.Phase, PhaseGameOver)
	}
	if state.WinnerPlayerID != "p2" {
		t.Fatalf("winner got %s, want p2", state.WinnerPlayerID)
	}
	if !hasEvent(events, EventPlayerEliminated) || !hasEvent(events, EventGameOver) {
		t.Fatalf("expected elimination and game over events, got %#v", events)
	}
}

func TestDrawExplosiveWithHolderAddsExplosiveToHand(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(15)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("holder-1", CardExplosiveHolder)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("danger-1", CardExplosive), engineTestCard("draw-2", CardSkipTurn)},
		"p1",
	)

	_, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("DrawCard returned error: %v", err)
	}

	if state.Phase != PhasePlayerTurn {
		t.Fatalf("phase got %s, want %s", state.Phase, PhasePlayerTurn)
	}
	if state.CurrentPlayerID != "p2" {
		t.Fatalf("current player got %s, want p2", state.CurrentPlayerID)
	}
	if countCode(state.Players[0].Hand, CardExplosive) != 1 {
		t.Fatalf("held explosive count got %d, want 1", countCode(state.Players[0].Hand, CardExplosive))
	}
	if countCode(state.Players[0].Hand, CardExplosiveHolder) != 1 {
		t.Fatal("holder should remain in hand")
	}
	if countCode(state.DiscardPile, CardShield) != 0 {
		t.Fatal("shield should not be consumed")
	}
}

func TestDrawSecondExplosiveWithHolderAndShieldMovesToPlacementPhase(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(16)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("holder-1", CardExplosiveHolder), engineTestCard("held-danger", CardExplosive), engineTestCard("shield-1", CardShield)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("danger-1", CardExplosive), engineTestCard("draw-2", CardSkipTurn)},
		"p1",
	)

	_, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("DrawCard returned error: %v", err)
	}

	if state.Phase != PhaseWaitingExplosivePlacement {
		t.Fatalf("phase got %s, want %s", state.Phase, PhaseWaitingExplosivePlacement)
	}
	if state.PendingAction == nil || state.PendingAction.CardIDs[0] != "danger-1" {
		t.Fatalf("expected newly drawn explosive to need placement, got %#v", state.PendingAction)
	}
	if countCode(state.Players[0].Hand, CardExplosive) != 1 {
		t.Fatalf("previously held explosive count got %d, want 1", countCode(state.Players[0].Hand, CardExplosive))
	}
	if countCode(state.Players[0].Hand, CardShield) != 0 {
		t.Fatal("shield should be consumed for second explosive")
	}
}

func TestDrawSecondExplosiveWithHolderAndNoShieldEliminatesPlayer(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(17)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("holder-1", CardExplosiveHolder), engineTestCard("held-danger", CardExplosive)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("danger-1", CardExplosive)},
		"p1",
	)

	_, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("DrawCard returned error: %v", err)
	}

	if state.Players[0].Alive {
		t.Fatal("p1 should be eliminated after drawing a second explosive without shield")
	}
	if state.WinnerPlayerID != "p2" {
		t.Fatalf("winner got %s, want p2", state.WinnerPlayerID)
	}
}

func TestPairComboStealingExplosiveHolderEliminatesTarget(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(1)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("explosive-1", CardExplosive), engineTestCard("holder-1", CardExplosiveHolder)}),
		},
		nil,
		"p1",
	)

	if _, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2"}, TargetID: "p2"}); err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.Players[1].Alive || state.WinnerPlayerID != "p1" || state.Phase != PhaseGameOver {
		t.Fatalf("holder loss should eliminate p2 and end the game, got %#v", state)
	}
	if countCode(state.DiscardPile, CardExplosive) != 1 || !hasEvent(events, EventPlayerEliminated) || !hasEvent(events, EventGameOver) {
		t.Fatalf("unsafe explosive elimination got discard=%#v events=%#v", state.DiscardPile, events)
	}
}

func TestTripleComboStealingExplosiveEliminatesRecipientWithoutHolder(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(42)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn), engineTestCard("skip-3", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("holder-1", CardExplosiveHolder), engineTestCard("explosive-1", CardExplosive)}),
		},
		nil,
		"p1",
	)

	if _, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2", "skip-3"}, TargetID: "p2", RequestedCode: CardExplosive}); err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.Players[0].Alive || state.WinnerPlayerID != "p2" || state.Phase != PhaseGameOver {
		t.Fatalf("unsafe explosive recipient should be eliminated, got %#v", state)
	}
	if !hasEvent(events, EventPlayerEliminated) || !hasEvent(events, EventGameOver) {
		t.Fatalf("expected recipient elimination and game over, got %#v", events)
	}
}

func TestUnsafeExplosiveEliminationAdvancesPastCurrentPlayer(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(43)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn), engineTestCard("skip-3", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("holder-1", CardExplosiveHolder), engineTestCard("explosive-1", CardExplosive)}),
			engineTestPlayer("p3", nil),
		},
		nil,
		"p1",
	)

	if _, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2", "skip-3"}, TargetID: "p2", RequestedCode: CardExplosive}); err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.Players[0].Alive || state.Phase != PhasePlayerTurn || state.CurrentPlayerID != "p2" {
		t.Fatalf("current player elimination should advance to p2, got %#v", state)
	}
	if !hasEvent(events, EventPlayerEliminated) || !hasEvent(events, EventTurnStarted) {
		t.Fatalf("expected elimination and next turn events, got %#v", events)
	}
}

func TestRequestCardTransferOfExplosiveHolderEliminatesTarget(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(43)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("request-1", CardRequestCard)}),
			engineTestPlayer("p2", []Card{engineTestCard("holder-1", CardExplosiveHolder), engineTestCard("explosive-1", CardExplosive)}),
		},
		nil,
		"p1",
	)

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"request-1"}, TargetID: "p2"}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(state); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	events, err := engine.ChooseCardForRequest(state, ChooseCardForRequestCommand{PlayerID: "p2", CardID: "holder-1"})
	if err != nil {
		t.Fatalf("ChooseCardForRequest returned error: %v", err)
	}

	if state.Players[1].Alive || state.WinnerPlayerID != "p1" || state.Phase != PhaseGameOver {
		t.Fatalf("holder loss should eliminate p2 and end the game, got %#v", state)
	}
	if !hasEvent(events, EventPlayerEliminated) || !hasEvent(events, EventGameOver) {
		t.Fatalf("expected target elimination and game over, got %#v", events)
	}
}

func TestHolderLossWithShieldRequiresExplosivePlacement(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(44)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn), engineTestCard("skip-3", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("holder-1", CardExplosiveHolder), engineTestCard("explosive-1", CardExplosive), engineTestCard("shield-1", CardShield)}),
		},
		nil,
		"p1",
	)

	if _, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2", "skip-3"}, TargetID: "p2", RequestedCode: CardExplosiveHolder}); err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if !state.Players[1].Alive || state.Phase != PhaseWaitingExplosivePlacement || state.PendingAction == nil || state.PendingAction.SourcePlayerID != "p2" {
		t.Fatalf("shielded holder loss should require placement, got %#v", state)
	}
	if countCode(state.Players[1].Hand, CardExplosive) != 0 || countCode(state.DiscardPile, CardShield) != 1 || !hasEvent(events, EventActionPending) {
		t.Fatalf("shielded holder loss state got hand=%#v discard=%#v events=%#v", state.Players[1].Hand, state.DiscardPile, events)
	}
	if _, err := engine.PlaceExplosive(state, PlaceExplosiveCommand{PlayerID: "p2", Index: 0}); err != nil {
		t.Fatalf("PlaceExplosive returned error: %v", err)
	}
	if state.Phase != PhasePlayerTurn || state.CurrentPlayerID != "p1" {
		t.Fatalf("reactive placement should resume p1's turn, got %#v", state)
	}
}

func TestTransferredExplosiveWithShieldRequiresPlacement(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(45)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn), engineTestCard("skip-3", CardSkipTurn), engineTestCard("shield-1", CardShield)}),
			engineTestPlayer("p2", []Card{engineTestCard("holder-1", CardExplosiveHolder), engineTestCard("explosive-1", CardExplosive)}),
		},
		nil,
		"p1",
	)

	if _, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2", "skip-3"}, TargetID: "p2", RequestedCode: CardExplosive}); err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(state); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if !state.Players[0].Alive || state.Phase != PhaseWaitingExplosivePlacement || state.PendingAction == nil || state.PendingAction.SourcePlayerID != "p1" {
		t.Fatalf("shielded explosive recipient should require placement, got %#v", state)
	}
}

func TestDiscardRecoveryOfExplosiveWithShieldRequiresPlacement(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(46)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("shield-1", CardShield)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("draw-1", CardSkipTurn)},
		"p1",
	)
	state.Phase = PhaseWaitingDiscardRecovery
	state.PendingAction = &PendingAction{ID: "recovery-1", SourcePlayerID: "p1", Type: PendingDiscardRecovery}
	state.DiscardPile = []Card{engineTestCard("explosive-1", CardExplosive)}

	if _, err := engine.ChooseCardFromDiscard(state, ChooseCardFromDiscardCommand{PlayerID: "p1", CardID: "explosive-1"}); err != nil {
		t.Fatalf("ChooseCardFromDiscard returned error: %v", err)
	}
	if !state.Players[0].Alive || state.Phase != PhaseWaitingExplosivePlacement || state.PendingAction == nil || state.PendingAction.SourcePlayerID != "p1" {
		t.Fatalf("shielded explosive recovery should require placement, got %#v", state)
	}
	if countCode(state.DiscardPile, CardShield) != 1 {
		t.Fatalf("recovery shield should be discarded, got %#v", state.DiscardPile)
	}
}

func TestMultipleUnsafeExplosivesUseShieldsBeforeElimination(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(47)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("explosive-1", CardExplosive), engineTestCard("explosive-2", CardExplosive), engineTestCard("shield-1", CardShield)}),
			engineTestPlayer("p2", nil),
		},
		nil,
		"p1",
	)

	engine.resolveUnsafeExplosives(state, "p1")
	if !state.Players[0].Alive || state.Phase != PhaseWaitingExplosivePlacement {
		t.Fatalf("first shield should allow the first explosive placement, got %#v", state)
	}
	if _, err := engine.PlaceExplosive(state, PlaceExplosiveCommand{PlayerID: "p1", Index: 0}); err != nil {
		t.Fatalf("PlaceExplosive returned error: %v", err)
	}
	if state.Players[0].Alive || state.Phase != PhaseGameOver || state.WinnerPlayerID != "p2" {
		t.Fatalf("player should be eliminated only after the remaining unshielded explosive, got %#v", state)
	}
}

func TestMultipleUnsafeExplosivesWithShieldsRequireSerialPlacements(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(47)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("explosive-1", CardExplosive), engineTestCard("explosive-2", CardExplosive), engineTestCard("shield-1", CardShield), engineTestCard("shield-2", CardShield)}),
			engineTestPlayer("p2", nil),
		},
		nil,
		"p1",
	)

	engine.resolveUnsafeExplosives(state, "p1")
	if state.Phase != PhaseWaitingExplosivePlacement || state.PendingAction == nil {
		t.Fatalf("multiple unsafe explosives should start a placement, got %#v", state)
	}
	if _, err := engine.PlaceExplosive(state, PlaceExplosiveCommand{PlayerID: "p1", Index: 0}); err != nil {
		t.Fatalf("first PlaceExplosive returned error: %v", err)
	}
	if state.Phase != PhaseWaitingExplosivePlacement || state.PendingAction == nil || state.PendingAction.SourcePlayerID != "p1" {
		t.Fatalf("second explosive should require placement, got %#v", state)
	}
	if _, err := engine.PlaceExplosive(state, PlaceExplosiveCommand{PlayerID: "p1", Index: 0}); err != nil {
		t.Fatalf("second PlaceExplosive returned error: %v", err)
	}
	if !state.Players[0].Alive || state.Phase != PhasePlayerTurn || countCode(state.DiscardPile, CardShield) != 2 {
		t.Fatalf("shielded multiple explosives should survive after placement, got %#v", state)
	}
}

func TestUnsafeExplosivePlacementsAreQueuedAndResumeOriginalTurn(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(47)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("p1-explosive", CardExplosive), engineTestCard("p1-shield", CardShield)}),
			engineTestPlayer("p2", []Card{engineTestCard("p2-explosive", CardExplosive), engineTestCard("p2-shield", CardShield)}),
			engineTestPlayer("p3", nil),
		},
		nil,
		"p1",
	)

	engine.resolveUnsafeExplosives(state, "p1", "p2")
	if state.Phase != PhaseWaitingExplosivePlacement || state.PendingAction.SourcePlayerID != "p1" || len(state.UnsafeExplosiveQueue) != 1 {
		t.Fatalf("first unsafe placement got %#v", state)
	}
	if _, err := engine.PlaceExplosive(state, PlaceExplosiveCommand{PlayerID: "p1", Index: 0}); err != nil {
		t.Fatalf("first PlaceExplosive returned error: %v", err)
	}
	if state.Phase != PhaseWaitingExplosivePlacement || state.PendingAction.SourcePlayerID != "p2" {
		t.Fatalf("second unsafe placement was not queued, got %#v", state)
	}
	if _, err := engine.PlaceExplosive(state, PlaceExplosiveCommand{PlayerID: "p2", Index: 0}); err != nil {
		t.Fatalf("second PlaceExplosive returned error: %v", err)
	}
	if state.Phase != PhasePlayerTurn || state.CurrentPlayerID != "p1" {
		t.Fatalf("queued placements should resume p1, got %#v", state)
	}
}

func TestUnsafeExplosiveQueueAdvancesFromEliminatedOriginalPlayer(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(48)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("p1-explosive", CardExplosive)}),
			engineTestPlayer("p2", []Card{engineTestCard("p2-explosive", CardExplosive), engineTestCard("p2-shield", CardShield)}),
			engineTestPlayer("p3", nil),
		},
		nil,
		"p1",
	)

	events := engine.resolveUnsafeExplosives(state, "p1", "p2")
	if state.Players[0].Alive || state.Phase != PhaseWaitingExplosivePlacement || state.PendingAction.SourcePlayerID != "p2" || hasEvent(events, EventGameOver) {
		t.Fatalf("mixed unsafe state got %#v events=%#v", state, events)
	}
	if _, err := engine.PlaceExplosive(state, PlaceExplosiveCommand{PlayerID: "p2", Index: 0}); err != nil {
		t.Fatalf("PlaceExplosive returned error: %v", err)
	}
	if state.Phase != PhasePlayerTurn || state.CurrentPlayerID != "p2" {
		t.Fatalf("mixed unsafe resolution should advance from p1 to p2, got %#v", state)
	}
}

func TestDrawExplosiveWithShieldMovesToPlacementPhase(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(18)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("shield-1", CardShield)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("danger-1", CardExplosive), engineTestCard("draw-2", CardSkipTurn)},
		"p1",
	)

	_, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("DrawCard returned error: %v", err)
	}

	if state.Phase != PhaseWaitingExplosivePlacement {
		t.Fatalf("phase got %s, want %s", state.Phase, PhaseWaitingExplosivePlacement)
	}
	if state.PendingAction == nil || state.PendingAction.Type != PendingExplosivePlacement {
		t.Fatalf("expected explosive placement pending action, got %#v", state.PendingAction)
	}
	if countCode(state.Players[0].Hand, CardShield) != 0 {
		t.Fatal("shield should be consumed")
	}
	if countCode(state.DiscardPile, CardShield) != 1 {
		t.Fatal("shield should be discarded")
	}
}

func TestPlaceExplosiveReturnsGameToNextTurn(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(16)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("shield-1", CardShield)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("danger-1", CardExplosive), engineTestCard("draw-2", CardSkipTurn)},
		"p1",
	)
	_, err := engine.DrawCard(state, DrawCardCommand{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("DrawCard returned error: %v", err)
	}

	_, err = engine.PlaceExplosive(state, PlaceExplosiveCommand{PlayerID: "p1", Index: 1})
	if err != nil {
		t.Fatalf("PlaceExplosive returned error: %v", err)
	}

	if state.Phase != PhasePlayerTurn {
		t.Fatalf("phase got %s, want %s", state.Phase, PhasePlayerTurn)
	}
	if state.CurrentPlayerID != "p2" {
		t.Fatalf("current player got %s, want p2", state.CurrentPlayerID)
	}
	if state.PendingAction != nil {
		t.Fatalf("pending action should be cleared: %#v", state.PendingAction)
	}
	if state.DrawPile[1].ID != "danger-1" {
		t.Fatalf("placed card got %s, want danger-1", state.DrawPile[1].ID)
	}
}

func TestPlaySkipTurnEndsCurrentTurn(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(17)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn)}), engineTestPlayer("p2", nil)}, nil, "p1")

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"skip-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.CurrentPlayerID != "p2" {
		t.Fatalf("current player got %s, want p2", state.CurrentPlayerID)
	}
	if countCode(state.DiscardPile, CardSkipTurn) != 1 {
		t.Fatal("skip card should be discarded")
	}
}

func TestPlaySkipAllTurnsClearsCurrentDebt(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(18)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("super-1", CardSkipAllTurns)}), engineTestPlayer("p2", nil)}, nil, "p1")
	state.TurnDebt["p1"] = 3

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"super-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.TurnDebt["p1"] != 0 {
		t.Fatalf("p1 debt got %d, want 0", state.TurnDebt["p1"])
	}
	if state.CurrentPlayerID != "p2" {
		t.Fatalf("current player got %s, want p2", state.CurrentPlayerID)
	}
}

func TestPlayForceExtraTurnsAddsDebtToNextPlayer(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(19)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("force-1", CardForceExtraTurns)}), engineTestPlayer("p2", nil)}, nil, "p1")

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"force-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.CurrentPlayerID != "p2" {
		t.Fatalf("current player got %s, want p2", state.CurrentPlayerID)
	}
	if state.TurnDebt["p2"] != 2 {
		t.Fatalf("p2 debt got %d, want 2", state.TurnDebt["p2"])
	}
}

func TestPlayTargetExtraTurnsAddsDebtToSelectedTarget(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(20)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("target-1", CardTargetExtraTurns)}), engineTestPlayer("p2", nil), engineTestPlayer("p3", nil)},
		nil,
		"p1",
	)

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"target-1"}, TargetID: "p3"})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.CurrentPlayerID != "p3" {
		t.Fatalf("current player got %s, want p3", state.CurrentPlayerID)
	}
	if state.TurnDebt["p3"] != 2 {
		t.Fatalf("p3 debt got %d, want 2", state.TurnDebt["p3"])
	}
}

func TestRequestCardRequiresTarget(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(21)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("request-1", CardRequestCard)}), engineTestPlayer("p2", []Card{engineTestCard("hand-1", CardSkipTurn)})}, nil, "p1")

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"request-1"}})
	if !errors.Is(err, ErrTargetRequired) {
		t.Fatalf("PlayCard error got %v, want ErrTargetRequired", err)
	}
	if len(state.Players[0].Hand) != 1 {
		t.Fatal("invalid request should not mutate hand")
	}
}

func TestRequestCardCreatesPrivatePrompt(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(22)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("request-1", CardRequestCard)}), engineTestPlayer("p2", []Card{engineTestCard("hand-1", CardSkipTurn)})}, nil, "p1")

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"request-1"}, TargetID: "p2"})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.Phase != PhaseWaitingRequestCardChoice {
		t.Fatalf("phase got %s, want %s", state.Phase, PhaseWaitingRequestCardChoice)
	}
	if state.PendingAction == nil || state.PendingAction.TargetPlayerID != "p2" {
		t.Fatalf("expected request pending action for p2, got %#v", state.PendingAction)
	}
	p1View, err := BuildViewForPlayer(state, "p1")
	if err != nil {
		t.Fatalf("BuildViewForPlayer p1 returned error: %v", err)
	}
	p2View, err := BuildViewForPlayer(state, "p2")
	if err != nil {
		t.Fatalf("BuildViewForPlayer p2 returned error: %v", err)
	}
	if commandListContains(p1View.AvailableActions, CommandChooseCardForRequest) || !commandListContains(p2View.AvailableActions, CommandChooseCardForRequest) {
		t.Fatalf("request actions source=%#v target=%#v, want only target to choose", p1View.AvailableActions, p2View.AvailableActions)
	}
	if !hasEvent(events, EventPrivatePromptSent) {
		t.Fatalf("expected private prompt event, got %#v", events)
	}
}

func TestRequestCardTargetChoosesAndTransfersCardPrivately(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(23)))
	state := engineTestState([]Player{
		engineTestPlayer("p1", []Card{engineTestCard("request-1", CardRequestCard)}),
		engineTestPlayer("p2", []Card{engineTestCard("give-1", CardSkipTurn), engineTestCard("keep-1", CardShuffleDeck)}),
	}, nil, "p1")

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"request-1"}, TargetID: "p2"}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(state); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if _, err := engine.ChooseCardForRequest(state, ChooseCardForRequestCommand{PlayerID: "p1", CardID: "give-1"}); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("non-target choice error got %v, want ErrNotYourTurn", err)
	}
	if _, err := engine.ChooseCardForRequest(state, ChooseCardForRequestCommand{PlayerID: "p2", CardID: "missing"}); !errors.Is(err, ErrCardNotInHand) {
		t.Fatalf("missing card choice error got %v, want ErrCardNotInHand", err)
	}

	events, err := engine.ChooseCardForRequest(state, ChooseCardForRequestCommand{PlayerID: "p2", CardID: "give-1"})
	if err != nil {
		t.Fatalf("ChooseCardForRequest returned error: %v", err)
	}
	if state.Phase != PhasePlayerTurn || state.PendingAction != nil {
		t.Fatalf("request state got phase=%s pending=%#v, want player turn without pending action", state.Phase, state.PendingAction)
	}
	if len(state.Players[0].Hand) != 1 || state.Players[0].Hand[0].ID != "give-1" {
		t.Fatalf("source hand got %#v, want transferred card", state.Players[0].Hand)
	}
	if len(state.Players[1].Hand) != 1 || state.Players[1].Hand[0].ID != "keep-1" {
		t.Fatalf("target hand got %#v, want unselected card", state.Players[1].Hand)
	}
	if len(events) != 1 || events[0].Type != EventActionResolved || len(events[0].CardIDs) != 0 || events[0].TargetID != "p2" {
		t.Fatalf("request resolution event got %#v, want public transfer without card identity", events)
	}
}

func TestCollectiveRecycleWaitsForAllEligiblePlayers(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(44)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("recycle-1", CardCollectiveRecycle), engineTestCard("p1-card", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("p2-card", CardShuffleDeck)}),
			engineTestPlayer("p3", nil),
		},
		[]Card{engineTestCard("draw-1", CardTokenA), engineTestCard("draw-2", CardTokenB)},
		"p1",
	)

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"recycle-1"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.Phase != PhaseWaitingRecycleChoices || state.PendingAction == nil || state.PendingAction.Type != PendingRecycleChoices {
		t.Fatalf("collective recycle state got phase=%s pending=%#v", state.Phase, state.PendingAction)
	}
	if !reflect.DeepEqual(state.PendingAction.RecyclePlayerIDs, []string{"p1", "p2"}) {
		t.Fatalf("recycle eligibility got %v, want p1 and p2", state.PendingAction.RecyclePlayerIDs)
	}
	for _, event := range events {
		if event.Type == EventPrivatePromptSent && len(event.CardIDs) != 0 {
			t.Fatalf("recycle prompt must not include card IDs: %#v", event)
		}
	}

	if _, err := engine.ChooseCardForRecycle(state, ChooseCardForRecycleCommand{PlayerID: "p3", CardID: "missing"}); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("ineligible recycle choice error got %v, want ErrNotYourTurn", err)
	}
	if _, err := engine.ChooseCardForRecycle(state, ChooseCardForRecycleCommand{PlayerID: "p1", CardID: "p1-card"}); err != nil {
		t.Fatalf("first recycle choice returned error: %v", err)
	}
	if len(state.DrawPile) != 2 || state.Phase != PhaseWaitingRecycleChoices {
		t.Fatalf("first recycle choice should wait, got draw=%#v phase=%s", state.DrawPile, state.Phase)
	}
	if _, err := engine.ChooseCardForRecycle(state, ChooseCardForRecycleCommand{PlayerID: "p1", CardID: "p1-card"}); !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("duplicate recycle choice error got %v, want ErrInvalidCardPlay", err)
	}

	publicView, err := BuildViewForPlayer(state, "p3")
	if err != nil {
		t.Fatalf("BuildViewForPlayer returned error: %v", err)
	}
	publicPayload, err := json.Marshal(publicView)
	if err != nil {
		t.Fatalf("Marshal public view returned error: %v", err)
	}
	if strings.Contains(string(publicPayload), "p1-card") || strings.Contains(string(publicPayload), "p2-card") {
		t.Fatalf("public recycle view leaked selected or selectable card IDs: %s", publicPayload)
	}

	events, err = engine.ChooseCardForRecycle(state, ChooseCardForRecycleCommand{PlayerID: "p2", CardID: "p2-card"})
	if err != nil {
		t.Fatalf("final recycle choice returned error: %v", err)
	}
	if state.Phase != PhasePlayerTurn || state.PendingAction != nil || len(state.DrawPile) != 4 {
		t.Fatalf("completed recycle state got phase=%s pending=%#v draw=%#v", state.Phase, state.PendingAction, state.DrawPile)
	}
	if findCardIndexByID(state.DrawPile, "p1-card") < 0 || findCardIndexByID(state.DrawPile, "p2-card") < 0 {
		t.Fatalf("recycled cards were not returned to draw pile: %#v", state.DrawPile)
	}
	if !hasEvent(events, EventActionResolved) {
		t.Fatalf("completed recycle should emit action resolved, got %#v", events)
	}
}

func TestCollectiveRecycleCanceledBeforeChoices(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(45)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("recycle-1", CardCollectiveRecycle), engineTestCard("p1-card", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("cancel-1", CardCancel), engineTestCard("p2-card", CardShuffleDeck)}),
		},
		nil,
		"p1",
	)

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"recycle-1"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1"}); err != nil {
		t.Fatalf("PlayCancel returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.Phase != PhasePlayerTurn || state.PendingAction != nil || !hasEvent(events, EventActionCanceled) {
		t.Fatalf("canceled recycle state got phase=%s pending=%#v events=%#v", state.Phase, state.PendingAction, events)
	}
}

func TestCollectiveRecycleWithNoEligiblePlayersResolvesImmediately(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(46)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("recycle-1", CardCollectiveRecycle)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("draw-1", CardSkipTurn)},
		"p1",
	)

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"recycle-1"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(state); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.Phase != PhasePlayerTurn || state.PendingAction != nil {
		t.Fatalf("zero-eligible recycle state got phase=%s pending=%#v", state.Phase, state.PendingAction)
	}
}

func TestPeekDeckSendsPrivatePromptOnly(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(23)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("peek-1", CardPeekDeck)}), engineTestPlayer("p2", nil)}, []Card{engineTestCard("draw-1", CardSkipTurn)}, "p1")

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"peek-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if state.Phase != PhasePlayerTurn {
		t.Fatalf("phase got %s, want %s", state.Phase, PhasePlayerTurn)
	}
	if !hasEvent(events, EventPrivatePromptSent) {
		t.Fatalf("expected private prompt event, got %#v", events)
	}
}

func TestShuffleDeckChangesDrawPileOrderAndPreservesCards(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(24)))
	drawPile := []Card{
		engineTestCard("draw-1", CardTokenA),
		engineTestCard("draw-2", CardTokenA),
		engineTestCard("draw-3", CardTokenA),
		engineTestCard("draw-4", CardTokenA),
		engineTestCard("draw-5", CardTokenA),
	}
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("shuffle-1", CardShuffleDeck)}), engineTestPlayer("p2", nil)}, append([]Card(nil), drawPile...), "p1")
	beforeIDs := cardIDs(state.DrawPile)

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"shuffle-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	afterIDs := cardIDs(state.DrawPile)
	if reflect.DeepEqual(beforeIDs, afterIDs) {
		t.Fatalf("draw pile order did not change: %v", afterIDs)
	}
	if !sameStringSet(beforeIDs, afterIDs) {
		t.Fatalf("draw pile cards changed, before=%v after=%v", beforeIDs, afterIDs)
	}
}

func TestCancelableActionCreatesPendingAction(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(25)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("shuffle-1", CardShuffleDeck)}), engineTestPlayer("p2", nil)}, []Card{engineTestCard("draw-1", CardTokenA), engineTestCard("draw-2", CardTokenA)}, "p1")
	beforeIDs := cardIDs(state.DrawPile)

	events, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"shuffle-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}

	if state.Phase != PhaseCancelWindow {
		t.Fatalf("phase got %s, want %s", state.Phase, PhaseCancelWindow)
	}
	if state.PendingAction == nil || state.PendingAction.Type != PendingPlayCard || state.PendingAction.SourcePlayerID != "p1" {
		t.Fatalf("expected pending play-card action, got %#v", state.PendingAction)
	}
	if state.PendingAction.ExpiresAtUnixMs == 0 {
		t.Fatal("pending action should have an expiration")
	}
	if !reflect.DeepEqual(beforeIDs, cardIDs(state.DrawPile)) {
		t.Fatal("action should not resolve before cancel window expires")
	}
	if !hasEvent(events, EventActionPending) {
		t.Fatalf("expected action pending event, got %#v", events)
	}
}

func TestPlayerWithCancelCardCanCancelPendingAction(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(26)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn)}), engineTestPlayer("p2", []Card{engineTestCard("cancel-1", CardCancel)})},
		nil,
		"p1",
	)

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"skip-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	_, err = engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1", PendingActionID: state.PendingAction.ID})
	if err != nil {
		t.Fatalf("PlayCancel returned error: %v", err)
	}

	if state.PendingAction.CancelCount != 1 {
		t.Fatalf("cancel count got %d, want 1", state.PendingAction.CancelCount)
	}
	if countCode(state.DiscardPile, CardCancel) != 1 {
		t.Fatal("cancel card should be discarded")
	}

	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.PendingAction != nil {
		t.Fatalf("pending action should be cleared: %#v", state.PendingAction)
	}
	if state.CurrentPlayerID != "p1" {
		t.Fatalf("canceled skip should not advance turn; got current player %s", state.CurrentPlayerID)
	}
	if !hasEvent(events, EventActionCanceled) {
		t.Fatalf("expected action canceled event, got %#v", events)
	}
}

func TestPlayerWithoutCancelCardCannotCancel(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(27)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn)}), engineTestPlayer("p2", nil)},
		nil,
		"p1",
	)

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"skip-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	_, err = engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2"})
	if !errors.Is(err, ErrCardNotInHand) {
		t.Fatalf("PlayCancel error got %v, want ErrCardNotInHand", err)
	}
	if state.PendingAction.CancelCount != 0 {
		t.Fatalf("cancel count got %d, want 0", state.PendingAction.CancelCount)
	}
}

func TestTwoCancelsAllowActionToResolve(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(28)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("cancel-2", CardCancel)}),
			engineTestPlayer("p2", []Card{engineTestCard("cancel-1", CardCancel)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"skip-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	_, err = engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1"})
	if err != nil {
		t.Fatalf("first PlayCancel returned error: %v", err)
	}
	_, err = engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p1", CardID: "cancel-2"})
	if err != nil {
		t.Fatalf("second PlayCancel returned error: %v", err)
	}

	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.CurrentPlayerID != "p2" {
		t.Fatalf("two cancels should allow skip to resolve; got current player %s", state.CurrentPlayerID)
	}
	if hasEvent(events, EventActionCanceled) || !hasEvent(events, EventActionResolved) {
		t.Fatalf("expected action resolved, got %#v", events)
	}
}

func TestCancelCannotBePlayedWhenNoPendingActionExists(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(29)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("cancel-1", CardCancel)}), engineTestPlayer("p2", nil)}, nil, "p1")

	_, err := engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p1", CardID: "cancel-1"})
	if !errors.Is(err, ErrNoPendingAction) {
		t.Fatalf("PlayCancel error got %v, want ErrNoPendingAction", err)
	}
}

func TestCancelCannotTargetNonCancelableEvents(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(30)))
	state := engineTestState([]Player{engineTestPlayer("p1", nil), engineTestPlayer("p2", []Card{engineTestCard("cancel-1", CardCancel)})}, nil, "p1")
	state.Phase = PhaseCancelWindow
	state.PendingAction = &PendingAction{ID: "pending-1", SourcePlayerID: "p1", Type: PendingExplosivePlacement, CardIDs: []string{"danger-1"}, Cards: []Card{engineTestCard("danger-1", CardExplosive)}}

	_, err := engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1", PendingActionID: "pending-1"})
	if !errors.Is(err, ErrActionNotCancelable) {
		t.Fatalf("PlayCancel error got %v, want ErrActionNotCancelable", err)
	}
	if countCode(state.Players[1].Hand, CardCancel) != 1 {
		t.Fatal("invalid cancel should not remove card from hand")
	}
}

func TestPendingActionResolvesWhenTimerExpires(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(31)))
	state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn)}), engineTestPlayer("p2", nil)}, nil, "p1")

	_, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"skip-1"}})
	if err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	expiresAt := state.PendingAction.ExpiresAtUnixMs
	_, err = engine.ExpireCancelWindow(state, time.UnixMilli(expiresAt).Add(time.Millisecond))
	if err != nil {
		t.Fatalf("ExpireCancelWindow returned error: %v", err)
	}

	if state.PendingAction != nil {
		t.Fatalf("pending action should be cleared: %#v", state.PendingAction)
	}
	if state.CurrentPlayerID != "p2" {
		t.Fatalf("resolved skip should advance turn to p2, got %s", state.CurrentPlayerID)
	}
}

func TestTwoOfKindNonTokenStealsRandomCardFromTarget(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(32)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardShuffleDeck)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2"}, TargetID: "p2"})
	if err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	if state.Phase != PhaseCancelWindow || state.PendingAction == nil || state.PendingAction.Type != PendingCardCombo || state.PendingAction.ComboKind != ComboPair {
		t.Fatalf("expected pending pair combo, got phase=%s pending=%#v", state.Phase, state.PendingAction)
	}

	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if len(state.Players[1].Hand) != 0 {
		t.Fatalf("target hand size got %d, want 0", len(state.Players[1].Hand))
	}
	if findCardIndexByID(state.Players[0].Hand, "target-card") < 0 {
		t.Fatal("source should receive stolen target card")
	}
	if countCode(state.DiscardPile, CardSkipTurn) != 2 {
		t.Fatal("combo cards should move to discard")
	}
}

func TestTokenPairWithWildIsValid(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(33)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("token-1", CardTokenA), engineTestCard("wild-1", CardWildToken)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardShuffleDeck)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"token-1", "wild-1"}, TargetID: "p2"})
	if err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
}

func TestTokenCannotMixWithActionCard(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(34)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("token-1", CardTokenA), engineTestCard("skip-1", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardShuffleDeck)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"token-1", "skip-1"}, TargetID: "p2"})
	if !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("PlayCombo error got %v, want ErrInvalidCardPlay", err)
	}
	if len(state.Players[0].Hand) != 2 || len(state.DiscardPile) != 0 {
		t.Fatal("invalid combo should not mutate state")
	}
}

func TestWildTokenCannotSubstituteForNonTokenPair(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(35)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("wild-1", CardWildToken), engineTestCard("skip-1", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardShuffleDeck)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"wild-1", "skip-1"}, TargetID: "p2"})
	if !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("PlayCombo error got %v, want ErrInvalidCardPlay", err)
	}
}

func TestThreeOfKindRequestsDeclaredCardCode(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(36)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn), engineTestCard("skip-3", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardShuffleDeck), engineTestCard("other-card", CardPeekDeck)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2", "skip-3"}, TargetID: "p2", RequestedCode: CardShuffleDeck})
	if err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if findCardIndexByID(state.Players[0].Hand, "target-card") < 0 {
		t.Fatal("source should receive requested card")
	}
	if findCardIndexByID(state.Players[1].Hand, "target-card") >= 0 {
		t.Fatal("target should lose requested card")
	}
}

func TestThreeOfKindTransfersNothingWhenTargetLacksRequestedCode(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(37)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn), engineTestCard("skip-3", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("other-card", CardPeekDeck)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2", "skip-3"}, TargetID: "p2", RequestedCode: CardShuffleDeck})
	if err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if len(state.Players[1].Hand) != 1 || state.Players[1].Hand[0].ID != "other-card" {
		t.Fatalf("target hand should be unchanged, got %#v", state.Players[1].Hand)
	}
}

func TestPairOrTripleWithExplosiveIsInvalid(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(38)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("danger-1", CardExplosive), engineTestCard("danger-2", CardExplosive)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardShuffleDeck)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"danger-1", "danger-2"}, TargetID: "p2"})
	if !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("PlayCombo error got %v, want ErrInvalidCardPlay", err)
	}
}

func TestFiveDifferentComboRecoversDiscardCard(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(39)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{
				engineTestCard("skip-1", CardSkipTurn),
				engineTestCard("shuffle-1", CardShuffleDeck),
				engineTestCard("peek-1", CardPeekDeck),
				engineTestCard("request-1", CardRequestCard),
				engineTestCard("shield-1", CardShield),
			}),
			engineTestPlayer("p2", nil),
		},
		nil,
		"p1",
	)
	state.DiscardPile = []Card{engineTestCard("recover-me", CardTargetExtraTurns)}

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "shuffle-1", "peek-1", "request-1", "shield-1"}})
	if err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.Phase != PhaseWaitingDiscardRecovery || state.PendingAction == nil || state.PendingAction.Type != PendingDiscardRecovery {
		t.Fatalf("expected discard recovery prompt, phase=%s pending=%#v", state.Phase, state.PendingAction)
	}

	_, err = engine.ChooseCardFromDiscard(state, ChooseCardFromDiscardCommand{PlayerID: "p1", CardID: "recover-me"})
	if err != nil {
		t.Fatalf("ChooseCardFromDiscard returned error: %v", err)
	}
	if findCardIndexByID(state.Players[0].Hand, "recover-me") < 0 {
		t.Fatal("source should recover chosen discard card")
	}
	if state.Phase != PhasePlayerTurn || state.PendingAction != nil {
		t.Fatalf("recovery should clear pending action and return to turn, phase=%s pending=%#v", state.Phase, state.PendingAction)
	}
}

func TestFiveDifferentComboRejectsDuplicatesAndExplosiveHolder(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(40)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{
			engineTestCard("skip-1", CardSkipTurn),
			engineTestCard("skip-2", CardSkipTurn),
			engineTestCard("peek-1", CardPeekDeck),
			engineTestCard("request-1", CardRequestCard),
			engineTestCard("shield-1", CardShield),
			engineTestCard("holder-1", CardExplosiveHolder),
		}), engineTestPlayer("p2", nil)},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2", "peek-1", "request-1", "shield-1"}})
	if !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("duplicate-code five combo error got %v, want ErrInvalidCardPlay", err)
	}
	_, err = engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "holder-1", "peek-1", "request-1", "shield-1"}})
	if !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("holder five combo error got %v, want ErrInvalidCardPlay", err)
	}
}

func TestFiveDifferentComboExplosiveRequiresHolder(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(41)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{
			engineTestCard("danger-1", CardExplosive),
			engineTestCard("skip-1", CardSkipTurn),
			engineTestCard("peek-1", CardPeekDeck),
			engineTestCard("request-1", CardRequestCard),
			engineTestCard("shield-1", CardShield),
			engineTestCard("holder-1", CardExplosiveHolder),
		}), engineTestPlayer("p2", nil)},
		nil,
		"p1",
	)

	stateWithoutHolder := *state
	stateWithoutHolder.Players = append([]Player(nil), state.Players...)
	stateWithoutHolder.Players[0].Hand = removeCardAt(state.Players[0].Hand, findCardIndexByID(state.Players[0].Hand, "holder-1"))
	_, err := engine.PlayCombo(&stateWithoutHolder, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"danger-1", "skip-1", "peek-1", "request-1", "shield-1"}})
	if !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("explosive without holder combo error got %v, want ErrInvalidCardPlay", err)
	}

	_, err = engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"danger-1", "skip-1", "peek-1", "request-1", "shield-1"}})
	if err != nil {
		t.Fatalf("explosive with holder PlayCombo returned error: %v", err)
	}
}

func TestCanceledComboDoesNotResolve(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(42)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardShuffleDeck), engineTestCard("cancel-1", CardCancel)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2"}, TargetID: "p2"})
	if err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	_, err = engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1"})
	if err != nil {
		t.Fatalf("PlayCancel returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if findCardIndexByID(state.Players[1].Hand, "target-card") < 0 {
		t.Fatal("canceled combo should not steal target card")
	}
	if findCardIndexByID(state.Players[0].Hand, "target-card") >= 0 {
		t.Fatal("source should not receive card from canceled combo")
	}
}

func TestTwoCancelsAllowComboToResolve(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(43)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn), engineTestCard("skip-2", CardSkipTurn), engineTestCard("cancel-2", CardCancel)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardShuffleDeck), engineTestCard("cancel-1", CardCancel)}),
		},
		nil,
		"p1",
	)

	_, err := engine.PlayCombo(state, PlayComboCommand{PlayerID: "p1", CardIDs: []string{"skip-1", "skip-2"}, TargetID: "p2"})
	if err != nil {
		t.Fatalf("PlayCombo returned error: %v", err)
	}
	_, err = engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1"})
	if err != nil {
		t.Fatalf("first PlayCancel returned error: %v", err)
	}
	_, err = engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p1", CardID: "cancel-2"})
	if err != nil {
		t.Fatalf("second PlayCancel returned error: %v", err)
	}
	_, err = engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}

	if findCardIndexByID(state.Players[0].Hand, "target-card") < 0 {
		t.Fatal("two cancels should allow combo to resolve")
	}
}

func TestRevealHeldCardCancellationAndTargetValidation(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(44)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("reveal-1", CardRevealHeldCard), engineTestCard("cancel-1", CardCancel)}),
			engineTestPlayer("p2", []Card{engineTestCard("target-card", CardSkipTurn), engineTestCard("cancel-2", CardCancel)}),
		},
		nil,
		"p1",
	)

	for _, targetID := range []string{"p1", "missing"} {
		if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"reveal-1"}, TargetID: targetID}); err == nil {
			t.Fatalf("PlayCard should reject target %q", targetID)
		}
	}
	state.Players[1].Alive = false
	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"reveal-1"}, TargetID: "p2"}); err == nil {
		t.Fatal("PlayCard should reject a dead target")
	}
	state.Players[1].Alive = true

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"reveal-1"}, TargetID: "p2"}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-2"}); err != nil {
		t.Fatalf("PlayCancel returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(state); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.PendingAction != nil || state.Phase != PhasePlayerTurn || len(state.MarkedCards) != 0 {
		t.Fatalf("canceled reveal should not prompt or mark: %#v", state)
	}
}

func TestRevealHeldCardMarksAndClearsWhenCardLeavesHand(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(45)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("reveal-1", CardRevealHeldCard)}),
			engineTestPlayer("p2", []Card{engineTestCard("marked-card", CardSkipTurn)}),
		},
		nil,
		"p1",
	)

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"reveal-1"}, TargetID: "p2"}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.Phase != PhaseWaitingMarkedCardChoice || state.PendingAction == nil || state.PendingAction.TargetPlayerID != "p2" {
		t.Fatalf("reveal should prompt its target after cancellation resolves: %#v", state)
	}
	if len(events) != 2 || events[1].Type != EventPrivatePromptSent || events[1].PlayerID != "p1" || events[1].TargetID != "p2" || !reflect.DeepEqual(events[1].CardIDs, []string{"marked-card"}) || len(events[1].Cards) != 0 {
		t.Fatalf("source should receive only the target hand's opaque card IDs: %#v", events)
	}
	if filtered := FilterEventsForPlayer(events, "p2"); len(filtered) != 1 {
		t.Fatalf("target should not receive source's private choice prompt: %#v", filtered)
	}
	if _, err := engine.ChooseMarkedCard(state, ChooseMarkedCardCommand{PlayerID: "p2", CardID: "marked-card"}); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("wrong player error got %v, want ErrNotYourTurn", err)
	}
	if _, err := engine.ChooseMarkedCard(state, ChooseMarkedCardCommand{PlayerID: "p1", CardID: "missing"}); !errors.Is(err, ErrCardNotInHand) {
		t.Fatalf("missing card error got %v, want ErrCardNotInHand", err)
	}
	markEvents, err := engine.ChooseMarkedCard(state, ChooseMarkedCardCommand{PlayerID: "p1", CardID: "marked-card"})
	if err != nil {
		t.Fatalf("ChooseMarkedCard returned error: %v", err)
	}
	if len(markEvents) != 1 || markEvents[0].TargetID != "p2" {
		t.Fatalf("marked-card resolution should retain the target owner: %#v", markEvents)
	}
	if marked := state.MarkedCards["marked-card"]; marked.OwnerID != "p2" || marked.Revealed.Code != CardSkipTurn {
		t.Fatalf("unexpected mark: %#v", marked)
	}

	state.CurrentPlayerID = "p2"
	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p2", CardIDs: []string{"marked-card"}}); err != nil {
		t.Fatalf("PlayCard marked card returned error: %v", err)
	}
	if _, marked := state.MarkedCards["marked-card"]; marked {
		t.Fatalf("mark should clear after the card leaves its owner hand: %#v", state.MarkedCards)
	}
}

func TestMarkedCardClearsAfterRequestTransfer(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(46)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", nil),
			engineTestPlayer("p2", []Card{engineTestCard("marked-card", CardSkipTurn)}),
		},
		nil,
		"p1",
	)
	state.Phase = PhaseWaitingRequestCardChoice
	state.PendingAction = &PendingAction{ID: "request-1", SourcePlayerID: "p1", TargetPlayerID: "p2", Type: PendingRequestCardChoice}
	state.MarkedCards["marked-card"] = MarkedCard{CardID: "marked-card", OwnerID: "p2", Revealed: engineTestCard("marked-card", CardSkipTurn)}

	if _, err := engine.ChooseCardForRequest(state, ChooseCardForRequestCommand{PlayerID: "p2", CardID: "marked-card"}); err != nil {
		t.Fatalf("ChooseCardForRequest returned error: %v", err)
	}
	if _, marked := state.MarkedCards["marked-card"]; marked {
		t.Fatalf("mark should clear after transfer: %#v", state.MarkedCards)
	}
}

func TestDrawFromBottomDrawsBottomCardAndKeepsIdentityPrivate(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(47)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("bottom-draw", CardDrawFromBottom)}),
			engineTestPlayer("p2", nil),
		},
		[]Card{engineTestCard("top-card", CardSkipTurn), engineTestCard("bottom-card", CardShuffleDeck)},
		"p1",
	)

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"bottom-draw"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if findCardIndexByID(state.Players[0].Hand, "bottom-card") < 0 || len(state.DrawPile) != 1 || state.DrawPile[0].ID != "top-card" {
		t.Fatalf("bottom draw should draw only the bottom card: %#v", state)
	}
	if state.CurrentPlayerID != "p2" {
		t.Fatalf("bottom draw should end p1's turn, current player=%s", state.CurrentPlayerID)
	}
	if filtered := FilterEventsForPlayer(events, "p2"); len(filtered) < 2 || filtered[1].Type != EventCardDrawn || len(filtered[1].CardIDs) != 0 || len(filtered[1].Cards) != 0 {
		t.Fatalf("bottom draw leaked card identity to another player: %#v", filtered)
	}
}

func TestDrawFromBottomCancellationAndEmptyPileAreAtomic(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(48)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("bottom-draw", CardDrawFromBottom)}),
			engineTestPlayer("p2", []Card{engineTestCard("cancel-1", CardCancel)}),
		},
		[]Card{engineTestCard("bottom-card", CardSkipTurn)},
		"p1",
	)
	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"bottom-draw"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.PlayCancel(state, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1"}); err != nil {
		t.Fatalf("PlayCancel returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(state); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if len(state.DrawPile) != 1 || state.DrawPile[0].ID != "bottom-card" || findCardIndexByID(state.Players[0].Hand, "bottom-card") >= 0 {
		t.Fatalf("canceled bottom draw should not draw: %#v", state)
	}

	emptyState := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("empty-bottom-draw", CardDrawFromBottom)}), engineTestPlayer("p2", nil)}, nil, "p1")
	if _, err := engine.PlayCard(emptyState, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"empty-bottom-draw"}}); !errors.Is(err, ErrDrawPileEmpty) {
		t.Fatalf("empty bottom draw error got %v, want ErrDrawPileEmpty", err)
	}
	if emptyState.Phase != PhasePlayerTurn || emptyState.PendingAction != nil || len(emptyState.DiscardPile) != 0 || findCardIndexByID(emptyState.Players[0].Hand, "empty-bottom-draw") < 0 {
		t.Fatalf("empty bottom draw should leave state unchanged: %#v", emptyState)
	}
}

func TestDrawFromBottomExplosiveMatchesNormalDrawResolution(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(49)))
	shieldState := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("bottom-draw", CardDrawFromBottom), engineTestCard("shield-1", CardShield)}),
			engineTestPlayer("p2", nil),
		},
		[]Card{engineTestCard("top-card", CardSkipTurn), engineTestCard("bottom-explosive", CardExplosive)},
		"p1",
	)
	if _, err := engine.PlayCard(shieldState, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"bottom-draw"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(shieldState); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if shieldState.Phase != PhaseWaitingExplosivePlacement || shieldState.PendingAction == nil || shieldState.PendingAction.Cards[0].ID != "bottom-explosive" || findCardIndexByID(shieldState.Players[0].Hand, "shield-1") >= 0 {
		t.Fatalf("shielded bottom explosive should require placement: %#v", shieldState)
	}

	holderState := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("bottom-draw", CardDrawFromBottom), engineTestCard("holder-1", CardExplosiveHolder)}),
			engineTestPlayer("p2", nil),
		},
		[]Card{engineTestCard("bottom-explosive", CardExplosive)},
		"p1",
	)
	if _, err := engine.PlayCard(holderState, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"bottom-draw"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(holderState); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if findCardIndexByID(holderState.Players[0].Hand, "bottom-explosive") < 0 || holderState.Phase != PhasePlayerTurn || holderState.CurrentPlayerID != "p2" {
		t.Fatalf("holder should retain a bottom-drawn explosive and end turn: %#v", holderState)
	}
}

func TestSwapTopBottomSwapsWithoutRevealingDrawPile(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(50)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("swap-1", CardSwapTopBottom)}),
			engineTestPlayer("p2", nil),
		},
		[]Card{engineTestCard("top-secret", CardExplosive), engineTestCard("middle-secret", CardShield), engineTestCard("bottom-secret", CardShuffleDeck)},
		"p1",
	)

	view, err := BuildViewForPlayer(state, "p1")
	if err != nil {
		t.Fatalf("BuildViewForPlayer returned error: %v", err)
	}
	if !containsCommand(view.AvailableActions, CommandPlayCard) {
		t.Fatalf("swap action should make PLAY_CARD available: %#v", view.AvailableActions)
	}
	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"swap-1"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if got := cardIDs(state.DrawPile); !reflect.DeepEqual(got, []string{"bottom-secret", "middle-secret", "top-secret"}) {
		t.Fatalf("unexpected swapped draw pile: %v", got)
	}
	if state.CurrentPlayerID != "p1" || state.Phase != PhasePlayerTurn {
		t.Fatalf("swap should not end the turn: %#v", state)
	}
	for _, event := range FilterEventsForPlayer(events, "p2") {
		if strings.Contains(fmt.Sprintf("%#v", event), "top-secret") || strings.Contains(fmt.Sprintf("%#v", event), "middle-secret") || strings.Contains(fmt.Sprintf("%#v", event), "bottom-secret") {
			t.Fatalf("swap leaked draw-pile identity: %#v", event)
		}
	}
}

func TestSwapTopBottomCancellationAndShortPilesAreNoOps(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(51)))
	canceledState := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("swap-1", CardSwapTopBottom)}),
			engineTestPlayer("p2", []Card{engineTestCard("cancel-1", CardCancel)}),
		},
		[]Card{engineTestCard("top-secret", CardSkipTurn), engineTestCard("bottom-secret", CardShuffleDeck)},
		"p1",
	)
	if _, err := engine.PlayCard(canceledState, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"swap-1"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.PlayCancel(canceledState, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1"}); err != nil {
		t.Fatalf("PlayCancel returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(canceledState); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if got := cardIDs(canceledState.DrawPile); !reflect.DeepEqual(got, []string{"top-secret", "bottom-secret"}) {
		t.Fatalf("canceled swap should not mutate draw pile: %v", got)
	}

	for _, drawPile := range [][]Card{nil, {engineTestCard("only-card", CardSkipTurn)}} {
		state := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("swap-1", CardSwapTopBottom)}), engineTestPlayer("p2", nil)}, drawPile, "p1")
		want := cardIDs(state.DrawPile)
		if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"swap-1"}}); err != nil {
			t.Fatalf("PlayCard returned error: %v", err)
		}
		if _, err := engine.ResolveCancelWindow(state); err != nil {
			t.Fatalf("ResolveCancelWindow returned error: %v", err)
		}
		if got := cardIDs(state.DrawPile); !reflect.DeepEqual(got, want) {
			t.Fatalf("short-pile swap should be a no-op: got %v want %v", got, want)
		}
	}
}

func TestReorderTopCardsPrivatePromptAndValidation(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(52)))
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("reorder-3", CardReorderTop3)}),
			engineTestPlayer("p2", nil),
		},
		[]Card{engineTestCard("top-1", CardExplosive), engineTestCard("top-2", CardShield), engineTestCard("top-3", CardShuffleDeck), engineTestCard("unchanged", CardSkipTurn)},
		"p1",
	)

	if _, err := engine.PlayCard(state, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"reorder-3"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(state)
	if err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if state.Phase != PhaseWaitingDeckReorder || state.PendingAction == nil || !reflect.DeepEqual(cardIDs(state.PendingAction.Cards), []string{"top-1", "top-2", "top-3"}) {
		t.Fatalf("unexpected reorder pending action: %#v", state.PendingAction)
	}
	if len(events) != 2 || events[1].Type != EventPrivatePromptSent || events[1].PlayerID != "p1" || !reflect.DeepEqual(events[1].Cards, state.PendingAction.Cards) {
		t.Fatalf("source should receive private top-card details: %#v", events)
	}
	if filtered := FilterEventsForPlayer(events, "p2"); len(filtered) != 1 {
		t.Fatalf("other players should not receive reorder prompt: %#v", filtered)
	}
	view, err := BuildViewForPlayer(state, "p1")
	if err != nil || !containsCommand(view.AvailableActions, CommandSubmitReorderedTopCards) {
		t.Fatalf("source should be able to submit reorder: view=%#v err=%v", view, err)
	}
	otherView, err := BuildViewForPlayer(state, "p2")
	if err != nil || containsCommand(otherView.AvailableActions, CommandSubmitReorderedTopCards) {
		t.Fatalf("other player should not submit reorder: view=%#v err=%v", otherView, err)
	}
	if _, err := engine.SubmitReorderedTopCards(state, SubmitReorderedTopCardsCommand{PlayerID: "p2", CardIDs: []string{"top-3", "top-2", "top-1"}}); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("other player error got %v, want ErrNotYourTurn", err)
	}
	if _, err := engine.SubmitReorderedTopCards(state, SubmitReorderedTopCardsCommand{PlayerID: "p1", CardIDs: []string{"top-1", "top-1", "top-3"}}); !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("duplicate submission error got %v, want ErrInvalidCardPlay", err)
	}
	if _, err := engine.SubmitReorderedTopCards(state, SubmitReorderedTopCardsCommand{PlayerID: "p1", CardIDs: []string{"top-1", "top-2", "unchanged"}}); !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("unseen submission error got %v, want ErrInvalidCardPlay", err)
	}
	if _, err := engine.SubmitReorderedTopCards(state, SubmitReorderedTopCardsCommand{PlayerID: "p1", CardIDs: []string{"top-3", "top-1", "top-2"}}); err != nil {
		t.Fatalf("SubmitReorderedTopCards returned error: %v", err)
	}
	if got := cardIDs(state.DrawPile); !reflect.DeepEqual(got, []string{"top-3", "top-1", "top-2", "unchanged"}) || state.Phase != PhasePlayerTurn || state.PendingAction != nil {
		t.Fatalf("unexpected reordered state: %#v", state)
	}
}

func TestReorderTopCardsRejectsStaleDeckPrefix(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(54)))
	state := engineTestState(
		[]Player{engineTestPlayer("p1", nil), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("changed-top", CardExplosive), engineTestCard("top-2", CardShield), engineTestCard("unchanged", CardSkipTurn)},
		"p1",
	)
	state.Phase = PhaseWaitingDeckReorder
	state.PendingAction = &PendingAction{
		ID:             "reorder-pending-1",
		SourcePlayerID: "p1",
		Type:           PendingDeckReorder,
		CardIDs:        []string{"reorder-action"},
		Cards:          []Card{engineTestCard("top-1", CardExplosive), engineTestCard("top-2", CardShield)},
	}

	if _, err := engine.SubmitReorderedTopCards(state, SubmitReorderedTopCardsCommand{PlayerID: "p1", CardIDs: []string{"top-2", "top-1"}}); !errors.Is(err, ErrInvalidCardPlay) {
		t.Fatalf("stale reorder error got %v, want ErrInvalidCardPlay", err)
	}
	if got := cardIDs(state.DrawPile); !reflect.DeepEqual(got, []string{"changed-top", "top-2", "unchanged"}) || state.Phase != PhaseWaitingDeckReorder || state.PendingAction == nil {
		t.Fatalf("stale reorder should not mutate state: %#v", state)
	}
}

func TestReorderTopCardsShortPilesCancellationAndEmptyNoOp(t *testing.T) {
	engine := NewEngine(rand.New(rand.NewSource(53)))
	shortState := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("reorder-5", CardReorderTop5)}), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("top-1", CardSkipTurn), engineTestCard("top-2", CardShield)},
		"p1",
	)
	if _, err := engine.PlayCard(shortState, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"reorder-5"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(shortState); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if _, err := engine.SubmitReorderedTopCards(shortState, SubmitReorderedTopCardsCommand{PlayerID: "p1", CardIDs: []string{"top-2", "top-1"}}); err != nil {
		t.Fatalf("SubmitReorderedTopCards returned error: %v", err)
	}
	if got := cardIDs(shortState.DrawPile); !reflect.DeepEqual(got, []string{"top-2", "top-1"}) {
		t.Fatalf("short pile reorder got %v", got)
	}

	emptyState := engineTestState([]Player{engineTestPlayer("p1", []Card{engineTestCard("reorder-3", CardReorderTop3)}), engineTestPlayer("p2", nil)}, nil, "p1")
	if _, err := engine.PlayCard(emptyState, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"reorder-3"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	events, err := engine.ResolveCancelWindow(emptyState)
	if err != nil || len(events) != 1 || emptyState.PendingAction != nil || emptyState.Phase != PhasePlayerTurn {
		t.Fatalf("empty reorder should resolve as a no-op: events=%#v state=%#v err=%v", events, emptyState, err)
	}

	canceledState := engineTestState(
		[]Player{engineTestPlayer("p1", []Card{engineTestCard("reorder-3", CardReorderTop3)}), engineTestPlayer("p2", []Card{engineTestCard("cancel-1", CardCancel)})},
		[]Card{engineTestCard("top-1", CardSkipTurn), engineTestCard("top-2", CardShield)},
		"p1",
	)
	if _, err := engine.PlayCard(canceledState, PlayCardCommand{PlayerID: "p1", CardIDs: []string{"reorder-3"}}); err != nil {
		t.Fatalf("PlayCard returned error: %v", err)
	}
	if _, err := engine.PlayCancel(canceledState, PlayCancelCommand{PlayerID: "p2", CardID: "cancel-1"}); err != nil {
		t.Fatalf("PlayCancel returned error: %v", err)
	}
	if _, err := engine.ResolveCancelWindow(canceledState); err != nil {
		t.Fatalf("ResolveCancelWindow returned error: %v", err)
	}
	if got := cardIDs(canceledState.DrawPile); !reflect.DeepEqual(got, []string{"top-1", "top-2"}) || canceledState.PendingAction != nil {
		t.Fatalf("canceled reorder should not change deck: %#v", canceledState)
	}
}

func containsCommand(commands []CommandType, command CommandType) bool {
	for _, candidate := range commands {
		if candidate == command {
			return true
		}
	}
	return false
}

func engineTestState(players []Player, drawPile []Card, currentPlayerID string) *GameState {
	turnDebt := make(map[string]int, len(players))
	for i := range players {
		players[i].Alive = true
		players[i].Connected = true
		turnDebt[players[i].ID] = 0
	}
	turnDebt[currentPlayerID] = 1

	return &GameState{
		RoomID:          "ROOM1",
		Phase:           PhasePlayerTurn,
		Players:         players,
		DrawPile:        drawPile,
		DiscardPile:     []Card{},
		CurrentPlayerID: currentPlayerID,
		TurnDebt:        turnDebt,
		MarkedCards:     map[string]MarkedCard{},
	}
}

func engineTestPlayer(id string, hand []Card) Player {
	return Player{ID: id, Name: id, Hand: hand, Alive: true, Connected: true}
}

func engineTestCard(id string, code CardCode) Card {
	return Card{ID: id, Code: code}
}

func hasEvent(events []Event, eventType EventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func cardIDs(cards []Card) []string {
	ids := make([]string, len(cards))
	for i, card := range cards {
		ids[i] = card.ID
	}
	return ids
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := map[string]int{}
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}
