package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ListRepositoryV2 implements list operations using BaseRepository
// This demonstrates significant code reduction for a focused repository
type ListRepositoryV2 struct {
	*BaseRepository[*models.List]
	logger *zap.Logger
}

// NewListRepositoryV2 creates a new list repository using BaseRepository
func NewListRepositoryV2(db core.DB, tableName string, logger *zap.Logger) *ListRepositoryV2 {
	return &ListRepositoryV2{
		BaseRepository: NewBaseRepository[*models.List](db, tableName, logger),
		logger:         logger,
	}
}

// CreateList creates a new list for a user
// BEFORE: 30+ lines with manual error handling
// AFTER: Focused on business logic only
func (r *ListRepositoryV2) CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error) {
	// Validate replies policy
	if repliesPolicy == "" {
		repliesPolicy = "list" // default
	}
	if repliesPolicy != "followed" && repliesPolicy != "list" && repliesPolicy != "none" {
		return nil, fmt.Errorf("invalid replies policy: %s", repliesPolicy)
	}

	// Create the list model
	list := &models.List{
		ID:            uuid.New().String(),
		Username:      username,
		Title:         title,
		RepliesPolicy: repliesPolicy,
	}

	// Use BaseRepository Create - saves ~20 lines of boilerplate
	if err := r.Create(ctx, list); err != nil {
		r.logger.Error("failed to create list", zap.Error(err))
		return nil, fmt.Errorf("failed to create list: %w", err)
	}

	// Convert to storage.List
	return r.modelToStorageList(list), nil
}

// GetList retrieves a list by ID
// BEFORE: 15+ lines of query construction
// AFTER: Single BaseRepository Get call
func (r *ListRepositoryV2) GetList(ctx context.Context, listID string) (*storage.List, error) {
	list := &models.List{}
	
	// Use BaseRepository Get - saves ~15 lines of boilerplate
	err := r.Get(ctx, fmt.Sprintf("LIST#%s", listID), "METADATA", list)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		r.logger.Error("failed to get list", zap.String("list_id", listID), zap.Error(err))
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	return r.modelToStorageList(list), nil
}

// UpdateList updates a list's title or replies policy
// BEFORE: Complex query and update logic
// AFTER: Get + Update using BaseRepository
func (r *ListRepositoryV2) UpdateList(ctx context.Context, listID, title, repliesPolicy string) (*storage.List, error) {
	// Get existing list
	list := &models.List{}
	err := r.Get(ctx, fmt.Sprintf("LIST#%s", listID), "METADATA", list)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get list for update: %w", err)
	}

	// Update fields
	list.Title = title
	if repliesPolicy != "" {
		list.RepliesPolicy = repliesPolicy
	}

	// Use BaseRepository Update - saves ~15 lines of boilerplate
	if err := r.Update(ctx, list); err != nil {
		r.logger.Error("failed to update list", zap.String("list_id", listID), zap.Error(err))
		return nil, fmt.Errorf("failed to update list: %w", err)
	}

	return r.modelToStorageList(list), nil
}

// DeleteList deletes a list
// BEFORE: 20+ lines with error handling
// AFTER: Single BaseRepository Delete call
func (r *ListRepositoryV2) DeleteList(ctx context.Context, listID string) error {
	// Use BaseRepository Delete - saves ~15 lines of boilerplate
	err := r.Delete(ctx, fmt.Sprintf("LIST#%s", listID), "METADATA")
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		r.logger.Error("failed to delete list", zap.String("list_id", listID), zap.Error(err))
		return fmt.Errorf("failed to delete list: %w", err)
	}

	// Note: In real implementation, would also need to delete list memberships
	// This would be handled in a transaction or separate cleanup process

	return nil
}

// GetUserLists retrieves all lists owned by a user
// Uses BaseRepository QueryGSI for efficient queries
func (r *ListRepositoryV2) GetUserLists(ctx context.Context, username string) ([]*storage.List, error) {
	// Use BaseRepository QueryGSI - saves ~20 lines of query construction
	lists, err := r.QueryGSI(ctx, "GSI1", fmt.Sprintf("USER_LISTS#%s", username), 100)
	if err != nil {
		r.logger.Error("failed to get user lists", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("failed to get user lists: %w", err)
	}

	// Convert to storage.List slice
	result := make([]*storage.List, len(lists))
	for i, list := range lists {
		result[i] = r.modelToStorageList(list)
	}

	return result, nil
}

// CountUserLists counts the number of lists a user has
func (r *ListRepositoryV2) CountUserLists(ctx context.Context, username string) (int, error) {
	// Use BaseRepository Count would be ideal, but it doesn't support GSI
	// For now, query and count
	lists, err := r.GetUserLists(ctx, username)
	if err != nil {
		return 0, err
	}
	return len(lists), nil
}

// Helper to convert model to storage type
func (r *ListRepositoryV2) modelToStorageList(model *models.List) *storage.List {
	return &storage.List{
		ID:            model.ID,
		Username:      model.Username,
		Title:         model.Title,
		RepliesPolicy: model.RepliesPolicy,
		CreatedAt:     model.CreatedAt,
		UpdatedAt:     model.UpdatedAt,
	}
}

// Note: List membership operations (AddToList, RemoveFromList, GetListMembers, etc.)
// would use a different model (ListMembership) and might need their own repository
// or could be added here with direct DynamORM calls for now

// Code Reduction Summary:
// - CreateList: ~20 lines saved (error handling, key generation)
// - GetList: ~15 lines saved (query construction)
// - UpdateList: ~15 lines saved (update logic)
// - DeleteList: ~15 lines saved (delete logic)
// - GetUserLists: ~20 lines saved (GSI query)
// Total: ~85 lines of boilerplate eliminated!
//
// Additional benefits:
// - Consistent error handling
// - Built-in logging at BaseRepository level
// - Type safety with generics
// - Easier to test and maintain