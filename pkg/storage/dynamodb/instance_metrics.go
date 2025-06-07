package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// GetTotalUserCount returns the total number of users in the system
func (s *dynamoDBStorage) GetTotalUserCount(ctx context.Context) (int64, error) {
	var count int64
	var lastKey map[string]types.AttributeValue

	// Scan for all USER# entries
	for {
		input := &dynamodb.ScanInput{
			TableName:        aws.String(s.tableName),
			FilterExpression: aws.String("begins_with(PK, :pk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "USER#"},
			},
			Select:            types.SelectCount,
			ExclusiveStartKey: lastKey,
		}

		result, err := s.client.Scan(ctx, input)
		if err != nil {
			return 0, fmt.Errorf("failed to count users: %w", err)
		}

		count += int64(result.Count)

		if result.LastEvaluatedKey == nil {
			break
		}
		lastKey = result.LastEvaluatedKey
	}

	return count, nil
}

// GetTotalStatusCount returns the total number of statuses in the system
func (s *dynamoDBStorage) GetTotalStatusCount(ctx context.Context) (int64, error) {
	var count int64
	var lastKey map[string]types.AttributeValue

	// Count all OBJECT# entries that are Note/Article/Page types
	for {
		input := &dynamodb.ScanInput{
			TableName:        aws.String(s.tableName),
			FilterExpression: aws.String("begins_with(PK, :pk) AND SK = :sk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "OBJECT#"},
				":sk": &types.AttributeValueMemberS{Value: "OBJECT"},
			},
			Select:            types.SelectCount,
			ExclusiveStartKey: lastKey,
		}

		result, err := s.client.Scan(ctx, input)
		if err != nil {
			return 0, fmt.Errorf("failed to count statuses: %w", err)
		}

		count += int64(result.Count)

		if result.LastEvaluatedKey == nil {
			break
		}
		lastKey = result.LastEvaluatedKey
	}

	return count, nil
}

// GetTotalDomainCount returns the total number of unique domains in the system
func (s *dynamoDBStorage) GetTotalDomainCount(ctx context.Context) (int64, error) {
	// Get unique domains from remote actors
	domainMap := make(map[string]bool)
	var lastKey map[string]types.AttributeValue

	for {
		input := &dynamodb.ScanInput{
			TableName:        aws.String(s.tableName),
			FilterExpression: aws.String("begins_with(PK, :pk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "ACTOR#"},
			},
			ProjectionExpression: aws.String("Actor.id"),
			ExclusiveStartKey:    lastKey,
		}

		result, err := s.client.Scan(ctx, input)
		if err != nil {
			return 0, fmt.Errorf("failed to scan actors: %w", err)
		}

		for _, item := range result.Items {
			if actorIDAttr, ok := item["Actor"].(*types.AttributeValueMemberM); ok {
				if idAttr, ok := actorIDAttr.Value["id"].(*types.AttributeValueMemberS); ok {
					// Extract domain from actor ID
					if strings.Contains(idAttr.Value, "https://") {
						parts := strings.Split(idAttr.Value, "/")
						if len(parts) >= 3 {
							domain := strings.Replace(parts[2], "www.", "", 1)
							// Skip our own domain
							if !strings.Contains(domain, s.domain) {
								domainMap[domain] = true
							}
						}
					}
				}
			}
		}

		if result.LastEvaluatedKey == nil {
			break
		}
		lastKey = result.LastEvaluatedKey
	}

	return int64(len(domainMap)), nil
}

// GetWeeklyActivity retrieves activity metrics for a specific week
func (s *dynamoDBStorage) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	pk := fmt.Sprintf("METRICS#WEEK#%d", weekTimestamp)

	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: "ACTIVITY"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get weekly activity: %w", err)
	}

	if result.Item == nil {
		// Return empty activity if not found
		return &storage.WeeklyActivity{
			Week:          weekTimestamp,
			Statuses:      0,
			Logins:        0,
			Registrations: 0,
		}, nil
	}

	var activity storage.WeeklyActivity
	if err := attributevalue.UnmarshalMap(result.Item, &activity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal activity: %w", err)
	}

	return &activity, nil
}

// RecordActivity records an activity event for metrics tracking
func (s *dynamoDBStorage) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	// Calculate week start (Monday at 00:00:00 UTC)
	weekStart := timestamp.Truncate(24 * time.Hour)
	for weekStart.Weekday() != time.Monday {
		weekStart = weekStart.Add(-24 * time.Hour)
	}
	weekTimestamp := weekStart.Unix()

	pk := fmt.Sprintf("METRICS#WEEK#%d", weekTimestamp)

	// Update the weekly activity counter
	var updateExpr expression.UpdateBuilder
	switch activityType {
	case "status":
		updateExpr = expression.Add(expression.Name("statuses"), expression.Value(1))
	case "login":
		// For logins, we'll track unique logins in a separate item
		// For now, just increment the counter
		updateExpr = expression.Add(expression.Name("logins"), expression.Value(1))
	case "registration":
		updateExpr = expression.Add(expression.Name("registrations"), expression.Value(1))
	default:
		return fmt.Errorf("unknown activity type: %s", activityType)
	}

	// Set the week timestamp if it doesn't exist
	updateExpr = updateExpr.Set(expression.Name("week"), expression.Value(weekTimestamp))

	expr, err := expression.NewBuilder().WithUpdate(updateExpr).Build()
	if err != nil {
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: "ACTIVITY"},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update activity metrics: %w", err)
	}

	// For unique login tracking, we could store daily unique users
	if activityType == "login" {
		dayKey := fmt.Sprintf("METRICS#DAY#%s", timestamp.Format("2006-01-02"))
		userKey := fmt.Sprintf("USER#%s", actorID)

		// Store user login for the day
		loginItem := map[string]types.AttributeValue{
			"PK":  &types.AttributeValueMemberS{Value: dayKey},
			"SK":  &types.AttributeValueMemberS{Value: userKey},
			"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", timestamp.Add(90*24*time.Hour).Unix())}, // 90 day TTL
		}

		putInput := &dynamodb.PutItemInput{
			TableName: aws.String(s.tableName),
			Item:      loginItem,
		}

		if _, err := s.client.PutItem(ctx, putInput); err != nil {
			s.logger().Warn("failed to record unique login", zap.Error(err))
		}
	}

	return nil
}

// GetContactAccount returns the admin account info for instance contact
func (s *dynamoDBStorage) GetContactAccount(ctx context.Context) (*storage.ActorRecord, error) {
	// Find the first admin user
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :pk) AND #role = :role"),
		ExpressionAttributeNames: map[string]string{
			"#role": "role",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":   &types.AttributeValueMemberS{Value: "USER#"},
			":role": &types.AttributeValueMemberS{Value: "admin"},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to find admin user: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil // No admin found
	}

	// Extract username from the first admin
	var user storage.User
	if err := attributevalue.UnmarshalMap(result.Items[0], &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	// Get the actor for this admin user
	actorInput := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", user.Username)},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	}

	actorResult, err := s.client.GetItem(ctx, actorInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get admin actor: %w", err)
	}

	if actorResult.Item == nil {
		return nil, nil
	}

	var actorRecord storage.ActorRecord
	if err := attributevalue.UnmarshalMap(actorResult.Item, &actorRecord); err != nil {
		return nil, fmt.Errorf("failed to unmarshal actor: %w", err)
	}

	return &actorRecord, nil
}
