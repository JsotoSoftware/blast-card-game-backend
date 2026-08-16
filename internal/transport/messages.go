package transport

import (
	"encoding/json"

	"exploding-game/server/internal/game"
)

const ProtocolVersion = 1

type ClientEnvelope struct {
	Version     int             `json:"version"`
	Type        string          `json:"type"`
	RequestID   string          `json:"requestId"`
	RoomID      string          `json:"roomId,omitempty"`
	PlayerToken string          `json:"playerToken,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

type ServerEnvelope struct {
	Version  int    `json:"version"`
	Type     string `json:"type"`
	Sequence int64  `json:"sequence"`
	Payload  any    `json:"payload,omitempty"`
}

type CommandErrorPayload struct {
	RequestID string `json:"requestId,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type CreateRoomPayload struct {
	PlayerName string `json:"playerName,omitempty"`
}

type JoinRoomPayload struct {
	RoomID     string `json:"roomId,omitempty"`
	PlayerName string `json:"playerName,omitempty"`
}

type RoomSessionPayload struct {
	RequestID   string `json:"requestId,omitempty"`
	RoomID      string `json:"roomId"`
	PlayerID    string `json:"playerId"`
	PlayerToken string `json:"playerToken"`
	IsHost      bool   `json:"isHost"`
}

type SetReadyPayload struct {
	Ready bool `json:"ready"`
}

type TransferHostPayload struct {
	TargetPlayerID string `json:"targetPlayerId"`
}

type StartKickVotePayload struct {
	TargetPlayerID string `json:"targetPlayerId"`
}

type CastKickVotePayload struct {
	Approve bool `json:"approve"`
}

type PlayCardPayload struct {
	CardIDs  []string `json:"cardIds"`
	TargetID string   `json:"targetId,omitempty"`
}

type PlayComboPayload struct {
	CardIDs       []string      `json:"cardIds"`
	TargetID      string        `json:"targetId,omitempty"`
	RequestedCode game.CardCode `json:"requestedCode,omitempty"`
}

type PlaceExplosivePayload struct {
	Index *int `json:"index"`
}

type ChooseCardForRequestPayload struct {
	CardID string `json:"cardId"`
}

type PlayCancelPayload struct {
	CardID          string `json:"cardId,omitempty"`
	PendingActionID string `json:"pendingActionId,omitempty"`
}

type ChooseCardFromDiscardPayload struct {
	CardID string `json:"cardId"`
}

type ChooseCardForRecyclePayload struct {
	CardID string `json:"cardId"`
}

type ChooseMarkedCardPayload struct {
	CardID string `json:"cardId"`
}

type SubmitReorderedTopCardsPayload struct {
	CardIDs []string `json:"cardIds"`
}

type CommandAckPayload struct {
	RequestID string `json:"requestId,omitempty"`
}

type GameStartedPayload struct {
	RequestID string `json:"requestId,omitempty"`
	RoomID    string `json:"roomId"`
	Events    any    `json:"events"`
}
