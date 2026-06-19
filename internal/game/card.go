package game

type CardCode string

const (
	CardExplosive            CardCode = "EXPLOSIVE"
	CardShield               CardCode = "SHIELD"
	CardCancel               CardCode = "CANCEL"
	CardForceExtraTurns      CardCode = "FORCE_EXTRA_TURNS"
	CardTargetExtraTurns     CardCode = "TARGET_EXTRA_TURNS"
	CardSkipTurn             CardCode = "SKIP_TURN"
	CardSkipAllTurns         CardCode = "SKIP_ALL_TURNS"
	CardRequestCard          CardCode = "REQUEST_CARD"
	CardShuffleDeck          CardCode = "SHUFFLE_DECK"
	CardPeekDeck             CardCode = "PEEK_DECK"
	CardPeekDeck5            CardCode = "PEEK_DECK_5"
	CardWildToken            CardCode = "WILD_TOKEN"
	CardExplosiveHolder      CardCode = "EXPLOSIVE_HOLDER"
	CardCollectiveRecycle    CardCode = "COLLECTIVE_RECYCLE"
	CardRevealHeldCard       CardCode = "REVEAL_HELD_CARD"
	CardDrawFromBottom       CardCode = "DRAW_FROM_BOTTOM"
	CardSwapTopBottom        CardCode = "SWAP_TOP_BOTTOM"
	CardReorderTop3          CardCode = "REORDER_TOP_3"
	CardReorderTop5          CardCode = "REORDER_TOP_5"
	CardBlindHand            CardCode = "BLIND_HAND"
	CardStackExplosivesOnTop CardCode = "STACK_EXPLOSIVES_ON_TOP"
	CardTokenA               CardCode = "TOKEN_A"
	CardTokenB               CardCode = "TOKEN_B"
	CardTokenC               CardCode = "TOKEN_C"
	CardTokenD               CardCode = "TOKEN_D"
	CardTokenE               CardCode = "TOKEN_E"
)

type Card struct {
	ID   string   `json:"id"`
	Code CardCode `json:"code"`
}

func IsToken(code CardCode) bool {
	switch code {
	case CardTokenA, CardTokenB, CardTokenC, CardTokenD, CardTokenE:
		return true
	default:
		return false
	}
}

func IsWildToken(code CardCode) bool {
	return code == CardWildToken
}

func IsAction(code CardCode) bool {
	switch code {
	case CardCancel,
		CardForceExtraTurns,
		CardTargetExtraTurns,
		CardSkipTurn,
		CardSkipAllTurns,
		CardRequestCard,
		CardShuffleDeck,
		CardPeekDeck,
		CardPeekDeck5,
		CardCollectiveRecycle,
		CardRevealHeldCard,
		CardDrawFromBottom,
		CardSwapTopBottom,
		CardReorderTop3,
		CardReorderTop5,
		CardBlindHand,
		CardStackExplosivesOnTop:
		return true
	default:
		return false
	}
}

func IsExplosive(code CardCode) bool {
	return code == CardExplosive
}

func IsShield(code CardCode) bool {
	return code == CardShield
}

func IsCancel(code CardCode) bool {
	return code == CardCancel
}

func CanHoldExplosive(code CardCode) bool {
	return code == CardExplosiveHolder
}

func RequiresTarget(code CardCode) bool {
	switch code {
	case CardTargetExtraTurns,
		CardRequestCard,
		CardRevealHeldCard,
		CardBlindHand:
		return true
	default:
		return false
	}
}

func RequiresPrivatePrompt(code CardCode) bool {
	switch code {
	case CardRequestCard,
		CardPeekDeck,
		CardPeekDeck5,
		CardCollectiveRecycle,
		CardRevealHeldCard,
		CardReorderTop3,
		CardReorderTop5:
		return true
	default:
		return false
	}
}
