package game

import (
	"reflect"
	"testing"
)

func TestBuildViewShowsOnlyRequestingPlayerHand(t *testing.T) {
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("p1-card", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("p2-card", CardShuffleDeck)}),
		},
		[]Card{engineTestCard("draw-1", CardExplosive), engineTestCard("draw-2", CardShield)},
		"p1",
	)

	view, err := BuildViewForPlayer(state, "p1")
	if err != nil {
		t.Fatalf("BuildViewForPlayer returned error: %v", err)
	}

	if view.You.ID != "p1" || len(view.You.Hand) != 1 || view.You.Hand[0].ID != "p1-card" || view.You.Hand[0].Code != CardSkipTurn {
		t.Fatalf("unexpected private player view: %#v", view.You)
	}
	if len(view.Players) != 2 {
		t.Fatalf("public players length got %d, want 2", len(view.Players))
	}
	for _, player := range view.Players {
		if player.ID == "p2" && player.HandCount != 1 {
			t.Fatalf("p2 hand count got %d, want 1", player.HandCount)
		}
	}
	if view.DrawPileCount != 2 {
		t.Fatalf("draw pile count got %d, want 2", view.DrawPileCount)
	}
}

func TestBuildViewDoesNotExposeOpponentHand(t *testing.T) {
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("p1-card", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("p2-card", CardShuffleDeck)}),
		},
		nil,
		"p1",
	)

	view, err := BuildViewForPlayer(state, "p2")
	if err != nil {
		t.Fatalf("BuildViewForPlayer returned error: %v", err)
	}

	if view.You.ID != "p2" || view.You.Hand[0].ID != "p2-card" {
		t.Fatalf("p2 should see own hand, got %#v", view.You)
	}
	if view.You.Hand[0].ID == "p1-card" {
		t.Fatal("p2 private hand should not contain p1 card")
	}
}

func TestBuildViewShowsPublicDiscardButNotDrawPileOrder(t *testing.T) {
	state := engineTestState(
		[]Player{engineTestPlayer("p1", nil), engineTestPlayer("p2", nil)},
		[]Card{engineTestCard("draw-secret-1", CardExplosive), engineTestCard("draw-secret-2", CardShield)},
		"p1",
	)
	state.DiscardPile = []Card{engineTestCard("discard-1", CardSkipTurn)}

	view, err := BuildViewForPlayer(state, "p1")
	if err != nil {
		t.Fatalf("BuildViewForPlayer returned error: %v", err)
	}

	if view.DrawPileCount != 2 {
		t.Fatalf("draw pile count got %d, want 2", view.DrawPileCount)
	}
	if len(view.DiscardPile) != 1 || view.DiscardPile[0].ID != "discard-1" || view.DiscardPile[0].Code != CardSkipTurn {
		t.Fatalf("unexpected discard pile view: %#v", view.DiscardPile)
	}
}

func TestBuildViewIncludesPublicMarkedCardsOnly(t *testing.T) {
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", nil),
			engineTestPlayer("p2", []Card{engineTestCard("marked-0", CardPeekDeck), engineTestCard("marked-1", CardShuffleDeck), engineTestCard("hidden-1", CardSkipTurn)}),
		},
		nil,
		"p1",
	)
	state.MarkedCards["marked-1"] = MarkedCard{CardID: "marked-1", OwnerID: "p2", Revealed: engineTestCard("marked-1", CardShuffleDeck)}
	state.MarkedCards["marked-0"] = MarkedCard{CardID: "marked-0", OwnerID: "p2", Revealed: engineTestCard("marked-0", CardPeekDeck)}

	view, err := BuildViewForPlayer(state, "p1")
	if err != nil {
		t.Fatalf("BuildViewForPlayer returned error: %v", err)
	}

	if len(view.PublicMarkedCards) != 2 || view.PublicMarkedCards[0].CardID != "marked-0" || view.PublicMarkedCards[0].Code != CardPeekDeck || view.PublicMarkedCards[1].CardID != "marked-1" || view.PublicMarkedCards[1].Code != CardShuffleDeck {
		t.Fatalf("unexpected marked card views: %#v", view.PublicMarkedCards)
	}
	if reflect.DeepEqual(view.PublicMarkedCards, []PublicMarkedCardView{{CardID: "hidden-1", OwnerID: "p2", Code: CardSkipTurn}}) {
		t.Fatal("unmarked cards should not be exposed through marked card view")
	}
}

func TestBuildViewHidesBlindedPlayerOwnHand(t *testing.T) {
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", nil),
			engineTestPlayer("p2", []Card{engineTestCard("blind-card", CardShuffleDeck)}),
		},
		nil,
		"p1",
	)
	state.Players[1].Blinded = true

	view, err := BuildViewForPlayer(state, "p2")
	if err != nil {
		t.Fatalf("BuildViewForPlayer returned error: %v", err)
	}

	if len(view.You.Hand) != 1 || !view.You.Hand[0].IsHidden || view.You.Hand[0].Code != "" {
		t.Fatalf("blinded hand should hide card identity, got %#v", view.You.Hand)
	}
}

func TestBuildViewShowsPendingActionWithoutPrivateCards(t *testing.T) {
	state := engineTestState([]Player{engineTestPlayer("p1", nil), engineTestPlayer("p2", nil)}, nil, "p1")
	state.Phase = PhaseCancelWindow
	state.PendingAction = &PendingAction{
		ID:             "pending-1",
		SourcePlayerID: "p1",
		Type:           PendingPlayCard,
		CardIDs:        []string{"secret-action"},
		Cards:          []Card{engineTestCard("secret-action", CardTargetExtraTurns)},
		TargetPlayerID: "p2",
		CancelCount:    1,
	}

	view, err := BuildViewForPlayer(state, "p2")
	if err != nil {
		t.Fatalf("BuildViewForPlayer returned error: %v", err)
	}

	if view.PendingAction == nil || view.PendingAction.ID != "pending-1" || view.PendingAction.TargetPlayerID != "p2" || view.PendingAction.CancelCount != 1 {
		t.Fatalf("unexpected pending action view: %#v", view.PendingAction)
	}
}

func TestBuildViewAvailableActionsArePlayerSpecific(t *testing.T) {
	state := engineTestState(
		[]Player{
			engineTestPlayer("p1", []Card{engineTestCard("skip-1", CardSkipTurn)}),
			engineTestPlayer("p2", []Card{engineTestCard("cancel-1", CardCancel)}),
		},
		nil,
		"p1",
	)

	p1View, err := BuildViewForPlayer(state, "p1")
	if err != nil {
		t.Fatalf("BuildViewForPlayer p1 returned error: %v", err)
	}
	p2View, err := BuildViewForPlayer(state, "p2")
	if err != nil {
		t.Fatalf("BuildViewForPlayer p2 returned error: %v", err)
	}

	if !commandListContains(p1View.AvailableActions, CommandDrawCard) || !commandListContains(p1View.AvailableActions, CommandPlayCard) {
		t.Fatalf("p1 available actions got %#v, want draw and play", p1View.AvailableActions)
	}
	if len(p2View.AvailableActions) != 0 {
		t.Fatalf("p2 available actions got %#v, want none", p2View.AvailableActions)
	}

	state.Phase = PhaseCancelWindow
	state.PendingAction = &PendingAction{ID: "pending-1", SourcePlayerID: "p1", Type: PendingPlayCard, Cards: []Card{engineTestCard("skip-1", CardSkipTurn)}}
	p2View, err = BuildViewForPlayer(state, "p2")
	if err != nil {
		t.Fatalf("BuildViewForPlayer p2 cancel returned error: %v", err)
	}
	if !commandListContains(p2View.AvailableActions, CommandPlayCancel) {
		t.Fatalf("p2 available actions got %#v, want cancel", p2View.AvailableActions)
	}
}

func TestFilterEventsForPlayerHidesPrivatePromptsForOthers(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventCardPlayed, PlayerID: "p1", CardIDs: []string{"peek-1"}},
		{Seq: 2, Type: EventPrivatePromptSent, PlayerID: "p1", CardIDs: []string{"draw-secret-1", "draw-secret-2"}},
	}

	p1Events := FilterEventsForPlayer(events, "p1")
	if len(p1Events) != 2 {
		t.Fatalf("p1 events got %#v, want both events", p1Events)
	}
	p2Events := FilterEventsForPlayer(events, "p2")
	if len(p2Events) != 1 || p2Events[0].Type != EventCardPlayed {
		t.Fatalf("p2 events got %#v, want only public event", p2Events)
	}
}

func TestBuildViewReturnsPlayerNotFound(t *testing.T) {
	state := engineTestState([]Player{engineTestPlayer("p1", nil), engineTestPlayer("p2", nil)}, nil, "p1")
	_, err := BuildViewForPlayer(state, "missing")
	if err != ErrPlayerNotFound {
		t.Fatalf("BuildViewForPlayer error got %v, want ErrPlayerNotFound", err)
	}
}

func commandListContains(commands []CommandType, target CommandType) bool {
	for _, command := range commands {
		if command == target {
			return true
		}
	}
	return false
}
