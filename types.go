package main

import "sync"

type WordList struct {
	Words []string `json:"words"`
}

type DailyWord struct {
	Word string
	Date string
	mu   sync.RWMutex
}

// DailyWordJSON is used for JSON serialization without the mutex
type DailyWordJSON struct {
	Word string `json:"word"`
	Date string `json:"date"`
}

type GameState struct {
	Guesses    [][]GuessResult
	CurrentRow int
	GameOver   bool
	Won        bool
	TargetWord string // Revealed only when game ends
}

type GuessResult struct {
	Letter string
	Status string // "correct", "present", or "absent"
}

// PlayerStats tracks user statistics (for future implementation)
type PlayerStats struct {
	GamesPlayed       int         `json:"gamesPlayed"`
	GamesWon          int         `json:"gamesWon"`
	CurrentStreak     int         `json:"currentStreak"`
	MaxStreak         int         `json:"maxStreak"`
	GuessDistribution map[int]int `json:"guessDistribution"`
}

// ToJSON safely converts DailyWord to a JSON-serializable struct
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

// GetWord returns the current word with thread safety
func (dw *DailyWord) GetWord() string {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return dw.Word
}

// GetDate returns the current date with thread safety
func (dw *DailyWord) GetDate() string {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return dw.Date
}

// toJSONUnsafe creates JSON struct without acquiring lock (for use when lock is already held)
func (dw *DailyWord) toJSONUnsafe() DailyWordJSON {
	return DailyWordJSON{
		Word: dw.Word,
		Date: dw.Date,
	}
}
