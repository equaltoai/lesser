package routing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"go.uber.org/zap"
)

// RoutingMetrics tracks and aggregates routing performance metrics
//
//nolint:revive // Routing prefix clarifies this is routing-specific metrics
type RoutingMetrics struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger

	// Local aggregation (synchronous)
	aggregator *metricsAggregator
}

type metricEvent struct {
	eventType   MetricEventType
	routeID     string
	destination string
	messageType fedTypes.MessageType
	timestamp   time.Time

	// Event-specific data
	latency   time.Duration
	success   bool
	bytesSent int64
	cost      float64
	errorType string
}

// MetricEventType represents the type of metric event
type MetricEventType string

// Metric event types
const (
	EventRouteSelected  MetricEventType = "route_selected"
	EventDeliveryResult MetricEventType = "delivery_result"
	EventCircuitChange  MetricEventType = "circuit_change"
	EventHealthCheck    MetricEventType = "health_check"
)

type metricsAggregator struct {
	// Current window metrics
	windowStart time.Time
	windowSize  time.Duration

	// Aggregated data
	routeMetrics    map[string]*aggregatedRouteMetrics
	instanceMetrics map[string]*aggregatedInstanceMetrics
	globalMetrics   *aggregatedGlobalMetrics

	mu sync.RWMutex
}

type aggregatedRouteMetrics struct {
	RouteID      string
	MessageCount int64
	SuccessCount int64
	FailureCount int64
	TotalBytes   int64
	TotalCost    float64
	TotalLatency time.Duration

	// Latency histogram (in ms buckets)
	LatencyBuckets map[int]int64 // bucket -> count

	// Error tracking
	ErrorTypes map[string]int64

	// Circuit breaker events
	CircuitChanges int64
}

type aggregatedInstanceMetrics struct {
	InstanceID    string
	TotalMessages int64
	TotalBytes    int64
	TotalCost     float64
	HealthChecks  int64
	Availability  float64

	// Message type distribution
	MessageTypes map[fedTypes.MessageType]int64
}

type aggregatedGlobalMetrics struct {
	TotalMessages   int64
	TotalBytes      int64
	TotalCost       float64
	UniqueInstances int64
	ActiveRoutes    int64

	// Time-based patterns
	HourlyVolume [24]int64

	// Top performers
	TopRoutes    []string
	BottomRoutes []string
}

// NewRoutingMetrics creates a new metrics tracker
func NewRoutingMetrics(db *dynamodb.Client, tableName string, logger *zap.Logger) *RoutingMetrics {
	rm := &RoutingMetrics{
		db:        db,
		tableName: tableName,
		logger:    logger,
		aggregator: &metricsAggregator{
			windowStart:     time.Now(),
			windowSize:      5 * time.Minute,
			routeMetrics:    make(map[string]*aggregatedRouteMetrics),
			instanceMetrics: make(map[string]*aggregatedInstanceMetrics),
			globalMetrics:   &aggregatedGlobalMetrics{},
		},
	}

	return rm
}

// RecordRouteSelection records a route selection event
func (rm *RoutingMetrics) RecordRouteSelection(routeID, destination string, messageType fedTypes.MessageType) {
	event := &metricEvent{
		eventType:   EventRouteSelected,
		routeID:     routeID,
		destination: destination,
		messageType: messageType,
		timestamp:   time.Now(),
	}
	rm.processEventSync(event)
}

// RecordDelivery records a delivery result
func (rm *RoutingMetrics) RecordDelivery(result *fedTypes.DeliveryResult) {
	event := &metricEvent{
		eventType: EventDeliveryResult,
		routeID:   result.RouteID,
		timestamp: result.Timestamp,
		latency:   result.Duration,
		success:   result.Success,
		bytesSent: result.BytesSent,
		cost:      result.Cost,
		errorType: result.ErrorMessage,
	}
	rm.processEventSync(event)
}

// RecordCircuitChange records a circuit breaker state change
func (rm *RoutingMetrics) RecordCircuitChange(instanceID string, oldState, newState fedTypes.CircuitStatus) {
	event := &metricEvent{
		eventType: EventCircuitChange,
		routeID:   instanceID,
		timestamp: time.Now(),
		errorType: fmt.Sprintf("%s->%s", oldState, newState),
	}
	rm.processEventSync(event)
}

// RecordHealthCheck records a health check result
func (rm *RoutingMetrics) RecordHealthCheck(instanceID string, health *fedTypes.HealthStatus) {
	event := &metricEvent{
		eventType: EventHealthCheck,
		routeID:   instanceID,
		timestamp: health.Timestamp,
		success:   health.Reachable,
		latency:   health.ResponseTime,
	}
	rm.processEventSync(event)
}

// GetRouteMetrics retrieves metrics for a specific route
func (rm *RoutingMetrics) GetRouteMetrics(ctx context.Context, routeID string, window time.Duration) (*fedTypes.RouteMetrics, error) {
	// Query from DynamoDB
	since := time.Now().Add(-window)

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(rm.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND SK > :since"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("METRICS#ROUTE#%s", routeID)},
			":since": &types.AttributeValueMemberS{Value: fmt.Sprintf("WINDOW#%d", since.Unix())},
		},
		Limit: aws.Int32(100),
	}

	result, err := rm.db.Query(ctx, queryInput)
	if err != nil {
		rm.logger.Error("query route metrics failed",
			zap.String("route_id", routeID),
			zap.Duration("window", window),
			zap.String("operation", "query_route_metrics"),
			zap.Error(err))
		return nil, errors.Join(ErrQueryRouteMetricsFailed, err)
	}

	// Aggregate results
	metrics := &fedTypes.RouteMetrics{
		LastUpdated: time.Now(),
	}

	for _, item := range result.Items {
		rm.aggregateRouteMetric(metrics, item)
	}

	// Add current window data
	rm.aggregator.mu.RLock()
	if current, ok := rm.aggregator.routeMetrics[routeID]; ok {
		metrics.TotalMessages += current.MessageCount
		metrics.SuccessfulCount += current.SuccessCount
		metrics.FailedCount += current.FailureCount
		metrics.TotalBytes += current.TotalBytes
		metrics.TotalCost += current.TotalCost
	}
	rm.aggregator.mu.RUnlock()

	// Calculate percentiles
	if metrics.SuccessfulCount > 0 {
		metrics.AvgLatency = time.Duration(metrics.TotalMessages) / time.Duration(metrics.SuccessfulCount)
	}

	return metrics, nil
}

// GetInstanceMetrics retrieves metrics for a specific instance
func (rm *RoutingMetrics) GetInstanceMetrics(ctx context.Context, instanceID string, window time.Duration) (*InstanceMetrics, error) {
	since := time.Now().Add(-window)

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(rm.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND SK > :since"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("METRICS#INSTANCE#%s", instanceID)},
			":since": &types.AttributeValueMemberS{Value: fmt.Sprintf("WINDOW#%d", since.Unix())},
		},
		Limit: aws.Int32(100),
	}

	result, err := rm.db.Query(ctx, queryInput)
	if err != nil {
		rm.logger.Error("query instance metrics failed",
			zap.String("instance_id", instanceID),
			zap.Duration("window", window),
			zap.String("operation", "query_instance_metrics"),
			zap.Error(err))
		return nil, errors.Join(ErrQueryInstanceMetricsFailed, err)
	}

	metrics := &InstanceMetrics{
		InstanceID:   instanceID,
		Window:       window,
		LastUpdated:  time.Now(),
		MessageTypes: make(map[fedTypes.MessageType]int64),
	}

	for _, item := range result.Items {
		rm.aggregateInstanceMetric(metrics, item)
	}

	return metrics, nil
}

// GetGlobalMetrics retrieves system-wide metrics
func (rm *RoutingMetrics) GetGlobalMetrics(ctx context.Context, window time.Duration) (*GlobalMetrics, error) {
	// Query global metrics
	since := time.Now().Add(-window)

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(rm.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK > :since"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: "METRICS#GLOBAL"},
			":since": &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", since.Unix())},
		},
		Limit: aws.Int32(100),
	}

	result, err := rm.db.Query(ctx, queryInput)
	if err != nil {
		rm.logger.Error("query global metrics failed",
			zap.Duration("window", window),
			zap.String("operation", "query_global_metrics"),
			zap.Time("since", since),
			zap.Error(err))
		return nil, errors.Join(ErrQueryGlobalMetricsFailed, err)
	}

	metrics := &GlobalMetrics{
		Window:       window,
		LastUpdated:  time.Now(),
		HourlyVolume: make(map[int]int64),
	}

	for _, item := range result.Items {
		rm.aggregateGlobalMetric(metrics, item)
	}

	// Add current window
	rm.aggregator.mu.RLock()
	metrics.TotalMessages += rm.aggregator.globalMetrics.TotalMessages
	metrics.TotalBytes += rm.aggregator.globalMetrics.TotalBytes
	metrics.TotalCost += rm.aggregator.globalMetrics.TotalCost
	rm.aggregator.mu.RUnlock()

	return metrics, nil
}

// Flush manually flushes accumulated metrics to DynamoDB
// This should be called at the end of Lambda invocations
func (rm *RoutingMetrics) Flush(ctx context.Context) error {
	rm.aggregator.mu.Lock()
	defer rm.aggregator.mu.Unlock()

	// Check if window should rotate
	if time.Since(rm.aggregator.windowStart) > rm.aggregator.windowSize {
		// Persist current window to DynamoDB
		if err := rm.persistWindow(ctx); err != nil {
			rm.logger.Error("persist metrics window failed",
				zap.Time("window_start", rm.aggregator.windowStart),
				zap.Duration("window_size", rm.aggregator.windowSize),
				zap.Int("route_metrics_count", len(rm.aggregator.routeMetrics)),
				zap.Int("instance_metrics_count", len(rm.aggregator.instanceMetrics)),
				zap.String("operation", "persist_metrics_window"),
				zap.Error(err))
			return errors.Join(ErrPersistMetricsWindowFailed, err)
		}

		// Reset aggregator
		rm.aggregator.windowStart = time.Now()
		rm.aggregator.routeMetrics = make(map[string]*aggregatedRouteMetrics)
		rm.aggregator.instanceMetrics = make(map[string]*aggregatedInstanceMetrics)
		rm.aggregator.globalMetrics = &aggregatedGlobalMetrics{}
	}

	return nil
}

// Private methods

func (rm *RoutingMetrics) processEventSync(event *metricEvent) {
	rm.aggregator.mu.Lock()
	defer rm.aggregator.mu.Unlock()

	switch event.eventType {
	case EventRouteSelected:
		rm.processRouteSelection(event)

	case EventDeliveryResult:
		rm.processDeliveryResult(event)

	case EventCircuitChange:
		rm.processCircuitChange(event)

	case EventHealthCheck:
		rm.processHealthCheck(event)
	}
}

func (rm *RoutingMetrics) processRouteSelection(event *metricEvent) {
	// Update route metrics
	routeMetrics, ok := rm.aggregator.routeMetrics[event.routeID]
	if !ok {
		routeMetrics = &aggregatedRouteMetrics{
			RouteID:        event.routeID,
			LatencyBuckets: make(map[int]int64),
			ErrorTypes:     make(map[string]int64),
		}
		rm.aggregator.routeMetrics[event.routeID] = routeMetrics
	}
	routeMetrics.MessageCount++

	// Update global metrics
	rm.aggregator.globalMetrics.TotalMessages++
	hour := event.timestamp.Hour()
	rm.aggregator.globalMetrics.HourlyVolume[hour]++
}

func (rm *RoutingMetrics) processDeliveryResult(event *metricEvent) {
	// Update route metrics
	routeMetrics, ok := rm.aggregator.routeMetrics[event.routeID]
	if !ok {
		routeMetrics = &aggregatedRouteMetrics{
			RouteID:        event.routeID,
			LatencyBuckets: make(map[int]int64),
			ErrorTypes:     make(map[string]int64),
		}
		rm.aggregator.routeMetrics[event.routeID] = routeMetrics
	}

	if event.success {
		routeMetrics.SuccessCount++
	} else {
		routeMetrics.FailureCount++
		if event.errorType != "" {
			routeMetrics.ErrorTypes[event.errorType]++
		}
	}

	routeMetrics.TotalBytes += event.bytesSent
	routeMetrics.TotalCost += event.cost
	routeMetrics.TotalLatency += event.latency

	// Update latency histogram
	bucket := int(event.latency.Milliseconds()/100) * 100 // 100ms buckets
	routeMetrics.LatencyBuckets[bucket]++

	// Update global metrics
	rm.aggregator.globalMetrics.TotalBytes += event.bytesSent
	rm.aggregator.globalMetrics.TotalCost += event.cost
}

func (rm *RoutingMetrics) processCircuitChange(event *metricEvent) {
	if routeMetrics, ok := rm.aggregator.routeMetrics[event.routeID]; ok {
		routeMetrics.CircuitChanges++
	}
}

func (rm *RoutingMetrics) processHealthCheck(event *metricEvent) {
	// Update instance metrics
	instanceMetrics, ok := rm.aggregator.instanceMetrics[event.routeID]
	if !ok {
		instanceMetrics = &aggregatedInstanceMetrics{
			InstanceID:   event.routeID,
			MessageTypes: make(map[fedTypes.MessageType]int64),
		}
		rm.aggregator.instanceMetrics[event.routeID] = instanceMetrics
	}

	instanceMetrics.HealthChecks++
	if event.success {
		// Update availability (simple moving average)
		instanceMetrics.Availability = (instanceMetrics.Availability*float64(instanceMetrics.HealthChecks-1) + 1.0) / float64(instanceMetrics.HealthChecks)
	} else {
		instanceMetrics.Availability = (instanceMetrics.Availability * float64(instanceMetrics.HealthChecks-1)) / float64(instanceMetrics.HealthChecks)
	}
}

func (rm *RoutingMetrics) persistWindow(ctx context.Context) error {
	windowID := rm.aggregator.windowStart.Unix()

	// Batch write all metrics
	writeRequests := make([]types.WriteRequest, 0, 100)

	// Persist route metrics
	for _, metrics := range rm.aggregator.routeMetrics {
		item := map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("METRICS#ROUTE#%s", metrics.RouteID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("WINDOW#%d", windowID)},

			"MessageCount":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metrics.MessageCount)},
			"SuccessCount":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metrics.SuccessCount)},
			"FailureCount":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metrics.FailureCount)},
			"TotalBytes":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metrics.TotalBytes)},
			"TotalCost":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%.6f", metrics.TotalCost)},
			"AvgLatency":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metrics.TotalLatency.Milliseconds()/metrics.SuccessCount)},
			"CircuitChanges": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", metrics.CircuitChanges)},

			// TTL for cleanup (30 days)
			"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(30*24*time.Hour).Unix())},
		}

		// Add latency histogram
		if len(metrics.LatencyBuckets) > 0 {
			histogram := &types.AttributeValueMemberM{Value: make(map[string]types.AttributeValue)}
			for bucket, count := range metrics.LatencyBuckets {
				histogram.Value[fmt.Sprintf("%d", bucket)] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", count)}
			}
			item["LatencyHistogram"] = histogram
		}

		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		})

		// Flush batch if full
		if len(writeRequests) >= 25 {
			if err := rm.writeBatch(ctx, writeRequests); err != nil {
				return err
			}
			writeRequests = writeRequests[:0]
		}
	}

	// Persist global metrics
	globalItem := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "METRICS#GLOBAL#SUMMARY"},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("WINDOW#%d", windowID)},

		"TotalMessages":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", rm.aggregator.globalMetrics.TotalMessages)},
		"TotalBytes":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", rm.aggregator.globalMetrics.TotalBytes)},
		"TotalCost":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%.6f", rm.aggregator.globalMetrics.TotalCost)},
		"UniqueInstances": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", len(rm.aggregator.instanceMetrics))},
		"ActiveRoutes":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", len(rm.aggregator.routeMetrics))},

		// GSI for time-based queries
		"gsi1PK": &types.AttributeValueMemberS{Value: "METRICS#GLOBAL"},
		"gsi1SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", windowID)},

		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(30*24*time.Hour).Unix())},
	}

	writeRequests = append(writeRequests, types.WriteRequest{
		PutRequest: &types.PutRequest{Item: globalItem},
	})

	// Write remaining batch
	if len(writeRequests) > 0 {
		if err := rm.writeBatch(ctx, writeRequests); err != nil {
			return err
		}
	}

	rm.logger.Info("flushed metrics window",
		zap.Time("windowStart", rm.aggregator.windowStart),
		zap.Int("routeMetrics", len(rm.aggregator.routeMetrics)),
		zap.Int("instanceMetrics", len(rm.aggregator.instanceMetrics)))

	return nil
}

func (rm *RoutingMetrics) writeBatch(ctx context.Context, requests []types.WriteRequest) error {
	batchInput := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			rm.tableName: requests,
		},
	}

	_, err := rm.db.BatchWriteItem(ctx, batchInput)
	if err != nil {
		rm.logger.Error("batch write metrics failed",
			zap.Int("batch_size", len(requests)),
			zap.String("table_name", rm.tableName),
			zap.String("operation", "batch_write_metrics"),
			zap.Error(err))
		return errors.Join(ErrBatchWriteMetricsFailed, err)
	}
	return nil
}

func (rm *RoutingMetrics) aggregateRouteMetric(metrics *fedTypes.RouteMetrics, item map[string]types.AttributeValue) {
	// Parse and aggregate stored metrics
	if v, ok := item["MessageCount"].(*types.AttributeValueMemberN); ok {
		var count int64
		if _, err := fmt.Sscanf(v.Value, "%d", &count); err != nil {
			rm.logger.Warn("failed to parse MessageCount", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.TotalMessages += count
	}
	if v, ok := item["SuccessCount"].(*types.AttributeValueMemberN); ok {
		var count int64
		if _, err := fmt.Sscanf(v.Value, "%d", &count); err != nil {
			rm.logger.Warn("failed to parse SuccessCount", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.SuccessfulCount += count
	}
	if v, ok := item["FailureCount"].(*types.AttributeValueMemberN); ok {
		var count int64
		if _, err := fmt.Sscanf(v.Value, "%d", &count); err != nil {
			rm.logger.Warn("failed to parse FailureCount", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.FailedCount += count
	}
	if v, ok := item["TotalBytes"].(*types.AttributeValueMemberN); ok {
		var bytes int64
		if _, err := fmt.Sscanf(v.Value, "%d", &bytes); err != nil {
			rm.logger.Warn("failed to parse TotalBytes", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.TotalBytes += bytes
	}
	if v, ok := item["TotalCost"].(*types.AttributeValueMemberN); ok {
		var cost float64
		if _, err := fmt.Sscanf(v.Value, "%f", &cost); err != nil {
			rm.logger.Warn("failed to parse TotalCost", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.TotalCost += cost
	}
}

func (rm *RoutingMetrics) aggregateInstanceMetric(metrics *InstanceMetrics, item map[string]types.AttributeValue) {
	// Parse and aggregate instance metrics
	if v, ok := item["TotalMessages"].(*types.AttributeValueMemberN); ok {
		var count int64
		if _, err := fmt.Sscanf(v.Value, "%d", &count); err != nil {
			rm.logger.Warn("failed to parse TotalMessages", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.TotalMessages += count
	}
	if v, ok := item["TotalBytes"].(*types.AttributeValueMemberN); ok {
		var bytes int64
		if _, err := fmt.Sscanf(v.Value, "%d", &bytes); err != nil {
			rm.logger.Warn("failed to parse TotalBytes", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.TotalBytes += bytes
	}
	if v, ok := item["Availability"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%f", &metrics.Availability); err != nil {
			rm.logger.Warn("failed to parse Availability", zap.String("value", v.Value), zap.Error(err))
		}
	}
}

func (rm *RoutingMetrics) aggregateGlobalMetric(metrics *GlobalMetrics, item map[string]types.AttributeValue) {
	// Parse and aggregate global metrics
	if v, ok := item["TotalMessages"].(*types.AttributeValueMemberN); ok {
		var count int64
		if _, err := fmt.Sscanf(v.Value, "%d", &count); err != nil {
			rm.logger.Warn("failed to parse TotalMessages", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.TotalMessages += count
	}
	if v, ok := item["TotalBytes"].(*types.AttributeValueMemberN); ok {
		var bytes int64
		if _, err := fmt.Sscanf(v.Value, "%d", &bytes); err != nil {
			rm.logger.Warn("failed to parse TotalBytes", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.TotalBytes += bytes
	}
	if v, ok := item["TotalCost"].(*types.AttributeValueMemberN); ok {
		var cost float64
		if _, err := fmt.Sscanf(v.Value, "%f", &cost); err != nil {
			rm.logger.Warn("failed to parse TotalCost", zap.String("value", v.Value), zap.Error(err))
		}
		metrics.TotalCost += cost
	}
}

// InstanceMetrics represents metrics for a specific instance
type InstanceMetrics struct {
	InstanceID    string
	Window        time.Duration
	TotalMessages int64
	TotalBytes    int64
	TotalCost     float64
	Availability  float64
	MessageTypes  map[fedTypes.MessageType]int64
	LastUpdated   time.Time
}

// GlobalMetrics represents system-wide metrics
type GlobalMetrics struct {
	Window          time.Duration
	TotalMessages   int64
	TotalBytes      int64
	TotalCost       float64
	ActiveRoutes    int64
	ActiveInstances int64
	HourlyVolume    map[int]int64
	TopRoutes       []RouteRanking
	LastUpdated     time.Time
}

// RouteRanking represents a route's performance ranking
type RouteRanking struct {
	RouteID      string
	SuccessRate  float64
	MessageCount int64
	AvgLatency   time.Duration
}
