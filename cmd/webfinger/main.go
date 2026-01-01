// Package main implements the webfinger Lambda function for serving WebFinger discovery protocol endpoints.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

const (
	// CacheControlMaxAge defines the cache control header for 24 hours
	CacheControlMaxAge = "max-age=86400"
)

// WebFingerHandler handles WebFinger and NodeInfo requests using Lift
type WebFingerHandler struct {
	actorRepo interfaces.ActorRepository
	repos     core.RepositoryStorage
	logger    *zap.Logger
	cfg       *config.Config
	lambdaCtx *common.LambdaContext
}

// parseWebFingerResource parses a WebFinger resource identifier
// Expected format: acct:username@domain
func parseWebFingerResource(resource string) (username, domain string, err error) {
	// Validate webfinger format using comprehensive validation
	if err := activitypub.ValidateWebfinger(resource); err != nil {
		return "", "", common.ValidationError{Field: "resource", Message: err.Error()}
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
	// WebFinger endpoint (inventory-owned).
	_ = app.GET("/.well-known/webfinger", wh.handleWebFinger)
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

	state, stateErr := wh.repos.Instance().GetInstanceState(ctx.Context)
	bootstrapUsername := storageModels.DefaultBootstrapUsername
	if stateErr == nil && strings.TrimSpace(state.BootstrapUsername) != "" {
		bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
	}

	locked := stateErr != nil || state.Locked
	if locked && !strings.EqualFold(username, bootstrapUsername) {
		return liftPkg.NotFound("actor not found")
	}

	// When locked, allow WebFinger discovery only for the bootstrap actor.
	// Ensure the bootstrap actor exists so federation endpoints can return empty collections.
	if locked && strings.EqualFold(username, bootstrapUsername) {
		if err := wh.ensureBootstrapActor(ctx.Context, bootstrapUsername); err != nil {
			wh.logger.Error("failed to ensure bootstrap actor", zap.Error(err))
			return liftPkg.NewLiftError("INTERNAL_ERROR", "failed to initialize bootstrap actor", 500)
		}
	}

	// For non-bootstrap actors (or unlocked instances), require the actor record to exist.
	if !locked || !strings.EqualFold(username, bootstrapUsername) {
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

func (wh *WebFingerHandler) ensureBootstrapActor(ctx context.Context, username string) error {
	_, err := wh.actorRepo.GetActor(ctx, username)
	if err == nil {
		return nil
	}
	if !common.IsNotFound(err) {
		return err
	}

	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return err
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	actorID := fmt.Sprintf("https://%s/users/%s", cfg.Domain, username)
	now := time.Now().UTC()
	actor := activitypub.NewActor(activitypub.PersonType, actorID, username)
	actor.Name = username
	actor.URL = fmt.Sprintf("https://%s/@%s", cfg.Domain, username)
	actor.CreatedAt = &now
	actor.PublicKey = &activitypub.PublicKey{
		ID:           fmt.Sprintf("%s#main-key", actorID),
		Owner:        actorID,
		PublicKeyPem: string(publicKeyPEM),
	}
	actor.Endpoints = &activitypub.Endpoints{
		SharedInbox: fmt.Sprintf("https://%s/inbox", cfg.Domain),
	}
	actor.Inbox = fmt.Sprintf("%s/inbox", actorID)
	actor.Outbox = fmt.Sprintf("%s/outbox", actorID)
	actor.Followers = fmt.Sprintf("%s/followers", actorID)
	actor.Following = fmt.Sprintf("%s/following", actorID)

	if err := wh.actorRepo.CreateActor(ctx, actor, string(privateKeyPEM)); err != nil {
		if _, ok := err.(common.ConflictError); ok {
			return nil
		}
		return err
	}

	return nil
}

// NewWebFingerHandler creates a new webfinger handler with standardized initialization
func NewWebFingerHandler() *WebFingerHandler {
	return &WebFingerHandler{
		actorRepo: repos.Actor(),
		repos:     repos,
		logger:    logger,
		cfg:       cfg,
		lambdaCtx: lambdaCtx,
	}
}

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

	// Panic recovery middleware (MUST be first to catch all panics)
	app.Use(middleware.PanicRecovery(lambdaCtx.Logger))

	// Apply federation security middleware
	middleware.ApplySecurityMiddleware(app, middleware.SecurityTypeFederation, lambdaCtx.Logger)

	// Add request ID middleware
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

	// Rate limiting is now handled by ApplySecurityMiddleware

	// Register webfinger routes
	handler.RegisterRoutes(app)

	// Use standardized Lambda handler with observability
	standardHandler := lambdaCtx.CreateStandardizedLambdaHandler(func(ctx context.Context, event interface{}) (interface{}, error) {
		return app.HandleRequest(ctx, event)
	})

	lambda.Start(standardHandler)
}
