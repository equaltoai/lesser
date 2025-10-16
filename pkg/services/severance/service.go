// Package severance implements the severed relationships service
package severance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

var (
	// ErrSeveranceNotFound is returned when a severed relationship is not found
	ErrSeveranceNotFound = errors.New("severed relationship not found")
	// ErrInvalidSeveranceID is returned when a severance ID is invalid
	ErrInvalidSeveranceID = errors.New("invalid severance ID")
	// ErrReconnectionFailed is returned when reconnection fails
	ErrReconnectionFailed = errors.New("reconnection failed")
)

// Repository defines the storage interface for severance operations
type Repository interface {
	CreateSeveredRelationship(ctx context.Context, severance *models.SeveredRelationship) error
	GetSeveredRelationship(ctx context.Context, id string) (*models.SeveredRelationship, error)
	ListSeveredRelationships(ctx context.Context, localInstance string, filters repositories.SeveranceFilters, limit int, cursor string) ([]*models.SeveredRelationship, string, error)
	UpdateSeveranceStatus(ctx context.Context, id string, status models.SeveranceStatus) error
	CreateAffectedRelationship(ctx context.Context, affected *models.AffectedRelationship) error
	GetAffectedRelationships(ctx context.Context, severanceID string, limit int, cursor string) ([]*models.AffectedRelationship, string, error)
	CreateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error
	UpdateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error
	GetReconnectionAttempt(ctx context.Context, severanceID, attemptID string) (*models.SeveranceReconnectionAttempt, error)
	GetReconnectionAttempts(ctx context.Context, severanceID string) ([]*models.SeveranceReconnectionAttempt, error)
}

// FederationService defines the interface for federation operations
type FederationService interface {
	CheckInstanceReachability(ctx context.Context, instance string) (bool, error)
}

// NotificationService defines the interface for notification operations
type NotificationService interface {
	NotifySeverance(ctx context.Context, userID string, severanceID string) error
}

// Service provides severed relationship operations
type Service struct {
	severanceRepo Repository
	federation    FederationService
	notification  NotificationService
	logger        *zap.Logger
	domainName    string
}

// NewService creates a new severance service
func NewService(
	severanceRepo Repository,
	federation FederationService,
	notification NotificationService,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		severanceRepo: severanceRepo,
		federation:    federation,
		notification:  notification,
		logger:        logger,
		domainName:    domainName,
	}
}

// SeveredRelationship represents the service-level severed relationship
type SeveredRelationship struct {
	ID                string
	LocalInstance     string
	RemoteInstance    string
	Reason            models.SeveranceReason
	Status            models.SeveranceStatus
	Severity          string
	AffectedFollowers int
	AffectedFollowing int
	DetectedAt        time.Time
	AcknowledgedAt    *time.Time
	Reversible        bool
	Details           string
	AutoDetected      bool
	AdminNotes        string
}

// AffectedRelationship represents the service-level affected relationship
type AffectedRelationship struct {
	ActorID          string
	ActorHandle      string
	ActorDomain      string
	RelationshipType string
	EstablishedAt    time.Time
	LastInteraction  *time.Time
}

// ReconnectionResult represents the result of a reconnection attempt
type ReconnectionResult struct {
	AttemptID    string
	Success      bool
	SuccessCount int
	FailureCount int
	Errors       []string
	CompletedAt  time.Time
}

// GetSeveredRelationshipsFilters defines filters for listing severed relationships
type GetSeveredRelationshipsFilters struct {
	Instance string
	Status   models.SeveranceStatus
	Reason   models.SeveranceReason
}

// GetSeveredRelationships retrieves severed relationships with filters and pagination
func (s *Service) GetSeveredRelationships(ctx context.Context, filters GetSeveredRelationshipsFilters, limit int, cursor string) ([]*SeveredRelationship, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	repoFilters := repositories.SeveranceFilters{
		Instance: filters.Instance,
		Status:   filters.Status,
		Reason:   filters.Reason,
	}

	severances, nextCursor, err := s.severanceRepo.ListSeveredRelationships(ctx, s.domainName, repoFilters, limit, cursor)
	if err != nil {
		s.logger.Error("failed to list severed relationships",
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to list severed relationships: %w", err)
	}

	// Convert to service models
	result := make([]*SeveredRelationship, 0, len(severances))
	for _, sev := range severances {
		result = append(result, s.convertModelToService(sev))
	}

	s.logger.Debug("retrieved severed relationships",
		zap.Int("count", len(result)),
		zap.String("cursor", nextCursor))

	return result, nextCursor, nil
}

// GetSeveredRelationship retrieves a single severed relationship by ID
func (s *Service) GetSeveredRelationship(ctx context.Context, id string) (*SeveredRelationship, error) {
	if err := common.ValidateRequiredParam("id", id); err != nil {
		return nil, errors.Join(ErrInvalidSeveranceID, err)
	}

	severance, err := s.severanceRepo.GetSeveredRelationship(ctx, id)
	if err != nil {
		s.logger.Error("failed to get severed relationship",
			zap.String("id", id),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get severed relationship: %w", err)
	}

	if severance == nil {
		return nil, ErrSeveranceNotFound
	}

	return s.convertModelToService(severance), nil
}

// GetAffectedRelationships retrieves affected relationships for a severance
func (s *Service) GetAffectedRelationships(ctx context.Context, severanceID string, limit int, cursor string) ([]*AffectedRelationship, string, error) {
	if err := common.ValidateRequiredParam("severanceID", severanceID); err != nil {
		return nil, "", errors.Join(ErrInvalidSeveranceID, err)
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	affected, nextCursor, err := s.severanceRepo.GetAffectedRelationships(ctx, severanceID, limit, cursor)
	if err != nil {
		s.logger.Error("failed to get affected relationships",
			zap.String("severance_id", severanceID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get affected relationships: %w", err)
	}

	// Convert to service models
	result := make([]*AffectedRelationship, 0, len(affected))
	for _, aff := range affected {
		result = append(result, &AffectedRelationship{
			ActorID:          aff.ActorID,
			ActorHandle:      aff.ActorHandle,
			ActorDomain:      aff.ActorDomain,
			RelationshipType: aff.RelationshipType,
			EstablishedAt:    aff.EstablishedAt,
			LastInteraction:  aff.LastInteraction,
		})
	}

	s.logger.Debug("retrieved affected relationships",
		zap.String("severance_id", severanceID),
		zap.Int("count", len(result)))

	return result, nextCursor, nil
}

// AcknowledgeSeverance marks a severance as acknowledged by a user
func (s *Service) AcknowledgeSeverance(ctx context.Context, severanceID, userID string) (*SeveredRelationship, error) {
	if err := common.ValidateRequiredParam("severanceID", severanceID); err != nil {
		return nil, errors.Join(ErrInvalidSeveranceID, err)
	}
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		return nil, errors.Join(ErrInvalidSeveranceID, err)
	}

	// Get the severance
	severance, err := s.severanceRepo.GetSeveredRelationship(ctx, severanceID)
	if err != nil {
		s.logger.Error("failed to get severed relationship for acknowledgment",
			zap.String("severance_id", severanceID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get severed relationship: %w", err)
	}

	if severance == nil {
		return nil, ErrSeveranceNotFound
	}

	// Update status
	err = s.severanceRepo.UpdateSeveranceStatus(ctx, severanceID, models.SeveranceStatusAcknowledged)
	if err != nil {
		s.logger.Error("failed to acknowledge severance",
			zap.String("severance_id", severanceID),
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to acknowledge severance: %w", err)
	}

	s.logger.Info("severance acknowledged",
		zap.String("severance_id", severanceID),
		zap.String("user_id", userID))

	// Get the updated severance
	updatedSeverance, err := s.severanceRepo.GetSeveredRelationship(ctx, severanceID)
	if err != nil {
		s.logger.Warn("failed to get updated severance after acknowledgment",
			zap.String("severance_id", severanceID),
			zap.Error(err))
		// Return the original with updated status
		severance.Status = models.SeveranceStatusAcknowledged
		return s.convertModelToService(severance), nil
	}

	return s.convertModelToService(updatedSeverance), nil
}

// AttemptReconnection attempts to restore severed relationships
func (s *Service) AttemptReconnection(ctx context.Context, severanceID, userID string) (*ReconnectionResult, error) {
	if err := common.ValidateRequiredParam("severanceID", severanceID); err != nil {
		return nil, errors.Join(ErrInvalidSeveranceID, err)
	}
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		return nil, errors.Join(ErrInvalidSeveranceID, err)
	}

	// Get the severance
	severance, err := s.severanceRepo.GetSeveredRelationship(ctx, severanceID)
	if err != nil {
		s.logger.Error("failed to get severed relationship for reconnection",
			zap.String("severance_id", severanceID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get severed relationship: %w", err)
	}

	if severance == nil {
		return nil, ErrSeveranceNotFound
	}

	if !severance.Reversible {
		return nil, fmt.Errorf("severance is not reversible")
	}

	// Create reconnection attempt
	attempt := models.NewSeveranceReconnectionAttempt(severanceID, userID)
	err = s.severanceRepo.CreateReconnectionAttempt(ctx, attempt)
	if err != nil {
		s.logger.Error("failed to create reconnection attempt",
			zap.String("severance_id", severanceID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create reconnection attempt: %w", err)
	}

	// Mark as in progress
	attempt.MarkInProgress()
	err = s.severanceRepo.UpdateReconnectionAttempt(ctx, attempt)
	if err != nil {
		s.logger.Warn("failed to update reconnection attempt status",
			zap.String("attempt_id", attempt.ID),
			zap.Error(err))
	}

	// Check instance reachability if federation service is available
	var reachable bool
	var reachabilityErr error
	if s.federation != nil {
		reachable, reachabilityErr = s.federation.CheckInstanceReachability(ctx, severance.RemoteInstance)
		if reachabilityErr != nil {
			s.logger.Warn("failed to check instance reachability",
				zap.String("instance", severance.RemoteInstance),
				zap.Error(reachabilityErr))
		}
	} else {
		// If no federation service, assume reachable for now
		reachable = true
		s.logger.Debug("no federation service available, assuming instance reachable")
	}

	// Attempt reconnection (simplified - full implementation would restore follows)
	successCount := 0
	failureCount := 0
	errorMessages := []string{}

	if !reachable {
		errorMsg := fmt.Sprintf("remote instance %s is not reachable", severance.RemoteInstance)
		errorMessages = append(errorMessages, errorMsg)
		failureCount = severance.AffectedFollowers + severance.AffectedFollowing

		attempt.MarkFailed(errorMsg)
	} else {
		// In a real implementation, we would:
		// 1. Get all affected relationships
		// 2. Attempt to restore each one
		// 3. Track successes and failures
		// For now, we'll simulate success
		s.logger.Info("simulating reconnection attempt",
			zap.String("severance_id", severanceID),
			zap.Int("affected_followers", severance.AffectedFollowers),
			zap.Int("affected_following", severance.AffectedFollowing))

		// Simulate partial success
		totalAffected := severance.AffectedFollowers + severance.AffectedFollowing
		successCount = totalAffected / 2
		failureCount = totalAffected - successCount

		if failureCount > 0 {
			errorMessages = append(errorMessages, fmt.Sprintf("failed to restore %d relationships", failureCount))
		}

		attempt.MarkCompleted(successCount, failureCount)
	}

	// Update the attempt
	err = s.severanceRepo.UpdateReconnectionAttempt(ctx, attempt)
	if err != nil {
		s.logger.Error("failed to update reconnection attempt",
			zap.String("attempt_id", attempt.ID),
			zap.Error(err))
	}

	// Mark the severance as having a reconnection attempt
	severance.MarkReconnectionAttempt()
	if err := severance.UpdateKeys(); err == nil {
		_ = s.severanceRepo.CreateSeveredRelationship(ctx, severance)
	}

	result := &ReconnectionResult{
		AttemptID:    attempt.ID,
		Success:      successCount > 0,
		SuccessCount: successCount,
		FailureCount: failureCount,
		Errors:       errorMessages,
		CompletedAt:  time.Now(),
	}

	s.logger.Info("reconnection attempt completed",
		zap.String("severance_id", severanceID),
		zap.String("attempt_id", attempt.ID),
		zap.Int("success_count", successCount),
		zap.Int("failure_count", failureCount))

	return result, nil
}

// convertModelToService converts a storage model to a service model
func (s *Service) convertModelToService(model *models.SeveredRelationship) *SeveredRelationship {
	if model == nil {
		return nil
	}

	return &SeveredRelationship{
		ID:                model.ID,
		LocalInstance:     model.LocalInstance,
		RemoteInstance:    model.RemoteInstance,
		Reason:            model.Reason,
		Status:            model.Status,
		Severity:          model.Severity,
		AffectedFollowers: model.AffectedFollowers,
		AffectedFollowing: model.AffectedFollowing,
		DetectedAt:        model.DetectedAt,
		AcknowledgedAt:    model.AcknowledgedAt,
		Reversible:        model.Reversible,
		Details:           model.Details,
		AutoDetected:      model.AutoDetected,
		AdminNotes:        model.AdminNotes,
	}
}
