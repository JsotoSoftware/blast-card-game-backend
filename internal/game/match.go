package game

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Match struct {
	ID          string    `json:"id"`
	Players     []*Player `json:"players"`
	Turn        int       `json:"turn"`
	Deck        *Deck     `json:"deck"`
	DiscardPile []Card    `json:"discardPile"`
	mu          sync.Mutex
	// Reconnection timeout in seconds
	ReconnectTimeout int `json:"-"`
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
		ID:               id,
		Players:          []*Player{},
		Turn:             0,
		DiscardPile:      make([]Card, 0),
		ReconnectTimeout: 15, // Default 60 seconds timeout
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

// SetReconnectTimeout sets the reconnection timeout in seconds
func (m *Match) SetReconnectTimeout(seconds int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReconnectTimeout = seconds
}

func (m *Match) RemovePlayer(playerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.Players {
		if p.ID == playerID {
			// Instead of removing the player, mark them as disconnected
			now := time.Now()
			p.DisconnectedAt = &now
			p.IsDisconnected = true
			p.Conn = nil
			p.Send = nil
			return
		}
	}
}

// CleanupDisconnectedPlayers removes players that have exceeded the reconnection timeout
func (m *Match) CleanupDisconnectedPlayers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	activePlayers := make([]*Player, 0, len(m.Players))

	for _, p := range m.Players {
		if p.IsDisconnected {
			if p.DisconnectedAt != nil && now.Sub(*p.DisconnectedAt).Seconds() > float64(m.ReconnectTimeout) {
				// Player has exceeded timeout, skip them
				log.Printf("Player %s timed out after %d seconds", p.ID, m.ReconnectTimeout)
				continue
			}
		}
		activePlayers = append(activePlayers, p)
	}

	// Update players list
	m.Players = activePlayers

	// Adjust turn if needed
	if m.Turn >= len(m.Players) {
		m.Turn = 0
	}
}

// ReconnectPlayer attempts to reconnect a player
func (m *Match) ReconnectPlayer(playerID string, conn *websocket.Conn) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.Players {
		if p.ID == playerID && p.IsDisconnected {
			if p.DisconnectedAt != nil && time.Since(*p.DisconnectedAt).Seconds() <= float64(m.ReconnectTimeout) {
				// Player is within timeout window, allow reconnection
				p.Conn = conn
				p.Send = make(chan []byte)
				p.IsDisconnected = false
				p.DisconnectedAt = nil
				return true
			} else {
				// Player has exceeded timeout, remove them
				m.CleanupDisconnectedPlayers()
				return false
			}
		}
	}
	return false
}

// GetActivePlayers returns the number of active (non-disconnected) players
func (m *Match) GetActivePlayers() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, p := range m.Players {
		if !p.IsDisconnected {
			count++
		}
	}
	return count
}
