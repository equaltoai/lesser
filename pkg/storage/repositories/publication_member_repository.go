package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// PublicationMemberRepository implements publication member operations
type PublicationMemberRepository struct {
	*EnhancedBaseRepository[*models.PublicationMember]
}

// NewPublicationMemberRepository creates a new publication member repository
func NewPublicationMemberRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *PublicationMemberRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.PublicationMember](db, tableName, logger, costService, "PublicationMemberRepository", "publication_member")
	
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &PublicationMemberRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateMember adds a new member
func (r *PublicationMemberRepository) CreateMember(ctx context.Context, member *models.PublicationMember) error {
	return r.ValidateAndCreate(ctx, member)
}

// GetMember retrieves a member by publication ID and user ID
func (r *PublicationMemberRepository) GetMember(ctx context.Context, publicationID, userID string) (*models.PublicationMember, error) {
	var member models.PublicationMember
	pk := fmt.Sprintf("PUBLICATION#%s#MEMBER", publicationID)
	sk := fmt.Sprintf("USER#%s", userID)

	err := r.Get(ctx, pk, sk, &member)
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// DeleteMember removes a member
func (r *PublicationMemberRepository) DeleteMember(ctx context.Context, publicationID, userID string) error {
	pk := fmt.Sprintf("PUBLICATION#%s#MEMBER", publicationID)
	sk := fmt.Sprintf("USER#%s", userID)
	return r.Delete(ctx, pk, sk)
}

// ListMembers lists all members of a publication
func (r *PublicationMemberRepository) ListMembers(ctx context.Context, publicationID string) ([]*models.PublicationMember, error) {
	var members []models.PublicationMember
	pk := fmt.Sprintf("PUBLICATION#%s#MEMBER", publicationID)
	
	err := r.db.WithContext(ctx).Model(&models.PublicationMember{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "USER#").
		All(&members)
		
	if err != nil {
		return nil, err
	}
	
	result := make([]*models.PublicationMember, len(members))
	for i := range members {
		result[i] = &members[i]
	}
	return result, nil
}