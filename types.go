package main

import "time"

// WordEntry represents a single word with its hint.
type WordEntry struct {
	Word string `json:"word"`
	Hint string `json:"hint"`
}

// WordList represents the JSON structure for loading valid words with hints.
type WordList struct {
	Words []WordEntry `json:"words"`
}

// GameState represents a player's current game session.
type GameState struct {
	Guesses        [][]GuessResult // 6 rows of 5 letters each with status.
	CurrentRow     int             // Which row the player is currently on (0-5).
	GameOver       bool            // Whether the game has ended.
	Won            bool            // Whether the player won.
	TargetWord     string          // The word for this game session (revealed only when game ends for display).
	SessionWord    string          // The actual target word for this session (hidden during gameplay).
	GuessHistory   []string        // All guesses made (for accurate try counting).
	LastAccessTime time.Time       `json:"lastAccessTime"` // Tracks when the session was last accessed.
}

// GuessResult represents a single letter's evaluation.
type GuessResult struct {
	Letter string // The guessed letter.
	Status string // "correct", "present", "absent", or "invalid".
}
