package repositories

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// SeveranceRepository handles severed relationship operations
type SeveranceRepository struct {
	*EnhancedBaseRepository[*models.SeveredRelationship]
	db     core.DB
	logger *zap.Logger
}

// NewSeveranceRepository creates a new SeveranceRepository instance
func NewSeveranceRepository(db core.DB, tableName string, logger *zap.Logger) *SeveranceRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	enhancedRepo := NewEnhancedBaseRepository[*models.SeveredRelationship](db, tableName, logger, nil, "SeveranceRepository", "severed_relationship")
	return &SeveranceRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		logger:                 logger,
	}
}

// SeveranceFilters defines filters for querying severed relationships
type SeveranceFilters struct {
	Instance string                 // Filter by remote instance
	Status   models.SeveranceStatus // Filter by status
	Reason   models.SeveranceReason // Filter by reason
}

// CreateSeveredRelationship creates a new severed relationship record
func (r *SeveranceRepository) CreateSeveredRelationship(ctx context.Context, severance *models.SeveredRelationship) error {
	if severance == nil {
		return fmt.Errorf("severed relationship cannot be nil")
	}

	// Update keys before saving
	if err := severance.UpdateKeys(); err != nil {
		return err
	}

	// Use Create to save
	err := r.db.WithContext(ctx).Model(severance).Create()
	if err != nil {
		r.logger.Error("failed to create severed relationship",
			zap.String("local_instance", severance.LocalInstance),
			zap.String("remote_instance", severance.RemoteInstance),
			zap.Error(err))
		return fmt.Errorf("failed to create severed relationship: %w", err)
	}

	r.logger.Debug("created severed relationship",
		zap.String("id", severance.ID),
		zap.String("remote_instance", severance.RemoteInstance),
		zap.String("reason", string(severance.Reason)))

	return nil
}

// GetSeveredRelationship retrieves a severed relationship by ID
func (r *SeveranceRepository) GetSeveredRelationship(ctx context.Context, id string) (*models.SeveredRelationship, error) {
	// ID format: localInstance_remoteInstance_timestamp
	parts := strings.Split(id, "_")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid severance ID format: %s", id)
	}

	localInstance := parts[0]
	remoteInstance := parts[1]

	// Query by PK and SK prefix.
	pk := fmt.Sprintf("SEVERED#%s", localInstance)
	skPrefix := fmt.Sprintf("INSTANCE#%s#", remoteInstance)

	// The keyed chain (PK partition + SK prefix) is read as a bounded page walk
	// (wave #1469, batch S1): Limit(500)/page, 100-page cap, fail-closed on
	// exhaustion. The consume callback stops at the first row whose ID matches,
	// preserving the old Scan's in-memory selection (first match wins).
	var found *models.SeveredRelationship
	err := walkKeyedPages(
		r.db.WithContext(ctx).Model(&models.SeveredRelationship{}).
			Where("PK", "=", pk).
			Where("SK", "begins_with", skPrefix),
		500, 100,
		func(page []*models.SeveredRelationship) (bool, error) {
			for _, s := range page {
				if s.ID == id {
					found = s
					return true, nil
				}
			}
			return false, nil
		},
	)

	if err != nil {
		r.logger.Error("failed to get severed relationship",
			zap.String("id", id),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntitySeveredRelationship, id)
	}

	// Find the matching ID
	if found != nil {
		return found, nil
	}

	return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntitySeveredRelationship, id)
}

// ListSeveredRelationships retrieves severed relationships with filters and pagination
func (r *SeveranceRepository) ListSeveredRelationships(ctx context.Context, localInstance string, filters SeveranceFilters, limit int, cursor string) ([]*models.SeveredRelationship, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var severances []*models.SeveredRelationship
	var err error

	// If filtering by status, use GSI1
	if filters.Status != "" {
		query := r.db.WithContext(ctx).Model(&models.SeveredRelationship{}).
			Index("gsi1").
			Where("gsi1PK", "=", fmt.Sprintf("STATUS#%s", filters.Status)).
			Where("gsi1SK", "begins_with", "TIMESTAMP#").
			OrderBy("gsi1SK", "DESC").
			Limit(limit + 1)

		if cursor != "" {
			query = query.Cursor(cursor)
		}

		err = query.All(&severances)
	} else if filters.Instance != "" {
		// Filter by specific instance
		pk := fmt.Sprintf("SEVERED#%s", localInstance)
		sk := fmt.Sprintf("INSTANCE#%s#", filters.Instance)

		query := r.db.WithContext(ctx).Model(&models.SeveredRelationship{}).
			Where("PK", "=", pk).
			Where("SK", "begins_with", sk).
			OrderBy("SK", "DESC").
			Limit(limit + 1)

		if cursor != "" {
			query = query.Cursor(cursor)
		}

		err = query.All(&severances)
	} else {
		// Query all for this local instance
		pk := fmt.Sprintf("SEVERED#%s", localInstance)

		query := r.db.WithContext(ctx).Model(&models.SeveredRelationship{}).
			Where("PK", "=", pk).
			Where("SK", "begins_with", "INSTANCE#").
			OrderBy("SK", "DESC").
			Limit(limit + 1)

		if cursor != "" {
			query = query.Cursor(cursor)
		}

		err = query.All(&severances)
	}

	if err != nil {
		if errors.IsNotFound(err) {
			return []*models.SeveredRelationship{}, "", nil
		}
		r.logger.Error("failed to list severed relationships",
			zap.String("local_instance", localInstance),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to list severed relationships: %w", err)
	}

	// Handle pagination
	nextCursor := ""
	if len(severances) > limit {
		nextCursor = severances[limit].SK
		severances = severances[:limit]
	}

	// Apply additional filters in memory if needed
	if filters.Reason != "" {
		filtered := make([]*models.SeveredRelationship, 0, len(severances))
		for _, s := range severances {
			if s.Reason == filters.Reason {
				filtered = append(filtered, s)
			}
		}
		severances = filtered
	}

	r.logger.Debug("listed severed relationships",
		zap.String("local_instance", localInstance),
		zap.Int("count", len(severances)),
		zap.String("next_cursor", nextCursor))

	return severances, nextCursor, nil
}

// UpdateSeveranceStatus updates the status of a severed relationship
func (r *SeveranceRepository) UpdateSeveranceStatus(ctx context.Context, id string, status models.SeveranceStatus) error {
	// Get the existing record
	severance, err := r.GetSeveredRelationship(ctx, id)
	if err != nil {
		return err
	}
	if severance == nil {
		return fmt.Errorf("severed relationship not found: %s", id)
	}

	// Update status
	severance.Status = status
	severance.UpdatedAt = time.Now()
	if status == models.SeveranceStatusAcknowledged {
		severance.Acknowledge()
	}

	// Update keys
	if err := severance.UpdateKeys(); err != nil {
		return err
	}

	// Save the updated record
	err = r.db.WithContext(ctx).Model(severance).CreateOrUpdate()
	if err != nil {
		r.logger.Error("failed to update severance status",
			zap.String("id", id),
			zap.String("status", string(status)),
			zap.Error(err))
		return fmt.Errorf("failed to update severance status: %w", err)
	}

	r.logger.Debug("updated severance status",
		zap.String("id", id),
		zap.String("status", string(status)))

	return nil
}

// CreateAffectedRelationship creates a new affected relationship record
func (r *SeveranceRepository) CreateAffectedRelationship(ctx context.Context, affected *models.AffectedRelationship) error {
	if affected == nil {
		return fmt.Errorf("affected relationship cannot be nil")
	}

	// Update keys before saving
	if err := affected.UpdateKeys(); err != nil {
		return err
	}

	err := r.db.WithContext(ctx).Model(affected).Create()
	if err != nil {
		r.logger.Error("failed to create affected relationship",
			zap.String("severance_id", affected.SeveranceID),
			zap.String("actor_id", affected.ActorID),
			zap.Error(err))
		return fmt.Errorf("failed to create affected relationship: %w", err)
	}

	r.logger.Debug("created affected relationship",
		zap.String("severance_id", affected.SeveranceID),
		zap.String("actor_id", affected.ActorID))

	return nil
}

// GetAffectedRelationships retrieves affected relationships for a severance
func (r *SeveranceRepository) GetAffectedRelationships(ctx context.Context, severanceID string, limit int, cursor string) ([]*models.AffectedRelationship, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	pk := fmt.Sprintf("SEVERED#%s", severanceID)

	query := r.db.WithContext(ctx).Model(&models.AffectedRelationship{}).
		Where("PK", "=", pk).
		Where("SK", "begins_with", "AFFECTED#").
		OrderBy("SK", "ASC").
		Limit(limit + 1)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var affected []*models.AffectedRelationship
	err := query.All(&affected)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*models.AffectedRelationship{}, "", nil
		}
		r.logger.Error("failed to get affected relationships",
			zap.String("severance_id", severanceID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get affected relationships: %w", err)
	}

	// Handle pagination
	nextCursor := ""
	if len(affected) > limit {
		nextCursor = affected[limit].SK
		affected = affected[:limit]
	}

	r.logger.Debug("retrieved affected relationships",
		zap.String("severance_id", severanceID),
		zap.Int("count", len(affected)),
		zap.String("next_cursor", nextCursor))

	return affected, nextCursor, nil
}

// CreateReconnectionAttempt creates a new reconnection attempt record
func (r *SeveranceRepository) CreateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error {
	if attempt == nil {
		return fmt.Errorf("reconnection attempt cannot be nil")
	}

	// Update keys before saving
	if err := attempt.UpdateKeys(); err != nil {
		return err
	}

	err := r.db.WithContext(ctx).Model(attempt).Create()
	if err != nil {
		r.logger.Error("failed to create reconnection attempt",
			zap.String("severance_id", attempt.SeveranceID),
			zap.String("id", attempt.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create reconnection attempt: %w", err)
	}

	r.logger.Debug("created reconnection attempt",
		zap.String("id", attempt.ID),
		zap.String("severance_id", attempt.SeveranceID))

	return nil
}

// UpdateReconnectionAttempt updates a reconnection attempt record
func (r *SeveranceRepository) UpdateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error {
	if attempt == nil {
		return fmt.Errorf("reconnection attempt cannot be nil")
	}

	// Update keys before saving
	if err := attempt.UpdateKeys(); err != nil {
		return err
	}

	err := r.db.WithContext(ctx).Model(attempt).CreateOrUpdate()
	if err != nil {
		r.logger.Error("failed to update reconnection attempt",
			zap.String("id", attempt.ID),
			zap.Error(err))
		return fmt.Errorf("failed to update reconnection attempt: %w", err)
	}

	r.logger.Debug("updated reconnection attempt",
		zap.String("id", attempt.ID),
		zap.String("status", attempt.Status))

	return nil
}

// GetReconnectionAttempt retrieves a reconnection attempt by ID
func (r *SeveranceRepository) GetReconnectionAttempt(ctx context.Context, severanceID, attemptID string) (*models.SeveranceReconnectionAttempt, error) {
	pk := fmt.Sprintf("SEVERED#%s", severanceID)
	sk := fmt.Sprintf("RECONNECT#%s", attemptID)

	var attempt models.SeveranceReconnectionAttempt
	err := r.db.WithContext(ctx).Model(&models.SeveranceReconnectionAttempt{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&attempt)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntitySeveredRelationship, attemptID)
		}
		r.logger.Error("failed to get reconnection attempt",
			zap.String("severance_id", severanceID),
			zap.String("attempt_id", attemptID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntitySeveredRelationship, attemptID)
	}

	return &attempt, nil
}

// GetReconnectionAttempts retrieves all reconnection attempts for a severance
func (r *SeveranceRepository) GetReconnectionAttempts(ctx context.Context, severanceID string) ([]*models.SeveranceReconnectionAttempt, error) {
	pk := fmt.Sprintf("SEVERED#%s", severanceID)

	// The keyed chain (PK partition + RECONNECT# SK prefix) is read as a bounded
	// page walk (wave #1469, batch S1): Limit(500)/page, 100-page cap, fail-closed
	// on exhaustion. OrderBy("SK","DESC") is preserved via cursor handoff, so the
	// accumulated page order matches the chain's declared ordering.
	var attempts []*models.SeveranceReconnectionAttempt
	err := walkKeyedPages(
		r.db.WithContext(ctx).Model(&models.SeveranceReconnectionAttempt{}).
			Where("PK", "=", pk).
			Where("SK", "begins_with", "RECONNECT#").
			OrderBy("SK", "DESC"),
		500, 100,
		func(page []*models.SeveranceReconnectionAttempt) (bool, error) {
			attempts = append(attempts, page...)
			return false, nil
		},
	)

	if err != nil {
		// Cap exhaustion fails the read closed BEFORE the pre-existing
		// not-found swallow — never an empty result on a capped walk.
		if stderrors.Is(err, errBoundedPageCapExceeded) {
			return nil, err
		}
		if errors.IsNotFound(err) {
			return []*models.SeveranceReconnectionAttempt{}, nil
		}
		r.logger.Error("failed to get reconnection attempts",
			zap.String("severance_id", severanceID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get reconnection attempts: %w", err)
	}

	r.logger.Debug("retrieved reconnection attempts",
		zap.String("severance_id", severanceID),
		zap.Int("count", len(attempts)))

	return attempts, nil
}
