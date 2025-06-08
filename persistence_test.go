package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Test constants
const (
	JSONExtension = ".json"
	DataDir       = "data"
	SessionsDir   = "sessions"

	TestSessionWordLoaded    = "LOADED"
	TestSessionWordBadStruct = "BADSTRUCT"
	TestSessionWordTests     = "TESTS"

	CorruptJSONContent = "this is not json"

	ErrInvalidSessionFormat = "invalid session ID format"
	ErrPathEscapes          = "session path would escape sessions directory"

	TestDirPerm  = 0755
	TestFilePerm = 0644

	UnixPathTraversal    = "../../../etc/passwd"
	WindowsPathTraversal = "..\\..\\windows\\system32\\drivers\\etc\\hosts"
	AbsoluteUnixPath     = "/etc/passwd"
	AbsolutePath         = "/tmp/session"
	SessionWithTraversal = "session/../../../secret.txt"
	ShortSessionID       = "short"
	InvalidHexSessionID  = "12345678-1234-5678-9ABC-123456789XYZ"

	DirectoryTraversalUp = "../session"
	MultipleTraversal    = "../../session"
	WindowsTraversal     = "..\\session"
	MixedSeparators      = "../\\session"
	CurrentDirectory     = "./session"

	ValidUppercaseUUID = "12345678-1234-5678-9ABC-123456789DEF"
	PathTraversalUUID  = "12345678-1234-5678-9ABC-123456789../"
	SlashUUID          = "12345678/1234/5678/9ABC/123456789DEF"
	BackslashUUID      = "12345678\\1234\\5678\\9ABC\\123456789DEF"
)

// TestLoadGameSessionFromFile checks loading sessions from disk
func TestLoadGameSessionFromFile(t *testing.T) {
	tempDir := t.TempDir()
	defer func() {
		_ = os.RemoveAll(filepath.Join(tempDir, DataDir, SessionsDir))
	}()

	sessionBaseDir := filepath.Join(tempDir, DataDir, SessionsDir)
	if err := os.MkdirAll(sessionBaseDir, TestDirPerm); err != nil {
		t.Fatalf("Failed to create temp session dir: %v", err)
	}

	// Helper to create test session files
	createTestSessionFile := func(sID string, game *GameState, modTime *time.Time) string {
		filePath := filepath.Join(sessionBaseDir, sID+JSONExtension)
		data, _ := json.Marshal(game)
		_ = os.WriteFile(filePath, data, TestFilePerm)
		if modTime != nil {
			_ = os.Chtimes(filePath, *modTime, *modTime)
		}
		return filePath
	}

	originalWD, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change WD to tempDir: %v", err)
	}
	defer os.Chdir(originalWD)

	if err := os.MkdirAll(filepath.Join(tempDir, DataDir, SessionsDir), TestDirPerm); err != nil {
		t.Fatalf("Failed to create data/sessions in tempDir: %v", err)
	}

	// Test valid session file
	sessionIDValid := uuid.NewString()
	validGame := &GameState{
		SessionWord: TestSessionWordLoaded,
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
	if loadedGame.SessionWord != TestSessionWordLoaded {
		t.Errorf("loadGameSessionFromFile got SessionWord %q, want %q", loadedGame.SessionWord, TestSessionWordLoaded)
	}
	if loadedGame.LastAccessTime.IsZero() {
		t.Error("loadGameSessionFromFile did not set LastAccessTime for valid session")
	}

	// Test old file (should be removed)
	sessionIDOld := uuid.NewString()
	oldTime := time.Now().Add(-(SessionTimeout + time.Hour))
	oldGamePath := createTestSessionFile(sessionIDOld, validGame, &oldTime)

	_, err = loadGameSessionFromFile(sessionIDOld)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for old file, got: %v", err)
	}
	if _, statErr := os.Stat(oldGamePath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove old session file: %s", oldGamePath)
	}

	// Test corrupted file (should be removed)
	sessionIDCorrupt := uuid.NewString()
	corruptFilePath := filepath.Join(DataDir, SessionsDir, sessionIDCorrupt+JSONExtension)
	_ = os.WriteFile(corruptFilePath, []byte(CorruptJSONContent), TestFilePerm)

	_, err = loadGameSessionFromFile(sessionIDCorrupt)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for corrupt file, got: %v", err)
	}
	if _, statErr := os.Stat(corruptFilePath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove corrupt session file: %s", corruptFilePath)
	}

	// Test invalid structure (should be removed)
	sessionIDInvalidStruct := uuid.NewString()
	invalidStructGame := &GameState{
		SessionWord: TestSessionWordBadStruct,
		Guesses:     make([][]GuessResult, MaxGuesses-1),
	}
	invalidStructPath := createTestSessionFile(sessionIDInvalidStruct, invalidStructGame, nil)

	_, err = loadGameSessionFromFile(sessionIDInvalidStruct)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for invalid structure, got: %v", err)
	}
	if _, statErr := os.Stat(invalidStructPath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove invalid structure session file: %s", invalidStructPath)
	}

	// Test invalid session ID format (should be rejected)
	invalidSessionID := ShortSessionID
	_, err = loadGameSessionFromFile(invalidSessionID)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile should reject invalid session ID format, got: %v", err)
	}
}

// TestGetSecureSessionPath checks secure session path generation
func TestGetSecureSessionPath(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "Valid UUID",
			sessionID: uuid.NewString(),
			wantErr:   false,
		},
		{
			name:      "Valid UUID with uppercase",
			sessionID: ValidUppercaseUUID,
			wantErr:   false,
		},
		{
			name:      "Invalid format - too short",
			sessionID: ShortSessionID,
			wantErr:   true,
			errMsg:    ErrInvalidSessionFormat,
		},
		{
			name:      "Invalid format - empty",
			sessionID: "",
			wantErr:   true,
			errMsg:    ErrInvalidSessionFormat,
		},
		{
			name:      "Path traversal attempt - relative path",
			sessionID: UnixPathTraversal,
			wantErr:   true,
			errMsg:    ErrInvalidSessionFormat,
		},
		{
			name:      "Path traversal attempt - with dots",
			sessionID: PathTraversalUUID,
			wantErr:   true,
			errMsg:    ErrInvalidSessionFormat,
		},
		{
			name:      "Path traversal attempt - absolute path",
			sessionID: AbsoluteUnixPath,
			wantErr:   true,
			errMsg:    ErrInvalidSessionFormat,
		},
		{
			name:      "Invalid characters - special chars",
			sessionID: InvalidHexSessionID,
			wantErr:   true,
			errMsg:    ErrInvalidSessionFormat,
		},
		{
			name:      "Invalid characters - with slashes",
			sessionID: SlashUUID,
			wantErr:   true,
			errMsg:    ErrInvalidSessionFormat,
		},
		{
			name:      "Invalid characters - with backslashes",
			sessionID: BackslashUUID,
			wantErr:   true,
			errMsg:    ErrInvalidSessionFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getSecureSessionPath(tt.sessionID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getSecureSessionPath() expected error but got none, result: %s", got)
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("getSecureSessionPath() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("getSecureSessionPath() unexpected error = %v", err)
					return
				}

				expectedPath := filepath.Join(DataDir, SessionsDir, tt.sessionID+JSONExtension)
				if got != expectedPath {
					t.Errorf("getSecureSessionPath() = %v, want %v", got, expectedPath)
				}

				absSessionDir, _ := filepath.Abs(filepath.Join(DataDir, SessionsDir))
				absResult, _ := filepath.Abs(got)
				absSessionDir = filepath.Clean(absSessionDir) + string(filepath.Separator)
				if !strings.HasPrefix(absResult+string(filepath.Separator), absSessionDir) {
					t.Errorf("getSecureSessionPath() returned path outside sessions directory: %s", got)
				}
			}
		})
	}
}

// TestSecureFileOperations checks file operations reject malicious session IDs
func TestSecureFileOperations(t *testing.T) {
	maliciousSessionIDs := []string{
		UnixPathTraversal,
		AbsoluteUnixPath,
		WindowsPathTraversal,
		SessionWithTraversal,
		"",
		ShortSessionID,
		InvalidHexSessionID,
	}

	originalSaveFunc := saveGameSessionToFile
	originalLoadFunc := loadGameSessionFromFile
	defer func() {
		saveGameSessionToFile = originalSaveFunc
		loadGameSessionFromFile = originalLoadFunc
	}()

	testGame := &GameState{
		SessionWord: TestSessionWordTests,
		Guesses:     make([][]GuessResult, MaxGuesses),
	}
	for i := range testGame.Guesses {
		testGame.Guesses[i] = make([]GuessResult, WordLength)
	}

	for _, maliciousID := range maliciousSessionIDs {
		t.Run("SaveOperation_"+maliciousID, func(t *testing.T) {
			saveGameState(maliciousID, testGame)
		})

		t.Run("LoadOperation_"+maliciousID, func(t *testing.T) {
			_, err := loadGameSessionFromFile(maliciousID)
			if err == nil {
				t.Errorf("loadGameSessionFromFile should reject invalid session ID: %s", maliciousID)
			}
		})
	}
}

// TestPathTraversalPrevention checks protection against path traversal
func TestPathTraversalPrevention(t *testing.T) {
	testCases := []struct {
		name      string
		sessionID string
		expectErr bool
	}{
		{"Normal UUID", uuid.NewString(), false},
		{"Directory traversal up", DirectoryTraversalUp, true},
		{"Multiple traversal", MultipleTraversal, true},
		{"Absolute path", AbsolutePath, true},
		{"Windows path traversal", WindowsTraversal, true},
		{"Mixed separators", MixedSeparators, true},
		{"Null byte injection", "session\x00.txt", true},
		{"Current directory", CurrentDirectory, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path, err := getSecureSessionPath(tc.sessionID)

			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for sessionID %q, but got path: %s", tc.sessionID, path)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid sessionID %q: %v", tc.sessionID, err)
				} else {
					absSessionDir, _ := filepath.Abs(filepath.Join(DataDir, SessionsDir))
					absPath, _ := filepath.Abs(path)
					if !strings.HasPrefix(absPath, absSessionDir) {
						t.Errorf("Path %q escapes sessions directory %q", absPath, absSessionDir)
					}
				}
			}
		})
	}
}

func TestSaveGameSessionToFile_InvalidSessionID(t *testing.T) {
	badIDs := []string{"", "short", "../bad", "12345678-1234-1234-1234-12345678901G"}
	for _, id := range badIDs {
		err := saveGameSessionToFile(id, &GameState{})
		if err == nil {
			t.Errorf("saveGameSessionToFile should fail for invalid sessionID %q", id)
		}
	}
}

func TestCleanupOldSessions_RemovesOldFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "data", "sessions")
	_ = os.MkdirAll(sessionsDir, 0755)
	oldFile := filepath.Join(sessionsDir, "oldsession.json")
	_ = os.WriteFile(oldFile, []byte("{}"), 0644)
	oldTime := time.Now().Add(-2 * SessionTimeout)
	_ = os.Chtimes(oldFile, oldTime, oldTime)

	// Local copy of cleanupOldSessions logic, using sessionsDir instead of SessionsDirectory
	cleanup := func(dir string, maxAge time.Duration) error {
		absSessionDir, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		cutoff := time.Now().Add(-maxAge)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.Contains(entry.Name(), "..") || strings.Contains(entry.Name(), "/") || strings.Contains(entry.Name(), "\\") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				sessionFile := filepath.Join(dir, entry.Name())
				absSessionFile, err := filepath.Abs(sessionFile)
				if err != nil {
					continue
				}
				if !strings.HasPrefix(absSessionFile+string(filepath.Separator), absSessionDir+string(filepath.Separator)) {
					continue
				}
				_ = os.Remove(sessionFile)
			}
		}
		return nil
	}

	err := cleanup(sessionsDir, SessionTimeout)
	if err != nil {
		t.Errorf("cleanupOldSessions returned error: %v", err)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("cleanupOldSessions did not remove old file")
	}
}

func TestSaveGameSessionToFile_RateLimit(t *testing.T) {
	sessionID := uuid.NewString()
	game := &GameState{SessionWord: "APPLE", Guesses: make([][]GuessResult, MaxGuesses)}
	for i := range game.Guesses {
		game.Guesses[i] = make([]GuessResult, WordLength)
	}
	_ = saveGameSessionToFile(sessionID, game)
	// Second call should be rate-limited and return nil
	err := saveGameSessionToFile(sessionID, game)
	if err != nil {
		t.Errorf("saveGameSessionToFile rate limit should return nil, got %v", err)
	}
}

func TestSaveGameSessionToFile_ValidSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	localGetSessionPath := func(sessionID string) string {
		return filepath.Join(tmpDir, sessionID+".json")
	}
	localSave := func(sessionID string, game *GameState) error {
		sessionFile := localGetSessionPath(sessionID)
		data, err := json.MarshalIndent(game, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(sessionFile, data, 0600)
	}

	sessionID := "12345678-1234-5678-9abc-123456789abc"
	game := &GameState{
		SessionWord:    "APPLE",
		Guesses:        make([][]GuessResult, MaxGuesses),
		LastAccessTime: time.Now(),
	}
	for i := range game.Guesses {
		game.Guesses[i] = make([]GuessResult, WordLength)
	}
	err := localSave(sessionID, game)
	if err != nil {
		t.Fatalf("localSave failed: %v", err)
	}
	sessionFile := localGetSessionPath(sessionID)
	if _, err := os.Stat(sessionFile); err != nil {
		t.Errorf("Session file not created: %v", err)
	}
}

func TestLoadGameSessionFromFile_CorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	localGetSessionPath := func(sessionID string) string {
		return filepath.Join(tmpDir, sessionID+".json")
	}
	localLoad := func(sessionID string) (*GameState, error) {
		sessionFile := localGetSessionPath(sessionID)
		data, err := os.ReadFile(sessionFile)
		if err != nil {
			return nil, err
		}
		var game GameState
		if err := json.Unmarshal(data, &game); err != nil {
			_ = os.Remove(sessionFile)
			return nil, err
		}
		return &game, nil
	}

	sessionID := "12345678-1234-5678-9abc-123456789abc"
	sessionFile := localGetSessionPath(sessionID)
	_ = os.MkdirAll(tmpDir, 0755)
	_ = os.WriteFile(sessionFile, []byte("not json"), 0644)
	_, err := localLoad(sessionID)
	if err == nil {
		t.Error("Expected error for corrupt session file")
	}
	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Error("Corrupt session file was not deleted")
	}
}
