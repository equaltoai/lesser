package repositories

import (
	"context"
	stdErrors "errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// InboxProcessingRepository records target-scoped inbox idempotency receipts.
type InboxProcessingRepository struct {
	*BaseRepository[*models.InboxProcessingReceipt]
}

// NewInboxProcessingRepository creates a repository for inbox processing receipts.
func NewInboxProcessingRepository(
	db core.DB,
	tableName string,
	logger *zap.Logger,
	costService *cost.TrackingService,
) *InboxProcessingRepository {
	return &InboxProcessingRepository{
		BaseRepository: NewBaseRepositoryWithCostTracking[*models.InboxProcessingReceipt](
			db,
			tableName,
			logger,
			costService,
			"InboxProcessingRepository",
		),
	}
}

// TryRecordTarget records an activity/target pair. It returns false when the
// pair has already been claimed and therefore should not be processed again.
func (r *InboxProcessingRepository) TryRecordTarget(ctx context.Context, activityID, targetActorID, activityType string) (bool, error) {
	receipt := models.NewInboxProcessingReceipt(activityID, targetActorID, activityType, time.Now().UTC())
	if err := r.CreateIfNotExists(ctx, receipt); err != nil {
		if isInboxProcessingAlreadyRecorded(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ForgetTarget removes a previously claimed activity/target pair so a retry can
// attempt the side effect again after a processing failure.
func (r *InboxProcessingRepository) ForgetTarget(ctx context.Context, activityID, targetActorID string) error {
	receipt := models.NewInboxProcessingReceipt(activityID, targetActorID, "", time.Now().UTC())
	if err := receipt.UpdateKeys(); err != nil {
		return err
	}
	return r.Delete(ctx, receipt.GetPK(), receipt.GetSK())
}

func isInboxProcessingAlreadyRecorded(err error) bool {
	if err == nil {
		return false
	}
	return apperrors.HasCode(err, apperrors.CodeAlreadyExists) ||
		apperrors.HasCode(err, apperrors.CodeConflict) ||
		stdErrors.Is(err, storage.ErrAlreadyExists) ||
		dynamormerrors.IsConditionFailed(err) ||
		strings.Contains(strings.ToLower(err.Error()), "already exists")
}
