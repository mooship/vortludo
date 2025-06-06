package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	cachecontrol "go.eigsys.de/gin-cachecontrol/v2"

	"github.com/gin-gonic/gin"
)

// Constants for game configuration
const (
	MaxGuesses     = 6
	WordLength     = 5
	SessionTimeout = 2 * time.Hour
	CookieMaxAge   = 2 * time.Hour
	StaticCacheAge = 5 * time.Minute // Decreased cache time for static assets
	sessionCookie  = "session_id"
)

// Global application state
var (
	wordList     []WordEntry                   // Valid 5-letter words with hints for the game
	wordSet      map[string]struct{}           // For O(1) word validation
	gameSessions = make(map[string]*GameState) // Session-based game storage
	sessionMutex sync.RWMutex                  // Protects gameSessions map
	isProduction bool                          // Environment flag for static file serving
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

	// Start session cleanup scheduler (every hour, removes sessions older than 2 hours)
	go sessionCleanupScheduler()

	// Setup web server
	router := gin.Default()
	router.SetTrustedProxies([]string{"127.0.0.1"})

	// Apply cache control middleware BEFORE loading templates and static files
	if isProduction {
		router.Use(func(c *gin.Context) {
			applyCacheHeaders(c, true)
		})
	} else {
		router.Use(func(c *gin.Context) {
			applyCacheHeaders(c, false)
		})
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

	// Define routes
	router.GET("/", homeHandler)
	router.GET("/new-game", newGameHandler)
	router.POST("/new-game", newGameHandler)
	router.POST("/guess", guessHandler)
	router.GET("/game-state", gameStateHandler)

	// Start server with graceful shutdown
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Graceful shutdown
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

// Helper to apply cache headers
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

// loadWords reads the word list from JSON file
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

	wordSet = make(map[string]struct{}, len(wordList))
	for _, entry := range wordList {
		wordSet[entry.Word] = struct{}{}
	}

	log.Printf("Successfully loaded %d words", len(wordList))
	return nil
}

// getRandomWordEntry returns a random word entry from the word list
func getRandomWordEntry() WordEntry {
	return wordList[rand.Intn(len(wordList))]
}

// sessionCleanupScheduler removes old session files and in-memory sessions every hour
func sessionCleanupScheduler() {
	log.Printf("Session cleanup scheduler started")
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		log.Printf("Running session cleanup")
		// File cleanup
		if err := cleanupOldSessions(SessionTimeout); err != nil {
			log.Printf("Failed to cleanup old session files: %v", err)
		} else {
			log.Printf("Session file cleanup completed successfully")
		}

		// In-memory cleanup
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

// getHintForWord retrieves the hint for a given word.
func getHintForWord(wordValue string) string {
	if wordValue == "" {
		return ""
	}
	for _, entry := range wordList {
		if entry.Word == wordValue {
			return entry.Hint
		}
	}
	// This case should ideally not be reached if wordValue is always a valid word from a game.
	log.Printf("Warning: Hint not found for word: %s", wordValue)
	return ""
}

// homeHandler serves the main game page
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

// newGameHandler resets the current session's game with a new word
func newGameHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	log.Printf("Creating new game for session: %s", sessionID)

	// Remove old session data completely
	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
	log.Printf("Cleared old session data for: %s", sessionID)

	// Also remove session file
	sessionFile := filepath.Join("data/sessions", sessionID+".json")
	os.Remove(sessionFile)

	// Only create a new session ID if explicitly requested (e.g., via query param "reset=1")
	if c.Query("reset") == "1" {
		// Force a completely new session by clearing the cookie
		c.SetCookie("session_id", "", -1, "/", "", false, true)
		// Create completely new session with UUID
		newSessionID := uuid.NewString()
		c.SetCookie("session_id", newSessionID, int(CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session ID: %s", newSessionID)
		createNewGame(newSessionID)
	} else {
		// Just create a new game for the current session
		createNewGame(sessionID)
	}
	c.Redirect(http.StatusSeeOther, "/")
}

// guessHandler processes a player's word guess
func guessHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	// Validate game state
	if err := validateGameState(c, game); err != nil {
		return
	}

	// Process guess
	guess := normalizeGuess(c.PostForm("guess"))
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

// normalizeGuess converts input to uppercase and trims whitespace
func normalizeGuess(input string) string {
	return strings.ToUpper(strings.TrimSpace(input))
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

	// Prevent guess overflow
	if game.CurrentRow >= MaxGuesses {
		log.Printf("Session %s attempted guess after max guesses reached", sessionID)
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "No more guesses allowed!"})
		return fmt.Errorf("guess overflow")
	}

	// Get target word
	targetWord := getTargetWord(game)

	// Only call isValidWord once
	isInvalid := !isValidWord(guess)

	// Process the guess
	result := checkGuess(guess, targetWord)
	updateGameState(game, guess, targetWord, result, isInvalid)

	// Save and render
	saveGameState(sessionID, game)

	if isInvalid {
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
	if game.CurrentRow >= MaxGuesses {
		return // Prevent out-of-bounds write
	}
	game.Guesses[game.CurrentRow] = result
	game.GuessHistory = append(game.GuessHistory, guess)
	game.LastAccessTime = time.Now() // Update last access time on guess

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
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)
	hint := getHintForWord(game.SessionWord)

	c.HTML(http.StatusOK, "game-board", gin.H{
		"game": game,
		"hint": hint,
	})
}

// checkGuess implements Wordle's letter comparison algorithm
func checkGuess(guess, target string) []GuessResult {
	result := make([]GuessResult, WordLength)
	targetCopy := []rune(target)

	// First pass: mark exact matches (green)
	for i := 0; i < WordLength; i++ {
		if guess[i] == target[i] {
			result[i] = GuessResult{Letter: string(guess[i]), Status: "correct"}
			targetCopy[i] = ' ' // Mark as used
		}
	}

	// Second pass: mark present letters in wrong position (yellow)
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

// isValidWord checks if a word exists in the word list
func isValidWord(word string) bool {
	_, ok := wordSet[word]
	return ok
}

// Session management functions

// getOrCreateSession retrieves or creates a session ID cookie
func getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie(sessionCookie)
	if err != nil || len(sessionID) < 10 {
		sessionID = uuid.NewString()
		// Set cookie for 2 hours to match session cleanup
		c.SetCookie(sessionCookie, sessionID, int(CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session: %s", sessionID)
	}
	return sessionID
}

// getGameState retrieves or creates a game state for a session
func getGameState(sessionID string) *GameState {
	sessionMutex.Lock() // Use full lock for read-then-potential-write of LastAccessTime
	game, exists := gameSessions[sessionID]
	if exists {
		game.LastAccessTime = time.Now()
		log.Printf("Retrieved cached game state for session: %s, updated last access time.", sessionID)
	}
	sessionMutex.Unlock()

	if exists {
		return game
	}

	// For debugging: don't load from file initially, always create fresh
	// This will prevent loading stale sessions during development
	if !isProduction {
		log.Printf("Development mode: creating fresh game for session: %s", sessionID)
		return createNewGame(sessionID)
	}

	// In production, try to load from file only if we have a valid sessionID
	if sessionID != "" && len(sessionID) > 10 {
		log.Printf("Attempting to load game state from file for session: %s", sessionID)
		if game, err := loadGameSessionFromFile(sessionID); err == nil {
			// Validate the loaded game state
			if game.SessionWord != "" && len(game.Guesses) == MaxGuesses {
				// game.LastAccessTime is set by loadGameSessionFromFile
				sessionMutex.Lock()
				gameSessions[sessionID] = game // Cache in memory
				sessionMutex.Unlock()
				log.Printf("Successfully loaded and cached game state for session: %s (last access time updated by load)", sessionID)
				return game
			} else {
				log.Printf("Loaded game state for session %s was invalid (word: '%s', guesses len: %d), creating new game", sessionID, game.SessionWord, len(game.Guesses))
			}
		} else {
			log.Printf("Failed to load game state for session %s: %v", sessionID, err)
		}
	}

	// Create new game if not found anywhere or invalid
	log.Printf("Creating new game for session: %s", sessionID)
	return createNewGame(sessionID)
}

// createNewGame creates a new game state with a random word
func createNewGame(sessionID string) *GameState {
	// Pick a random word for this game session
	selectedEntry := getRandomWordEntry()

	log.Printf("New game created for session %s with word: %s (hint: %s)", sessionID, selectedEntry.Word, selectedEntry.Hint)

	game := &GameState{
		Guesses:        make([][]GuessResult, MaxGuesses),
		CurrentRow:     0,
		GameOver:       false,
		Won:            false,
		TargetWord:     "",
		SessionWord:    selectedEntry.Word, // The actual target word for this session
		GuessHistory:   []string{},
		LastAccessTime: time.Now(), // Set last access time on creation
	}

	// Initialize empty guess rows
	for i := range game.Guesses {
		game.Guesses[i] = make([]GuessResult, WordLength)
	}

	sessionMutex.Lock()
	gameSessions[sessionID] = game
	sessionMutex.Unlock()

	return game
}

// saveGameState updates the stored game state for a session
func saveGameState(sessionID string, game *GameState) {
	sessionMutex.Lock()
	gameSessions[sessionID] = game
	game.LastAccessTime = time.Now() // Ensure LastAccessTime is current when saving
	sessionMutex.Unlock()
	log.Printf("Updated in-memory game state for session: %s", sessionID)

	// Also save to file for persistence across server restarts
	// Validate sessionID before using in path expression for defense-in-depth
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

// dirExists checks if a directory path exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func isValidSessionID(sessionID string) bool {
	// Accept only UUIDs (36 chars, hex + dashes)
	if len(sessionID) != 36 {
		return false
	}
	for i, c := range sessionID {
		switch {
		case (i == 8 || i == 13 || i == 18 || i == 23):
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'):
			// valid
		default:
			return false
		}
	}
	return true
}
