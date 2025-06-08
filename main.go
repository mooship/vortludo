package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
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
	cachecontrol "go.eigsys.de/gin-cachecontrol/v2"

	"golang.org/x/time/rate"

	"errors"

	"github.com/gin-gonic/gin"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

// Game configuration
const (
	MaxGuesses     = 6
	WordLength     = 5
	SessionTimeout = 2 * time.Hour
	CookieMaxAge   = 2 * time.Hour
	StaticCacheAge = 5 * time.Minute
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
	minifier        *minify.M
)

func main() {
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

	// Setup minifier for production
	if isProduction {
		minifier = minify.New()
		minifier.AddFunc("text/html", html.Minify)
		minifier.AddFunc("text/css", css.Minify)
		minifier.AddFunc("application/javascript", js.Minify)
		minifier.AddFunc("text/javascript", js.Minify)
	}

	// Serve static files with appropriate assets for environment.
	if isProduction && dirExists("dist") {
		log.Printf("Serving minified assets from dist/ directory")
		router.LoadHTMLGlob("dist/templates/*.html")
		// Serve minified static files
		router.GET("/static/*filepath", minifiedStaticHandler("dist/static"))
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

// applyCacheHeaders sets cache headers
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

// loadWords loads word list from JSON
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

// loadAcceptedWords loads accepted words from JSON
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

// getRandomWordEntry returns a random word
func getRandomWordEntry() WordEntry {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(wordList))))
	if err != nil {
		// Fallback to first word if crypto/rand fails.
		log.Printf("Error generating random number: %v, using fallback", err)
		return wordList[0]
	}
	return wordList[n.Int64()]
}

// sessionCleanupScheduler cleans up sessions hourly
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

// getHintForWord returns the hint for a word
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

// minifiedStaticHandler serves minified CSS/JS in production
func minifiedStaticHandler(root string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Validate and sanitize the filepath parameter
		requestedPath := c.Param("filepath")
		if requestedPath == "" {
			c.Status(http.StatusNotFound)
			return
		}

		// Remove leading slash if present (Gin's Param includes it)
		if strings.HasPrefix(requestedPath, "/") || strings.HasPrefix(requestedPath, "\\") {
			requestedPath = requestedPath[1:]
		}

		// Remove any path traversal attempts and normalize
		cleanPath := filepath.Clean(requestedPath)

		// Ensure the path doesn't contain directory traversal
		if strings.Contains(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") || strings.HasPrefix(cleanPath, "\\") {
			log.Printf("Rejected potentially malicious file path: %s", requestedPath)
			c.Status(http.StatusForbidden)
			return
		}

		// Construct the full file path
		fp := filepath.Join(root, cleanPath)

		// Validate that the resolved path is within the root directory
		absRoot, err := filepath.Abs(root)
		if err != nil {
			log.Printf("Failed to resolve root path %s: %v", root, err)
			c.Status(http.StatusInternalServerError)
			return
		}

		absFilePath, err := filepath.Abs(fp)
		if err != nil {
			log.Printf("Failed to resolve file path %s: %v", fp, err)
			c.Status(http.StatusNotFound)
			return
		}

		// Ensure the file path is within the allowed root directory
		if !strings.HasPrefix(absFilePath+string(filepath.Separator), absRoot+string(filepath.Separator)) {
			log.Printf("File path %s escapes root directory %s", absFilePath, absRoot)
			c.Status(http.StatusForbidden)
			return
		}

		ext := strings.ToLower(filepath.Ext(fp))
		var mediatype string
		switch ext {
		case ".css":
			mediatype = "text/css"
		case ".js":
			mediatype = "application/javascript"
		default:
			// Serve as-is for other types (e.g. images, fonts)
			c.File(fp)
			return
		}

		f, err := os.Open(fp)
		if err != nil {
			if os.IsNotExist(err) {
				c.Status(http.StatusNotFound)
			} else {
				log.Printf("Error opening file %s: %v", fp, err)
				c.Status(http.StatusInternalServerError)
			}
			return
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				log.Printf("Error closing file %s: %v", fp, closeErr)
			}
		}()

		c.Header("Content-Type", mediatype)
		if minifier != nil {
			if err := minifier.Minify(mediatype, c.Writer, f); err != nil {
				log.Printf("Minify error for %s: %v", fp, err)
				// Seek back to beginning and serve unminified
				if _, seekErr := f.Seek(0, 0); seekErr != nil {
					log.Printf("Error seeking file %s: %v", fp, seekErr)
					c.Status(http.StatusInternalServerError)
					return
				}
				if _, copyErr := io.Copy(c.Writer, f); copyErr != nil {
					log.Printf("Error serving file %s: %v", fp, copyErr)
				}
			}
		} else {
			if _, err := io.Copy(c.Writer, f); err != nil {
				log.Printf("Error serving file %s: %v", fp, err)
			}
		}
	}
}

// getTemplateEngine retrieves the *template.Template from Gin's HTMLRender
func getTemplateEngine(c *gin.Context) *template.Template {
	// Gin does not export Engine(), but stores the engine in the context under this key.
	engineAny, exists := c.Get("gin-gonic/gin/context/engine")
	if !exists {
		return nil
	}
	engine, ok := engineAny.(*gin.Engine)
	if !ok {
		return nil
	}
	// Try HTMLProduction (used by LoadHTMLGlob)
	if htmlProd, ok := engine.HTMLRender.(interface{ Template() *template.Template }); ok {
		return htmlProd.Template()
	}
	// Try HTMLDebug (used by LoadHTMLFiles)
	if htmlDebug, ok := engine.HTMLRender.(interface{ Template() *template.Template }); ok {
		return htmlDebug.Template()
	}
	return nil
}

// homeHandler serves the main page
func homeHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)
	hint := getHintForWord(game.SessionWord)

	// Minify HTML output in production
	if isProduction && minifier != nil {
		var buf strings.Builder
		tmpl := getTemplateEngine(c)
		if tmpl == nil {
			c.String(http.StatusInternalServerError, "Template error")
			return
		}
		if err := tmpl.ExecuteTemplate(&buf, "index.html", gin.H{
			"title":   "Vortludo - A Libre Wordle Clone",
			"message": "Guess the 5-letter word!",
			"hint":    hint,
			"game":    game,
		}); err != nil {
			c.String(http.StatusInternalServerError, "Template error")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := minifier.Minify("text/html", c.Writer, strings.NewReader(buf.String())); err != nil {
			c.String(http.StatusInternalServerError, "Minify error")
		}
		return
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":   "Vortludo - A Libre Wordle Clone",
		"message": "Guess the 5-letter word!",
		"hint":    hint,
		"game":    game,
	})
}

// newGameHandler starts a new game
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

// guessHandler handles a guess
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
		if isProduction && minifier != nil {
			var buf strings.Builder
			tmpl := getTemplateEngine(c)
			if tmpl == nil {
				c.String(http.StatusInternalServerError, "Template error")
				return
			}
			if err := tmpl.ExecuteTemplate(&buf, "game-board", gin.H{
				"game":        game,
				"notAccepted": true,
			}); err != nil {
				c.String(http.StatusInternalServerError, "Template error")
				return
			}
			c.Header("Content-Type", "text/html; charset=utf-8")
			if err := minifier.Minify("text/html", c.Writer, strings.NewReader(buf.String())); err != nil {
				c.String(http.StatusInternalServerError, "Minify error")
			}
			return
		}
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

// validateGameState checks if game can accept guesses
func validateGameState(c *gin.Context, game *GameState) error {
	if game.GameOver {
		log.Print("session attempted guess on completed game")
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
		return errors.New(ErrorGameOver)
	}
	return nil
}

// normalizeGuess normalizes input
func normalizeGuess(input string) string {
	return strings.ToUpper(strings.TrimSpace(input))
}

// processGuess processes a guess
func processGuess(c *gin.Context, sessionID string, game *GameState, guess string) error {
	log.Printf("session %s guessed: %s (attempt %d/%d)", sessionID, guess, game.CurrentRow+1, MaxGuesses)

	if len(guess) != WordLength {
		log.Printf("session %s submitted invalid length guess: %s (%d letters)", sessionID, guess, len(guess))
		if isProduction && minifier != nil {
			var buf strings.Builder
			tmpl := getTemplateEngine(c)
			if tmpl == nil {
				c.String(http.StatusInternalServerError, "Template error")
				return fmt.Errorf("template error")
			}
			if err := tmpl.ExecuteTemplate(&buf, "game-board", gin.H{"game": game}); err != nil {
				c.String(http.StatusInternalServerError, "Template error")
				return err
			}
			c.Header("Content-Type", "text/html; charset=utf-8")
			if err := minifier.Minify("text/html", c.Writer, strings.NewReader(buf.String())); err != nil {
				c.String(http.StatusInternalServerError, "Minify error")
			}
			return errors.New(ErrorInvalidLength)
		}
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
		return errors.New(ErrorInvalidLength)
	}

	if game.CurrentRow >= MaxGuesses {
		log.Printf("session %s attempted guess after max guesses reached", sessionID)
		if isProduction && minifier != nil {
			var buf strings.Builder
			tmpl := getTemplateEngine(c)
			if tmpl == nil {
				c.String(http.StatusInternalServerError, "Template error")
				return fmt.Errorf("template error")
			}
			if err := tmpl.ExecuteTemplate(&buf, "game-board", gin.H{"game": game}); err != nil {
				c.String(http.StatusInternalServerError, "Template error")
				return err
			}
			c.Header("Content-Type", "text/html; charset=utf-8")
			if err := minifier.Minify("text/html", c.Writer, strings.NewReader(buf.String())); err != nil {
				c.String(http.StatusInternalServerError, "Minify error")
			}
			return errors.New(ErrorNoMoreGuesses)
		}
		c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
		return errors.New(ErrorNoMoreGuesses)
	}

	targetWord := getTargetWord(game)
	isInvalid := !isValidWord(guess)
	result := checkGuess(guess, targetWord)
	updateGameState(game, guess, targetWord, result, isInvalid)
	saveGameState(sessionID, game)

	if isProduction && minifier != nil {
		var buf strings.Builder
		tmpl := getTemplateEngine(c)
		if tmpl == nil {
			c.String(http.StatusInternalServerError, "Template error")
			return fmt.Errorf("template error")
		}
		if err := tmpl.ExecuteTemplate(&buf, "game-board", gin.H{"game": game}); err != nil {
			c.String(http.StatusInternalServerError, "Template error")
			return err
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := minifier.Minify("text/html", c.Writer, strings.NewReader(buf.String())); err != nil {
			c.String(http.StatusInternalServerError, "Minify error")
		}
		return nil
	}

	c.HTML(http.StatusOK, "game-board", gin.H{"game": game})
	return nil
}

// getTargetWord returns or assigns the target word
func getTargetWord(game *GameState) string {
	if game.SessionWord == "" {
		selectedEntry := getRandomWordEntry()
		game.SessionWord = selectedEntry.Word
		log.Printf("Warning: SessionWord was empty, assigned random word: %s", selectedEntry.Word)
	}
	return game.SessionWord
}

// updateGameState updates the game state
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

// gameStateHandler returns current game state
func gameStateHandler(c *gin.Context) {
	sessionID := getOrCreateSession(c)
	game := getGameState(sessionID)
	hint := getHintForWord(game.SessionWord)

	// Minify HTML fragment in production
	if isProduction && minifier != nil {
		var buf strings.Builder
		tmpl := getTemplateEngine(c)
		if tmpl == nil {
			c.String(http.StatusInternalServerError, "Template error")
			return
		}
		if err := tmpl.ExecuteTemplate(&buf, "game-board", gin.H{
			"game": game,
			"hint": hint,
		}); err != nil {
			c.String(http.StatusInternalServerError, "Template error")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := minifier.Minify("text/html", c.Writer, strings.NewReader(buf.String())); err != nil {
			c.String(http.StatusInternalServerError, "Minify error")
		}
		return
	}

	c.HTML(http.StatusOK, "game-board", gin.H{
		"game": game,
		"hint": hint,
	})
}

// checkGuess compares guess to target
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
	for i := range WordLength {
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

// isValidWord checks if a word exists in the word set.
func isValidWord(word string) bool {
	_, ok := wordSet[word]
	return ok
}

// isAcceptedWord checks if a word is in the accepted word set.
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

// getGameState gets or creates a game state
func getGameState(sessionID string) *GameState {
	sessionMutex.RLock()
	game, exists := gameSessions[sessionID]
	if exists {
		game.LastAccessTime = time.Now()
		log.Printf("Retrieved cached game state for session: %s, updated last access time.", sessionID)
	}
	sessionMutex.RUnlock()

	if exists {
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

// createNewGame creates a new game state
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

// saveGameState saves game state
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

// dirExists checks if a directory exists
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

// isValidSessionID validates session ID format
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
			// Valid hex character.
		default:
			return false
		}
	}
	return true
}

// getSecureSessionPath returns a safe session file path
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
	if !strings.HasPrefix(absSessionFile+string(filepath.Separator), absSessionDir+string(filepath.Separator)) {
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

// retryWordHandler resets guesses but keeps the word
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

// getLimiter returns a rate limiter for a key
func getLimiter(key string) *rate.Limiter {
	limiterMutex.Lock()
	defer limiterMutex.Unlock()
	if lim, ok := limiterMap[key]; ok {
		return lim
	}
	// Allow 5 requests/sec with burst of 10.
	lim := rate.NewLimiter(rate.Every(time.Second/5), 10)
	limiterMap[key] = lim
	return lim
}

// rateLimitMiddleware applies rate limiting
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

// healthHandler returns health status
func healthHandler(c *gin.Context) {
	sessionMutex.RLock()
	sessionCount := len(gameSessions)
	sessionMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"env":            map[bool]string{true: "production", false: "development"}[isProduction],
		"words_loaded":   len(wordList),
		"sessions_count": sessionCount,
		"uptime":         time.Since(startTime).String(),
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
}
