package main

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	cachecontrol "go.eigsys.de/gin-cachecontrol/v2"

	ginGzip "github.com/gin-contrib/gzip"

	"golang.org/x/time/rate"

	"github.com/gin-gonic/gin"

	"github.com/samber/lo"
)

// main is the entry point for the application. It loads configuration, sets up routes, and starts the server.
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
		RuneBufPool: &sync.Pool{
			New: func() any { buf := make([]rune, WordLength); return &buf },
		},
	}

	setGlobalApp(app)

	router := gin.Default()

	router.Use(requestIDMiddleware())
	router.Use(securityHeadersMiddleware())

	router.Use(app.csrfMiddleware())
	router.Use(app.validateCSRFMiddleware())

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

	funcMap := template.FuncMap{"hasPrefix": strings.HasPrefix}

	var baseTplDir string
	if isProduction && dirExists("dist") {
		logInfo("Serving assets from dist/ directory")
		baseTplDir = filepath.ToSlash(filepath.Join("dist", "templates"))
		router.Static("/static", "./dist/static")
	} else {
		logInfo("Serving development assets from source directories")
		baseTplDir = "templates"
		router.Static("/static", "./static")
	}

	rootPattern := filepath.ToSlash(filepath.Join(baseTplDir, "*.html"))
	partialsPattern := filepath.ToSlash(filepath.Join(baseTplDir, "partials", "*.html"))

	master := template.New("").Funcs(funcMap)
	if _, err := master.ParseGlob(rootPattern); err != nil {
		logFatal("Failed to parse root templates: %v", err)
	}
	if _, err := master.ParseGlob(partialsPattern); err != nil {
		logFatal("Failed to parse partial templates: %v", err)
	}
	router.SetHTMLTemplate(master)

	router.GET("/", app.homeHandler)
	router.GET("/new-game", app.newGameHandler)
	router.POST("/new-game", app.rateLimitMiddleware(), app.newGameHandler)
	router.POST("/guess", app.rateLimitMiddleware(), app.guessHandler)
	router.GET("/game-state", app.gameStateHandler)
	router.POST("/retry-word", app.rateLimitMiddleware(), app.retryWordHandler)
	router.GET("/healthz", app.healthzHandler)

	app.startServer(router)
}

// startServer launches the HTTP server and handles graceful shutdown on SIGINT/SIGTERM.
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

// applyCacheHeaders sets HTTP cache headers for static and dynamic content based on environment.
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

// loadWords loads the playable words from data/words.json and returns a filtered list and set.
func loadWords() ([]WordEntry, map[string]struct{}, error) {
	logInfo("Loading words from data/words.json")

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
			logWarn("Skipping word %q: not 5 letters", entry.Word)
			return false
		}
		return true
	})

	wordSet := make(map[string]struct{}, len(wordList))
	lo.ForEach(wordList, func(entry WordEntry, _ int) {
		wordSet[entry.Word] = struct{}{}
	})

	logInfo("Successfully loaded %d words", len(wordList))
	return wordList, wordSet, nil
}

// loadAcceptedWords loads the accepted guess words from data/accepted_words.txt.
func loadAcceptedWords() (map[string]struct{}, error) {
	logInfo("Loading accepted words from data/accepted_words.txt")

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
