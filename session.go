package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// getOrCreateSession retrieves the session ID from the cookie or creates a new one.
func (app *App) getOrCreateSession(c *gin.Context) string {
	sessionID, err := c.Cookie(SessionCookieName)
	if err != nil || len(sessionID) < 10 {
		sessionID = uuid.NewString()
		c.SetSameSite(http.SameSiteStrictMode)
		secure := app.IsProduction
		c.SetCookie(SessionCookieName, sessionID, int(app.CookieMaxAge.Seconds()), "/", "", secure, true)
		logInfo("Created new session: %s", sessionID)
	}
	return sessionID
}

// getGameState retrieves or creates the GameState for a session.
func (app *App) getGameState(ctx context.Context, sessionID string) *GameState {
	app.SessionMutex.RLock()
	game, exists := app.GameSessions[sessionID]
	app.SessionMutex.RUnlock()
	if exists {
		app.SessionMutex.Lock()
		game.LastAccessTime = time.Now()
		app.SessionMutex.Unlock()
		logInfo("Retrieved cached game state for session: %s, updated last access time.", sessionID)
		return game
	}

	logInfo("Creating new game for session: %s", sessionID)
	return app.createNewGame(ctx, sessionID)
}

// saveGameState updates the in-memory game state for a session.
func (app *App) saveGameState(sessionID string, game *GameState) {
	app.SessionMutex.Lock()
	app.GameSessions[sessionID] = game
	game.LastAccessTime = time.Now()
	app.SessionMutex.Unlock()
	logInfo("Updated in-memory game state for session: %s", sessionID)
}
