package game

import "errors"

var (
	ErrInvalidPhase        = errors.New("invalid phase")
	ErrNotYourTurn         = errors.New("not your turn")
	ErrPlayerNotFound      = errors.New("player not found")
	ErrPlayerNotAlive      = errors.New("player not alive")
	ErrCardNotInHand       = errors.New("card not in hand")
	ErrInvalidCardPlay     = errors.New("invalid card play")
	ErrTargetRequired      = errors.New("target required")
	ErrInvalidTarget       = errors.New("invalid target")
	ErrDrawPileEmpty       = errors.New("draw pile empty")
	ErrNoPendingAction     = errors.New("no pending action")
	ErrInvalidPlacement    = errors.New("invalid placement")
	ErrActionNotCancelable = errors.New("action not cancelable")
	ErrCancelWindowActive  = errors.New("cancel window still active")
)
