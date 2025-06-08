package main

import "time"

// WordEntry represents a word and its hint
type WordEntry struct {
	Word string `json:"word"`
	Hint string `json:"hint"`
}

// WordList is a list of WordEntry
type WordList struct {
	Words []WordEntry `json:"words"`
}

// GameState represents a player's game session
type GameState struct {
	Guesses        [][]GuessResult `json:"guesses"`
	CurrentRow     int             `json:"currentRow"`
	GameOver       bool            `json:"gameOver"`
	Won            bool            `json:"won"`
	TargetWord     string          `json:"targetWord"`
	SessionWord    string          `json:"sessionWord"`
	GuessHistory   []string        `json:"guessHistory"`
	LastAccessTime time.Time       `json:"lastAccessTime"`
}

// GuessResult represents a letter's evaluation in a guess
type GuessResult struct {
	Letter string `json:"letter"`
	Status string `json:"status"` // "correct", "present", "absent", or "invalid"
}
