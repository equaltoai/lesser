package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/trust"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// TrustRecord represents how trust relationships are stored in DynamoDB
type TrustRecord struct {
	PK        string                   `dynamodbav:"PK"`
	SK        string                   `dynamodbav:"SK"`
	GSI1PK    string                   `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK    string                   `dynamodbav:"GSI1SK,omitempty"`
	GSI2PK    string                   `dynamodbav:"GSI2PK,omitempty"`
	GSI2SK    string                   `dynamodbav:"GSI2SK,omitempty"`
	Type      string                   `dynamodbav:"Type"`
	Relation  *trust.TrustRelationship `dynamodbav:"Relation,omitempty"`
	Score     *trust.TrustScore        `dynamodbav:"Score,omitempty"`
	Update    *trust.TrustUpdate       `dynamodbav:"Update,omitempty"`
	TTL       int64                    `dynamodbav:"TTL,omitempty"`
	CreatedAt time.Time                `dynamodbav:"CreatedAt"`
}

// CreateTrustRelationship creates or updates a trust relationship
func (s *dynamoDBStorage) CreateTrustRelationship(ctx context.Context, relationship *trust.TrustRelationship) error {
	if relationship.ID == "" {
		relationship.ID = fmt.Sprintf("trust_%s", generateRandomString(12))
	}

	now := time.Now()
	relationship.Created = now
	relationship.Updated = now

	// Set TTL if not specified (1 year default)
	if relationship.TTL == 0 {
		relationship.TTL = now.Add(365 * 24 * time.Hour).Unix()
	}

	record := &TrustRecord{
		PK:        fmt.Sprintf("TRUST#%s#%s", relationship.TrusterID, relationship.Category),
		SK:        fmt.Sprintf("TRUSTEE#%s", relationship.TrusteeID),
		GSI1PK:    fmt.Sprintf("TRUSTED#%s#%s", relationship.TrusteeID, relationship.Category),
		GSI1SK:    fmt.Sprintf("TRUSTER#%s", relationship.TrusterID),
		GSI2PK:    fmt.Sprintf("DOMAIN#%s", getDomain(relationship.TrusteeID)),
		GSI2SK:    fmt.Sprintf("TRUST#%s#%f", relationship.Category, relationship.Score),
		Type:      "RELATIONSHIP",
		Relation:  relationship,
		TTL:       relationship.TTL,
		CreatedAt: now,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal trust relationship: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	if err != nil {
		return fmt.Errorf("failed to create trust relationship: %w", err)
	}

	common.Logger().Debug("Created trust relationship",
		zap.String("id", relationship.ID),
		zap.String("truster", relationship.TrusterID),
		zap.String("trustee", relationship.TrusteeID),
		zap.Float64("score", relationship.Score),
	)

	// Invalidate cached trust scores
	s.invalidateTrustScoreCache(ctx, relationship.TrusteeID, string(relationship.Category))

	return nil
}

// GetTrustRelationship retrieves a specific trust relationship
func (s *dynamoDBStorage) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*trust.TrustRelationship, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUST#%s#%s", trusterID, category)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUSTEE#%s", trusteeID)},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get trust relationship: %w", err)
	}

	if result.Item == nil {
		return nil, nil // No relationship exists
	}

	var record TrustRecord
	err = attributevalue.UnmarshalMap(result.Item, &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal trust relationship: %w", err)
	}

	return record.Relation, nil
}

// UpdateTrustRelationship updates an existing trust relationship
func (s *dynamoDBStorage) UpdateTrustRelationship(ctx context.Context, relationship *trust.TrustRelationship) error {
	// Just use CreateTrustRelationship as it's an upsert operation
	relationship.Updated = time.Now()
	return s.CreateTrustRelationship(ctx, relationship)
}

// DeleteTrustRelationship removes a trust relationship
func (s *dynamoDBStorage) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUST#%s#%s", trusterID, category)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUSTEE#%s", trusteeID)},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to delete trust relationship: %w", err)
	}

	common.Logger().Debug("Deleted trust relationship",
		zap.String("truster", trusterID),
		zap.String("trustee", trusteeID),
		zap.String("category", category),
	)

	// Invalidate cached trust scores
	s.invalidateTrustScoreCache(ctx, trusteeID, category)

	return nil
}

// GetTrustRelationships retrieves all trust relationships for a truster
func (s *dynamoDBStorage) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*trust.TrustRelationship, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("begins_with(PK, :pk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUST#%s#", trusterID)},
		},
		Limit: aws.Int32(int32(limit)),
	}

	if cursor != "" {
		// Parse cursor to reconstruct last evaluated key
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: cursor},
			"SK": &types.AttributeValueMemberS{Value: ""},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query trust relationships: %w", err)
	}

	relationships := make([]*trust.TrustRelationship, 0, len(result.Items))
	for _, item := range result.Items {
		var record TrustRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal trust record", zap.Error(err))
			continue
		}

		if record.Type == "RELATIONSHIP" && record.Relation != nil {
			relationships = append(relationships, record.Relation)
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if pk, ok := result.LastEvaluatedKey["PK"]; ok {
			if pkStr, ok := pk.(*types.AttributeValueMemberS); ok {
				nextCursor = pkStr.Value
			}
		}
	}

	return relationships, nextCursor, nil
}

// GetTrustedByRelationships retrieves all relationships where the actor is trusted
func (s *dynamoDBStorage) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*trust.TrustRelationship, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("begins_with(GSI1PK, :pk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUSTED#%s#", trusteeID)},
		},
		Limit: aws.Int32(int32(limit)),
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: cursor},
			"GSI1SK": &types.AttributeValueMemberS{Value: ""},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query trusted-by relationships: %w", err)
	}

	relationships := make([]*trust.TrustRelationship, 0, len(result.Items))
	for _, item := range result.Items {
		var record TrustRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal trust record", zap.Error(err))
			continue
		}

		if record.Type == "RELATIONSHIP" && record.Relation != nil {
			relationships = append(relationships, record.Relation)
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if pk, ok := result.LastEvaluatedKey["GSI1PK"]; ok {
			if pkStr, ok := pk.(*types.AttributeValueMemberS); ok {
				nextCursor = pkStr.Value
			}
		}
	}

	return relationships, nextCursor, nil
}

// GetTrustScore retrieves a cached trust score or calculates it
func (s *dynamoDBStorage) GetTrustScore(ctx context.Context, actorID, category string) (*trust.TrustScore, error) {
	// First, try to get cached score
	cacheKey := fmt.Sprintf("SCORE#%s#%s", actorID, category)

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: cacheKey},
			"SK": &types.AttributeValueMemberS{Value: "CURRENT"},
		},
	})

	if err == nil && result.Item != nil {
		var record TrustRecord
		err = attributevalue.UnmarshalMap(result.Item, &record)
		if err == nil && record.Score != nil {
			// Check if cache is still valid
			if record.Score.CacheTTL.After(time.Now()) {
				return record.Score, nil
			}
		}
	}

	// Cache miss or expired, calculate new score
	score, err := s.calculateTrustScore(ctx, actorID, category)
	if err != nil {
		return nil, err
	}

	// Cache the score
	s.UpdateTrustScore(ctx, score)

	return score, nil
}

// UpdateTrustScore updates a cached trust score
func (s *dynamoDBStorage) UpdateTrustScore(ctx context.Context, score *trust.TrustScore) error {
	score.LastCalculated = time.Now()
	score.CacheTTL = score.LastCalculated.Add(2 * time.Hour) // 2 hour cache

	record := &TrustRecord{
		PK:        fmt.Sprintf("SCORE#%s#%s", score.ActorID, score.Category),
		SK:        "CURRENT",
		Type:      "SCORE",
		Score:     score,
		CreatedAt: score.LastCalculated,
		TTL:       score.CacheTTL.Unix(),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal trust score: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	if err != nil {
		return fmt.Errorf("failed to update trust score: %w", err)
	}

	return nil
}

// RecordTrustUpdate records a trust score update event
func (s *dynamoDBStorage) RecordTrustUpdate(ctx context.Context, update *trust.TrustUpdate) error {
	update.Timestamp = time.Now()

	record := &TrustRecord{
		PK:        fmt.Sprintf("UPDATES#%s", update.ActorID),
		SK:        fmt.Sprintf("TIME#%s#%s", update.Timestamp.Format(time.RFC3339), update.EventID),
		Type:      "UPDATE",
		Update:    update,
		CreatedAt: update.Timestamp,
		TTL:       time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days retention
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal trust update: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	if err != nil {
		return fmt.Errorf("failed to record trust update: %w", err)
	}

	common.Logger().Debug("Recorded trust update",
		zap.String("actor", update.ActorID),
		zap.String("category", string(update.Category)),
		zap.Float64("delta", update.Delta),
		zap.String("reason", update.Reason),
	)

	return nil
}

// calculateTrustScore calculates the trust score for an actor using PageRank-inspired algorithm
func (s *dynamoDBStorage) calculateTrustScore(ctx context.Context, actorID, category string) (*trust.TrustScore, error) {
	score := &trust.TrustScore{
		ActorID:         actorID,
		Category:        trust.TrustCategory(category),
		Score:           0.0,
		DirectScore:     0.0,
		PropagatedScore: 0.0,
		Confidence:      0.0,
		TrusterCount:    0,
		CategoryScores:  make(map[string]float64),
	}

	// Get direct trust relationships
	relationships, _, err := s.GetTrustedByRelationships(ctx, actorID, 100, "")
	if err != nil {
		return nil, err
	}

	if len(relationships) == 0 {
		return score, nil // No trust relationships
	}

	// Calculate direct trust score
	var totalWeight float64
	for _, rel := range relationships {
		if string(rel.Category) == category || rel.Category == trust.TrustCategoryGeneral {
			weight := rel.Confidence
			score.DirectScore += rel.Score * weight
			totalWeight += weight
			score.TrusterCount++
		}
	}

	if totalWeight > 0 {
		score.DirectScore /= totalWeight
		score.Confidence = totalWeight / float64(score.TrusterCount)
	}

	// TODO: Implement trust propagation through the network
	// For now, just use direct score
	score.Score = score.DirectScore

	return score, nil
}

// invalidateTrustScoreCache invalidates cached trust scores for an actor
func (s *dynamoDBStorage) invalidateTrustScoreCache(ctx context.Context, actorID, category string) {
	// Delete cached score
	cacheKey := fmt.Sprintf("SCORE#%s#%s", actorID, category)

	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: cacheKey},
			"SK": &types.AttributeValueMemberS{Value: "CURRENT"},
		},
	})

	if err != nil {
		common.Logger().Warn("Failed to invalidate trust score cache",
			zap.String("actor", actorID),
			zap.String("category", category),
			zap.Error(err),
		)
	}
}

// getDomain extracts the domain from an actor ID
func getDomain(actorID string) string {
	// Simple extraction - in real implementation, parse the actor ID properly
	if idx := len(actorID) - 1; idx > 0 {
		for i := idx; i >= 0; i-- {
			if actorID[i] == '@' {
				return actorID[i+1:]
			}
		}
	}
	return "local"
}
