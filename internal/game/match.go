package game

import (
	"encoding/json"
	"sync"
)

type Match struct {
	ID          string    `json:"id"`
	Players     []*Player `json:"players"`
	Turn        int       `json:"turn"`
	Deck        *Deck     `json:"deck"`
	DiscardPile []Card    `json:"discardPile"`
	mu          sync.Mutex
}

// MatchJSON is a JSON-safe version of Match
type MatchJSON struct {
	ID          string       `json:"id"`
	Players     []PlayerJSON `json:"players"`
	Turn        int          `json:"turn"`
	Deck        *Deck        `json:"deck"`
	DiscardPile []Card       `json:"discardPile"`
}

// PlayerJSON is a JSON-safe version of Player
type PlayerJSON struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func NewMatch(id string) *Match {
	match := &Match{
		ID:          id,
		Players:     []*Player{},
		Turn:        0,
		DiscardPile: make([]Card, 0),
	}
	match.InitializeDeck()
	return match
}

func (m *Match) InitializeDeck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Deck = NewDeck()
	m.Deck.Shuffle()
}

func (m *Match) AddPlayer(player *Player) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Players = append(m.Players, player)
}

func (m *Match) GetCurrentPlayer() *Player {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Players[m.Turn]
}

func (m *Match) NextTurn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Turn++
}

func (m *Match) ToJSON() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	jsonMatch := MatchJSON{
		ID:          m.ID,
		Turn:        m.Turn,
		Deck:        m.Deck,
		DiscardPile: m.DiscardPile,
		Players:     make([]PlayerJSON, len(m.Players)),
	}

	for i, p := range m.Players {
		jsonMatch.Players[i] = PlayerJSON{
			ID:       p.ID,
			Username: p.Username,
		}
	}

	return json.Marshal(jsonMatch)
}

func (m *Match) DiscardCard(card Card) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DiscardPile = append(m.DiscardPile, card)
}
