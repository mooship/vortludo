package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	cachecontrol "go.eigsys.de/gin-cachecontrol/v2"

	ginGzip "github.com/gin-contrib/gzip"

	"golang.org/x/time/rate"

	"errors"

	"github.com/gin-gonic/gin"

	"github.com/samber/lo"
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

// Route and error string constants
const (
	SessionCookieName  = "session_id"
	RouteHome          = "/"
	RouteNewGame       = "/new-game"
	RouteRetryWord     = "/retry-word"
	RouteGuess         = "/guess"
	RouteGameState     = "/game-state"
	ErrorGameOver      = "game is over"
	ErrorInvalidLength = "word must be 5 letters"
	ErrorNoMoreGuesses = "no more guesses allowed"
	ErrorNotInWordList = "word not recognized"
)

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

// main is the entry point for the application. It loads configuration, sets up routes, and starts the server.
func main() {
	_ = godotenv.Load()

	isProduction := os.Getenv("GIN_MODE") == "release" || os.Getenv("ENV") == "production"
	logInfo("Starting Vortludo in %s mode", map[bool]string{true: "production", false: "development"}[isProduction])

	wordList, wordSet, err := loadWords()
	if err != nil {
		logFatal("Failed to load words: %v", err)
	}
	logInfo("Loaded %d words from dictionary", len(wordList))

	acceptedWordSet, err := loadAcceptedWords()
	if err != nil {
		logFatal("Failed to load accepted words: %v", err)
	}
	logInfo("Loaded %d accepted words", len(acceptedWordSet))

	hintMap := buildHintMap(wordList)

	app := &App{
		WordList:        wordList,
		WordSet:         wordSet,
		AcceptedWordSet: acceptedWordSet,
		HintMap:         hintMap,
		GameSessions:    make(map[string]*GameState),
		IsProduction:    isProduction,
		StartTime:       time.Now(),
		CookieMaxAge:    getEnvDuration("COOKIE_MAX_AGE", 2*time.Hour),
		StaticCacheAge:  getEnvDuration("STATIC_CACHE_AGE", 5*time.Minute),
		RateLimitRPS:    getEnvInt("RATE_LIMIT_RPS", 5),
		RateLimitBurst:  getEnvInt("RATE_LIMIT_BURST", 10),
		LimiterMap:      make(map[string]*rate.Limiter),
	}

	router := gin.Default()

	// Inject request ID into context for each request
	router.Use(requestIDMiddleware())

	// Enable gzip compression for static assets
	router.Use(ginGzip.Gzip(ginGzip.DefaultCompression,
		ginGzip.WithExcludedExtensions([]string{".svg", ".ico", ".png", ".jpg", ".jpeg", ".gif"}),
		ginGzip.WithExcludedPaths([]string{"/static/fonts"})))

	// Set trusted proxy for Gin
	if err := router.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
		logWarn("Failed to set trusted proxies: %v", err)
	}

	// Apply cache headers depending on environment
	if isProduction {
		router.Use(func(c *gin.Context) {
			app.applyCacheHeaders(c, true)
		})
	} else {
		router.Use(func(c *gin.Context) {
			app.applyCacheHeaders(c, false)
		})
	}

	// Serve templates and static assets from appropriate directories
	if isProduction && dirExists("dist") {
		logInfo("Serving assets from dist/ directory")
		router.LoadHTMLGlob("dist/templates/*.html")
		router.Static("/static", "./dist/static")
	} else {
		logInfo("Serving development assets from source directories")
		router.LoadHTMLGlob("templates/*.html")
		router.Static("/static", "./static")
	}

	// Add template functions
	funcMap := template.FuncMap{
		"hasPrefix": strings.HasPrefix,
	}
	router.SetFuncMap(funcMap)

	// Register HTTP routes
	router.GET("/", app.homeHandler)
	router.GET("/new-game", app.newGameHandler)
	router.POST("/new-game", app.rateLimitMiddleware(), app.newGameHandler)
	router.POST("/guess", app.rateLimitMiddleware(), app.guessHandler)
	router.GET("/game-state", app.gameStateHandler)
	router.POST("/retry-word", app.rateLimitMiddleware(), app.retryWordHandler)
	router.GET("/healthz", app.healthzHandler)

	app.startServer(router)
}

// startServer launches the HTTP server and handles graceful shutdown on SIGINT/SIGTERM.
func (app *App) startServer(router *gin.Engine) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, syscall.SIGINT, syscall.SIGTERM)
		<-sigint
		logInfo("Shutdown signal received, shutting down server gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logWarn("HTTP server Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	logInfo("Server starting on http://localhost:%s", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logFatal("Server failed to start: %v", err)
	}
	<-idleConnsClosed
	logInfo("Server shutdown complete")
}

// applyCacheHeaders sets HTTP cache headers for static and dynamic content based on environment.
func (app *App) applyCacheHeaders(c *gin.Context, production bool) {
	if production {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") {
			cachecontrol.New(cachecontrol.Config{
				Public: true,
				MaxAge: cachecontrol.Duration(app.StaticCacheAge),
			})(c)
			c.Header("Vary", "Accept-Encoding")
		} else {
			cachecontrol.New(cachecontrol.Config{
				NoStore:        true,
				NoCache:        true,
				MustRevalidate: true,
			})(c)
		}
	} else {
		cachecontrol.New(cachecontrol.Config{
			NoStore:        true,
			NoCache:        true,
			MustRevalidate: true,
		})(c)
	}
}

// loadWords loads the playable words from data/words.json and returns a filtered list and set.
func loadWords() ([]WordEntry, map[string]struct{}, error) {
	logInfo("Loading words from data/words.json")
	data, err := os.ReadFile("data/words.json")
	if err != nil {
		return nil, nil, err
	}
	var wl WordList
	if err := json.Unmarshal(data, &wl); err != nil {
		return nil, nil, err
	}
	wordList := lo.Filter(wl.Words, func(entry WordEntry, _ int) bool {
		if len(entry.Word) != 5 {
			logWarn("Skipping word %q: not 5 letters", entry.Word)
			return false
		}
		return true
	})
	wordSet := make(map[string]struct{}, len(wordList))
	lo.ForEach(wordList, func(entry WordEntry, _ int) {
		wordSet[entry.Word] = struct{}{}
	})
	logInfo("Successfully loaded %d words", len(wordList))
	return wordList, wordSet, nil
}

// loadAcceptedWords loads the accepted guess words from data/accepted_words.txt.
func loadAcceptedWords() (map[string]struct{}, error) {
	logInfo("Loading accepted words from data/accepted_words.txt")
	data, err := os.ReadFile("data/accepted_words.txt")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	acceptedWordSet := make(map[string]struct{}, len(lines))
	for _, w := range lines {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		acceptedWordSet[strings.ToUpper(w)] = struct{}{}
	}
	return acceptedWordSet, nil
}

// getRandomWordEntry returns a random WordEntry from the loaded word list.
func (app *App) getRandomWordEntry(ctx context.Context) WordEntry {
	reqID, _ := ctx.Value(requestIDKey).(string)
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

// homeHandler renders the main game page for the current session.
func (app *App) homeHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	game := app.getGameState(ctx, sessionID)
	hint := app.getHintForWord(game.SessionWord)

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":   "Vortludo - A Libre Wordle Clone",
		"message": "Guess the 5-letter word!",
		"hint":    hint,
		"game":    game,
	})
}

// newGameHandler starts a new game session, optionally resetting the session ID.
func (app *App) newGameHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	logInfo("Creating new game for session: %s", sessionID)

	app.SessionMutex.Lock()
	delete(app.GameSessions, sessionID)
	app.SessionMutex.Unlock()
	logInfo("Cleared old session data for: %s", sessionID)

	if c.Query("reset") == "1" {
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, "", -1, "/", "", false, true)
		newSessionID := uuid.NewString()
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, newSessionID, int(app.CookieMaxAge.Seconds()), "/", "", false, true)
		logInfo("Created new session ID: %s", newSessionID)
		app.createNewGame(ctx, newSessionID)
	} else {
		app.createNewGame(ctx, sessionID)
	}
	c.Redirect(http.StatusSeeOther, RouteHome)
}

// guessHandler processes a guess submission, validates it, and updates the game state.
func (app *App) guessHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	game := app.getGameState(ctx, sessionID)
	hint := app.getHintForWord(game.SessionWord)

	renderBoard := func(errMsg string) {
		c.HTML(http.StatusOK, "game-board", gin.H{
			"game":  game,
			"hint":  hint,
			"error": errMsg,
		})
	}

	renderFullPage := func(errMsg string) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title":   "Vortludo - A Libre Wordle Clone",
			"message": "Guess the 5-letter word!",
			"hint":    hint,
			"game":    game,
			"error":   errMsg,
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
		errMsg = "Word not accepted, please try another word"
		if isHTMX {
			renderBoard(errMsg)
		} else {
			renderFullPage(errMsg)
		}
		return
	}

	if slices.Contains(game.GuessHistory, guess) {
		errMsg = "You already guessed that word"
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

// validateGameState returns an error if the game is already over.
// The gin.Context parameter is included for future extensibility and best practice, but is currently unused.
func (app *App) validateGameState(_ *gin.Context, game *GameState) error {
	if game.GameOver {
		logWarn("session attempted guess on completed game")
		return errors.New("game is already over, please start a new game")
	}
	return nil
}

// normalizeGuess trims and uppercases a guess string for comparison.
func normalizeGuess(input string) string {
	return strings.ToUpper(strings.TrimSpace(input))
}

// processGuess applies a guess to the game state, updates session, and renders the result.
func (app *App) processGuess(ctx context.Context, c *gin.Context, sessionID string, game *GameState, guess string, isHTMX bool, hint string) error {
	logInfo("session %s guessed: %s (attempt %d/%d)", sessionID, guess, game.CurrentRow+1, MaxGuesses)

	if len(guess) != WordLength {
		logWarn("session %s submitted invalid length guess: %s (%d letters)", sessionID, guess, len(guess))
		return errors.New("word must be 5 letters")
	}

	if game.CurrentRow >= MaxGuesses {
		logWarn("session %s attempted guess after max guesses reached", sessionID)
		return errors.New("no more guesses allowed")
	}

	targetWord := app.getTargetWord(ctx, game)
	isInvalid := !app.isValidWord(guess)
	result := checkGuess(guess, targetWord)
	app.updateGameState(ctx, game, guess, targetWord, result, isInvalid)
	app.saveGameState(sessionID, game)

	if isHTMX {
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "hint": hint})
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
	if game.CurrentRow >= MaxGuesses {
		return
	}
	game.Guesses[game.CurrentRow] = result
	game.GuessHistory = append(game.GuessHistory, guess)
	game.LastAccessTime = time.Now()

	if !isInvalid && guess == targetWord {
		game.Won = true
		game.GameOver = true
		if reqID != "" {
			logInfo("[request_id=%v] Player won! Target word was: %s", reqID, targetWord)
		} else {
			logInfo("Player won! Target word was: %s", targetWord)
		}
	} else {
		game.CurrentRow++
		if game.CurrentRow >= MaxGuesses {
			game.GameOver = true
			if reqID != "" {
				logInfo("[request_id=%v] Player lost. Target word was: %s", reqID, targetWord)
			} else {
				logInfo("Player lost. Target word was: %s", targetWord)
			}
		}
	}

	if game.GameOver {
		game.TargetWord = targetWord
	}
}

// gameStateHandler renders the current game board as an HTML fragment.
func (app *App) gameStateHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	game := app.getGameState(ctx, sessionID)
	hint := app.getHintForWord(game.SessionWord)

	c.HTML(http.StatusOK, "game-board", gin.H{
		"game": game,
		"hint": hint,
	})
}

// checkGuess compares a guess to the target word and returns per-letter results.
func checkGuess(guess, target string) []GuessResult {
	result := make([]GuessResult, WordLength)
	targetCopy := []rune(target)

	for i := 0; i < WordLength; i++ {
		if guess[i] == target[i] {
			result[i] = GuessResult{Letter: string(guess[i]), Status: GuessStatusCorrect}
			targetCopy[i] = ' '
		}
	}

	for i := 0; i < WordLength; i++ {
		if result[i].Status == "" {
			letter := string(guess[i])
			result[i].Letter = letter

			found := false
			for j := 0; j < WordLength; j++ {
				if targetCopy[j] == rune(guess[i]) {
					result[i].Status = GuessStatusPresent
					targetCopy[j] = ' '
					found = true
					break
				}
			}

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

// getOrCreateSession retrieves the session ID from the cookie or creates a new one.
func (app *App) getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie(SessionCookieName)
	if err != nil || len(sessionID) < 10 {
		sessionID = uuid.NewString()
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, sessionID, int(app.CookieMaxAge.Seconds()), "/", "", false, true)
		logInfo("Created new session: %s", sessionID)
	}
	return sessionID
}

// getGameState retrieves or creates the GameState for a session.
func (app *App) getGameState(ctx context.Context, sessionID string) *GameState {
	app.SessionMutex.RLock()
	game, exists := app.GameSessions[sessionID]
	app.SessionMutex.RUnlock()
	if exists {
		app.SessionMutex.Lock()
		game.LastAccessTime = time.Now()
		app.SessionMutex.Unlock()
		logInfo("Retrieved cached game state for session: %s, updated last access time.", sessionID)
		return game
	}

	logInfo("Creating new game for session: %s", sessionID)
	return app.createNewGame(ctx, sessionID)
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

	app.SessionMutex.Lock()
	app.GameSessions[sessionID] = game
	app.SessionMutex.Unlock()

	return game
}

// saveGameState updates the in-memory game state for a session.
func (app *App) saveGameState(sessionID string, game *GameState) {
	app.SessionMutex.Lock()
	app.GameSessions[sessionID] = game
	game.LastAccessTime = time.Now()
	app.SessionMutex.Unlock()
	logInfo("Updated in-memory game state for session: %s", sessionID)
}

// dirExists returns true if the given path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		logWarn("Error checking directory existence: %v", err)
		return false
	}
	return info.IsDir()
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
	app.SessionMutex.Unlock()

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

	app.SessionMutex.Lock()
	app.GameSessions[sessionID] = newGame
	app.SessionMutex.Unlock()

	c.Redirect(http.StatusSeeOther, "/")
}

// getLimiter returns a rate limiter for the given key (usually client IP).
func (app *App) getLimiter(key string) *rate.Limiter {
	app.LimiterMutex.Lock()
	defer app.LimiterMutex.Unlock()
	if lim, ok := app.LimiterMap[key]; ok {
		return lim
	}
	if key == "" || key == "::1" {
		logWarn("Rate limiter key is empty or loopback: %q", key)
	}
	lim := rate.NewLimiter(rate.Every(time.Second/time.Duration(app.RateLimitRPS)), app.RateLimitBurst)
	app.LimiterMap[key] = lim
	return lim
}

// rateLimitMiddleware returns a Gin middleware that enforces per-client rate limiting.
func (app *App) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !app.getLimiter(key).Allow() {
			if c.GetHeader("HX-Request") == "true" {
				c.Header("HX-Trigger", "rate-limit-exceeded")
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests. Please slow down."})
			return
		}
		c.Next()
	}
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

// formatUptime returns a human-readable string for a duration.
func formatUptime(d time.Duration) string {
	seconds := int(d.Seconds()) % 60
	minutes := int(d.Minutes()) % 60
	hours := int(d.Hours())
	switch {
	case hours > 0:
		return fmt.Sprintf("%d hour%s, %d minute%s, %d second%s",
			hours, plural(hours),
			minutes, plural(minutes),
			seconds, plural(seconds))
	case minutes > 0:
		return fmt.Sprintf("%d minute%s, %d second%s",
			minutes, plural(minutes),
			seconds, plural(seconds))
	default:
		return fmt.Sprintf("%d second%s", seconds, plural(seconds))
	}
}

// plural returns "s" if n != 1, otherwise "".
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// getEnvDuration reads a time.Duration from the environment or returns a fallback.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		logWarn("Invalid duration for %s: %v, using default %v", key, err, fallback)
		return fallback
	}
	return d
}

// getEnvInt reads an int from the environment or returns a fallback.
func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	var i int
	_, err := fmt.Sscanf(val, "%d", &i)
	if err != nil {
		logWarn("Invalid int for %s: %v, using default %d", key, err, fallback)
		return fallback
	}
	return i
}

const requestIDKey contextKey = "request_id"

// requestIDMiddleware injects a request ID into the context for each request.
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.Request.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		ctx := context.WithValue(c.Request.Context(), requestIDKey, reqID)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-Id", reqID)
		c.Next()
	}
}

// logInfo logs an info-level message.
func logInfo(format string, v ...any) {
	log.Printf("[INFO] "+format, v...)
}

// logWarn logs a warning-level message.
func logWarn(format string, v ...any) {
	log.Printf("[WARN] "+format, v...)
}

// logFatal logs a fatal error and exits.
func logFatal(format string, v ...any) {
	log.Fatalf("[FATAL] "+format, v...)
}
