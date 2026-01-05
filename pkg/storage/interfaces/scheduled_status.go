// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ScheduledStatusRepository defines the interface for scheduled status operations.
// This handles creating, retrieving, and managing scheduled posts.
type ScheduledStatusRepository interface {
	// CreateScheduledStatus creates a new scheduled status
	CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error

	// GetScheduledStatus retrieves a scheduled status by ID
	GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error)

	// GetScheduledStatuses retrieves scheduled statuses for a user
	GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error)

	// UpdateScheduledStatus updates a scheduled status
	UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error

	// DeleteScheduledStatus deletes a scheduled status
	DeleteScheduledStatus(ctx context.Context, id string) error

	// GetDueScheduledStatuses retrieves scheduled statuses that are due to be published
	GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error)

	// MarkScheduledStatusPublished marks a scheduled status as published
	MarkScheduledStatusPublished(ctx context.Context, id string) error

	// GetScheduledStatusMedia gets media for scheduled status
	GetScheduledStatusMedia(ctx context.Context, id string) ([]*models.Media, error)

	// SetMediaRepository sets the media repository dependency
	SetMediaRepository(mediaRepo MediaRepositoryInterface)
}

// MediaRepositoryInterface defines the interface for media operations needed by scheduled status
type MediaRepositoryInterface interface {
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
}
