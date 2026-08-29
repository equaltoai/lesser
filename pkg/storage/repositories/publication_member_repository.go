package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
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

	// The whole keyed PUBLICATION#<id>#MEMBER partition must be read to return
	// every member, so the read is a bounded page walk (wave #1469):
	// Limit(500)/page, 100-page cap, fail-closed on exhaustion.
	err := walkKeyedPages(
		r.db.WithContext(ctx).Model(&models.PublicationMember{}).
			Where("PK", "=", pk).
			Where("SK", "BEGINS_WITH", "USER#"),
		500, 100,
		func(page []models.PublicationMember) (bool, error) {
			members = append(members, page...)
			return false, nil
		},
	)

	if err != nil {
		return nil, err
	}

	result := make([]*models.PublicationMember, len(members))
	for i := range members {
		result[i] = &members[i]
	}
	return result, nil
}

// ListMembershipsForUserPaginated lists publications a user is a member of.
// Cursor values are gsi1SK values (PUBLICATION#...).
func (r *PublicationMemberRepository) ListMembershipsForUserPaginated(ctx context.Context, userID string, limit int, cursor string) ([]*models.PublicationMember, string, error) {
	userID = strings.TrimSpace(userID)
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		return nil, "", err
	}
	if limit <= 0 {
		limit = 25
	}

	pk := fmt.Sprintf("USER#%s#PUBLICATION", userID)
	query := r.db.WithContext(ctx).Model(&models.PublicationMember{}).
		Index("gsi1").
		Where("gsi1PK", "=", pk).
		OrderBy("gsi1SK", "ASC")

	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		query = query.Where("gsi1SK", ">", cursor)
	}

	query = query.Limit(limit + 1)

	var membershipModels []models.PublicationMember
	if err := query.All(&membershipModels); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if err := common.ValidateSliceLength("memberships", membershipModels, limit); err != nil {
		nextCursor = membershipModels[limit-1].GSI1SK
		membershipModels = membershipModels[:limit]
	}

	result := make([]*models.PublicationMember, len(membershipModels))
	for i := range membershipModels {
		result[i] = &membershipModels[i]
	}

	return result, nextCursor, nil
}
