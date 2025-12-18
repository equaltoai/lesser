package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
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
			Index("GSI1").
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