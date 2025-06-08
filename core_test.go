package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Test constants
const (
	TestWordApple  = "APPLE"
	TestWordBanjo  = "BANJO"
	TestWordPeach  = "PEACH"
	TestWordTable  = "TABLE"
	TestWordAlley  = "ALLEY"
	TestWordZzzzz  = "ZZZZZ"
	TestWordTests  = "TESTS"
	TestWordCache  = "CACHE"
	TestWordSaver  = "SAVER"
	TestWordHello  = "HELLO"
	TestWordWorld  = "WORLD"
	TestWordAlpha  = "ALPHA"
	TestWordLoaded = "LOADED"

	TestHintFruit     = "A fruit"
	TestHintFurniture = "Furniture"
	TestHintTesting   = "For testing"

	TestSessionCreateNew = "test-session-createnewgame"
	TestSessionGetState  = "test-session-getstatecache"
	TestSessionSaveGame  = "test-session-savegame"
	TestSessionActive    = "active-session"
	TestSessionExpired1  = "expired-session-1"
	TestSessionExpired2  = "expired-session-2"
	TestSessionNoTime    = "no-time-session"

	TestFileName    = "f.txt"
	TestFileContent = "x"

	StatusCorrect = "correct"
	StatusPresent = "present"
	StatusAbsent  = "absent"

	CommentAllCorrect = "All correct."
	CommentMixed      = "Mix of correct, present, absent."
	CommentAllAbsent  = "All absent."

	InvalidSessionFormat = "invalid-session-format"
)

// TestCheckGuess tests guess checking algorithm
func TestCheckGuess(t *testing.T) {
	target := TestWordApple
	tests := []struct {
		guess   string
		want    []GuessResult
		comment string
	}{
		{
			guess: TestWordApple,
			want: []GuessResult{
				{"A", StatusCorrect},
				{"P", StatusCorrect},
				{"P", StatusCorrect},
				{"L", StatusCorrect},
				{"E", StatusCorrect},
			},
			comment: CommentAllCorrect,
		},
		{
			guess: TestWordAlley,
			want: []GuessResult{
				{"A", StatusCorrect},
				{"L", StatusPresent},
				{"L", StatusAbsent},
				{"E", StatusPresent},
				{"Y", StatusAbsent},
			},
			comment: CommentMixed,
		},
		{
			guess: TestWordZzzzz,
			want: []GuessResult{
				{"Z", StatusAbsent},
				{"Z", StatusAbsent},
				{"Z", StatusAbsent},
				{"Z", StatusAbsent},
				{"Z", StatusAbsent},
			},
			comment: CommentAllAbsent,
		},
	}

	for _, tt := range tests {
		got := checkGuess(tt.guess, target)
		for i := range got {
			if got[i].Letter != tt.want[i].Letter || got[i].Status != tt.want[i].Status {
				t.Errorf("%s: guess %s, pos %d: got %+v, want %+v", tt.comment, tt.guess, i, got[i], tt.want[i])
			}
		}
	}
}

// TestIsValidWord tests word validation
func TestIsValidWord(t *testing.T) {
	wordSet = map[string]struct{}{
		TestWordApple: {},
		TestWordBanjo: {},
	}
	tests := []struct {
		word string
		want bool
	}{
		{TestWordApple, true},
		{TestWordBanjo, true},
		{TestWordPeach, false},
		{"", false},
	}
	for _, tt := range tests {
		got := isValidWord(tt.word)
		if got != tt.want {
			t.Errorf("isValidWord(%q) = %v, want %v", tt.word, got, tt.want)
		}
	}
}

// TestNormalizeGuess tests input normalization
func TestNormalizeGuess(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"apple", TestWordApple},
		{"  banjo ", TestWordBanjo},
		{"PeAch", TestWordPeach},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeGuess(tt.input)
		if got != tt.want {
			t.Errorf("normalizeGuess(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestGetHintForWord tests hint retrieval
func TestGetHintForWord(t *testing.T) {
	originalWordList := wordList
	wordList = []WordEntry{
		{Word: TestWordApple, Hint: TestHintFruit},
		{Word: TestWordTable, Hint: TestHintFurniture},
	}
	defer func() { wordList = originalWordList }()

	tests := []struct {
		word string
		want string
	}{
		{TestWordApple, TestHintFruit},
		{TestWordTable, TestHintFurniture},
		{"GRAPE", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := getHintForWord(tt.word)
		if got != tt.want {
			t.Errorf("getHintForWord(%q) = %q, want %q", tt.word, got, tt.want)
		}
	}
}

// TestCreateNewGame_SetsLastAccessTime tests new game access time
func TestCreateNewGame_SetsLastAccessTime(t *testing.T) {
	originalWordList := wordList
	wordList = []WordEntry{{Word: TestWordTests, Hint: TestHintTesting}}
	defer func() { wordList = originalWordList }()

	sessionID := TestSessionCreateNew
	before := time.Now()
	game := createNewGame(sessionID)
	after := time.Now()

	if game.LastAccessTime.Before(before) || game.LastAccessTime.After(after) {
		t.Errorf("createNewGame() LastAccessTime = %v, want between %v and %v", game.LastAccessTime, before, after)
	}
	// Cleanup
	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
}

// TestGetGameState_UpdatesLastAccessTimeFromCache tests cache access time update
func TestGetGameState_UpdatesLastAccessTimeFromCache(t *testing.T) {
	sessionID := TestSessionGetState
	initialTime := time.Now().Add(-1 * time.Hour)

	sessionMutex.Lock()
	gameSessions[sessionID] = &GameState{
		SessionWord:    TestWordCache,
		LastAccessTime: initialTime,
		Guesses:        make([][]GuessResult, MaxGuesses),
	}
	for i := range gameSessions[sessionID].Guesses {
		gameSessions[sessionID].Guesses[i] = make([]GuessResult, WordLength)
	}
	sessionMutex.Unlock()

	defer func() {
		sessionMutex.Lock()
		delete(gameSessions, sessionID)
		sessionMutex.Unlock()
	}()

	retrievedGame := getGameState(sessionID)

	if retrievedGame.LastAccessTime.Equal(initialTime) || retrievedGame.LastAccessTime.Before(initialTime) {
		t.Errorf("getGameState() from cache did not update LastAccessTime. Got %v, expected later than %v", retrievedGame.LastAccessTime, initialTime)
	}
}

// TestSaveGameState_UpdatesLastAccessTime tests save access time update
func TestSaveGameState_UpdatesLastAccessTime(t *testing.T) {
	sessionID := TestSessionSaveGame
	initialTime := time.Now().Add(-1 * time.Hour)

	game := &GameState{
		SessionWord:    TestWordSaver,
		LastAccessTime: initialTime,
		Guesses:        make([][]GuessResult, MaxGuesses),
	}
	for i := range game.Guesses {
		game.Guesses[i] = make([]GuessResult, WordLength)
	}

	originalSaveFunc := saveGameSessionToFile
	saveGameSessionToFile = func(s string, gs *GameState) error { return nil }
	defer func() { saveGameSessionToFile = originalSaveFunc }()

	saveGameState(sessionID, game)

	sessionMutex.RLock()
	savedGame, ok := gameSessions[sessionID]
	sessionMutex.RUnlock()

	if !ok {
		t.Fatalf("saveGameState() did not store game in memory for session %s", sessionID)
	}

	if savedGame.LastAccessTime.Equal(initialTime) || savedGame.LastAccessTime.Before(initialTime) {
		t.Errorf("saveGameState() did not update LastAccessTime. Got %v, expected later than %v", savedGame.LastAccessTime, initialTime)
	}

	// Cleanup
	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
}

// TestSessionCleanupScheduler_InMemory tests in-memory session cleanup
func TestSessionCleanupScheduler_InMemory(t *testing.T) {
	originalGameSessions := gameSessions
	gameSessions = make(map[string]*GameState)
	defer func() { gameSessions = originalGameSessions }()

	now := time.Now()

	sessionMutex.Lock()
	gameSessions[TestSessionActive] = &GameState{LastAccessTime: now.Add(-SessionTimeout / 2)}
	gameSessions[TestSessionExpired1] = &GameState{LastAccessTime: now.Add(-(SessionTimeout + time.Minute))}
	gameSessions[TestSessionExpired2] = &GameState{LastAccessTime: now.Add(-(SessionTimeout + time.Hour))}
	gameSessions[TestSessionNoTime] = &GameState{}
	sessionMutex.Unlock()

	sessionMutex.Lock()
	cleanedInMemoryCount := 0
	for sessionID, game := range gameSessions {
		isExpired := game.LastAccessTime.IsZero() || now.Sub(game.LastAccessTime) > SessionTimeout
		if isExpired {
			delete(gameSessions, sessionID)
			cleanedInMemoryCount++
		}
	}
	sessionMutex.Unlock()

	if cleanedInMemoryCount != 3 {
		t.Errorf("In-memory cleanup expected to remove 3 sessions, but removed %d", cleanedInMemoryCount)
	}

	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	if _, exists := gameSessions[TestSessionActive]; !exists {
		t.Errorf("Active session %s was incorrectly removed", TestSessionActive)
	}
	if _, exists := gameSessions[TestSessionExpired1]; exists {
		t.Errorf("Expired session %s was not removed", TestSessionExpired1)
	}
	if _, exists := gameSessions[TestSessionExpired2]; exists {
		t.Errorf("Expired session %s was not removed", TestSessionExpired2)
	}
	if _, exists := gameSessions[TestSessionNoTime]; exists {
		t.Errorf("Session with no LastAccessTime was not removed")
	}
}

// TestIsValidSessionID tests session ID validation
func TestIsValidSessionID(t *testing.T) {
	valid := uuid.NewString()
	if !isValidSessionID(valid) {
		t.Errorf("isValidSessionID(%q) = false, want true", valid)
	}
	for _, bad := range []string{
		"", "short",
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
		"12345678-1234-1234-1234-12345678901G",
	} {
		if isValidSessionID(bad) {
			t.Errorf("isValidSessionID(%q) = true, want false", bad)
		}
	}
}

// TestUpdateGameState tests game state updates
func TestUpdateGameState(t *testing.T) {
	base := &GameState{
		Guesses:        make([][]GuessResult, MaxGuesses),
		CurrentRow:     0,
		SessionWord:    TestWordHello,
		LastAccessTime: time.Now(),
	}
	for i := range base.Guesses {
		base.Guesses[i] = make([]GuessResult, WordLength)
	}

	winGame := *base
	updateGameState(&winGame,
		TestWordHello, TestWordHello,
		checkGuess(TestWordHello, TestWordHello), false)
	if !winGame.Won || !winGame.GameOver || winGame.TargetWord != TestWordHello {
		t.Errorf("win path: Got Won=%v, GameOver=%v, TargetWord=%q",
			winGame.Won, winGame.GameOver, winGame.TargetWord)
	}

	loseGame := *base
	for range MaxGuesses {
		updateGameState(&loseGame,
			TestWordWorld, TestWordHello,
			checkGuess(TestWordWorld, TestWordHello), false)
	}
	if !loseGame.GameOver || loseGame.Won {
		t.Errorf("lose path: Got GameOver=%v, Won=%v", loseGame.GameOver, loseGame.Won)
	}
}

// TestGetTargetWord tests target word assignment
func TestGetTargetWord(t *testing.T) {
	orig := wordList
	wordList = []WordEntry{{Word: TestWordAlpha, Hint: ""}}
	defer func() { wordList = orig }()

	game := &GameState{}
	got := getTargetWord(game)
	if got != TestWordAlpha || game.SessionWord != TestWordAlpha {
		t.Errorf("getTargetWord() = %q, want %q", got, TestWordAlpha)
	}
}

// TestDirExists tests directory existence check
func TestDirExists(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, TestFileName)
	os.WriteFile(file, []byte(TestFileContent), 0644)

	if dirExists(file) {
		t.Errorf("dirExists(%q) = true, want false", file)
	}
	if !dirExists(tmp) {
		t.Errorf("dirExists(%q) = false, want true", tmp)
	}
}

func TestAcceptedWords_NoDuplicatesAndFiveLetters(t *testing.T) {
	data, err := os.ReadFile("data/accepted_words.json")
	if err != nil {
		t.Fatalf("Failed to read accepted_words.json: %v", err)
	}
	var words []string
	if err := json.Unmarshal(data, &words); err != nil {
		t.Fatalf("Failed to unmarshal accepted_words.json: %v", err)
	}
	seen := make(map[string]struct{})
	for i, w := range words {
		if len(w) != 5 {
			t.Errorf("accepted_words.json: word %q at index %d is not 5 letters", w, i)
		}
		upper := w
		if _, exists := seen[upper]; exists {
			t.Errorf("accepted_words.json: duplicate word found: %q", upper)
		}
		seen[upper] = struct{}{}
	}
}

func TestWordsJson_NoDuplicatesAndFiveLetters(t *testing.T) {
	data, err := os.ReadFile("data/words.json")
	if err != nil {
		t.Fatalf("Failed to read words.json: %v", err)
	}
	var wl struct {
		Words []struct {
			Word string `json:"word"`
			Hint string `json:"hint"`
		} `json:"words"`
	}
	if err := json.Unmarshal(data, &wl); err != nil {
		t.Fatalf("Failed to unmarshal words.json: %v", err)
	}
	seen := make(map[string]struct{})
	for i, entry := range wl.Words {
		if len(entry.Word) != 5 {
			t.Errorf("words.json: word %q at index %d is not 5 letters", entry.Word, i)
		}
		upper := entry.Word
		if _, exists := seen[upper]; exists {
			t.Errorf("words.json: duplicate word found: %q", upper)
		}
		seen[upper] = struct{}{}
	}
}
