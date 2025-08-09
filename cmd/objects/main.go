// Package main implements the objects Lambda function for serving ActivityPub object endpoints.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Handler handles ActivityPub federation object requests
type Handler struct {
	db         core.DB
	objectRepo *repositories.ObjectRepository
	logger     *zap.Logger
	cfg        *config.Config
}

// NewHandler creates a new objects handler
func NewHandler() (*Handler, error) {
	logger := common.Logger()
	cfg := config.Get()

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Initialize object repository
	objectRepo := repositories.NewObjectRepository(db, cfg.DynamoTableName, cfg.Domain, logger)

	return &Handler{
		db:         db,
		objectRepo: objectRepo,
		logger:     logger,
		cfg:        cfg,
	}, nil
}

// HandleGetObject handles GET requests for ActivityPub objects
func (h *Handler) HandleGetObject(ctx *lift.Context) error {
	// Extract object ID from path parameters
	objectID := ctx.Param("id")
	if objectID == "" {
		return lift.ValidationError("object ID is required")
	}

	h.logger.Info("fetching object",
		zap.String("object_id", objectID),
		zap.String("request_id", ctx.GetRequestID()),
	)

	// Get the object from storage
	objInterface, err := h.objectRepo.GetObject(ctx.Request.Context(), objectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Debug("object not found",
				zap.String("object_id", objectID),
				zap.String("request_id", ctx.GetRequestID()),
			)
			return lift.NotFound(fmt.Sprintf("object %s not found", objectID))
		}
		h.logger.Error("failed to get object",
			zap.String("object_id", objectID),
			zap.String("request_id", ctx.GetRequestID()),
			zap.Error(err),
		)
		return lift.NewLiftError("OBJECT_FETCH_ERROR", "failed to fetch object", 500).WithCause(err)
	}

	// Check Accept header for content negotiation
	acceptHeader := ctx.Header("Accept")
	if acceptHeader == "" {
		acceptHeader = ctx.Header("accept")
	}

	// Return HTML for browsers
	if strings.Contains(acceptHeader, "text/html") {
		h.logger.Debug("returning HTML representation",
			zap.String("object_id", objectID),
			zap.String("request_id", ctx.GetRequestID()),
		)
		htmlContent := h.generateObjectHTML(objInterface)
		ctx.Response.Header("Content-Type", "text/html; charset=utf-8")
		return ctx.HTML(htmlContent)
	}

	// Return ActivityPub JSON (default)
	h.logger.Debug("returning ActivityPub JSON representation",
		zap.String("object_id", objectID),
		zap.String("request_id", ctx.GetRequestID()),
	)
	ctx.Response.Header("Content-Type", "application/activity+json")
	return ctx.Status(http.StatusOK).JSON(objInterface)
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

// generateHTML creates the actual HTML content
func (h *Handler) generateHTML(objectType, content, name, summary, attributedTo, _ string, published, updated time.Time, sensitive bool, attachments []activitypub.Attachment, tags []activitypub.Tag) string {
	// Generate content based on object type and available fields
	var htmlContent string
	if content != "" {
		htmlContent = html.EscapeString(content)
	} else if name != "" && objectType == "Article" {
		htmlContent = fmt.Sprintf("<h1>%s</h1>", html.EscapeString(name))
		if summary != "" {
			htmlContent += fmt.Sprintf("<p class=\"summary\">%s</p>", html.EscapeString(summary))
		}
	}

	// Handle attachments
	var attachmentsHTML string
	if len(attachments) > 0 {
		attachmentsHTML = `<div class="attachments">`
		for _, att := range attachments {
			if att.Type == "Image" {
				attachmentsHTML += fmt.Sprintf(`<img src="%s" alt="%s" class="attachment-image">`,
					html.EscapeString(att.URL), html.EscapeString(att.Name))
			}
		}
		attachmentsHTML += `</div>`
	}

	// Handle tags
	var tagsHTML string
	if len(tags) > 0 {
		tagsHTML = `<div class="tags">`
		for _, tag := range tags {
			if tag.Type == "Hashtag" {
				tagsHTML += fmt.Sprintf(`<a href="%s" class="hashtag">%s</a> `,
					html.EscapeString(tag.Href), html.EscapeString(tag.Name))
			}
		}
		tagsHTML += `</div>`
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - Lesser</title>
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
            max-width: 100%%;
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
            <div class="object-type">%s</div>
        </div>
        
        %s
        
        <div class="object-content">
            %s
        </div>
        
        %s
        
        %s
        
        <div class="object-meta">
            <p>Published: %s</p>
            <p>By: <a href="%s">%s</a></p>
            %s
        </div>
    </div>
</body>
</html>`,
		html.EscapeString(objectType),
		objectType,
		h.generateWarningHTML(sensitive, summary),
		htmlContent,
		attachmentsHTML,
		tagsHTML,
		published.Format("January 2, 2006 at 3:04 PM"),
		html.EscapeString(attributedTo),
		html.EscapeString(h.extractUsernameFromURL(attributedTo)),
		h.generateUpdatedHTML(updated),
	)
}

// generateWarningHTML creates content warning HTML if needed
func (h *Handler) generateWarningHTML(sensitive bool, summary string) string {
	if sensitive && summary != "" {
		return fmt.Sprintf(`<div class="warning">
            <strong>Content Warning:</strong> %s
        </div>`, html.EscapeString(summary))
	}
	return ""
}

// generateUpdatedHTML creates updated date HTML if object was updated
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

func main() {
	// Initialize handler
	handler, err := NewHandler()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize handler: %v", err))
	}

	// Create Lift application
	app := lift.New()

	// Add request ID middleware (first - generates request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("objects-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware (second - logs with request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			err := next.Handle(ctx)

			handler.logger.Info("objects request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			if err != nil {
				handler.logger.Error("objects handler error",
					zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
					zap.Error(err),
				)
			}
			return err
		})
	})

	// Add recovery middleware (third - catches panics)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					handler.logger.Error("panic recovered in objects handler",
						zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
						zap.Any("panic", r),
					)
				}
			}()
			return next.Handle(ctx)
		})
	})

	// ActivityPub federation endpoint
	_ = app.GET("/objects/:id", handler.HandleGetObject)

	// Start Lambda handler
	lambda.Start(app.HandleRequest)
}
