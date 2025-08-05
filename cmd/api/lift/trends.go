package lift

import (
	"fmt"
	netUrl "net/url"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/trends"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetTrendsLift handles GET /api/v1/trends
// Returns general trends (mix of all types)
func (h *Handler) HandleGetTrendsLift(ctx *lift.Context) error {
	// Initialize trend service if not already initialized
	trendService := trends.NewService(h.repos)

	// Get limit from query params, default to 10
	limit := 10
	limitStr := ctx.Query("limit")
	
	// Fallback to direct query param access if ctx.Query doesn't work
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	// Get mixed trends
	trends, err := trendService.GetTrends(ctx.Context, limit)
	if err != nil {
		h.logger.Error("failed to get trends", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	return ctx.JSON(trends)
}

// HandleGetTrendingStatusesLift handles GET /api/v1/trends/statuses
// Returns trending statuses
func (h *Handler) HandleGetTrendingStatusesLift(ctx *lift.Context) error {
	// Initialize trend service if not already initialized
	trendService := trends.NewService(h.repos)

	// Get limit from query params, default to 20
	limit := 20
	limitStr := ctx.Query("limit")
	
	// Fallback to direct query param access if ctx.Query doesn't work
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 40 {
			limit = l
		}
	}

	// Get trending statuses
	statuses, err := trendService.GetTrendingStatuses(ctx.Context, limit)
	if err != nil {
		h.logger.Error("failed to get trending statuses", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert to Mastodon API format
	response := make([]map[string]any, len(statuses))
	for i, s := range statuses {
		response[i] = map[string]any{
			"id":         s.StatusID,
			"url":        s.URL,
			"account":    map[string]any{"id": s.AuthorID},
			"content":    s.Content,
			"created_at": s.PublishedAt.Format("2006-01-02T15:04:05.000Z"),
		}
	}

	return ctx.JSON(response)
}

// HandleGetTrendingTagsLift handles GET /api/v1/trends/tags
// Returns trending hashtags
func (h *Handler) HandleGetTrendingTagsLift(ctx *lift.Context) error {
	// Initialize trend service if not already initialized
	trendService := trends.NewService(h.repos)

	// Get limit from query params, default to 10
	limit := 10
	limitStr := ctx.Query("limit")
	
	// Fallback to direct query param access if ctx.Query doesn't work
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 20 {
			limit = l
		}
	}

	// Get trending hashtags
	hashtags, err := trendService.GetTrendingHashtags(ctx.Context, limit)
	if err != nil {
		h.logger.Error("failed to get trending hashtags", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert to Mastodon API format
	response := make([]map[string]any, len(hashtags))
	for i, hashtagItem := range hashtags {
		// Convert history to strings for Mastodon API
		history := make([]map[string]string, len(hashtagItem.History))
		for j, count := range hashtagItem.History {
			history[j] = map[string]string{
				"day":      strconv.Itoa(j), // Day offset from today
				"uses":     strconv.FormatInt(count, 10),
				"accounts": strconv.FormatInt(hashtagItem.Accounts/int64(len(hashtagItem.History)), 10), // Rough estimate
			}
		}

		response[i] = map[string]any{
			"name":    hashtagItem.Name,
			"url":     hashtagItem.URL,
			"history": history,
		}
	}

	return ctx.JSON(response)
}

// HandleGetTrendingLinksLift handles GET /api/v1/trends/links
// Returns trending links
func (h *Handler) HandleGetTrendingLinksLift(ctx *lift.Context) error {
	// Initialize trend service if not already initialized
	trendService := trends.NewService(h.repos)

	// Get limit from query params, default to 10
	limit := 10
	limitStr := ctx.Query("limit")
	
	// Fallback to direct query param access if ctx.Query doesn't work
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 20 {
			limit = l
		}
	}

	// Get trending links
	links, err := trendService.GetTrendingLinks(ctx.Context, limit)
	if err != nil {
		h.logger.Error("failed to get trending links", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert to Mastodon API format
	response := make([]map[string]any, len(links))
	for i, l := range links {
		response[i] = map[string]any{
			"url":           l.URL,
			"title":         l.Title,
			"description":   l.Description,
			"type":          l.Type,
			"author_name":   l.AuthorName,
			"author_url":    "",
			"provider_name": h.extractProviderNameLift(l.URL),
			"provider_url":  h.extractProviderURLLift(l.URL),
			"html":          "",
			"width":         0,
			"height":        0,
			"image":         l.Image,
			"embed_url":     "",
			"blurhash":      "",
		}
	}

	return ctx.JSON(response)
}

// HandleGetLinkTimelineLift handles GET /api/v1/timelines/link
// Returns timeline for a specific link
func (h *Handler) HandleGetLinkTimelineLift(ctx *lift.Context) error {
	// Extract URL from query params
	url := ctx.Query("url")
	
	// Fallback to direct query param access if ctx.Query doesn't work
	if url == "" && ctx.Request != nil && ctx.Request.Request != nil {
		url = ctx.Request.Request.QueryParams["url"]
	}
	
	if url == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "URL parameter required"})
	}

	// Get all statuses that contain this link
	statuses, err := h.repos.Analytics().GetStatusesByLink(ctx.Context, url, 20)
	if err != nil {
		h.logger.Error("failed to get statuses by link", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert statuses to timeline format
	trendingStatuses := make([]*storage.TrendingStatus, 0, len(statuses))
	for _, s := range statuses {
		if ts, ok := s.(*storage.TrendingStatus); ok {
			trendingStatuses = append(trendingStatuses, ts)
		}
	}
	timeline := h.convertStatusesToTimelineLift(trendingStatuses)
	return ctx.JSON(timeline)
}

// Helper methods for trends

// extractProviderNameLift extracts the provider name from a URL
func (h *Handler) extractProviderNameLift(url string) string {
	parsed, err := netUrl.Parse(url)
	if err != nil {
		return ""
	}

	domain := parsed.Hostname()
	// Remove www. prefix
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}

// extractProviderURLLift extracts the provider URL from a URL
func (h *Handler) extractProviderURLLift(url string) string {
	parsed, err := netUrl.Parse(url)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
}

// convertStatusesToTimelineLift converts trending statuses to timeline format
func (h *Handler) convertStatusesToTimelineLift(statuses []*storage.TrendingStatus) []any {
	result := make([]any, len(statuses))
	for i, status := range statuses {
		result[i] = map[string]any{
			"id":      status.ID,
			"content": status.Content,
			"url":     fmt.Sprintf("%s/statuses/%s", h.cfg.BaseURL(), status.ID),
			// Add other status fields as needed
		}
	}
	return result
}