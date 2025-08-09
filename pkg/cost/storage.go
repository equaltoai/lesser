package cost

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// Storage handles persistence of cost data
type Storage struct {
	client    *dynamodb.Client
	tableName string
	logger    *zap.Logger
}

// NewStorage creates a new cost storage instance
func NewStorage(client *dynamodb.Client, tableName string, logger *zap.Logger) *Storage {
	return &Storage{
		client:    client,
		tableName: tableName,
		logger:    logger,
	}
}

// SaveOperationCost saves a single operation cost to DynamoDB
func (s *Storage) SaveOperationCost(ctx context.Context, cost *OperationCost) error {
	// Use composite keys for efficient querying
	// PK: COST#YYYY-MM-DD
	// SK: TIMESTAMP#REQUEST_ID

	date := cost.Timestamp.Format(common.DateFormat)
	pk := fmt.Sprintf("COST#%s", date)
	sk := fmt.Sprintf("%d#%s", cost.Timestamp.UnixNano(), cost.RequestID)

	// Also store in GSI for monthly aggregation
	// GSI1PK: COST#YYYY-MM
	// GSI1SK: TIMESTAMP
	month := cost.Timestamp.Format(common.MonthFormat)
	gsi1pk := fmt.Sprintf("COST#%s", month)
	gsi1sk := fmt.Sprintf("%d", cost.Timestamp.UnixNano())

	item := map[string]types.AttributeValue{
		"PK":                  &types.AttributeValueMemberS{Value: pk},
		"SK":                  &types.AttributeValueMemberS{Value: sk},
		"GSI1PK":              &types.AttributeValueMemberS{Value: gsi1pk},
		"GSI1SK":              &types.AttributeValueMemberS{Value: gsi1sk},
		"RequestID":           &types.AttributeValueMemberS{Value: cost.RequestID},
		"OperationType":       &types.AttributeValueMemberS{Value: cost.OperationType},
		"Timestamp":           &types.AttributeValueMemberS{Value: cost.Timestamp.Format(time.RFC3339)},
		"TotalCostMicrocents": &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.TotalCostMicroCents, 10)},
		"DynamoDBReads":       &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.DynamoDBReads, 10)},
		"DynamoDBWrites":      &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.DynamoDBWrites, 10)},
		"LambdaInvocations":   &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.LambdaInvocations, 10)},
		"LambdaDurationMs":    &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.LambdaDurationMs, 10)},
		"LambdaMemoryMB":      &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.LambdaMemoryMB, 10)},
		"S3Gets":              &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.S3Gets, 10)},
		"S3Puts":              &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.S3Puts, 10)},
		"DataTransferBytes":   &types.AttributeValueMemberN{Value: strconv.FormatInt(cost.DataTransferBytes, 10)},
		"Type":                &types.AttributeValueMemberS{Value: "OPERATION"},
	}

	// Set TTL to 90 days
	ttl := cost.Timestamp.Add(90 * 24 * time.Hour).Unix()
	item["TTL"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)}

	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to save operation cost",
				zap.String("request_id", cost.RequestID),
				zap.Error(err),
			)
		}
		return fmt.Errorf("failed to save operation cost: %w", err)
	}

	return nil
}

// GetDailyCosts retrieves daily cost aggregates for a date range
func (s *Storage) GetDailyCosts(ctx context.Context, startDate, endDate time.Time) ([]DailyCostAggregate, error) {
	var results []DailyCostAggregate

	// Query each day in the range
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		date := d.Format(common.DateFormat)
		pk := fmt.Sprintf("COST_DAILY#%s", date)

		result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: pk},
				"SK": &types.AttributeValueMemberS{Value: "AGGREGATE"},
			},
		})

		if err != nil {
			if s.logger != nil {
				s.logger.Error("failed to get daily cost",
					zap.String("date", date),
					zap.Error(err),
				)
			}
			continue
		}

		if result.Item == nil {
			continue
		}

		var aggregate DailyCostAggregate
		if err := unmarshalDailyCostAggregate(result.Item, &aggregate); err != nil {
			if s.logger != nil {
				s.logger.Error("failed to unmarshal daily cost",
					zap.String("date", date),
					zap.Error(err),
				)
			}
			continue
		}

		results = append(results, aggregate)
	}

	return results, nil
}

// GetMonthlyCost retrieves the monthly cost aggregate
func (s *Storage) GetMonthlyCost(ctx context.Context, year int, month time.Month) (*MonthlyCostAggregate, error) {
	pk := fmt.Sprintf("COST_MONTHLY#%04d-%02d", year, month)

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: "AGGREGATE"},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get monthly cost: %w", err)
	}

	if result.Item == nil {
		// Return empty aggregate if none exists
		return &MonthlyCostAggregate{
			Year:  year,
			Month: int(month),
		}, nil
	}

	var aggregate MonthlyCostAggregate
	if err := unmarshalMonthlyCostAggregate(result.Item, &aggregate); err != nil {
		return nil, fmt.Errorf("failed to unmarshal monthly cost: %w", err)
	}

	return &aggregate, nil
}

// SaveDailyAggregate saves a daily cost aggregate
func (s *Storage) SaveDailyAggregate(ctx context.Context, aggregate *DailyCostAggregate) error {
	pk := fmt.Sprintf("COST_DAILY#%s", aggregate.Date)

	item := map[string]types.AttributeValue{
		"PK":                  &types.AttributeValueMemberS{Value: pk},
		"SK":                  &types.AttributeValueMemberS{Value: "AGGREGATE"},
		"Date":                &types.AttributeValueMemberS{Value: aggregate.Date},
		"TotalCostMicrocents": &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.TotalCostMicrocents, 10)},
		"RequestCount":        &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.RequestCount, 10)},
		"UniqueUsers":         &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.UniqueUsers, 10)},
		"DynamoDBReads":       &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.DynamoDBReads, 10)},
		"DynamoDBWrites":      &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.DynamoDBWrites, 10)},
		"LambdaInvocations":   &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.LambdaInvocations, 10)},
		"LambdaDurationMs":    &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.LambdaDurationMs, 10)},
		"DataTransferBytes":   &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.DataTransferBytes, 10)},
		"Type":                &types.AttributeValueMemberS{Value: "DAILY_AGGREGATE"},
		"UpdatedAt":           &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	return err
}

// SaveMonthlyAggregate saves a monthly cost aggregate
func (s *Storage) SaveMonthlyAggregate(ctx context.Context, aggregate *MonthlyCostAggregate) error {
	pk := fmt.Sprintf("COST_MONTHLY#%04d-%02d", aggregate.Year, aggregate.Month)

	item := map[string]types.AttributeValue{
		"PK":                      &types.AttributeValueMemberS{Value: pk},
		"SK":                      &types.AttributeValueMemberS{Value: "AGGREGATE"},
		"Year":                    &types.AttributeValueMemberN{Value: strconv.Itoa(aggregate.Year)},
		"Month":                   &types.AttributeValueMemberN{Value: strconv.Itoa(aggregate.Month)},
		"TotalCostMicrocents":     &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.TotalCostMicrocents, 10)},
		"ProjectedCostMicrocents": &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.ProjectedCostMicrocents, 10)},
		"RequestCount":            &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.RequestCount, 10)},
		"UniqueUsers":             &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.UniqueUsers, 10)},
		"DynamoDBReads":           &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.DynamoDBReads, 10)},
		"DynamoDBWrites":          &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.DynamoDBWrites, 10)},
		"LambdaInvocations":       &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.LambdaInvocations, 10)},
		"LambdaDurationMs":        &types.AttributeValueMemberN{Value: strconv.FormatInt(aggregate.LambdaDurationMs, 10)},
		"DataTransferGB":          &types.AttributeValueMemberN{Value: fmt.Sprintf("%.6f", aggregate.DataTransferGB)},
		"Type":                    &types.AttributeValueMemberS{Value: "MONTHLY_AGGREGATE"},
		"UpdatedAt":               &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	return err
}

// QueryCostsByDate queries cost records for a specific date
func (s *Storage) QueryCostsByDate(ctx context.Context, date string) ([]map[string]types.AttributeValue, error) {
	pk := fmt.Sprintf("COST#%s", date)

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query costs by date: %w", err)
	}

	return result.Items, nil
}

// DailyCostAggregate represents aggregated costs for a single day
type DailyCostAggregate struct {
	Date                string
	TotalCostMicrocents int64
	RequestCount        int64
	UniqueUsers         int64
	DynamoDBReads       int64
	DynamoDBWrites      int64
	LambdaInvocations   int64
	LambdaDurationMs    int64
	DataTransferBytes   int64
}

// MonthlyCostAggregate represents aggregated costs for a month
type MonthlyCostAggregate struct {
	Year                    int
	Month                   int
	TotalCostMicrocents     int64
	ProjectedCostMicrocents int64
	RequestCount            int64
	UniqueUsers             int64
	DynamoDBReads           int64
	DynamoDBWrites          int64
	LambdaInvocations       int64
	LambdaDurationMs        int64
	DataTransferGB          float64
}

// Helper functions for unmarshaling
func unmarshalDailyCostAggregate(item map[string]types.AttributeValue, aggregate *DailyCostAggregate) error {
	if v, ok := item["Date"].(*types.AttributeValueMemberS); ok {
		aggregate.Date = v.Value
	}
	if v, ok := item["TotalCostMicrocents"].(*types.AttributeValueMemberN); ok {
		aggregate.TotalCostMicrocents, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["RequestCount"].(*types.AttributeValueMemberN); ok {
		aggregate.RequestCount, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["UniqueUsers"].(*types.AttributeValueMemberN); ok {
		aggregate.UniqueUsers, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["DynamoDBReads"].(*types.AttributeValueMemberN); ok {
		aggregate.DynamoDBReads, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["DynamoDBWrites"].(*types.AttributeValueMemberN); ok {
		aggregate.DynamoDBWrites, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["LambdaInvocations"].(*types.AttributeValueMemberN); ok {
		aggregate.LambdaInvocations, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["LambdaDurationMs"].(*types.AttributeValueMemberN); ok {
		aggregate.LambdaDurationMs, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["DataTransferBytes"].(*types.AttributeValueMemberN); ok {
		aggregate.DataTransferBytes, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	return nil
}

func unmarshalMonthlyCostAggregate(item map[string]types.AttributeValue, aggregate *MonthlyCostAggregate) error {
	if v, ok := item["Year"].(*types.AttributeValueMemberN); ok {
		aggregate.Year, _ = strconv.Atoi(v.Value)
	}
	if v, ok := item["Month"].(*types.AttributeValueMemberN); ok {
		aggregate.Month, _ = strconv.Atoi(v.Value)
	}
	if v, ok := item["TotalCostMicrocents"].(*types.AttributeValueMemberN); ok {
		aggregate.TotalCostMicrocents, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["ProjectedCostMicrocents"].(*types.AttributeValueMemberN); ok {
		aggregate.ProjectedCostMicrocents, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["RequestCount"].(*types.AttributeValueMemberN); ok {
		aggregate.RequestCount, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["UniqueUsers"].(*types.AttributeValueMemberN); ok {
		aggregate.UniqueUsers, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["DynamoDBReads"].(*types.AttributeValueMemberN); ok {
		aggregate.DynamoDBReads, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["DynamoDBWrites"].(*types.AttributeValueMemberN); ok {
		aggregate.DynamoDBWrites, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["LambdaInvocations"].(*types.AttributeValueMemberN); ok {
		aggregate.LambdaInvocations, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["LambdaDurationMs"].(*types.AttributeValueMemberN); ok {
		aggregate.LambdaDurationMs, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["DataTransferGB"].(*types.AttributeValueMemberN); ok {
		aggregate.DataTransferGB, _ = strconv.ParseFloat(v.Value, 64)
	}
	return nil
}
