package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"go.uber.org/zap"
)

type fakeCategoryRepo struct {
	db dynamormcore.DB

	categories map[string]*models.Category

	createErr error
	updateErr error
	deleteErr error
}

func (f *fakeCategoryRepo) GetDB() dynamormcore.DB { return f.db }

func (f *fakeCategoryRepo) CreateCategory(_ context.Context, category *models.Category) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.categories == nil {
		f.categories = map[string]*models.Category{}
	}
	f.categories[category.ID] = category
	return nil
}

func (f *fakeCategoryRepo) GetCategory(_ context.Context, id string) (*models.Category, error) {
	if f.categories == nil {
		return nil, apperrors.ItemNotFoundWithID("category", id)
	}
	c, ok := f.categories[id]
	if !ok {
		return nil, apperrors.ItemNotFoundWithID("category", id)
	}
	return c, nil
}

func (f *fakeCategoryRepo) Update(_ context.Context, category *models.Category) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.categories == nil {
		f.categories = map[string]*models.Category{}
	}
	f.categories[category.ID] = category
	return nil
}

func (f *fakeCategoryRepo) Delete(_ context.Context, _ string, _ string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return nil
}

func (f *fakeCategoryRepo) ListCategories(_ context.Context, _ *string, _ int) ([]*models.Category, error) {
	out := make([]*models.Category, 0, len(f.categories))
	for _, c := range f.categories {
		out = append(out, c)
	}
	return out, nil
}

func TestCategoryService_Round25_CreateUpdateDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)

	repo := &fakeCategoryRepo{db: db, categories: map[string]*models.Category{}}
	svc := NewCategoryService(repo, zap.NewNop())

	t.Run("create validates", func(t *testing.T) {
		require.Error(t, svc.CreateCategory(ctx, nil))
		require.Error(t, svc.CreateCategory(ctx, &models.Category{ID: "c1"})) // missing slug
		require.Error(t, svc.CreateCategory(ctx, &models.Category{Slug: "slug"}))
	})

	t.Run("create validates parent", func(t *testing.T) {
		parentID := "missing-parent"
		err := svc.CreateCategory(ctx, &models.Category{
			ID:       "c2",
			Name:     "name",
			Slug:     "slug",
			ParentID: &parentID,
		})
		require.Error(t, err)

		repo.categories[parentID] = &models.Category{ID: parentID, Name: "p", Slug: "p"}
		q.On("Create").Return(nil).Once()
		err = svc.CreateCategory(ctx, &models.Category{
			ID:       "c3",
			Name:     "name",
			Slug:     "slug-3",
			ParentID: &parentID,
		})
		require.NoError(t, err)
	})

	t.Run("create blocks legacy slug collision", func(t *testing.T) {
		slug := "my-slug"
		legacy := common.GenerateObjectID("example.com", "categories", slug)
		repo.categories[legacy] = &models.Category{ID: legacy, Name: "legacy", Slug: slug}

		db, _ := newCMSMockDB(t)
		repo.db = db

		err := svc.CreateCategory(ctx, &models.Category{
			ID:   "https://example.com/categories/other",
			Name: "name",
			Slug: slug,
		})
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeAlreadyExists))
	})

	t.Run("create rolls back slug index on repository failure", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		repo.db = db

		repo.createErr = errors.New("create failed")
		q.On("Create").Return(nil).Once()
		q.On("Delete").Return(nil).Once()

		err := svc.CreateCategory(ctx, &models.Category{ID: "c4", Name: "name", Slug: "slug-4"})
		require.Error(t, err)
		repo.createErr = nil
	})

	t.Run("update validates hierarchy", func(t *testing.T) {
		self := "c5"
		err := svc.UpdateCategory(ctx, &models.Category{ID: "c5", Name: "name", Slug: "slug", ParentID: &self})
		require.Error(t, err)

		parentID := "parent"
		repo.categories["c6"] = &models.Category{ID: "c6", Name: "name", Slug: "slug"}
		err = svc.UpdateCategory(ctx, &models.Category{ID: "c6", Name: "name", Slug: "slug", ParentID: &parentID})
		require.Error(t, err)

		grandParent := "gp"
		repo.categories[parentID] = &models.Category{ID: parentID, Name: "p", Slug: "p", ParentID: &grandParent}
		repo.categories[grandParent] = &models.Category{ID: grandParent, Name: "gp", Slug: "gp", ParentID: &self}

		err = svc.UpdateCategory(ctx, &models.Category{ID: "c6", Name: "name", Slug: "slug", ParentID: &parentID})
		require.Error(t, err)
	})

	t.Run("update succeeds and reserves slug index", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		repo.db = db
		q.On("Create").Return(nil).Once()

		parentID := "parent-ok"
		repo.categories[parentID] = &models.Category{ID: parentID, Name: "p", Slug: "p"}

		start := time.Now()
		err := svc.UpdateCategory(ctx, &models.Category{ID: "c7", Name: "name", Slug: "slug-7", ParentID: &parentID})
		require.NoError(t, err)
		require.NotNil(t, repo.categories["c7"])
		assert.True(t, repo.categories["c7"].UpdatedAt.After(start))
	})

	t.Run("delete issues repository delete", func(t *testing.T) {
		err := svc.DeleteCategory(ctx, "c7")
		require.NoError(t, err)

		repo.deleteErr = dynamormerrors.ErrItemNotFound
		err = svc.DeleteCategory(ctx, "missing")
		require.Error(t, err)
		repo.deleteErr = nil
	})

	t.Run("list returns items", func(t *testing.T) {
		items, err := svc.ListCategories(ctx, nil, 10)
		require.NoError(t, err)
		require.NotNil(t, items)
	})
}
