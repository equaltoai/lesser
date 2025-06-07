package dynamodb

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	cfg "github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"go.uber.org/zap"
)

// encryptPrivateKey encrypts a private key using AWS KMS
func (s *dynamoDBStorage) encryptPrivateKey(ctx context.Context, privateKey string) (string, error) {
	log := common.WithContext(ctx)

	// Get KMS key ID from config, fallback to AWS managed key
	keyID := cfg.Get().KMSKeyID
	if keyID == "" {
		keyID = "alias/aws/dynamodb"
		log.Debug("using default AWS managed key for encryption")
	}

	// Create KMS client if not already created
	if s.kmsClient == nil {
		log.Warn("KMS client not configured, storing private key in plaintext")
		return privateKey, nil
	}

	// Encrypt the private key
	result, err := s.kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(keyID),
		Plaintext: []byte(privateKey),
	})
	if err != nil {
		log.Error("failed to encrypt private key with KMS",
			zap.String("keyID", keyID),
			zap.Error(err))
		return "", fmt.Errorf("failed to encrypt private key: %w", err)
	}

	// Base64 encode the ciphertext for storage
	encrypted := base64.StdEncoding.EncodeToString(result.CiphertextBlob)
	log.Debug("private key encrypted successfully")

	return encrypted, nil
}

// decryptPrivateKey decrypts a private key using AWS KMS
func (s *dynamoDBStorage) decryptPrivateKey(ctx context.Context, encryptedKey string) (string, error) {
	log := common.WithContext(ctx)

	// Check if KMS client is available
	if s.kmsClient == nil {
		log.Debug("KMS client not configured, assuming plaintext key")
		return encryptedKey, nil
	}

	// Decode the base64 ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedKey)
	if err != nil {
		// If it fails to decode, assume it's a plaintext key (for backwards compatibility)
		log.Debug("failed to decode as base64, assuming plaintext key")
		return encryptedKey, nil
	}

	// Decrypt the private key
	result, err := s.kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: ciphertext,
	})
	if err != nil {
		log.Error("failed to decrypt private key with KMS", zap.Error(err))
		return "", fmt.Errorf("failed to decrypt private key: %w", err)
	}

	return string(result.Plaintext), nil
}

// CreateActor creates a new actor in DynamoDB with encrypted private key
func (s *dynamoDBStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	log := common.WithContext(ctx)

	// Extract username from PreferredUsername
	username := actor.PreferredUsername
	if username == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	// Encrypt the private key before storing
	encryptedKey, err := s.encryptPrivateKey(ctx, privateKey)
	if err != nil {
		log.Error("failed to encrypt private key",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to encrypt private key: %w", err)
	}

	// Build the actor record
	now := time.Now()
	record := storage.ActorRecord{
		PK:         storage.ActorPKPrefix + username,
		SK:         storage.ActorSK,
		Actor:      actor,
		PrivateKey: encryptedKey, // Now encrypted
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Marshal the record to DynamoDB attributes
	item, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal actor record",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to marshal actor record: %w", err)
	}

	// Add GSI attributes for optimized search
	usernameLower := strings.ToLower(username)

	// GSI1 - Username Search (with 2-char prefix partitioning)
	if len(usernameLower) >= 2 {
		item["GSI1PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("USERNAME_SEARCH#%s", usernameLower[:2])}
		item["GSI1SK"] = &types.AttributeValueMemberS{Value: usernameLower}
	}

	// GSI2 - Display Name Search (if display name exists)
	if actor.Name != "" {
		displayNameLower := strings.ToLower(actor.Name)
		if len(displayNameLower) >= 2 {
			item["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("NAME_SEARCH#%s", displayNameLower[:2])}
			item["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", displayNameLower, username)}
		}
	}

	// GSI3 - Domain Search (for federation)
	domain := cfg.Get().Domain
	if domain != "" {
		item["GSI3PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN#%s", domain)}
		item["GSI3SK"] = &types.AttributeValueMemberS{Value: username}
	}

	// GSI4 - Popularity Ranking (initially 0 followers)
	initialFollowerCount := 0
	bucket := GetFollowerCountBucket(initialFollowerCount)
	item["GSI4PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR_RANK#%s", bucket)}
	item["GSI4SK"] = &types.AttributeValueMemberS{Value: FormatFollowerCountForGSI(initialFollowerCount, username)}

	// Store counts as separate attributes for easy access
	item["FollowerCount"] = &types.AttributeValueMemberN{Value: "0"}
	item["FollowingCount"] = &types.AttributeValueMemberN{Value: "0"}
	item["StatusCount"] = &types.AttributeValueMemberN{Value: "0"}

	// GSI5 - Recent Activity
	item["GSI5PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTIVE#%s", now.Format("2006-01-02"))}
	item["GSI5SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%d#%s", now.Unix(), username)}

	// Put the item with condition that it doesn't already exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})

	if err != nil {
		// Check if it's a conditional check failure (actor already exists)
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			log.Warn("actor already exists",
				zap.String("username", username))
			return common.ConflictError{
				Resource: "actor",
				Message:  fmt.Sprintf("actor %s already exists", username),
			}
		}

		log.Error("failed to create actor",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to create actor: %w", err)
	}

	log.Info("actor created successfully",
		zap.String("username", username),
		zap.String("actor_id", actor.ID))

	return nil
}

// GetActor retrieves an actor by username from DynamoDB
func (s *dynamoDBStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Get the item
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})

	if err != nil {
		log.Error("failed to get actor",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	if result.Item == nil {
		return nil, common.ActorNotFoundError{Username: username}
	}

	// Unmarshal the actor record
	var record storage.ActorRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		log.Error("failed to unmarshal actor",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal actor: %w", err)
	}

	return record.Actor, nil
}

// GetActorWithMetadata retrieves an actor by username from DynamoDB along with metadata
func (s *dynamoDBStorage) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Get the item
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})

	if err != nil {
		log.Error("failed to get actor with metadata",
			zap.String("username", username),
			zap.Error(err))
		return nil, nil, fmt.Errorf("failed to get actor with metadata: %w", err)
	}

	if result.Item == nil {
		return nil, nil, common.ActorNotFoundError{Username: username}
	}

	// Unmarshal the actor record
	var record storage.ActorRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		log.Error("failed to unmarshal actor",
			zap.String("username", username),
			zap.Error(err))
		return nil, nil, fmt.Errorf("failed to unmarshal actor: %w", err)
	}

	// Create metadata struct
	metadata := &storage.ActorMetadata{
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
		LastStatusAt: record.LastStatusAt,
		Fields:       record.Fields,
	}

	return record.Actor, metadata, nil
}

// GetActorPrivateKey retrieves an actor's private key from DynamoDB
func (s *dynamoDBStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Get only the private key attribute
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ProjectionExpression: aws.String("PrivateKey"),
	})

	if err != nil {
		log.Error("failed to get actor private key",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("failed to get actor private key: %w", err)
	}

	if result.Item == nil {
		return "", common.ActorNotFoundError{Username: username}
	}

	// Extract the private key
	privateKeyAttr, ok := result.Item["PrivateKey"]
	if !ok {
		return "", fmt.Errorf("private key not found for actor %s", username)
	}

	privateKeyStr, ok := privateKeyAttr.(*types.AttributeValueMemberS)
	if !ok || privateKeyStr.Value == "" {
		return "", fmt.Errorf("invalid private key format for actor %s", username)
	}

	// Decrypt the private key using AWS KMS
	decryptedKey, err := s.decryptPrivateKey(ctx, privateKeyStr.Value)
	if err != nil {
		log.Error("failed to decrypt private key",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("failed to decrypt private key: %w", err)
	}

	return decryptedKey, nil
}

// UpdateActor updates an existing actor in DynamoDB
func (s *dynamoDBStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	username := actor.PreferredUsername
	if username == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Marshal the actor to attribute value
	actorAttr, err := attributevalue.Marshal(actor)
	if err != nil {
		log.Error("failed to marshal actor",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to marshal actor: %w", err)
	}

	// Build update expression with GSI attributes
	updateExpr := "SET Actor = :actor, UpdatedAt = :updated"
	exprValues := map[string]types.AttributeValue{
		":actor":   actorAttr,
		":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	// Update GSI2 if display name changed
	if actor.Name != "" {
		displayNameLower := strings.ToLower(actor.Name)
		if len(displayNameLower) >= 2 {
			updateExpr += ", GSI2PK = :gsi2pk, GSI2SK = :gsi2sk"
			exprValues[":gsi2pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("NAME_SEARCH#%s", displayNameLower[:2])}
			exprValues[":gsi2sk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", displayNameLower, username)}
		}
	}

	// Update GSI5 for recent activity
	now := time.Now()
	updateExpr += ", GSI5PK = :gsi5pk, GSI5SK = :gsi5sk"
	exprValues[":gsi5pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTIVE#%s", now.Format("2006-01-02"))}
	exprValues[":gsi5sk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%d#%s", now.Unix(), username)}

	// Update the item
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprValues,
		ConditionExpression:       aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		// Check if actor doesn't exist
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return common.ActorNotFoundError{Username: username}
		}

		log.Error("failed to update actor",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to update actor: %w", err)
	}

	log.Info("actor updated successfully",
		zap.String("username", username),
		zap.String("actor_id", actor.ID))

	return nil
}

// UpdateActorLastStatusTime updates the last status timestamp for an actor
func (s *dynamoDBStorage) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	now := time.Now()

	// Update the last status time
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("SET LastStatusAt = :lastStatus, UpdatedAt = :updated"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":lastStatus": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":updated":    &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		// Check if actor doesn't exist
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return common.ActorNotFoundError{Username: username}
		}

		log.Error("failed to update actor last status time",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to update actor last status time: %w", err)
	}

	log.Debug("actor last status time updated",
		zap.String("username", username),
		zap.Time("lastStatusAt", now))

	return nil
}

// SetActorFields updates the profile fields for an actor
func (s *dynamoDBStorage) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Marshal fields to DynamoDB attribute
	fieldsAttr, err := attributevalue.Marshal(fields)
	if err != nil {
		log.Error("failed to marshal actor fields",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to marshal actor fields: %w", err)
	}

	// Update the fields
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("SET #fields = :fields, UpdatedAt = :updated"),
		ExpressionAttributeNames: map[string]string{
			"#fields": "Fields", // Use alias since "Fields" might be reserved
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":fields":  fieldsAttr,
			":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		// Check if actor doesn't exist
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return common.ActorNotFoundError{Username: username}
		}

		log.Error("failed to update actor fields",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to update actor fields: %w", err)
	}

	log.Info("actor fields updated",
		zap.String("username", username),
		zap.Int("fieldCount", len(fields)))

	return nil
}

// DeleteActor deletes an actor from DynamoDB
func (s *dynamoDBStorage) DeleteActor(ctx context.Context, username string) error {
	log := common.WithContext(ctx)

	// Get actor ID first for cleanup operations
	actor, err := s.GetActor(ctx, username)
	if err != nil {
		return err
	}

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Start with deleting related data
	log.Info("starting cascading delete for actor",
		zap.String("username", username),
		zap.String("actorID", actor.ID))

	// 1. Delete all activities by this actor
	if err := s.deleteActorActivities(ctx, actor.ID); err != nil {
		log.Error("failed to delete actor activities",
			zap.String("username", username),
			zap.Error(err))
		// Continue with deletion even if this fails
	}

	// 2. Delete all objects created by this actor
	if err := s.deleteActorObjects(ctx, actor.ID); err != nil {
		log.Error("failed to delete actor objects",
			zap.String("username", username),
			zap.Error(err))
	}

	// 3. Delete all follow relationships (both following and followers)
	if err := s.deleteActorFollowRelationships(ctx, username); err != nil {
		log.Error("failed to delete follow relationships",
			zap.String("username", username),
			zap.Error(err))
	}

	// 4. Delete likes
	if err := s.deleteActorLikes(ctx, actor.ID); err != nil {
		log.Error("failed to delete actor likes",
			zap.String("username", username),
			zap.Error(err))
	}

	// 5. Delete announces/boosts
	if err := s.deleteActorAnnounces(ctx, actor.ID); err != nil {
		log.Error("failed to delete actor announces",
			zap.String("username", username),
			zap.Error(err))
	}

	// 6. Delete blocks
	if err := s.deleteActorBlocks(ctx, actor.ID); err != nil {
		log.Error("failed to delete actor blocks",
			zap.String("username", username),
			zap.Error(err))
	}

	// 7. Delete mutes
	if err := s.deleteActorMutes(ctx, actor.ID); err != nil {
		log.Error("failed to delete actor mutes",
			zap.String("username", username),
			zap.Error(err))
	}

	// 8. Delete bookmarks
	if err := s.deleteActorBookmarks(ctx, username); err != nil {
		log.Error("failed to delete actor bookmarks",
			zap.String("username", username),
			zap.Error(err))
	}

	// 9. Delete lists
	if err := s.deleteActorLists(ctx, username); err != nil {
		log.Error("failed to delete actor lists",
			zap.String("username", username),
			zap.Error(err))
	}

	// 10. Delete notifications
	if err := s.ClearNotifications(ctx, username); err != nil {
		log.Error("failed to delete actor notifications",
			zap.String("username", username),
			zap.Error(err))
	}

	// 11. Delete filters
	if err := s.deleteActorFilters(ctx, username); err != nil {
		log.Error("failed to delete actor filters",
			zap.String("username", username),
			zap.Error(err))
	}

	// 12. Delete conversations
	if err := s.deleteActorConversations(ctx, username); err != nil {
		log.Error("failed to delete actor conversations",
			zap.String("username", username),
			zap.Error(err))
	}

	// 13. Delete scheduled statuses
	if err := s.deleteActorScheduledStatuses(ctx, username); err != nil {
		log.Error("failed to delete actor scheduled statuses",
			zap.String("username", username),
			zap.Error(err))
	}

	// 14. Delete account pins/notes
	if err := s.deleteActorAccountPins(ctx, username); err != nil {
		log.Error("failed to delete actor account pins",
			zap.String("username", username),
			zap.Error(err))
	}

	// 15. Delete status pins
	if err := s.deleteActorStatusPins(ctx, username); err != nil {
		log.Error("failed to delete actor status pins",
			zap.String("username", username),
			zap.Error(err))
	}

	// 16. Delete push subscriptions
	if err := s.DeleteAllPushSubscriptions(ctx, username); err != nil {
		log.Error("failed to delete push subscriptions",
			zap.String("username", username),
			zap.Error(err))
	}

	// 17. Delete timeline entries
	if err := s.deleteActorTimelineEntries(ctx, username); err != nil {
		log.Error("failed to delete timeline entries",
			zap.String("username", username),
			zap.Error(err))
	}

	// Finally, delete the actor itself
	_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})

	if err != nil {
		// Check if actor doesn't exist
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return common.ActorNotFoundError{Username: username}
		}

		log.Error("failed to delete actor",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to delete actor: %w", err)
	}

	log.Info("actor and all related data deleted successfully",
		zap.String("username", username))

	return nil
}

// SearchAccounts searches for actors by username or display name
// Uses the advanced SearchService if available, otherwise falls back to basic scan
func (s *dynamoDBStorage) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	log := common.WithContext(ctx)

	if query == "" {
		return []*activitypub.Actor{}, nil
	}

	// Use advanced search service if available
	if s.searchService != nil {
		options := SearchOptions{
			Limit:         limit,
			Offset:        offset,
			FollowingOnly: followingOnly,
			Fuzzy:         true, // Enable fuzzy matching
		}

		results, err := s.searchService.Search(ctx, query, options)
		if err != nil {
			log.Error("search service failed, falling back to basic search",
				zap.String("query", query),
				zap.Error(err))
			// Continue with basic search below
		} else {
			// Extract actors from search results
			actors := make([]*activitypub.Actor, 0, len(results))
			for _, result := range results {
				if result.Actor != nil {
					actors = append(actors, result.Actor)
				}
			}
			return actors, nil
		}
	}

	// Normalize query for case-insensitive search
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))

	// For now, use a scan with filter - this will be replaced with GSI queries
	var filterExpr expression.ConditionBuilder

	// Search in both username and display name
	filterExpr = expression.Or(
		expression.Contains(expression.Name("Actor.preferredUsername"), normalizedQuery),
		expression.Contains(expression.Name("Actor.name"), normalizedQuery),
	)

	// Add actor type filter to only get actors
	filterExpr = expression.And(
		filterExpr,
		expression.BeginsWith(expression.Name("PK"), storage.ActorPKPrefix),
	)

	expr, err := expression.NewBuilder().
		WithFilter(filterExpr).
		Build()

	if err != nil {
		log.Error("failed to build filter expression", zap.Error(err))
		return nil, fmt.Errorf("failed to build filter expression: %w", err)
	}

	scanInput := &dynamodb.ScanInput{
		TableName:                 s.getTableName(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(int32(limit + offset)), // Get extra for offset
	}

	result, err := s.client.Scan(ctx, scanInput)
	if err != nil {
		log.Error("failed to scan for actors",
			zap.String("query", query),
			zap.Error(err))
		return nil, fmt.Errorf("failed to search accounts: %w", err)
	}

	// Convert results to actors
	actors := make([]*activitypub.Actor, 0)
	for i, item := range result.Items {
		// Skip items before offset
		if i < offset {
			continue
		}
		if len(actors) >= limit {
			break
		}

		var record storage.ActorRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Warn("failed to unmarshal actor record", zap.Error(err))
			continue
		}

		if record.Actor != nil {
			actors = append(actors, record.Actor)
		}
	}

	log.Info("search completed",
		zap.String("query", query),
		zap.Int("results", len(actors)),
		zap.Int("limit", limit),
		zap.Int("offset", offset))

	return actors, nil
}

// GetSearchSuggestions returns search suggestions for autocomplete
func (s *dynamoDBStorage) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	log := common.WithContext(ctx)

	// Use the search service if available
	if s.searchService == nil {
		log.Warn("search service not available for suggestions")
		return []storage.SearchSuggestion{}, nil
	}

	// Get suggestions from search service
	suggestions, err := s.searchService.GetSuggestions(ctx, prefix)
	if err != nil {
		log.Error("failed to get search suggestions",
			zap.String("prefix", prefix),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get search suggestions: %w", err)
	}

	// Convert to storage layer type
	result := make([]storage.SearchSuggestion, 0, len(suggestions))
	for _, sugg := range suggestions {
		result = append(result, storage.SearchSuggestion{
			Type:  sugg.Type,
			Value: sugg.Value,
			Score: int(sugg.Score),
		})
	}

	log.Debug("search suggestions retrieved",
		zap.String("prefix", prefix),
		zap.Int("count", len(result)))

	return result, nil
}

// Helper methods for cascading deletes

func (s *dynamoDBStorage) deleteActorActivities(ctx context.Context, actorID string) error {
	// Query GSI1 for activities by actor
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR_ACTIVITY#%s", extractUsernameFromActorID(actorID))},
		},
	}

	result, err := s.client.Query(ctx, query)
	if err != nil {
		return err
	}

	// Delete each activity
	for _, item := range result.Items {
		if pkAttr, ok := item["PK"]; ok {
			if skAttr, ok := item["SK"]; ok {
				s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
					TableName: s.getTableName(),
					Key: map[string]types.AttributeValue{
						"PK": pkAttr,
						"SK": skAttr,
					},
				})
			}
		}
	}

	return nil
}

func (s *dynamoDBStorage) deleteActorObjects(ctx context.Context, actorID string) error {
	// Query for objects created by actor
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", extractUsernameFromActorID(actorID))},
			":sk": &types.AttributeValueMemberS{Value: "OBJECT#"},
		},
	}

	result, err := s.client.Query(ctx, query)
	if err != nil {
		return err
	}

	// Delete each object
	for _, item := range result.Items {
		s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: s.getTableName(),
			Key: map[string]types.AttributeValue{
				"PK": item["PK"],
				"SK": item["SK"],
			},
		})
	}

	return nil
}

func (s *dynamoDBStorage) deleteActorFollowRelationships(ctx context.Context, username string) error {
	// Delete where actor is following others
	followingQuery := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "FOLLOWING#"},
		},
	}

	if result, err := s.client.Query(ctx, followingQuery); err == nil {
		for _, item := range result.Items {
			s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: s.getTableName(),
				Key: map[string]types.AttributeValue{
					"PK": item["PK"],
					"SK": item["SK"],
				},
			})
		}
	}

	// Delete where others are following actor (use GSI1)
	followersQuery := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND begins_with(GSI1SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "FOLLOWER#"},
		},
	}

	if result, err := s.client.Query(ctx, followersQuery); err == nil {
		for _, item := range result.Items {
			if pkAttr, ok := item["PK"]; ok {
				if skAttr, ok := item["SK"]; ok {
					s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
						TableName: s.getTableName(),
						Key: map[string]types.AttributeValue{
							"PK": pkAttr,
							"SK": skAttr,
						},
					})
				}
			}
		}
	}

	return nil
}

func (s *dynamoDBStorage) deleteActorLikes(ctx context.Context, actorID string) error {
	// Query for likes by actor
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIKE#%s", actorID)},
			":sk": &types.AttributeValueMemberS{Value: "OBJECT#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorAnnounces(ctx context.Context, actorID string) error {
	// Query for announces by actor
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANNOUNCE#%s", actorID)},
			":sk": &types.AttributeValueMemberS{Value: "OBJECT#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorBlocks(ctx context.Context, actorID string) error {
	// Query for blocks by actor
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("BLOCK#%s", actorID)},
			":sk": &types.AttributeValueMemberS{Value: "ACTOR#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorMutes(ctx context.Context, actorID string) error {
	// Query for mutes by actor
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("MUTE#%s", actorID)},
			":sk": &types.AttributeValueMemberS{Value: "ACTOR#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorBookmarks(ctx context.Context, username string) error {
	// Query for bookmarks by user
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "BOOKMARK#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorLists(ctx context.Context, username string) error {
	// Query for lists by user
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "LIST#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorFilters(ctx context.Context, username string) error {
	// Query for filters by user
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "FILTER#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorConversations(_ context.Context, _ string) error {
	// This is more complex as conversations may have multiple participants
	// For now, we'll remove the user from conversations but not delete them entirely
	return nil
}

func (s *dynamoDBStorage) deleteActorScheduledStatuses(ctx context.Context, username string) error {
	// Query for scheduled statuses by user
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "SCHEDULED#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorAccountPins(ctx context.Context, username string) error {
	// Query for account pins by user
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "ACCOUNT_PIN#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorStatusPins(ctx context.Context, username string) error {
	// Query for status pins by user
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "STATUS_PIN#"},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

func (s *dynamoDBStorage) deleteActorTimelineEntries(ctx context.Context, username string) error {
	// Query for timeline entries
	query := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TIMELINE#HOME#%s", username)},
		},
	}

	return s.deleteItemsFromQuery(ctx, query)
}

// Helper function to delete all items from a query result
func (s *dynamoDBStorage) deleteItemsFromQuery(ctx context.Context, query *dynamodb.QueryInput) error {
	result, err := s.client.Query(ctx, query)
	if err != nil {
		return err
	}

	for _, item := range result.Items {
		if pkAttr, ok := item["PK"]; ok {
			if skAttr, ok := item["SK"]; ok {
				s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
					TableName: s.getTableName(),
					Key: map[string]types.AttributeValue{
						"PK": pkAttr,
						"SK": skAttr,
					},
				})
			}
		}
	}

	// Handle pagination if needed
	if result.LastEvaluatedKey != nil {
		query.ExclusiveStartKey = result.LastEvaluatedKey
		return s.deleteItemsFromQuery(ctx, query)
	}

	return nil
}
