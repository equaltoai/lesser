// Package main implements the webfinger Lambda function for serving WebFinger discovery protocol endpoints.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
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
	cfg        *config.Config
	repService *reputation.Service
	lambdaCtx  *common.LambdaContext
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
func (wh *WebFingerHandler) RegisterRoutes(app *liftPkg.App) {
	// WebFinger and NodeInfo endpoints
	_ = app.GET("/.well-known/webfinger", wh.handleWebFinger)
	_ = app.GET("/.well-known/nodeinfo", wh.handleNodeInfoDiscovery)
	_ = app.GET("/.well-known/reputation-keys", wh.handleReputationKeys)
	_ = app.GET("/.well-known/host-meta", wh.handleHostMeta)
	_ = app.GET("/nodeinfo/2.0", wh.handleNodeInfo20)
	_ = app.GET("/nodeinfo/2.1", wh.handleNodeInfo21)
}

// handleWebFinger handles webfinger requests using DynamORM
func (wh *WebFingerHandler) handleWebFinger(ctx *liftPkg.Context) error {
	// Validate resource parameter
	resource := ctx.Query("resource")
	if err := common.ValidateRequiredParam("resource", resource); err != nil {
		wh.logger.Warn("missing resource parameter")
		return liftPkg.ValidationError("resource parameter is required")
	}

	// Validate webfinger resource format
	if err := common.ValidateWebfingerResource(resource); err != nil {
		wh.logger.Warn("invalid webfinger resource format",
			zap.String("resource", resource),
			zap.Error(err))
		return liftPkg.ValidationError(err.Error())
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
		return liftPkg.ValidationError(err.Error())
	}

	// Check if the domain matches our instance
	if domain != cfg.Domain {
		wh.logger.Debug("domain mismatch",
			zap.String("requested_domain", domain),
			zap.String("our_domain", cfg.Domain),
		)
		return liftPkg.NotFound("actor not found")
	}

	// Check if the actor exists using DynamORM repository
	_, err = wh.actorRepo.GetActor(ctx.Context, username)
	if err != nil {
		if common.IsNotFound(err) {
			wh.logger.Debug("actor not found",
				zap.String("username", username),
			)
			return liftPkg.NotFound("actor not found")
		}
		wh.logger.Error("failed to get actor", zap.Error(err))
		return liftPkg.NewLiftError("DATABASE_ERROR", "database error", 500)
	}

	// Build WebFinger response
	actorURL := cfg.ActorURL(username)
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
				Template: fmt.Sprintf("%s/authorize_interaction?uri={uri}", cfg.BaseURL()),
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
func (wh *WebFingerHandler) handleNodeInfoDiscovery(ctx *liftPkg.Context) error {
	wh.logger.Info("handling nodeinfo discovery request",
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	discovery := map[string]any{
		"links": []map[string]string{
			{
				"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.0",
				"href": cfg.BaseURL() + "/nodeinfo/2.0",
			},
			{
				"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.1",
				"href": cfg.BaseURL() + "/nodeinfo/2.1",
			},
		},
	}

	ctx.Response.Headers["Cache-Control"] = CacheControlMaxAge
	return ctx.JSON(discovery)
}

// handleNodeInfo20 returns nodeinfo 2.0 format using DynamORM
func (wh *WebFingerHandler) handleNodeInfo20(ctx *liftPkg.Context) error {
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
			"nodeName":        cfg.InstanceName,
			"nodeDescription": "A serverless ActivityPub implementation",
		},
	}

	ctx.Response.Headers["Content-Type"] = "application/json; profile=\"http://nodeinfo.diaspora.software/ns/schema/2.0#\""
	ctx.Response.Headers["Cache-Control"] = "max-age=300"
	return ctx.JSON(nodeinfo)
}

// handleNodeInfo21 returns nodeinfo 2.1 format using DynamORM
func (wh *WebFingerHandler) handleNodeInfo21(ctx *liftPkg.Context) error {
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
			"nodeName":        cfg.InstanceName,
			"nodeDescription": "A serverless ActivityPub implementation",
		},
	}

	ctx.Response.Headers["Content-Type"] = "application/json; profile=\"http://nodeinfo.diaspora.software/ns/schema/2.1#\""
	ctx.Response.Headers["Cache-Control"] = "max-age=300"
	return ctx.JSON(nodeinfo)
}

// handleReputationKeys returns the instance's public key for reputation signing
func (wh *WebFingerHandler) handleReputationKeys(ctx *liftPkg.Context) error {
	wh.logger.Info("handling reputation keys request",
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Check if reputation service is available
	if wh.repService == nil {
		wh.logger.Warn("reputation service unavailable")
		return liftPkg.NewLiftError("SERVICE_UNAVAILABLE", "reputation service temporarily disabled", 503)
	}

	// Get the actual public key from the reputation service
	publicKeyBase64 := wh.repService.GetPublicKey()
	if err := common.ValidateRequiredParam("publicKey", publicKeyBase64); err != nil {
		wh.logger.Error("reputation service returned empty public key")
		return liftPkg.NewLiftError("INTERNAL_ERROR", "reputation service unavailable", 500)
	}

	keys := map[string]any{
		"publicKey": publicKeyBase64,
		"algorithm": "Ed25519",
		"keyId":     cfg.BaseURL() + "#reputation-key",
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
func (wh *WebFingerHandler) handleHostMeta(ctx *liftPkg.Context) error {
	wh.logger.Info("handling host-meta request",
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Generate XRD host-meta document as required by ActivityPub federation
	xrd := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<XRD xmlns="http://docs.oasis-open.org/ns/xri/xrd-1.0">
  <Link rel="lrdd" type="application/xrd+xml" template="%s/.well-known/webfinger?resource={uri}"/>
</XRD>`, cfg.BaseURL())

	// Set proper content type for XRD documents and caching
	ctx.Response.Headers["Content-Type"] = "application/xrd+xml"
	ctx.Response.Headers["Cache-Control"] = CacheControlMaxAge // Cache for 24 hours

	wh.logger.Debug("returning host-meta XRD document",
		zap.String("baseURL", cfg.BaseURL()),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	return ctx.Text(xrd)
}

// Helper methods for user and post counts using DynamORM repositories

// getUserCount gets the total user count using UserRepository
func (wh *WebFingerHandler) getUserCount(ctx *liftPkg.Context) (int, error) {
	count, err := wh.userRepo.GetTotalUserCount(ctx)
	if err != nil {
		wh.logger.Error("failed to get total user count", zap.Error(err))
		return 0, err
	}
	return int(count), nil
}

// getActiveUserCount gets active user count for a given number of days
func (wh *WebFingerHandler) getActiveUserCount(ctx *liftPkg.Context, days int) int {
	count, err := wh.userRepo.GetActiveUserCount(ctx, days)
	if err != nil {
		wh.logger.Error("failed to get active user count", zap.Error(err), zap.Int("days", days))
		return -1
	}
	return int(count)
}

// getPostCount gets the total post count using StatusRepository
func (wh *WebFingerHandler) getPostCount(ctx *liftPkg.Context) (int, error) {
	count, err := wh.statusRepo.GetTotalStatusCount(ctx)
	if err != nil {
		wh.logger.Error("failed to get total status count", zap.Error(err))
		return 0, err
	}
	return int(count), nil
}

// NewWebFingerHandler creates a new webfinger handler with standardized initialization
func NewWebFingerHandler() *WebFingerHandler {
	// Create reputation service directly
	repService, err := reputation.NewService(&reputation.Config{
		Storage:     repos,
		Logger:      logger,
		InstanceURL: cfg.BaseURL(),
		PrivateKey:  cfg.ReputationPrivateKey,
	})
	if err != nil {
		logger.Warn("failed to initialize reputation service, disabling reputation features", zap.Error(err))
		repService = nil
	}

	return &WebFingerHandler{
		actorRepo:  repos.Actor(),
		userRepo:   repos.User(),
		statusRepo: repos.Status(),
		repos:      repos,
		logger:     logger,
		cfg:        cfg,
		repService: repService,
		lambdaCtx:  lambdaCtx,
	}
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
)

func init() {
	// Standardized Lambda initialization with automatic service detection
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "webfinger",
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

func main() {
	// Create webfinger handler using standardized services
	handler := NewWebFingerHandler()

	// Create Lift application
	app := liftPkg.New()

	// Add request ID middleware (first - generates request ID)
	app.Use(func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			requestID := fmt.Sprintf("webfinger-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware (second - logs with request ID)
	app.Use(func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			start := time.Now()
			err := next.Handle(ctx)

			logger.Info("webfinger request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			if err != nil {
				logger.Error("webfinger handler error",
					zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
					zap.Error(err),
				)
			}
			return err
		})
	})

	// Add recovery middleware (third - catches panics)
	app.Use(func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered in webfinger handler",
						zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
						zap.Any("panic", r),
					)
				}
			}()
			return next.Handle(ctx)
		})
	})

	// Add federation rate limiting middleware if enabled
	if !cfg.DisableFederationRateLimiting {
		app.Use(ratelimit.FederationRateLimitMiddleware(repos))
		logger.Info("enabled federation rate limiting middleware for webfinger service")
	}

	// Register webfinger routes
	handler.RegisterRoutes(app)

	// Use standardized Lambda handler with observability
	standardHandler := lambdaCtx.CreateStandardizedLambdaHandler(func(ctx context.Context, event interface{}) (interface{}, error) {
		return app.HandleRequest(ctx, event)
	})

	lambda.Start(standardHandler)
}
