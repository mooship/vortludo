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

	// Test 1: Valid session file - use proper UUID format
	sessionIDValid := uuid.NewString()
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

	// Test 2: Old file (should be removed) - use proper UUID format
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

	// Test 3: Corrupted file (should be removed) - use proper UUID format
	sessionIDCorrupt := uuid.NewString()
	corruptFilePath := filepath.Join("data", "sessions", sessionIDCorrupt+".json")
	_ = os.WriteFile(corruptFilePath, []byte("this is not json"), 0644)

	_, err = loadGameSessionFromFile(sessionIDCorrupt)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile did not return ErrNotExist for corrupt file, got: %v", err)
	}
	if _, statErr := os.Stat(corruptFilePath); !os.IsNotExist(statErr) {
		t.Errorf("loadGameSessionFromFile did not remove corrupt session file: %s", corruptFilePath)
	}

	// Test 4: Invalid structure (should be removed) - use proper UUID format
	sessionIDInvalidStruct := uuid.NewString()
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

	// Test 5: Invalid session ID format (should be rejected)
	invalidSessionID := "invalid-session-format"
	_, err = loadGameSessionFromFile(invalidSessionID)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("loadGameSessionFromFile should reject invalid session ID format, got: %v", err)
	}
}

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
			sessionID: "12345678-1234-5678-9ABC-123456789DEF",
			wantErr:   false,
		},
		{
			name:      "Invalid format - too short",
			sessionID: "short",
			wantErr:   true,
			errMsg:    "invalid session ID format",
		},
		{
			name:      "Invalid format - empty",
			sessionID: "",
			wantErr:   true,
			errMsg:    "invalid session ID format",
		},
		{
			name:      "Path traversal attempt - relative path",
			sessionID: "../../../etc/passwd",
			wantErr:   true,
			errMsg:    "invalid session ID format",
		},
		{
			name:      "Path traversal attempt - with dots",
			sessionID: "12345678-1234-5678-9ABC-123456789../",
			wantErr:   true,
			errMsg:    "invalid session ID format",
		},
		{
			name:      "Path traversal attempt - absolute path",
			sessionID: "/etc/passwd",
			wantErr:   true,
			errMsg:    "invalid session ID format",
		},
		{
			name:      "Invalid characters - special chars",
			sessionID: "12345678-1234-5678-9ABC-123456789XYZ",
			wantErr:   true,
			errMsg:    "invalid session ID format",
		},
		{
			name:      "Invalid characters - with slashes",
			sessionID: "12345678/1234/5678/9ABC/123456789DEF",
			wantErr:   true,
			errMsg:    "invalid session ID format",
		},
		{
			name:      "Invalid characters - with backslashes",
			sessionID: "12345678\\1234\\5678\\9ABC\\123456789DEF",
			wantErr:   true,
			errMsg:    "invalid session ID format",
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

				// Validate the returned path is safe
				expectedPath := filepath.Join("data", "sessions", tt.sessionID+".json")
				if got != expectedPath {
					t.Errorf("getSecureSessionPath() = %v, want %v", got, expectedPath)
				}

				// Ensure path is within sessions directory
				absSessionDir, _ := filepath.Abs("data/sessions")
				absResult, _ := filepath.Abs(got)
				absSessionDir = filepath.Clean(absSessionDir) + string(filepath.Separator)
				if !strings.HasPrefix(absResult+string(filepath.Separator), absSessionDir) {
					t.Errorf("getSecureSessionPath() returned path outside sessions directory: %s", got)
				}
			}
		})
	}
}

func TestSecureFileOperations(t *testing.T) {
	// Test that file operations properly reject invalid session IDs
	maliciousSessionIDs := []string{
		"../../../etc/passwd",
		"/etc/passwd",
		"..\\..\\windows\\system32\\drivers\\etc\\hosts",
		"session/../../../secret.txt",
		"",
		"short",
		"12345678-1234-5678-9ABC-123456789XYZ", // Invalid hex
	}

	// Store original functions to restore after test
	originalSaveFunc := saveGameSessionToFile
	originalLoadFunc := loadGameSessionFromFile
	defer func() {
		saveGameSessionToFile = originalSaveFunc
		loadGameSessionFromFile = originalLoadFunc
	}()

	testGame := &GameState{
		SessionWord: "TESTS",
		Guesses:     make([][]GuessResult, MaxGuesses),
	}
	for i := range testGame.Guesses {
		testGame.Guesses[i] = make([]GuessResult, WordLength)
	}

	for _, maliciousID := range maliciousSessionIDs {
		t.Run("SaveOperation_"+maliciousID, func(t *testing.T) {
			// This should not panic and should handle the invalid ID gracefully
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

func TestPathTraversalPrevention(t *testing.T) {
	// Test specific path traversal scenarios
	testCases := []struct {
		name      string
		sessionID string
		expectErr bool
	}{
		{"Normal UUID", uuid.NewString(), false},
		{"Directory traversal up", "../session", true},
		{"Multiple traversal", "../../session", true},
		{"Absolute path", "/tmp/session", true},
		{"Windows path traversal", "..\\session", true},
		{"Mixed separators", "../\\session", true},
		{"Null byte injection", "session\x00.txt", true},
		{"Current directory", "./session", true},
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
					absSessionDir, _ := filepath.Abs("data/sessions")
					absPath, _ := filepath.Abs(path)
					if !strings.HasPrefix(absPath, absSessionDir) {
						t.Errorf("Path %q escapes sessions directory %q", absPath, absSessionDir)
					}
				}
			}
		})
	}
}
