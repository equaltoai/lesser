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
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/crawler"
	"github.com/equaltoai/lesser/pkg/federation"
	securityheaders "github.com/equaltoai/lesser/pkg/security/headers"
	"github.com/equaltoai/lesser/pkg/security/htmlsafe"
	"github.com/equaltoai/lesser/pkg/storage/core"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
)

var (
	mustInitializeLambdaFn   = common.MustInitializeLambda
	initializeWithDefaultsFn = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	lambdaStartFn            = lambda.Start
	newHandlerFn             = NewHandler
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeActor()
}

// Handler contains dependencies for the actor service
type Handler struct {
	actorRepo              actorGetter
	authorizedFetchService authorizedFetchVerifier
	instanceRepo           instanceStateGetter
}

type actorGetter interface {
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
}

type authorizedFetchVerifier interface {
	IsAuthorizedFetchEnabled(ctx context.Context) bool
	VerifyAuthorizedFetch(ctx context.Context, req *http.Request) (*activitypub.Actor, error)
}

type instanceStateGetter interface {
	GetInstanceState(ctx context.Context) (*storageModels.InstanceState, error)
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
		instanceRepo:           repos.Instance(),
	}
}

func initializeActor() {
	// Standardized Lambda initialization with automatic service detection
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "actor",
		LambdaType:  common.LambdaTypeAPI,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	storage, ok := lambdaCtx.Repos.(core.RepositoryStorage)
	if !ok || storage == nil {
		logger.Fatal("lambda context repository is not core.RepositoryStorage")
	}
	repos = storage

	// Initialize with default options for API Lambda type
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Warn("failed to initialize with defaults, some features may be limited", zap.Error(err))
	}
}

// HandleActorProfile handles ActivityPub actor profile requests
func (h *Handler) HandleActorProfile(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract username from path parameters
	username := ""
	if ctx != nil {
		username = ctx.Param("username")
	}
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return actorJSONError(http.StatusUnprocessableEntity, "missing username"), nil
	}

	var state *storageModels.InstanceState
	var stateErr error
	if h.instanceRepo != nil {
		state, stateErr = h.instanceRepo.GetInstanceState(ctx.Context())
	} else {
		stateErr = errors.New("missing instance repository")
	}
	bootstrapUsername := storageModels.DefaultBootstrapUsername
	if stateErr == nil && state != nil && strings.TrimSpace(state.BootstrapUsername) != "" {
		bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
	}
	locked := stateErr != nil || state == nil || state.Locked
	if locked && strings.EqualFold(username, bootstrapUsername) {
		return actorJSONError(http.StatusForbidden, "bootstrap actor is not available while instance is locked"), nil
	}

	// Get request ID from context
	requestID := actorContextRequestID(ctx)

	logger.Info("fetching actor profile",
		zap.String("username", username),
		zap.String("accept", actorHeaderValue(ctx, "accept")),
		zap.String("request_id", requestID),
	)

	// Get actor from repository
	actor, err := h.actorRepo.GetActorByUsername(ctx.Context(), username)
	if err != nil {
		if common.IsNotFound(err) {
			return actorJSONError(http.StatusNotFound, "actor not found"), nil
		}
		logger.Error("failed to get actor",
			zap.Error(err),
			zap.String("username", username),
			zap.String("request_id", requestID),
		)
		return actorJSONError(http.StatusInternalServerError, "failed to get actor"), nil
	}

	// Content negotiation
	accept := actorHeaderValue(ctx, "accept")

	// Check if client wants ActivityStreams JSON
	if strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json") ||
		strings.Contains(accept, "application/json") {
		// Check if authorized fetch is enabled for ActivityPub JSON requests
		if h.authorizedFetchService.IsAuthorizedFetchEnabled(ctx.Context()) {
			logger.Debug("authorized fetch enabled, verifying request",
				zap.String("username", username),
				zap.String("request_id", requestID),
			)

			// Convert request context for signature verification.
			httpReq, err := h.convertAppTheoryRequest(ctx)
			if err != nil {
				logger.Error("failed to convert request for authorized fetch",
					zap.String("username", username),
					zap.String("request_id", requestID),
					zap.Error(err),
				)
				return actorJSONError(http.StatusBadRequest, "malformed request"), nil
			}

			// Verify authorized fetch
			_, err = h.authorizedFetchService.VerifyAuthorizedFetch(ctx.Context(), httpReq)
			if err != nil {
				// Check if signature is missing vs invalid
				if strings.Contains(err.Error(), "missing signature") {
					logger.Debug("unauthorized request - missing signature",
						zap.String("username", username),
						zap.String("request_id", requestID),
					)
					return actorJSONError(http.StatusUnauthorized, "signature required for authorized fetch"), nil
				}
				logger.Debug("authorized fetch verification failed",
					zap.String("username", username),
					zap.String("request_id", requestID),
					zap.Error(err),
				)
				return actorJSONError(http.StatusForbidden, "signature verification failed"), nil
			}

			logger.Debug("authorized fetch verification successful",
				zap.String("username", username),
				zap.String("request_id", requestID),
			)
		}

		// Return ActivityStreams JSON
		return actorActivityJSON(http.StatusOK, actor)
	}

	// Return HTML for browsers
	html, renderErr := h.generateHTMLProfile(actor)
	if renderErr != nil {
		logger.Error("failed to render actor html profile",
			zap.String("username", username),
			zap.String("request_id", requestID),
			zap.Error(renderErr),
		)
		html = "<!DOCTYPE html><html><head><meta charset=\"UTF-8\"><title>Lesser</title></head><body>Failed to render profile</body></html>"
	}
	return &apptheory.Response{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"content-type": {"text/html; charset=utf-8"},
		},
		Body: []byte(html),
	}, nil
}

const actorProfileHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{.DisplayName}} (@{{.Username}}@{{.Domain}})</title>

	<meta property="og:type" content="profile">
	<meta property="og:title" content="{{.DisplayName}}">
	<meta property="og:description" content="{{.SummaryText}}">
	<meta property="og:url" content="{{.ActorID}}">
	{{- if .IconURL }}
	<meta property="og:image" content="{{.IconURL}}">
	{{- end }}

	<link rel="alternate" type="application/activity+json" href="{{.ActorID}}">
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
			border-radius: 50%;
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
	<div class="profile">
		{{- if .IconURL }}
		<img src="{{.IconURL}}" alt="{{.DisplayName}}" class="avatar">
		{{- end }}

		<h1>{{.DisplayName}}</h1>
		<div class="username">@{{.Username}}@{{.Domain}}</div>

		{{- if .BioHTML }}
		<div class="bio">{{.BioHTML}}</div>
		{{- end }}

		{{- if and .FollowersURL .FollowingURL }}
		<div class="stats">
			<p><a href="{{.FollowersURL}}">Followers</a> | <a href="{{.FollowingURL}}">Following</a></p>
		</div>
		{{- end }}

		<div class="meta">
			<p>This is an ActivityPub profile. You can follow @{{.Username}}@{{.Domain}} from any compatible server.</p>
			<p><a href="{{.ActorID}}" type="application/activity+json">View ActivityPub data</a></p>
		</div>
	</div>
</body>
</html>`

type actorProfileTemplateData struct {
	DisplayName  string
	Username     string
	Domain       string
	ActorID      string
	SummaryText  string
	BioHTML      template.HTML
	IconURL      string
	FollowersURL string
	FollowingURL string
}

func (h *Handler) generateHTMLProfile(actor *activitypub.Actor) (string, error) {
	displayName := strings.TrimSpace(actor.Name)
	if err := common.ValidateRequiredParam("displayName", displayName); err != nil {
		displayName = strings.TrimSpace(actor.PreferredUsername)
	}

	sanitizedSummary := strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(actor.BaseObject.Summary))

	data := actorProfileTemplateData{
		DisplayName:  displayName,
		Username:     strings.TrimSpace(actor.PreferredUsername),
		Domain:       strings.TrimSpace(cfg.Domain),
		ActorID:      strings.TrimSpace(actor.ID),
		SummaryText:  sanitizedSummary,
		FollowersURL: strings.TrimSpace(actor.Followers),
		FollowingURL: strings.TrimSpace(actor.Following),
	}

	if actor.Icon != nil {
		data.IconURL = strings.TrimSpace(actor.Icon.URL)
	}
	if sanitizedSummary != "" {
		data.BioHTML = template.HTML(sanitizedSummary)
	}

	return htmlsafe.RenderTemplate("actor_profile", actorProfileHTMLTemplate, data)
}

// convertAppTheoryRequest converts an AppTheory request to an http.Request for signature verification.
func (h *Handler) convertAppTheoryRequest(ctx *apptheory.Context) (*http.Request, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}

	// Build URL
	u := &url.URL{
		Scheme: "https",
		Host:   actorHeaderValue(ctx, "host"),
		Path:   ctx.Request.Path,
	}

	q := u.Query()
	for key, values := range ctx.Request.Query {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	u.RawQuery = q.Encode()

	// Create request with context (no body for GET requests)
	req, err := http.NewRequestWithContext(ctx.Context(), ctx.Request.Method, u.String(), nil)
	if err != nil {
		return nil, err
	}

	// Copy headers
	for k, v := range ctx.Request.Headers {
		if len(v) == 0 {
			continue
		}
		for _, value := range v {
			req.Header.Add(k, value)
		}
	}

	// Set host header if not present
	if err := common.ValidateRequiredParam("host", req.Host); err != nil && actorHeaderValue(ctx, "host") != "" {
		req.Host = actorHeaderValue(ctx, "host")
	}

	return req, nil
}

func main() {
	runActor()
}

func runActor() {
	// Initialize handler dependencies
	handler := newHandlerFn()

	app := buildApp(handler, logger)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func buildApp(handler *Handler, lambdaLogger *zap.Logger) *apptheory.App {
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
	app.Use(actorPanicRecovery(lambdaLogger))

	// Request ID middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				ctx.Set("requestID", actorRequestID(ctx, "actor"))
			}
			return next(ctx)
		}
	})

	// Crawler classification middleware (observe-only; configurable via CRAWLER_PROTECTION_MODE).
	app.Use(crawler.NewMiddleware(lambdaLogger))

	// Security headers middleware (federation-friendly).
	app.Use(actorActivityPubSecurityHeaders())

	// Logging middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			resp, err := next(ctx)

			requestID := actorContextRequestID(ctx)
			hasError := err != nil
			if !hasError && resp != nil && resp.Status >= 400 {
				hasError = true
			}

			logger.Info("actor request completed",
				zap.String("request_id", requestID),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", hasError),
			)

			if hasError {
				logger.Error("actor handler error",
					zap.String("request_id", requestID),
					zap.Error(err),
				)
			}

			return resp, err
		}
	})

	app.Get("/users/:username", handler.HandleActorProfile)

	return app
}

func actorPanicRecovery(logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (resp *apptheory.Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered",
						zap.String("request_id", actorContextRequestID(ctx)),
						zap.Any("panic", r),
					)
					resp = actorJSONError(http.StatusInternalServerError, "internal server error")
					err = nil
				}
			}()
			return next(ctx)
		}
	}
}

func actorActivityPubSecurityHeaders() apptheory.Middleware {
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
			contentTypes := strings.Join(resp.Headers["content-type"], ",")
			if strings.Contains(strings.ToLower(contentTypes), "text/html") {
				resp.Headers["content-security-policy"] = []string{securityheaders.StaticHTMLPageCSP()}
			}
			return resp, err
		}
	}
}

func actorHeaderValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	values := ctx.Request.Headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func actorRequestID(ctx *apptheory.Context, prefix string) string {
	if ctx != nil && strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "actor"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func actorContextRequestID(ctx *apptheory.Context) string {
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

func actorJSONError(status int, message string) *apptheory.Response {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "internal server error"
	}
	return apptheory.MustJSON(status, map[string]string{"error": message})
}

func actorActivityJSON(status int, value any) (*apptheory.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &apptheory.Response{
		Status: status,
		Headers: map[string][]string{
			"content-type": {"application/activity+json"},
		},
		Body: body,
	}, nil
}
