package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
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

	// EditorialMedia has its own field-scoped writer; content updates must not
	// replay a binding list carried by a stale full-model snapshot. The
	// publish-attempt stamp likewise has its own field-scoped writer (the
	// publishing transition); content updates must not advance or clear it, or
	// an author could re-arm the stale-publishing sweep by editing a
	// crash-stuck draft. A nil omitempty field is unselected by the sparse
	// update, so the stored attribute is left untouched.
	sparse := *draft
	sparse.EditorialMedia = nil
	sparse.PublishAttemptedAt = nil
	return r.db.WithContext(ctx).Model(&sparse).Update()
}

// TransitionDraftToPublishing atomically applies only the fields that enter the
// publishing status: the status, the cleared schedule, the transition
// timestamp, the publish-attempt stamp, and the derived index keys. It is the
// ONLY writer of PublishAttemptedAt; content writers and the editorial-media
// lane run through UpdateDraft / UpdateDraftEditorialMedia, which never select
// the attribute, so a crash-stuck publishing draft cannot be re-armed by an
// author editing it. The index keys are written here because the status change
// must move the row between GSI4 status partitions (and refresh the object
// index) exactly as the full-model UpdateKeys derivation would.
func (r *DraftRepository) TransitionDraftToPublishing(ctx context.Context, authorID string, draft *models.Draft) error {
	if err := validateDraftWriteOwner(authorID, draft); err != nil {
		return err
	}
	if err := draft.UpdateKeys(); err != nil {
		return err
	}

	builder := r.db.WithContext(ctx).
		Model(draft).
		Where("PK", "=", draft.PK).
		Where("SK", "=", draft.SK).
		UpdateBuilder()
	builder.Set("Status", draft.Status)
	builder.Set("UpdatedAt", draft.UpdatedAt)
	if draft.PublishAttemptedAt != nil {
		builder.Set("PublishAttemptedAt", draft.PublishAttemptedAt)
	} else {
		builder.Remove("PublishAttemptedAt")
	}
	builder.Set("GSI1PK", draft.GSI1PK)
	builder.Set("GSI1SK", draft.GSI1SK)
	builder.Set("GSI4PK", draft.GSI4PK)
	builder.Set("GSI4SK", draft.GSI4SK)
	builder.Remove("ScheduledAt")
	builder.ConditionExists("PK")
	if err := builder.Execute(); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.ItemNotFoundWithID("draft", draft.ID).
				WithInternalError(errors.Join(err, storage.ErrNotFound))
		}
		return ErrorHandler.HandleUpdateError(err, "draft", draft.ID)
	}
	return nil
}

// UpdateDraftEditorialMedia atomically replaces only the editorial-media
// association and its update timestamp. An empty association removes the
// sparse attribute explicitly instead of relying on omitempty update behavior.
// The write is version-conditioned: the ModelVersion captured at GetDraft time
// must still hold (or the row may predate the M4 version surface — the
// migration-safe first write stamps version 1), and the winner bumps the
// version. A concurrent media-set loser receives a CONFLICT instead of
// silently losing its update, closing the setDraftEditorialMedia lost-update
// seam (M4 fold-in). ExecuteWithResult is required because Execute() compiles
// every condition as AND and would drop the OR disjunct.
func (r *DraftRepository) UpdateDraftEditorialMedia(ctx context.Context, authorID string, draft *models.Draft) error {
	if err := validateDraftWriteOwner(authorID, draft); err != nil {
		return err
	}
	if err := draft.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := draft.ModelVersion + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(draft).
		Where("PK", "=", draft.PK).
		Where("SK", "=", draft.SK).
		UpdateBuilder()
	if len(draft.EditorialMedia) == 0 {
		builder.Remove("EditorialMedia")
	} else {
		builder.Set("EditorialMedia", draft.EditorialMedia)
	}
	builder.Set("UpdatedAt", draft.UpdatedAt)
	// The key-exists condition leads so a missing row fails closed instead of
	// being upserted by the attribute_not_exists disjunct.
	builder.ConditionExists("PK")
	builder.ConditionNotExists("ModelVersion")
	builder.OrCondition("ModelVersion", "=", draft.ModelVersion)
	builder.Set("ModelVersion", nextVersion)
	var updated models.Draft
	if err := builder.ExecuteWithResult(&updated); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.DynamoDBConditionalCheckFailed("draft " + draft.ID).
				WithInternalError(errors.Join(err, storage.ErrVersionConflict))
		}
		return ErrorHandler.HandleUpdateError(err, "draft", draft.ID)
	}
	draft.ModelVersion = nextVersion
	return nil
}

// UpdateDraftReviewFields atomically updates only the mutable review summary.
// Content and other owner-controlled fields are deliberately excluded so a
// concurrent owner edit cannot be overwritten by review submission.
func (r *DraftRepository) UpdateDraftReviewFields(ctx context.Context, authorID string, draft *models.Draft) error {
	if err := validateDraftWriteOwner(authorID, draft); err != nil {
		return err
	}
	if err := draft.UpdateKeys(); err != nil {
		return err
	}

	builder := r.db.WithContext(ctx).
		Model(draft).
		Where("PK", "=", draft.PK).
		Where("SK", "=", draft.SK).
		UpdateBuilder()
	builder.Set("ReviewedBy", draft.ReviewedBy)
	builder.Set("ReviewStatus", draft.ReviewStatus)
	builder.Set("EditorNotes", draft.EditorNotes)
	builder.ConditionExists("PK")
	if err := builder.Execute(); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.ItemNotFoundWithID("draft", draft.ID).
				WithInternalError(errors.Join(err, storage.ErrNotFound))
		}
		return ErrorHandler.HandleUpdateError(err, "draft", draft.ID)
	}
	return nil
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

// ListDraftsByStatusPaginated lists drafts in one status, paginated by GSI4SK
// cursor values. It powers orphan reconciliation over terminally failed drafts
// whose bound media mints may have survived a best-effort rollback.
func (r *DraftRepository) ListDraftsByStatusPaginated(ctx context.Context, status string, limit int, cursor string) ([]*models.Draft, string, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return nil, "", common.ValidateRequiredParam("status", status)
	}
	if limit <= 0 {
		limit = 25
	}

	query := r.db.WithContext(ctx).Model(&models.Draft{}).
		Index("gsi4").
		Where("gsi4PK", "=", "DRAFT#STATUS#"+status).
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

// CreateDraftReviewGrant creates a first-time review grant.
func (r *DraftRepository) CreateDraftReviewGrant(ctx context.Context, grant *models.DraftReviewGrant) error {
	if err := grant.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "draft review grant", grant.SK)
	}
	err := r.db.WithContext(ctx).Model(grant).IfNotExists().Create()
	if dynamormerrors.IsConditionFailed(err) {
		return apperrors.DynamoDBConditionalCheckFailed("").WithInternalError(err)
	}
	return ErrorHandler.HandleCreateError(err, "draft review grant", grant.SK)
}

// RegrantDraftReviewGrant clears revocation and restores the sparse queue keys.
func (r *DraftRepository) RegrantDraftReviewGrant(ctx context.Context, grant *models.DraftReviewGrant) error {
	if grant == nil || grant.RevokedAt != nil {
		return fmt.Errorf("active draft review grant is required")
	}
	if err := grant.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "draft review grant", grant.SK)
	}

	nextVersion := grant.Version + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(grant).
		Where("PK", "=", grant.PK).
		Where("SK", "=", grant.SK).
		UpdateBuilder()
	builder.Set("GrantedAt", grant.GrantedAt.UTC())
	if grant.ExpiresAt != nil {
		expiresAt := grant.ExpiresAt.UTC()
		builder.Set("ExpiresAt", expiresAt)
	} else {
		builder.Remove("ExpiresAt")
	}
	builder.Set("GSI2PK", grant.GSI2PK)
	builder.Set("GSI2SK", grant.GSI2SK)
	builder.Remove("RevokedAt")
	builder.ConditionVersion(int64(grant.Version))
	builder.Set("Version", nextVersion)
	if err := builder.Execute(); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.DynamoDBConditionalCheckFailed("").WithInternalError(err)
		}
		return ErrorHandler.HandleCreateError(err, "draft review grant", grant.SK)
	}
	grant.Version = nextVersion
	return nil
}

// RevokeDraftReviewGrant persists revocation and removes the sparse queue keys.
func (r *DraftRepository) RevokeDraftReviewGrant(ctx context.Context, grant *models.DraftReviewGrant) error {
	if grant == nil || grant.RevokedAt == nil {
		return fmt.Errorf("revoked draft review grant is required")
	}
	if err := grant.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := grant.Version + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(grant).
		Where("PK", "=", grant.PK).
		Where("SK", "=", grant.SK).
		UpdateBuilder()
	builder.Set("RevokedAt", grant.RevokedAt.UTC())
	builder.Remove("GSI2PK")
	builder.Remove("GSI2SK")
	builder.ConditionVersion(int64(grant.Version))
	builder.Set("Version", nextVersion)
	if err := builder.Execute(); err != nil {
		return err
	}
	grant.Version = nextVersion
	return nil
}

// GetDraftReviewGrant loads a grant by its owner, draft, and reviewer.
func (r *DraftRepository) GetDraftReviewGrant(ctx context.Context, ownerID, draftID, reviewer string) (*models.DraftReviewGrant, error) {
	var grant models.DraftReviewGrant
	err := r.db.WithContext(ctx).Model(&models.DraftReviewGrant{}).Where("PK", "=", fmt.Sprintf("USER#%s#DRAFT#REVIEW", ownerID)).Where("SK", "=", fmt.Sprintf("GRANT#%s#REVIEWER#%s", draftID, reviewer)).First(&grant)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return &grant, nil
}

// ListActiveDraftReviewGrants returns one page from the sparse reviewer queue.
func (r *DraftRepository) ListActiveDraftReviewGrants(ctx context.Context, reviewer string, limit int, cursor string) ([]*models.DraftReviewGrant, string, error) {
	if limit <= 0 {
		limit = 25
	}
	query := r.db.WithContext(ctx).
		Model(&models.DraftReviewGrant{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("DRAFT#REVIEWER#%s", reviewer)).
		Filter("RevokedAt", "attribute_not_exists", nil).
		OrderBy("gsi2SK", "DESC")
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		query = query.Where("gsi2SK", "<", cursor)
	}

	var rows []models.DraftReviewGrant
	err := query.Limit(limit + 1).All(&rows)
	if err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(rows) > limit {
		nextCursor = rows[limit-1].GSI2SK
		rows = rows[:limit]
	}
	out := make([]*models.DraftReviewGrant, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nextCursor, nil
}

// ListDraftReviewGrants returns all grant records for one draft.
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

// ListDraftReviewGrantsByOwner returns every review assignment created by one draft owner.
// Callers apply active-state filtering before pagination so revoked grants cannot shrink pages.
func (r *DraftRepository) ListDraftReviewGrantsByOwner(ctx context.Context, ownerID string) ([]*models.DraftReviewGrant, error) {
	ownerID = strings.TrimSpace(ownerID)
	if err := common.ValidateRequiredParam("ownerID", ownerID); err != nil {
		return nil, err
	}
	var rows []models.DraftReviewGrant
	err := r.db.WithContext(ctx).Model(&models.DraftReviewGrant{}).
		Where("PK", "=", fmt.Sprintf("USER#%s#DRAFT#REVIEW", ownerID)).
		Where("SK", "begins_with", "GRANT#").
		OrderBy("SK", "ASC").
		All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]*models.DraftReviewGrant, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

// CreateDraftReviewVerdict records an immutable verdict.
func (r *DraftRepository) CreateDraftReviewVerdict(ctx context.Context, verdict *models.DraftReviewVerdict) error {
	if err := verdict.UpdateKeys(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(verdict).Create()
}

// ListDraftReviewVerdicts returns ordered verdict history.
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
