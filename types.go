package main

import "time"

// WordEntry is a word and its hint
type WordEntry struct {
	Word string `json:"word"`
	Hint string `json:"hint"`
}

// WordList is a list of WordEntry
type WordList struct {
	Words []WordEntry `json:"words"`
}

// GameState is a player's game session
type GameState struct {
	Guesses        [][]GuessResult
	CurrentRow     int
	GameOver       bool
	Won            bool
	TargetWord     string
	SessionWord    string
	GuessHistory   []string
	LastAccessTime time.Time `json:"lastAccessTime"`
}

// GuessResult is a letter's evaluation
type GuessResult struct {
	Letter string
	Status string // "correct", "present", "absent", or "invalid"
}
