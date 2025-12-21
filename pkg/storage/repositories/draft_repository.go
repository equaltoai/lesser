package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// DraftRepository implements draft operations
type DraftRepository struct {
	*EnhancedBaseRepository[*models.Draft]
}

// NewDraftRepository creates a new draft repository
func NewDraftRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *DraftRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.Draft](db, tableName, logger, costService, "DraftRepository", "draft")

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &DraftRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateDraft creates a new draft
func (r *DraftRepository) CreateDraft(ctx context.Context, draft *models.Draft) error {
	return r.ValidateAndCreate(ctx, draft)
}

// GetDraft retrieves a draft by author ID and draft ID
func (r *DraftRepository) GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error) {
	var draft models.Draft
	pk := fmt.Sprintf("USER#%s#DRAFT", authorID)
	sk := fmt.Sprintf("ID#%s", draftID)

	err := r.Get(ctx, pk, sk, &draft)
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

// UpdateDraft updates an existing draft
func (r *DraftRepository) UpdateDraft(ctx context.Context, draft *models.Draft) error {
	if err := draft.UpdateKeys(); err != nil {
		return err
	}

	// Use UpdateBuilder for specific fields to avoid overwriting everything if needed
	// For drafts, we usually update content and metadata
	return r.db.WithContext(ctx).Model(&models.Draft{}).
		Where("PK", "=", draft.PK).
		Where("SK", "=", draft.SK).
		UpdateBuilder().
		Set("Content", draft.Content).
		Set("Title", draft.Title).
		Set("MetadataJSON", draft.MetadataJSON).
		Set("UpdatedAt", time.Now()).
		Set("AutosaveVersion", draft.AutosaveVersion).
		Set("LastSavedAt", time.Now()).
		Execute()
}

// DeleteDraft deletes a draft
func (r *DraftRepository) DeleteDraft(ctx context.Context, authorID, draftID string) error {
	pk := fmt.Sprintf("USER#%s#DRAFT", authorID)
	sk := fmt.Sprintf("ID#%s", draftID)
	return r.Delete(ctx, pk, sk)
}

// ListDraftsByAuthor lists drafts for an author
func (r *DraftRepository) ListDraftsByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Draft, error) {
	// Since drafts are stored with PK=USER#{author_id}#DRAFT, we can query by PK prefix if we had a GSI or if we scan.
	// However, the design doc suggests GSI1 is for Object drafts.
	// To list by author, we can query the main table with PK = USER#{author_id}#DRAFT and SK begins with ID#.
	// But PK must be exact.
	// Actually, the PK is USER#{author_id}#DRAFT. So all drafts for a user share the same PK.
	// So we can Query by PK.

	var drafts []models.Draft
	pk := fmt.Sprintf("USER#%s#DRAFT", authorID)

	err := r.db.WithContext(ctx).Model(&models.Draft{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "ID#").
		Limit(limit).
		All(&drafts)

	if err != nil {
		return nil, err
	}

	result := make([]*models.Draft, len(drafts))
	for i := range drafts {
		result[i] = &drafts[i]
	}
	return result, nil
}
