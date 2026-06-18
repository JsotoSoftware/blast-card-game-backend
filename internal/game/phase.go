package game

type GamePhase string

const (
	PhaseLobby                     GamePhase = "LOBBY"
	PhaseSetup                     GamePhase = "SETUP"
	PhasePlayerTurn                GamePhase = "PLAYER_TURN"
	PhaseCancelWindow              GamePhase = "CANCEL_WINDOW"
	PhaseResolvingAction           GamePhase = "RESOLVING_ACTION"
	PhaseWaitingRequestCardChoice  GamePhase = "WAITING_REQUEST_CARD_CHOICE"
	PhaseWaitingRecycleChoices     GamePhase = "WAITING_RECYCLE_CHOICES"
	PhaseWaitingMarkedCardChoice   GamePhase = "WAITING_MARKED_CARD_CHOICE"
	PhaseWaitingDeckReorder        GamePhase = "WAITING_DECK_REORDER"
	PhaseWaitingExplosivePlacement GamePhase = "WAITING_EXPLOSIVE_PLACEMENT"
	PhaseGameOver                  GamePhase = "GAME_OVER"
)
