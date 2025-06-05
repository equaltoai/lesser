package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// InstanceData represents dynamic instance data stored in DynamoDB
type InstanceData struct {
	PK                  string                 `dynamodbav:"PK"`
	SK                  string                 `dynamodbav:"SK"`
	Rules               []storage.InstanceRule `dynamodbav:"Rules,omitempty"`
	ExtendedDescription string                 `dynamodbav:"ExtendedDescription,omitempty"`
	UpdatedAt           time.Time              `dynamodbav:"UpdatedAt"`
}

// GetInstanceRules retrieves the instance rules
func (s *dynamoDBStorage) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "INSTANCE#CONFIG"},
			"SK": &types.AttributeValueMemberS{Value: "RULES"},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get instance rules: %w", err)
	}

	if result.Item == nil {
		// Return empty rules if not set
		return []storage.InstanceRule{}, nil
	}

	var data InstanceData
	if err := attributevalue.UnmarshalMap(result.Item, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instance rules: %w", err)
	}

	return data.Rules, nil
}

// SetInstanceRules updates the instance rules
func (s *dynamoDBStorage) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	// Assign IDs if not present
	for i := range rules {
		if rules[i].ID == "" {
			rules[i].ID = fmt.Sprintf("%d", i+1)
		}
	}

	data := InstanceData{
		PK:        "INSTANCE#CONFIG",
		SK:        "RULES",
		Rules:     rules,
		UpdatedAt: time.Now(),
	}

	item, err := attributevalue.MarshalMap(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instance rules: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("failed to save instance rules: %w", err)
	}

	return nil
}

// GetExtendedDescription retrieves the instance extended description
func (s *dynamoDBStorage) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "INSTANCE#CONFIG"},
			"SK": &types.AttributeValueMemberS{Value: "EXTENDED_DESC"},
		},
	})

	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get extended description: %w", err)
	}

	if result.Item == nil {
		// Return default if not set
		return "<p>Welcome to Lesser ActivityPub Server</p>", time.Now(), nil
	}

	var data InstanceData
	if err := attributevalue.UnmarshalMap(result.Item, &data); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to unmarshal extended description: %w", err)
	}

	return data.ExtendedDescription, data.UpdatedAt, nil
}

// SetExtendedDescription updates the instance extended description
func (s *dynamoDBStorage) SetExtendedDescription(ctx context.Context, description string) error {
	data := InstanceData{
		PK:                  "INSTANCE#CONFIG",
		SK:                  "EXTENDED_DESC",
		ExtendedDescription: description,
		UpdatedAt:           time.Now(),
	}

	item, err := attributevalue.MarshalMap(data)
	if err != nil {
		return fmt.Errorf("failed to marshal extended description: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("failed to save extended description: %w", err)
	}

	return nil
}
