package handlers

import (
	"context"
	"fmt"
	netUrl "net/url"
	"strconv"
	"strings"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/trends"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// TrendHandlers provides handlers for trend-related endpoints
type TrendHandlers struct {
	trendService *trends.Service
	logger       *zap.Logger
}

// NewTrendHandlers creates new trend handlers
func NewTrendHandlers(trendService *trends.Service, logger *zap.Logger) *TrendHandlers {
	return &TrendHandlers{
		trendService: trendService,
		logger:       logger,
	}
}

// HandleGetTrends handles GET /api/v1/trends
// Returns general trends (mix of all types)
func (h *Handler) HandleGetTrends(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get limit from query params, default to 10
	limit := 10
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	// Get mixed trends
	trends, err := h.trendService.GetTrends(ctx, limit)
	if err != nil {
		h.logger.Error("failed to get trends", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get trends: %w", err)), nil
	}

	return common.OK(trends), nil
}

// HandleGetTrendingStatuses handles GET /api/v1/trends/statuses
// Returns trending statuses
func (h *Handler) HandleGetTrendingStatuses(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get limit from query params, default to 20
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	// Get trending statuses
	statuses, err := h.trendService.GetTrendingStatuses(ctx, limit)
	if err != nil {
		h.logger.Error("failed to get trending statuses", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get trending statuses: %w", err)), nil
	}

	// Convert to Mastodon API format
	response := make([]map[string]interface{}, len(statuses))
	for i, s := range statuses {
		response[i] = map[string]interface{}{
			"id":         s.StatusID,
			"url":        s.URL,
			"account":    map[string]interface{}{"id": s.AuthorID},
			"content":    s.Content,
			"created_at": s.PublishedAt.Format("2006-01-02T15:04:05.000Z"),
		}
	}

	return common.OK(response), nil
}

// HandleGetTrendingTags handles GET /api/v1/trends/tags
// Returns trending hashtags
func (h *Handler) HandleGetTrendingTags(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get limit from query params, default to 10
	limit := 10
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 20 {
			limit = l
		}
	}

	// Get trending hashtags
	hashtags, err := h.trendService.GetTrendingHashtags(ctx, limit)
	if err != nil {
		h.logger.Error("failed to get trending hashtags", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get trending hashtags: %w", err)), nil
	}

	// Convert to Mastodon API format
	response := make([]map[string]interface{}, len(hashtags))
	for i, h := range hashtags {
		// Convert history to strings for Mastodon API
		history := make([]map[string]string, len(h.History))
		for j, count := range h.History {
			history[j] = map[string]string{
				"day":      strconv.Itoa(j), // Day offset from today
				"uses":     strconv.FormatInt(count, 10),
				"accounts": strconv.FormatInt(h.Accounts/int64(len(h.History)), 10), // Rough estimate
			}
		}

		response[i] = map[string]interface{}{
			"name":    h.Name,
			"url":     h.URL,
			"history": history,
		}
	}

	return common.OK(response), nil
}

// HandleGetTrendingLinks handles GET /api/v1/trends/links
// Returns trending links
func (h *Handler) HandleGetTrendingLinks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get limit from query params, default to 10
	limit := 10
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 20 {
			limit = l
		}
	}

	// Get trending links
	links, err := h.trendService.GetTrendingLinks(ctx, limit)
	if err != nil {
		h.logger.Error("failed to get trending links", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get trending links: %w", err)), nil
	}

	// Convert to Mastodon API format
	response := make([]map[string]interface{}, len(links))
	for i, l := range links {
		response[i] = map[string]interface{}{
			"url":           l.URL,
			"title":         l.Title,
			"description":   l.Description,
			"type":          l.Type,
			"author_name":   l.AuthorName,
			"author_url":    "",
			"provider_name": h.extractProviderName(l.URL),
			"provider_url":  h.extractProviderURL(l.URL),
			"html":          "",
			"width":         0,
			"height":        0,
			"image":         l.Image,
			"embed_url":     "",
			"blurhash":      "",
		}
	}

	return common.OK(response), nil
}

// HandleGetLinkTimeline handles GET /api/v1/timelines/link
// Returns timeline for a specific link
func (h *Handler) HandleGetLinkTimeline(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract URL from query params
	url := request.QueryStringParameters["url"]
	if url == "" {
		return common.BadRequest(fmt.Errorf("URL parameter required")), nil
	}

	// Get all statuses that contain this link
	statuses, err := h.store.GetStatusesByLink(ctx, url, 20)
	if err != nil {
		h.logger.Error("failed to get statuses by link", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert statuses to timeline format
	trendingStatuses := make([]*storage.TrendingStatus, 0, len(statuses))
	for _, s := range statuses {
		if ts, ok := s.(*storage.TrendingStatus); ok {
			trendingStatuses = append(trendingStatuses, ts)
		}
	}
	timeline := h.convertStatusesToTimeline(ctx, trendingStatuses)
	return common.OK(timeline), nil
}

// Helper methods for trends
func (h *Handler) getFullAccountInfo(ctx context.Context, userID string) *models.Account {
	user, err := h.store.GetUser(ctx, userID)
	if err != nil {
		h.logger.Warn("failed to get account info", zap.Error(err))
		return nil
	}

	return &models.Account{
		ID:          user.Username,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		// Add other fields as needed
	}
}

func (h *Handler) getAuthorURL(authorID string) string {
	if authorID == "" {
		return ""
	}
	return fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), authorID)
}

func (h *Handler) extractProviderName(url string) string {
	parsed, err := netUrl.Parse(url)
	if err != nil {
		return ""
	}

	domain := parsed.Hostname()
	// Remove www. prefix
	if strings.HasPrefix(domain, "www.") {
		domain = domain[4:]
	}

	return domain
}

func (h *Handler) extractProviderURL(url string) string {
	parsed, err := netUrl.Parse(url)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
}

func (h *Handler) generateLinkEmbedHTML(link *storage.TrendingLink) string {
	if link.ImageURL == "" {
		return fmt.Sprintf("<a href=\"%s\">%s</a>", link.URL, link.Title)
	}

	return fmt.Sprintf(`<div class="link-preview">
		<img src="%s" alt="%s" />
		<h3><a href="%s">%s</a></h3>
		<p>%s</p>
	</div>`, link.ImageURL, link.Title, link.URL, link.Title, link.Description)
}

func (h *Handler) convertStatusesToTimeline(ctx context.Context, statuses []*storage.TrendingStatus) []interface{} {
	result := make([]interface{}, len(statuses))
	for i, status := range statuses {
		result[i] = map[string]interface{}{
			"id":      status.ID,
			"content": status.Content,
			"url":     fmt.Sprintf("%s/statuses/%s", h.cfg.BaseURL(), status.ID),
			// Add other status fields as needed
		}
	}
	return result
}
