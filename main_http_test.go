package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// setupTestRouter initializes a Gin router with all main routes for testing.
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.LoadHTMLGlob("templates/*.html")
	router.GET("/", homeHandler)
	router.GET("/new-game", newGameHandler)
	router.POST("/new-game", newGameHandler)
	router.POST("/guess", guessHandler)
	router.GET("/game-state", gameStateHandler)
	router.POST("/retry-word", retryWordHandler)
	return router
}

// TestHomeHandler checks that the home page ("/") returns HTTP 200.
func TestHomeHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET / returned status %d, want 200", w.Code)
	}
}

// TestNewGameHandler checks that GET /new-game redirects (303 or 302).
func TestNewGameHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/new-game", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("GET /new-game returned status %d, want 303 or 302", w.Code)
	}
}

// TestGameStateHandler checks that GET /game-state returns HTTP 200.
func TestGameStateHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/game-state", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /game-state returned status %d, want 200", w.Code)
	}
}

// TestGuessHandler_InvalidMethod checks that GET /guess is not allowed (405 or 404).
func TestGuessHandler_InvalidMethod(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/guess", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("GET /guess returned status %d, want 405 or 404", w.Code)
	}
}

// TestRetryWordHandler checks that POST /retry-word redirects (303 or 302).
func TestRetryWordHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("POST", "/retry-word", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("POST /retry-word returned status %d, want 303 or 302", w.Code)
	}
}

// TestRateLimitMiddleware checks that the rate limiter blocks requests after the burst limit.
func TestRateLimitMiddleware(t *testing.T) {
	// Setup a router with rate limiting and a dummy endpoint.
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(rateLimitMiddleware())
	router.GET("/limited", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Simulate requests from the same IP.
	req, _ := http.NewRequest("GET", "/limited", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	// The default limiter allows 10 burst requests.
	for i := range 10 {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// The 11th request should be rate limited (429).
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("11th request: expected 429 Too Many Requests, got %d", w.Code)
	}
}

// TestMain sets up a minimal word list and word set for all HTTP tests.
func TestMain(m *testing.M) {
	wordList = []WordEntry{{Word: "APPLE", Hint: "fruit"}}
	wordSet = map[string]struct{}{"APPLE": {}}
	os.Exit(m.Run())
}
