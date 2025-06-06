package trust

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Service provides methods for managing trust relationships
type Service struct {
	dynamoClient *dynamodb.Client
	tableName    string
}

// NewService creates a new trust service
func NewService(dynamoClient *dynamodb.Client) *Service {
	return &Service{
		dynamoClient: dynamoClient,
		tableName:    "lesser-main",
	}
}

// GetTrustScore retrieves the trust score between two actors
func (s *Service) GetTrustScore(ctx context.Context, fromActor, toActor string) (*TrustScore, error) {
	// Query DynamoDB for trust relationship
	result, err := s.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUST#%s", fromActor)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUST#%s", toActor)},
		},
	})

	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		// No trust relationship exists, return default
		return &TrustScore{
			ActorID:  toActor,
			Category: TrustCategoryGeneral,
			Score:    0.5, // Default neutral trust
		}, nil
	}

	// Parse the trust score from the item
	// This is simplified - real implementation would unmarshal properly
	score := &TrustScore{
		ActorID:  toActor,
		Category: TrustCategoryGeneral,
		Score:    0.5, // Default for now
	}

	// Check if there's a Score attribute
	if scoreAttr, ok := result.Item["Score"]; ok {
		if _, ok := scoreAttr.(*types.AttributeValueMemberN); ok {
			// Parse score - simplified
			score.Score = 0.5 // Would parse scoreNum.Value properly in real implementation
		}
	}

	return score, nil
}
