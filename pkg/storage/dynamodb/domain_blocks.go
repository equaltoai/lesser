package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// AddDomainBlock adds a domain to the user's block list
func (s *dynamoDBStorage) AddDomainBlock(ctx context.Context, username, domain string) error {
	item := storage.DomainBlock{
		Username:  username,
		Domain:    domain,
		CreatedAt: time.Now(),
	}

	av, err := s.MarshalItem(map[string]interface{}{
		"PK":        s.userPK(username),
		"SK":        fmt.Sprintf("DOMAIN_BLOCK#%s", domain),
		"Username":  item.Username,
		"Domain":    item.Domain,
		"CreatedAt": item.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal domain block: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if err != nil {
		// Check if it's a duplicate
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			// Already blocked, not an error
			return nil
		}
		return fmt.Errorf("failed to add domain block: %w", err)
	}

	return nil
}

// RemoveDomainBlock removes a domain from the user's block list
func (s *dynamoDBStorage) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: s.userPK(username)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
	}

	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		return fmt.Errorf("failed to remove domain block: %w", err)
	}

	return nil
}

// GetUserDomainBlocks retrieves all domains blocked by a user
func (s *dynamoDBStorage) GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: s.userPK(username)},
			":prefix": &types.AttributeValueMemberS{Value: "DOMAIN_BLOCK#"},
		},
		Limit: aws.Int32(int32(limit)),
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(username)},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query domain blocks: %w", err)
	}

	domains := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		// Extract domain from SK
		if sk, ok := item["SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok && len(skStr.Value) > 13 {
				// Remove "DOMAIN_BLOCK#" prefix
				domain := skStr.Value[13:]
				domains = append(domains, domain)
			}
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return domains, nextCursor, nil
}

// IsBlockedDomain checks if a domain is blocked by a user
func (s *dynamoDBStorage) IsBlockedDomain(ctx context.Context, username, domain string) (bool, error) {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: s.userPK(username)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
	}

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check domain block: %w", err)
	}

	return result.Item != nil, nil
}
