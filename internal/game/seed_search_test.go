package game

import (
	"math/rand"
	"os"
	"strconv"
	"testing"
)

const (
	// Change this to the card code required at DrawPile[0].
	seedSearchTargetFirstDraw          CardCode = CardExplosive
	seedSearchFirstHandCard                     = CardCancel
	seedSearchSecondHandCard                    = CardRequestCard
	seedSearchCancelOverCancelAction            = CardRequestCard
	seedSearchCancelOverCancelResponse          = CardCancel
	seedSearchComboCard                         = CardSkipTurn
	seedSearchComboCardCount                    = 2
	seedSearchPlayerCount                       = 2
	seedSearchMax                               = int64(100_000)
)

// TestFindSeedForFirstDraw is a manual test utility. Run it with:
// RUN_SEED_SEARCH=1 go test ./internal/game -run TestFindSeedForFirstDraw -v
func TestFindSeedForFirstDraw(t *testing.T) {
	if os.Getenv("RUN_SEED_SEARCH") != "1" {
		t.Skip("set RUN_SEED_SEARCH=1 to run the seed search")
	}

	players := seedSearchPlayers(seedSearchPlayerCount)
	for seed := int64(0); seed < seedSearchMax; seed++ {
		engine := NewEngine(rand.New(rand.NewSource(seed)))
		state, _, err := engine.StartGame("SEED_SEARCH", players)
		if err != nil {
			t.Fatalf("StartGame returned error: %v", err)
		}
		if len(state.DrawPile) > 0 && state.DrawPile[0].Code == seedSearchTargetFirstDraw {
			t.Logf("GAME_TEST_SEED=%d gives %s at DrawPile[0] for %d players", seed, seedSearchTargetFirstDraw, seedSearchPlayerCount)
			return
		}
	}

	t.Fatalf("no seed below %d gives %s at DrawPile[0] for %d players", seedSearchMax, seedSearchTargetFirstDraw, seedSearchPlayerCount)
}

// TestFindSeedForSplitOpeningHands finds a seed where different players receive
// the two configured cards in their initial hands. Run it with:
// RUN_SEED_SEARCH=1 go test ./internal/game -run TestFindSeedForSplitOpeningHands -v
func TestFindSeedForSplitOpeningHands(t *testing.T) {
	if os.Getenv("RUN_SEED_SEARCH") != "1" {
		t.Skip("set RUN_SEED_SEARCH=1 to run the seed search")
	}

	players := seedSearchPlayers(seedSearchPlayerCount)
	for seed := int64(0); seed < seedSearchMax; seed++ {
		engine := NewEngine(rand.New(rand.NewSource(seed)))
		state, _, err := engine.StartGame("SEED_SEARCH", players)
		if err != nil {
			t.Fatalf("StartGame returned error: %v", err)
		}
		for firstIndex, firstPlayer := range state.Players {
			if !seedSearchHandContains(firstPlayer.Hand, seedSearchFirstHandCard) {
				continue
			}
			for secondIndex, secondPlayer := range state.Players {
				if firstIndex == secondIndex || !seedSearchHandContains(secondPlayer.Hand, seedSearchSecondHandCard) {
					continue
				}
				t.Logf("GAME_TEST_SEED=%d gives %s to player %d and %s to player %d", seed, seedSearchFirstHandCard, firstIndex+1, seedSearchSecondHandCard, secondIndex+1)
				return
			}
		}
	}

	t.Fatalf("no seed below %d gives %s and %s to different players", seedSearchMax, seedSearchFirstHandCard, seedSearchSecondHandCard)
}

// TestFindSeedForCancelOverCancelOpeningHands finds a seed where one player
// can play the configured action and then cancel the other player's cancel.
// Run it with:
// RUN_SEED_SEARCH=1 go test ./internal/game -run TestFindSeedForCancelOverCancelOpeningHands -v
func TestFindSeedForCancelOverCancelOpeningHands(t *testing.T) {
	if os.Getenv("RUN_SEED_SEARCH") != "1" {
		t.Skip("set RUN_SEED_SEARCH=1 to run the seed search")
	}

	players := seedSearchPlayers(seedSearchPlayerCount)
	for seed := int64(0); seed < seedSearchMax; seed++ {
		engine := NewEngine(rand.New(rand.NewSource(seed)))
		state, _, err := engine.StartGame("SEED_SEARCH", players)
		if err != nil {
			t.Fatalf("StartGame returned error: %v", err)
		}
		for sourceIndex, source := range state.Players {
			if !seedSearchHandContains(source.Hand, seedSearchCancelOverCancelAction) || !seedSearchHandContains(source.Hand, seedSearchCancelOverCancelResponse) {
				continue
			}
			for responderIndex, responder := range state.Players {
				if sourceIndex == responderIndex || !seedSearchHandContains(responder.Hand, seedSearchCancelOverCancelResponse) {
					continue
				}
				t.Logf("GAME_TEST_SEED=%d gives %s and %s to player %d, plus %s to player %d", seed, seedSearchCancelOverCancelAction, seedSearchCancelOverCancelResponse, sourceIndex+1, seedSearchCancelOverCancelResponse, responderIndex+1)
				return
			}
		}
	}

	t.Fatalf("no seed below %d gives %s and %s to one player plus %s to another", seedSearchMax, seedSearchCancelOverCancelAction, seedSearchCancelOverCancelResponse, seedSearchCancelOverCancelResponse)
}

// TestFindSeedForOpeningCombo finds a seed where player 1 has the configured
// matching cards and player 2 can be targeted by a pair combo. Run it with:
// RUN_SEED_SEARCH=1 go test ./internal/game -run TestFindSeedForOpeningCombo -v
func TestFindSeedForOpeningCombo(t *testing.T) {
	if os.Getenv("RUN_SEED_SEARCH") != "1" {
		t.Skip("set RUN_SEED_SEARCH=1 to run the seed search")
	}
	if seedSearchPlayerCount < 2 {
		t.Fatal("seedSearchPlayerCount must be at least 2 for a pair target")
	}
	if seedSearchComboCardCount < 2 {
		t.Fatal("seedSearchComboCardCount must be at least 2 for a pair combo")
	}

	players := seedSearchPlayers(seedSearchPlayerCount)
	for seed := int64(0); seed < seedSearchMax; seed++ {
		engine := NewEngine(rand.New(rand.NewSource(seed)))
		state, _, err := engine.StartGame("SEED_SEARCH", players)
		if err != nil {
			t.Fatalf("StartGame returned error: %v", err)
		}
		if countCardsByCode(state.Players[0].Hand, seedSearchComboCard) >= seedSearchComboCardCount && len(state.Players[1].Hand) > 0 {
			t.Logf("GAME_TEST_SEED=%d gives player 1 at least %d %s cards; player 2 has %d targetable cards", seed, seedSearchComboCardCount, seedSearchComboCard, len(state.Players[1].Hand))
			return
		}
	}

	t.Fatalf("no seed below %d gives player 1 at least %d %s cards with a targetable player 2", seedSearchMax, seedSearchComboCardCount, seedSearchComboCard)
}

func seedSearchHandContains(cards []Card, target CardCode) bool {
	for _, card := range cards {
		if card.Code == target {
			return true
		}
	}
	return false
}

func seedSearchPlayers(count int) []Player {
	players := make([]Player, count)
	for i := range players {
		players[i] = Player{
			ID:        "seed-search-player-" + strconv.Itoa(i+1),
			Name:      "Seed Search Player",
			Alive:     true,
			Connected: true,
		}
	}
	return players
}
