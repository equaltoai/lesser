package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"go.uber.org/zap"
)

// RelayService handles ActivityPub relay functionality
type RelayService struct {
	store      storage.Storage
	logger     *zap.Logger
	httpClient *http.Client
	domain     string
}

// NewRelayService creates a new relay service
func NewRelayService(store storage.Storage, domain string, logger *zap.Logger) *RelayService {
	return &RelayService{
		store:  store,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
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
	r.logger.Info("subscribing to relay",
		zap.String("relay_url", relayURL),
		zap.String("actor", actorUsername))

	// Parse relay URL
	_, err := url.Parse(relayURL)
	if err != nil {
		return fmt.Errorf("invalid relay URL: %w", err)
	}

	// Get relay actor information
	relayActor, err := r.fetchRelayActor(ctx, relayURL)
	if err != nil {
		return fmt.Errorf("failed to fetch relay actor: %w", err)
	}

	// Get subscribing actor
	actor, err := r.store.GetActor(ctx, actorUsername)
	if err != nil {
		return fmt.Errorf("failed to get actor: %w", err)
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
		return fmt.Errorf("failed to store relay info: %w", err)
	}

	// Send follow activity to relay
	deliverySvc := NewDeliveryService(r.store)
	if err := deliverySvc.DeliverActivity(ctx, followActivity, relayActor.Inbox, actor); err != nil {
		return fmt.Errorf("failed to deliver follow activity: %w", err)
	}

	r.logger.Info("successfully sent follow request to relay",
		zap.String("relay_url", relayURL))

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
		return fmt.Errorf("relay not found: %w", err)
	}

	// Get actor
	actor, err := r.store.GetActor(ctx, actorUsername)
	if err != nil {
		return fmt.Errorf("failed to get actor: %w", err)
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
		Object: map[string]interface{}{
			"type":   activitypub.FollowType,
			"actor":  actor.ID,
			"object": relayURL,
		},
	}

	// Send undo activity to relay
	deliverySvc := NewDeliveryService(r.store)
	if err := deliverySvc.DeliverActivity(ctx, undoActivity, relayInfo.InboxURL, actor); err != nil {
		r.logger.Error("failed to deliver undo activity",
			zap.String("relay_url", relayURL),
			zap.Error(err))
		// Continue with local removal even if delivery fails
	}

	// Remove relay info
	if err := r.removeRelayInfo(ctx, relayURL); err != nil {
		return fmt.Errorf("failed to remove relay info: %w", err)
	}

	r.logger.Info("successfully unsubscribed from relay",
		zap.String("relay_url", relayURL))

	return nil
}

// HandleRelayActivity handles an activity received from a relay
func (r *RelayService) HandleRelayActivity(ctx context.Context, activity *activitypub.Activity, relayURL string) error {
	r.logger.Debug("handling relay activity",
		zap.String("activity_type", activity.Type),
		zap.String("relay_url", relayURL))

	// Verify the activity is from a known relay
	relayInfo, err := r.getRelayInfo(ctx, relayURL)
	if err != nil || !relayInfo.Active {
		return fmt.Errorf("activity from unknown or inactive relay: %s", relayURL)
	}

	// Update last seen timestamp
	relayInfo.LastSeenAt = time.Now()
	if err := r.storeRelayInfo(ctx, relayInfo); err != nil {
		r.logger.Warn("failed to update relay last seen",
			zap.String("relay_url", relayURL),
			zap.Error(err))
	}

	// Process based on activity type
	switch activity.Type {
	case activitypub.AcceptType:
		// Relay accepted our follow request
		return r.handleRelayAccept(ctx, activity, relayInfo)

	case activitypub.RejectType:
		// Relay rejected our follow request
		return r.handleRelayReject(ctx, activity, relayInfo)

	case activitypub.AnnounceType:
		// Relay is announcing an activity
		return r.handleRelayAnnounce(ctx, activity, relayInfo)

	default:
		// Forward other activities to normal processing
		return nil
	}
}

// ForwardToRelays forwards an activity to all active relays
func (r *RelayService) ForwardToRelays(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Get all active relays
	relays, err := r.getActiveRelays(ctx)
	if err != nil {
		r.logger.Error("failed to get active relays", zap.Error(err))
		return err
	}

	if len(relays) == 0 {
		r.logger.Debug("no active relays to forward to")
		return nil
	}

	r.logger.Info("forwarding activity to relays",
		zap.String("activity_id", activity.ID),
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
	deliverySvc := NewDeliveryService(r.store)
	var errors []error

	for _, relay := range relays {
		if err := deliverySvc.DeliverActivity(ctx, announceActivity, relay.InboxURL, actor); err != nil {
			r.logger.Error("failed to forward to relay",
				zap.String("relay_url", relay.URL),
				zap.Error(err))
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to forward to %d relays", len(errors))
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch relay actor: status %d", resp.StatusCode)
	}

	var actor activitypub.Actor
	if err := json.NewDecoder(resp.Body).Decode(&actor); err != nil {
		return nil, err
	}

	// Verify it's a relay actor
	if actor.Type != "Application" && actor.Type != "Service" {
		return nil, fmt.Errorf("not a relay actor: type %s", actor.Type)
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
	case map[string]interface{}:
		// Convert map to activity
		data, err := json.Marshal(obj)
		if err != nil {
			return fmt.Errorf("failed to marshal announced object: %w", err)
		}
		if err := json.Unmarshal(data, &announcedActivity); err != nil {
			return fmt.Errorf("failed to unmarshal announced activity: %w", err)
		}
	default:
		return fmt.Errorf("invalid announced object type: %T", obj)
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
	_ = ctx // Will be used when DynamoDB implementation is added
	// Store in DynamoDB with pattern:
	// PK: RELAY#<relay_url>
	// SK: INFO
	r.logger.Debug("storing relay info", zap.String("relay_url", relay.URL))
	return nil
}

func (r *RelayService) getRelayInfo(ctx context.Context, relayURL string) (*RelayInfo, error) {
	_ = ctx // Will be used when DynamoDB implementation is added
	// Get from DynamoDB
	r.logger.Debug("getting relay info", zap.String("relay_url", relayURL))
	return &RelayInfo{
		URL:      relayURL,
		InboxURL: relayURL + "/inbox",
		Active:   true,
	}, nil
}

func (r *RelayService) removeRelayInfo(ctx context.Context, relayURL string) error {
	_ = ctx // Will be used when DynamoDB implementation is added
	// Remove from DynamoDB
	r.logger.Debug("removing relay info", zap.String("relay_url", relayURL))
	return nil
}

func (r *RelayService) getActiveRelays(ctx context.Context) ([]*RelayInfo, error) {
	_ = ctx // Will be used when DynamoDB implementation is added
	// Query DynamoDB for active relays
	// GSI: RELAY_STATUS
	// PK: ACTIVE#true
	r.logger.Debug("getting active relays")
	return []*RelayInfo{}, nil
}
