package dynamodb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// FlagRecord represents a flag stored in DynamoDB
type FlagRecord struct {
	PK        string        `dynamodbav:"PK"`     // FLAG#{id}
	SK        string        `dynamodbav:"SK"`     // FLAG
	GSI1PK    string        `dynamodbav:"GSI1PK"` // FLAG#STATUS#{status}
	GSI1SK    string        `dynamodbav:"GSI1SK"` // CREATED#timestamp
	GSI2PK    string        `dynamodbav:"GSI2PK"` // FLAG#OBJECT#object_id
	GSI2SK    string        `dynamodbav:"GSI2SK"` // CREATED#timestamp
	Flag      *storage.Flag `dynamodbav:"Flag"`
	CreatedAt time.Time     `dynamodbav:"CreatedAt"`
	UpdatedAt time.Time     `dynamodbav:"UpdatedAt"`
	TTL       *int64        `dynamodbav:"TTL,omitempty"` // Optional TTL for auto-cleanup
}

// CreateFlag creates a new flag in the database
func (s *dynamoDBStorage) CreateFlag(ctx context.Context, flag *storage.Flag) error {
	log := common.WithContext(ctx).With(
		zap.String("flag_id", flag.ID),
		zap.String("actor", flag.Actor),
		zap.Strings("objects", flag.Object),
		zap.String("status", string(flag.Status)),
	)

	flag.CreatedAt = time.Now()

	// Create the flag record
	record := &FlagRecord{
		PK:        fmt.Sprintf("FLAG#%s", flag.ID),
		SK:        "FLAG",
		GSI1PK:    fmt.Sprintf("FLAG#STATUS#%s", flag.Status),
		GSI1SK:    fmt.Sprintf("CREATED#%s", flag.CreatedAt.Format(time.RFC3339Nano)),
		Flag:      flag,
		CreatedAt: flag.CreatedAt,
	}

	// If there are objects, we'll also create a GSI2 entry for the first object
	if len(flag.Object) > 0 {
		record.GSI2PK = fmt.Sprintf("FLAG#OBJECT#%s", flag.Object[0])
		record.GSI2SK = fmt.Sprintf("CREATED#%s", flag.CreatedAt.Format(time.RFC3339Nano))
	}

	item, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal flag record", zap.Error(err))
		return fmt.Errorf("failed to marshal flag record: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	})
	if err != nil {
		log.Error("failed to create flag", zap.Error(err))
		return fmt.Errorf("failed to create flag: %w", err)
	}

	log.Info("flag created successfully")
	return nil
}

// GetFlag retrieves a flag by ID
func (s *dynamoDBStorage) GetFlag(ctx context.Context, id string) (*storage.Flag, error) {
	log := common.WithContext(ctx).With(zap.String("flag_id", id))

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FLAG#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "FLAG"},
		},
	})
	if err != nil {
		log.Error("failed to get flag", zap.Error(err))
		return nil, fmt.Errorf("failed to get flag: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("flag not found: %s", id)
	}

	var record FlagRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		log.Error("failed to unmarshal flag record", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal flag record: %w", err)
	}

	return record.Flag, nil
}

// GetFlagsByObject retrieves flags for a specific object with pagination
func (s *dynamoDBStorage) GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	log := common.WithContext(ctx).With(
		zap.String("object_id", objectID),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Query using GSI2 for flags on this object
	keyCondition := expression.Key("GSI2PK").Equal(expression.Value(fmt.Sprintf("FLAG#OBJECT#%s", objectID)))

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		log.Error("failed to build expression", zap.Error(err))
		return nil, "", fmt.Errorf("failed to build expression: %w", err)
	}

	input := &dynamodb.QueryInput{
		TableName:                 s.getTableName(),
		IndexName:                 aws.String("GSI2"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     safeInt32(limit),
		ScanIndexForward:          aws.Bool(false), // Most recent first
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI2PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FLAG#OBJECT#%s", objectID)},
			"GSI2SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query flags by object", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query flags by object: %w", err)
	}

	// Convert results to flags
	flags := make([]*storage.Flag, 0, len(result.Items))
	for _, item := range result.Items {
		var record FlagRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal flag record", zap.Error(err))
			continue
		}
		// Only include flags that actually reference this object
		for _, obj := range record.Flag.Object {
			if obj == objectID {
				flags = append(flags, record.Flag)
				break
			}
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI2SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	log.Info("retrieved flags by object",
		zap.Int("count", len(flags)),
		zap.Bool("has_more", nextCursor != ""),
	)

	return flags, nextCursor, nil
}

// GetFlagsByActor retrieves flags created by a specific actor with pagination
func (s *dynamoDBStorage) GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	log := common.WithContext(ctx).With(
		zap.String("actor_id", actorID),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// We need to scan and filter by actor since we don't have a GSI for actor
	filterExpr := expression.Name("Flag.Actor").Equal(expression.Value(actorID))

	expr, err := expression.NewBuilder().
		WithFilter(filterExpr).
		Build()
	if err != nil {
		log.Error("failed to build expression", zap.Error(err))
		return nil, "", fmt.Errorf("failed to build expression: %w", err)
	}

	input := &dynamodb.ScanInput{
		TableName:                 s.getTableName(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     safeInt32(limit),
	}

	if cursor != "" {
		// Decode cursor for scan
		decodedCursor, err := base64.StdEncoding.DecodeString(cursor)
		if err == nil {
			var lastKey map[string]types.AttributeValue
			if err := json.Unmarshal(decodedCursor, &lastKey); err == nil {
				input.ExclusiveStartKey = lastKey
			}
		}
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		log.Error("failed to scan flags by actor", zap.Error(err))
		return nil, "", fmt.Errorf("failed to scan flags by actor: %w", err)
	}

	// Convert results to flags
	flags := make([]*storage.Flag, 0, len(result.Items))
	for _, item := range result.Items {
		// Only process FLAG records
		if pk, ok := item["PK"]; ok {
			if pkStr, ok := pk.(*types.AttributeValueMemberS); ok && strings.HasPrefix(pkStr.Value, "FLAG#") {
				var record FlagRecord
				if err := s.UnmarshalItem(item, &record); err != nil {
					log.Error("failed to unmarshal flag record", zap.Error(err))
					continue
				}
				if record.Flag != nil && record.Flag.Actor == actorID {
					flags = append(flags, record.Flag)
				}
			}
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if encoded, err := json.Marshal(result.LastEvaluatedKey); err == nil {
			nextCursor = base64.StdEncoding.EncodeToString(encoded)
		}
	}

	log.Info("retrieved flags by actor",
		zap.Int("count", len(flags)),
		zap.Bool("has_more", nextCursor != ""),
	)

	return flags, nextCursor, nil
}

// GetPendingFlags retrieves all pending flags with pagination
func (s *dynamoDBStorage) GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	log := common.WithContext(ctx).With(
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Query using GSI1 for pending flags
	keyCondition := expression.Key("GSI1PK").Equal(expression.Value(fmt.Sprintf("FLAG#STATUS#%s", storage.FlagStatusPending)))

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		log.Error("failed to build expression", zap.Error(err))
		return nil, "", fmt.Errorf("failed to build expression: %w", err)
	}

	input := &dynamodb.QueryInput{
		TableName:                 s.getTableName(),
		IndexName:                 aws.String("GSI1"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     safeInt32(limit),
		ScanIndexForward:          aws.Bool(false), // Most recent first
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FLAG#STATUS#%s", storage.FlagStatusPending)},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("failed to query pending flags", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query pending flags: %w", err)
	}

	// Convert results to flags
	flags := make([]*storage.Flag, 0, len(result.Items))
	for _, item := range result.Items {
		var record FlagRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			log.Error("failed to unmarshal flag record", zap.Error(err))
			continue
		}
		flags = append(flags, record.Flag)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	log.Info("retrieved pending flags",
		zap.Int("count", len(flags)),
		zap.Bool("has_more", nextCursor != ""),
	)

	return flags, nextCursor, nil
}

// UpdateFlagStatus updates the status of a flag
func (s *dynamoDBStorage) UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	log := common.WithContext(ctx).With(
		zap.String("flag_id", id),
		zap.String("status", string(status)),
		zap.String("reviewed_by", reviewedBy),
	)

	// First, get the current flag to update the GSI
	currentFlag, err := s.GetFlag(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get current flag: %w", err)
	}

	// Create update expression
	now := time.Now()
	update := expression.
		Set(expression.Name("Flag.Status"), expression.Value(status)).
		Set(expression.Name("Flag.ReviewedBy"), expression.Value(reviewedBy)).
		Set(expression.Name("Flag.ReviewedAt"), expression.Value(now)).
		Set(expression.Name("Flag.ReviewNote"), expression.Value(reviewNote)).
		Set(expression.Name("GSI1PK"), expression.Value(fmt.Sprintf("FLAG#STATUS#%s", status))).
		Set(expression.Name("GSI1SK"), expression.Value(fmt.Sprintf("CREATED#%s", currentFlag.CreatedAt.Format(time.RFC3339Nano))))

	expr, err := expression.NewBuilder().
		WithUpdate(update).
		Build()
	if err != nil {
		log.Error("failed to build update expression", zap.Error(err))
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FLAG#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "FLAG"},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		log.Error("failed to update flag status", zap.Error(err))
		return fmt.Errorf("failed to update flag status: %w", err)
	}

	log.Info("flag status updated successfully")
	return nil
}

// CountPendingFlags returns the count of pending flags
func (s *dynamoDBStorage) CountPendingFlags(ctx context.Context) (int, error) {
	log := common.WithContext(ctx)

	// Query using GSI1 for pending flags count
	keyCondition := expression.Key("GSI1PK").Equal(expression.Value(fmt.Sprintf("FLAG#STATUS#%s", storage.FlagStatusPending)))

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		log.Error("failed to build expression", zap.Error(err))
		return 0, fmt.Errorf("failed to build expression: %w", err)
	}

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 s.getTableName(),
		IndexName:                 aws.String("GSI1"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Select:                    types.SelectCount,
	})
	if err != nil {
		log.Error("failed to count pending flags", zap.Error(err))
		return 0, fmt.Errorf("failed to count pending flags: %w", err)
	}

	count := int(result.Count)
	log.Info("counted pending flags", zap.Int("count", count))

	return count, nil
}
