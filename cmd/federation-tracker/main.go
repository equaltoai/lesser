package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamodb"
	"go.uber.org/zap"
)

type handler struct {
	logger *zap.Logger
	store  storage.Storage
	domain string
}

func main() {
	lambda.Start(handleDynamoDBStream)
}

func handleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error {
	// Initialize logger
	logger := common.Logger()

	// Load configuration
	cfg := config.Get()

	// Initialize storage
	store, err := dynamodb.New()
	if err != nil {
		logger.Error("failed to initialize storage", zap.Error(err))
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	h := &handler{
		logger: logger,
		store:  store,
		domain: cfg.Domain,
	}

	// Process each record in the stream
	for _, record := range event.Records {
		if err := h.processRecord(ctx, record); err != nil {
			logger.Error("failed to process record",
				zap.String("eventID", record.EventID),
				zap.Error(err))
			// Continue processing other records
		}
	}

	return nil
}

func (h *handler) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// We're interested in INSERT and MODIFY events that indicate federation activity
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Look for activities from remote instances
	pk, pkExists := record.Change.NewImage["PK"]
	if !pkExists {
		return nil
	}

	// Extract PK string value
	pkStr := ""
	if pk.DataType() == events.DataTypeString {
		pkStr = pk.String()
	}

	if pkStr == "" {
		return nil
	}

	// Track activities from remote actors
	if strings.HasPrefix(pkStr, "ACTIVITY#") {
		return h.trackActivityFromInstance(ctx, record)
	}

	// Track remote actors
	if strings.HasPrefix(pkStr, "ACTOR#") {
		return h.trackActorFromInstance(ctx, record)
	}

	return nil
}

func (h *handler) trackActivityFromInstance(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Extract the activity data
	activityData, ok := record.Change.NewImage["Activity"]
	if !ok || activityData.DataType() != events.DataTypeMap {
		return nil
	}

	// Get the activity map
	activityMap := activityData.Map()

	// Extract actor information
	actorID := ""
	if actor, ok := activityMap["actor"]; ok && actor.DataType() == events.DataTypeString {
		actorID = actor.String()
	}

	if actorID == "" {
		return nil
	}

	// Parse the domain from the actor ID
	domain := extractDomain(actorID)
	if domain == "" || domain == h.domain {
		// Local activity, not from a remote instance
		return nil
	}

	// Update instance info
	info := &storage.InstanceInfo{
		Domain: domain,
	}

	// Try to detect software type from activity format
	if _, hasOrderedItems := activityMap["orderedItems"]; hasOrderedItems {
		info.Software = "mastodon" // Mastodon-style collections
	} else if _, hasItems := activityMap["items"]; hasItems {
		info.Software = "activitypub" // Generic ActivityPub
	}

	// Update instance last seen time and increment message count
	if err := h.store.UpsertInstanceInfo(ctx, info); err != nil {
		h.logger.Error("failed to update instance info",
			zap.String("domain", domain),
			zap.Error(err))
		return err
	}

	h.logger.Debug("tracked activity from instance",
		zap.String("domain", domain),
		zap.String("actorID", actorID))

	return nil
}

func (h *handler) trackActorFromInstance(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Extract the actor data
	actorData, ok := record.Change.NewImage["Actor"]
	if !ok || actorData.DataType() != events.DataTypeMap {
		return nil
	}

	// Get the actor map
	actorMap := actorData.Map()

	// Extract actor ID
	actorID := ""
	if id, ok := actorMap["id"]; ok && id.DataType() == events.DataTypeString {
		actorID = id.String()
	}

	if actorID == "" {
		return nil
	}

	// Parse the domain from the actor ID
	domain := extractDomain(actorID)
	if domain == "" || domain == h.domain {
		// Local actor, not from a remote instance
		return nil
	}

	// Extract instance information from actor
	info := &storage.InstanceInfo{
		Domain: domain,
	}

	// Extract public key for instance verification
	if publicKeyData, ok := actorMap["publicKey"]; ok && publicKeyData.DataType() == events.DataTypeMap {
		publicKeyMap := publicKeyData.Map()
		if keyPem, ok := publicKeyMap["publicKeyPem"]; ok && keyPem.DataType() == events.DataTypeString {
			info.PublicKey = keyPem.String()
		}
	}

	// Extract shared inbox
	if endpointsData, ok := actorMap["endpoints"]; ok && endpointsData.DataType() == events.DataTypeMap {
		endpointsMap := endpointsData.Map()
		if sharedInbox, ok := endpointsMap["sharedInbox"]; ok && sharedInbox.DataType() == events.DataTypeString {
			info.SharedInbox = sharedInbox.String()
		}
	}

	// Try to detect software from actor properties
	if _, hasPropertyValue := actorMap["attachment"]; hasPropertyValue {
		// Mastodon and compatible software use attachment for profile fields
		info.Software = "mastodon"
	}

	// Update instance info
	if err := h.store.UpsertInstanceInfo(ctx, info); err != nil {
		h.logger.Error("failed to update instance info from actor",
			zap.String("domain", domain),
			zap.Error(err))
		return err
	}

	h.logger.Debug("tracked actor from instance",
		zap.String("domain", domain),
		zap.String("actorID", actorID))

	return nil
}

// extractDomain extracts the domain from an ActivityPub ID URL
func extractDomain(actorID string) string {
	u, err := url.Parse(actorID)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
