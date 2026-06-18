package game

import "testing"

func TestCardHelpers(t *testing.T) {
	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "TOKEN_A returns IsToken true", got: IsToken(CardTokenA), want: true},
		{name: "WILD_TOKEN returns IsWildToken true", got: IsWildToken(CardWildToken), want: true},
		{name: "SKIP_TURN returns IsToken false", got: IsToken(CardSkipTurn), want: false},
		{name: "EXPLOSIVE returns IsExplosive true", got: IsExplosive(CardExplosive), want: true},
		{name: "EXPLOSIVE_HOLDER returns CanHoldExplosive true", got: CanHoldExplosive(CardExplosiveHolder), want: true},
		{name: "TARGET_EXTRA_TURNS returns RequiresTarget true", got: RequiresTarget(CardTargetExtraTurns), want: true},
		{name: "REORDER_TOP_3 returns RequiresPrivatePrompt true", got: RequiresPrivatePrompt(CardReorderTop3), want: true},
		{name: "SHIELD returns IsShield true", got: IsShield(CardShield), want: true},
		{name: "TOKEN_A returns IsWildToken false", got: IsWildToken(CardTokenA), want: false},
		{name: "SHUFFLE_DECK returns IsAction true", got: IsAction(CardShuffleDeck), want: true},
		{name: "EXPLOSIVE returns IsAction false", got: IsAction(CardExplosive), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("got %v, want %v", test.got, test.want)
			}
		})
	}
}
