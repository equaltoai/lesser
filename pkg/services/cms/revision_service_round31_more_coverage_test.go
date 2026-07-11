package cms

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"go.uber.org/zap"
)

type fakeClaims struct {
	username string
}

func (c fakeClaims) HasScope(string) bool { return false }
func (c fakeClaims) GetUsername() string  { return c.username }

type erroringRevisionRepo struct {
	*memRevisionRepo
	createErr error
}

func (r *erroringRevisionRepo) CreateRevision(ctx context.Context, revision *models.Revision) error {
	if r.createErr != nil {
		return r.createErr
	}
	return r.memRevisionRepo.CreateRevision(ctx, revision)
}

type retentionRevisionRepo struct {
	listErr    error
	deleteErr  error
	listResult [][]*models.Revision

	listCalls   int
	deleteCalls []struct {
		pk string
		sk string
	}
}

func (r *retentionRevisionRepo) ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if r.listCalls >= len(r.listResult) {
		return []*models.Revision{}, nil
	}
	out := r.listResult[r.listCalls]
	r.listCalls++
	return out, nil
}

func (r *retentionRevisionRepo) GetRevision(ctx context.Context, objectID string, version int) (*models.Revision, error) {
	return nil, errors.New("not implemented")
}

func (r *retentionRevisionRepo) CreateRevision(ctx context.Context, revision *models.Revision) error {
	return errors.New("not implemented")
}

func (r *retentionRevisionRepo) Delete(ctx context.Context, pk, sk string) error {
	r.deleteCalls = append(r.deleteCalls, struct {
		pk string
		sk string
	}{pk: pk, sk: sk})
	return r.deleteErr
}

type erroringIndexWriter struct {
	createErr error
	deleteErr error
}

func (w *erroringIndexWriter) Create(ctx context.Context, entry *models.CMSArticleIndex) error {
	return w.createErr
}

func (w *erroringIndexWriter) Delete(ctx context.Context, entry *models.CMSArticleIndex) error {
	return w.deleteErr
}

func TestRevisionServiceCreateRevision_NilArticleReturnsError(t *testing.T) {
	t.Parallel()

	svc := &RevisionService{logger: zap.NewNop()}

	_, err := svc.createRevision(context.Background(), nil, revisionChangeTypeUpdate, "")
	require.Error(t, err)
}

func TestRevisionServiceCreateRevision_UsesClaimsUsernameAndNormalizesChangeType(t *testing.T) {
	t.Parallel()

	revisionRepo := newMemRevisionRepo()
	svc := &RevisionService{
		revisionRepo:          revisionRepo,
		maxRevisionsPerObject: 0,
		logger:                zap.NewNop(),
	}

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			Type:         activitypub.ArticleType,
			AttributedTo: "https://example.com/users/alice",
			Content:      "# Hello\n\ncontent",
		},
		ContentFormat: "markdown",
	}

	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, fakeClaims{username: "viewer"})
	rev, err := svc.createRevision(ctx, article, "RESTORE", "  summary  ")
	require.NoError(t, err)
	require.NotNil(t, rev)
	require.Equal(t, 1, rev.Version)
	require.Equal(t, "restore", rev.ChangeType)
	require.Equal(t, "summary", rev.ChangeSummary)
	require.Equal(t, "viewer", rev.ChangedBy)
	require.Contains(t, rev.ID, "#00000001")
	require.False(t, rev.CreatedAt.IsZero())
}

func TestRevisionServiceCreateRevision_IncrementsVersionFromExisting(t *testing.T) {
	t.Parallel()

	revisionRepo := newMemRevisionRepo()
	revisionRepo.byObject["https://example.com/articles/hello"] = map[int]*models.Revision{
		7: {ObjectID: "https://example.com/articles/hello", Version: 7},
	}

	svc := &RevisionService{
		revisionRepo:          revisionRepo,
		maxRevisionsPerObject: 0,
		logger:                zap.NewNop(),
	}

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			Type:         activitypub.ArticleType,
			AttributedTo: "https://example.com/users/alice",
			Content:      "content",
		},
	}

	rev, err := svc.createRevision(context.Background(), article, revisionChangeTypeUpdate, "")
	require.NoError(t, err)
	require.NotNil(t, rev)
	require.Equal(t, 8, rev.Version)
}

func TestRevisionServiceCreateRevision_ReturnsErrorWhenRepositoryCreateFails(t *testing.T) {
	t.Parallel()

	base := newMemRevisionRepo()
	revisionRepo := &erroringRevisionRepo{
		memRevisionRepo: base,
		createErr:       errors.New("create failed"),
	}

	svc := &RevisionService{
		revisionRepo:          revisionRepo,
		maxRevisionsPerObject: 0,
		logger:                zap.NewNop(),
	}

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			Type:         activitypub.ArticleType,
			AttributedTo: "https://example.com/users/alice",
			Content:      "content",
		},
	}

	_, err := svc.createRevision(context.Background(), article, revisionChangeTypeUpdate, "")
	require.Error(t, err)
}

func TestRevisionServiceTrimRevisionsBestEffort_DeletesOldestUntilWithinRetention(t *testing.T) {
	t.Parallel()

	revisionRepo := &retentionRevisionRepo{
		listResult: [][]*models.Revision{
			{
				{ObjectID: "obj", Version: 2, SK: "SK#2"},
				{ObjectID: "obj", Version: 1, SK: "SK#1"},
			},
			{
				{ObjectID: "obj", Version: 2, SK: "SK#2"},
			},
		},
	}

	svc := &RevisionService{
		revisionRepo:          revisionRepo,
		maxRevisionsPerObject: 1,
		logger:                zap.NewNop(),
	}

	svc.trimRevisionsBestEffort(context.Background(), "obj")
	require.Len(t, revisionRepo.deleteCalls, 1)
	require.Equal(t, "SK#1", revisionRepo.deleteCalls[0].sk)
}

func TestRevisionServiceTrimRevisionsBestEffort_ReturnsOnListError(t *testing.T) {
	t.Parallel()

	revisionRepo := &retentionRevisionRepo{
		listErr: errors.New("list failed"),
	}

	svc := &RevisionService{
		revisionRepo:          revisionRepo,
		maxRevisionsPerObject: 1,
		logger:                zap.NewNop(),
	}

	svc.trimRevisionsBestEffort(context.Background(), "obj")
	require.Empty(t, revisionRepo.deleteCalls)
}

func TestRevisionServiceTrimRevisionsBestEffort_ReturnsWhenOldestNil(t *testing.T) {
	t.Parallel()

	revisionRepo := &retentionRevisionRepo{
		listResult: [][]*models.Revision{
			{
				{ObjectID: "obj", Version: 2, SK: "SK#2"},
				nil,
			},
		},
	}

	svc := &RevisionService{
		revisionRepo:          revisionRepo,
		maxRevisionsPerObject: 1,
		logger:                zap.NewNop(),
	}

	svc.trimRevisionsBestEffort(context.Background(), "obj")
	require.Empty(t, revisionRepo.deleteCalls)
}

func TestRevisionServiceApplyRevisionMetadataJSON_CoversEdgeCases(t *testing.T) {
	t.Parallel()

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			Name:         "Before",
			AttributedTo: "https://example.com/users/alice",
		},
		FeaturedImage: &models.Media{MediaID: "keep"},
	}

	svc := &RevisionService{}

	svc.applyRevisionMetadataJSON(article, "")
	require.Equal(t, "Before", article.Name)

	svc.applyRevisionMetadataJSON(article, "{not-json")
	require.Equal(t, "Before", article.Name)

	meta := articleRevisionMetadata{
		Name:          "",
		Summary:       "Updated summary",
		AttributedTo:  "",
		FeaturedImage: &articleRevisionMedia{ID: ""},
	}
	metaJSON, err := json.Marshal(meta)
	require.NoError(t, err)

	svc.applyRevisionMetadataJSON(article, string(metaJSON))
	require.Equal(t, "Before", article.Name)
	require.Equal(t, "Updated summary", article.Summary)
	require.Equal(t, "https://example.com/users/alice", article.AttributedTo)
	require.Nil(t, article.FeaturedImage)
}

func TestRevisionServiceUpsertCMSArticleIndexes_ReturnsErrorWhenWriterMissing(t *testing.T) {
	t.Parallel()

	svc := &RevisionService{
		articleIndexWriter: nil,
		logger:             zap.NewNop(),
	}

	err := svc.upsertCMSArticleIndexes(context.Background(), &models.Article{Object: models.Object{ID: "https://example.com/articles/hello"}})
	require.Error(t, err)
}

func TestRevisionServiceUpsertCMSArticleIndexes_PropagatesWriterCreateError(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	seriesID := "alice|series-1"
	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    now,
		},
		SeriesID:    &seriesID,
		CategoryIDs: []string{"cat-1"},
	}

	svc := &RevisionService{
		articleIndexWriter: &erroringIndexWriter{createErr: errors.New("create failed")},
		logger:             zap.NewNop(),
	}

	err := svc.upsertCMSArticleIndexes(context.Background(), article)
	require.Error(t, err)
}

func TestRevisionServiceDeleteRemovedCMSArticleIndexesBestEffort_IgnoresNotFound(t *testing.T) {
	t.Parallel()

	published := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	before := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    published,
		},
		CategoryIDs: []string{"cat-1"},
	}
	after := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    published,
		},
		CategoryIDs: []string{},
	}

	svc := &RevisionService{
		articleIndexWriter: &erroringIndexWriter{deleteErr: dynamormerrors.ErrItemNotFound},
		logger:             zap.NewNop(),
	}
	svc.deleteRemovedCMSArticleIndexesBestEffort(context.Background(), before, after)

	svc.articleIndexWriter = &erroringIndexWriter{deleteErr: errors.New("delete failed")}
	svc.deleteRemovedCMSArticleIndexesBestEffort(context.Background(), before, after)
}
