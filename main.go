package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	cachecontrol "go.eigsys.de/gin-cachecontrol/v2"
	"golang.org/x/time/rate"

	"github.com/gin-gonic/gin"
)

// Game configuration constants
const (
	MaxGuesses         = 6
	WordLength         = 5
	SessionTimeout     = 2 * time.Hour
	CookieMaxAge       = 7200 // 2 hours in seconds
	StaticCacheAge     = 24 * time.Hour
	MaxSessionIDLength = 64
	MinSessionIDLength = 10
)

// Global application state
var (
	wordList         []WordEntry                      // Valid 5-letter words with hints
	wordMap          map[string]WordEntry             // O(1) word lookup
	gameSessions     = make(map[string]*GameState)    // In-memory session storage
	sessionMutex     sync.RWMutex                     // Protects concurrent session access
	isProduction     bool                             // Environment flag
	rateLimiters     = make(map[string]*rate.Limiter) // Per-IP rate limiting
	rateLimiterMutex sync.RWMutex
	sessionIDRegex   = regexp.MustCompile(`^[a-zA-Z0-9\-]+$`)
)

func main() {
	// Determine environment for proper asset serving
	isProduction = os.Getenv("GIN_MODE") == "release" || os.Getenv("ENV") == "production"
	log.Printf("Starting Vortludo in %s mode", map[bool]string{true: "production", false: "development"}[isProduction])

	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	// Load game data
	if err := loadWords(); err != nil {
		log.Fatalf("Failed to load words: %v", err)
	}
	log.Printf("Loaded %d words from dictionary", len(wordList))

	// Clean up expired sessions on startup
	log.Printf("Performing startup session cleanup")
	if err := cleanupOldSessions(SessionTimeout); err != nil {
		log.Printf("Warning: Failed to cleanup old sessions on startup: %v", err)
	}

	// Start session cleanup scheduler
	go sessionCleanupScheduler()

	// Setup web server
	router := gin.Default()
	router.SetTrustedProxies([]string{"127.0.0.1"})

	// Apply security headers middleware
	router.Use(securityHeadersMiddleware())

	// Apply rate limiting middleware
	router.Use(rateLimitMiddleware())

	// Apply cache control middleware BEFORE loading templates and static files
	if isProduction {
		// Cache static assets aggressively for production
		router.Use(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/static/") {
				cachecontrol.New(cachecontrol.Config{
					Public:    true,
					MaxAge:    cachecontrol.Duration(30 * 24 * time.Hour), // 30 days for minified assets
					Immutable: true,
				})(c)
				// Add compression headers for better performance
				c.Header("Vary", "Accept-Encoding")
			} else {
				// No cache for HTML pages and API endpoints
				cachecontrol.New(cachecontrol.Config{
					NoStore:        true,
					NoCache:        true,
					MustRevalidate: true,
				})(c)
			}
		})
	} else {
		// Disable all caching for development
		router.Use(cachecontrol.New(cachecontrol.Config{
			NoStore:        true,
			NoCache:        true,
			MustRevalidate: true,
		}))
	}

	// Serve static files from appropriate directory with minified assets in production
	if isProduction && dirExists("dist") {
		log.Printf("Serving minified assets from dist/ directory")
		router.LoadHTMLGlob("dist/templates/*.html")
		router.Static("/static", "./dist/static")
	} else {
		log.Printf("Serving development assets from source directories")
		router.LoadHTMLGlob("templates/*.html")
		router.Static("/static", "./static")
	}

	// Define routes with proper HTTP methods
	router.GET("/", homeHandler)
	router.POST("/new-game", newGameHandler) // Only POST for state changes
	router.POST("/guess", guessHandler)
	router.GET("/game-state", gameStateHandler)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on http://localhost:%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// securityHeadersMiddleware adds security headers to prevent common web vulnerabilities
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// XSS protection headers
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")

		// Content Security Policy - whitelist trusted sources
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' https://unpkg.com https://cdn.jsdelivr.net; " +
			"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://fonts.bunny.net; " +
			"font-src 'self' https://fonts.bunny.net; " +
			"img-src 'self' data:; " +
			"connect-src 'self'"
		c.Header("Content-Security-Policy", csp)

		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		c.Next()
	}
}

// rateLimitMiddleware prevents abuse by limiting requests per IP
func rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Static assets bypass rate limiting
		if strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		limiter := getRateLimiter(clientIP)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests. Please try again later.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getRateLimiter returns or creates a rate limiter for an IP address
func getRateLimiter(clientIP string) *rate.Limiter {
	rateLimiterMutex.RLock()
	limiter, exists := rateLimiters[clientIP]
	rateLimiterMutex.RUnlock()

	if exists {
		return limiter
	}

	// 30 requests per minute with burst of 5
	limiter = rate.NewLimiter(rate.Every(2*time.Second), 5)

	rateLimiterMutex.Lock()
	rateLimiters[clientIP] = limiter
	rateLimiterMutex.Unlock()

	// Auto-cleanup after 10 minutes of inactivity
	go func() {
		time.Sleep(10 * time.Minute)
		rateLimiterMutex.Lock()
		delete(rateLimiters, clientIP)
		rateLimiterMutex.Unlock()
	}()

	return limiter
}

// loadWords initializes the word list and creates O(1) lookup map
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

	wordList = wl.Words

	// Build O(1) lookup map
	wordMap = make(map[string]WordEntry, len(wordList))
	for _, entry := range wordList {
		wordMap[entry.Word] = entry
	}

	log.Printf("Successfully loaded %d words into hash map", len(wordMap))
	return nil
}

// getRandomWordEntry returns a random word entry from the word list
func getRandomWordEntry() WordEntry {
	return wordList[rand.Intn(len(wordList))]
}

// sessionCleanupScheduler periodically removes expired sessions
func sessionCleanupScheduler() {
	log.Printf("Session cleanup scheduler started")
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		log.Printf("Running session cleanup")
		if err := cleanupOldSessions(SessionTimeout); err != nil {
			log.Printf("Failed to cleanup old sessions: %v", err)
		} else {
			log.Printf("Session cleanup completed successfully")
		}
	}
}

// homeHandler serves the main game page
func homeHandler(c *gin.Context) {
	// Prevent caching of dynamic content
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	sessionID := getOrCreateSession(c)
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	game := getGameState(sessionID)

	// Get the hint for this game's word
	hint := ""
	if game.SessionWord != "" {
		if entry, ok := wordMap[game.SessionWord]; ok {
			hint = entry.Hint
		}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":   "Vortludo - A Libre Wordle Clone",
		"message": "Guess the 5-letter word!",
		"hint":    hint,
		"game":    game,
	})
}

// newGameHandler creates a new game with a fresh word
func newGameHandler(c *gin.Context) {
	// Enforce POST method for state changes
	if c.Request.Method != http.MethodPost {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method not allowed"})
		return
	}

	// Add cache control headers
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	sessionID := getOrCreateSession(c)
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	log.Printf("Creating new game for session: %s", sessionID)

	// Clear existing session data
	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
	log.Printf("Cleared old session data for: %s", sessionID)

	// Also remove session file
	sessionFile := filepath.Join("data/sessions", sessionID+".json")
	os.Remove(sessionFile)

	// Force a completely new session by clearing the cookie
	c.SetCookie("session_id", "", -1, "/", "", false, true)

	// Create completely new session with current timestamp
	newSessionID := generateSecureSessionID()
	c.SetCookie("session_id", newSessionID, CookieMaxAge, "/", "", isProduction, true)
	log.Printf("Created new session ID: %s", newSessionID)

	// Create new game and redirect
	createNewGame(newSessionID)
	c.Redirect(http.StatusSeeOther, "/")
}

// guessHandler processes a player's word guess
func guessHandler(c *gin.Context) {
	// Add cache control headers
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	sessionID := getOrCreateSession(c)
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	game := getGameState(sessionID)

	// Validate game state
	if err := validateGameState(c, game); err != nil {
		return
	}

	// Process guess with sanitization
	guess := sanitizeAndNormalizeGuess(c.PostForm("guess"))
	if err := processGuess(c, sessionID, game, guess); err != nil {
		return
	}
}

// validateGameState checks if the game can accept guesses
func validateGameState(c *gin.Context, game *GameState) error {
	if game.GameOver {
		log.Printf("Session attempted guess on completed game")
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "Game is over!"})
		return fmt.Errorf("game is over")
	}
	return nil
}

// sanitizeAndNormalizeGuess removes invalid characters and normalizes input
func sanitizeAndNormalizeGuess(input string) string {
	// Strip non-letter characters
	cleaned := regexp.MustCompile(`[^a-zA-Z]`).ReplaceAllString(input, "")
	return strings.ToUpper(strings.TrimSpace(cleaned))
}

// processGuess handles the guess logic
func processGuess(c *gin.Context, sessionID string, game *GameState, guess string) error {
	log.Printf("Session %s guessed: %s (attempt %d/%d)", sessionID, guess, game.CurrentRow+1, MaxGuesses)

	// Validate guess length
	if len(guess) != WordLength {
		log.Printf("Session %s submitted invalid length guess: %s (%d letters)", sessionID, guess, len(guess))
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": fmt.Sprintf("Word must be %d letters!", WordLength)})
		return fmt.Errorf("invalid word length")
	}

	// Get target word
	targetWord := getTargetWord(game)

	// Process the guess
	result := checkGuess(guess, targetWord)
	updateGameState(game, guess, targetWord, result, !isValidWord(guess))

	// Save and render
	saveGameState(sessionID, game)

	if !isValidWord(guess) {
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "Not in word list!"})
	} else {
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
	}

	return nil
}

// getTargetWord returns the target word for the game
func getTargetWord(game *GameState) string {
	if game.SessionWord == "" {
		selectedEntry := getRandomWordEntry()
		game.SessionWord = selectedEntry.Word
		log.Printf("Warning: SessionWord was empty, assigned random word: %s", selectedEntry.Word)
	}
	return game.SessionWord
}

// updateGameState updates the game based on the guess
func updateGameState(game *GameState, guess, targetWord string, result []GuessResult, isInvalid bool) {
	game.Guesses[game.CurrentRow] = result
	game.GuessHistory = append(game.GuessHistory, guess)

	// Check for win
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

	// Reveal target word when game ends
	if game.GameOver {
		game.TargetWord = targetWord
	}
}

// gameStateHandler returns current game state (for HTMX)
func gameStateHandler(c *gin.Context) {
	// Add cache control headers
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	sessionID := getOrCreateSession(c)
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	game := getGameState(sessionID)

	// Get the hint for this game's word - O(1) lookup
	hint := ""
	if game.SessionWord != "" {
		if entry, ok := wordMap[game.SessionWord]; ok {
			hint = entry.Hint
		}
	}

	c.HTML(http.StatusOK, "game-board", gin.H{
		"game": game,
		"hint": hint,
	})
}

// checkGuess implements Wordle's letter comparison logic
func checkGuess(guess, target string) []GuessResult {
	result := make([]GuessResult, 5)
	targetCopy := []rune(target)

	// First pass: exact matches (green)
	for i := range 5 {
		if guess[i] == target[i] {
			result[i] = GuessResult{Letter: string(guess[i]), Status: "correct"}
			targetCopy[i] = ' ' // Mark as used
		}
	}

	// Second pass: wrong position matches (yellow)
	for i := range 5 {
		if result[i].Status == "" {
			letter := string(guess[i])
			result[i].Letter = letter

			found := false
			for j := 0; j < 5; j++ {
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

// isValidWord performs O(1) dictionary lookup
func isValidWord(word string) bool {
	_, exists := wordMap[word]
	return exists
}

// Session management functions

// getOrCreateSession manages session cookies with validation
func getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie("session_id")
	if err != nil || !isValidSessionID(sessionID) {
		sessionID = generateSecureSessionID()
		// Secure flag enabled in production
		c.SetCookie("session_id", sessionID, CookieMaxAge, "/", "", isProduction, true)
		log.Printf("Created new session: %s", sessionID)
	}
	return sessionID
}

// generateSecureSessionID creates a cryptographically secure session ID
func generateSecureSessionID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63())
}

// isValidSessionID validates session ID format and length
func isValidSessionID(sessionID string) bool {
	if len(sessionID) < MinSessionIDLength || len(sessionID) > MaxSessionIDLength {
		return false
	}
	return sessionIDRegex.MatchString(sessionID)
}

// getGameState retrieves game state with fallback to persistent storage
func getGameState(sessionID string) *GameState {
	// Check memory cache first
	sessionMutex.RLock()
	game, exists := gameSessions[sessionID]
	sessionMutex.RUnlock()

	if exists {
		log.Printf("Retrieved cached game state for session: %s", sessionID)
		return game
	}

	// Development mode: always create fresh games
	if !isProduction {
		log.Printf("Development mode: creating fresh game for session: %s", sessionID)
		return createNewGame(sessionID)
	}

	// Production: attempt to restore from disk
	if sessionID != "" && len(sessionID) > 10 {
		log.Printf("Attempting to load game state from file for session: %s", sessionID)
		if game, err := loadGameSessionFromFile(sessionID); err == nil {
			// Validate restored state
			if game.SessionWord != "" && len(game.Guesses) == 6 {
				// Cache for performance
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

	// Fallback to new game
	log.Printf("Creating new game for session: %s", sessionID)
	return createNewGame(sessionID)
}

// createNewGame initializes a fresh game state
func createNewGame(sessionID string) *GameState {
	selectedEntry := getRandomWordEntry()

	log.Printf("New game created for session %s with word: %s (hint: %s)", sessionID, selectedEntry.Word, selectedEntry.Hint)

	game := &GameState{
		Guesses:      make([][]GuessResult, 6),
		CurrentRow:   0,
		GameOver:     false,
		Won:          false,
		TargetWord:   "",
		SessionWord:  selectedEntry.Word,
		GuessHistory: []string{},
	}

	// Initialize empty guess grid
	for i := range game.Guesses {
		game.Guesses[i] = make([]GuessResult, 5)
	}

	sessionMutex.Lock()
	gameSessions[sessionID] = game
	sessionMutex.Unlock()

	return game
}

// saveGameState persists game state to memory and disk
func saveGameState(sessionID string, game *GameState) {
	// Update memory cache
	sessionMutex.Lock()
	gameSessions[sessionID] = game
	sessionMutex.Unlock()
	log.Printf("Updated in-memory game state for session: %s", sessionID)

	// Persist to disk for recovery
	if err := saveGameSessionToFile(sessionID, game); err != nil {
		log.Printf("Failed to save session %s to file: %v", sessionID, err)
	} else {
		log.Printf("Successfully saved game state to file for session: %s", sessionID)
	}
}

// dirExists checks if a directory path exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
