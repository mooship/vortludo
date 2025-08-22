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

type WordEntry struct {
	Word string `json:"word"`
	Hint string `json:"hint"`
}

type WordList struct {
	Words []WordEntry `json:"words"`
}

type GameState struct {
	Guesses        [][]GuessResult `json:"guesses"`
	CurrentRow     int             `json:"currentRow"`
	GameOver       bool            `json:"gameOver"`
	Won            bool            `json:"won"`
	TargetWord     string          `json:"targetWord"`
	SessionWord    string          `json:"sessionWord"`
	GuessHistory   []string        `json:"guessHistory"`
	LastAccessTime time.Time       `json:"lastAccessTime"`
}

type GuessResult struct {
	Letter string `json:"letter"`
	Status string `json:"status"`
}

const (
	MaxGuesses = 6
	WordLength = 5
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

type App struct {
	WordList        []WordEntry
	WordSet         map[string]struct{}
	AcceptedWordSet map[string]struct{}
	HintMap         map[string]string
	GameSessions    map[string]*GameState
	SessionMutex    sync.RWMutex
	LimiterMap      map[string]*rate.Limiter
	LimiterMutex    sync.Mutex
	IsProduction    bool
	StartTime       time.Time
	CookieMaxAge    time.Duration
	StaticCacheAge  time.Duration
	RateLimitRPS    int
	RateLimitBurst  int
}

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

	router.Use(ginGzip.Gzip(ginGzip.DefaultCompression,
		ginGzip.WithExcludedExtensions([]string{".svg", ".ico", ".png", ".jpg", ".jpeg", ".gif"}),
		ginGzip.WithExcludedPaths([]string{"/static/fonts"})))

	if err := router.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
		logWarn("Failed to set trusted proxies: %v", err)
	}

	if isProduction {
		router.Use(func(c *gin.Context) {
			app.applyCacheHeaders(c, true)
		})
	} else {
		router.Use(func(c *gin.Context) {
			app.applyCacheHeaders(c, false)
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

	router.GET("/", app.homeHandler)
	router.GET("/new-game", app.newGameHandler)
	router.POST("/new-game", app.rateLimitMiddleware(), app.newGameHandler)
	router.POST("/guess", app.rateLimitMiddleware(), app.guessHandler)
	router.GET("/game-state", app.gameStateHandler)
	router.POST("/retry-word", app.rateLimitMiddleware(), app.retryWordHandler)
	router.GET("/healthz", app.healthzHandler)

	app.startServer(router)
}

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

func loadWords() ([]WordEntry, map[string]struct{}, error) {
	log.Printf("Loading words from data/words.json")
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
			log.Printf("Skipping word %q: not 5 letters", entry.Word)
			return false
		}
		return true
	})
	wordSet := make(map[string]struct{}, len(wordList))
	lo.ForEach(wordList, func(entry WordEntry, _ int) {
		wordSet[entry.Word] = struct{}{}
	})
	log.Printf("Successfully loaded %d words", len(wordList))
	return wordList, wordSet, nil
}

func loadAcceptedWords() (map[string]struct{}, error) {
	log.Printf("Loading accepted words from data/accepted_words.txt")
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

func (app *App) getRandomWordEntry(ctx context.Context) WordEntry {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(app.WordList))))
	if err != nil {
		log.Printf("Error generating random number: %v, using fallback", err)
		return app.WordList[0]
	}
	return app.WordList[n.Int64()]
}

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

func buildHintMap(wordList []WordEntry) map[string]string {
	return lo.Associate(wordList, func(entry WordEntry) (string, string) {
		return entry.Word, entry.Hint
	})
}

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

func (app *App) newGameHandler(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := app.getOrCreateSession(c)
	log.Printf("Creating new game for session: %s", sessionID)

	app.SessionMutex.Lock()
	delete(app.GameSessions, sessionID)
	app.SessionMutex.Unlock()
	log.Printf("Cleared old session data for: %s", sessionID)

	if c.Query("reset") == "1" {
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, "", -1, "/", "", false, true)
		newSessionID := uuid.NewString()
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, newSessionID, int(app.CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session ID: %s", newSessionID)
		app.createNewGame(ctx, newSessionID)
	} else {
		app.createNewGame(ctx, sessionID)
	}
	c.Redirect(http.StatusSeeOther, RouteHome)
}

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
		errMsg = "word not accepted, please try another word"
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

func (app *App) validateGameState(c *gin.Context, game *GameState) error {
	if game.GameOver {
		log.Print("session attempted guess on completed game")
		return errors.New("game is already over, please start a new game")
	}
	return nil
}

func normalizeGuess(input string) string {
	return strings.ToUpper(strings.TrimSpace(input))
}

func (app *App) processGuess(ctx context.Context, c *gin.Context, sessionID string, game *GameState, guess string, isHTMX bool, hint string) error {
	log.Printf("session %s guessed: %s (attempt %d/%d)", sessionID, guess, game.CurrentRow+1, MaxGuesses)

	if len(guess) != WordLength {
		log.Printf("session %s submitted invalid length guess: %s (%d letters)", sessionID, guess, len(guess))
		return errors.New("word must be 5 letters")
	}

	if game.CurrentRow >= MaxGuesses {
		log.Printf("session %s attempted guess after max guesses reached", sessionID)
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

func (app *App) getTargetWord(ctx context.Context, game *GameState) string {
	if game.SessionWord == "" {
		selectedEntry := app.getRandomWordEntry(ctx)
		game.SessionWord = selectedEntry.Word
		log.Printf("Warning: SessionWord was empty, assigned random word: %s", selectedEntry.Word)
	}
	return game.SessionWord
}

func (app *App) updateGameState(ctx context.Context, game *GameState, guess, targetWord string, result []GuessResult, isInvalid bool) {
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

func (app *App) isValidWord(word string) bool {
	_, ok := app.WordSet[word]
	return ok
}

func (app *App) isAcceptedWord(word string) bool {
	_, ok := app.AcceptedWordSet[word]
	return ok
}

func (app *App) getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie(SessionCookieName)
	if err != nil || len(sessionID) < 10 {
		sessionID = uuid.NewString()
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(SessionCookieName, sessionID, int(app.CookieMaxAge.Seconds()), "/", "", false, true)
		log.Printf("Created new session: %s", sessionID)
	}
	return sessionID
}

func (app *App) getGameState(ctx context.Context, sessionID string) *GameState {
	app.SessionMutex.RLock()
	game, exists := app.GameSessions[sessionID]
	app.SessionMutex.RUnlock()
	if exists {
		app.SessionMutex.Lock()
		game.LastAccessTime = time.Now()
		app.SessionMutex.Unlock()
		log.Printf("Retrieved cached game state for session: %s, updated last access time.", sessionID)
		return game
	}

	log.Printf("Creating new game for session: %s", sessionID)
	return app.createNewGame(ctx, sessionID)
}

func (app *App) createNewGame(ctx context.Context, sessionID string) *GameState {
	selectedEntry := app.getRandomWordEntry(ctx)

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

	app.SessionMutex.Lock()
	app.GameSessions[sessionID] = game
	app.SessionMutex.Unlock()

	return game
}

func (app *App) saveGameState(sessionID string, game *GameState) {
	app.SessionMutex.Lock()
	app.GameSessions[sessionID] = game
	game.LastAccessTime = time.Now()
	app.SessionMutex.Unlock()
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

func (app *App) renderHTMXError(c *gin.Context, err error) {
	logWarn("HTMX error: %v", err)
	c.HTML(http.StatusOK, "game-board", gin.H{
		"error": err.Error(),
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
