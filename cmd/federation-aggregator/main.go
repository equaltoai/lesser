// Package main implements the federation-aggregator Lambda function for aggregating federation statistics.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// FederationAggregatorProcessor implements both CloudWatch and SQS event handlers for federation aggregation
type FederationAggregatorProcessor struct {
	db                           dynamormCore.DB
	tableName                    string
	logger                       *zap.Logger
	federationActivityRepository *repositories.FederationActivityRepository
	lambdaClient                 *awslambda.Client
	sqsClient                    *sqs.Client
	lambdaCtx                    *common.LambdaContext
}

// AggregationEvent represents the input for federation aggregation
type AggregationEvent struct {
	Type      string    `json:"type"` // "hourly", "daily", "weekly"
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Domains   []string  `json:"domains,omitempty"` // Optional: specific domains to aggregate
}

// FederationAggregation represents aggregated federation statistics
type FederationAggregation struct {
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`

	Period    string    `json:"period"` // hourly, daily, weekly
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`

	// Aggregated metrics
	TotalActivities      int                    `json:"totalActivities"`
	SuccessfulActivities int                    `json:"successfulActivities"`
	FailedActivities     int                    `json:"failedActivities"`
	ActiveDomains        int                    `json:"activeDomains"`
	TotalInboundBytes    int64                  `json:"totalInboundBytes"`
	TotalOutboundBytes   int64                  `json:"totalOutboundBytes"`
	AvgResponseTime      float64                `json:"avgResponseTime"`
	ActivityTypeCounts   map[string]int         `json:"activityTypeCounts"`
	DomainStats          map[string]*DomainStat `json:"domainStats"`
	SoftwareDistribution map[string]int         `json:"softwareDistribution"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DomainStat represents per-domain statistics
type DomainStat struct {
	Domain          string    `json:"domain"`
	ActivityCount   int       `json:"activityCount"`
	SuccessCount    int       `json:"successCount"`
	ErrorCount      int       `json:"errorCount"`
	InboundBytes    int64     `json:"inboundBytes"`
	OutboundBytes   int64     `json:"outboundBytes"`
	AvgResponseTime float64   `json:"avgResponseTime"`
	LastSeen        time.Time `json:"lastSeen"`
}

// NewFederationAggregatorProcessor creates a new federation aggregator processor
func NewFederationAggregatorProcessor(db dynamormCore.DB, tableName string, lambdaCtx *common.LambdaContext) *FederationAggregatorProcessor {
	federationActivityRepository := repositories.NewFederationActivityRepository(db, tableName, lambdaCtx.Logger)

	return &FederationAggregatorProcessor{
		db:                           db,
		tableName:                    tableName,
		logger:                       lambdaCtx.Logger,
		federationActivityRepository: federationActivityRepository,
		lambdaCtx:                    lambdaCtx,
	}
}

// HandleEvent implements the EventBridge handler interface for CloudWatch events
func (p *FederationAggregatorProcessor) HandleEvent(ctx *lift.Context, event events.CloudWatchEvent) error {
	// Initialize AWS clients
	if err := p.initializeAWSClients(ctx.Request.Context()); err != nil {
		p.logger.Error("failed to initialize AWS clients", zap.Error(err))
		return lift.NewLiftError("AWS_INIT_FAILED", "failed to initialize AWS clients", 500).WithCause(err)
	}

	return p.handleCloudWatchEvent(ctx.Request.Context(), event)
}

// initializeAWSClients initializes AWS service clients
func (p *FederationAggregatorProcessor) initializeAWSClients(ctx context.Context) error {
	// Use the AWS config from lambdaCtx
	awsCfg := p.lambdaCtx.AWSServices.Config

	p.lambdaClient = awslambda.NewFromConfig(awsCfg)
	p.sqsClient = sqs.NewFromConfig(awsCfg)

	return nil
}

// HandleSQS implements the SQS handler interface for Lift
func (p *FederationAggregatorProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	// Initialize AWS clients
	if err := p.initializeAWSClients(ctx.Request.Context()); err != nil {
		p.logger.Error("failed to initialize AWS clients", zap.Error(err))
		return lift.NewLiftError("AWS_INIT_FAILED", "failed to initialize AWS clients", 500).WithCause(err)
	}

	p.logger.Info("processing federation aggregation SQS batch",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("message_count", len(event.Records)),
	)

	// Process each SQS message
	for _, record := range event.Records {
		if err := p.processSQSMessage(ctx.Request.Context(), record); err != nil {
			p.logger.Error("failed to process SQS message",
				zap.String("message_id", record.MessageId),
				zap.Error(err),
			)
			return lift.NewLiftError("MESSAGE_PROCESSING_FAILED",
				fmt.Sprintf("failed to process message %s: %v", record.MessageId, err),
				500)
		}
	}

	return nil
}

// processSQSMessage processes a single SQS message
func (p *FederationAggregatorProcessor) processSQSMessage(ctx context.Context, record events.SQSMessage) error {
	// Try to parse as AggregationEvent
	var aggEvent AggregationEvent
	if err := json.Unmarshal([]byte(record.Body), &aggEvent); err != nil {
		return fmt.Errorf("failed to unmarshal aggregation event: %w", err)
	}

	return p.handleAggregationEvent(ctx, aggEvent)
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
	processor *FederationAggregatorProcessor
	db        dynamormCore.DB
)

func init() {
	// Standardized Lambda initialization for federation-aggregator function
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "federation-aggregator", // federation-aggregator
		LambdaType:  common.LambdaTypeProcessor, // These are background processing functions
	})
	
	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	repos = lambdaCtx.Repos.(core.RepositoryStorage)
	
	// Initialize with processor-specific defaults
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}
	
	// Function-specific initialization only
	// Initialize DynamORM with Lambda optimizations
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize processor
	processor = NewFederationAggregatorProcessor(db, cfg.DynamoTableName, lambdaCtx)
}

func main() {
	// Create Lift app
	app := lift.New()

	// Add standard middleware
	app.Use(patterns.RequestIDMiddleware("federation-aggregator"))
	app.Use(patterns.LoggingMiddleware(logger))
	app.Use(patterns.RecoveryMiddleware(logger))

	// Handle SQS events for custom aggregation requests
	_ = app.SQS("federation-aggregator", func(ctx *lift.Context) error {
		// Extract SQS event from Lift context
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no SQS event in request", 400)
		}

		// Try direct cast first
		if event, ok := ctx.Request.RawEvent.(events.SQSEvent); ok {
			return processor.HandleSQS(ctx, event)
		}

		// Fall back to JSON marshaling
		eventBytes, err := json.Marshal(ctx.Request.RawEvent)
		if err != nil {
			return lift.NewLiftError("EVENT_MARSHAL_ERROR", "failed to marshal raw event", 500).WithCause(err)
		}

		var event events.SQSEvent
		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse SQS event", 500).WithCause(err)
		}

		return processor.HandleSQS(ctx, event)
	})

	// Use the EventBridge pattern for CloudWatch scheduled events
	// Register as an EventBridge processor for scheduled aggregation
	eventBridgeProcessor := patterns.NewEventBridgeProcessor("federation-aggregator-schedule", processor, logger)
	patterns.RegisterEventBridge(app, eventBridgeProcessor)

	lambda.Start(app.HandleRequest)
}

// handleCloudWatchEvent processes CloudWatch scheduled events
func (p *FederationAggregatorProcessor) handleCloudWatchEvent(ctx context.Context, event events.CloudWatchEvent) error {
	// Initialize AWS clients
	if err := p.initializeAWSClients(ctx); err != nil {
		p.logger.Error("failed to initialize AWS clients", zap.Error(err))
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	p.logger.Info("Processing CloudWatch scheduled event",
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

	return p.handleAggregationEvent(ctx, aggEvent)
}

// handleAggregationEvent processes federation aggregation events
func (p *FederationAggregatorProcessor) handleAggregationEvent(ctx context.Context, event AggregationEvent) error {
	p.logger.Info("Processing federation aggregation event",
		zap.String("type", event.Type),
		zap.Time("start_time", event.StartTime),
		zap.Time("end_time", event.EndTime),
		zap.Strings("domains", event.Domains))

	// Get filtered activities for the time period
	filteredActivities, err := p.getFilteredActivities(ctx, event)
	if err != nil {
		return err
	}

	if err := common.ValidateSliceNotEmpty("filteredActivities", filteredActivities); err != nil {
		p.logger.Info("No activities to aggregate",
			zap.String("period", event.Type),
			zap.Time("start", event.StartTime),
			zap.Time("end", event.EndTime))
		return nil
	}

	// Create and populate aggregation
	aggregation := p.createAggregation(event)
	domainSoftware := p.processActivities(filteredActivities, event.Domains, aggregation)

	// Calculate per-domain averages and software distribution
	if err := p.calculateDomainMetrics(ctx, event, aggregation); err != nil {
		p.logger.Warn("failed to calculate domain metrics", zap.Error(err))
	}
	p.buildSoftwareDistribution(ctx, aggregation, domainSoftware)

	// Store aggregation
	if err := p.storeAggregation(ctx, aggregation); err != nil {
		return fmt.Errorf("failed to store aggregation: %w", err)
	}

	p.logger.Info("Federation aggregation completed",
		zap.String("period", event.Type),
		zap.Int("total_activities", aggregation.TotalActivities),
		zap.Int("active_domains", aggregation.ActiveDomains),
		zap.Int("success_count", aggregation.SuccessfulActivities),
		zap.Int("failed_count", aggregation.FailedActivities),
		zap.Int64("inbound_bytes", aggregation.TotalInboundBytes),
		zap.Int64("outbound_bytes", aggregation.TotalOutboundBytes),
		zap.Float64("avg_response_time", aggregation.AvgResponseTime))

	// Trigger next level aggregation if applicable
	p.triggerNextLevelAggregation(ctx, event)

	return nil
}

// storeAggregation stores federation aggregation data
func (p *FederationAggregatorProcessor) storeAggregation(_ context.Context, agg *FederationAggregation) error {
	// Store using DynamORM directly since no specific interface method exists
	err := p.db.Model(agg).Create()
	if err != nil {
		// Try update if create fails (aggregation might already exist)
		agg.UpdatedAt = time.Now()
		err = p.db.Model(agg).Update()
		if err != nil {
			return fmt.Errorf("failed to store aggregation: %w", err)
		}
	}
	return nil
}

// triggerAggregation triggers the next level of aggregation
func (p *FederationAggregatorProcessor) triggerAggregation(ctx context.Context, event AggregationEvent) error {
	p.logger.Info("Triggering next level aggregation",
		zap.String("type", event.Type),
		zap.Time("start", event.StartTime),
		zap.Time("end", event.EndTime))

	// Prepare the event payload
	eventPayload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal aggregation event: %w", err)
	}

	// Option 1: Use SQS for async processing (preferred for resilience)
	queueURL := fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/federation-aggregator-queue",
		p.lambdaCtx.Config.Region, p.lambdaCtx.Config.AWSAccountID)

	sqsInput := &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: aws.String(string(eventPayload)),
		MessageAttributes: map[string]sqstypes.MessageAttributeValue{
			"AggregationType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(event.Type),
			},
		},
		DelaySeconds: 0, // Process immediately
	}

	sqsResult, sqsErr := p.sqsClient.SendMessage(ctx, sqsInput)
	if sqsErr == nil {
		p.logger.Info("Successfully queued aggregation via SQS",
			zap.String("message_id", *sqsResult.MessageId),
			zap.String("type", event.Type))
		return nil
	}

	// If SQS fails, fallback to direct Lambda invocation
	p.logger.Warn("SQS send failed, falling back to direct Lambda invocation",
		zap.Error(sqsErr))

	// Option 2: Direct Lambda invocation (fallback)
	functionName := "federation-aggregator" // This Lambda function name

	lambdaInput := &awslambda.InvokeInput{
		FunctionName:   aws.String(functionName),
		InvocationType: types.InvocationTypeEvent, // Async invocation
		Payload:        eventPayload,
	}

	lambdaResult, err := p.lambdaClient.Invoke(ctx, lambdaInput)
	if err != nil {
		return fmt.Errorf("failed to invoke lambda and send SQS message: lambda_err=%w, sqs_err=%v", err, sqsErr)
	}

	if lambdaResult.FunctionError != nil {
		return fmt.Errorf("lambda function returned error: %s", *lambdaResult.FunctionError)
	}

	p.logger.Info("Successfully triggered aggregation via direct Lambda invocation",
		zap.String("function_name", functionName),
		zap.String("type", event.Type),
		zap.Int32("status_code", lambdaResult.StatusCode))

	return nil
}

// Helper functions - removed as we use aws.String, aws.Int32 instead

// TableName returns the DynamoDB table name for FederationAggregation
func (FederationAggregation) TableName() string {
	return "lesser-main"
}

// BeforeCreate hook for FederationAggregation
func (f *FederationAggregation) BeforeCreate() error {
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	if f.UpdatedAt.IsZero() {
		f.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate hook for FederationAggregation
func (f *FederationAggregation) BeforeUpdate() error {
	f.UpdatedAt = time.Now()
	return nil
}

// getFilteredActivities retrieves and filters activities for the aggregation period
func (p *FederationAggregatorProcessor) getFilteredActivities(ctx context.Context, event AggregationEvent) ([]*models.FederationActivity, error) {
	// Get all federation activities for the time period
	activities, err := p.federationActivityRepository.GetRecentActivities(ctx, event.StartTime, 10000)
	if err != nil {
		return nil, fmt.Errorf("failed to get federation activities: %w", err)
	}

	// Filter activities by end time
	filteredActivities := make([]*models.FederationActivity, 0, len(activities))
	for _, activity := range activities {
		if activity.Timestamp.After(event.EndTime) {
			continue
		}
		filteredActivities = append(filteredActivities, activity)
	}

	return filteredActivities, nil
}

// createAggregation creates a new aggregation structure
func (p *FederationAggregatorProcessor) createAggregation(event AggregationEvent) *FederationAggregation {
	return &FederationAggregation{
		PK:                   fmt.Sprintf("fed_agg#%s", event.Type),
		SK:                   fmt.Sprintf("agg#%s", event.StartTime.Format("20060102150405")),
		Period:               event.Type,
		StartTime:            event.StartTime,
		EndTime:              event.EndTime,
		ActivityTypeCounts:   make(map[string]int),
		DomainStats:          make(map[string]*DomainStat),
		SoftwareDistribution: make(map[string]int),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
}

// processActivities processes activities and updates aggregation stats
func (p *FederationAggregatorProcessor) processActivities(activities []*models.FederationActivity, domains []string, aggregation *FederationAggregation) map[string]string {
	totalResponseTime := float64(0)
	responseTimeCount := 0
	domainSoftware := make(map[string]string)

	for _, activity := range activities {
		// Filter by specific domains if provided
		if len(domains) > 0 && !p.isDomainIncluded(activity.Domain, domains) {
			continue
		}

		// Update counts and metrics
		p.updateActivityMetrics(activity, aggregation, &totalResponseTime, &responseTimeCount)

		// Update domain stats
		p.updateDomainStats(activity, aggregation)

		// Track software if available
		if activity.InstanceInfo != nil && activity.InstanceInfo.Software != "" {
			domainSoftware[activity.Domain] = activity.InstanceInfo.Software
		}
	}

	// Calculate average response time
	if responseTimeCount > 0 {
		aggregation.AvgResponseTime = totalResponseTime / float64(responseTimeCount)
	}

	return domainSoftware
}

// isDomainIncluded checks if a domain is in the filter list
func (p *FederationAggregatorProcessor) isDomainIncluded(domain string, domains []string) bool {
	for _, d := range domains {
		if domain == d {
			return true
		}
	}
	return false
}

// updateActivityMetrics updates aggregation metrics from an activity
func (p *FederationAggregatorProcessor) updateActivityMetrics(activity *models.FederationActivity, aggregation *FederationAggregation, totalResponseTime *float64, responseTimeCount *int) {
	aggregation.TotalActivities++

	if activity.Success {
		aggregation.SuccessfulActivities++
		if activity.ResponseTime > 0 {
			*totalResponseTime += activity.ResponseTime
			*responseTimeCount++
		}
	} else {
		aggregation.FailedActivities++
	}

	// Update bytes transferred
	aggregation.TotalInboundBytes += activity.InboundSize
	aggregation.TotalOutboundBytes += activity.OutboundSize

	// Update activity type counts
	aggregation.ActivityTypeCounts[activity.ActivityType]++
}

// updateDomainStats updates per-domain statistics
func (p *FederationAggregatorProcessor) updateDomainStats(activity *models.FederationActivity, aggregation *FederationAggregation) {
	domainStat, exists := aggregation.DomainStats[activity.Domain]
	if !exists {
		domainStat = &DomainStat{
			Domain: activity.Domain,
		}
		aggregation.DomainStats[activity.Domain] = domainStat
	}

	domainStat.ActivityCount++
	if activity.Success {
		domainStat.SuccessCount++
	} else {
		domainStat.ErrorCount++
	}

	domainStat.InboundBytes += activity.InboundSize
	domainStat.OutboundBytes += activity.OutboundSize

	if activity.Timestamp.After(domainStat.LastSeen) {
		domainStat.LastSeen = activity.Timestamp
	}
}

// calculateDomainMetrics calculates per-domain average response times
func (p *FederationAggregatorProcessor) calculateDomainMetrics(ctx context.Context, event AggregationEvent, aggregation *FederationAggregation) error {
	for _, domainStat := range aggregation.DomainStats {
		if domainStat.SuccessCount > 0 {
			// Get domain-specific response times
			domainActivities, err := p.federationActivityRepository.ListByDomain(
				ctx, domainStat.Domain, event.StartTime, event.EndTime, 1000)
			if err != nil {
				continue // Skip this domain on error
			}

			totalDomainResponseTime := float64(0)
			domainResponseCount := 0
			for _, act := range domainActivities {
				if act.Success && act.ResponseTime > 0 {
					totalDomainResponseTime += act.ResponseTime
					domainResponseCount++
				}
			}

			if domainResponseCount > 0 {
				domainStat.AvgResponseTime = totalDomainResponseTime / float64(domainResponseCount)
			}
		}
	}

	// Set active domains count
	aggregation.ActiveDomains = len(aggregation.DomainStats)
	return nil
}

// buildSoftwareDistribution builds the software distribution map
func (p *FederationAggregatorProcessor) buildSoftwareDistribution(ctx context.Context, aggregation *FederationAggregation, domainSoftware map[string]string) {
	// Build software distribution from known software
	for domain, software := range domainSoftware {
		if _, exists := aggregation.DomainStats[domain]; exists {
			aggregation.SoftwareDistribution[software]++
		}
	}

	// Get instance info for domains without software detection
	for domain := range aggregation.DomainStats {
		if _, hasSoftware := domainSoftware[domain]; !hasSoftware {
			info, err := p.federationActivityRepository.GetInstanceInfo(ctx, domain)
			if err == nil && info.Software != "" {
				aggregation.SoftwareDistribution[info.Software]++
			} else {
				aggregation.SoftwareDistribution["unknown"]++
			}
		}
	}
}

// triggerNextLevelAggregation triggers daily aggregation after hourly completion
func (p *FederationAggregatorProcessor) triggerNextLevelAggregation(ctx context.Context, event AggregationEvent) {
	if event.Type == "hourly" && event.EndTime.Hour() == 0 {
		dailyEvent := AggregationEvent{
			Type:      "daily",
			StartTime: event.EndTime.Add(-24 * time.Hour),
			EndTime:   event.EndTime,
		}
		if err := p.triggerAggregation(ctx, dailyEvent); err != nil {
			p.logger.Warn("failed to trigger daily aggregation", zap.Error(err))
		}
	}
}
