package quotes

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestQuoteService_CheckQuotePermissions_CoversBranches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	target := &models.Status{StatusID: "t1", AuthorUsername: "bob", Visibility: "public"}

	t.Run("blocked returns false", func(t *testing.T) {
		quoteRepo := &fakeQuoteRepo{
			permissions: map[string]*models.QuotePermissions{
				"bob": {Username: "bob", AllowPublic: true, BlockList: []string{"alice"}},
			},
		}
		qs := &QuoteService{storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo}, logger: zap.NewNop()}
		ok, err := qs.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("allow public returns true", func(t *testing.T) {
		quoteRepo := &fakeQuoteRepo{
			permissions: map[string]*models.QuotePermissions{
				"bob": {Username: "bob", AllowPublic: true},
			},
		}
		qs := &QuoteService{storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo}, logger: zap.NewNop()}
		ok, err := qs.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("allow followers calls follow check and still returns false", func(t *testing.T) {
		quoteRepo := &fakeQuoteRepo{
			permissions: map[string]*models.QuotePermissions{
				"bob": {Username: "bob", AllowPublic: false, AllowFollowers: true},
			},
		}
		qs := &QuoteService{storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo}, logger: zap.NewNop()}
		ok, err := qs.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("allow mentioned calls mention check and still returns false", func(t *testing.T) {
		quoteRepo := &fakeQuoteRepo{
			permissions: map[string]*models.QuotePermissions{
				"bob": {Username: "bob", AllowPublic: false, AllowMentioned: true},
			},
		}
		qs := &QuoteService{storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo}, logger: zap.NewNop()}
		ok, err := qs.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestQuoteService_CreateQuotePost_CoversMoreErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("GetStatus error returns expected error", func(t *testing.T) {
		statusRepo := &fakeQuoteStatusRepo{getErr: errors.New("boom")}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: statusRepo, quote: &fakeQuoteRepo{}},
			logger:  zap.NewNop(),
		}
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "alice", TargetStatusID: "t1", Visibility: "public"})
		require.Error(t, err)
	})

	t.Run("permissions lookup error returns expected error", func(t *testing.T) {
		statusRepo := &fakeQuoteStatusRepo{
			statuses: map[string]*models.Status{
				"t1": {StatusID: "t1", AuthorUsername: "bob", Visibility: "public"},
			},
		}
		quoteRepo := &fakeQuoteRepo{getPermissionsErr: errors.New("boom")}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
			logger:  zap.NewNop(),
		}
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "alice", TargetStatusID: "t1", Visibility: "public"})
		require.Error(t, err)
	})

	t.Run("create status failures return expected error", func(t *testing.T) {
		statusRepo := &fakeQuoteStatusRepo{
			statuses: map[string]*models.Status{
				"t1": {StatusID: "t1", AuthorUsername: "bob", Visibility: "public"},
			},
			createErr: errors.New("create failed"),
		}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: statusRepo, quote: &fakeQuoteRepo{}},
			logger:  zap.NewNop(),
		}
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "alice", TargetStatusID: "t1", Visibility: "public"})
		require.ErrorIs(t, err, ErrCreateQuoteStatus)
	})

	t.Run("create relationship failures return expected error", func(t *testing.T) {
		statusRepo := &fakeQuoteStatusRepo{
			statuses: map[string]*models.Status{
				"t1": {StatusID: "t1", AuthorUsername: "bob", Visibility: "public"},
			},
		}
		quoteRepo := &fakeQuoteRepo{createRelErr: errors.New("create rel failed")}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
			logger:  zap.NewNop(),
		}
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "alice", TargetStatusID: "t1", Visibility: "public"})
		require.ErrorIs(t, err, ErrCreateQuoteRelationship)
	})
}

func TestQuoteService_WithdrawFromQuotes_TargetStatusNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: &fakeQuoteRepo{}},
		logger:  zap.NewNop(),
	}

	_, _, err := qs.WithdrawFromQuotes(ctx, "missing", "user")
	require.ErrorIs(t, err, ErrTargetStatusNotFound)
}

func TestQuoteService_getQuotePermissions_NonNotFoundErrorsBubble(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	quoteRepo := &fakeQuoteRepo{getPermissionsErr: errors.New("boom")}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	_, err := qs.getQuotePermissions(ctx, "alice")
	require.Error(t, err)
	require.False(t, errors.Is(err, storage.ErrNotFound))
}
