package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// FederationDeliveryService represents the interface needed for federation delivery
type FederationDeliveryService interface {
	DeliverActivity(ctx context.Context, activity *activitypub.Activity, targetInbox string) error
}

// RecoveryFederationService handles ActivityPub notifications for recovery
type RecoveryFederationService struct {
	store      storage.Storage
	fedService FederationDeliveryService
	logger     *zap.Logger
	domain     string
}

// NewRecoveryFederationService creates a new recovery federation service
func NewRecoveryFederationService(store storage.Storage, fedService FederationDeliveryService, domain string, logger *zap.Logger) *RecoveryFederationService {
	return &RecoveryFederationService{
		store:      store,
		fedService: fedService,
		domain:     domain,
		logger:     logger,
	}
}

// SendTrusteeInvitation sends an ActivityPub notification to a trustee
func (s *RecoveryFederationService) SendTrusteeInvitation(ctx context.Context, fromUser string, trusteeActorID string) error {
	// Create a custom Activity for trustee invitation
	now := time.Now()
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: []any{
				"https://www.w3.org/ns/activitystreams",
				map[string]any{
					"lesser": "https://lesser.social/ns#",
					"TrusteeInvitation": map[string]string{
						"@id":   "lesser:TrusteeInvitation",
						"@type": "@id",
					},
				},
			},
			Type:      "Create",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domain, generateActivityID()),
			To:        []string{trusteeActorID},
			Published: &now,
		},
		Actor: fmt.Sprintf("https://%s/users/%s", s.domain, fromUser),
		Object: map[string]any{
			"type":         "Note",
			"id":           fmt.Sprintf("https://%s/objects/%s", s.domain, generateActivityID()),
			"attributedTo": fmt.Sprintf("https://%s/users/%s", s.domain, fromUser),
			"content": fmt.Sprintf(
				"<p>You have been invited to be a recovery trustee for <span class=\"h-card\"><a href=\"https://%s/@%s\" class=\"u-url mention\">@<span>%s</span></a></span>. This means you can help them recover their account if they lose access.</p><p>To accept, please visit: <a href=\"https://%s/auth/recovery/trustee/accept?user=%s\">Accept Trustee Invitation</a></p>",
				s.domain, fromUser, fromUser, s.domain, fromUser,
			),
			"to": []string{trusteeActorID},
			"tag": []map[string]any{
				{
					"type": "Mention",
					"href": trusteeActorID,
					"name": getActorHandle(trusteeActorID),
				},
			},
			"lesser:trusteeInvitation": map[string]any{
				"type":      "TrusteeInvitation",
				"inviter":   fmt.Sprintf("https://%s/users/%s", s.domain, fromUser),
				"invitee":   trusteeActorID,
				"invitedAt": time.Now().Format(time.RFC3339),
			},
		},
	}

	// Get the signing actor (not used in current interface)
	_, err := s.store.GetActor(ctx, fromUser)
	if err != nil {
		return fmt.Errorf("failed to get signing actor: %w", err)
	}

	// Send via federation
	return s.fedService.DeliverActivity(ctx, activity, trusteeActorID+"/inbox")
}

// SendRecoveryRequest sends a recovery request to a trustee
func (s *RecoveryFederationService) SendRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest, trusteeActorID string) error {
	confirmURL := fmt.Sprintf("https://%s/auth/recovery/social/confirm?request=%s&trustee=%s",
		s.domain, request.ID, trusteeActorID)

	now := time.Now()
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: []any{
				"https://www.w3.org/ns/activitystreams",
				map[string]any{
					"lesser": "https://lesser.social/ns#",
					"RecoveryRequest": map[string]string{
						"@id":   "lesser:RecoveryRequest",
						"@type": "@id",
					},
				},
			},
			Type:      "Create",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domain, generateActivityID()),
			To:        []string{trusteeActorID},
			Published: &now,
		},
		Actor: fmt.Sprintf("https://%s/actor/system", s.domain), // System actor for recovery
		Object: map[string]any{
			"type":         "Note",
			"id":           fmt.Sprintf("https://%s/objects/%s", s.domain, generateActivityID()),
			"attributedTo": fmt.Sprintf("https://%s/actor/system", s.domain),
			"content": fmt.Sprintf(
				"<p>🚨 <strong>Account Recovery Request</strong></p><p><span class=\"h-card\"><a href=\"https://%s/@%s\" class=\"u-url mention\">@<span>%s</span></a></span> has initiated account recovery and needs your confirmation.</p><p>⏰ This request expires in 48 hours.</p><p>✅ <a href=\"%s\">Confirm Recovery Request</a></p>",
				s.domain, request.Username, request.Username, confirmURL,
			),
			"to": []string{trusteeActorID},
			"tag": []map[string]any{
				{
					"type": "Mention",
					"href": trusteeActorID,
					"name": getActorHandle(trusteeActorID),
				},
			},
			"lesser:recoveryRequest": map[string]any{
				"type":       "RecoveryRequest",
				"requestId":  request.ID,
				"username":   request.Username,
				"expiresAt":  request.ExpiresAt.Format(time.RFC3339),
				"confirmUrl": confirmURL,
			},
			"sensitive": true,
			"summary":   "Urgent: Account Recovery Request",
		},
	}

	// Get the system actor for signing (not used in current interface)
	_, err := s.store.GetActor(ctx, "system")
	if err != nil {
		// Create a minimal system actor if it doesn't exist (not used)
		_ = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("https://%s/actor/system", s.domain),
				Type: "Service",
			},
			PreferredUsername: "system",
			Inbox:             fmt.Sprintf("https://%s/actor/system/inbox", s.domain),
			Outbox:            fmt.Sprintf("https://%s/actor/system/outbox", s.domain),
		}
	}

	// Send via federation
	return s.fedService.DeliverActivity(ctx, activity, trusteeActorID+"/inbox")
}

// HandleTrusteeConfirmation processes incoming trustee confirmations
func (s *RecoveryFederationService) HandleTrusteeConfirmation(ctx context.Context, activity *activitypub.Activity) error {
	// Extract recovery request information
	object, ok := activity.Object.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid activity object")
	}

	// Look for our custom recovery confirmation
	recoveryData, ok := object["lesser:recoveryConfirmation"].(map[string]any)
	if !ok {
		return fmt.Errorf("not a recovery confirmation activity")
	}

	requestID, ok := recoveryData["requestId"].(string)
	if !ok {
		return fmt.Errorf("missing request ID")
	}

	trusteeActorID := activity.Actor

	// Process the confirmation
	socialRecovery := NewSocialRecoveryService(s.store, s.logger)
	if err := socialRecovery.ConfirmRecovery(ctx, requestID, trusteeActorID); err != nil {
		return fmt.Errorf("failed to process recovery confirmation: %w", err)
	}

	s.logger.Info("processed recovery confirmation via federation",
		zap.String("request_id", requestID),
		zap.String("trustee", trusteeActorID))

	return nil
}

// SendRecoveryApprovalNotification notifies the user their recovery was approved
func (s *RecoveryFederationService) SendRecoveryApprovalNotification(ctx context.Context, username string, recoveryToken string) error {
	// Get user's actor
	actor, err := s.store.GetActor(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to get actor: %w", err)
	}

	// If the user has alternative contact methods (like push notifications), use them
	// For now, we'll log it (in production, this would integrate with push notification service)

	recoveryURL := fmt.Sprintf("https://%s/auth/recovery/reset?token=%s", s.domain, recoveryToken)

	// Create a notification activity
	now := time.Now()
	notificationActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type:      "Create",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domain, generateActivityID()),
			To:        []string{actor.ID},
			Published: &now,
		},
		Actor: fmt.Sprintf("https://%s/actor/system", s.domain),
		Object: map[string]any{
			"type": "Note",
			"content": fmt.Sprintf(
				"<p>✅ <strong>Account Recovery Approved</strong></p><p>Your account recovery request has been approved by your trustees. You can now reset your authentication methods.</p><p>🔐 <a href=\"%s\">Complete Account Recovery</a></p><p>⚠️ This link expires in 24 hours.</p>",
				recoveryURL,
			),
			"to": []string{actor.ID},
		},
	}

	// Store for when user logs in next
	// In production, this would also trigger push notifications, SMS, etc.
	s.logger.Info("recovery approved, notification prepared",
		zap.String("username", username),
		zap.String("recovery_url", recoveryURL),
		zap.String("activity_id", notificationActivity.ID))

	return nil
}

// Helper functions

func generateActivityID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), generateRandomString(8))
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func getActorHandle(actorID string) string {
	// Extract handle from actor ID
	// Example: https://mastodon.social/users/alice -> @alice@mastodon.social
	// This is a simplified version - in production, you'd parse the URL properly
	return "@" + actorID
}

// RecoveryActivity represents a custom ActivityPub activity for recovery
type RecoveryActivity struct {
	activitypub.Activity
	RecoveryType string         `json:"lesser:recoveryType,omitempty"`
	RecoveryData map[string]any `json:"lesser:recoveryData,omitempty"`
}

// MarshalJSON implements custom JSON marshaling
func (r *RecoveryActivity) MarshalJSON() ([]byte, error) {
	// First marshal the base activity
	baseJSON, err := json.Marshal(r.Activity)
	if err != nil {
		return nil, err
	}

	// Unmarshal to add our custom fields
	var m map[string]any
	if err := json.Unmarshal(baseJSON, &m); err != nil {
		return nil, err
	}

	// Add custom fields
	if r.RecoveryType != "" {
		m["lesser:recoveryType"] = r.RecoveryType
	}
	if r.RecoveryData != nil {
		m["lesser:recoveryData"] = r.RecoveryData
	}

	return json.Marshal(m)
}
