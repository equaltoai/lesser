package main

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// RecoveryActivityHandler handles recovery-related ActivityPub activities
type RecoveryActivityHandler struct {
	userRepository     *repositories.UserRepository
	recoveryFedService *auth.RecoveryFederationService
	logger             *zap.Logger
}

// NewRecoveryActivityHandler creates a new recovery activity handler
func NewRecoveryActivityHandler(userRepository *repositories.UserRepository, recoveryFedService *auth.RecoveryFederationService, logger *zap.Logger) *RecoveryActivityHandler {
	return &RecoveryActivityHandler{
		userRepository:     userRepository,
		recoveryFedService: recoveryFedService,
		logger:             logger,
	}
}

// HandleActivity processes incoming recovery-related activities
func (h *RecoveryActivityHandler) HandleActivity(ctx context.Context, activity *activitypub.Activity) error {
	// Check if this is a recovery-related activity
	object, ok := activity.Object.(map[string]any)
	if !ok {
		return nil // Not our concern
	}

	// Check for trustee confirmation
	if recoveryData, ok := object["lesser:recoveryConfirmation"].(map[string]any); ok {
		return h.handleTrusteeConfirmation(ctx, activity, recoveryData)
	}

	// Check for trustee acceptance of invitation
	if inviteData, ok := object["lesser:trusteeAcceptance"].(map[string]any); ok {
		return h.handleTrusteeAcceptance(ctx, activity, inviteData)
	}

	return nil
}

// handleTrusteeConfirmation processes a trustee's confirmation of a recovery request
func (h *RecoveryActivityHandler) handleTrusteeConfirmation(ctx context.Context, activity *activitypub.Activity, recoveryData map[string]any) error {
	// Verify request ID is present
	_, ok := recoveryData["requestId"].(string)
	if !ok {
		return fmt.Errorf("missing request ID in recovery confirmation")
	}

	// Verify the activity is signed by the trustee
	trusteeActorID := activity.Actor
	if trusteeActorID == "" {
		return fmt.Errorf("missing actor in recovery confirmation")
	}

	// Process the confirmation
	return h.recoveryFedService.HandleTrusteeConfirmation(ctx, activity)
}

// handleTrusteeAcceptance processes a trustee accepting an invitation
func (h *RecoveryActivityHandler) handleTrusteeAcceptance(_ context.Context, activity *activitypub.Activity, inviteData map[string]any) error {
	inviterUsername, ok := inviteData["inviterUsername"].(string)
	if !ok {
		return fmt.Errorf("missing inviter username in trustee acceptance")
	}

	trusteeActorID := activity.Actor
	if trusteeActorID == "" {
		return fmt.Errorf("missing actor in trustee acceptance")
	}

	// Update the trustee confirmation status
	// TODO: This needs to be implemented in the user repository or a separate recovery repository
	h.logger.Info("would update trustee confirmation",
		zap.String("inviter", inviterUsername),
		zap.String("trustee", trusteeActorID))
	// err := h.userRepository.UpdateTrusteeConfirmed(ctx, inviterUsername, trusteeActorID, true)
	// if err != nil {
	// 	return fmt.Errorf("failed to update trustee confirmation: %w", err)
	// 	}

	h.logger.Info("trustee accepted invitation",
		zap.String("inviter", inviterUsername),
		zap.String("trustee", trusteeActorID))

	return nil
}

// CreateRecoveryConfirmationActivity creates an ActivityPub activity for confirming a recovery request
func CreateRecoveryConfirmationActivity(requestID, trusteeActorID, systemActorID string) *activitypub.Activity {
	now := generateTime()
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: []any{
				"https://www.w3.org/ns/activitystreams",
				map[string]any{
					"lesser": "https://lesser.social/ns#",
					"RecoveryConfirmation": map[string]string{
						"@id":   "lesser:RecoveryConfirmation",
						"@type": "@id",
					},
				},
			},
			Type:      "Create",
			ID:        fmt.Sprintf("%s/activities/%d", trusteeActorID, generateTimestamp()),
			To:        []string{systemActorID},
			Published: &now,
		},
		Actor: trusteeActorID,
		Object: map[string]any{
			"type":    "Note",
			"content": "Recovery request confirmed",
			"lesser:recoveryConfirmation": map[string]any{
				"type":      "RecoveryConfirmation",
				"requestId": requestID,
				"confirmed": true,
			},
		},
	}
}

// CreateTrusteeAcceptanceActivity creates an ActivityPub activity for accepting a trustee invitation
func CreateTrusteeAcceptanceActivity(inviterUsername, trusteeActorID, inviterActorID string) *activitypub.Activity {
	now := generateTime()
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: []any{
				"https://www.w3.org/ns/activitystreams",
				map[string]any{
					"lesser": "https://lesser.social/ns#",
					"TrusteeAcceptance": map[string]string{
						"@id":   "lesser:TrusteeAcceptance",
						"@type": "@id",
					},
				},
			},
			Type:      "Accept",
			ID:        fmt.Sprintf("%s/activities/%d", trusteeActorID, generateTimestamp()),
			To:        []string{inviterActorID},
			Published: &now,
		},
		Actor: trusteeActorID,
		Object: map[string]any{
			"type":    "Note",
			"content": "Trustee invitation accepted",
			"lesser:trusteeAcceptance": map[string]any{
				"type":            "TrusteeAcceptance",
				"inviterUsername": inviterUsername,
				"accepted":        true,
			},
		},
	}
}

// Helper functions
func generateTimestamp() int64 {
	return time.Now().UnixNano()
}

func generateTime() time.Time {
	return time.Now()
}
