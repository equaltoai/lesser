package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// CategoryRepository implements category operations
type CategoryRepository struct {
	*EnhancedBaseRepository[*models.Category]
}

// NewCategoryRepository creates a new category repository
func NewCategoryRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *CategoryRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.Category](db, tableName, logger, costService, "CategoryRepository", "category")

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &CategoryRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateCategory creates a new category
func (r *CategoryRepository) CreateCategory(ctx context.Context, category *models.Category) error {
	return r.ValidateAndCreate(ctx, category)
}

// GetCategory retrieves a category by ID
func (r *CategoryRepository) GetCategory(ctx context.Context, id string) (*models.Category, error) {
	var category models.Category
	pk := "INSTANCE#CATEGORY"
	sk := fmt.Sprintf("ID#%s", id)

	err := r.Get(ctx, pk, sk, &category)
	if err != nil {
		return nil, err
	}
	return &category, nil
}

// ListCategories lists all categories (optionally filtered by parent)
func (r *CategoryRepository) ListCategories(ctx context.Context, parentID *string, limit int) ([]*models.Category, error) {
	var categories []models.Category

	// If parentID is provided, use GSI1 to find children
	if parentID != nil {
		gsiPK := fmt.Sprintf("CATEGORY#%s", *parentID)
		if *parentID == "" {
			gsiPK = "CATEGORY#ROOT"
		}

		err := r.db.WithContext(ctx).Model(&models.Category{}).
			Index("gsi1").
			Where("gsi1PK", "=", gsiPK).
			Limit(limit).
			All(&categories)

		if err != nil {
			return nil, err
		}
	} else {
		// List all categories from main table
		err := r.db.WithContext(ctx).Model(&models.Category{}).
			Where("PK", "=", "INSTANCE#CATEGORY").
			Where("SK", "BEGINS_WITH", "ID#").
			Limit(limit).
			All(&categories)

		if err != nil {
			return nil, err
		}
	}

	result := make([]*models.Category, len(categories))
	for i := range categories {
		result[i] = &categories[i]
	}
	return result, nil
}

// UpdateArticleCount atomically increments/decrements a category's ArticleCount.
// Missing categories are treated as a no-op to avoid breaking writes when legacy/stale IDs exist.
func (r *CategoryRepository) UpdateArticleCount(ctx context.Context, categoryID string, delta int) error {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" || delta == 0 {
		return nil
	}

	pk := "INSTANCE#CATEGORY"
	sk := fmt.Sprintf("ID#%s", categoryID)

	err := r.GetDB().WithContext(ctx).Model(&models.Category{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder().
		Add("ArticleCount", delta).
		Condition("ArticleCount", ">=", -delta).
		Execute()
	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Warn("category not found for article count update", zap.String("category_id", categoryID))
			return nil
		}
		return err
	}

	return nil
}
