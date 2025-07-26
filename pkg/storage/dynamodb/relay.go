package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// StoreRelayInfo stores relay information in DynamoDB
func (s *dynamoDBStorage) StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error {
	logger := s.logger().With(zap.String("operation", "StoreRelayInfo"), zap.String("relay_url", relay.URL))

	// Extract domain from URL for indexing
	domain := extractDomainFromURL(relay.URL)
	relay.Domain = domain

	// Set TTL for automatic cleanup (90 days for inactive relays, 365 days for active)
	ttlDays := 90
	if relay.Active {
		ttlDays = 365
	}
	relay.TTL = time.Now().Add(time.Duration(ttlDays) * 24 * time.Hour).Unix()

	// Convert to DynamoDB item
	item, err := attributevalue.MarshalMap(relay)
	if err != nil {
		logger.Error("failed to marshal relay info", zap.Error(err))
		return fmt.Errorf("failed to marshal relay info: %w", err)
	}

	// Set the primary key
	item["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("RELAY#%s", relay.URL)}
	item["SK"] = &types.AttributeValueMemberS{Value: "INFO"}

	// Add GSI entries for queries
	if relay.Active {
		item["GSI1PK"] = &types.AttributeValueMemberS{Value: "ACTIVE_RELAYS"}
		item["GSI1SK"] = &types.AttributeValueMemberS{Value: relay.URL}
	}

	// GSI for querying by domain
	if domain != "" {
		item["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("RELAY_DOMAIN#%s", domain)}
		item["GSI2SK"] = &types.AttributeValueMemberS{Value: relay.URL}
	}

	// Store in DynamoDB
	putInput := &dynamodb.PutItemInput{
		TableName: &s.tableName,
		Item:      item,
		// Use condition to prevent overwriting newer data
		ConditionExpression: aws.String("attribute_not_exists(PK) OR last_seen_at <= :new_last_seen"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":new_last_seen": &types.AttributeValueMemberS{Value: relay.LastSeenAt.Format(time.RFC3339)},
		},
	}

	_, err = s.client.PutItem(ctx, putInput)
	if err != nil {
		// Don't treat condition check failures as errors for concurrent updates
		var conditionalCheckFailed *types.ConditionalCheckFailedException
		if errors.As(err, &conditionalCheckFailed) {
			logger.Debug("relay info not updated - newer version exists")
			return nil
		}

		logger.Error("failed to store relay info", zap.Error(err))
		return fmt.Errorf("failed to store relay info: %w", err)
	}

	// Track cost
	cost.TrackWrite(ctx, s.costTracker, "StoreRelayInfo", 1)

	logger.Info("stored relay info successfully")
	return nil
}

// GetRelayInfo retrieves relay information from DynamoDB
func (s *dynamoDBStorage) GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	logger := s.logger().With(zap.String("operation", "GetRelayInfo"), zap.String("relay_url", relayURL))

	getInput := &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("RELAY#%s", relayURL)},
			"SK": &types.AttributeValueMemberS{Value: "INFO"},
		},
	}

	result, err := s.client.GetItem(ctx, getInput)
	if err != nil {
		logger.Error("failed to get relay info", zap.Error(err))
		return nil, fmt.Errorf("failed to get relay info: %w", err)
	}

	// Track cost
	cost.TrackRead(ctx, s.costTracker, "GetRelayInfo", 1)

	if len(result.Item) == 0 {
		return nil, fmt.Errorf("relay not found: %s", relayURL)
	}

	// Unmarshal the result
	var relay storage.RelayInfo
	if err := attributevalue.UnmarshalMap(result.Item, &relay); err != nil {
		logger.Error("failed to unmarshal relay info", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal relay info: %w", err)
	}

	logger.Debug("retrieved relay info successfully")
	return &relay, nil
}

// RemoveRelayInfo removes relay information from DynamoDB
func (s *dynamoDBStorage) RemoveRelayInfo(ctx context.Context, relayURL string) error {
	logger := s.logger().With(zap.String("operation", "RemoveRelayInfo"), zap.String("relay_url", relayURL))

	deleteInput := &dynamodb.DeleteItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("RELAY#%s", relayURL)},
			"SK": &types.AttributeValueMemberS{Value: "INFO"},
		},
	}

	_, err := s.client.DeleteItem(ctx, deleteInput)
	if err != nil {
		logger.Error("failed to remove relay info", zap.Error(err))
		return fmt.Errorf("failed to remove relay info: %w", err)
	}

	// Track cost
	cost.TrackWrite(ctx, s.costTracker, "RemoveRelayInfo", 1)

	logger.Info("removed relay info successfully")
	return nil
}

// GetActiveRelays retrieves all active relays
func (s *dynamoDBStorage) GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	logger := s.logger().With(zap.String("operation", "GetActiveRelays"))

	queryInput := &dynamodb.QueryInput{
		TableName:              &s.tableName,
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "ACTIVE_RELAYS"},
		},
	}

	var relays []*storage.RelayInfo

	paginator := dynamodb.NewQueryPaginator(s.client, queryInput)
	for paginator.HasMorePages() {
		result, err := paginator.NextPage(ctx)
		if err != nil {
			logger.Error("failed to query active relays", zap.Error(err))
			return nil, fmt.Errorf("failed to query active relays: %w", err)
		}

		// Track cost
		cost.TrackRead(ctx, s.costTracker, "GetActiveRelays", int64(len(result.Items)))

		for _, item := range result.Items {
			var relay storage.RelayInfo
			if err := attributevalue.UnmarshalMap(item, &relay); err != nil {
				logger.Warn("failed to unmarshal relay item", zap.Error(err))
				continue
			}
			relays = append(relays, &relay)
		}
	}

	logger.Info("retrieved active relays", zap.Int("count", len(relays)))
	return relays, nil
}

// GetAllRelays retrieves all relays with pagination
func (s *dynamoDBStorage) GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	logger := s.logger().With(zap.String("operation", "GetAllRelays"))

	queryInput := &dynamodb.QueryInput{
		TableName:              &s.tableName,
		KeyConditionExpression: aws.String("begins_with(PK, :pk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk_prefix": &types.AttributeValueMemberS{Value: "RELAY#"},
		},
		Limit: aws.Int32(int32(limit)),
	}

	// Handle pagination cursor
	if cursor != "" {
		exclusiveStartKey, err := decodeCursor(cursor)
		if err != nil {
			logger.Warn("invalid cursor", zap.String("cursor", cursor), zap.Error(err))
		} else {
			queryInput.ExclusiveStartKey = exclusiveStartKey
		}
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		logger.Error("failed to query all relays", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query all relays: %w", err)
	}

	// Track cost
	cost.TrackRead(ctx, s.costTracker, "GetAllRelays", int64(len(result.Items)))

	var relays []*storage.RelayInfo
	for _, item := range result.Items {
		var relay storage.RelayInfo
		if err := attributevalue.UnmarshalMap(item, &relay); err != nil {
			logger.Warn("failed to unmarshal relay item", zap.Error(err))
			continue
		}
		relays = append(relays, &relay)
	}

	// Generate next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(result.LastEvaluatedKey)
	}

	logger.Info("retrieved all relays", zap.Int("count", len(relays)))
	return relays, nextCursor, nil
}

// UpdateRelayStatus updates the active status of a relay
func (s *dynamoDBStorage) UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error {
	logger := s.logger().With(zap.String("operation", "UpdateRelayStatus"),
		zap.String("relay_url", relayURL), zap.Bool("active", active))

	updateExpr := "SET active = :active, last_seen_at = :now"
	exprValues := map[string]types.AttributeValue{
		":active": &types.AttributeValueMemberBOOL{Value: active},
		":now":    &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	// Update GSI1 entries based on status
	if active {
		updateExpr += ", GSI1PK = :gsi1pk, GSI1SK = :gsi1sk"
		exprValues[":gsi1pk"] = &types.AttributeValueMemberS{Value: "ACTIVE_RELAYS"}
		exprValues[":gsi1sk"] = &types.AttributeValueMemberS{Value: relayURL}
	} else {
		updateExpr += " REMOVE GSI1PK, GSI1SK"
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("RELAY#%s", relayURL)},
			"SK": &types.AttributeValueMemberS{Value: "INFO"},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprValues,
		ConditionExpression:       aws.String("attribute_exists(PK)"), // Only update if relay exists
	}

	_, err := s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		var conditionalCheckFailed *types.ConditionalCheckFailedException
		if errors.As(err, &conditionalCheckFailed) {
			return fmt.Errorf("relay not found: %s", relayURL)
		}

		logger.Error("failed to update relay status", zap.Error(err))
		return fmt.Errorf("failed to update relay status: %w", err)
	}

	// Track cost
	cost.TrackWrite(ctx, s.costTracker, "UpdateRelayStatus", 1)

	logger.Info("updated relay status successfully")
	return nil
}
