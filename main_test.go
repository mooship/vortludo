package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNormalizeGuess(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		{" apple ", "APPLE"},
		{"Banana", "BANANA"},
		{"  kiwi", "KIWI"},
	}
	for _, c := range cases {
		if got := normalizeGuess(c.in); got != c.out {
			t.Errorf("normalizeGuess(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestCheckGuess(t *testing.T) {
	guess := "CRANE"
	target := "CRATE"
	result := checkGuess(guess, target)
	statuses := []string{"correct", "correct", "correct", "absent", "correct"}
	for i, r := range result {
		if r.Status != statuses[i] {
			t.Errorf("checkGuess: letter %d got status %q, want %q", i, r.Status, statuses[i])
		}
	}
}

func TestBuildHintMap(t *testing.T) {
	words := []WordEntry{{Word: "APPLE", Hint: "A fruit"}, {Word: "BERRY", Hint: "Another fruit"}}
	m := buildHintMap(words)
	if m["APPLE"] != "A fruit" || m["BERRY"] != "Another fruit" {
		t.Errorf("buildHintMap failed: got %v", m)
	}
}

func TestIsValidWord(t *testing.T) {
	app := &App{WordSet: map[string]struct{}{"APPLE": {}, "BERRY": {}}}
	if !app.isValidWord("APPLE") || app.isValidWord("PEACH") {
		t.Errorf("isValidWord logic error")
	}
}

func TestIsAcceptedWord(t *testing.T) {
	app := &App{AcceptedWordSet: map[string]struct{}{"APPLE": {}, "BERRY": {}}}
	if !app.isAcceptedWord("BERRY") || app.isAcceptedWord("PEACH") {
		t.Errorf("isAcceptedWord logic error")
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{time.Second * 5, "5 seconds"},
		{time.Minute*2 + time.Second*3, "2 minutes, 3 seconds"},
		{time.Hour*1 + time.Minute*2 + time.Second*3, "1 hour, 2 minutes, 3 seconds"},
	}
	for _, c := range cases {
		got := formatUptime(c.d)
		if got != c.want {
			t.Errorf("formatUptime(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if plural(1) != "" || plural(2) != "s" {
		t.Errorf("plural logic error")
	}
}

func TestCreateNewGame(t *testing.T) {
	app := &App{
		WordList:     []WordEntry{{Word: "APPLE", Hint: "A fruit"}},
		SessionMutex: sync.RWMutex{},
		GameSessions: make(map[string]*GameState),
	}
	ctx := context.Background()
	game := app.createNewGame(ctx, "testsession")
	if game.SessionWord != "APPLE" {
		t.Errorf("createNewGame did not set SessionWord")
	}
	if len(game.Guesses) != MaxGuesses {
		t.Errorf("createNewGame did not set correct number of guesses")
	}
}
