package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
		// Force browsers to always revalidate static assets
		router.Use(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/static/") {
				cachecontrol.New(cachecontrol.Config{
					NoCache:        true,
					MustRevalidate: true,
					Public:         true,
				})(c)
				c.Header("Vary", "Accept-Encoding")
			} else {
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

	// Define routes
	router.GET("/", homeHandler)
	router.GET("/new-game", newGameHandler)
	router.POST("/new-game", newGameHandler)
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

// sessionCleanupScheduler removes old session files every hour
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
	// Add cache control headers to prevent stale content
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	// Get the hint for this game's word
	hint := ""
	if game.SessionWord != "" {
		for _, entry := range wordList {
			if entry.Word == game.SessionWord {
				hint = entry.Hint
				break
			}
		}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":   "Vortludo - A Libre Wordle Clone",
		"message": "Guess the 5-letter word!",
		"hint":    hint,
		"game":    game,
	})
}

// newGameHandler resets the current session's game with a new word
func newGameHandler(c *gin.Context) {
	// Add cache control headers
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

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
		// Create completely new session with current timestamp
		newSessionID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63())
		c.SetCookie("session_id", newSessionID, 7200, "/", "", false, true)
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
	// Add cache control headers
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

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
		return
	}
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
	game := getGameState(sessionID)

	// Get the hint for this game's word
	hint := ""
	if game.SessionWord != "" {
		for _, entry := range wordList {
			if entry.Word == game.SessionWord {
				hint = entry.Hint
				break
			}
		}
	}

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
	for i := range WordLength {
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
	if err != nil {
		sessionID = fmt.Sprintf("%d", time.Now().UnixNano())
		// Set cookie for 2 hours to match session cleanup
		c.SetCookie(sessionCookie, sessionID, int(CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session: %s", sessionID)
	}
	return sessionID
}

// getGameState retrieves or creates a game state for a session
func getGameState(sessionID string) *GameState {
	sessionMutex.RLock()
	game, exists := gameSessions[sessionID]
	sessionMutex.RUnlock()

	if exists {
		log.Printf("Retrieved cached game state for session: %s", sessionID)
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
			if game.SessionWord != "" && len(game.Guesses) == 6 {
				// Cache in memory for faster access
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
		Guesses:      make([][]GuessResult, 6),
		CurrentRow:   0,
		GameOver:     false,
		Won:          false,
		TargetWord:   "",
		SessionWord:  selectedEntry.Word, // The actual target word for this session
		GuessHistory: []string{},
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
	sessionMutex.Unlock()
	log.Printf("Updated in-memory game state for session: %s", sessionID)

	// Also save to file for persistence across server restarts
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
