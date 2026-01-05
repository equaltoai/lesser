package cms

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type createFailRevisionRepo struct {
	listErr   error
	createErr error
}

func (r *createFailRevisionRepo) ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return []*models.Revision{}, nil
}

func (r *createFailRevisionRepo) GetRevision(ctx context.Context, objectID string, version int) (*models.Revision, error) {
	return nil, errors.New("not implemented")
}

func (r *createFailRevisionRepo) CreateRevision(ctx context.Context, revision *models.Revision) error {
	return r.createErr
}

func (r *createFailRevisionRepo) Delete(ctx context.Context, pk, sk string) error {
	return errors.New("not implemented")
}

func TestRevisionServiceBuildRevisionMetadata_NoFeaturedImage(t *testing.T) {
	t.Parallel()

	svc := &RevisionService{}
	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			Type:         activitypub.ArticleType,
			Name:         "Hello",
			Summary:      "Summary",
			AttributedTo: "https://example.com/users/alice",
		},
		FeaturedImage: nil,
	}

	raw, err := svc.buildRevisionMetadata(article)
	require.NoError(t, err)

	var meta articleRevisionMetadata
	require.NoError(t, json.Unmarshal([]byte(raw), &meta))
	require.Nil(t, meta.FeaturedImage)
}

func TestRevisionServiceApplyRevisionMetadataJSON_SetsNameAndFeaturedImage(t *testing.T) {
	t.Parallel()

	svc := &RevisionService{}
	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			Name:         "Before",
			AttributedTo: "https://example.com/users/alice",
		},
	}

	meta := articleRevisionMetadata{
		Name:         "After",
		AttributedTo: "https://example.com/users/bob",
		FeaturedImage: &articleRevisionMedia{
			ID:          "media-1",
			ContentType: "image/png",
			CDNURL:      "https://cdn.example.com/m.png",
			Description: "alt",
		},
	}
	metaJSON, err := json.Marshal(meta)
	require.NoError(t, err)

	svc.applyRevisionMetadataJSON(article, string(metaJSON))
	require.Equal(t, "After", article.Name)
	require.Equal(t, "https://example.com/users/bob", article.AttributedTo)
	require.NotNil(t, article.FeaturedImage)
	require.Equal(t, "media-1", article.FeaturedImage.MediaID)
	require.Equal(t, "image/png", article.FeaturedImage.ContentType)
	require.Equal(t, "https://cdn.example.com/m.png", article.FeaturedImage.CDNUrl)
	require.Equal(t, "alt", article.FeaturedImage.Description)
}

func TestRevisionServiceRecordPreRestoreBackupRevisionBestEffort_LogsWarnOnCreateFailure(t *testing.T) {
	t.Parallel()

	revisionRepo := &createFailRevisionRepo{createErr: errors.New("create failed")}
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

	svc.recordPreRestoreBackupRevisionBestEffort(context.Background(), article, 1)
}

func TestRevisionServiceTrimRevisionsBestEffort_ReturnsOnDeleteError(t *testing.T) {
	t.Parallel()

	revisionRepo := &retentionRevisionRepo{
		deleteErr: errors.New("delete failed"),
		listResult: [][]*models.Revision{
			{
				{ObjectID: "obj", Version: 2, SK: "SK#2"},
				{ObjectID: "obj", Version: 1, SK: "SK#1"},
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
}

func TestRevisionServiceDeleteRemovedCMSArticleIndexesBestEffort_ReturnsWhenWriterMissing(t *testing.T) {
	t.Parallel()

	svc := &RevisionService{
		articleIndexWriter: nil,
		logger:             zap.NewNop(),
	}

	before := &models.Article{Object: models.Object{ID: "a1", AttributedTo: "actor", Published: time.Now()}, CategoryIDs: []string{"cat-1"}}
	after := &models.Article{Object: models.Object{ID: "a1", AttributedTo: "actor", Published: time.Now()}, CategoryIDs: []string{}}

	svc.deleteRemovedCMSArticleIndexesBestEffort(context.Background(), before, after)
}
