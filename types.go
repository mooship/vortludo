package main

import "sync"

// WordList represents the JSON structure for loading valid words
type WordList struct {
	Words []string `json:"words"`
}

// DailyWord holds the current daily word with thread-safe access
type DailyWord struct {
	Word string
	Date string
	mu   sync.RWMutex // Protects concurrent access to Word and Date
}

// DailyWordJSON is used for JSON serialization (excludes mutex)
type DailyWordJSON struct {
	Word string `json:"word"`
	Date string `json:"date"`
}

// GameState represents a player's current game session
type GameState struct {
	Guesses      [][]GuessResult // 6 rows of 5 letters each with status
	CurrentRow   int             // Which row the player is currently on (0-5)
	GameOver     bool            // Whether the game has ended
	Won          bool            // Whether the player won
	TargetWord   string          // Revealed only when game ends
	GuessHistory []string        // All guesses made (for accurate try counting)
}

// GuessResult represents a single letter's evaluation
type GuessResult struct {
	Letter string // The guessed letter
	Status string // "correct", "present", "absent", or "invalid"
}

// PlayerStats tracks user statistics (for future implementation)
type PlayerStats struct {
	GamesPlayed       int         `json:"gamesPlayed"`
	GamesWon          int         `json:"gamesWon"`
	CurrentStreak     int         `json:"currentStreak"`
	MaxStreak         int         `json:"maxStreak"`
	GuessDistribution map[int]int `json:"guessDistribution"` // Tries -> count
}

// Thread-safe methods for DailyWord

// ToJSON safely converts DailyWord to JSON-serializable struct
func (dw *DailyWord) ToJSON() DailyWordJSON {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return DailyWordJSON{
		Word: dw.Word,
		Date: dw.Date,
	}
}

// FromJSON safely updates DailyWord from JSON data
func (dw *DailyWord) FromJSON(dwj DailyWordJSON) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	dw.Word = dwj.Word
	dw.Date = dwj.Date
}

// GetWord returns the current word with read lock
func (dw *DailyWord) GetWord() string {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return dw.Word
}

// GetDate returns the current date with read lock
func (dw *DailyWord) GetDate() string {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return dw.Date
}

// toJSONUnsafe creates JSON struct without locking (for internal use when lock is held)
func (dw *DailyWord) toJSONUnsafe() DailyWordJSON {
	return DailyWordJSON{
		Word: dw.Word,
		Date: dw.Date,
	}
}
