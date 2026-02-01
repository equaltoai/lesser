package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// RoutingMetrics tracks and aggregates routing performance metrics
//
//nolint:revive // Routing prefix clarifies this is routing-specific metrics
type RoutingMetrics struct {
	db     core.DB
	logger *zap.Logger

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
func NewRoutingMetrics(db core.DB, logger *zap.Logger) *RoutingMetrics {
	rm := &RoutingMetrics{
		db:     db,
		logger: logger,
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
	if rm == nil || rm.db == nil {
		return nil, errors.Join(ErrQueryRouteMetricsFailed, fmt.Errorf("routing metrics DB is not configured"))
	}

	since := time.Now().Add(-window)
	pk := fmt.Sprintf("METRICS#ROUTE#%s", routeID)
	sinceKey := fmt.Sprintf("WINDOW#%d", since.Unix())

	var windows []*storagemodels.RouteMetricsWindow
	err := rm.db.WithContext(ctx).Model(&storagemodels.RouteMetricsWindow{}).
		Where("PK", "=", pk).
		Where("SK", ">", sinceKey).
		Limit(100).
		All(&windows)
	if err != nil {
		if rm.logger != nil {
			rm.logger.Error("query route metrics failed",
				zap.String("route_id", routeID),
				zap.Duration("window", window),
				zap.String("operation", "query_route_metrics"),
				zap.Error(err))
		}
		return nil, errors.Join(ErrQueryRouteMetricsFailed, err)
	}

	metrics := &fedTypes.RouteMetrics{LastUpdated: time.Now()}

	var weightedLatencyMs int64
	var successCount int64

	for _, w := range windows {
		if w == nil {
			continue
		}

		metrics.TotalMessages += w.MessageCount
		metrics.SuccessfulCount += w.SuccessCount
		metrics.FailedCount += w.FailureCount
		metrics.TotalBytes += w.TotalBytes
		metrics.TotalCost += w.TotalCost

		if w.SuccessCount > 0 {
			successCount += w.SuccessCount
			weightedLatencyMs += w.AvgLatency * w.SuccessCount
		}
	}

	// Add current window data
	rm.aggregator.mu.RLock()
	if current, ok := rm.aggregator.routeMetrics[routeID]; ok && current != nil {
		metrics.TotalMessages += current.MessageCount
		metrics.SuccessfulCount += current.SuccessCount
		metrics.FailedCount += current.FailureCount
		metrics.TotalBytes += current.TotalBytes
		metrics.TotalCost += current.TotalCost

		if current.SuccessCount > 0 {
			successCount += current.SuccessCount
			weightedLatencyMs += current.TotalLatency.Milliseconds()
		}
	}
	rm.aggregator.mu.RUnlock()

	if successCount > 0 {
		metrics.AvgLatency = time.Duration(weightedLatencyMs/successCount) * time.Millisecond
	}

	return metrics, nil
}

// GetInstanceMetrics retrieves metrics for a specific instance
func (rm *RoutingMetrics) GetInstanceMetrics(ctx context.Context, instanceID string, window time.Duration) (*InstanceMetrics, error) {
	if rm == nil || rm.db == nil {
		return nil, errors.Join(ErrQueryInstanceMetricsFailed, fmt.Errorf("routing metrics DB is not configured"))
	}

	since := time.Now().Add(-window)
	pk := fmt.Sprintf("METRICS#INSTANCE#%s", instanceID)
	sinceKey := fmt.Sprintf("WINDOW#%d", since.Unix())

	var windows []*storagemodels.InstanceMetricsWindow
	err := rm.db.WithContext(ctx).Model(&storagemodels.InstanceMetricsWindow{}).
		Where("PK", "=", pk).
		Where("SK", ">", sinceKey).
		Limit(100).
		OrderBy("SK", "DESC").
		All(&windows)
	if err != nil {
		if rm.logger != nil {
			rm.logger.Error("query instance metrics failed",
				zap.String("instance_id", instanceID),
				zap.Duration("window", window),
				zap.String("operation", "query_instance_metrics"),
				zap.Error(err))
		}
		return nil, errors.Join(ErrQueryInstanceMetricsFailed, err)
	}

	metrics := &InstanceMetrics{
		InstanceID:   instanceID,
		Window:       window,
		LastUpdated:  time.Now(),
		MessageTypes: make(map[fedTypes.MessageType]int64),
	}

	availabilitySet := false
	for _, w := range windows {
		if w == nil {
			continue
		}

		metrics.TotalMessages += w.TotalMessages
		metrics.TotalBytes += w.TotalBytes
		metrics.TotalCost += w.TotalCost

		if !availabilitySet {
			metrics.Availability = w.Availability
			availabilitySet = true
		}

		if w.MessageTypes != "" {
			var msgTypes map[string]int64
			if jsonErr := json.Unmarshal([]byte(w.MessageTypes), &msgTypes); jsonErr == nil {
				for mt, count := range msgTypes {
					metrics.MessageTypes[fedTypes.MessageType(mt)] += count
				}
			}
		}
	}

	// Add current window (in-memory) instance metrics, if present.
	rm.aggregator.mu.RLock()
	if current, ok := rm.aggregator.instanceMetrics[instanceID]; ok && current != nil {
		metrics.TotalMessages += current.TotalMessages
		metrics.TotalBytes += current.TotalBytes
		metrics.TotalCost += current.TotalCost
		if !availabilitySet {
			metrics.Availability = current.Availability
			availabilitySet = true
		}
		for mt, count := range current.MessageTypes {
			metrics.MessageTypes[mt] += count
		}
	}
	rm.aggregator.mu.RUnlock()

	return metrics, nil
}

// GetGlobalMetrics retrieves system-wide metrics
func (rm *RoutingMetrics) GetGlobalMetrics(ctx context.Context, window time.Duration) (*GlobalMetrics, error) {
	if rm == nil || rm.db == nil {
		return nil, errors.Join(ErrQueryGlobalMetricsFailed, fmt.Errorf("routing metrics DB is not configured"))
	}

	since := time.Now().Add(-window)
	sinceKey := fmt.Sprintf("%d", since.Unix())

	var windows []*storagemodels.GlobalMetricsWindow
	err := rm.db.WithContext(ctx).Model(&storagemodels.GlobalMetricsWindow{}).
		Index("gsi1").
		Where("gsi1PK", "=", "METRICS#GLOBAL").
		Where("gsi1SK", ">", sinceKey).
		Limit(100).
		OrderBy("gsi1SK", "DESC").
		All(&windows)
	if err != nil {
		if rm.logger != nil {
			rm.logger.Error("query global metrics failed",
				zap.Duration("window", window),
				zap.String("operation", "query_global_metrics"),
				zap.Time("since", since),
				zap.Error(err))
		}
		return nil, errors.Join(ErrQueryGlobalMetricsFailed, err)
	}

	metrics := &GlobalMetrics{
		Window:       window,
		LastUpdated:  time.Now(),
		HourlyVolume: make(map[int]int64),
	}

	activeSet := false
	for _, w := range windows {
		if w == nil {
			continue
		}

		metrics.TotalMessages += w.TotalMessages
		metrics.TotalBytes += w.TotalBytes
		metrics.TotalCost += w.TotalCost

		if !activeSet {
			metrics.ActiveRoutes = w.ActiveRoutes
			metrics.ActiveInstances = w.UniqueInstances
			activeSet = true
		}

		if w.HourlyVolume != "" {
			var hourly []int64
			if jsonErr := json.Unmarshal([]byte(w.HourlyVolume), &hourly); jsonErr == nil {
				for i := 0; i < len(hourly) && i < 24; i++ {
					metrics.HourlyVolume[i] += hourly[i]
				}
			}
		}
	}

	// Add current window
	rm.aggregator.mu.RLock()
	metrics.TotalMessages += rm.aggregator.globalMetrics.TotalMessages
	metrics.TotalBytes += rm.aggregator.globalMetrics.TotalBytes
	metrics.TotalCost += rm.aggregator.globalMetrics.TotalCost
	for hour, count := range rm.aggregator.globalMetrics.HourlyVolume {
		metrics.HourlyVolume[hour] += count
	}
	rm.aggregator.mu.RUnlock()

	return metrics, nil
}

// Flush manually flushes accumulated metrics to DynamoDB (via TableTheory).
// This should be called at the end of Lambda invocations
func (rm *RoutingMetrics) Flush(ctx context.Context) error {
	rm.aggregator.mu.Lock()
	defer rm.aggregator.mu.Unlock()

	// Check if window should rotate
	if time.Since(rm.aggregator.windowStart) > rm.aggregator.windowSize {
		// Persist current window to DynamoDB
		if err := rm.persistWindow(ctx); err != nil {
			if rm.logger != nil {
				rm.logger.Error("persist metrics window failed",
					zap.Time("window_start", rm.aggregator.windowStart),
					zap.Duration("window_size", rm.aggregator.windowSize),
					zap.Int("route_metrics_count", len(rm.aggregator.routeMetrics)),
					zap.Int("instance_metrics_count", len(rm.aggregator.instanceMetrics)),
					zap.String("operation", "persist_metrics_window"),
					zap.Error(err),
				)
			}
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
	if rm == nil || rm.db == nil {
		return errors.Join(ErrPersistMetricsWindowFailed, fmt.Errorf("routing metrics DB is not configured"))
	}

	windowStart := rm.aggregator.windowStart
	windowSizeMinutes := int64(rm.aggregator.windowSize / time.Minute)

	routeWindows := make([]*storagemodels.RouteMetricsWindow, 0, len(rm.aggregator.routeMetrics))
	for _, m := range rm.aggregator.routeMetrics {
		if m == nil {
			continue
		}

		avgLatencyMs := int64(0)
		if m.SuccessCount > 0 {
			avgLatencyMs = m.TotalLatency.Milliseconds() / m.SuccessCount
		}

		latencyHistogram, err := json.Marshal(m.LatencyBuckets)
		if err != nil && rm.logger != nil {
			rm.logger.Warn("failed to encode latency histogram", zap.String("route_id", m.RouteID), zap.Error(err))
		}

		errorTypes, err := json.Marshal(m.ErrorTypes)
		if err != nil && rm.logger != nil {
			rm.logger.Warn("failed to encode error types", zap.String("route_id", m.RouteID), zap.Error(err))
		}

		window := &storagemodels.RouteMetricsWindow{
			RouteID:     m.RouteID,
			WindowStart: windowStart,
			WindowSize:  windowSizeMinutes,

			MessageCount:   m.MessageCount,
			SuccessCount:   m.SuccessCount,
			FailureCount:   m.FailureCount,
			TotalBytes:     m.TotalBytes,
			TotalCost:      m.TotalCost,
			AvgLatency:     avgLatencyMs,
			CircuitChanges: m.CircuitChanges,

			LatencyHistogram: string(latencyHistogram),
			ErrorTypes:       string(errorTypes),
		}
		if err := window.UpdateKeys(); err != nil {
			return err
		}
		routeWindows = append(routeWindows, window)
	}

	instanceWindows := make([]*storagemodels.InstanceMetricsWindow, 0, len(rm.aggregator.instanceMetrics))
	for _, m := range rm.aggregator.instanceMetrics {
		if m == nil {
			continue
		}

		messageTypes, err := json.Marshal(m.MessageTypes)
		if err != nil && rm.logger != nil {
			rm.logger.Warn("failed to encode message types", zap.String("instance_id", m.InstanceID), zap.Error(err))
		}

		window := &storagemodels.InstanceMetricsWindow{
			InstanceID:  m.InstanceID,
			WindowStart: windowStart,
			WindowSize:  windowSizeMinutes,

			TotalMessages: m.TotalMessages,
			TotalBytes:    m.TotalBytes,
			TotalCost:     m.TotalCost,
			HealthChecks:  m.HealthChecks,
			Availability:  m.Availability,

			MessageTypes: string(messageTypes),
		}
		if err := window.UpdateKeys(); err != nil {
			return err
		}
		instanceWindows = append(instanceWindows, window)
	}

	hourlyVolume, err := json.Marshal(rm.aggregator.globalMetrics.HourlyVolume)
	if err != nil && rm.logger != nil {
		rm.logger.Warn("failed to encode hourly volume", zap.Error(err))
	}

	globalWindow := &storagemodels.GlobalMetricsWindow{
		WindowStart: windowStart,
		WindowSize:  windowSizeMinutes,

		TotalMessages:   rm.aggregator.globalMetrics.TotalMessages,
		TotalBytes:      rm.aggregator.globalMetrics.TotalBytes,
		TotalCost:       rm.aggregator.globalMetrics.TotalCost,
		UniqueInstances: int64(len(rm.aggregator.instanceMetrics)),
		ActiveRoutes:    int64(len(rm.aggregator.routeMetrics)),

		HourlyVolume: string(hourlyVolume),
	}
	if err := globalWindow.UpdateKeys(); err != nil {
		return err
	}

	if len(routeWindows) > 0 {
		if err := rm.db.WithContext(ctx).Model(&storagemodels.RouteMetricsWindow{}).BatchCreate(routeWindows); err != nil {
			return errors.Join(ErrBatchWriteMetricsFailed, err)
		}
	}

	if len(instanceWindows) > 0 {
		if err := rm.db.WithContext(ctx).Model(&storagemodels.InstanceMetricsWindow{}).BatchCreate(instanceWindows); err != nil {
			return errors.Join(ErrBatchWriteMetricsFailed, err)
		}
	}

	if err := rm.db.WithContext(ctx).Model(globalWindow).CreateOrUpdate(); err != nil {
		return errors.Join(ErrBatchWriteMetricsFailed, err)
	}

	if rm.logger != nil {
		rm.logger.Info("flushed metrics window",
			zap.Time("windowStart", rm.aggregator.windowStart),
			zap.Int("routeMetrics", len(rm.aggregator.routeMetrics)),
			zap.Int("instanceMetrics", len(rm.aggregator.instanceMetrics)),
		)
	}

	return nil
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
