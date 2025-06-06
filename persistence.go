package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

// saveGameSessionToFile persists a game session to disk
var saveGameSessionToFile = func(sessionID string, game *GameState) error {
	if sessionID == "" || len(sessionID) < 10 {
		log.Printf("Skipping save for invalid session ID: %s", sessionID)
		return nil
	}

	// Create sessions directory
	sessionDir := "data/sessions"
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		log.Printf("Failed to create sessions directory: %v", err)
		return err
	}

	sessionFile := filepath.Join(sessionDir, sessionID+".json")
	log.Printf("Saving game session to file: %s", sessionFile)

	game.LastAccessTime = time.Now()
	data, err := json.MarshalIndent(game, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal game state for session %s: %v", sessionID, err)
		return err
	}

	err = os.WriteFile(sessionFile, data, 0644)
	if err != nil {
		log.Printf("Failed to write session file %s: %v", sessionFile, err)
	} else {
		log.Printf("Successfully saved session file: %s", sessionFile)
	}

	return err
}

// loadGameSessionFromFile loads a game session from disk
var loadGameSessionFromFile = func(sessionID string) (*GameState, error) {
	if sessionID == "" || len(sessionID) < 10 {
		log.Printf("Invalid session ID for loading: %s", sessionID)
		return nil, os.ErrNotExist
	}

	sessionFile := filepath.Join("data/sessions", sessionID+".json")
	log.Printf("Attempting to load session from file: %s", sessionFile)

	// Check if file exists and age
	info, err := os.Stat(sessionFile)
	if err != nil {
		log.Printf("Session file not found: %s", sessionFile)
		return nil, err
	}

	fileAge := time.Since(info.ModTime())
	if fileAge > SessionTimeout {
		log.Printf("Session file is too old (%v, max: %v), removing: %s", fileAge, SessionTimeout, sessionFile)
		os.Remove(sessionFile)
		return nil, os.ErrNotExist
	}

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		log.Printf("Failed to read session file %s: %v", sessionFile, err)
		return nil, err
	}

	var game GameState
	if err := json.Unmarshal(data, &game); err != nil {
		log.Printf("Failed to unmarshal session file %s (corrupted), removing: %v", sessionFile, err)
		os.Remove(sessionFile)
		return nil, os.ErrNotExist
	}

	game.LastAccessTime = time.Now()

	// Validate game state structure
	if len(game.Guesses) != MaxGuesses || game.SessionWord == "" {
		log.Printf("Session file %s has invalid structure (guesses: %d, word: '%s'), removing", sessionFile, len(game.Guesses), game.SessionWord)
		os.Remove(sessionFile)
		return nil, os.ErrNotExist
	}

	log.Printf("Successfully loaded session from file: %s (word: %s, row: %d)", sessionFile, game.SessionWord, game.CurrentRow)
	return &game, nil
}

// cleanupOldSessions removes session files older than specified duration
var cleanupOldSessions = func(maxAge time.Duration) error {
	sessionDir := "data/sessions"
	log.Printf("Starting cleanup of sessions older than %v in directory: %s", maxAge, sessionDir)

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

		info, err := entry.Info()
		if err != nil {
			log.Printf("Failed to get info for session file %s: %v", entry.Name(), err)
			errorCount++
			continue
		}

		if info.ModTime().Before(cutoff) {
			sessionFile := filepath.Join(sessionDir, entry.Name())
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
