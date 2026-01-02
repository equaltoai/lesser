package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// SocialRecoveryService handles account recovery through trusted contacts
type SocialRecoveryService struct {
	repo       socialRecoveryRepository
	logger     *zap.Logger
	fedService socialRecoveryFederationService
}

type socialRecoveryRepository interface {
	StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error
	DeleteTrustee(ctx context.Context, username, trusteeActorID string) error
	GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error)
	StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error
	GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error)
	UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error
	StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error
}

type socialRecoveryFederationService interface {
	SendTrusteeInvitation(ctx context.Context, fromUser string, trusteeActorID string) error
	SendRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest, trusteeActorID string) error
	SendRecoveryApprovalNotification(ctx context.Context, username string, recoveryToken string) error
}

// NewSocialRecoveryService creates a new social recovery service
func NewSocialRecoveryService(repos StorageProvider, logger *zap.Logger) *SocialRecoveryService {
	return &SocialRecoveryService{
		repo:   repos.Recovery(),
		logger: logger,
		// fedService is optional and can be set separately
	}
}

// SetFederationService sets the federation service for sending notifications
func (s *SocialRecoveryService) SetFederationService(fedService socialRecoveryFederationService) {
	s.fedService = fedService
}

// We use TrusteeConfig and SocialRecoveryRequest types from storage package

// AddTrustee adds a trusted contact for social recovery
func (s *SocialRecoveryService) AddTrustee(ctx context.Context, username, trusteeActorID string) error {
	// Validate trustee exists (could be remote actor)
	if err := common.ValidateRequiredParam("trusteeActorID", trusteeActorID); err != nil {
		return ErrTrusteeActorIDRequired
	}

	// Store trustee configuration
	trustee := &storage.TrusteeConfig{
		Username:  username,
		ActorID:   trusteeActorID,
		AddedAt:   time.Now(),
		Confirmed: false,
	}

	// Store in DynamoDB
	if err := s.repo.StoreTrustee(ctx, username, trustee); err != nil {
		s.logger.Error("failed to store trustee",
			zap.String("username", username),
			zap.String("trustee", trusteeActorID),
			zap.Error(err))
		return errors.Join(ErrTrusteeStorage, err)
	}

	s.logger.Info("adding recovery trustee",
		zap.String("username", username),
		zap.String("trustee", trusteeActorID))

	// Send notification to trustee via ActivityPub
	return s.notifyTrusteeAdded(ctx, username, trusteeActorID)
}

// RemoveTrustee removes a trusted contact
func (s *SocialRecoveryService) RemoveTrustee(ctx context.Context, username, trusteeActorID string) error {
	// Delete from DynamoDB
	if err := s.repo.DeleteTrustee(ctx, username, trusteeActorID); err != nil {
		s.logger.Error("failed to delete trustee",
			zap.String("username", username),
			zap.String("trustee", trusteeActorID),
			zap.Error(err))
		return errors.Join(ErrTrusteeDeletion, err)
	}

	s.logger.Info("removing recovery trustee",
		zap.String("username", username),
		zap.String("trustee", trusteeActorID))

	return nil
}

// GetTrustees returns all trustees for a user
func (s *SocialRecoveryService) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	return s.repo.GetTrustees(ctx, username)
}

// InitiateRecovery starts the social recovery process
func (s *SocialRecoveryService) InitiateRecovery(ctx context.Context, username string) (*storage.SocialRecoveryRequest, error) {
	// Get user's trustees
	trustees, err := s.GetTrustees(ctx, username)
	if err != nil {
		s.logger.Error("failed to get trustees",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(ErrTrusteeRetrieval, err)
	}

	if len(trustees) < 2 {
		return nil, ErrInsufficientTrustees
	}

	// Generate recovery request ID
	requestID := s.generateRecoveryID()

	// Generate recovery token (used after approval)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		s.logger.Error("failed to generate recovery token",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(ErrRecoveryTokenGeneration, err)
	}
	recoveryToken := base64.URLEncoding.EncodeToString(tokenBytes)

	// Calculate required votes (majority of trustees)
	requiredVotes := (len(trustees) / 2) + 1
	if requiredVotes < 2 {
		requiredVotes = 2
	}

	// Create recovery request
	request := &storage.SocialRecoveryRequest{
		ID:            requestID,
		Username:      username,
		InitiatedAt:   time.Now(),
		ExpiresAt:     time.Now().Add(48 * time.Hour), // 48 hour window
		RequiredVotes: requiredVotes,
		ReceivedVotes: 0,
		TrusteeVotes:  []string{},
		RecoveryToken: recoveryToken,
		Status:        "pending",
	}

	// Store recovery request
	if err := s.repo.StoreRecoveryRequest(ctx, request); err != nil {
		s.logger.Error("failed to store recovery request",
			zap.String("username", username),
			zap.String("request_id", requestID),
			zap.Error(err))
		return nil, errors.Join(ErrRecoveryRequestStorage, err)
	}

	// Notify all trustees
	for _, trustee := range trustees {
		if trustee.Confirmed {
			if err := s.sendRecoveryRequest(ctx, request, trustee.ActorID); err != nil {
				s.logger.Error("failed to notify trustee",
					zap.String("trustee", trustee.ActorID),
					zap.Error(err))
			}
		}
	}

	s.logger.Info("initiated social recovery",
		zap.String("username", username),
		zap.String("request_id", requestID),
		zap.Int("trustees", len(trustees)),
		zap.Int("required_votes", requiredVotes))

	return request, nil
}

// ConfirmRecovery processes a trustee's confirmation
func (s *SocialRecoveryService) ConfirmRecovery(ctx context.Context, requestID, trusteeActorID string) error {
	// Get recovery request
	request, err := s.repo.GetRecoveryRequest(ctx, requestID)
	if err != nil {
		s.logger.Error("failed to get recovery request",
			zap.String("request_id", requestID),
			zap.String("trustee", trusteeActorID),
			zap.Error(err))
		return errors.Join(ErrRecoveryRequestRetrieval, err)
	}

	if request == nil {
		return ErrRecoveryRequestNotFound
	}

	if request.Status != "pending" {
		return ErrRecoveryRequestNotPending
	}

	if time.Now().After(request.ExpiresAt) {
		request.Status = "expired"
		if updateErr := s.repo.UpdateRecoveryRequest(ctx, request); updateErr != nil {
			// Log the error but still return the expiration error as primary
			s.logger.Error("failed to update expired recovery request",
				zap.String("request_id", requestID),
				zap.Error(updateErr))
		}
		return ErrRecoveryRequestExpired
	}

	// Check if trustee already voted
	for _, voter := range request.TrusteeVotes {
		if voter == trusteeActorID {
			return ErrTrusteeAlreadyVoted
		}
	}

	// Record vote
	request.TrusteeVotes = append(request.TrusteeVotes, trusteeActorID)
	request.ReceivedVotes++

	// Check if we have enough votes
	if request.ReceivedVotes >= request.RequiredVotes {
		request.Status = "approved"

		// Enable recovery token for password reset
		if err := s.enableRecoveryToken(ctx, request); err != nil {
			s.logger.Error("failed to enable recovery token",
				zap.String("request_id", requestID),
				zap.String("username", request.Username),
				zap.Error(err))
			return errors.Join(ErrRecoveryTokenStorage, err)
		}

		// Notify user (if they have other auth methods)
		if err := s.notifyRecoveryApproved(ctx, request); err != nil {
			s.logger.Warn("failed to send recovery approval notification", zap.Error(err))
		}
	}

	// Update request in DynamoDB
	if err := s.repo.UpdateRecoveryRequest(ctx, request); err != nil {
		s.logger.Error("failed to update recovery request",
			zap.String("request_id", requestID),
			zap.String("trustee", trusteeActorID),
			zap.Error(err))
		return errors.Join(ErrRecoveryRequestUpdate, err)
	}

	s.logger.Info("recovery vote recorded",
		zap.String("request_id", requestID),
		zap.String("trustee", trusteeActorID),
		zap.Int("votes", request.ReceivedVotes),
		zap.Int("required", request.RequiredVotes))

	return nil
}

// Helper methods

func (s *SocialRecoveryService) generateRecoveryID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based ID on crypto error (should not happen)
		return fmt.Sprintf("recovery_%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}

func (s *SocialRecoveryService) notifyTrusteeAdded(ctx context.Context, username, trusteeActorID string) error {
	// Use federation service if available
	if s.fedService != nil {
		return s.fedService.SendTrusteeInvitation(ctx, username, trusteeActorID)
	}

	// Fallback: log the notification
	s.logger.Info("trustee invitation (federation service not available)",
		zap.String("username", username),
		zap.String("trustee", trusteeActorID))

	return nil
}

func (s *SocialRecoveryService) sendRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest, trusteeActorID string) error {
	// Use federation service if available
	if s.fedService != nil {
		return s.fedService.SendRecoveryRequest(ctx, request, trusteeActorID)
	}

	// Fallback: log the request
	s.logger.Info("recovery request notification (federation service not available)",
		zap.String("request_id", request.ID),
		zap.String("username", request.Username),
		zap.String("trustee", trusteeActorID))

	return nil
}

func (s *SocialRecoveryService) enableRecoveryToken(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	// Store recovery token with 24 hour expiration
	recoveryData := map[string]any{
		"username":  request.Username,
		"token":     request.RecoveryToken,
		"type":      "social_recovery",
		"expiresAt": time.Now().Add(24 * time.Hour).Unix(),
		"used":      false,
	}

	recoveryKey := fmt.Sprintf("RECOVERY#%s", request.RecoveryToken)

	// Store recovery token using existing recovery token storage
	return s.repo.StoreRecoveryToken(ctx, recoveryKey, recoveryData)
}

func (s *SocialRecoveryService) notifyRecoveryApproved(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	// Use federation service if available
	if s.fedService != nil {
		return s.fedService.SendRecoveryApprovalNotification(ctx, request.Username, request.RecoveryToken)
	}

	// Fallback: log the approval
	s.logger.Info("recovery approved, notifying user",
		zap.String("username", request.Username),
		zap.String("request_id", request.ID))
	return nil
}
