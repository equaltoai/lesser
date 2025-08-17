// Package main implements the webfinger Lambda function for serving WebFinger discovery protocol endpoints.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

const (
	// CacheControlMaxAge defines the cache control header for 24 hours
	CacheControlMaxAge = "max-age=86400"
)

// WebFingerHandler handles WebFinger and NodeInfo requests using Lift
type WebFingerHandler struct {
	actorRepo  *repositories.ActorRepository
	userRepo   *repositories.UserRepository
	statusRepo *repositories.StatusRepository
	repos      core.RepositoryStorage
	logger     *zap.Logger
	cfg        interface{} // config.Config interface
	repService *reputation.Service
	lambdaCtx  *common.LambdaContext
}

// NewWebFingerHandler creates a new webfinger handler with standardized initialization
func NewWebFingerHandler() (*WebFingerHandler, error) {
	// Use standardized Lambda initialization
	lambdaCtx, err := common.InitializeLambda(common.LambdaConfig{
		ServiceName:        "webfinger",
		LambdaType:         common.LambdaTypeBasic,
		Version:            "1.0.0",
		EnableMetrics:      true,
		EnableHealthCheck:  true,
		EnableTracing:      true,
		EnableCostTracking: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Lambda: %w", err)
	}

	// Initialize basic services using default options
	options := common.DefaultLambdaInitOptions(common.LambdaTypeBasic)
	if err := lambdaCtx.InitializeWithOptions(options); err != nil {
		return nil, fmt.Errorf("failed to initialize Lambda services: %w", err)
	}

	// Extract repositories and services from Lambda context
	repos, ok := lambdaCtx.Repos.(core.RepositoryStorage)
	if !ok {
		return nil, fmt.Errorf("failed to get repository storage from Lambda context")
	}

	// Initialize reputation service with repository storage
	repService, err := reputation.NewService(&reputation.Config{
		Storage:     repos,
		Logger:      lambdaCtx.Logger,
		InstanceURL: lambdaCtx.Config.BaseURL(),
		PrivateKey:  lambdaCtx.Config.ReputationPrivateKey,
	})
	if err != nil {
		lambdaCtx.Logger.Warn("failed to initialize reputation service, disabling reputation features", zap.Error(err))
		// Continue without reputation service rather than failing
		repService = nil
	}

	return &WebFingerHandler{
		actorRepo:  repos.Actor(),
		userRepo:   repos.User(),
		statusRepo: repos.Status(),
		repos:      repos,
		logger:     lambdaCtx.Logger,
		cfg:        lambdaCtx.Config,
		repService: repService,
		lambdaCtx:  lambdaCtx,
	}, nil
}

// parseWebFingerResource parses a WebFinger resource identifier
// Expected format: acct:username@domain
func parseWebFingerResource(resource string) (username, domain string, err error) {
	// Validate webfinger format using comprehensive validation
	if err := activitypub.ValidateWebfinger(resource); err != nil {
		return "", "", common.ValidationError{
			Field:   "resource",
			Message: err.Error(),
		}
	}

	// Extract username and domain
	acct := strings.TrimPrefix(resource, "acct:")
	parts := strings.Split(acct, "@")
	username = parts[0]
	domain = parts[1]

	return username, domain, nil
}

// RegisterRoutes registers all webfinger routes
func (wh *WebFingerHandler) RegisterRoutes(app *lift.App) {
	// WebFinger and NodeInfo endpoints
	_ = app.GET("/.well-known/webfinger", wh.handleWebFinger)
	_ = app.GET("/.well-known/nodeinfo", wh.handleNodeInfoDiscovery)
	_ = app.GET("/.well-known/reputation-keys", wh.handleReputationKeys)
	_ = app.GET("/.well-known/host-meta", wh.handleHostMeta)
	_ = app.GET("/nodeinfo/2.0", wh.handleNodeInfo20)
	_ = app.GET("/nodeinfo/2.1", wh.handleNodeInfo21)
}

// handleWebFinger handles webfinger requests using DynamORM
func (wh *WebFingerHandler) handleWebFinger(ctx *lift.Context) error {
	// Validate resource parameter
	resource := ctx.Query("resource")
	if err := common.ValidateRequiredParam("resource", resource); err != nil {
		wh.logger.Warn("missing resource parameter")
		return lift.ValidationError("resource parameter is required")
	}

	wh.logger.Info("processing webfinger request",
		zap.String("resource", resource),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Parse the resource
	username, domain, err := parseWebFingerResource(resource)
	if err != nil {
		wh.logger.Debug("invalid resource format",
			zap.String("resource", resource),
			zap.Error(err),
		)
		return lift.ValidationError(err.Error())
	}

	// Check if the domain matches our instance
	if domain != wh.lambdaCtx.Config.Domain {
		wh.logger.Debug("domain mismatch",
			zap.String("requested_domain", domain),
			zap.String("our_domain", wh.lambdaCtx.Config.Domain),
		)
		return lift.NotFound("actor not found")
	}

	// Check if the actor exists using DynamORM repository
	_, err = wh.actorRepo.GetActor(ctx.Context, username)
	if err != nil {
		if common.IsNotFound(err) {
			wh.logger.Debug("actor not found",
				zap.String("username", username),
			)
			return lift.NotFound("actor not found")
		}
		wh.logger.Error("failed to get actor", zap.Error(err))
		return lift.NewLiftError("DATABASE_ERROR", "database error", 500)
	}

	// Build WebFinger response
	actorURL := wh.lambdaCtx.Config.ActorURL(username)
	response := activitypub.WebFingerResource{
		Subject: resource,
		Aliases: []string{actorURL},
		Links: []activitypub.WebFingerLink{
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: actorURL,
			},
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: actorURL,
			},
			{
				Rel:      "http://ostatus.org/schema/1.0/subscribe",
				Template: fmt.Sprintf("%s/authorize_interaction?uri={uri}", wh.lambdaCtx.Config.BaseURL()),
			},
		},
	}

	wh.logger.Info("webfinger request successful",
		zap.String("username", username),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Return WebFinger response with proper content type and caching
	ctx.Response.Headers["Content-Type"] = "application/jrd+json"
	ctx.Response.Headers["Cache-Control"] = CacheControlMaxAge
	return ctx.JSON(response)
}

// handleNodeInfoDiscovery returns the well-known nodeinfo discovery document
func (wh *WebFingerHandler) handleNodeInfoDiscovery(ctx *lift.Context) error {
	wh.logger.Info("handling nodeinfo discovery request",
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	discovery := map[string]any{
		"links": []map[string]string{
			{
				"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.0",
				"href": wh.lambdaCtx.Config.BaseURL() + "/nodeinfo/2.0",
			},
			{
				"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.1",
				"href": wh.lambdaCtx.Config.BaseURL() + "/nodeinfo/2.1",
			},
		},
	}

	ctx.Response.Headers["Cache-Control"] = CacheControlMaxAge
	return ctx.JSON(discovery)
}

// handleNodeInfo20 returns nodeinfo 2.0 format using DynamORM
func (wh *WebFingerHandler) handleNodeInfo20(ctx *lift.Context) error {
	wh.logger.Info("handling nodeinfo 2.0 request",
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Get actual user count from user repository
	userCount, err := wh.getUserCount(ctx)
	if err != nil {
		wh.logger.Warn("failed to get user count, using default", zap.Error(err))
		userCount = 1
	}

	// Get post count
	postCount, err := wh.getPostCount(ctx)
	if err != nil {
		wh.logger.Warn("failed to get post count, using default", zap.Error(err))
		postCount = 0
	}

	nodeinfo := map[string]any{
		"version": "2.0",
		"software": map[string]any{
			"name":    "lesser",
			"version": "0.1.0",
		},
		"protocols": []string{"activitypub"},
		"services": map[string]any{
			"outbound": []string{},
			"inbound":  []string{},
		},
		"usage": map[string]any{
			"users": map[string]any{
				"total": userCount,
			},
			"localPosts": postCount,
		},
		"openRegistrations": true,
		"metadata": map[string]any{
			"nodeName":        wh.lambdaCtx.Config.InstanceName,
			"nodeDescription": "A serverless ActivityPub implementation",
		},
	}

	ctx.Response.Headers["Content-Type"] = "application/json; profile=\"http://nodeinfo.diaspora.software/ns/schema/2.0#\""
	ctx.Response.Headers["Cache-Control"] = "max-age=300"
	return ctx.JSON(nodeinfo)
}

// handleNodeInfo21 returns nodeinfo 2.1 format using DynamORM
func (wh *WebFingerHandler) handleNodeInfo21(ctx *lift.Context) error {
	wh.logger.Info("handling nodeinfo 2.1 request",
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Get actual user count from user repository
	userCount, err := wh.getUserCount(ctx)
	if err != nil {
		wh.logger.Warn("failed to get user count, using default", zap.Error(err))
		userCount = 1
	}

	// Get active user counts
	activeMonth := wh.getActiveUserCount(ctx, 30)
	if activeMonth == -1 {
		activeMonth = userCount // fallback
	}

	activeHalfyear := wh.getActiveUserCount(ctx, 180)
	if activeHalfyear == -1 {
		activeHalfyear = userCount // fallback
	}

	// Get post count
	postCount, err := wh.getPostCount(ctx)
	if err != nil {
		wh.logger.Warn("failed to get post count, using default", zap.Error(err))
		postCount = 0
	}

	nodeinfo := map[string]any{
		"version": "2.1",
		"software": map[string]any{
			"name":       "lesser",
			"version":    "0.1.0",
			"repository": "https://github.com/equaltoai/lesser",
		},
		"protocols": []string{"activitypub"},
		"services": map[string]any{
			"outbound": []string{},
			"inbound":  []string{},
		},
		"usage": map[string]any{
			"users": map[string]any{
				"total":          userCount,
				"activeMonth":    activeMonth,
				"activeHalfyear": activeHalfyear,
			},
			"localPosts": postCount,
		},
		"openRegistrations": true,
		"metadata": map[string]any{
			"nodeName":        wh.lambdaCtx.Config.InstanceName,
			"nodeDescription": "A serverless ActivityPub implementation",
		},
	}

	ctx.Response.Headers["Content-Type"] = "application/json; profile=\"http://nodeinfo.diaspora.software/ns/schema/2.1#\""
	ctx.Response.Headers["Cache-Control"] = "max-age=300"
	return ctx.JSON(nodeinfo)
}

// handleReputationKeys returns the instance's public key for reputation signing
func (wh *WebFingerHandler) handleReputationKeys(ctx *lift.Context) error {
	wh.logger.Info("handling reputation keys request",
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Check if reputation service is available
	if wh.repService == nil {
		wh.logger.Warn("reputation service unavailable")
		return lift.NewLiftError("SERVICE_UNAVAILABLE", "reputation service temporarily disabled", 503)
	}

	// Get the actual public key from the reputation service
	publicKeyBase64 := wh.repService.GetPublicKey()
	if err := common.ValidateRequiredParam("publicKey", publicKeyBase64); err != nil {
		wh.logger.Error("reputation service returned empty public key")
		return lift.NewLiftError("INTERNAL_ERROR", "reputation service unavailable", 500)
	}

	keys := map[string]any{
		"publicKey": publicKeyBase64,
		"algorithm": "Ed25519",
		"keyId":     wh.lambdaCtx.Config.BaseURL() + "#reputation-key",
		"created":   time.Now().UTC().Format(time.RFC3339),
	}

	wh.logger.Debug("returning reputation keys",
		zap.String("keyId", keys["keyId"].(string)),
		zap.String("publicKey", publicKeyBase64[:20]+"..."),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	ctx.Response.Headers["Cache-Control"] = "max-age=3600"
	return ctx.JSON(keys)
}

// handleHostMeta returns the XRD host-meta document for federation discovery
func (wh *WebFingerHandler) handleHostMeta(ctx *lift.Context) error {
	wh.logger.Info("handling host-meta request",
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Generate XRD host-meta document as required by ActivityPub federation
	xrd := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<XRD xmlns="http://docs.oasis-open.org/ns/xri/xrd-1.0">
  <Link rel="lrdd" type="application/xrd+xml" template="%s/.well-known/webfinger?resource={uri}"/>
</XRD>`, wh.lambdaCtx.Config.BaseURL())

	// Set proper content type for XRD documents and caching
	ctx.Response.Headers["Content-Type"] = "application/xrd+xml"
	ctx.Response.Headers["Cache-Control"] = CacheControlMaxAge // Cache for 24 hours

	wh.logger.Debug("returning host-meta XRD document",
		zap.String("baseURL", wh.lambdaCtx.Config.BaseURL()),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	return ctx.Text(xrd)
}

// Helper methods for user and post counts using DynamORM repositories

// getUserCount gets the total user count using UserRepository
func (wh *WebFingerHandler) getUserCount(ctx *lift.Context) (int, error) {
	count, err := wh.userRepo.GetTotalUserCount(ctx)
	if err != nil {
		wh.logger.Error("failed to get total user count", zap.Error(err))
		return 0, err
	}
	return int(count), nil
}

// getActiveUserCount gets active user count for a given number of days
func (wh *WebFingerHandler) getActiveUserCount(ctx *lift.Context, days int) int {
	count, err := wh.userRepo.GetActiveUserCount(ctx, days)
	if err != nil {
		wh.logger.Error("failed to get active user count", zap.Error(err), zap.Int("days", days))
		return -1
	}
	return int(count)
}

// getPostCount gets the total post count using StatusRepository
func (wh *WebFingerHandler) getPostCount(ctx *lift.Context) (int, error) {
	count, err := wh.statusRepo.GetTotalStatusCount(ctx)
	if err != nil {
		wh.logger.Error("failed to get total status count", zap.Error(err))
		return 0, err
	}
	return int(count), nil
}

func main() {
	// Create the handler with standardized initialization
	handler, err := NewWebFingerHandler()
	if err != nil {
		panic(fmt.Sprintf("failed to create handler: %v", err))
	}

	// Create new Lift app
	app := lift.New()

	// Add request ID middleware (first - generates request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("webfinger-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware (second - logs with request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			path := ctx.Request.Path
			method := ctx.Request.Method

			err := next.Handle(ctx)

			handler.logger.Info("webfinger request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			return err
		})
	})

	// Add recovery middleware (third - catches panics)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					handler.logger.Error("panic recovered in webfinger handler",
						zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
						zap.Any("panic", r))
					if err := ctx.Status(500).Text("Internal server error"); err != nil {
						handler.logger.Error("failed to send error response", zap.Error(err))
					}
				}
			}()

			return next.Handle(ctx)
		})
	})

	// Add federation rate limiting middleware (fourth in chain)
	if os.Getenv("DISABLE_FEDERATION_RATE_LIMITING") != "true" {
		app.Use(ratelimit.FederationRateLimitMiddleware(handler.repos))
		handler.logger.Info("enabled federation rate limiting middleware for webfinger service")
	}

	// Register all webfinger routes
	handler.RegisterRoutes(app)

	// Use standardized Lambda handler wrapper with observability
	standardHandler := handler.lambdaCtx.CreateStandardizedLambdaHandler(func(ctx context.Context, event interface{}) (interface{}, error) {
		return app.HandleRequest(ctx, event)
	})

	lambda.Start(standardHandler)
}
