package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	cachecontrol "go.eigsys.de/gin-cachecontrol/v2"

	"github.com/gin-gonic/gin"
)

// Global application state
var (
	wordList     []WordEntry                   // Valid 5-letter words with hints for the game
	wordStrings  []string                      // Just the word strings for validation
	dailyWord    DailyWord                     // Current daily word with thread safety
	gameSessions = make(map[string]*GameState) // Session-based game storage
	sessionMutex sync.RWMutex                  // Protects gameSessions map
	isProduction bool                          // Environment flag for static file serving
)

func main() {
	// Determine environment for proper asset serving
	isProduction = os.Getenv("GIN_MODE") == "release" || os.Getenv("ENV") == "production"
	log.Printf("Starting Vortludo in %s mode", map[bool]string{true: "production", false: "development"}[isProduction])

	// Load game data
	if err := loadWords(); err != nil {
		log.Fatalf("Failed to load words: %v", err)
	}
	log.Printf("Loaded %d words from dictionary", len(wordList))

	if err := loadDailyWord(); err != nil {
		log.Fatalf("Failed to load daily word: %v", err)
	}
	log.Printf("Daily word loaded for date: %s", dailyWord.GetDate())

	// Start daily word rotation scheduler
	go dailyWordScheduler()

	// Start session cleanup scheduler (every hour, removes sessions older than 2 hours)
	go sessionCleanupScheduler()

	// Setup web server
	router := gin.Default()
	router.SetTrustedProxies([]string{"127.0.0.1"})

	// Apply cache control middleware BEFORE loading templates and static files
	if isProduction {
		// Cache static assets for 1 day, but not HTML pages
		router.Use(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/static/") {
				cachecontrol.New(cachecontrol.Config{
					Public:    true,
					MaxAge:    cachecontrol.Duration(24 * time.Hour),
					Immutable: true,
				})(c)
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

	// Serve static files from appropriate directory
	if isProduction && dirExists("dist") {
		router.LoadHTMLGlob("dist/templates/*.html")
		router.Static("/static", "./dist/static")
	} else {
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

	// Create string-only slice for validation
	wordStrings = make([]string, len(wordList))
	for i, entry := range wordList {
		wordStrings[i] = entry.Word
	}

	log.Printf("Successfully loaded %d words", len(wordList))
	return nil
}

// loadDailyWord loads or creates today's word
func loadDailyWord() error {
	log.Printf("Loading daily word from data/daily-word.json")
	data, err := os.ReadFile("data/daily-word.json")
	if err != nil {
		// File doesn't exist, create with random word
		log.Printf("Daily word file not found, creating new daily word")
		return setNewDailyWord()
	}

	var dwj DailyWordJSON
	if err := json.Unmarshal(data, &dwj); err != nil {
		return err
	}

	dailyWord.FromJSON(dwj)

	// Check if word is still valid for today
	today := time.Now().Format("2006-01-02")
	if dailyWord.GetDate() != today {
		log.Printf("Daily word expired (was %s, now %s), generating new word", dailyWord.GetDate(), today)
		return setNewDailyWord()
	}

	log.Printf("Daily word is current for date: %s", today)
	return nil
}

// setNewDailyWord generates and saves a new daily word
func setNewDailyWord() error {
	dailyWord.mu.Lock()
	defer dailyWord.mu.Unlock()

	// Pick random word entry
	selectedEntry := wordList[rand.Intn(len(wordList))]
	dailyWord.Word = selectedEntry.Word
	dailyWord.Date = time.Now().Format("2006-01-02")
	dailyWord.Hint = selectedEntry.Hint

	log.Printf("Generated new daily word: %s (hint: %s) for date: %s", selectedEntry.Word, selectedEntry.Hint, dailyWord.Date)

	// Save to file (using unsafe version since we hold the lock)
	dwj := dailyWord.toJSONUnsafe()
	data, err := json.MarshalIndent(dwj, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile("data/daily-word.json", data, 0644)
}

// dailyWordScheduler updates the daily word at midnight
func dailyWordScheduler() {
	log.Printf("Daily word scheduler started")
	for {
		now := time.Now()
		next := now.Add(24 * time.Hour)
		next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, next.Location())

		timeUntilNext := next.Sub(now)
		log.Printf("Next daily word update scheduled in %v (at %v)", timeUntilNext, next)
		timer := time.NewTimer(timeUntilNext)
		<-timer.C

		log.Printf("Updating daily word at scheduled time")
		if err := setNewDailyWord(); err != nil {
			log.Printf("Failed to set new daily word: %v", err)
		} else {
			log.Printf("Successfully updated daily word")
		}
	}
}

// sessionCleanupScheduler removes old session files every hour
func sessionCleanupScheduler() {
	log.Printf("Session cleanup scheduler started")
	for {
		timer := time.NewTimer(time.Hour)
		<-timer.C

		log.Printf("Running session cleanup")
		if err := cleanupOldSessions(2 * time.Hour); err != nil {
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

	// Use the game's hint if it has one, otherwise use daily word hint
	hint := dailyWord.GetHint()
	if game.SessionWord != "" {
		// Find the hint for this game's word
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

	// Force a completely new session by clearing the cookie
	c.SetCookie("session_id", "", -1, "/", "", false, true)

	// Create completely new session with current timestamp
	newSessionID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63())
	c.SetCookie("session_id", newSessionID, 7200, "/", "", false, true)
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
	game := getGameState(sessionID)
	
	guess := strings.ToUpper(strings.TrimSpace(c.PostForm("guess")))
	log.Printf("Session %s guessed: %s (attempt %d/6)", sessionID, guess, game.CurrentRow+1)

	if game.GameOver {
		log.Printf("Session %s attempted guess on completed game", sessionID)
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "Game is over!"})
		return
	}

	// Validate guess length
	if len(guess) != 5 {
		log.Printf("Session %s submitted invalid length guess: %s (%d letters)", sessionID, guess, len(guess))
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "Word must be 5 letters!"})
		return
	}

	// Use the game's target word instead of daily word
	targetWord := game.SessionWord
	if targetWord == "" {
		// Fallback to daily word for existing sessions
		targetWord = dailyWord.GetWord()
		game.SessionWord = targetWord
	}

	// Check guess against target word (always done for color feedback)
	result := checkGuess(guess, targetWord)

	// Handle invalid words (not in dictionary)
	if !isValidWord(guess) {
		log.Printf("Session %s guessed invalid word: %s", sessionID, guess)
		// Store guess with color feedback but mark as invalid
		game.Guesses[game.CurrentRow] = result
		game.GuessHistory = append(game.GuessHistory, guess)
		game.CurrentRow++

		// Check for game over
		if game.CurrentRow >= 6 {
			game.GameOver = true
			game.TargetWord = targetWord
		}

		saveGameState(sessionID, game)
		c.HTML(http.StatusOK, "game-board", gin.H{
			"game":  game,
			"error": "Not in word list!",
		})
		return
	}

	// Process valid guess
	game.Guesses[game.CurrentRow] = result
	game.GuessHistory = append(game.GuessHistory, guess)

	// Check for win condition
	if guess == targetWord {
		game.Won = true
		game.GameOver = true
		log.Printf("Session %s won the game! Target word was: %s", sessionID, targetWord)
	} else {
		game.CurrentRow++
		if game.CurrentRow >= 6 {
			game.GameOver = true
			log.Printf("Session %s lost the game. Target word was: %s", sessionID, targetWord)
		}
	}

	// Reveal target word when game ends
	if game.GameOver {
		game.TargetWord = targetWord
	}

	saveGameState(sessionID, game)
	c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
}

// gameStateHandler returns current game state (for HTMX)
func gameStateHandler(c *gin.Context) {
	// Add cache control headers
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	// Use the game's hint if it has one, otherwise use daily word hint
	hint := dailyWord.GetHint()
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
	result := make([]GuessResult, 5)
	targetCopy := []rune(target)

	// First pass: mark exact matches (green)
	for i := range 5 {
		if guess[i] == target[i] {
			result[i] = GuessResult{Letter: string(guess[i]), Status: "correct"}
			targetCopy[i] = ' ' // Mark as used
		}
	}

	// Second pass: mark present letters in wrong position (yellow)
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

// isValidWord checks if a word exists in the word list
func isValidWord(word string) bool {
	return slices.Contains(wordStrings, word)
}

// Session management functions

// getOrCreateSession retrieves or creates a session ID cookie
func getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		sessionID = fmt.Sprintf("%d", time.Now().UnixNano())
		// Set cookie for 2 hours to match session cleanup
		c.SetCookie("session_id", sessionID, 7200, "/", "", false, true)
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
	selectedEntry := wordList[rand.Intn(len(wordList))]

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
		game.Guesses[i] = make([]GuessResult, 5)
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
