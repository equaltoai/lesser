package cms

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

type categoryRepository interface {
	GetDB() dynamormcore.DB
	CreateCategory(ctx context.Context, category *models.Category) error
	GetCategory(ctx context.Context, id string) (*models.Category, error)
	Update(ctx context.Context, category *models.Category) error
	Delete(ctx context.Context, pk, sk string) error
	ListCategories(ctx context.Context, parentID *string, limit int) ([]*models.Category, error)
}

// CategoryService handles business logic for categories
type CategoryService struct {
	categoryRepo categoryRepository
	logger       *zap.Logger
}

// NewCategoryService creates a new CategoryService
func NewCategoryService(categoryRepo categoryRepository, logger *zap.Logger) *CategoryService {
	return &CategoryService{
		categoryRepo: categoryRepo,
		logger:       logger,
	}
}

// CreateCategory creates a new category
func (s *CategoryService) CreateCategory(ctx context.Context, category *models.Category) error {
	if category == nil {
		return errors.New("category is required")
	}

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

	slug, slugCreated, err := s.reserveSlugIndex(ctx, category)
	if err != nil {
		return err
	}

	if err := s.categoryRepo.CreateCategory(ctx, category); err != nil {
		if slugCreated {
			cmsDeleteCategorySlugIndex(ctx, s.categoryRepo.GetDB(), slug)
		}
		return err
	}

	return nil
}

// GetCategory retrieves a category by ID
func (s *CategoryService) GetCategory(ctx context.Context, id string) (*models.Category, error) {
	return s.categoryRepo.GetCategory(ctx, id)
}

// UpdateCategory updates an existing category
func (s *CategoryService) UpdateCategory(ctx context.Context, category *models.Category) error {
	if category == nil {
		return errors.New("category is required")
	}

	s.logger.Info("updating category", zap.String("id", category.ID))

	if err := s.validateHierarchy(ctx, category); err != nil {
		return err
	}

	slug, slugCreated, err := s.reserveSlugIndex(ctx, category)
	if err != nil {
		return err
	}

	category.UpdatedAt = time.Now()
	if err := s.categoryRepo.Update(ctx, category); err != nil {
		if slugCreated {
			cmsDeleteCategorySlugIndex(ctx, s.categoryRepo.GetDB(), slug)
		}
		return err
	}

	return nil
}

func (s *CategoryService) reserveSlugIndex(ctx context.Context, category *models.Category) (slug string, created bool, err error) {
	if category == nil {
		return "", false, errors.New("category is required")
	}

	slug = strings.TrimSpace(category.Slug)
	if slug == "" {
		return "", false, apperrors.ValidationFailedWithField("slug")
	}
	category.Slug = slug

	categoryID := strings.TrimSpace(category.ID)
	if categoryID == "" {
		return "", false, apperrors.ValidationFailedWithField("id")
	}

	host := cmsHostFromURL(categoryID)
	if host != "" {
		legacyID := common.GenerateObjectID(host, "categories", slug)
		if legacyID != "" && !strings.EqualFold(legacyID, categoryID) {
			_, lookupErr := s.categoryRepo.GetCategory(ctx, legacyID)
			if lookupErr == nil {
				return "", false, apperrors.ItemAlreadyExistsWithID("category slug", slug)
			}
			if lookupErr != nil && !apperrors.HasCode(lookupErr, apperrors.CodeNotFound) {
				return "", false, lookupErr
			}
		}
	}

	created, err = cmsEnsureCategorySlugIndex(ctx, s.categoryRepo.GetDB(), slug, categoryID)
	return slug, created, err
}

func (s *CategoryService) validateHierarchy(ctx context.Context, category *models.Category) error {
	if category == nil {
		return errors.New("category is required")
	}

	if category.ParentID == nil || strings.TrimSpace(*category.ParentID) == "" {
		return nil
	}

	if *category.ParentID == category.ID {
		return errors.New("category cannot be its own parent")
	}

	currentParentID := strings.TrimSpace(*category.ParentID)
	for currentParentID != "" {
		parent, err := s.categoryRepo.GetCategory(ctx, currentParentID)
		if err != nil {
			return errors.New("invalid parent category in hierarchy")
		}
		if parent.ParentID != nil && strings.EqualFold(strings.TrimSpace(*parent.ParentID), category.ID) {
			return errors.New("circular dependency detected")
		}
		if parent.ParentID == nil {
			return nil
		}
		currentParentID = strings.TrimSpace(*parent.ParentID)
	}

	return nil
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
