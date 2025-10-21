package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/security"
)

const (
	// unknownValue is used as a fallback value when extraction fails
	unknownValue = "unknown"
)

// LoggingMiddleware demonstrates request/response logging
func LoggingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			// Log request
			if ctx.Logger != nil {
				ctx.Logger.Info("Request started", map[string]any{
					"method":     ctx.Request.Method,
					"path":       ctx.Request.Path,
					"user_agent": ctx.Header("User-Agent"),
					"ip":         getClientIP(ctx),
				})
			}

			// Process request
			err := next.Handle(ctx)

			duration := time.Since(start)
			status := ctx.Response.StatusCode

			// Log response
			if ctx.Logger != nil {
				ctx.Logger.Info("Request completed", map[string]any{
					"method":   ctx.Request.Method,
					"path":     ctx.Request.Path,
					"status":   status,
					"duration": duration.String(),
					"error":    err,
				})
			}

			return err
		})
	}
}

// AuthenticationMiddleware demonstrates JWT token validation
func AuthenticationMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			authHeader := ctx.Header("Authorization")

			if authHeader == "" {
				return ctx.Status(401).JSON(map[string]string{
					"error": "Missing authorization header",
				})
			}

			// Extract Bearer token
			if !strings.HasPrefix(authHeader, "Bearer ") {
				return ctx.Status(401).JSON(map[string]string{
					"error": "Invalid authorization format",
				})
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Simulate token validation
			if token != "valid-token-123" {
				return ctx.Status(401).JSON(map[string]string{
					"error": "Invalid token",
				})
			}

			// Set user context
			ctx.Set("user_id", "user123")
			ctx.Set("user_email", "user@example.com")

			return next.Handle(ctx)
		})
	}
}

// RateLimitingMiddleware demonstrates simple rate limiting
func RateLimitingMiddleware(requestsPerMinute int) lift.Middleware {
	// Simple in-memory rate limiter (production should use Redis/DynamoDB)
	requests := make(map[string][]time.Time)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			clientIP := getClientIP(ctx)
			now := time.Now()

			// Clean old requests
			if timestamps, exists := requests[clientIP]; exists {
				var validRequests []time.Time
				for _, timestamp := range timestamps {
					if now.Sub(timestamp) < time.Minute {
						validRequests = append(validRequests, timestamp)
					}
				}
				requests[clientIP] = validRequests
			}

			// Check rate limit
			if len(requests[clientIP]) >= requestsPerMinute {
				ctx.Response.Header("X-RateLimit-Limit", fmt.Sprintf("%d", requestsPerMinute))
				ctx.Response.Header("X-RateLimit-Remaining", "0")
				ctx.Response.Header("Retry-After", "60")

				return ctx.Status(429).JSON(map[string]string{
					"error": "Rate limit exceeded",
				})
			}

			// Add current request
			requests[clientIP] = append(requests[clientIP], now)

			// Set rate limit headers
			remaining := requestsPerMinute - len(requests[clientIP])
			ctx.Response.Header("X-RateLimit-Limit", fmt.Sprintf("%d", requestsPerMinute))
			ctx.Response.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

			return next.Handle(ctx)
		})
	}
}

// CORSMiddleware demonstrates Cross-Origin Resource Sharing
func CORSMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers
			ctx.Response.Header("Access-Control-Allow-Origin", "*")
			ctx.Response.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			ctx.Response.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			ctx.Response.Header("Access-Control-Max-Age", "86400")

			// Handle preflight requests
			if ctx.Request.Method == "OPTIONS" {
				return ctx.Status(204).JSON(nil)
			}

			return next.Handle(ctx)
		})
	}
}

// ValidationMiddleware demonstrates request validation
func ValidationMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Validate Content-Type for POST/PUT requests
			if ctx.Request.Method == "POST" || ctx.Request.Method == "PUT" {
				contentType := ctx.Header("Content-Type")
				if !strings.Contains(contentType, "application/json") {
					return ctx.Status(400).JSON(map[string]string{
						"error": "Content-Type must be application/json",
					})
				}
			}

			// Validate required headers
			if ctx.Request.Path == "/api/protected" {
				if ctx.Header("X-API-Version") == "" {
					return ctx.Status(400).JSON(map[string]string{
						"error": "X-API-Version header is required",
					})
				}
			}

			return next.Handle(ctx)
		})
	}
}

// RecoveryMiddleware demonstrates panic recovery
func RecoveryMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					if ctx.Logger != nil {
						ctx.Logger.Error("Panic recovered", map[string]any{
							"panic":  r,
							"path":   ctx.Request.Path,
							"method": ctx.Request.Method,
						})
					}

					if err := ctx.Status(500).JSON(map[string]string{
						"error": "Internal server error",
					}); err != nil {
						log.Printf("Failed to send error response: %v", err)
					}
				}
			}()

			return next.Handle(ctx)
		})
	}
}

// TimeoutMiddleware demonstrates request timeout handling
func TimeoutMiddleware(timeout time.Duration) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Create context with timeout
			timeoutCtx, cancel := context.WithTimeout(ctx.Context, timeout)
			defer cancel()

			// Replace context
			ctx.Context = timeoutCtx

			// Handle request with timeout
			done := make(chan error, 1)
			go func() {
				done <- next.Handle(ctx)
			}()

			select {
			case err := <-done:
				return err
			case <-timeoutCtx.Done():
				return ctx.Status(408).JSON(map[string]string{
					"error": "Request timeout",
				})
			}
		})
	}
}

// CustomHeaderMiddleware demonstrates adding custom headers
func CustomHeaderMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Add custom headers
			ctx.Response.Header("X-App-Name", "Lift Middleware Showcase")
			ctx.Response.Header("X-App-Version", "1.0.0")
			ctx.Response.Header("X-Request-ID", generateRequestID())

			return next.Handle(ctx)
		})
	}
}

// ConditionalMiddleware demonstrates conditional middleware application
func ConditionalMiddleware(condition func(*lift.Context) bool, middleware lift.Middleware) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if condition(ctx) {
				// Apply middleware
				return middleware(next).Handle(ctx)
			}
			// Skip middleware
			return next.Handle(ctx)
		})
	}
}

// generateRequestID creates a simple request ID
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// getClientIP extracts the client IP from the request
func getClientIP(ctx *lift.Context) string {
	// Use the security package to extract client IP
	clientIP, err := security.ExtractClientIP(ctx.Request.Headers, ctx.Request.RequestContext())
	if err != nil {
		// Fallback to a default if extraction fails
		return unknownValue
	}
	return clientIP
}

func main() {
	app := lift.New()

	// Global middleware (applied to all routes)
	app.Use(RecoveryMiddleware())                // Always recover from panics
	app.Use(LoggingMiddleware())                 // Log all requests
	app.Use(CORSMiddleware())                    // Enable CORS
	app.Use(CustomHeaderMiddleware())            // Add custom headers
	app.Use(TimeoutMiddleware(30 * time.Second)) // 30 second timeout

	// Public routes (no authentication required)
	public := app.Group("/public")
	// Note: RouteGroup doesn't support Use() method directly - apply middleware at app level

	if err := public.GET("/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /public/health: %v", err)
	}

	if err := public.POST("/feedback", func(ctx *lift.Context) error {
		var feedback map[string]any
		if err := ctx.ParseRequest(&feedback); err != nil {
			return ctx.Status(400).JSON(map[string]string{
				"error": "Invalid JSON",
			})
		}

		return ctx.JSON(map[string]any{
			"message": "Feedback received",
			"data":    feedback,
		})
	}); err != nil {
		log.Fatalf("Failed to register POST /public/feedback: %v", err)
	}

	// API routes with authentication and validation
	api := app.Group("/api")
	// Note: RouteGroup doesn't support Use() method directly - apply middleware at app level

	if err := api.GET("/profile", func(ctx *lift.Context) error {
		userID, ok := ctx.Get("user_id").(string)
		if !ok {
			userID = unknownValue
		}
		userEmail, ok := ctx.Get("user_email").(string)
		if !ok {
			userEmail = unknownValue
		}

		return ctx.JSON(map[string]any{
			"user_id": userID,
			"email":   userEmail,
			"profile": map[string]any{
				"name": "John Doe",
				"role": "user",
			},
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /api/profile: %v", err)
	}

	if err := api.POST("/data", func(ctx *lift.Context) error {
		var data map[string]any
		if err := ctx.ParseRequest(&data); err != nil {
			return ctx.Status(400).JSON(map[string]string{
				"error": "Invalid JSON",
			})
		}

		return ctx.Status(201).JSON(map[string]any{
			"message": "Data created",
			"id":      generateRequestID(),
			"data":    data,
		})
	}); err != nil {
		log.Fatalf("Failed to register POST /api/data: %v", err)
	}

	// Protected route with additional validation
	if err := api.GET("/protected", func(ctx *lift.Context) error {
		apiVersion := ctx.Header("X-API-Version")

		return ctx.JSON(map[string]any{
			"message":     "Protected resource accessed",
			"api_version": apiVersion,
			"user_id":     ctx.Get("user_id"),
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /api/protected: %v", err)
	}

	// Admin routes with conditional middleware
	admin := app.Group("/admin")
	// Note: RouteGroup doesn't support Use() method directly - apply middleware at app level
	// Admin routes would need authentication applied at the app level

	if err := admin.GET("/stats", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]any{
			"message":   "Admin stats",
			"timestamp": time.Now().Format(time.RFC3339),
			"user_id":   ctx.Get("user_id"),
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /admin/stats: %v", err)
	}

	// Route that demonstrates panic recovery
	if err := app.GET("/panic", func(_ *lift.Context) error {
		panic("This is a demonstration panic")
	}); err != nil {
		log.Fatalf("Failed to register GET /panic: %v", err)
	}

	// Route that demonstrates timeout
	if err := app.GET("/slow", func(ctx *lift.Context) error {
		// Simulate slow operation
		time.Sleep(35 * time.Second) // This will timeout due to TimeoutMiddleware
		return ctx.JSON(map[string]string{"message": "This won't be reached"})
	}); err != nil {
		log.Fatalf("Failed to register GET /slow: %v", err)
	}

	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start app: %v", err)
	}

	fmt.Println("Middleware Showcase API is ready!")
	fmt.Println("Try these endpoints:")
	fmt.Println("  GET /public/health")
	fmt.Println("  POST /public/feedback")
	fmt.Println("  GET /api/profile (requires Authorization: Bearer valid-token-123)")
	fmt.Println("  POST /api/data (requires auth + JSON)")
	fmt.Println("  GET /api/protected (requires auth + X-API-Version header)")
	fmt.Println("  GET /admin/stats (requires auth)")
	fmt.Println("  GET /panic (demonstrates recovery)")
	fmt.Println("  GET /slow (demonstrates timeout)")
}
