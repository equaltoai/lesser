package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	// Activity type constants
	activityTypeCreate = "Create"
)

// DynamoDBAPI defines the subset of DynamoDB operations we use for relay storage (disabled to break circular dependency)
/*
type DynamoDBAPI interface {
	PutItem(ctx context.Context, params *dynamodbsvc.PutItemInput, optFns ...func(*dynamodbsvc.Options)) (*dynamodbsvc.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodbsvc.GetItemInput, optFns ...func(*dynamodbsvc.Options)) (*dynamodbsvc.GetItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodbsvc.DeleteItemInput, optFns ...func(*dynamodbsvc.Options)) (*dynamodbsvc.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodbsvc.QueryInput, optFns ...func(*dynamodbsvc.Options)) (*dynamodbsvc.QueryOutput, error)
}
*/

// RelayService handles ActivityPub relay functionality
type RelayService struct {
	store      core.RepositoryStorage
	logger     *zap.Logger
	httpClient *httpclient.SecureClient
	domain     string
}

// NewRelayService creates a new relay service
func NewRelayService(store core.RepositoryStorage, domain string, logger *zap.Logger) *RelayService {
	return &RelayService{
		store:  store,
		logger: logger,
		httpClient: httpclient.NewSecureClient(
			httpclient.WithTimeout(10*time.Second),
			httpclient.WithLogger(logger),
		),
		domain: domain,
	}
}

// RelayInfo represents information about a relay
type RelayInfo struct {
	URL        string    `json:"url"`
	InboxURL   string    `json:"inbox_url"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// SubscribeToRelay subscribes to a relay
func (r *RelayService) SubscribeToRelay(ctx context.Context, relayURL string, actorUsername string) error {
	start := time.Now()
	operationID := fmt.Sprintf("subscribe-%d", start.UnixNano())

	r.logger.Info("subscribing to relay",
		zap.String("relay_url", relayURL),
		zap.String("actor", actorUsername),
		zap.String("operation_id", operationID))

	// Track cost for this operation
	defer func() {
		r.trackRelayCost(ctx, relayURL, "subscription", "outbound", "",
			start, operationID, true, "") // Will be updated if error occurs
	}()

	// Parse relay URL
	_, err := url.Parse(relayURL)
	if err != nil {
		r.trackRelayCost(ctx, relayURL, "subscription", "outbound", "",
			start, operationID, false, fmt.Sprintf("invalid URL: %v", err))
		r.logger.Error("invalid relay URL", zap.String("relay_url", relayURL), zap.Error(err))
		return errors.Join(ErrInvalidRelayURL, err)
	}

	// Get relay actor information
	relayActor, err := r.fetchRelayActor(ctx, relayURL)
	if err != nil {
		r.trackRelayCost(ctx, relayURL, "subscription", "outbound", "",
			start, operationID, false, fmt.Sprintf("fetch actor failed: %v", err))
		r.logger.Error("failed to fetch relay actor", zap.String("relay_url", relayURL), zap.Error(err))
		return errors.Join(ErrFetchRelayActorFailed, err)
	}

	// Get subscribing actor
	actor, err := r.store.Actor().GetActorByUsername(ctx, actorUsername)
	if err != nil {
		r.trackRelayCost(ctx, relayURL, "subscription", "outbound", "",
			start, operationID, false, fmt.Sprintf("get actor failed: %v", err))
		r.logger.Error("failed to get actor", zap.String("actor_username", actorUsername), zap.Error(err))
		return errors.Join(ErrGetActorFailed, err)
	}

	// Create Follow activity
	now := time.Now()
	followActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        fmt.Sprintf("https://%s/activities/follow/%d", r.domain, now.UnixNano()),
			Type:      activitypub.FollowType,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: relayActor.ID,
	}

	// Store relay info
	relayInfo := &RelayInfo{
		URL:        relayURL,
		InboxURL:   relayActor.Inbox,
		Active:     false, // Will be activated when relay accepts
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
	}

	if err := r.storeRelayInfo(ctx, relayInfo); err != nil {
		r.trackRelayCost(ctx, relayURL, "subscription", "outbound", followActivity.Type,
			start, operationID, false, fmt.Sprintf("store relay info failed: %v", err))
		r.logger.Error("failed to store relay info", zap.String("relay_url", relayURL), zap.Error(err))
		return errors.Join(ErrStoreRelayInfoFailed, err)
	}

	// Send follow activity to relay
	deliverySvc := NewDeliveryService(NewRepositoryStorageAdapter(r.store), config.Get())
	if err := deliverySvc.DeliverActivity(ctx, followActivity, relayActor.Inbox, actor); err != nil {
		r.trackRelayCost(ctx, relayURL, "subscription", "outbound", followActivity.Type,
			start, operationID, false, fmt.Sprintf("delivery failed: %v", err))
		r.logger.Error("failed to deliver follow activity", zap.String("relay_url", relayURL), zap.Error(err))
		return errors.Join(ErrDeliverFollowActivityFailed, err)
	}

	r.logger.Info("successfully sent follow request to relay",
		zap.String("relay_url", relayURL),
		zap.String("operation_id", operationID))

	return nil
}

// UnsubscribeFromRelay unsubscribes from a relay
func (r *RelayService) UnsubscribeFromRelay(ctx context.Context, relayURL string, actorUsername string) error {
	r.logger.Info("unsubscribing from relay",
		zap.String("relay_url", relayURL),
		zap.String("actor", actorUsername))

	// Get relay info
	relayInfo, err := r.getRelayInfo(ctx, relayURL)
	if err != nil {
		r.logger.Error("relay not found", zap.String("relay_url", relayURL), zap.Error(err))
		return errors.Join(ErrRelayNotFound, err)
	}

	// Get actor
	actor, err := r.store.Actor().GetActorByUsername(ctx, actorUsername)
	if err != nil {
		r.logger.Error("failed to get actor for unsubscribe", zap.String("actor_username", actorUsername), zap.Error(err))
		return errors.Join(ErrGetActorFailed, err)
	}

	// Create Undo Follow activity
	now := time.Now()
	undoActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        fmt.Sprintf("https://%s/activities/undo/%d", r.domain, now.UnixNano()),
			Type:      activitypub.UndoType,
			Published: &now,
		},
		Actor: actor.ID,
		Object: map[string]any{
			"type":   activitypub.FollowType,
			"actor":  actor.ID,
			"object": relayURL,
		},
	}

	// Send undo activity to relay
	deliverySvc := NewDeliveryService(NewRepositoryStorageAdapter(r.store), config.Get())
	if err := deliverySvc.DeliverActivity(ctx, undoActivity, relayInfo.InboxURL, actor); err != nil {
		r.logger.Error("failed to deliver undo activity",
			zap.String("relay_url", relayURL),
			zap.Error(err))
		// Continue with local removal even if delivery fails
	}

	// Remove relay info
	if err := r.removeRelayInfo(ctx, relayURL); err != nil {
		r.logger.Error("failed to remove relay info", zap.String("relay_url", relayURL), zap.Error(err))
		return errors.Join(ErrRemoveRelayInfoFailed, err)
	}

	r.logger.Info("successfully unsubscribed from relay",
		zap.String("relay_url", relayURL))

	return nil
}

// HandleRelayActivity handles an activity received from a relay
func (r *RelayService) HandleRelayActivity(ctx context.Context, activity *activitypub.Activity, relayURL string) error {
	start := time.Now()
	operationID := fmt.Sprintf("handle-%d", start.UnixNano())

	r.logger.Debug("handling relay activity",
		zap.String("activity_type", activity.Type),
		zap.String("relay_url", relayURL),
		zap.String("operation_id", operationID))

	// Track cost for inbound processing
	defer func() {
		r.trackRelayCost(ctx, relayURL, "processing", "inbound", activity.Type,
			start, operationID, true, "") // Will be updated if error occurs
	}()

	// Verify the activity is from a known relay
	relayInfo, err := r.getRelayInfo(ctx, relayURL)
	if err != nil || !relayInfo.Active {
		errMsg := fmt.Sprintf("activity from unknown or inactive relay: %s", relayURL)
		r.trackRelayCost(ctx, relayURL, "processing", "inbound", activity.Type,
			start, operationID, false, errMsg)
		r.logger.Error("activity from unknown or inactive relay", zap.String("relay_url", relayURL), zap.Bool("relay_active", relayInfo != nil && relayInfo.Active))
		return ErrUnknownInactiveRelay
	}

	// Update last seen timestamp
	relayInfo.LastSeenAt = time.Now()
	if err := r.storeRelayInfo(ctx, relayInfo); err != nil {
		r.logger.Warn("failed to update relay last seen",
			zap.String("relay_url", relayURL),
			zap.String("operation_id", operationID),
			zap.Error(err))
	}

	// Process based on activity type
	var processErr error
	switch activity.Type {
	case activitypub.AcceptType:
		// Relay accepted our follow request
		processErr = r.handleRelayAccept(ctx, activity, relayInfo)

	case activitypub.RejectType:
		// Relay rejected our follow request
		processErr = r.handleRelayReject(ctx, activity, relayInfo)

	case activitypub.AnnounceType:
		// Relay is announcing an activity
		processErr = r.handleRelayAnnounce(ctx, activity, relayInfo)

	default:
		// Forward other activities to normal processing
		r.logger.Debug("forwarding activity to normal processing",
			zap.String("activity_type", activity.Type),
			zap.String("relay_url", relayURL))
		processErr = nil
	}

	// Update cost tracking with final result
	if processErr != nil {
		r.trackRelayCost(ctx, relayURL, "processing", "inbound", activity.Type,
			start, operationID, false, fmt.Sprintf("processing failed: %v", processErr))
	}

	return processErr
}

// ForwardToRelays forwards an activity to all active relays
func (r *RelayService) ForwardToRelays(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	start := time.Now()
	operationID := fmt.Sprintf("forward-%d", start.UnixNano())

	// Get all active relays
	relays, err := r.getActiveRelays(ctx)
	if err != nil {
		r.logger.Error("failed to get active relays", zap.Error(err))
		return err
	}

	if err := common.ValidateSliceNotEmpty("relays", relays); err != nil {
		r.logger.Debug("no active relays to forward to")
		return nil
	}

	r.logger.Info("forwarding activity to relays",
		zap.String("activity_id", activity.ID),
		zap.String("operation_id", operationID),
		zap.Int("relay_count", len(relays)))

	// Wrap activity in Announce for relay distribution
	now := time.Now()
	announceActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        fmt.Sprintf("https://%s/activities/announce/%d", r.domain, now.UnixNano()),
			Type:      activitypub.AnnounceType,
			Published: &now,
			To:        []string{activitypub.PublicAddress},
			CC:        []string{actor.Followers},
		},
		Actor:  actor.ID,
		Object: activity,
	}

	// Send to each relay
	deliverySvc := NewDeliveryService(NewRepositoryStorageAdapter(r.store), config.Get())
	var errors []error
	successCount := 0

	for _, relay := range relays {
		relayStart := time.Now()
		relayOpID := fmt.Sprintf("%s-relay-%s", operationID, extractDomainFromRelayURL(relay.URL))

		r.logger.Debug("forwarding to relay",
			zap.String("relay_url", relay.URL),
			zap.String("activity_type", activity.Type),
			zap.String("relay_operation_id", relayOpID))

		// Check budget before attempting delivery
		estimatedCost := int64(140) // Base cost estimate in microdollars
		if budgetErr := r.checkRelayBudget(ctx, relay.URL, estimatedCost); budgetErr != nil {
			r.logger.Warn("skipping relay due to budget limit",
				zap.String("relay_url", relay.URL),
				zap.Error(budgetErr))

			// Track the skipped operation
			r.trackRelayCost(ctx, relay.URL, "delivery", "outbound", activity.Type,
				relayStart, relayOpID, false, fmt.Sprintf("budget exceeded: %v", budgetErr))
			continue
		}

		// Attempt delivery
		if err := deliverySvc.DeliverActivity(ctx, announceActivity, relay.InboxURL, actor); err != nil {
			r.logger.Error("failed to forward to relay",
				zap.String("relay_url", relay.URL),
				zap.String("operation_id", relayOpID),
				zap.Error(err))
			errors = append(errors, err)

			// Track failed delivery
			r.trackRelayCost(ctx, relay.URL, "delivery", "outbound", activity.Type,
				relayStart, relayOpID, false, fmt.Sprintf("delivery failed: %v", err))
		} else {
			successCount++

			// Track successful delivery
			r.trackRelayCost(ctx, relay.URL, "delivery", "outbound", activity.Type,
				relayStart, relayOpID, true, "")

			r.logger.Debug("successfully forwarded to relay",
				zap.String("relay_url", relay.URL),
				zap.String("operation_id", relayOpID))
		}
	}

	// Log summary
	totalDuration := time.Since(start)
	r.logger.Info("completed relay forwarding",
		zap.String("activity_id", activity.ID),
		zap.String("operation_id", operationID),
		zap.Int("total_relays", len(relays)),
		zap.Int("successful_deliveries", successCount),
		zap.Int("failed_deliveries", len(errors)),
		zap.Duration("total_duration", totalDuration))

	if len(errors) > 0 {
		r.logger.Error("failed to forward to some relays", zap.Int("failed_count", len(errors)), zap.String("operation_id", operationID))
		return ErrRelayForwardingFailed
	}

	return nil
}

// Private methods

func (r *RelayService) fetchRelayActor(ctx context.Context, relayURL string) (*activitypub.Actor, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, relayURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Lesser/1.0")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		r.logger.Error("failed to fetch relay actor: non-OK status", zap.String("relay_url", relayURL), zap.Int("status_code", resp.StatusCode))
		return nil, ErrFetchRelayActorHTTPFailed
	}

	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		return nil, err
	}

	// Verify it's a relay actor
	if actor.Type != "Application" && actor.Type != "Service" {
		r.logger.Error("not a relay actor", zap.String("relay_url", relayURL), zap.String("actor_type", actor.Type))
		return nil, ErrNotRelayActor
	}

	return &actor, nil
}

func (r *RelayService) handleRelayAccept(ctx context.Context, activity *activitypub.Activity, relay *RelayInfo) error {
	_ = activity // Activity details not needed for accept handling
	r.logger.Info("relay accepted follow request",
		zap.String("relay_url", relay.URL))

	// Mark relay as active
	relay.Active = true
	return r.storeRelayInfo(ctx, relay)
}

func (r *RelayService) handleRelayReject(ctx context.Context, activity *activitypub.Activity, relay *RelayInfo) error {
	_ = activity // Activity details not needed for reject handling
	r.logger.Warn("relay rejected follow request",
		zap.String("relay_url", relay.URL))

	// Remove relay
	return r.removeRelayInfo(ctx, relay.URL)
}

func (r *RelayService) handleRelayAnnounce(ctx context.Context, activity *activitypub.Activity, relay *RelayInfo) error {
	_ = ctx // Context will be used when processing is implemented
	// Extract the announced activity
	var announcedActivity *activitypub.Activity

	switch obj := activity.Object.(type) {
	case *activitypub.Activity:
		announcedActivity = obj
	case map[string]any:
		// Convert map to activity
		data, err := json.Marshal(obj)
		if err != nil {
			r.logger.Error("failed to marshal announced object", zap.String("relay_url", relay.URL), zap.Error(err))
			return errors.Join(ErrMarshalAnnouncedObjectFailed, err)
		}
		if err := json.Unmarshal(data, &announcedActivity); err != nil {
			r.logger.Error("failed to unmarshal announced activity", zap.String("relay_url", relay.URL), zap.Error(err))
			return errors.Join(ErrUnmarshalAnnouncedActivityFailed, err)
		}
	default:
		r.logger.Error("invalid announced object type", zap.String("relay_url", relay.URL), zap.String("object_type", fmt.Sprintf("%T", obj)))
		return ErrInvalidAnnouncedObjectType
	}

	r.logger.Debug("processing relayed activity",
		zap.String("activity_type", announcedActivity.Type),
		zap.String("activity_id", announcedActivity.ID),
		zap.String("relay_url", relay.URL))

	// Process the announced activity normally
	// This will be handled by the inbox processor
	return nil
}

// Storage methods (would use DynamoDB in production)

func (r *RelayService) storeRelayInfo(ctx context.Context, relay *RelayInfo) error {
	// Convert federation RelayInfo to storage RelayInfo
	storageRelay := &storage.RelayInfo{
		URL:        relay.URL,
		InboxURL:   relay.InboxURL,
		Active:     relay.Active,
		CreatedAt:  relay.CreatedAt,
		LastSeenAt: relay.LastSeenAt,
	}

	return r.store.Relay().StoreRelayInfo(ctx, storageRelay)
}

func (r *RelayService) getRelayInfo(ctx context.Context, relayURL string) (*RelayInfo, error) {
	storageRelay, err := r.store.Relay().GetRelayInfo(ctx, relayURL)
	if err != nil {
		return nil, err
	}

	// Convert storage RelayInfo to federation RelayInfo
	return &RelayInfo{
		URL:        storageRelay.URL,
		InboxURL:   storageRelay.InboxURL,
		Active:     storageRelay.Active,
		CreatedAt:  storageRelay.CreatedAt,
		LastSeenAt: storageRelay.LastSeenAt,
	}, nil
}

func (r *RelayService) removeRelayInfo(ctx context.Context, relayURL string) error {
	return r.store.Relay().RemoveRelayInfo(ctx, relayURL)
}

func (r *RelayService) getActiveRelays(ctx context.Context) ([]*RelayInfo, error) {
	storageRelays, err := r.store.Relay().GetActiveRelays(ctx)
	if err != nil {
		return nil, err
	}

	// Convert storage RelayInfo slice to federation RelayInfo slice
	relays := make([]*RelayInfo, len(storageRelays))
	for i, sr := range storageRelays {
		relays[i] = &RelayInfo{
			URL:        sr.URL,
			InboxURL:   sr.InboxURL,
			Active:     sr.Active,
			CreatedAt:  sr.CreatedAt,
			LastSeenAt: sr.LastSeenAt,
		}
	}

	return relays, nil
}

// Cost Tracking Helper Methods

// trackRelayCost tracks the cost of a relay operation
func (r *RelayService) trackRelayCost(ctx context.Context, relayURL, operationType, direction, activityType string, startTime time.Time, requestID string, success bool, errorMessage string) {
	// Don't fail main operation if cost tracking fails
	if err := r.doTrackRelayCost(ctx, relayURL, operationType, direction, activityType, startTime, requestID, success, errorMessage); err != nil {
		r.logger.Debug("failed to track relay cost",
			zap.String("relay_url", relayURL),
			zap.String("operation_type", operationType),
			zap.Error(err))
	}
}

// doTrackRelayCost performs the actual cost tracking
func (r *RelayService) doTrackRelayCost(ctx context.Context, relayURL, operationType, direction, activityType string, startTime time.Time, requestID string, success bool, errorMessage string) error {
	duration := time.Since(startTime)
	domain := extractDomainFromRelayURL(relayURL)

	// Calculate costs based on operation type
	relayCost := &models.RelayCost{
		RelayURL:       relayURL,
		Domain:         domain,
		OperationType:  operationType,
		Direction:      direction,
		ActivityType:   activityType,
		RequestID:      requestID,
		Timestamp:      startTime,
		Success:        success,
		ErrorMessage:   errorMessage,
		ResponseTimeMs: duration.Milliseconds(),
	}

	// Calculate operation-specific costs
	switch operationType {
	case "subscription":
		// HTTP request to relay
		relayCost.HTTPRequestCount = 1
		relayCost.HTTPRequestCost = 100 // $0.0001 per request in microdollars

		// Data transfer (estimated 2KB for follow activity)
		relayCost.DataTransferBytes = 2048
		relayCost.DataTransferCost = calculateDataTransferCost(relayCost.DataTransferBytes, direction)

		// Lambda processing time (estimated 200ms)
		relayCost.LambdaDurationMs = 200
		relayCost.LambdaCost = calculateLambdaCost(relayCost.LambdaDurationMs)

		// DynamoDB operations (store relay info)
		relayCost.DynamoDBOperations = 2 // GetItem + PutItem
		relayCost.DynamoDBCost = calculateDynamoDBCost(relayCost.DynamoDBOperations)

	case "delivery":
		// HTTP request to relay
		relayCost.HTTPRequestCount = 1
		relayCost.HTTPRequestCost = 100 // $0.0001 per request

		// Data transfer (estimated based on activity size)
		var estimatedSize int64 = 1024 // Default 1KB
		switch activityType {
		case activityTypeCreate:
			estimatedSize = 4096 // 4KB for Create activities
		case "Announce":
			estimatedSize = 2048 // 2KB for Announce activities
		}

		relayCost.DataTransferBytes = estimatedSize
		relayCost.DataTransferCost = calculateDataTransferCost(estimatedSize, direction)

		// Lambda processing time (estimated based on activity type)
		var estimatedDuration int64 = 100
		if activityType == "Create" {
			estimatedDuration = 300 // Create activities take longer
		}

		relayCost.LambdaDurationMs = estimatedDuration
		relayCost.LambdaCost = calculateLambdaCost(estimatedDuration)

		// DynamoDB operations (update relay status)
		relayCost.DynamoDBOperations = 1
		relayCost.DynamoDBCost = calculateDynamoDBCost(1)

		// SQS message if using async delivery
		relayCost.SQSMessages = 1
		relayCost.SQSCost = 40 // $0.0000004 per message in microdollars

	case "processing":
		// Inbound processing costs
		// Lambda processing time
		relayCost.LambdaDurationMs = duration.Milliseconds()
		relayCost.LambdaCost = calculateLambdaCost(duration.Milliseconds())

		// DynamoDB operations for storing activity
		relayCost.DynamoDBOperations = 3 // Read relay info, store activity, update metrics
		relayCost.DynamoDBCost = calculateDynamoDBCost(3)

		// Data transfer (inbound is free, but track for metrics)
		relayCost.DataTransferBytes = 2048 // Estimated
		relayCost.DataTransferCost = 0     // Inbound is free

	default:
		// Generic operation costs
		relayCost.LambdaDurationMs = duration.Milliseconds()
		relayCost.LambdaCost = calculateLambdaCost(duration.Milliseconds())
		relayCost.DynamoDBOperations = 1
		relayCost.DynamoDBCost = calculateDynamoDBCost(1)
	}

	// Add retry costs if this was a retry
	if strings.Contains(errorMessage, "retry") {
		relayCost.RetryCount = 1
		// Double the HTTP and Lambda costs for retries
		relayCost.HTTPRequestCost *= 2
		relayCost.LambdaCost *= 2
	}

	// Store the cost record
	return r.store.Cost().CreateRelayCost(ctx, relayCost)
}

// extractDomainFromRelayURL extracts domain from relay URL
func extractDomainFromRelayURL(relayURL string) string {
	if err := common.ValidateRequiredParam("relayURL", relayURL); err != nil {
		return keyTypeUnknown
	}

	// Validate URL format
	if err := common.ValidateURL(relayURL, "relay_url"); err != nil {
		return keyTypeUnknown
	}

	// Parse URL to extract domain
	parsedURL, err := url.Parse(relayURL)
	if err != nil {
		// Should not happen after ValidateURL, but fallback anyway
		return keyTypeUnknown
	}

	return parsedURL.Hostname()
}

// Cost calculation helper functions

// calculateDataTransferCost calculates data transfer costs in microdollars
func calculateDataTransferCost(bytes int64, direction string) int64 {
	if direction == "inbound" {
		return 0 // Inbound data transfer is free
	}

	// Outbound data transfer: $0.09 per GB
	// Convert to microdollars: $0.09 * 1,000,000 = 90,000 microdollars per GB
	gb := float64(bytes) / (1024 * 1024 * 1024)
	return int64(gb * 90000)
}

// calculateLambdaCost calculates Lambda execution costs in microdollars
func calculateLambdaCost(durationMs int64) int64 {
	// Lambda pricing: $0.0000166667 per GB-second
	// Assume 512MB (0.5GB) memory allocation
	// Convert to microdollars and calculate for duration

	memoryGB := 0.5
	durationSeconds := float64(durationMs) / 1000.0

	// Cost = $0.0000166667 * GB * seconds
	// In microdollars = 16.6667 * GB * seconds
	return int64(16.6667 * memoryGB * durationSeconds)
}

// calculateDynamoDBCost calculates DynamoDB operation costs in microdollars
func calculateDynamoDBCost(operations int64) int64 {
	// DynamoDB on-demand pricing:
	// Write: $1.25 per million requests = 1.25 microdollars per request
	// Read: $0.25 per million requests = 0.25 microdollars per request
	// Assume 50/50 read/write mix

	avgCostPerOp := (1.25 + 0.25) / 2 // 0.75 microdollars per operation
	return int64(float64(operations) * avgCostPerOp)
}

// checkRelayBudget checks if relay operation would exceed budget
func (r *RelayService) checkRelayBudget(ctx context.Context, relayURL string, estimatedCostMicroCents int64) error {
	// Get daily budget for this relay
	budget, err := r.store.Cost().GetRelayBudget(ctx, relayURL, "daily")
	if err != nil {
		// No budget configured - allow operation
		return nil
	}

	// Check if adding this cost would exceed budget
	if budget.CurrentUsageMicroCents+estimatedCostMicroCents > budget.LimitMicroCents {
		r.logger.Error("relay operation would exceed daily budget", zap.String("relay_url", relayURL), zap.Int64("current_usage", budget.CurrentUsageMicroCents), zap.Int64("estimated_cost", estimatedCostMicroCents), zap.Int64("limit", budget.LimitMicroCents))
		return ErrRelayBudgetExceeded
	}

	// Check for warning threshold
	newUsagePercent := float64(budget.CurrentUsageMicroCents+estimatedCostMicroCents) / float64(budget.LimitMicroCents) * 100.0
	if newUsagePercent >= budget.WarningThresholdPercent && !budget.WarningAlertSent {
		r.logger.Warn("relay budget warning threshold reached",
			zap.String("relay_url", relayURL),
			zap.Float64("usage_percent", newUsagePercent),
			zap.Float64("threshold", budget.WarningThresholdPercent))

		// Mark warning as sent
		budget.WarningAlertSent = true
		if updateErr := r.store.Cost().UpdateRelayBudget(ctx, budget); updateErr != nil {
			r.logger.Error("failed to update relay budget warning flag", zap.Error(updateErr))
		}
	}

	return nil
}
