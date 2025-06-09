package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// SeveranceReason represents why a federation relationship was severed
type SeveranceReason string

const (
	SeveranceReasonBlocked     SeveranceReason = "blocked"
	SeveranceReasonUnavailable SeveranceReason = "unavailable"
	SeveranceReasonSuspended   SeveranceReason = "suspended"
	SeveranceReasonDefederated SeveranceReason = "defederated"
	SeveranceReasonLimited     SeveranceReason = "limited"
)

// SeveredRelationship represents a broken federation relationship
type SeveredRelationship struct {
	ID              string           `dynamodbav:"ID"`
	LocalInstance   string           `dynamodbav:"LocalInstance"`
	RemoteInstance  string           `dynamodbav:"RemoteInstance"`
	Reason          SeveranceReason  `dynamodbav:"Reason"`
	AffectedFollows []AffectedFollow `dynamodbav:"AffectedFollows"`
	Timestamp       time.Time        `dynamodbav:"Timestamp"`
	Reversible      bool             `dynamodbav:"Reversible"`
	Details         string           `dynamodbav:"Details,omitempty"`
	EstimatedImpact int              `dynamodbav:"EstimatedImpact"` // Number of affected relationships

	// DynamoDB keys
	PK string `dynamodbav:"PK"` // SEVERED#localInstance#remoteInstance
	SK string `dynamodbav:"SK"` // TIMESTAMP#timestamp
}

// AffectedFollow represents a follow relationship affected by severance
type AffectedFollow struct {
	LocalUser    string    `dynamodbav:"LocalUser"`
	RemoteUser   string    `dynamodbav:"RemoteUser"`
	Direction    string    `dynamodbav:"Direction"` // "following", "follower", "mutual"
	LastActivity time.Time `dynamodbav:"LastActivity"`
}

// CreateSeveredRelationship records a new severed federation relationship
func (s *dynamoDBStorage) CreateSeveredRelationship(ctx context.Context, rel *SeveredRelationship) error {
	// Generate ID if not provided
	if rel.ID == "" {
		rel.ID = fmt.Sprintf("%s-%s-%d", rel.LocalInstance, rel.RemoteInstance, time.Now().Unix())
	}

	// Set timestamp if not provided
	if rel.Timestamp.IsZero() {
		rel.Timestamp = time.Now()
	}

	// Set DynamoDB keys
	rel.PK = fmt.Sprintf("SEVERED#%s#%s", rel.LocalInstance, rel.RemoteInstance)
	rel.SK = fmt.Sprintf("TIMESTAMP#%d", rel.Timestamp.Unix())

	// Marshal the relationship
	av, err := s.MarshalItem(rel)
	if err != nil {
		return fmt.Errorf("failed to marshal severed relationship: %w", err)
	}

	// Store the relationship
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to create severed relationship: %w", err)
	}

	// Log the severance
	s.logger().Info("severed relationship created",
		zap.String("id", rel.ID),
		zap.String("local", rel.LocalInstance),
		zap.String("remote", rel.RemoteInstance),
		zap.String("reason", string(rel.Reason)),
		zap.Int("impact", rel.EstimatedImpact))

	return nil
}

// GetSeveredRelationships retrieves severed relationships for a local instance
func (s *dynamoDBStorage) GetSeveredRelationships(ctx context.Context, localInstance string, limit int, cursor string) ([]*SeveredRelationship, string, error) {
	var exclusiveStartKey map[string]types.AttributeValue
	if cursor != "" {
		exclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERED#%s", localInstance)},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	// Query severed relationships
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("begins_with(PK, :pk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERED#%s#", localInstance)},
		},
		Limit:             safeInt32(limit),
		ExclusiveStartKey: exclusiveStartKey,
		ScanIndexForward:  aws.Bool(false), // Most recent first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query severed relationships: %w", err)
	}

	// Unmarshal results
	relationships := make([]*SeveredRelationship, 0, len(result.Items))
	for _, item := range result.Items {
		var rel SeveredRelationship
		if err := s.UnmarshalItem(item, &rel); err != nil {
			s.logger().Warn("failed to unmarshal severed relationship", zap.Error(err))
			continue
		}
		relationships = append(relationships, &rel)
	}

	// Prepare next cursor
	nextCursor := ""
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return relationships, nextCursor, nil
}

// GetSeveredRelationship retrieves a specific severed relationship
func (s *dynamoDBStorage) GetSeveredRelationship(ctx context.Context, localInstance, remoteInstance string) (*SeveredRelationship, error) {
	// Query for the most recent severance between these instances
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERED#%s#%s", localInstance, remoteInstance)},
		},
		Limit:            aws.Int32(1),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query severed relationship: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no severed relationship found between %s and %s", localInstance, remoteInstance)
	}

	var rel SeveredRelationship
	if err := s.UnmarshalItem(result.Items[0], &rel); err != nil {
		return nil, fmt.Errorf("failed to unmarshal severed relationship: %w", err)
	}

	return &rel, nil
}

// UpdateSeveredRelationship updates an existing severed relationship
func (s *dynamoDBStorage) UpdateSeveredRelationship(ctx context.Context, rel *SeveredRelationship) error {
	// Update the relationship
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: rel.PK},
			"SK": &types.AttributeValueMemberS{Value: rel.SK},
		},
		UpdateExpression: aws.String("SET #reason = :reason, Reversible = :reversible, Details = :details, EstimatedImpact = :impact"),
		ExpressionAttributeNames: map[string]string{
			"#reason": "Reason", // 'Reason' might be a reserved word
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":reason":     &types.AttributeValueMemberS{Value: string(rel.Reason)},
			":reversible": &types.AttributeValueMemberBOOL{Value: rel.Reversible},
			":details":    &types.AttributeValueMemberS{Value: rel.Details},
			":impact":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", rel.EstimatedImpact)},
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	}

	_, err := s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to update severed relationship: %w", err)
	}

	return nil
}

// GetAffectedFollows retrieves follow relationships affected by a severance
func (s *dynamoDBStorage) GetAffectedFollows(ctx context.Context, localInstance, remoteInstance string) ([]AffectedFollow, error) {
	// Get the severed relationship
	rel, err := s.GetSeveredRelationship(ctx, localInstance, remoteInstance)
	if err != nil {
		return nil, err
	}

	return rel.AffectedFollows, nil
}

// RecordAffectedFollow adds an affected follow to a severed relationship
func (s *dynamoDBStorage) RecordAffectedFollow(ctx context.Context, localInstance, remoteInstance string, follow AffectedFollow) error {
	// Get the current relationship
	rel, err := s.GetSeveredRelationship(ctx, localInstance, remoteInstance)
	if err != nil {
		return fmt.Errorf("failed to get severed relationship: %w", err)
	}

	// Add the affected follow
	rel.AffectedFollows = append(rel.AffectedFollows, follow)
	rel.EstimatedImpact = len(rel.AffectedFollows)

	// Update the relationship
	av, err := s.MarshalItem(rel)
	if err != nil {
		return fmt.Errorf("failed to marshal updated relationship: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to update severed relationship: %w", err)
	}

	return nil
}

// ReverseSeverance marks a severed relationship as restored
func (s *dynamoDBStorage) ReverseSeverance(ctx context.Context, localInstance, remoteInstance string) error {
	// Get the current relationship
	rel, err := s.GetSeveredRelationship(ctx, localInstance, remoteInstance)
	if err != nil {
		return fmt.Errorf("failed to get severed relationship: %w", err)
	}

	if !rel.Reversible {
		return fmt.Errorf("severed relationship is not reversible")
	}

	// Create a new "restored" entry
	restored := &SeveredRelationship{
		ID:              fmt.Sprintf("%s-restored-%d", rel.ID, time.Now().Unix()),
		LocalInstance:   localInstance,
		RemoteInstance:  remoteInstance,
		Reason:          "restored",
		Timestamp:       time.Now(),
		Reversible:      false,
		Details:         fmt.Sprintf("Relationship restored after previous severance: %s", rel.Reason),
		EstimatedImpact: 0,
		PK:              rel.PK,
		SK:              fmt.Sprintf("TIMESTAMP#%d", time.Now().Unix()),
	}

	av, err := s.MarshalItem(restored)
	if err != nil {
		return fmt.Errorf("failed to marshal restored relationship: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to create restored relationship entry: %w", err)
	}

	s.logger().Info("severed relationship restored",
		zap.String("local", localInstance),
		zap.String("remote", remoteInstance))

	return nil
}

// GetSeveranceHistory retrieves the history of severances between two instances
func (s *dynamoDBStorage) GetSeveranceHistory(ctx context.Context, localInstance, remoteInstance string, limit int) ([]*SeveredRelationship, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERED#%s#%s", localInstance, remoteInstance)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query severance history: %w", err)
	}

	history := make([]*SeveredRelationship, 0, len(result.Items))
	for _, item := range result.Items {
		var rel SeveredRelationship
		if err := s.UnmarshalItem(item, &rel); err != nil {
			s.logger().Warn("failed to unmarshal severance history item", zap.Error(err))
			continue
		}
		history = append(history, &rel)
	}

	return history, nil
}
