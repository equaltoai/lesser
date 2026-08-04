package quotes

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeQuoteObjectRepo struct {
	quoteType string
	err       error
	calls     int
}

func (f *fakeQuoteObjectRepo) GetQuoteType(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.quoteType, f.err
}

func TestCheckQuotePermissionsEnforcesPerNoteControlAfterAccountControl(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	target := &models.Status{
		StatusID:       "target-1",
		AuthorID:       "https://example.com/users/bob",
		AuthorUsername: "bob",
		Visibility:     models.VisibilityPublic,
		Mentions:       []string{"https://example.com/users/alice"},
	}

	newService := func(account *models.QuotePermissions, noteType string, rel *quoteRelationshipStub) (*QuoteService, *fakeQuoteObjectRepo) {
		objectRepo := &fakeQuoteObjectRepo{quoteType: noteType}
		return &QuoteService{
			storage: fakeQuoteStorage{
				status:       &fakeQuoteStatusRepo{statuses: map[string]*models.Status{target.StatusID: target}},
				quote:        &fakeQuoteRepo{permissions: map[string]*models.QuotePermissions{"bob": account}},
				relationship: rel,
				object:       objectRepo,
			},
			logger: zap.NewNop(),
		}, objectRepo
	}

	t.Run("none denies even when account allows public", func(t *testing.T) {
		service, _ := newService(&models.QuotePermissions{Username: "bob", AllowPublic: true}, "disabled", nil)
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("followers allows follower and denies non-follower", func(t *testing.T) {
		service, _ := newService(&models.QuotePermissions{Username: "bob", AllowPublic: true}, "followers", &quoteRelationshipStub{following: true})
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.True(t, allowed)

		service, _ = newService(&models.QuotePermissions{Username: "bob", AllowPublic: true}, "followers", &quoteRelationshipStub{})
		allowed, err = service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("mentioned reuses persisted mention seam", func(t *testing.T) {
		service, _ := newService(&models.QuotePermissions{Username: "bob", AllowPublic: true}, "mentioned", nil)
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.True(t, allowed)

		service, _ = newService(&models.QuotePermissions{Username: "bob", AllowPublic: true}, "mentioned", nil)
		allowed, err = service.CheckQuotePermissions(ctx, "mallory", target)
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("per-note read error denies", func(t *testing.T) {
		service, objectRepo := newService(&models.QuotePermissions{Username: "bob", AllowPublic: true}, models.VisibilityPublic, nil)
		objectRepo.err = errors.New("status metadata unavailable")
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("per-note public cannot widen account denial", func(t *testing.T) {
		service, objectRepo := newService(&models.QuotePermissions{Username: "bob"}, models.VisibilityPublic, nil)
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, allowed)
		require.Zero(t, objectRepo.calls, "account denial must short-circuit before per-note lookup")
	})
}
