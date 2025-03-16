package game

import (
	"sync"
)

type Match struct {
	ID      string
	Players []*Player
	Turn    int
	mu      sync.Mutex
}

func NewMatch(id string) *Match {
	return &Match{
		ID:      id,
		Players: []*Player{},
		Turn:    0,
	}
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
