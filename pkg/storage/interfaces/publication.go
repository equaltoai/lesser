// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/pay-theory/dynamorm/pkg/core"
)

// PublicationRepository defines the interface for publication operations.
// This handles CMS publication management for multi-contributor blogs/newsletters.
type PublicationRepository interface {
	// ===== Database Access =====

	// GetDB returns the underlying DynamoDB connection for advanced operations
	GetDB() dynamormcore.DB

	// ===== Core CRUD Operations =====

	// CreatePublication creates a new publication
	CreatePublication(ctx context.Context, publication *models.Publication) error

	// GetPublication retrieves a publication by ID
	GetPublication(ctx context.Context, id string) (*models.Publication, error)

	// Update updates an existing publication
	Update(ctx context.Context, publication *models.Publication) error

	// Delete deletes a publication by PK and SK
	Delete(ctx context.Context, pk, sk string) error
}
