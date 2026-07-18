package main

import "testing"

func TestGameTestSeed(t *testing.T) {
	t.Setenv("GAME_TEST_SEED", "12345")
	seed := gameTestSeed()
	if seed == nil || *seed != 12345 {
		t.Fatalf("gameTestSeed got %v, want 12345", seed)
	}
}

func TestGameTestSeedIgnoresEmptyAndInvalidValues(t *testing.T) {
	t.Setenv("GAME_TEST_SEED", "")
	if seed := gameTestSeed(); seed != nil {
		t.Fatalf("empty GAME_TEST_SEED got %d, want nil", *seed)
	}

	t.Setenv("GAME_TEST_SEED", "invalid")
	if seed := gameTestSeed(); seed != nil {
		t.Fatalf("invalid GAME_TEST_SEED got %d, want nil", *seed)
	}
}
