package federation

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/httpclient"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.uber.org/zap"
)

// DeliveryService handles sending activities to remote instances
type DeliveryService struct {
	store      storage.Storage
	httpClient *httpclient.SecureClient
	logger     *zap.Logger
	sqsClient  *sqs.Client
	queueURL   string
}

// NewDeliveryService creates a new delivery service
func NewDeliveryService(store storage.Storage) *DeliveryService {
	logger := common.Logger()

	svc := &DeliveryService{
		store:      store,
		httpClient: httpclient.NewSecureClient(httpclient.WithLogger(logger)),
		logger:     logger,
	}

	// Initialize SQS client if queue URL is configured
	queueURL := os.Getenv("FEDERATION_QUEUE_URL")
	if queueURL != "" {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err == nil {
			svc.sqsClient = sqs.NewFromConfig(cfg)
			svc.queueURL = queueURL
			logger.Info("SQS queue configured for federation delivery",
				zap.String("queue_url", queueURL))
		} else {
			logger.Warn("Failed to initialize SQS client", zap.Error(err))
		}
	} else {
		logger.Info("No SQS queue configured, using synchronous delivery")
	}

	return svc
}

// DeliverActivity delivers an activity to a remote inbox
func (d *DeliveryService) DeliverActivity(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error {
	log := common.WithContext(ctx).With(
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
		zap.String("target_inbox", targetInbox),
		zap.String("signing_actor", signingActor.ID),
	)

	log.Info("delivering activity to remote inbox")

	// Serialize the activity
	body, err := json.Marshal(activity)
	if err != nil {
		log.Error("failed to marshal activity", zap.Error(err))
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetInbox, bytes.NewReader(body))
	if err != nil {
		log.Error("failed to create request", zap.Error(err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("User-Agent", "Lesser/1.0")

	// Get the actor's private key from storage
	privateKeyPEM, err := d.store.GetActorPrivateKey(ctx, signingActor.PreferredUsername)
	if err != nil {
		log.Error("failed to get private key", zap.Error(err))
		return fmt.Errorf("failed to get private key: %w", err)
	}

	// Parse the private key
	privateKey, err := ParsePrivateKeyPEM([]byte(privateKeyPEM))
	if err != nil {
		log.Error("failed to parse private key", zap.Error(err))
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	// Sign the request
	if err := SignHTTPRequest(req, privateKey, signingActor.PublicKey.ID); err != nil {
		log.Error("failed to sign request", zap.Error(err))
		return fmt.Errorf("failed to sign request: %w", err)
	}

	// Send the request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Error("failed to send request", zap.Error(err))
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body for logging
	respBody, _ := io.ReadAll(resp.Body)

	// Check the response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Info("activity delivered successfully",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(respBody)))
		return nil
	}

	log.Warn("activity delivery failed",
		zap.Int("status_code", resp.StatusCode),
		zap.String("response", string(respBody)))

	// Return error for non-2xx status codes
	return fmt.Errorf("delivery failed with status %d: %s", resp.StatusCode, string(respBody))
}

// DeliverToFollowers delivers an activity to all followers of an actor
func (d *DeliveryService) DeliverToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	log := common.WithContext(ctx).With(
		zap.String("activity_id", activity.ID),
		zap.String("actor", actor.ID),
	)

	log.Info("delivering activity to followers")

	// Get all followers (usernames)
	followerUsernames, _, err := d.store.GetFollowers(ctx, actor.PreferredUsername, 1000, "")
	if err != nil {
		log.Error("failed to get followers", zap.Error(err))
		return fmt.Errorf("failed to get followers: %w", err)
	}

	log.Info("found followers", zap.Int("count", len(followerUsernames)))

	// Group followers by shared inbox
	inboxMap := make(map[string][]string) // inbox URL -> follower IDs

	for _, followerUsername := range followerUsernames {
		// Get follower actor details
		follower, err := d.store.GetActor(ctx, followerUsername)
		if err != nil {
			log.Warn("failed to get follower actor",
				zap.String("username", followerUsername),
				zap.Error(err))
			continue
		}

		// Skip local followers (they already have the activity via fan-out)
		if isLocalActor(follower.ID, actor.ID) {
			continue
		}

		// Determine inbox URL (prefer shared inbox)
		inboxURL := follower.Inbox
		if follower.Endpoints != nil && follower.Endpoints.SharedInbox != "" {
			inboxURL = follower.Endpoints.SharedInbox
		}

		inboxMap[inboxURL] = append(inboxMap[inboxURL], follower.ID)
	}

	// Deliver to each unique inbox
	var deliveryErrors []error
	for inbox, followerIDs := range inboxMap {
		log.Info("delivering to inbox",
			zap.String("inbox", inbox),
			zap.Int("follower_count", len(followerIDs)))

		if err := d.DeliverActivity(ctx, activity, inbox, actor); err != nil {
			log.Error("failed to deliver to inbox",
				zap.String("inbox", inbox),
				zap.Error(err))
			deliveryErrors = append(deliveryErrors, fmt.Errorf("failed to deliver to %s: %w", inbox, err))
			// Continue delivering to other inboxes
		}
	}

	if len(deliveryErrors) > 0 {
		return fmt.Errorf("failed to deliver to %d inboxes", len(deliveryErrors))
	}

	return nil
}

// DeliverToRecipients delivers an activity to specific recipients (to, cc, bto, bcc)
func (d *DeliveryService) DeliverToRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	log := common.WithContext(ctx).With(
		zap.String("activity_id", activity.ID),
		zap.String("actor", actor.ID),
	)

	// Collect all recipients
	recipients := make(map[string]bool) // Use map to deduplicate

	// Helper to add recipients
	addRecipients := func(addresses []string) {
		for _, addr := range addresses {
			// Skip special addresses
			if addr == activitypub.PublicAddress || addr == actor.Followers {
				continue
			}
			recipients[addr] = true
		}
	}

	// Add all recipients
	addRecipients(activity.To)
	addRecipients(activity.CC)
	addRecipients(activity.BTo)
	addRecipients(activity.BCC)

	log.Info("delivering to recipients", zap.Int("count", len(recipients)))

	// Resolve recipients to inbox URLs
	inboxMap := make(map[string][]string) // inbox URL -> recipient IDs

	for recipientID := range recipients {
		// Skip local recipients
		if isLocalActor(recipientID, actor.ID) {
			continue
		}

		// Fetch the recipient's actor document
		recipientActor, err := d.fetchRemoteActor(ctx, recipientID)
		if err != nil {
			log.Warn("failed to fetch recipient actor",
				zap.String("recipient", recipientID),
				zap.Error(err))
			continue
		}

		// Determine inbox URL
		inboxURL := recipientActor.Inbox
		if recipientActor.Endpoints != nil && recipientActor.Endpoints.SharedInbox != "" {
			inboxURL = recipientActor.Endpoints.SharedInbox
		}

		inboxMap[inboxURL] = append(inboxMap[inboxURL], recipientID)
	}

	// Deliver to each unique inbox
	var deliveryErrors []error
	for inbox, recipientIDs := range inboxMap {
		log.Info("delivering to inbox",
			zap.String("inbox", inbox),
			zap.Int("recipient_count", len(recipientIDs)))

		if err := d.DeliverActivity(ctx, activity, inbox, actor); err != nil {
			log.Error("failed to deliver to inbox",
				zap.String("inbox", inbox),
				zap.Error(err))
			deliveryErrors = append(deliveryErrors, fmt.Errorf("failed to deliver to %s: %w", inbox, err))
		}
	}

	if len(deliveryErrors) > 0 {
		return fmt.Errorf("failed to deliver to %d inboxes", len(deliveryErrors))
	}

	return nil
}

// fetchRemoteActor fetches a remote actor by their ID
func (d *DeliveryService) fetchRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	// Check cache first
	cached, err := d.store.GetCachedRemoteActor(ctx, actorID)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Fetch from remote using secure client
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Lesser/1.0")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch actor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch actor: status %d: %s", resp.StatusCode, string(body))
	}

	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		return nil, fmt.Errorf("failed to decode actor: %w", err)
	}

	// Cache the actor (ignore errors)
	// Extract handle from actor ID
	handle := extractHandleFromActorID(actor.ID, actor.PreferredUsername)
	if handle != "" {
		if err := d.store.CacheRemoteActor(ctx, handle, &actor, 24*time.Hour); err != nil {
			d.logger.Warn("failed to cache remote actor",
				zap.String("actor_id", actor.ID),
				zap.Error(err))
		}
	}

	return &actor, nil
}

// isLocalActor checks if an actor ID belongs to the same instance
func isLocalActor(actorID, localActorID string) bool {
	// Extract domain from actor IDs
	// Format: https://domain.com/users/username
	localDomain := extractDomain(localActorID)
	actorDomain := extractDomain(actorID)

	return localDomain == actorDomain
}

// extractDomain extracts the domain from an actor ID
func extractDomain(actorID string) string {
	// Simple extraction - in production, use proper URL parsing
	if len(actorID) > 8 && actorID[:8] == "https://" {
		parts := actorID[8:]
		if idx := bytes.IndexByte([]byte(parts), '/'); idx > 0 {
			return parts[:idx]
		}
	}
	return actorID
}

// QueueDelivery queues an activity for delivery (for future SQS implementation)
func (d *DeliveryService) QueueDelivery(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error {
	// Create delivery message
	deliveryID := fmt.Sprintf("delivery_%s_%d", generateDeliveryID(), time.Now().UnixNano())

	message := map[string]interface{}{
		"delivery_id":      deliveryID,
		"activity":         activity,
		"target_inbox":     targetInbox,
		"signing_actor_id": signingActor.PreferredUsername,
		"retry_count":      0,
		"max_retries":      5,
		"created_at":       time.Now(),
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		d.logger.Error("failed to marshal delivery message", zap.Error(err))
		return fmt.Errorf("failed to marshal delivery message: %w", err)
	}

	// Get SQS client from config
	sqsClient := d.getSQSClient()
	if sqsClient == nil {
		// SQS not configured, fall back to synchronous delivery
		d.logger.Warn("SQS not configured, using synchronous delivery")
		return d.DeliverActivity(ctx, activity, targetInbox, signingActor)
	}

	// Send to SQS queue
	queueURL := d.getQueueURL()
	_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(messageJSON)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"activity_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(activity.Type),
			},
			"target_domain": {
				DataType:    aws.String("String"),
				StringValue: aws.String(extractDomain(targetInbox)),
			},
		},
	})

	if err != nil {
		d.logger.Error("failed to send to SQS, falling back to synchronous delivery",
			zap.String("delivery_id", deliveryID),
			zap.Error(err))
		// Fall back to synchronous delivery
		return d.DeliverActivity(ctx, activity, targetInbox, signingActor)
	}

	d.logger.Info("queued activity for delivery",
		zap.String("delivery_id", deliveryID),
		zap.String("activity_id", activity.ID),
		zap.String("target_inbox", targetInbox))

	return nil
}

// getSQSClient returns the SQS client if configured
func (d *DeliveryService) getSQSClient() *sqs.Client {
	// This would be initialized in NewDeliveryService if SQS is configured
	// For now, return nil to indicate SQS is not configured
	return d.sqsClient
}

// getQueueURL returns the configured queue URL
func (d *DeliveryService) getQueueURL() string {
	// This would come from configuration
	return d.queueURL
}

// generateDeliveryID generates a unique delivery ID
func generateDeliveryID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to less random source on error
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}

// extractHandleFromActorID extracts a handle (e.g., @user@domain) from an actor ID
func extractHandleFromActorID(actorID, preferredUsername string) string {
	// Extract domain from actor ID
	// Format: https://domain.com/users/username
	domain := extractDomain(actorID)
	if domain == "" || preferredUsername == "" {
		return ""
	}
	return fmt.Sprintf("@%s@%s", preferredUsername, domain)
}
