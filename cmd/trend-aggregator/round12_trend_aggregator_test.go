package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type fakeTrendingRepo struct {
	hashtags []*storage.TrendingHashtag
	statuses []*storage.TrendingStatus
	links    []*storage.TrendingLink

	getHashtagsErr error
	getStatusesErr error
	getLinksErr    error

	storeHashtagErr error
	storeStatusErr  error
	storeLinkErr    error

	deleteHashtagErr error
	deleteStatusErr  error
	deleteLinkErr    error

	storedHashtags int
	storedStatuses int
	storedLinks    int
}

func (f *fakeTrendingRepo) GetRecentHashtags(context.Context, time.Time, int) ([]*storage.TrendingHashtag, error) {
	if f.getHashtagsErr != nil {
		return nil, f.getHashtagsErr
	}
	return f.hashtags, nil
}

func (f *fakeTrendingRepo) GetRecentStatusesWithEngagement(context.Context, time.Time, int) ([]*storage.TrendingStatus, error) {
	if f.getStatusesErr != nil {
		return nil, f.getStatusesErr
	}
	return f.statuses, nil
}

func (f *fakeTrendingRepo) GetRecentLinks(context.Context, time.Time, int) ([]*storage.TrendingLink, error) {
	if f.getLinksErr != nil {
		return nil, f.getLinksErr
	}
	return f.links, nil
}

func (f *fakeTrendingRepo) StoreHashtagTrend(context.Context, any) error {
	f.storedHashtags++
	return f.storeHashtagErr
}

func (f *fakeTrendingRepo) StoreStatusTrend(context.Context, any) error {
	f.storedStatuses++
	return f.storeStatusErr
}

func (f *fakeTrendingRepo) StoreLinkTrend(context.Context, any) error {
	f.storedLinks++
	return f.storeLinkErr
}

func (f *fakeTrendingRepo) DeleteOldHashtagTrends(context.Context, time.Time) error {
	return f.deleteHashtagErr
}

func (f *fakeTrendingRepo) DeleteOldStatusTrends(context.Context, time.Time) error {
	return f.deleteStatusErr
}

func (f *fakeTrendingRepo) DeleteOldLinkTrends(context.Context, time.Time) error {
	return f.deleteLinkErr
}

func TestInitializeTrendAggregator_Round12(t *testing.T) {
	origMustInitialize := mustInitializeLambdaFn
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInitialize
		newLambdaOptimizedClientFn = origNewClient
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:   &config.Config{Region: "us-east-1"},
			Logger:   zap.NewNop(),
			DynamoDB: nil,
		}
	}

	mockDB := new(dynamormmocks.MockDB)
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormcore.DB, error) {
		return mockDB, nil
	}

	initializeTrendAggregator()

	require.NotNil(t, lambdaCtx)
	require.NotNil(t, handler)
	require.NotNil(t, db)
}

func TestTrendAggregator_AggregationAndCleanup_Round12(t *testing.T) {
	now := time.Now().UTC()
	hashtags := []*storage.TrendingHashtag{
		{Name: "golang", UserID: "u1", FirstSeen: now, LastUsed: now},
		{Name: "golang", UserID: "u2", FirstSeen: now, LastUsed: now},
		{Name: "golang", UserID: "u1", FirstSeen: now, LastUsed: now},
		{Name: "golang", UserID: "u3", FirstSeen: now, LastUsed: now},
		{Name: "skipme", UserID: "", FirstSeen: now, LastUsed: now},
		{Name: "tiny", UserID: "u1", FirstSeen: now, LastUsed: now},
		{Name: "tiny", UserID: "u2", FirstSeen: now, LastUsed: now},
	}

	statuses := []*storage.TrendingStatus{
		{ID: "s1", AuthorID: "a1", Content: "hi", Likes: 10, Boosts: 1, Replies: 0, CreatedAt: now.Add(2 * time.Hour)},
		{ID: "s2", AuthorID: "a2", Content: "low", Likes: 1, Boosts: 0, Replies: 0, CreatedAt: now.Add(-2 * time.Hour)},
	}

	links := []*storage.TrendingLink{
		{URL: "https://example.com/a", Title: "a", Description: "a", UserID: "u1", CreatedAt: now},
		{URL: "https://example.com/a", Title: "a", Description: "a", UserID: "u2", CreatedAt: now},
		{URL: "https://example.com/a", Title: "a", Description: "a", UserID: "u3", CreatedAt: now},
		{URL: "https://example.com/b", Title: "b", Description: "b", UserID: "", CreatedAt: now},
	}

	repo := &fakeTrendingRepo{
		hashtags:         hashtags,
		statuses:         statuses,
		links:            links,
		deleteHashtagErr: errors.New("boom"),
		deleteLinkErr:    errors.New("boom"),
	}

	h := &TrendAggregatorHandler{
		db:           new(dynamormmocks.MockDB),
		trendingRepo: repo,
		logger:       zap.NewNop(),
	}

	// Exercise: aggregation helpers.
	_, err := h.aggregateHashtagTrends(context.Background(), now.Add(-1*time.Hour))
	require.NoError(t, err)

	_, err = h.aggregateStatusTrends(context.Background(), now.Add(-1*time.Hour))
	require.NoError(t, err)

	_, err = h.aggregateLinkTrends(context.Background(), now.Add(-1*time.Hour))
	require.NoError(t, err)

	// Exercise: scheduled handler path (includes cleanup).
	_, err = h.HandleScheduledEvent(&apptheory.EventContext{}, events.EventBridgeEvent{})
	require.NoError(t, err)

	require.GreaterOrEqual(t, repo.storedHashtags, 1)
	require.GreaterOrEqual(t, repo.storedStatuses, 1)
	require.GreaterOrEqual(t, repo.storedLinks, 1)
}

func TestTrendAggregator_Errors_Round12(t *testing.T) {
	h := &TrendAggregatorHandler{
		db:           new(dynamormmocks.MockDB),
		trendingRepo: &fakeTrendingRepo{getHashtagsErr: errors.New("db down")},
		logger:       zap.NewNop(),
	}

	_, err := h.aggregateHashtagTrends(context.Background(), time.Now())
	require.Error(t, err)
}

func TestRunTrendAggregator_Round12(t *testing.T) {
	origLambdaStart := lambdaStartFn
	t.Cleanup(func() { lambdaStartFn = origLambdaStart })

	lambdaStartCalled := false
	lambdaStartFn = func(handler any) {
		lambdaStartCalled = true

		fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := map[string]any{
			"id":          "event-id",
			"source":      "aws.events",
			"detail-type": "Scheduled Event",
			"detail":      map[string]any{},
			"time":        time.Now().Format(time.RFC3339),
			"resources": []any{
				"arn:aws:events:us-east-1:123456789012:rule/lesser-dev-trend-aggregator-schedule-0",
			},
		}
		raw, err := json.Marshal(event)
		require.NoError(t, err)
		_, err = fn(context.Background(), raw)
		require.NoError(t, err)
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	lambdaCtx = &common.LambdaContext{Logger: zap.NewNop()}
	handler = &TrendAggregatorHandler{logger: zap.NewNop(), trendingRepo: &fakeTrendingRepo{}}

	main()
	require.True(t, lambdaStartCalled)
}
