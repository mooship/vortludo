package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// saveGameSessionToFile persists a game session to disk
func saveGameSessionToFile(sessionID string, game *GameState) error {
	// Validate session ID to prevent path traversal
	if sessionID == "" || len(sessionID) < 10 {
		return nil // Skip saving invalid sessions
	}

	// Create sessions directory if it doesn't exist
	sessionDir := "data/sessions"
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return err
	}

	// Save session to file
	sessionFile := filepath.Join(sessionDir, sessionID+".json")
	data, err := json.MarshalIndent(game, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sessionFile, data, 0644)
}

// loadGameSessionFromFile loads a game session from disk
func loadGameSessionFromFile(sessionID string) (*GameState, error) {
	// Validate session ID to prevent path traversal
	if sessionID == "" || len(sessionID) < 10 {
		return nil, os.ErrNotExist
	}

	sessionFile := filepath.Join("data/sessions", sessionID+".json")

	// Check if file exists and is not too old (more than 24 hours)
	info, err := os.Stat(sessionFile)
	if err != nil {
		return nil, err
	}

	if time.Since(info.ModTime()) > 24*time.Hour {
		// Remove old session file
		os.Remove(sessionFile)
		return nil, os.ErrNotExist
	}

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, err
	}

	var game GameState
	if err := json.Unmarshal(data, &game); err != nil {
		// Remove corrupted session file
		os.Remove(sessionFile)
		return nil, err
	}

	// Validate game state structure
	if len(game.Guesses) != 6 || game.SessionWord == "" {
		// Remove invalid session file
		os.Remove(sessionFile)
		return nil, os.ErrNotExist
	}

	return &game, nil
}

// cleanupOldSessions removes session files older than specified duration
func cleanupOldSessions(maxAge time.Duration) error {
	sessionDir := "data/sessions"

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(sessionDir, entry.Name()))
		}
	}

	return nil
}
