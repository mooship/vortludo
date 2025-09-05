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
	ErrorGameOver        = "game is already over! start a new game"
	ErrorInvalidLength   = "word must be 5 letters"
	ErrorNoMoreGuesses   = "no more guesses allowed"
	ErrorNotInWordList   = "word not recognised"
	ErrorWordNotAccepted = "word not accepted, try another word"
	ErrorDuplicateGuess  = "you already guessed that word"
)

// Context key constants
const (
	requestIDKey contextKey = "request_id"
)
