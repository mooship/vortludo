package main

// Game configuration constants
const (
	MaxGuesses = 6 // Maximum number of guesses per game
	WordLength = 5 // Length of the word to guess
)

// Guess status constants
const (
	GuessStatusCorrect = "correct"
	GuessStatusPresent = "present"
	GuessStatusAbsent  = "absent"
)

// Session configuration constants
const (
	SessionCookieName = "session_id"
)

// Route constants
const (
	RouteHome      = "/"
	RouteNewGame   = "/new-game"
	RouteRetryWord = "/retry-word"
	RouteGuess     = "/guess"
	RouteGameState = "/game-state"
)

// Error message constants
const (
	ErrorGameOver      = "Game is over."
	ErrorInvalidLength = "Word must be 5 letters."
	ErrorNoMoreGuesses = "No more guesses allowed."
	ErrorNotInWordList = "Word not recognised."
)

// Context key constants
const (
	requestIDKey contextKey = "request_id"
)
