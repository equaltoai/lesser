package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// DirectMessageTombstoneRepository persists per-viewer tombstones for direct messages.
// These records support "delete for me" semantics without globally deleting the underlying Status.
type DirectMessageTombstoneRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewDirectMessageTombstoneRepository creates a new DirectMessageTombstoneRepository.
func NewDirectMessageTombstoneRepository(db core.DB, tableName string, logger *zap.Logger) *DirectMessageTombstoneRepository {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &DirectMessageTombstoneRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateTombstone records that viewerUsername has deleted statusID "for me".
func (r *DirectMessageTombstoneRepository) CreateTombstone(ctx context.Context, viewerUsername, statusID string) error {
	viewerUsername = strings.TrimSpace(viewerUsername)
	statusID = strings.TrimSpace(statusID)
	if viewerUsername == "" || statusID == "" {
		return storage.ErrInvalidInput
	}

	record := &models.DirectMessageTombstone{
		ViewerUsername: viewerUsername,
		StatusID:       statusID,
		CreatedAt:      time.Now().UTC(),
	}
	if err := record.UpdateKeys(); err != nil {
		return err
	}

	err := r.db.WithContext(ctx).Model(record).IfNotExists().Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			return nil
		}
		return err
	}

	return nil
}

// TombstonesByStatusID returns a set of status IDs that have been tombstoned by the viewer.
func (r *DirectMessageTombstoneRepository) TombstonesByStatusID(ctx context.Context, viewerUsername string, statusIDs []string) (map[string]bool, error) {
	viewerUsername = strings.TrimSpace(viewerUsername)
	if viewerUsername == "" {
		return nil, storage.ErrInvalidInput
	}

	result := make(map[string]bool, len(statusIDs))
	if len(statusIDs) == 0 {
		return result, nil
	}

	pk := fmt.Sprintf("DM_MESSAGE_TOMBSTONE#%s", viewerUsername)
	for _, statusID := range statusIDs {
		statusID = strings.TrimSpace(statusID)
		if statusID == "" {
			continue
		}

		row := &models.DirectMessageTombstone{}
		err := r.db.WithContext(ctx).Model(row).
			Where("PK", "=", pk).
			Where("SK", "=", fmt.Sprintf("STATUS#%s", statusID)).
			First(row)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			r.logger.Warn("failed to get DM tombstone",
				zap.String("viewer", viewerUsername),
				zap.String("status_id", statusID),
				zap.Error(err))
			return nil, err
		}

		if id := strings.TrimSpace(row.StatusID); id != "" {
			result[id] = true
		}
	}

	return result, nil
}
