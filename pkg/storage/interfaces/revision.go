// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// RevisionRepository defines the interface for revision operations.
// This handles CMS revision management for version history tracking.
type RevisionRepository interface {
	// ===== Core CRUD Operations =====

	// CreateRevision creates a new revision
	CreateRevision(ctx context.Context, revision *models.Revision) error

	// GetRevision retrieves a revision by object ID and version
	GetRevision(ctx context.Context, objectID string, version int) (*models.Revision, error)

	// Delete deletes a revision by PK and SK
	Delete(ctx context.Context, pk, sk string) error

	// ===== List Operations =====

	// ListRevisions lists revisions for an object
	ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error)

	// ListRevisionsPaginated lists revisions for an object with cursor pagination
	ListRevisionsPaginated(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Revision, string, error)
}
