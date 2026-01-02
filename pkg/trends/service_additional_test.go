package trends

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

type fakeAnalyticsRepo struct {
	hashtags []*storage.TrendingHashtag
	statuses []*storage.TrendingStatus
	links    []*storage.TrendingLink

	errHashtags error
	errStatuses error
	errLinks    error

	recordHashtagCalled bool
	recordStatusCalled  bool
	recordLinkCalled    bool
}

func (f *fakeAnalyticsRepo) GetTrendingHashtags(_ context.Context, _ time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	if f.errHashtags != nil {
		return nil, f.errHashtags
	}
	if limit > 0 && len(f.hashtags) > limit {
		return f.hashtags[:limit], nil
	}
	return f.hashtags, nil
}

func (f *fakeAnalyticsRepo) GetTrendingStatuses(_ context.Context, _ time.Time, limit int) ([]*storage.TrendingStatus, error) {
	if f.errStatuses != nil {
		return nil, f.errStatuses
	}
	if limit > 0 && len(f.statuses) > limit {
		return f.statuses[:limit], nil
	}
	return f.statuses, nil
}

func (f *fakeAnalyticsRepo) GetTrendingLinks(_ context.Context, _ time.Time, limit int) ([]*storage.TrendingLink, error) {
	if f.errLinks != nil {
		return nil, f.errLinks
	}
	if limit > 0 && len(f.links) > limit {
		return f.links[:limit], nil
	}
	return f.links, nil
}

func (f *fakeAnalyticsRepo) RecordHashtagUsage(_ context.Context, _ string, _ string, _ string) error {
	f.recordHashtagCalled = true
	return nil
}

func (f *fakeAnalyticsRepo) RecordStatusEngagement(_ context.Context, _ string, _ string, _ string) error {
	f.recordStatusCalled = true
	return nil
}

func (f *fakeAnalyticsRepo) RecordLinkShare(_ context.Context, _ string, _ string, _ string) error {
	f.recordLinkCalled = true
	return nil
}

func (f *fakeAnalyticsRepo) GetStatusesByLink(_ context.Context, _ string, _ int) ([]interface{}, error) {
	return []interface{}{"a", "b"}, nil
}

type fakeHashtagRepo struct {
	history []int64
	err     error
	calls   int
}

func (f *fakeHashtagRepo) GetHashtagUsageHistory(_ context.Context, _ string, _ int) ([]int64, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.history, nil
}

func TestService_GetTrends_CombinesAndLimits(t *testing.T) {
	t.Parallel()

	analytics := &fakeAnalyticsRepo{
		hashtags: []*storage.TrendingHashtag{
			{Name: "a", URL: "/tags/a", UsageCount: 1, UniqueUsers: 1},
			{Name: "b", URL: "/tags/b", UsageCount: 2, UniqueUsers: 2},
		},
		statuses: []*storage.TrendingStatus{{ID: "s1", URL: "/s/1"}},
		links:    []*storage.TrendingLink{{URL: "https://example.com"}},
	}

	svc := &Service{
		analytics: analytics,
		hashtag:   &fakeHashtagRepo{history: []int64{1, 2, 3}},
		algorithm: NewDefaultAlgorithm(),
	}

	trends, err := svc.GetTrends(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, trends, 2)
	require.Equal(t, "hashtag", trends[0].Type)
}

func TestService_GetTrends_WrapsCategoryErrors(t *testing.T) {
	t.Parallel()

	svc := &Service{
		analytics: &fakeAnalyticsRepo{errHashtags: errors.New("boom")},
		hashtag:   &fakeHashtagRepo{},
		algorithm: NewDefaultAlgorithm(),
	}

	_, err := svc.GetTrends(context.Background(), 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get hashtag trends")
}

func TestService_GetTrendingHashtags_UsesHistoryAndIgnoresHistoryErrors(t *testing.T) {
	t.Parallel()

	analytics := &fakeAnalyticsRepo{
		hashtags: []*storage.TrendingHashtag{{Name: "a", URL: "/tags/a", UsageCount: 1, UniqueUsers: 1}},
	}

	hashtag := &fakeHashtagRepo{history: []int64{1, 2, 3}}
	svc := &Service{analytics: analytics, hashtag: hashtag, algorithm: NewDefaultAlgorithm()}

	out, err := svc.GetTrendingHashtags(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, []int64{1, 2, 3}, out[0].History)
	require.Equal(t, 1, hashtag.calls)

	hashtag.err = errors.New("history down")
	out, err = svc.GetTrendingHashtags(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Nil(t, out[0].History)
}

func TestService_RecordMethods_DelegateToAnalytics(t *testing.T) {
	t.Parallel()

	analytics := &fakeAnalyticsRepo{}
	svc := &Service{analytics: analytics, hashtag: &fakeHashtagRepo{}, algorithm: NewDefaultAlgorithm()}

	require.NoError(t, svc.RecordHashtagUsage(context.Background(), "a", "s", "u"))
	require.True(t, analytics.recordHashtagCalled)

	require.NoError(t, svc.RecordStatusEngagement(context.Background(), "s", "like", "u"))
	require.True(t, analytics.recordStatusCalled)

	require.NoError(t, svc.RecordLinkShare(context.Background(), "https://example.com", "s", "u"))
	require.True(t, analytics.recordLinkCalled)
}

func TestService_GetStatusesByLink_DelegatesToAnalytics(t *testing.T) {
	t.Parallel()

	analytics := &fakeAnalyticsRepo{}
	svc := &Service{analytics: analytics, hashtag: &fakeHashtagRepo{}, algorithm: NewDefaultAlgorithm()}

	out, err := svc.GetStatusesByLink(context.Background(), "https://example.com", 10)
	require.NoError(t, err)
	require.Equal(t, []interface{}{"a", "b"}, out)
}

