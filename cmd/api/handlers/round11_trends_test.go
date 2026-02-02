package handlers

import (
	"net/http"
	"testing"
	"time"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestTrendsHandlers(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	linkURL := "https://example.com/articles/1"

	state := &round10QueryState{
		trendingHashtags: []storagemodels.HashtagTrend{
			{Name: "go", URL: cfg.BaseURL() + "/tags/go", UsageCount: 10, UniqueUsers: 5, FirstSeen: now.Add(-24 * time.Hour), LastUsed: now},
		},
		trendingStatuses: []storagemodels.StatusTrend{
			{ID: "s1", URL: cfg.BaseURL() + "/statuses/s1", AuthorID: cfg.ActorURL("alice"), Content: "status", PublishedAt: now.Add(-1 * time.Hour)},
		},
		trendingLinks: []storagemodels.LinkTrend{
			{URL: linkURL, Title: "Example", Description: "desc", Type: "link", AuthorName: "alice", Image: "https://example.com/image.png", ShareCount: 2},
		},
		statusByID: map[string]storagemodels.Status{
			"s1": {StatusID: "s1", Content: "see " + linkURL},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	ctxTrends, err := round10NewLiftContext(http.MethodGet, "/api/v1/trends", nil, map[string]string{"limit": "5"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetTrendsLift(ctxTrends))

	ctxStatuses, err := round10NewLiftContext(http.MethodGet, "/api/v1/trends/statuses", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetTrendingStatusesLift(ctxStatuses))

	ctxTags, err := round10NewLiftContext(http.MethodGet, "/api/v1/trends/tags", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetTrendingTagsLift(ctxTags))

	ctxLinks, err := round10NewLiftContext(http.MethodGet, "/api/v1/trends/links", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetTrendingLinksLift(ctxLinks))

	ctxLinkTimeline, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/link", nil, map[string]string{"url": linkURL}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetLinkTimelineLift(ctxLinkTimeline))

	ctxTrendsV2, err := round10NewLiftContext(http.MethodGet, "/api/v2/trends", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetTrendsV2Lift(ctxTrendsV2))

	ctxTagsV2, err := round10NewLiftContext(http.MethodGet, "/api/v2/trends/tags", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetTrendingTagsV2Lift(ctxTagsV2))

	ctxStatusesV2, err := round10NewLiftContext(http.MethodGet, "/api/v2/trends/statuses", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetTrendingStatusesV2Lift(ctxStatusesV2))

	ctxLinksV2, err := round10NewLiftContext(http.MethodGet, "/api/v2/trends/links", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetTrendingLinksV2Lift(ctxLinksV2))

	require.Equal(t, "example.com", handler.extractProviderNameLift(linkURL))
	require.Equal(t, "https://example.com", handler.extractProviderURLLift(linkURL))
	require.Equal(t, "", handler.extractProviderURLLift(":bad"))
}
