// Package main implements the objects Lambda function for serving ActivityPub object endpoints.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	"github.com/equaltoai/lesser/pkg/storage/factory"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
)

var (
	mustInitializeLambdaFn     = common.MustInitializeLambda
	initializeWithDefaultsFn   = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	lambdaStartFn              = lambda.Start
	newHandlerFn               = NewHandler
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeObjects()
}

func initializeObjects() {
	// Standardized Lambda initialization with automatic service detection
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "objects",
		LambdaType:  common.LambdaTypeAPI,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Initialize with default options for API Lambda type (best-effort; some lambdas still do manual wiring).
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
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
		logger.Fatal("DYNAMODB_TABLE environment variable is required for objects lambda")
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

// Handler handles ActivityPub federation object requests
type Handler struct {
	objectRepo             objectGetter
	authorizedFetchService authorizedFetchVerifier
	instanceRepo           instanceStateGetter
}

type objectGetter interface {
	GetObject(ctx context.Context, id string) (any, error)
}

type authorizedFetchVerifier interface {
	IsAuthorizedFetchEnabled(ctx context.Context) bool
	VerifyAuthorizedFetch(ctx context.Context, req *http.Request) (*activitypub.Actor, error)
}

type instanceStateGetter interface {
	GetInstanceState(ctx context.Context) (*storageModels.InstanceState, error)
}

// NewHandler creates a new objects handler using standardized services
func NewHandler() *Handler {
	// Initialize object repository
	objectRepo := repositories.NewObjectRepository(
		repos.GetDB(),
		repos.GetTableName(),
		cfg.Domain,
		logger)

	// Initialize authorized fetch service
	authorizedFetchService := federation.NewAuthorizedFetchService(
		repos,
		cfg.Domain,
		logger)

	return &Handler{
		objectRepo:             objectRepo,
		authorizedFetchService: authorizedFetchService,
		instanceRepo:           repos.Instance(),
	}
}

// HandleGetObject handles GET requests for ActivityPub objects
func (h *Handler) HandleGetObject(ctx *apptheory.Context) (*apptheory.Response, error) {
	objectID, lookupID := h.resolveObjectLookup(ctx)
	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return objectsJSONError(http.StatusUnprocessableEntity, "missing object id"), nil
	}

	requestID := objectsContextRequestID(ctx)

	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	// When the instance is locked, treat all objects as absent.
	var state *storageModels.InstanceState
	var stateErr error
	if h.instanceRepo != nil {
		state, stateErr = h.instanceRepo.GetInstanceState(runCtx)
	} else {
		stateErr = errors.New("missing instance repository")
	}

	locked := stateErr != nil || state == nil || state.Locked
	if locked {
		if stateErr != nil {
			logger.Warn("failed to get instance lock state; defaulting to locked",
				zap.Error(stateErr),
				zap.String("object_id", objectID),
				zap.String("request_id", requestID),
			)
		}
		return objectsJSONError(http.StatusNotFound, fmt.Sprintf("object %s not found", objectID)), nil
	}

	// Check Accept header for content negotiation
	acceptHeader := strings.ToLower(objectsHeaderValue(ctx, "accept"))
	wantsHTML := strings.Contains(acceptHeader, "text/html")

	authorizedFetchEnabled := h.authorizedFetchService != nil && h.authorizedFetchService.IsAuthorizedFetchEnabled(runCtx)

	// Enforce authorized fetch whenever this handler would return ActivityPub JSON.
	// Do not rely on client-controlled Accept header substring checks as a security gate.
	if authorizedFetchEnabled && !wantsHTML {
		logger.Debug("authorized fetch enabled, verifying request",
			zap.String("object_id", objectID),
			zap.String("request_id", requestID),
		)

		// Convert AppTheory request to http.Request for signature verification.
		httpReq, err := h.convertAppTheoryRequest(ctx)
		if err != nil {
			logger.Error("failed to convert request for authorized fetch",
				zap.String("object_id", objectID),
				zap.String("request_id", requestID),
				zap.Error(err),
			)
			return objectsJSONError(http.StatusBadRequest, "malformed request"), nil
		}

		// Verify authorized fetch
		_, err = h.authorizedFetchService.VerifyAuthorizedFetch(runCtx, httpReq)
		if err != nil {
			// Check if signature is missing vs invalid
			if strings.Contains(err.Error(), "missing signature") {
				logger.Debug("unauthorized request - missing signature",
					zap.String("object_id", objectID),
					zap.String("request_id", requestID),
				)
				return objectsJSONError(http.StatusUnauthorized, "signature required for authorized fetch"), nil
			}
			logger.Debug("authorized fetch verification failed",
				zap.String("object_id", objectID),
				zap.String("request_id", requestID),
				zap.Error(err),
			)
			return objectsJSONError(http.StatusForbidden, "signature verification failed"), nil
		}

		logger.Debug("authorized fetch verification successful",
			zap.String("object_id", objectID),
			zap.String("request_id", requestID),
		)
	}

	logger.Info("fetching object",
		zap.String("object_id", objectID),
		zap.String("request_id", requestID),
	)

	// Get the object from storage
	objInterface, err := h.objectRepo.GetObject(runCtx, lookupID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			logger.Debug("object not found",
				zap.String("object_id", objectID),
				zap.String("request_id", requestID),
			)
			return objectsJSONError(http.StatusNotFound, fmt.Sprintf("object %s not found", objectID)), nil
		}
		logger.Error("failed to get object",
			zap.String("object_id", objectID),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return objectsJSONError(http.StatusInternalServerError, "failed to fetch object"), nil
	}

	// Return HTML for browsers
	if wantsHTML {
		if authorizedFetchEnabled && !objectsIsPubliclyAddressed(objInterface) {
			// When Authorized Fetch is enabled, do not expose HTML renderings for non-public objects.
			// This avoids leaking followers-only (or otherwise restricted) content to unauthenticated browsers.
			logger.Debug("suppressing HTML response for non-public object while authorized fetch is enabled",
				zap.String("object_id", objectID),
				zap.String("request_id", requestID),
			)
			return objectsJSONError(http.StatusNotFound, fmt.Sprintf("object %s not found", objectID)), nil
		}

		logger.Debug("returning HTML representation",
			zap.String("object_id", objectID),
			zap.String("request_id", requestID),
		)
		htmlContent := h.generateObjectHTML(objInterface)
		return &apptheory.Response{
			Status: http.StatusOK,
			Headers: map[string][]string{
				"content-type": {"text/html; charset=utf-8"},
			},
			Body: []byte(htmlContent),
		}, nil
	}

	// Return ActivityPub JSON (default)
	logger.Debug("returning ActivityPub JSON representation",
		zap.String("object_id", objectID),
		zap.String("request_id", requestID),
	)
	return objectsActivityJSON(http.StatusOK, objInterface)
}

func (h *Handler) resolveObjectLookup(ctx *apptheory.Context) (string, string) {
	if ctx == nil {
		return "", ""
	}

	objectID := strings.TrimSpace(ctx.Param("id"))
	if objectID == "" {
		return "", ""
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if username != "" {
		canonicalID := fmt.Sprintf("%s/users/%s/statuses/%s", cfg.BaseURL(), username, objectID)
		return canonicalID, canonicalID
	}

	lookupID := objectID
	if !strings.HasPrefix(lookupID, "http://") && !strings.HasPrefix(lookupID, "https://") {
		lookupID = fmt.Sprintf("%s/objects/%s", cfg.BaseURL(), objectID)
	}

	return objectID, lookupID
}

// generateObjectHTML creates HTML representation of an ActivityPub object
// objectData holds extracted object data for HTML generation
type objectData struct {
	objectType   string
	content      string
	name         string
	summary      string
	attributedTo string
	id           string
	published    time.Time
	updated      time.Time
	sensitive    bool
	attachments  []activitypub.Attachment
	tags         []activitypub.Tag
}

func (h *Handler) generateObjectHTML(objInterface any) string {
	data := h.extractObjectData(objInterface)
	return h.generateHTML(data.objectType, data.content, data.name, data.summary,
		data.attributedTo, data.id, data.published, data.updated,
		data.sensitive, data.attachments, data.tags)
}

// extractObjectData extracts data from various object types
func (h *Handler) extractObjectData(objInterface any) *objectData {
	// Try to convert to Note first
	if note, ok := objInterface.(*activitypub.Note); ok {
		return h.extractNoteData(note)
	}

	// Handle generic object as map
	if objMap, ok := objInterface.(map[string]any); ok {
		return h.extractMapData(objMap)
	}

	// Fallback for unknown object types
	return &objectData{
		objectType: "Object",
		content:    "Unknown object type",
		id:         "unknown",
	}
}

// extractNoteData extracts data from an ActivityPub Note
func (h *Handler) extractNoteData(note *activitypub.Note) *objectData {
	data := &objectData{
		objectType:   note.Type,
		content:      note.Content,
		id:           note.ID,
		attributedTo: note.AttributedTo,
		sensitive:    note.Sensitive,
		attachments:  note.Attachment,
		tags:         note.Tag,
		summary:      note.Summary,
	}

	if note.Published != nil {
		data.published = *note.Published
	}
	if note.Updated != nil {
		data.updated = *note.Updated
	}

	return data
}

// extractMapData extracts data from a generic map object
func (h *Handler) extractMapData(objMap map[string]any) *objectData {
	data := &objectData{}

	// Extract basic fields
	h.extractBasicFields(objMap, data)

	// Extract date fields
	h.extractDateFields(objMap, data)

	// Extract complex fields
	h.extractComplexFields(objMap, data)

	return data
}

// extractBasicFields extracts simple string and boolean fields
func (h *Handler) extractBasicFields(objMap map[string]any, data *objectData) {
	if v, ok := objMap["type"].(string); ok {
		data.objectType = v
	}
	if v, ok := objMap["content"].(string); ok {
		data.content = v
	}
	if v, ok := objMap["id"].(string); ok {
		data.id = v
	}
	if v, ok := objMap["attributedTo"].(string); ok {
		data.attributedTo = v
	}
	if v, ok := objMap["name"].(string); ok {
		data.name = v
	}
	if v, ok := objMap["summary"].(string); ok {
		data.summary = v
	}
	if v, ok := objMap["sensitive"].(bool); ok {
		data.sensitive = v
	}
}

// extractDateFields extracts and parses date fields
func (h *Handler) extractDateFields(objMap map[string]any, data *objectData) {
	data.published = h.parseDateTime(objMap["published"])
	data.updated = h.parseDateTime(objMap["updated"])
}

// parseDateTime parses a date/time value from various formats
func (h *Handler) parseDateTime(value any) time.Time {
	if t, ok := value.(time.Time); ok {
		return t
	}
	if s, ok := value.(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// extractComplexFields extracts attachments and tags
func (h *Handler) extractComplexFields(objMap map[string]any, data *objectData) {
	if v, ok := objMap["attachment"]; ok {
		if attBytes, err := json.Marshal(v); err == nil {
			_ = json.Unmarshal(attBytes, &data.attachments)
		}
	}
	if v, ok := objMap["tag"]; ok {
		if tagBytes, err := json.Marshal(v); err == nil {
			_ = json.Unmarshal(tagBytes, &data.tags)
		}
	}
}

const objectHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Lesser</title>
    <style>
        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background-color: #f5f5f5;
            padding: 20px;
        }
        
        .container {
            max-width: 600px;
            margin: 0 auto;
            background-color: white;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
            padding: 30px;
        }
        
        .object-header {
            margin-bottom: 20px;
            padding-bottom: 20px;
            border-bottom: 1px solid #e0e0e0;
        }
        
        .object-type {
            font-size: 14px;
            color: #666;
            margin-bottom: 5px;
        }
        
        .object-content {
            font-size: 16px;
            margin-bottom: 20px;
        }
        
        .object-content h1 {
            font-size: 24px;
            margin-bottom: 10px;
        }
        
        .summary {
            color: #666;
            font-style: italic;
            margin-bottom: 20px;
        }
        
        .attachments {
            margin-bottom: 20px;
        }
        
        .attachment-image {
            max-width: 100%;
            height: auto;
            border-radius: 4px;
            margin-bottom: 10px;
        }
        
        .tags {
            margin-bottom: 20px;
        }
        
        .hashtag {
            display: inline-block;
            background-color: #e3f2fd;
            color: #1976d2;
            padding: 4px 8px;
            border-radius: 4px;
            text-decoration: none;
            font-size: 14px;
            margin-right: 5px;
        }
        
        .hashtag:hover {
            background-color: #bbdefb;
        }
        
        .object-meta {
            font-size: 14px;
            color: #666;
        }
        
        .object-meta a {
            color: #1976d2;
            text-decoration: none;
        }
        
        .object-meta a:hover {
            text-decoration: underline;
        }
        
        .warning {
            background-color: #fff3cd;
            border: 1px solid #ffeaa7;
            color: #856404;
            padding: 10px;
            border-radius: 4px;
            margin-bottom: 20px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="object-header">
            <div class="object-type">{{.ObjectType}}</div>
        </div>
        
        {{if and .Sensitive .Summary}}
        <div class="warning">
            <strong>Content Warning:</strong> {{.Summary}}
        </div>
        {{end}}
        
        <div class="object-content">
            {{if .Content}}
            {{.Content}}
            {{else if and .IsArticle .Name}}
            <h1>{{.Name}}</h1>
            {{if .Summary}}
            <p class="summary">{{.Summary}}</p>
            {{end}}
            {{end}}
        </div>
        
        {{if .Attachments}}
        <div class="attachments">
            {{range .Attachments}}
            {{if eq .Type "Image"}}
            <img src="{{.URL}}" alt="{{.Name}}" class="attachment-image">
            {{end}}
            {{end}}
        </div>
        {{end}}
        
        {{if .Tags}}
        <div class="tags">
            {{range .Tags}}
            {{if eq .Type "Hashtag"}}
            <a href="{{.Href}}" class="hashtag">{{.Name}}</a>
            {{end}}
            {{end}}
        </div>
        {{end}}
        
        <div class="object-meta">
            <p>Published: {{.Published}}</p>
            <p>By: <a href="{{.AttributedTo}}">{{.AttributedUsername}}</a></p>
            {{if .Updated}}
            <p>Updated: {{.Updated}}</p>
            {{end}}
        </div>
    </div>
</body>
</html>`

type objectHTMLTemplateData struct {
	Title              string
	ObjectType         string
	Content            string
	IsArticle          bool
	Name               string
	Sensitive          bool
	Summary            string
	Attachments        []activitypub.Attachment
	Tags               []activitypub.Tag
	Published          string
	Updated            string
	AttributedTo       string
	AttributedUsername string
}

// generateHTML creates the actual HTML content
func (h *Handler) generateHTML(objectType, content, name, summary, attributedTo, _ string, published, updated time.Time, sensitive bool, attachments []activitypub.Attachment, tags []activitypub.Tag) string {
	data := objectHTMLTemplateData{
		Title:              objectType,
		ObjectType:         objectType,
		Content:            content,
		IsArticle:          objectType == "Article",
		Name:               name,
		Sensitive:          sensitive,
		Summary:            summary,
		Attachments:        attachments,
		Tags:               tags,
		Published:          published.Format("January 2, 2006 at 3:04 PM"),
		AttributedTo:       attributedTo,
		AttributedUsername: h.extractUsernameFromURL(attributedTo),
	}
	if !updated.IsZero() {
		data.Updated = updated.Format("January 2, 2006 at 3:04 PM")
	}

	out, err := htmlsafe.RenderTemplate("object", objectHTMLTemplate, data)
	if err != nil {
		logger.Warn("failed to render object HTML template", zap.Error(err))
		return "<!DOCTYPE html><html><head><meta charset=\"UTF-8\"><title>Lesser</title></head><body>Failed to render object</body></html>"
	}
	return out
}

// generateWarningHTML creates content warning HTML if needed.
// Deprecated: prefer template-first rendering via objectHTMLTemplate.
//
//nolint:unused // Used by tests to validate legacy behavior during the template migration.
func (h *Handler) generateWarningHTML(sensitive bool, summary string) string {
	if sensitive && strings.TrimSpace(summary) != "" {
		return fmt.Sprintf(`<div class="warning">
            <strong>Content Warning:</strong> %s
        </div>`, htmlsafe.Escape(summary))
	}
	return ""
}

// generateUpdatedHTML creates updated date HTML if object was updated.
// Deprecated: prefer template-first rendering via objectHTMLTemplate.
//
//nolint:unused // Used by tests to validate legacy behavior during the template migration.
func (h *Handler) generateUpdatedHTML(updated time.Time) string {
	if !updated.IsZero() {
		return fmt.Sprintf(`<p>Updated: %s</p>`, updated.Format("January 2, 2006 at 3:04 PM"))
	}
	return ""
}

// extractUsernameFromURL extracts username from ActivityPub actor URL
func (h *Handler) extractUsernameFromURL(url string) string {
	// Extract username from URL like https://example.com/users/alice
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return "@" + parts[len(parts)-1]
	}
	return url
}

// convertAppTheoryRequest converts an AppTheory request to an http.Request for signature verification.
func (h *Handler) convertAppTheoryRequest(ctx *apptheory.Context) (*http.Request, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}

	u := common.RequestURLFromHeaders(ctx.Request.Headers, ctx.Request.Path, ctx.Request.Query)

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

	// Signature verification depends on the reconstructed public host.
	if u.Host != "" {
		req.Host = u.Host
		req.Header.Set("Host", u.Host)
	}

	return req, nil
}

func main() {
	runObjects()
}

func runObjects() {
	// Initialize handler using standardized services.
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
				common.XLesserForwardedHost,
				common.XLesserForwardedProto,
			},
		}),
		apptheory.WithLimits(apptheory.Limits{
			MaxRequestBytes:  1024 * 1024,
			MaxResponseBytes: 0,
		}),
	)

	// Panic recovery middleware (MUST be first to catch all panics).
	app.Use(objectsPanicRecovery(lambdaLogger))

	// Request ID middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				ctx.Set("requestID", objectsRequestID(ctx, "objects"))
			}
			return next(ctx)
		}
	})

	// Crawler classification middleware (observe-only; configurable via CRAWLER_PROTECTION_MODE).
	app.Use(crawler.NewMiddleware(lambdaLogger))

	// Security headers middleware (federation-friendly).
	app.Use(objectsActivityPubSecurityHeaders())

	// Logging middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			resp, err := next(ctx)

			requestID := objectsContextRequestID(ctx)
			hasError := err != nil
			if !hasError && resp != nil && resp.Status >= 400 {
				hasError = true
			}

			method := ""
			path := ""
			if ctx != nil {
				method = ctx.Request.Method
				path = ctx.Request.Path
			}

			logger.Info("objects request completed",
				zap.String("request_id", requestID),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", hasError),
			)

			if hasError {
				logger.Error("objects handler error",
					zap.String("request_id", requestID),
					zap.Error(err),
				)
			}

			return resp, err
		}
	})

	// ActivityPub federation endpoints.
	app.Get("/objects/:id", handler.HandleGetObject)
	app.Get("/users/:username/statuses/:id", handler.HandleGetObject)

	return app
}

func objectsPanicRecovery(logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (resp *apptheory.Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered",
						zap.String("request_id", objectsContextRequestID(ctx)),
						zap.Any("panic", r),
					)
					resp = objectsJSONError(http.StatusInternalServerError, "internal server error")
					err = nil
				}
			}()
			return next(ctx)
		}
	}
}

func objectsActivityPubSecurityHeaders() apptheory.Middleware {
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

func objectsHeaderValue(ctx *apptheory.Context, key string) string {
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

func objectsIsPubliclyAddressed(obj any) bool {
	if obj == nil {
		return false
	}

	if note, ok := obj.(*activitypub.Note); ok {
		return objectsRecipientsContainPublic(note.To) || objectsRecipientsContainPublic(note.CC)
	}

	if activity, ok := obj.(*activitypub.Activity); ok {
		return objectsRecipientsContainPublic(activity.To) || objectsRecipientsContainPublic(activity.CC)
	}

	if objMap, ok := obj.(map[string]any); ok {
		to := objectsStringSliceFromAny(objMap["to"])
		cc := objectsStringSliceFromAny(objMap["cc"])
		return objectsRecipientsContainPublic(to) || objectsRecipientsContainPublic(cc)
	}

	body, err := json.Marshal(obj)
	if err != nil {
		return false
	}

	var addressing struct {
		To []string `json:"to"`
		CC []string `json:"cc"`
	}
	if err := json.Unmarshal(body, &addressing); err != nil {
		return false
	}

	return objectsRecipientsContainPublic(addressing.To) || objectsRecipientsContainPublic(addressing.CC)
}

func objectsRecipientsContainPublic(recipients []string) bool {
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient) == activitypub.PublicAddress {
			return true
		}
	}
	return false
}

func objectsStringSliceFromAny(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		values := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				values = append(values, s)
			}
		}
		return values
	case string:
		value := strings.TrimSpace(v)
		if value == "" {
			return nil
		}
		return []string{value}
	default:
		return nil
	}
}

func objectsRequestID(ctx *apptheory.Context, prefix string) string {
	if ctx != nil && strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "objects"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func objectsContextRequestID(ctx *apptheory.Context) string {
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

func objectsJSONError(status int, message string) *apptheory.Response {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "internal server error"
	}
	return apptheory.MustJSON(status, map[string]string{"error": message})
}

func objectsActivityJSON(status int, value any) (*apptheory.Response, error) {
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
