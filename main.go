package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

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

	// Load game data
	if err := loadWords(); err != nil {
		log.Fatalf("Failed to load words: %v", err)
	}

	if err := loadDailyWord(); err != nil {
		log.Fatalf("Failed to load daily word: %v", err)
	}

	// Start daily word rotation scheduler
	go dailyWordScheduler()

	// Setup web server
	router := gin.Default()

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

	return nil
}

// loadDailyWord loads or creates today's word
func loadDailyWord() error {
	data, err := os.ReadFile("data/daily-word.json")
	if err != nil {
		// File doesn't exist, create with random word
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
		return setNewDailyWord()
	}

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
	for {
		now := time.Now()
		next := now.Add(24 * time.Hour)
		next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, next.Location())

		timer := time.NewTimer(next.Sub(now))
		<-timer.C

		if err := setNewDailyWord(); err != nil {
			log.Printf("Failed to set new daily word: %v", err)
		}
	}
}

// homeHandler serves the main game page
func homeHandler(c *gin.Context) {
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
	sessionID := getOrCreateSession(c)
	createNewGame(sessionID)
	c.Redirect(http.StatusSeeOther, "/")
}

// guessHandler processes a player's word guess
func guessHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	if game.GameOver {
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "Game is over!"})
		return
	}

	guess := strings.ToUpper(strings.TrimSpace(c.PostForm("guess")))

	// Validate guess length
	if len(guess) != 5 {
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
	} else {
		game.CurrentRow++
		if game.CurrentRow >= 6 {
			game.GameOver = true
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
		c.SetCookie("session_id", sessionID, 86400, "/", "", false, true)
	}
	return sessionID
}

// getGameState retrieves or creates a game state for a session
func getGameState(sessionID string) *GameState {
	sessionMutex.RLock()
	game, exists := gameSessions[sessionID]
	sessionMutex.RUnlock()

	if !exists {
		return createNewGame(sessionID)
	}

	return game
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
}

// dirExists checks if a directory path exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
