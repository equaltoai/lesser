package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CreateList creates a new list for a user
func (s *dynamoDBStorage) CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error) {
	// Validate replies policy
	if repliesPolicy == "" {
		repliesPolicy = "list" // default
	}
	if repliesPolicy != "followed" && repliesPolicy != "list" && repliesPolicy != "none" {
		return nil, fmt.Errorf("invalid replies policy: %s", repliesPolicy)
	}

	list := &storage.List{
		ID:            uuid.New().String(),
		Username:      username,
		Title:         title,
		RepliesPolicy: repliesPolicy,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Marshal the list
	av, err := attributevalue.MarshalMap(list)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal list: %w", err)
	}

	// Add keys
	av["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST#%s", list.ID)}
	av["SK"] = &types.AttributeValueMemberS{Value: "METADATA"}

	// Create the list metadata
	putItem := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	if _, err := s.client.PutItem(ctx, putItem); err != nil {
		return nil, fmt.Errorf("failed to create list: %w", err)
	}

	// Also create an index entry for user's lists
	userListItem := map[string]types.AttributeValue{
		"PK":            &types.AttributeValueMemberS{Value: fmt.Sprintf("USER_LISTS#%s", username)},
		"SK":            &types.AttributeValueMemberS{Value: list.ID},
		"Title":         &types.AttributeValueMemberS{Value: title},
		"RepliesPolicy": &types.AttributeValueMemberS{Value: repliesPolicy},
		"CreatedAt":     &types.AttributeValueMemberS{Value: list.CreatedAt.Format(time.RFC3339)},
	}

	putUserListItem := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      userListItem,
	}

	if _, err := s.client.PutItem(ctx, putUserListItem); err != nil {
		// Try to clean up the list metadata
		s.DeleteList(ctx, list.ID)
		return nil, fmt.Errorf("failed to create user list index: %w", err)
	}

	return list, nil
}

// GetList retrieves a list by ID
func (s *dynamoDBStorage) GetList(ctx context.Context, listID string) (*storage.List, error) {
	getItem := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST#%s", listID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	}

	result, err := s.client.GetItem(ctx, getItem)
	if err != nil {
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("list not found: %s", listID)
	}

	var list storage.List
	if err := attributevalue.UnmarshalMap(result.Item, &list); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list: %w", err)
	}

	return &list, nil
}

// GetListsForUser retrieves all lists owned by a user
func (s *dynamoDBStorage) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	log := common.WithContext(ctx)

	// First get list IDs from the user index
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER_LISTS#%s", username)},
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("failed to query user lists: %w", err)
	}

	lists := make([]*storage.List, 0, len(result.Items))
	for _, item := range result.Items {
		listID, ok := item["SK"].(*types.AttributeValueMemberS)
		if !ok {
			continue
		}

		// Get the full list details
		list, err := s.GetList(ctx, listID.Value)
		if err != nil {
			log.Warn("failed to get list details",
				zap.String("list_id", listID.Value),
				zap.Error(err))
			continue
		}

		lists = append(lists, list)
	}

	return lists, nil
}

// UpdateList updates a list's properties
func (s *dynamoDBStorage) UpdateList(ctx context.Context, listID string, updates map[string]interface{}) error {
	log := common.WithContext(ctx)

	// Build update expression
	updateExpr := "SET UpdatedAt = :updated"
	exprAttrValues := map[string]types.AttributeValue{
		":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	if title, ok := updates["title"].(string); ok {
		updateExpr += ", Title = :title"
		exprAttrValues[":title"] = &types.AttributeValueMemberS{Value: title}
	}

	if repliesPolicy, ok := updates["replies_policy"].(string); ok {
		// Validate replies policy
		if repliesPolicy != "followed" && repliesPolicy != "list" && repliesPolicy != "none" {
			return fmt.Errorf("invalid replies policy: %s", repliesPolicy)
		}
		updateExpr += ", RepliesPolicy = :replies_policy"
		exprAttrValues[":replies_policy"] = &types.AttributeValueMemberS{Value: repliesPolicy}
	}

	updateItem := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST#%s", listID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprAttrValues,
		ConditionExpression:       aws.String("attribute_exists(PK)"), // Ensure list exists
	}

	if _, err := s.client.UpdateItem(ctx, updateItem); err != nil {
		return fmt.Errorf("failed to update list: %w", err)
	}

	// Also update the user index if title changed
	if title, ok := updates["title"].(string); ok {
		// Get the list to find the owner
		list, err := s.GetList(ctx, listID)
		if err != nil {
			return err
		}

		updateUserIndex := &dynamodb.UpdateItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER_LISTS#%s", list.Username)},
				"SK": &types.AttributeValueMemberS{Value: listID},
			},
			UpdateExpression: aws.String("SET Title = :title"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":title": &types.AttributeValueMemberS{Value: title},
			},
		}

		if _, err := s.client.UpdateItem(ctx, updateUserIndex); err != nil {
			log.Warn("failed to update user list index",
				zap.String("list_id", listID),
				zap.Error(err))
		}
	}

	return nil
}

// DeleteList deletes a list and all its memberships
func (s *dynamoDBStorage) DeleteList(ctx context.Context, listID string) error {
	log := common.WithContext(ctx)

	// Get the list first to find the owner
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Delete list metadata
	deleteItem := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST#%s", listID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	}

	if _, err := s.client.DeleteItem(ctx, deleteItem); err != nil {
		return fmt.Errorf("failed to delete list: %w", err)
	}

	// Delete from user index
	deleteUserIndex := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER_LISTS#%s", list.Username)},
			"SK": &types.AttributeValueMemberS{Value: listID},
		},
	}

	if _, err := s.client.DeleteItem(ctx, deleteUserIndex); err != nil {
		log.Warn("failed to delete from user list index",
			zap.String("list_id", listID),
			zap.Error(err))
	}

	// Delete all list memberships
	// Query for all members
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST_MEMBERS#%s", listID)},
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		log.Error("failed to query list members for deletion",
			zap.String("list_id", listID),
			zap.Error(err))
		return nil // Don't fail the list deletion
	}

	// Delete each membership and its reverse index
	for _, item := range result.Items {
		accountID, ok := item["SK"].(*types.AttributeValueMemberS)
		if !ok {
			continue
		}

		// Delete membership
		deleteMember := &dynamodb.DeleteItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST_MEMBERS#%s", listID)},
				"SK": &types.AttributeValueMemberS{Value: accountID.Value},
			},
		}
		s.client.DeleteItem(ctx, deleteMember)

		// Delete reverse index
		deleteReverse := &dynamodb.DeleteItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACCOUNT_LISTS#%s", accountID.Value)},
				"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", listID, list.Username)},
			},
		}
		s.client.DeleteItem(ctx, deleteReverse)
	}

	return nil
}

// AddAccountsToList adds accounts to a list
func (s *dynamoDBStorage) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	log := common.WithContext(ctx)

	// Get the list to verify it exists and get the owner
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Add each account
	for _, accountID := range accountIDs {
		// Check if already in list
		exists, err := s.IsAccountInList(ctx, listID, accountID)
		if err != nil {
			log.Warn("failed to check if account in list",
				zap.String("list_id", listID),
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}
		if exists {
			continue // Skip if already in list
		}

		// Add membership
		memberItem := map[string]types.AttributeValue{
			"PK":      &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST_MEMBERS#%s", listID)},
			"SK":      &types.AttributeValueMemberS{Value: accountID},
			"AddedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		}

		putMember := &dynamodb.PutItemInput{
			TableName: aws.String(s.tableName),
			Item:      memberItem,
		}

		if _, err := s.client.PutItem(ctx, putMember); err != nil {
			log.Error("failed to add account to list",
				zap.String("list_id", listID),
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}

		// Add reverse index for fan-out
		reverseItem := map[string]types.AttributeValue{
			"PK":      &types.AttributeValueMemberS{Value: fmt.Sprintf("ACCOUNT_LISTS#%s", accountID)},
			"SK":      &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", listID, list.Username)},
			"AddedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		}

		putReverse := &dynamodb.PutItemInput{
			TableName: aws.String(s.tableName),
			Item:      reverseItem,
		}

		if _, err := s.client.PutItem(ctx, putReverse); err != nil {
			log.Error("failed to add reverse index",
				zap.String("list_id", listID),
				zap.String("account_id", accountID),
				zap.Error(err))
		}
	}

	return nil
}

// RemoveAccountsFromList removes accounts from a list
func (s *dynamoDBStorage) RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error {
	log := common.WithContext(ctx)

	// Get the list to verify it exists and get the owner
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Remove each account
	for _, accountID := range accountIDs {
		// Delete membership
		deleteMember := &dynamodb.DeleteItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST_MEMBERS#%s", listID)},
				"SK": &types.AttributeValueMemberS{Value: accountID},
			},
		}

		if _, err := s.client.DeleteItem(ctx, deleteMember); err != nil {
			log.Error("failed to remove account from list",
				zap.String("list_id", listID),
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}

		// Delete reverse index
		deleteReverse := &dynamodb.DeleteItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACCOUNT_LISTS#%s", accountID)},
				"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", listID, list.Username)},
			},
		}

		if _, err := s.client.DeleteItem(ctx, deleteReverse); err != nil {
			log.Warn("failed to delete reverse index",
				zap.String("list_id", listID),
				zap.String("account_id", accountID),
				zap.Error(err))
		}
	}

	return nil
}

// GetListAccounts retrieves all accounts in a list
func (s *dynamoDBStorage) GetListAccounts(ctx context.Context, listID string) ([]string, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST_MEMBERS#%s", listID)},
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("failed to query list members: %w", err)
	}

	accountIDs := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if accountID, ok := item["SK"].(*types.AttributeValueMemberS); ok {
			accountIDs = append(accountIDs, accountID.Value)
		}
	}

	return accountIDs, nil
}

// IsAccountInList checks if an account is in a list
func (s *dynamoDBStorage) IsAccountInList(ctx context.Context, listID, accountID string) (bool, error) {
	getItem := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST_MEMBERS#%s", listID)},
			"SK": &types.AttributeValueMemberS{Value: accountID},
		},
	}

	result, err := s.client.GetItem(ctx, getItem)
	if err != nil {
		return false, fmt.Errorf("failed to check list membership: %w", err)
	}

	return result.Item != nil, nil
}

// GetListsContainingAccount retrieves all lists (for a specific user) that contain an account
func (s *dynamoDBStorage) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	log := common.WithContext(ctx)

	// Query the reverse index
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        &types.AttributeValueMemberS{Value: fmt.Sprintf("ACCOUNT_LISTS#%s", accountID)},
			":sk_prefix": &types.AttributeValueMemberS{Value: ""}, // Get all lists
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("failed to query account lists: %w", err)
	}

	lists := make([]*storage.List, 0)
	for _, item := range result.Items {
		// SK format is "listID#username"
		sk, ok := item["SK"].(*types.AttributeValueMemberS)
		if !ok {
			continue
		}

		// Parse list ID and owner
		var listID, owner string
		if n, _ := fmt.Sscanf(sk.Value, "%[^#]#%s", &listID, &owner); n != 2 {
			continue
		}

		// Filter by username if specified
		if username != "" && owner != username {
			continue
		}

		// Get the full list details
		list, err := s.GetList(ctx, listID)
		if err != nil {
			log.Warn("failed to get list details",
				zap.String("list_id", listID),
				zap.Error(err))
			continue
		}

		lists = append(lists, list)
	}

	return lists, nil
}
