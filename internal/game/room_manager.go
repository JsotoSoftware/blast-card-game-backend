package game

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// RoomManager handles all game rooms
type RoomManager struct {
	rooms map[string]*Room
	mu    sync.RWMutex
}

// NewRoomManager creates a new room manager
func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]*Room),
	}
}

// CreateRoom creates a new room
func (rm *RoomManager) CreateRoom(name string, maxPlayers int, createdBy string, isPrivate bool, password string) *Room {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if a room with this name already exists
	for _, room := range rm.rooms {
		if room.Name == name {
			return room
		}
	}

	roomID := uuid.New().String()
	room := NewRoom(roomID, name, maxPlayers, createdBy, isPrivate, password)
	rm.rooms[roomID] = room
	return room
}

// GetRoom returns a room by ID
func (rm *RoomManager) GetRoom(roomID string) *Room {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.rooms[roomID]
}

// GetRoomByName returns a room by its name
func (rm *RoomManager) GetRoomByName(name string) *Room {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for _, room := range rm.rooms {
		if room.Name == name {
			return room
		}
	}
	return nil
}

// ListRooms returns a list of all public rooms
func (rm *RoomManager) ListRooms() []map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rooms := make([]map[string]interface{}, 0, len(rm.rooms))
	for _, room := range rm.rooms {
		if !room.IsPrivate {
			rooms = append(rooms, room.GetPublicInfo())
		}
	}
	return rooms
}

// RemoveRoom removes a room if it's empty
func (rm *RoomManager) RemoveRoom(roomID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if room, exists := rm.rooms[roomID]; exists {
		if room.GetPlayerCount() == 0 && room.Name != "lobby" { // Don't remove the lobby
			delete(rm.rooms, roomID)
		}
	}
}

// StartCleanupRoutine starts a routine to clean up empty rooms
func (rm *RoomManager) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			rm.mu.Lock()
			for roomID, room := range rm.rooms {
				if room.GetPlayerCount() == 0 && room.Name != "lobby" { // Don't remove the lobby
					delete(rm.rooms, roomID)
				}
			}
			rm.mu.Unlock()
		}
	}()
}
