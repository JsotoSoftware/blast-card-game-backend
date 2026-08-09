package game

import (
	"errors"
	"fmt"
	"math/rand"
	"time"
)

const (
	MinPlayers             = 2
	MaxPlayers             = 10
	InitialRandomHandCards = 7
	RequiredShieldCards    = 1
	InitialHandSize        = InitialRandomHandCards + RequiredShieldCards
)

var ErrInvalidPlayerCount = errors.New("invalid player count")

type deckProfile struct {
	minPlayers       int
	maxPlayers       int
	fixedShieldCount int
	extraShieldCards int
	counts           map[CardCode]int
}

var deckCardOrder = []CardCode{
	CardExplosive,
	CardShield,
	CardCancel,
	CardForceExtraTurns,
	CardTargetExtraTurns,
	CardSkipTurn,
	CardSkipAllTurns,
	CardRequestCard,
	CardShuffleDeck,
	CardPeekDeck,
	CardPeekDeck5,
	CardWildToken,
	CardExplosiveHolder,
	CardCollectiveRecycle,
	CardRevealHeldCard,
	CardDrawFromBottom,
	CardSwapTopBottom,
	CardReorderTop3,
	CardReorderTop5,
	CardBlindHand,
	CardStackExplosivesOnTop,
	CardTokenA,
	CardTokenB,
	CardTokenC,
	CardTokenD,
	CardTokenE,
}

var deckProfiles = []deckProfile{
	{
		minPlayers:       2,
		maxPlayers:       3,
		extraShieldCards: 3,
		counts: map[CardCode]int{
			CardCancel:               4,
			CardForceExtraTurns:      2,
			CardTargetExtraTurns:     2,
			CardSkipTurn:             4,
			CardSkipAllTurns:         1,
			CardRequestCard:          2,
			CardShuffleDeck:          2,
			CardPeekDeck:             3,
			CardWildToken:            2,
			CardExplosiveHolder:      1,
			CardCollectiveRecycle:    1,
			CardRevealHeldCard:       1,
			CardDrawFromBottom:       3,
			CardSwapTopBottom:        1,
			CardReorderTop3:          3,
			CardBlindHand:            2,
			CardStackExplosivesOnTop: 1,
			CardTokenA:               3,
		},
	},
	{
		minPlayers:       4,
		maxPlayers:       7,
		fixedShieldCount: 8,
		counts: map[CardCode]int{
			CardCancel:               5,
			CardForceExtraTurns:      3,
			CardTargetExtraTurns:     3,
			CardSkipTurn:             6,
			CardSkipAllTurns:         1,
			CardRequestCard:          4,
			CardShuffleDeck:          4,
			CardPeekDeck:             3,
			CardPeekDeck5:            1,
			CardWildToken:            4,
			CardExplosiveHolder:      1,
			CardCollectiveRecycle:    1,
			CardRevealHeldCard:       2,
			CardDrawFromBottom:       4,
			CardSwapTopBottom:        2,
			CardReorderTop3:          4,
			CardReorderTop5:          1,
			CardBlindHand:            2,
			CardStackExplosivesOnTop: 1,
			CardTokenA:               4,
		},
	},
	{
		minPlayers:       8,
		maxPlayers:       10,
		fixedShieldCount: 10,
		counts: map[CardCode]int{
			CardCancel:               9,
			CardForceExtraTurns:      5,
			CardTargetExtraTurns:     5,
			CardSkipTurn:             10,
			CardSkipAllTurns:         1,
			CardRequestCard:          6,
			CardShuffleDeck:          6,
			CardPeekDeck:             6,
			CardPeekDeck5:            1,
			CardWildToken:            6,
			CardExplosiveHolder:      1,
			CardCollectiveRecycle:    1,
			CardRevealHeldCard:       3,
			CardDrawFromBottom:       7,
			CardSwapTopBottom:        3,
			CardReorderTop3:          6,
			CardReorderTop5:          1,
			CardBlindHand:            2,
			CardStackExplosivesOnTop: 1,
			CardTokenA:               7,
		},
	},
}

type DeckFactory struct {
	rng    *rand.Rand
	nextID int
}

func NewDeckFactory(rng *rand.Rand) *DeckFactory {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return &DeckFactory{rng: rng}
}

func DeckCardCounts(playerCount int) (map[CardCode]int, error) {
	profile, err := profileForPlayerCount(playerCount)
	if err != nil {
		return nil, err
	}

	counts := cloneCounts(profile.counts)
	counts[CardShield] = profile.shieldCount(playerCount)
	counts[CardExplosive] = playerCount - 1

	return counts, nil
}

func ExpectedDeckSize(playerCount int) (int, error) {
	counts, err := DeckCardCounts(playerCount)
	if err != nil {
		return 0, err
	}

	total := 0
	for _, count := range counts {
		total += count
	}

	return total, nil
}

func ValidatePlayerCount(playerCount int) error {
	if playerCount < MinPlayers || playerCount > MaxPlayers {
		return fmt.Errorf("%w: got %d, want %d-%d", ErrInvalidPlayerCount, playerCount, MinPlayers, MaxPlayers)
	}

	return nil
}

func (f *DeckFactory) NewDeck(playerCount int) ([]Card, error) {
	counts, err := DeckCardCounts(playerCount)
	if err != nil {
		return nil, err
	}

	deck := make([]Card, 0)
	for _, code := range deckCardOrder {
		for range counts[code] {
			deck = append(deck, f.newCard(code))
		}
	}

	return deck, nil
}

func (f *DeckFactory) NewSetup(roomID string, players []Player) (*GameState, error) {
	counts, err := DeckCardCounts(len(players))
	if err != nil {
		return nil, err
	}

	dealCounts := cloneCounts(counts)
	delete(dealCounts, CardExplosive)
	delete(dealCounts, CardShield)

	dealPool := f.newCardsFromCounts(dealCounts)
	f.Shuffle(dealPool)

	setupPlayers := make([]Player, len(players))
	dealIndex := 0
	for i, player := range players {
		player.Hand = make([]Card, 0, InitialHandSize)
		player.Hand = append(player.Hand, f.newCard(CardShield))
		player.Hand = append(player.Hand, dealPool[dealIndex:dealIndex+InitialRandomHandCards]...)
		dealIndex += InitialRandomHandCards
		player.Alive = true
		player.Blinded = false
		setupPlayers[i] = player
	}

	remainingShieldCount := counts[CardShield] - len(players)*RequiredShieldCards
	if remainingShieldCount < 0 {
		return nil, fmt.Errorf("deck profile has too few shield cards for %d players", len(players))
	}

	drawPile := make([]Card, 0, len(dealPool)-dealIndex+remainingShieldCount+counts[CardExplosive])
	drawPile = append(drawPile, dealPool[dealIndex:]...)
	for range remainingShieldCount {
		drawPile = append(drawPile, f.newCard(CardShield))
	}
	for range counts[CardExplosive] {
		drawPile = append(drawPile, f.newCard(CardExplosive))
	}
	f.Shuffle(drawPile)

	turnDebt := make(map[string]int, len(setupPlayers))
	for _, player := range setupPlayers {
		turnDebt[player.ID] = 0
	}
	turnDebt[setupPlayers[0].ID] = 1

	return &GameState{
		RoomID:          roomID,
		Phase:           PhasePlayerTurn,
		Players:         setupPlayers,
		DrawPile:        drawPile,
		DiscardPile:     []Card{},
		CurrentPlayerID: setupPlayers[0].ID,
		TurnDebt:        turnDebt,
		MarkedCards:     map[string]MarkedCard{},
	}, nil
}

func (f *DeckFactory) Shuffle(cards []Card) {
	f.rng.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
}

func (f *DeckFactory) newCardsFromCounts(counts map[CardCode]int) []Card {
	total := 0
	for _, count := range counts {
		total += count
	}

	cards := make([]Card, 0, total)
	for _, code := range deckCardOrder {
		for range counts[code] {
			cards = append(cards, f.newCard(code))
		}
	}

	return cards
}

func (f *DeckFactory) newCard(code CardCode) Card {
	f.nextID++
	return Card{
		ID:   fmt.Sprintf("card-%06d", f.nextID),
		Code: code,
	}
}

func profileForPlayerCount(playerCount int) (deckProfile, error) {
	if err := ValidatePlayerCount(playerCount); err != nil {
		return deckProfile{}, err
	}

	for _, profile := range deckProfiles {
		if playerCount >= profile.minPlayers && playerCount <= profile.maxPlayers {
			return profile, nil
		}
	}

	return deckProfile{}, fmt.Errorf("%w: no deck profile for %d players", ErrInvalidPlayerCount, playerCount)
}

func (p deckProfile) shieldCount(playerCount int) int {
	if p.fixedShieldCount > 0 {
		return p.fixedShieldCount
	}
	return playerCount + p.extraShieldCards
}

func cloneCounts(source map[CardCode]int) map[CardCode]int {
	counts := make(map[CardCode]int, len(source))
	for code, count := range source {
		counts[code] = count
	}
	return counts
}
