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
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FilterRecord represents a filter in DynamoDB
type FilterRecord struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
	storage.Filter
}

// FilterKeywordRecord represents a filter keyword in DynamoDB
type FilterKeywordRecord struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
	storage.FilterKeyword
}

// FilterStatusRecord represents a filter status in DynamoDB
type FilterStatusRecord struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
	storage.FilterStatus
}

// CreateFilter creates a new filter
func (s *dynamoDBStorage) CreateFilter(ctx context.Context, filter *storage.Filter) error {
	s.logger().Info("creating filter", zap.String("username", filter.Username), zap.String("title", filter.Title))

	// Generate ID if not provided
	if filter.ID == "" {
		filter.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	filter.CreatedAt = now
	filter.UpdatedAt = now

	record := FilterRecord{
		PK:     fmt.Sprintf("USER#%s", filter.Username),
		SK:     fmt.Sprintf("FILTER#%s", filter.ID),
		Filter: *filter,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal filter: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create filter: %w", err)
	}

	return nil
}

// GetFilter retrieves a filter by ID
func (s *dynamoDBStorage) GetFilter(ctx context.Context, filterID string) (*storage.Filter, error) {
	// We need to scan for this since we don't know the username
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(SK, :sk) AND #id = :id"),
		ExpressionAttributeNames: map[string]string{
			"#id": "id",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk": &types.AttributeValueMemberS{Value: "FILTER#"},
			":id": &types.AttributeValueMemberS{Value: filterID},
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for filter: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil
	}

	var record FilterRecord
	err = attributevalue.UnmarshalMap(result.Items[0], &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal filter: %w", err)
	}

	return &record.Filter, nil
}

// GetFiltersForUser retrieves all filters for a user
func (s *dynamoDBStorage) GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			":sk": &types.AttributeValueMemberS{Value: "FILTER#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query filters: %w", err)
	}

	filters := make([]*storage.Filter, 0, len(result.Items))
	for _, item := range result.Items {
		var record FilterRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			s.logger().Error("failed to unmarshal filter record", zap.Error(err))
			continue
		}
		filters = append(filters, &record.Filter)
	}

	return filters, nil
}

// UpdateFilter updates a filter
func (s *dynamoDBStorage) UpdateFilter(ctx context.Context, filterID string, updates map[string]interface{}) error {
	// First get the existing filter to find the username
	filter, err := s.GetFilter(ctx, filterID)
	if err != nil {
		return err
	}
	if filter == nil {
		return fmt.Errorf("filter not found")
	}

	// Build update expression
	updateExpr := "SET UpdatedAt = :updated"
	exprAttrValues := map[string]types.AttributeValue{
		":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	if title, ok := updates["title"].(string); ok {
		updateExpr += ", Title = :title"
		exprAttrValues[":title"] = &types.AttributeValueMemberS{Value: title}
	}

	if context, ok := updates["context"].([]string); ok {
		updateExpr += ", #context = :context"
		contextList := make([]types.AttributeValue, len(context))
		for i, c := range context {
			contextList[i] = &types.AttributeValueMemberS{Value: c}
		}
		exprAttrValues[":context"] = &types.AttributeValueMemberL{Value: contextList}
	}

	if filterAction, ok := updates["filter_action"].(string); ok {
		updateExpr += ", FilterAction = :filter_action"
		exprAttrValues[":filter_action"] = &types.AttributeValueMemberS{Value: filterAction}
	}

	if expiresAt, ok := updates["expires_at"].(*time.Time); ok {
		updateExpr += ", ExpiresAt = :expires_at"
		if expiresAt != nil {
			exprAttrValues[":expires_at"] = &types.AttributeValueMemberS{Value: expiresAt.Format(time.RFC3339)}
		} else {
			exprAttrValues[":expires_at"] = &types.AttributeValueMemberNULL{Value: true}
		}
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", filter.Username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FILTER#%s", filterID)},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprAttrValues,
	}

	// Add expression attribute names if we're updating context
	if _, hasContext := updates["context"]; hasContext {
		input.ExpressionAttributeNames = map[string]string{
			"#context": "context",
		}
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update filter: %w", err)
	}

	return nil
}

// DeleteFilter deletes a filter
func (s *dynamoDBStorage) DeleteFilter(ctx context.Context, filterID string) error {
	// First get the filter to find the username
	filter, err := s.GetFilter(ctx, filterID)
	if err != nil {
		return err
	}
	if filter == nil {
		return fmt.Errorf("filter not found")
	}

	// Delete all keywords and statuses first
	keywords, err := s.GetFilterKeywords(ctx, filterID)
	if err != nil {
		return err
	}
	for _, keyword := range keywords {
		if err := s.DeleteFilterKeyword(ctx, keyword.ID); err != nil {
			return err
		}
	}

	statuses, err := s.GetFilterStatuses(ctx, filterID)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		if err := s.DeleteFilterStatus(ctx, status.ID); err != nil {
			return err
		}
	}

	// Delete the filter itself
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", filter.Username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("FILTER#%s", filterID)},
		},
	}

	_, err = s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete filter: %w", err)
	}

	return nil
}

// AddFilterKeyword adds a keyword to a filter
func (s *dynamoDBStorage) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	// Generate ID if not provided
	if keyword.ID == "" {
		keyword.ID = uuid.New().String()
	}
	keyword.FilterID = filterID
	keyword.CreatedAt = time.Now()

	record := FilterKeywordRecord{
		PK:            fmt.Sprintf("FILTER#%s", filterID),
		SK:            fmt.Sprintf("KEYWORD#%s", keyword.ID),
		FilterKeyword: *keyword,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal filter keyword: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create filter keyword: %w", err)
	}

	return nil
}

// GetFilterKeywords retrieves all keywords for a filter
func (s *dynamoDBStorage) GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("FILTER#%s", filterID)},
			":sk": &types.AttributeValueMemberS{Value: "KEYWORD#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query filter keywords: %w", err)
	}

	keywords := make([]*storage.FilterKeyword, 0, len(result.Items))
	for _, item := range result.Items {
		var record FilterKeywordRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			s.logger().Error("failed to unmarshal filter keyword record", zap.Error(err))
			continue
		}
		keywords = append(keywords, &record.FilterKeyword)
	}

	return keywords, nil
}

// UpdateFilterKeyword updates a filter keyword
func (s *dynamoDBStorage) UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]interface{}) error {
	// Need to find the filter ID first
	// For now, we'll scan - in production, you might want to maintain a GSI
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(SK, :sk) AND #id = :id"),
		ExpressionAttributeNames: map[string]string{
			"#id": "id",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk": &types.AttributeValueMemberS{Value: "KEYWORD#"},
			":id": &types.AttributeValueMemberS{Value: keywordID},
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to scan for keyword: %w", err)
	}

	if len(result.Items) == 0 {
		return fmt.Errorf("keyword not found")
	}

	var record FilterKeywordRecord
	err = attributevalue.UnmarshalMap(result.Items[0], &record)
	if err != nil {
		return fmt.Errorf("failed to unmarshal keyword: %w", err)
	}

	// Build update expression
	updateExpr := "SET "
	exprAttrValues := make(map[string]types.AttributeValue)
	first := true

	if keyword, ok := updates["keyword"].(string); ok {
		if !first {
			updateExpr += ", "
		}
		updateExpr += "Keyword = :keyword"
		exprAttrValues[":keyword"] = &types.AttributeValueMemberS{Value: keyword}
		first = false
	}

	if wholeWord, ok := updates["whole_word"].(bool); ok {
		if !first {
			updateExpr += ", "
		}
		updateExpr += "WholeWord = :whole_word"
		exprAttrValues[":whole_word"] = &types.AttributeValueMemberBOOL{Value: wholeWord}
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: record.PK},
			"SK": &types.AttributeValueMemberS{Value: record.SK},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprAttrValues,
	}

	_, err = s.client.UpdateItem(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to update keyword: %w", err)
	}

	return nil
}

// DeleteFilterKeyword deletes a filter keyword
func (s *dynamoDBStorage) DeleteFilterKeyword(ctx context.Context, keywordID string) error {
	// Need to find the keyword first
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(SK, :sk) AND #id = :id"),
		ExpressionAttributeNames: map[string]string{
			"#id": "id",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk": &types.AttributeValueMemberS{Value: "KEYWORD#"},
			":id": &types.AttributeValueMemberS{Value: keywordID},
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to scan for keyword: %w", err)
	}

	if len(result.Items) == 0 {
		return fmt.Errorf("keyword not found")
	}

	// Extract PK and SK from the found item
	pk := result.Items[0]["PK"].(*types.AttributeValueMemberS).Value
	sk := result.Items[0]["SK"].(*types.AttributeValueMemberS).Value

	deleteInput := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	}

	_, err = s.client.DeleteItem(ctx, deleteInput)
	if err != nil {
		return fmt.Errorf("failed to delete keyword: %w", err)
	}

	return nil
}

// AddFilterStatus adds a status to a filter
func (s *dynamoDBStorage) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	// Generate ID if not provided
	if status.ID == "" {
		status.ID = uuid.New().String()
	}
	status.FilterID = filterID
	status.CreatedAt = time.Now()

	record := FilterStatusRecord{
		PK:           fmt.Sprintf("FILTER#%s", filterID),
		SK:           fmt.Sprintf("STATUS#%s", status.ID),
		FilterStatus: *status,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal filter status: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create filter status: %w", err)
	}

	return nil
}

// GetFilterStatuses retrieves all statuses for a filter
func (s *dynamoDBStorage) GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("FILTER#%s", filterID)},
			":sk": &types.AttributeValueMemberS{Value: "STATUS#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query filter statuses: %w", err)
	}

	statuses := make([]*storage.FilterStatus, 0, len(result.Items))
	for _, item := range result.Items {
		var record FilterStatusRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			s.logger().Error("failed to unmarshal filter status record", zap.Error(err))
			continue
		}
		statuses = append(statuses, &record.FilterStatus)
	}

	return statuses, nil
}

// DeleteFilterStatus deletes a filter status
func (s *dynamoDBStorage) DeleteFilterStatus(ctx context.Context, statusID string) error {
	// Need to find the status first
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(SK, :sk) AND #id = :id"),
		ExpressionAttributeNames: map[string]string{
			"#id": "id",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk": &types.AttributeValueMemberS{Value: "STATUS#"},
			":id": &types.AttributeValueMemberS{Value: statusID},
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to scan for status: %w", err)
	}

	if len(result.Items) == 0 {
		return fmt.Errorf("status not found")
	}

	// Extract PK and SK from the found item
	pk := result.Items[0]["PK"].(*types.AttributeValueMemberS).Value
	sk := result.Items[0]["SK"].(*types.AttributeValueMemberS).Value

	deleteInput := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	}

	_, err = s.client.DeleteItem(ctx, deleteInput)
	if err != nil {
		return fmt.Errorf("failed to delete status: %w", err)
	}

	return nil
}
