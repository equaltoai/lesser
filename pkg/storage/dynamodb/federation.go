package dynamodb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// Federation-related constants
const (
	domainBlockPrefix     = "DOMAIN_BLOCK#"
	domainAllowPrefix     = "DOMAIN_ALLOW#"
	federationPrefix      = "FEDERATION#"
	instancePrefix        = "INSTANCE#"
	emailDomainPrefix     = "EMAIL_DOMAIN_BLOCK#"
	federationStatsPrefix = "STATS#"
)

// GetDomainBlocks retrieves instance-level domain blocks with pagination
func (s *dynamoDBStorage) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	var startKey map[string]types.AttributeValue
	if cursor != "" {
		startKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: "DOMAIN_BLOCKS"},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "DOMAIN_BLOCKS"},
		},
		Limit:             safeInt32(limit),
		ExclusiveStartKey: startKey,
		ScanIndexForward:  aws.Bool(false), // Newest first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query domain blocks: %w", err)
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

// GetDomainBlock retrieves a specific domain block by ID
func (s *dynamoDBStorage) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	// First, we need to look up the domain by ID
	// For this, we'll scan with a filter (or maintain an ID index)
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND ID = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: domainBlockPrefix},
			":id":     &types.AttributeValueMemberS{Value: id},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain block: %w", err)
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

// CreateDomainBlock creates a new instance-level domain block
func (s *dynamoDBStorage) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	block.ID = uuid.New().String()
	block.CreatedAt = time.Now()
	block.UpdatedAt = block.CreatedAt

	item := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: domainBlockPrefix + block.Domain},
		"SK":             &types.AttributeValueMemberS{Value: domainBlockPrefix + block.Domain},
		"GSI1PK":         &types.AttributeValueMemberS{Value: "DOMAIN_BLOCKS"},
		"GSI1SK":         &types.AttributeValueMemberS{Value: block.CreatedAt.Format(time.RFC3339)},
		"ID":             &types.AttributeValueMemberS{Value: block.ID},
		"Domain":         &types.AttributeValueMemberS{Value: block.Domain},
		"Severity":       &types.AttributeValueMemberS{Value: block.Severity},
		"RejectMedia":    &types.AttributeValueMemberBOOL{Value: block.RejectMedia},
		"RejectReports":  &types.AttributeValueMemberBOOL{Value: block.RejectReports},
		"PrivateComment": &types.AttributeValueMemberS{Value: block.PrivateComment},
		"PublicComment":  &types.AttributeValueMemberS{Value: block.PublicComment},
		"Obfuscate":      &types.AttributeValueMemberBOOL{Value: block.Obfuscate},
		"CreatedBy":      &types.AttributeValueMemberS{Value: block.CreatedBy},
		"CreatedAt":      &types.AttributeValueMemberS{Value: block.CreatedAt.Format(time.RFC3339)},
		"UpdatedAt":      &types.AttributeValueMemberS{Value: block.UpdatedAt.Format(time.RFC3339)},
	}

	putInput := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	if _, err := s.client.PutItem(ctx, putInput); err != nil {
		if isConditionalCheckFailedException(err) {
			return fmt.Errorf("domain block already exists")
		}
		return fmt.Errorf("failed to create domain block: %w", err)
	}

	return nil
}

// UpdateDomainBlock updates an existing domain block
func (s *dynamoDBStorage) UpdateDomainBlock(ctx context.Context, id string, updates map[string]interface{}) error {
	// First get the block to find the domain
	block, err := s.GetDomainBlock(ctx, id)
	if err != nil {
		return err
	}

	updateExpr := "SET UpdatedAt = :updated"
	exprValues := map[string]types.AttributeValue{
		":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	if severity, ok := updates["severity"].(string); ok {
		updateExpr += ", Severity = :severity"
		exprValues[":severity"] = &types.AttributeValueMemberS{Value: severity}
	}

	if rejectMedia, ok := updates["reject_media"].(bool); ok {
		updateExpr += ", RejectMedia = :reject_media"
		exprValues[":reject_media"] = &types.AttributeValueMemberBOOL{Value: rejectMedia}
	}

	if rejectReports, ok := updates["reject_reports"].(bool); ok {
		updateExpr += ", RejectReports = :reject_reports"
		exprValues[":reject_reports"] = &types.AttributeValueMemberBOOL{Value: rejectReports}
	}

	if privateComment, ok := updates["private_comment"].(string); ok {
		updateExpr += ", PrivateComment = :private_comment"
		exprValues[":private_comment"] = &types.AttributeValueMemberS{Value: privateComment}
	}

	if publicComment, ok := updates["public_comment"].(string); ok {
		updateExpr += ", PublicComment = :public_comment"
		exprValues[":public_comment"] = &types.AttributeValueMemberS{Value: publicComment}
	}

	if obfuscate, ok := updates["obfuscate"].(bool); ok {
		updateExpr += ", Obfuscate = :obfuscate"
		exprValues[":obfuscate"] = &types.AttributeValueMemberBOOL{Value: obfuscate}
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: domainBlockPrefix + block.Domain},
			"SK": &types.AttributeValueMemberS{Value: domainBlockPrefix + block.Domain},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprValues,
	}

	if _, err := s.client.UpdateItem(ctx, input); err != nil {
		return fmt.Errorf("failed to update domain block: %w", err)
	}

	return nil
}

// DeleteDomainBlock removes a domain block
func (s *dynamoDBStorage) DeleteDomainBlock(ctx context.Context, id string) error {
	// First get the block to find the domain
	block, err := s.GetDomainBlock(ctx, id)
	if err != nil {
		return err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: domainBlockPrefix + block.Domain},
			"SK": &types.AttributeValueMemberS{Value: domainBlockPrefix + block.Domain},
		},
	}

	if _, err := s.client.DeleteItem(ctx, input); err != nil {
		return fmt.Errorf("failed to delete domain block: %w", err)
	}

	return nil
}

// IsDomainBlocked checks if a domain is blocked at the instance level
func (s *dynamoDBStorage) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: domainBlockPrefix + domain},
			"SK": &types.AttributeValueMemberS{Value: domainBlockPrefix + domain},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, nil, fmt.Errorf("failed to check domain block: %w", err)
	}

	if result.Item == nil {
		return false, nil, nil
	}

	var block storage.InstanceDomainBlock
	if err := attributevalue.UnmarshalMap(result.Item, &block); err != nil {
		return false, nil, fmt.Errorf("failed to unmarshal domain block: %w", err)
	}

	return true, &block, nil
}

// GetDomainAllows retrieves domain allows (for allowlist mode)
func (s *dynamoDBStorage) GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	var startKey map[string]types.AttributeValue
	if cursor != "" {
		startKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: "DOMAIN_ALLOWS"},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "DOMAIN_ALLOWS"},
		},
		Limit:             safeInt32(limit),
		ExclusiveStartKey: startKey,
		ScanIndexForward:  aws.Bool(false), // Newest first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query domain allows: %w", err)
	}

	allows := make([]*storage.DomainAllow, 0, len(result.Items))
	for _, item := range result.Items {
		var allow storage.DomainAllow
		if err := attributevalue.UnmarshalMap(item, &allow); err != nil {
			continue
		}
		allows = append(allows, &allow)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return allows, nextCursor, nil
}

// CreateDomainAllow adds a domain to the allowlist
func (s *dynamoDBStorage) CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error {
	allow.ID = uuid.New().String()
	allow.CreatedAt = time.Now()

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: domainAllowPrefix + allow.Domain},
		"SK":        &types.AttributeValueMemberS{Value: domainAllowPrefix + allow.Domain},
		"GSI1PK":    &types.AttributeValueMemberS{Value: "DOMAIN_ALLOWS"},
		"GSI1SK":    &types.AttributeValueMemberS{Value: allow.CreatedAt.Format(time.RFC3339)},
		"ID":        &types.AttributeValueMemberS{Value: allow.ID},
		"Domain":    &types.AttributeValueMemberS{Value: allow.Domain},
		"CreatedBy": &types.AttributeValueMemberS{Value: allow.CreatedBy},
		"CreatedAt": &types.AttributeValueMemberS{Value: allow.CreatedAt.Format(time.RFC3339)},
	}

	putInput := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	if _, err := s.client.PutItem(ctx, putInput); err != nil {
		if isConditionalCheckFailedException(err) {
			return fmt.Errorf("domain allow already exists")
		}
		return fmt.Errorf("failed to create domain allow: %w", err)
	}

	return nil
}

// DeleteDomainAllow removes a domain from the allowlist
func (s *dynamoDBStorage) DeleteDomainAllow(ctx context.Context, id string) error {
	// First, find the domain by ID
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND ID = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: domainAllowPrefix},
			":id":     &types.AttributeValueMemberS{Value: id},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Scan(ctx, scanInput)
	if err != nil {
		return fmt.Errorf("failed to find domain allow: %w", err)
	}

	if len(result.Items) == 0 {
		return storage.ErrNotFound
	}

	var allow storage.DomainAllow
	if err := attributevalue.UnmarshalMap(result.Items[0], &allow); err != nil {
		return fmt.Errorf("failed to unmarshal domain allow: %w", err)
	}

	deleteInput := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: domainAllowPrefix + allow.Domain},
			"SK": &types.AttributeValueMemberS{Value: domainAllowPrefix + allow.Domain},
		},
	}

	if _, err := s.client.DeleteItem(ctx, deleteInput); err != nil {
		return fmt.Errorf("failed to delete domain allow: %w", err)
	}

	return nil
}

// GetInstanceInfo retrieves information about a federated instance
func (s *dynamoDBStorage) GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: instancePrefix + domain},
			"SK": &types.AttributeValueMemberS{Value: instancePrefix + domain},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance info: %w", err)
	}

	if result.Item == nil {
		return nil, storage.ErrNotFound
	}

	var info storage.InstanceInfo
	if err := attributevalue.UnmarshalMap(result.Item, &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instance info: %w", err)
	}

	return &info, nil
}

// UpsertInstanceInfo creates or updates instance information
func (s *dynamoDBStorage) UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error {
	info.LastSeen = time.Now()
	if info.FirstSeen.IsZero() {
		info.FirstSeen = info.LastSeen
	}

	item, err := attributevalue.MarshalMap(info)
	if err != nil {
		return fmt.Errorf("failed to marshal instance info: %w", err)
	}

	item["PK"] = &types.AttributeValueMemberS{Value: instancePrefix + info.Domain}
	item["SK"] = &types.AttributeValueMemberS{Value: instancePrefix + info.Domain}
	item["GSI1PK"] = &types.AttributeValueMemberS{Value: "FEDERATION_ACTIVE"}
	item["GSI1SK"] = &types.AttributeValueMemberS{Value: info.LastSeen.Format(time.RFC3339)}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to upsert instance info: %w", err)
	}

	return nil
}

// GetFederationStatistics retrieves federation statistics for a time range
func (s *dynamoDBStorage) GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error) {
	// Query all instances with recent activity
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK BETWEEN :start AND :end"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: "FEDERATION_ACTIVE"},
			":start": &types.AttributeValueMemberS{Value: startTime.Format(time.RFC3339)},
			":end":   &types.AttributeValueMemberS{Value: endTime.Format(time.RFC3339)},
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("failed to query federation statistics: %w", err)
	}

	stats := &storage.FederationStats{
		ActiveInstances: len(result.Items),
		TotalMessages:   0,
		TotalUsers:      0,
	}

	// Aggregate statistics from instance data
	for _, item := range result.Items {
		if messagesStr, ok := item["TotalMessages"]; ok {
			if n, ok := messagesStr.(*types.AttributeValueMemberN); ok {
				if messages, err := strconv.ParseInt(n.Value, 10, 64); err == nil {
					stats.TotalMessages += messages
				}
			}
		}
		if usersStr, ok := item["ActiveUsers"]; ok {
			if n, ok := usersStr.(*types.AttributeValueMemberN); ok {
				if users, err := strconv.ParseInt(n.Value, 10, 64); err == nil {
					stats.TotalUsers += int(users)
				}
			}
		}
	}

	return stats, nil
}

// GetKnownInstances retrieves a list of known federated instances
func (s *dynamoDBStorage) GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	var startKey map[string]types.AttributeValue
	if cursor != "" {
		startKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: "FEDERATION_ACTIVE"},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "FEDERATION_ACTIVE"},
		},
		Limit:             safeInt32(limit),
		ExclusiveStartKey: startKey,
		ScanIndexForward:  aws.Bool(false), // Most recently active first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query known instances: %w", err)
	}

	instances := make([]*storage.InstanceInfo, 0, len(result.Items))
	for _, item := range result.Items {
		var info storage.InstanceInfo
		if err := attributevalue.UnmarshalMap(item, &info); err != nil {
			continue
		}
		instances = append(instances, &info)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return instances, nextCursor, nil
}

// CreateEmailDomainBlock creates an email domain block
func (s *dynamoDBStorage) CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error {
	block.ID = uuid.New().String()
	block.CreatedAt = time.Now()

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: emailDomainPrefix + block.Domain},
		"SK":        &types.AttributeValueMemberS{Value: emailDomainPrefix + block.Domain},
		"GSI1PK":    &types.AttributeValueMemberS{Value: "EMAIL_DOMAIN_BLOCKS"},
		"GSI1SK":    &types.AttributeValueMemberS{Value: block.CreatedAt.Format(time.RFC3339)},
		"ID":        &types.AttributeValueMemberS{Value: block.ID},
		"Domain":    &types.AttributeValueMemberS{Value: block.Domain},
		"CreatedBy": &types.AttributeValueMemberS{Value: block.CreatedBy},
		"CreatedAt": &types.AttributeValueMemberS{Value: block.CreatedAt.Format(time.RFC3339)},
	}

	putInput := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	if _, err := s.client.PutItem(ctx, putInput); err != nil {
		if isConditionalCheckFailedException(err) {
			return fmt.Errorf("email domain block already exists")
		}
		return fmt.Errorf("failed to create email domain block: %w", err)
	}

	return nil
}

// GetEmailDomainBlocks retrieves email domain blocks with pagination
func (s *dynamoDBStorage) GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	var startKey map[string]types.AttributeValue
	if cursor != "" {
		startKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: "EMAIL_DOMAIN_BLOCKS"},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "EMAIL_DOMAIN_BLOCKS"},
		},
		Limit:             safeInt32(limit),
		ExclusiveStartKey: startKey,
		ScanIndexForward:  aws.Bool(false), // Newest first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query email domain blocks: %w", err)
	}

	blocks := make([]*storage.EmailDomainBlock, 0, len(result.Items))
	for _, item := range result.Items {
		var block storage.EmailDomainBlock
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

// DeleteEmailDomainBlock deletes an email domain block
func (s *dynamoDBStorage) DeleteEmailDomainBlock(ctx context.Context, id string) error {
	// First, find the domain by ID
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND ID = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: emailDomainPrefix},
			":id":     &types.AttributeValueMemberS{Value: id},
		},
		Limit: aws.Int32(1),
	}

	result, err := s.client.Scan(ctx, scanInput)
	if err != nil {
		return fmt.Errorf("failed to find email domain block: %w", err)
	}

	if len(result.Items) == 0 {
		return storage.ErrNotFound
	}

	var block storage.EmailDomainBlock
	if err := attributevalue.UnmarshalMap(result.Items[0], &block); err != nil {
		return fmt.Errorf("failed to unmarshal email domain block: %w", err)
	}

	deleteInput := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: emailDomainPrefix + block.Domain},
			"SK": &types.AttributeValueMemberS{Value: emailDomainPrefix + block.Domain},
		},
	}

	if _, err := s.client.DeleteItem(ctx, deleteInput); err != nil {
		return fmt.Errorf("failed to delete email domain block: %w", err)
	}

	return nil
}

// isConditionalCheckFailedException checks if an error is a ConditionalCheckFailedException
func isConditionalCheckFailedException(err error) bool {
	// Check if the error contains the conditional check failed message
	return err != nil && strings.Contains(err.Error(), "ConditionalCheckFailedException")
}
