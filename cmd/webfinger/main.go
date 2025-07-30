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
	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/equaltoai/lesser/pkg/storage/dynamodb"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// WebFingerHandler handles WebFinger and NodeInfo requests using Lift
type WebFingerHandler struct {
	actorRepo   *repositories.ActorRepository
	userRepo    *repositories.UserRepository
	logger      *zap.Logger
	cfg         *config.Config
	repService  *reputation.Service
}

// NewWebFingerHandler creates a new webfinger handler with DynamORM repositories
func NewWebFingerHandler() (*WebFingerHandler, error) {
	logger := common.Logger()
	cfg := config.Get()

	// Initialize DynamORM database connection using the established pattern
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM database: %w", err)
	}

	// Initialize repositories
	tableName := "lesser-main"
	actorRepo := repositories.NewActorRepository(db, tableName, logger)
	userRepo := repositories.NewUserRepository(db, tableName, logger)

	// Initialize legacy storage for reputation service (temporary bridge)
	// TODO: Migrate reputation service to DynamORM in future phase
	legacyStore, err := dynamodb.New()
	if err != nil {
		logger.Warn("failed to initialize legacy storage for reputation, disabling reputation features", zap.Error(err))
		// Continue without reputation service rather than failing
		return &WebFingerHandler{
			actorRepo:  actorRepo,
			userRepo:   userRepo,
			logger:     logger,
			cfg:        cfg,
			repService: nil, // Disabled
		}, nil
	}

	repService, err := reputation.NewService(&reputation.Config{
		Storage:     legacyStore,
		Logger:      logger,
		InstanceURL: cfg.BaseURL(),
		PrivateKey:  cfg.ReputationPrivateKey,
	})
	if err != nil {
		logger.Warn("failed to initialize reputation service, disabling reputation features", zap.Error(err))
		// Continue without reputation service rather than failing
		repService = nil
	}

	return &WebFingerHandler{
		actorRepo:  actorRepo,
		userRepo:   userRepo,
		logger:     logger,
		cfg:        cfg,
		repService: repService,
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
	app.GET("/.well-known/webfinger", wh.handleWebFinger)
	app.GET("/.well-known/nodeinfo", wh.handleNodeInfoDiscovery)
	app.GET("/.well-known/reputation-keys", wh.handleReputationKeys)
	app.GET("/nodeinfo/2.0", wh.handleNodeInfo20)
	app.GET("/nodeinfo/2.1", wh.handleNodeInfo21)
}

// handleWebFinger handles webfinger requests using DynamORM
func (wh *WebFingerHandler) handleWebFinger(ctx *lift.Context) error {
	// Validate resource parameter
	resource := ctx.Query("resource")
	if resource == "" {
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
	if domain != wh.cfg.Domain {
		wh.logger.Debug("domain mismatch",
			zap.String("requested_domain", domain),
			zap.String("our_domain", wh.cfg.Domain),
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
	actorURL := wh.cfg.ActorURL(username)
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
				Template: fmt.Sprintf("%s/authorize_interaction?uri={uri}", wh.cfg.BaseURL()),
			},
		},
	}

	wh.logger.Info("webfinger request successful",
		zap.String("username", username),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
	)

	// Return WebFinger response with proper content type and caching
	ctx.Response.Headers["Content-Type"] = "application/jrd+json"
	ctx.Response.Headers["Cache-Control"] = "max-age=86400"
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
				"href": wh.cfg.BaseURL() + "/nodeinfo/2.0",
			},
			{
				"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.1",
				"href": wh.cfg.BaseURL() + "/nodeinfo/2.1",
			},
		},
	}

	ctx.Response.Headers["Cache-Control"] = "max-age=86400"
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
			"nodeName":        wh.cfg.InstanceName,
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

	// Get active user counts (TODO: implement these methods in UserRepository)
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
				"total":           userCount,
				"activeMonth":     activeMonth,
				"activeHalfyear":  activeHalfyear,
			},
			"localPosts": postCount,
		},
		"openRegistrations": true,
		"metadata": map[string]any{
			"nodeName":        wh.cfg.InstanceName,
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
	if publicKeyBase64 == "" {
		wh.logger.Error("reputation service returned empty public key")
		return lift.NewLiftError("INTERNAL_ERROR", "reputation service unavailable", 500)
	}

	keys := map[string]any{
		"publicKey": publicKeyBase64,
		"algorithm": "Ed25519",
		"keyId":     wh.cfg.BaseURL() + "#reputation-key",
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

// Helper methods for user and post counts using DynamORM repositories

// getUserCount gets the total user count using UserRepository
func (wh *WebFingerHandler) getUserCount(ctx *lift.Context) (int, error) {
	// TODO: Implement GetTotalUserCount in UserRepository
	// For now, return a placeholder count
	wh.logger.Debug("getUserCount not fully implemented, returning placeholder")
	return 1, nil
}

// getActiveUserCount gets active user count for a given number of days
func (wh *WebFingerHandler) getActiveUserCount(ctx *lift.Context, days int) int {
	// TODO: Implement GetActiveUserCount in UserRepository
	// For now, return -1 to indicate not implemented
	wh.logger.Debug("getActiveUserCount not implemented", zap.Int("days", days))
	return -1
}

// getPostCount gets the total post count using StatusRepository
func (wh *WebFingerHandler) getPostCount(ctx *lift.Context) (int, error) {
	// TODO: Implement GetTotalStatusCount in StatusRepository
	// For now, return a placeholder count
	wh.logger.Debug("getPostCount not fully implemented, returning placeholder")
	return 0, nil
}

func main() {
	// Create the handler
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
					ctx.Status(500).Text("Internal server error")
				}
			}()

			return next.Handle(ctx)
		})
	})

	// Register all webfinger routes
	handler.RegisterRoutes(app)

	// Use app.HandleRequest for Lambda (not app.Start())
	lambda.Start(app.HandleRequest)
}
