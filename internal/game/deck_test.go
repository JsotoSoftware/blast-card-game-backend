package game

import (
	"errors"
	"math/rand"
	"testing"
)

func TestDeckCountsForPlayerCounts(t *testing.T) {
	tests := []struct {
		players       int
		wantTotal     int
		wantExplosive int
		wantShield    int
		wantPeek5     int
		wantReorder5  int
	}{
		{players: 2, wantTotal: 44, wantExplosive: 1, wantShield: 5, wantPeek5: 0, wantReorder5: 0},
		{players: 3, wantTotal: 46, wantExplosive: 2, wantShield: 6, wantPeek5: 0, wantReorder5: 0},
		{players: 4, wantTotal: 67, wantExplosive: 3, wantShield: 8, wantPeek5: 1, wantReorder5: 1},
		{players: 7, wantTotal: 70, wantExplosive: 6, wantShield: 8, wantPeek5: 1, wantReorder5: 1},
		{players: 8, wantTotal: 104, wantExplosive: 7, wantShield: 10, wantPeek5: 1, wantReorder5: 1},
		{players: 10, wantTotal: 106, wantExplosive: 9, wantShield: 10, wantPeek5: 1, wantReorder5: 1},
	}

	for _, test := range tests {
		t.Run("player count", func(t *testing.T) {
			counts, err := DeckCardCounts(test.players)
			if err != nil {
				t.Fatalf("DeckCardCounts returned error: %v", err)
			}

			if got := counts[CardExplosive]; got != test.wantExplosive {
				t.Fatalf("explosive count got %d, want %d", got, test.wantExplosive)
			}

			if got := counts[CardShield]; got != test.wantShield {
				t.Fatalf("shield count got %d, want %d", got, test.wantShield)
			}

			if got := counts[CardPeekDeck5]; got != test.wantPeek5 {
				t.Fatalf("peek-5 count got %d, want %d", got, test.wantPeek5)
			}

			if got := counts[CardReorderTop5]; got != test.wantReorder5 {
				t.Fatalf("reorder-5 count got %d, want %d", got, test.wantReorder5)
			}

			got, err := ExpectedDeckSize(test.players)
			if err != nil {
				t.Fatalf("ExpectedDeckSize returned error: %v", err)
			}
			if got != test.wantTotal {
				t.Fatalf("deck size got %d, want %d", got, test.wantTotal)
			}
		})
	}
}

func TestNewDeckEveryCardHasUniqueID(t *testing.T) {
	factory := NewDeckFactory(rand.New(rand.NewSource(1)))
	deck, err := factory.NewDeck(10)
	if err != nil {
		t.Fatalf("NewDeck returned error: %v", err)
	}

	seen := map[string]bool{}
	for _, card := range deck {
		if card.ID == "" {
			t.Fatal("card ID is empty")
		}
		if seen[card.ID] {
			t.Fatalf("duplicate card ID: %s", card.ID)
		}
		seen[card.ID] = true
	}
}

func TestShufflePreservesCards(t *testing.T) {
	factory := NewDeckFactory(rand.New(rand.NewSource(2)))
	deck, err := factory.NewDeck(3)
	if err != nil {
		t.Fatalf("NewDeck returned error: %v", err)
	}

	before := countCardsByID(deck)
	factory.Shuffle(deck)
	after := countCardsByID(deck)

	if len(after) != len(before) {
		t.Fatalf("card count got %d, want %d", len(after), len(before))
	}

	for id, beforeCount := range before {
		if after[id] != beforeCount {
			t.Fatalf("card %s count after shuffle got %d, want %d", id, after[id], beforeCount)
		}
	}
}

func TestSetupInitialHandsAndRequiredShield(t *testing.T) {
	players := testPlayers(3)
	factory := NewDeckFactory(rand.New(rand.NewSource(3)))

	state, err := factory.NewSetup("ROOM1", players)
	if err != nil {
		t.Fatalf("NewSetup returned error: %v", err)
	}

	if state.Phase != PhasePlayerTurn {
		t.Fatalf("phase got %s, want %s", state.Phase, PhasePlayerTurn)
	}
	if state.CurrentPlayerID != players[0].ID {
		t.Fatalf("current player got %s, want %s", state.CurrentPlayerID, players[0].ID)
	}

	for _, player := range state.Players {
		if !player.Alive {
			t.Fatalf("player %s should be alive", player.ID)
		}
		if len(player.Hand) != InitialHandSize {
			t.Fatalf("player %s hand size got %d, want %d", player.ID, len(player.Hand), InitialHandSize)
		}
		if countCode(player.Hand, CardShield) != RequiredShieldCards {
			t.Fatalf("player %s shield count got %d, want %d", player.ID, countCode(player.Hand, CardShield), RequiredShieldCards)
		}
		if countCode(player.Hand, CardExplosive) != 0 {
			t.Fatalf("player %s was dealt explosive", player.ID)
		}
	}
}

func TestSetupDrawPileContainsExpectedExplosives(t *testing.T) {
	players := testPlayers(10)
	factory := NewDeckFactory(rand.New(rand.NewSource(4)))

	state, err := factory.NewSetup("ROOM1", players)
	if err != nil {
		t.Fatalf("NewSetup returned error: %v", err)
	}

	counts, err := DeckCardCounts(len(players))
	if err != nil {
		t.Fatalf("DeckCardCounts returned error: %v", err)
	}

	if got := countCode(state.DrawPile, CardExplosive); got != counts[CardExplosive] {
		t.Fatalf("draw pile explosive count got %d, want %d", got, counts[CardExplosive])
	}

	expectedDeckSize, err := ExpectedDeckSize(len(players))
	if err != nil {
		t.Fatalf("ExpectedDeckSize returned error: %v", err)
	}
	expectedDrawPileSize := expectedDeckSize - len(players)*InitialHandSize
	if got := len(state.DrawPile); got != expectedDrawPileSize {
		t.Fatalf("draw pile size got %d, want %d", got, expectedDrawPileSize)
	}
}

func TestSetupRejectsInvalidPlayerCounts(t *testing.T) {
	factory := NewDeckFactory(rand.New(rand.NewSource(5)))

	for _, players := range [][]Player{testPlayers(1), testPlayers(11)} {
		_, err := factory.NewSetup("ROOM1", players)
		if !errors.Is(err, ErrInvalidPlayerCount) {
			t.Fatalf("NewSetup error got %v, want ErrInvalidPlayerCount", err)
		}
	}
}

func countCardsByID(cards []Card) map[string]int {
	counts := make(map[string]int, len(cards))
	for _, card := range cards {
		counts[card.ID]++
	}
	return counts
}

func countCode(cards []Card, code CardCode) int {
	count := 0
	for _, card := range cards {
		if card.Code == code {
			count++
		}
	}
	return count
}

func testPlayers(count int) []Player {
	players := make([]Player, count)
	for i := range count {
		players[i] = Player{
			ID:        "player-" + string(rune('A'+i)),
			Name:      "Player " + string(rune('A'+i)),
			Connected: true,
			Ready:     true,
		}
	}
	return players
}
