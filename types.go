package main

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// contextKey is a type for context keys defined in this package.
type contextKey string

// WordEntry represents a word and its associated hint.
type WordEntry struct {
	Word string `json:"word"`
	Hint string `json:"hint"`
}

// WordList is a container for a list of WordEntry items, used for JSON unmarshalling.
type WordList struct {
	Words []WordEntry `json:"words"`
}

// GameState holds the state of a user's current game session.
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

// GuessResult represents the result of a single letter in a guess.
type GuessResult struct {
	Letter string `json:"letter"`
	Status string `json:"status"`
}

// App is the main application struct holding all global state and configuration.
type App struct {
	WordList        []WordEntry
	WordSet         map[string]struct{}
	AcceptedWordSet map[string]struct{}
	HintMap         map[string]string
	GameSessions    map[string]*GameState
	SessionMutex    sync.RWMutex
	LimiterMap      map[string]*rate.Limiter
	LimiterMutex    sync.RWMutex
	IsProduction    bool
	StartTime       time.Time
	CookieMaxAge    time.Duration
	StaticCacheAge  time.Duration
	RateLimitRPS    int
	RateLimitBurst  int
	RuneBufPool     *sync.Pool
}

// globalApp holds a reference to the running App instance for small helpers.
var globalApp *App

// setGlobalApp sets the package-level App pointer.
func setGlobalApp(a *App) {
	globalApp = a
}

// getAppInstance returns the package-level App pointer (may be nil in tests).
func getAppInstance() *App {
	return globalApp
}
