package cms

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// PublicationService handles business logic for publications
type PublicationService struct {
	pubRepo       *repositories.PublicationRepository
	pubMemberRepo *repositories.PublicationMemberRepository
	logger        *zap.Logger
}

// NewPublicationService creates a new PublicationService
func NewPublicationService(pubRepo *repositories.PublicationRepository, pubMemberRepo *repositories.PublicationMemberRepository, logger *zap.Logger) *PublicationService {
	return &PublicationService{
		pubRepo:       pubRepo,
		pubMemberRepo: pubMemberRepo,
		logger:        logger,
	}
}

// CreatePublication creates a new publication
func (s *PublicationService) CreatePublication(ctx context.Context, publication *models.Publication) error {
	s.logger.Info("creating publication", zap.String("name", publication.Name))
	
	if publication.CreatedAt.IsZero() {
		publication.CreatedAt = time.Now()
	}
	publication.UpdatedAt = time.Now()

	return s.pubRepo.CreatePublication(ctx, publication)
}

// GetPublication retrieves a publication by ID
func (s *PublicationService) GetPublication(ctx context.Context, id string) (*models.Publication, error) {
	return s.pubRepo.GetPublication(ctx, id)
}

// UpdatePublication updates an existing publication
func (s *PublicationService) UpdatePublication(ctx context.Context, publication *models.Publication) error {
	s.logger.Info("updating publication", zap.String("id", publication.ID))
	publication.UpdatedAt = time.Now()
	return s.pubRepo.Update(ctx, publication)
}

// DeletePublication deletes a publication
func (s *PublicationService) DeletePublication(ctx context.Context, id string) error {
	s.logger.Info("deleting publication", zap.String("id", id))
	pk := "PUBLICATION#" + id
	sk := "METADATA"
	return s.pubRepo.Delete(ctx, pk, sk)
}

// AddMember adds a member to a publication
func (s *PublicationService) AddMember(ctx context.Context, member *models.PublicationMember) error {
	s.logger.Info("adding member to publication", zap.String("userID", member.UserID), zap.String("pubID", member.PublicationID))
	
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}
	member.CreatedAt = time.Now()
	member.UpdatedAt = time.Now()

	return s.pubMemberRepo.CreateMember(ctx, member)
}

// RemoveMember removes a member from a publication
func (s *PublicationService) RemoveMember(ctx context.Context, publicationID, userID string) error {
	s.logger.Info("removing member from publication", zap.String("userID", userID), zap.String("pubID", publicationID))
	return s.pubMemberRepo.DeleteMember(ctx, publicationID, userID)
}

// UpdateMemberRole updates a member's role
func (s *PublicationService) UpdateMemberRole(ctx context.Context, publicationID, userID, role string) error {
	s.logger.Info("updating member role", zap.String("userID", userID), zap.String("role", role))
	
	member, err := s.pubMemberRepo.GetMember(ctx, publicationID, userID)
	if err != nil {
		return err
	}

	member.Role = role
	member.UpdatedAt = time.Now()

	return s.pubMemberRepo.Update(ctx, member)
}

// GetMember retrieves a specific member
func (s *PublicationService) GetMember(ctx context.Context, publicationID, userID string) (*models.PublicationMember, error) {
	return s.pubMemberRepo.GetMember(ctx, publicationID, userID)
}

// ListMembers lists all members of a publication
func (s *PublicationService) ListMembers(ctx context.Context, publicationID string) ([]*models.PublicationMember, error) {
	return s.pubMemberRepo.ListMembers(ctx, publicationID)
}