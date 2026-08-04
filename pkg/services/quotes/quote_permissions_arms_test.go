package quotes

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type quoteRelationshipStub struct {
	following bool
	err       error
	calls     int
	follower  string
	target    string
}

func (s *quoteRelationshipStub) IsFollowing(_ context.Context, followerID, followingID string) (bool, error) {
	s.calls++
	s.follower = followerID
	s.target = followingID
	return s.following, s.err
}

func TestCheckQuotePermissionsFollowerAndMentionedArms(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	target := &models.Status{
		StatusID:       "target-1",
		AuthorID:       "https://example.com/users/bob",
		AuthorUsername: "bob",
		Visibility:     models.VisibilityPublic,
		Mentions:       []string{"https://example.com/users/alice"},
	}

	newService := func(t *testing.T, permissions *models.QuotePermissions, status *models.Status, rel *quoteRelationshipStub) *QuoteService {
		t.Helper()
		return &QuoteService{
			storage: fakeQuoteStorage{
				status: &fakeQuoteStatusRepo{statuses: map[string]*models.Status{status.StatusID: status}},
				quote: &fakeQuoteRepo{permissions: map[string]*models.QuotePermissions{
					status.AuthorUsername: permissions,
				}},
				relationship: rel,
			},
			logger: zap.NewNop(),
		}
	}

	t.Run("follower is allowed by one exact relationship lookup", func(t *testing.T) {
		rel := &quoteRelationshipStub{following: true}
		service := newService(t, &models.QuotePermissions{Username: "bob", AllowFollowers: true}, target, rel)
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.True(t, allowed)
		require.Equal(t, 1, rel.calls)
		require.Equal(t, "alice", rel.follower)
		require.Equal(t, "bob", rel.target)
	})

	t.Run("non-follower is denied", func(t *testing.T) {
		service := newService(t, &models.QuotePermissions{Username: "bob", AllowFollowers: true}, target, &quoteRelationshipStub{})
		allowed, err := service.CheckQuotePermissions(ctx, "mallory", target)
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("relationship storage error denies rather than propagating permission", func(t *testing.T) {
		service := newService(t, &models.QuotePermissions{Username: "bob", AllowFollowers: true}, target, &quoteRelationshipStub{err: errors.New("relationship store unavailable")})
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("persisted local mention allows the mentioned quoter", func(t *testing.T) {
		service := newService(t, &models.QuotePermissions{Username: "bob", AllowMentioned: true}, target, nil)
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.True(t, allowed)
	})

	t.Run("unmentioned and same-path remote actors are denied", func(t *testing.T) {
		untrusted := *target
		untrusted.Mentions = []string{"https://remote.example/users/alice"}
		service := newService(t, &models.QuotePermissions{Username: "bob", AllowMentioned: true}, &untrusted, nil)
		allowed, err := service.CheckQuotePermissions(ctx, "alice", &untrusted)
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("persisted mention read error denies", func(t *testing.T) {
		statusRepo := &fakeQuoteStatusRepo{getErr: errors.New("status store unavailable")}
		service := &QuoteService{
			storage: fakeQuoteStorage{
				status: statusRepo,
				quote: &fakeQuoteRepo{permissions: map[string]*models.QuotePermissions{
					"bob": {Username: "bob", AllowMentioned: true},
				}},
			},
			logger: zap.NewNop(),
		}
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("persisted mention read error is the GraphQL forbidden class", func(t *testing.T) {
		statusRepo := &fakeQuoteStatusRepo{
			statuses: map[string]*models.Status{target.StatusID: target},
			getErr:   errors.New("status store unavailable"),
			getErrAt: 2,
		}
		service := &QuoteService{
			storage: fakeQuoteStorage{
				status: statusRepo,
				quote: &fakeQuoteRepo{permissions: map[string]*models.QuotePermissions{
					"bob": {Username: "bob", AllowMentioned: true},
				}},
			},
			logger: zap.NewNop(),
		}
		_, err := service.AttachQuoteToStatus(ctx, &models.Status{
			StatusID: "alice-quote", AuthorUsername: "alice",
		}, target.StatusID)
		require.ErrorIs(t, err, ErrNotAuthorizedToQuote)
		require.Equal(t, 2, statusRepo.getCalls)
	})

	t.Run("block list wins before all positive arms", func(t *testing.T) {
		rel := &quoteRelationshipStub{following: true}
		service := newService(t, &models.QuotePermissions{
			Username:       "bob",
			AllowPublic:    true,
			AllowFollowers: true,
			AllowMentioned: true,
			BlockList:      []string{"alice"},
		}, target, rel)
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.False(t, allowed)
		require.Zero(t, rel.calls)
	})

	t.Run("allow public short-circuits storage lookups", func(t *testing.T) {
		rel := &quoteRelationshipStub{err: errors.New("must not be reached")}
		service := newService(t, &models.QuotePermissions{
			Username:       "bob",
			AllowPublic:    true,
			AllowFollowers: true,
			AllowMentioned: true,
		}, target, rel)
		allowed, err := service.CheckQuotePermissions(ctx, "alice", target)
		require.NoError(t, err)
		require.True(t, allowed)
		require.Zero(t, rel.calls)
	})
}
