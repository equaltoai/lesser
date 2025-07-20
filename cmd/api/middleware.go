package main

import (
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// createLoggingMiddleware creates a custom logging middleware
func createLoggingMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			// Process the request
			err := next.Handle(ctx)

			// Log the request after processing
			logger.Info("API request",
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.Int("status", ctx.Response.StatusCode),
				zap.Duration("duration", time.Since(start)),
				zap.String("request_id", ctx.RequestID()))

			return err
		})
	}
}

// createCORSMiddleware creates a CORS middleware
func createCORSMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers
			ctx.SetHeader("Access-Control-Allow-Origin", "*")
			ctx.SetHeader("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
			ctx.SetHeader("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto, X-CSRF-Token")

			// Handle OPTIONS requests
			if ctx.Request.Method == "OPTIONS" {
				return ctx.NoContent(200)
			}

			// Process the request
			return next.Handle(ctx)
		})
	}
}

// createAuthMiddleware creates an authentication middleware
func createAuthMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Get the auth token from the request
			token := ctx.GetHeader("Authorization")
			if token == "" {
				return lift.Unauthorized("Missing authorization token")
			}

			// Validate the token using the auth middleware
			claims, err := authMiddleware.ValidateToken(token)
			if err != nil {
				return lift.Unauthorized("Invalid authorization token")
			}

			// Store the claims in the context
			ctx.Set("claims", claims)

			// Process the request
			return next.Handle(ctx)
		})
	}
}

// createAdminMiddleware creates a middleware that checks for admin role
func createAdminMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Get the claims from the context
			claims, ok := ctx.Get("claims")
			if !ok {
				return lift.Forbidden("Authentication required")
			}

			// TODO: Add role check to claims when role support is added
			// For now, admin check would need to be done via username or a separate lookup

			// Process the request
			return next.Handle(ctx)
		})
	}
}
