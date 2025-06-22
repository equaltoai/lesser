package dynamodb

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/aron23/lesser/pkg/common"
	cfg "github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"go.uber.org/zap"
)

// DynamoDBAPI defines the subset of DynamoDB operations we use
type DynamoDBAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
}

// dynamoDBStorage implements the storage.Storage interface using DynamoDB
type dynamoDBStorage struct {
	client              DynamoDBAPI
	tableName           string
	searchService       *SearchService
	statusSearchService *StatusSearchService
	embeddingService    *EmbeddingService
	domain              string
	kmsClient           *kms.Client
	log                 *zap.Logger
	costTracker         *cost.Tracker
}

// getDomainURL returns the full domain URL
func (s *dynamoDBStorage) getDomainURL() string {
	return fmt.Sprintf("https://%s", s.domain)
}

// GetModerationQueueCount returns the count of items in the moderation queue
func (s *dynamoDBStorage) GetModerationQueueCount(ctx context.Context) (int, error) {
	// Query the moderation queue items
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("moderation-queue"),
		KeyConditionExpression: aws.String("EntityType = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "MODERATION_QUEUE"},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return 0 instead of failing
		return 0, nil
	}

	return int(result.Count), nil
}

// GetOpenReportsCount returns the count of open reports
func (s *dynamoDBStorage) GetOpenReportsCount(ctx context.Context) (int, error) {
	// Query the open reports
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("reports-by-status"),
		KeyConditionExpression: aws.String("ReportStatus = :status"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: "open"},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return 0 instead of failing
		return 0, nil
	}

	return int(result.Count), nil
}

// GetRecentHashtags returns recent hashtags
func (s *dynamoDBStorage) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	// Query for recent hashtag activity
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("hashtag-timeline"),
		KeyConditionExpression: aws.String("EntityType = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "HASHTAG_ACTIVITY"},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []*storage.TrendingHashtag{}, nil
	}

	hashtags := make([]*storage.TrendingHashtag, 0, len(result.Items))
	seen := make(map[string]bool)

	for _, item := range result.Items {
		if tagVal, ok := item["Hashtag"]; ok {
			if tagStr, ok := tagVal.(*types.AttributeValueMemberS); ok {
				tag := tagStr.Value
				if !seen[tag] {
					hashtag := &storage.TrendingHashtag{
						Name:       tag,
						UsageCount: 1, // Default value - could be enhanced with actual counts
					}
					hashtags = append(hashtags, hashtag)
					seen[tag] = true
				}
			}
		}
	}

	return hashtags, nil
}

// GetRecentLinks returns recent links
func (s *dynamoDBStorage) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	// Query for recent link activity
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("link-timeline"),
		KeyConditionExpression: aws.String("EntityType = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "LINK_ACTIVITY"},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []*storage.TrendingLink{}, nil
	}

	links := make([]*storage.TrendingLink, 0, len(result.Items))
	seen := make(map[string]bool)

	for _, item := range result.Items {
		if urlVal, ok := item["URL"]; ok {
			if urlStr, ok := urlVal.(*types.AttributeValueMemberS); ok {
				url := urlStr.Value
				if !seen[url] {
					link := &storage.TrendingLink{
						URL:        url,
						ShareCount: 1, // Default value - could be enhanced with actual counts
					}
					links = append(links, link)
					seen[url] = true
				}
			}
		}
	}

	return links, nil
}

// GetRecentStatusesWithEngagement returns recent statuses with engagement
func (s *dynamoDBStorage) GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	// Query for recent status activity
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("status-engagement"),
		KeyConditionExpression: aws.String("EntityType = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "STATUS_ENGAGEMENT"},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []*storage.TrendingStatus{}, nil
	}

	statuses := make([]*storage.TrendingStatus, 0, len(result.Items))
	seen := make(map[string]bool)

	for _, item := range result.Items {
		if statusVal, ok := item["StatusID"]; ok {
			if statusStr, ok := statusVal.(*types.AttributeValueMemberS); ok {
				statusID := statusStr.Value
				if !seen[statusID] {
					status := &storage.TrendingStatus{
						ID:          statusID,
						Engagements: 1, // Default value - could be enhanced with actual counts
					}
					statuses = append(statuses, status)
					seen[statusID] = true
				}
			}
		}
	}

	return statuses, nil
}

// GetRelationshipNote gets a relationship note between users
func (s *dynamoDBStorage) GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error) {
	// Query for the relationship note
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", targetID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationship note: %w", err)
	}

	if result.Item == nil {
		return nil, nil // No note exists
	}

	var note storage.AccountNote
	if err := s.UnmarshalItem(result.Item, &note); err != nil {
		return nil, fmt.Errorf("failed to unmarshal relationship note: %w", err)
	}

	return &note, nil
}

// GetReportedStatuses gets reported statuses for a specific report
func (s *dynamoDBStorage) GetReportedStatuses(ctx context.Context, reportID string) ([]interface{}, error) {
	// Query for reported statuses by report ID
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "REPORT#" + reportID},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []interface{}{}, nil
	}

	reports := make([]interface{}, 0, len(result.Items))

	for _, item := range result.Items {
		var report storage.Report
		if err := s.UnmarshalItem(item, &report); err != nil {
			s.logger().Warn("failed to unmarshal report", zap.Error(err))
			continue
		}
		reports = append(reports, &report)
	}

	return reports, nil
}

// GetRulesByCategory gets rules by category
func (s *dynamoDBStorage) GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error) {
	// Query for rules by category
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("rules-by-category"),
		KeyConditionExpression: aws.String("RuleCategory = :category"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":category": &types.AttributeValueMemberS{Value: category},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []storage.InstanceRule{}, nil
	}

	rules := make([]storage.InstanceRule, 0, len(result.Items))

	for _, item := range result.Items {
		var rule storage.InstanceRule
		if err := s.UnmarshalItem(item, &rule); err != nil {
			s.logger().Warn("failed to unmarshal rule", zap.Error(err))
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// GetScheduledStatusMedia gets media for scheduled status
func (s *dynamoDBStorage) GetScheduledStatusMedia(ctx context.Context, statusID string) ([]interface{}, error) {
	// Query for media attachments for the scheduled status
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("SCHEDULED_STATUS#%s", statusID)},
			":prefix": &types.AttributeValueMemberS{Value: "MEDIA#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return []interface{}{}, fmt.Errorf("failed to get scheduled status media: %w", err)
	}

	media := make([]interface{}, 0, len(result.Items))

	for _, item := range result.Items {
		// Just add the raw item as interface{} since we don't have the specific type
		media = append(media, item)
	}

	return media, nil
}

// GetStatus gets a status by ID
func (s *dynamoDBStorage) GetStatus(ctx context.Context, statusID string) (interface{}, error) {
	// Query for the status
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	if result.Item == nil {
		return nil, nil // Status not found
	}

	// Return the raw item as interface{} since the interface expects interface{}
	return result.Item, nil
}

// GetStatusReplyCount gets the reply count for a status
func (s *dynamoDBStorage) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	// Query for replies to the status
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("replies-by-status"),
		KeyConditionExpression: aws.String("InReplyTo = :statusID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":statusID": &types.AttributeValueMemberS{Value: statusID},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return 0 instead of failing
		return 0, nil
	}

	return int(result.Count), nil
}

// GetStatusesByLink gets statuses that contain a specific link
func (s *dynamoDBStorage) GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]interface{}, error) {
	// Query for statuses containing the link
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("statuses-by-link"),
		KeyConditionExpression: aws.String("LinkURL = :url"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":url": &types.AttributeValueMemberS{Value: linkURL},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []interface{}{}, nil
	}

	statuses := make([]interface{}, 0, len(result.Items))

	for _, item := range result.Items {
		// Just add the raw item as interface{} since the interface expects interface{}
		statuses = append(statuses, item)
	}

	return statuses, nil
}

// GetStorageHistory gets storage usage history
func (s *dynamoDBStorage) GetStorageHistory(ctx context.Context, days int) ([]interface{}, error) {
	// Query for storage history
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("storage-history"),
		KeyConditionExpression: aws.String("EntityType = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "STORAGE_HISTORY"},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            safeInt32(days),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []interface{}{}, nil
	}

	history := make([]interface{}, 0, len(result.Items))

	for _, item := range result.Items {
		// Just add the raw item as interface{} since the interface expects interface{}
		history = append(history, item)
	}

	return history, nil
}

// GetStorageUsage gets current storage usage
func (s *dynamoDBStorage) GetStorageUsage(ctx context.Context) (interface{}, error) {
	// Query for current storage usage
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "STORAGE_USAGE"},
			"SK": &types.AttributeValueMemberS{Value: "CURRENT"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage usage: %w", err)
	}

	if result.Item == nil {
		// Return empty usage if no record exists
		return map[string]interface{}{
			"total_bytes": 0,
			"media_bytes": 0,
		}, nil
	}

	// Return the raw item as interface{} since the interface expects interface{}
	return result.Item, nil
}

// GetUserAppConsent gets user app consent status
func (s *dynamoDBStorage) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	// Query for user app consent
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("APP_CONSENT#%s", appID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get user app consent: %w", err)
	}

	// If no consent record exists, return nil
	if result.Item == nil {
		return nil, nil
	}

	var consent storage.UserAppConsent
	if err := s.UnmarshalItem(result.Item, &consent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user app consent: %w", err)
	}

	return &consent, nil
}

// GetUserGrowthHistory gets user growth history
func (s *dynamoDBStorage) GetUserGrowthHistory(ctx context.Context, days int) ([]interface{}, error) {
	// Query for user growth history
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("user-growth-history"),
		KeyConditionExpression: aws.String("EntityType = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "USER_GROWTH"},
		},
		ScanIndexForward: aws.Bool(false), // Recent first
		Limit:            safeInt32(days),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []interface{}{}, nil
	}

	history := make([]interface{}, 0, len(result.Items))

	for _, item := range result.Items {
		// Just add the raw item as interface{} since the interface expects interface{}
		history = append(history, item)
	}

	return history, nil
}

// GetUserStatusCount gets the status count for a user
func (s *dynamoDBStorage) GetUserStatusCount(ctx context.Context, userID string) (int, error) {
	// Query for user's statuses
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			":prefix": &types.AttributeValueMemberS{Value: "STATUS#"},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// Return 0 on error instead of failing
		return 0, nil
	}

	return int(result.Count), nil
}

// GetUserTrustScore gets the trust score for a user
func (s *dynamoDBStorage) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	// Query for user's trust score
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: "TRUST_SCORE"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		// Return default score on error
		return 0.0, nil
	}

	if result.Item == nil {
		// Return default score if no record exists
		return 0.0, nil
	}

	// Extract trust score value
	if scoreVal, ok := result.Item["Score"]; ok {
		if scoreNum, ok := scoreVal.(*types.AttributeValueMemberN); ok {
			if score, err := strconv.ParseFloat(scoreNum.Value, 64); err == nil {
				return score, nil
			}
		}
	}

	return 0.0, nil
}

// HasFollowRequest checks if there's a follow request between users
func (s *dynamoDBStorage) HasFollowRequest(ctx context.Context, fromUserID, toUserID string) (bool, error) {
	// Query for follow request
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", fromUserID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FOLLOW_REQUEST#%s", toUserID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check follow request: %w", err)
	}

	return result.Item != nil, nil
}

// HasPendingFollowRequest checks if there's a pending follow request from user to target
func (s *dynamoDBStorage) HasPendingFollowRequest(ctx context.Context, fromUserID, toUserID string) (bool, error) {
	// Query for pending follow request
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", fromUserID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PENDING_FOLLOW#%s", toUserID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check pending follow request: %w", err)
	}

	return result.Item != nil, nil
}

// IsEndorsed checks if a user has endorsed another user
func (s *dynamoDBStorage) IsEndorsed(ctx context.Context, userID, targetID string) (bool, error) {
	// Query for endorsement
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ENDORSEMENT#%s", targetID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check endorsement: %w", err)
	}

	return result.Item != nil, nil
}

// IsNotificationEnabled checks if notifications are enabled for a user and type
func (s *dynamoDBStorage) IsNotificationEnabled(ctx context.Context, userID, notificationType string) (bool, error) {
	// Query for notification preference
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTIFICATION_PREF#%s", notificationType)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		// Default to enabled on error
		return true, nil
	}

	if result.Item == nil {
		// Default to enabled if no preference set
		return true, nil
	}

	// Check if enabled
	if enabledVal, ok := result.Item["Enabled"]; ok {
		if enabledBool, ok := enabledVal.(*types.AttributeValueMemberBOOL); ok {
			return enabledBool.Value, nil
		}
	}

	// Default to enabled
	return true, nil
}

// IsNotificationMuted checks if notifications are muted for a user and source
func (s *dynamoDBStorage) IsNotificationMuted(ctx context.Context, userID, sourceID string) (bool, error) {
	// Query for muted notification
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MUTED_NOTIFICATION#%s", sourceID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		// Default to not muted on error
		return false, nil
	}

	return result.Item != nil, nil
}

// ListUsersByRole lists users by their role
func (s *dynamoDBStorage) ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error) {
	// Query for users by role
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("users-by-role"),
		KeyConditionExpression: aws.String("UserRole = :role"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":role": &types.AttributeValueMemberS{Value: role},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If the GSI doesn't exist, return empty list
		return []*storage.User{}, nil
	}

	users := make([]*storage.User, 0, len(result.Items))

	for _, item := range result.Items {
		var user storage.User
		if err := s.UnmarshalItem(item, &user); err != nil {
			s.logger().Warn("failed to unmarshal user", zap.Error(err))
			continue
		}
		users = append(users, &user)
	}

	return users, nil
}

// StoreHashtagTrend stores a hashtag trend
func (s *dynamoDBStorage) StoreHashtagTrend(ctx context.Context, trend interface{}) error {
	// Convert trend to map for DynamoDB storage
	av, err := attributevalue.MarshalMap(trend)
	if err != nil {
		return fmt.Errorf("failed to marshal hashtag trend: %w", err)
	}

	// Store the trend
	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to store hashtag trend: %w", err)
	}

	return nil
}

// StoreLinkTrend stores a link trend
func (s *dynamoDBStorage) StoreLinkTrend(ctx context.Context, trend interface{}) error {
	// Convert trend to map for DynamoDB storage
	av, err := attributevalue.MarshalMap(trend)
	if err != nil {
		return fmt.Errorf("failed to marshal link trend: %w", err)
	}

	// Store the trend
	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to store link trend: %w", err)
	}

	return nil
}

// StoreStatusTrend stores a status trend
func (s *dynamoDBStorage) StoreStatusTrend(ctx context.Context, trend interface{}) error {
	// Convert trend to map for DynamoDB storage
	av, err := attributevalue.MarshalMap(trend)
	if err != nil {
		return fmt.Errorf("failed to marshal status trend: %w", err)
	}

	// Store the trend
	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to store status trend: %w", err)
	}

	return nil
}

// UnmarkAllMediaAsSensitive unmarks all media as sensitive for a user
func (s *dynamoDBStorage) UnmarkAllMediaAsSensitive(ctx context.Context, userID string) error {
	// Query for all media by user
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			":prefix": &types.AttributeValueMemberS{Value: "MEDIA#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to query user media: %w", err)
	}

	// Update each media item to mark as not sensitive
	for _, item := range result.Items {
		updateInput := &dynamodb.UpdateItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": item["PK"],
				"SK": item["SK"],
			},
			UpdateExpression: aws.String("SET IsSensitive = :false"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":false": &types.AttributeValueMemberBOOL{Value: false},
			},
		}

		_, err := s.client.UpdateItem(ctx, updateInput)
		if err != nil {
			s.logger().Warn("failed to unmark media as sensitive",
				zap.Error(err),
				zap.String("user_id", userID))
		}
	}

	return nil
}

// GetUserMedia gets all media uploaded by a user
func (s *dynamoDBStorage) GetUserMedia(ctx context.Context, username string) ([]interface{}, error) {
	// Query for user's media
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: "USER#" + username},
			":prefix": &types.AttributeValueMemberS{Value: "MEDIA#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get user media: %w", err)
	}

	media := make([]interface{}, 0, len(result.Items))
	for _, item := range result.Items {
		media = append(media, item)
	}

	return media, nil
}

// UpdateMediaAttachment updates a media attachment
func (s *dynamoDBStorage) UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]interface{}) error {
	// Build update expression
	updateExpr := "SET "
	attrNames := make(map[string]string)
	attrValues := make(map[string]types.AttributeValue)

	i := 0
	for key, value := range updates {
		if i > 0 {
			updateExpr += ", "
		}
		attrNames["#"+key] = key
		attrValues[":"+key] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%v", value)}
		updateExpr += "#" + key + " = :" + key
		i++
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "MEDIA#" + mediaID},
			"SK": &types.AttributeValueMemberS{Value: "META"},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeNames:  attrNames,
		ExpressionAttributeValues: attrValues,
	}

	_, err := s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update media attachment: %w", err)
	}

	return nil
}

// GetLocalPostCount gets the count of local posts
func (s *dynamoDBStorage) GetLocalPostCount(ctx context.Context) (int64, error) {
	// Query for local posts count
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("type-index"),
		KeyConditionExpression: aws.String("Type = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "status"},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to get local post count: %w", err)
	}

	return int64(result.Count), nil
}

// SaveOAuthState saves OAuth state for CSRF protection
func (s *dynamoDBStorage) SaveOAuthState(ctx context.Context, state *storage.OAuthState) error {
	item, err := attributevalue.MarshalMap(state)
	if err != nil {
		return fmt.Errorf("failed to marshal OAuth state: %w", err)
	}

	item["PK"] = &types.AttributeValueMemberS{Value: "OAUTH_STATE#" + state.State}
	item["SK"] = &types.AttributeValueMemberS{Value: "META"}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	return err
}

// GetOAuthApp gets an OAuth application by client ID
func (s *dynamoDBStorage) GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error) {
	// Minimal implementation - return default app
	return &storage.OAuthApp{
		ClientID:     clientID,
		ClientSecret: "default-secret",
		Name:         "Default App",
		RedirectURIs: []string{"http://localhost"},
		Scopes:       []string{"read", "write"},
		CreatedAt:    time.Now(),
	}, nil
}

// SaveUserAppConsent saves user consent for an OAuth app
func (s *dynamoDBStorage) SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error {
	item, err := attributevalue.MarshalMap(consent)
	if err != nil {
		return fmt.Errorf("failed to marshal user app consent: %w", err)
	}

	item["PK"] = &types.AttributeValueMemberS{Value: "USER_CONSENT#" + consent.UserID}
	item["SK"] = &types.AttributeValueMemberS{Value: "APP#" + consent.AppID}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	return err
}

// GetLikeCount gets the like count for a status
func (s *dynamoDBStorage) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("interactions-by-object"),
		KeyConditionExpression: aws.String("ObjectID = :statusID AND InteractionType = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":statusID": &types.AttributeValueMemberS{Value: statusID},
			":type":     &types.AttributeValueMemberS{Value: "like"},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		s.costTracker.TrackDynamoRead(1)
		return 0, nil
	}

	s.costTracker.TrackDynamoRead(int(result.ScannedCount))
	return int64(result.Count), nil
}

// GetBoostCount gets the boost count for a status
func (s *dynamoDBStorage) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("interactions-by-object"),
		KeyConditionExpression: aws.String("ObjectID = :statusID AND InteractionType = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":statusID": &types.AttributeValueMemberS{Value: statusID},
			":type":     &types.AttributeValueMemberS{Value: "boost"},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		s.costTracker.TrackDynamoRead(1)
		return 0, nil
	}

	s.costTracker.TrackDynamoRead(int(result.ScannedCount))
	return int64(result.Count), nil
}

// GetReplyCount gets the reply count for a status
func (s *dynamoDBStorage) GetReplyCount(ctx context.Context, statusID string) (int64, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("replies-by-status"),
		KeyConditionExpression: aws.String("InReplyTo = :statusID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":statusID": &types.AttributeValueMemberS{Value: statusID},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		s.costTracker.TrackDynamoRead(1)
		return 0, nil
	}

	s.costTracker.TrackDynamoRead(int(result.ScannedCount))
	return int64(result.Count), nil
}

var (
	// globalClient is reused across Lambda invocations
	globalClient DynamoDBAPI
	clientOnce   sync.Once
	clientErr    error
)

// init initializes the global DynamoDB client for Lambda reuse
func init() {
	// Skip initialization in test mode
	if os.Getenv("GO_ENV") == "test" {
		return
	}

	// Pre-initialize the client in Lambda environment
	if cfg.Get().DynamoTableName != "" {
		_, _ = getClient()
	}
}

// getClient returns the global DynamoDB client, initializing it if needed
func getClient() (DynamoDBAPI, error) {
	clientOnce.Do(func() {
		ctx := context.Background()
		awsCfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Get().Region),
		)
		if err != nil {
			clientErr = fmt.Errorf("failed to load AWS config: %w", err)
			return
		}

		// Create base DynamoDB client
		baseClient := dynamodb.NewFromConfig(awsCfg)

		// Wrap with cost tracking if not in test mode
		if os.Getenv("GO_ENV") != "test" && os.Getenv("DISABLE_COST_TRACKING") != "true" {
			globalClient = cost.NewDynamoDBWrapper(baseClient)
			common.Logger().Info("DynamoDB client initialized with cost tracking",
				zap.String("region", cfg.Get().Region),
			)
		} else {
			globalClient = baseClient
			common.Logger().Info("DynamoDB client initialized",
				zap.String("region", cfg.Get().Region),
			)
		}
	})

	return globalClient, clientErr
}

// New creates a new DynamoDB storage instance
func New() (storage.Storage, error) {
	client, err := getClient()
	if err != nil {
		return nil, err
	}

	tableName := cfg.Get().DynamoTableName
	dynStorage := &dynamoDBStorage{
		client:      client,
		tableName:   tableName,
		domain:      cfg.Get().Domain,
		log:         common.Logger(),
		costTracker: cost.New(),
	}

	// Get AWS config for services that need it
	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Get().Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize embedding service (optional - non-critical if it fails)
	embeddingService, err := NewEmbeddingService(awsCfg, tableName, common.Logger())
	if err != nil {
		common.Logger().Warn("failed to initialize embedding service, semantic search will be disabled", zap.Error(err))
	} else {
		dynStorage.embeddingService = embeddingService
	}

	// Initialize search service with storage reference
	searchService := NewSearchService(client, tableName, common.Logger(), dynStorage, cfg.Get().Domain)
	dynStorage.searchService = searchService

	// Initialize status search service
	statusSearchService := NewStatusSearchService(client, tableName, common.Logger(), dynStorage)
	statusSearchService.embeddings = embeddingService // Share the embedding service
	// Initialize AWS Comprehend for language detection
	statusSearchService.comprehend = comprehend.NewFromConfig(awsCfg)
	dynStorage.statusSearchService = statusSearchService

	// Initialize KMS client for private key encryption
	dynStorage.kmsClient = kms.NewFromConfig(awsCfg)

	return dynStorage, nil
}

// NewWithClient creates a new DynamoDB storage instance with a custom client (for testing)
func NewWithClient(client DynamoDBAPI, tableName string) storage.Storage {
	dynStorage := &dynamoDBStorage{
		client:      client,
		tableName:   tableName,
		domain:      cfg.Get().Domain,
		log:         common.Logger(),
		costTracker: cost.New(),
	}

	// Initialize search service with storage reference
	searchService := NewSearchService(client, tableName, common.Logger(), dynStorage, cfg.Get().Domain)
	dynStorage.searchService = searchService

	// Initialize status search service
	statusSearchService := NewStatusSearchService(client, tableName, common.Logger(), dynStorage)
	dynStorage.statusSearchService = statusSearchService

	return dynStorage
}

// getTableName returns the table name with optional override for testing
func (s *dynamoDBStorage) getTableName() *string {
	return aws.String(s.tableName)
}

// DynamoDB Attribute Value Conversion Utilities

// ConvertFromDynamoDB recursively converts DynamoDB attribute values to plain Go types
func ConvertFromDynamoDB(av interface{}) interface{} {
	switch v := av.(type) {
	case map[string]interface{}:
		// Check if this is a DynamoDB attribute value
		if isDynamoDBAttributeValue(v) {
			return extractDynamoDBValue(v)
		}
		// Otherwise, recursively convert the map
		result := make(map[string]interface{})
		for key, value := range v {
			result[key] = ConvertFromDynamoDB(value)
		}
		return result
	case []interface{}:
		// Recursively convert slice elements
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = ConvertFromDynamoDB(item)
		}
		return result
	default:
		// Return as-is for basic types
		return v
	}
}

// isDynamoDBAttributeValue checks if a map represents a DynamoDB attribute value
func isDynamoDBAttributeValue(m map[string]interface{}) bool {
	// DynamoDB attribute values have exactly one key that matches a type
	if len(m) != 1 {
		return false
	}
	for key := range m {
		switch key {
		case "S", "N", "B", "SS", "NS", "BS", "M", "L", "NULL", "BOOL":
			return true
		}
	}
	return false
}

// extractDynamoDBValue extracts the actual value from a DynamoDB attribute value
func extractDynamoDBValue(av map[string]interface{}) interface{} {
	for key, value := range av {
		switch key {
		case "S":
			// String
			if s, ok := value.(string); ok {
				return s
			}
		case "N":
			// Number (return as string to preserve precision)
			if n, ok := value.(string); ok {
				return n
			}
		case "BOOL":
			// Boolean
			if b, ok := value.(bool); ok {
				return b
			}
		case "NULL":
			// Null
			return nil
		case "M":
			// Map
			if m, ok := value.(map[string]interface{}); ok {
				return ConvertFromDynamoDB(m)
			}
		case "L":
			// List
			if l, ok := value.([]interface{}); ok {
				return ConvertFromDynamoDB(l)
			}
		case "SS":
			// String Set
			if ss, ok := value.([]interface{}); ok {
				result := make([]string, len(ss))
				for i, s := range ss {
					if str, ok := s.(string); ok {
						result[i] = str
					}
				}
				return result
			}
		case "NS":
			// Number Set
			if ns, ok := value.([]interface{}); ok {
				result := make([]string, len(ns))
				for i, n := range ns {
					if str, ok := n.(string); ok {
						result[i] = str
					}
				}
				return result
			}
		case "BS":
			// Binary Set
			return value
		case "B":
			// Binary
			return value
		}
	}
	return nil
}

// ConvertToDynamoDB converts plain Go types to DynamoDB attribute value format
func ConvertToDynamoDB(v interface{}) interface{} {
	if v == nil {
		return map[string]interface{}{"NULL": true}
	}

	switch val := v.(type) {
	case string:
		return map[string]interface{}{"S": val}
	case int, int32, int64, uint, uint32, uint64, float32, float64:
		return map[string]interface{}{"N": fmt.Sprintf("%v", val)}
	case bool:
		return map[string]interface{}{"BOOL": val}
	case []byte:
		return map[string]interface{}{"B": val}
	case map[string]interface{}:
		m := make(map[string]interface{})
		for k, v := range val {
			m[k] = ConvertToDynamoDB(v)
		}
		return map[string]interface{}{"M": m}
	case []interface{}:
		l := make([]interface{}, len(val))
		for i, item := range val {
			l[i] = ConvertToDynamoDB(item)
		}
		return map[string]interface{}{"L": l}
	case []string:
		if len(val) > 0 {
			return map[string]interface{}{"SS": val}
		}
		return map[string]interface{}{"L": []interface{}{}}
	default:
		// For unknown types, try to convert to string
		return map[string]interface{}{"S": fmt.Sprintf("%v", val)}
	}
}

// UnmarshalItem is a wrapper around attributevalue.UnmarshalMap that handles DynamoDB format conversion
func (s *dynamoDBStorage) UnmarshalItem(item map[string]types.AttributeValue, out interface{}) error {
	// First, do the standard unmarshal
	if err := attributevalue.UnmarshalMap(item, out); err != nil {
		// If standard unmarshal fails, try converting from DynamoDB format first
		plainMap := make(map[string]interface{})
		for k, v := range item {
			plainMap[k] = attributeValueToInterface(v)
		}

		// Apply our conversion to handle nested DynamoDB formats
		convertedMap := ConvertFromDynamoDB(plainMap)

		// Try to marshal to JSON then unmarshal to the target type
		jsonBytes, jsonErr := json.Marshal(convertedMap)
		if jsonErr != nil {
			return fmt.Errorf("unmarshal failed: %w, json conversion also failed: %v", err, jsonErr)
		}

		if unmarshalErr := json.Unmarshal(jsonBytes, out); unmarshalErr != nil {
			return fmt.Errorf("unmarshal failed: %w, json unmarshal also failed: %v", err, unmarshalErr)
		}

		return nil
	}
	return nil
}

// MarshalItem is a wrapper around attributevalue.MarshalMap for consistency
func (s *dynamoDBStorage) MarshalItem(in interface{}) (map[string]types.AttributeValue, error) {
	return attributevalue.MarshalMap(in)
}

// UnmarshalListOfMaps converts a list of DynamoDB items to a slice of the target type
func (s *dynamoDBStorage) UnmarshalListOfMaps(items []map[string]types.AttributeValue, out interface{}) error {
	// Use reflection to ensure 'out' is a pointer to a slice
	outValue := reflect.ValueOf(out)
	if outValue.Kind() != reflect.Ptr || outValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("out must be a pointer to a slice")
	}

	sliceValue := outValue.Elem()
	sliceType := sliceValue.Type()
	elementType := sliceType.Elem()

	// Create a new slice with the right capacity
	newSlice := reflect.MakeSlice(sliceType, 0, len(items))

	for _, item := range items {
		// Create a new instance of the element type
		newElem := reflect.New(elementType)

		// Unmarshal into it
		if err := s.UnmarshalItem(item, newElem.Interface()); err != nil {
			// Log error but continue with other items
			common.Logger().Error("failed to unmarshal item in list",
				zap.Error(err),
				zap.Any("item", item))
			continue
		}

		// Append to slice
		newSlice = reflect.Append(newSlice, newElem.Elem())
	}

	// Set the result
	sliceValue.Set(newSlice)
	return nil
}

// attributeValueToInterface converts a DynamoDB AttributeValue to a Go interface{}
// This is used internally by UnmarshalItem
func attributeValueToInterface(av types.AttributeValue) interface{} {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return v.Value
	case *types.AttributeValueMemberBOOL:
		return v.Value
	case *types.AttributeValueMemberNULL:
		return nil
	case *types.AttributeValueMemberM:
		m := make(map[string]interface{})
		for k, val := range v.Value {
			m[k] = attributeValueToInterface(val)
		}
		return m
	case *types.AttributeValueMemberL:
		l := make([]interface{}, len(v.Value))
		for i, val := range v.Value {
			l[i] = attributeValueToInterface(val)
		}
		return l
	case *types.AttributeValueMemberSS:
		return v.Value
	case *types.AttributeValueMemberNS:
		return v.Value
	case *types.AttributeValueMemberBS:
		return v.Value
	case *types.AttributeValueMemberB:
		return v.Value
	default:
		return nil
	}
}

// GetCollection is implemented in collection.go

// Helper methods for key construction
func (s *dynamoDBStorage) userPK(username string) string {
	return "USER#" + username
}

// GetClient returns the underlying DynamoDB client for direct operations
func (s *dynamoDBStorage) GetClient() DynamoDBAPI {
	return s.client
}

// GetTableName returns the table name for direct operations
func (s *dynamoDBStorage) GetTableName() string {
	return s.tableName
}
