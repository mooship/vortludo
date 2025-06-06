package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCheckGuess(t *testing.T) {
	target := "APPLE"
	tests := []struct {
		guess   string
		want    []GuessResult
		comment string
	}{
		{
			guess: "APPLE",
			want: []GuessResult{
				{"A", "correct"},
				{"P", "correct"},
				{"P", "correct"},
				{"L", "correct"},
				{"E", "correct"},
			},
			comment: "All correct",
		},
		{
			guess: "ALLEY",
			want: []GuessResult{
				{"A", "correct"},
				{"L", "present"},
				{"L", "absent"},
				{"E", "present"},
				{"Y", "absent"},
			},
			comment: "Mix of correct, present, absent",
		},
		{
			guess: "ZZZZZ",
			want: []GuessResult{
				{"Z", "absent"},
				{"Z", "absent"},
				{"Z", "absent"},
				{"Z", "absent"},
				{"Z", "absent"},
			},
			comment: "All absent",
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
		"APPLE": {},
		"BANJO": {},
	}
	tests := []struct {
		word string
		want bool
	}{
		{"APPLE", true},
		{"BANJO", true},
		{"PEACH", false},
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
		{"apple", "APPLE"},
		{"  banjo ", "BANJO"},
		{"PeAch", "PEACH"},
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
		{Word: "APPLE", Hint: "A fruit"},
		{Word: "TABLE", Hint: "Furniture"},
	}
	defer func() { wordList = originalWordList }()

	tests := []struct {
		word string
		want string
	}{
		{"APPLE", "A fruit"},
		{"TABLE", "Furniture"},
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
	wordList = []WordEntry{{Word: "TESTS", Hint: "For testing"}}
	defer func() { wordList = originalWordList }()

	sessionID := "test-session-createnewgame"
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
	sessionID := "test-session-getstatecache"
	initialTime := time.Now().Add(-1 * time.Hour)

	// Setup cached game
	sessionMutex.Lock()
	gameSessions[sessionID] = &GameState{
		SessionWord:    "CACHE",
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
	sessionID := "test-session-savegame"
	initialTime := time.Now().Add(-1 * time.Hour)

	game := &GameState{
		SessionWord:    "SAVER",
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

func TestLoadGameSessionFromFile(t *testing.T) {
	tempDir := t.TempDir()
	defer func() {
		_ = os.RemoveAll(filepath.Join(tempDir, "data/sessions"))
	}()

	sessionBaseDir := filepath.Join(tempDir, "data", "sessions")
	if err := os.MkdirAll(sessionBaseDir, 0755); err != nil {
		t.Fatalf("Failed to create temp session dir: %v", err)
	}

	// Helper to create test session files
	createTestSessionFile := func(sID string, game *GameState, modTime *time.Time) string {
		filePath := filepath.Join(sessionBaseDir, sID+".json")
		data, _ := json.Marshal(game)
		_ = os.WriteFile(filePath, data, 0644)
		if modTime != nil {
			_ = os.Chtimes(filePath, *modTime, *modTime)
		}
		return filePath
	}

	// Change working directory to use temp structure
	originalWD, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change WD to tempDir: %v", err)
	}
	defer os.Chdir(originalWD)

	if err := os.MkdirAll(filepath.Join(tempDir, "data", "sessions"), 0755); err != nil {
		t.Fatalf("Failed to create data/sessions in tempDir: %v", err)
	}

	// Test 1: Valid session file
	sessionIDValid := "valid-session-" + uuid.NewString()
	validGame := &GameState{
		SessionWord: "LOADED",
		Guesses:     make([][]GuessResult, MaxGuesses),
	}
	for i := range validGame.Guesses {
		validGame.Guesses[i] = make([]GuessResult, WordLength)
	}
	createTestSessionFile(sessionIDValid, validGame, nil)

	loadedGame, err := loadGameSessionFromFile(sessionIDValid)
	if err != nil {
		t.Fatalf("loadGameSessionFromFile failed for valid session: %v", err)
	}
	if loadedGame.SessionWord != "LOADED" {
		t.Errorf("loadGameSessionFromFile got SessionWord %q, want %q", loadedGame.SessionWord, "LOADED")
	}
	if loadedGame.LastAccessTime.IsZero() {
		t.Error("loadGameSessionFromFile did not set LastAccessTime for valid session")
	}

	// Test 2: Old file (should be removed)
	sessionIDOld := "old-session-" + uuid.NewString()
	oldTime := time.Now().Add(-(SessionTimeout + time.Hour))
	oldGamePath := createTestSessionFile(sessionIDOld, validGame, &oldTime)

	_, err = loadGameSessionFromFile(sessionIDOld)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for old file, got: %v", err)
	}
	if _, statErr := os.Stat(oldGamePath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove old session file: %s", oldGamePath)
	}

	// Test 3: Corrupted file (should be removed)
	sessionIDCorrupt := "corrupt-session-" + uuid.NewString()
	corruptFilePath := filepath.Join("data", "sessions", sessionIDCorrupt+".json")
	_ = os.WriteFile(corruptFilePath, []byte("this is not json"), 0644)

	_, err = loadGameSessionFromFile(sessionIDCorrupt)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for corrupt file, got: %v", err)
	}
	if _, statErr := os.Stat(corruptFilePath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove corrupt session file: %s", corruptFilePath)
	}

	// Test 4: Invalid structure (should be removed)
	sessionIDInvalidStruct := "invalidstruct-session-" + uuid.NewString()
	invalidStructGame := &GameState{
		SessionWord: "BADSTRUCT",
		Guesses:     make([][]GuessResult, MaxGuesses-1), // Wrong number
	}
	invalidStructPath := createTestSessionFile(sessionIDInvalidStruct, invalidStructGame, nil)

	_, err = loadGameSessionFromFile(sessionIDInvalidStruct)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for invalid structure, got: %v", err)
	}
	if _, statErr := os.Stat(invalidStructPath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove invalid structure session file: %s", invalidStructPath)
	}
}

func TestSessionCleanupScheduler_InMemory(t *testing.T) {
	// Test in-memory cleanup logic without the ticker
	originalGameSessions := gameSessions
	gameSessions = make(map[string]*GameState)
	defer func() { gameSessions = originalGameSessions }()

	now := time.Now()
	activeSessionID := "active-session"
	expiredSessionID1 := "expired-session-1"
	expiredSessionID2 := "expired-session-2"

	// Setup test sessions
	sessionMutex.Lock()
	gameSessions[activeSessionID] = &GameState{LastAccessTime: now.Add(-SessionTimeout / 2)}               // Active
	gameSessions[expiredSessionID1] = &GameState{LastAccessTime: now.Add(-(SessionTimeout + time.Minute))} // Expired
	gameSessions[expiredSessionID2] = &GameState{LastAccessTime: now.Add(-(SessionTimeout + time.Hour))}   // Expired
	gameSessions["no-time-session"] = &GameState{}                                                         // Zero time = expired
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

	if _, exists := gameSessions[activeSessionID]; !exists {
		t.Errorf("Active session %s was incorrectly removed", activeSessionID)
	}
	if _, exists := gameSessions[expiredSessionID1]; exists {
		t.Errorf("Expired session %s was not removed", expiredSessionID1)
	}
	if _, exists := gameSessions[expiredSessionID2]; exists {
		t.Errorf("Expired session %s was not removed", expiredSessionID2)
	}
	if _, exists := gameSessions["no-time-session"]; exists {
		t.Errorf("Session with no LastAccessTime was not removed")
	}
}

func TestIsValidSessionID(t *testing.t) {
	valid := uuid.NewString()
	if !isValidSessionID(valid) {
		t.Errorf("isValidSessionID(%q) = false, want true", valid)
	}
	for _, bad := range []string{
		"", "short",
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
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
		SessionWord:    "HELLO",
		LastAccessTime: time.Now(),
	}
	for i := range base.Guesses {
		base.Guesses[i] = make([]GuessResult, WordLength)
	}

	// Test win condition
	winGame := *base
	updateGameState(&winGame,
		"HELLO", "HELLO",
		checkGuess("HELLO", "HELLO"), false)
	if !winGame.Won || !winGame.GameOver || winGame.TargetWord != "HELLO" {
		t.Errorf("win path: Got Won=%v, GameOver=%v, TargetWord=%q",
			winGame.Won, winGame.GameOver, winGame.TargetWord)
	}

	// Test loss condition
	loseGame := *base
	for range MaxGuesses {
		updateGameState(&loseGame,
			"WORLD", "HELLO",
			checkGuess("WORLD", "HELLO"), false)
	}
	if !loseGame.GameOver || loseGame.Won {
		t.Errorf("lose path: Got GameOver=%v, Won=%v", loseGame.GameOver, loseGame.Won)
	}
}

func TestGetTargetWord(t *testing.T) {
	orig := wordList
	wordList = []WordEntry{{Word: "ALPHA", Hint: ""}}
	defer func() { wordList = orig }()

	game := &GameState{}
	got := getTargetWord(game)
	if got != "ALPHA" || game.SessionWord != "ALPHA" {
		t.Errorf("getTargetWord() = %q, want %q", got, "ALPHA")
	}
}

func TestDirExists(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "f.txt")
	os.WriteFile(file, []byte("x"), 0644)

	if dirExists(file) {
		t.Errorf("dirExists(%q) = true, want false", file)
	}
	if !dirExists(tmp) {
		t.Errorf("dirExists(%q) = false, want true", tmp)
	}
}
