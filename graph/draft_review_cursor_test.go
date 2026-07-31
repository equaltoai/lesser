package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

type cursorRecordingDraftRepository struct {
	*inmemory.DraftRepository
	myDraftsCursor           string
	sharedDraftReviewsCursor string
}

func (r *cursorRecordingDraftRepository) ListDraftsByAuthorPaginated(_ context.Context, _ string, _ int, cursor string) ([]*models.Draft, string, error) {
	r.myDraftsCursor = cursor
	return nil, "", nil
}

func (*cursorRecordingDraftRepository) CreateDraftReviewGrant(context.Context, *models.DraftReviewGrant) error {
	return nil
}

func (*cursorRecordingDraftRepository) RegrantDraftReviewGrant(context.Context, *models.DraftReviewGrant) error {
	return nil
}

func (*cursorRecordingDraftRepository) RevokeDraftReviewGrant(context.Context, *models.DraftReviewGrant) error {
	return nil
}

func (*cursorRecordingDraftRepository) GetDraftReviewGrant(context.Context, string, string, string) (*models.DraftReviewGrant, error) {
	return nil, nil
}

func (r *cursorRecordingDraftRepository) ListActiveDraftReviewGrants(_ context.Context, _ string, _ int, cursor string) ([]*models.DraftReviewGrant, string, error) {
	r.sharedDraftReviewsCursor = cursor
	return nil, "", nil
}

func (*cursorRecordingDraftRepository) CountActiveDraftReviewGrants(context.Context, string) (int, error) {
	return 0, nil
}

func (*cursorRecordingDraftRepository) ListDraftReviewGrants(context.Context, string, string) ([]*models.DraftReviewGrant, error) {
	return nil, nil
}

func (*cursorRecordingDraftRepository) CreateDraftReviewVerdict(context.Context, *models.DraftReviewVerdict) error {
	return nil
}

func (*cursorRecordingDraftRepository) ListDraftReviewVerdicts(context.Context, string, string) ([]*models.DraftReviewVerdict, error) {
	return nil, nil
}

type cursorResolverStorage struct {
	core.RepositoryStorage
	draft interfaces.DraftRepository
}

func (s *cursorResolverStorage) Draft() interfaces.DraftRepository {
	return s.draft
}

func newDraftReviewCursorResolver(t *testing.T) (*Resolver, *cursorRecordingDraftRepository) {
	t.Helper()

	base, storage := newRound12GraphResolver(t)
	drafts := &cursorRecordingDraftRepository{DraftRepository: inmemory.NewDraftRepository()}
	wrapped := &cursorResolverStorage{RepositoryStorage: storage, draft: drafts}
	registry, err := services.NewRegistry(
		services.WithStorage(wrapped),
		services.WithPublisher(base.Registry.GetPublisher()),
		services.WithLogger(base.Registry.GetLogger()),
		services.WithConfig(base.Registry.GetConfig()),
	)
	require.NoError(t, err)

	return &Resolver{
		Registry: registry,
		Config:   base.Config,
		Storage:  wrapped,
		Logger:   base.Logger,
	}, drafts
}

func TestDraftReviewResolversTreatWhitespaceCursorAsAbsent(t *testing.T) {
	t.Run("MyDrafts", func(t *testing.T) {
		resolver, drafts := newDraftReviewCursorResolver(t)
		after := model.Cursor(" \t\n ")

		result, err := resolver.Query().MyDrafts(round12AuthContext("reviewer"), nil, nil, nil, &after)
		require.NoError(t, err)
		require.NotNil(t, result.PageInfo)
		require.False(t, result.PageInfo.HasPreviousPage)
		require.Empty(t, drafts.myDraftsCursor, "MyDrafts must pass the trimmed cursor downstream")
	})

	t.Run("SharedDraftReviews", func(t *testing.T) {
		resolver, drafts := newDraftReviewCursorResolver(t)
		after := model.Cursor(" \t\n ")

		result, err := resolver.Query().SharedDraftReviews(round12AuthContext("reviewer"), nil, &after)
		require.NoError(t, err)
		require.NotNil(t, result.PageInfo)
		require.False(t, result.PageInfo.HasPreviousPage)
		require.Empty(t, drafts.sharedDraftReviewsCursor, "SharedDraftReviews must pass the trimmed cursor downstream")
	})
}
