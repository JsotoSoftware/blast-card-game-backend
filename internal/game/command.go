package game

import "encoding/json"

type CommandType string

const (
	CommandCreateRoom              CommandType = "CREATE_ROOM"
	CommandJoinRoom                CommandType = "JOIN_ROOM"
	CommandRejoinRoom              CommandType = "REJOIN_ROOM"
	CommandLeaveRoom               CommandType = "LEAVE_ROOM"
	CommandSetReady                CommandType = "SET_READY"
	CommandStartGame               CommandType = "START_GAME"
	CommandTransferHost            CommandType = "TRANSFER_HOST"
	CommandStartKickVote           CommandType = "START_KICK_VOTE"
	CommandCastKickVote            CommandType = "CAST_KICK_VOTE"
	CommandPlayCard                CommandType = "PLAY_CARD"
	CommandDrawCard                CommandType = "DRAW_CARD"
	CommandPlayCancel              CommandType = "PLAY_CANCEL"
	CommandChooseTarget            CommandType = "CHOOSE_TARGET"
	CommandChooseCardForRequest    CommandType = "CHOOSE_CARD_FOR_REQUEST"
	CommandChooseCardForRecycle    CommandType = "CHOOSE_CARD_FOR_RECYCLE"
	CommandChooseCardFromDiscard   CommandType = "CHOOSE_CARD_FROM_DISCARD"
	CommandChooseMarkedCard        CommandType = "CHOOSE_MARKED_CARD"
	CommandSubmitReorderedTopCards CommandType = "SUBMIT_REORDERED_TOP_CARDS"
	CommandPlaceExplosive          CommandType = "PLACE_EXPLOSIVE"
	CommandSendEmote               CommandType = "SEND_EMOTE"
	CommandPing                    CommandType = "PING"
)

type Command struct {
	Type     CommandType     `json:"type"`
	PlayerID string          `json:"playerId"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type PlayCardCommand struct {
	PlayerID string   `json:"playerId"`
	CardIDs  []string `json:"cardIds"`
	TargetID string   `json:"targetId,omitempty"`
}

type PlayComboCommand struct {
	PlayerID      string   `json:"playerId"`
	CardIDs       []string `json:"cardIds"`
	TargetID      string   `json:"targetId,omitempty"`
	RequestedCode CardCode `json:"requestedCode,omitempty"`
}

type DrawCardCommand struct {
	PlayerID string `json:"playerId"`
}

type PlaceExplosiveCommand struct {
	PlayerID string `json:"playerId"`
	Index    int    `json:"index"`
}

type PlayCancelCommand struct {
	PlayerID        string `json:"playerId"`
	CardID          string `json:"cardId,omitempty"`
	PendingActionID string `json:"pendingActionId,omitempty"`
}

type ChooseCardFromDiscardCommand struct {
	PlayerID string `json:"playerId"`
	CardID   string `json:"cardId"`
}
