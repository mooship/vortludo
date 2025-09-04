package main

import (
	"context"
	"testing"
)

func testAppWithWords(words []WordEntry) *App {
	wordSet := make(map[string]struct{})
	acceptedSet := make(map[string]struct{})
	hintMap := make(map[string]string)
	for _, w := range words {
		wordSet[w.Word] = struct{}{}
		acceptedSet[w.Word] = struct{}{}
		hintMap[w.Word] = w.Hint
	}
	return &App{
		WordList:        words,
		WordSet:         wordSet,
		AcceptedWordSet: acceptedSet,
		HintMap:         hintMap,
		GameSessions:    make(map[string]*GameState),
	}
}

func dummyContext() context.Context {
	return context.Background()
}

func TestGetRandomWordEntry(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}, {Word: "table", Hint: "furniture"}}
	app := testAppWithWords(words)
	ctx := dummyContext()
	found := false
	for i := 0; i < 10; i++ {
		w := app.getRandomWordEntry(ctx)
		if w.Word == "apple" || w.Word == "table" {
			found = true
		} else {
			t.Errorf("Unexpected word: %v", w.Word)
		}
	}
	if !found {
		t.Error("getRandomWordEntry did not return any valid word")
	}
}

func TestGetRandomWordEntryExcluding(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}, {Word: "table", Hint: "furniture"}}
	app := testAppWithWords(words)
	ctx := dummyContext()
	w, reset := app.getRandomWordEntryExcluding(ctx, []string{"apple"})
	if w.Word != "table" || reset {
		t.Errorf("Expected table, got %v, reset=%v", w.Word, reset)
	}
	w, reset = app.getRandomWordEntryExcluding(ctx, []string{"apple", "table"})
	if reset != true {
		t.Error("Expected reset=true when all words completed")
	}
}

func TestGetHintForWord(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}}
	app := testAppWithWords(words)
	if app.getHintForWord("apple") != "fruit" {
		t.Error("Expected hint 'fruit'")
	}
	if app.getHintForWord("") != "" {
		t.Error("Expected empty string for empty word")
	}
	if app.getHintForWord("unknown") != "" {
		t.Error("Expected empty string for unknown word")
	}
}

func TestBuildHintMap(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}, {Word: "table", Hint: "furniture"}}
	hm := buildHintMap(words)
	if hm["apple"] != "fruit" || hm["table"] != "furniture" {
		t.Error("Hint map not built correctly")
	}
}

func TestGetTargetWord(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}}
	app := testAppWithWords(words)
	ctx := dummyContext()
	game := &GameState{SessionWord: ""}
	w := app.getTargetWord(ctx, game)
	if w != "apple" {
		t.Errorf("Expected 'apple', got %v", w)
	}
	game.SessionWord = "table"
	w = app.getTargetWord(ctx, game)
	if w != "table" {
		t.Errorf("Expected 'table', got %v", w)
	}
}

func TestUpdateGameState_WinLose(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}}
	app := testAppWithWords(words)
	ctx := dummyContext()
	game := &GameState{
		Guesses:      make([][]GuessResult, MaxGuesses),
		CurrentRow:   0,
		GameOver:     false,
		Won:          false,
		TargetWord:   "",
		SessionWord:  "apple",
		GuessHistory: []string{},
	}
	result := []GuessResult{{Letter: "a", Status: GuessStatusCorrect}, {Letter: "p", Status: GuessStatusCorrect}, {Letter: "p", Status: GuessStatusCorrect}, {Letter: "l", Status: GuessStatusCorrect}, {Letter: "e", Status: GuessStatusCorrect}}
	app.updateGameState(ctx, game, "apple", "apple", result, false)
	if !game.Won || !game.GameOver || game.TargetWord != "apple" {
		t.Error("Game should be won and over, target word revealed")
	}
	// Test lose
	game = &GameState{
		Guesses:      make([][]GuessResult, MaxGuesses),
		CurrentRow:   MaxGuesses - 1,
		GameOver:     false,
		Won:          false,
		TargetWord:   "",
		SessionWord:  "apple",
		GuessHistory: []string{},
	}
	app.updateGameState(ctx, game, "wrong", "apple", result, false)
	if !game.GameOver || game.Won {
		t.Error("Game should be over and lost")
	}
	if game.TargetWord != "apple" {
		t.Error("Target word should be revealed on loss")
	}
}

func TestCheckGuess(t *testing.T) {
	// All correct
	res := checkGuess("apple", "apple")
	for _, r := range res {
		if r.Status != GuessStatusCorrect {
			t.Error("All should be correct")
		}
	}
	// All absent
	res = checkGuess("zzzzz", "apple")
	for _, r := range res {
		if r.Status != GuessStatusAbsent {
			t.Error("All should be absent")
		}
	}
	// Mixed
	res = checkGuess("pleap", "apple")
	statuses := []string{GuessStatusPresent, GuessStatusPresent, GuessStatusPresent, GuessStatusPresent, GuessStatusPresent}
	for i, r := range res {
		if r.Status != statuses[i] {
			t.Errorf("Expected %v at %d, got %v", statuses[i], i, r.Status)
		}
	}
}

func TestIsValidWordAndIsAcceptedWord(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}}
	app := testAppWithWords(words)
	if !app.isValidWord("apple") {
		t.Error("apple should be valid")
	}
	if app.isValidWord("table") {
		t.Error("table should not be valid")
	}
	if !app.isAcceptedWord("apple") {
		t.Error("apple should be accepted")
	}
	if app.isAcceptedWord("table") {
		t.Error("table should not be accepted")
	}
}

func TestCreateNewGame(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}}
	app := testAppWithWords(words)
	ctx := dummyContext()
	game := app.createNewGame(ctx, "sess1")
	if game.SessionWord != "apple" {
		t.Error("SessionWord should be 'apple'")
	}
	if len(game.Guesses) != MaxGuesses {
		t.Error("Guesses length incorrect")
	}
	if app.GameSessions["sess1"] == nil {
		t.Error("Game not stored in session map")
	}
}

func TestCreateNewGameWithCompletedWords(t *testing.T) {
	words := []WordEntry{{Word: "apple", Hint: "fruit"}, {Word: "table", Hint: "furniture"}}
	app := testAppWithWords(words)
	ctx := dummyContext()
	game, reset := app.createNewGameWithCompletedWords(ctx, "sess2", []string{"apple"})
	if game.SessionWord != "table" || reset {
		t.Error("Should select 'table' and reset=false")
	}
	_, reset = app.createNewGameWithCompletedWords(ctx, "sess3", []string{"apple", "table"})
	if !reset {
		t.Error("Should set reset=true when all words completed")
	}
}
