package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	ginGzip "github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// setupTestRouter creates a test router with all routes
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.LoadHTMLGlob("templates/*.html")
	router.GET("/", homeHandler)
	router.GET("/new-game", newGameHandler)
	router.POST("/new-game", rateLimitMiddleware(), newGameHandler)
	router.POST("/guess", rateLimitMiddleware(), guessHandler)
	router.GET("/game-state", gameStateHandler)
	router.POST("/retry-word", rateLimitMiddleware(), retryWordHandler)
	return router
}

func setupGzipTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.Use(
		ginGzip.Gzip(ginGzip.DefaultCompression,
			ginGzip.WithExcludedExtensions([]string{".svg", ".ico", ".png", ".jpg", ".jpeg", ".gif"}),
			ginGzip.WithExcludedPaths([]string{"/static/fonts"})),
	)
	router.GET("/static/test.js", func(c *gin.Context) {
		c.Header("Content-Type", "application/javascript")
		c.String(http.StatusOK, "var x = 1;")
	})
	router.GET("/static/test.css", func(c *gin.Context) {
		c.Header("Content-Type", "text/css")
		c.String(http.StatusOK, "body{}")
	})
	router.GET("/static/test.png", func(c *gin.Context) {
		c.Header("Content-Type", "image/png")
		c.String(http.StatusOK, "PNGDATA")
	})
	router.GET("/static/fonts/font.woff2", func(c *gin.Context) {
		c.Header("Content-Type", "font/woff2")
		c.String(http.StatusOK, "FONTDATA")
	})
	return router
}

// TestHomeHandler checks home page returns 200
func TestHomeHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET / returned status %d, want 200", w.Code)
	}
}

// TestNewGameHandler checks new game redirects
func TestNewGameHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/new-game", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("GET /new-game returned status %d, want 303 or 302", w.Code)
	}
}

// TestGameStateHandler checks game state returns 200
func TestGameStateHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/game-state", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /game-state returned status %d, want 200", w.Code)
	}
}

// TestGuessHandler_InvalidMethod checks GET /guess is not allowed
func TestGuessHandler_InvalidMethod(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/guess", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("GET /guess returned status %d, want 405 or 404", w.Code)
	}
}

// TestRetryWordHandler checks retry word redirects
func TestRetryWordHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("POST", "/retry-word", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("POST /retry-word returned status %d, want 303 or 302", w.Code)
	}
}

// TestRateLimitMiddleware checks rate limiting blocks excessive requests
func TestRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/limited", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest("GET", "/limited", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	// First 10 requests should succeed
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 11th request should be rate limited
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("11th request: expected 429 Too Many Requests, got %d", w.Code)
	}
}

// TestMain sets up test data for all HTTP tests
func TestMain(m *testing.M) {
	wordList = []WordEntry{{Word: "APPLE", Hint: "fruit"}}
	wordSet = map[string]struct{}{"APPLE": {}}
	os.Exit(m.Run())
}

// TestHealthHandlerFields checks /health endpoint for required fields
func TestHealthHandlerFields(t *testing.T) {
	router := gin.Default()
	router.GET("/health", healthHandler)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /health returned status %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal /health response: %v", err)
	}

	if _, ok := resp["words_loaded"]; !ok {
		t.Error("Expected 'words_loaded' field in /health response")
	}
	if _, ok := resp["accepted_words"]; !ok {
		t.Error("Expected 'accepted_words' field in /health response")
	}
}

func TestGuessHandler_PostInvalidGuess(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("POST", "/guess", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("POST /guess with no data returned status %d, want 200", w.Code)
	}
}

func TestGuessHandler_PostNotAcceptedWord(t *testing.T) {
	router := setupTestRouter()
	acceptedWordSet = map[string]struct{}{"APPLE": {}}
	form := "guess=ZZZZZ"
	req, _ := http.NewRequest("POST", "/guess", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("POST /guess with not accepted word returned status %d, want 200", w.Code)
	}
}

func TestGuessHandler_PostShortGuess(t *testing.T) {
	router := setupTestRouter()
	acceptedWordSet = map[string]struct{}{"APPLE": {}}
	form := "guess=AB"
	req, _ := http.NewRequest("POST", "/guess", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("POST /guess with short guess returned status %d, want 200", w.Code)
	}
}

func TestGuessHandler_PostLongGuess(t *testing.T) {
	router := setupTestRouter()
	acceptedWordSet = map[string]struct{}{"APPLE": {}}
	form := "guess=ABCDEFGHIJK"
	req, _ := http.NewRequest("POST", "/guess", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("POST /guess with long guess returned status %d, want 200", w.Code)
	}
}

func TestHealthHandler_EnvField(t *testing.T) {
	router := gin.Default()
	router.GET("/health", healthHandler)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if env, ok := resp["env"].(string); !ok || (env != "production" && env != "development") {
		t.Errorf("healthHandler env field = %v, want 'production' or 'development'", resp["env"])
	}
}

func TestNewGameHandler_Reset(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/new-game?reset=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("GET /new-game?reset=1 returned status %d, want 303 or 302", w.Code)
	}
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "session_id" {
			found = true
		}
	}
	if !found {
		t.Error("Expected session_id cookie to be set on reset")
	}
}

func TestHealthHandler_Fields(t *testing.T) {
	router := gin.Default()
	router.GET("/health", healthHandler)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /health returned status %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal /health response: %v", err)
	}
	for _, field := range []string{"timestamp", "uptime"} {
		if _, ok := resp[field]; !ok {
			t.Errorf("Expected '%s' field in /health response", field)
		}
	}
}

func isGzipped(w *httptest.ResponseRecorder) bool {
	return w.Header().Get("Content-Encoding") == "gzip"
}

func decompressGzip(data []byte) (string, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	return string(out), err
}

func TestGzipMiddleware_CompressesJS(t *testing.T) {
	router := setupGzipTestRouter()
	req, _ := http.NewRequest("GET", "/static/test.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if !isGzipped(w) {
		t.Errorf("Expected gzip Content-Encoding for .js file")
	}
	body, err := decompressGzip(w.Body.Bytes())
	if err != nil || body != "var x = 1;" {
		t.Errorf("Failed to decompress gzipped JS: %v, got: %q", err, body)
	}
}

func TestGzipMiddleware_CompressesCSS(t *testing.T) {
	router := setupGzipTestRouter()
	req, _ := http.NewRequest("GET", "/static/test.css", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if !isGzipped(w) {
		t.Errorf("Expected gzip Content-Encoding for .css file")
	}
	body, err := decompressGzip(w.Body.Bytes())
	if err != nil || body != "body{}" {
		t.Errorf("Failed to decompress gzipped CSS: %v, got: %q", err, body)
	}
}

func TestGzipMiddleware_SkipsPNG(t *testing.T) {
	router := setupGzipTestRouter()
	req, _ := http.NewRequest("GET", "/static/test.png", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if isGzipped(w) {
		t.Errorf("Did not expect gzip Content-Encoding for .png file")
	}
	if w.Body.String() != "PNGDATA" {
		t.Errorf("Unexpected body for .png file: %q", w.Body.String())
	}
}

func TestGzipMiddleware_SkipsFontsPath(t *testing.T) {
	router := setupGzipTestRouter()
	req, _ := http.NewRequest("GET", "/static/fonts/font.woff2", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if isGzipped(w) {
		t.Errorf("Did not expect gzip Content-Encoding for /static/fonts path")
	}
	if w.Body.String() != "FONTDATA" {
		t.Errorf("Unexpected body for font file: %q", w.Body.String())
	}
}
