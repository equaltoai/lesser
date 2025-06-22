package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/notifications"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"go.uber.org/zap"
)

// ActivityDirection represents the direction of an activity
type ActivityDirection string

const (
	ActivityDirectionInbox  ActivityDirection = "inbox"
	ActivityDirectionOutbox ActivityDirection = "outbox"
)

var (
	store            storage.Storage
	logger           *zap.Logger
	httpClient       *http.Client
	pushService      *notifications.PushService
	comprehendClient *comprehend.Client
)

func init() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}

	// HTTP client with timeout for delivery
	httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	// Initialize push service (can be nil if not configured)
	pushService, err = notifications.NewPushService()
	if err != nil {
		logger.Warn("Failed to initialize push service, push notifications will be disabled", zap.Error(err))
	}

	// Initialize AWS Comprehend client for language detection
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Warn("Failed to load AWS config, language detection will be disabled", zap.Error(err))
	} else {
		comprehendClient = comprehend.NewFromConfig(cfg)
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
		return nil, "", "", fmt.Errorf("activity attribute is not a map")
	}

	// Convert DynamoDB attribute map to JSON for unmarshaling
	activityJSON, err := convertDynamoDBMapToJSON(activityAttr.Map())
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to convert activity map: %w", err)
	}

	// Unmarshal activity
	var activity activitypub.Activity
	if err := common.ParseActivityPubObject(activityJSON, &activity); err != nil {
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
		// Process follow
		if err := processFollow(ctx, activity, recipientUsername); err != nil {
			log.Error("failed to process follow",
				zap.String("activity_id", activity.ID),
				zap.Error(err))
		}

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

	case activitypub.AnnounceType:
		// Store the announce
		return processAnnounce(ctx, activity, recipientUsername)

	case activitypub.DeleteType:
		// Process deletion
		return processDelete(ctx, activity, recipientUsername)

	case activitypub.UpdateType:
		// Process update
		return processUpdate(ctx, activity, recipientUsername)

	case activitypub.UndoType:
		// Process undo
		return processUndo(ctx, activity, recipientUsername)

	case activitypub.BlockType:
		// Process block
		return processBlock(ctx, activity, recipientUsername)

	case activitypub.FlagType:
		// Process flag (content moderation)
		return processFlag(ctx, activity, recipientUsername)

	case activitypub.MoveType:
		// Process move (account migration)
		return processMove(ctx, activity, recipientUsername)

	case activitypub.AddType:
		// Process add (add to collection)
		return processAdd(ctx, activity, recipientUsername)

	case activitypub.RemoveType:
		// Process remove (remove from collection)
		return processRemove(ctx, activity, recipientUsername)

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

	// Fan-out to timelines for Create activities
	if activity.Type == activitypub.CreateType {
		if err := fanOutToTimelines(ctx, activity); err != nil {
			log.Error("failed to fan out to timelines",
				zap.String("activity_id", activity.ID),
				zap.Error(err))
			// Don't fail the whole process if timeline fan-out fails
			// The activity has already been delivered
		}
	}

	// Fan-out Announce activities to timelines
	if activity.Type == activitypub.AnnounceType {
		if err := fanOutAnnounceToTimelines(ctx, activity); err != nil {
			log.Error("failed to fan out announce to timelines",
				zap.String("activity_id", activity.ID),
				zap.Error(err))
		}
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

	// Check if this is a local user being followed
	localActor, err := store.GetActor(ctx, recipientUsername)
	if err == nil && localActor != nil {
		// This is a local user being followed, create a notification
		notification := &storage.Notification{
			Type:      "follow",
			Username:  recipientUsername, // The person being followed
			AccountID: followerUsername,  // The person doing the following
			CreatedAt: time.Now(),
		}
		if err := store.CreateNotification(ctx, notification); err != nil {
			log.Warn("failed to create follow notification",
				zap.String("follower", followerUsername),
				zap.String("followed", recipientUsername),
				zap.Error(err))
		}

		// Queue push notification
		if pushService != nil {
			// Get follower info for notification
			followerActor, err := store.GetActor(ctx, followerUsername)
			if err == nil {
				displayName := followerActor.Name
				if displayName == "" {
					displayName = followerActor.PreferredUsername
				}

				pushMsg := &notifications.PushMessage{
					Username:         recipientUsername,
					NotificationType: "follow",
					Title:            notifications.FormatNotificationTitle("follow", displayName),
					Body:             "",
					Icon:             followerActor.Icon.URL,
					NotificationID:   notification.ID,
					AccessToken:      "", // Will be populated by client
				}

				if err := pushService.QueueNotification(ctx, pushMsg); err != nil {
					log.Warn("failed to queue push notification",
						zap.String("type", "follow"),
						zap.String("username", recipientUsername),
						zap.Error(err))
				}
			}
		}
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

	// Create a follow notification for the follower
	notification := &storage.Notification{
		Type:      "follow",
		Username:  followerUsername,
		AccountID: recipientUsername,
		CreatedAt: time.Now(),
	}
	if err := store.CreateNotification(ctx, notification); err != nil {
		log.Warn("failed to create follow notification",
			zap.String("follower", followerUsername),
			zap.String("followed", recipientUsername),
			zap.Error(err))
		// Don't fail the whole operation if notification creation fails
	}

	return nil
}

func processCreate(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing create activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract and store the object
	if obj, ok := activity.Object.(map[string]interface{}); ok {
		// Convert the object map to our Object type
		object := &dynamodb.Object{
			ID:           getStringField(obj, "id"),
			Type:         getStringField(obj, "type"),
			Content:      getStringField(obj, "content"),
			AttributedTo: activity.Actor,
			Summary:      getStringField(obj, "summary"),
			Name:         getStringField(obj, "name"),
			URL:          getStringField(obj, "url"),
			Sensitive:    getBoolField(obj, "sensitive"),
		}

		// Set published time
		if publishedStr := getStringField(obj, "published"); publishedStr != "" {
			if published, err := time.Parse(time.RFC3339, publishedStr); err == nil {
				object.Published = published
			} else {
				object.Published = time.Now()
			}
		} else {
			object.Published = time.Now()
		}

		// Handle addressing
		object.To = getStringSliceField(obj, "to")
		object.CC = getStringSliceField(obj, "cc")

		// Handle attachments
		if attachments, ok := obj["attachment"].([]interface{}); ok {
			for _, att := range attachments {
				if attMap, ok := att.(map[string]interface{}); ok {
					attachment := dynamodb.ObjectAttachment{
						Type:      getStringField(attMap, "type"),
						URL:       getStringField(attMap, "url"),
						MediaType: getStringField(attMap, "mediaType"),
						Name:      getStringField(attMap, "name"),
						Width:     getIntField(attMap, "width"),
						Height:    getIntField(attMap, "height"),
					}
					object.Attachment = append(object.Attachment, attachment)
				}
			}
		}

		// Handle tags
		if tags, ok := obj["tag"].([]interface{}); ok {
			for _, tag := range tags {
				if tagMap, ok := tag.(map[string]interface{}); ok {
					objectTag := dynamodb.ObjectTag{
						Type: getStringField(tagMap, "type"),
						Href: getStringField(tagMap, "href"),
						Name: getStringField(tagMap, "name"),
					}
					object.Tag = append(object.Tag, objectTag)
				}
			}
		}

		// Handle inReplyTo
		if inReplyTo := getStringField(obj, "inReplyTo"); inReplyTo != "" {
			object.InReplyTo = &inReplyTo
		}

		// Store the object
		err := store.CreateObject(ctx, object)
		if err != nil {
			return fmt.Errorf("failed to create object: %w", err)
		}

		log.Info("object created from activity",
			zap.String("object_id", object.ID),
			zap.String("type", object.Type))

		// Check for mentions in the object tags
		if object.Type == "Note" {
			for _, tag := range object.Tag {
				if tag.Type == "Mention" && tag.Href != "" {
					// Extract mentioned username from href
					mentionedUsername := extractUsernameFromActorID(tag.Href)

					// Check if this is a local user
					if mentionedActor, err := store.GetActor(ctx, mentionedUsername); err == nil && mentionedActor != nil {
						// Create mention notification
						notification := &storage.Notification{
							Type:      "mention",
							Username:  mentionedUsername,
							AccountID: extractUsernameFromActorID(activity.Actor),
							StatusID:  object.ID,
							CreatedAt: time.Now(),
						}
						if err := store.CreateNotification(ctx, notification); err != nil {
							log.Warn("failed to create mention notification",
								zap.String("mentioned", mentionedUsername),
								zap.String("actor", activity.Actor),
								zap.String("object", object.ID),
								zap.Error(err))
						}
					}
				}
			}
		}
	} else {
		log.Warn("activity object is not a map, skipping object creation",
			zap.String("activity_id", activity.ID))
	}

	return nil
}

// Helper functions for extracting fields from map[string]interface{}
func getStringField(m map[string]interface{}, key string) string {
	// Try lowercase key first
	if val, ok := m[key].(string); ok {
		return val
	}
	// Try uppercase key (for DynamoDB field names)
	upperKey := strings.ToUpper(key[:1]) + key[1:]
	if val, ok := m[upperKey].(string); ok {
		return val
	}
	// Try all uppercase
	if val, ok := m[strings.ToUpper(key)].(string); ok {
		return val
	}
	return ""
}

func getBoolField(m map[string]interface{}, key string) bool {
	// Try lowercase key first
	if val, ok := m[key].(bool); ok {
		return val
	}
	// Try uppercase key (for DynamoDB field names)
	upperKey := strings.ToUpper(key[:1]) + key[1:]
	if val, ok := m[upperKey].(bool); ok {
		return val
	}
	// Try all uppercase
	if val, ok := m[strings.ToUpper(key)].(bool); ok {
		return val
	}
	return false
}

func getIntField(m map[string]interface{}, key string) int {
	// Try lowercase key first
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	if val, ok := m[key].(int); ok {
		return val
	}
	// Try uppercase key (for DynamoDB field names)
	upperKey := strings.ToUpper(key[:1]) + key[1:]
	if val, ok := m[upperKey].(float64); ok {
		return int(val)
	}
	if val, ok := m[upperKey].(int); ok {
		return val
	}
	// Try all uppercase
	if val, ok := m[strings.ToUpper(key)].(float64); ok {
		return int(val)
	}
	if val, ok := m[strings.ToUpper(key)].(int); ok {
		return val
	}
	return 0
}

func getStringSliceField(m map[string]interface{}, key string) []string {
	var result []string
	// Helper function to process the value
	processValue := func(val interface{}) {
		switch v := val.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
		case []string:
			result = v
		case string:
			result = []string{v}
		}
	}

	// Try lowercase key first
	if val, ok := m[key]; ok {
		processValue(val)
		return result
	}
	// Try uppercase key (for DynamoDB field names)
	upperKey := strings.ToUpper(key[:1]) + key[1:]
	if val, ok := m[upperKey]; ok {
		processValue(val)
		return result
	}
	// Try all uppercase
	if val, ok := m[strings.ToUpper(key)]; ok {
		processValue(val)
		return result
	}
	return result
}

func processLike(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing like activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract object ID from the activity
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if objectID == "" {
		return fmt.Errorf("like activity missing object ID")
	}

	// Create Like record
	like := &storage.Like{
		Actor:  activity.Actor,
		Object: objectID,
		ID:     activity.ID,
	}

	// Set published time
	if activity.Published != nil {
		like.Published = *activity.Published
	} else {
		like.Published = time.Now()
	}

	// Store the like
	err := store.CreateLike(ctx, like)
	if err != nil {
		// Check if it's a duplicate like (not an error)
		if strings.Contains(err.Error(), "already liked") {
			log.Info("duplicate like ignored",
				zap.String("actor", activity.Actor),
				zap.String("object", objectID))
			return nil
		}
		return fmt.Errorf("failed to create like: %w", err)
	}

	log.Info("like stored successfully",
		zap.String("actor", activity.Actor),
		zap.String("object", objectID))

	// Check if the liked object belongs to our local user
	// Get the object to find out who created it
	obj, err := store.GetObject(ctx, objectID)
	if err == nil {
		// Extract the owner of the object
		var objectOwner string
		switch v := obj.(type) {
		case *dynamodb.Object:
			// Extract username from AttributedTo
			if v.AttributedTo != "" {
				objectOwner = extractUsernameFromActorID(v.AttributedTo)
			}
		case *activitypub.Note:
			if v.AttributedTo != "" {
				objectOwner = extractUsernameFromActorID(v.AttributedTo)
			}
		case map[string]interface{}:
			if attr, ok := v["attributedTo"].(string); ok {
				objectOwner = extractUsernameFromActorID(attr)
			}
		}

		// If this is a local user's object, create a notification
		if objectOwner != "" && objectOwner == recipientUsername {
			notification := &storage.Notification{
				Type:      "favourite",
				Username:  objectOwner,
				AccountID: extractUsernameFromActorID(activity.Actor),
				StatusID:  objectID,
				CreatedAt: time.Now(),
			}
			if err := store.CreateNotification(ctx, notification); err != nil {
				log.Warn("failed to create favourite notification",
					zap.String("actor", activity.Actor),
					zap.String("object", objectID),
					zap.Error(err))
			}

			// Queue push notification
			if pushService != nil {
				// Get actor info for notification
				actorUsername := extractUsernameFromActorID(activity.Actor)
				actor, err := store.GetActor(ctx, actorUsername)
				if err == nil {
					displayName := actor.Name
					if displayName == "" {
						displayName = actor.PreferredUsername
					}

					pushMsg := &notifications.PushMessage{
						Username:         objectOwner,
						NotificationType: "favourite",
						Title:            notifications.FormatNotificationTitle("favourite", displayName),
						Body:             "",
						Icon:             actor.Icon.URL,
						NotificationID:   notification.ID,
						AccessToken:      "", // Will be populated by client
					}

					if err := pushService.QueueNotification(ctx, pushMsg); err != nil {
						log.Warn("failed to queue push notification",
							zap.String("type", "favourite"),
							zap.String("username", objectOwner),
							zap.Error(err))
					}
				}
			}
		}
	}

	return nil
}

func processAnnounce(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing announce activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract object ID from the activity
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if objectID == "" {
		return fmt.Errorf("announce activity missing object ID")
	}

	// Create Announce record
	announce := &storage.Announce{
		Actor:  activity.Actor,
		Object: objectID,
		ID:     activity.ID,
		To:     convertToStringSlice(activity.To),
		CC:     convertToStringSlice(activity.CC),
	}

	// Set published time
	if activity.Published != nil {
		announce.Published = *activity.Published
	} else {
		announce.Published = time.Now()
	}

	// Store the announce
	err := store.CreateAnnounce(ctx, announce)
	if err != nil {
		// Check if it's a duplicate announce (not an error)
		if strings.Contains(err.Error(), "already announced") {
			log.Info("duplicate announce ignored",
				zap.String("actor", activity.Actor),
				zap.String("object", objectID))
			return nil
		}
		return fmt.Errorf("failed to create announce: %w", err)
	}

	log.Info("announce stored successfully",
		zap.String("actor", activity.Actor),
		zap.String("object", objectID))

	// Check if the announced object belongs to our local user
	// Get the object to find out who created it
	obj, err := store.GetObject(ctx, objectID)
	if err == nil {
		// Extract the owner of the object
		var objectOwner string
		switch v := obj.(type) {
		case *dynamodb.Object:
			// Extract username from AttributedTo
			if v.AttributedTo != "" {
				objectOwner = extractUsernameFromActorID(v.AttributedTo)
			}
		case *activitypub.Note:
			if v.AttributedTo != "" {
				objectOwner = extractUsernameFromActorID(v.AttributedTo)
			}
		case map[string]interface{}:
			if attr, ok := v["attributedTo"].(string); ok {
				objectOwner = extractUsernameFromActorID(attr)
			}
		}

		// If this is a local user's object, create a notification
		if objectOwner != "" && objectOwner == recipientUsername {
			notification := &storage.Notification{
				Type:      "reblog",
				Username:  objectOwner,
				AccountID: extractUsernameFromActorID(activity.Actor),
				StatusID:  objectID,
				CreatedAt: time.Now(),
			}
			if err := store.CreateNotification(ctx, notification); err != nil {
				log.Warn("failed to create reblog notification",
					zap.String("actor", activity.Actor),
					zap.String("object", objectID),
					zap.Error(err))
			}
		}
	}

	return nil
}

func processDelete(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing delete activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract object ID from the activity
	var objectID string
	var objectType string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
		if t, ok := obj["type"].(string); ok {
			objectType = t
		}
	}

	if objectID == "" {
		return fmt.Errorf("delete activity missing object ID")
	}

	// Check if we already have this object locally
	existingObj, err := store.GetObject(ctx, objectID)
	if err != nil {
		// Object not found locally - we can't delete what we don't have
		log.Info("object not found locally, ignoring delete",
			zap.String("object_id", objectID))
		return nil
	}

	// Check if it's already a tombstone
	if tombstone, ok := existingObj.(*storage.Tombstone); ok {
		log.Info("object already tombstoned",
			zap.String("object_id", objectID),
			zap.Time("deleted", tombstone.Deleted))
		return nil
	}

	// Verify the actor has permission to delete this object
	// For now, we'll check if the actor matches the attributedTo field
	var attributedTo string
	switch v := existingObj.(type) {
	case *dynamodb.Object:
		attributedTo = v.AttributedTo
	case map[string]interface{}:
		if attr, ok := v["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo != "" && attributedTo != activity.Actor {
		log.Warn("actor not authorized to delete object",
			zap.String("actor", activity.Actor),
			zap.String("attributed_to", attributedTo))
		return fmt.Errorf("actor not authorized to delete this object")
	}

	// Create tombstone
	err = store.TombstoneObject(ctx, objectID, activity.Actor)
	if err != nil {
		return fmt.Errorf("failed to tombstone object: %w", err)
	}

	log.Info("object tombstoned successfully",
		zap.String("object_id", objectID),
		zap.String("deleted_by", activity.Actor),
		zap.String("object_type", objectType))

	return nil
}

func processUpdate(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing update activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract object from the activity
	objMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		return fmt.Errorf("update activity object must be a map")
	}

	objectID, ok := objMap["id"].(string)
	if !ok || objectID == "" {
		return fmt.Errorf("update activity object missing id")
	}

	// Check if we have this object locally
	existingObj, err := store.GetObject(ctx, objectID)
	if err != nil {
		// Object not found locally - we can't update what we don't have
		log.Info("object not found locally, ignoring update",
			zap.String("object_id", objectID))
		return nil
	}

	// Check if it's a tombstone
	if _, ok := existingObj.(*storage.Tombstone); ok {
		log.Warn("cannot update a tombstoned object",
			zap.String("object_id", objectID))
		return fmt.Errorf("cannot update a deleted object")
	}

	// Verify the actor has permission to update this object
	var attributedTo string
	switch v := existingObj.(type) {
	case *dynamodb.Object:
		attributedTo = v.AttributedTo
	case map[string]interface{}:
		if attr, ok := v["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo != "" && attributedTo != activity.Actor {
		log.Warn("actor not authorized to update object",
			zap.String("actor", activity.Actor),
			zap.String("attributed_to", attributedTo))
		return fmt.Errorf("actor not authorized to update this object")
	}

	// Convert the object map to our Object type
	object := &dynamodb.Object{
		ID:           objectID,
		Type:         getStringField(objMap, "type"),
		Content:      getStringField(objMap, "content"),
		AttributedTo: activity.Actor,
		Summary:      getStringField(objMap, "summary"),
		Name:         getStringField(objMap, "name"),
		URL:          getStringField(objMap, "url"),
		Sensitive:    getBoolField(objMap, "sensitive"),
	}

	// Set updated time from activity
	if updatedStr := getStringField(objMap, "updated"); updatedStr != "" {
		if updated, err := time.Parse(time.RFC3339, updatedStr); err == nil {
			object.Updated = updated
		} else {
			object.Updated = time.Now()
		}
	} else {
		object.Updated = time.Now()
	}

	// Keep the original published time from existing object
	switch v := existingObj.(type) {
	case *dynamodb.Object:
		object.Published = v.Published
	case map[string]interface{}:
		if pubStr, ok := v["published"].(string); ok {
			if pub, err := time.Parse(time.RFC3339, pubStr); err == nil {
				object.Published = pub
			}
		}
	}

	// Handle addressing
	object.To = getStringSliceField(objMap, "to")
	object.CC = getStringSliceField(objMap, "cc")

	// Handle attachments
	if attachments, ok := objMap["attachment"].([]interface{}); ok {
		for _, att := range attachments {
			if attMap, ok := att.(map[string]interface{}); ok {
				attachment := dynamodb.ObjectAttachment{
					Type:      getStringField(attMap, "type"),
					URL:       getStringField(attMap, "url"),
					MediaType: getStringField(attMap, "mediaType"),
					Name:      getStringField(attMap, "name"),
					Width:     getIntField(attMap, "width"),
					Height:    getIntField(attMap, "height"),
				}
				object.Attachment = append(object.Attachment, attachment)
			}
		}
	}

	// Handle tags
	if tags, ok := objMap["tag"].([]interface{}); ok {
		for _, tag := range tags {
			if tagMap, ok := tag.(map[string]interface{}); ok {
				objectTag := dynamodb.ObjectTag{
					Type: getStringField(tagMap, "type"),
					Href: getStringField(tagMap, "href"),
					Name: getStringField(tagMap, "name"),
				}
				object.Tag = append(object.Tag, objectTag)
			}
		}
	}

	// Handle inReplyTo
	if inReplyTo := getStringField(objMap, "inReplyTo"); inReplyTo != "" {
		object.InReplyTo = &inReplyTo
	}

	// Update the object
	err = store.UpdateObject(ctx, object)
	if err != nil {
		return fmt.Errorf("failed to update object: %w", err)
	}

	log.Info("object updated successfully",
		zap.String("object_id", object.ID),
		zap.String("type", object.Type),
		zap.Time("updated", object.Updated))

	return nil
}

func processUndo(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing undo activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// The object of an Undo should be the activity being undone
	objMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		return fmt.Errorf("undo activity object must be a map")
	}

	// Get the type of the activity being undone
	undoType, ok := objMap["type"].(string)
	if !ok {
		return fmt.Errorf("undo activity object missing type")
	}

	switch undoType {
	case activitypub.FollowType:
		return processUndoFollow(ctx, activity, objMap, recipientUsername)
	case activitypub.LikeType:
		return processUndoLike(ctx, activity, objMap, recipientUsername)
	case activitypub.AnnounceType:
		return processUndoAnnounce(ctx, activity, objMap, recipientUsername)
	default:
		log.Warn("unhandled undo activity type",
			zap.String("undo_type", undoType),
			zap.String("activity_id", activity.ID))
		return nil
	}
}

func processUndoFollow(ctx context.Context, activity *activitypub.Activity, followActivity map[string]interface{}, _ string) error {
	log := common.WithContext(ctx)

	// Extract the original follow activity details
	followActor, ok := followActivity["actor"].(string)
	if !ok {
		return fmt.Errorf("undo follow missing actor")
	}

	followObject, ok := followActivity["object"].(string)
	if !ok {
		return fmt.Errorf("undo follow missing object")
	}

	// Verify the undo actor matches the original follow actor
	if activity.Actor != followActor {
		log.Warn("undo actor does not match follow actor",
			zap.String("undo_actor", activity.Actor),
			zap.String("follow_actor", followActor))
		return fmt.Errorf("actor mismatch in undo follow")
	}

	// Extract usernames
	followerUsername := extractUsernameFromActorID(followActor)
	followedUsername := extractUsernameFromActorID(followObject)

	if followerUsername == "" || followedUsername == "" {
		return fmt.Errorf("invalid actor IDs in follow activity")
	}

	log.Info("processing undo follow",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	// Remove the follow relationship
	err := store.RemoveFollow(ctx, followerUsername, followedUsername)
	if err != nil {
		// Log but don't fail if follow doesn't exist
		log.Info("follow relationship not found or already removed",
			zap.String("follower", followerUsername),
			zap.String("followed", followedUsername),
			zap.Error(err))
	}

	return nil
}

func processUndoLike(ctx context.Context, activity *activitypub.Activity, likeActivity map[string]interface{}, _ string) error {
	log := common.WithContext(ctx)

	// Extract the original like activity details
	likeActor, ok := likeActivity["actor"].(string)
	if !ok {
		return fmt.Errorf("undo like missing actor")
	}

	var likeObject string
	switch obj := likeActivity["object"].(type) {
	case string:
		likeObject = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			likeObject = id
		}
	}

	if likeObject == "" {
		return fmt.Errorf("undo like missing object")
	}

	// Verify the undo actor matches the original like actor
	if activity.Actor != likeActor {
		log.Warn("undo actor does not match like actor",
			zap.String("undo_actor", activity.Actor),
			zap.String("like_actor", likeActor))
		return fmt.Errorf("actor mismatch in undo like")
	}

	log.Info("processing undo like",
		zap.String("actor", likeActor),
		zap.String("object", likeObject))

	// Remove the like
	err := store.DeleteLike(ctx, likeActor, likeObject)
	if err != nil {
		// Log but don't fail if like doesn't exist
		log.Info("like not found or already removed",
			zap.String("actor", likeActor),
			zap.String("object", likeObject),
			zap.Error(err))
	}

	return nil
}

func processUndoAnnounce(ctx context.Context, activity *activitypub.Activity, announceActivity map[string]interface{}, _ string) error {
	log := common.WithContext(ctx)

	// Extract the original announce activity details
	announceActor, ok := announceActivity["actor"].(string)
	if !ok {
		return fmt.Errorf("undo announce missing actor")
	}

	var announceObject string
	switch obj := announceActivity["object"].(type) {
	case string:
		announceObject = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			announceObject = id
		}
	}

	if announceObject == "" {
		return fmt.Errorf("undo announce missing object")
	}

	// Verify the undo actor matches the original announce actor
	if activity.Actor != announceActor {
		log.Warn("undo actor does not match announce actor",
			zap.String("undo_actor", activity.Actor),
			zap.String("announce_actor", announceActor))
		return fmt.Errorf("actor mismatch in undo announce")
	}

	log.Info("processing undo announce",
		zap.String("actor", announceActor),
		zap.String("object", announceObject))

	// Remove the announce
	err := store.DeleteAnnounce(ctx, announceActor, announceObject)
	if err != nil {
		// Log but don't fail if announce doesn't exist
		log.Info("announce not found or already removed",
			zap.String("actor", announceActor),
			zap.String("object", announceObject),
			zap.Error(err))
	}

	return nil
}

// convertToStringSlice converts an interface{} to []string
func convertToStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return []string{val}
	default:
		return nil
	}
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
	// Check if recipientURL is a collection that needs to be resolved
	if isCollectionURL(recipientURL) {
		return resolveAndDeliverToCollection(ctx, activity, recipientURL)
	}

	// Handle direct actor URL
	return deliverToActor(ctx, activity, recipientURL)
}

func isCollectionURL(url string) bool {
	// Check if URL ends with known collection types
	return strings.HasSuffix(url, "/followers") ||
		strings.HasSuffix(url, "/following") ||
		strings.Contains(url, "/collections/")
}

func resolveAndDeliverToCollection(ctx context.Context, activity *activitypub.Activity, collectionURL string) error {
	log := logger.With(zap.String("collection_url", collectionURL))

	// Extract actor from collection URL (e.g., https://example.com/users/alice/followers -> alice)
	actorUsername := extractActorFromCollectionURL(collectionURL)
	if actorUsername == "" {
		log.Warn("could not extract actor from collection URL")
		return fmt.Errorf("invalid collection URL: %s", collectionURL)
	}

	var recipients []string
	var err error

	if strings.HasSuffix(collectionURL, "/followers") {
		// Get all followers of the actor
		recipients, _, err = store.GetFollowers(ctx, actorUsername, 1000, "")
		if err != nil {
			return fmt.Errorf("failed to get followers for collection: %w", err)
		}
	} else if strings.HasSuffix(collectionURL, "/following") {
		// Get all users the actor is following
		recipients, _, err = store.GetFollowing(ctx, actorUsername, 1000, "")
		if err != nil {
			return fmt.Errorf("failed to get following for collection: %w", err)
		}
	} else {
		// For other collections, try to fetch and parse the collection
		return fetchAndDeliverToRemoteCollection(ctx, activity, collectionURL)
	}

	// Deliver to each recipient in the collection
	for _, recipientUsername := range recipients {
		recipientActor, err := store.GetActor(ctx, recipientUsername)
		if err != nil {
			log.Warn("failed to get recipient actor", zap.String("username", recipientUsername), zap.Error(err))
			continue
		}

		if err := deliverToActor(ctx, activity, recipientActor.ID); err != nil {
			log.Warn("failed to deliver to collection member",
				zap.String("recipient", recipientActor.ID),
				zap.Error(err))
		}
	}

	return nil
}

func extractActorFromCollectionURL(collectionURL string) string {
	// Parse URL to extract username (e.g., https://example.com/users/alice/followers -> alice)
	parts := strings.Split(collectionURL, "/")
	for i, part := range parts {
		if part == "users" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func fetchAndDeliverToRemoteCollection(ctx context.Context, activity *activitypub.Activity, collectionURL string) error {
	log := logger.With(zap.String("collection_url", collectionURL))

	log.Info("fetching remote collection for delivery")

	// Create request to fetch the collection
	req, err := http.NewRequestWithContext(ctx, "GET", collectionURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create collection request: %w", err)
	}

	req.Header.Set("Accept", "application/activity+json")

	// Fetch the collection
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("collection fetch failed with status %d", resp.StatusCode)
	}

	// Parse the collection
	var collection map[string]interface{}
	if err := common.ParseHTTPResponse(resp.Body, &collection); err != nil {
		return fmt.Errorf("failed to parse collection: %w", err)
	}

	// Check collection type
	collType, ok := collection["type"].(string)
	if !ok {
		return fmt.Errorf("collection missing type field")
	}

	var members []string

	switch collType {
	case "Collection":
		// Direct Collection - get items
		if items, ok := collection["items"].([]interface{}); ok {
			for _, item := range items {
				if itemStr, ok := item.(string); ok {
					members = append(members, itemStr)
				}
			}
		}

	case "OrderedCollection":
		// OrderedCollection - get orderedItems
		if items, ok := collection["orderedItems"].([]interface{}); ok {
			for _, item := range items {
				if itemStr, ok := item.(string); ok {
					members = append(members, itemStr)
				}
			}
		}

	case "CollectionPage", "OrderedCollectionPage":
		// This is a paginated collection
		return fetchAndDeliverToPaginatedCollection(ctx, activity, collection)

	default:
		return fmt.Errorf("unsupported collection type: %s", collType)
	}

	// If this is a paginated collection with a first page, fetch that
	if first, ok := collection["first"].(string); ok && len(members) == 0 {
		return fetchAndDeliverToRemoteCollection(ctx, activity, first)
	}

	// Deliver to each member
	var deliveryErrors []error
	for _, memberURL := range members {
		if err := deliverToActor(ctx, activity, memberURL); err != nil {
			log.Warn("failed to deliver to collection member",
				zap.String("member", memberURL),
				zap.Error(err))
			deliveryErrors = append(deliveryErrors, err)
		}
	}

	log.Info("collection delivery completed",
		zap.Int("total_members", len(members)),
		zap.Int("delivery_errors", len(deliveryErrors)))

	if len(deliveryErrors) > 0 {
		return fmt.Errorf("delivery failed to %d out of %d collection members", len(deliveryErrors), len(members))
	}

	return nil
}

// fetchAndDeliverToPaginatedCollection handles paginated collections
func fetchAndDeliverToPaginatedCollection(ctx context.Context, activity *activitypub.Activity, page map[string]interface{}) error {
	log := logger.With(zap.String("operation", "paginated_collection"))

	var allMembers []string
	currentPage := page
	pageCount := 0
	maxPages := 50 // Reasonable limit to prevent infinite loops

	for pageCount < maxPages {
		pageCount++
		log.Debug("processing collection page", zap.Int("page_number", pageCount))

		// Extract members from current page
		var pageMembers []string
		pageType, ok := currentPage["type"].(string)
		if !ok {
			break
		}

		switch pageType {
		case "CollectionPage":
			if items, ok := currentPage["items"].([]interface{}); ok {
				for _, item := range items {
					if itemStr, ok := item.(string); ok {
						pageMembers = append(pageMembers, itemStr)
					}
				}
			}
		case "OrderedCollectionPage":
			if items, ok := currentPage["orderedItems"].([]interface{}); ok {
				for _, item := range items {
					if itemStr, ok := item.(string); ok {
						pageMembers = append(pageMembers, itemStr)
					}
				}
			}
		}

		allMembers = append(allMembers, pageMembers...)

		// Check for next page
		nextURL, hasNext := currentPage["next"].(string)
		if !hasNext || nextURL == "" {
			break
		}

		// Fetch next page
		req, err := http.NewRequestWithContext(ctx, "GET", nextURL, nil)
		if err != nil {
			log.Warn("failed to create next page request", zap.String("next_url", nextURL), zap.Error(err))
			break
		}

		req.Header.Set("Accept", "application/activity+json")

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Warn("failed to fetch next page", zap.String("next_url", nextURL), zap.Error(err))
			break
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Warn("next page fetch failed", zap.String("next_url", nextURL), zap.Int("status", resp.StatusCode))
			break
		}

		var nextPage map[string]interface{}
		if err := common.ParseHTTPResponse(resp.Body, &nextPage); err != nil {
			resp.Body.Close()
			log.Warn("failed to parse next page", zap.String("next_url", nextURL), zap.Error(err))
			break
		}
		resp.Body.Close()

		currentPage = nextPage
	}

	if pageCount >= maxPages {
		log.Warn("reached maximum page limit for collection", zap.Int("max_pages", maxPages))
	}

	// Deliver to all collected members
	var deliveryErrors []error
	for _, memberURL := range allMembers {
		if err := deliverToActor(ctx, activity, memberURL); err != nil {
			log.Warn("failed to deliver to paginated collection member",
				zap.String("member", memberURL),
				zap.Error(err))
			deliveryErrors = append(deliveryErrors, err)
		}
	}

	log.Info("paginated collection delivery completed",
		zap.Int("total_pages", pageCount),
		zap.Int("total_members", len(allMembers)),
		zap.Int("delivery_errors", len(deliveryErrors)))

	if len(deliveryErrors) > 0 {
		return fmt.Errorf("delivery failed to %d out of %d paginated collection members", len(deliveryErrors), len(allMembers))
	}

	return nil
}

func deliverToActor(ctx context.Context, activity *activitypub.Activity, actorURL string) error {
	// Fetch the recipient actor to get their inbox
	actor, err := fetchRemoteActor(ctx, actorURL)
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
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		return nil, fmt.Errorf("failed to decode actor: %w", err)
	}

	return &actor, nil
}

// extractUsernameFromActorID extracts username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func extractUsernameFromActorID(actorID string) string {
	// Try to extract username from various actor ID formats
	// e.g., "https://example.com/users/alice" -> "alice"
	parts := strings.Split(actorID, "/users/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func processBlock(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing block activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract object (the actor being blocked)
	var blockedActor string
	switch obj := activity.Object.(type) {
	case string:
		blockedActor = obj
	case map[string]interface{}:
		if actor, ok := obj["id"].(string); ok {
			blockedActor = actor
		}
	}

	if blockedActor == "" {
		return fmt.Errorf("invalid block object")
	}

	block := &storage.Block{
		Actor:     activity.Actor,
		Object:    blockedActor,
		ID:        activity.ID,
		Published: time.Now(),
		CreatedAt: time.Now(),
	}

	err := store.CreateBlock(ctx, block)
	if err != nil {
		return fmt.Errorf("failed to create block: %w", err)
	}

	log.Info("block created",
		zap.String("actor", activity.Actor),
		zap.String("blocked", blockedActor))

	return nil
}

func processFlag(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing flag activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract objects being flagged
	var flaggedObjects []string

	switch obj := activity.Object.(type) {
	case string:
		flaggedObjects = []string{obj}
	case []interface{}:
		for _, item := range obj {
			switch v := item.(type) {
			case string:
				flaggedObjects = append(flaggedObjects, v)
			case map[string]interface{}:
				if id, ok := v["id"].(string); ok {
					flaggedObjects = append(flaggedObjects, id)
				}
			}
		}
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			flaggedObjects = []string{id}
		}
	}

	if len(flaggedObjects) == 0 {
		return fmt.Errorf("no objects to flag in activity")
	}

	// Extract content/reason for the flag
	content := ""

	// Check if the activity itself has a summary
	if activity.Summary != "" {
		content = activity.Summary
	}

	// Also check if there's a content field in the activity object
	if objMap, ok := activity.Object.(map[string]interface{}); ok {
		if c, ok := objMap["content"].(string); ok && c != "" {
			content = c
		}
	}

	// Create the flag
	flag := &storage.Flag{
		ID:        activity.ID,
		Actor:     activity.Actor,
		Object:    flaggedObjects,
		Content:   content,
		Published: time.Now(),
		Status:    storage.FlagStatusPending,
		CreatedAt: time.Now(),
	}

	// If the activity has a published date, use that
	if activity.Published != nil {
		flag.Published = *activity.Published
	}

	err := store.CreateFlag(ctx, flag)
	if err != nil {
		return fmt.Errorf("failed to create flag: %w", err)
	}

	log.Info("flag created",
		zap.String("actor", activity.Actor),
		zap.Strings("objects", flaggedObjects),
		zap.String("content", content))

	// TODO: In production, you might want to:
	// 1. Send notifications to moderators
	// 2. Auto-hide content if multiple flags
	// 3. Apply machine learning for spam detection
	// 4. Rate limit flags from the same actor

	return nil
}

func processMove(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing move activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract target (the new account location)
	var target string

	// Check if activity has a direct target field
	if moveActivity, ok := activity.Object.(map[string]interface{}); ok {
		if t, ok := moveActivity["target"].(string); ok {
			target = t
		}
	}

	// If not found in object, check for target in activity itself
	if target == "" {
		switch v := activity.Object.(type) {
		case string:
			// If object is a string, it might be the target
			target = v
		case map[string]interface{}:
			// Look for target field
			if t, ok := v["target"].(string); ok {
				target = t
			}
		}
	}

	if target == "" {
		return fmt.Errorf("move activity missing target")
	}

	// Create the move record
	move := &storage.Move{
		ID:        activity.ID,
		Actor:     activity.Actor,
		Target:    target,
		Published: time.Now(),
	}

	if activity.Published != nil {
		move.Published = *activity.Published
	}

	err := store.CreateMove(ctx, move)
	if err != nil {
		return fmt.Errorf("failed to create move: %w", err)
	}

	log.Info("move created",
		zap.String("actor", activity.Actor),
		zap.String("target", target))

	// TODO: In production, you might want to:
	// 1. Update followers to follow the new account
	// 2. Redirect profile requests
	// 3. Send notifications

	return nil
}

func processAdd(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing add activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract object being added
	var objectID string
	var objectType string = "Object" // Default type

	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
		// objectType already defaulted to "Object"
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
		if t, ok := obj["type"].(string); ok && t != "" {
			objectType = t
		}
		// If type is not specified or empty, use default "Object"
	}

	if objectID == "" {
		return fmt.Errorf("add activity missing object")
	}

	// Extract target collection
	var target string

	// Check for target field in activity
	if addActivity, ok := activity.Object.(map[string]interface{}); ok {
		if t, ok := addActivity["target"].(string); ok {
			target = t
		}
	}

	if target == "" {
		return fmt.Errorf("add activity missing target collection")
	}

	// Create collection item
	item := &storage.CollectionItem{
		Collection: target,
		ItemID:     objectID,
		ItemType:   objectType,
		AddedBy:    activity.Actor,
	}

	err := store.AddToCollection(ctx, target, item)
	if err != nil {
		return fmt.Errorf("failed to add to collection: %w", err)
	}

	log.Info("item added to collection",
		zap.String("item", objectID),
		zap.String("collection", target),
		zap.String("added_by", activity.Actor))

	return nil
}

func processRemove(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
	log := common.WithContext(ctx)

	log.Info("processing remove activity",
		zap.String("activity_id", activity.ID),
		zap.String("recipient", recipientUsername))

	// Extract object being removed
	var objectID string

	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if objectID == "" {
		return fmt.Errorf("remove activity missing object")
	}

	// Extract target collection
	var target string

	// Check for target field in activity
	if removeActivity, ok := activity.Object.(map[string]interface{}); ok {
		if t, ok := removeActivity["target"].(string); ok {
			target = t
		}
	}

	if target == "" {
		return fmt.Errorf("remove activity missing target collection")
	}

	err := store.RemoveFromCollection(ctx, target, objectID)
	if err != nil {
		return fmt.Errorf("failed to remove from collection: %w", err)
	}

	log.Info("item removed from collection",
		zap.String("item", objectID),
		zap.String("collection", target),
		zap.String("removed_by", activity.Actor))

	return nil
}

// fanOutToTimelines writes a Create activity to relevant timelines
func fanOutToTimelines(ctx context.Context, activity *activitypub.Activity) error {
	log := common.WithContext(ctx)

	// Extract the object from the activity
	objMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		log.Warn("create activity object is not a map, skipping timeline fan-out")
		return nil
	}

	// Extract key fields from the object
	objectID := getStringField(objMap, "id")
	objectType := getStringField(objMap, "type")
	content := getStringField(objMap, "content")
	summary := getStringField(objMap, "summary")
	sensitive := getBoolField(objMap, "sensitive")

	// Extract actor username
	actorUsername := extractUsernameFromActorID(activity.Actor)
	if actorUsername == "" {
		return fmt.Errorf("invalid actor ID: %s", activity.Actor)
	}

	// Determine visibility from addressing
	to := getStringSliceField(objMap, "to")
	cc := getStringSliceField(objMap, "cc")
	visibility := determineVisibility(to, cc)

	// Create base timeline entry
	now := time.Now()
	baseEntry := &storage.TimelineEntry{
		PostID:      objectID,
		ActorID:     activity.Actor,
		ActorHandle: fmt.Sprintf("@%s", actorUsername),
		Content:     truncateContent(content, 500), // First 500 chars for preview
		ContentType: objectType,
		HasMedia:    hasMediaAttachments(objMap),
		IsReply:     getStringField(objMap, "inReplyTo") != "",
		InReplyTo:   getStringField(objMap, "inReplyTo"),
		IsBoost:     false,
		Visibility:  visibility,
		Language:    extractLanguage(objMap),
		Sensitive:   sensitive,
		SpoilerText: summary,
		CreatedAt:   now,
		TimelineAt:  now,
		ExpiresAt:   now.Add(30 * 24 * time.Hour), // 30 days TTL
	}

	var timelineEntries []*storage.TimelineEntry

	// 1. Write to public timeline if public
	if visibility == "public" {
		// Add to FEDERATED timeline
		federatedEntry := *baseEntry
		federatedEntry.TimelineType = "PUBLIC"
		federatedEntry.TimelineID = "FEDERATED"
		federatedEntry.EntryID = fmt.Sprintf("%d#%s", now.Unix(), objectID)
		timelineEntries = append(timelineEntries, &federatedEntry)

		// Also add to LOCAL timeline since this is a local post
		cfg := config.Get()
		if strings.HasPrefix(activity.Actor, cfg.BaseURL()) {
			localEntry := *baseEntry
			localEntry.TimelineType = "PUBLIC"
			localEntry.TimelineID = "LOCAL"
			localEntry.EntryID = fmt.Sprintf("%d#%s", now.Unix(), objectID)
			timelineEntries = append(timelineEntries, &localEntry)
		}
	}

	// 2. Fan-out to followers' home timelines
	allFollowers := make([]string, 0)
	cursor := ""

	// Paginate through all followers
	for {
		followers, nextCursor, err := store.GetFollowers(ctx, actorUsername, 1000, cursor)
		if err != nil {
			log.Error("failed to get followers for timeline fan-out",
				zap.String("actor", actorUsername),
				zap.String("cursor", cursor),
				zap.Error(err))
			break
		}

		allFollowers = append(allFollowers, followers...)

		// If no more pages, break
		if nextCursor == "" || len(followers) < 1000 {
			break
		}
		cursor = nextCursor
	}

	// Create timeline entries for all followers
	for _, followerUsername := range allFollowers {
		followerEntry := *baseEntry
		followerEntry.TimelineType = "HOME"
		followerEntry.TimelineID = followerUsername
		followerEntry.EntryID = fmt.Sprintf("%d#%s", now.Unix(), objectID)
		timelineEntries = append(timelineEntries, &followerEntry)
	}

	// 3. Also add to the author's own home timeline
	authorEntry := *baseEntry
	authorEntry.TimelineType = "HOME"
	authorEntry.TimelineID = actorUsername
	authorEntry.EntryID = fmt.Sprintf("%d#%s", now.Unix(), objectID)
	timelineEntries = append(timelineEntries, &authorEntry)

	// Batch write to timelines
	if len(timelineEntries) > 0 {
		if err := store.WriteToTimelines(ctx, timelineEntries); err != nil {
			return fmt.Errorf("failed to write to timelines: %w", err)
		}
		log.Info("successfully fanned out to timelines",
			zap.String("object_id", objectID),
			zap.Int("timeline_count", len(timelineEntries)))
	}

	return nil
}

// fanOutAnnounceToTimelines writes an Announce activity to relevant timelines
func fanOutAnnounceToTimelines(ctx context.Context, activity *activitypub.Activity) error {
	log := common.WithContext(ctx)

	// Extract object ID from the activity
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if objectID == "" {
		return fmt.Errorf("announce activity missing object ID")
	}

	// Get the original object being announced
	originalObj, err := store.GetObject(ctx, objectID)
	if err != nil {
		log.Warn("cannot find announced object for timeline fan-out",
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil // Don't fail if we can't find the object
	}

	// Extract actor username
	actorUsername := extractUsernameFromActorID(activity.Actor)
	if actorUsername == "" {
		return fmt.Errorf("invalid actor ID: %s", activity.Actor)
	}

	// Create timeline entry from the announced object
	now := time.Now()
	var baseEntry *storage.TimelineEntry

	// Handle different object types
	switch obj := originalObj.(type) {
	case *dynamodb.Object:
		baseEntry = &storage.TimelineEntry{
			PostID:      obj.ID,
			ActorID:     obj.AttributedTo,
			ActorHandle: extractHandleFromActorID(obj.AttributedTo),
			Content:     truncateContent(obj.Content, 500),
			ContentType: obj.Type,
			HasMedia:    len(obj.Attachment) > 0,
			IsReply:     obj.InReplyTo != nil && *obj.InReplyTo != "",
			IsBoost:     true,
			BoostedBy:   activity.Actor,
			Visibility:  determineVisibility(obj.To, obj.CC),
			Language:    detectLanguage(ctx, obj.Content),
			Sensitive:   obj.Sensitive,
			SpoilerText: obj.Summary,
			CreatedAt:   obj.Published,
			TimelineAt:  now, // When it was boosted
			ExpiresAt:   now.Add(30 * 24 * time.Hour),
		}
		if obj.InReplyTo != nil {
			baseEntry.InReplyTo = *obj.InReplyTo
		}
	default:
		log.Warn("unknown object type for announce, skipping timeline fan-out")
		return nil
	}

	var timelineEntries []*storage.TimelineEntry

	// 1. Write to public timeline if the boost is public
	if isPubliclyAddressed(activity.To, activity.CC) {
		// Add to FEDERATED timeline
		federatedEntry := *baseEntry
		federatedEntry.TimelineType = "PUBLIC"
		federatedEntry.TimelineID = "FEDERATED"
		federatedEntry.EntryID = fmt.Sprintf("%d#announce#%s", now.Unix(), activity.ID)
		timelineEntries = append(timelineEntries, &federatedEntry)

		// Only add to LOCAL timeline if the announcer is a local user
		cfg := config.Get()
		if strings.HasPrefix(activity.Actor, cfg.BaseURL()) {
			localEntry := *baseEntry
			localEntry.TimelineType = "PUBLIC"
			localEntry.TimelineID = "LOCAL"
			localEntry.EntryID = fmt.Sprintf("%d#announce#%s", now.Unix(), activity.ID)
			timelineEntries = append(timelineEntries, &localEntry)
		}
	}

	// 2. Fan-out to followers' home timelines
	followers, _, err := store.GetFollowers(ctx, actorUsername, 1000, "")
	if err != nil {
		log.Error("failed to get followers for announce fan-out",
			zap.String("actor", actorUsername),
			zap.Error(err))
	} else {
		for _, followerUsername := range followers {
			followerEntry := *baseEntry
			followerEntry.TimelineType = "HOME"
			followerEntry.TimelineID = followerUsername
			followerEntry.EntryID = fmt.Sprintf("%d#announce#%s", now.Unix(), activity.ID)
			timelineEntries = append(timelineEntries, &followerEntry)
		}
	}

	// 3. Also add to the announcer's own home timeline
	authorEntry := *baseEntry
	authorEntry.TimelineType = "HOME"
	authorEntry.TimelineID = actorUsername
	authorEntry.EntryID = fmt.Sprintf("%d#announce#%s", now.Unix(), activity.ID)
	timelineEntries = append(timelineEntries, &authorEntry)

	// Batch write to timelines
	if len(timelineEntries) > 0 {
		if err := store.WriteToTimelines(ctx, timelineEntries); err != nil {
			return fmt.Errorf("failed to write announce to timelines: %w", err)
		}
		log.Info("successfully fanned out announce to timelines",
			zap.String("announce_id", activity.ID),
			zap.Int("timeline_count", len(timelineEntries)))
	}

	return nil
}

// Helper functions for timeline fan-out

func determineVisibility(to, cc []string) string {
	hasPublic := false
	hasFollowers := false

	for _, addr := range to {
		if addr == activitypub.PublicAddress {
			hasPublic = true
		}
		if strings.Contains(addr, "/followers") {
			hasFollowers = true
		}
	}

	for _, addr := range cc {
		if addr == activitypub.PublicAddress {
			hasPublic = true
		}
		if strings.Contains(addr, "/followers") {
			hasFollowers = true
		}
	}

	if hasPublic {
		return "public"
	} else if hasFollowers {
		return "private"
	}
	return "direct"
}

func isPubliclyAddressed(to, cc []string) bool {
	allAddrs := append(to, cc...)
	for _, addr := range allAddrs {
		if addr == activitypub.PublicAddress {
			return true
		}
	}
	return false
}

func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

func hasMediaAttachments(objMap map[string]interface{}) bool {
	attachments, ok := objMap["attachment"].([]interface{})
	return ok && len(attachments) > 0
}

func extractLanguage(objMap map[string]interface{}) string {
	// Check for contentMap first (Mastodon style)
	if contentMap, ok := objMap["contentMap"].(map[string]interface{}); ok {
		// Return the first language found
		for lang := range contentMap {
			return lang
		}
	}
	// Default to English
	return "en"
}

func extractHandleFromActorID(actorID string) string {
	username := extractUsernameFromActorID(actorID)
	if username != "" {
		return fmt.Sprintf("@%s", username)
	}
	// For remote actors, try to extract domain too
	if strings.Contains(actorID, "://") {
		parts := strings.Split(actorID, "://")
		if len(parts) >= 2 {
			domainPath := parts[1]
			domainParts := strings.Split(domainPath, "/")
			if len(domainParts) > 0 {
				domain := domainParts[0]
				if username != "" {
					return fmt.Sprintf("@%s@%s", username, domain)
				}
			}
		}
	}
	return "@unknown"
}

// detectLanguage uses AWS Comprehend to detect the language of content
func detectLanguage(ctx context.Context, content string) string {
	// Return default language if no comprehend client
	if comprehendClient == nil {
		return "en"
	}

	// Clean content for language detection
	cleanContent := cleanTextForLanguageDetection(content)

	// Skip detection for very short content
	if len(cleanContent) < 10 {
		return "en"
	}

	// Truncate to Comprehend's limit (5000 bytes for language detection)
	if len(cleanContent) > 5000 {
		cleanContent = cleanContent[:5000]
	}

	// Call AWS Comprehend
	input := &comprehend.DetectDominantLanguageInput{
		Text: aws.String(cleanContent),
	}

	result, err := comprehendClient.DetectDominantLanguage(ctx, input)
	if err != nil {
		logger.Debug("language detection failed, using default",
			zap.String("content_preview", cleanContent[:min(50, len(cleanContent))]),
			zap.Error(err))
		return "en"
	}

	// Find the highest confidence language
	if len(result.Languages) > 0 {
		bestLang := result.Languages[0]
		for _, lang := range result.Languages {
			if lang.Score != nil && bestLang.Score != nil && *lang.Score > *bestLang.Score {
				bestLang = lang
			}
		}

		// Only use the detected language if confidence is reasonable
		if bestLang.LanguageCode != nil && bestLang.Score != nil && *bestLang.Score > 0.5 {
			return *bestLang.LanguageCode
		}
	}

	// Default to English if detection fails or confidence is low
	return "en"
}

// cleanTextForLanguageDetection removes HTML tags and extra whitespace
func cleanTextForLanguageDetection(content string) string {
	// Remove HTML tags
	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)
	cleaned := htmlTagRegex.ReplaceAllString(content, " ")

	// Remove extra whitespace
	spaceRegex := regexp.MustCompile(`\s+`)
	cleaned = spaceRegex.ReplaceAllString(cleaned, " ")

	return strings.TrimSpace(cleaned)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	lambda.Start(handler)
}
