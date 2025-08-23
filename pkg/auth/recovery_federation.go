package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// FederationDeliveryService represents the interface needed for federation delivery
type FederationDeliveryService interface {
	DeliverActivity(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error
}

// Use the common StorageProvider interface from interfaces.go

// RecoveryFederationService handles ActivityPub notifications for recovery
type RecoveryFederationService struct {
	repos          StorageProvider
	fedService     FederationDeliveryService
	logger         *zap.Logger
	domain         string
	secretsManager SecretsManager
	config         *config.Config
}

// NewRecoveryFederationService creates a new recovery federation service
func NewRecoveryFederationService(cfg *config.Config, repos StorageProvider, fedService FederationDeliveryService, domain string, logger *zap.Logger) *RecoveryFederationService {
	// Initialize Secrets Manager for system actor keys
	secretsManager, err := NewAWSSecretsManager(SecretsManagerConfig{
		Region:      cfg.Region,
		KeyPrefix:   "lesser/system-actor-keys",
		CacheTTL:    5 * time.Minute,
		Description: "Lesser system actor private keys for recovery federation",
	}, logger.Named("secrets-manager"))
	if err != nil {
		logger.Warn("failed to initialize AWS Secrets Manager, system actor keys will not be available",
			zap.Error(err),
			zap.String("fallback", "system actor creation will be disabled"))
	}

	return &RecoveryFederationService{
		repos:          repos,
		fedService:     fedService,
		domain:         domain,
		logger:         logger,
		secretsManager: secretsManager,
		config:         cfg,
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

	// Get the signing actor
	signingActor, err := s.repos.Actor().GetActor(ctx, fromUser)
	if err != nil {
		s.logger.Error("failed to get signing actor for trustee invitation",
			zap.Error(err),
			zap.String("from_user", fromUser),
			zap.String("trustee_actor_id", trusteeActorID))
		return errors.Join(ErrSigningActorRetrievalFailed, err)
	}

	// Send via federation
	return s.fedService.DeliverActivity(ctx, activity, trusteeActorID+"/inbox", signingActor)
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

	// Get or create the system actor for signing
	systemActor, err := s.repos.Actor().GetActor(ctx, "system")
	if err != nil {
		// Create a system actor if it doesn't exist
		systemActor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("https://%s/actor/system", s.domain),
				Type: "Service",
			},
			PreferredUsername: "system",
			Inbox:             fmt.Sprintf("https://%s/actor/system/inbox", s.domain),
			Outbox:            fmt.Sprintf("https://%s/actor/system/outbox", s.domain),
			PublicKey: &activitypub.PublicKey{
				ID:           fmt.Sprintf("https://%s/actor/system#main-key", s.domain),
				Owner:        fmt.Sprintf("https://%s/actor/system", s.domain),
				PublicKeyPem: s.getSystemPublicKey(),
			},
		}

		// Store the system actor for future use (with empty private key as we'll store it separately)
		if storeErr := s.repos.Actor().CreateActor(ctx, systemActor, ""); storeErr != nil {
			s.logger.Warn("failed to store system actor", zap.Error(storeErr))
			// Continue anyway - we have the actor in memory
		}
	}

	// Send via federation with proper signing actor
	return s.fedService.DeliverActivity(ctx, activity, trusteeActorID+"/inbox", systemActor)
}

// HandleTrusteeConfirmation processes incoming trustee confirmations
func (s *RecoveryFederationService) HandleTrusteeConfirmation(ctx context.Context, activity *activitypub.Activity) error {
	// Extract recovery request information
	object, ok := activity.Object.(map[string]any)
	if !ok {
		return ErrInvalidActivityObject
	}

	// Look for our custom recovery confirmation
	recoveryData, ok := object["lesser:recoveryConfirmation"].(map[string]any)
	if !ok {
		return ErrNotRecoveryConfirmationActivity
	}

	requestID, ok := recoveryData["requestId"].(string)
	if !ok {
		return ErrMissingRequestID
	}

	trusteeActorID := activity.Actor

	// Process the confirmation
	socialRecovery := NewSocialRecoveryService(s.repos, s.logger)
	if err := socialRecovery.ConfirmRecovery(ctx, requestID, trusteeActorID); err != nil {
		s.logger.Error("failed to process recovery confirmation",
			zap.Error(err),
			zap.String("request_id", requestID),
			zap.String("trustee_actor_id", trusteeActorID))
		return errors.Join(ErrRecoveryConfirmationFailed, err)
	}

	s.logger.Info("processed recovery confirmation via federation",
		zap.String("request_id", requestID),
		zap.String("trustee", trusteeActorID))

	return nil
}

// SendRecoveryApprovalNotification notifies the user their recovery was approved
func (s *RecoveryFederationService) SendRecoveryApprovalNotification(ctx context.Context, username string, recoveryToken string) error {
	// Get user's actor
	actor, err := s.repos.Actor().GetActor(ctx, username)
	if err != nil {
		s.logger.Error("failed to get actor for recovery approval notification",
			zap.Error(err),
			zap.String("username", username))
		return errors.Join(ErrActorRetrievalFailed, err)
	}

	// If the user has alternative contact methods (like push notifications), use them
	// For now, we'll log it (in production, this would integrate with push notification service)

	recoveryURL := fmt.Sprintf("https://%s/auth/recovery/reset?token=%s", s.domain, recoveryToken)

	// Create a notification activity
	now := time.Now()
	// Create the recovery notification as a proper Note object
	recoveryNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Type:      "Note",
			ID:        fmt.Sprintf("https://%s/notes/%s", s.domain, generateActivityID()),
			To:        []string{actor.ID},
			Published: &now,
		},
		Content: fmt.Sprintf(
			"<p>✅ <strong>Account Recovery Approved</strong></p><p>Your account recovery request has been approved by your trustees. You can now reset your authentication methods.</p><p>🔐 <a href=\"%s\">Complete Account Recovery</a></p><p>⚠️ This link expires in 24 hours.</p>",
			recoveryURL,
		),
		AttributedTo: fmt.Sprintf("https://%s/actor/system", s.domain),
	}

	notificationActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type:      "Create",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domain, generateActivityID()),
			To:        []string{actor.ID},
			Published: &now,
		},
		Actor:  fmt.Sprintf("https://%s/actor/system", s.domain),
		Object: recoveryNote,
	}

	// Store the notification locally for when user logs in next
	notification := &models.Notification{
		ID:       generateActivityID(),
		UserID:   username,
		Type:     "recovery_approved",
		ActorID:  fmt.Sprintf("https://%s/actor/system", s.domain),
		TargetID: notificationActivity.ID,
		Title:    "Account Recovery Approved",
		Body:     recoveryNote.Content,
		IsRead:   false,
		Data: map[string]interface{}{
			"recovery_url": recoveryURL,
			"expires_at":   now.Add(24 * time.Hour),
			"activity":     notificationActivity,
		},
	}

	if err := s.repos.Notification().CreateNotification(ctx, notification); err != nil {
		s.logger.Error("failed to store recovery notification",
			zap.Error(err),
			zap.String("username", username))
		// Continue anyway - notification is not critical
	}

	// If the user's actor has a known inbox, deliver the notification via federation
	if actor.Inbox != "" {
		// Get or create the system actor for signing
		systemActor, err := s.repos.Actor().GetActor(ctx, "system")
		if err != nil {
			// Create system actor if it doesn't exist
			systemActor = &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   fmt.Sprintf("https://%s/actor/system", s.domain),
					Type: "Service",
				},
				PreferredUsername: "system",
				Inbox:             fmt.Sprintf("https://%s/actor/system/inbox", s.domain),
				Outbox:            fmt.Sprintf("https://%s/actor/system/outbox", s.domain),
				PublicKey: &activitypub.PublicKey{
					ID:           fmt.Sprintf("https://%s/actor/system#main-key", s.domain),
					Owner:        fmt.Sprintf("https://%s/actor/system", s.domain),
					PublicKeyPem: s.getSystemPublicKey(),
				},
			}
		}

		// Deliver the notification activity via federation
		if deliverErr := s.fedService.DeliverActivity(ctx, notificationActivity, actor.Inbox, systemActor); deliverErr != nil {
			s.logger.Warn("failed to deliver recovery notification via federation",
				zap.Error(deliverErr),
				zap.String("username", username),
				zap.String("inbox", actor.Inbox))
			// Don't fail - we have the local notification stored
		}
	}

	// In production, this would also trigger WebPush notifications and WebAuthn challenges
	s.logger.Info("recovery approved, notification stored",
		zap.String("username", username),
		zap.String("recovery_url", recoveryURL),
		zap.String("activity_id", notificationActivity.ID))

	return nil
}

// getSystemPublicKey returns the system actor's public key PEM
func (s *RecoveryFederationService) getSystemPublicKey() string {
	// Try to get from config first
	if key := s.config.SystemActorPublicKey; key != "" {
		return key
	}

	// Check if we have a cached key in storage
	ctx := context.Background()
	systemActor, err := s.repos.Actor().GetActor(ctx, "system")
	if err == nil && systemActor != nil && systemActor.PublicKey != nil {
		return systemActor.PublicKey.PublicKeyPem
	}

	// Check if Secrets Manager is available
	if s.secretsManager == nil {
		s.logger.Warn("Secrets Manager not available, cannot generate system actor keys")
		return ""
	}

	// Try to retrieve existing private key from Secrets Manager
	systemKeyID := fmt.Sprintf("system-actor-%s", s.domain)
	privateKeyPEM, err := s.secretsManager.RetrievePrivateKey(ctx, systemKeyID)
	if err != nil {
		// Generate a new key pair if none exists
		s.logger.Info("generating new system actor key pair",
			zap.String("key_id", systemKeyID),
			zap.Error(err))

		publicKeyPEM, newPrivateKeyPEM, err := s.secretsManager.GenerateAndStoreKeyPair(ctx, systemKeyID)
		if err != nil {
			s.logger.Error("failed to generate and store system actor key pair", zap.Error(err))
			return ""
		}

		// Store the system actor with the new public key
		if err := s.storeSystemActorKeys(publicKeyPEM, newPrivateKeyPEM); err != nil {
			s.logger.Warn("failed to store system actor keys in database", zap.Error(err))
		}

		return publicKeyPEM
	}

	// Derive public key from private key
	publicKeyPEM, err := s.derivePublicKeyFromPrivate(privateKeyPEM)
	if err != nil {
		s.logger.Error("failed to derive public key from private key", zap.Error(err))
		return ""
	}

	// Store the system actor with the existing keys (if not already stored)
	if err := s.storeSystemActorKeys(publicKeyPEM, privateKeyPEM); err != nil {
		s.logger.Debug("system actor already exists or failed to store", zap.Error(err))
	}

	return publicKeyPEM
}

// storeSystemActorKeys stores the system actor (without the private key in database)
func (s *RecoveryFederationService) storeSystemActorKeys(publicKeyPEM, _ string) error {
	ctx := context.Background()

	// Create or update the system actor with only the public key
	systemActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("https://%s/actor/system", s.domain),
			Type: "Service",
		},
		PreferredUsername: "system",
		Inbox:             fmt.Sprintf("https://%s/actor/system/inbox", s.domain),
		Outbox:            fmt.Sprintf("https://%s/actor/system/outbox", s.domain),
		PublicKey: &activitypub.PublicKey{
			ID:           fmt.Sprintf("https://%s/actor/system#main-key", s.domain),
			Owner:        fmt.Sprintf("https://%s/actor/system", s.domain),
			PublicKeyPem: publicKeyPEM,
		},
	}

	// Store the actor with empty private key (private key is in Secrets Manager)
	if err := s.repos.Actor().CreateActor(ctx, systemActor, ""); err != nil {
		// Try updating if it already exists
		s.logger.Debug("system actor may already exist, continuing", zap.Error(err))
	}

	return nil
}

// derivePublicKeyFromPrivate derives a public key PEM from a private key PEM
func (s *RecoveryFederationService) derivePublicKeyFromPrivate(privateKeyPEM string) (string, error) {
	// Decode the private key PEM
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", ErrFailedToDecodePEM
	}

	// Parse the private key
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 format
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			s.logger.Error("failed to parse private key in both PKCS8 and PKCS1 formats", zap.Error(err))
			return "", errors.Join(ErrPrivateKeyParseFailed, err)
		}
	}

	// Extract the public key
	var publicKey interface{}
	switch priv := privateKey.(type) {
	case *rsa.PrivateKey:
		publicKey = &priv.PublicKey
	default:
		s.logger.Error("unsupported private key type for system actor",
			zap.String("type", fmt.Sprintf("%T", privateKey)))
		return "", ErrUnsupportedPrivateKeyType
	}

	// Marshal the public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		s.logger.Error("failed to marshal public key for system actor", zap.Error(err))
		return "", errors.Join(ErrPublicKeyMarshalFailed, err)
	}

	// Encode to PEM
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM), nil
}

// GetSystemActorPrivateKey retrieves the system actor's private key from Secrets Manager
func (s *RecoveryFederationService) GetSystemActorPrivateKey(ctx context.Context) (string, error) {
	if s.secretsManager == nil {
		return "", ErrSecretsManagerNotAvailable
	}

	systemKeyID := fmt.Sprintf("system-actor-%s", s.domain)
	privateKeyPEM, err := s.secretsManager.RetrievePrivateKey(ctx, systemKeyID)
	if err != nil {
		s.logger.Error("failed to retrieve system actor private key",
			zap.Error(err),
			zap.String("system_key_id", systemKeyID))
		return "", errors.Join(ErrSystemActorKeyRetrievalFailed, err)
	}

	return privateKeyPEM, nil
}

// RotateSystemActorKey rotates the system actor's key pair
func (s *RecoveryFederationService) RotateSystemActorKey(ctx context.Context) error {
	if s.secretsManager == nil {
		return ErrSecretsManagerNotAvailable
	}

	systemKeyID := fmt.Sprintf("system-actor-%s", s.domain)
	s.logger.Info("rotating system actor key", zap.String("key_id", systemKeyID))

	// Generate new key pair
	publicKeyPEM, privateKeyPEM, err := s.secretsManager.RotateKey(ctx, systemKeyID)
	if err != nil {
		s.logger.Error("failed to rotate system actor key",
			zap.Error(err),
			zap.String("system_key_id", systemKeyID))
		return errors.Join(ErrSystemActorKeyRotationFailed, err)
	}

	// Update the system actor with the new public key
	if err := s.storeSystemActorKeys(publicKeyPEM, privateKeyPEM); err != nil {
		s.logger.Warn("failed to update system actor with new public key", zap.Error(err))
		// Don't fail the rotation - the new key is stored in Secrets Manager
	}

	s.logger.Info("system actor key rotation completed", zap.String("key_id", systemKeyID))
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
