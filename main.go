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
	"path/filepath"
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
)

// Game configuration (defaults, can be overridden by env)
var (
	MaxGuesses     = 6
	WordLength     = 5
	SessionTimeout = getEnvDuration("SESSION_TIMEOUT", 2*time.Hour)
	CookieMaxAge   = getEnvDuration("COOKIE_MAX_AGE", 2*time.Hour)
	StaticCacheAge = getEnvDuration("STATIC_CACHE_AGE", 5*time.Minute)
	RateLimitRPS   = getEnvInt("RATE_LIMIT_RPS", 5) // requests per second
	RateLimitBurst = getEnvInt("RATE_LIMIT_BURST", 10)
)

// Route and error constants
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

// Global state
var (
	wordList        []WordEntry
	wordSet         map[string]struct{}
	acceptedWordSet map[string]struct{}
	gameSessions    = make(map[string]*GameState)
	sessionMutex    sync.RWMutex
	isProduction    bool
	limiterMap      = make(map[string]*rate.Limiter)
	limiterMutex    sync.Mutex
	startTime       = time.Now()
)

func main() {
	_ = godotenv.Load()

	// Determine environment.
	isProduction = os.Getenv("GIN_MODE") == "release" || os.Getenv("ENV") == "production"
	log.Printf("Starting Vortludo in %s mode", map[bool]string{true: "production", false: "development"}[isProduction])

	// Load game data from JSON files.
	if err := loadWords(); err != nil {
		log.Fatalf("Failed to load words: %v", err)
	}
	log.Printf("Loaded %d words from dictionary", len(wordList))

	// Load accepted words list
	if err := loadAcceptedWords(); err != nil {
		log.Fatalf("Failed to load accepted words: %v", err)
	}
	log.Printf("Loaded %d accepted words", len(acceptedWordSet))

	// Clean up expired sessions on startup.
	log.Printf("Performing startup session cleanup")
	if err := cleanupOldSessions(SessionTimeout); err != nil {
		log.Printf("Warning: Failed to cleanup old sessions on startup: %v", err)
	}

	// Start session cleanup scheduler in background.
	go sessionCleanupScheduler()

	// Setup web server.
	router := gin.Default()

	// Add GZIP middleware for JS and CSS files.
	router.Use(ginGzip.Gzip(ginGzip.DefaultCompression,
		ginGzip.WithExcludedExtensions([]string{".svg", ".ico", ".png", ".jpg", ".jpeg", ".gif"}),
		ginGzip.WithExcludedPaths([]string{"/static/fonts"})))

	if err := router.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
		log.Printf("Warning: Failed to set trusted proxies: %v", err)
	}

	// Apply cache control middleware.
	if isProduction {
		router.Use(func(c *gin.Context) {
			applyCacheHeaders(c, true)
		})
	} else {
		router.Use(func(c *gin.Context) {
			applyCacheHeaders(c, false)
		})
	}

	// Serve static files with appropriate assets for environment.
	if isProduction && dirExists("dist") {
		log.Printf("Serving assets from dist/ directory")
		router.LoadHTMLGlob("dist/templates/*.html")
		router.Static("/static", "./dist/static")
	} else {
		log.Printf("Serving development assets from source directories")
		router.LoadHTMLGlob("templates/*.html")
		router.Static("/static", "./static")
	}

	// Register template functions.
	funcMap := template.FuncMap{
		"hasPrefix": strings.HasPrefix,
	}
	router.SetFuncMap(funcMap)

	// Define routes.
	router.GET("/", homeHandler)
	router.GET("/new-game", newGameHandler)
	router.POST("/new-game", rateLimitMiddleware(), newGameHandler)
	router.POST("/guess", rateLimitMiddleware(), guessHandler)
	router.GET("/game-state", gameStateHandler)
	router.POST("/retry-word", rateLimitMiddleware(), retryWordHandler)
	router.GET("/health", healthHandler)

	// Start server with graceful shutdown.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second, // Prevent Slowloris attacks
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown handler.
	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, syscall.SIGINT, syscall.SIGTERM)
		<-sigint
		log.Println("Shutdown signal received, shutting down server gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("HTTP server Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	log.Printf("Server starting on http://localhost:%s", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
	<-idleConnsClosed
	log.Println("Server shutdown complete")
}

// applyCacheHeaders sets cache headers for static and dynamic content
func applyCacheHeaders(c *gin.Context, production bool) {
	if production {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") {
			cachecontrol.New(cachecontrol.Config{
				Public: true,
				MaxAge: cachecontrol.Duration(StaticCacheAge),
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

// loadWords loads the word list from JSON file
func loadWords() error {
	log.Printf("Loading words from data/words.json")
	data, err := os.ReadFile("data/words.json")
	if err != nil {
		return err
	}

	var wl WordList
	if err := json.Unmarshal(data, &wl); err != nil {
		return err
	}

	// Only keep words that are exactly 5 characters long
	filtered := make([]WordEntry, 0, len(wl.Words))
	for _, entry := range wl.Words {
		if len(entry.Word) == 5 {
			filtered = append(filtered, entry)
		} else {
			log.Printf("Skipping word %q: not 5 letters", entry.Word)
		}
	}
	wordList = filtered

	wordSet = make(map[string]struct{}, len(wordList))
	for _, entry := range wordList {
		wordSet[entry.Word] = struct{}{}
	}

	log.Printf("Successfully loaded %d words", len(wordList))
	return nil
}

// loadAcceptedWords loads accepted words from JSON file
func loadAcceptedWords() error {
	log.Printf("Loading accepted words from data/accepted_words.json")
	data, err := os.ReadFile("data/accepted_words.json")
	if err != nil {
		return err
	}
	var accepted []string
	if err := json.Unmarshal(data, &accepted); err != nil {
		return err
	}
	acceptedWordSet = make(map[string]struct{}, len(accepted))
	for _, w := range accepted {
		acceptedWordSet[strings.ToUpper(w)] = struct{}{}
	}
	return nil
}

// getRandomWordEntry returns a random word entry from the word list
func getRandomWordEntry() WordEntry {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(wordList))))
	if err != nil {
		// Fallback to first word if crypto/rand fails.
		log.Printf("Error generating random number: %v, using fallback", err)
		return wordList[0]
	}
	return wordList[n.Int64()]
}

// sessionCleanupScheduler periodically cleans up expired sessions
func sessionCleanupScheduler() {
	log.Printf("Session cleanup scheduler started")
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		log.Printf("Running session cleanup")
		// File cleanup from disk.
		if err := cleanupOldSessions(SessionTimeout); err != nil {
			log.Printf("Failed to cleanup old session files: %v", err)
		} else {
			log.Printf("Session file cleanup completed successfully")
		}

		// In-memory cleanup from cache.
		sessionMutex.Lock()
		cleanedInMemoryCount := 0
		now := time.Now()
		for sessionID, game := range gameSessions {
			if now.Sub(game.LastAccessTime) > SessionTimeout {
				delete(gameSessions, sessionID)
				cleanedInMemoryCount++
				log.Printf("Removed expired in-memory session: %s (last access: %v ago)", sessionID, now.Sub(game.LastAccessTime))
			}
		}
		sessionMutex.Unlock()
		if cleanedInMemoryCount > 0 {
			log.Printf("In-memory session cleanup removed %d sessions.", cleanedInMemoryCount)
		}
	}
}

// getHintForWord returns the hint for a given word
func getHintForWord(wordValue string) string {
	if wordValue == "" {
		return ""
	}
	for _, entry := range wordList {
		if entry.Word == wordValue {
			return entry.Hint
		}
	}
	log.Printf("Warning: Hint not found for word: %s", wordValue)
	return ""
}

// homeHandler serves the main page
func homeHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)
	hint := getHintForWord(game.SessionWord)

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":   "Vortludo - A Libre Wordle Clone",
		"message": "Guess the 5-letter word!",
		"hint":    hint,
		"game":    game,
	})
}

// newGameHandler starts a new game session
func newGameHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	log.Printf("Creating new game for session: %s", sessionID)

	// Remove old session data from memory and disk.
	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
	log.Printf("Cleared old session data for: %s", sessionID)

	// Remove session file.
	if sessionFile, err := getSecureSessionPath(sessionID); err == nil {
		if err := os.Remove(sessionFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to remove session file: %v", err)
		}
	}

	// Create completely new session if reset parameter is provided.
	if c.Query("reset") == "1" {
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, "", -1, "/", "", false, true)
		newSessionID := uuid.NewString()
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, newSessionID, int(CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session ID: %s", newSessionID)
		createNewGame(newSessionID)
	} else {
		createNewGame(sessionID)
	}
	c.Redirect(http.StatusSeeOther, RouteHome)
}

// guessHandler processes a guess from the user
func guessHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	if err := validateGameState(c, game); err != nil {
		return
	}

	guess := normalizeGuess(c.PostForm("guess"))
	// Check if guess is in accepted list.
	if !isAcceptedWord(guess) {
		// Return board with notAccepted flag for toast.
		c.HTML(http.StatusOK, "game-board", gin.H{
			"game":        game,
			"notAccepted": true,
		})
		return
	}
	if err := processGuess(c, sessionID, game, guess); err != nil {
		return
	}
}

// validateGameState returns an error if the game is over
func validateGameState(c *gin.Context, game *GameState) error {
	if game.GameOver {
		log.Print("session attempted guess on completed game")
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
		return errors.New(ErrorGameOver)
	}
	return nil
}

// normalizeGuess returns the guess in uppercase and trimmed
func normalizeGuess(input string) string {
	return strings.ToUpper(strings.TrimSpace(input))
}

// processGuess processes a guess and updates the game state
func processGuess(c *gin.Context, sessionID string, game *GameState, guess string) error {
	log.Printf("session %s guessed: %s (attempt %d/%d)", sessionID, guess, game.CurrentRow+1, MaxGuesses)

	if len(guess) != WordLength {
		log.Printf("session %s submitted invalid length guess: %s (%d letters)", sessionID, guess, len(guess))
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
		return errors.New(ErrorInvalidLength)
	}

	if game.CurrentRow >= MaxGuesses {
		log.Printf("session %s attempted guess after max guesses reached", sessionID)
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
		return errors.New(ErrorNoMoreGuesses)
	}

	targetWord := getTargetWord(game)
	isInvalid := !isValidWord(guess)
	result := checkGuess(guess, targetWord)
	updateGameState(game, guess, targetWord, result, isInvalid)
	saveGameState(sessionID, game)

	c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
	return nil
}

// getTargetWord returns the target word for the session, assigns if missing
func getTargetWord(game *GameState) string {
	if game.SessionWord == "" {
		selectedEntry := getRandomWordEntry()
		game.SessionWord = selectedEntry.Word
		log.Printf("Warning: SessionWord was empty, assigned random word: %s", selectedEntry.Word)
	}
	return game.SessionWord
}

// updateGameState updates the game state after a guess
func updateGameState(game *GameState, guess, targetWord string, result []GuessResult, isInvalid bool) {
	if game.CurrentRow >= MaxGuesses {
		return // Prevent out-of-bounds write
	}
	game.Guesses[game.CurrentRow] = result
	game.GuessHistory = append(game.GuessHistory, guess)
	game.LastAccessTime = time.Now()

	// Check for win condition.
	if !isInvalid && guess == targetWord {
		game.Won = true
		game.GameOver = true
		log.Printf("Player won! Target word was: %s", targetWord)
	} else {
		game.CurrentRow++
		if game.CurrentRow >= MaxGuesses {
			game.GameOver = true
			log.Printf("Player lost. Target word was: %s", targetWord)
		}
	}

	// Reveal target word when game ends.
	if game.GameOver {
		game.TargetWord = targetWord
	}
}

// gameStateHandler returns the current game state
func gameStateHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)
	hint := getHintForWord(game.SessionWord)

	c.HTML(http.StatusOK, "game-board", gin.H{
		"game": game,
		"hint": hint,
	})
}

// checkGuess compares guess to target and returns the result for each letter
func checkGuess(guess, target string) []GuessResult {
	result := make([]GuessResult, WordLength)
	targetCopy := []rune(target)

	// First pass: mark exact matches.
	for i := 0; i < WordLength; i++ {
		if guess[i] == target[i] {
			result[i] = GuessResult{Letter: string(guess[i]), Status: "correct"}
			targetCopy[i] = ' ' // Mark as used
		}
	}

	// Second pass: mark present letters in wrong positions.
	for i := 0; i < WordLength; i++ {
		if result[i].Status == "" {
			letter := string(guess[i])
			result[i].Letter = letter

			found := false
			for j := 0; j < WordLength; j++ {
				if targetCopy[j] == rune(guess[i]) {
					result[i].Status = "present"
					targetCopy[j] = ' ' // Mark as used
					found = true
					break
				}
			}

			if !found {
				result[i].Status = "absent"
			}
		}
	}

	return result
}

// isValidWord returns true if the word exists in the word set
func isValidWord(word string) bool {
	_, ok := wordSet[word]
	return ok
}

// isAcceptedWord returns true if the word is in the accepted word set
func isAcceptedWord(word string) bool {
	_, ok := acceptedWordSet[word]
	return ok
}

// getOrCreateSession gets or creates a session cookie
func getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie(SessionCookieName)
	if err != nil || len(sessionID) < 10 {
		sessionID = uuid.NewString()
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, sessionID, int(CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session: %s", sessionID)
	}
	return sessionID
}

// getGameState gets or creates a game state for the session
func getGameState(sessionID string) *GameState {
	sessionMutex.RLock()
	game, exists := gameSessions[sessionID]
	sessionMutex.RUnlock()
	if exists {
		// Lock for writing LastAccessTime to avoid race
		sessionMutex.Lock()
		game.LastAccessTime = time.Now()
		sessionMutex.Unlock()
		log.Printf("Retrieved cached game state for session: %s, updated last access time.", sessionID)
		return game
	}

	// Development mode: create fresh game.
	if !isProduction {
		log.Printf("Development mode: creating fresh game for session: %s", sessionID)
		return createNewGame(sessionID)
	}

	// Production: try to load from file.
	if sessionID != "" && len(sessionID) > 10 {
		log.Printf("Attempting to load game state from file for session: %s", sessionID)
		if game, err := loadGameSessionFromFile(sessionID); err == nil {
			if game.SessionWord != "" && len(game.Guesses) == MaxGuesses {
				sessionMutex.Lock()
				gameSessions[sessionID] = game
				sessionMutex.Unlock()
				log.Printf("Successfully loaded and cached game state for session: %s", sessionID)
				return game
			} else {
				log.Printf("Loaded game state for session %s was invalid, creating new game", sessionID)
			}
		} else {
			log.Printf("Failed to load game state for session %s: %v", sessionID, err)
		}
	}

	log.Printf("Creating new game for session: %s", sessionID)
	return createNewGame(sessionID)
}

// createNewGame creates a new game state for the session
func createNewGame(sessionID string) *GameState {
	selectedEntry := getRandomWordEntry()

	log.Printf("New game created for session %s with word: %s (hint: %s)", sessionID, selectedEntry.Word, selectedEntry.Hint)

	game := &GameState{
		Guesses:        make([][]GuessResult, MaxGuesses),
		CurrentRow:     0,
		GameOver:       false,
		Won:            false,
		TargetWord:     "",
		SessionWord:    selectedEntry.Word,
		GuessHistory:   []string{},
		LastAccessTime: time.Now(),
	}

	// Initialize empty guess rows.
	for i := range game.Guesses {
		game.Guesses[i] = make([]GuessResult, WordLength)
	}

	sessionMutex.Lock()
	gameSessions[sessionID] = game
	sessionMutex.Unlock()

	return game
}

// saveGameState saves the game state in memory and to disk
func saveGameState(sessionID string, game *GameState) {
	sessionMutex.Lock()
	gameSessions[sessionID] = game
	game.LastAccessTime = time.Now()
	sessionMutex.Unlock()
	log.Printf("Updated in-memory game state for session: %s", sessionID)

	// Save to file for persistence with validation.
	if isValidSessionID(sessionID) {
		if err := saveGameSessionToFile(sessionID, game); err != nil {
			log.Printf("Failed to save session %s to file: %v", sessionID, err)
		} else {
			log.Printf("Successfully saved game state to file for session: %s", sessionID)
		}
	} else {
		log.Printf("Refused to save session to file: invalid sessionID format: %s", sessionID)
	}
}

// dirExists returns true if the directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		log.Printf("Error checking directory existence: %v", err)
		return false
	}
	return info.IsDir()
}

// isValidSessionID validates the session ID format (UUID)
func isValidSessionID(sessionID string) bool {
	if len(sessionID) != 36 {
		return false
	}
	for i, c := range sessionID {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
			// Valid hex character.
		default:
			return false
		}
	}
	return true
}

// getSecureSessionPath returns a safe session file path for the session ID
func getSecureSessionPath(sessionID string) (string, error) {
	if !isValidSessionID(sessionID) {
		return "", errors.New("invalid session ID format")
	}

	// Build path using only the validated session ID.
	sessionDir := "data/sessions"
	sessionFile := filepath.Join(sessionDir, sessionID+".json")

	// Normalize path to resolve any traversal attempts.
	sessionFile = filepath.Clean(sessionFile)

	// Resolve to absolute paths for secure comparison.
	absSessionDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sessions directory: %w", err)
	}

	absSessionFile, err := filepath.Abs(sessionFile)
	if err != nil {
		return "", fmt.Errorf("failed to resolve session file path: %w", err)
	}

	// Ensure file path remains within sessions directory.
	absSessionDir = filepath.Clean(absSessionDir) + string(filepath.Separator)
	if !strings.HasPrefix(absSessionFile, absSessionDir) {
		return "", errors.New("session path would escape sessions directory")
	}

	// Verify filename matches expected pattern.
	expectedFilename := sessionID + ".json"
	actualFilename := filepath.Base(absSessionFile)
	if actualFilename != expectedFilename {
		return "", fmt.Errorf("session filename mismatch: expected %s, got %s", expectedFilename, actualFilename)
	}

	return sessionFile, nil
}

// retryWordHandler resets guesses but keeps the word for the session
func retryWordHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	sessionMutex.Lock()
	game, exists := gameSessions[sessionID]
	if !exists {
		sessionMutex.Unlock()
		createNewGame(sessionID)
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	// Keep the same word, reset everything else.
	sessionWord := game.SessionWord
	sessionMutex.Unlock()

	newGame := &GameState{
		Guesses:        make([][]GuessResult, MaxGuesses),
		CurrentRow:     0,
		GameOver:       false,
		Won:            false,
		TargetWord:     "",
		SessionWord:    sessionWord,
		GuessHistory:   []string{},
		LastAccessTime: time.Now(),
	}
	for i := range newGame.Guesses {
		newGame.Guesses[i] = make([]GuessResult, WordLength)
	}

	sessionMutex.Lock()
	gameSessions[sessionID] = newGame
	sessionMutex.Unlock()

	// Remove stale session file.
	if sessionFile, err := getSecureSessionPath(sessionID); err == nil {
		if err := os.Remove(sessionFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to remove session file: %v", err)
		}
	}

	c.Redirect(http.StatusSeeOther, "/")
}

// getLimiter returns a rate limiter for the given key (IP)
func getLimiter(key string) *rate.Limiter {
	limiterMutex.Lock()
	defer limiterMutex.Unlock()
	if lim, ok := limiterMap[key]; ok {
		return lim
	}
	// Use env-configurable rate and burst.
	lim := rate.NewLimiter(rate.Every(time.Second/time.Duration(RateLimitRPS)), RateLimitBurst)
	limiterMap[key] = lim
	return lim
}

// rateLimitMiddleware applies rate limiting to requests
func rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !getLimiter(key).Allow() {
			// For HTMX requests, return an error response that can be handled by the client
			if c.GetHeader("HX-Request") == "true" {
				c.Header("HX-Trigger", "rate-limit-exceeded")
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
		c.Next()
	}
}

// healthHandler returns health and status information
func healthHandler(c *gin.Context) {
	sessionMutex.RLock()
	sessionCount := len(gameSessions)
	sessionMutex.RUnlock()
	uptime := time.Since(startTime)
	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"env":            map[bool]string{true: "production", false: "development"}[isProduction],
		"words_loaded":   len(wordList),
		"accepted_words": len(acceptedWordSet),
		"sessions_count": sessionCount,
		"uptime":         formatUptime(uptime),
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
}

// formatUptime returns a human-friendly uptime string
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

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		log.Printf("Invalid duration for %s: %v, using default %v", key, err, fallback)
		return fallback
	}
	return d
}

func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	var i int
	_, err := fmt.Sscanf(val, "%d", &i)
	if err != nil {
		log.Printf("Invalid int for %s: %v, using default %d", key, err, fallback)
		return fallback
	}
	return i
}
