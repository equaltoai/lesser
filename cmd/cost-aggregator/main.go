package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

var (
	logger      *zap.Logger
	costStorage *cost.Storage
	cfg         *config.Config
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	// Initialize AWS clients
	ctx := context.Background()
	awsCfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(cfg.Region),
	)
	if err != nil {
		logger.Fatal("failed to load AWS config", zap.Error(err))
	}

	// Create DynamoDB client for cost storage
	dynamoClient := dynamodb.NewFromConfig(awsCfg)

	// Get cost history table name from environment
	costTableName := os.Getenv("COST_HISTORY_TABLE_NAME")
	if costTableName == "" {
		logger.Fatal("COST_HISTORY_TABLE_NAME environment variable not set")
	}

	costStorage = cost.NewStorage(dynamoClient, costTableName, logger)
}

func handleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error {
	logger.Info("Processing DynamoDB stream event",
		zap.Int("records", len(event.Records)),
	)

	dailyAggregates := make(map[string]*cost.DailyCostAggregate)
	monthlyAggregates := make(map[string]*cost.MonthlyCostAggregate)

	for _, record := range event.Records {
		// Only process INSERT events for operation costs
		if record.EventName != "INSERT" {
			continue
		}

		// Check if this is an operation cost record
		if pk, ok := record.Change.NewImage["PK"]; ok {
			if pk.DataType() == events.DataTypeString && pk.String() != "" {
				// Look for COST#YYYY-MM-DD pattern
				pkStr := pk.String()
				if len(pkStr) > 5 && pkStr[:5] == "COST#" {
					processCostRecord(record.Change.NewImage, dailyAggregates, monthlyAggregates)
				}
			}
		}
	}

	// Save aggregates
	for _, aggregate := range dailyAggregates {
		if err := costStorage.SaveDailyAggregate(ctx, aggregate); err != nil {
			logger.Error("failed to save daily aggregate",
				zap.String("date", aggregate.Date),
				zap.Error(err),
			)
		}
	}

	for _, aggregate := range monthlyAggregates {
		if err := costStorage.SaveMonthlyAggregate(ctx, aggregate); err != nil {
			logger.Error("failed to save monthly aggregate",
				zap.Int("year", aggregate.Year),
				zap.Int("month", aggregate.Month),
				zap.Error(err),
			)
		}
	}

	logger.Info("Processed cost aggregates",
		zap.Int("daily", len(dailyAggregates)),
		zap.Int("monthly", len(monthlyAggregates)),
	)

	return nil
}

func processCostRecord(
	item map[string]events.DynamoDBAttributeValue,
	dailyAggregates map[string]*cost.DailyCostAggregate,
	monthlyAggregates map[string]*cost.MonthlyCostAggregate,
) {
	// Extract timestamp
	var timestamp time.Time
	if ts, ok := item["Timestamp"]; ok && ts.DataType() == events.DataTypeString {
		timestamp, _ = time.Parse(time.RFC3339, ts.String())
	}

	if timestamp.IsZero() {
		return
	}

	// Extract cost data
	var totalCost int64
	var dynamoReads int64
	var dynamoWrites int64
	var lambdaInvocations int64
	var lambdaDurationMs int64
	var dataTransferBytes int64

	if v, ok := item["TotalCostMicrocents"]; ok && v.DataType() == events.DataTypeNumber {
		fmt.Sscanf(v.Number(), "%d", &totalCost)
	}
	if v, ok := item["DynamoDBReads"]; ok && v.DataType() == events.DataTypeNumber {
		fmt.Sscanf(v.Number(), "%d", &dynamoReads)
	}
	if v, ok := item["DynamoDBWrites"]; ok && v.DataType() == events.DataTypeNumber {
		fmt.Sscanf(v.Number(), "%d", &dynamoWrites)
	}
	if v, ok := item["LambdaInvocations"]; ok && v.DataType() == events.DataTypeNumber {
		fmt.Sscanf(v.Number(), "%d", &lambdaInvocations)
	}
	if v, ok := item["LambdaDurationMs"]; ok && v.DataType() == events.DataTypeNumber {
		fmt.Sscanf(v.Number(), "%d", &lambdaDurationMs)
	}
	if v, ok := item["DataTransferBytes"]; ok && v.DataType() == events.DataTypeNumber {
		fmt.Sscanf(v.Number(), "%d", &dataTransferBytes)
	}

	// Update daily aggregate
	dateKey := timestamp.Format("2006-01-02")
	if _, exists := dailyAggregates[dateKey]; !exists {
		dailyAggregates[dateKey] = &cost.DailyCostAggregate{
			Date: dateKey,
		}
	}

	daily := dailyAggregates[dateKey]
	daily.TotalCostMicrocents += totalCost
	daily.RequestCount++
	daily.DynamoDBReads += dynamoReads
	daily.DynamoDBWrites += dynamoWrites
	daily.LambdaInvocations += lambdaInvocations
	daily.LambdaDurationMs += lambdaDurationMs
	daily.DataTransferBytes += dataTransferBytes

	// Update monthly aggregate
	monthKey := timestamp.Format("2006-01")
	if _, exists := monthlyAggregates[monthKey]; !exists {
		monthlyAggregates[monthKey] = &cost.MonthlyCostAggregate{
			Year:  timestamp.Year(),
			Month: int(timestamp.Month()),
		}
	}

	monthly := monthlyAggregates[monthKey]
	monthly.TotalCostMicrocents += totalCost
	monthly.RequestCount++
	monthly.DynamoDBReads += dynamoReads
	monthly.DynamoDBWrites += dynamoWrites
	monthly.LambdaInvocations += lambdaInvocations
	monthly.LambdaDurationMs += lambdaDurationMs
	monthly.DataTransferGB += float64(dataTransferBytes) / (1024 * 1024 * 1024)

	// Calculate projected monthly cost based on current run rate
	now := time.Now()
	daysInMonth := daysInMonth(now.Year(), int(now.Month()))
	dayOfMonth := now.Day()
	if dayOfMonth > 0 {
		projectionFactor := float64(daysInMonth) / float64(dayOfMonth)
		monthly.ProjectedCostMicrocents = int64(float64(monthly.TotalCostMicrocents) * projectionFactor)
	}
}

func daysInMonth(year int, month int) int {
	// Get the first day of the next month
	firstOfNext := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC)
	// Subtract one day to get the last day of the current month
	lastOfCurrent := firstOfNext.AddDate(0, 0, -1)
	return lastOfCurrent.Day()
}

// Alternative handler for periodic aggregation (can be triggered by EventBridge)
func handlePeriodicAggregation(ctx context.Context, event events.CloudWatchEvent) error {
	logger.Info("Running periodic cost aggregation")

	// Query cost data for the current day
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Query all cost records for today
	items, err := costStorage.QueryCostsByDate(ctx, startOfDay.Format("2006-01-02"))
	if err != nil {
		logger.Error("failed to query cost data", zap.Error(err))
		return err
	}

	// Process and aggregate the results
	aggregate := &cost.DailyCostAggregate{
		Date: startOfDay.Format("2006-01-02"),
	}

	uniqueUsers := make(map[string]bool)

	for _, item := range items {
		// Parse cost data from each item
		var totalCost int64
		var dynamoReads int64
		var dynamoWrites int64
		var lambdaInvocations int64
		var lambdaDurationMs int64
		var dataTransferBytes int64

		if v, ok := item["TotalCostMicrocents"].(*types.AttributeValueMemberN); ok {
			fmt.Sscanf(v.Value, "%d", &totalCost)
		}
		if v, ok := item["DynamoDBReads"].(*types.AttributeValueMemberN); ok {
			fmt.Sscanf(v.Value, "%d", &dynamoReads)
		}
		if v, ok := item["DynamoDBWrites"].(*types.AttributeValueMemberN); ok {
			fmt.Sscanf(v.Value, "%d", &dynamoWrites)
		}
		if v, ok := item["LambdaInvocations"].(*types.AttributeValueMemberN); ok {
			fmt.Sscanf(v.Value, "%d", &lambdaInvocations)
		}
		if v, ok := item["LambdaDurationMs"].(*types.AttributeValueMemberN); ok {
			fmt.Sscanf(v.Value, "%d", &lambdaDurationMs)
		}
		if v, ok := item["DataTransferBytes"].(*types.AttributeValueMemberN); ok {
			fmt.Sscanf(v.Value, "%d", &dataTransferBytes)
		}

		// Track unique users if user ID is available
		if v, ok := item["UserID"].(*types.AttributeValueMemberS); ok && v.Value != "" {
			uniqueUsers[v.Value] = true
		}

		aggregate.TotalCostMicrocents += totalCost
		aggregate.RequestCount++
		aggregate.DynamoDBReads += dynamoReads
		aggregate.DynamoDBWrites += dynamoWrites
		aggregate.LambdaInvocations += lambdaInvocations
		aggregate.LambdaDurationMs += lambdaDurationMs
		aggregate.DataTransferBytes += dataTransferBytes
	}

	aggregate.UniqueUsers = int64(len(uniqueUsers))

	// Save the aggregate
	if err := costStorage.SaveDailyAggregate(ctx, aggregate); err != nil {
		logger.Error("failed to save daily aggregate", zap.Error(err))
		return err
	}

	logger.Info("Saved daily cost aggregate",
		zap.String("date", aggregate.Date),
		zap.Int64("total_microcents", aggregate.TotalCostMicrocents),
		zap.Int64("requests", aggregate.RequestCount),
		zap.Int64("unique_users", aggregate.UniqueUsers),
	)

	return nil
}

func main() {
	// Determine which handler to use based on event source
	// This allows the same Lambda to handle both DynamoDB streams and EventBridge
	lambda.Start(func(ctx context.Context, event interface{}) error {
		switch e := event.(type) {
		case events.DynamoDBEvent:
			return handleDynamoDBStream(ctx, e)
		case events.CloudWatchEvent:
			return handlePeriodicAggregation(ctx, e)
		default:
			logger.Error("unknown event type", zap.Any("event", event))
			return fmt.Errorf("unknown event type")
		}
	})
}
