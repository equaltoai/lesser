package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
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
	var revisions []models.Revision
	pk := fmt.Sprintf("OBJECT#%s#REVISION", objectID)
	
	err := r.db.WithContext(ctx).Model(&models.Revision{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "VERSION#").
		OrderBy("SK", "DESC"). // Newest first
		Limit(limit).
		All(&revisions)
		
	if err != nil {
		return nil, err
	}
	
	result := make([]*models.Revision, len(revisions))
	for i := range revisions {
		result[i] = &revisions[i]
	}
	return result, nil
}