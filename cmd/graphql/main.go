package main

/*
Lesser GraphQL Server - GraphQL API for ActivityPub implementation

This Lambda function serves the Lesser GraphQL API using the Lift framework.

IMPORTANT: This service is temporarily disabled during the DynamORM migration.
All GraphQL requests will return a "service disabled" message until migration is complete.
*/

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

var (
	logger   *zap.Logger
	initTime time.Time
)

func init() {
	initTime = time.Now()
	logger = common.Logger()
	
	logger.Warn("GraphQL service starting in disabled mode during DynamORM migration")
}

// Simplified GraphQL handler that returns service disabled message
func handleGraphQL(ctx *lift.Context) error {
	logger.Info("GraphQL request received during migration",
		zap.String("method", ctx.Request.Method),
		zap.String("path", ctx.Request.Path))

	// Set content type for GraphQL response
	ctx.Response.Headers["Content-Type"] = "application/json"
	
	// Return GraphQL-formatted error indicating service is disabled
	return ctx.JSON(map[string]interface{}{
		"errors": []map[string]interface{}{
			{
				"message": "GraphQL API is temporarily disabled during DynamORM migration. Please check back later.",
				"extensions": map[string]interface{}{
					"code":      "SERVICE_DISABLED",
					"temporary": true,
				},
			},
		},
	})
}

// Simplified playground handler
func handlePlayground(ctx *lift.Context) error {
	if os.Getenv("ENABLE_PLAYGROUND") != "true" {
		return lift.NotFound("Playground not enabled")
	}

	logger.Info("GraphQL playground request received during migration")

	// Set content type for HTML response
	ctx.Response.Headers["Content-Type"] = "text/html"
	
	// Return HTML page explaining the service is disabled
	return ctx.HTML(`<!DOCTYPE html>
<html>
<head>
	<title>GraphQL Playground - Service Disabled</title>
	<style>
		body { font-family: Arial, sans-serif; margin: 40px; }
		.banner { background: #ff6b6b; color: white; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
		.info { background: #f8f9fa; padding: 20px; border-radius: 8px; border-left: 4px solid #007bff; }
	</style>
</head>
<body>
	<div class="banner">
		<h1>🚧 GraphQL Playground - Service Temporarily Disabled</h1>
	</div>
	<div class="info">
		<h2>Migration in Progress</h2>
		<p>The Lesser GraphQL API is currently undergoing a migration to the DynamORM framework.</p>
		<p>This service will be restored once the migration is complete.</p>
		<p><strong>Status:</strong> Phase 4 - GraphQL Service Migration</p>
		<p><strong>Expected:</strong> Service will be restored after storage layer migration</p>
	</div>
</body>
</html>`)
}

func main() {
	// Create a new Lift application
	app := lift.New()
	if os.Getenv("DEBUG") == "true" {
		app = lift.New(lift.WithDebug())
	}

	// Add basic request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("graphql-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			path := ctx.Request.Path
			method := ctx.Request.Method

			err := next.Handle(ctx)

			logger.Info("request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Int("status", ctx.Response.StatusCode))

			return err
		})
	})

	// Add CORS middleware for GraphQL compatibility
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers
			ctx.Response.Headers["Access-Control-Allow-Origin"] = "*"
			ctx.Response.Headers["Access-Control-Allow-Methods"] = "GET, POST, OPTIONS"
			ctx.Response.Headers["Access-Control-Allow-Headers"] = "Content-Type, Authorization"

			// Handle preflight requests
			if ctx.Request.Method == "OPTIONS" {
				ctx.Response.StatusCode = 200
				return nil
			}

			return next.Handle(ctx)
		})
	})

	// Configure routes for GraphQL
	app.POST("/graphql", handleGraphQL)
	app.GET("/graphql", handleGraphQL)
	app.GET("/playground", handlePlayground)

	// Add a health check endpoint
	app.GET("/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]interface{}{
			"status":      "disabled",
			"service":     "graphql",
			"migration":   "in_progress",
			"phase":       "dynamorm_migration",
			"uptime":      time.Since(initTime).String(),
		})
	})

	logger.Info("GraphQL service starting (disabled mode)",
		zap.String("version", "lift-migration"),
		zap.Bool("enabled", false),
		zap.String("reason", "DynamORM migration in progress"))

	// Start the Lambda handler
	lambda.Start(app.HandleRequest)
}