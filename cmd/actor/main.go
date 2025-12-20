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
	"net/url"

	"strings"

	appErrors "github.com/equaltoai/lesser/pkg/errors"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	liftErrors "github.com/equaltoai/lesser/pkg/lift"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	// Standardized Lambda initialization with automatic service detection
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "actor",
		LambdaType:  common.LambdaTypeAPI,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	repos = lambdaCtx.Repos.(core.RepositoryStorage)

	// Initialize with default options for API Lambda type
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		logger.Warn("failed to initialize with defaults, some features may be limited", zap.Error(err))
	}
}

// Handler contains dependencies for the actor service
type Handler struct {
	actorRepo              *repositories.ActorRepository
	authorizedFetchService *federation.AuthorizedFetchService
}

// NewHandler creates a new handler instance using standardized services
func NewHandler() *Handler {
	// Initialize actor repository
	actorRepo := repositories.NewActorRepository(
		repos.GetDB(),
		repos.GetTableName(),
		logger)

	// Initialize authorized fetch service
	authorizedFetchService := federation.NewAuthorizedFetchService(
		repos,
		cfg.Domain,
		logger)

	return &Handler{
		actorRepo:              actorRepo,
		authorizedFetchService: authorizedFetchService,
	}
}

// HandleActorProfile handles ActivityPub actor profile requests
func (h *Handler) HandleActorProfile(ctx *lift.Context) error {
	// Extract username from path parameters
	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return liftErrors.ValidationErrorWithField("username", "missing username")
	}

	// Get request ID from context
	requestID := ctx.Get("requestID")
	if requestID == nil {
		requestID = "unknown"
	}

	logger.Info("fetching actor profile",
		zap.String("username", username),
		zap.String("accept", ctx.Header("Accept")),
		zap.Any("request_id", requestID))

	// Get actor from repository
	actor, err := h.actorRepo.GetActorByUsername(ctx.Context, username)
	if err != nil {
		if common.IsNotFound(err) {
			return liftErrors.NotFoundError("actor")
		}
		logger.Error("failed to get actor",
			zap.Error(err),
			zap.String("username", username),
			zap.Any("request_id", requestID))
		return appErrors.FailedToGet("actor", err)
	}

	// Content negotiation
	accept := ctx.Header("Accept")
	if err := common.ValidateRequiredParam("accept", accept); err != nil {
		accept = ctx.Header("accept") // Try lowercase
	}

	// Check if client wants ActivityStreams JSON
	if strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json") ||
		strings.Contains(accept, "application/json") {
		// Check if authorized fetch is enabled for ActivityPub JSON requests
		if h.authorizedFetchService.IsAuthorizedFetchEnabled(ctx.Context) {
			logger.Debug("authorized fetch enabled, verifying request",
				zap.String("username", username),
				zap.Any("request_id", requestID),
			)

			// Convert lift.Context to http.Request for signature verification
			httpReq, err := h.convertLiftRequest(ctx)
			if err != nil {
				logger.Error("failed to convert request for authorized fetch",
					zap.String("username", username),
					zap.Any("request_id", requestID),
					zap.Error(err),
				)
				return lift.NewLiftError("REQUEST_CONVERSION_ERROR", "malformed request", 400).WithCause(err)
			}

			// Verify authorized fetch
			_, err = h.authorizedFetchService.VerifyAuthorizedFetch(ctx.Context, httpReq)
			if err != nil {
				// Check if signature is missing vs invalid
				if strings.Contains(err.Error(), "missing signature") {
					logger.Debug("unauthorized request - missing signature",
						zap.String("username", username),
						zap.Any("request_id", requestID),
					)
					return lift.NewLiftError("UNAUTHORIZED", "signature required for authorized fetch", 401)
				}
				logger.Debug("authorized fetch verification failed",
					zap.String("username", username),
					zap.Any("request_id", requestID),
					zap.Error(err),
				)
				return lift.NewLiftError("FORBIDDEN", "signature verification failed", 403).WithCause(err)
			}

			logger.Debug("authorized fetch verification successful",
				zap.String("username", username),
				zap.Any("request_id", requestID),
			)
		}

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
	if err := common.ValidateRequiredParam("displayName", displayName); err != nil {
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
		displayName, actor.PreferredUsername, cfg.Domain,
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
		<div class="username">@%s@%s</div>`, displayName, actor.PreferredUsername, cfg.Domain)

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
		</div>`, actor.PreferredUsername, cfg.Domain, actor.ID)

	html += `
	</div>
</body>
</html>`

	return html
}

// convertLiftRequest converts a Lift request to an http.Request for signature verification
func (h *Handler) convertLiftRequest(ctx *lift.Context) (*http.Request, error) {
	// Build URL
	u := &url.URL{
		Scheme: "https",
		Host:   ctx.Header("Host"),
		Path:   ctx.Request.Path,
	}
	if ctx.Request.QueryParams != nil {
		q := u.Query()
		for k, v := range ctx.Request.QueryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	// Create request with context (no body for GET requests)
	req, err := http.NewRequestWithContext(ctx.Context, ctx.Request.Method, u.String(), nil)
	if err != nil {
		return nil, err
	}

	// Copy headers
	for k, v := range ctx.Request.Headers {
		req.Header.Set(k, v)
	}

	// Set host header if not present
	if err := common.ValidateRequiredParam("host", req.Header.Get("Host")); err != nil && ctx.Header("Host") != "" {
		req.Host = ctx.Header("Host")
	}

	return req, nil
}

func main() {
	// Create a new Lift application
	app := lift.New()

	// Panic recovery middleware (MUST be first to catch all panics)
	app.Use(middleware.PanicRecovery(lambdaCtx.Logger))

	// Apply federation security middleware
	middleware.ApplySecurityMiddleware(app, middleware.SecurityTypeFederation, lambdaCtx.Logger)

	// Initialize handler dependencies
	handler := NewHandler()

	// Register actor profile route
	app.GET("/users/:username", handler.HandleActorProfile)

	// Use standardized Lambda handler wrapper with observability
	standardHandler := lambdaCtx.CreateStandardizedLambdaHandler(func(ctx context.Context, event interface{}) (interface{}, error) {
		return app.HandleRequest(ctx, event)
	})

	lambda.Start(standardHandler)
}
