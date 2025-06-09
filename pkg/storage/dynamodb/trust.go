package dynamodb

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/trust"
	"github.com/aws/aws-sdk-go-v2/aws"
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
	err = s.UnmarshalItem(result.Item, &record)
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
	if limit > math.MaxInt32 {
		limit = math.MaxInt32
	}
	// We need to scan instead of query since we want all categories
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("begins_with(PK, :pk) AND #type = :type"),
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":   &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUST#%s#", trusterID)},
			":type": &types.AttributeValueMemberS{Value: "RELATIONSHIP"},
		},
		Limit: safeInt32(limit),
	}

	if cursor != "" {
		// Parse cursor to reconstruct last evaluated key
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: cursor},
			"SK": &types.AttributeValueMemberS{Value: ""},
		}
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan trust relationships: %w", err)
	}

	relationships := make([]*trust.TrustRelationship, 0, len(result.Items))
	for _, item := range result.Items {
		var record TrustRecord
		err = s.UnmarshalItem(item, &record)
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
	if limit > math.MaxInt32 {
		limit = math.MaxInt32
	}
	// Need to scan with filter instead of query on GSI
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("begins_with(GSI1PK, :pk) AND #type = :type"),
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":   &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUSTED#%s#", trusteeID)},
			":type": &types.AttributeValueMemberS{Value: "RELATIONSHIP"},
		},
		Limit: safeInt32(limit),
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: cursor},
			"SK": &types.AttributeValueMemberS{Value: ""},
		}
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan trusted-by relationships: %w", err)
	}

	relationships := make([]*trust.TrustRelationship, 0, len(result.Items))
	for _, item := range result.Items {
		var record TrustRecord
		err = s.UnmarshalItem(item, &record)
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
		err = s.UnmarshalItem(result.Item, &record)
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
	if err := s.UpdateTrustScore(ctx, score); err != nil {
		// Log the error but return the calculated score as it's still valid
		s.log.Warn("failed to cache updated trust score",
			zap.String("actorID", actorID),
			zap.String("category", category),
			zap.Error(err))
	}

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
	trusterScores := make(map[string]float64) // Store truster scores for propagation

	for _, rel := range relationships {
		if string(rel.Category) == category || rel.Category == trust.TrustCategoryGeneral {
			weight := rel.Confidence
			score.DirectScore += rel.Score * weight
			totalWeight += weight
			score.TrusterCount++
			trusterScores[rel.TrusterID] = rel.Score * weight
		}
	}

	if totalWeight > 0 {
		score.DirectScore /= totalWeight
		score.Confidence = totalWeight / float64(score.TrusterCount)
	}

	// Implement trust propagation through the network
	// PageRank-style algorithm with dampening factor
	const (
		dampingFactor   = 0.85 // How much trust propagates through the network
		maxDepth        = 3    // Maximum depth of trust propagation
		minTrustScore   = 0.1  // Minimum trust score to propagate
		propagationRate = 0.5  // How much of the trust score propagates to next level
	)

	// Track visited actors to avoid cycles
	visited := make(map[string]bool)
	visited[actorID] = true

	// Propagated trust accumulator
	propagatedTrust := 0.0
	propagatedWeight := 0.0

	// BFS-style propagation through trust network
	type propagationNode struct {
		actorID   string
		trustPath float64 // Accumulated trust along the path
		depth     int
	}

	queue := make([]propagationNode, 0)

	// Initialize queue with direct trusters
	for trusterID, trustValue := range trusterScores {
		if trustValue >= minTrustScore {
			queue = append(queue, propagationNode{
				actorID:   trusterID,
				trustPath: trustValue,
				depth:     1,
			})
		}
	}

	// Process propagation queue
	for len(queue) > 0 && len(visited) < 100 { // Limit total actors examined
		node := queue[0]
		queue = queue[1:]

		if visited[node.actorID] || node.depth > maxDepth {
			continue
		}
		visited[node.actorID] = true

		// Get trust score of the current node
		nodeScore, err := s.GetTrustScore(ctx, node.actorID, category)
		if err != nil {
			common.Logger().Warn("Failed to get trust score for propagation",
				zap.String("actor", node.actorID),
				zap.Error(err))
			continue
		}

		// Skip if the node has low trust
		if nodeScore.Score < minTrustScore {
			continue
		}

		// Calculate propagated trust contribution
		// Trust diminishes with each hop (propagationRate) and is weighted by the path trust
		propagationFactor := math.Pow(propagationRate, float64(node.depth-1))
		contribution := node.trustPath * nodeScore.Score * propagationFactor * dampingFactor

		propagatedTrust += contribution
		propagatedWeight += node.trustPath * propagationFactor

		// Get this node's trust relationships for further propagation
		if node.depth < maxDepth {
			nodeRelationships, _, err := s.GetTrustedByRelationships(ctx, node.actorID, 50, "")
			if err == nil {
				for _, rel := range nodeRelationships {
					if !visited[rel.TrusterID] && (string(rel.Category) == category || rel.Category == trust.TrustCategoryGeneral) {
						queue = append(queue, propagationNode{
							actorID:   rel.TrusterID,
							trustPath: contribution * rel.Score * rel.Confidence,
							depth:     node.depth + 1,
						})
					}
				}
			}
		}
	}

	// Normalize propagated score
	if propagatedWeight > 0 {
		score.PropagatedScore = propagatedTrust / propagatedWeight
	}

	// Combine direct and propagated scores
	// Weight direct trust more heavily than propagated trust
	const directWeight = 0.7
	const propagatedWeightFactor = 0.3

	if score.DirectScore > 0 && score.PropagatedScore > 0 {
		score.Score = (score.DirectScore * directWeight) + (score.PropagatedScore * propagatedWeightFactor)
	} else if score.DirectScore > 0 {
		score.Score = score.DirectScore
	} else {
		score.Score = score.PropagatedScore
	}

	// Apply bounds
	if score.Score > 1.0 {
		score.Score = 1.0
	} else if score.Score < 0.0 {
		score.Score = 0.0
	}

	common.Logger().Debug("Calculated trust score with propagation",
		zap.String("actor", actorID),
		zap.String("category", category),
		zap.Float64("direct_score", score.DirectScore),
		zap.Float64("propagated_score", score.PropagatedScore),
		zap.Float64("final_score", score.Score),
		zap.Int("visited_actors", len(visited)))

	return score, nil
}

// GetAllTrustRelationships retrieves all trust relationships for admin visualization
func (s *dynamoDBStorage) GetAllTrustRelationships(ctx context.Context, limit int) ([]*trust.TrustRelationship, error) {
	relationships := make([]*trust.TrustRelationship, 0)

	// Use a scan to get all trust relationships
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("#type = :type"),
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "RELATIONSHIP"},
		},
	}

	// Use paginator to handle large result sets
	paginator := dynamodb.NewScanPaginator(s.client, input)

	count := 0
	for paginator.HasMorePages() && count < limit {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trust relationships: %w", err)
		}

		for _, item := range page.Items {
			if count >= limit {
				break
			}

			var record TrustRecord
			err = s.UnmarshalItem(item, &record)
			if err != nil {
				common.Logger().Error("Failed to unmarshal trust record", zap.Error(err))
				continue
			}

			if record.Type == "RELATIONSHIP" && record.Relation != nil {
				relationships = append(relationships, record.Relation)
				count++
			}
		}
	}

	common.Logger().Debug("Retrieved all trust relationships",
		zap.Int("count", len(relationships)),
		zap.Int("limit", limit),
	)

	return relationships, nil
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
