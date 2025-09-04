package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// getLimiter returns a rate limiter for the given key (usually client IP).
func (app *App) getLimiter(key string) *rate.Limiter {
	app.LimiterMutex.Lock()
	defer app.LimiterMutex.Unlock()
	if lim, ok := app.LimiterMap[key]; ok {
		return lim
	}

	// Use relaxed limits for localhost connections
	if key == "" || key == "::1" {
		logWarn("Rate limiter key is empty or loopback: %q", key)
	}
	rps := app.RateLimitRPS
	if rps <= 0 {
		rps = 1
	}
	lim := rate.NewLimiter(rate.Every(time.Second/time.Duration(rps)), app.RateLimitBurst)
	app.LimiterMap[key] = lim
	return lim
}

// rateLimitMiddleware returns a Gin middleware that enforces per-client rate limiting.
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

// requestIDMiddleware injects a request ID into the context for each request.
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.Request.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		ctx := context.WithValue(c.Request.Context(), requestIDKey, reqID)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-Id", reqID)
		c.Next()
	}
}
