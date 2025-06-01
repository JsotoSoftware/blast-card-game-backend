package game

import (
	"sync"
	"time"
)

// Room represents a game room that can host multiple matches
type Room struct {
	ID         string
	Name       string
	CreatedAt  time.Time
	CreatedBy  string
	MaxPlayers int
	IsPrivate  bool
	Password   string // Only used if IsPrivate is true
	Players    map[string]*Player
	Match      *Match
	Mu         sync.RWMutex
}

// NewRoom creates a new room
func NewRoom(id string, name string, maxPlayers int, createdBy string, isPrivate bool, password string) *Room {
	return &Room{
		ID:         id,
		Name:       name,
		CreatedAt:  time.Now(),
		CreatedBy:  createdBy,
		MaxPlayers: maxPlayers,
		IsPrivate:  isPrivate,
		Password:   password,
		Players:    make(map[string]*Player),
	}
}

// AddPlayer adds a player to the room
func (r *Room) AddPlayer(player *Player) bool {
	r.Mu.Lock()
	defer r.Mu.Unlock()

	if len(r.Players) >= r.MaxPlayers {
		return false
	}

	r.Players[player.ID] = player
	return true
}

// RemovePlayer removes a player from the room
func (r *Room) RemovePlayer(playerID string) {
	r.Mu.Lock()
	defer r.Mu.Unlock()

	delete(r.Players, playerID)
}

// GetPlayerCount returns the number of players in the room
func (r *Room) GetPlayerCount() int {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	return len(r.Players)
}

// IsFull returns true if the room has reached its maximum capacity
func (r *Room) IsFull() bool {
	return r.GetPlayerCount() >= r.MaxPlayers
}

// StartMatch starts a new match in the room if there are enough players
func (r *Room) StartMatch() bool {
	r.Mu.Lock()
	defer r.Mu.Unlock()

	if len(r.Players) != r.MaxPlayers {
		return false
	}

	// Create a new match with the room ID
	r.Match = NewMatch(r.ID)

	// Add all players to the match
	for _, player := range r.Players {
		r.Match.AddPlayer(player)
	}

	return true
}

// GetPublicInfo returns public information about the room
func (r *Room) GetPublicInfo() map[string]interface{} {
	r.Mu.RLock()
	defer r.Mu.RUnlock()

	return map[string]interface{}{
		"id":           r.ID,
		"name":         r.Name,
		"created_at":   r.CreatedAt,
		"created_by":   r.CreatedBy,
		"max_players":  r.MaxPlayers,
		"is_private":   r.IsPrivate,
		"player_count": len(r.Players),
		"has_match":    r.Match != nil,
	}
}
