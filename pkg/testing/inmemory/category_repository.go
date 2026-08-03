// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// RootCategoryKey is the parent key used for top-level categories.
const RootCategoryKey = "ROOT"

// CategoryRepository is a thread-safe in-memory implementation of interfaces.CategoryRepository.
type CategoryRepository struct {
	mu sync.RWMutex

	// Categories by ID
	categories map[string]*models.Category

	// Categories by parent: parentID -> []categoryID (use "ROOT" for top-level)
	categoriesByParent map[string][]string
}

// NewCategoryRepository creates a new in-memory category repository
func NewCategoryRepository() *CategoryRepository {
	return &CategoryRepository{
		categories:         make(map[string]*models.Category),
		categoriesByParent: make(map[string][]string),
	}
}

// GetDB returns the underlying DynamoDB connection.
// For in-memory implementation, this returns nil.
func (r *CategoryRepository) GetDB() dynamormcore.DB {
	return nil
}

// CreateCategory creates a new category
func (r *CategoryRepository) CreateCategory(_ context.Context, category *models.Category) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if category == nil || category.ID == "" {
		return storage.ErrInvalidInput
	}

	if _, exists := r.categories[category.ID]; exists {
		return storage.ErrAlreadyExists
	}

	// Store category
	r.categories[category.ID] = category

	// Index by parent
	parentKey := RootCategoryKey
	if category.ParentID != nil && *category.ParentID != "" {
		parentKey = *category.ParentID
	}
	r.categoriesByParent[parentKey] = append(r.categoriesByParent[parentKey], category.ID)

	return nil
}

// GetCategory retrieves a category by ID
func (r *CategoryRepository) GetCategory(_ context.Context, id string) (*models.Category, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	category, exists := r.categories[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return category, nil
}

// ListCategories lists all categories (optionally filtered by parent)
func (r *CategoryRepository) ListCategories(_ context.Context, parentID *string, limit int) ([]*models.Category, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	var result []*models.Category

	if parentID != nil {
		// Filter by parent
		parentKey := RootCategoryKey
		if *parentID != "" {
			parentKey = *parentID
		}

		categoryIDs := r.categoriesByParent[parentKey]
		for _, id := range categoryIDs {
			if category, exists := r.categories[id]; exists {
				result = append(result, category)
				if len(result) >= limit {
					break
				}
			}
		}
	} else {
		// Return all categories
		for _, category := range r.categories {
			result = append(result, category)
			if len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// UpdateArticleCount atomically increments/decrements a category's ArticleCount
func (r *CategoryRepository) UpdateArticleCount(_ context.Context, categoryID string, delta int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" || delta == 0 {
		return nil
	}

	category, exists := r.categories[categoryID]
	if !exists {
		// Treat missing category as no-op (matches real implementation)
		return nil
	}

	newCount := category.ArticleCount + delta
	if newCount < 0 {
		newCount = 0
	}
	category.ArticleCount = newCount

	return nil
}

// Update updates an existing category
func (r *CategoryRepository) Update(_ context.Context, category *models.Category) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if category == nil || category.ID == "" {
		return storage.ErrInvalidInput
	}

	if _, exists := r.categories[category.ID]; !exists {
		return storage.ErrNotFound
	}

	r.categories[category.ID] = category
	return nil
}

// Delete deletes a category by PK and SK
func (r *CategoryRepository) Delete(_ context.Context, _, sk string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Extract categoryID from SK (format: ID#<categoryID>)
	sk = strings.TrimSpace(sk)
	if !strings.HasPrefix(sk, "ID#") {
		return storage.ErrInvalidInput
	}
	categoryID := strings.TrimPrefix(sk, "ID#")

	category, exists := r.categories[categoryID]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from categoriesByParent index
	parentKey := RootCategoryKey
	if category.ParentID != nil && *category.ParentID != "" {
		parentKey = *category.ParentID
	}
	ids := r.categoriesByParent[parentKey]
	for i, id := range ids {
		if id == categoryID {
			r.categoriesByParent[parentKey] = append(ids[:i], ids[i+1:]...)
			break
		}
	}

	// Remove from categories map
	delete(r.categories, categoryID)

	return nil
}

// Clear clears all data (test helper)
func (r *CategoryRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.categories = make(map[string]*models.Category)
	r.categoriesByParent = make(map[string][]string)
}

// Ensure CategoryRepository implements interfaces.CategoryRepository
var _ interfaces.CategoryRepository = (*CategoryRepository)(nil)
