// Package main implements the actor service Lambda function that handles
// ActivityPub actor operations, federation lookups, and actor profile management.
package main

/*
Actor Service - ActivityPub Federation Handler

This Lambda function handles ActivityPub federation requests for actor profiles.
It serves ActivityPub JSON to other ActivityPub servers and HTML to browsers.

This is NOT the Mastodon client API - that's handled by the /cmd/api service.
This service handles federation requests from other ActivityPub servers.
*/

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	liftErrors "github.com/equaltoai/lesser/pkg/lift"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

var (
	cfg       *config.Config
	actorRepo *repositories.ActorRepository
	logger    *zap.Logger
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	// Initialize DynamORM database connection
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
	}

	// Initialize actor repository
	tableName := cfg.DynamoTableName
	if tableName == "" {
		tableName = "lesser-main" // Default table name
	}
	actorRepo = repositories.NewActorRepository(db, tableName, logger)
}

// Handler contains dependencies for the actor service
type Handler struct {
	cfg       *config.Config
	actorRepo *repositories.ActorRepository
	logger    *zap.Logger
}

// NewHandler creates a new handler instance
func NewHandler(cfg *config.Config, actorRepo *repositories.ActorRepository, logger *zap.Logger) *Handler {
	return &Handler{
		cfg:       cfg,
		actorRepo: actorRepo,
		logger:    logger,
	}
}

// HandleActorProfile handles ActivityPub actor profile requests
func (h *Handler) HandleActorProfile(ctx *lift.Context) error {
	// Extract username from path parameters
	username := ctx.Param("username")
	if username == "" {
		return liftErrors.ValidationErrorWithField("username", "missing username")
	}

	// Get request ID from context
	requestID := ctx.Get("requestID")
	if requestID == nil {
		requestID = "unknown"
	}

	h.logger.Info("fetching actor profile",
		zap.String("username", username),
		zap.String("accept", ctx.Header("Accept")),
		zap.Any("request_id", requestID))

	// Get actor from repository
	actor, err := h.actorRepo.GetActorByUsername(ctx.Context, username)
	if err != nil {
		if common.IsNotFound(err) {
			return liftErrors.NotFoundError("actor")
		}
		h.logger.Error("failed to get actor",
			zap.Error(err),
			zap.String("username", username),
			zap.Any("request_id", requestID))
		return fmt.Errorf("failed to get actor: %w", err)
	}

	// Content negotiation
	accept := ctx.Header("Accept")
	if accept == "" {
		accept = ctx.Header("accept") // Try lowercase
	}

	// Check if client wants ActivityStreams JSON
	if strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json") ||
		strings.Contains(accept, "application/json") {
		// Return ActivityStreams JSON
		ctx.Response.Headers["Content-Type"] = "application/activity+json"
		return ctx.JSON(actor)
	}

	// Return HTML for browsers
	html := h.generateHTMLProfile(actor)
	ctx.Response.Headers["Content-Type"] = "text/html; charset=utf-8"
	ctx.Response.StatusCode = http.StatusOK
	ctx.Response.Body = html
	return nil
}

func (h *Handler) generateHTMLProfile(actor *activitypub.Actor) string {
	// Extract display name or fall back to username
	displayName := actor.Name
	if displayName == "" {
		displayName = actor.PreferredUsername
	}

	// Build social media meta tags for better sharing
	metaTags := fmt.Sprintf(`
		<meta property="og:type" content="profile">
		<meta property="og:title" content="%s">
		<meta property="og:description" content="%s">
		<meta property="og:url" content="%s">`,
		displayName,
		actor.BaseObject.Summary,
		actor.ID)

	if actor.Icon != nil && actor.Icon.URL != "" {
		metaTags += fmt.Sprintf(`
		<meta property="og:image" content="%s">`, actor.Icon.URL)
	}

	// Generate followers/following counts if available
	statsHTML := ""
	if actor.Followers != "" && actor.Following != "" {
		statsHTML = fmt.Sprintf(`
		<div class="stats">
			<p><a href="%s">Followers</a> | <a href="%s">Following</a></p>
		</div>`, actor.Followers, actor.Following)
	}

	// Build the HTML page
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>%s (@%s@%s)</title>
	%s
	<link rel="alternate" type="application/activity+json" href="%s">
	<style>
		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
			max-width: 600px;
			margin: 40px auto;
			padding: 20px;
			line-height: 1.6;
			color: #333;
			background-color: #f5f5f5;
		}
		.profile {
			background: white;
			border-radius: 8px;
			padding: 30px;
			box-shadow: 0 2px 4px rgba(0,0,0,0.1);
		}
		.avatar {
			width: 100px;
			height: 100px;
			border-radius: 50%%;
			margin-bottom: 20px;
		}
		h1 {
			margin: 0 0 10px 0;
			font-size: 24px;
		}
		.username {
			color: #666;
			font-size: 16px;
			margin-bottom: 20px;
		}
		.bio {
			margin-bottom: 20px;
		}
		.stats {
			border-top: 1px solid #eee;
			padding-top: 20px;
			margin-top: 20px;
		}
		.stats a {
			color: #0066cc;
			text-decoration: none;
			margin-right: 20px;
		}
		.stats a:hover {
			text-decoration: underline;
		}
		.meta {
			margin-top: 20px;
			padding-top: 20px;
			border-top: 1px solid #eee;
			font-size: 14px;
			color: #666;
		}
		.meta a {
			color: #0066cc;
			text-decoration: none;
		}
	</style>
</head>
<body>
	<div class="profile">`,
		displayName, actor.PreferredUsername, h.cfg.Domain,
		metaTags,
		actor.ID)

	// Add avatar if available
	if actor.Icon != nil && actor.Icon.URL != "" {
		html += fmt.Sprintf(`
		<img src="%s" alt="%s" class="avatar">`, actor.Icon.URL, displayName)
	}

	// Add profile content
	html += fmt.Sprintf(`
		<h1>%s</h1>
		<div class="username">@%s@%s</div>`, displayName, actor.PreferredUsername, h.cfg.Domain)

	// Add bio if available
	if actor.BaseObject.Summary != "" {
		html += fmt.Sprintf(`
		<div class="bio">%s</div>`, actor.BaseObject.Summary)
	}

	// Add stats if available
	html += statsHTML

	// Add ActivityPub discovery info
	html += fmt.Sprintf(`
		<div class="meta">
			<p>This is an ActivityPub profile. You can follow @%s@%s from any compatible server.</p>
			<p><a href="%s" type="application/activity+json">View ActivityPub data</a></p>
		</div>`, actor.PreferredUsername, h.cfg.Domain, actor.ID)

	html += `
	</div>
</body>
</html>`

	return html
}

func main() {
	// Create a new Lift application
	app := lift.New()

	// Add request ID middleware (first - generates request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("actor-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware (second - logs with request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			err := next.Handle(ctx)
			duration := time.Since(start)

			requestID := ctx.Get("requestID")
			logger.Info("request completed",
				zap.Any("request_id", requestID),
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.URL().Path),
				zap.Duration("duration", duration),
				zap.Int("status", ctx.Response.StatusCode),
			)
			return err
		})
	})

	// Add recovery middleware (third - catches panics)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					requestID := ctx.Get("requestID")
					logger.Error("panic recovered",
						zap.Any("request_id", requestID),
						zap.Any("panic", r),
					)
					_ = ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
						"error": "Internal server error",
					})
				}
			}()
			return next.Handle(ctx)
		})
	})

	// Add CORS middleware for federation compatibility
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers for ActivityPub federation
			ctx.Response.Headers["Access-Control-Allow-Origin"] = "*"
			ctx.Response.Headers["Access-Control-Allow-Methods"] = "GET, HEAD, OPTIONS"
			ctx.Response.Headers["Access-Control-Allow-Headers"] = "Accept, Authorization, Content-Type, Signature, Date, Digest"

			// Handle preflight requests
			if ctx.Request.Method == "OPTIONS" {
				return ctx.Status(http.StatusNoContent).JSON(nil)
			}

			return next.Handle(ctx)
		})
	})

	// Create handler instance
	handler := NewHandler(cfg, actorRepo, logger)

	// Define routes for ActivityPub federation
	_ = app.GET("/users/:username", handler.HandleActorProfile)

	// Start the Lambda handler
	lambda.Start(app.HandleRequest)
}
