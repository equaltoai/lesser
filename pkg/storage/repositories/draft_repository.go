package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
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

	return r.db.WithContext(ctx).Model(draft).Update()
}

// DeleteDraft deletes a draft
func (r *DraftRepository) DeleteDraft(ctx context.Context, authorID, draftID string) error {
	pk := fmt.Sprintf("USER#%s#DRAFT", authorID)
	sk := fmt.Sprintf("ID#%s", draftID)
	return r.Delete(ctx, pk, sk)
}

// ListDraftsByAuthor lists drafts for an author
func (r *DraftRepository) ListDraftsByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Draft, error) {
	drafts, _, err := r.ListDraftsByAuthorPaginated(ctx, authorID, limit, "")
	return drafts, err
}

// ListDraftsByAuthorPaginated lists drafts for an author with cursor pagination.
// Cursor values are either full SK values (ID#...) or raw draft IDs.
func (r *DraftRepository) ListDraftsByAuthorPaginated(ctx context.Context, authorID string, limit int, cursor string) ([]*models.Draft, string, error) {
	authorID = strings.TrimSpace(authorID)
	if err := common.ValidateRequiredParam("authorID", authorID); err != nil {
		return nil, "", err
	}
	if limit <= 0 {
		limit = 25
	}

	pk := fmt.Sprintf("USER#%s#DRAFT", authorID)
	cursor = strings.TrimSpace(cursor)
	if cursor != "" && !strings.HasPrefix(cursor, "ID#") {
		cursor = "ID#" + cursor
	}

	return listByPKSKPrefixPaginated[*models.Draft](ctx, r.db, &models.Draft{}, pk, "ID#", limit, cursor)
}

// ListScheduledDraftsDuePaginated lists drafts that are scheduled to publish at or before the provided time.
// Cursor values are gsi4SK values.
func (r *DraftRepository) ListScheduledDraftsDuePaginated(ctx context.Context, dueBefore time.Time, limit int, cursor string) ([]*models.Draft, string, error) {
	if dueBefore.IsZero() {
		dueBefore = time.Now()
	}
	if limit <= 0 {
		limit = 25
	}

	pk := "DRAFT#STATUS#scheduled"
	cutoff := fmt.Sprintf("TIME#%s~", dueBefore.UTC().Format(time.RFC3339Nano))

	query := r.db.WithContext(ctx).Model(&models.Draft{}).
		Index("gsi4").
		Where("gsi4PK", "=", pk).
		Where("gsi4SK", "<=", cutoff).
		OrderBy("gsi4SK", "ASC")

	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		query = query.Where("gsi4SK", ">", cursor)
	}

	query = query.Limit(limit + 1)

	var draftModels []models.Draft
	if err := query.All(&draftModels); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(draftModels) > limit {
		nextCursor = draftModels[limit-1].GSI4SK
		draftModels = draftModels[:limit]
	}

	result := make([]*models.Draft, len(draftModels))
	for i := range draftModels {
		result[i] = &draftModels[i]
	}

	return result, nextCursor, nil
}
