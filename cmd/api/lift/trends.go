package lift

import (
	"fmt"
	netUrl "net/url"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/trends"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// getTrendService returns a new trend service instance
func (h *Handler) getTrendService() *trends.Service {
	return trends.NewService(h.repos)
}

// handleTrendError handles common trend service errors
func (h *Handler) handleTrendError(ctx *lift.Context, err error, operation string) error {
	if err != nil {
		h.logger.Error(fmt.Sprintf("failed to %s", operation), zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}
	return nil
}

// HandleGetTrendsLift handles GET /api/v1/trends
// Returns general trends (mix of all types)
func (h *Handler) HandleGetTrendsLift(ctx *lift.Context) error {
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, 10, 40)

	// Get mixed trends
	trends, err := trendService.GetTrends(ctx.Context, limit)
	if err := h.handleTrendError(ctx, err, "get trends"); err != nil {
		return err
	}

	return ctx.JSON(trends)
}

// HandleGetTrendingStatusesLift handles GET /api/v1/trends/statuses
// Returns trending statuses
func (h *Handler) HandleGetTrendingStatusesLift(ctx *lift.Context) error {
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, 20, 40)

	// Get trending statuses
	statuses, err := trendService.GetTrendingStatuses(ctx.Context, limit)
	if err := h.handleTrendError(ctx, err, "get trending statuses"); err != nil {
		return err
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
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, 10, 20)

	// Get trending hashtags
	hashtags, err := trendService.GetTrendingHashtags(ctx.Context, limit)
	if err := h.handleTrendError(ctx, err, "get trending hashtags"); err != nil {
		return err
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
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, 10, 20)

	// Get trending links
	links, err := trendService.GetTrendingLinks(ctx.Context, limit)
	if err := h.handleTrendError(ctx, err, "get trending links"); err != nil {
		return err
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
	if err := common.ValidateRequiredParam("url", url); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		url = ctx.Request.Request.QueryParams["url"]
	}

	if err := common.ValidateRequiredParam("url", url); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	trendService := h.getTrendService()

	// Get all statuses that contain this link
	statuses, err := trendService.GetStatusesByLink(ctx.Context, url, 20)
	if err := h.handleTrendError(ctx, err, "get statuses by link"); err != nil {
		return err
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
	if err != nil {
		return ""
	}
	if err := common.ValidateRequiredParam("scheme", parsed.Scheme); err != nil {
		return ""
	}
	if err := common.ValidateRequiredParam("host", parsed.Host); err != nil {
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

// HandleGetTrendsV2Lift handles GET /api/v2/trends
// Returns general trends with enhanced metadata
func (h *Handler) HandleGetTrendsV2Lift(ctx *lift.Context) error {
	// Initialize trend service if not already initialized
	trendService := trends.NewService(h.repos)

	// Get limit from query params, default to 10
	limitStr := ctx.Query("limit")

	// Fallback to direct query param access if ctx.Query doesn't work
	if err := common.ValidateRequiredParam("limit", limitStr); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}

	limit, err := common.ParseTimelineLimit(limitStr)
	if err != nil {
		limit = 10
	}

	// Note: offset parameter available but not used by underlying service

	// Get mixed trends
	trends, err := trendService.GetTrends(ctx.Context, limit)
	if err != nil {
		h.logger.Error("failed to get trends", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert to v2 format with enhanced metadata
	response := h.convertTrendsToV2Format(trends)

	return ctx.JSON(response)
}

// HandleGetTrendingTagsV2Lift handles GET /api/v2/trends/tags
// Returns trending hashtags with enhanced metrics
func (h *Handler) HandleGetTrendingTagsV2Lift(ctx *lift.Context) error {
	return h.handleTrendingV2Request(ctx, "hashtags", 10, func(service *trends.Service, limit int) (any, error) {
		return service.GetTrendingHashtags(ctx.Context, limit)
	}, h.convertHashtagsToV2Format)
}

// HandleGetTrendingStatusesV2Lift handles GET /api/v2/trends/statuses
// Returns trending statuses with enhanced metrics
func (h *Handler) HandleGetTrendingStatusesV2Lift(ctx *lift.Context) error {
	// Initialize trend service if not already initialized
	trendService := trends.NewService(h.repos)

	// Get limit from query params, default to 20
	limitStr := ctx.Query("limit")

	if err := common.ValidateRequiredParam("limit", limitStr); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}

	limit, err := common.ParseTimelineLimit(limitStr)
	if err != nil {
		limit = 20
	}

	// Note: offset parameter available but not used by underlying service

	// Get trending statuses with enhanced metrics
	statuses, err := trendService.GetTrendingStatuses(ctx.Context, limit)
	if err != nil {
		h.logger.Error("failed to get trending statuses", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert to v2 format with enhanced metrics
	response := h.convertStatusesToV2Format(statuses)

	return ctx.JSON(response)
}

// HandleGetTrendingLinksV2Lift handles GET /api/v2/trends/links
// Returns trending links with enhanced metadata
func (h *Handler) HandleGetTrendingLinksV2Lift(ctx *lift.Context) error {
	return h.handleTrendingV2Request(ctx, "links", 10, func(service *trends.Service, limit int) (any, error) {
		return service.GetTrendingLinks(ctx.Context, limit)
	}, h.convertLinksToV2Format)
}

// Helper functions for v2 format conversion

// handleTrendingV2Request handles common v2 trending request pattern
func (h *Handler) handleTrendingV2Request(
	ctx *lift.Context,
	itemType string,
	defaultLimit int,
	fetcher func(*trends.Service, int) (any, error),
	converter func(any) []map[string]any,
) error {
	// Initialize trend service
	trendService := trends.NewService(h.repos)

	// Get limit from query params
	limitStr := ctx.Query("limit")
	if err := common.ValidateRequiredParam("limit", limitStr); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}

	limit, err := common.ParseAndValidateIntWithBounds("limit", limitStr, 0, 20, defaultLimit)
	if err != nil {
		limit = defaultLimit
	}

	// Get trending items using provided fetcher
	items, err := fetcher(trendService, limit)
	if err != nil {
		h.logger.Error(fmt.Sprintf("failed to get trending %s", itemType), zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert to v2 format using provided converter
	response := converter(items)

	return ctx.JSON(response)
}

func (h *Handler) convertTrendsToV2Format(_ any) []map[string]any {
	// This is a placeholder - would convert to v2 format with enhanced metadata
	// Including usage metrics, trend duration, velocity, etc.
	return []map[string]any{}
}

func (h *Handler) convertHashtagsToV2Format(_ any) []map[string]any {
	// Enhanced hashtag format with metrics like velocity, peak usage, demographics
	return []map[string]any{}
}

func (h *Handler) convertStatusesToV2Format(_ any) []map[string]any {
	// Enhanced status format with engagement metrics, viral coefficients, etc.
	return []map[string]any{}
}

func (h *Handler) convertLinksToV2Format(_ any) []map[string]any {
	// Enhanced link format with click-through rates, source diversity, etc.
	return []map[string]any{}
}
