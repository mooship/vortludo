package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Globals for session save rate limiting
var (
	lastSaveTimes  = make(map[string]time.Time)
	lastSaveTimesM sync.Mutex
)

// Persistence constants
const (
	SessionsDirectory     = "data/sessions"
	SessionsDirPerm       = 0750 // Owner: read/write/execute, group: read/execute
	SessionFilePerm       = 0600 // Owner: read/write only
	JSONMarshalPrefix     = ""
	JSONMarshalIndent     = "  "
	SaveRateLimitInterval = time.Second
)

// saveGameSessionToFile saves a session to disk
var saveGameSessionToFile = func(sessionID string, game *GameState) error {
	// Validate session ID format first
	if !isValidSessionID(sessionID) {
		log.Printf("Rejected save attempt with invalid session ID format: %s", sessionID)
		return errors.New("invalid session ID format")
	}

	// Rate-limit disk writes to prevent excessive I/O.
	lastSaveTimesM.Lock()
	last, ok := lastSaveTimes[sessionID]
	now := time.Now()
	if ok && now.Sub(last) < SaveRateLimitInterval {
		lastSaveTimesM.Unlock()
		return nil
	}
	lastSaveTimes[sessionID] = now
	lastSaveTimesM.Unlock()

	sessionFile, err := getSecureSessionPath(sessionID)
	if err != nil {
		log.Printf("Invalid session ID for saving: %s, error: %v", sessionID, err)
		return err
	}

	// Additional validation: ensure the session file path is secure
	absSessionFile, err := filepath.Abs(sessionFile)
	if err != nil {
		log.Printf("Failed to resolve session file path for saving: %v", err)
		return fmt.Errorf("failed to resolve session file path: %w", err)
	}

	absSessionDir, err := filepath.Abs(SessionsDirectory)
	if err != nil {
		log.Printf("Failed to resolve sessions directory for saving: %v", err)
		return fmt.Errorf("failed to resolve sessions directory: %w", err)
	}

	// Ensure the file path is within the sessions directory and is a direct child
	relPath, err := filepath.Rel(absSessionDir, absSessionFile)
	if err != nil || strings.Contains(relPath, "..") || strings.ContainsRune(relPath, os.PathSeparator) {
		log.Printf("Session file path escapes sessions directory or is not a direct child: %s", absSessionFile)
		return errors.New("session path would escape sessions directory or is not a direct child")
	}

	// Ensure filename matches expected pattern
	expectedFilename := sessionID + ".json"
	actualFilename := filepath.Base(absSessionFile)
	if actualFilename != expectedFilename {
		log.Printf("Session filename mismatch: expected %s, got %s", expectedFilename, actualFilename)
		return errors.New("session filename mismatch")
	}

	// Create sessions directory with restrictive permissions.
	if err := os.MkdirAll(SessionsDirectory, SessionsDirPerm); err != nil {
		log.Printf("Failed to create sessions directory: %v", err)
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	log.Printf("Saving game session to file: %s", sessionFile)

	game.LastAccessTime = time.Now()
	data, err := json.MarshalIndent(game, JSONMarshalPrefix, JSONMarshalIndent)
	if err != nil {
		log.Printf("Failed to marshal game state for session %s: %v", sessionID, err)
		return fmt.Errorf("failed to marshal game state: %w", err)
	}

	// Write session data with restrictive file permissions.
	err = os.WriteFile(sessionFile, data, SessionFilePerm)
	if err != nil {
		log.Printf("Failed to write session file %s: %v", sessionFile, err)
		return fmt.Errorf("failed to write session file: %w", err)
	} else {
		log.Printf("Successfully saved session file: %s", sessionFile)
	}

	return nil
}

// loadGameSessionFromFile loads a session from disk
var loadGameSessionFromFile = func(sessionID string) (*GameState, error) {
	// Validate session ID format first
	if !isValidSessionID(sessionID) {
		log.Printf("Rejected load attempt with invalid session ID format: %s", sessionID)
		return nil, os.ErrNotExist
	}

	// Validate session ID format and get secure path.
	sessionFile, err := getSecureSessionPath(sessionID)
	if err != nil {
		log.Printf("Invalid session ID for loading: %s, error: %v", sessionID, err)
		return nil, os.ErrNotExist
	}

	log.Printf("Attempting to load session from file: %s", sessionFile)

	// Additional validation: ensure file is in sessions directory and is a direct child
	absSessionFile, err := filepath.Abs(sessionFile)
	if err != nil {
		log.Printf("Failed to resolve session file path for loading: %v", err)
		return nil, fmt.Errorf("failed to resolve session file path: %w", err)
	}

	absSessionDir, err := filepath.Abs(SessionsDirectory)
	if err != nil {
		log.Printf("Failed to resolve sessions directory for loading: %v", err)
		return nil, fmt.Errorf("failed to resolve sessions directory: %w", err)
	}

	relPath, err := filepath.Rel(absSessionDir, absSessionFile)
	if err != nil || strings.Contains(relPath, "..") || strings.ContainsRune(relPath, os.PathSeparator) {
		log.Printf("Session file path escapes sessions directory or is not a direct child: %s", absSessionFile)
		return nil, errors.New("session path would escape sessions directory or is not a direct child")
	}

	// Ensure filename matches expected pattern
	expectedFilename := sessionID + ".json"
	actualFilename := filepath.Base(absSessionFile)
	if actualFilename != expectedFilename {
		log.Printf("Session filename mismatch: expected %s, got %s", expectedFilename, actualFilename)
		return nil, os.ErrNotExist
	}

	// Check if file exists and validate age.
	info, err := os.Stat(sessionFile)
	if err != nil {
		log.Printf("Session file not found: %s", sessionFile)
		return nil, err
	}

	fileAge := time.Since(info.ModTime())
	if fileAge > SessionTimeout {
		log.Printf("Session file is too old (%v, max: %v), removing: %s", fileAge, SessionTimeout, sessionFile)
		if err := os.Remove(sessionFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to remove old session file: %v", err)
		}
		return nil, os.ErrNotExist
	}

	// Read and unmarshal session data.
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		log.Printf("Failed to read session file %s: %v", sessionFile, err)
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var game GameState
	if err := json.Unmarshal(data, &game); err != nil {
		log.Printf("Failed to unmarshal session file %s (corrupted), removing: %v", sessionFile, err)
		if err := os.Remove(sessionFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to remove corrupted session file: %v", err)
		}
		return nil, os.ErrNotExist
	}

	game.LastAccessTime = time.Now()

	// Validate game state structure before returning.
	if len(game.Guesses) != MaxGuesses || game.SessionWord == "" {
		log.Printf("Session file %s has invalid structure (guesses: %d, word: '%s'), removing", sessionFile, len(game.Guesses), game.SessionWord)
		if err := os.Remove(sessionFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to remove invalid session file: %v", err)
		}
		return nil, os.ErrNotExist
	}

	log.Printf("Successfully loaded session from file: %s (word: %s, row: %d)", sessionFile, game.SessionWord, game.CurrentRow)
	return &game, nil
}

// cleanupOldSessions removes old session files
var cleanupOldSessions = func(maxAge time.Duration) error {
	log.Printf("Starting cleanup of sessions older than %v in directory: %s", maxAge, SessionsDirectory)

	// Validate sessions directory path
	absSessionDir, err := filepath.Abs(SessionsDirectory)
	if err != nil {
		log.Printf("Failed to resolve sessions directory for cleanup: %v", err)
		return fmt.Errorf("failed to resolve sessions directory: %w", err)
	}

	entries, err := os.ReadDir(SessionsDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Sessions directory doesn't exist, skipping cleanup")
			return nil
		}
		log.Printf("Failed to read sessions directory: %v", err)
		return fmt.Errorf("failed to read sessions directory: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	removedCount := 0
	errorCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Validate filename to prevent path traversal in cleanup
		if strings.Contains(entry.Name(), "..") || strings.Contains(entry.Name(), "/") || strings.Contains(entry.Name(), "\\") {
			log.Printf("Skipping file with suspicious name during cleanup: %s", entry.Name())
			continue
		}

		info, err := entry.Info()
		if err != nil {
			log.Printf("Failed to get info for session file %s: %v", entry.Name(), err)
			errorCount++
			continue
		}

		if info.ModTime().Before(cutoff) {
			sessionFile := filepath.Join(SessionsDirectory, entry.Name())

			// Additional safety check: ensure the file is within sessions directory
			absSessionFile, err := filepath.Abs(sessionFile)
			if err != nil {
				log.Printf("Failed to resolve path for cleanup file %s: %v", sessionFile, err)
				errorCount++
				continue
			}

			if !strings.HasPrefix(absSessionFile+string(filepath.Separator), absSessionDir+string(filepath.Separator)) {
				log.Printf("Skipping file outside sessions directory during cleanup: %s", absSessionFile)
				continue
			}

			if err := os.Remove(sessionFile); err != nil {
				log.Printf("Failed to remove old session file %s: %v", sessionFile, err)
				errorCount++
			} else {
				log.Printf("Removed old session file: %s (age: %v)", sessionFile, time.Since(info.ModTime()))
				removedCount++
			}
		}
	}

	log.Printf("Session cleanup completed: removed %d files, %d errors", removedCount, errorCount)
	return nil
}
