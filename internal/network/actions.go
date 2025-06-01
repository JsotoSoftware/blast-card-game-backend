package network

// Action types for game synchronization
const (
	// Full state synchronization
	ActionFullState = "full_state"

	// Room related actions
	ActionRoomCreated = "room_created"
	ActionRoomList    = "room_list"
	ActionRoomJoined  = "room_joined"

	// Match related actions
	ActionMatchStarted = "match_started"

	// Player related actions
	ActionPlayerJoined     = "player_joined"
	ActionPlayerLeft       = "player_left"
	ActionPlayerIdentified = "player_identified"
	ActionEndTurn          = "end_turn"

	// Game state actions
	ActionCardPlayed   = "card_played"
	ActionCardDrawn    = "card_drawn"
	ActionTurnChanged  = "turn_changed"
	ActionDeckShuffled = "deck_shuffled"

	// Chat and system messages
	ActionChatMessage   = "chat_message"
	ActionSystemMessage = "system_message"
)
