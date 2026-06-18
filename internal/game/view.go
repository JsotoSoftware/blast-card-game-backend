package game

type PlayerGameView struct {
	RoomID            string                   `json:"roomId"`
	Phase             GamePhase                `json:"phase"`
	You               PlayerPrivateView        `json:"you"`
	Players           []PlayerPublicView       `json:"players"`
	DrawPileCount     int                      `json:"drawPileCount"`
	DiscardPile       []PublicCardView         `json:"discardPile"`
	CurrentPlayerID   string                   `json:"currentPlayerId"`
	PendingAction     *PublicPendingActionView `json:"pendingAction,omitempty"`
	PublicMarkedCards []PublicMarkedCardView   `json:"publicMarkedCards"`
	AvailableActions  []CommandType            `json:"availableActions"`
}

type PlayerPrivateView struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Hand    []PrivateCardView `json:"hand"`
	Alive   bool              `json:"alive"`
	Blinded bool              `json:"blinded"`
}

type PlayerPublicView struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	HandCount     int      `json:"handCount"`
	Alive         bool     `json:"alive"`
	Connected     bool     `json:"connected"`
	Ready         bool     `json:"ready"`
	IsHost        bool     `json:"isHost"`
	Blinded       bool     `json:"blinded"`
	MarkedCardIDs []string `json:"markedCardIds"`
}

type PrivateCardView struct {
	ID       string   `json:"id"`
	Code     CardCode `json:"code,omitempty"`
	IsHidden bool     `json:"isHidden"`
}

type PublicCardView struct {
	ID   string   `json:"id"`
	Code CardCode `json:"code"`
}

type PublicPendingActionView struct {
	ID             string            `json:"id"`
	SourcePlayerID string            `json:"sourcePlayerId"`
	Type           PendingActionType `json:"type"`
	TargetPlayerID string            `json:"targetPlayerId,omitempty"`
	CancelCount    int               `json:"cancelCount"`
}

type PublicMarkedCardView struct {
	CardID  string   `json:"cardId"`
	OwnerID string   `json:"ownerId"`
	Code    CardCode `json:"code"`
}
