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
	"github.com/equaltoai/lesser/pkg/lambdastorage"
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

	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName:          "objects",
		RequireRepositories:  true,
		NewDB:                newLambdaOptimizedClientFn,
		NewRepositoryStorage: newRepositoryFactoryFn,
	})
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}
	repos = deps.Repos
}

// Handler handles ActivityPub federation object requests
type Handler struct {
	objectRepo             objectGetter
	authorizedFetchService authorizedFetchVerifier
	instanceRepo           instanceStateGetter
	relationshipRepo       relationshipChecker
}

type objectGetter interface {
	GetObject(ctx context.Context, id string) (any, error)
	IsTombstoned(ctx context.Context, id string) (bool, error)
	GetTombstone(ctx context.Context, objectID string) (*storageModels.Tombstone, error)
}

type authorizedFetchVerifier interface {
	IsAuthorizedFetchEnabled(ctx context.Context) bool
	VerifyAuthorizedFetch(ctx context.Context, req *http.Request) (*activitypub.Actor, error)
}

type instanceStateGetter interface {
	GetInstanceState(ctx context.Context) (*storageModels.InstanceState, error)
}

type relationshipChecker interface {
	IsFollowing(ctx context.Context, followerUsername, targetActorID string) (bool, error)
}

type objectsRouteInventoryEntry struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

func objectsRouteInventory() []objectsRouteInventoryEntry {
	return []objectsRouteInventoryEntry{
		{Method: http.MethodGet, Path: "/objects/:id"},
		{Method: http.MethodGet, Path: "/users/:username/statuses/:id"},
	}
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
		relationshipRepo:       repos.Relationship(),
	}
}

// RegisterRoutes registers all ActivityPub object routes.
func (h *Handler) RegisterRoutes(app *apptheory.App) error {
	return registerObjectsRoutes(app, h.HandleGetObject)
}

func registerObjectsRoutes(app *apptheory.App, handleGetObject apptheory.Handler) error {
	for _, route := range objectsRouteInventory() {
		switch route.Method {
		case http.MethodGet:
			if _, err := app.GetStrict(route.Path, handleGetObject); err != nil {
				return fmt.Errorf("register objects route %s %s: %w", route.Method, route.Path, err)
			}
		default:
			return fmt.Errorf("unsupported objects route method %q for %s", route.Method, route.Path)
		}
	}
	return nil
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
	var verifiedFetchActor *activitypub.Actor

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
		verifiedFetchActor, err = h.authorizedFetchService.VerifyAuthorizedFetch(runCtx, httpReq)
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
			if tombstoneResp, handled, tombErr := h.handleTombstonedObject(runCtx, lookupID, objectID, requestID, wantsHTML); handled {
				return tombstoneResp, tombErr
			}
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
		if !objectsIsPubliclyAddressed(objInterface) {
			// Never expose browser HTML renderings for non-public objects.
			// This avoids leaking followers-only, direct, or otherwise restricted content.
			logger.Debug("suppressing HTML response for non-public object",
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

	if !objectsIsPubliclyAddressed(objInterface) {
		if verifiedFetchActor == nil {
			var resp *apptheory.Response
			var err error
			verifiedFetchActor, resp, err = h.verifyObjectAuthorizedFetch(runCtx, ctx, objectID, requestID, true)
			if err != nil {
				return nil, err
			}
			if resp != nil {
				return resp, nil
			}
		}

		allowed, err := h.authorizedActorCanFetchObject(runCtx, objInterface, verifiedFetchActor)
		if err != nil {
			logger.Warn("failed to evaluate non-public object fetch authorization",
				zap.String("object_id", objectID),
				zap.String("request_id", requestID),
				zap.Error(err),
			)
		}
		if err != nil || !allowed {
			logger.Debug("suppressing ActivityPub JSON response for unauthorized non-public object fetch",
				zap.String("object_id", objectID),
				zap.String("request_id", requestID),
			)
			return objectsJSONError(http.StatusNotFound, fmt.Sprintf("object %s not found", objectID)), nil
		}
	}

	// Return ActivityPub JSON (default)
	logger.Debug("returning ActivityPub JSON representation",
		zap.String("object_id", objectID),
		zap.String("request_id", requestID),
	)
	return objectsActivityJSON(http.StatusOK, objInterface)
}

func (h *Handler) verifyObjectAuthorizedFetch(
	ctx context.Context,
	reqCtx *apptheory.Context,
	objectID string,
	requestID string,
	hideUnauthorized bool,
) (*activitypub.Actor, *apptheory.Response, error) {
	if h.authorizedFetchService == nil {
		return nil, objectsJSONError(http.StatusNotFound, fmt.Sprintf("object %s not found", objectID)), nil
	}

	httpReq, err := h.convertAppTheoryRequest(reqCtx)
	if err != nil {
		logger.Error("failed to convert request for authorized object fetch",
			zap.String("object_id", objectID),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		if hideUnauthorized {
			return nil, objectsJSONError(http.StatusNotFound, fmt.Sprintf("object %s not found", objectID)), nil
		}
		return nil, objectsJSONError(http.StatusBadRequest, "malformed request"), nil
	}

	actor, err := h.authorizedFetchService.VerifyAuthorizedFetch(ctx, httpReq)
	if err == nil {
		return actor, nil, nil
	}

	if hideUnauthorized {
		logger.Debug("authorized object fetch verification failed for non-public object",
			zap.String("object_id", objectID),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return nil, objectsJSONError(http.StatusNotFound, fmt.Sprintf("object %s not found", objectID)), nil
	}

	if strings.Contains(err.Error(), "missing signature") {
		return nil, objectsJSONError(http.StatusUnauthorized, "signature required for authorized fetch"), nil
	}

	return nil, objectsJSONError(http.StatusForbidden, "signature verification failed"), nil
}

func (h *Handler) handleTombstonedObject(ctx context.Context, lookupID, objectID, requestID string, wantsHTML bool) (*apptheory.Response, bool, error) {
	if h.objectRepo == nil {
		return nil, false, nil
	}

	tombstoned, err := h.objectRepo.IsTombstoned(ctx, lookupID)
	if err != nil || !tombstoned {
		return nil, false, nil
	}

	tombstone, err := h.objectRepo.GetTombstone(ctx, lookupID)
	if err != nil {
		logger.Warn("failed to load tombstone details for deleted object",
			zap.String("object_id", objectID),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return objectsJSONError(http.StatusGone, fmt.Sprintf("object %s deleted", objectID)), true, nil
	}

	logger.Debug("returning tombstone for deleted object",
		zap.String("object_id", objectID),
		zap.String("request_id", requestID),
	)

	if wantsHTML {
		return objectsJSONError(http.StatusGone, fmt.Sprintf("object %s deleted", objectID)), true, nil
	}

	resp, err := objectsActivityJSON(http.StatusGone, &activitypub.Tombstone{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      tombstone.ID,
			Type:    "Tombstone",
		},
		FormerType: tombstone.FormerType,
		Deleted:    tombstone.Deleted.Format(time.RFC3339),
	})
	return resp, true, err
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
	if err := handler.RegisterRoutes(app); err != nil {
		lambdaLogger.Fatal("failed to register objects routes", zap.Error(err))
	}

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

func (h *Handler) authorizedActorCanFetchObject(ctx context.Context, obj any, actor *activitypub.Actor) (bool, error) {
	if objectsIsPubliclyAddressed(obj) {
		return true, nil
	}
	if actor == nil {
		return false, nil
	}

	actorID := objectsActorID(actor)
	if actorID == "" {
		return false, nil
	}

	objectActorID := objectsAttributedActorID(obj)
	if objectActorID != "" && objectsActorIdentifiersMatch(actorID, objectActorID) {
		return true, nil
	}

	recipients := objectsAllRecipients(obj)
	if objectsRecipientsContainActor(recipients, actor) {
		return true, nil
	}

	if objectActorID == "" || !objectsRecipientsContainFollowersCollection(recipients, objectActorID) {
		return false, nil
	}
	if h.relationshipRepo == nil {
		return false, nil
	}

	return h.relationshipRepo.IsFollowing(ctx, actorID, objectActorID)
}

func objectsActorID(actor *activitypub.Actor) string {
	if actor == nil {
		return ""
	}
	if actorID := strings.TrimSpace(actor.ID); actorID != "" {
		return actorID
	}
	if actorURL := strings.TrimSpace(actor.URL); actorURL != "" {
		return actorURL
	}
	return strings.TrimSpace(actor.PreferredUsername)
}

func objectsAttributedActorID(obj any) string {
	if obj == nil {
		return ""
	}

	if note, ok := obj.(*activitypub.Note); ok {
		return strings.TrimSpace(note.AttributedTo)
	}
	if activity, ok := obj.(*activitypub.Activity); ok {
		return strings.TrimSpace(activity.Actor)
	}
	if objMap, ok := obj.(map[string]any); ok {
		if attributedTo, ok := objMap["attributedTo"].(string); ok && strings.TrimSpace(attributedTo) != "" {
			return strings.TrimSpace(attributedTo)
		}
		if actor, ok := objMap["actor"].(string); ok && strings.TrimSpace(actor) != "" {
			return strings.TrimSpace(actor)
		}
	}

	body, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	var actorFields struct {
		AttributedTo string `json:"attributedTo"`
		Actor        string `json:"actor"`
	}
	if err := json.Unmarshal(body, &actorFields); err != nil {
		return ""
	}
	if strings.TrimSpace(actorFields.AttributedTo) != "" {
		return strings.TrimSpace(actorFields.AttributedTo)
	}
	return strings.TrimSpace(actorFields.Actor)
}

func objectsAllRecipients(obj any) []string {
	if obj == nil {
		return nil
	}

	if note, ok := obj.(*activitypub.Note); ok {
		return objectsAppendRecipients(nil, note.To, note.CC, note.BTo, note.BCC)
	}
	if activity, ok := obj.(*activitypub.Activity); ok {
		return objectsAppendRecipients(nil, activity.To, activity.CC, activity.BTo, activity.BCC)
	}
	if objMap, ok := obj.(map[string]any); ok {
		return objectsAppendRecipients(nil,
			objectsStringSliceFromAny(objMap["to"]),
			objectsStringSliceFromAny(objMap["cc"]),
			objectsStringSliceFromAny(objMap["bto"]),
			objectsStringSliceFromAny(objMap["bcc"]),
		)
	}

	body, err := json.Marshal(obj)
	if err != nil {
		return nil
	}
	var addressing struct {
		To  []string `json:"to"`
		CC  []string `json:"cc"`
		BTo []string `json:"bto"`
		BCC []string `json:"bcc"`
	}
	if err := json.Unmarshal(body, &addressing); err != nil {
		return nil
	}
	return objectsAppendRecipients(nil, addressing.To, addressing.CC, addressing.BTo, addressing.BCC)
}

func objectsAppendRecipients(base []string, recipientSets ...[]string) []string {
	for _, recipients := range recipientSets {
		for _, recipient := range recipients {
			recipient = strings.TrimSpace(recipient)
			if recipient == "" {
				continue
			}
			base = append(base, recipient)
		}
	}
	return base
}

func objectsRecipientsContainPublic(recipients []string) bool {
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient) == activitypub.PublicAddress {
			return true
		}
	}
	return false
}

func objectsRecipientsContainActor(recipients []string, actor *activitypub.Actor) bool {
	actorID := objectsActorID(actor)
	if actorID == "" {
		return false
	}
	for _, recipient := range recipients {
		if objectsActorIdentifiersMatch(recipient, actorID) {
			return true
		}
	}
	return false
}

func objectsRecipientsContainFollowersCollection(recipients []string, authorActorID string) bool {
	followersCollection := strings.TrimRight(strings.TrimSpace(authorActorID), "/") + "/followers"
	if followersCollection == "/followers" {
		return false
	}
	for _, recipient := range recipients {
		if strings.TrimRight(strings.TrimSpace(recipient), "/") == followersCollection {
			return true
		}
	}
	return false
}

func objectsActorIdentifiersMatch(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	if strings.TrimRight(left, "/") == strings.TrimRight(right, "/") {
		return true
	}

	leftNormalized := storageModels.NormalizeRelationshipIdentity(left, objectsLocalDomain())
	rightNormalized := storageModels.NormalizeRelationshipIdentity(right, objectsLocalDomain())
	return leftNormalized != "" && leftNormalized == rightNormalized
}

func objectsLocalDomain() string {
	if cfg != nil {
		if domain := strings.TrimSpace(cfg.Domain); domain != "" {
			return domain
		}
	}
	return strings.TrimSpace(config.Get().Domain)
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
