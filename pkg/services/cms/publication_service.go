package cms

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

type publicationRepository interface {
	GetDB() dynamormcore.DB
	CreatePublication(ctx context.Context, publication *models.Publication) error
	GetPublication(ctx context.Context, id string) (*models.Publication, error)
	Update(ctx context.Context, publication *models.Publication) error
	Delete(ctx context.Context, pk, sk string) error
}

type publicationMemberRepository interface {
	CreateMember(ctx context.Context, member *models.PublicationMember) error
	DeleteMember(ctx context.Context, publicationID, userID string) error
	GetMember(ctx context.Context, publicationID, userID string) (*models.PublicationMember, error)
	Update(ctx context.Context, member *models.PublicationMember) error
	ListMembers(ctx context.Context, publicationID string) ([]*models.PublicationMember, error)
}

// PublicationService handles business logic for publications
type PublicationService struct {
	pubRepo       publicationRepository
	pubMemberRepo publicationMemberRepository
	logger        *zap.Logger
}

// NewPublicationService creates a new PublicationService
func NewPublicationService(pubRepo publicationRepository, pubMemberRepo publicationMemberRepository, logger *zap.Logger) *PublicationService {
	return &PublicationService{
		pubRepo:       pubRepo,
		pubMemberRepo: pubMemberRepo,
		logger:        logger,
	}
}

// CreatePublication creates a new publication
func (s *PublicationService) CreatePublication(ctx context.Context, publication *models.Publication) error {
	if publication == nil {
		return errors.New("publication is required")
	}

	slug := strings.TrimSpace(publication.Slug)
	if slug == "" {
		return apperrors.ValidationFailedWithField("slug")
	}
	publication.Slug = slug

	publicationID := strings.TrimSpace(publication.ID)
	if publicationID == "" {
		return apperrors.ValidationFailedWithField("id")
	}

	s.logger.Info("creating publication", zap.String("name", publication.Name))

	if publication.CreatedAt.IsZero() {
		publication.CreatedAt = time.Now()
	}
	publication.UpdatedAt = time.Now()

	host := cmsHostFromURL(publicationID)
	if host != "" {
		legacyID := common.GenerateObjectID(host, "publications", slug)
		if legacyID != "" && !strings.EqualFold(legacyID, publicationID) {
			_, err := s.pubRepo.GetPublication(ctx, legacyID)
			if err == nil {
				return apperrors.ItemAlreadyExistsWithID("publication slug", slug)
			}
			if err != nil && !apperrors.HasCode(err, apperrors.CodeNotFound) {
				return err
			}
		}
	}

	slugCreated, err := cmsEnsurePublicationSlugIndex(ctx, s.pubRepo.GetDB(), slug, publicationID)
	if err != nil {
		return err
	}

	if err := s.pubRepo.CreatePublication(ctx, publication); err != nil {
		if slugCreated {
			cmsDeletePublicationSlugIndex(ctx, s.pubRepo.GetDB(), slug)
		}
		return err
	}

	return nil
}

// GetPublication retrieves a publication by ID
func (s *PublicationService) GetPublication(ctx context.Context, id string) (*models.Publication, error) {
	return s.pubRepo.GetPublication(ctx, id)
}

// UpdatePublication updates an existing publication
func (s *PublicationService) UpdatePublication(ctx context.Context, publication *models.Publication) error {
	if publication == nil {
		return errors.New("publication is required")
	}

	slug := strings.TrimSpace(publication.Slug)
	if slug == "" {
		return apperrors.ValidationFailedWithField("slug")
	}
	publication.Slug = slug

	publicationID := strings.TrimSpace(publication.ID)
	if publicationID == "" {
		return apperrors.ValidationFailedWithField("id")
	}

	s.logger.Info("updating publication", zap.String("id", publication.ID))

	host := cmsHostFromURL(publicationID)
	if host != "" {
		legacyID := common.GenerateObjectID(host, "publications", slug)
		if legacyID != "" && !strings.EqualFold(legacyID, publicationID) {
			_, err := s.pubRepo.GetPublication(ctx, legacyID)
			if err == nil {
				return apperrors.ItemAlreadyExistsWithID("publication slug", slug)
			}
			if err != nil && !apperrors.HasCode(err, apperrors.CodeNotFound) {
				return err
			}
		}
	}

	slugCreated, err := cmsEnsurePublicationSlugIndex(ctx, s.pubRepo.GetDB(), slug, publicationID)
	if err != nil {
		return err
	}

	publication.UpdatedAt = time.Now()
	if err := s.pubRepo.Update(ctx, publication); err != nil {
		if slugCreated {
			cmsDeletePublicationSlugIndex(ctx, s.pubRepo.GetDB(), slug)
		}
		return err
	}

	return nil
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
