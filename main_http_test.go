package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// setupTestRouter creates test router with all routes
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

// TestHomeHandler tests home page returns 200
func TestHomeHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET / returned status %d, want 200", w.Code)
	}
}

// TestNewGameHandler tests new game redirects
func TestNewGameHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/new-game", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("GET /new-game returned status %d, want 303 or 302", w.Code)
	}
}

// TestGameStateHandler tests game state returns 200
func TestGameStateHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/game-state", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /game-state returned status %d, want 200", w.Code)
	}
}

// TestGuessHandler_InvalidMethod tests GET /guess not allowed
func TestGuessHandler_InvalidMethod(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("GET", "/guess", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("GET /guess returned status %d, want 405 or 404", w.Code)
	}
}

// TestRetryWordHandler tests retry word redirects
func TestRetryWordHandler(t *testing.T) {
	router := setupTestRouter()
	req, _ := http.NewRequest("POST", "/retry-word", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("POST /retry-word returned status %d, want 303 or 302", w.Code)
	}
}

// TestRateLimitMiddleware tests rate limiting blocks excessive requests
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

// TestMain sets up test data
func TestMain(m *testing.M) {
	wordList = []WordEntry{{Word: "APPLE", Hint: "fruit"}}
	wordSet = map[string]struct{}{"APPLE": {}}
	os.Exit(m.Run())
}

// TestHealthHandlerFields tests /health endpoint for required fields
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
