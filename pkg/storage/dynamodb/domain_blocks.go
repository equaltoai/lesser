package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/google/uuid"
)

// AddDomainBlock adds a domain to the user's block list
func (s *dynamoDBStorage) AddDomainBlock(ctx context.Context, username, domain string) error {
	item := storage.DomainBlock{
		Username:  username,
		Domain:    domain,
		CreatedAt: time.Now(),
	}

	av, err := s.MarshalItem(map[string]any{
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
		Limit: safeInt32(limit),
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

// CreateInstanceDomainBlock creates an instance-level domain block
func (s *dynamoDBStorage) CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	// Generate ID if not provided
	if block.ID == "" {
		block.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	block.CreatedAt = now
	block.UpdatedAt = now

	// Normalize domain to lowercase
	block.Domain = strings.ToLower(strings.TrimSpace(block.Domain))

	// Create the record
	item := map[string]any{
		"PK":             fmt.Sprintf("DOMAIN_BLOCK#%s", block.Domain),
		"SK":             fmt.Sprintf("DOMAIN_BLOCK#%s", block.Domain),
		"GSI1PK":         "DOMAIN_BLOCKS",
		"GSI1SK":         fmt.Sprintf("%d#%s", now.Unix(), block.Domain),
		"ID":             block.ID,
		"Domain":         block.Domain,
		"Severity":       block.Severity,
		"RejectMedia":    block.RejectMedia,
		"RejectReports":  block.RejectReports,
		"PrivateComment": block.PrivateComment,
		"PublicComment":  block.PublicComment,
		"Obfuscate":      block.Obfuscate,
		"CreatedBy":      block.CreatedBy,
		"CreatedByID":    block.CreatedByID,
		"CreatedAt":      block.CreatedAt,
		"UpdatedAt":      block.UpdatedAt,
		"Type":           "INSTANCE_DOMAIN_BLOCK",
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("failed to marshal domain block: %w", err)
	}

	// Add condition to prevent duplicate blocks
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		// Check if it's a conditional check failure (duplicate)
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return fmt.Errorf("domain block already exists for %s", block.Domain)
		}
		return fmt.Errorf("failed to create domain block: %w", err)
	}

	return nil
}

// GetInstanceDomainBlock retrieves a domain block by domain
func (s *dynamoDBStorage) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))

	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
	}

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get domain block: %w", err)
	}

	if result.Item == nil {
		return nil, storage.ErrNotFound
	}

	var block storage.InstanceDomainBlock
	if err := attributevalue.UnmarshalMap(result.Item, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal domain block: %w", err)
	}

	return &block, nil
}

// GetInstanceDomainBlockByID retrieves a domain block by ID
func (s *dynamoDBStorage) GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	// Query GSI1 to find the domain block by ID
	expr, err := expression.NewBuilder().
		WithKeyCondition(
			expression.Key("GSI1PK").Equal(expression.Value("DOMAIN_BLOCKS")),
		).
		WithFilter(
			expression.Name("ID").Equal(expression.Value(id)),
		).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build query expression: %w", err)
	}

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		IndexName:                 aws.String("GSI1"),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query domain block by ID: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, storage.ErrNotFound
	}

	var block storage.InstanceDomainBlock
	if err := attributevalue.UnmarshalMap(result.Items[0], &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal domain block: %w", err)
	}

	return &block, nil
}

// ListInstanceDomainBlocks lists all instance domain blocks with pagination
func (s *dynamoDBStorage) ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "DOMAIN_BLOCKS"},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: "DOMAIN_BLOCKS"},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list domain blocks: %w", err)
	}

	blocks := make([]*storage.InstanceDomainBlock, 0, len(result.Items))
	for _, item := range result.Items {
		var block storage.InstanceDomainBlock
		if err := attributevalue.UnmarshalMap(item, &block); err != nil {
			continue
		}
		blocks = append(blocks, &block)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return blocks, nextCursor, nil
}

// UpdateInstanceDomainBlock updates an existing domain block
func (s *dynamoDBStorage) UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error {
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Build update expression
	updateExpr := "SET UpdatedAt = :now"
	exprAttrValues := map[string]types.AttributeValue{
		":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}
	exprAttrNames := make(map[string]string)

	// Handle each possible update field
	if severity, ok := updates["severity"].(string); ok {
		updateExpr += ", Severity = :severity"
		exprAttrValues[":severity"] = &types.AttributeValueMemberS{Value: severity}
	}

	if rejectMedia, ok := updates["reject_media"].(bool); ok {
		updateExpr += ", RejectMedia = :reject_media"
		exprAttrValues[":reject_media"] = &types.AttributeValueMemberBOOL{Value: rejectMedia}
	}

	if rejectReports, ok := updates["reject_reports"].(bool); ok {
		updateExpr += ", RejectReports = :reject_reports"
		exprAttrValues[":reject_reports"] = &types.AttributeValueMemberBOOL{Value: rejectReports}
	}

	if privateComment, ok := updates["private_comment"].(string); ok {
		updateExpr += ", PrivateComment = :private_comment"
		exprAttrValues[":private_comment"] = &types.AttributeValueMemberS{Value: privateComment}
	}

	if publicComment, ok := updates["public_comment"].(string); ok {
		updateExpr += ", PublicComment = :public_comment"
		exprAttrValues[":public_comment"] = &types.AttributeValueMemberS{Value: publicComment}
	}

	if obfuscate, ok := updates["obfuscate"].(bool); ok {
		updateExpr += ", Obfuscate = :obfuscate"
		exprAttrValues[":obfuscate"] = &types.AttributeValueMemberBOOL{Value: obfuscate}
	}

	// If we have expression attribute names, include them
	var updateInput *dynamodb.UpdateItemInput
	if len(exprAttrNames) > 0 {
		updateInput = &dynamodb.UpdateItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
				"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
			},
			UpdateExpression:          aws.String(updateExpr),
			ExpressionAttributeValues: exprAttrValues,
			ExpressionAttributeNames:  exprAttrNames,
			ConditionExpression:       aws.String("attribute_exists(PK)"),
		}
	} else {
		updateInput = &dynamodb.UpdateItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
				"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
			},
			UpdateExpression:          aws.String(updateExpr),
			ExpressionAttributeValues: exprAttrValues,
			ConditionExpression:       aws.String("attribute_exists(PK)"),
		}
	}

	_, err := s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to update domain block: %w", err)
	}

	return nil
}

// DeleteInstanceDomainBlock deletes a domain block
func (s *dynamoDBStorage) DeleteInstanceDomainBlock(ctx context.Context, domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))

	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN_BLOCK#%s", domain)},
	}

	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(s.tableName),
		Key:                 key,
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to delete domain block: %w", err)
	}

	return nil
}

// IsInstanceDomainBlocked checks if a domain is blocked at the instance level
func (s *dynamoDBStorage) IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	block, err := s.GetInstanceDomainBlock(ctx, domain)
	if err != nil {
		if err == storage.ErrNotFound {
			// Check parent domains (e.g., if sub.example.com is queried, check example.com)
			parts := strings.Split(domain, ".")
			for i := 1; i < len(parts); i++ {
				parentDomain := strings.Join(parts[i:], ".")
				parentBlock, err := s.GetInstanceDomainBlock(ctx, parentDomain)
				if err == nil {
					return true, parentBlock, nil
				}
			}
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, block, nil
}
