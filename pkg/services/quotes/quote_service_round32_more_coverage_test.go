package quotes

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestQuoteService_AttachQuoteToStatus_Succeeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{
		statuses: map[string]*models.Status{
			"t1": {StatusID: "t1", AuthorUsername: "bob", Visibility: "public"},
		},
	}
	quoteRepo := &fakeQuoteRepo{}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	quoteStatus := &models.Status{StatusID: "q1", AuthorUsername: "alice", Visibility: "public"}
	out, err := qs.AttachQuoteToStatus(ctx, quoteStatus, "t1")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.QuoteRelationship)
	require.Equal(t, "q1", out.QuoteRelationship.QuoterNoteID)
	require.Equal(t, "t1", out.QuoteRelationship.TargetNoteID)
	require.Equal(t, "t1", quoteStatus.QuoteTargetStatusID)
}

func TestQuoteService_AttachQuoteToStatus_RepoErrorAndBusinessRuleFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("repo error returns get target status error", func(t *testing.T) {
		statusRepo := &statusRepoWithErrors{getErr: map[string]error{"t1": errors.New("boom")}}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: statusRepo, quote: &fakeQuoteRepo{}},
			logger:  zap.NewNop(),
		}
		_, err := qs.AttachQuoteToStatus(ctx, &models.Status{StatusID: "q1", AuthorUsername: "alice"}, "t1")
		require.Error(t, err)
	})

	t.Run("private target is not quotable", func(t *testing.T) {
		statusRepo := &fakeQuoteStatusRepo{
			statuses: map[string]*models.Status{
				"t1": {StatusID: "t1", AuthorUsername: "bob", Visibility: "private"},
			},
		}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: statusRepo, quote: &fakeQuoteRepo{}},
			logger:  zap.NewNop(),
		}
		_, err := qs.AttachQuoteToStatus(ctx, &models.Status{StatusID: "q1", AuthorUsername: "alice"}, "t1")
		require.ErrorIs(t, err, ErrTargetStatusNotQuotable)
	})

	t.Run("blocked user cannot quote", func(t *testing.T) {
		statusRepo := &fakeQuoteStatusRepo{
			statuses: map[string]*models.Status{
				"t1": {StatusID: "t1", AuthorUsername: "bob", Visibility: "public"},
			},
		}
		quoteRepo := &fakeQuoteRepo{
			permissions: map[string]*models.QuotePermissions{
				"bob": {
					Username:       "bob",
					AllowPublic:    false,
					AllowFollowers: false,
					AllowMentioned: false,
					BlockList:      []string{"alice"},
				},
			},
		}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
			logger:  zap.NewNop(),
		}
		_, err := qs.AttachQuoteToStatus(ctx, &models.Status{StatusID: "q1", AuthorUsername: "alice"}, "t1")
		require.ErrorIs(t, err, ErrNotAuthorizedToQuote)
	})
}

func TestQuoteService_DeleteQuotePost_RepoErrorAndSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("repo error returns wrapped error", func(t *testing.T) {
		quoteRepo := &fakeQuoteRepo{getRelErr: errors.New("boom")}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
			logger:  zap.NewNop(),
		}
		require.Error(t, qs.DeleteQuotePost(ctx, "q1", "t1", "alice"))
	})

	t.Run("success withdraws relationship", func(t *testing.T) {
		quoteRepo := &fakeQuoteRepo{
			relationships: map[string]*models.QuoteRelationship{
				quoteRelKey("q1", "t1"): {
					QuoterNoteID: "q1",
					TargetNoteID: "t1",
					QuoterID:     "alice",
				},
			},
		}
		qs := &QuoteService{
			storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
			logger:  zap.NewNop(),
		}

		require.NoError(t, qs.DeleteQuotePost(ctx, "q1", "t1", "alice"))
		rel := quoteRepo.relationships[quoteRelKey("q1", "t1")]
		require.NotNil(t, rel)
		require.True(t, rel.Withdrawn)
	})
}

func TestQuoteErrors_ConstructorsCovered(t *testing.T) {
	t.Parallel()

	require.NotNil(t, ErrCheckQuotePermissions(errors.New("boom")))
	require.NotNil(t, ErrGetQuoteRelationship(errors.New("boom")))
}
