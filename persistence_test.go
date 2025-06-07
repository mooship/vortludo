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

// Test constants for persistence layer validation.
const (
	// File system constants.
	JSONExtension = ".json"
	DataDir       = "data"
	SessionsDir   = "sessions"

	// Test session words for validation.
	TestSessionWordLoaded    = "LOADED"
	TestSessionWordBadStruct = "BADSTRUCT"
	TestSessionWordTests     = "TESTS"

	// Test content for file operations.
	CorruptJSONContent = "this is not json"

	// Error messages for validation.
	ErrInvalidSessionFormat = "invalid session ID format"
	ErrPathEscapes          = "session file path escapes sessions directory"

	// File permission constants.
	TestDirPerm  = 0755
	TestFilePerm = 0644

	// Path traversal attack patterns.
	UnixPathTraversal    = "../../../etc/passwd"
	WindowsPathTraversal = "..\\..\\windows\\system32\\drivers\\etc\\hosts"
	AbsoluteUnixPath     = "/etc/passwd"
	AbsolutePath         = "/tmp/session"
	SessionWithTraversal = "session/../../../secret.txt"
	ShortSessionID       = "short"
	InvalidHexSessionID  = "12345678-1234-5678-9ABC-123456789XYZ"

	// Additional path traversal patterns.
	DirectoryTraversalUp = "../session"
	MultipleTraversal    = "../../session"
	WindowsTraversal     = "..\\session"
	MixedSeparators      = "../\\session"
	CurrentDirectory     = "./session"

	// Session ID format validation patterns.
	ValidUppercaseUUID = "12345678-1234-5678-9ABC-123456789DEF"
	PathTraversalUUID  = "12345678-1234-5678-9ABC-123456789../"
	SlashUUID          = "12345678/1234/5678/9ABC/123456789DEF"
	BackslashUUID      = "12345678\\1234\\5678\\9ABC\\123456789DEF"
)

// TestLoadGameSessionFromFile validates session loading from disk with various file conditions.
func TestLoadGameSessionFromFile(t *testing.T) {
	tempDir := t.TempDir()
	defer func() {
		_ = os.RemoveAll(filepath.Join(tempDir, DataDir, SessionsDir))
	}()

	sessionBaseDir := filepath.Join(tempDir, DataDir, SessionsDir)
	if err := os.MkdirAll(sessionBaseDir, TestDirPerm); err != nil {
		t.Fatalf("Failed to create temp session dir: %v", err)
	}

	// Helper to create test session files.
	createTestSessionFile := func(sID string, game *GameState, modTime *time.Time) string {
		filePath := filepath.Join(sessionBaseDir, sID+JSONExtension)
		data, _ := json.Marshal(game)
		_ = os.WriteFile(filePath, data, TestFilePerm)
		if modTime != nil {
			_ = os.Chtimes(filePath, *modTime, *modTime)
		}
		return filePath
	}

	// Change working directory to use temp structure.
	originalWD, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change WD to tempDir: %v", err)
	}
	defer os.Chdir(originalWD)

	if err := os.MkdirAll(filepath.Join(tempDir, DataDir, SessionsDir), TestDirPerm); err != nil {
		t.Fatalf("Failed to create data/sessions in tempDir: %v", err)
	}

	// Test 1: Valid session file - use proper UUID format.
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

	// Test 2: Old file (should be removed) - use proper UUID format.
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

	// Test 3: Corrupted file (should be removed) - use proper UUID format.
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

	// Test 4: Invalid structure (should be removed) - use proper UUID format.
	sessionIDInvalidStruct := uuid.NewString()
	invalidStructGame := &GameState{
		SessionWord: TestSessionWordBadStruct,
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

	// Test 5: Invalid session ID format (should be rejected).
	invalidSessionID := InvalidSessionFormat
	_, err = loadGameSessionFromFile(invalidSessionID)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile should reject invalid session ID format, got: %v", err)
	}
}

// TestGetSecureSessionPath validates secure path generation and path traversal prevention.
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

				// Validate the returned path is safe.
				expectedPath := filepath.Join(DataDir, SessionsDir, tt.sessionID+JSONExtension)
				if got != expectedPath {
					t.Errorf("getSecureSessionPath() = %v, want %v", got, expectedPath)
				}

				// Ensure path is within sessions directory.
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

// TestSecureFileOperations validates that file operations reject malicious session IDs.
func TestSecureFileOperations(t *testing.T) {
	// Test that file operations properly reject invalid session IDs.
	maliciousSessionIDs := []string{
		UnixPathTraversal,
		AbsoluteUnixPath,
		WindowsPathTraversal,
		SessionWithTraversal,
		"",
		ShortSessionID,
		InvalidHexSessionID, // Invalid hex
	}

	// Store original functions to restore after test.
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
			// This should not panic and should handle the invalid ID gracefully.
			saveGameState(maliciousID, testGame)
			// The save operation should either succeed with validation or fail gracefully
			// We don't expect the system to crash or access unintended files
		})

		t.Run("LoadOperation_"+maliciousID, func(t *testing.T) {
			// This should not panic and should handle the invalid ID gracefully
			_, err := loadGameSessionFromFile(maliciousID)
			// Should return an error for invalid session IDs
			if err == nil {
				t.Errorf("loadGameSessionFromFile should reject invalid session ID: %s", maliciousID)
			}
		})
	}
}

// TestPathTraversalPrevention validates protection against various path traversal attacks.
func TestPathTraversalPrevention(t *testing.T) {
	// Test specific path traversal scenarios
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
					// Verify the path stays within the sessions directory
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
