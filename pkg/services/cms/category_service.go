package cms

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// CategoryService handles business logic for categories
type CategoryService struct {
	categoryRepo *repositories.CategoryRepository
	logger       *zap.Logger
}

// NewCategoryService creates a new CategoryService
func NewCategoryService(categoryRepo *repositories.CategoryRepository, logger *zap.Logger) *CategoryService {
	return &CategoryService{
		categoryRepo: categoryRepo,
		logger:       logger,
	}
}

// CreateCategory creates a new category
func (s *CategoryService) CreateCategory(ctx context.Context, category *models.Category) error {
	s.logger.Info("creating category", zap.String("name", category.Name))

	if category.CreatedAt.IsZero() {
		category.CreatedAt = time.Now()
	}
	category.UpdatedAt = time.Now()

	// Validate parent if exists
	if category.ParentID != nil && *category.ParentID != "" {
		if _, err := s.categoryRepo.GetCategory(ctx, *category.ParentID); err != nil {
			return errors.New("parent category not found")
		}
	}

	return s.categoryRepo.CreateCategory(ctx, category)
}

// GetCategory retrieves a category by ID
func (s *CategoryService) GetCategory(ctx context.Context, id string) (*models.Category, error) {
	return s.categoryRepo.GetCategory(ctx, id)
}

// UpdateCategory updates an existing category
func (s *CategoryService) UpdateCategory(ctx context.Context, category *models.Category) error {
	s.logger.Info("updating category", zap.String("id", category.ID))

	// Check for circular dependency if parent is changing
	if category.ParentID != nil && *category.ParentID != "" {
		if *category.ParentID == category.ID {
			return errors.New("category cannot be its own parent")
		}

		// Traverse up to ensure no cycle
		currentParentID := *category.ParentID
		for currentParentID != "" {
			parent, err := s.categoryRepo.GetCategory(ctx, currentParentID)
			if err != nil {
				return errors.New("invalid parent category in hierarchy")
			}
			if parent.ParentID != nil && *parent.ParentID == category.ID {
				return errors.New("circular dependency detected")
			}
			if parent.ParentID != nil {
				currentParentID = *parent.ParentID
			} else {
				break
			}
		}
	}

	category.UpdatedAt = time.Now()
	return s.categoryRepo.Update(ctx, category)
}

// DeleteCategory deletes a category
func (s *CategoryService) DeleteCategory(ctx context.Context, id string) error {
	s.logger.Info("deleting category", zap.String("id", id))
	// Check if it has children? For now, just delete.
	pk := "INSTANCE#CATEGORY"
	sk := "ID#" + id
	return s.categoryRepo.Delete(ctx, pk, sk)
}

// ListCategories lists categories
func (s *CategoryService) ListCategories(ctx context.Context, parentID *string, limit int) ([]*models.Category, error) {
	return s.categoryRepo.ListCategories(ctx, parentID, limit)
}
