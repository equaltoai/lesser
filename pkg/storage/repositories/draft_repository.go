package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
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

func validateDraftWriteOwner(authorID string, draft *models.Draft) error {
	authorID = strings.TrimSpace(authorID)
	if err := common.ValidateRequiredParam("authorID", authorID); err != nil {
		return err
	}
	if draft == nil {
		return common.ValidationError{Field: "draft", Message: "is required"}
	}
	draftAuthor := strings.TrimSpace(draft.AuthorID)
	if err := common.ValidateRequiredParam("draft.AuthorID", draftAuthor); err != nil {
		return err
	}
	if draftAuthor != authorID {
		return common.ValidationError{Field: "authorID", Message: "does not match draft author"}
	}
	draft.AuthorID = draftAuthor
	return nil
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
func (r *DraftRepository) UpdateDraft(ctx context.Context, authorID string, draft *models.Draft) error {
	if err := validateDraftWriteOwner(authorID, draft); err != nil {
		return err
	}
	if err := draft.UpdateKeys(); err != nil {
		return err
	}

	return r.db.WithContext(ctx).Model(draft).Update()
}

// DeleteDraft deletes a draft
func (r *DraftRepository) DeleteDraft(ctx context.Context, authorID, draftID string) error {
	authorID = strings.TrimSpace(authorID)
	draftID = strings.TrimSpace(draftID)
	if err := common.ValidateRequiredParam("authorID", authorID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("draftID", draftID); err != nil {
		return err
	}
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

func (r *DraftRepository) PutDraftReviewGrant(ctx context.Context, grant *models.DraftReviewGrant) error {
	if err := grant.UpdateKeys(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(grant).Update()
}
func (r *DraftRepository) GetDraftReviewGrant(ctx context.Context, ownerID, draftID, reviewer string) (*models.DraftReviewGrant, error) {
	var grant models.DraftReviewGrant
	err := r.db.WithContext(ctx).Model(&models.DraftReviewGrant{}).Where("PK", "=", fmt.Sprintf("USER#%s#DRAFT#REVIEW", ownerID)).Where("SK", "=", fmt.Sprintf("GRANT#%s#REVIEWER#%s", draftID, reviewer)).First(&grant)
	if err != nil {
		return nil, err
	}
	return &grant, nil
}
func (r *DraftRepository) ListActiveDraftReviewGrants(ctx context.Context, reviewer string, limit int) ([]*models.DraftReviewGrant, error) {
	if limit <= 0 {
		limit = 25
	}
	var rows []models.DraftReviewGrant
	err := r.db.WithContext(ctx).Model(&models.DraftReviewGrant{}).Index("gsi2").Where("gsi2PK", "=", fmt.Sprintf("DRAFT#REVIEWER#%s", reviewer)).OrderBy("gsi2SK", "DESC").Limit(limit).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]*models.DraftReviewGrant, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}
func (r *DraftRepository) ListDraftReviewGrants(ctx context.Context, ownerID, draftID string) ([]*models.DraftReviewGrant, error) {
	var rows []models.DraftReviewGrant
	err := r.db.WithContext(ctx).Model(&models.DraftReviewGrant{}).
		Where("PK", "=", fmt.Sprintf("USER#%s#DRAFT#REVIEW", ownerID)).
		Where("SK", "begins_with", fmt.Sprintf("GRANT#%s#", draftID)).
		OrderBy("SK", "ASC").All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]*models.DraftReviewGrant, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}
func (r *DraftRepository) CreateDraftReviewVerdict(ctx context.Context, verdict *models.DraftReviewVerdict) error {
	if err := verdict.UpdateKeys(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(verdict).Create()
}
func (r *DraftRepository) ListDraftReviewVerdicts(ctx context.Context, ownerID, draftID string) ([]*models.DraftReviewVerdict, error) {
	var rows []models.DraftReviewVerdict
	err := r.db.WithContext(ctx).Model(&models.DraftReviewVerdict{}).Where("PK", "=", fmt.Sprintf("USER#%s#DRAFT#REVIEW", ownerID)).Where("SK", "begins_with", fmt.Sprintf("VERDICT#%s#", draftID)).OrderBy("SK", "ASC").All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]*models.DraftReviewVerdict, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}
