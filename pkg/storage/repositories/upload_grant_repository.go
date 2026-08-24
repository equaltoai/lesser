package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// UploadGrantRepository implements storage for one-time, hash-bound,
// actor-scoped upload grants. Grants live in the owner's partition
// (USER#{owner}#UPLOAD / GRANT#{grantID}) so every read is owner-scoped by
// construction and no grant is ever world-readable.
type UploadGrantRepository struct {
	*EnhancedBaseRepository[*models.UploadGrant]
}

// NewUploadGrantRepository creates an upload grant repository.
func NewUploadGrantRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *UploadGrantRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.UploadGrant](db, tableName, logger, costService, "UploadGrantRepository", "upload_grant")
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())
	return &UploadGrantRepository{EnhancedBaseRepository: enhancedRepo}
}

// CreateUploadGrant persists a freshly minted grant. The conditional create is
// keyed on the unguessable grant ID so a colliding mint fails closed.
func (r *UploadGrantRepository) CreateUploadGrant(ctx context.Context, grant *models.UploadGrant) error {
	if grant == nil {
		return fmt.Errorf("upload grant is required")
	}
	if err := grant.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "upload grant", grant.SK)
	}
	err := r.db.WithContext(ctx).Model(grant).IfNotExists().Create()
	if dynamormerrors.IsConditionFailed(err) {
		return apperrors.DynamoDBConditionalCheckFailed("upload grant " + grant.GrantID).WithInternalError(err)
	}
	return ErrorHandler.HandleCreateError(err, "upload grant", grant.SK)
}

// GetUploadGrant loads one grant within the owner's partition. Not-found is
// returned as storage.ErrNotFound so callers fail closed on unknown grants.
func (r *UploadGrantRepository) GetUploadGrant(ctx context.Context, ownerID, grantID string) (*models.UploadGrant, error) {
	ownerID = strings.TrimSpace(ownerID)
	grantID = strings.TrimSpace(grantID)
	if ownerID == "" || grantID == "" {
		return nil, fmt.Errorf("owner and grantID are required")
	}
	var grant models.UploadGrant
	err := r.db.WithContext(ctx).
		Model(&models.UploadGrant{}).
		Where("PK", "=", fmt.Sprintf("USER#%s#UPLOAD", ownerID)).
		Where("SK", "=", fmt.Sprintf("GRANT#%s", grantID)).
		First(&grant)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return &grant, nil
}

// ConsumeUploadGrant atomically transitions a MINTED grant to one terminal
// status, conditioned on the observed version and the MINTED state. Exactly
// one concurrent finalize wins; a loser receives an error wrapping
// interfaces.ErrUploadGrantConsumed.
func (r *UploadGrantRepository) ConsumeUploadGrant(ctx context.Context, grant *models.UploadGrant, status, failureReason string, now time.Time) error {
	if grant == nil {
		return fmt.Errorf("upload grant is required")
	}
	status = strings.TrimSpace(status)
	switch status {
	case models.UploadGrantStatusUsed, models.UploadGrantStatusFailedDigest:
	default:
		return fmt.Errorf("invalid upload grant terminal status %q", status)
	}
	if err := grant.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := grant.Version + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(&models.UploadGrant{}).
		Where("PK", "=", grant.PK).
		Where("SK", "=", grant.SK).
		UpdateBuilder()
	builder.Set("Status", status)
	switch status {
	case models.UploadGrantStatusUsed:
		builder.Set("UsedAt", now.UTC())
		builder.Remove("FailedAt")
		builder.Remove("FailureReason")
	case models.UploadGrantStatusFailedDigest:
		builder.Set("FailedAt", now.UTC())
		builder.Set("FailureReason", strings.TrimSpace(failureReason))
		builder.Remove("UsedAt")
	}
	// The MINTED condition is belt-and-braces on top of the version condition:
	// a consumed grant cannot be re-transitioned even if a stale read carried a
	// current version.
	builder.Condition("Status", "=", models.UploadGrantStatusMinted)
	builder.ConditionVersion(int64(grant.Version))
	builder.Set("Version", nextVersion)
	if err := builder.Execute(); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return fmt.Errorf("%w: %w", interfaces.ErrUploadGrantConsumed, err)
		}
		return ErrorHandler.HandleUpdateError(err, "upload grant", grant.SK)
	}
	grant.Status = status
	grant.Version = nextVersion
	switch status {
	case models.UploadGrantStatusUsed:
		usedAt := now.UTC()
		grant.UsedAt = &usedAt
		grant.FailedAt = nil
		grant.FailureReason = ""
	case models.UploadGrantStatusFailedDigest:
		failedAt := now.UTC()
		grant.FailedAt = &failedAt
		grant.FailureReason = strings.TrimSpace(failureReason)
		grant.UsedAt = nil
	}
	return nil
}
