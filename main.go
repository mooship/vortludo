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

var (
	wordList     []string
	dailyWord    DailyWord
	gameSessions = make(map[string]*GameState)
	sessionMutex sync.RWMutex
	isProduction bool
)

func main() {
	isProduction = os.Getenv("GIN_MODE") == "release" || os.Getenv("ENV") == "production"

	if err := loadWords(); err != nil {
		log.Fatalf("Failed to load words: %v", err)
	}

	if err := loadDailyWord(); err != nil {
		log.Fatalf("Failed to load daily word: %v", err)
	}

	go dailyWordScheduler()

	router := gin.Default()

	if isProduction && dirExists("dist") {
		router.LoadHTMLGlob("dist/templates/*.html")
		router.Static("/static", "./dist/static")
	} else {
		router.LoadHTMLGlob("templates/*.html")
		router.Static("/static", "./static")
	}

	router.GET("/", homeHandler)
	router.GET("/new-game", newGameHandler)
	router.POST("/new-game", newGameHandler)
	router.POST("/guess", guessHandler)
	router.GET("/game-state", gameStateHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on http://localhost:%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

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
	return nil
}

func loadDailyWord() error {
	data, err := os.ReadFile("data/daily-word.json")
	if err != nil {
		// If file doesn't exist, create it with a random word
		return setNewDailyWord()
	}

	var dwj DailyWordJSON
	if err := json.Unmarshal(data, &dwj); err != nil {
		return err
	}

	dailyWord.FromJSON(dwj)

	// Check if the word is from today
	today := time.Now().Format("2006-01-02")
	if dailyWord.GetDate() != today {
		return setNewDailyWord()
	}

	return nil
}

func setNewDailyWord() error {
	dailyWord.mu.Lock()
	defer dailyWord.mu.Unlock()

	dailyWord.Word = wordList[rand.Intn(len(wordList))]
	dailyWord.Date = time.Now().Format("2006-01-02")

	// Use unsafe version since we already hold the lock
	dwj := dailyWord.toJSONUnsafe()

	data, err := json.MarshalIndent(dwj, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile("data/daily-word.json", data, 0644)
}

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

func homeHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":   "Vortludo - A Libre Wordle Clone",
		"message": "Guess the 5-letter word!",
		"game":    game,
	})
}

func newGameHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	resetGameState(sessionID)
	c.Redirect(http.StatusSeeOther, "/")
}

func guessHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	if game.GameOver {
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "Game is over!"})
		return
	}

	guess := strings.ToUpper(strings.TrimSpace(c.PostForm("guess")))

	if len(guess) != 5 {
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "Word must be 5 letters!"})
		return
	}

	if !isValidWord(guess) {
		// Store the invalid guess in the game state
		invalidResult := make([]GuessResult, 5)
		for i := 0; i < 5; i++ {
			invalidResult[i] = GuessResult{
				Letter: string(guess[i]),
				Status: "invalid",
			}
		}
		game.Guesses[game.CurrentRow] = invalidResult
		game.CurrentRow++

		// Check if we've run out of guesses
		if game.CurrentRow >= 6 {
			game.GameOver = true
			game.TargetWord = dailyWord.GetWord()
		}

		saveGameState(sessionID, game)
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game, "error": "Not in word list!"})
		return
	}

	result := checkGuess(guess, dailyWord.GetWord())
	game.Guesses[game.CurrentRow] = result

	if guess == dailyWord.GetWord() {
		game.Won = true
		game.GameOver = true
	} else {
		game.CurrentRow++
		if game.CurrentRow >= 6 {
			game.GameOver = true
		}
	}
	if game.GameOver {
		game.TargetWord = dailyWord.GetWord()
	}

	saveGameState(sessionID, game)
	c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
}

func gameStateHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)
	c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
}

// checkGuess compares the guess against the target word using Wordle rules
func checkGuess(guess, target string) []GuessResult {
	result := make([]GuessResult, 5)
	targetCopy := []rune(target)

	// First pass: mark correct positions
	for i := range 5 {
		if guess[i] == target[i] {
			result[i] = GuessResult{Letter: string(guess[i]), Status: "correct"}
			targetCopy[i] = ' '
		}
	}

	// Second pass: mark present letters
	for i := range 5 {
		if result[i].Status == "" {
			letter := string(guess[i])
			result[i].Letter = letter

			found := false
			for j := 0; j < 5; j++ {
				if targetCopy[j] == rune(guess[i]) {
					result[i].Status = "present"
					targetCopy[j] = ' '
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

func isValidWord(word string) bool {
	return slices.Contains(wordList, word)
}

func getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		sessionID = fmt.Sprintf("%d", time.Now().UnixNano())
		c.SetCookie("session_id", sessionID, 86400, "/", "", false, true)
	}
	return sessionID
}

func getGameState(sessionID string) *GameState {
	sessionMutex.RLock()
	game, exists := gameSessions[sessionID]
	sessionMutex.RUnlock()

	if !exists {
		game = &GameState{
			Guesses:    make([][]GuessResult, 6),
			CurrentRow: 0,
			GameOver:   false,
			Won:        false,
			TargetWord: "",
		}
		for i := range game.Guesses {
			game.Guesses[i] = make([]GuessResult, 5)
		}

		sessionMutex.Lock()
		gameSessions[sessionID] = game
		sessionMutex.Unlock()
	}

	return game
}

func resetGameState(sessionID string) {
	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
}

func saveGameState(sessionID string, game *GameState) {
	sessionMutex.Lock()
	gameSessions[sessionID] = game
	sessionMutex.Unlock()
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
