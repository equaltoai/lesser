package handlers

import (
	"fmt"
	netUrl "net/url"
	"strconv"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/trends"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// getTrendService returns a new trend service instance
func (h *Handler) getTrendService() *trends.Service {
	return trends.NewService(h.repos)
}

// HandleGetTrendsLift handles GET /api/v1/trends
// Returns general trends (mix of all types)
func (h *Handler) HandleGetTrendsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, 10, 40)

	// Get mixed trends
	trends, err := trendService.GetTrends(ctx.Context(), limit)
	if err != nil {
		h.logger.Error("failed to get trends", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	response := make([]apimodels.Trend, 0, len(trends))
	for _, t := range trends {
		response = append(response, apimodels.Trend{
			Type:  t.Type,
			Value: t.Value,
		})
	}

	return okJSON(response)
}

// HandleGetTrendingStatusesLift handles GET /api/v1/trends/statuses
// Returns trending statuses
func (h *Handler) HandleGetTrendingStatusesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, 20, 40)

	// Get trending statuses
	statuses, err := trendService.GetTrendingStatuses(ctx.Context(), limit)
	if err != nil {
		h.logger.Error("failed to get trending statuses", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert to Mastodon API format
	response := make([]apimodels.TrendingStatusSummary, 0, len(statuses))
	for _, s := range statuses {
		response = append(response, apimodels.TrendingStatusSummary{
			ID:        s.StatusID,
			URL:       s.URL,
			Account:   apimodels.TrendingStatusAccount{ID: s.AuthorID},
			Content:   s.Content,
			CreatedAt: s.PublishedAt.Format("2006-01-02T15:04:05.000Z"),
		})
	}

	return okJSON(response)
}

// HandleGetTrendingTagsLift handles GET /api/v1/trends/tags
// Returns trending hashtags
func (h *Handler) HandleGetTrendingTagsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleTrendingTags(ctx, 10, 20)
}

// HandleGetTrendingLinksLift handles GET /api/v1/trends/links
// Returns trending links
func (h *Handler) HandleGetTrendingLinksLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleTrendingLinks(ctx, 10, 20)
}

func (h *Handler) handleTrendingTags(ctx *apptheory.Context, defaultLimit, maxLimit int) (*apptheory.Response, error) {
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, defaultLimit, maxLimit)

	hashtags, err := trendService.GetTrendingHashtags(ctx.Context(), limit)
	if err != nil {
		h.logger.Error("failed to get trending hashtags", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	response := make([]apimodels.Tag, 0, len(hashtags))
	for _, hashtagItem := range hashtags {
		history := make([]apimodels.TagHistory, 0, len(hashtagItem.History))

		accountsPerDay := int64(0)
		if len(hashtagItem.History) > 0 {
			accountsPerDay = hashtagItem.Accounts / int64(len(hashtagItem.History))
		}

		for j, count := range hashtagItem.History {
			history = append(history, apimodels.TagHistory{
				Day:      strconv.Itoa(j), // Day offset from today
				Uses:     strconv.FormatInt(count, 10),
				Accounts: strconv.FormatInt(accountsPerDay, 10), // Rough estimate
			})
		}

		response = append(response, apimodels.Tag{
			Name:    hashtagItem.Name,
			URL:     hashtagItem.URL,
			History: history,
		})
	}

	return okJSON(response)
}

func (h *Handler) handleTrendingLinks(ctx *apptheory.Context, defaultLimit, maxLimit int) (*apptheory.Response, error) {
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, defaultLimit, maxLimit)

	links, err := trendService.GetTrendingLinks(ctx.Context(), limit)
	if err != nil {
		h.logger.Error("failed to get trending links", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	response := make([]apimodels.PreviewCard, 0, len(links))
	for _, l := range links {
		response = append(response, apimodels.PreviewCard{
			URL:          l.URL,
			Title:        l.Title,
			Description:  l.Description,
			Type:         l.Type,
			AuthorName:   l.AuthorName,
			AuthorURL:    "",
			ProviderName: h.extractProviderNameLift(l.URL),
			ProviderURL:  h.extractProviderURLLift(l.URL),
			HTML:         "",
			Width:        0,
			Height:       0,
			Image:        l.Image,
			EmbedURL:     "",
			Blurhash:     "",
		})
	}

	return okJSON(response)
}

// HandleGetLinkTimelineLift handles GET /api/v1/timelines/link
// Returns timeline for a specific link
func (h *Handler) HandleGetLinkTimelineLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract URL from query params
	url := queryValue(ctx, "url")

	if err := common.ValidateRequiredParam("url", url); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	trendService := h.getTrendService()

	// Get all statuses that contain this link
	statuses, err := trendService.GetStatusesByLink(ctx.Context(), url, 20)
	if err != nil {
		h.logger.Error("failed to get statuses by link", zap.String("url", url), zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert statuses to timeline format
	trendingStatuses := make([]*storage.TrendingStatus, 0, len(statuses))
	for _, s := range statuses {
		if ts, ok := s.(*storage.TrendingStatus); ok {
			trendingStatuses = append(trendingStatuses, ts)
		}
	}
	timeline := make([]apimodels.LinkTimelineEntry, 0, len(trendingStatuses))
	for _, status := range trendingStatuses {
		timeline = append(timeline, apimodels.LinkTimelineEntry{
			ID:      status.ID,
			Content: status.Content,
			URL:     fmt.Sprintf("%s/statuses/%s", h.cfg.BaseURL(), status.ID),
		})
	}
	return okJSON(timeline)
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

// HandleGetTrendsV2Lift handles GET /api/v2/trends
// Returns general trends with enhanced metadata
func (h *Handler) HandleGetTrendsV2Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, 10, 40)

	trends, err := trendService.GetTrends(ctx.Context(), limit)
	if err != nil {
		h.logger.Error("failed to get trends", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	response := make([]apimodels.Trend, 0, len(trends))
	for _, t := range trends {
		response = append(response, apimodels.Trend{
			Type:  t.Type,
			Value: t.Value,
		})
	}

	return okJSON(response)
}

// HandleGetTrendingTagsV2Lift handles GET /api/v2/trends/tags
// Returns trending hashtags with enhanced metrics
func (h *Handler) HandleGetTrendingTagsV2Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleTrendingTags(ctx, 10, 20)
}

// HandleGetTrendingStatusesV2Lift handles GET /api/v2/trends/statuses
// Returns trending statuses with enhanced metrics
func (h *Handler) HandleGetTrendingStatusesV2Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	trendService := h.getTrendService()
	limit := h.parseLimitParam(ctx, 20, 40)

	statuses, err := trendService.GetTrendingStatuses(ctx.Context(), limit)
	if err != nil {
		h.logger.Error("failed to get trending statuses", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	response := make([]apimodels.TrendingStatusSummary, 0, len(statuses))
	for _, s := range statuses {
		response = append(response, apimodels.TrendingStatusSummary{
			ID:        s.StatusID,
			URL:       s.URL,
			Account:   apimodels.TrendingStatusAccount{ID: s.AuthorID},
			Content:   s.Content,
			CreatedAt: s.PublishedAt.Format("2006-01-02T15:04:05.000Z"),
		})
	}

	return okJSON(response)
}

// HandleGetTrendingLinksV2Lift handles GET /api/v2/trends/links
// Returns trending links with enhanced metadata
func (h *Handler) HandleGetTrendingLinksV2Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleTrendingLinks(ctx, 10, 20)
}
