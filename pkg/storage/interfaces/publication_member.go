// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// PublicationMemberRepository defines the interface for publication member operations.
// This handles CMS publication membership management for contributors.
type PublicationMemberRepository interface {
	// ===== Core CRUD Operations =====

	// CreateMember adds a new member to a publication
	CreateMember(ctx context.Context, member *models.PublicationMember) error

	// GetMember retrieves a member by publication ID and user ID
	GetMember(ctx context.Context, publicationID, userID string) (*models.PublicationMember, error)

	// Update updates an existing publication member
	Update(ctx context.Context, member *models.PublicationMember) error

	// DeleteMember removes a member from a publication
	DeleteMember(ctx context.Context, publicationID, userID string) error

	// ===== List Operations =====

	// ListMembers lists all members of a publication
	ListMembers(ctx context.Context, publicationID string) ([]*models.PublicationMember, error)

	// ListMembershipsForUserPaginated lists publications a user is a member of with pagination
	ListMembershipsForUserPaginated(ctx context.Context, userID string, limit int, cursor string) ([]*models.PublicationMember, string, error)
}
