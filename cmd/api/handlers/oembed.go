package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// OEmbedResponse represents the oEmbed response format
type OEmbedResponse struct {
	Type            string `json:"type"`                       // always "rich" for statuses
	Version         string `json:"version"`                    // always "1.0"
	Title           string `json:"title,omitempty"`            // optional title
	AuthorName      string `json:"author_name"`                // account display name
	AuthorURL       string `json:"author_url"`                 // account URL
	ProviderName    string `json:"provider_name"`              // instance name
	ProviderURL     string `json:"provider_url"`               // instance URL
	CacheAge        int    `json:"cache_age"`                  // cache duration in seconds
	HTML            string `json:"html"`                       // embeddable HTML
	Width           int    `json:"width"`                      // width of embed
	Height          *int   `json:"height,omitempty"`           // height if known
	ThumbnailURL    string `json:"thumbnail_url,omitempty"`    // thumbnail if available
	ThumbnailWidth  *int   `json:"thumbnail_width,omitempty"`  // thumbnail width
	ThumbnailHeight *int   `json:"thumbnail_height,omitempty"` // thumbnail height
}

// HandleOEmbed handles GET /api/oembed
func (h *Handler) HandleOEmbed(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get URL parameter
	requestedURL := request.QueryStringParameters["url"]
	if requestedURL == "" {
		return common.BadRequest(fmt.Errorf("missing required parameter: url")), nil
	}

	// Parse the URL to extract status ID
	parsedURL, err := url.Parse(requestedURL)
	if err != nil {
		return common.BadRequest(fmt.Errorf("invalid URL")), nil
	}

	// Check if it's from our instance
	expectedHost := strings.TrimPrefix(h.cfg.BaseURL(), "https://")
	expectedHost = strings.TrimPrefix(expectedHost, "http://")
	if parsedURL.Host != expectedHost {
		return common.NotFound(fmt.Errorf("URL does not belong to this instance")), nil
	}

	// Extract status ID from path
	// Expected formats:
	// - /web/@username/statusid
	// - /@username/statusid
	// - /users/username/statuses/statusid
	statusID := h.extractStatusID(parsedURL.Path)
	if statusID == "" {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Get format parameter (default to json)
	format := request.QueryStringParameters["format"]
	if format == "" {
		format = "json"
	}

	// Get maxwidth parameter
	maxWidth := 650 // default
	if mw := request.QueryStringParameters["maxwidth"]; mw != "" {
		if parsed, err := strconv.Atoi(mw); err == nil && parsed > 0 {
			maxWidth = parsed
		}
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Fetch the status
	obj, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		h.logger.Error("failed to get object", zap.String("object_id", objectID), zap.Error(err))
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Convert to Note
	note, ok := obj.(*activitypub.Note)
	if !ok {
		// Try to extract Note from map
		if objMap, mapOk := obj.(map[string]interface{}); mapOk {
			// Convert map to Note
			noteData, _ := json.Marshal(objMap)
			note = &activitypub.Note{}
			if err := json.Unmarshal(noteData, note); err != nil {
				h.logger.Error("failed to parse note", zap.Error(err))
				return common.InternalServerError(fmt.Errorf("failed to parse status")), nil
			}
		} else {
			return common.BadRequest(fmt.Errorf("object is not a status")), nil
		}
	}

	// Check if status is public or unlisted
	isPublic := false
	for _, to := range note.To {
		if to == activitypub.PublicAddress {
			isPublic = true
			break
		}
	}
	if !isPublic {
		for _, cc := range note.CC {
			if cc == activitypub.PublicAddress {
				isPublic = true
				break
			}
		}
	}

	if !isPublic {
		return common.Forbidden(fmt.Errorf("status is not embeddable")), nil
	}

	// Get the author's actor
	var authorActor *activitypub.Actor
	if note.AttributedTo != "" {
		// Extract username from actor ID
		parts := strings.Split(note.AttributedTo, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			authorActor, err = h.store.GetActor(ctx, username)
			if err != nil {
				h.logger.Warn("failed to get author actor", zap.String("actor_id", note.AttributedTo), zap.Error(err))
				// Create a minimal actor
				authorActor = &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: note.AttributedTo,
					},
					PreferredUsername: username,
					Name:              username,
					URL:               note.AttributedTo,
				}
			}
		}
	}

	// Generate oEmbed response
	oembed := h.generateOEmbed(note, authorActor, maxWidth)

	// Return based on format
	switch format {
	case "json":
		body, _ := json.Marshal(oembed)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(body),
		}, nil
	case "xml":
		return h.sendXMLResponse(oembed), nil
	default:
		return common.BadRequest(fmt.Errorf("unsupported format")), nil
	}
}

// extractStatusID extracts the status ID from various URL formats
func (h *Handler) extractStatusID(urlPath string) string {
	// Remove leading slash
	urlPath = strings.TrimPrefix(urlPath, "/")

	// Try different patterns
	patterns := []struct {
		prefix string
		parts  int
		index  int
	}{
		{"web/@", 2, 1},    // /web/@username/statusid
		{"@", 2, 1},        // /@username/statusid
		{"users/", 4, 3},   // /users/username/statuses/statusid
		{"objects/", 1, 0}, // /objects/statusid (direct object URL)
	}

	for _, pattern := range patterns {
		if strings.HasPrefix(urlPath, pattern.prefix) {
			if pattern.prefix == "objects/" {
				// Direct object ID
				return strings.TrimPrefix(urlPath, "objects/")
			}
			parts := strings.Split(urlPath, "/")
			if len(parts) > pattern.index {
				return parts[pattern.index]
			}
		}
	}

	return ""
}

// generateOEmbed creates the oEmbed response
func (h *Handler) generateOEmbed(note *activitypub.Note, author *activitypub.Actor, maxWidth int) *OEmbedResponse {
	// Generate HTML embed
	embedHTML := h.generateEmbedHTML(note, maxWidth)

	// Calculate height estimate (rough approximation)
	// Base height + content height + media height
	baseHeight := 150                      // header + footer
	contentHeight := len(note.Content) / 2 // very rough estimate
	mediaHeight := 0
	if len(note.Attachment) > 0 {
		mediaHeight = 300 // standard media preview height
	}
	estimatedHeight := baseHeight + contentHeight + mediaHeight

	oembed := &OEmbedResponse{
		Type:         "rich",
		Version:      "1.0",
		AuthorName:   author.Name,
		AuthorURL:    author.URL,
		ProviderName: "Lesser Instance", // Use a default since we don't have instance title
		ProviderURL:  h.cfg.BaseURL(),
		CacheAge:     86400, // 24 hours
		HTML:         embedHTML,
		Width:        maxWidth,
		Height:       &estimatedHeight,
	}

	// Add title if status has spoiler text
	if note.Summary != "" {
		oembed.Title = note.Summary
	}

	// Add thumbnail if available
	if len(note.Attachment) > 0 {
		// Use first image/video as thumbnail
		for _, attachment := range note.Attachment {
			if attachment.Type == "Document" || attachment.Type == "Image" {
				if attachment.MediaType != "" && strings.HasPrefix(attachment.MediaType, "image/") {
					oembed.ThumbnailURL = attachment.URL
					// We don't have width/height metadata in the simplified model
					break
				}
			}
		}
	}

	return oembed
}

// generateEmbedHTML creates the HTML for embedding
func (h *Handler) generateEmbedHTML(note *activitypub.Note, maxWidth int) string {
	// Extract clean status ID from note ID
	statusID := strings.TrimPrefix(note.ID, h.cfg.BaseURL()+"/objects/")

	// Build embed HTML
	var htmlBuilder strings.Builder

	// Iframe wrapper
	htmlBuilder.WriteString(fmt.Sprintf(`<iframe src="%s/embed/%s" class="mastodon-embed" style="max-width: 100%%; border: 0" width="%d" allowfullscreen="allowfullscreen"></iframe>`,
		h.cfg.BaseURL(),
		statusID,
		maxWidth,
	))

	// Add script for dynamic resizing
	htmlBuilder.WriteString(fmt.Sprintf(`<script src="%s/embed.js" async="async"></script>`, h.cfg.BaseURL()))

	return htmlBuilder.String()
}

// sendXMLResponse sends the oEmbed response in XML format
func (h *Handler) sendXMLResponse(oembed *OEmbedResponse) *events.APIGatewayV2HTTPResponse {
	// Simple XML generation
	var xml strings.Builder
	xml.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	xml.WriteString("\n<oembed>")
	xml.WriteString(fmt.Sprintf("\n  <type>%s</type>", oembed.Type))
	xml.WriteString(fmt.Sprintf("\n  <version>%s</version>", oembed.Version))
	xml.WriteString(fmt.Sprintf("\n  <author_name>%s</author_name>", htmlpkg.EscapeString(oembed.AuthorName)))
	xml.WriteString(fmt.Sprintf("\n  <author_url>%s</author_url>", htmlpkg.EscapeString(oembed.AuthorURL)))
	xml.WriteString(fmt.Sprintf("\n  <provider_name>%s</provider_name>", htmlpkg.EscapeString(oembed.ProviderName)))
	xml.WriteString(fmt.Sprintf("\n  <provider_url>%s</provider_url>", htmlpkg.EscapeString(oembed.ProviderURL)))
	xml.WriteString(fmt.Sprintf("\n  <cache_age>%d</cache_age>", oembed.CacheAge))
	xml.WriteString(fmt.Sprintf("\n  <html><![CDATA[%s]]></html>", oembed.HTML))
	xml.WriteString(fmt.Sprintf("\n  <width>%d</width>", oembed.Width))

	if oembed.Height != nil {
		xml.WriteString(fmt.Sprintf("\n  <height>%d</height>", *oembed.Height))
	}

	if oembed.Title != "" {
		xml.WriteString(fmt.Sprintf("\n  <title>%s</title>", htmlpkg.EscapeString(oembed.Title)))
	}

	if oembed.ThumbnailURL != "" {
		xml.WriteString(fmt.Sprintf("\n  <thumbnail_url>%s</thumbnail_url>", htmlpkg.EscapeString(oembed.ThumbnailURL)))
		if oembed.ThumbnailWidth != nil {
			xml.WriteString(fmt.Sprintf("\n  <thumbnail_width>%d</thumbnail_width>", *oembed.ThumbnailWidth))
		}
		if oembed.ThumbnailHeight != nil {
			xml.WriteString(fmt.Sprintf("\n  <thumbnail_height>%d</thumbnail_height>", *oembed.ThumbnailHeight))
		}
	}

	xml.WriteString("\n</oembed>")

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "text/xml; charset=utf-8",
		},
		Body: xml.String(),
	}
}

// HandleEmbedPage handles GET /embed/:id
// This would serve the actual embed page in an iframe
func (h *Handler) HandleEmbedPage(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Fetch the status
	obj, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Convert to Note
	note, ok := obj.(*activitypub.Note)
	if !ok {
		// Try to extract Note from map
		if objMap, mapOk := obj.(map[string]interface{}); mapOk {
			// Convert map to Note
			noteData, _ := json.Marshal(objMap)
			note = &activitypub.Note{}
			if err := json.Unmarshal(noteData, note); err != nil {
				return common.InternalServerError(fmt.Errorf("failed to parse status")), nil
			}
		} else {
			return common.BadRequest(fmt.Errorf("object is not a status")), nil
		}
	}

	// Check if status is public or unlisted
	isPublic := false
	for _, to := range note.To {
		if to == activitypub.PublicAddress {
			isPublic = true
			break
		}
	}
	if !isPublic {
		for _, cc := range note.CC {
			if cc == activitypub.PublicAddress {
				isPublic = true
				break
			}
		}
	}

	if !isPublic {
		return common.Forbidden(fmt.Errorf("status is not embeddable")), nil
	}

	// Get the author's actor
	var authorActor *activitypub.Actor
	authorName := "Unknown"
	authorUsername := "unknown"
	if note.AttributedTo != "" {
		// Extract username from actor ID
		parts := strings.Split(note.AttributedTo, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			authorUsername = username
			authorActor, err = h.store.GetActor(ctx, username)
			if err == nil {
				authorName = authorActor.Name
				if authorName == "" {
					authorName = authorActor.PreferredUsername
				}
			}
		}
	}

	// Format timestamp
	timestamp := "Unknown"
	if note.Published != nil {
		timestamp = note.Published.Format("Jan 2, 2006")
	}

	// Generate minimal HTML page for embed
	var htmlBuilder strings.Builder
	htmlBuilder.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>`)
	htmlBuilder.WriteString(htmlpkg.EscapeString(authorName))
	htmlBuilder.WriteString(" - Lesser Instance")
	htmlBuilder.WriteString(`</title>
    <style>
        body {
            margin: 0;
            padding: 16px;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #fff;
            color: #282c37;
        }
        .status {
            border: 1px solid #e1e8ed;
            border-radius: 8px;
            padding: 16px;
        }
        .author {
            display: flex;
            align-items: center;
            margin-bottom: 12px;
        }
        .avatar {
            width: 48px;
            height: 48px;
            border-radius: 50%;
            margin-right: 12px;
            background: #ccc;
        }
        .author-info {
            flex: 1;
        }
        .display-name {
            font-weight: bold;
            color: #282c37;
            text-decoration: none;
        }
        .username {
            color: #606984;
            font-size: 14px;
        }
        .content {
            margin-bottom: 12px;
            line-height: 1.5;
        }
        .timestamp {
            color: #606984;
            font-size: 14px;
            text-decoration: none;
        }
        .media {
            margin-top: 12px;
            max-width: 100%;
            border-radius: 8px;
        }
        a {
            color: #2b90d9;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <article class="status">
        <div class="author">
            <div class="avatar"></div>
            <div class="author-info">
                <a href="`)
	htmlBuilder.WriteString(h.cfg.BaseURL())
	htmlBuilder.WriteString("/@")
	htmlBuilder.WriteString(authorUsername)
	htmlBuilder.WriteString(`" target="_blank" class="display-name">`)
	htmlBuilder.WriteString(htmlpkg.EscapeString(authorName))
	htmlBuilder.WriteString(`</a>
                <div class="username">@`)
	htmlBuilder.WriteString(authorUsername)
	htmlBuilder.WriteString(`</div>
            </div>
        </div>
        <div class="content">`)
	htmlBuilder.WriteString(note.Content) // Already HTML
	htmlBuilder.WriteString(`</div>`)

	// Add media if present
	if len(note.Attachment) > 0 {
		for _, attachment := range note.Attachment {
			if attachment.Type == "Document" || attachment.Type == "Image" {
				if attachment.MediaType != "" && strings.HasPrefix(attachment.MediaType, "image/") {
					htmlBuilder.WriteString(`<img src="`)
					htmlBuilder.WriteString(attachment.URL)
					htmlBuilder.WriteString(`" alt="`)
					htmlBuilder.WriteString(htmlpkg.EscapeString(attachment.Name))
					htmlBuilder.WriteString(`" class="media">`)
				}
			}
		}
	}

	// Add timestamp
	htmlBuilder.WriteString(`
        <div>
            <a href="`)
	htmlBuilder.WriteString(note.ID)
	htmlBuilder.WriteString(`" target="_blank" class="timestamp">`)
	htmlBuilder.WriteString(timestamp)
	htmlBuilder.WriteString(`</a>
        </div>
    </article>
    <script>
        // Send height to parent
        function sendHeight() {
            const height = document.body.scrollHeight;
            window.parent.postMessage({type: 'embed-height', height: height}, '*');
        }
        
        // Send height on load and resize
        window.addEventListener('load', sendHeight);
        window.addEventListener('resize', sendHeight);
        
        // Also send periodically in case content changes
        setInterval(sendHeight, 1000);
    </script>
</body>
</html>`)

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":    "text/html; charset=utf-8",
			"X-Frame-Options": "ALLOWALL", // Allow embedding
		},
		Body: htmlBuilder.String(),
	}, nil
}
