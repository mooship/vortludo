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

func TestGetRandomWordEntry(t *testing.T) {
	app := &App{WordList: []WordEntry{{Word: "APPLE", Hint: "A fruit"}, {Word: "BERRY", Hint: "Another fruit"}}}
	ctx := context.Background()
	entry := app.getRandomWordEntry(ctx)
	if entry.Word != "APPLE" && entry.Word != "BERRY" {
		t.Errorf("getRandomWordEntry returned unexpected word: %v", entry.Word)
	}
}

func TestGetHintForWord(t *testing.T) {
	app := &App{HintMap: map[string]string{"APPLE": "A fruit"}}
	hint := app.getHintForWord("APPLE")
	if hint != "A fruit" {
		t.Errorf("getHintForWord failed: got %v", hint)
	}
	hintMissing := app.getHintForWord("BERRY")
	if hintMissing != "" {
		t.Errorf("getHintForWord for missing word should be empty, got %v", hintMissing)
	}
}

func TestGetTargetWord(t *testing.T) {
	app := &App{WordList: []WordEntry{{Word: "APPLE", Hint: "A fruit"}}}
	ctx := context.Background()
	game := &GameState{SessionWord: "APPLE"}
	target := app.getTargetWord(ctx, game)
	if target != "APPLE" {
		t.Errorf("getTargetWord did not return SessionWord")
	}
	game2 := &GameState{SessionWord: ""}
	target2 := app.getTargetWord(ctx, game2)
	if target2 == "" {
		t.Errorf("getTargetWord did not assign a word when SessionWord was empty")
	}
}

func TestUpdateGameStateWinLose(t *testing.T) {
	app := &App{}
	ctx := context.Background()
	game := &GameState{
		Guesses:      make([][]GuessResult, MaxGuesses),
		CurrentRow:   0,
		GameOver:     false,
		Won:          false,
		GuessHistory: []string{},
	}
	target := "APPLE"
	guess := "APPLE"
	result := checkGuess(guess, target)
	app.updateGameState(ctx, game, guess, target, result, false)
	if !game.Won || !game.GameOver {
		t.Errorf("updateGameState should set Won and GameOver on correct guess")
	}

	game2 := &GameState{
		Guesses:      make([][]GuessResult, MaxGuesses),
		CurrentRow:   MaxGuesses - 1,
		GameOver:     false,
		Won:          false,
		GuessHistory: []string{},
	}
	guess2 := "BERRY"
	result2 := checkGuess(guess2, target)
	app.updateGameState(ctx, game2, guess2, target, result2, false)
	if !game2.GameOver || game2.Won {
		t.Errorf("updateGameState should set GameOver true and Won false on last guess fail")
	}
}

func TestSaveGameStateAndGetGameState(t *testing.T) {
	app := &App{
		GameSessions: make(map[string]*GameState),
		SessionMutex: sync.RWMutex{},
	}
	sessionID := "testsession"
	game := &GameState{SessionWord: "APPLE"}
	app.saveGameState(sessionID, game)
	ctx := context.Background()
	got := app.getGameState(ctx, sessionID)
	if got.SessionWord != "APPLE" {
		t.Errorf("getGameState did not retrieve saved game state")
	}
}

func TestIsValidWordAndIsAcceptedWord(t *testing.T) {
	app := &App{
		WordSet:         map[string]struct{}{"APPLE": {}},
		AcceptedWordSet: map[string]struct{}{"BERRY": {}},
	}
	if !app.isValidWord("APPLE") || app.isValidWord("BERRY") {
		t.Errorf("isValidWord logic error (extended)")
	}
	if !app.isAcceptedWord("BERRY") || app.isAcceptedWord("APPLE") {
		t.Errorf("isAcceptedWord logic error (extended)")
	}
}
