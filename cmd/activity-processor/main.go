package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

// ActivityDirection represents the direction of an activity
type ActivityDirection string

const (
	ActivityDirectionInbox  ActivityDirection = "inbox"
	ActivityDirectionOutbox ActivityDirection = "outbox"
)

var (
	store      storage.Storage
	logger     *zap.Logger
	httpClient *http.Client
)

func init() {
	logger = common.Logger()

	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// HTTP client with timeout for delivery
	httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}
}

// handler processes DynamoDB stream events for activities
func handler(ctx context.Context, event events.DynamoDBEvent) error {
	log := common.WithContext(ctx)

	for _, record := range event.Records {
		if record.EventName != "INSERT" && record.EventName != "MODIFY" {
			continue
		}

		// Parse the DynamoDB record
		activity, direction, username, err := parseActivityRecord(record.Change.NewImage)
		if err != nil {
			log.Error("failed to parse record", zap.Error(err))
			continue
		}

		log.Info("processing activity",
			zap.String("id", activity.ID),
			zap.String("type", activity.Type),
			zap.String("direction", string(direction)),
			zap.String("username", username))

		if direction == ActivityDirectionInbox {
			// Process inbox activity
			if err := processInboxActivity(ctx, activity, username); err != nil {
				log.Error("failed to process inbox activity",
					zap.String("activity_id", activity.ID),
					zap.Error(err))
			}
		} else {
			// Process outbox activity - deliver to recipients
			if err := processOutboxActivity(ctx, activity); err != nil {
				log.Error("failed to process outbox activity",
					zap.String("activity_id", activity.ID),
					zap.Error(err))
			}
		}
	}

	return nil
}

// parseActivityRecord extracts activity data from DynamoDB stream record
func parseActivityRecord(image map[string]events.DynamoDBAttributeValue) (*activitypub.Activity, ActivityDirection, string, error) {
	// Extract PK to get username
	pkAttr, ok := image["PK"]
	if !ok {
		return nil, "", "", fmt.Errorf("missing PK attribute")
	}
	if pkAttr.DataType() != events.DataTypeString || pkAttr.String() == "" {
		return nil, "", "", fmt.Errorf("invalid PK attribute")
	}

	// Extract username from PK (format: ACTOR#username)
	pk := pkAttr.String()
	if !strings.HasPrefix(pk, "ACTOR#") {
		return nil, "", "", fmt.Errorf("not an actor record")
	}
	username := strings.TrimPrefix(pk, "ACTOR#")

	// Check if this is an activity record by looking at SK
	skAttr, ok := image["SK"]
	if !ok {
		return nil, "", "", fmt.Errorf("missing SK attribute")
	}
	if skAttr.DataType() != events.DataTypeString || !strings.Contains(skAttr.String(), "ACTIVITY#") {
		return nil, "", "", fmt.Errorf("not an activity record")
	}

	// Extract GSI1PK to determine direction (optional - only present for inbox activities)
	var direction ActivityDirection = ActivityDirectionOutbox // Default to outbox
	if gsi1pkAttr, ok := image["GSI1PK"]; ok && gsi1pkAttr.DataType() == events.DataTypeString {
		gsi1pk := gsi1pkAttr.String()
		if strings.HasPrefix(gsi1pk, "INBOX#") {
			direction = ActivityDirectionInbox
		}
	}

	// Extract activity data from Activity field
	activityAttr, ok := image["Activity"]
	if !ok {
		return nil, "", "", fmt.Errorf("missing Activity attribute")
	}

	// The Activity field should be a Map type
	if activityAttr.DataType() != events.DataTypeMap {
		return nil, "", "", fmt.Errorf("Activity attribute is not a map")
	}

	// Convert DynamoDB attribute map to JSON for unmarshaling
	activityJSON, err := convertDynamoDBMapToJSON(activityAttr.Map())
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to convert activity map: %w", err)
	}

	// Unmarshal activity
	var activity activitypub.Activity
	if err := json.Unmarshal(activityJSON, &activity); err != nil {
		return nil, "", "", fmt.Errorf("failed to unmarshal activity: %w", err)
	}

	return &activity, direction, username, nil
}

// convertDynamoDBMapToJSON converts a DynamoDB attribute map to JSON
func convertDynamoDBMapToJSON(m map[string]events.DynamoDBAttributeValue) ([]byte, error) {
	result := make(map[string]interface{})

	for k, v := range m {
		val, err := extractDynamoDBValue(v)
		if err != nil {
			return nil, fmt.Errorf("failed to extract value for key %s: %w", k, err)
		}
		result[k] = val
	}

	return json.Marshal(result)
}

// extractDynamoDBValue extracts the actual value from a DynamoDB attribute
func extractDynamoDBValue(attr events.DynamoDBAttributeValue) (interface{}, error) {
	switch attr.DataType() {
	case events.DataTypeString:
		return attr.String(), nil
	case events.DataTypeNumber:
		return attr.Number(), nil
	case events.DataTypeBoolean:
		return attr.Boolean(), nil
	case events.DataTypeNull:
		return nil, nil
	case events.DataTypeList:
		list := make([]interface{}, 0)
		for _, item := range attr.List() {
			val, err := extractDynamoDBValue(item)
			if err != nil {
				return nil, err
			}
			list = append(list, val)
		}
		return list, nil
	case events.DataTypeMap:
		m := make(map[string]interface{})
		for k, v := range attr.Map() {
			val, err := extractDynamoDBValue(v)
			if err != nil {
				return nil, err
			}
			m[k] = val
		}
		return m, nil
	case events.DataTypeStringSet:
		return attr.StringSet(), nil
	case events.DataTypeNumberSet:
		return attr.NumberSet(), nil
	default:
		return nil, fmt.Errorf("unsupported data type: %v", attr.DataType())
	}
}

// processInboxActivity processes activities delivered to a user's inbox
func processInboxActivity(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	switch activity.Type {
	case activitypub.FollowType:
		// Create pending follow relationship
		return processFollow(ctx, activity, recipientUsername)

	case activitypub.AcceptType:
		// Check if this is accepting a follow
		if innerActivity, ok := activity.Object.(map[string]interface{}); ok {
			if innerType, ok := innerActivity["type"].(string); ok && innerType == activitypub.FollowType {
				return processFollowAccept(ctx, activity, recipientUsername)
			}
		}

	case activitypub.CreateType:
		// Store the created object
		return processCreate(ctx, activity, recipientUsername)

	case activitypub.LikeType:
		// Store the like
		return processLike(ctx, activity, recipientUsername)

	default:
		log.Warn("unhandled inbox activity type",
			zap.String("type", activity.Type),
			zap.String("id", activity.ID))
	}

	return nil
}

// processOutboxActivity processes activities created by a local user
func processOutboxActivity(ctx context.Context, activity *activitypub.Activity) error {
	log := common.WithContext(ctx)

	// Extract all recipients
	recipients := extractAllRecipients(activity)

	log.Info("delivering activity to recipients",
		zap.String("activity_id", activity.ID),
		zap.Int("recipient_count", len(recipients)))

	// Deliver to each recipient
	var deliveryErrors []error
	for _, recipient := range recipients {
		if err := deliverToRecipient(ctx, activity, recipient); err != nil {
			log.Error("failed to deliver to recipient",
				zap.String("recipient", recipient),
				zap.Error(err))
			deliveryErrors = append(deliveryErrors, err)
		}
	}

	if len(deliveryErrors) > 0 {
		return fmt.Errorf("delivery failed to %d recipients", len(deliveryErrors))
	}

	return nil
}

// Process specific activity types

func processFollow(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	// Extract follower username from actor
	followerUsername := extractUsernameFromActorID(activity.Actor)
	if followerUsername == "" {
		return fmt.Errorf("invalid actor ID: %s", activity.Actor)
	}

	log.Info("processing follow request",
		zap.String("follower", followerUsername),
		zap.String("followed", recipientUsername))

	// Create pending follow relationship
	err := store.CreateFollow(ctx, followerUsername, recipientUsername, activity.ID)
	if err != nil {
		return fmt.Errorf("failed to create follow relationship: %w", err)
	}

	return nil
}

func processFollowAccept(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	// Extract the original follow activity
	innerActivity, ok := activity.Object.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Accept object type")
	}

	// Get the actor from the original follow (the follower)
	followerActor, ok := innerActivity["actor"].(string)
	if !ok {
		return fmt.Errorf("missing actor in original follow activity")
	}

	followerUsername := extractUsernameFromActorID(followerActor)
	if followerUsername == "" {
		return fmt.Errorf("invalid follower actor ID: %s", followerActor)
	}

	log.Info("processing follow accept",
		zap.String("follower", followerUsername),
		zap.String("followed", recipientUsername))

	// Accept the follow relationship
	err := store.AcceptFollow(ctx, followerUsername, recipientUsername)
	if err != nil {
		return fmt.Errorf("failed to accept follow: %w", err)
	}

	return nil
}

func processCreate(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing create activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Store the created object
	err := store.CreateObject(ctx, activity.Object)
	if err != nil {
		return fmt.Errorf("failed to create object: %w", err)
	}

	return nil
}

func processLike(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing like activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// TODO: Implement like storage
	// This would typically create a relationship between the liker and the liked object

	return nil
}

// Delivery functions

func extractAllRecipients(activity *activitypub.Activity) []string {
	recipientMap := make(map[string]bool)

	// Helper function to add recipients from a field
	addRecipients := func(field interface{}) {
		switch v := field.(type) {
		case string:
			recipientMap[v] = true
		case []string:
			for _, r := range v {
				recipientMap[r] = true
			}
		case []interface{}:
			for _, r := range v {
				if s, ok := r.(string); ok {
					recipientMap[s] = true
				}
			}
		}
	}

	// Add recipients from all addressing fields
	addRecipients(activity.To)
	addRecipients(activity.CC)
	addRecipients(activity.BTo)
	addRecipients(activity.BCC)

	// Filter out special addresses
	recipients := make([]string, 0)
	for r := range recipientMap {
		// Skip public addressing
		if r == activitypub.PublicAddress {
			continue
		}
		// Skip local actors (we don't deliver to ourselves)
		if strings.Contains(r, "/users/") && !strings.Contains(r, "://") {
			continue
		}
		recipients = append(recipients, r)
	}

	return recipients
}

func deliverToRecipient(ctx context.Context, activity *activitypub.Activity, recipientURL string) error {
	// TODO: Resolve collections (followers, following) to individual actors
	// For now, we assume recipientURL is an actor URL

	// Fetch the recipient actor to get their inbox
	actor, err := fetchRemoteActor(ctx, recipientURL)
	if err != nil {
		return fmt.Errorf("failed to fetch recipient actor: %w", err)
	}

	// Get the sender's private key
	senderUsername := extractUsernameFromActorID(activity.Actor)
	privateKey, err := store.GetActorPrivateKey(ctx, senderUsername)
	if err != nil {
		return fmt.Errorf("failed to get sender private key: %w", err)
	}

	// Deliver to the actor's inbox
	return deliverActivity(ctx, activity, actor.Inbox, privateKey, activity.Actor+"#main-key")
}

func deliverActivity(ctx context.Context, activity *activitypub.Activity, inboxURL string, privateKey string, keyID string) error {
	log := common.WithContext(ctx)

	// Parse the PEM-encoded private key
	key, err := federation.ParsePrivateKeyPEM([]byte(privateKey))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	// Marshal activity to JSON
	body, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", inboxURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Accept", "application/activity+json")

	// Sign the request
	if err := federation.SignHTTPRequest(req, key, keyID); err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	// Send the request
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 400 {
		return fmt.Errorf("delivery failed with status %d", resp.StatusCode)
	}

	log.Info("activity delivered successfully",
		zap.String("activity_id", activity.ID),
		zap.String("inbox", inboxURL),
		zap.Int("status", resp.StatusCode))

	return nil
}

func fetchRemoteActor(ctx context.Context, actorURL string) (*activitypub.Actor, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", actorURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/activity+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch actor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch actor: status %d", resp.StatusCode)
	}

	var actor activitypub.Actor
	if err := json.NewDecoder(resp.Body).Decode(&actor); err != nil {
		return nil, fmt.Errorf("failed to decode actor: %w", err)
	}

	return &actor, nil
}

// extractUsernameFromActorID extracts username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func extractUsernameFromActorID(actorID string) string {
	parts := strings.Split(actorID, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

func main() {
	lambda.Start(handler)
}
