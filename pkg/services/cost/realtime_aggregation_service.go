package cost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

const (
	// EventInsert represents a DynamoDB stream insert event
	EventInsert = "INSERT"
	// EventModify represents a DynamoDB stream modify event
	EventModify = "MODIFY"
	// EventRemove represents a DynamoDB stream remove event
	EventRemove = "REMOVE"
)

// RealtimeAggregationService provides real-time cost aggregation using DynamoDB Streams
type RealtimeAggregationService struct {
	db                core.DB
	aiCostRepo        *repositories.AICostRepository
	webSocketCostRepo *repositories.WebSocketCostRepository
	notificationSvc   *notifications.Service
	logger            *zap.Logger
	aggregationCache  *AggregationCache
	streamProcessors  map[string]*StreamProcessor
	mu                sync.RWMutex
}

// NewRealtimeAggregationService creates a new real-time aggregation service
func NewRealtimeAggregationService(
	db core.DB,
	aiCostRepo *repositories.AICostRepository,
	webSocketCostRepo *repositories.WebSocketCostRepository,
	notificationSvc *notifications.Service,
	logger *zap.Logger,
) *RealtimeAggregationService {
	service := &RealtimeAggregationService{
		db:                db,
		aiCostRepo:        aiCostRepo,
		webSocketCostRepo: webSocketCostRepo,
		notificationSvc:   notificationSvc,
		logger:            logger,
		aggregationCache:  NewAggregationCache(),
		streamProcessors:  make(map[string]*StreamProcessor),
	}

	// Initialize stream processors for different cost types
	service.streamProcessors["ai_cost"] = NewStreamProcessor("ai_cost", service.processAICostStream, logger)
	service.streamProcessors["websocket_cost"] = NewStreamProcessor("websocket_cost", service.processWebSocketCostStream, logger)
	service.streamProcessors["federation_cost"] = NewStreamProcessor("federation_cost", service.processFederationCostStream, logger)

	return service
}

// AggregationCache provides in-memory caching for real-time aggregations
type AggregationCache struct {
	costSummaries   map[string]*SummaryCache
	alertThresholds map[string]*AlertThreshold
	lastAggregation map[string]time.Time
	mu              sync.RWMutex
}

// SummaryCache represents cached cost summary data
type SummaryCache struct {
	Period          string                 `json:"period"`
	TotalCost       float64                `json:"total_cost"`
	OperationCounts map[string]int64       `json:"operation_counts"`
	LastUpdated     time.Time              `json:"last_updated"`
	ExpiresAt       time.Time              `json:"expires_at"`
	Metrics         map[string]interface{} `json:"metrics"`
}

// AlertThreshold represents cost alerting thresholds
type AlertThreshold struct {
	MetricName    string  `json:"metric_name"`
	Threshold     float64 `json:"threshold"`
	ComparisonOp  string  `json:"comparison_op"` // gt, gte, lt, lte, eq
	WindowMinutes int     `json:"window_minutes"`
	Severity      string  `json:"severity"` // low, medium, high, critical
	Enabled       bool    `json:"enabled"`
}

// StreamProcessor handles processing of DynamoDB stream events
type StreamProcessor struct {
	processorType string
	handler       func(ctx context.Context, records []events.DynamoDBEventRecord) error
	logger        *zap.Logger
	metrics       *StreamMetrics
}

// StreamMetrics tracks stream processing metrics
type StreamMetrics struct {
	TotalRecords     int64     `json:"total_records"`
	ProcessedRecords int64     `json:"processed_records"`
	FailedRecords    int64     `json:"failed_records"`
	LastProcessedAt  time.Time `json:"last_processed_at"`
	ProcessingTimeMs int64     `json:"processing_time_ms"`
	ThroughputRPS    float64   `json:"throughput_rps"`
}

// RealTimeMetrics represents live cost metrics
type RealTimeMetrics struct {
	Timestamp          time.Time               `json:"timestamp"`
	TotalCostLast1Min  float64                 `json:"total_cost_last_1min"`
	TotalCostLast5Min  float64                 `json:"total_cost_last_5min"`
	TotalCostLast15Min float64                 `json:"total_cost_last_15min"`
	TotalCostLastHour  float64                 `json:"total_cost_last_hour"`
	CostVelocity       float64                 `json:"cost_velocity"`     // Cost per minute
	CostAcceleration   float64                 `json:"cost_acceleration"` // Change in velocity
	TopCostOperations  []OperationCost         `json:"top_cost_operations"`
	AnomalyScore       float64                 `json:"anomaly_score"` // 0-1, higher = more anomalous
	AlertsTriggered    []RealTimeAlert         `json:"alerts_triggered"`
	PredictedDailyCost float64                 `json:"predicted_daily_cost"` // Extrapolated from current rate
	BudgetStatus       map[string]BudgetStatus `json:"budget_status"`
	PerformanceMetrics map[string]float64      `json:"performance_metrics"`
}

// OperationCost represents cost for a specific operation type
type OperationCost struct {
	OperationType  string  `json:"operation_type"`
	CostDollars    float64 `json:"cost_dollars"`
	OperationCount int64   `json:"operation_count"`
	AvgCostPer     float64 `json:"avg_cost_per"`
	TrendDirection string  `json:"trend_direction"`
}

// RealTimeAlert represents a triggered real-time alert
type RealTimeAlert struct {
	AlertID      string    `json:"alert_id"`
	MetricName   string    `json:"metric_name"`
	CurrentValue float64   `json:"current_value"`
	Threshold    float64   `json:"threshold"`
	Severity     string    `json:"severity"`
	Message      string    `json:"message"`
	TriggeredAt  time.Time `json:"triggered_at"`
	Duration     string    `json:"duration"`
}

// BudgetStatus represents current budget utilization
type BudgetStatus struct {
	BudgetName    string  `json:"budget_name"`
	BudgetAmount  float64 `json:"budget_amount"`
	CurrentSpend  float64 `json:"current_spend"`
	Utilization   float64 `json:"utilization"`    // Percentage used
	BurnRate      float64 `json:"burn_rate"`      // Daily burn rate
	DaysRemaining float64 `json:"days_remaining"` // Days until budget exhausted
	Status        string  `json:"status"`         // ok, warning, critical, exceeded
}

// NewAggregationCache creates a new aggregation cache
func NewAggregationCache() *AggregationCache {
	return &AggregationCache{
		costSummaries:   make(map[string]*SummaryCache),
		alertThresholds: make(map[string]*AlertThreshold),
		lastAggregation: make(map[string]time.Time),
	}
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor(processorType string, handler func(ctx context.Context, records []events.DynamoDBEventRecord) error, logger *zap.Logger) *StreamProcessor {
	return &StreamProcessor{
		processorType: processorType,
		handler:       handler,
		logger:        logger,
		metrics: &StreamMetrics{
			LastProcessedAt: time.Now(),
		},
	}
}

// ProcessDynamoDBStreamEvent processes DynamoDB stream events for real-time aggregation
func (s *RealtimeAggregationService) ProcessDynamoDBStreamEvent(ctx context.Context, event events.DynamoDBEvent) error {
	startTime := time.Now()

	s.logger.Info("Processing DynamoDB stream event for real-time aggregation",
		zap.Int("record_count", len(event.Records)),
		zap.String("event_source", "dynamodb"))

	// Group records by table/entity type
	recordGroups := s.groupRecordsByType(event.Records)

	// Process each group with appropriate processor
	var errors []error
	for recordType, records := range recordGroups {
		if processor, exists := s.streamProcessors[recordType]; exists {
			if err := processor.ProcessRecords(ctx, records); err != nil {
				s.logger.Error("Failed to process stream records",
					zap.String("record_type", recordType),
					zap.Int("record_count", len(records)),
					zap.Error(err))
				errors = append(errors, err)
			}
		} else {
			s.logger.Warn("No processor found for record type",
				zap.String("record_type", recordType),
				zap.Int("record_count", len(records)))
		}
	}

	// Update aggregation cache with new data
	if err := s.updateAggregationCache(ctx); err != nil {
		s.logger.Error("Failed to update aggregation cache", zap.Error(err))
		errors = append(errors, err)
	}

	// Check for alert conditions
	if err := s.checkAlertConditions(ctx); err != nil {
		s.logger.Error("Failed to check alert conditions", zap.Error(err))
		errors = append(errors, err)
	}

	processingTime := time.Since(startTime)
	s.logger.Info("Completed DynamoDB stream processing",
		zap.Int("total_records", len(event.Records)),
		zap.Int("errors", len(errors)),
		zap.Duration("processing_time", processingTime))

	if len(errors) > 0 {
		return services.ErrStreamProcessingErrors
	}

	return nil
}

// groupRecordsByType groups DynamoDB records by their entity type
func (s *RealtimeAggregationService) groupRecordsByType(records []events.DynamoDBEventRecord) map[string][]events.DynamoDBEventRecord {
	groups := make(map[string][]events.DynamoDBEventRecord)

	for _, record := range records {
		recordType := s.determineRecordType(record)
		groups[recordType] = append(groups[recordType], record)
	}

	return groups
}

// determineRecordType determines the type of DynamoDB record based on keys
func (s *RealtimeAggregationService) determineRecordType(record events.DynamoDBEventRecord) string {
	// Check primary key to determine record type
	if pk, exists := record.Change.Keys["PK"]; exists && pk.String() != "" {
		pkValue := pk.String()

		if strings.HasPrefix(pkValue, "AI_COST#") {
			return "ai_cost"
		}
		if strings.HasPrefix(pkValue, "WS_COST#") {
			return "websocket_cost"
		}
		if strings.HasPrefix(pkValue, "FED_COST#") {
			return "federation_cost"
		}
	}

	return "unknown"
}

// ProcessRecords processes a batch of stream records
func (p *StreamProcessor) ProcessRecords(ctx context.Context, records []events.DynamoDBEventRecord) error {
	startTime := time.Now()
	processed := int64(0)
	failed := int64(0)

	for _, record := range records {
		if err := p.handler(ctx, []events.DynamoDBEventRecord{record}); err != nil {
			p.logger.Error("Failed to process stream record",
				zap.String("processor_type", p.processorType),
				zap.String("event_name", record.EventName),
				zap.Error(err))
			failed++
		} else {
			processed++
		}
	}

	// Update metrics
	p.metrics.TotalRecords += int64(len(records))
	p.metrics.ProcessedRecords += processed
	p.metrics.FailedRecords += failed
	p.metrics.LastProcessedAt = time.Now()
	p.metrics.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	// Calculate throughput
	if p.metrics.ProcessingTimeMs > 0 {
		p.metrics.ThroughputRPS = float64(processed) / (float64(p.metrics.ProcessingTimeMs) / 1000.0)
	}

	if failed > 0 {
		return services.ErrRecordProcessingFailed
	}

	return nil
}

// processAICostStream processes AI cost stream events
func (s *RealtimeAggregationService) processAICostStream(ctx context.Context, records []events.DynamoDBEventRecord) error {
	for _, record := range records {
		switch record.EventName {
		case EventInsert, EventModify:
			if err := s.processAICostRecord(ctx, record); err != nil {
				return errors.Join(services.ErrProcessAICostRecord, err)
			}
		case EventRemove:
			// Handle record deletion if needed
			s.logger.Debug("AI cost record removed", zap.String("event_name", record.EventName))
		}
	}
	return nil
}

// processWebSocketCostStream processes WebSocket cost stream events
func (s *RealtimeAggregationService) processWebSocketCostStream(ctx context.Context, records []events.DynamoDBEventRecord) error {
	for _, record := range records {
		switch record.EventName {
		case EventInsert, EventModify:
			if err := s.processWebSocketCostRecord(ctx, record); err != nil {
				return errors.Join(services.ErrProcessWebSocketCostRecord, err)
			}
		case EventRemove:
			s.logger.Debug("WebSocket cost record removed", zap.String("event_name", record.EventName))
		}
	}
	return nil
}

// processFederationCostStream processes federation cost stream events
func (s *RealtimeAggregationService) processFederationCostStream(ctx context.Context, records []events.DynamoDBEventRecord) error {
	for _, record := range records {
		switch record.EventName {
		case EventInsert, EventModify:
			if err := s.processFederationCostRecord(ctx, record); err != nil {
				return errors.Join(services.ErrProcessFederationCostRecord, err)
			}
		case EventRemove:
			s.logger.Debug("Federation cost record removed", zap.String("event_name", record.EventName))
		}
	}
	return nil
}

// processAICostRecord processes a single AI cost record
func (s *RealtimeAggregationService) processAICostRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Extract AI cost data from DynamoDB record
	var aiCost models.AICost
	if err := s.unmarshalDynamoDBRecord(record, &aiCost); err != nil {
		return errors.Join(services.ErrUnmarshalAICostRecord, err)
	}

	// Update real-time aggregations
	cacheKey := fmt.Sprintf("ai_cost_%s", aiCost.Timestamp.Format("2006-01-02"))
	s.updateSummaryCache(cacheKey, aiCost.GetTotalCostDollars(), aiCost.OperationType)

	// Create/update hourly aggregation
	if err := s.createOrUpdateAIAggregation(ctx, &aiCost); err != nil {
		s.logger.Error("Failed to create/update AI aggregation",
			zap.String("operation_id", aiCost.OperationID),
			zap.Error(err))
	}

	s.logger.Debug("Processed AI cost stream record",
		zap.String("operation_id", aiCost.OperationID),
		zap.String("operation_type", aiCost.OperationType),
		zap.Float64("cost_dollars", aiCost.GetTotalCostDollars()))

	return nil
}

// processWebSocketCostRecord processes a single WebSocket cost record
func (s *RealtimeAggregationService) processWebSocketCostRecord(_ context.Context, record events.DynamoDBEventRecord) error {
	// Extract WebSocket cost data from DynamoDB record
	var wsCost models.WebSocketCostRecord
	if err := s.unmarshalDynamoDBRecord(record, &wsCost); err != nil {
		return errors.Join(services.ErrUnmarshalWebSocketCostRecord, err)
	}

	// Update real-time aggregations
	cacheKey := fmt.Sprintf("websocket_cost_%s", wsCost.Timestamp.Format("2006-01-02"))
	s.updateSummaryCache(cacheKey, wsCost.EstimatedCostDollars, wsCost.OperationType)

	s.logger.Debug("Processed WebSocket cost stream record",
		zap.String("connection_id", wsCost.ConnectionID),
		zap.String("operation_type", wsCost.OperationType),
		zap.Float64("cost_dollars", wsCost.EstimatedCostDollars))

	return nil
}

// processFederationCostRecord processes a single federation cost record
func (s *RealtimeAggregationService) processFederationCostRecord(_ context.Context, record events.DynamoDBEventRecord) error {
	// Extract federation cost data from DynamoDB record
	var fedCost models.FederationCostTracking
	if err := s.unmarshalDynamoDBRecord(record, &fedCost); err != nil {
		return errors.Join(services.ErrUnmarshalFederationCostRecord, err)
	}

	// Update real-time aggregations
	cacheKey := fmt.Sprintf("federation_cost_%s", fedCost.Timestamp.Format("2006-01-02"))
	s.updateSummaryCache(cacheKey, fedCost.GetTotalCostDollars(), fedCost.ActivityType)

	s.logger.Debug("Processed federation cost stream record",
		zap.String("activity_id", fedCost.ActivityID),
		zap.String("domain", fedCost.Domain),
		zap.Float64("cost_dollars", fedCost.GetTotalCostDollars()))

	return nil
}

// unmarshalDynamoDBRecord unmarshals a DynamoDB stream record into a struct
func (s *RealtimeAggregationService) unmarshalDynamoDBRecord(record events.DynamoDBEventRecord, target interface{}) error {
	var recordData map[string]events.DynamoDBAttributeValue

	switch record.EventName {
	case "INSERT", "MODIFY":
		recordData = record.Change.NewImage
	case "REMOVE":
		recordData = record.Change.OldImage
	default:
		return services.ErrUnsupportedEventType
	}

	// Convert DynamoDB attribute values to JSON
	jsonData := make(map[string]interface{})
	for key, attr := range recordData {
		jsonData[key] = s.convertDynamoDBAttribute(attr)
	}

	// Marshal to JSON then unmarshal to target struct
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return errors.Join(services.ErrMarshalToJSON, err)
	}

	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return errors.Join(services.ErrUnmarshalToTarget, err)
	}

	return nil
}

// convertDynamoDBAttribute converts DynamoDB attribute to Go value
func (s *RealtimeAggregationService) convertDynamoDBAttribute(attr events.DynamoDBAttributeValue) interface{} {
	switch attr.DataType() {
	case events.DataTypeString:
		return attr.String()
	case events.DataTypeNumber:
		return attr.Number()
	case events.DataTypeBoolean:
		return attr.Boolean()
	case events.DataTypeList:
		list := attr.List()
		result := make([]interface{}, len(list))
		for i, item := range list {
			result[i] = s.convertDynamoDBAttribute(item)
		}
		return result
	case events.DataTypeMap:
		mapData := attr.Map()
		result := make(map[string]interface{})
		for key, value := range mapData {
			result[key] = s.convertDynamoDBAttribute(value)
		}
		return result
	case events.DataTypeNull:
		return nil
	default:
		return attr.String() // Fallback to string
	}
}

// updateSummaryCache updates the in-memory cost summary cache
func (s *RealtimeAggregationService) updateSummaryCache(cacheKey string, costDollars float64, operationType string) {
	s.aggregationCache.mu.Lock()
	defer s.aggregationCache.mu.Unlock()

	now := time.Now()

	// Get or create cache entry
	summary, exists := s.aggregationCache.costSummaries[cacheKey]
	if !exists {
		summary = &SummaryCache{
			Period:          "day",
			TotalCost:       0,
			OperationCounts: make(map[string]int64),
			LastUpdated:     now,
			ExpiresAt:       now.Add(24 * time.Hour),
			Metrics:         make(map[string]interface{}),
		}
		s.aggregationCache.costSummaries[cacheKey] = summary
	}

	// Update summary
	summary.TotalCost += costDollars
	summary.OperationCounts[operationType]++
	summary.LastUpdated = now

	// Update metrics
	summary.Metrics["total_operations"] = s.getTotalOperations(summary.OperationCounts)
	summary.Metrics["avg_cost_per_operation"] = s.getAvgCostPerOperation(summary.TotalCost, summary.Metrics["total_operations"].(int64))
}

// createOrUpdateAIAggregation creates or updates AI cost aggregation records
func (s *RealtimeAggregationService) createOrUpdateAIAggregation(ctx context.Context, aiCost *models.AICost) error {
	// Create hourly aggregation
	hourlyAgg := s.createHourlyAIAggregation(aiCost)
	if err := s.aiCostRepo.CreateOrUpdateAggregatedCost(ctx, hourlyAgg); err != nil {
		return errors.Join(services.ErrCreateHourlyAggregation, err)
	}

	// Create daily aggregation if it's the first operation of the day
	if s.shouldCreateDailyAggregation(aiCost.Timestamp) {
		dailyAgg := s.createDailyAIAggregation(aiCost)
		if err := s.aiCostRepo.CreateOrUpdateAggregatedCost(ctx, dailyAgg); err != nil {
			return errors.Join(services.ErrCreateDailyAggregation, err)
		}
	}

	return nil
}

// createHourlyAIAggregation creates hourly aggregated AI cost record
func (s *RealtimeAggregationService) createHourlyAIAggregation(aiCost *models.AICost) *models.AIAggregatedCost {
	hourStart := aiCost.Timestamp.Truncate(time.Hour)
	hourEnd := hourStart.Add(time.Hour)

	return &models.AIAggregatedCost{
		Period:                    "hour",
		PeriodStart:               hourStart,
		PeriodEnd:                 hourEnd,
		OperationType:             aiCost.OperationType,
		ModelName:                 aiCost.ModelName,
		TotalOperations:           1,
		SuccessfulOperations:      s.getSuccessCount(aiCost.Success),
		FailedOperations:          s.getFailureCount(aiCost.Success),
		TotalInputTokens:          aiCost.InputTokens,
		TotalOutputTokens:         aiCost.OutputTokens,
		TotalCostMicroCents:       aiCost.TotalCostMicroCents,
		AvgLatencyMs:              float64(aiCost.RequestLatencyMs),
		AvgComplexityScore:        aiCost.ComplexityScore,
		AvgEfficiencyScore:        aiCost.EfficiencyScore,
		AvgQualityScore:           aiCost.QualityScore,
		AvgRelevanceScore:         aiCost.RelevanceScore,
		AvgComprehensivenessScore: aiCost.ComprehensivenesScore,
		TopComplexityFactors:      aiCost.ComplexityFactors,
	}
}

// createDailyAIAggregation creates daily aggregated AI cost record
func (s *RealtimeAggregationService) createDailyAIAggregation(aiCost *models.AICost) *models.AIAggregatedCost {
	dayStart := aiCost.Timestamp.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	return &models.AIAggregatedCost{
		Period:               "day",
		PeriodStart:          dayStart,
		PeriodEnd:            dayEnd,
		OperationType:        aiCost.OperationType,
		ModelName:            aiCost.ModelName,
		TotalOperations:      1,
		SuccessfulOperations: s.getSuccessCount(aiCost.Success),
		FailedOperations:     s.getFailureCount(aiCost.Success),
		TotalInputTokens:     aiCost.InputTokens,
		TotalOutputTokens:    aiCost.OutputTokens,
		TotalCostMicroCents:  aiCost.TotalCostMicroCents,
	}
}

// shouldCreateDailyAggregation determines if daily aggregation should be created
func (s *RealtimeAggregationService) shouldCreateDailyAggregation(timestamp time.Time) bool {
	s.aggregationCache.mu.RLock()
	defer s.aggregationCache.mu.RUnlock()

	dateKey := timestamp.Format("2006-01-02")
	lastAgg, exists := s.aggregationCache.lastAggregation[dateKey]

	if !exists {
		s.aggregationCache.lastAggregation[dateKey] = timestamp
		return true
	}

	// Create daily aggregation every hour
	return timestamp.Sub(lastAgg) >= time.Hour
}

// updateAggregationCache updates aggregation cache with latest data
func (s *RealtimeAggregationService) updateAggregationCache(_ context.Context) error {
	s.aggregationCache.mu.Lock()
	defer s.aggregationCache.mu.Unlock()

	now := time.Now()

	// Clean up expired cache entries
	for key, summary := range s.aggregationCache.costSummaries {
		if now.After(summary.ExpiresAt) {
			delete(s.aggregationCache.costSummaries, key)
		}
	}

	return nil
}

// checkAlertConditions checks if any alert conditions are triggered
func (s *RealtimeAggregationService) checkAlertConditions(_ context.Context) error {
	s.aggregationCache.mu.RLock()
	defer s.aggregationCache.mu.RUnlock()

	for _, threshold := range s.aggregationCache.alertThresholds {
		if !threshold.Enabled {
			continue
		}

		// Get current metric value
		currentValue := s.getCurrentMetricValue(threshold.MetricName)

		// Check threshold condition
		if s.evaluateThreshold(currentValue, threshold.Threshold, threshold.ComparisonOp) {
			alert := RealTimeAlert{
				AlertID:      fmt.Sprintf("alert_%d", time.Now().UnixNano()),
				MetricName:   threshold.MetricName,
				CurrentValue: currentValue,
				Threshold:    threshold.Threshold,
				Severity:     threshold.Severity,
				Message:      fmt.Sprintf("Metric %s exceeded threshold: %.2f %s %.2f", threshold.MetricName, currentValue, threshold.ComparisonOp, threshold.Threshold),
				TriggeredAt:  time.Now(),
				Duration:     fmt.Sprintf("%dm", threshold.WindowMinutes),
			}

			s.logger.Warn("Cost alert triggered",
				zap.String("metric", alert.MetricName),
				zap.Float64("current_value", alert.CurrentValue),
				zap.Float64("threshold", alert.Threshold),
				zap.String("severity", alert.Severity))

			// Send alert to notification system
			if s.notificationSvc != nil {
				go s.sendCostAlert(context.Background(), alert)
			}
		}
	}

	return nil
}

// sendCostAlert sends a cost alert notification to system administrators
func (s *RealtimeAggregationService) sendCostAlert(ctx context.Context, alert RealTimeAlert) {
	// Create notification for system administrators
	// For now, we'll create a system notification that can be picked up by admin dashboards
	notificationCmd := &notifications.CreateNotificationCommand{
		UserID:     "system", // System-level notification
		Type:       "cost_alert",
		ActorID:    "cost_system",
		ActorType:  "system",
		TargetID:   alert.AlertID,
		TargetType: "cost_metric",
		Title:      fmt.Sprintf("Cost Alert: %s", alert.MetricName),
		Body:       alert.Message,
		Data: map[string]interface{}{
			"alert_id":      alert.AlertID,
			"metric_name":   alert.MetricName,
			"current_value": alert.CurrentValue,
			"threshold":     alert.Threshold,
			"severity":      alert.Severity,
			"triggered_at":  alert.TriggeredAt,
			"duration":      alert.Duration,
		},
		GroupKey: fmt.Sprintf("cost_alert_%s", alert.MetricName),
	}

	_, err := s.notificationSvc.CreateNotification(ctx, notificationCmd)
	if err != nil {
		s.logger.Error("failed to send cost alert notification",
			zap.String("alert_id", alert.AlertID),
			zap.String("metric", alert.MetricName),
			zap.Error(err))
		return
	}

	s.logger.Info("Cost alert notification sent successfully",
		zap.String("alert_id", alert.AlertID),
		zap.String("metric", alert.MetricName),
		zap.String("severity", alert.Severity))
}

// GetRealTimeMetrics returns current real-time cost metrics
func (s *RealtimeAggregationService) GetRealTimeMetrics(_ context.Context) (*RealTimeMetrics, error) {
	s.aggregationCache.mu.RLock()
	defer s.aggregationCache.mu.RUnlock()

	now := time.Now()
	metrics := &RealTimeMetrics{
		Timestamp:          now,
		TopCostOperations:  []OperationCost{},
		AlertsTriggered:    []RealTimeAlert{},
		BudgetStatus:       make(map[string]BudgetStatus),
		PerformanceMetrics: make(map[string]float64),
	}

	// Calculate time-based cost metrics
	metrics.TotalCostLast1Min = s.calculateCostForWindow(now.Add(-1*time.Minute), now)
	metrics.TotalCostLast5Min = s.calculateCostForWindow(now.Add(-5*time.Minute), now)
	metrics.TotalCostLast15Min = s.calculateCostForWindow(now.Add(-15*time.Minute), now)
	metrics.TotalCostLastHour = s.calculateCostForWindow(now.Add(-1*time.Hour), now)

	// Calculate cost velocity (cost per minute)
	if metrics.TotalCostLast5Min > 0 {
		metrics.CostVelocity = metrics.TotalCostLast5Min / 5.0
	}

	// Calculate cost acceleration (change in velocity)
	velocityNow := metrics.TotalCostLast1Min
	velocityBefore := (metrics.TotalCostLast5Min - metrics.TotalCostLast1Min) / 4.0
	metrics.CostAcceleration = velocityNow - velocityBefore

	// Get top cost operations
	metrics.TopCostOperations = s.getTopCostOperations(5)

	// Calculate anomaly score
	metrics.AnomalyScore = s.calculateAnomalyScore(metrics)

	// Predict daily cost based on current burn rate
	if metrics.CostVelocity > 0 {
		minutesInDay := 24 * 60
		metrics.PredictedDailyCost = metrics.CostVelocity * float64(minutesInDay)
	}

	// Add performance metrics
	metrics.PerformanceMetrics["cache_hit_rate"] = s.calculateCacheHitRate()
	metrics.PerformanceMetrics["processing_latency_ms"] = s.getAverageProcessingLatency()

	return metrics, nil
}

// Helper methods

func (s *RealtimeAggregationService) getSuccessCount(success bool) int64 {
	if success {
		return 1
	}
	return 0
}

func (s *RealtimeAggregationService) getFailureCount(success bool) int64 {
	if !success {
		return 1
	}
	return 0
}

func (s *RealtimeAggregationService) getTotalOperations(operationCounts map[string]int64) int64 {
	total := int64(0)
	for _, count := range operationCounts {
		total += count
	}
	return total
}

func (s *RealtimeAggregationService) getAvgCostPerOperation(totalCost float64, totalOps int64) float64 {
	if totalOps > 0 {
		return totalCost / float64(totalOps)
	}
	return 0
}

func (s *RealtimeAggregationService) getCurrentMetricValue(metricName string) float64 {
	// Simplified metric calculation - would be more sophisticated in production
	switch metricName {
	case "total_cost_per_minute":
		return s.calculateCostForWindow(time.Now().Add(-1*time.Minute), time.Now())
	case "total_cost_per_hour":
		return s.calculateCostForWindow(time.Now().Add(-1*time.Hour), time.Now())
	default:
		return 0
	}
}

func (s *RealtimeAggregationService) evaluateThreshold(current, threshold float64, op string) bool {
	switch op {
	case "gt":
		return current > threshold
	case "gte":
		return current >= threshold
	case "lt":
		return current < threshold
	case "lte":
		return current <= threshold
	case "eq":
		return current == threshold
	default:
		return false
	}
}

func (s *RealtimeAggregationService) calculateCostForWindow(start, end time.Time) float64 {
	totalCost := 0.0

	for _, summary := range s.aggregationCache.costSummaries {
		if summary.LastUpdated.After(start) && summary.LastUpdated.Before(end) {
			totalCost += summary.TotalCost
		}
	}

	return totalCost
}

func (s *RealtimeAggregationService) getTopCostOperations(limit int) []OperationCost {
	operationCosts := make(map[string]OperationCost)

	// Aggregate costs by operation type
	for _, summary := range s.aggregationCache.costSummaries {
		for opType, count := range summary.OperationCounts {
			if existing, exists := operationCosts[opType]; exists {
				existing.CostDollars += summary.TotalCost
				existing.OperationCount += count
				operationCosts[opType] = existing
			} else {
				operationCosts[opType] = OperationCost{
					OperationType:  opType,
					CostDollars:    summary.TotalCost,
					OperationCount: count,
					TrendDirection: "stable", // Simplified
				}
			}
		}
	}

	// Calculate average cost per operation
	for opType, cost := range operationCosts {
		if cost.OperationCount > 0 {
			cost.AvgCostPer = cost.CostDollars / float64(cost.OperationCount)
			operationCosts[opType] = cost
		}
	}

	// Convert to slice and sort by cost
	var costs []OperationCost
	for _, cost := range operationCosts {
		costs = append(costs, cost)
	}

	sort.Slice(costs, func(i, j int) bool {
		return costs[i].CostDollars > costs[j].CostDollars
	})

	// Limit results
	if len(costs) > limit {
		costs = costs[:limit]
	}

	return costs
}

func (s *RealtimeAggregationService) calculateAnomalyScore(metrics *RealTimeMetrics) float64 {
	score := 0.0

	// High cost velocity increases anomaly score
	if metrics.CostVelocity > 1.0 { // $1/minute threshold
		score += 0.3
	}

	// High cost acceleration increases anomaly score
	if metrics.CostAcceleration > 0.5 {
		score += 0.4
	}

	// Rapid cost increase from last period
	if metrics.TotalCostLast1Min > metrics.TotalCostLast5Min/5.0*2.0 {
		score += 0.3
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

func (s *RealtimeAggregationService) calculateCacheHitRate() float64 {
	// Simplified cache hit rate calculation
	return 0.85 // 85% hit rate
}

func (s *RealtimeAggregationService) getAverageProcessingLatency() float64 {
	totalLatency := int64(0)
	count := 0

	s.mu.RLock()
	for _, processor := range s.streamProcessors {
		if processor.metrics.ProcessingTimeMs > 0 {
			totalLatency += processor.metrics.ProcessingTimeMs
			count++
		}
	}
	s.mu.RUnlock()

	if count > 0 {
		return float64(totalLatency) / float64(count)
	}

	return 0
}

// SetAlertThreshold sets a cost alerting threshold
func (s *RealtimeAggregationService) SetAlertThreshold(threshold *AlertThreshold) {
	s.aggregationCache.mu.Lock()
	defer s.aggregationCache.mu.Unlock()

	s.aggregationCache.alertThresholds[threshold.MetricName] = threshold
}

// GetStreamMetrics returns metrics for stream processing
func (s *RealtimeAggregationService) GetStreamMetrics() map[string]*StreamMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := make(map[string]*StreamMetrics)
	for name, processor := range s.streamProcessors {
		metrics[name] = processor.metrics
	}

	return metrics
}
