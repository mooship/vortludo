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

// Global variables for session save rate limiting.
var (
	lastSaveTimes  = make(map[string]time.Time)
	lastSaveTimesM sync.Mutex
)

// Constants for persistence operations.
const (
	SessionsDirectory     = "data/sessions"
	SessionsDirPerm       = 0750 // Owner: read/write/execute, group: read/execute
	SessionFilePerm       = 0600 // Owner: read/write only
	JSONMarshalPrefix     = ""
	JSONMarshalIndent     = "  "
	SaveRateLimitInterval = time.Second
)

// saveGameSessionToFile persists a game session to disk with rate limiting.
var saveGameSessionToFile = func(sessionID string, game *GameState) error {
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

	// Create sessions directory with restrictive permissions.
	if err := os.MkdirAll(SessionsDirectory, SessionsDirPerm); err != nil {
		log.Printf("Failed to create sessions directory: %v", err)
		return err
	}

	log.Printf("Saving game session to file: %s", sessionFile)

	game.LastAccessTime = time.Now()
	data, err := json.MarshalIndent(game, JSONMarshalPrefix, JSONMarshalIndent)
	if err != nil {
		log.Printf("Failed to marshal game state for session %s: %v", sessionID, err)
		return err
	}

	// Write session data with restrictive file permissions.
	err = os.WriteFile(sessionFile, data, SessionFilePerm)
	if err != nil {
		log.Printf("Failed to write session file %s: %v", sessionFile, err)
	} else {
		log.Printf("Successfully saved session file: %s", sessionFile)
	}

	return err
}

// loadGameSessionFromFile loads a game session from disk with validation.
var loadGameSessionFromFile = func(sessionID string) (*GameState, error) {
	// Validate session ID format and get secure path.
	sessionFile, err := getSecureSessionPath(sessionID)
	if err != nil {
		log.Printf("Invalid session ID for loading: %s, error: %v", sessionID, err)
		return nil, os.ErrNotExist
	}

	log.Printf("Attempting to load session from file: %s", sessionFile)

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

	// Additional validation: ensure file is in sessions directory.
	absSessionFile, err := filepath.Abs(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve session file path: %w", err)
	}

	absSessionDir, err := filepath.Abs(SessionsDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve sessions directory: %w", err)
	}

	if !strings.HasPrefix(absSessionFile, absSessionDir+string(filepath.Separator)) {
		return nil, errors.New("session file path escapes sessions directory")
	}

	// Read and unmarshal session data.
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		log.Printf("Failed to read session file %s: %v", sessionFile, err)
		return nil, err
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

// cleanupOldSessions removes session files older than specified duration.
var cleanupOldSessions = func(maxAge time.Duration) error {
	log.Printf("Starting cleanup of sessions older than %v in directory: %s", maxAge, SessionsDirectory)

	entries, err := os.ReadDir(SessionsDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Sessions directory doesn't exist, skipping cleanup")
			return nil
		}
		log.Printf("Failed to read sessions directory: %v", err)
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	removedCount := 0
	errorCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
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
