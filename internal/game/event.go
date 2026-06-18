package game

type EventType string

const (
	EventGameStarted       EventType = "GAME_STARTED"
	EventTurnStarted       EventType = "TURN_STARTED"
	EventCardPlayed        EventType = "CARD_PLAYED"
	EventCardDrawn         EventType = "CARD_DRAWN"
	EventCardDiscarded     EventType = "CARD_DISCARDED"
	EventActionPending     EventType = "ACTION_PENDING"
	EventActionResolved    EventType = "ACTION_RESOLVED"
	EventActionCanceled    EventType = "ACTION_CANCELED"
	EventPlayerEliminated  EventType = "PLAYER_ELIMINATED"
	EventGameOver          EventType = "GAME_OVER"
	EventPrivatePromptSent EventType = "PRIVATE_PROMPT_SENT"
)

type Event struct {
	Seq      int64     `json:"seq"`
	Type     EventType `json:"type"`
	PlayerID string    `json:"playerId,omitempty"`
	CardIDs  []string  `json:"cardIds,omitempty"`
	TargetID string    `json:"targetId,omitempty"`
	Message  string    `json:"message,omitempty"`
}
