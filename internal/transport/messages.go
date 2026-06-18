package transport

import "encoding/json"

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
