package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func loadWordListFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var words []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		w := strings.TrimSpace(scanner.Text())
		if w != "" {
			words = append(words, strings.ToUpper(w))
		}
	}
	return words, scanner.Err()
}

func TestAcceptedWordsNoDuplicates(t *testing.T) {
	words, err := loadWordListFromFile("data/accepted_words.txt")
	if err != nil {
		t.Fatalf("failed to load accepted_words.txt: %v", err)
	}
	seen := make(map[string]struct{})
	for _, w := range words {
		if _, ok := seen[w]; ok {
			t.Errorf("duplicate word in accepted_words.txt: %s", w)
		}
		seen[w] = struct{}{}
	}
}

func TestWordsNoDuplicates(t *testing.T) {
	f, err := os.Open("data/words.json")
	if err != nil {
		t.Fatalf("failed to open words.json: %v", err)
	}
	defer f.Close()
	var wordList struct {
		Words []struct {
			Word string `json:"word"`
			Hint string `json:"hint"`
		}
	}
	if err := json.NewDecoder(f).Decode(&wordList); err != nil {
		t.Fatalf("failed to decode words.json: %v", err)
	}
	seen := make(map[string]struct{})
	for _, entry := range wordList.Words {
		w := strings.ToUpper(strings.TrimSpace(entry.Word))
		if _, ok := seen[w]; ok {
			t.Errorf("duplicate word in words.json: %s", w)
		}
		seen[w] = struct{}{}
	}
}

func TestAllWordsInAcceptedList(t *testing.T) {
	accepted, err := loadWordListFromFile("data/accepted_words.txt")
	if err != nil {
		t.Fatalf("failed to load accepted_words.txt: %v", err)
	}
	acceptedSet := make(map[string]struct{}, len(accepted))
	for _, w := range accepted {
		acceptedSet[w] = struct{}{}
	}
	f, err := os.Open("data/words.json")
	if err != nil {
		t.Fatalf("failed to open words.json: %v", err)
	}
	defer f.Close()
	var wordList struct {
		Words []struct {
			Word string `json:"word"`
			Hint string `json:"hint"`
		}
	}
	if err := json.NewDecoder(f).Decode(&wordList); err != nil {
		t.Fatalf("failed to decode words.json: %v", err)
	}
	for _, entry := range wordList.Words {
		w := strings.ToUpper(strings.TrimSpace(entry.Word))
		if _, ok := acceptedSet[w]; !ok {
			t.Errorf("word in words.json not found in accepted_words.txt: %s", w)
		}
	}
}

func TestAllWordsHaveHints(t *testing.T) {
	f, err := os.Open("data/words.json")
	if err != nil {
		t.Fatalf("failed to open words.json: %v", err)
	}
	defer f.Close()
	var wordList struct {
		Words []struct {
			Word string `json:"word"`
			Hint string `json:"hint"`
		}
	}
	if err := json.NewDecoder(f).Decode(&wordList); err != nil {
		t.Fatalf("failed to decode words.json: %v", err)
	}
	for _, entry := range wordList.Words {
		w := strings.ToUpper(strings.TrimSpace(entry.Word))
		hint := strings.TrimSpace(entry.Hint)
		if hint == "" {
			t.Errorf("word in words.json missing hint: %s", w)
		}
	}
}

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

func TestValidateGameState(t *testing.T) {
	app := &App{}
	game := &GameState{GameOver: true}
	err := app.validateGameState(nil, game)
	if err == nil {
		t.Errorf("validateGameState should return error if game is over")
	}
	game.GameOver = false
	err2 := app.validateGameState(nil, game)
	if err2 != nil {
		t.Errorf("validateGameState should not return error if game is not over")
	}
}

func TestGetEnvDuration(t *testing.T) {
	os.Setenv("TEST_DURATION", "3s")
	d := getEnvDuration("TEST_DURATION", 5*time.Second)
	if d != 3*time.Second {
		t.Errorf("getEnvDuration did not parse duration correctly")
	}
	os.Setenv("TEST_DURATION", "bad")
	d2 := getEnvDuration("TEST_DURATION", 7*time.Second)
	if d2 != 7*time.Second {
		t.Errorf("getEnvDuration fallback not used on bad input")
	}
	os.Unsetenv("TEST_DURATION")
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	i := getEnvInt("TEST_INT", 5)
	if i != 42 {
		t.Errorf("getEnvInt did not parse int correctly")
	}
	os.Setenv("TEST_INT", "bad")
	i2 := getEnvInt("TEST_INT", 7)
	if i2 != 7 {
		t.Errorf("getEnvInt fallback not used on bad input")
	}
	os.Unsetenv("TEST_INT")
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
