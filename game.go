package main

import (
	"context"
	"crypto/rand"
	"math/big"
	"slices"
	"time"

	"github.com/samber/lo"
)

// getRandomWordEntry returns a random WordEntry from the loaded word list.
func (app *App) getRandomWordEntry(ctx context.Context) WordEntry {
	reqID, _ := ctx.Value(requestIDKey).(string)

	// Check if context is cancelled
	select {
	case <-ctx.Done():
		if reqID != "" {
			logWarn("[request_id=%v] getRandomWordEntry cancelled: %v", reqID, ctx.Err())
		} else {
			logWarn("getRandomWordEntry cancelled: %v", ctx.Err())
		}
		return app.WordList[0]
	default:
	}

	// Generate cryptographically secure random index
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(app.WordList))))
	if err != nil {
		if reqID != "" {
			logWarn("[request_id=%v] Error generating random number: %v, using fallback", reqID, err)
		} else {
			logWarn("Error generating random number: %v, using fallback", err)
		}
		return app.WordList[0]
	}

	if reqID != "" {
		logInfo("[request_id=%v] Selected random word index: %d", reqID, n.Int64())
	}
	return app.WordList[n.Int64()]
}

// getRandomWordEntryExcluding returns a random WordEntry excluding completed words.
// Returns the selected word and a boolean indicating if all words are completed (reset needed).
func (app *App) getRandomWordEntryExcluding(ctx context.Context, completedWords []string) (WordEntry, bool) {
	reqID, _ := ctx.Value(requestIDKey).(string)

	// If no completed words, use standard selection
	if len(completedWords) == 0 {
		return app.getRandomWordEntry(ctx), false
	}

	// Filter out completed words from available pool
	availableWords := lo.Filter(app.WordList, func(entry WordEntry, _ int) bool {
		return !slices.Contains(completedWords, entry.Word)
	})

	// If all words completed, signal reset needed
	if len(availableWords) == 0 {
		if reqID != "" {
			logInfo("[request_id=%v] All words completed, reset needed. Total words: %d, Completed: %d", reqID, len(app.WordList), len(completedWords))
		} else {
			logInfo("All words completed, reset needed. Total words: %d, Completed: %d", len(app.WordList), len(completedWords))
		}
		return app.getRandomWordEntry(ctx), true
	}

	// Check for context cancellation
	select {
	case <-ctx.Done():
		if reqID != "" {
			logWarn("[request_id=%v] getRandomWordEntryExcluding cancelled: %v", reqID, ctx.Err())
		} else {
			logWarn("getRandomWordEntryExcluding cancelled: %v", ctx.Err())
		}
		return availableWords[0], false
	default:
	}

	// Select random word from filtered list
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(availableWords))))
	if err != nil {
		if reqID != "" {
			logWarn("[request_id=%v] Error generating random number for filtered words: %v, using fallback", reqID, err)
		} else {
			logWarn("Error generating random number for filtered words: %v, using fallback", err)
		}
		return availableWords[0], false
	}

	selected := availableWords[n.Int64()]
	if reqID != "" {
		logInfo("[request_id=%v] Selected word from %d available options (excluding %d completed): %s", reqID, len(availableWords), len(completedWords), selected.Word)
	} else {
		logInfo("Selected word from %d available options (excluding %d completed): %s", len(availableWords), len(completedWords), selected.Word)
	}

	return selected, false
}

// getHintForWord returns the hint for a given word, or an empty string if not found.
func (app *App) getHintForWord(wordValue string) string {
	if wordValue == "" {
		return ""
	}
	hint, ok := app.HintMap[wordValue]
	if ok {
		return hint
	}
	logWarn("Hint not found for word: %s", wordValue)
	return ""
}

// buildHintMap creates a map from word to hint for fast lookup.
func buildHintMap(wordList []WordEntry) map[string]string {
	return lo.Associate(wordList, func(entry WordEntry) (string, string) {
		return entry.Word, entry.Hint
	})
}

// getTargetWord returns the session's target word, assigning one if missing.
func (app *App) getTargetWord(ctx context.Context, game *GameState) string {
	if game.SessionWord == "" {
		selectedEntry := app.getRandomWordEntry(ctx)
		game.SessionWord = selectedEntry.Word
		logWarn("SessionWord was empty, assigned random word: %s", selectedEntry.Word)
	}
	return game.SessionWord
}

// updateGameState updates the game state after a guess, handling win/lose logic.
func (app *App) updateGameState(ctx context.Context, game *GameState, guess, targetWord string, result []GuessResult, isInvalid bool) {
	reqID, _ := ctx.Value(requestIDKey).(string)

	// Prevent updates beyond max guesses
	if game.CurrentRow >= MaxGuesses {
		return
	}

	// Update game state with new guess
	game.Guesses[game.CurrentRow] = result
	game.GuessHistory = append(game.GuessHistory, guess)
	game.LastAccessTime = time.Now()

	// Check for win condition
	if !isInvalid && guess == targetWord {
		game.Won = true
		game.GameOver = true
		if reqID != "" {
			logInfo("[request_id=%v] Player won! Target word was: %s", reqID, targetWord)
		} else {
			logInfo("Player won! Target word was: %s", targetWord)
		}
	} else {
		// Move to next row
		game.CurrentRow++

		// Check for game over (max guesses reached)
		if game.CurrentRow >= MaxGuesses {
			game.GameOver = true
			if reqID != "" {
				logInfo("[request_id=%v] Player lost. Target word was: %s", reqID, targetWord)
			} else {
				logInfo("Player lost. Target word was: %s", targetWord)
			}
		}
	}

	// Reveal target word when game ends
	if game.GameOver {
		game.TargetWord = targetWord
	}
}

// checkGuess compares a guess to the target word and returns per-letter results.
func checkGuess(guess, target string) []GuessResult {
	result := make([]GuessResult, WordLength)
	targetCopy := []rune(target)

	// First pass: mark correct positions (exact matches)
	for i := range WordLength {
		if guess[i] == target[i] {
			result[i] = GuessResult{Letter: string(guess[i]), Status: GuessStatusCorrect}
			targetCopy[i] = ' ' // Mark as used
		}
	}

	// Second pass: mark present/absent for non-exact matches
	for i := range WordLength {
		if result[i].Status == "" {
			letter := string(guess[i])
			result[i].Letter = letter

			// Check if letter exists elsewhere in target
			found := false
			for j := range WordLength {
				if targetCopy[j] == rune(guess[i]) {
					result[i].Status = GuessStatusPresent
					targetCopy[j] = ' ' // Mark as used
					found = true
					break
				}
			}

			// Letter not found in target
			if !found {
				result[i].Status = GuessStatusAbsent
			}
		}
	}

	return result
}

// isValidWord returns true if the word is in the playable word set.
func (app *App) isValidWord(word string) bool {
	_, ok := app.WordSet[word]
	return ok
}

// isAcceptedWord returns true if the word is in the accepted guess set.
func (app *App) isAcceptedWord(word string) bool {
	_, ok := app.AcceptedWordSet[word]
	return ok
}

// createNewGame initializes a new GameState for a session and stores it.
func (app *App) createNewGame(ctx context.Context, sessionID string) *GameState {
	selectedEntry := app.getRandomWordEntry(ctx)
	logInfo("New game created for session %s with word: %s (hint: %s)", sessionID, selectedEntry.Word, selectedEntry.Hint)
	guesses := lo.Times(MaxGuesses, func(_ int) []GuessResult {
		return lo.Times(WordLength, func(_ int) GuessResult { return GuessResult{} })
	})
	game := &GameState{
		Guesses:        guesses,
		CurrentRow:     0,
		GameOver:       false,
		Won:            false,
		TargetWord:     "",
		SessionWord:    selectedEntry.Word,
		GuessHistory:   []string{},
		LastAccessTime: time.Now(),
	}
	app.GameSessions[sessionID] = game
	return game
}

// createNewGameWithCompletedWords initializes a new GameState excluding completed words.
func (app *App) createNewGameWithCompletedWords(ctx context.Context, sessionID string, completedWords []string) (*GameState, bool) {
	selectedEntry, needsReset := app.getRandomWordEntryExcluding(ctx, completedWords)
	logInfo("New game created for session %s with word: %s (hint: %s, completed words: %d, needs reset: %v)",
		sessionID, selectedEntry.Word, selectedEntry.Hint, len(completedWords), needsReset)

	guesses := lo.Times(MaxGuesses, func(_ int) []GuessResult {
		return lo.Times(WordLength, func(_ int) GuessResult { return GuessResult{} })
	})
	game := &GameState{
		Guesses:        guesses,
		CurrentRow:     0,
		GameOver:       false,
		Won:            false,
		TargetWord:     "",
		SessionWord:    selectedEntry.Word,
		GuessHistory:   []string{},
		LastAccessTime: time.Now(),
	}
	app.GameSessions[sessionID] = game
	return game, needsReset
}
