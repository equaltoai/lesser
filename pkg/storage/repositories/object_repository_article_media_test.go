package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// TestModelToActivityPubObject_ArticleComposesPersistedMedia proves the object
// read path (AP object fetch and REST status source) composes the persisted
// inline bindings into article content and attaches the minted servings as
// Document attachments.
func TestModelToActivityPubObject_ArticleComposesPersistedMedia(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2026, time.May, 20, 3, 10, 0, 0, time.UTC)
	position := 1

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Article")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "object#https://example.com/articles/media").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "object#https://example.com/articles/media").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Article")).Run(func(args mock.Arguments) {
		article := args.Get(0).(*models.Article)
		*article = models.Article{
			Object: models.Object{
				ID:           "https://example.com/articles/media",
				Type:         activitypub.ArticleType,
				Name:         "Media",
				Content:      "# T\n\nOne.\n\nTwo.",
				Published:    baseTime,
				Updated:      baseTime.Add(time.Hour),
				AttributedTo: "https://example.com/users/alice",
			},
			ContentFormat: "markdown",
			Slug:          "media",
			EditorialMedia: []models.ArticleEditorialMedia{
				{MediaID: "inline", Role: models.EditorialMediaRoleInline, InlinePosition: &position, URL: "https://cdn.example.test/inline.png", ContentType: "image/png", AltText: "inline"},
			},
		}
	}).Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	result, err := repo.modelToActivityPubObject(ctx, &models.Object{
		ID:           "https://example.com/articles/media",
		Type:         activitypub.ArticleType,
		Name:         "Fallback Object",
		Content:      "# T\n\nOne.\n\nTwo.",
		AttributedTo: "https://example.com/users/alice",
	})
	require.NoError(t, err)

	article, ok := result.(*activitypub.Article)
	require.True(t, ok, "expected *activitypub.Article, got %T", result)
	require.Contains(t, article.Content, `src="https://cdn.example.test/inline.png"`)
	require.Contains(t, article.Content, "<figure>")
	require.Len(t, article.Attachment, 1)
	require.Equal(t, "Document", article.Attachment[0].Type)
	require.Equal(t, "https://cdn.example.test/inline.png", article.Attachment[0].URL)
	require.Equal(t, "image/png", article.Attachment[0].MediaType)
	require.Equal(t, "inline", article.Attachment[0].Name)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
