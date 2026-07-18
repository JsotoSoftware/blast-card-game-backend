package room

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"exploding-game/server/internal/game"
)

func TestCreateRoomsHaveDifferentIDs(t *testing.T) {
	manager := NewManager()

	roomA, _, err := manager.CreateRoom("host-a")
	if err != nil {
		t.Fatalf("CreateRoom A returned error: %v", err)
	}
	roomB, _, err := manager.CreateRoom("host-b")
	if err != nil {
		t.Fatalf("CreateRoom B returned error: %v", err)
	}

	if roomA.ID() == roomB.ID() {
		t.Fatalf("room IDs should differ, both got %s", roomA.ID())
	}
	if manager.RoomCount() != 2 {
		t.Fatalf("room count got %d, want 2", manager.RoomCount())
	}
}

func TestJoiningOneRoomDoesNotAffectAnother(t *testing.T) {
	manager := NewManager()
	roomA, _, err := manager.CreateRoom("host-a")
	if err != nil {
		t.Fatalf("CreateRoom A returned error: %v", err)
	}
	roomB, _, err := manager.CreateRoom("host-b")
	if err != nil {
		t.Fatalf("CreateRoom B returned error: %v", err)
	}

	_, err = manager.JoinRoom(roomA.ID(), "player-a2")
	if err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}

	if roomA.PlayerCount() != 2 {
		t.Fatalf("room A player count got %d, want 2", roomA.PlayerCount())
	}
	if roomB.PlayerCount() != 1 {
		t.Fatalf("room B player count got %d, want 1", roomB.PlayerCount())
	}
}

func TestRoomRejectsEleventhPlayer(t *testing.T) {
	manager := NewManager()
	room, _, err := manager.CreateRoom("host")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}

	for i := 0; i < MaxPlayers-1; i++ {
		_, err := manager.JoinRoom(room.ID(), "player")
		if err != nil {
			t.Fatalf("JoinRoom player %d returned error: %v", i+2, err)
		}
	}

	_, err = manager.JoinRoom(room.ID(), "overflow")
	if !errors.Is(err, ErrRoomFull) {
		t.Fatalf("overflow JoinRoom error got %v, want ErrRoomFull", err)
	}
	if room.PlayerCount() != MaxPlayers {
		t.Fatalf("player count got %d, want %d", room.PlayerCount(), MaxPlayers)
	}
}

func TestStartingGameInOneRoomDoesNotStartAnother(t *testing.T) {
	manager := NewManager()
	roomA, _, err := manager.CreateRoom("host-a")
	if err != nil {
		t.Fatalf("CreateRoom A returned error: %v", err)
	}
	roomB, _, err := manager.CreateRoom("host-b")
	if err != nil {
		t.Fatalf("CreateRoom B returned error: %v", err)
	}
	_, err = manager.JoinRoom(roomA.ID(), "player-a2")
	if err != nil {
		t.Fatalf("JoinRoom A returned error: %v", err)
	}
	_, err = manager.JoinRoom(roomB.ID(), "player-b2")
	if err != nil {
		t.Fatalf("JoinRoom B returned error: %v", err)
	}

	_, err = manager.StartGameWithoutAuth(roomA.ID())
	if err != nil {
		t.Fatalf("StartGame room A returned error: %v", err)
	}

	if roomA.State() == nil {
		t.Fatal("room A should have game state")
	}
	if roomB.State() != nil {
		t.Fatal("room B should not be started")
	}
}

func TestDeletingEmptyRoomRemovesItFromManager(t *testing.T) {
	manager := NewManager()
	room, host, err := manager.CreateRoom("host")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}

	if err := manager.LeaveRoom(room.ID(), host.ID); err != nil {
		t.Fatalf("LeaveRoom returned error: %v", err)
	}

	if _, exists := manager.GetRoom(room.ID()); exists {
		t.Fatal("empty room should be removed")
	}
	if manager.RoomCount() != 0 {
		t.Fatalf("room count got %d, want 0", manager.RoomCount())
	}
}

func TestReadyAndHostStartGame(t *testing.T) {
	manager := NewManager()
	room, host, err := manager.CreateRoom("host")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	player, err := manager.JoinRoom(room.ID(), "player")
	if err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}

	_, err = manager.StartGame(room.ID(), host.Token)
	if !errors.Is(err, ErrPlayersNotReady) {
		t.Fatalf("StartGame error got %v, want ErrPlayersNotReady", err)
	}
	if err := manager.SetReady(room.ID(), host.Token, true); err != nil {
		t.Fatalf("SetReady host returned error: %v", err)
	}
	if err := manager.SetReady(room.ID(), player.Token, true); err != nil {
		t.Fatalf("SetReady player returned error: %v", err)
	}

	_, err = manager.StartGame(room.ID(), player.Token)
	if !errors.Is(err, ErrNotHost) {
		t.Fatalf("non-host StartGame error got %v, want ErrNotHost", err)
	}
	_, err = manager.StartGame(room.ID(), host.Token)
	if err != nil {
		t.Fatalf("host StartGame returned error: %v", err)
	}
	if room.State() == nil {
		t.Fatal("room should have game state after host starts")
	}
}

func TestTransferHost(t *testing.T) {
	manager := NewManager()
	room, host, err := manager.CreateRoom("host")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	player, err := manager.JoinRoom(room.ID(), "player")
	if err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}

	if err := manager.TransferHost(room.ID(), player.Token, host.ID); !errors.Is(err, ErrNotHost) {
		t.Fatalf("non-host TransferHost error got %v, want ErrNotHost", err)
	}
	if err := manager.TransferHost(room.ID(), host.Token, player.ID); err != nil {
		t.Fatalf("TransferHost returned error: %v", err)
	}

	view, err := manager.RoomView(room.ID())
	if err != nil {
		t.Fatalf("RoomView returned error: %v", err)
	}
	if view.HostPlayerID != player.ID {
		t.Fatalf("host player ID got %s, want %s", view.HostPlayerID, player.ID)
	}
}

func TestHostLeaveTransfersHost(t *testing.T) {
	manager := NewManager()
	room, host, err := manager.CreateRoom("host")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	player, err := manager.JoinRoom(room.ID(), "player")
	if err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}

	if err := manager.LeaveRoom(room.ID(), host.ID); err != nil {
		t.Fatalf("LeaveRoom host returned error: %v", err)
	}
	view, err := manager.RoomView(room.ID())
	if err != nil {
		t.Fatalf("RoomView returned error: %v", err)
	}
	if view.HostPlayerID != player.ID {
		t.Fatalf("host player ID got %s, want %s", view.HostPlayerID, player.ID)
	}
}

func TestKickVoteRemovesTargetAndTransfersHostIfNeeded(t *testing.T) {
	manager := NewManager()
	room, host, err := manager.CreateRoom("host")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	playerA, err := manager.JoinRoom(room.ID(), "player-a")
	if err != nil {
		t.Fatalf("JoinRoom player A returned error: %v", err)
	}
	playerB, err := manager.JoinRoom(room.ID(), "player-b")
	if err != nil {
		t.Fatalf("JoinRoom player B returned error: %v", err)
	}

	_, _, err = manager.StartKickVote(room.ID(), host.Token, host.ID)
	if !errors.Is(err, ErrCannotVoteKickSelf) {
		t.Fatalf("self kick vote error got %v, want ErrCannotVoteKickSelf", err)
	}
	passed, kickedID, err := manager.StartKickVote(room.ID(), playerA.Token, host.ID)
	if err != nil {
		t.Fatalf("StartKickVote returned error: %v", err)
	}
	if passed {
		t.Fatal("vote should not pass with one approval out of two eligible voters")
	}
	passed, kickedID, err = manager.CastKickVote(room.ID(), playerB.Token, true)
	if err != nil {
		t.Fatalf("CastKickVote returned error: %v", err)
	}
	if !passed || kickedID != host.ID {
		t.Fatalf("vote result passed=%v kicked=%s, want passed host %s", passed, kickedID, host.ID)
	}

	view, err := manager.RoomView(room.ID())
	if err != nil {
		t.Fatalf("RoomView returned error: %v", err)
	}
	if view.PlayerCount != 2 {
		t.Fatalf("player count got %d, want 2", view.PlayerCount)
	}
	if view.HostPlayerID == "" || view.HostPlayerID == host.ID {
		t.Fatalf("host should transfer away from kicked host, got %s", view.HostPlayerID)
	}
}

func TestFixedSeedProducesRepeatableSetup(t *testing.T) {
	stateA := seededTestGameState(t, 42)
	stateB := seededTestGameState(t, 42)

	if !reflect.DeepEqual(gameCardCodes(stateA.DrawPile), gameCardCodes(stateB.DrawPile)) {
		t.Fatalf("draw piles differ for the same seed: %v != %v", gameCardCodes(stateA.DrawPile), gameCardCodes(stateB.DrawPile))
	}
	for i := range stateA.Players {
		if stateA.Players[i].Name != stateB.Players[i].Name {
			t.Fatalf("player %d name got %s, want %s", i, stateA.Players[i].Name, stateB.Players[i].Name)
		}
		if !reflect.DeepEqual(gameCardCodes(stateA.Players[i].Hand), gameCardCodes(stateB.Players[i].Hand)) {
			t.Fatalf("hand %d differs for the same seed", i)
		}
	}
}

func TestJoinOrderRemainsCurrentAfterLeaveAndReplacement(t *testing.T) {
	manager := NewManagerWithSeed(42)
	room, host, err := manager.CreateRoom("host")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	if _, err := manager.JoinRoom(room.ID(), "survivor"); err != nil {
		t.Fatalf("JoinRoom survivor returned error: %v", err)
	}
	if err := manager.LeaveRoom(room.ID(), host.ID); err != nil {
		t.Fatalf("LeaveRoom host returned error: %v", err)
	}
	if _, err := manager.JoinRoom(room.ID(), "replacement"); err != nil {
		t.Fatalf("JoinRoom replacement returned error: %v", err)
	}
	if _, err := manager.StartGameWithoutAuth(room.ID()); err != nil {
		t.Fatalf("StartGameWithoutAuth returned error: %v", err)
	}

	state := room.State()
	if len(state.Players) != 2 || state.Players[0].Name != "survivor" || state.Players[1].Name != "replacement" {
		t.Fatalf("started players got %#v, want survivor then replacement", state.Players)
	}
}

func seededTestGameState(t *testing.T, seed int64) *game.GameState {
	t.Helper()

	manager := NewManagerWithSeed(seed)
	room, _, err := manager.CreateRoom("host")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	if _, err := manager.JoinRoom(room.ID(), "player-a"); err != nil {
		t.Fatalf("JoinRoom player-a returned error: %v", err)
	}
	if _, err := manager.JoinRoom(room.ID(), "player-b"); err != nil {
		t.Fatalf("JoinRoom player-b returned error: %v", err)
	}
	if _, err := manager.StartGameWithoutAuth(room.ID()); err != nil {
		t.Fatalf("StartGameWithoutAuth returned error: %v", err)
	}
	return room.State()
}

func gameCardCodes(cards []game.Card) []game.CardCode {
	codes := make([]game.CardCode, len(cards))
	for i, card := range cards {
		codes[i] = card.Code
	}
	return codes
}

func TestManagerConcurrentAccess(t *testing.T) {
	manager := NewManager()
	const roomCount = 20
	const playersPerRoom = 4

	var wg sync.WaitGroup
	rooms := make(chan *Room, roomCount)
	for i := 0; i < roomCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			room, _, err := manager.CreateRoom("host")
			if err != nil {
				t.Errorf("CreateRoom returned error: %v", err)
				return
			}
			rooms <- room
		}()
	}
	wg.Wait()
	close(rooms)

	for room := range rooms {
		for i := 0; i < playersPerRoom; i++ {
			wg.Add(1)
			go func(roomID string) {
				defer wg.Done()
				if _, err := manager.JoinRoom(roomID, "player"); err != nil {
					t.Errorf("JoinRoom returned error: %v", err)
				}
			}(room.ID())
		}
	}
	wg.Wait()

	if manager.RoomCount() != roomCount {
		t.Fatalf("room count got %d, want %d", manager.RoomCount(), roomCount)
	}
}
