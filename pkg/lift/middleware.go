package lift

import (
	"log"
	"time"
)

// Middleware is a function that wraps a handler
type Middleware func(Handler) Handler

// RequestLogger is a middleware that logs requests
func RequestLogger() Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			start := time.Now()

			// Log request
			log.Printf("Request started: %s %s", ctx.Method, ctx.Path)

			// Call next handler
			err := next.Handle(ctx)

			// Log response
			duration := time.Since(start)
			if err != nil {
				log.Printf("Request failed: %s %s - %v (took %v)", ctx.Method, ctx.Path, err, duration)
			} else {
				log.Printf("Request completed: %s %s - %d (took %v)", ctx.Method, ctx.Path, ctx.Response.StatusCode, duration)
			}

			return err
		})
	}
}

// RequestID is a middleware that adds a request ID to the context
func RequestID() Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			// Request ID is already set in the context
			return next.Handle(ctx)
		})
	}
}

// Recover is a middleware that recovers from panics
func Recover() Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic: %v", r)
					ctx.Error(500, "Internal Server Error")
				}
			}()

			return next.Handle(ctx)
		})
	}
}

// CORS is a middleware that adds CORS headers
func CORS(allowOrigin string, allowMethods []string, allowHeaders []string) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			// Set CORS headers
			ctx.SetHeader("Access-Control-Allow-Origin", allowOrigin)

			if len(allowMethods) > 0 {
				ctx.SetHeader("Access-Control-Allow-Methods", joinStrings(allowMethods))
			}

			if len(allowHeaders) > 0 {
				ctx.SetHeader("Access-Control-Allow-Headers", joinStrings(allowHeaders))
			}

			// Handle preflight requests
			if ctx.Method == "OPTIONS" {
				return ctx.NoContent(204)
			}

			// Call next handler
			return next.Handle(ctx)
		})
	}
}

// joinStrings joins strings with commas
func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}

	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += ", " + strs[i]
	}

	return result
}
