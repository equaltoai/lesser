package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// CreateUser creates a new user in DynamoDB
func (s *dynamoDBStorage) CreateUser(ctx context.Context, user *storage.User) error {
	if user.Username == "" || user.Email == "" || user.PasswordHash == "" {
		return errors.New("username, email, and password_hash are required")
	}

	// Set timestamps
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	// Default role if not set
	if user.Role == "" {
		user.Role = "user"
	}

	// Marshal user data
	av, err := attributevalue.MarshalMap(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	// Add keys
	av["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", user.Username)}
	av["SK"] = &types.AttributeValueMemberS{Value: "METADATA"}
	av["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("EMAIL#%s", user.Email)}
	av["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("USERNAME#%s", user.Username)}

	// Put item with condition that it doesn't already exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			return fmt.Errorf("user with username %s already exists", user.Username)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by username
func (s *dynamoDBStorage) GetUser(ctx context.Context, username string) (*storage.User, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("user not found: %s", username)
	}

	var user storage.User
	if err := attributevalue.UnmarshalMap(result.Item, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// GetUserByEmail retrieves a user by email address
func (s *dynamoDBStorage) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	// Query GSI2 for email
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :email"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":email": &types.AttributeValueMemberS{Value: fmt.Sprintf("EMAIL#%s", email)},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("user not found with email: %s", email)
	}

	var user storage.User
	if err := attributevalue.UnmarshalMap(result.Items[0], &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// UpdateUser updates an existing user
func (s *dynamoDBStorage) UpdateUser(ctx context.Context, username string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return errors.New("no updates provided")
	}

	// Build update expression
	updateExpr := "SET updated_at = :updated_at"
	exprAttrValues := map[string]types.AttributeValue{
		":updated_at": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	for key, value := range updates {
		switch key {
		case "email", "password_hash", "approved", "suspended", "role", "locale":
			updateExpr += fmt.Sprintf(", %s = :%s", key, key)
			av, err := attributevalue.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal %s: %w", key, err)
			}
			exprAttrValues[":"+key] = av
		}
	}

	// Update item
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprAttrValues,
		ConditionExpression:       aws.String("attribute_exists(PK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			return fmt.Errorf("user not found: %s", username)
		}
		return fmt.Errorf("failed to update user: %w", err)
	}

	// If email was updated, we need to update GSI2
	if newEmail, ok := updates["email"].(string); ok {
		// Get current user to find old email
		user, err := s.GetUser(ctx, username)
		if err != nil {
			return fmt.Errorf("failed to get user for email update: %w", err)
		}

		// Delete old GSI2 entry
		_, _ = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EMAIL#%s", user.Email)},
				"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USERNAME#%s", username)},
			},
		})

		// Create new GSI2 entry
		_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(s.tableName),
			Item: map[string]types.AttributeValue{
				"PK":     &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
				"SK":     &types.AttributeValueMemberS{Value: "METADATA"},
				"GSI2PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EMAIL#%s", newEmail)},
				"GSI2SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USERNAME#%s", username)},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update email index: %w", err)
		}
	}

	return nil
}

// DeleteUser deletes a user and all associated data
func (s *dynamoDBStorage) DeleteUser(ctx context.Context, username string) error {
	// Get user to find email for GSI2 deletion
	user, err := s.GetUser(ctx, username)
	if err != nil {
		return err
	}

	// Delete main user record
	_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Delete email index entry
	_, _ = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EMAIL#%s", user.Email)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USERNAME#%s", username)},
		},
	})

	// TODO: Delete associated actor, activities, objects, etc.

	return nil
}

// ListUsers retrieves a paginated list of users
func (s *dynamoDBStorage) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Build query input
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USERS"},
		},
		Limit: aws.Int32(limit),
	}

	// Add cursor if provided
	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: "USERS"},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list users: %w", err)
	}

	users := make([]*storage.User, 0, len(result.Items))
	for _, item := range result.Items {
		var user storage.User
		if err := attributevalue.UnmarshalMap(item, &user); err != nil {
			continue
		}
		users = append(users, &user)
	}

	// Extract next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if gsi1sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if s, ok := gsi1sk.(*types.AttributeValueMemberS); ok {
				nextCursor = s.Value
			}
		}
	}

	return users, nextCursor, nil
}
