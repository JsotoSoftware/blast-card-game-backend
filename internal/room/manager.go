package room

import (
	"crypto/rand"
	"errors"
	"math/big"
	mathrand "math/rand"
	"sync"
	"time"

	"exploding-game/server/internal/game"
)

const (
	roomCodeLength = 6
	playerIDLength = 12
	tokenLength    = 24
)

var (
	ErrRoomNotFound          = errors.New("room not found")
	ErrRoomFull              = errors.New("room full")
	ErrPlayerAlreadyInRoom   = errors.New("player already in room")
	ErrPlayerNotInRoom       = errors.New("player not in room")
	ErrRoomCodeCollision     = errors.New("room code collision")
	ErrInvalidPlayerToken    = errors.New("invalid player token")
	ErrNotHost               = errors.New("not host")
	ErrPlayersNotReady       = errors.New("players not ready")
	ErrInvalidHostTransfer   = errors.New("invalid host transfer")
	ErrKickVoteAlreadyActive = errors.New("kick vote already active")
	ErrNoKickVoteActive      = errors.New("no kick vote active")
	ErrCannotVoteKickSelf    = errors.New("cannot vote kick self")
	ErrInvalidKickVoteTarget = errors.New("invalid kick vote target")
	ErrGameNotStarted        = errors.New("game not started")
)

type Manager struct {
	mu        sync.RWMutex
	rooms     map[string]*Room
	fixedSeed *int64
}

func NewManager() *Manager {
	return &Manager{rooms: map[string]*Room{}}
}

// NewManagerWithSeed creates deterministic game engines for test environments.
// Production code should use NewManager.
func NewManagerWithSeed(seed int64) *Manager {
	return &Manager{rooms: map[string]*Room{}, fixedSeed: &seed}
}

func (m *Manager) newEngine() *game.Engine {
	if m.fixedSeed == nil {
		return game.NewEngine(nil)
	}
	return game.NewEngine(mathrand.New(mathrand.NewSource(*m.fixedSeed)))
}

func (m *Manager) CreateRoom(hostName string) (*Room, *Player, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	roomID, err := m.uniqueRoomCodeLocked()
	if err != nil {
		return nil, nil, err
	}

	host, err := newPlayer(hostName)
	if err != nil {
		return nil, nil, err
	}

	room := newRoom(roomID, host, m.newEngine())
	m.rooms[roomID] = room
	return room, host, nil
}

func (m *Manager) GetRoom(roomID string) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	room, exists := m.rooms[roomID]
	return room, exists
}

func (m *Manager) RoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}

func (m *Manager) JoinRoom(roomID string, playerName string) (*Player, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}

	player, err := newPlayer(playerName)
	if err != nil {
		return nil, err
	}
	if err := room.Join(player); err != nil {
		return nil, err
	}
	return player, nil
}

func (m *Manager) LeaveRoom(roomID string, playerID string) error {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return ErrRoomNotFound
	}
	if err := room.Leave(playerID); err != nil {
		return err
	}

	if room.PlayerCount() == 0 {
		m.mu.Lock()
		defer m.mu.Unlock()
		if current, exists := m.rooms[roomID]; exists && current == room && room.PlayerCount() == 0 {
			delete(m.rooms, roomID)
		}
	}
	return nil
}

func (m *Manager) SetReady(roomID string, playerToken string, ready bool) error {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return ErrRoomNotFound
	}
	return room.SetReady(playerToken, ready)
}

func (m *Manager) TransferHost(roomID string, playerToken string, targetPlayerID string) error {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return ErrRoomNotFound
	}
	return room.TransferHost(playerToken, targetPlayerID)
}

func (m *Manager) StartGame(roomID string, playerToken string) ([]game.Event, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room.StartGame(playerToken)
}

func (m *Manager) StartGameWithoutAuth(roomID string) ([]game.Event, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room.StartGameWithoutAuth()
}

func (m *Manager) PlayerIDForToken(roomID string, playerToken string) (string, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return "", ErrRoomNotFound
	}
	return room.PlayerIDForToken(playerToken)
}

func (m *Manager) StartKickVote(roomID string, playerToken string, targetPlayerID string) (bool, string, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return false, "", ErrRoomNotFound
	}
	return room.StartKickVote(playerToken, targetPlayerID)
}

func (m *Manager) CastKickVote(roomID string, playerToken string, approve bool) (bool, string, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return false, "", ErrRoomNotFound
	}
	return room.CastKickVote(playerToken, approve)
}

func (m *Manager) DrawCard(roomID string, playerToken string) ([]game.Event, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room.DrawCard(playerToken)
}

func (m *Manager) PlayCard(roomID string, playerToken string, cardIDs []string, targetPlayerID string) ([]game.Event, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room.PlayCard(playerToken, cardIDs, targetPlayerID)
}

func (m *Manager) PlaceExplosive(roomID string, playerToken string, index int) ([]game.Event, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room.PlaceExplosive(playerToken, index)
}

func (m *Manager) ExpireCancelWindow(roomID string, now time.Time) ([]game.Event, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room.ExpireCancelWindow(now)
}

func (m *Manager) RoomView(roomID string) (RoomView, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return RoomView{}, ErrRoomNotFound
	}
	return room.View(), nil
}

func (m *Manager) GameViews(roomID string) (map[string]game.PlayerGameView, error) {
	room, exists := m.GetRoom(roomID)
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room.GameViews()
}

func (m *Manager) uniqueRoomCodeLocked() (string, error) {
	for range 100 {
		code, err := randomCode(roomCodeLength)
		if err != nil {
			return "", err
		}
		if _, exists := m.rooms[code]; !exists {
			return code, nil
		}
	}
	return "", ErrRoomCodeCollision
}

func newPlayer(name string) (*Player, error) {
	id, err := randomCode(playerIDLength)
	if err != nil {
		return nil, err
	}
	token, err := randomCode(tokenLength)
	if err != nil {
		return nil, err
	}
	return &Player{ID: id, Name: name, Token: token, Connected: true}, nil
}

func randomCode(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]byte, length)
	max := big.NewInt(int64(len(alphabet)))

	for i := range result {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		result[i] = alphabet[n.Int64()]
	}
	return string(result), nil
}
