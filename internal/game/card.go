package game

import (
	"math/rand"
	"time"
)

type Card struct {
	Suit  string `json:"suit"`
	Value string `json:"value"`
}

type Deck struct {
	Cards []Card `json:"cards"`
}

func NewDeck() *Deck {
	suits := []string{"Hearts", "Diamonds", "Clubs", "Spades"}
	values := []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}

	deck := &Deck{
		Cards: make([]Card, 0, 52),
	}

	for _, suit := range suits {
		for _, value := range values {
			deck.Cards = append(deck.Cards, Card{Suit: suit, Value: value})
		}
	}

	return deck
}

func (d *Deck) Shuffle() {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(d.Cards), func(i, j int) {
		d.Cards[i], d.Cards[j] = d.Cards[j], d.Cards[i]
	})
}

func (d *Deck) Draw() (Card, bool) {
	if len(d.Cards) == 0 {
		return Card{}, false
	}
	card := d.Cards[0]
	d.Cards = d.Cards[1:]
	return card, true
}
