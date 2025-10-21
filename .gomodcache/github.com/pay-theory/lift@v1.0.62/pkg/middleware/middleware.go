package middleware

import (
	"fmt"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// Middleware represents a middleware function
type Middleware func(lift.Handler) lift.Handler

// Chain combines multiple middleware into a single middleware
func Chain(middlewares ...Middleware) Middleware {
	return func(handler lift.Handler) lift.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
		return handler
	}
}

// Logger provides structured request/response logging
func Logger() Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			// Add request ID to logger if available
			if ctx.Logger != nil {
				ctx.Logger = ctx.Logger.WithField("request_id", ctx.RequestID)
			}

			err := next.Handle(ctx)

			// Log request completion
			if ctx.Logger != nil {
				fields := map[string]any{
					"method":   ctx.Request.Method,
					"path":     ctx.Request.Path,
					"status":   ctx.Response.StatusCode,
					"duration": time.Since(start).Milliseconds(),
				}

				if err != nil {
					fields["error"] = "[REDACTED_ERROR_DETAIL]" // Sanitized for security
					ctx.Logger.Error("Request failed", fields)
				} else {
					ctx.Logger.Info("Request completed", fields)
				}
			}

			return err
		})
	}
}

// Recover provides panic recovery and graceful error handling
func Recover() Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					if ctx.Logger != nil {
						ctx.Logger.Error("Handler panicked", map[string]any{
							"panic": "[REDACTED_PANIC_DETAIL]", // Sanitized for security
						})
					}

					// Set error response
					if err := ctx.Response.Status(500).JSON(map[string]any{
						"error": "Internal server error",
						"code":  "PANIC_RECOVERED",
					}); err != nil {
						// Log that we couldn't send the error response
						if ctx.Logger != nil {
							ctx.Logger.Error("Failed to send panic recovery response", map[string]any{
								"response_error": "[SANITIZED_ERROR]", // Sanitized for security
							})
						}
					}
				}
			}()

			return next.Handle(ctx)
		})
	}
}

// CORS provides cross-origin resource sharing headers
func CORS(allowedOrigins []string) Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			origin := ctx.Header("Origin")

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range allowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				ctx.Response.Header("Access-Control-Allow-Origin", origin)
				ctx.Response.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				ctx.Response.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
				ctx.Response.Header("Access-Control-Allow-Credentials", "true")
			}

			// Handle preflight requests
			if ctx.Request.Method == "OPTIONS" {
				ctx.Response.Status(204)
				return nil
			}

			return next.Handle(ctx)
		})
	}
}

// Timeout adds request timeout handling
func Timeout(duration time.Duration) Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Use the context's WithTimeout utility
			_, err := ctx.WithTimeout(duration, func() (any, error) {
				return nil, next.Handle(ctx)
			})

			return err
		})
	}
}

// Metrics collects basic performance metrics
func Metrics() Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			err := next.Handle(ctx)

			if ctx.Metrics != nil {
				// Request counter
				counter := ctx.Metrics.Counter("requests_total", map[string]string{
					"method": ctx.Request.Method,
					"status": fmt.Sprintf("%d", ctx.Response.StatusCode),
				})
				counter.Inc()

				// Request duration
				histogram := ctx.Metrics.Histogram("request_duration_ms", map[string]string{
					"method": ctx.Request.Method,
				})
				histogram.Observe(float64(time.Since(start).Milliseconds()))

				// Error counter
				if err != nil {
					errorCounter := ctx.Metrics.Counter("errors_total", map[string]string{
						"method": ctx.Request.Method,
					})
					errorCounter.Inc()
				}
			}

			return err
		})
	}
}

// RequestID generates and sets a unique request ID
func RequestID() Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Check if request ID already exists in headers
			requestID := ctx.Header("X-Request-ID")
			if requestID == "" {
				// Generate a simple request ID (in production, use UUID)
				requestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
			}

			// Set in context
			ctx.RequestID = requestID

			// Add to response headers
			ctx.Response.Header("X-Request-ID", requestID)

			return next.Handle(ctx)
		})
	}
}

// ErrorHandler converts errors to appropriate HTTP responses
func ErrorHandler() Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)
			if err == nil {
				return nil
			}

			// Handle LiftError specifically
			if liftErr, ok := err.(*lift.LiftError); ok {
				if jsonErr := ctx.Response.Status(liftErr.StatusCode).JSON(liftErr); jsonErr != nil {
					// Log that we couldn't send the error response
					if ctx.Logger != nil {
						ctx.Logger.Error("Failed to send LiftError response", map[string]any{
							"original_error": liftErr.Error(),
							"response_error": jsonErr.Error(),
						})
					}
				}
				return nil
			}

			// Handle generic errors
			if jsonErr := ctx.Response.Status(500).JSON(map[string]any{
				"error": "Internal server error",
				"code":  "GENERIC_ERROR",
			}); jsonErr != nil {
				// Log that we couldn't send the error response
				if ctx.Logger != nil {
					ctx.Logger.Error("Failed to send generic error response", map[string]any{
						"original_error": err.Error(),
						"response_error": jsonErr.Error(),
					})
				}
			}

			return nil // Error handled, don't propagate
		})
	}
}
