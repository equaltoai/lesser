// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// DraftRepository defines the interface for draft operations.
// This handles CMS draft management including CRUD operations and scheduling.
type DraftRepository interface {
	// ===== Core CRUD Operations =====

	// CreateDraft creates a new draft
	CreateDraft(ctx context.Context, draft *models.Draft) error

	// GetDraft retrieves a draft by author ID and draft ID
	GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error)

	// UpdateDraft updates an existing draft owned by authorID
	UpdateDraft(ctx context.Context, authorID string, draft *models.Draft) error

	// UpdateDraftEditorialMedia replaces only the draft's editorial-media association and update timestamp.
	UpdateDraftEditorialMedia(ctx context.Context, authorID string, draft *models.Draft) error

	// DeleteDraft deletes a draft
	DeleteDraft(ctx context.Context, authorID, draftID string) error

	// ===== List Operations =====

	// ListDraftsByAuthor lists drafts for an author
	ListDraftsByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Draft, error)

	// ListDraftsByAuthorPaginated lists drafts for an author with cursor pagination
	ListDraftsByAuthorPaginated(ctx context.Context, authorID string, limit int, cursor string) ([]*models.Draft, string, error)

	// ===== Scheduled Operations =====

	// ListScheduledDraftsDuePaginated lists drafts scheduled to publish at or before the provided time
	ListScheduledDraftsDuePaginated(ctx context.Context, dueBefore time.Time, limit int, cursor string) ([]*models.Draft, string, error)
}
