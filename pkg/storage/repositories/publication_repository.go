package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// PublicationRepository implements publication operations
type PublicationRepository struct {
	*EnhancedBaseRepository[*models.Publication]
}

// NewPublicationRepository creates a new publication repository
func NewPublicationRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *PublicationRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.Publication](db, tableName, logger, costService, "PublicationRepository", "publication")

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &PublicationRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreatePublication creates a new publication
func (r *PublicationRepository) CreatePublication(ctx context.Context, publication *models.Publication) error {
	return r.ValidateAndCreate(ctx, publication)
}

// GetPublication retrieves a publication by ID
func (r *PublicationRepository) GetPublication(ctx context.Context, id string) (*models.Publication, error) {
	var publication models.Publication
	pk := fmt.Sprintf("PUBLICATION#%s", id)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, &publication)
	if err != nil {
		return nil, err
	}
	return &publication, nil
}
