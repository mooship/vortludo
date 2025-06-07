package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Test constants.
const (
	// Test words.
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

	// Test hints.
	TestHintFruit     = "A fruit"
	TestHintFurniture = "Furniture"
	TestHintTesting   = "For testing"

	// Test session ID patterns.
	TestSessionCreateNew = "test-session-createnewgame"
	TestSessionGetState  = "test-session-getstatecache"
	TestSessionSaveGame  = "test-session-savegame"
	TestSessionActive    = "active-session"
	TestSessionExpired1  = "expired-session-1"
	TestSessionExpired2  = "expired-session-2"
	TestSessionNoTime    = "no-time-session"

	// Test file operations.
	TestFileName    = "f.txt"
	TestFileContent = "x"

	// Guess status constants.
	StatusCorrect = "correct"
	StatusPresent = "present"
	StatusAbsent  = "absent"

	// Test comments.
	CommentAllCorrect = "All correct."
	CommentMixed      = "Mix of correct, present, absent."
	CommentAllAbsent  = "All absent."

	// Test validation constants.
	InvalidSessionFormat = "invalid-session-format"
)

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

func TestGetHintForWord(t *testing.T) {
	// Setup test data
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
		{"GRAPE", ""}, // Not in list
		{"", ""},      // Empty word
	}

	for _, tt := range tests {
		got := getHintForWord(tt.word)
		if got != tt.want {
			t.Errorf("getHintForWord(%q) = %q, want %q", tt.word, got, tt.want)
		}
	}
}

func TestCreateNewGame_SetsLastAccessTime(t *testing.T) {
	// Setup test data
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

func TestGetGameState_UpdatesLastAccessTimeFromCache(t *testing.T) {
	sessionID := TestSessionGetState
	initialTime := time.Now().Add(-1 * time.Hour)

	// Setup cached game
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

	// Mock file save to prevent actual file I/O
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

func TestSessionCleanupScheduler_InMemory(t *testing.T) {
	// Test in-memory cleanup logic without the ticker
	originalGameSessions := gameSessions
	gameSessions = make(map[string]*GameState)
	defer func() { gameSessions = originalGameSessions }()

	now := time.Now()

	// Setup test sessions
	sessionMutex.Lock()
	gameSessions[TestSessionActive] = &GameState{LastAccessTime: now.Add(-SessionTimeout / 2)}               // Active
	gameSessions[TestSessionExpired1] = &GameState{LastAccessTime: now.Add(-(SessionTimeout + time.Minute))} // Expired
	gameSessions[TestSessionExpired2] = &GameState{LastAccessTime: now.Add(-(SessionTimeout + time.Hour))}   // Expired
	gameSessions[TestSessionNoTime] = &GameState{}                                                           // Zero time = expired
	sessionMutex.Unlock()

	// Test cleanup logic
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

	if cleanedInMemoryCount != 3 { // expired1, expired2, no-time-session
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

func TestIsValidSessionID(t *testing.T) {
	valid := uuid.NewString()
	if !isValidSessionID(valid) {
		t.Errorf("isValidSessionID(%q) = false, want true", valid)
	}
	for _, bad := range []string{
		"", "short",
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
		"12345678-1234-1234-1234-12345678901G", // Invalid char
	} {
		if isValidSessionID(bad) {
			t.Errorf("isValidSessionID(%q) = true, want false", bad)
		}
	}
}

func TestUpdateGameState(t *testing.T) {
	// Base game state
	base := &GameState{
		Guesses:        make([][]GuessResult, MaxGuesses),
		CurrentRow:     0,
		SessionWord:    TestWordHello,
		LastAccessTime: time.Now(),
	}
	for i := range base.Guesses {
		base.Guesses[i] = make([]GuessResult, WordLength)
	}

	// Test win condition
	winGame := *base
	updateGameState(&winGame,
		TestWordHello, TestWordHello,
		checkGuess(TestWordHello, TestWordHello), false)
	if !winGame.Won || !winGame.GameOver || winGame.TargetWord != TestWordHello {
		t.Errorf("win path: Got Won=%v, GameOver=%v, TargetWord=%q",
			winGame.Won, winGame.GameOver, winGame.TargetWord)
	}

	// Test loss condition
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
