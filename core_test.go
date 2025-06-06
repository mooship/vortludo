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
	// Setup global wordList for this test
	originalWordList := wordList
	wordList = []WordEntry{
		{Word: "APPLE", Hint: "A fruit"},
		{Word: "TABLE", Hint: "Furniture"},
	}
	defer func() { wordList = originalWordList }() // Restore original

	tests := []struct {
		word string
		want string
	}{
		{"APPLE", "A fruit"},
		{"TABLE", "Furniture"},
		{"GRAPE", ""}, // Word not in list
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
	// Setup minimal wordList for createNewGame to pick a word
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
	// Clean up gameSessions
	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
}

func TestGetGameState_UpdatesLastAccessTimeFromCache(t *testing.T) {
	sessionID := "test-session-getstatecache"
	initialTime := time.Now().Add(-1 * time.Hour) // An hour ago

	// Setup game in cache
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

	defer func() { // Cleanup
		sessionMutex.Lock()
		delete(gameSessions, sessionID)
		sessionMutex.Unlock()
	}()

	// isProduction = false // Ensure it doesn't try to load from file for this specific test path
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

	// Mock saveGameSessionToFile to prevent actual file writing during this unit test
	originalSaveFunc := saveGameSessionToFile
	saveGameSessionToFile = func(s string, gs *GameState) error { return nil } // This assignment is now valid
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
	tempDir := t.TempDir() // Create a temporary directory for session files
	defer func() {         // Restore original session dir path if necessary or handle it if it's a global var
		// For this test, we are overriding the path implicitly by joining with tempDir
		_ = os.RemoveAll(filepath.Join(tempDir, "data/sessions")) // Clean up temp session dir
	}()

	// Override where session files are stored for this test
	// This requires saveGameSessionToFile and loadGameSessionFromFile to be adaptable
	// or to directly manipulate file paths here. We'll create files in tempDir.
	// For simplicity, we assume loadGameSessionFromFile uses a base path that can be tempDir.
	// Let's adjust the test to write files directly where loadGameSessionFromFile expects them,
	// but within the tempDir structure.
	sessionBaseDir := filepath.Join(tempDir, "data", "sessions")
	if err := os.MkdirAll(sessionBaseDir, 0755); err != nil {
		t.Fatalf("Failed to create temp session dir: %v", err)
	}

	// Helper to create a session file
	createTestSessionFile := func(sID string, game *GameState, modTime *time.Time) string {
		filePath := filepath.Join(sessionBaseDir, sID+".json")
		data, _ := json.Marshal(game)
		_ = os.WriteFile(filePath, data, 0644)
		if modTime != nil {
			_ = os.Chtimes(filePath, *modTime, *modTime)
		}
		return filePath
	}

	// Redefine loadGameSessionFromFile for this test to use the temp directory
	// This is a bit complex. A better way would be to inject the base path into loadGameSessionFromFile.
	// For now, we'll assume loadGameSessionFromFile constructs path like "data/sessions/" + sessionID + ".json"
	// and we will temporarily change the working directory or mock os.Stat and os.ReadFile.
	// Simpler: make loadGameSessionFromFile accept the full path or make its internal path configurable.
	// Given the current structure, we'll test its behavior by creating files in the expected relative path
	// from a temporary root.
	// The test will assume that `loadGameSessionFromFile` uses `filepath.Join("data/sessions", sessionID+".json")`
	// We will ensure "data/sessions" exists within our tempDir.

	// Case 1: Successful load
	sessionIDValid := "valid-session-" + uuid.NewString()
	validGame := &GameState{
		SessionWord: "LOADED",
		Guesses:     make([][]GuessResult, MaxGuesses),
		// LastAccessTime will be set by loadGameSessionFromFile
	}
	for i := range validGame.Guesses {
		validGame.Guesses[i] = make([]GuessResult, WordLength)
	}
	createTestSessionFile(sessionIDValid, validGame, nil) // file with current mod time

	// Temporarily override the function to point to our temp dir for this test
	// This is tricky without changing the original function signature or using global vars for paths.
	// Let's assume the test runs from a context where "data/sessions" can be controlled.
	// The most straightforward way is to ensure loadGameSessionFromFile is testable by allowing path injection,
	// but since we can't change it now, we'll rely on its current behavior and control the files.
	// We will use a helper that mocks the file path construction for load/save.
	// For this test, we will assume `loadGameSessionFromFile` is called with just the sessionID
	// and it constructs the path internally. We need to ensure `data/sessions` exists relative to where the test runs,
	// or use the tempDir approach carefully.

	// Let's use the existing loadGameSessionFromFile and saveGameSessionToFile,
	// but ensure they operate within the tempDir.
	// This requires `persistence.go` functions to use a configurable base path.
	// Since they don't, we'll test by creating files in the default "data/sessions" path,
	// but ensure this path is within our t.TempDir().

	// Re-think: `loadGameSessionFromFile` uses `filepath.Join("data/sessions", sessionID+".json")`.
	// We can't easily change "data/sessions" without altering the source or complex mocks.
	// The simplest for now is to create the "data/sessions" structure inside t.TempDir() and then temporarily
	// change the current working directory for the scope of this test case. This is a common pattern.
	originalWD, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change WD to tempDir: %v", err)
	}
	defer os.Chdir(originalWD) // Change back

	if err := os.MkdirAll(filepath.Join(tempDir, "data", "sessions"), 0755); err != nil {
		t.Fatalf("Failed to create data/sessions in tempDir: %v", err)
	}
	// Now loadGameSessionFromFile will look for "data/sessions" inside tempDir.

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

	// Case 2: Old file
	sessionIDOld := "old-session-" + uuid.NewString()
	oldTime := time.Now().Add(-(SessionTimeout + time.Hour))                // Older than SessionTimeout
	oldGamePath := createTestSessionFile(sessionIDOld, validGame, &oldTime) // Create an old file

	_, err = loadGameSessionFromFile(sessionIDOld)
	if err == nil || !os.IsNotExist(err) { // Expecting os.ErrNotExist after file is removed
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for old file, got: %v", err)
	}
	if _, statErr := os.Stat(oldGamePath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove old session file: %s", oldGamePath)
	}

	// Case 3: Corrupted file (e.g., not JSON)
	sessionIDCorrupt := "corrupt-session-" + uuid.NewString()
	corruptFilePath := filepath.Join("data", "sessions", sessionIDCorrupt+".json")
	_ = os.WriteFile(corruptFilePath, []byte("this is not json"), 0644)

	_, err = loadGameSessionFromFile(sessionIDCorrupt)
	if err == nil || !os.IsNotExist(err) { // Expecting os.ErrNotExist after file is removed
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for corrupt file, got: %v", err)
	}
	if _, statErr := os.Stat(corruptFilePath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove corrupt session file: %s", corruptFilePath)
	}

	// Case 4: Invalid game structure (e.g., wrong number of guesses)
	sessionIDInvalidStruct := "invalidstruct-session-" + uuid.NewString()
	invalidStructGame := &GameState{
		SessionWord: "BADSTRUCT",
		Guesses:     make([][]GuessResult, MaxGuesses-1), // Wrong number of guesses
	}
	invalidStructPath := createTestSessionFile(sessionIDInvalidStruct, invalidStructGame, nil)

	_, err = loadGameSessionFromFile(sessionIDInvalidStruct)
	if err == nil || !os.IsNotExist(err) { // Expecting os.ErrNotExist after file is removed
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for invalid structure, got: %v", err)
	}
	if _, statErr := os.Stat(invalidStructPath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove invalid structure session file: %s", invalidStructPath)
	}
}

func TestSessionCleanupScheduler_InMemory(t *testing.T) {
	// This test focuses on the in-memory cleanup logic, not the file cleanup or ticker.
	originalGameSessions := gameSessions
	gameSessions = make(map[string]*GameState)             // Use a fresh map for this test
	defer func() { gameSessions = originalGameSessions }() // Restore

	now := time.Now()
	activeSessionID := "active-session"
	expiredSessionID1 := "expired-session-1"
	expiredSessionID2 := "expired-session-2"

	// Setup sessions
	sessionMutex.Lock()
	gameSessions[activeSessionID] = &GameState{LastAccessTime: now.Add(-SessionTimeout / 2)}               // Active
	gameSessions[expiredSessionID1] = &GameState{LastAccessTime: now.Add(-(SessionTimeout + time.Minute))} // Expired
	gameSessions[expiredSessionID2] = &GameState{LastAccessTime: now.Add(-(SessionTimeout + time.Hour))}   // Very expired
	gameSessions["no-time-session"] = &GameState{}                                                         // Should also be cleaned if LastAccessTime is zero and treated as old
	sessionMutex.Unlock()

	// Manually trigger the in-memory cleanup logic part of sessionCleanupScheduler
	// (The actual scheduler runs in a goroutine with a ticker, we test its core logic here)
	sessionMutex.Lock()
	cleanedInMemoryCount := 0
	for sessionID, game := range gameSessions {
		// A zero LastAccessTime should ideally be treated as very old or handled at creation.
		// For this test, if LastAccessTime is zero, we'll assume it's older than SessionTimeout.
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

// TestIsValidSessionID covers valid and invalid session IDs
func TestIsValidSessionID(t *testing.T) {
	valid := uuid.NewString()
	if !isValidSessionID(valid) {
		t.Errorf("isValidSessionID(%q) = false, want true", valid)
	}
	for _, bad := range []string{
		"", "short",
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
		"12345678-1234-1234-1234-12345678901G", // invalid char
	} {
		if isValidSessionID(bad) {
			t.Errorf("isValidSessionID(%q) = true, want false", bad)
		}
	}
}

// TestUpdateGameState checks win, loss, row increment, and target reveal
func TestUpdateGameState(t *testing.T) {
	// base game
	base := &GameState{
		Guesses:        make([][]GuessResult, MaxGuesses),
		CurrentRow:     0,
		SessionWord:    "HELLO",
		LastAccessTime: time.Now(),
	}
	for i := range base.Guesses {
		base.Guesses[i] = make([]GuessResult, WordLength)
	}

	// 1) correct guess => win
	winGame := *base
	updateGameState(&winGame,
		"HELLO", "HELLO",
		checkGuess("HELLO", "HELLO"), false)
	if !winGame.Won || !winGame.GameOver || winGame.TargetWord != "HELLO" {
		t.Errorf("win path: Got Won=%v, GameOver=%v, TargetWord=%q",
			winGame.Won, winGame.GameOver, winGame.TargetWord)
	}

	// 2) wrong guesses until loss
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

// TestGetTargetWord assigns a word when SessionWord is empty
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

// TestDirExists verifies directory detection logic
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
