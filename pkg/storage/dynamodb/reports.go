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
	"github.com/google/uuid"
)

// CreateReport creates a new report in the database
func (s *dynamoDBStorage) CreateReport(ctx context.Context, report *storage.Report) error {
	// Generate ID if not provided
	if report.ID == "" {
		report.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	report.CreatedAt = now
	report.UpdatedAt = now
	report.Status = storage.ReportStatusOpen

	// Create the main report item
	reportItem := map[string]any{
		"PK":                fmt.Sprintf("REPORT#%s", report.ID),
		"SK":                "REPORT",
		"GSI1PK":            fmt.Sprintf("USER#%s", report.ReporterID),
		"GSI1SK":            fmt.Sprintf("REPORT#%d", now.Unix()),
		"GSI2PK":            fmt.Sprintf("REPORTED#%s", report.TargetAccountID),
		"GSI2SK":            fmt.Sprintf("REPORT#%d", now.Unix()),
		"GSI3PK":            fmt.Sprintf("STATUS#%s", report.Status),
		"GSI3SK":            fmt.Sprintf("REPORT#%d", now.Unix()),
		"ID":                report.ID,
		"ReporterID":        report.ReporterID,
		"TargetAccountID":   report.TargetAccountID,
		"StatusIDs":         report.StatusIDs,
		"Comment":           report.Comment,
		"Category":          report.Category,
		"RuleIDs":           report.RuleIDs,
		"Forwarded":         report.Forwarded,
		"Status":            string(report.Status),
		"ActionTaken":       report.ActionTaken,
		"ActionTakenAt":     report.ActionTakenAt,
		"ModeratorID":       report.ModeratorID,
		"ModerationEventID": report.ModerationEventID,
		"CreatedAt":         report.CreatedAt,
		"UpdatedAt":         report.UpdatedAt,
		"TTL":               now.Add(90 * 24 * time.Hour).Unix(), // 90 day TTL
	}

	av, err := attributevalue.MarshalMap(reportItem)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}

	// Update reporter stats
	statsKey := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", report.ReporterID)},
		"SK": &types.AttributeValueMemberS{Value: "REPORT_STATS"},
	}

	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:        aws.String(s.tableName),
		Key:              statsKey,
		UpdateExpression: aws.String("ADD TotalReports :one SET LastReportAt = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one": &types.AttributeValueMemberN{Value: "1"},
			":now": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		},
	})

	return err
}

// GetReport retrieves a report by ID
func (s *dynamoDBStorage) GetReport(ctx context.Context, id string) (*storage.Report, error) {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPORT#%s", id)},
		"SK": &types.AttributeValueMemberS{Value: "REPORT"},
	}

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	if result.Item == nil {
		return nil, storage.ErrNotFound
	}

	var report storage.Report
	if err := attributevalue.UnmarshalMap(result.Item, &report); err != nil {
		return nil, fmt.Errorf("failed to unmarshal report: %w", err)
	}

	return &report, nil
}

// GetUserReports retrieves reports created by a specific user
func (s *dynamoDBStorage) GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query user reports: %w", err)
	}

	reports := make([]*storage.Report, 0, len(result.Items))
	for _, item := range result.Items {
		var report storage.Report
		if err := attributevalue.UnmarshalMap(item, &report); err != nil {
			continue
		}
		reports = append(reports, &report)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return reports, nextCursor, nil
}

// GetReportsByTarget retrieves reports targeting a specific account
func (s *dynamoDBStorage) GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPORTED#%s", targetAccountID)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI2PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPORTED#%s", targetAccountID)},
			"GSI2SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query reports by target: %w", err)
	}

	reports := make([]*storage.Report, 0, len(result.Items))
	for _, item := range result.Items {
		var report storage.Report
		if err := attributevalue.UnmarshalMap(item, &report); err != nil {
			continue
		}
		reports = append(reports, &report)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI2SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return reports, nextCursor, nil
}

// GetReportsByStatus retrieves reports with a specific status
func (s *dynamoDBStorage) GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", status)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Newest first
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI3PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", status)},
			"GSI3SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query reports by status: %w", err)
	}

	reports := make([]*storage.Report, 0, len(result.Items))
	for _, item := range result.Items {
		var report storage.Report
		if err := attributevalue.UnmarshalMap(item, &report); err != nil {
			continue
		}
		reports = append(reports, &report)
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI3SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return reports, nextCursor, nil
}

// UpdateReportStatus updates the status of a report
func (s *dynamoDBStorage) UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	now := time.Now()
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPORT#%s", id)},
		"SK": &types.AttributeValueMemberS{Value: "REPORT"},
	}

	updateExpression := "SET #status = :status, UpdatedAt = :now, ActionTaken = :action, ModeratorID = :mod"
	expressionAttributeNames := map[string]string{
		"#status": "Status",
	}
	expressionAttributeValues := map[string]types.AttributeValue{
		":status": &types.AttributeValueMemberS{Value: string(status)},
		":now":    &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		":action": &types.AttributeValueMemberS{Value: actionTaken},
		":mod":    &types.AttributeValueMemberS{Value: moderatorID},
	}

	if status != storage.ReportStatusOpen {
		updateExpression += ", ActionTakenAt = :actionAt"
		expressionAttributeValues[":actionAt"] = &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)}
	}

	// First, get the current report to update GSI
	report, err := s.GetReport(ctx, id)
	if err != nil {
		return err
	}

	// Update the main item
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.tableName),
		Key:                       key,
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	})
	if err != nil {
		return fmt.Errorf("failed to update report status: %w", err)
	}

	// Update reporter stats if resolved
	if status == storage.ReportStatusResolved {
		statsKey := map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", report.ReporterID)},
			"SK": &types.AttributeValueMemberS{Value: "REPORT_STATS"},
		}

		_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:        aws.String(s.tableName),
			Key:              statsKey,
			UpdateExpression: aws.String("ADD ResolvedReports :one"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":one": &types.AttributeValueMemberN{Value: "1"},
			},
		})
	}

	return err
}

// IncrementFalseReports increments the false report count for a user
func (s *dynamoDBStorage) IncrementFalseReports(ctx context.Context, username string) error {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		"SK": &types.AttributeValueMemberS{Value: "REPORT_STATS"},
	}

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:        aws.String(s.tableName),
		Key:              key,
		UpdateExpression: aws.String("ADD FalseReports :one SET LastFalseReportAt = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one": &types.AttributeValueMemberN{Value: "1"},
			":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	})

	return err
}

// GetReportStats retrieves reporting statistics for a user
func (s *dynamoDBStorage) GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error) {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		"SK": &types.AttributeValueMemberS{Value: "REPORT_STATS"},
	}

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get report stats: %w", err)
	}

	if result.Item == nil {
		// Return empty stats if none exist
		return &storage.ReportStats{}, nil
	}

	var stats storage.ReportStats
	if err := attributevalue.UnmarshalMap(result.Item, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal report stats: %w", err)
	}

	return &stats, nil
}

// AssignReport assigns a report to a moderator/admin
func (s *dynamoDBStorage) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	now := time.Now()
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPORT#%s", reportID)},
		"SK": &types.AttributeValueMemberS{Value: "REPORT"},
	}

	updateExpression := "SET AssignedTo = :assignedTo, UpdatedAt = :now"
	expressionAttributeValues := map[string]types.AttributeValue{
		":assignedTo": &types.AttributeValueMemberS{Value: assignedTo},
		":now":        &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
	}

	// Add condition to ensure report exists
	conditionExpression := "attribute_exists(PK)"

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.tableName),
		Key:                       key,
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeValues: expressionAttributeValues,
		ConditionExpression:       aws.String(conditionExpression),
	})
	if err != nil {
		// Check if the error is because the report doesn't exist
		var ccfe *types.ConditionalCheckFailedException
		if errors.As(err, &ccfe) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to assign report: %w", err)
	}

	return nil
}

// UnassignReport removes assignment from a report
func (s *dynamoDBStorage) UnassignReport(ctx context.Context, reportID string) error {
	now := time.Now()
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPORT#%s", reportID)},
		"SK": &types.AttributeValueMemberS{Value: "REPORT"},
	}

	// Remove AssignedTo field
	updateExpression := "REMOVE AssignedTo SET UpdatedAt = :now"
	expressionAttributeValues := map[string]types.AttributeValue{
		":now": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
	}

	// Add condition to ensure report exists
	conditionExpression := "attribute_exists(PK)"

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.tableName),
		Key:                       key,
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeValues: expressionAttributeValues,
		ConditionExpression:       aws.String(conditionExpression),
	})
	if err != nil {
		// Check if the error is because the report doesn't exist
		var ccfe *types.ConditionalCheckFailedException
		if errors.As(err, &ccfe) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to unassign report: %w", err)
	}

	return nil
}
