package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// FederationSeveranceRecord represents a severed federation relationship
type FederationSeveranceRecord struct {
	PK           string    `dynamodbav:"PK"` // USER#userID
	SK           string    `dynamodbav:"SK"` // SEVERANCE#domain
	UserID       string    `dynamodbav:"UserID"`
	Domain       string    `dynamodbav:"Domain"`
	SeveredAt    time.Time `dynamodbav:"SeveredAt"`
	Acknowledged bool      `dynamodbav:"Acknowledged"`
	Reason       string    `dynamodbav:"Reason"`
	Type         string    `dynamodbav:"Type"` // "domain_block", "suspension", "defederation"

	// GSI attributes for global severance tracking
	GSI1PK string `dynamodbav:"GSI1PK"` // SEVERANCE#domain
	GSI1SK string `dynamodbav:"GSI1SK"` // USER#userID
}

// FederationIssueRecord tracks federation issues for monitoring
type FederationIssueRecord struct {
	PK          string    `dynamodbav:"PK"` // FEDERATION_ISSUE#domain
	SK          string    `dynamodbav:"SK"` // TIMESTAMP#timestamp
	Domain      string    `dynamodbav:"Domain"`
	IssueType   string    `dynamodbav:"IssueType"` // "timeout", "error", "unreachable", "blocked"
	Timestamp   time.Time `dynamodbav:"Timestamp"`
	Description string    `dynamodbav:"Description,omitempty"`
	Severity    string    `dynamodbav:"Severity"` // "low", "medium", "high", "critical"
	Resolved    bool      `dynamodbav:"Resolved"`
	TTL         int64     `dynamodbav:"TTL"` // Auto-cleanup after 90 days
}

// ReconnectionAttempt tracks attempts to reconnect to severed domains
type ReconnectionAttempt struct {
	PK           string    `dynamodbav:"PK"` // RECONNECTION#userID#domain
	SK           string    `dynamodbav:"SK"` // ATTEMPT#timestamp
	UserID       string    `dynamodbav:"UserID"`
	Domain       string    `dynamodbav:"Domain"`
	AttemptedAt  time.Time `dynamodbav:"AttemptedAt"`
	Success      bool      `dynamodbav:"Success"`
	ErrorMessage string    `dynamodbav:"ErrorMessage,omitempty"`
	Method       string    `dynamodbav:"Method"` // "manual", "automatic", "scheduled"
}

// AcknowledgeSeverance marks a severance as acknowledged by the user
func (s *dynamoDBStorage) AcknowledgeSeverance(ctx context.Context, userID, domain string) error {
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERANCE#%s", domain)},
		},
		UpdateExpression: aws.String("SET Acknowledged = :true"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true": &types.AttributeValueMemberBOOL{Value: true},
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	}

	_, err := s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to acknowledge severance: %w", err)
	}

	s.logger().Info("severance acknowledged",
		zap.String("user_id", userID),
		zap.String("domain", domain))

	return nil
}

// AttemptReconnection records an attempt to reconnect to a severed domain
func (s *dynamoDBStorage) AttemptReconnection(ctx context.Context, userID, domain string) error {
	now := time.Now()

	attempt := ReconnectionAttempt{
		PK:          fmt.Sprintf("RECONNECTION#%s#%s", userID, domain),
		SK:          fmt.Sprintf("ATTEMPT#%d", now.Unix()),
		UserID:      userID,
		Domain:      domain,
		AttemptedAt: now,
		Success:     false, // Will be updated if successful
		Method:      "manual",
	}

	av, err := s.MarshalItem(attempt)
	if err != nil {
		return fmt.Errorf("failed to marshal reconnection attempt: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to record reconnection attempt: %w", err)
	}

	// TODO: Implement actual reconnection logic here
	// This would involve:
	// 1. Testing connectivity to the domain
	// 2. Attempting to re-establish federation
	// 3. Updating the attempt record with results

	s.logger().Info("reconnection attempt recorded",
		zap.String("user_id", userID),
		zap.String("domain", domain))

	return nil
}

// GetUserSeveredRelationships returns all severed relationships for a user
func (s *dynamoDBStorage) GetUserSeveredRelationships(ctx context.Context, userID string) ([]*storage.SeveredRelationship, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			":prefix": &types.AttributeValueMemberS{Value: "SEVERANCE#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query severed relationships: %w", err)
	}

	relationships := make([]*storage.SeveredRelationship, 0)
	for _, item := range result.Items {
		var record FederationSeveranceRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			s.logger().Warn("failed to unmarshal severance record", zap.Error(err))
			continue
		}

		relationship := &storage.SeveredRelationship{
			Domain:       record.Domain,
			SeveredAt:    record.SeveredAt,
			Acknowledged: record.Acknowledged,
			Reason:       record.Reason,
			Type:         record.Type,
		}

		relationships = append(relationships, relationship)
	}

	return relationships, nil
}

// GetAffectedRelationships returns relationships affected by domain severance
func (s *dynamoDBStorage) GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*storage.RelationshipRecord, error) {
	// Query for relationships with users from the specified domain
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("relationships-by-domain"),
		KeyConditionExpression: aws.String("FollowedDomain = :domain"),
		FilterExpression:       aws.String("FollowerUserID = :userID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":domain": &types.AttributeValueMemberS{Value: domain},
			":userID": &types.AttributeValueMemberS{Value: userID},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If GSI doesn't exist, fall back to basic scan
		s.logger().Warn("relationships-by-domain GSI not available, using fallback",
			zap.String("domain", domain),
			zap.Error(err))
		return s.fallbackGetAffectedRelationships(ctx, userID, domain)
	}

	relationships := make([]*storage.RelationshipRecord, 0)
	for _, item := range result.Items {
		var relationship storage.RelationshipRecord
		if err := s.UnmarshalItem(item, &relationship); err != nil {
			s.logger().Warn("failed to unmarshal relationship record", zap.Error(err))
			continue
		}

		relationships = append(relationships, &relationship)
	}

	return relationships, nil
}

// fallbackGetAffectedRelationships provides a fallback when GSI is not available
func (s *dynamoDBStorage) fallbackGetAffectedRelationships(ctx context.Context, userID, domain string) ([]*storage.RelationshipRecord, error) {
	// This is expensive but necessary fallback
	// In practice, you'd want to ensure the GSI exists

	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(SK, :prefix) AND FollowerUserID = :userID AND contains(FollowedUsername, :domainSuffix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix":       &types.AttributeValueMemberS{Value: "RELATIONSHIP#"},
			":userID":       &types.AttributeValueMemberS{Value: userID},
			":domainSuffix": &types.AttributeValueMemberS{Value: "@" + domain},
		},
		Limit: aws.Int32(100), // Limit to prevent expensive scans
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for affected relationships: %w", err)
	}

	relationships := make([]*storage.RelationshipRecord, 0)
	for _, item := range result.Items {
		var relationship storage.RelationshipRecord
		if err := s.UnmarshalItem(item, &relationship); err != nil {
			s.logger().Warn("failed to unmarshal relationship record in fallback", zap.Error(err))
			continue
		}

		relationships = append(relationships, &relationship)
	}

	return relationships, nil
}

// TrackFederationIssue records a federation issue for monitoring
func (s *dynamoDBStorage) TrackFederationIssue(ctx context.Context, domain, issueType string) error {
	now := time.Now()
	ttl := now.Add(90 * 24 * time.Hour).Unix() // 90 days TTL

	issue := FederationIssueRecord{
		PK:        fmt.Sprintf("FEDERATION_ISSUE#%s", domain),
		SK:        fmt.Sprintf("TIMESTAMP#%d", now.Unix()),
		Domain:    domain,
		IssueType: issueType,
		Timestamp: now,
		Severity:  s.determineSeverity(issueType),
		Resolved:  false,
		TTL:       ttl,
	}

	av, err := s.MarshalItem(issue)
	if err != nil {
		return fmt.Errorf("failed to marshal federation issue: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to track federation issue: %w", err)
	}

	s.logger().Warn("federation issue tracked",
		zap.String("domain", domain),
		zap.String("issue_type", issueType),
		zap.String("severity", issue.Severity))

	return nil
}

// Helper method to determine issue severity
func (s *dynamoDBStorage) determineSeverity(issueType string) string {
	switch issueType {
	case "blocked", "defederation":
		return "critical"
	case "unreachable", "timeout":
		return "high"
	case "error":
		return "medium"
	default:
		return "low"
	}
}

// CreateSeveranceRecord creates a new severance record (internal helper)
func (s *dynamoDBStorage) CreateSeveranceRecord(ctx context.Context, userID, domain, reason, severanceType string) error {
	now := time.Now()

	record := FederationSeveranceRecord{
		PK:           fmt.Sprintf("USER#%s", userID),
		SK:           fmt.Sprintf("SEVERANCE#%s", domain),
		UserID:       userID,
		Domain:       domain,
		SeveredAt:    now,
		Acknowledged: false,
		Reason:       reason,
		Type:         severanceType,
		GSI1PK:       fmt.Sprintf("SEVERANCE#%s", domain),
		GSI1SK:       fmt.Sprintf("USER#%s", userID),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal severance record: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create severance record: %w", err)
	}

	return nil
}

// GetDomainSeverances returns all severances for a specific domain (admin view)
func (s *dynamoDBStorage) GetDomainSeverances(ctx context.Context, domain string) ([]*storage.SeveredRelationship, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("severances-by-domain"),
		KeyConditionExpression: aws.String("GSI1PK = :domainPK"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":domainPK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERANCE#%s", domain)},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query domain severances: %w", err)
	}

	severances := make([]*storage.SeveredRelationship, 0)
	for _, item := range result.Items {
		var record FederationSeveranceRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			s.logger().Warn("failed to unmarshal severance record", zap.Error(err))
			continue
		}

		severance := &storage.SeveredRelationship{
			Domain:       record.Domain,
			SeveredAt:    record.SeveredAt,
			Acknowledged: record.Acknowledged,
			Reason:       record.Reason,
			Type:         record.Type,
		}

		severances = append(severances, severance)
	}

	return severances, nil
}
