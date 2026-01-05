// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// SeveranceFilters defines filters for querying severed relationships
type SeveranceFilters struct {
	Instance string                 // Filter by remote instance
	Status   models.SeveranceStatus // Filter by status
	Reason   models.SeveranceReason // Filter by reason
}

// SeveranceRepository defines the interface for severed relationship operations.
// This handles tracking and managing severed federation relationships.
type SeveranceRepository interface {
	// ===== Severed Relationship Operations =====

	// CreateSeveredRelationship creates a new severed relationship record
	CreateSeveredRelationship(ctx context.Context, severance *models.SeveredRelationship) error

	// GetSeveredRelationship retrieves a severed relationship by ID
	GetSeveredRelationship(ctx context.Context, id string) (*models.SeveredRelationship, error)

	// ListSeveredRelationships retrieves severed relationships with filters and pagination
	ListSeveredRelationships(ctx context.Context, localInstance string, filters SeveranceFilters, limit int, cursor string) ([]*models.SeveredRelationship, string, error)

	// UpdateSeveranceStatus updates the status of a severed relationship
	UpdateSeveranceStatus(ctx context.Context, id string, status models.SeveranceStatus) error

	// ===== Affected Relationship Operations =====

	// CreateAffectedRelationship creates a new affected relationship record
	CreateAffectedRelationship(ctx context.Context, affected *models.AffectedRelationship) error

	// GetAffectedRelationships retrieves affected relationships for a severance
	GetAffectedRelationships(ctx context.Context, severanceID string, limit int, cursor string) ([]*models.AffectedRelationship, string, error)

	// ===== Reconnection Attempt Operations =====

	// CreateReconnectionAttempt creates a new reconnection attempt record
	CreateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error

	// UpdateReconnectionAttempt updates a reconnection attempt record
	UpdateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error

	// GetReconnectionAttempt retrieves a reconnection attempt by ID
	GetReconnectionAttempt(ctx context.Context, severanceID, attemptID string) (*models.SeveranceReconnectionAttempt, error)

	// GetReconnectionAttempts retrieves all reconnection attempts for a severance
	GetReconnectionAttempts(ctx context.Context, severanceID string) ([]*models.SeveranceReconnectionAttempt, error)
}
