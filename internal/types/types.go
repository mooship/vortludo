package types

import "time"

type WordEntry struct {
	Word string `json:"word"`
	Hint string `json:"hint"`
}

type WordList struct {
	Words []WordEntry `json:"words"`
}

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
type GuessResult struct {
	Letter string `json:"letter"`
	Status string `json:"status"`
}
