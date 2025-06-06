package dynamodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// Object represents a generic ActivityPub object stored in DynamoDB
type Object struct {
	ID             string             `dynamodbav:"id" json:"id"`
	Type           string             `dynamodbav:"type" json:"type"`
	AttributedTo   string             `dynamodbav:"attributedTo,omitempty" json:"attributedTo,omitempty"`
	Content        string             `dynamodbav:"content,omitempty" json:"content,omitempty"`
	Name           string             `dynamodbav:"name,omitempty" json:"name,omitempty"`
	Summary        string             `dynamodbav:"summary,omitempty" json:"summary,omitempty"`
	URL            string             `dynamodbav:"url,omitempty" json:"url,omitempty"`
	Published      time.Time          `dynamodbav:"published" json:"published"`
	Updated        time.Time          `dynamodbav:"updated,omitempty" json:"updated,omitempty"`
	To             []string           `dynamodbav:"to,omitempty" json:"to,omitempty"`
	CC             []string           `dynamodbav:"cc,omitempty" json:"cc,omitempty"`
	InReplyTo      *string            `dynamodbav:"inReplyTo,omitempty" json:"inReplyTo,omitempty"`
	Sensitive      bool               `dynamodbav:"sensitive,omitempty" json:"sensitive,omitempty"`
	Attachment     []ObjectAttachment `dynamodbav:"attachment,omitempty" json:"attachment,omitempty"`
	Tag            []ObjectTag        `dynamodbav:"tag,omitempty" json:"tag,omitempty"`
	Context        interface{}        `dynamodbav:"@context,omitempty" json:"@context,omitempty"`
	ConversationID string             `dynamodbav:"conversationId,omitempty" json:"conversationId,omitempty"`
}

// ObjectAttachment represents an attachment on an object
type ObjectAttachment struct {
	Type      string `dynamodbav:"type" json:"type"`
	URL       string `dynamodbav:"url" json:"url"`
	MediaType string `dynamodbav:"mediaType,omitempty" json:"mediaType,omitempty"`
	Name      string `dynamodbav:"name,omitempty" json:"name,omitempty"`
	Width     int    `dynamodbav:"width,omitempty" json:"width,omitempty"`
	Height    int    `dynamodbav:"height,omitempty" json:"height,omitempty"`
}

// ObjectTag represents a tag on an object
type ObjectTag struct {
	Type string `dynamodbav:"type" json:"type"`
	Href string `dynamodbav:"href,omitempty" json:"href,omitempty"`
	Name string `dynamodbav:"name" json:"name"`
}

// ObjectRecord represents an object stored in DynamoDB
type ObjectRecord struct {
	PK        string    `dynamodbav:"PK"`
	SK        string    `dynamodbav:"SK"`
	GSI1PK    string    `dynamodbav:"GSI1PK,omitempty"` // For actor's objects timeline
	GSI1SK    string    `dynamodbav:"GSI1SK,omitempty"` // Published timestamp
	Object    *Object   `dynamodbav:"Object"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt time.Time `dynamodbav:"UpdatedAt"`
}

// CreateObject creates a new object in DynamoDB
func (s *dynamoDBStorage) CreateObject(ctx context.Context, object interface{}) error {
	log := common.WithContext(ctx)

	// Convert the object to our internal Object type
	obj, err := convertToObject(object)
	if err != nil {
		return fmt.Errorf("failed to convert object: %w", err)
	}

	// Ensure object has an ID
	if obj.ID == "" {
		return errors.New("object must have an ID")
	}

	// Set timestamps
	if obj.Published.IsZero() {
		obj.Published = time.Now()
	}

	// Extract username from actor ID for GSI
	username := extractUsernameFromActorID(obj.AttributedTo)

	// Handle conversation tracking for Note types
	if obj.Type == "Note" || obj.Type == "Article" {
		if obj.InReplyTo != nil && *obj.InReplyTo != "" {
			// This is a reply - inherit conversation ID from parent
			parentObj, err := s.GetObject(ctx, *obj.InReplyTo)
			if err == nil {
				if parent, ok := parentObj.(*Object); ok && parent.ConversationID != "" {
					obj.ConversationID = parent.ConversationID

					// Update conversation with new status
					if err := s.UpdateConversationLastStatus(ctx, obj.ConversationID, obj.ID); err != nil {
						log.Warn("failed to update conversation last status",
							zap.String("conversation_id", obj.ConversationID),
							zap.Error(err))
					}

					// Add author to conversation if not already there
					if err := s.AddParticipantToConversation(ctx, obj.ConversationID, obj.AttributedTo); err != nil {
						log.Warn("failed to add participant to conversation",
							zap.String("conversation_id", obj.ConversationID),
							zap.String("participant", obj.AttributedTo),
							zap.Error(err))
					}
				}
			}
		} else {
			// This is a new post - generate conversation ID
			obj.ConversationID = generateRandomString(12)

			// Create conversation record
			conversation := &storage.Conversation{
				ID:           obj.ConversationID,
				Participants: []string{obj.AttributedTo},
				LastStatusID: obj.ID,
			}

			if err := s.CreateConversation(ctx, conversation); err != nil {
				log.Warn("failed to create conversation",
					zap.String("conversation_id", obj.ConversationID),
					zap.Error(err))
				// Don't fail the object creation if conversation creation fails
				obj.ConversationID = ""
			}
		}
	}

	// Create the DynamoDB record
	record := &ObjectRecord{
		PK:        fmt.Sprintf("OBJECT#%s", obj.ID),
		SK:        "METADATA",
		Object:    obj,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Add GSI fields if we have a valid username
	if username != "" {
		record.GSI1PK = fmt.Sprintf("ACTOR#%s#OBJECTS", username)
		record.GSI1SK = obj.Published.Format(time.RFC3339)
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal object",
			zap.String("object_id", obj.ID),
			zap.Error(err))
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	// Put with condition that the item doesn't already exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			log.Warn("object already exists",
				zap.String("object_id", obj.ID))
			return common.ConflictError{
				Resource: "object",
				Message:  fmt.Sprintf("object %s already exists", obj.ID),
			}
		}
		log.Error("failed to create object",
			zap.String("object_id", obj.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create object: %w", err)
	}

	log.Info("object created successfully",
		zap.String("object_id", obj.ID),
		zap.String("type", obj.Type),
		zap.String("attributed_to", obj.AttributedTo),
		zap.String("conversation_id", obj.ConversationID))

	// Update status count for Note and Article types
	if (obj.Type == "Note" || obj.Type == "Article") && username != "" {
		if err := s.UpdateStatusCount(ctx, username, 1); err != nil {
			log.Error("failed to update status count",
				zap.String("username", username),
				zap.Error(err))
			// Don't fail the operation, just log the error
		}

		// Index hashtags from the content
		if obj.Content != "" {
			hashtags := ExtractHashtags(obj.Content)
			visibility := "public" // Default visibility
			if len(obj.To) > 0 {
				// Determine visibility based on To field
				for _, to := range obj.To {
					if to == "https://www.w3.org/ns/activitystreams#Public" {
						visibility = "public"
						break
					}
				}
				if visibility != "public" && len(obj.CC) > 0 {
					for _, cc := range obj.CC {
						if cc == "https://www.w3.org/ns/activitystreams#Public" {
							visibility = "unlisted"
							break
						}
					}
				}
			}

			// Index each hashtag
			for _, hashtag := range hashtags {
				if err := s.IndexHashtag(ctx, hashtag, obj.ID, obj.AttributedTo, visibility); err != nil {
					log.Warn("failed to index hashtag",
						zap.String("hashtag", hashtag),
						zap.String("object_id", obj.ID),
						zap.Error(err))
					// Don't fail the operation, just log the error
				}
			}
		}
	}

	return nil
}

// GetObject retrieves an object by ID from DynamoDB
func (s *dynamoDBStorage) GetObject(ctx context.Context, id string) (interface{}, error) {
	log := common.WithContext(ctx)

	// First check if it's a tombstone
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "TOMBSTONE"},
		},
	})
	if err == nil && result.Item != nil {
		// It's a tombstone
		var tombstoneRecord struct {
			Tombstone *storage.Tombstone `dynamodbav:"Tombstone"`
		}
		if err := s.UnmarshalItem(result.Item, &tombstoneRecord); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tombstone: %w", err)
		}
		return tombstoneRecord.Tombstone, nil
	}

	// Try to get the regular object
	result, err = s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		log.Error("failed to get object",
			zap.String("object_id", id),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("object not found: %s", id)
	}

	// Unmarshal the object record using our wrapper that handles conversions
	var record ObjectRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		log.Error("failed to unmarshal object",
			zap.String("object_id", id),
			zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal object: %w", err)
	}

	// If Object is still nil after unmarshaling, try direct conversion
	if record.Object == nil {
		// Get the raw object data and convert it
		if objItem, ok := result.Item["Object"]; ok {
			// Create a temporary map with just the Object field
			tempItem := map[string]types.AttributeValue{"Object": objItem}
			var tempRecord struct {
				Object interface{} `dynamodbav:"Object"`
			}
			if err := s.UnmarshalItem(tempItem, &tempRecord); err == nil && tempRecord.Object != nil {
				return tempRecord.Object, nil
			}
		}
		return nil, fmt.Errorf("object data is missing or invalid")
	}

	return record.Object, nil
}

// UpdateObject updates an existing object in DynamoDB
func (s *dynamoDBStorage) UpdateObject(ctx context.Context, object interface{}) error {
	log := common.WithContext(ctx)

	// Convert the object to our internal Object type
	obj, err := convertToObject(object)
	if err != nil {
		return fmt.Errorf("failed to convert object: %w", err)
	}

	if obj.ID == "" {
		return errors.New("object must have an ID")
	}

	// Get the current object to save to history
	currentObj, err := s.GetObject(ctx, obj.ID)
	if err != nil {
		return fmt.Errorf("failed to get current object: %w", err)
	}

	// Check if it's a tombstone
	if _, ok := currentObj.(*storage.Tombstone); ok {
		return fmt.Errorf("cannot update a deleted object")
	}

	// Marshal current state for history
	currentJSON, err := json.Marshal(currentObj)
	if err != nil {
		return fmt.Errorf("failed to marshal current state: %w", err)
	}

	// Get current version
	currentVersion := 1
	histories, err := s.GetUpdateHistory(ctx, obj.ID, 1)
	if err == nil && len(histories) > 0 {
		currentVersion = histories[0].Version + 1
	}

	// Create update history entry
	history := &storage.UpdateHistory{
		ObjectID:      obj.ID,
		Version:       currentVersion - 1, // Previous version
		UpdatedAt:     time.Now(),
		UpdatedBy:     obj.AttributedTo, // Assumes the updater is the attributedTo
		PreviousState: string(currentJSON),
	}

	// Save history
	if err := s.CreateUpdateHistory(ctx, history); err != nil {
		log.Warn("failed to save update history",
			zap.String("object_id", obj.ID),
			zap.Error(err))
		// Continue with update even if history fails
	}

	// Set updated timestamp
	obj.Updated = time.Now()

	// Create the updated record
	record := &ObjectRecord{
		PK:        fmt.Sprintf("OBJECT#%s", obj.ID),
		SK:        "METADATA",
		Object:    obj,
		UpdatedAt: time.Now(),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal object",
			zap.String("object_id", obj.ID),
			zap.Error(err))
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	// Update the item
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                av,
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			log.Warn("object not found",
				zap.String("object_id", obj.ID))
			return fmt.Errorf("object not found: %s", obj.ID)
		}
		log.Error("failed to update object",
			zap.String("object_id", obj.ID),
			zap.Error(err))
		return fmt.Errorf("failed to update object: %w", err)
	}

	log.Info("object updated successfully",
		zap.String("object_id", obj.ID),
		zap.String("type", obj.Type),
		zap.Int("version", currentVersion))

	return nil
}

// DeleteObject soft deletes an object by creating a tombstone
func (s *dynamoDBStorage) DeleteObject(ctx context.Context, id string) error {
	// This is a placeholder - the actual deletion should be done through TombstoneObject
	// which requires the actor who is deleting it
	return fmt.Errorf("use TombstoneObject instead of DeleteObject")
}

// TombstoneObject creates a tombstone for a deleted object
func (s *dynamoDBStorage) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	log := common.WithContext(ctx)

	// First get the original object to get its type
	obj, err := s.GetObject(ctx, objectID)
	if err != nil {
		return fmt.Errorf("failed to get object for tombstoning: %w", err)
	}

	// Determine the former type
	formerType := "Object"
	switch v := obj.(type) {
	case *Object:
		formerType = v.Type
	case map[string]interface{}:
		if t, ok := v["type"].(string); ok {
			formerType = t
		}
	}

	// Create tombstone
	tombstone := &storage.Tombstone{
		ID:         objectID,
		Type:       "Tombstone",
		FormerType: formerType,
		Deleted:    time.Now(),
		DeletedBy:  deletedBy,
	}

	// Create the tombstone record
	tombstoneRecord := map[string]interface{}{
		"PK":        fmt.Sprintf("OBJECT#%s", objectID),
		"SK":        "TOMBSTONE",
		"Tombstone": tombstone,
		"CreatedAt": time.Now(),
	}

	av, err := s.MarshalItem(tombstoneRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal tombstone: %w", err)
	}

	// Create tombstone
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to create tombstone: %w", err)
	}

	// Delete the original object metadata
	_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		// Log but don't fail - tombstone is already created
		log.Warn("failed to delete original object metadata",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Cascade delete likes and announces
	if err := s.CascadeDeleteLikes(ctx, objectID); err != nil {
		log.Warn("failed to cascade delete likes",
			zap.String("object_id", objectID),
			zap.Error(err))
	}
	if err := s.CascadeDeleteAnnounces(ctx, objectID); err != nil {
		log.Warn("failed to cascade delete announces",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	log.Info("object tombstoned successfully",
		zap.String("object_id", objectID),
		zap.String("deleted_by", deletedBy))

	// Update status count if it was a Note or Article
	if formerType == "Note" || formerType == "Article" {
		// Extract username from the original object's AttributedTo
		if originalObj, ok := obj.(*Object); ok && originalObj.AttributedTo != "" {
			username := extractUsernameFromActorID(originalObj.AttributedTo)
			if username != "" {
				if err := s.UpdateStatusCount(ctx, username, -1); err != nil {
					log.Error("failed to update status count",
						zap.String("username", username),
						zap.Error(err))
					// Don't fail the operation, just log the error
				}
			}
		}
	}

	return nil
}

// GetTombstone retrieves a tombstone by object ID
func (s *dynamoDBStorage) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectID)},
			"SK": &types.AttributeValueMemberS{Value: "TOMBSTONE"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get tombstone: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("tombstone not found for object: %s", objectID)
	}

	var record struct {
		Tombstone *storage.Tombstone `dynamodbav:"Tombstone"`
	}
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tombstone: %w", err)
	}

	return record.Tombstone, nil
}

// CascadeDeleteLikes deletes all likes for an object
func (s *dynamoDBStorage) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	log := common.WithContext(ctx)

	// Query all likes for the object
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#LIKES", objectID)},
		},
	}

	// Delete all likes
	for {
		result, err := s.client.Query(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to query likes for deletion: %w", err)
		}

		// Delete each like
		for _, item := range result.Items {
			pk := item["PK"]
			sk := item["SK"]

			_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: s.getTableName(),
				Key: map[string]types.AttributeValue{
					"PK": pk,
					"SK": sk,
				},
			})
			if err != nil {
				// Log but continue
				log.Warn("failed to delete like",
					zap.Error(err))
			}
		}

		// Check if there are more results
		if result.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
	}

	return nil
}

// CascadeDeleteAnnounces deletes all announces for an object
func (s *dynamoDBStorage) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	log := common.WithContext(ctx)

	// Query all announces for the object
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#ANNOUNCES", objectID)},
		},
	}

	// Delete all announces
	for {
		result, err := s.client.Query(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to query announces for deletion: %w", err)
		}

		// Delete each announce
		for _, item := range result.Items {
			pk := item["PK"]
			sk := item["SK"]

			_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: s.getTableName(),
				Key: map[string]types.AttributeValue{
					"PK": pk,
					"SK": sk,
				},
			})
			if err != nil {
				// Log but continue
				log.Warn("failed to delete announce",
					zap.Error(err))
			}
		}

		// Check if there are more results
		if result.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
	}

	return nil
}

// GetObjectsByActor retrieves objects created by a specific actor with pagination
func (s *dynamoDBStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]interface{}, string, error) {
	log := common.WithContext(ctx)

	// Extract username from actor ID
	username := extractUsernameFromActorID(actorID)
	if username == "" {
		return nil, "", common.ValidationError{Field: "actorID", Message: "invalid actor ID format"}
	}

	// Validate limit
	if limit <= 0 || limit > 100 {
		limit = 20 // default
	}

	// Build query input
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#OBJECTS", username)},
		},
		Limit:            aws.Int32(int32(limit + 1)), // Request one extra to determine if there's a next page
		ScanIndexForward: aws.Bool(false),             // Newest first
	}

	// Add cursor if provided
	if cursor != "" {
		lastEvaluatedKey := map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#OBJECTS", username)},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
		queryInput.ExclusiveStartKey = lastEvaluatedKey
	}

	// Execute query
	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		log.Error("failed to query objects by actor",
			zap.String("actor_id", actorID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query objects: %w", err)
	}

	// Unmarshal results
	objects := make([]interface{}, 0, len(result.Items))
	for i, item := range result.Items {
		// Skip the extra item used for pagination
		if i >= limit {
			break
		}

		var record ObjectRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal object record",
				zap.Error(err))
			continue
		}

		objects = append(objects, record.Object)
	}

	// Determine next cursor
	var nextCursor string
	if len(result.Items) > limit {
		// There are more items, use the timestamp of the last item we're returning
		if len(objects) > 0 {
			// Type assert to get the Published field
			if obj, ok := objects[len(objects)-1].(*Object); ok {
				nextCursor = obj.Published.Format(time.RFC3339)
			}
		}
	}

	return objects, nextCursor, nil
}

// CreateUpdateHistory creates a new update history entry
func (s *dynamoDBStorage) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	log := common.WithContext(ctx)

	// Create the DynamoDB record
	record := map[string]interface{}{
		"PK":        fmt.Sprintf("OBJECT#%s#HISTORY", history.ObjectID),
		"SK":        fmt.Sprintf("VERSION#%05d", history.Version), // Pad for proper sorting
		"History":   history,
		"CreatedAt": time.Now(),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal update history: %w", err)
	}

	// Put the item
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to create update history: %w", err)
	}

	log.Info("update history created",
		zap.String("object_id", history.ObjectID),
		zap.Int("version", history.Version))

	return nil
}

// GetUpdateHistory retrieves update history for an object
func (s *dynamoDBStorage) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	log := common.WithContext(ctx)

	// Validate limit
	if limit <= 0 || limit > 100 {
		limit = 10 // default
	}

	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#HISTORY", objectID)},
			":sk": &types.AttributeValueMemberS{Value: "VERSION#"},
		},
		Limit:            aws.Int32(int32(limit)),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("failed to query update history: %w", err)
	}

	histories := make([]*storage.UpdateHistory, 0, len(result.Items))
	for _, item := range result.Items {
		var record struct {
			History *storage.UpdateHistory `dynamodbav:"History"`
		}
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal history record",
				zap.Error(err))
			continue
		}
		histories = append(histories, record.History)
	}

	return histories, nil
}

// convertToObject converts various object representations to our internal Object type
func convertToObject(obj interface{}) (*Object, error) {
	// If it's already an Object, return it
	if o, ok := obj.(*Object); ok {
		return o, nil
	}

	// If it's a map, try to convert it
	if m, ok := obj.(map[string]interface{}); ok {
		jsonBytes, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal map to JSON: %w", err)
		}

		var object Object
		if err := json.Unmarshal(jsonBytes, &object); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON to Object: %w", err)
		}

		return &object, nil
	}

	return nil, fmt.Errorf("unsupported object type: %T", obj)
}
