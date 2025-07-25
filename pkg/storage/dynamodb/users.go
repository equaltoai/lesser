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
	"go.uber.org/zap"
)

// CreateUser creates a new user in DynamoDB
func (s *dynamoDBStorage) CreateUser(ctx context.Context, user *storage.User) error {
	if user.Username == "" {
		return errors.New("username is required")
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
	av, err := s.MarshalItem(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	// Add keys
	av["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", user.Username)}
	av["SK"] = &types.AttributeValueMemberS{Value: "METADATA"}

	// Only add email index if email is provided
	if user.Email != "" {
		av["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("EMAIL#%s", user.Email)}
		av["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("USERNAME#%s", user.Username)}
	}

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
	if err := s.UnmarshalItem(result.Item, &user); err != nil {
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
	if err := s.UnmarshalItem(result.Items[0], &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// UpdateUser updates an existing user
func (s *dynamoDBStorage) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
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

	// Delete associated actor and all its data first
	// This will cascade delete all activities, objects, follows, etc.
	// The actor might not exist for some users, so we ignore any errors
	_ = s.DeleteActor(ctx, username)

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

	// Delete user-specific data that's not handled by DeleteActor
	// This includes user preferences, OAuth tokens, etc.
	userDataQuery := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		},
	}

	if result, err := s.client.Query(ctx, userDataQuery); err == nil {
		for _, item := range result.Items {
			if _, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: aws.String(s.tableName),
				Key: map[string]types.AttributeValue{
					"PK": item["PK"],
					"SK": item["SK"],
				},
			}); err != nil {
				s.log.Warn("failed to delete user data item during cascade delete",
					zap.String("username", username),
					zap.Any("item_pk", item["PK"]),
					zap.Any("item_sk", item["SK"]),
					zap.Error(err))
			}
		}
	}

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
		if err := s.UnmarshalItem(item, &user); err != nil {
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

// GetActiveUserCount returns the number of users active in the last N days
func (s *dynamoDBStorage) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	// Calculate cutoff date
	cutoff := time.Now().AddDate(0, 0, -days)

	// Query GSI5 for recent activity
	// GSI5 is partitioned by date (ACTIVE#YYYY-MM-DD) for efficient querying
	var totalCount int64 = 0

	// Query each day from cutoff to today
	for d := cutoff; d.Before(time.Now()) || d.Equal(time.Now()); d = d.AddDate(0, 0, 1) {
		dateKey := fmt.Sprintf("ACTIVE#%s", d.Format("2006-01-02"))

		input := &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			IndexName:              aws.String("GSI5"),
			KeyConditionExpression: aws.String("GSI5PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: dateKey},
			},
			Select: types.SelectCount,
		}

		// Paginate through all results for this day
		for {
			result, err := s.client.Query(ctx, input)
			if err != nil {
				// Skip this day if there's an error
				break
			}

			totalCount += int64(result.Count)

			// Check if there are more pages
			if result.LastEvaluatedKey == nil {
				break
			}

			// Set start key for next page
			input.ExclusiveStartKey = result.LastEvaluatedKey
		}
	}

	// Note: This counts activities, not unique users
	// To get unique users, we would need to:
	// 1. Use a Set to track unique usernames
	// 2. Or use a more complex query with grouping
	// For now, this gives us a reasonable approximation of activity level

	return totalCount, nil
}

// GetUserByProviderID gets a user by their OAuth provider ID
func (d *dynamoDBStorage) GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error) {
	// Query using the GSI for provider accounts
	input := &dynamodb.QueryInput{
		TableName:              aws.String(d.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("PROVIDER#%s", provider)},
			":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ID#%s", providerID)},
		},
	}

	result, err := d.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query user by provider ID: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	// Extract username from the result
	var linkRecord struct {
		Username string `dynamodbav:"Username"`
	}
	err = attributevalue.UnmarshalMap(result.Items[0], &linkRecord)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal provider link: %w", err)
	}

	// Get the actual user
	return d.GetUser(ctx, linkRecord.Username)
}

// LinkProviderAccount links an OAuth provider account to a user
func (d *dynamoDBStorage) LinkProviderAccount(ctx context.Context, username, provider, providerID string) error {
	item := map[string]any{
		"PK":         fmt.Sprintf("USER#%s", username),
		"SK":         fmt.Sprintf("PROVIDER#%s", provider),
		"GSI1PK":     fmt.Sprintf("PROVIDER#%s", provider),
		"GSI1SK":     fmt.Sprintf("ID#%s", providerID),
		"Username":   username,
		"Provider":   provider,
		"ProviderID": providerID,
		"LinkedAt":   time.Now(),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("failed to marshal provider link: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      av,
	}

	_, err = d.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to link provider account: %w", err)
	}

	return nil
}

// UnlinkProviderAccount unlinks an OAuth provider account from a user
func (d *dynamoDBStorage) UnlinkProviderAccount(ctx context.Context, username, provider string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PROVIDER#%s", provider)},
		},
	}

	_, err := d.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to unlink provider account: %w", err)
	}

	return nil
}

// GetLinkedProviders gets all linked OAuth providers for a user
func (d *dynamoDBStorage) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(d.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "PROVIDER#"},
		},
	}

	result, err := d.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query linked providers: %w", err)
	}

	providers := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		var linkRecord struct {
			Provider string `dynamodbav:"Provider"`
		}
		err = attributevalue.UnmarshalMap(item, &linkRecord)
		if err != nil {
			continue // Skip invalid records
		}
		providers = append(providers, linkRecord.Provider)
	}

	return providers, nil
}
