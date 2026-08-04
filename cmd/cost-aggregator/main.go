// Package main implements the cost-aggregator Lambda function for aggregating and analyzing AWS costs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// CostAggregator implements the DynamoDBStreamHandler interface for Lift
type CostAggregator struct {
	db                     dynamormCore.DB
	tableName              string
	logger                 *zap.Logger
	costTrackingRepository costTrackingStore
	snsClient              snsPublisher
	cloudWatchClient       cloudWatchPutter
	sqsClient              sqsSender
	lambdaClient           lambdaInvoker
	cfg                    *config.Config
	lambdaCtx              *common.LambdaContext
}

type costTrackingStore interface {
	CreateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error
	Create(ctx context.Context, tracking *models.DynamoDBCostRecord) error
	Aggregate(ctx context.Context, operationType, period string, windowStart, windowEnd time.Time) error
	GetAggregated(ctx context.Context, period, operationType string, windowStart time.Time) (*models.DynamoDBCostAggregation, error)
	GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error)
}

type snsPublisher interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type cloudWatchPutter interface {
	PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error)
}

type sqsSender interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

type lambdaInvoker interface {
	Invoke(ctx context.Context, params *awslambda.InvokeInput, optFns ...func(*awslambda.Options)) (*awslambda.InvokeOutput, error)
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	processor *CostAggregator
)

// AggregationEvent represents the input for cost aggregation
type AggregationEvent struct {
	Type           string    `json:"type"` // "hourly", "daily", "monthly"
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime"`
	OperationTypes []string  `json:"operationTypes,omitempty"` // Optional: specific operations to aggregate
	Tables         []string  `json:"tables,omitempty"`         // Optional: specific tables to aggregate
}

// CostStreamRecord represents a cost tracking record from DynamoDB streams
type CostStreamRecord struct {
	OperationType      string
	TableName          string
	ReadCapacityUnits  float64
	WriteCapacityUnits float64
	ItemCount          int
	RequestDuration    int64
	ServiceName        string
	Timestamp          time.Time
}

// CostAlert represents a cost threshold alert
type CostAlert struct {
	Type          string                 `json:"type"` // "threshold_exceeded", "high_operation"
	OperationType string                 `json:"operationType"`
	Period        string                 `json:"period"`
	Cost          float64                `json:"cost"`
	Threshold     float64                `json:"threshold"`
	Operations    int64                  `json:"operations"`
	Timestamp     time.Time              `json:"timestamp"`
	Severity      string                 `json:"severity"` // "warning", "critical", "emergency"
	Message       string                 `json:"message"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeCostAggregator()
}

var (
	mustInitializeLambdaFn     = common.MustInitializeLambda
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newCostTrackingRepoFn      = func(db dynamormCore.DB, tableName string, logger *zap.Logger) costTrackingStore {
		return repositories.NewTrackingRepository(db, tableName, logger, nil)
	}
	lambdaStartFn = lambda.Start
)

func initializeCostAggregator() {
	// Standardized Lambda initialization for cost-aggregator function
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "cost-aggregator",          // cost-aggregator
		LambdaType:  common.LambdaTypeProcessor, // These are background processing functions
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger

	// Function-specific initialization only
	tableName := cfg.DynamoTableName

	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName: "cost-aggregator",
		NewDB:       newLambdaOptimizedClientFn,
	})
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	costTrackingRepository := newCostTrackingRepoFn(deps.DB, tableName, logger)

	var snsClient snsPublisher
	var cloudWatchClient cloudWatchPutter
	var sqsClient sqsSender
	var lambdaClient lambdaInvoker
	if lambdaCtx.AWSServices != nil {
		if lambdaCtx.AWSServices.SNS != nil {
			snsClient = lambdaCtx.AWSServices.SNS
		}
		if lambdaCtx.AWSServices.CloudWatch != nil {
			cloudWatchClient = lambdaCtx.AWSServices.CloudWatch
		}
		if lambdaCtx.AWSServices.SQS != nil {
			sqsClient = lambdaCtx.AWSServices.SQS
		}
		if lambdaCtx.AWSServices.Lambda != nil {
			lambdaClient = lambdaCtx.AWSServices.Lambda
		}
	}

	processor = &CostAggregator{
		db:                     deps.DB,
		tableName:              tableName,
		logger:                 logger,
		costTrackingRepository: costTrackingRepository,
		snsClient:              snsClient,
		cloudWatchClient:       cloudWatchClient,
		sqsClient:              sqsClient,
		lambdaClient:           lambdaClient,
		cfg:                    cfg,
		lambdaCtx:              lambdaCtx,
	}
}

func main() {
	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	tableName := naming.ResourceNameWithApp(appName, "main-table", stage)

	app.DynamoDB(tableName, handleCostAggregatorStreamRecord)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func handleCostAggregatorStreamRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	if processor == nil {
		return fmt.Errorf("cost aggregator processor not initialized")
	}
	return processor.HandleDynamoDBRecord(ctx, record)
}

func (ca *CostAggregator) HandleDynamoDBRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) (err error) {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}

	if ca.logger == nil {
		ca.logger = zap.NewNop()
	}

	defer func() {
		if r := recover(); r != nil {
			ca.logger.Error("panic processing cost tracking stream record",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.Any("panic", r),
			)
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	// Process INSERT and MODIFY events that might contain cost information.
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	costRecord, extractErr := ca.extractCostFromStreamRecord(record)
	if extractErr != nil {
		ca.logger.Warn("failed to extract cost from stream record",
			zap.String("request_id", requestID),
			zap.String("event_id", record.EventID),
			zap.Error(extractErr),
		)
		return nil
	}
	if costRecord == nil {
		return nil
	}

	return ca.processRealtimeCosts(runCtx, requestID, []*models.DynamoDBCostRecord{costRecord})
}

// HandleCloudWatchEvent handles scheduled aggregation events
// This method can be called separately for scheduled aggregation tasks
func (ca *CostAggregator) HandleCloudWatchEvent(ctx context.Context, event events.CloudWatchEvent) error {
	ca.logger.Info("Processing CloudWatch scheduled event",
		zap.String("detail_type", event.DetailType),
		zap.String("source", event.Source))

	// Parse the aggregation configuration from the event
	var aggEvent AggregationEvent
	if err := json.Unmarshal(event.Detail, &aggEvent); err != nil {
		// Default to hourly aggregation for scheduled events
		now := time.Now()
		aggEvent = AggregationEvent{
			Type:      "hourly",
			StartTime: now.Add(-1 * time.Hour).Truncate(time.Hour),
			EndTime:   now.Truncate(time.Hour),
		}
	}

	return ca.handleAggregationEvent(ctx, aggEvent)
}

// Note: handleDynamoDBStream is now replaced by HandleStream method above

func (ca *CostAggregator) handleAggregationEvent(ctx context.Context, event AggregationEvent) error {
	ca.logger.Info("Processing cost aggregation event",
		zap.String("type", event.Type),
		zap.Time("start_time", event.StartTime),
		zap.Time("end_time", event.EndTime),
		zap.Strings("operation_types", event.OperationTypes),
		zap.Strings("tables", event.Tables))

	// Determine what to aggregate
	operationTypes := event.OperationTypes
	if err := common.ValidateSliceNotEmpty("operationTypes", operationTypes); err != nil {
		// Default to all operation types
		operationTypes = []string{
			"GetItem", "PutItem", "UpdateItem", "DeleteItem",
			"Query", "Scan", "BatchGetItem", "BatchWriteItem",
			"TransactGetItems", "TransactWriteItems",
		}
	}

	// Perform aggregation for each operation type
	for _, opType := range operationTypes {
		if err := ca.aggregateCosts(ctx, opType, event.Type, event.StartTime, event.EndTime); err != nil {
			ca.logger.Error("failed to aggregate costs",
				zap.String("operation_type", opType),
				zap.String("period", event.Type),
				zap.Error(err))
			// Continue with other aggregations
		}
	}

	// Trigger next level aggregation if applicable
	switch event.Type {
	case "hourly":
		// Check if we should trigger daily aggregation
		if event.EndTime.Hour() == 0 {
			dailyEvent := AggregationEvent{
				Type:           "daily",
				StartTime:      event.EndTime.Add(-24 * time.Hour),
				EndTime:        event.EndTime,
				OperationTypes: operationTypes,
			}
			if err := ca.triggerAggregation(ctx, dailyEvent); err != nil {
				ca.logger.Warn("failed to trigger daily aggregation", zap.Error(err))
			}
		}
	case "daily":
		// Check if we should trigger monthly aggregation
		if event.EndTime.Day() == 1 {
			monthlyEvent := AggregationEvent{
				Type:           "monthly",
				StartTime:      event.EndTime.AddDate(0, -1, 0),
				EndTime:        event.EndTime,
				OperationTypes: operationTypes,
			}
			if err := ca.triggerAggregation(ctx, monthlyEvent); err != nil {
				ca.logger.Warn("failed to trigger monthly aggregation", zap.Error(err))
			}
		}
	}

	// Generate cost alerts if thresholds are exceeded
	ca.checkCostAlerts(ctx, event.Type, event.StartTime, event.EndTime)

	return nil
}

func (ca *CostAggregator) extractCostFromStreamRecord(record events.DynamoDBEventRecord) (*models.DynamoDBCostRecord, error) {
	// Look for cost tracking information in the stream record
	// This could be from:
	// 1. Direct cost tracking records
	// 2. Consumed capacity information attached to other operations

	var image map[string]events.DynamoDBAttributeValue
	if record.EventName == "INSERT" {
		image = record.Change.NewImage
	} else {
		image = record.Change.NewImage
	}
	if image == nil {
		return nil, nil
	}

	// Check if this is a cost tracking record
	pk, pkExists := image["PK"]
	if pkExists && pk.DataType() == events.DataTypeString && ca.isCostRecord(pk.String()) {
		return ca.extractCostTrackingRecord(image)
	}

	// Check for embedded cost information in other records
	// Look for ConsumedCapacity field that might be stored
	if consumedCapacity, exists := image["ConsumedCapacity"]; exists {
		return ca.extractEmbeddedCostInfo(image, consumedCapacity)
	}

	return nil, nil
}

func (ca *CostAggregator) isCostRecord(pk string) bool {
	// Check if this is a cost tracking record based on PK pattern
	return len(pk) > 5 && pk[:5] == "cost#"
}

func (ca *CostAggregator) extractCostTrackingRecord(image map[string]events.DynamoDBAttributeValue) (*models.DynamoDBCostRecord, error) {
	// For cost tracking records, use manual extraction since the image format might be different
	// than what stream.UnmarshalItem expects
	tracking := &models.DynamoDBCostRecord{}

	// Extract fields from DynamoDB image
	if operationType, ok := getAttribute(image, "operation_type"); ok {
		tracking.OperationType = operationType
	}

	if tableName, ok := getAttribute(image, "table_name"); ok {
		tracking.Table = tableName
	}

	if readUnits, ok := getNumberAttribute(image, "read_capacity_units"); ok {
		tracking.ReadCapacityUnits = readUnits
	}

	if writeUnits, ok := getNumberAttribute(image, "write_capacity_units"); ok {
		tracking.WriteCapacityUnits = writeUnits
	}

	if itemCount, ok := getNumberAttribute(image, "item_count"); ok {
		tracking.ItemCount = int(itemCount)
	}

	if serviceName, ok := getAttribute(image, "service_name"); ok {
		tracking.ServiceName = serviceName
	}

	if timestamp, ok := getAttribute(image, "timestamp"); ok {
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			tracking.Timestamp = t
		}
	}

	// Calculate costs
	readCost, writeCost, totalCost := models.CalculateCost(tracking.ReadCapacityUnits, tracking.WriteCapacityUnits)
	tracking.ReadCostMicroCents = readCost
	tracking.WriteCostMicroCents = writeCost
	tracking.TotalCostMicroCents = totalCost

	return tracking, nil
}

func (ca *CostAggregator) extractEmbeddedCostInfo(image map[string]events.DynamoDBAttributeValue, consumedCapacity events.DynamoDBAttributeValue) (*models.DynamoDBCostRecord, error) {
	// Extract cost information embedded in other records
	// This would parse ConsumedCapacity structures that might be stored

	tracking := models.NewDynamoDBCostRecordBuilder()

	// Extract operation context
	if operationType, ok := getAttribute(image, "operation_type"); ok {
		tracking.ForOperation(operationType)
	}

	if tableName, ok := getAttribute(image, "table_name"); ok {
		tracking.OnTable(tableName)
	}

	// Parse consumed capacity (this would need proper structure parsing)
	// For now, we'll handle simple cases
	if consumedCapacity.DataType() == events.DataTypeMap && consumedCapacity.Map() != nil {
		capacityMap := consumedCapacity.Map()

		var readUnits, writeUnits float64
		if rcu, ok := getNumberFromMap(capacityMap, "ReadCapacityUnits"); ok {
			readUnits = rcu
		}
		if wcu, ok := getNumberFromMap(capacityMap, "WriteCapacityUnits"); ok {
			writeUnits = wcu
		}

		tracking.WithCapacityUnits(readUnits, writeUnits)

		// Calculate costs
		readCost, writeCost, _ := models.CalculateCost(readUnits, writeUnits)
		tracking.WithCostMicroCents(readCost, writeCost)
	}

	return tracking.Build(), nil
}

func (ca *CostAggregator) processRealtimeCosts(ctx context.Context, requestID string, costs []*models.DynamoDBCostRecord) error {
	// Create or update minute-level aggregations in real-time
	now := time.Now()
	windowStart := now.Truncate(time.Minute)
	windowEnd := windowStart.Add(time.Minute)

	// Group costs by operation type
	grouped := make(map[string][]*models.DynamoDBCostRecord)
	for _, cost := range costs {
		grouped[cost.OperationType] = append(grouped[cost.OperationType], cost)
	}

	// Create minute-level aggregations
	for opType, opCosts := range grouped {
		aggregated := &models.DynamoDBCostAggregation{
			Period:           "minute",
			OperationType:    opType,
			Table:            "all",
			WindowStart:      windowStart,
			WindowEnd:        windowEnd,
			TableBreakdown:   make(map[string]*models.DynamoDBTableCostStats),
			ServiceBreakdown: make(map[string]*models.DynamoDBServiceCostStats),
		}

		// Aggregate the costs
		for _, cost := range opCosts {
			aggregated.TotalOperations++
			aggregated.TotalReadCapacityUnits += cost.ReadCapacityUnits
			aggregated.TotalWriteCapacityUnits += cost.WriteCapacityUnits
			aggregated.TotalReadCostMicroCents += cost.ReadCostMicroCents
			aggregated.TotalWriteCostMicroCents += cost.WriteCostMicroCents
			aggregated.TotalCostMicroCents += cost.TotalCostMicroCents
			aggregated.TotalItemCount += int64(cost.ItemCount)
			aggregated.AverageDuration += float64(cost.RequestDuration)

			// Update table breakdown
			if tableStats, exists := aggregated.TableBreakdown[cost.Table]; exists {
				tableStats.OperationCount++
				tableStats.ReadCapacityUnits += cost.ReadCapacityUnits
				tableStats.WriteCapacityUnits += cost.WriteCapacityUnits
				tableStats.TotalCostMicroCents += cost.TotalCostMicroCents
			} else {
				aggregated.TableBreakdown[cost.Table] = &models.DynamoDBTableCostStats{
					Table:               cost.Table,
					OperationCount:      1,
					ReadCapacityUnits:   cost.ReadCapacityUnits,
					WriteCapacityUnits:  cost.WriteCapacityUnits,
					TotalCostMicroCents: cost.TotalCostMicroCents,
				}
			}

			// Update service breakdown
			if cost.ServiceName != "" {
				if serviceStats, exists := aggregated.ServiceBreakdown[cost.ServiceName]; exists {
					serviceStats.OperationCount++
					serviceStats.TotalCostMicroCents += cost.TotalCostMicroCents
				} else {
					aggregated.ServiceBreakdown[cost.ServiceName] = &models.DynamoDBServiceCostStats{
						ServiceName:         cost.ServiceName,
						OperationCount:      1,
						TotalCostMicroCents: cost.TotalCostMicroCents,
					}
				}
			}
		}

		// Calculate averages
		if aggregated.TotalOperations > 0 {
			aggregated.AverageDuration = aggregated.AverageDuration / float64(aggregated.TotalOperations)
		}

		// Store or update the aggregation
		if err := ca.costTrackingRepository.CreateAggregated(ctx, aggregated); err != nil {
			ca.logger.Error("failed to create real-time aggregation",
				zap.String("request_id", requestID),
				zap.String("operation_type", opType),
				zap.Error(err))
		}
	}

	// Store the raw cost tracking records individually
	for _, cost := range costs {
		if err := ca.costTrackingRepository.Create(ctx, cost); err != nil {
			ca.logger.Error("failed to create cost tracking record",
				zap.String("request_id", requestID),
				zap.Error(err))
		}
	}

	return nil
}

func (ca *CostAggregator) aggregateCosts(ctx context.Context, operationType, period string, startTime, endTime time.Time) error {
	ca.logger.Debug("aggregating costs",
		zap.String("operation_type", operationType),
		zap.String("period", period))

	// Use repository's aggregation method
	if err := ca.costTrackingRepository.Aggregate(ctx, operationType, period, startTime, endTime); err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to aggregate costs")
	}

	// Get the aggregated data to log summary
	aggregated, err := ca.costTrackingRepository.GetAggregated(ctx, period, operationType, startTime)
	if err == nil && aggregated != nil {
		ca.logger.Info("cost aggregation completed",
			zap.String("operation_type", operationType),
			zap.String("period", period),
			zap.Int64("total_operations", aggregated.TotalOperations),
			zap.Float64("total_cost_dollars", aggregated.TotalCostDollars),
			zap.Float64("avg_cost_per_op", aggregated.AverageCostPerOperation))
	}

	return nil
}

func (ca *CostAggregator) checkCostAlerts(ctx context.Context, period string, startTime, endTime time.Time) {
	// Check for operations or periods that exceed cost thresholds
	// This is a simplified version - you would want configurable thresholds

	thresholds := map[string]float64{
		"hourly":  0.10,  // $0.10 per hour warning
		"daily":   2.00,  // $2.00 per day warning
		"monthly": 50.00, // $50.00 per month warning
	}

	threshold, exists := thresholds[period]
	if !exists {
		return
	}

	// Get all operation types
	operationTypes := []string{
		"GetItem", "PutItem", "UpdateItem", "DeleteItem",
		"Query", "Scan", "BatchGetItem", "BatchWriteItem",
		"TransactGetItems", "TransactWriteItems",
	}

	for _, opType := range operationTypes {
		aggregated, err := ca.costTrackingRepository.GetAggregated(ctx, period, opType, startTime)
		if err != nil {
			continue
		}

		if aggregated.TotalCostDollars > threshold {
			ca.logger.Warn("cost threshold exceeded",
				zap.String("operation_type", opType),
				zap.String("period", period),
				zap.Float64("cost", aggregated.TotalCostDollars),
				zap.Float64("threshold", threshold),
				zap.Int64("operations", aggregated.TotalOperations))

			// Send alert via multiple channels
			ca.sendCostAlert(ctx, CostAlert{
				Type:          "threshold_exceeded",
				OperationType: opType,
				Period:        period,
				Cost:          aggregated.TotalCostDollars,
				Threshold:     threshold,
				Operations:    aggregated.TotalOperations,
				Timestamp:     time.Now(),
				Severity:      ca.determineSeverity(aggregated.TotalCostDollars, threshold),
			})
		}
	}

	// Check high-cost individual operations
	highCostOps, err := ca.costTrackingRepository.GetHighCostOperations(ctx, 0.01, startTime, endTime, 10)
	if err == nil && len(highCostOps) > 0 {
		for _, op := range highCostOps {
			ca.logger.Warn("high cost operation detected",
				zap.String("operation_type", op.OperationType),
				zap.String("table", op.Table),
				zap.Float64("cost", op.EstimatedCostDollars),
				zap.Int("item_count", op.ItemCount),
				zap.String("service", op.ServiceName))
		}
	}
}

func (ca *CostAggregator) sendCostAlert(ctx context.Context, alert CostAlert) {
	// Create alert message
	alert.Message = fmt.Sprintf("DynamoDB cost threshold exceeded: %s operation costs $%.4f in %s period (threshold: $%.2f)",
		alert.OperationType, alert.Cost, alert.Period, alert.Threshold)

	// Add additional details
	alert.Details = map[string]interface{}{
		"region":           ca.cfg.Region,
		"table":            ca.cfg.DynamoTableName,
		"cost_per_op":      alert.Cost / float64(alert.Operations),
		"operations_count": alert.Operations,
		"alert_id":         fmt.Sprintf("%s-%s-%d", alert.OperationType, alert.Period, alert.Timestamp.Unix()),
	}

	ca.logger.Info("Sending cost alert",
		zap.String("type", alert.Type),
		zap.String("severity", alert.Severity),
		zap.Float64("cost", alert.Cost),
		zap.String("message", alert.Message))

	// 1. Send SNS notification for immediate alerts
	if err := ca.sendSNSAlert(ctx, alert); err != nil {
		ca.logger.Warn("Failed to send SNS alert", zap.Error(err))
	}

	// 2. Put CloudWatch metric for monitoring
	if err := ca.putCloudWatchMetric(ctx, alert); err != nil {
		ca.logger.Warn("Failed to put CloudWatch metric", zap.Error(err))
	}

	// 3. Log structured alert for log-based monitoring
	ca.logger.Warn("COST_ALERT",
		zap.String("alert_type", alert.Type),
		zap.String("operation_type", alert.OperationType),
		zap.String("period", alert.Period),
		zap.Float64("cost_dollars", alert.Cost),
		zap.Float64("threshold_dollars", alert.Threshold),
		zap.Int64("operation_count", alert.Operations),
		zap.String("severity", alert.Severity),
		zap.String("alert_id", alert.Details["alert_id"].(string)),
		zap.Time("timestamp", alert.Timestamp))
}

func (ca *CostAggregator) sendSNSAlert(ctx context.Context, alert CostAlert) error {
	if ca.snsClient == nil {
		return pkgErrors.NewAppError(pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "SNS client not configured")
	}

	// Construct SNS topic ARN - assuming standard naming convention
	topicARN := fmt.Sprintf("arn:aws:sns:%s:%s:cost-alerts", ca.cfg.Region, ca.cfg.AWSAccountID)

	// Create message with structured data
	message := map[string]interface{}{
		"alert":     alert,
		"timestamp": alert.Timestamp.Format(time.RFC3339),
		"service":   "lesser-cost-aggregator",
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to marshal SNS message")
	}

	// Send SNS message
	input := &sns.PublishInput{
		TopicArn: &topicARN,
		Subject: aws.String(fmt.Sprintf("%s Cost Alert - %s",
			alert.Severity, alert.OperationType)),
		Message: aws.String(string(messageJSON)),
		MessageAttributes: map[string]snstypes.MessageAttributeValue{
			"severity": {
				DataType:    aws.String("String"),
				StringValue: aws.String(alert.Severity),
			},
			"operation_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(alert.OperationType),
			},
		},
	}

	result, err := ca.snsClient.Publish(ctx, input)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to publish SNS message")
	}

	ca.logger.Info("SNS alert sent successfully",
		zap.String("message_id", aws.ToString(result.MessageId)),
		zap.String("topic_arn", topicARN))

	return nil
}

func (ca *CostAggregator) putCloudWatchMetric(ctx context.Context, alert CostAlert) error {
	if ca.cloudWatchClient == nil {
		return pkgErrors.NewAppError(pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "CloudWatch client not configured")
	}

	namespace := "Lesser/DynamoDB/Costs"

	// Put metric for cost threshold breach
	metricData := []cwTypes.MetricDatum{
		{
			MetricName: stringPtr("ThresholdExceeded"),
			Dimensions: []cwTypes.Dimension{
				{
					Name:  stringPtr("OperationType"),
					Value: stringPtr(alert.OperationType),
				},
				{
					Name:  stringPtr("Period"),
					Value: stringPtr(alert.Period),
				},
			},
			Value:     float64Ptr(1), // Count of threshold breach
			Unit:      cwTypes.StandardUnitCount,
			Timestamp: &alert.Timestamp,
		},
		{
			MetricName: stringPtr("CostAmount"),
			Dimensions: []cwTypes.Dimension{
				{
					Name:  stringPtr("OperationType"),
					Value: stringPtr(alert.OperationType),
				},
				{
					Name:  stringPtr("Period"),
					Value: stringPtr(alert.Period),
				},
			},
			Value:     &alert.Cost,
			Unit:      cwTypes.StandardUnitNone, // Dollars
			Timestamp: &alert.Timestamp,
		},
	}

	input := &cloudwatch.PutMetricDataInput{
		Namespace:  &namespace,
		MetricData: metricData,
	}

	_, err := ca.cloudWatchClient.PutMetricData(ctx, input)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to put CloudWatch metric")
	}

	ca.logger.Info("CloudWatch metrics published",
		zap.String("namespace", namespace),
		zap.Int("metric_count", len(metricData)))

	return nil
}

func (ca *CostAggregator) determineSeverity(cost, threshold float64) string {
	ratio := cost / threshold

	if ratio >= 5.0 {
		return "emergency" // 5x threshold
	}
	if ratio >= 2.0 {
		return "critical" // 2x threshold
	}
	return "warning" // Above threshold but not critical
}

func (ca *CostAggregator) triggerAggregation(ctx context.Context, event AggregationEvent) error {
	ca.logger.Info("Triggering next level cost aggregation",
		zap.String("type", event.Type),
		zap.Time("start", event.StartTime),
		zap.Time("end", event.EndTime))

	// Prepare the event payload
	eventPayload, err := json.Marshal(event)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeEventProcessingFailed, pkgErrors.CategoryLambda, "Failed to marshal aggregation event")
	}

	// Option 1: Use SQS for async processing (preferred for resilience)
	queueURL := fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/cost-aggregator-queue",
		ca.cfg.Region, ca.cfg.AWSAccountID)

	sqsInput := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(eventPayload)),
		MessageAttributes: map[string]sqstypes.MessageAttributeValue{
			"AggregationType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(event.Type),
			},
			"Priority": {
				DataType:    aws.String("String"),
				StringValue: aws.String("high"), // Cost aggregations are high priority
			},
		},
		DelaySeconds: 0, // Process immediately
	}

	var sqsErr error
	if ca.sqsClient != nil {
		var sqsResult *sqs.SendMessageOutput
		sqsResult, sqsErr = ca.sqsClient.SendMessage(ctx, sqsInput)
		if sqsErr == nil {
			ca.logger.Info("Successfully queued cost aggregation via SQS",
				zap.String("message_id", aws.ToString(sqsResult.MessageId)),
				zap.String("type", event.Type))
			return nil
		}
	}

	// If SQS fails, fallback to direct Lambda invocation
	if sqsErr != nil {
		ca.logger.Warn("SQS send failed, falling back to direct Lambda invocation",
			zap.Error(sqsErr))
	}
	if ca.lambdaClient == nil {
		return pkgErrors.NewAppError(pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Lambda client not configured")
	}

	// Option 2: Direct Lambda invocation (fallback)
	functionName := "cost-aggregator" // This Lambda function name

	lambdaInput := &awslambda.InvokeInput{
		FunctionName:   &functionName,
		InvocationType: lambdaTypes.InvocationTypeEvent, // Async invocation
		Payload:        eventPayload,
		Qualifier:      stringPtr("$LATEST"), // Use latest version
	}

	lambdaResult, err := ca.lambdaClient.Invoke(ctx, lambdaInput)
	if err != nil {
		ca.logger.Error("lambda invocation failed after SQS failure",
			zap.Error(err),
			zap.NamedError("sqs_error", sqsErr))
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to invoke lambda and send SQS message")
	}

	if lambdaResult.FunctionError != nil {
		ca.logger.Error("lambda function execution error",
			zap.String("function_error", *lambdaResult.FunctionError))
		return pkgErrors.CostAggregatorLambdaFunctionError()
	}

	ca.logger.Info("Successfully triggered cost aggregation via direct Lambda invocation",
		zap.String("function_name", functionName),
		zap.String("type", event.Type),
		zap.Int32("status_code", lambdaResult.StatusCode))

	return nil
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

// Helper functions for extracting attributes from DynamoDB images

func getAttribute(image map[string]events.DynamoDBAttributeValue, key string) (string, bool) {
	if attr, exists := image[key]; exists && attr.DataType() == events.DataTypeString {
		return attr.String(), true
	}
	return "", false
}

func getNumberAttribute(image map[string]events.DynamoDBAttributeValue, key string) (float64, bool) {
	if attr, exists := image[key]; exists && attr.DataType() == events.DataTypeNumber {
		if val, err := attr.Float(); err == nil {
			return val, true
		}
	}
	return 0, false
}

func getNumberFromMap(m map[string]events.DynamoDBAttributeValue, key string) (float64, bool) {
	if attr, exists := m[key]; exists {
		switch attr.DataType() {
		case events.DataTypeNumber:
			if val, err := attr.Float(); err == nil {
				return val, true
			}
		case events.DataTypeString:
			// Sometimes numbers are stored as strings
			var val float64
			if _, err := fmt.Sscanf(attr.String(), "%f", &val); err == nil {
				return val, true
			}
		}
	}
	return 0, false
}
