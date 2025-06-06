package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
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

// Game configuration constants
const (
	MaxGuesses     = 6
	WordLength     = 5
	SessionTimeout = 2 * time.Hour
	CookieMaxAge   = 2 * time.Hour
	StaticCacheAge = 5 * time.Minute
	sessionCookie  = "session_id"
)

// Global application state
var (
	wordList     []WordEntry                   // Valid words with hints for the game
	wordSet      map[string]struct{}           // For O(1) word validation
	gameSessions = make(map[string]*GameState) // Session-based game storage
	sessionMutex sync.RWMutex                  // Protects gameSessions map
	isProduction bool                          // Environment flag for static file serving
)

func main() {
	// Determine environment
	isProduction = os.Getenv("GIN_MODE") == "release" || os.Getenv("ENV") == "production"
	log.Printf("Starting Vortludo in %s mode", map[bool]string{true: "production", false: "development"}[isProduction])

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

	// Apply cache control middleware
	if isProduction {
		router.Use(func(c *gin.Context) {
			applyCacheHeaders(c, true)
		})
	} else {
		router.Use(func(c *gin.Context) {
			applyCacheHeaders(c, false)
		})
	}

	// Serve static files with appropriate assets for environment
	if isProduction && dirExists("dist") {
		log.Printf("Serving minified assets from dist/ directory")
		router.LoadHTMLGlob("dist/templates/*.html")
		router.Static("/static", "./dist/static")
	} else {
		log.Printf("Serving development assets from source directories")
		router.LoadHTMLGlob("templates/*.html")
		router.Static("/static", "./static")
	}

	// Register template functions
	funcMap := template.FuncMap{
		"hasPrefix": strings.HasPrefix,
	}
	router.SetFuncMap(funcMap)

	// Define routes
	router.GET("/", homeHandler)
	router.GET("/new-game", newGameHandler)
	router.POST("/new-game", newGameHandler)
	router.POST("/guess", guessHandler)
	router.GET("/game-state", gameStateHandler)
	router.POST("/retry-word", retryWordHandler)

	// Start server with graceful shutdown
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Graceful shutdown handler
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

// applyCacheHeaders sets appropriate cache headers based on environment
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

// getHintForWord retrieves the hint for a given word
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

	// Remove old session data
	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
	log.Printf("Cleared old session data for: %s", sessionID)

	// Remove session file
	if sessionFile, err := getSecureSessionPath(sessionID); err == nil {
		os.Remove(sessionFile)
	}

	// Create completely new session if requested
	if c.Query("reset") == "1" {
		c.SetCookie("session_id", "", -1, "/", "", false, true)
		newSessionID := uuid.NewString()
		c.SetCookie("session_id", newSessionID, int(CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session ID: %s", newSessionID)
		createNewGame(newSessionID)
	} else {
		createNewGame(sessionID)
	}
	c.Redirect(http.StatusSeeOther, "/")
}

// guessHandler processes a player's word guess
func guessHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	if err := validateGameState(c, game); err != nil {
		return
	}

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

	targetWord := getTargetWord(game)
	isInvalid := !isValidWord(guess)
	result := checkGuess(guess, targetWord)
	updateGameState(game, guess, targetWord, result, isInvalid)
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
	game.LastAccessTime = time.Now()

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

// gameStateHandler returns current game state for HTMX
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

	// First pass: mark exact matches
	for i := 0; i < WordLength; i++ {
		if guess[i] == target[i] {
			result[i] = GuessResult{Letter: string(guess[i]), Status: "correct"}
			targetCopy[i] = ' ' // Mark as used
		}
	}

	// Second pass: mark present letters in wrong position
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

// getOrCreateSession retrieves or creates a session ID cookie
func getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie(sessionCookie)
	if err != nil || len(sessionID) < 10 {
		sessionID = uuid.NewString()
		c.SetCookie(sessionCookie, sessionID, int(CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session: %s", sessionID)
	}
	return sessionID
}

// getGameState retrieves or creates a game state for a session
func getGameState(sessionID string) *GameState {
	sessionMutex.Lock()
	game, exists := gameSessions[sessionID]
	if exists {
		game.LastAccessTime = time.Now()
		log.Printf("Retrieved cached game state for session: %s, updated last access time.", sessionID)
	}
	sessionMutex.Unlock()

	if exists {
		return game
	}

	// Development mode: create fresh game
	if !isProduction {
		log.Printf("Development mode: creating fresh game for session: %s", sessionID)
		return createNewGame(sessionID)
	}

	// Production: try to load from file
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

// createNewGame creates a new game state with a random word
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
	game.LastAccessTime = time.Now()
	sessionMutex.Unlock()
	log.Printf("Updated in-memory game state for session: %s", sessionID)

	// Save to file for persistence
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

// isValidSessionID validates session ID format (UUID)
func isValidSessionID(sessionID string) bool {
	if len(sessionID) != 36 {
		return false
	}
	for i, c := range sessionID {
		switch {
		case (i == 8 || i == 13 || i == 18 || i == 23):
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
			// Valid hex character
		default:
			return false
		}
	}
	return true
}

// getSecureSessionPath returns a validated file path for a session, preventing path traversal
func getSecureSessionPath(sessionID string) (string, error) {
	if !isValidSessionID(sessionID) {
		return "", fmt.Errorf("invalid session ID format")
	}

	// Clean the sessionID to ensure it doesn't contain path traversal attempts
	cleanSessionID := filepath.Base(sessionID)
	if cleanSessionID != sessionID {
		return "", fmt.Errorf("session ID contains invalid path characters")
	}

	// Construct the secure path
	sessionDir := "data/sessions"
	sessionFile := filepath.Join(sessionDir, cleanSessionID+".json")

	// Verify the resulting path is within our expected directory
	absSessionDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return "", err
	}

	absSessionFile, err := filepath.Abs(sessionFile)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(absSessionFile, absSessionDir) {
		return "", fmt.Errorf("session path would escape sessions directory")
	}

	return sessionFile, nil
}

// retryWordHandler resets guesses but keeps the same word
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
	// Keep the same word, reset everything else
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

	// Remove stale session file
	if sessionFile, err := getSecureSessionPath(sessionID); err == nil {
		os.Remove(sessionFile)
	}

	c.Redirect(http.StatusSeeOther, "/")
}
