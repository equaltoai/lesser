package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// RevisionRepository implements revision operations
type RevisionRepository struct {
	*EnhancedBaseRepository[*models.Revision]
}

// NewRevisionRepository creates a new revision repository
func NewRevisionRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *RevisionRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.Revision](db, tableName, logger, costService, "RevisionRepository", "revision")

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &RevisionRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateRevision creates a new revision
func (r *RevisionRepository) CreateRevision(ctx context.Context, revision *models.Revision) error {
	return r.ValidateAndCreate(ctx, revision)
}

// GetRevision retrieves a revision by object ID and version
func (r *RevisionRepository) GetRevision(ctx context.Context, objectID string, version int) (*models.Revision, error) {
	var revision models.Revision
	pk := fmt.Sprintf("OBJECT#%s#REVISION", objectID)
	sk := fmt.Sprintf("VERSION#%08d", version)

	err := r.Get(ctx, pk, sk, &revision)
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

// ListRevisions lists revisions for an object
func (r *RevisionRepository) ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error) {
	revisions, _, err := r.ListRevisionsPaginated(ctx, objectID, limit, "")
	return revisions, err
}

// ListRevisionsPaginated lists revisions for an object with cursor pagination.
// Cursor values are full SK values (VERSION#...).
func (r *RevisionRepository) ListRevisionsPaginated(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Revision, string, error) {
	objectID = strings.TrimSpace(objectID)
	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return nil, "", err
	}
	if limit <= 0 {
		limit = 25
	}

	pk := fmt.Sprintf("OBJECT#%s#REVISION", objectID)
	query := r.db.WithContext(ctx).Model(&models.Revision{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "VERSION#").
		OrderBy("SK", "DESC")

	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		if !strings.HasPrefix(cursor, "VERSION#") {
			cursor = fmt.Sprintf("VERSION#%s", cursor)
		}
		query = query.Where("SK", "<", cursor)
	}

	query = query.Limit(limit + 1)

	var revisionModels []models.Revision
	if err := query.All(&revisionModels); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(revisionModels) > limit {
		nextCursor = revisionModels[limit-1].SK
		revisionModels = revisionModels[:limit]
	}

	result := make([]*models.Revision, len(revisionModels))
	for i := range revisionModels {
		result[i] = &revisionModels[i]
	}

	return result, nextCursor, nil
}
