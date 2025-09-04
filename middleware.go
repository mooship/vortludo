package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// securityHeadersMiddleware sets recommended security headers including CSP.
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Content Security Policy (adjust as needed for your stack)
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net https://cdn.jsdelivr.net/npm 'unsafe-inline' 'unsafe-eval'; style-src 'self' https://cdn.jsdelivr.net https://fonts.bunny.net 'unsafe-inline'; font-src 'self' https://cdn.jsdelivr.net https://fonts.bunny.net; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none';")
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")
		// Prevent MIME sniffing
		c.Header("X-Content-Type-Options", "nosniff")
		// Referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		// HSTS (only if using HTTPS)
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}
		c.Next()
	}
}

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

// validateCSRFMiddleware enforces that unsafe methods include a matching CSRF token
func (app *App) validateCSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete || method == http.MethodPatch {
			cookie, _ := c.Cookie("csrf_token")
			header := c.GetHeader("X-CSRF-Token")
			form := c.PostForm("csrf_token")
			var token string
			if header != "" {
				token = header
			} else if form != "" {
				token = form
			}
			if token == "" || cookie == "" || token != cookie {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid csrf token"})
				return
			}
		}
		c.Next()
	}
}

// csrfMiddleware ensures a per-session CSRF token cookie exists and stores it in the context.
// It does not validate requests; handlers should validate the token on unsafe methods.
func (app *App) csrfMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to read token cookie
		token, err := c.Cookie("csrf_token")
		if err != nil || len(token) < 8 {
			// Generate new token
			b := make([]byte, 32)
			if _, err := rand.Read(b); err == nil {
				token = fmt.Sprintf("%x", b)
				// Set cookie; allow JS to not read it (HttpOnly) while handlers can still read it from request cookies
				secure := app.IsProduction
				c.SetSameSite(http.SameSiteStrictMode)
				c.SetCookie("csrf_token", token, int(app.CookieMaxAge.Seconds()), "/", "", secure, true)
			}
		}
		// Expose token in context for handlers/templates
		c.Set("csrf_token", token)
		c.Next()
	}
}
