package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// SocialRecoveryService handles account recovery through trusted contacts
type SocialRecoveryService struct {
	repos      core.RepositoryStorage
	logger     *zap.Logger
	fedService *RecoveryFederationService
}

// NewSocialRecoveryService creates a new social recovery service
func NewSocialRecoveryService(repos core.RepositoryStorage, logger *zap.Logger) *SocialRecoveryService {
	return &SocialRecoveryService{
		repos:  repos,
		logger: logger,
		// fedService is optional and can be set separately
	}
}

// SetFederationService sets the federation service for sending notifications
func (s *SocialRecoveryService) SetFederationService(fedService *RecoveryFederationService) {
	s.fedService = fedService
}

// We use TrusteeConfig and SocialRecoveryRequest types from storage package

// AddTrustee adds a trusted contact for social recovery
func (s *SocialRecoveryService) AddTrustee(ctx context.Context, username, trusteeActorID string) error {
	// Validate trustee exists (could be remote actor)
	if trusteeActorID == "" {
		return fmt.Errorf("trustee actor ID required")
	}

	// Store trustee configuration
	trustee := &storage.TrusteeConfig{
		Username:  username,
		ActorID:   trusteeActorID,
		AddedAt:   time.Now(),
		Confirmed: false,
	}

	// Store in DynamoDB
	if err := s.repos.Recovery().StoreTrustee(ctx, username, trustee); err != nil {
		return fmt.Errorf("failed to store trustee: %w", err)
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
	if err := s.repos.Recovery().DeleteTrustee(ctx, username, trusteeActorID); err != nil {
		return fmt.Errorf("failed to delete trustee: %w", err)
	}

	s.logger.Info("removing recovery trustee",
		zap.String("username", username),
		zap.String("trustee", trusteeActorID))

	return nil
}

// GetTrustees returns all trustees for a user
func (s *SocialRecoveryService) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	return s.repos.Recovery().GetTrustees(ctx, username)
}

// InitiateRecovery starts the social recovery process
func (s *SocialRecoveryService) InitiateRecovery(ctx context.Context, username string) (*storage.SocialRecoveryRequest, error) {
	// Get user's trustees
	trustees, err := s.GetTrustees(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get trustees: %w", err)
	}

	if len(trustees) < 2 {
		return nil, fmt.Errorf("insufficient trustees configured (minimum 2 required)")
	}

	// Generate recovery request ID
	requestID := s.generateRecoveryID()

	// Generate recovery token (used after approval)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate recovery token: %w", err)
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
	if err := s.repos.Recovery().StoreRecoveryRequest(ctx, request); err != nil {
		return nil, fmt.Errorf("failed to store recovery request: %w", err)
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
	request, err := s.repos.Recovery().GetRecoveryRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get recovery request: %w", err)
	}

	if request == nil {
		return fmt.Errorf("recovery request not found")
	}

	if request.Status != "pending" {
		return fmt.Errorf("recovery request is not pending")
	}

	if time.Now().After(request.ExpiresAt) {
		request.Status = "expired"
		s.repos.Recovery().UpdateRecoveryRequest(ctx, request)
		return fmt.Errorf("recovery request expired")
	}

	// Check if trustee already voted
	for _, voter := range request.TrusteeVotes {
		if voter == trusteeActorID {
			return fmt.Errorf("trustee already voted")
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
			return fmt.Errorf("failed to enable recovery token: %w", err)
		}

		// Notify user (if they have other auth methods)
		s.notifyRecoveryApproved(ctx, request)
	}

	// Update request in DynamoDB
	if err := s.repos.Recovery().UpdateRecoveryRequest(ctx, request); err != nil {
		return fmt.Errorf("failed to update recovery request: %w", err)
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
	rand.Read(b)
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
	return s.repos.Recovery().StoreRecoveryToken(ctx, recoveryKey, recoveryData)
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
