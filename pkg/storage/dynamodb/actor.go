package dynamodb

import (
	"context"
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
	"go.uber.org/zap"
)

// CreateActor creates a new actor in DynamoDB with encrypted private key
func (s *dynamoDBStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	log := common.WithContext(ctx)

	// Extract username from PreferredUsername
	username := actor.PreferredUsername
	if username == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	// Build the actor record
	now := time.Now()
	record := storage.ActorRecord{
		PK:         storage.ActorPKPrefix + username,
		SK:         storage.ActorSK,
		Actor:      actor,
		PrivateKey: privateKey, // TODO: Encrypt this with AWS KMS before storing
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
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		log.Error("failed to unmarshal actor",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal actor: %w", err)
	}

	return record.Actor, nil
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

	// TODO: Decrypt the private key using AWS KMS

	return privateKeyStr.Value, nil
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

// DeleteActor deletes an actor from DynamoDB
func (s *dynamoDBStorage) DeleteActor(ctx context.Context, username string) error {
	log := common.WithContext(ctx)

	// Build the key
	pk := storage.ActorPKPrefix + username
	sk := storage.ActorSK

	// Delete the item
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
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

	log.Info("actor deleted successfully",
		zap.String("username", username))

	// TODO: Also delete related data (activities, follows, etc.) or use DynamoDB transactions

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
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
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
