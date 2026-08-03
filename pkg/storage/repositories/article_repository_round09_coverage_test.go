package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type testArticleValidator struct {
	requiredErr error
	businessErr error
}

func (v testArticleValidator) ValidateModel(context.Context, BaseModel) error { return nil }
func (v testArticleValidator) ValidateRequiredFields(context.Context, BaseModel) error {
	return v.requiredErr
}
func (v testArticleValidator) ValidateBusinessRules(context.Context, BaseModel, string) error {
	return v.businessErr
}

type testArticleEvents struct{ seen []Event }

func (e *testArticleEvents) Emit(_ context.Context, event Event) error {
	e.seen = append(e.seen, event)
	return nil
}

func TestArticleRepository_round09_crud_and_listing(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewArticleRepository(mockDB, "tbl", zap.NewNop(), nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	t.Run("create_article_validation_errors", func(t *testing.T) {
		repo.SetValidationService(testArticleValidator{requiredErr: fmt.Errorf("missing")})
		err := repo.CreateArticle(ctx, &models.Article{Object: models.Object{ID: "a1"}})
		assert.Error(t, err)

		repo.SetValidationService(testArticleValidator{businessErr: fmt.Errorf("rules")})
		err = repo.CreateArticle(ctx, &models.Article{Object: models.Object{ID: "a1"}})
		assert.Error(t, err)
	})

	t.Run("create_get_update_delete_article", func(t *testing.T) {
		repo.SetValidationService(nil)

		mockQuery.On("IfNotExists").Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()
		ev := &testArticleEvents{}
		repo.SetEventService(ev)

		err := repo.CreateArticle(ctx, &models.Article{Object: models.Object{ID: "a1"}})
		require.NoError(t, err)
		assert.NotEmpty(t, ev.seen)

		mockQuery.On("Where", "PK", "=", "object#a1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "object#a1").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Article)
			dest.ID = "a1"
		}).Return(nil).Once()
		got, err := repo.GetArticle(ctx, "a1")
		require.NoError(t, err)
		assert.Equal(t, "a1", got.ID)

		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		err = repo.UpdateArticle(ctx, &models.Article{Object: models.Object{ID: "a1"}})
		require.NoError(t, err)

		err = repo.UpdateArticle(ctx, &models.Article{Object: models.Object{ID: ""}})
		assert.Error(t, err)

		mockQuery.On("Where", "PK", "=", "object#a1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "object#a1").Return(mockQuery).Once()
		mockQuery.On("Delete").Return(nil).Once()
		require.NoError(t, repo.DeleteArticle(ctx, "a1"))
	})

	t.Run("list_articles_paginated_and_by_index", func(t *testing.T) {
		// limit default branch + no next cursor (len <= limit)
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2PK", "=", "object#type#Article").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 26).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Article)
			*dest = []models.Article{{Object: models.Object{ID: "ad", GSI2SK: "t0"}}}
		}).Return(nil).Once()
		arts, cursor, err := repo.ListArticlesPaginated(ctx, 0, "")
		require.NoError(t, err)
		assert.Len(t, arts, 1)
		assert.Empty(t, cursor)

		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2PK", "=", "object#type#Article").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2SK", "<", "cur").Return(mockQuery).Once()
		mockQuery.On("Limit", 3).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Article)
			*dest = []models.Article{
				{Object: models.Object{ID: "a1", GSI2SK: "t3"}},
				{Object: models.Object{ID: "a2", GSI2SK: "t2"}},
				{Object: models.Object{ID: "a3", GSI2SK: "t1"}},
			}
		}).Return(nil).Once()

		arts, cursor, err = repo.ListArticlesPaginated(ctx, 2, "cur")
		require.NoError(t, err)
		assert.Len(t, arts, 2)
		assert.Equal(t, "t2", cursor)

		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2PK", "=", "object#type#Article").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Article)
			*dest = []models.Article{
				{Object: models.Object{ID: "a1", GSI2SK: "t1"}},
			}
		}).Return(nil).Once()

		arts, err = repo.ListArticles(ctx, 1)
		require.NoError(t, err)
		assert.NotEmpty(t, arts)

		// Empty author -> short-circuit branch
		arts, cursor, err = repo.ListArticlesByAuthorPaginated(ctx, "   ", 2, "")
		require.NoError(t, err)
		assert.Empty(t, arts)
		assert.Empty(t, cursor)

		// CMS index path: 2 IDs, with duplicate and blank.
		mockQuery.On("Where", "PK", "=", models.CMSArticleIndexPKForSeries("series-1")).Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", models.CMSArticleIndexSKPrefix).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 3).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.CMSArticleIndex)
			now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			*dest = []models.CMSArticleIndex{
				{ArticleID: "a1", SK: models.CMSArticleIndexSK(now, "a1")},
				{ArticleID: "", SK: models.CMSArticleIndexSK(now.Add(-time.Minute), "a2")},
				{ArticleID: "a1", SK: models.CMSArticleIndexSK(now.Add(-2*time.Minute), "a1")},
			}
		}).Return(nil).Once()

		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(1).(*[]*models.Article)
			*out = []*models.Article{
				{Object: models.Object{ID: "a2", PK: "object#a2", SK: "object#a2"}},
				{Object: models.Object{ID: "a1", PK: "object#a1", SK: "object#a1"}},
			}
		}).Return(nil).Once()

		arts, cursor, err = repo.ListArticlesBySeriesPaginated(ctx, "series-1", 2, "")
		require.NoError(t, err)
		assert.Len(t, arts, 2)
		assert.NotEmpty(t, cursor)

		// Author index success path.
		mockQuery.On("Where", "PK", "=", models.CMSArticleIndexPKForAuthor("actor-1")).Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", models.CMSArticleIndexSKPrefix).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.CMSArticleIndex)
			*dest = []models.CMSArticleIndex{{ArticleID: "a1", SK: models.CMSArticleIndexSK(time.Now(), "a1")}}
		}).Return(nil).Once()
		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(1).(*[]*models.Article)
			*out = []*models.Article{{Object: models.Object{ID: "a1"}}}
		}).Return(nil).Once()
		arts, _, err = repo.ListArticlesByAuthorPaginated(ctx, "actor-1", 1, "")
		require.NoError(t, err)
		assert.Len(t, arts, 1)

		// Category index: no article IDs extracted branch.
		mockQuery.On("Where", "PK", "=", models.CMSArticleIndexPKForCategory("cat-empty")).Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", models.CMSArticleIndexSKPrefix).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.CMSArticleIndex)
			*dest = []models.CMSArticleIndex{
				{ArticleID: "", SK: "TIME#no-marker"},
			}
		}).Return(nil).Once()
		arts, _, err = repo.ListArticlesByCategoryPaginated(ctx, "cat-empty", 1, "")
		require.NoError(t, err)
		assert.Empty(t, arts)

		// Not-found error from index query should bubble up.
		mockQuery.On("Where", "PK", "=", models.CMSArticleIndexPKForCategory("cat-1")).Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", models.CMSArticleIndexSKPrefix).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 26).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, _, err = repo.ListArticlesByCategoryPaginated(ctx, "cat-1", 25, "")
		assert.Error(t, err)

		// Reference to avoid unused import edge cases if errors are wrapped.
		_ = appErrors.CodeInternal
	})

	t.Run("batch_get_articles_ordered_edge_cases", func(t *testing.T) {
		arts, err := repo.batchGetArticlesOrdered(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, arts)

		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(fmt.Errorf("boom")).Once()
		_, err = repo.batchGetArticlesOrdered(ctx, []string{"a1"})
		assert.Error(t, err)
	})
}
