// Package main implements the webfinger Lambda function for serving WebFinger discovery protocol endpoints.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/crawler"
	"github.com/equaltoai/lesser/pkg/federation/surface"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

const (
	// CacheControlMaxAge defines the cache control header for 24 hours
	CacheControlMaxAge = "max-age=86400"
)

// WebFingerHandler handles WebFinger and NodeInfo requests.
type WebFingerHandler struct {
	actorRepo    interfaces.ActorRepository
	repos        core.RepositoryStorage
	instanceRepo instanceStateRepository
	logger       *zap.Logger
	cfg          *config.Config
	lambdaCtx    *common.LambdaContext
}

type instanceStateRepository interface {
	GetInstanceState(ctx context.Context) (*storageModels.InstanceState, error)
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
func (wh *WebFingerHandler) RegisterRoutes(app *apptheory.App) {
	// WebFinger endpoint (inventory-owned).
	_ = app.Get("/.well-known/webfinger", wh.handleWebFinger)
}

// handleWebFinger handles webfinger requests using DynamORM
func (wh *WebFingerHandler) handleWebFinger(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Validate resource parameter
	resource := webfingerQueryValue(ctx, "resource")
	if err := common.ValidateRequiredParam("resource", resource); err != nil {
		wh.logger.Warn("missing resource parameter")
		return webfingerJSONError(http.StatusUnprocessableEntity, "resource parameter is required"), nil
	}

	// Validate webfinger resource format
	if err := common.ValidateWebfingerResource(resource); err != nil {
		wh.logger.Warn("invalid webfinger resource format",
			zap.String("resource", resource),
			zap.Error(err))
		return webfingerJSONError(http.StatusUnprocessableEntity, err.Error()), nil
	}

	wh.logger.Info("processing webfinger request",
		zap.String("resource", resource),
		zap.String("request_id", webfingerContextRequestID(ctx)),
	)

	// Parse the resource
	username, domain, err := parseWebFingerResource(resource)
	if err != nil {
		wh.logger.Debug("invalid resource format",
			zap.String("resource", resource),
			zap.Error(err),
		)
		return webfingerJSONError(http.StatusUnprocessableEntity, err.Error()), nil
	}

	// Check if the domain matches our instance
	if wh.cfg == nil || domain != wh.cfg.Domain {
		wh.logger.Debug("domain mismatch",
			zap.String("requested_domain", domain),
			zap.String("our_domain", safeConfigDomain(wh.cfg)),
		)
		return webfingerJSONError(http.StatusNotFound, "actor not found"), nil
	}

	var state *storageModels.InstanceState
	stateErr := fmt.Errorf("instance state repository not available")
	if wh.instanceRepo != nil {
		state, stateErr = wh.instanceRepo.GetInstanceState(ctx.Context())
	}

	bootstrapUsername := storageModels.DefaultBootstrapUsername
	if stateErr == nil && state != nil && strings.TrimSpace(state.BootstrapUsername) != "" {
		bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
	}

	locked := stateErr != nil || state == nil || state.Locked
	if locked && !strings.EqualFold(username, bootstrapUsername) {
		return webfingerJSONError(http.StatusNotFound, "actor not found"), nil
	}

	// When locked, allow WebFinger discovery only for the bootstrap actor.
	// Ensure the bootstrap actor exists so federation endpoints can return empty collections.
	if locked && strings.EqualFold(username, bootstrapUsername) {
		if err := wh.ensureBootstrapActor(ctx.Context(), bootstrapUsername); err != nil {
			wh.logger.Error("failed to ensure bootstrap actor", zap.Error(err))
			return webfingerJSONError(http.StatusInternalServerError, "failed to initialize bootstrap actor"), nil
		}
	}

	// For non-bootstrap actors (or unlocked instances), require the actor record to exist.
	if !locked || !strings.EqualFold(username, bootstrapUsername) {
		_, err = wh.actorRepo.GetActor(ctx.Context(), username)
		if err != nil {
			if common.IsNotFound(err) {
				wh.logger.Debug("actor not found",
					zap.String("username", username),
				)
				return webfingerJSONError(http.StatusNotFound, "actor not found"), nil
			}
			wh.logger.Error("failed to get actor", zap.Error(err))
			return webfingerJSONError(http.StatusInternalServerError, "database error"), nil
		}
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
		zap.String("request_id", webfingerContextRequestID(ctx)),
	)

	// Return WebFinger response with proper content type and caching
	resp, err := webfingerJRDJSON(http.StatusOK, response)
	if err != nil {
		return nil, err
	}
	resp.Headers["cache-control"] = []string{CacheControlMaxAge}
	return resp, nil
}

func (wh *WebFingerHandler) ensureBootstrapActor(ctx context.Context, username string) error {
	_, err := wh.actorRepo.GetActor(ctx, username)
	if err == nil {
		return nil
	}
	if !common.IsNotFound(err) {
		return err
	}

	priv, err := rsaGenerateKeyFn(rand.Reader, 4096)
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
	now := timeNowFn().UTC()
	actor := activitypub.NewActor(activitypub.PersonType, actorID, username)
	actor.Name = username
	actor.URL = fmt.Sprintf("https://%s/@%s", cfg.Domain, username)
	actor.CreatedAt = &now
	actor.PublicKey = &activitypub.PublicKey{
		ID:           fmt.Sprintf("%s#main-key", actorID),
		Owner:        actorID,
		PublicKeyPem: string(publicKeyPEM),
	}
	surface.ApplyLocalActorIdentifiers(actor, fmt.Sprintf("https://%s", cfg.Domain), username)

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
		actorRepo:    repos.Actor(),
		repos:        repos,
		instanceRepo: repos.Instance(),
		logger:       logger,
		cfg:          cfg,
		lambdaCtx:    lambdaCtx,
	}
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage

	mustInitializeLambdaFn     = common.MustInitializeLambda
	initializeWithDefaultsFn   = (*common.LambdaContext).InitializeWithDefaults
	lambdaStartFn              = lambda.Start
	rsaGenerateKeyFn           = rsa.GenerateKey
	timeNowFn                  = time.Now
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
)

func init() {
	if common.RunningUnitTests() {
		return
	}

	initializeWebFinger()
}

func initializeWebFinger() {
	// WebFinger is a public endpoint and does not require JWT validation.
	// Ensure config loading does not attempt to resolve the JWT secret from Secrets
	// Manager (which may not be permitted for this Lambda role).
	if os.Getenv("JWT_SECRET") == "" {
		_ = os.Setenv("JWT_SECRET", "webfinger-noauth")
	}

	// Standardized Lambda initialization with automatic service detection
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "webfinger",
		LambdaType:  common.LambdaTypeAPI,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Initialize with default options for API Lambda type
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Warn("failed to initialize with defaults, some features may be limited", zap.Error(err))
	}

	storage, ok := lambdaCtx.Repos.(core.RepositoryStorage)
	if !ok || storage == nil {
		logger.Warn("lambda context repository missing after initialization, attempting manual storage initialization")
		initializeManualStorage()
		storage, ok = lambdaCtx.Repos.(core.RepositoryStorage)
	}
	if !ok || storage == nil {
		logger.Fatal("lambda context repository is not core.RepositoryStorage")
	}
	repos = storage
}

func initializeManualStorage() {
	if lambdaCtx == nil {
		logger.Fatal("manual storage initialization requires lambda context")
	}

	tableName := strings.TrimSpace(cfg.DynamoTableName)
	if tableName == "" {
		logger.Fatal("DYNAMODB_TABLE environment variable is required for webfinger lambda")
	}

	db, err := newLambdaOptimizedClientFn(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("failed to initialize DynamORM", zap.Error(err))
	}

	storage, err := newRepositoryFactoryFn(db, tableName, logger)
	if err != nil {
		logger.Fatal("failed to initialize repository factory", zap.Error(err))
	}

	lambdaCtx.DynamoDB = db
	lambdaCtx.Repos = storage
}

func main() {
	runWebFinger(NewWebFingerHandler(), lambdaCtx)
}

func runWebFinger(handler *WebFingerHandler, lambdaCtx *common.LambdaContext) {
	lambdaLogger := zap.NewNop()
	if lambdaCtx != nil && lambdaCtx.Logger != nil {
		lambdaLogger = lambdaCtx.Logger
	}

	app := buildApp(handler, lambdaLogger)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func buildApp(handler *WebFingerHandler, lambdaLogger *zap.Logger) *apptheory.App {
	app := apptheory.New(
		apptheory.WithCORS(apptheory.CORSConfig{
			AllowedOrigins:   []string{"*"},
			AllowCredentials: false,
			AllowHeaders: []string{
				"Accept",
				"Content-Type",
				"Date",
				"Digest",
				"Host",
				"Signature",
				"User-Agent",
				"X-Forwarded-For",
				"X-Forwarded-Proto",
			},
		}),
		apptheory.WithLimits(apptheory.Limits{
			MaxRequestBytes:  64 * 1024,
			MaxResponseBytes: 0,
		}),
	)

	// Panic recovery middleware (MUST be first to catch all panics).
	app.Use(webfingerPanicRecovery(lambdaLogger))

	// Request ID middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				ctx.Set("requestID", webfingerRequestID(ctx, "webfinger"))
			}
			return next(ctx)
		}
	})

	// Crawler classification middleware (observe-only; configurable via CRAWLER_PROTECTION_MODE).
	app.Use(crawler.NewMiddleware(lambdaLogger))

	// Security headers middleware (federation-friendly).
	app.Use(webfingerActivityPubSecurityHeaders())

	// Logging middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			resp, err := next(ctx)

			requestID := webfingerContextRequestID(ctx)
			hasError := err != nil
			if !hasError && resp != nil && resp.Status >= 400 {
				hasError = true
			}

			logger.Info("webfinger request completed",
				zap.String("request_id", requestID),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", hasError),
			)

			if hasError {
				logger.Error("webfinger handler error",
					zap.String("request_id", requestID),
					zap.Error(err),
				)
			}

			return resp, err
		}
	})

	// Register webfinger routes
	handler.RegisterRoutes(app)

	return app
}

func webfingerPanicRecovery(logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (resp *apptheory.Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered",
						zap.String("request_id", webfingerContextRequestID(ctx)),
						zap.Any("panic", r),
					)
					resp = webfingerJSONError(http.StatusInternalServerError, "internal server error")
					err = nil
				}
			}()
			return next(ctx)
		}
	}
}

func webfingerActivityPubSecurityHeaders() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			resp, err := next(ctx)
			if resp == nil {
				return resp, err
			}
			if resp.Headers == nil {
				resp.Headers = map[string][]string{}
			}
			resp.Headers["x-content-type-options"] = []string{"nosniff"}
			resp.Headers["x-frame-options"] = []string{"SAMEORIGIN"}
			resp.Headers["referrer-policy"] = []string{"strict-origin-when-cross-origin"}
			resp.Headers["cross-origin-resource-policy"] = []string{"cross-origin"}
			resp.Headers["x-robots-tag"] = []string{"noindex, nofollow"}
			return resp, err
		}
	}
}

func webfingerQueryValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	values := ctx.Request.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func webfingerRequestID(ctx *apptheory.Context, prefix string) string {
	if ctx != nil && strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "webfinger"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func webfingerContextRequestID(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}
	if rid, ok := ctx.Get("requestID").(string); ok && strings.TrimSpace(rid) != "" {
		return strings.TrimSpace(rid)
	}
	if strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	return ""
}

func webfingerJSONError(status int, message string) *apptheory.Response {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "internal server error"
	}
	return apptheory.MustJSON(status, map[string]string{"error": message})
}

func webfingerJRDJSON(status int, value any) (*apptheory.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &apptheory.Response{
		Status: status,
		Headers: map[string][]string{
			"content-type": {"application/jrd+json"},
		},
		Body: body,
	}, nil
}

func safeConfigDomain(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Domain
}
