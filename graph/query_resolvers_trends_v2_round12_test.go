package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/trends"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_TrendsV2_ServiceAndConverters(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := &queryResolver{resolver}

	// Storage required.
	qNoStorage := &queryResolver{&Resolver{}}
	_, err := qNoStorage.Trends(context.Background(), nil)
	require.Error(t, err)

	// Resolver paths (empty results are fine in this harness).
	items, err := q.Trends(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, items)

	limit := 5
	tags, err := q.TrendingTags(context.Background(), &limit)
	require.NoError(t, err)
	require.NotNil(t, tags)

	statuses, err := q.TrendingStatuses(context.Background(), &limit)
	require.NoError(t, err)
	require.NotNil(t, statuses)

	links, err := q.TrendingLinks(context.Background(), &limit)
	require.NoError(t, err)
	require.NotNil(t, links)

	// Converters.
	require.Nil(t, convertHashtagTrendToModel(nil))
	require.Nil(t, convertStatusTrendToModel(nil))
	require.Nil(t, convertLinkTrendToModel(nil))

	tag := convertHashtagTrendToModel(&trends.HashtagTrend{
		Name:     "tag",
		URL:      "https://localhost/tags/tag",
		History:  []int64{1, 2},
		Uses:     3,
		Accounts: 4,
	})
	require.NotNil(t, tag)
	require.Len(t, tag.History, 2)

	st := convertStatusTrendToModel(&trends.StatusTrend{
		StatusID:    "s1",
		URL:         "https://localhost/statuses/s1",
		AuthorID:    "alice",
		Content:     "hi",
		Engagements: 2,
		PublishedAt: time.Now(),
	})
	require.NotNil(t, st)

	ln := convertLinkTrendToModel(&trends.LinkTrend{
		URL:         "https://example.com",
		Title:       "t",
		Description: "d",
		Type:        "link",
		AuthorName:  "alice",
		Image:       "https://example.com/i.png",
		Shares:      1,
	})
	require.NotNil(t, ln)

	// TrendingItem conversion covers value-vs-pointer handling and unsupported cases.
	require.Nil(t, convertTrendToTrendingItem(trends.Trend{Type: trendTypeHashtag, Value: "nope"}))
	require.Nil(t, convertTrendToTrendingItem(trends.Trend{Type: trendTypeStatus, Value: "nope"}))
	require.Nil(t, convertTrendToTrendingItem(trends.Trend{Type: trendTypeLink, Value: "nope"}))
	require.Nil(t, convertTrendToTrendingItem(trends.Trend{Type: "unknown", Value: nil}))

	item := convertTrendToTrendingItem(trends.Trend{Type: trendTypeHashtag, Value: trends.HashtagTrend{Name: "tag"}})
	require.NotNil(t, item)
	require.NotNil(t, item.Hashtag)

	item = convertTrendToTrendingItem(trends.Trend{Type: trendTypeHashtag, Value: &trends.HashtagTrend{Name: "tag"}})
	require.NotNil(t, item)

	item = convertTrendToTrendingItem(trends.Trend{Type: trendTypeStatus, Value: trends.StatusTrend{StatusID: "s1"}})
	require.NotNil(t, item)

	item = convertTrendToTrendingItem(trends.Trend{Type: trendTypeLink, Value: &trends.LinkTrend{URL: "https://example.com"}})
	require.NotNil(t, item)
}
