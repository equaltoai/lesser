package graph

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type quoteControlCountingRepository struct {
	interfaces.ObjectRepository
	calls      atomic.Int32
	quoteTypes map[string]string
}

func (r *quoteControlCountingRepository) GetQuoteTypes(_ context.Context, statusIDs []string) (map[string]string, error) {
	r.calls.Add(1)
	result := make(map[string]string, len(statusIDs))
	for _, statusID := range statusIDs {
		result[statusID] = r.quoteTypes[statusID]
		if result[statusID] == "" {
			result[statusID] = models.VisibilityPublic
		}
	}
	return result, nil
}

type quoteControlCountingStorage struct {
	core.RepositoryStorage
	object interfaces.ObjectRepository
}

func (s quoteControlCountingStorage) Object() interfaces.ObjectRepository { return s.object }

func TestQuoteControlLoaderBatchesRequestProjectionAndTracksCost(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	objectRepo := &quoteControlCountingRepository{
		ObjectRepository: storageRepo.Object(),
		quoteTypes: map[string]string{
			"public": models.VisibilityPublic, "followers": EventTypeFollowers,
			"mentioned": "mentioned", "disabled": "disabled",
			"reply-parent": EventTypeFollowers, "boost-original": "disabled",
		},
	}
	requestStorage := quoteControlCountingStorage{RepositoryStorage: storageRepo, object: objectRepo}
	tracker := cost.NewUnifiedTracker(nil, zap.NewNop(), "alice", "quote-control-request")
	t.Cleanup(func() { require.NoError(t, tracker.Close(context.Background())) })
	resolver.UnifiedTracker = tracker
	resolver.TableName = "lesser-test"

	statuses := []*models.Status{
		{StatusID: "public"}, {StatusID: "followers"}, {StatusID: "mentioned"},
		{StatusID: "disabled"}, {StatusID: "missing"},
		{StatusID: "reply-child", InReplyToID: "reply-parent"},
		{StatusID: "boost-wrapper", ReblogOfID: "boost-original"},
	}
	ctx := WithLoaders(context.Background(), NewLoaders(requestStorage, zap.NewNop()))
	resolver.prefetchQuoteControls(ctx, statuses)

	expected := map[string]struct {
		quoteable  bool
		permission model.QuotePermission
	}{
		"public":         {true, model.QuotePermissionEveryone},
		"followers":      {true, model.QuotePermissionFollowers},
		"mentioned":      {true, model.QuotePermissionMentioned},
		"disabled":       {false, model.QuotePermissionNone},
		"missing":        {true, model.QuotePermissionEveryone},
		"reply-parent":   {true, model.QuotePermissionFollowers},
		"boost-original": {false, model.QuotePermissionNone},
	}
	for statusID, want := range expected {
		quoteable, permission := resolver.determineQuoteable(ctx, &models.Status{StatusID: statusID})
		require.Equal(t, want.quoteable, quoteable, statusID)
		require.Equal(t, want.permission, permission, statusID)
	}

	require.Equal(t, int32(1), objectRepo.calls.Load(), "all root and recursive quote controls must use one batch")
	require.Equal(t, int64(1), tracker.GetOperationCounts()["Read"], "the batch read must be cost-visible once")

	// NewLoaders is called once per GraphQL request by middleware. A second loader
	// set must not reuse the first request's permission cache.
	secondCtx := WithLoaders(context.Background(), NewLoaders(requestStorage, zap.NewNop()))
	quoteable, permission := resolver.determineQuoteable(secondCtx, &models.Status{StatusID: "public"})
	require.True(t, quoteable)
	require.Equal(t, model.QuotePermissionEveryone, permission)
	require.Equal(t, int32(2), objectRepo.calls.Load())
}

func TestQuoteControlLoaderFailureAndPrefetchEdges(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	batchErr := errors.New("quote metadata unavailable")
	ctx := WithLoaders(context.Background(), &Loaders{
		QuoteControlLoader: newQuoteControlLoaderWithLookup(
			func(context.Context, []string) (map[string]string, error) { return nil, batchErr },
			zap.NewNop(),
		),
	})
	quoteable, permission := resolver.determineQuoteable(ctx, &models.Status{StatusID: "status-1"})
	require.False(t, quoteable)
	require.Equal(t, model.QuotePermissionNone, permission)

	missingLoaderCtx := WithLoaders(context.Background(), &Loaders{
		QuoteControlLoader: newQuoteControlLoaderWithLookup(nil, zap.NewNop()),
	})
	_, err := loadQuoteControl(missingLoaderCtx, "status-1")
	require.ErrorIs(t, err, errQuoteControlLoaderUnavailable)
	_, errs := loadQuoteControls(context.Background(), []string{"status-1"})
	require.ErrorIs(t, errs[0], errQuoteControlLoaderUnavailable)

	resolver.prefetchQuoteControls(context.Background(), []*models.Status{{StatusID: "ignored"}})
	resolver.prefetchQuoteControls(ctx, nil)
	require.Equal(t, []string{"one", "parent", "boost"}, quoteControlPrefetchIDs([]*models.Status{
		nil,
		{},
		{StatusID: "one", InReplyToID: "parent", ReblogOfID: "boost"},
		{StatusID: "one", InReplyToID: "parent"},
	}))

	// Keep the non-loader fallback pinned for non-GraphQL conversion callers.
	quoteable, permission = resolver.determineQuoteable(context.Background(), &models.Status{StatusID: "missing"})
	require.False(t, quoteable)
	require.Equal(t, model.QuotePermissionNone, permission)
	require.NotNil(t, storageRepo.Object())
}
