package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

// homeHandler renders the main game page for the current session.
func (app *App) homeHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	game := app.getGameState(ctx, sessionID)
	hint := app.getHintForWord(game.SessionWord)

	csrfToken, _ := c.Cookie("csrf_token")
	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":      "Vortludo - A Libre Wordle Clone",
		"message":    "Guess the 5-letter word!",
		"hint":       hint,
		"game":       game,
		"csrf_token": csrfToken,
	})
}

// newGameHandler starts a new game session, optionally resetting the session ID.
func (app *App) newGameHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	logInfo("Creating new game for session: %s", sessionID)

	var completedWords []string
	if c.Request.Method == "POST" {
		completedWordsStr := c.PostForm("completedWords")
		if completedWordsStr != "" {
			if err := json.Unmarshal([]byte(completedWordsStr), &completedWords); err != nil {
				logWarn("Failed to parse completed words: %v", err)
				completedWords = []string{}
			} else {
				validCompletedWords := lo.Filter(completedWords, func(word string, _ int) bool {
					_, exists := app.WordSet[word]
					if !exists {
						logWarn("Invalid completed word ignored: %s", word)
					}
					return exists
				})
				completedWords = validCompletedWords
				logInfo("Validated %d completed words for session %s", len(completedWords), sessionID)
			}
		}
	}

	app.SessionMutex.Lock()
	delete(app.GameSessions, sessionID)
	app.SessionMutex.Unlock()
	logInfo("Cleared old session data for: %s", sessionID)

	if c.Query("reset") == "1" {
		c.SetSameSite(http.SameSiteStrictMode)
		secure := app.IsProduction
		c.SetCookie(SessionCookieName, "", -1, "/", "", secure, true)

		newSessionID := uuid.NewString()
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, newSessionID, int(app.CookieMaxAge.Seconds()), "/", "", secure, true)
		logInfo("Created new session ID: %s", newSessionID)

		if len(completedWords) > 0 {
			_, needsReset := app.createNewGameWithCompletedWords(ctx, newSessionID, completedWords)
			if needsReset {
				c.Header("HX-Trigger", "clear-completed-words")
			}
		} else {
			app.createNewGame(ctx, newSessionID)
		}
		sessionID = newSessionID
	} else {
		if len(completedWords) > 0 {
			_, needsReset := app.createNewGameWithCompletedWords(ctx, sessionID, completedWords)
			if needsReset {
				c.Header("HX-Trigger", "clear-completed-words")
			}
		} else {
			app.createNewGame(ctx, sessionID)
		}
	}

	isHTMX := c.GetHeader("HX-Request") == "true"
	if isHTMX {
		game := app.getGameState(ctx, sessionID)
		hint := app.getHintForWord(game.SessionWord)
		csrfToken, _ := c.Cookie("csrf_token")
		c.HTML(http.StatusOK, "game-content", gin.H{
			"game":       game,
			"hint":       hint,
			"newGame":    true,
			"csrf_token": csrfToken,
		})
	} else {
		c.Redirect(http.StatusSeeOther, RouteHome)
	}
}

// guessHandler processes a guess submission, validates it, and updates the game state.
func (app *App) guessHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	game := app.getGameState(ctx, sessionID)
	hint := app.getHintForWord(game.SessionWord)

	renderBoard := func(errMsg string) {
		csrfToken, _ := c.Cookie("csrf_token")
		if errMsg != "" {
			payload := map[string]string{"server_error": errMsg}
			if b, jerr := json.Marshal(payload); jerr == nil {
				c.Header("HX-Trigger", string(b))
			} else {
				logWarn("Failed to marshal HX-Trigger payload: %v", jerr)
			}
		}
		c.HTML(http.StatusOK, "game-content", gin.H{
			"game":       game,
			"hint":       hint,
			"error":      errMsg,
			"csrf_token": csrfToken,
		})
	}

	renderFullPage := func(errMsg string) {
		csrfToken, _ := c.Cookie("csrf_token")
		if errMsg != "" {
			payload := map[string]string{"server_error": errMsg}
			if b, jerr := json.Marshal(payload); jerr == nil {
				c.Header("HX-Trigger", string(b))
			} else {
				logWarn("Failed to marshal HX-Trigger payload: %v", jerr)
			}
		}
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title":      "Vortludo - A Libre Wordle Clone",
			"message":    "Guess the 5-letter word!",
			"hint":       hint,
			"game":       game,
			"error":      errMsg,
			"csrf_token": csrfToken,
		})
	}

	isHTMX := c.GetHeader("HX-Request") == "true"
	var errMsg string
	if err := app.validateGameState(c, game); err != nil {
		errMsg = err.Error()
		if isHTMX {
			renderBoard(errMsg)
		} else {
			renderFullPage(errMsg)
		}
		return
	}

	guess := normalizeGuess(c.PostForm("guess"))
	if !app.isAcceptedWord(guess) {
		errMsg = ErrorWordNotAccepted
		if isHTMX {
			renderBoard(errMsg)
		} else {
			renderFullPage(errMsg)
		}
		return
	}

	if slices.Contains(game.GuessHistory, guess) {
		errMsg = ErrorDuplicateGuess
		if isHTMX {
			renderBoard(errMsg)
		} else {
			renderFullPage(errMsg)
		}
		return
	}
	if err := app.processGuess(ctx, c, sessionID, game, guess, isHTMX, hint); err != nil {
		errMsg = err.Error()
		if isHTMX {
			renderBoard(errMsg)
		} else {
			renderFullPage(errMsg)
		}
		return
	}
}

// gameStateHandler renders the current game board as an HTML fragment.
func (app *App) gameStateHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	game := app.getGameState(ctx, sessionID)
	hint := app.getHintForWord(game.SessionWord)

	csrfToken, _ := c.Cookie("csrf_token")
	c.HTML(http.StatusOK, "game-content", gin.H{
		"game":       game,
		"hint":       hint,
		"csrf_token": csrfToken,
	})
}

// retryWordHandler resets the game state for the current session but keeps the same word.
func (app *App) retryWordHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	app.SessionMutex.Lock()
	game, exists := app.GameSessions[sessionID]
	if !exists {
		app.SessionMutex.Unlock()
		app.createNewGame(ctx, sessionID)
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	sessionWord := game.SessionWord
	guesses := lo.Times(MaxGuesses, func(_ int) []GuessResult {
		return lo.Times(WordLength, func(_ int) GuessResult { return GuessResult{} })
	})
	newGame := &GameState{
		Guesses:        guesses,
		CurrentRow:     0,
		GameOver:       false,
		Won:            false,
		TargetWord:     "",
		SessionWord:    sessionWord,
		GuessHistory:   []string{},
		LastAccessTime: time.Now(),
	}
	app.GameSessions[sessionID] = newGame
	app.SessionMutex.Unlock()
	c.Redirect(http.StatusSeeOther, "/")
}

// healthzHandler returns a JSON health check with server stats.
func (app *App) healthzHandler(c *gin.Context) {
	uptime := time.Since(app.StartTime)
	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"env":            map[bool]string{true: "production", false: "development"}[app.IsProduction],
		"words_loaded":   len(app.WordList),
		"accepted_words": len(app.AcceptedWordSet),
		"uptime":         formatUptime(uptime),
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
}

// validateGameState returns an error if the game is already over.
// The gin.Context parameter is included for future extensibility and best practice, but is currently unused.
func (app *App) validateGameState(_ *gin.Context, game *GameState) error {
	if game.GameOver {
		logWarn("Session attempted guess on completed game")
		return errors.New(ErrorGameOver)
	}
	return nil
}

// normalizeGuess trims and uppercases a guess string for comparison.
func normalizeGuess(input string) string {
	return strings.ToUpper(strings.TrimSpace(input))
}

func (app *App) processGuess(ctx context.Context, c *gin.Context, sessionID string, game *GameState, guess string, isHTMX bool, hint string) error {
	logInfo("Session %s guessed: %s (attempt %d/%d)", sessionID, guess, game.CurrentRow+1, MaxGuesses)

	if len(guess) != WordLength {
		logWarn("Session %s submitted invalid length guess: %s (%d letters)", sessionID, guess, len(guess))
		return errors.New(ErrorInvalidLength)
	}

	if game.CurrentRow >= MaxGuesses {
		logWarn("Session %s attempted guess after max guesses reached", sessionID)
		return errors.New(ErrorNoMoreGuesses)
	}

	targetWord := app.getTargetWord(ctx, game)
	isInvalid := !app.isValidWord(guess)
	result := checkGuess(guess, targetWord)
	app.updateGameState(ctx, game, guess, targetWord, result, isInvalid)
	app.saveGameState(sessionID, game)

	if isHTMX {
		c.HTML(http.StatusOK, "game-content", gin.H{"game": game, "hint": hint})
	} else {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title":   "Vortludo - A Libre Wordle Clone",
			"message": "Guess the 5-letter word!",
			"hint":    hint,
			"game":    game,
		})
	}
	return nil
}
