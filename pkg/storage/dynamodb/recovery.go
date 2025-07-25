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

// Trustee operations

// StoreTrustee stores a trustee configuration for social recovery
func (d *dynamoDBStorage) StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error {
	trustee.Username = username // Ensure username is set
	item, err := attributevalue.MarshalMap(trustee)
	if err != nil {
		return fmt.Errorf("failed to marshal trustee: %w", err)
	}

	// Add keys
	item["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)}
	item["SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUSTEE#%s", trustee.ActorID)}

	_, err = d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	})
	return err
}

// GetTrustees retrieves all trustees for a user
func (d *dynamoDBStorage) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	result, err := d.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(d.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "TRUSTEE#"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query trustees: %w", err)
	}

	trustees := make([]*storage.TrusteeConfig, 0, len(result.Items))
	for _, item := range result.Items {
		var trustee storage.TrusteeConfig
		if err := attributevalue.UnmarshalMap(item, &trustee); err != nil {
			continue
		}
		trustees = append(trustees, &trustee)
	}

	return trustees, nil
}

// DeleteTrustee removes a trustee
func (d *dynamoDBStorage) DeleteTrustee(ctx context.Context, username, trusteeActorID string) error {
	_, err := d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUSTEE#%s", trusteeActorID)},
		},
	})
	return err
}

// UpdateTrusteeConfirmed updates the confirmed status of a trustee
func (d *dynamoDBStorage) UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error {
	_, err := d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRUSTEE#%s", trusteeActorID)},
		},
		UpdateExpression: aws.String("SET confirmed = :confirmed"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":confirmed": &types.AttributeValueMemberBOOL{Value: confirmed},
		},
	})
	return err
}

// Recovery request operations

// StoreRecoveryRequest stores a social recovery request
func (d *dynamoDBStorage) StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	item, err := attributevalue.MarshalMap(request)
	if err != nil {
		return fmt.Errorf("failed to marshal recovery request: %w", err)
	}

	// Add keys
	item["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("RECOVERY#%s", request.ID)}
	item["SK"] = &types.AttributeValueMemberS{Value: "REQUEST"}
	// Add TTL for automatic cleanup
	item["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", request.ExpiresAt.Unix())}

	_, err = d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	})
	return err
}

// GetRecoveryRequest retrieves a recovery request by ID
func (d *dynamoDBStorage) GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	result, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("RECOVERY#%s", requestID)},
			"SK": &types.AttributeValueMemberS{Value: "REQUEST"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get recovery request: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var request storage.SocialRecoveryRequest
	if err := attributevalue.UnmarshalMap(result.Item, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recovery request: %w", err)
	}

	return &request, nil
}

// UpdateRecoveryRequest updates a recovery request
func (d *dynamoDBStorage) UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	return d.StoreRecoveryRequest(ctx, request)
}

// DeleteRecoveryRequest deletes a recovery request
func (d *dynamoDBStorage) DeleteRecoveryRequest(ctx context.Context, requestID string) error {
	_, err := d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("RECOVERY#%s", requestID)},
			"SK": &types.AttributeValueMemberS{Value: "REQUEST"},
		},
	})
	return err
}

// GetActiveRecoveryRequests gets all active recovery requests for a user
func (d *dynamoDBStorage) GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error) {
	// Use GSI to query by username
	result, err := d.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(d.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND begins_with(GSI1SK, :sk)"),
		FilterExpression:       aws.String("#status = :status"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk":     &types.AttributeValueMemberS{Value: "RECOVERY#"},
			":status": &types.AttributeValueMemberS{Value: "pending"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query recovery requests: %w", err)
	}

	requests := make([]*storage.SocialRecoveryRequest, 0, len(result.Items))
	for _, item := range result.Items {
		var request storage.SocialRecoveryRequest
		if err := attributevalue.UnmarshalMap(item, &request); err != nil {
			continue
		}
		// Check if not expired
		if time.Now().Before(request.ExpiresAt) {
			requests = append(requests, &request)
		}
	}

	return requests, nil
}

// Recovery code operations

// StoreRecoveryCode stores a recovery code
func (d *dynamoDBStorage) StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error {
	code.Username = username // Ensure username is set
	item, err := attributevalue.MarshalMap(code)
	if err != nil {
		return fmt.Errorf("failed to marshal recovery code: %w", err)
	}

	// Add keys
	item["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)}
	item["SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("RECOVERY_CODE#%d", code.Position)}

	_, err = d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	})
	return err
}

// GetRecoveryCodes retrieves all recovery codes for a user
func (d *dynamoDBStorage) GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	result, err := d.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(d.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "RECOVERY_CODE#"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query recovery codes: %w", err)
	}

	codes := make([]*storage.RecoveryCodeItem, 0, len(result.Items))
	for _, item := range result.Items {
		var code storage.RecoveryCodeItem
		if err := attributevalue.UnmarshalMap(item, &code); err != nil {
			continue
		}
		codes = append(codes, &code)
	}

	return codes, nil
}

// MarkRecoveryCodeUsed marks a recovery code as used
func (d *dynamoDBStorage) MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error {
	// First, find the code by hash
	codes, err := d.GetRecoveryCodes(ctx, username)
	if err != nil {
		return err
	}

	var targetCode *storage.RecoveryCodeItem
	for _, code := range codes {
		if code.CodeHash == codeHash {
			targetCode = code
			break
		}
	}

	if targetCode == nil {
		return fmt.Errorf("recovery code not found")
	}

	// Update the code
	now := time.Now()
	_, err = d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("RECOVERY_CODE#%d", targetCode.Position)},
		},
		UpdateExpression: aws.String("SET used_at = :used_at"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":used_at": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		},
	})
	return err
}

// DeleteAllRecoveryCodes deletes all recovery codes for a user
func (d *dynamoDBStorage) DeleteAllRecoveryCodes(ctx context.Context, username string) error {
	// First, get all codes
	codes, err := d.GetRecoveryCodes(ctx, username)
	if err != nil {
		return err
	}

	// Delete each code
	for _, code := range codes {
		_, err := d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(d.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
				"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("RECOVERY_CODE#%d", code.Position)},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to delete recovery code: %w", err)
		}
	}

	return nil
}

// CountUnusedRecoveryCodes counts how many unused recovery codes the user has
func (d *dynamoDBStorage) CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error) {
	codes, err := d.GetRecoveryCodes(ctx, username)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, code := range codes {
		if code.UsedAt == nil {
			count++
		}
	}

	return count, nil
}

// StoreRecoveryToken stores a generic recovery token with data
func (d *dynamoDBStorage) StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error {
	// Create a TTL for 24 hours
	ttl := time.Now().Add(24 * time.Hour).Unix()

	item := map[string]any{
		"PK":        key,
		"SK":        "TOKEN",
		"Data":      data,
		"CreatedAt": time.Now(),
		"TTL":       ttl,
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("failed to marshal recovery token: %w", err)
	}

	_, err = d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to store recovery token: %w", err)
	}

	return nil
}

// GetRecoveryToken retrieves a recovery token by key
func (d *dynamoDBStorage) GetRecoveryToken(ctx context.Context, key string) (map[string]any, error) {
	result, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: key},
			"SK": &types.AttributeValueMemberS{Value: "TOKEN"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get recovery token: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("recovery token not found")
	}

	var item struct {
		Data map[string]any `dynamodbav:"Data"`
	}

	err = attributevalue.UnmarshalMap(result.Item, &item)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal recovery token: %w", err)
	}

	return item.Data, nil
}

// DeleteRecoveryToken deletes a recovery token
func (d *dynamoDBStorage) DeleteRecoveryToken(ctx context.Context, key string) error {
	_, err := d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: key},
			"SK": &types.AttributeValueMemberS{Value: "TOKEN"},
		},
	})
	return err
}
