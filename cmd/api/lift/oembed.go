package lift

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/url"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/pay-theory/lift/pkg/lift"
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

// HandleOEmbedLift handles GET /api/oembed using Lift framework
func (h *Handler) HandleOEmbedLift(ctx *lift.Context) error {
	h.logger.Info("oembed request",
		zap.String("user_agent", ctx.Header("User-Agent")))

	// Get URL parameter
	requestedURL := ctx.Query("url")
	
	// Fallback to direct query param access if ctx.Query doesn't work
	if requestedURL == "" && ctx.Request != nil && ctx.Request.Request != nil {
		requestedURL = ctx.Request.Request.QueryParams["url"]
	}
	
	if requestedURL == "" {
		h.logger.Warn("missing required parameter: url")
		return ctx.Status(400).JSON(map[string]string{
			"error": "missing required parameter: url",
		})
	}

	// Parse the URL to extract status ID
	parsedURL, err := url.Parse(requestedURL)
	if err != nil {
		h.logger.Warn("invalid URL", zap.String("url", requestedURL), zap.Error(err))
		return ctx.Status(400).JSON(map[string]string{
			"error": "invalid URL",
		})
	}

	// Check if it's from our instance
	expectedHost := strings.TrimPrefix(h.cfg.BaseURL(), "https://")
	expectedHost = strings.TrimPrefix(expectedHost, "http://")
	if parsedURL.Host != expectedHost {
		h.logger.Warn("URL does not belong to this instance",
			zap.String("requested_host", parsedURL.Host),
			zap.String("expected_host", expectedHost))
		return ctx.Status(404).JSON(map[string]string{
			"error": "URL does not belong to this instance",
		})
	}

	// Extract status ID from path
	statusID := h.extractStatusID(parsedURL.Path)
	if statusID == "" {
		h.logger.Warn("status not found in URL path", zap.String("path", parsedURL.Path))
		return ctx.Status(404).JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Get format parameter (default to json)
	format := ctx.Query("format")
	if format == "" && ctx.Request != nil && ctx.Request.Request != nil {
		format = ctx.Request.Request.QueryParams["format"]
	}
	if format == "" {
		format = "json"
	}

	// Get maxwidth parameter
	maxWidth := 650 // default
	mw := ctx.Query("maxwidth")
	if mw == "" && ctx.Request != nil && ctx.Request.Request != nil {
		mw = ctx.Request.Request.QueryParams["maxwidth"]
	}
	if mw != "" {
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
	obj, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		h.logger.Error("failed to get object", zap.String("object_id", objectID), zap.Error(err))
		return ctx.Status(404).JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Convert to Note
	note, ok := obj.(*activitypub.Note)
	if !ok {
		// Try to extract Note from map
		if objMap, mapOk := obj.(map[string]any); mapOk {
			// Convert map to Note
			noteData, _ := json.Marshal(objMap)
			note = &activitypub.Note{}
			if err := json.Unmarshal(noteData, note); err != nil {
				h.logger.Error("failed to parse note", zap.Error(err))
				return ctx.Status(500).JSON(map[string]string{
					"error": "failed to parse status",
				})
			}
		} else {
			h.logger.Warn("object is not a status", zap.String("object_id", objectID))
			return ctx.Status(400).JSON(map[string]string{
				"error": "object is not a status",
			})
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
		h.logger.Warn("status is not embeddable", zap.String("object_id", objectID))
		return ctx.Status(403).JSON(map[string]string{
			"error": "status is not embeddable",
		})
	}

	// Get the author's actor
	var authorActor *activitypub.Actor
	if note.AttributedTo != "" {
		// Extract username from actor ID
		parts := strings.Split(note.AttributedTo, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			authorActor, err = h.repos.Actor().GetActor(ctx.Context, username)
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
		return ctx.JSON(oembed)
	case "xml":
		return h.sendXMLResponseLift(ctx, oembed)
	default:
		h.logger.Warn("unsupported format", zap.String("format", format))
		return ctx.Status(400).JSON(map[string]string{
			"error": "unsupported format",
		})
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
		{"web/@", 3, 2},    // /web/@username/statusid - parts: ["web", "@username", "statusid"]
		{"@", 2, 1},        // /@username/statusid - parts: ["@username", "statusid"] 
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
		} else if pattern.prefix == "web/@" && strings.HasPrefix(urlPath, "web/") {
			// Handle web/@username/statusid pattern more carefully
			parts := strings.Split(urlPath, "/")
			if len(parts) >= 3 && strings.HasPrefix(parts[1], "@") {
				return parts[2]
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

// sendXMLResponseLift sends the oEmbed response in XML format using Lift context
func (h *Handler) sendXMLResponseLift(ctx *lift.Context, oembed *OEmbedResponse) error {
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

	// Set status and send XML response
	ctx.Status(200)
	ctx.Response.Header("Content-Type", "text/xml; charset=utf-8")
	ctx.Response.Body = xml.String()
	return nil
}

// HandleEmbedPageLift handles GET /embed/:id using Lift framework
func (h *Handler) HandleEmbedPageLift(ctx *lift.Context) error {
	// Get status ID from path parameter or extract from path
	statusID := ctx.Param("id")
	
	// Fallback: extract from path if param not available (for testing)
	if statusID == "" {
		path := ""
		if ctx.Request != nil && ctx.Request.Request != nil {
			path = ctx.Request.Request.Path
		}
		if strings.HasPrefix(path, "/embed/") {
			statusID = strings.TrimPrefix(path, "/embed/")
		}
	}
	
	if statusID == "" {
		h.logger.Warn("missing status ID in embed page request")
		return ctx.Status(400).JSON(map[string]string{
			"error": "missing status ID",
		})
	}

	h.logger.Info("embed page request",
		zap.String("status_id", statusID),
		zap.String("user_agent", ctx.Header("User-Agent")))

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Fetch the status
	obj, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		h.logger.Error("failed to get object for embed", zap.String("object_id", objectID), zap.Error(err))
		return ctx.Status(404).JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Convert to Note
	note, ok := obj.(*activitypub.Note)
	if !ok {
		// Try to extract Note from map
		if objMap, mapOk := obj.(map[string]any); mapOk {
			// Convert map to Note
			noteData, _ := json.Marshal(objMap)
			note = &activitypub.Note{}
			if err := json.Unmarshal(noteData, note); err != nil {
				h.logger.Error("failed to parse note for embed", zap.Error(err))
				return ctx.Status(500).JSON(map[string]string{
					"error": "failed to parse status",
				})
			}
		} else {
			h.logger.Warn("object is not a status for embed", zap.String("object_id", objectID))
			return ctx.Status(400).JSON(map[string]string{
				"error": "object is not a status",
			})
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
		h.logger.Warn("status is not embeddable", zap.String("object_id", objectID))
		return ctx.Status(403).JSON(map[string]string{
			"error": "status is not embeddable",
		})
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
			authorActor, err = h.repos.Actor().GetActor(ctx.Context, username)
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

	// Set HTML content type and additional headers
	ctx.Response.Header("Content-Type", "text/html; charset=utf-8")
	ctx.Response.Header("X-Frame-Options", "ALLOWALL") // Allow embedding
	ctx.Status(200)
	return ctx.HTML(htmlBuilder.String())
}