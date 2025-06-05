package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// saveGameSessionToFile atomically persists game state to disk
func saveGameSessionToFile(sessionID string, game *GameState) error {
	// Validate session ID
	if !isValidSessionID(sessionID) {
		log.Printf("Invalid session ID for saving: %s", sessionID)
		return fmt.Errorf("invalid session ID")
	}

	// Ensure sessions directory exists
	sessionDir := "data/sessions"
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		log.Printf("Failed to create sessions directory: %v", err)
		return err
	}

	// Prevent path traversal attacks
	safeSessionID := filepath.Base(sessionID)
	if safeSessionID != sessionID || strings.Contains(sessionID, "..") {
		log.Printf("Potential path traversal attempt with session ID: %s", sessionID)
		return fmt.Errorf("invalid session ID")
	}

	sessionFile := filepath.Join(sessionDir, safeSessionID+".json")
	tempFile := sessionFile + ".tmp"

	log.Printf("Saving game session to file: %s", sessionFile)

	data, err := json.MarshalIndent(game, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal game state for session %s: %v", sessionID, err)
		return err
	}

	// Write atomically via temp file
	err = os.WriteFile(tempFile, data, 0644)
	if err != nil {
		log.Printf("Failed to write temp session file %s: %v", tempFile, err)
		return err
	}

	// Atomic rename
	err = os.Rename(tempFile, sessionFile)
	if err != nil {
		log.Printf("Failed to rename session file %s: %v", sessionFile, err)
		os.Remove(tempFile) // Cleanup on failure
		return err
	}

	log.Printf("Successfully saved session file: %s", sessionFile)
	return nil
}

// loadGameSessionFromFile loads and validates a persisted game session
func loadGameSessionFromFile(sessionID string) (*GameState, error) {
	// Validate session ID
	if !isValidSessionID(sessionID) {
		log.Printf("Invalid session ID for loading: %s", sessionID)
		return nil, fmt.Errorf("invalid session ID")
	}

	// Sanitize path
	safeSessionID := filepath.Base(sessionID)
	if safeSessionID != sessionID || strings.Contains(sessionID, "..") {
		log.Printf("Potential path traversal attempt with session ID: %s", sessionID)
		return nil, fmt.Errorf("invalid session ID")
	}

	sessionFile := filepath.Join("data/sessions", safeSessionID+".json")

	// Verify path is within sessions directory
	absPath, err := filepath.Abs(sessionFile)
	if err != nil {
		return nil, err
	}

	expectedDir, err := filepath.Abs("data/sessions")
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(absPath, expectedDir) {
		log.Printf("Path traversal attempt detected: %s", sessionFile)
		return nil, fmt.Errorf("invalid path")
	}

	log.Printf("Attempting to load session from file: %s", sessionFile)

	// Check file age
	info, err := os.Stat(sessionFile)
	if err != nil {
		log.Printf("Session file not found: %s", sessionFile)
		return nil, err
	}

	fileAge := time.Since(info.ModTime())
	if fileAge > 24*time.Hour {
		log.Printf("Session file is too old (%v), removing: %s", fileAge, sessionFile)
		os.Remove(sessionFile)
		return nil, os.ErrNotExist
	}

	// Load and parse
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		log.Printf("Failed to read session file %s: %v", sessionFile, err)
		return nil, err
	}

	var game GameState
	if err := json.Unmarshal(data, &game); err != nil {
		log.Printf("Failed to unmarshal session file %s (corrupted), removing: %v", sessionFile, err)
		os.Remove(sessionFile)
		return nil, err
	}

	// Validate structure
	if len(game.Guesses) != 6 || game.SessionWord == "" {
		log.Printf("Session file %s has invalid structure, removing", sessionFile)
		os.Remove(sessionFile)
		return nil, os.ErrNotExist
	}

	// Verify word exists in dictionary
	if _, exists := wordMap[game.SessionWord]; !exists {
		log.Printf("Session file %s contains invalid word, removing", sessionFile)
		os.Remove(sessionFile)
		return nil, os.ErrNotExist
	}

	log.Printf("Successfully loaded session from file: %s (word: %s, row: %d)", sessionFile, game.SessionWord, game.CurrentRow)
	return &game, nil
}

// cleanupOldSessions removes expired session files
func cleanupOldSessions(maxAge time.Duration) error {
	sessionDir := "data/sessions"

	// Get absolute path for security
	absSessionDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return err
	}

	log.Printf("Starting cleanup of sessions older than %v in directory: %s", maxAge, absSessionDir)

	entries, err := os.ReadDir(sessionDir)
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

		// Only process JSON files
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			log.Printf("Failed to get info for session file %s: %v", entry.Name(), err)
			errorCount++
			continue
		}

		if info.ModTime().Before(cutoff) {
			sessionFile := filepath.Join(sessionDir, entry.Name())

			// Security check
			absPath, err := filepath.Abs(sessionFile)
			if err != nil || !strings.HasPrefix(absPath, absSessionDir) {
				log.Printf("Skipping suspicious file: %s", entry.Name())
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
