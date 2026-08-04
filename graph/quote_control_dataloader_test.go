package graph

import (
	"context"
	"errors"
	"fmt"
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
	keys       atomic.Int32
	quoteTypes map[string]string
}

func (r *quoteControlCountingRepository) GetQuoteTypes(_ context.Context, statusIDs []string) (map[string]string, error) {
	r.calls.Add(1)
	r.keys.Add(int32(len(statusIDs)))
	result := make(map[string]string, len(statusIDs))
	for _, statusID := range statusIDs {
		result[statusID] = r.quoteTypes[statusID]
		if result[statusID] == "" {
			result[statusID] = models.VisibilityPublic
		}
	}
	return result, nil
}

type quoteControlCountingStatusRepository struct {
	interfaces.StatusRepository
	calls atomic.Int32
	keys  atomic.Int32
}

func (r *quoteControlCountingStatusRepository) GetStatusesByIDs(ctx context.Context, statusIDs []string) ([]*models.Status, error) {
	r.calls.Add(1)
	r.keys.Add(int32(len(statusIDs)))
	return r.StatusRepository.GetStatusesByIDs(ctx, statusIDs)
}

type quoteControlCountingStorage struct {
	core.RepositoryStorage
	object interfaces.ObjectRepository
	status interfaces.StatusRepository
}

func (s quoteControlCountingStorage) Object() interfaces.ObjectRepository { return s.object }
func (s quoteControlCountingStorage) Status() interfaces.StatusRepository {
	if s.status != nil {
		return s.status
	}
	return s.RepositoryStorage.Status()
}

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
	var accountCalls atomic.Int32
	newRequestLoaders := func() *Loaders {
		loaders := NewLoaders(requestStorage, zap.NewNop())
		loaders.QuoteAccountLoader = newQuoteAccountLoaderWithLookup(
			func(_ context.Context, usernames []string) (map[string]*models.QuotePermissions, error) {
				accountCalls.Add(1)
				permissions := make(map[string]*models.QuotePermissions, len(usernames))
				for _, username := range usernames {
					permissions[username] = &models.QuotePermissions{
						Username: username, AllowPublic: true, AllowFollowers: true, AllowMentioned: true,
					}
				}
				return permissions, nil
			},
			zap.NewNop(),
		)
		return loaders
	}

	statuses := []*models.Status{
		{StatusID: "public"}, {StatusID: "followers"}, {StatusID: "mentioned"},
		{StatusID: "disabled"}, {StatusID: "missing"},
		{StatusID: "reply-child", InReplyToID: "reply-parent"},
		{StatusID: "boost-wrapper", ReblogOfID: "boost-original"},
	}
	for _, status := range statuses {
		status.AuthorUsername = "alice"
		status.Visibility = models.VisibilityPublic
	}
	ctx := WithLoaders(context.Background(), newRequestLoaders())
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
		quoteable, permission := resolver.determineQuoteable(ctx, &models.Status{
			StatusID: statusID, AuthorUsername: "alice", Visibility: models.VisibilityPublic,
		})
		require.Equal(t, want.quoteable, quoteable, statusID)
		require.Equal(t, want.permission, permission, statusID)
	}

	require.Equal(t, int32(1), objectRepo.calls.Load(), "all root and recursive quote controls must use one batch")
	require.Equal(t, int32(1), accountCalls.Load(), "all authors must use one account-permission batch")
	require.Equal(t, int64(2), tracker.GetOperationCounts()["Read"], "both batch reads must be cost-visible once")

	// NewLoaders is called once per GraphQL request by middleware. A second loader
	// set must not reuse the first request's permission cache.
	secondCtx := WithLoaders(context.Background(), newRequestLoaders())
	quoteable, permission := resolver.determineQuoteable(secondCtx, &models.Status{
		StatusID: "public", AuthorUsername: "alice", Visibility: models.VisibilityPublic,
	})
	require.True(t, quoteable)
	require.Equal(t, model.QuotePermissionEveryone, permission)
	require.Equal(t, int32(2), objectRepo.calls.Load())
	require.Equal(t, int32(2), accountCalls.Load())
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
	quoteable, permission := resolver.determineQuoteable(ctx, &models.Status{
		StatusID: "status-1", AuthorUsername: "alice", Visibility: models.VisibilityPublic,
	})
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
	resolver.prefetchQuoteControls(ctx, []*models.Status{{
		StatusID: "related-root", AuthorUsername: "alice", InReplyToID: "missing-parent",
	}})
	require.Equal(t, []string{"one", "parent", "boost"}, quoteControlPrefetchIDs([]*models.Status{
		nil,
		{},
		{StatusID: "one", InReplyToID: "parent", ReblogOfID: "boost"},
		{StatusID: "one", InReplyToID: "parent"},
	}))
	require.Equal(t, []string{"boost"}, quoteControlRelatedStatusIDs([]*models.Status{
		nil,
		{},
		{StatusID: "one", InReplyToID: "parent", ReblogOfID: "boost"},
		{StatusID: "parent", InReplyToID: "one", ReblogOfID: "boost"},
	}))

	// Keep the non-loader fallback pinned for non-GraphQL conversion callers.
	quoteable, permission = resolver.determineQuoteable(context.Background(), &models.Status{
		StatusID: "missing", AuthorUsername: "alice", Visibility: models.VisibilityPublic,
	})
	require.False(t, quoteable)
	require.Equal(t, model.QuotePermissionNone, permission)
	require.NotNil(t, storageRepo.Object())
}

func TestQuoteControlLoaderBatchesBoostOriginalAuthors(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	objectRepo := &quoteControlCountingRepository{
		ObjectRepository: storageRepo.Object(),
		quoteTypes:       make(map[string]string, 80),
	}
	statusRepo := &quoteControlCountingStatusRepository{StatusRepository: storageRepo.Status()}
	requestStorage := quoteControlCountingStorage{
		RepositoryStorage: storageRepo,
		object:            objectRepo,
		status:            statusRepo,
	}

	var accountCalls atomic.Int32
	var accountKeys atomic.Int32
	loaders := NewLoaders(requestStorage, zap.NewNop())
	loaders.QuoteAccountLoader = newQuoteAccountLoaderWithLookup(
		func(_ context.Context, usernames []string) (map[string]*models.QuotePermissions, error) {
			accountCalls.Add(1)
			accountKeys.Add(int32(len(usernames)))
			permissions := make(map[string]*models.QuotePermissions, len(usernames))
			for _, username := range usernames {
				permissions[username] = &models.QuotePermissions{Username: username, AllowPublic: true}
			}
			return permissions, nil
		},
		zap.NewNop(),
	)
	ctx := WithLoaders(context.Background(), loaders)

	const count = 40
	boosts := make([]*models.Status, 0, count)
	originals := make([]*models.Status, 0, count)
	for i := range count {
		boostID := fmt.Sprintf("boost-%02d", i)
		originalID := fmt.Sprintf("original-%02d", i)
		original := &models.Status{
			StatusID:       originalID,
			AuthorUsername: fmt.Sprintf("original-author-%02d", i),
			Visibility:     models.VisibilityPublic,
		}
		require.NoError(t, storageRepo.Status().CreateStatus(ctx, original))
		boosts = append(boosts, &models.Status{
			StatusID:       boostID,
			AuthorUsername: "boost-author",
			ReblogOfID:     originalID,
			Visibility:     models.VisibilityPublic,
		})
		originals = append(originals, original)
		objectRepo.quoteTypes[boostID] = models.VisibilityPublic
		objectRepo.quoteTypes[originalID] = models.VisibilityPublic
	}

	resolver.prefetchQuoteControls(ctx, boosts)
	for _, status := range append(boosts, originals...) {
		quoteable, permission := resolver.determineQuoteable(ctx, status)
		require.True(t, quoteable, status.StatusID)
		require.Equal(t, model.QuotePermissionEveryone, permission, status.StatusID)
	}

	require.Equal(t, int32(1), objectRepo.calls.Load(), "all root and original per-note controls must use one batch")
	require.Equal(t, int32(80), objectRepo.keys.Load())
	require.Equal(t, int32(1), accountCalls.Load(), "root and distinct original authors must use one account batch")
	require.Equal(t, int32(41), accountKeys.Load())
	require.Equal(t, int32(1), statusRepo.calls.Load(), "all boost originals must use one status batch")
	require.Equal(t, int32(40), statusRepo.keys.Load())
}

func TestThreadAncestorQuoteControlsPrefetchBeforeProjection(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	objectRepo := &quoteControlCountingRepository{
		ObjectRepository: storageRepo.Object(),
		quoteTypes: map[string]string{
			"ancestor-root":   models.VisibilityPublic,
			"ancestor-parent": models.VisibilityPublic,
		},
	}
	requestStorage := quoteControlCountingStorage{RepositoryStorage: storageRepo, object: objectRepo}
	var accountCalls atomic.Int32
	loaders := NewLoaders(requestStorage, zap.NewNop())
	loaders.QuoteAccountLoader = newQuoteAccountLoaderWithLookup(
		func(_ context.Context, usernames []string) (map[string]*models.QuotePermissions, error) {
			accountCalls.Add(1)
			permissions := make(map[string]*models.QuotePermissions, len(usernames))
			for _, username := range usernames {
				permissions[username] = &models.QuotePermissions{Username: username, AllowPublic: true}
			}
			return permissions, nil
		},
		zap.NewNop(),
	)
	ctx := WithLoaders(context.Background(), loaders)

	for _, status := range []*models.Status{
		{StatusID: "ancestor-root", AuthorUsername: "root-author", Visibility: models.VisibilityPublic},
		{
			StatusID: "ancestor-parent", AuthorUsername: "parent-author",
			InReplyToID: "ancestor-root", Visibility: models.VisibilityPublic,
		},
	} {
		require.NoError(t, storageRepo.Status().CreateStatus(ctx, status))
	}
	child := &models.Status{
		StatusID: "ancestor-child", AuthorUsername: "child-author",
		InReplyToID: "ancestor-parent", Visibility: models.VisibilityPublic,
	}

	ancestors := (&queryResolver{resolver}).buildThreadAncestors(ctx, storageRepo.Status(), child, "")
	require.Len(t, ancestors, 2)
	require.Equal(t, int32(1), objectRepo.calls.Load(), "all ancestors must use one per-note batch")
	require.Equal(t, int32(1), accountCalls.Load(), "all ancestor authors must use one account batch")
}
