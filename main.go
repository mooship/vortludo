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

var (
	MaxGuesses     = 6
	WordLength     = 5
	SessionTimeout = getEnvDuration("SESSION_TIMEOUT", 2*time.Hour)
	CookieMaxAge   = getEnvDuration("COOKIE_MAX_AGE", 2*time.Hour)
	StaticCacheAge = getEnvDuration("STATIC_CACHE_AGE", 5*time.Minute)
	RateLimitRPS   = getEnvInt("RATE_LIMIT_RPS", 5)
	RateLimitBurst = getEnvInt("RATE_LIMIT_BURST", 10)
)

const (
	GuessStatusCorrect = "correct"
	GuessStatusPresent = "present"
	GuessStatusAbsent  = "absent"
)

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

var (
	wordList        []WordEntry
	wordSet         map[string]struct{}
	acceptedWordSet map[string]struct{}
	hintMap         map[string]string
	gameSessions    = make(map[string]*GameState)
	sessionMutex    sync.RWMutex
	isProduction    bool
	limiterMap      = make(map[string]*rate.Limiter)
	limiterMutex    sync.Mutex
	startTime       = time.Now()
)

func main() {
	_ = godotenv.Load()

	isProduction = os.Getenv("GIN_MODE") == "release" || os.Getenv("ENV") == "production"
	logInfo("Starting Vortludo in %s mode", map[bool]string{true: "production", false: "development"}[isProduction])

	if err := loadWords(); err != nil {
		logFatal("Failed to load words: %v", err)
	}
	logInfo("Loaded %d words from dictionary", len(wordList))

	if err := loadAcceptedWords(); err != nil {
		logFatal("Failed to load accepted words: %v", err)
	}
	logInfo("Loaded %d accepted words", len(acceptedWordSet))

	buildHintMap()

	router := gin.Default()

	router.Use(ginGzip.Gzip(ginGzip.DefaultCompression,
		ginGzip.WithExcludedExtensions([]string{".svg", ".ico", ".png", ".jpg", ".jpeg", ".gif"}),
		ginGzip.WithExcludedPaths([]string{"/static/fonts"})))

	if err := router.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
		logWarn("Failed to set trusted proxies: %v", err)
	}

	if isProduction {
		router.Use(func(c *gin.Context) {
			applyCacheHeaders(c, true)
		})
	} else {
		router.Use(func(c *gin.Context) {
			applyCacheHeaders(c, false)
		})
	}

	if isProduction && dirExists("dist") {
		logInfo("Serving assets from dist/ directory")
		router.LoadHTMLGlob("dist/templates/*.html")
		router.Static("/static", "./dist/static")
	} else {
		logInfo("Serving development assets from source directories")
		router.LoadHTMLGlob("templates/*.html")
		router.Static("/static", "./static")
	}

	funcMap := template.FuncMap{
		"hasPrefix": strings.HasPrefix,
	}
	router.SetFuncMap(funcMap)

	router.GET("/", homeHandler)
	router.GET("/new-game", newGameHandler)
	router.POST("/new-game", rateLimitMiddleware(), newGameHandler)
	router.POST("/guess", rateLimitMiddleware(), guessHandler)
	router.GET("/game-state", gameStateHandler)
	router.POST("/retry-word", rateLimitMiddleware(), retryWordHandler)
	router.GET("/healthz", healthHandler)

	startServer(router)
}

func startServer(router *gin.Engine) {
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
	wordList = lo.Filter(wl.Words, func(entry WordEntry, _ int) bool {
		if len(entry.Word) != 5 {
			log.Printf("Skipping word %q: not 5 letters", entry.Word)
			return false
		}
		return true
	})
	wordSet = make(map[string]struct{}, len(wordList))
	lo.ForEach(wordList, func(entry WordEntry, _ int) {
		wordSet[entry.Word] = struct{}{}
	})
	log.Printf("Successfully loaded %d words", len(wordList))
	return nil
}

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
	lo.ForEach(accepted, func(w string, _ int) {
		acceptedWordSet[strings.ToUpper(w)] = struct{}{}
	})
	return nil
}

func getRandomWordEntry() WordEntry {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(wordList))))
	if err != nil {
		log.Printf("Error generating random number: %v, using fallback", err)
		return wordList[0]
	}
	return wordList[n.Int64()]
}

func getHintForWord(wordValue string) string {
	if wordValue == "" {
		return ""
	}
	hint, ok := hintMap[wordValue]
	if ok {
		return hint
	}
	logWarn("Hint not found for word: %s", wordValue)
	return ""
}

func buildHintMap() {
	hintMap = lo.Associate(wordList, func(entry WordEntry) (string, string) {
		return entry.Word, entry.Hint
	})
}

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

func newGameHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	log.Printf("Creating new game for session: %s", sessionID)

	sessionMutex.Lock()
	delete(gameSessions, sessionID)
	sessionMutex.Unlock()
	log.Printf("Cleared old session data for: %s", sessionID)

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

func guessHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)

	if err := validateGameState(c, game); err != nil {
		return
	}

	guess := normalizeGuess(c.PostForm("guess"))
	if !isAcceptedWord(guess) {
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

func validateGameState(c *gin.Context, game *GameState) error {
	if game.GameOver {
		log.Print("session attempted guess on completed game")
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
		return errors.New(ErrorGameOver)
	}
	return nil
}

func normalizeGuess(input string) string {
	return strings.ToUpper(strings.TrimSpace(input))
}

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

func getTargetWord(game *GameState) string {
	if game.SessionWord == "" {
		selectedEntry := getRandomWordEntry()
		game.SessionWord = selectedEntry.Word
		log.Printf("Warning: SessionWord was empty, assigned random word: %s", selectedEntry.Word)
	}
	return game.SessionWord
}

func updateGameState(game *GameState, guess, targetWord string, result []GuessResult, isInvalid bool) {
	if game.CurrentRow >= MaxGuesses {
		return
	}
	game.Guesses[game.CurrentRow] = result
	game.GuessHistory = append(game.GuessHistory, guess)
	game.LastAccessTime = time.Now()

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

	if game.GameOver {
		game.TargetWord = targetWord
	}
}

func gameStateHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)
	hint := getHintForWord(game.SessionWord)

	c.HTML(http.StatusOK, "game-board", gin.H{
		"game": game,
		"hint": hint,
	})
}

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

func isValidWord(word string) bool {
	return lo.Contains(lo.Keys(wordSet), word)
}

func isAcceptedWord(word string) bool {
	return lo.Contains(lo.Keys(acceptedWordSet), word)
}

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

func getGameState(sessionID string) *GameState {
	sessionMutex.RLock()
	game, exists := gameSessions[sessionID]
	sessionMutex.RUnlock()
	if exists {
		sessionMutex.Lock()
		game.LastAccessTime = time.Now()
		sessionMutex.Unlock()
		log.Printf("Retrieved cached game state for session: %s, updated last access time.", sessionID)
		return game
	}

	log.Printf("Creating new game for session: %s", sessionID)
	return createNewGame(sessionID)
}

func createNewGame(sessionID string) *GameState {
	selectedEntry := getRandomWordEntry()

	log.Printf("New game created for session %s with word: %s (hint: %s)", sessionID, selectedEntry.Word, selectedEntry.Hint)

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

	sessionMutex.Lock()
	gameSessions[sessionID] = game
	sessionMutex.Unlock()

	return game
}

func saveGameState(sessionID string, game *GameState) {
	sessionMutex.Lock()
	gameSessions[sessionID] = game
	game.LastAccessTime = time.Now()
	sessionMutex.Unlock()
	log.Printf("Updated in-memory game state for session: %s", sessionID)
}

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
	sessionWord := game.SessionWord
	sessionMutex.Unlock()

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

	sessionMutex.Lock()
	gameSessions[sessionID] = newGame
	sessionMutex.Unlock()

	c.Redirect(http.StatusSeeOther, "/")
}

func getLimiter(key string) *rate.Limiter {
	limiterMutex.Lock()
	defer limiterMutex.Unlock()
	if lim, ok := limiterMap[key]; ok {
		return lim
	}
	if key == "" || key == "::1" {
		logWarn("Rate limiter key is empty or loopback: %q", key)
	}
	lim := rate.NewLimiter(rate.Every(time.Second/time.Duration(RateLimitRPS)), RateLimitBurst)
	limiterMap[key] = lim
	return lim
}

func rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !getLimiter(key).Allow() {
			if c.GetHeader("HX-Request") == "true" {
				c.Header("HX-Trigger", "rate-limit-exceeded")
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
		c.Next()
	}
}

func healthHandler(c *gin.Context) {
	uptime := time.Since(startTime)
	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"env":            map[bool]string{true: "production", false: "development"}[isProduction],
		"words_loaded":   len(wordList),
		"accepted_words": len(acceptedWordSet),
		"uptime":         formatUptime(uptime),
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
}

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
		logWarn("Invalid duration for %s: %v, using default %v", key, err, fallback)
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
		logWarn("Invalid int for %s: %v, using default %d", key, err, fallback)
		return fallback
	}
	return i
}

func logInfo(format string, v ...any) {
	log.Printf("[INFO] "+format, v...)
}
func logWarn(format string, v ...any) {
	log.Printf("[WARN] "+format, v...)
}
func logFatal(format string, v ...any) {
	log.Fatalf("[FATAL] "+format, v...)
}
