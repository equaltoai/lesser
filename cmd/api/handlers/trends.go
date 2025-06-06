package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aron23/lesser/pkg/common"
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
		// TODO: Get full account info from storage
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
			"author_url":    "", // TODO: Get author URL if available
			"provider_name": "", // TODO: Extract provider from URL
			"provider_url":  "", // TODO: Extract provider URL
			"html":          "", // TODO: Generate embed HTML
			"width":         0,  // TODO: Get dimensions from image
			"height":        0,
			"image":         l.Image,
			"embed_url":     "",
			"blurhash":      "", // TODO: Generate blurhash
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

	// TODO: Implement link timeline - get all statuses that contain this link
	// For now, return empty array
	return common.OK([]interface{}{}), nil
}
