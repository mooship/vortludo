package main

// Game configuration constants
const (
	MaxGuesses = 6
	WordLength = 5
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

// Error code constants
const (
	ErrorCodeGameOver        = "game_over"
	ErrorCodeInvalidLength   = "invalid_length"
	ErrorCodeNoMoreGuesses   = "no_more_guesses"
	ErrorCodeNotInWordList   = "not_in_word_list"
	ErrorCodeWordNotAccepted = "word_not_accepted"
	ErrorCodeDuplicateGuess  = "duplicate_guess"
)

// Context key constants
const (
	requestIDKey contextKey = "request_id"
)
