// Package main implements the metrics-processor Lambda function for processing individual metrics events.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// DynamoDB stream event type constants
const (
	eventTypeInsert = "INSERT"
	eventTypeModify = "MODIFY"
	eventTypeRemove = "REMOVE"
)

// Metric aggregation and unit constants
const (
	aggregationLevelRaw = "raw"
	metricUnitCount     = "count"
)

// Service and metric type constants
const (
	serviceDataCleanup = "data-cleanup"
	metricTypeDeletion = "deletion"
)

// Metric event type constants
const (
	metricEventUser         = "user_event"
	metricEventActor        = "actor_event"
	metricEventObject       = "object_event"
	metricEventNotification = "notification_event"
	metricEventEngagement   = "engagement_event"
	metricEventSocial       = "social_event"
	metricEventMedia        = "media_event"
	metricEventModeration   = "moderation_event"
	metricEventSecurity     = "security_event"
	metricEventCostTracking = "cost_tracking_event"
	metricEventFederation   = "federation_event"
)

// Handler processes DynamoDB streams for real-time metrics pipeline
type Handler struct {
	processor *MetricsStreamProcessor
	repos     core.RepositoryStorage
	logger    *zap.Logger
}

// MetricsStreamProcessor transforms operational data into reporting metrics
type MetricsStreamProcessor struct {
	repos          core.RepositoryStorage
	reportingRepo  *repositories.MetricRecordRepository
	costCalculator *CostCalculator
	dlqHandler     *DLQHandler
	logger         *zap.Logger
}

// CostCalculator calculates precise AWS costs per operation
type CostCalculator struct {
	// AWS pricing data (updated monthly - all prices in microcents)
	DynamoDBReadCostPerUnit  int64 // 25 microcents per 1000 RCUs
	DynamoDBWriteCostPerUnit int64 // 125 microcents per 1000 WCUs
	LambdaCostPerInvocation  int64 // Base Lambda cost
	S3CostPerOperation       int64 // S3 operation cost
	KMSCostPerOperation      int64 // KMS encryption cost
}

// DLQHandler handles dead letter queue operations for failed processing
type DLQHandler struct {
	dlqRepo *repositories.DLQRepository
	logger  *zap.Logger
}

// UserCostSummary tracks per-user costs (Architecture Decision requirement)
type UserCostSummary struct {
	UserID               string
	WebAuthnOperations   int64 // Primary auth method
	FederationOperations int64
	MediaOperations      int64
	StorageUsage         int64
	TotalCostMicrocents  int64
}

// InstanceCostSummary tracks per-instance costs (Architecture Decision requirement)
type InstanceCostSummary struct {
	InstanceDomain      string
	InboundOperations   int64
	OutboundOperations  int64
	BandwidthBytes      int64
	TotalCostMicrocents int64
}

// OperationCosts represents calculated costs for a specific operation
type OperationCosts struct {
	UserCost     int64
	InstanceCost int64
	TotalCost    int64
}

// NewMetricsStreamProcessor creates a new stream processor
func NewMetricsStreamProcessor(repos core.RepositoryStorage, logger *zap.Logger) *MetricsStreamProcessor {
	return &MetricsStreamProcessor{
		repos: repos,
		reportingRepo: repositories.NewMetricRecordRepository(
			repos.GetDB(),
			repos.GetTableName(),
			logger,
		),
		costCalculator: &CostCalculator{
			// AWS On-Demand pricing (as of 2024, in microcents)
			DynamoDBReadCostPerUnit:  25,  // $0.25 per million RRUs = 25 microcents per 1000
			DynamoDBWriteCostPerUnit: 125, // $1.25 per million WRUs = 125 microcents per 1000
			LambdaCostPerInvocation:  20,  // Base Lambda cost per invocation
			S3CostPerOperation:       5,   // S3 operation cost
			KMSCostPerOperation:      6,   // KMS operation cost (0.000000006-009 cents)
		},
		dlqHandler: &DLQHandler{
			dlqRepo: repos.DLQ(),
			logger:  logger,
		},
		logger: logger,
	}
}

// HandleDynamoDBStreamEvent processes DynamoDB stream events in real-time
func (h *Handler) HandleDynamoDBStreamEvent(ctx context.Context, event events.DynamoDBEvent) error {
	h.logger.Info("Processing DynamoDB stream event",
		zap.Int("record_count", len(event.Records)),
		zap.String("function", "metrics-processor"),
	)

	var successCount, errorCount int

	for _, record := range event.Records {
		if err := h.processor.ProcessStreamRecord(ctx, &record); err != nil {
			// DynamoDB-oriented retry strategy
			// Not critical but impactful if lots of loss (per Architecture Decisions)
			h.logger.Error("failed to process stream record",
				zap.Error(err),
				zap.String("event_name", record.EventName),
				zap.String("event_source", record.EventSource),
			)
			errorCount++

			// Send to DLQ for investigation
			if dlqErr := h.processor.dlqHandler.HandleStreamFailure(ctx, &record, err); dlqErr != nil {
				h.logger.Error("failed to send to DLQ", zap.Error(dlqErr))
			}
		} else {
			successCount++
		}
	}

	h.logger.Info("completed stream processing",
		zap.Int("success_count", successCount),
		zap.Int("error_count", errorCount),
	)

	// Allow partial success - don't fail entire batch for some errors
	return nil
}

// ProcessStreamRecord processes a single DynamoDB stream record
func (p *MetricsStreamProcessor) ProcessStreamRecord(ctx context.Context, record *events.DynamoDBEventRecord) error {
	// Determine the type of operation and data
	switch record.EventName {
	case eventTypeInsert, eventTypeModify:
		return p.processDataChange(ctx, record)
	case eventTypeRemove:
		return p.processDataRemoval(ctx, record)
	default:
		p.logger.Debug("ignoring unknown event", zap.String("event_name", record.EventName))
		return nil // Ignore unknown events
	}
}

// processDataChange handles INSERT and MODIFY events
func (p *MetricsStreamProcessor) processDataChange(ctx context.Context, record *events.DynamoDBEventRecord) error {
	// Extract the table record
	newImage := record.Change.NewImage
	if newImage == nil {
		return nil // No new data to process
	}

	// Get PK and SK from the record
	pk := ""
	sk := ""
	if pkAttr, exists := newImage["PK"]; exists && pkAttr.DataType() == events.DataTypeString {
		pk = pkAttr.String()
	}
	if skAttr, exists := newImage["SK"]; exists && skAttr.DataType() == events.DataTypeString {
		sk = skAttr.String()
	}

	if common.ValidateRequiredParam("pk", pk) != nil || common.ValidateRequiredParam("sk", sk) != nil {
		return nil // Skip records without proper keys
	}

	// Determine what type of operational data this is
	metricType := p.determineMetricType(pk, sk)
	if err := common.ValidateRequiredParam("metricType", metricType); err != nil {
		return nil // Not a trackable metric
	}

	// Extract service name from the data
	serviceName := p.determineService(pk, sk, newImage)

	// Calculate costs for this operation
	costs := p.calculateOperationCosts(ctx, record, metricType, serviceName)

	// Create metric record using the builder pattern
	metricRecord := models.NewMetricRecordBuilder().
		ForService(serviceName).
		OfType(metricType).
		WithAggregationLevel(aggregationLevelRaw). // Real-time raw data
		WithTimestamp(time.Now()).
		WithStats(1, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0). // Single operation
		WithUnit(metricUnitCount).
		WithDimension("user_cost_microcents", fmt.Sprintf("%d", costs.UserCost)).
		WithDimension("instance_cost_microcents", fmt.Sprintf("%d", costs.InstanceCost)).
		WithDimension("total_cost_microcents", fmt.Sprintf("%d", costs.TotalCost)).
		WithDimension("operation_type", record.EventName).
		Build()

	// Add additional dimensions from the record
	p.extractDimensions(record, metricRecord)

	// Store in reporting table (optimized for analytics)
	if err := p.reportingRepo.CreateMetricRecord(ctx, metricRecord); err != nil {
		return fmt.Errorf("failed to store metric record: %w", err)
	}

	// Trigger real-time updates to all 6 subscription types
	p.triggerRealTimeUpdates(metricType, metricRecord)

	p.logger.Debug("processed stream record",
		zap.String("metric_type", metricType),
		zap.String("service", serviceName),
		zap.Int64("user_cost", costs.UserCost),
		zap.Int64("instance_cost", costs.InstanceCost),
	)

	return nil
}

// processDataRemoval handles REMOVE events
func (p *MetricsStreamProcessor) processDataRemoval(ctx context.Context, record *events.DynamoDBEventRecord) error {
	// For deletions, we might want to track cleanup metrics
	oldImage := record.Change.OldImage
	if oldImage == nil {
		return nil
	}

	// Extract keys
	pk := ""
	if pkAttr, exists := oldImage["PK"]; exists && pkAttr.DataType() == events.DataTypeString {
		pk = pkAttr.String()
	}

	// Track deletion metrics for certain types
	if strings.HasPrefix(pk, "USER#") || strings.HasPrefix(pk, "ACTOR#") || strings.HasPrefix(pk, "object#") {
		metricRecord := models.NewMetricRecordBuilder().
			ForService(serviceDataCleanup).
			OfType(metricTypeDeletion).
			WithAggregationLevel(aggregationLevelRaw).
			WithTimestamp(time.Now()).
			WithStats(1, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0).
			WithUnit(metricUnitCount).
			WithDimension("deleted_type", strings.Split(pk, "#")[0]).
			Build()

		return p.reportingRepo.CreateMetricRecord(ctx, metricRecord)
	}

	return nil
}

// determineMetricType determines the metric type based on PK/SK patterns
func (p *MetricsStreamProcessor) determineMetricType(pk, _ string) string {
	// Map PK patterns to metric types
	switch {
	case strings.HasPrefix(pk, "USER#"):
		return metricEventUser
	case strings.HasPrefix(pk, "ACTOR#"):
		return metricEventActor
	case strings.HasPrefix(pk, "object#"):
		return metricEventObject
	case strings.HasPrefix(pk, "NOTIFICATION#"):
		return metricEventNotification
	case strings.HasPrefix(pk, "LIKE#"):
		return metricEventEngagement
	case strings.HasPrefix(pk, "FOLLOW#"):
		return metricEventSocial
	case strings.HasPrefix(pk, "MEDIA#"):
		return metricEventMedia
	case strings.HasPrefix(pk, "MODERATION#"):
		return metricEventModeration
	case strings.HasPrefix(pk, "TRUST#"):
		return metricEventSecurity
	case strings.HasPrefix(pk, "THREAT#"):
		return metricEventSecurity
	case strings.HasPrefix(pk, "cost#"):
		return metricEventCostTracking
	case strings.HasPrefix(pk, "FEDERATION#"):
		return metricEventFederation
	case strings.HasPrefix(pk, "RELAY#"):
		return metricEventFederation
	case strings.HasPrefix(pk, "HASHTAG#"):
		return "content_event"
	case strings.HasPrefix(pk, "SEARCH#"):
		return "search_event"
	case strings.HasPrefix(pk, "RATE_LIMIT#"):
		return "performance_event"
	default:
		return "" // Not trackable
	}
}

// determineService extracts service name from the operational data
func (p *MetricsStreamProcessor) determineService(pk, _ string, image map[string]events.DynamoDBAttributeValue) string {
	// Try to extract from attributes first
	if serviceAttr, exists := image["ServiceName"]; exists && serviceAttr.DataType() == events.DataTypeString {
		return serviceAttr.String()
	}

	if functionAttr, exists := image["FunctionName"]; exists && functionAttr.DataType() == events.DataTypeString {
		return functionAttr.String()
	}

	// Determine from PK patterns
	switch {
	case strings.HasPrefix(pk, "USER#"), strings.HasPrefix(pk, "ACTOR#"):
		return "api"
	case strings.HasPrefix(pk, "object#"):
		return "objects"
	case strings.HasPrefix(pk, "NOTIFICATION#"):
		return "notification-processor"
	case strings.HasPrefix(pk, "MEDIA#"):
		return "media-processor"
	case strings.HasPrefix(pk, "FEDERATION#"):
		return "federation-delivery"
	case strings.HasPrefix(pk, "SEARCH#"):
		return "search-indexer"
	case strings.HasPrefix(pk, "MODERATION#"):
		return "moderation-processor"
	default:
		return "unknown"
	}
}

// calculateOperationCosts calculates precise per-user and per-instance costs
func (p *MetricsStreamProcessor) calculateOperationCosts(_ context.Context, record *events.DynamoDBEventRecord, metricType, serviceName string) *OperationCosts {
	costs := &OperationCosts{}

	// Base costs based on operation type and service
	switch metricType {
	case metricEventUser, metricEventActor:
		// Authentication costs (WebAuthn primary - Architecture Decision)
		if serviceName == "api" {
			costs.UserCost = 625   // WebAuthn verification: $0.000000625 in microcents
			costs.UserCost += 1280 // Session creation: $0.00000128 in microcents
		}
	case metricEventFederation:
		// Federation costs from Architecture Decisions
		costs.InstanceCost = 3000 // Follow request: ~$0.000003 in microcents
		if record.EventName == eventTypeInsert {
			costs.InstanceCost += 5000 // Status creation: ~$0.000005 in microcents
		}
		costs.InstanceCost += p.costCalculator.KMSCostPerOperation // KMS encryption
	case metricEventMedia:
		// Media processing costs
		costs.UserCost = 2000     // Media operation cost
		costs.InstanceCost = 1000 // S3 operation cost
	case metricEventModeration, metricEventSecurity:
		// Security and moderation costs
		costs.UserCost = 500
		costs.InstanceCost = 300
	default:
		// Base DynamoDB operation costs
		costs.UserCost = 100
		costs.InstanceCost = 50
	}

	// Add DynamoDB capacity unit costs
	readCost := p.costCalculator.DynamoDBReadCostPerUnit / 1000   // Per single RCU
	writeCost := p.costCalculator.DynamoDBWriteCostPerUnit / 1000 // Per single WCU

	costs.UserCost += readCost + writeCost
	costs.InstanceCost += readCost

	costs.TotalCost = costs.UserCost + costs.InstanceCost

	return costs
}

// extractDimensions extracts additional dimensions from the stream record
func (p *MetricsStreamProcessor) extractDimensions(record *events.DynamoDBEventRecord, metricRecord *models.MetricRecord) {
	image := record.Change.NewImage
	if image == nil {
		return
	}

	// Extract common dimensions
	if userIDAttr, exists := image["UserID"]; exists && userIDAttr.DataType() == events.DataTypeString {
		metricRecord.AddDimension("user_id", userIDAttr.String())
	}

	if tenantIDAttr, exists := image["TenantID"]; exists && tenantIDAttr.DataType() == events.DataTypeString {
		metricRecord.AddDimension("tenant_id", tenantIDAttr.String())
	}

	if domainAttr, exists := image["Domain"]; exists && domainAttr.DataType() == events.DataTypeString {
		metricRecord.AddDimension("instance_domain", domainAttr.String())
	}

	// Add record size as dimension
	recordSize := p.calculateRecordSize(image)
	metricRecord.AddDimension("record_size_bytes", fmt.Sprintf("%d", recordSize))
}

// calculateRecordSize estimates the size of the DynamoDB record
func (p *MetricsStreamProcessor) calculateRecordSize(image map[string]events.DynamoDBAttributeValue) int64 {
	// Rough estimation of record size for cost calculations
	var size int64
	for key, value := range image {
		size += int64(len(key))
		if value.DataType() == events.DataTypeString {
			size += int64(len(value.String()))
		}
		if value.DataType() == events.DataTypeNumber {
			size += 8 // Approximate number size
		}
		// Add more attribute type estimations as needed
		size += 10 // Overhead per attribute
	}
	return size
}

// triggerRealTimeUpdates triggers real-time updates for all subscription types with GraphQL integration
func (p *MetricsStreamProcessor) triggerRealTimeUpdates(metricType string, record *models.MetricRecord) {
	// Publish metrics events to GraphQL subscriptions via streaming EventBus
	// This supports all subscription types: performance, cost, moderation, security, infrastructure, and custom

	switch metricType {
	case metricEventModeration:
		p.triggerModerationQueueUpdate(record)
		p.publishMetricsSubscriptionEvent(record, "moderation")
	case metricEventSecurity:
		p.triggerThreatIntelligence(record)
		p.publishMetricsSubscriptionEvent(record, "security")
	case "performance_event":
		p.triggerPerformanceAlert(record)
		p.publishMetricsSubscriptionEvent(record, "performance")
	case metricEventFederation:
		p.triggerInfrastructureEvent(record)
		p.publishMetricsSubscriptionEvent(record, "federation")
	case metricEventUser, metricEventActor:
		p.triggerInfrastructureEvent(record)
		p.publishMetricsSubscriptionEvent(record, "user_activity")
	case metricEventCostTracking:
		p.publishMetricsSubscriptionEvent(record, "cost")
	case metricEventMedia:
		p.publishMetricsSubscriptionEvent(record, "media")
	case metricEventObject, metricEventEngagement, metricEventSocial:
		p.publishMetricsSubscriptionEvent(record, "content")
	case metricEventNotification:
		p.publishMetricsSubscriptionEvent(record, "notifications")
	default:
		// Handle custom and other metric types
		p.publishMetricsSubscriptionEvent(record, "general")
	}

	// Also publish to general metrics subscription for dashboard and analytics
	p.publishMetricsSubscriptionEvent(record, "metrics_dashboard")

	p.logger.Debug("triggered real-time GraphQL subscription updates",
		zap.String("metric_type", metricType),
		zap.String("service", record.ServiceName),
		zap.String("metric_id", record.MetricID),
	)
}

// triggerModerationQueueUpdate triggers moderation queue updates
func (p *MetricsStreamProcessor) triggerModerationQueueUpdate(record *models.MetricRecord) {
	// Publish moderation event to GraphQL subscription system via streaming EventBus
	event := p.createStreamingEvent(streaming.EventTypeModeration, streaming.ActionUpdate, record)
	if err := streaming.PublishGlobal(event); err != nil {
		p.logger.Error("failed to publish moderation event to GraphQL subscriptions",
			zap.Error(err),
			zap.String("record_id", record.MetricID))
	} else {
		p.logger.Debug("published moderation queue update to GraphQL subscriptions",
			zap.String("record_id", record.MetricID))
	}
}

// triggerThreatIntelligence triggers threat intelligence updates
func (p *MetricsStreamProcessor) triggerThreatIntelligence(record *models.MetricRecord) {
	// Publish security event to GraphQL subscription system via streaming EventBus
	// Use moderation flag for security threats as they require review
	event := p.createStreamingEvent(streaming.EventTypeModerationFlag, streaming.ActionFlag, record)
	if err := streaming.PublishGlobal(event); err != nil {
		p.logger.Error("failed to publish security event to GraphQL subscriptions",
			zap.Error(err),
			zap.String("record_id", record.MetricID))
	} else {
		p.logger.Debug("published threat intelligence update to GraphQL subscriptions",
			zap.String("record_id", record.MetricID))
	}
}

// triggerPerformanceAlert triggers performance alerts
func (p *MetricsStreamProcessor) triggerPerformanceAlert(record *models.MetricRecord) {
	// Publish performance event to GraphQL subscription system via streaming EventBus
	// Cost alerts are used for performance metrics as they relate to resource consumption
	event := p.createStreamingEvent(streaming.EventTypeCostAlert, streaming.ActionUpdate, record)
	if err := streaming.PublishGlobal(event); err != nil {
		p.logger.Error("failed to publish performance event to GraphQL subscriptions",
			zap.Error(err),
			zap.String("record_id", record.MetricID))
	} else {
		p.logger.Debug("published performance alert to GraphQL subscriptions",
			zap.String("record_id", record.MetricID))
	}
}

// triggerInfrastructureEvent triggers infrastructure event notifications
func (p *MetricsStreamProcessor) triggerInfrastructureEvent(record *models.MetricRecord) {
	// Publish infrastructure event to GraphQL subscription system via streaming EventBus
	// Cost updates are used for infrastructure metrics as they relate to resource usage
	event := p.createStreamingEvent(streaming.EventTypeCostUpdate, streaming.ActionUpdate, record)
	if err := streaming.PublishGlobal(event); err != nil {
		p.logger.Error("failed to publish infrastructure event to GraphQL subscriptions",
			zap.Error(err),
			zap.String("record_id", record.MetricID))
	} else {
		p.logger.Debug("published infrastructure event to GraphQL subscriptions",
			zap.String("record_id", record.MetricID))
	}
}

// createStreamingEvent creates a streaming event from a metric record
func (p *MetricsStreamProcessor) createStreamingEvent(eventType streaming.EventType, action streaming.EventAction, record *models.MetricRecord) *streaming.InternalEvent {
	userID, tenantID, actorID := p.extractEventUserInfo(record)
	metadata := p.buildEventMetadata(record, tenantID, actorID)
	priority := p.calculateEventPriority(record)

	return &streaming.InternalEvent{
		ID:        record.MetricID,
		Type:      eventType,
		Action:    action,
		UserID:    userID,
		ActorID:   actorID,
		Timestamp: record.Timestamp,
		Data:      record,
		Metadata:  metadata,
		Priority:  priority,
		Streams:   p.determineEventStreams(record),
	}
}

// extractEventUserInfo extracts user information from record dimensions
func (p *MetricsStreamProcessor) extractEventUserInfo(record *models.MetricRecord) (string, string, string) {
	if record.Dimensions == nil {
		return "", "", ""
	}

	userID := record.Dimensions["user_id"]
	tenantID := record.Dimensions["tenant_id"]
	actorID := record.Dimensions["actor_id"]

	return userID, tenantID, actorID
}

// buildEventMetadata creates comprehensive metadata for the streaming event
func (p *MetricsStreamProcessor) buildEventMetadata(record *models.MetricRecord, tenantID, actorID string) map[string]string {
	metadata := p.createBaseEventMetadata(record)
	p.addEventUserInfo(metadata, tenantID, actorID)
	p.addEventPercentiles(metadata, record)
	p.addEventDimensions(metadata, record)

	return metadata
}

// createBaseEventMetadata creates the base metadata structure
func (p *MetricsStreamProcessor) createBaseEventMetadata(record *models.MetricRecord) map[string]string {
	return map[string]string{
		"service_name":      record.ServiceName,
		"metric_type":       record.MetricType,
		"aggregation_level": record.AggregationLevel,
		"unit":              record.Unit,
		"count":             fmt.Sprintf("%d", record.Count),
		"sum":               fmt.Sprintf("%.2f", record.Sum),
		"min":               fmt.Sprintf("%.2f", record.Min),
		"max":               fmt.Sprintf("%.2f", record.Max),
		"timestamp":         record.Timestamp.Format(time.RFC3339),
		"subscription_type": "metrics",
	}
}

// addEventUserInfo adds tenant and actor information to metadata
func (p *MetricsStreamProcessor) addEventUserInfo(metadata map[string]string, tenantID, actorID string) {
	if tenantID != "" {
		metadata["tenant_id"] = tenantID
	}
	if actorID != "" {
		metadata["actor_id"] = actorID
	}
}

// addEventPercentiles adds percentile data to metadata
func (p *MetricsStreamProcessor) addEventPercentiles(metadata map[string]string, record *models.MetricRecord) {
	percentiles := map[string]float64{
		"p50": record.P50,
		"p95": record.P95,
		"p99": record.P99,
	}

	for key, value := range percentiles {
		if value > 0 {
			metadata[key] = fmt.Sprintf("%.2f", value)
		}
	}
}

// addEventDimensions adds dimension information to metadata
func (p *MetricsStreamProcessor) addEventDimensions(metadata map[string]string, record *models.MetricRecord) {
	if record.Dimensions == nil {
		return
	}

	p.addEventKnownDimensions(metadata, record.Dimensions)
	p.addEventCustomDimensions(metadata, record.Dimensions)
}

// addEventKnownDimensions adds known dimension keys to metadata
func (p *MetricsStreamProcessor) addEventKnownDimensions(metadata map[string]string, dimensions map[string]string) {
	knownKeys := []string{
		"user_cost_microcents", "instance_cost_microcents", "total_cost_microcents",
		"instance_domain", "record_size_bytes",
	}

	for _, key := range knownKeys {
		if value, exists := dimensions[key]; exists {
			metadata[key] = value
		}
	}
}

// addEventCustomDimensions adds custom dimensions with prefix to metadata
func (p *MetricsStreamProcessor) addEventCustomDimensions(metadata map[string]string, dimensions map[string]string) {
	for key, value := range dimensions {
		if _, exists := metadata[key]; !exists {
			metadata["dim_"+key] = value
		}
	}
}

// calculateEventPriority determines event priority based on metric type and values
func (p *MetricsStreamProcessor) calculateEventPriority(record *models.MetricRecord) streaming.EventPriority {
	if p.isSecurityOrModerationEvent(record) {
		return streaming.PriorityHigh
	}
	if p.isHighVolumeEvent(record) {
		return streaming.PriorityHigh
	}
	return streaming.PriorityNormal
}

// isSecurityOrModerationEvent checks if the event is security or moderation related
func (p *MetricsStreamProcessor) isSecurityOrModerationEvent(record *models.MetricRecord) bool {
	return record.MetricType == metricEventSecurity || record.MetricType == metricEventModeration
}

// isHighVolumeEvent checks if the event represents high volume activity
func (p *MetricsStreamProcessor) isHighVolumeEvent(record *models.MetricRecord) bool {
	return record.Count > 1000 || record.Sum > 10000
}

// publishMetricsSubscriptionEvent publishes a metrics event specifically for GraphQL subscriptions
func (p *MetricsStreamProcessor) publishMetricsSubscriptionEvent(record *models.MetricRecord, subscriptionCategory string) {
	metadata := p.buildMetricsMetadata(record, subscriptionCategory)
	userID, tenantID, actorID := p.extractUserInfo(record)
	priority := p.determinePriority(record, subscriptionCategory)

	metricsEvent := &streaming.InternalEvent{
		ID:        fmt.Sprintf("metrics_%s_%s", record.MetricID, subscriptionCategory),
		Type:      streaming.EventTypeMetricsUpdate,
		Action:    streaming.ActionUpdate,
		UserID:    userID,
		ActorID:   actorID,
		Timestamp: record.Timestamp,
		Data:      record,
		Metadata:  metadata,
		Priority:  priority,
		Streams:   p.getSubscriptionStreams(subscriptionCategory, userID, tenantID, record.ServiceName),
	}

	p.publishEventAndLog(metricsEvent, subscriptionCategory, record)
}

// buildMetricsMetadata creates comprehensive metadata for GraphQL subscription filtering
func (p *MetricsStreamProcessor) buildMetricsMetadata(record *models.MetricRecord, subscriptionCategory string) map[string]string {
	metadata := map[string]string{
		"subscription_category": subscriptionCategory,
		"service_name":          record.ServiceName,
		"metric_type":           record.MetricType,
		"aggregation_level":     record.AggregationLevel,
		"timestamp":             record.Timestamp.Format(time.RFC3339),
		"subscription_type":     "metrics",
	}

	p.addMetricValues(metadata, record)
	p.addPercentiles(metadata, record)
	p.addDimensionInfo(metadata, record)

	return metadata
}

// addMetricValues adds metric values to metadata for threshold filtering
func (p *MetricsStreamProcessor) addMetricValues(metadata map[string]string, record *models.MetricRecord) {
	if record.Count > 0 {
		metadata["count"] = fmt.Sprintf("%d", record.Count)
		metadata["sum"] = fmt.Sprintf("%.2f", record.Sum)
		metadata["min"] = fmt.Sprintf("%.2f", record.Min)
		metadata["max"] = fmt.Sprintf("%.2f", record.Max)
		average := record.Sum / float64(record.Count)
		metadata["average"] = fmt.Sprintf("%.2f", average)
	}
}

// addPercentiles adds percentile information to metadata
func (p *MetricsStreamProcessor) addPercentiles(metadata map[string]string, record *models.MetricRecord) {
	if record.P50 > 0 {
		metadata["p50"] = fmt.Sprintf("%.2f", record.P50)
	}
	if record.P95 > 0 {
		metadata["p95"] = fmt.Sprintf("%.2f", record.P95)
	}
	if record.P99 > 0 {
		metadata["p99"] = fmt.Sprintf("%.2f", record.P99)
	}
}

// addDimensionInfo extracts and adds dimension information to metadata
func (p *MetricsStreamProcessor) addDimensionInfo(metadata map[string]string, record *models.MetricRecord) {
	if record.Dimensions == nil {
		return
	}

	p.addUserDimensions(metadata, record.Dimensions)
	p.addCostDimensions(metadata, record.Dimensions)
}

// addUserDimensions adds user/tenant/actor information from dimensions
func (p *MetricsStreamProcessor) addUserDimensions(metadata map[string]string, dimensions map[string]string) {
	for key, value := range dimensions {
		switch key {
		case "user_id", "tenant_id", "actor_id", "instance_domain":
			metadata[key] = value
		}
	}
}

// addCostDimensions adds cost information from dimensions
func (p *MetricsStreamProcessor) addCostDimensions(metadata map[string]string, dimensions map[string]string) {
	costKeys := []string{"user_cost_microcents", "instance_cost_microcents", "total_cost_microcents"}
	for _, key := range costKeys {
		if value, exists := dimensions[key]; exists {
			metadata[key] = value
		}
	}
}

// extractUserInfo gets user, tenant, and actor IDs from record dimensions
func (p *MetricsStreamProcessor) extractUserInfo(record *models.MetricRecord) (string, string, string) {
	if record.Dimensions == nil {
		return "", "", ""
	}

	userID := record.Dimensions["user_id"]
	tenantID := record.Dimensions["tenant_id"]
	actorID := record.Dimensions["actor_id"]

	return userID, tenantID, actorID
}

// determinePriority calculates event priority based on category and values
func (p *MetricsStreamProcessor) determinePriority(record *models.MetricRecord, subscriptionCategory string) streaming.EventPriority {
	switch subscriptionCategory {
	case "security", "moderation":
		return streaming.PriorityHigh
	case "performance":
		if record.P95 > 1000 || record.Max > 5000 {
			return streaming.PriorityHigh
		}
	case "cost":
		if p.isHighCost(record) {
			return streaming.PriorityHigh
		}
	}
	return streaming.PriorityNormal
}

// isHighCost checks if the record represents high cost activity
func (p *MetricsStreamProcessor) isHighCost(record *models.MetricRecord) bool {
	if record.Dimensions == nil {
		return false
	}

	totalCost, exists := record.Dimensions["total_cost_microcents"]
	if !exists {
		return false
	}

	cost, err := strconv.ParseInt(totalCost, 10, 64)
	return err == nil && cost > 100000 // >$0.001
}

// publishEventAndLog publishes the event and logs the result
func (p *MetricsStreamProcessor) publishEventAndLog(event *streaming.InternalEvent, subscriptionCategory string, record *models.MetricRecord) {
	if err := streaming.PublishGlobal(event); err != nil {
		p.logPublishError(err, subscriptionCategory, record)
	} else {
		p.logPublishSuccess(event, subscriptionCategory, record)
	}
}

// logPublishError logs failed event publishing
func (p *MetricsStreamProcessor) logPublishError(err error, subscriptionCategory string, record *models.MetricRecord) {
	p.logger.Error("failed to publish metrics subscription event to GraphQL",
		zap.Error(err),
		zap.String("subscription_category", subscriptionCategory),
		zap.String("metric_id", record.MetricID),
		zap.String("service", record.ServiceName),
		zap.String("metric_type", record.MetricType))
}

// logPublishSuccess logs successful event publishing
func (p *MetricsStreamProcessor) logPublishSuccess(event *streaming.InternalEvent, subscriptionCategory string, record *models.MetricRecord) {
	p.logger.Debug("published metrics event to GraphQL subscriptions",
		zap.String("subscription_category", subscriptionCategory),
		zap.String("metric_id", record.MetricID),
		zap.String("service", record.ServiceName),
		zap.String("metric_type", record.MetricType),
		zap.Strings("streams", event.Streams))
}

// getSubscriptionStreams determines which GraphQL subscription streams should receive the metrics event
func (p *MetricsStreamProcessor) getSubscriptionStreams(category, userID, tenantID, serviceName string) []string {
	streams := []string{}

	// Global metrics dashboard stream
	streams = append(streams, "metrics:global")

	// Category-specific streams
	streams = append(streams, fmt.Sprintf("metrics:%s", category))

	// Service-specific streams
	streams = append(streams, fmt.Sprintf("metrics:service:%s", serviceName))

	// User-specific streams if user ID available
	if userID != "" {
		streams = append(streams, fmt.Sprintf("metrics:user:%s", userID))
		streams = append(streams, fmt.Sprintf("metrics:%s:user:%s", category, userID))
	}

	// Tenant-specific streams if tenant ID available
	if tenantID != "" {
		streams = append(streams, fmt.Sprintf("metrics:tenant:%s", tenantID))
		streams = append(streams, fmt.Sprintf("metrics:%s:tenant:%s", category, tenantID))
	}

	// Combined category-service streams for more specific filtering
	streams = append(streams, fmt.Sprintf("metrics:%s:service:%s", category, serviceName))

	return streams
}

// determineEventStreams determines target streams for general metric events (non-subscription specific)
func (p *MetricsStreamProcessor) determineEventStreams(record *models.MetricRecord) []string {
	streams := []string{}

	// Service-based routing
	streams = append(streams, fmt.Sprintf("service:%s", record.ServiceName))

	// Metric type routing
	streams = append(streams, fmt.Sprintf("metrics:%s", record.MetricType))

	// User-based routing if available
	if record.Dimensions != nil {
		if userID, exists := record.Dimensions["user_id"]; exists && userID != "" {
			streams = append(streams, fmt.Sprintf("user:%s", userID))
		}
		if tenantID, exists := record.Dimensions["tenant_id"]; exists && tenantID != "" {
			streams = append(streams, fmt.Sprintf("tenant:%s", tenantID))
		}
	}

	// Global stream for dashboard and monitoring
	streams = append(streams, "global")

	return streams
}

// HandleStreamFailure handles failed stream processing by sending to DLQ
func (h *DLQHandler) HandleStreamFailure(ctx context.Context, record *events.DynamoDBEventRecord, processingErr error) error {
	// Create DLQ message for the failed stream record
	dlqMessage := models.NewDLQMessageBuilder().
		ForService("metrics-processor").
		WithOriginalMessage(record.EventID, fmt.Sprintf("DynamoDB Stream Record: %+v", record)).
		WithError("StreamProcessingError", processingErr.Error(), "").
		WithFailureReason("Failed to process stream record for metrics generation").
		WithPriority("medium"). // Not critical but impactful
		WithContext(os.Getenv("AWS_LAMBDA_FUNCTION_NAME"), "", "", generateValidatedUUID()).
		Build()

	return h.dlqRepo.CreateDLQMessage(ctx, dlqMessage)
}

// generateValidatedUUID creates a UUID and validates it using common validation
func generateValidatedUUID() string {
	requestID := uuid.New().String()
	
	// Validate the generated UUID
	if err := common.ValidateUUID("uuid", requestID); err != nil {
		// Fall back to a simple timestamp-based ID if validation fails
		return fmt.Sprintf("id_%d", time.Now().UnixNano())
	}
	
	return requestID
}

// Global variables for Lambda initialization
var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
	handler   *Handler
)

func init() {
	// Standardized Lambda initialization for metrics-processor function
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "metrics-processor", // metrics-processor
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
	// Initialize metrics stream processor
	processor := NewMetricsStreamProcessor(repos, logger)

	// Create handler instance
	handler = &Handler{
		processor: processor,
		repos:     repos,
		logger:    logger,
	}
}

func main() {
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
		}
	}()

	logger.Info("starting metrics stream processor Lambda",
		zap.String("service", "metrics-processor"),
		zap.String("lambda_type", "processor"))

	// Start Lambda handler for DynamoDB streams
	lambda.Start(handler.HandleDynamoDBStreamEvent)
}
