package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	htmlpkg "html"
	"net/url"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	securityheaders "github.com/equaltoai/lesser/pkg/security/headers"
	"github.com/equaltoai/lesser/pkg/security/htmlsafe"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleOEmbedLift handles GET /api/oembed using Lift framework
func (h *Handler) HandleOEmbedLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	h.logger.Info("oembed request",
		zap.String("user_agent", headerValue(ctx, "User-Agent")))

	// Extract and validate URL parameter
	requestedURL, resp, err := h.extractOEmbedURL(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Parse and validate the URL
	parsedURL, resp, err := h.validateOEmbedURL(ctx, requestedURL)
	if resp != nil || err != nil {
		return resp, err
	}

	// Extract status ID from path
	statusID := h.extractStatusID(parsedURL.Path)
	if err := common.ValidateRequiredParam("statusID", statusID); err != nil {
		h.logger.Warn("status not found in URL path", zap.String("path", parsedURL.Path))
		return common.RespondStatusNotFound(ctx)
	}

	// Get request parameters
	format := h.getOEmbedFormat(ctx)
	maxWidth := h.getOEmbedMaxWidth(ctx)

	// Fetch and process the status
	objectID := h.normalizeStatusID(statusID)
	note, resp, err := h.fetchAndConvertNote(ctx, objectID)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check if status is embeddable
	if !h.isStatusEmbeddable(note) {
		h.logger.Warn("status is not embeddable", zap.String("object_id", objectID))
		return common.RespondForbidden(ctx, "status is not embeddable")
	}

	// Get the author's actor
	authorActor := h.getOEmbedAuthorActor(ctx, note)

	// Generate oEmbed response
	oembed := h.generateOEmbed(note, authorActor, maxWidth)

	// Return based on format
	return h.sendOEmbedResponse(ctx, oembed, format)
}

// extractOEmbedURL extracts the URL parameter from the request
func (h *Handler) extractOEmbedURL(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	requestedURL := queryValue(ctx, "url")

	if err := common.ValidateRequiredParam("requestedURL", requestedURL); err != nil {
		h.logger.Warn("missing required parameter: url")
		resp, respErr := common.RespondBadRequest(ctx, "missing required parameter: url")
		return "", resp, respErr
	}

	return requestedURL, nil, nil
}

// validateOEmbedURL parses and validates the requested URL
func (h *Handler) validateOEmbedURL(ctx *apptheory.Context, requestedURL string) (*url.URL, *apptheory.Response, error) {
	parsedURL, err := url.Parse(requestedURL)
	if err != nil {
		h.logger.Warn("invalid URL", zap.String("url", requestedURL), zap.Error(err))
		resp, respErr := common.RespondBadRequest(ctx, "invalid URL")
		return nil, resp, respErr
	}

	// Check if it's from our instance
	expectedHost := h.getExpectedHost()
	if parsedURL.Host != expectedHost {
		h.logger.Warn("URL does not belong to this instance",
			zap.String("requested_host", parsedURL.Host),
			zap.String("expected_host", expectedHost))
		resp, respErr := common.RespondNotFound(ctx, "URL does not belong to this instance")
		return nil, resp, respErr
	}

	return parsedURL, nil, nil
}

// getExpectedHost extracts the hostname from the base URL
func (h *Handler) getExpectedHost() string {
	host := strings.TrimPrefix(h.cfg.BaseURL(), "https://")
	return strings.TrimPrefix(host, "http://")
}

// getOEmbedFormat extracts the format parameter (defaults to "json")
func (h *Handler) getOEmbedFormat(ctx *apptheory.Context) string {
	format := queryValue(ctx, "format")
	if err := common.ValidateRequiredParam("format", format); err != nil {
		format = "json"
	}
	return format
}

// getOEmbedMaxWidth extracts the maxwidth parameter (defaults to 650)
func (h *Handler) getOEmbedMaxWidth(ctx *apptheory.Context) int {
	mw := queryValue(ctx, "maxwidth")

	// Use validation function with reasonable bounds for embed width
	if maxWidth, err := common.ParseAndValidateIntWithBounds("maxwidth", mw, 0, 2000, 650); err == nil {
		return maxWidth
	}

	// Return default on any validation error
	return 650
}

// normalizeStatusID converts a status ID to a full URL if needed
func (h *Handler) normalizeStatusID(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// fetchAndConvertNote fetches an object and converts it to a Note
func (h *Handler) fetchAndConvertNote(ctx *apptheory.Context, objectID string) (*activitypub.Note, *apptheory.Response, error) {
	// Extract status ID from object ID
	statusID := strings.TrimPrefix(objectID, h.cfg.BaseURL()+"/objects/")

	// Fetch the status using Notes service
	result, err := h.registry.Notes().GetNote(ctx.Context(), statusID)
	if err != nil {
		h.logger.Error("failed to get note", zap.String("status_id", statusID), zap.Error(err))
		resp, respErr := common.RespondStatusNotFound(ctx)
		return nil, resp, respErr
	}

	// Return the Note directly.
	if result.Note == nil {
		resp, respErr := common.RespondStatusNotFound(ctx)
		return nil, resp, respErr
	}
	return result.Note, nil, nil
}

// convertToNote converts an object to an ActivityPub Note

// getOEmbedAuthorActor retrieves the author actor for the note
func (h *Handler) getOEmbedAuthorActor(ctx *apptheory.Context, note *activitypub.Note) *activitypub.Actor {
	if err := common.ValidateRequiredParam("attributedTo", note.AttributedTo); err != nil {
		return nil
	}

	// Extract username from actor ID
	parts := strings.Split(note.AttributedTo, "/")
	if err := common.ValidateSliceNotEmpty("parts", parts); err != nil {
		return nil
	}

	username := parts[len(parts)-1]
	result, err := h.registry.Accounts().GetAccount(ctx.Context(), username)
	if err != nil {
		h.logger.Warn("failed to get author account", zap.String("username", username), zap.Error(err))
		// Create a minimal actor
		return &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID: note.AttributedTo,
			},
			PreferredUsername: username,
			Name:              username,
			URL:               note.AttributedTo,
		}
	}
	authorActor := result.Actor

	return authorActor
}

// sendOEmbedResponse sends the oEmbed response in the requested format
func (h *Handler) sendOEmbedResponse(ctx *apptheory.Context, oembed *apimodels.OEmbedResponse, format string) (*apptheory.Response, error) {
	switch format {
	case "json":
		return okJSON(oembed)
	case "xml":
		return h.sendXMLResponseLift(ctx, oembed)
	default:
		h.logger.Warn("unsupported format", zap.String("format", format))
		return common.RespondBadRequest(ctx, "unsupported format")
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
func (h *Handler) generateOEmbed(note *activitypub.Note, author *activitypub.Actor, maxWidth int) *apimodels.OEmbedResponse {
	// Generate HTML embed
	embedHTML := h.generateOEmbedHTML(note, maxWidth)

	// Calculate height estimate (rough approximation)
	// Base height + content height + media height
	baseHeight := 150                      // header + footer
	contentHeight := len(note.Content) / 2 // very rough estimate
	mediaHeight := 0
	if err := common.ValidateSliceNotEmpty("attachments", note.Attachment); err == nil {
		mediaHeight = 300 // standard media preview height
	}
	estimatedHeight := baseHeight + contentHeight + mediaHeight

	oembed := &apimodels.OEmbedResponse{
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
	if err := common.ValidateSliceNotEmpty("attachments", note.Attachment); err == nil {
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

// generateOEmbedHTML creates the HTML for oEmbed
func (h *Handler) generateOEmbedHTML(note *activitypub.Note, maxWidth int) string {
	// Extract clean status ID from note ID
	statusID := strings.TrimPrefix(note.ID, h.cfg.BaseURL()+"/objects/")
	statusID = url.PathEscape(statusID)

	// Build embed HTML
	var htmlBuilder strings.Builder

	// Iframe wrapper
	embedSrc := htmlpkg.EscapeString(fmt.Sprintf("%s/embed/%s", h.cfg.BaseURL(), statusID))
	htmlBuilder.WriteString(fmt.Sprintf(`<iframe src="%s" class="mastodon-embed" style="max-width: 100%%; border: 0" width="%d" allowfullscreen="allowfullscreen"></iframe>`,
		embedSrc,
		maxWidth,
	))

	// Add script for dynamic resizing
	scriptSrc := htmlpkg.EscapeString(fmt.Sprintf("%s/embed.js", h.cfg.BaseURL()))
	htmlBuilder.WriteString(fmt.Sprintf(`<script src="%s" async="async"></script>`, scriptSrc))

	return htmlBuilder.String()
}

// sendXMLResponseLift sends the oEmbed response in XML format using Lift context
func (h *Handler) sendXMLResponseLift(_ *apptheory.Context, oembed *apimodels.OEmbedResponse) (*apptheory.Response, error) {
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

	return &apptheory.Response{
		Status: 200,
		Headers: map[string][]string{
			"content-type": {"text/xml; charset=utf-8"},
		},
		Body: []byte(xml.String()),
	}, nil
}

// HandleEmbedPageLift handles GET /embed/:id using Lift framework
func (h *Handler) HandleEmbedPageLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract and validate status ID
	statusID, resp, err := h.extractEmbedStatusID(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	h.logger.Info("embed page request",
		zap.String("status_id", statusID),
		zap.String("user_agent", headerValue(ctx, "User-Agent")))

	// Normalize and fetch the status
	objectID := h.normalizeEmbedObjectID(statusID)
	note, resp, err := h.fetchEmbedNote(ctx, objectID)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check if status is embeddable (public/unlisted)
	if !h.isStatusEmbeddable(note) {
		h.logger.Warn("status is not embeddable", zap.String("object_id", objectID))
		return common.RespondForbidden(ctx, "status is not embeddable")
	}

	// Get author information
	authorInfo := h.getEmbedAuthorInfo(ctx, note)

	// Format timestamp
	timestamp := h.formatEmbedTimestamp(note)

	// Generate embed HTML
	scriptNonce := newCSPNonce()
	htmlContent := h.generateEmbedHTML(note, authorInfo, timestamp, scriptNonce)

	// Set HTML content type and additional headers
	resp = &apptheory.Response{
		Status: 200,
		Headers: map[string][]string{
			"content-type":                 {"text/html; charset=utf-8"},
			"content-security-policy":      {securityheaders.EmbedHTMLPageCSP(scriptNonce)},
			"x-content-type-options":       {"nosniff"},
			"x-frame-options":              {"ALLOWALL"},
			"referrer-policy":              {"strict-origin-when-cross-origin"},
			"cross-origin-resource-policy": {"cross-origin"},
			"x-robots-tag":                 {"noindex, nofollow"},
		},
		Body: []byte(htmlContent),
	}
	return resp, nil
}

// embedAuthorInfo holds author information for embeds
type embedAuthorInfo struct {
	name     string
	username string
	actor    *activitypub.Actor
}

// extractEmbedStatusID extracts the status ID from the request
func (h *Handler) extractEmbedStatusID(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	statusID := ctx.Param("id")

	// Fallback: extract from path if param not available (for testing)
	if err := common.ValidateRequiredParam("statusID", statusID); err != nil {
		path := ctx.Request.Path
		if strings.HasPrefix(path, "/embed/") {
			statusID = strings.TrimPrefix(path, "/embed/")
		}
	}

	if err := common.ValidateRequiredParam("statusID", statusID); err != nil {
		h.logger.Warn("missing status ID in embed page request")
		resp, respErr := common.RespondBadRequest(ctx, "missing status ID")
		return "", resp, respErr
	}

	return statusID, nil, nil
}

// normalizeEmbedObjectID normalizes the status ID to a full URL
func (h *Handler) normalizeEmbedObjectID(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// fetchEmbedNote fetches the note for embedding
func (h *Handler) fetchEmbedNote(ctx *apptheory.Context, objectID string) (*activitypub.Note, *apptheory.Response, error) {
	// Extract status ID from object ID
	statusID := strings.TrimPrefix(objectID, h.cfg.BaseURL()+"/objects/")

	// Fetch the status using Notes service
	result, err := h.registry.Notes().GetNote(ctx.Context(), statusID)
	if err != nil {
		h.logger.Error("failed to get note for embed", zap.String("status_id", statusID), zap.Error(err))
		resp, respErr := common.RespondStatusNotFound(ctx)
		return nil, resp, respErr
	}
	// Return the Note directly.
	if result.Note == nil {
		resp, respErr := common.RespondStatusNotFound(ctx)
		return nil, resp, respErr
	}
	return result.Note, nil, nil
}

// convertObjectToNote converts an object to an ActivityPub Note

// isStatusEmbeddable checks if a status is public or unlisted
func (h *Handler) isStatusEmbeddable(note *activitypub.Note) bool {
	// Check "to" field
	for _, to := range note.To {
		if to == activitypub.PublicAddress {
			return true
		}
	}

	// Check "cc" field
	for _, cc := range note.CC {
		if cc == activitypub.PublicAddress {
			return true
		}
	}

	return false
}

// getEmbedAuthorInfo retrieves author information for embedding
func (h *Handler) getEmbedAuthorInfo(ctx *apptheory.Context, note *activitypub.Note) embedAuthorInfo {
	info := embedAuthorInfo{
		name:     "Unknown",
		username: "unknown",
	}

	if err := common.ValidateRequiredParam("attributedTo", note.AttributedTo); err != nil {
		return info
	}

	// Extract username from actor ID
	parts := strings.Split(note.AttributedTo, "/")
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
		username := parts[len(parts)-1]
		info.username = username

		result, err := h.registry.Accounts().GetAccount(ctx.Context(), username)
		if err == nil && result != nil && result.Actor != nil {
			info.actor = result.Actor
			info.name = result.Actor.Name
			if err := common.ValidateRequiredParam("actorName", info.name); err != nil {
				info.name = result.Actor.PreferredUsername
			}
		}
	}

	return info
}

// formatEmbedTimestamp formats the timestamp for embedding
func (h *Handler) formatEmbedTimestamp(note *activitypub.Note) string {
	if note.Published != nil {
		return note.Published.Format("Jan 2, 2006")
	}
	return "Unknown"
}

func newCSPNonce() string {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return ""
	}
	return base64.RawStdEncoding.EncodeToString(nonceBytes)
}

// generateEmbedHTML generates the HTML for the embed page
func (h *Handler) generateEmbedHTML(note *activitypub.Note, authorInfo embedAuthorInfo, timestamp string, scriptNonce string) string {
	var htmlBuilder strings.Builder

	// Add HTML header and styles
	h.writeEmbedHTMLHeader(&htmlBuilder, authorInfo.name)

	// Add body content
	h.writeEmbedBodyContent(&htmlBuilder, note, authorInfo, timestamp)

	// Add footer and scripts
	h.writeEmbedHTMLFooter(&htmlBuilder, scriptNonce)

	return htmlBuilder.String()
}

// writeEmbedHTMLHeader writes the HTML header and styles
func (h *Handler) writeEmbedHTMLHeader(builder *strings.Builder, authorName string) {
	builder.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>`)
	builder.WriteString(htmlpkg.EscapeString(authorName))
	builder.WriteString(" - Lesser Instance")
	builder.WriteString(`</title>
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
<body>`)
}

// writeEmbedBodyContent writes the main body content
func (h *Handler) writeEmbedBodyContent(builder *strings.Builder, note *activitypub.Note, authorInfo embedAuthorInfo, timestamp string) {
	authorUsername := strings.TrimSpace(authorInfo.username)
	authorProfileURL := fmt.Sprintf("%s/@%s", h.cfg.BaseURL(), url.PathEscape(authorUsername))

	builder.WriteString(`
    <article class="status">
        <div class="author">
            <div class="avatar"></div>
            <div class="author-info">
                <a href="`)
	builder.WriteString(htmlpkg.EscapeString(authorProfileURL))
	builder.WriteString(`" target="_blank" class="display-name">`)
	builder.WriteString(htmlpkg.EscapeString(authorInfo.name))
	builder.WriteString(`</a>
                <div class="username">@`)
	builder.WriteString(htmlpkg.EscapeString(authorUsername))
	builder.WriteString(`</div>
            </div>
        </div>
        <div class="content">`)
	builder.WriteString(htmlsafe.SanitizeHTMLByContract(note.Content))
	builder.WriteString(`</div>`)

	// Add media attachments
	h.writeEmbedMediaAttachments(builder, note)

	// Add timestamp
	builder.WriteString(`
        <div>
            <a href="`)
	builder.WriteString(htmlpkg.EscapeString(note.ID))
	builder.WriteString(`" target="_blank" class="timestamp">`)
	builder.WriteString(htmlpkg.EscapeString(timestamp))
	builder.WriteString(`</a>
        </div>
    </article>`)
}

// writeEmbedMediaAttachments writes media attachments to the embed
func (h *Handler) writeEmbedMediaAttachments(builder *strings.Builder, note *activitypub.Note) {
	if err := common.ValidateSliceNotEmpty("attachments", note.Attachment); err != nil {
		return
	}

	for _, attachment := range note.Attachment {
		if h.isImageAttachment(attachment) {
			safeURL, ok := safeHTTPURL(attachment.URL)
			if !ok {
				continue
			}

			builder.WriteString(`<img src="`)
			builder.WriteString(htmlpkg.EscapeString(safeURL))
			builder.WriteString(`" alt="`)
			builder.WriteString(htmlpkg.EscapeString(attachment.Name))
			builder.WriteString(`" class="media">`)
		}
	}
}

// isImageAttachment checks if an attachment is an image
func (h *Handler) isImageAttachment(attachment activitypub.Attachment) bool {
	if attachment.Type != "Document" && attachment.Type != "Image" {
		return false
	}
	return attachment.MediaType != "" && strings.HasPrefix(attachment.MediaType, "image/")
}

func safeHTTPURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != schemeHTTP && scheme != schemeHTTPS {
		return "", false
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", false
	}

	return parsed.String(), true
}

// writeEmbedHTMLFooter writes the HTML footer and scripts
func (h *Handler) writeEmbedHTMLFooter(builder *strings.Builder, scriptNonce string) {
	if strings.TrimSpace(scriptNonce) == "" {
		builder.WriteString(`
</body>
</html>`)
		return
	}

	builder.WriteString(`
    <script nonce="`)
	builder.WriteString(htmlpkg.EscapeString(scriptNonce))
	builder.WriteString(`">
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
}
