package game

type Player struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Hand          []Card   `json:"hand"`
	Alive         bool     `json:"alive"`
	Connected     bool     `json:"connected"`
	Ready         bool     `json:"ready"`
	IsHost        bool     `json:"isHost"`
	Blinded       bool     `json:"blinded"`
	MarkedCardIDs []string `json:"markedCardIds"`
}

type GameState struct {
	RoomID          string                `json:"roomId"`
	Phase           GamePhase             `json:"phase"`
	Players         []Player              `json:"players"`
	DrawPile        []Card                `json:"drawPile"`
	DiscardPile     []Card                `json:"discardPile"`
	CurrentPlayerID string                `json:"currentPlayerId"`
	TurnDebt        map[string]int        `json:"turnDebt"`
	PendingAction   *PendingAction        `json:"pendingAction,omitempty"`
	MarkedCards     map[string]MarkedCard `json:"markedCards"`
	WinnerPlayerID  string                `json:"winnerPlayerId,omitempty"`
	EventSeq        int64                 `json:"eventSeq"`
}

type MarkedCard struct {
	CardID   string `json:"cardId"`
	OwnerID  string `json:"ownerId"`
	Revealed Card   `json:"revealed"`
}

type PendingActionType string

const (
	PendingPlayCard           PendingActionType = "PLAY_CARD"
	PendingCardCombo          PendingActionType = "CARD_COMBO"
	PendingTokenCombo         PendingActionType = "TOKEN_COMBO"
	PendingDiscardRecovery    PendingActionType = "DISCARD_RECOVERY"
	PendingRequestCardChoice  PendingActionType = "REQUEST_CARD_CHOICE"
	PendingRecycleChoices     PendingActionType = "RECYCLE_CHOICES"
	PendingMarkedCardChoice   PendingActionType = "MARKED_CARD_CHOICE"
	PendingDeckReorder        PendingActionType = "DECK_REORDER"
	PendingExplosivePlacement PendingActionType = "EXPLOSIVE_PLACEMENT"
)

type ComboKind string

const (
	ComboPair          ComboKind = "PAIR"
	ComboTriple        ComboKind = "TRIPLE"
	ComboFiveDifferent ComboKind = "FIVE_DIFFERENT"
)

type PendingAction struct {
	ID              string            `json:"id"`
	SourcePlayerID  string            `json:"sourcePlayerId"`
	Type            PendingActionType `json:"type"`
	CardIDs         []string          `json:"cardIds"`
	Cards           []Card            `json:"-"`
	TargetPlayerID  string            `json:"targetPlayerId,omitempty"`
	ComboKind       ComboKind         `json:"comboKind,omitempty"`
	RequestedCode   CardCode          `json:"requestedCode,omitempty"`
	CancelCount     int               `json:"cancelCount"`
	ExpiresAtUnixMs int64             `json:"expiresAtUnixMs"`
}
