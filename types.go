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
	Guesses        [][]GuessResult `json:"guesses"`        // All guesses made so far
	CurrentRow     int             `json:"currentRow"`     // Index of the current guess
	GameOver       bool            `json:"gameOver"`       // Whether the game is over
	Won            bool            `json:"won"`            // Whether the player has won
	TargetWord     string          `json:"targetWord"`     // The word to guess (revealed at end)
	SessionWord    string          `json:"sessionWord"`    // The word assigned to this session
	GuessHistory   []string        `json:"guessHistory"`   // List of previous guesses
	LastAccessTime time.Time       `json:"lastAccessTime"` // Last time this session was accessed
}

// GuessResult represents the result of a single letter in a guess.
type GuessResult struct {
	Letter string `json:"letter"`
	Status string `json:"status"`
}

// App is the main application struct holding all global state and configuration.
type App struct {
	WordList        []WordEntry              // List of all playable words
	WordSet         map[string]struct{}      // Set for fast word lookup
	AcceptedWordSet map[string]struct{}      // Set of all accepted guess words
	HintMap         map[string]string        // Map from word to hint
	GameSessions    map[string]*GameState    // Active game sessions by session ID
	SessionMutex    sync.RWMutex             // Mutex for session map
	LimiterMap      map[string]*rate.Limiter // Rate limiters by client IP
	LimiterMutex    sync.Mutex               // Mutex for limiter map
	IsProduction    bool                     // True if running in production
	StartTime       time.Time                // Server start time
	CookieMaxAge    time.Duration            // Max age for session cookies
	StaticCacheAge  time.Duration            // Cache age for static assets
	RateLimitRPS    int                      // Requests per second for rate limiting
	RateLimitBurst  int                      // Burst size for rate limiting
}
