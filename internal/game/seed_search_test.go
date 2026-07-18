package game

import (
	"math/rand"
	"os"
	"strconv"
	"testing"
)

const (
	// Change this to the card code required at DrawPile[0].
	seedSearchTargetFirstDraw CardCode = CardExplosive
	seedSearchPlayerCount              = 2
	seedSearchMax                      = int64(100_000)
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
