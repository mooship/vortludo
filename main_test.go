package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// loadWordListFromFile loads a list of words from a file, trims whitespace, and uppercases them.
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

// TestAcceptedWordsNoDuplicates ensures there are no duplicate words in accepted_words.txt.
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

// TestWordsNoDuplicates checks for duplicate words in words.json.
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

// TestAllWordsInAcceptedList ensures every word in words.json is present in accepted_words.txt.
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

// TestAllWordsHaveHints checks that every word in words.json has a non-empty hint.
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

// TestNormalizeGuess verifies that normalizeGuess trims and uppercases input correctly.
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

// TestCheckGuess checks that checkGuess returns correct letter statuses.
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

// TestBuildHintMap ensures buildHintMap creates a correct word-to-hint mapping.
func TestBuildHintMap(t *testing.T) {
	words := []WordEntry{{Word: "APPLE", Hint: "A fruit"}, {Word: "BERRY", Hint: "Another fruit"}}
	m := buildHintMap(words)
	if m["APPLE"] != "A fruit" || m["BERRY"] != "Another fruit" {
		t.Errorf("buildHintMap failed: got %v", m)
	}
}

// TestIsValidWord checks isValidWord returns true for valid words and false otherwise.
func TestIsValidWord(t *testing.T) {
	app := &App{WordSet: map[string]struct{}{"APPLE": {}, "BERRY": {}}}
	if !app.isValidWord("APPLE") || app.isValidWord("PEACH") {
		t.Errorf("isValidWord logic error")
	}
}

// TestIsAcceptedWord checks isAcceptedWord returns true for accepted words and false otherwise.
func TestIsAcceptedWord(t *testing.T) {
	app := &App{AcceptedWordSet: map[string]struct{}{"APPLE": {}, "BERRY": {}}}
	if !app.isAcceptedWord("BERRY") || app.isAcceptedWord("PEACH") {
		t.Errorf("isAcceptedWord logic error")
	}
}

// TestFormatUptime verifies formatUptime returns human-readable durations.
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

// TestValidateGameState checks validateGameState returns error if game is over.
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

// TestGetEnvDuration checks getEnvDuration parses durations and falls back on bad input.
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

// TestGetEnvInt checks getEnvInt parses integers and falls back on bad input.
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

// TestPlural checks plural returns "s" for values not equal to 1.
func TestPlural(t *testing.T) {
	if plural(1) != "" || plural(2) != "s" {
		t.Errorf("plural logic error")
	}
}

// TestCreateNewGame checks createNewGame initializes a new game session correctly.
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

// TestGetRandomWordEntry checks getRandomWordEntry returns a valid word from the list.
func TestGetRandomWordEntry(t *testing.T) {
	app := &App{WordList: []WordEntry{{Word: "APPLE", Hint: "A fruit"}, {Word: "BERRY", Hint: "Another fruit"}}}
	ctx := context.Background()
	entry := app.getRandomWordEntry(ctx)
	if entry.Word != "APPLE" && entry.Word != "BERRY" {
		t.Errorf("getRandomWordEntry returned unexpected word: %v", entry.Word)
	}
}

// TestGetHintForWord checks getHintForWord returns the correct hint or empty string.
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

// TestGetTargetWord checks getTargetWord returns the session word or assigns a new one.
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

// TestUpdateGameStateWinLose checks updateGameState sets win/lose flags correctly.
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

// TestSaveGameStateAndGetGameState checks saving and retrieving game state works.
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

// TestIsValidWordAndIsAcceptedWord checks both isValidWord and isAcceptedWord logic.
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

// TestDirExists checks directory existence and error handling.
func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	if !dirExists(dir) {
		t.Errorf("dirExists should return true for existing directory")
	}
	if dirExists(dir + "_doesnotexist") {
		t.Errorf("dirExists should return false for non-existent directory")
	}
	file := dir + "/file.txt"
	os.WriteFile(file, []byte("test"), 0644)
	if dirExists(file) {
		t.Errorf("dirExists should return false for a file, not a directory")
	}
}

// TestGetLimiter checks rate limiter creation and cache behavior.
func TestGetLimiter(t *testing.T) {
	app := &App{
		LimiterMap:     make(map[string]*rate.Limiter),
		LimiterMutex:   sync.Mutex{},
		RateLimitRPS:   2,
		RateLimitBurst: 3,
	}
	lim1 := app.getLimiter("127.0.0.1")
	lim2 := app.getLimiter("127.0.0.1")
	if lim1 != lim2 {
		t.Errorf("getLimiter should return the same limiter for the same key")
	}
	lim3 := app.getLimiter("other")
	if lim3 == lim1 {
		t.Errorf("getLimiter should return different limiters for different keys")
	}
}

// TestRequestIDMiddleware checks that the middleware injects a request ID into context and header.
func TestRequestIDMiddleware(t *testing.T) {
	r := gin.New()
	r.Use(requestIDMiddleware())
	r.GET("/", func(c *gin.Context) {
		id := c.Request.Context().Value(requestIDKey)
		if id == nil {
			t.Errorf("requestIDMiddleware did not set request_id in context")
		}
		c.String(200, "ok")
	})
	w := performRequest(r, "GET", "/", nil, nil)
	if w.Header().Get("X-Request-Id") == "" {
		t.Errorf("requestIDMiddleware did not set X-Request-Id header")
	}
}

// performRequest is a test helper to execute HTTP requests against a handler.
func performRequest(r http.Handler, method, path string, body *strings.Reader, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, http.NoBody)
	} else {
		req = httptest.NewRequest(method, path, body)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(w, req)
	return w
}

// TestLogHelpers ensures the log helpers do not panic.
func TestLogHelpers(t *testing.T) {
	logInfo("info: %s", "test")
	logWarn("warn: %s", "test")
}
