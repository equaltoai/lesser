package cms

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPublicationService_GetPublication_Delegates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pubRepo := &fakePublicationRepo{
		publications: map[string]*models.Publication{
			"p1": {ID: "p1", Slug: "slug", Name: "name"},
		},
	}

	svc := NewPublicationService(pubRepo, &fakePublicationMemberRepo{members: map[string]*models.PublicationMember{}}, zap.NewNop())

	got, err := svc.GetPublication(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "p1", got.ID)

	_, err = svc.GetPublication(ctx, "missing")
	require.Error(t, err)
}

func TestRevisionService_WrapperMethods(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	revisionRepo := newMemRevisionRepo()

	svc := &RevisionService{
		revisionRepo:          revisionRepo,
		maxRevisionsPerObject: 0,
		logger:                zap.NewNop(),
	}

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello-world",
			AttributedTo: "https://example.com/users/alice",
			Content:      "hello",
			Name:         "hello",
		},
		Slug: "hello-world",
	}

	rev1, err := svc.CreateRevision(ctx, article)
	require.NoError(t, err)
	require.NotNil(t, rev1)
	assert.Equal(t, 1, rev1.Version)

	rev2, err := svc.CreateRevision(ctx, article)
	require.NoError(t, err)
	require.NotNil(t, rev2)
	assert.Equal(t, 2, rev2.Version)

	list, err := svc.ListRevisions(ctx, article.ID, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)

	got, err := svc.GetRevision(ctx, article.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, got.Version)
}

func TestNewRevisionService_NilReposDoesNotPanic(t *testing.T) {
	t.Parallel()

	svc := NewRevisionService(nil, nil, nil, nil, 7, zap.NewNop())
	require.NotNil(t, svc)
	assert.Equal(t, 7, svc.maxRevisionsPerObject)
	assert.Nil(t, svc.articleIndexWriter)
}

func TestDynamormCMSArticleIndexWriter_ReturnsErrorWhenDBNil(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	writer := dynamormCMSArticleIndexWriter{}

	err := writer.Create(ctx, &models.CMSArticleIndex{PK: "p", SK: "s"})
	require.Error(t, err)

	err = writer.Delete(ctx, &models.CMSArticleIndex{PK: "p", SK: "s"})
	require.Error(t, err)
}
