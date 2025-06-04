package main

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	store storage.Storage
)

func init() {
	// Initialize storage
	var err error
	store, err = dynamodb.New()
	if err != nil {
		common.Logger().Fatal("failed to initialize storage", zap.Error(err))
	}
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log := common.WithContext(ctx)

	// Extract object ID from path
	objectID := request.PathParameters["id"]
	if objectID == "" {
		return common.BadRequest(fmt.Errorf("object ID is required")), nil
	}

	// Get the object
	objInterface, err := store.GetObject(ctx, objectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.NotFound(fmt.Errorf("object %s not found", objectID)), nil
		}
		log.Error("failed to get object", zap.String("object_id", objectID), zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Type assert to our Object type
	obj, ok := objInterface.(*dynamodb.Object)
	if !ok {
		log.Error("invalid object type in storage", zap.String("object_id", objectID))
		return common.InternalServerError(fmt.Errorf("invalid object type")), nil
	}

	// Check if HTML is requested
	acceptHeader := request.Headers["Accept"]
	if acceptHeader == "" {
		acceptHeader = request.Headers["accept"]
	}

	if strings.Contains(acceptHeader, "text/html") {
		// Return HTML representation
		htmlContent := generateObjectHTML(obj)
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"Content-Type": "text/html; charset=utf-8",
			},
			Body: htmlContent,
		}, nil
	}

	// Return JSON representation
	return common.JSONResponse(http.StatusOK, obj), nil
}

func generateObjectHTML(obj *dynamodb.Object) string {
	// Generate a beautiful HTML page for the object
	var content string
	if obj.Content != "" {
		content = html.EscapeString(obj.Content)
	} else if obj.Name != "" && obj.Type == "Article" {
		content = fmt.Sprintf("<h1>%s</h1>", html.EscapeString(obj.Name))
		if obj.Summary != "" {
			content += fmt.Sprintf("<p class=\"summary\">%s</p>", html.EscapeString(obj.Summary))
		}
	}

	// Handle attachments
	var attachmentsHTML string
	if len(obj.Attachment) > 0 {
		attachmentsHTML = `<div class="attachments">`
		for _, att := range obj.Attachment {
			if att.Type == "Image" {
				attachmentsHTML += fmt.Sprintf(`<img src="%s" alt="%s" class="attachment-image">`,
					html.EscapeString(att.URL), html.EscapeString(att.Name))
			}
		}
		attachmentsHTML += `</div>`
	}

	// Handle tags
	var tagsHTML string
	if len(obj.Tag) > 0 {
		tagsHTML = `<div class="tags">`
		for _, tag := range obj.Tag {
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
		html.EscapeString(obj.Type),
		obj.Type,
		generateWarningHTML(obj),
		content,
		attachmentsHTML,
		tagsHTML,
		obj.Published.Format("January 2, 2006 at 3:04 PM"),
		html.EscapeString(obj.AttributedTo),
		html.EscapeString(extractUsernameFromURL(obj.AttributedTo)),
		generateUpdatedHTML(obj),
	)
}

func generateWarningHTML(obj *dynamodb.Object) string {
	if obj.Sensitive && obj.Summary != "" {
		return fmt.Sprintf(`<div class="warning">
            <strong>Content Warning:</strong> %s
        </div>`, html.EscapeString(obj.Summary))
	}
	return ""
}

func generateUpdatedHTML(obj *dynamodb.Object) string {
	if !obj.Updated.IsZero() {
		return fmt.Sprintf(`<p>Updated: %s</p>`, obj.Updated.Format("January 2, 2006 at 3:04 PM"))
	}
	return ""
}

func extractUsernameFromURL(url string) string {
	// Extract username from URL like https://example.com/users/alice
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return "@" + parts[len(parts)-1]
	}
	return url
}

func main() {
	lambda.Start(handler)
}
