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
				zap.String("request_id", ctx.GetRequestID()))

			return err
		})
	}
}

// createCORSMiddleware creates a CORS middleware
func createCORSMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers
			ctx.Response.Header("Access-Control-Allow-Origin", "*")
			ctx.Response.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
			ctx.Response.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto, X-CSRF-Token")

			// Handle OPTIONS requests
			if ctx.Request.Method == "OPTIONS" {
				return ctx.Status(200).Text("")
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
			token := ctx.Header("Authorization")
			if token == "" {
				return ctx.Unauthorized("Missing authorization token", nil)
			}

			// Validate the token using the auth middleware
			claims, err := authMiddleware.ValidateToken(token)
			if err != nil {
				return ctx.Unauthorized("Invalid authorization token", err)
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
			claims := ctx.Get("claims")
			if claims == nil {
				return ctx.Forbidden("Authentication required", nil)
			}

			// TODO: Add role check to claims when role support is added
			// For now, admin check would need to be done via username or a separate lookup

			// Process the request
			return next.Handle(ctx)
		})
	}
}
