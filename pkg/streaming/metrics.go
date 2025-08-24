// Package streaming provides comprehensive metrics collection for WebSocket connections
package streaming

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// MetricsCollector collects and aggregates streaming connection metrics
type MetricsCollector struct {
	connRepo *repositories.StreamingConnectionRepository
	logger   *zap.Logger

	// Aggregated metrics
	mu                  sync.RWMutex
	totalConnections    int64
	activeConnections   int64
	totalMessages       int64
	totalBytes          int64
	averageLatency      time.Duration
	connectionsByState  map[models.ConnectionState]int64
	errorsByType        map[string]int64
	connectionDurations []time.Duration
	messageRates        []float64
	qualityScores       []float64

	// Collection settings
	collectionInterval time.Duration
	retentionPeriod    time.Duration
	isCollecting       bool
	stopChan           chan struct{}
}

// ConnectionMetrics represents metrics for a specific time period
type ConnectionMetrics struct {
	Timestamp             time.Time        `json:"timestamp"`
	TotalConnections      int64            `json:"total_connections"`
	ActiveConnections     int64            `json:"active_connections"`
	ConnectionsByState    map[string]int64 `json:"connections_by_state"`
	TotalMessages         int64            `json:"total_messages"`
	TotalBytes            int64            `json:"total_bytes"`
	AverageLatency        time.Duration    `json:"average_latency"`
	ErrorsByType          map[string]int64 `json:"errors_by_type"`
	AverageQualityScore   float64          `json:"average_quality_score"`
	MessageRate           float64          `json:"message_rate"`           // messages per second
	ByteRate              float64          `json:"byte_rate"`              // bytes per second
	ConnectionUtilization float64          `json:"connection_utilization"` // percentage of max connections used
}

// PerformanceMetrics represents performance-specific metrics
type PerformanceMetrics struct {
	AverageConnectionDuration time.Duration `json:"average_connection_duration"`
	MedianConnectionDuration  time.Duration `json:"median_connection_duration"`
	P95ConnectionDuration     time.Duration `json:"p95_connection_duration"`
	P99ConnectionDuration     time.Duration `json:"p99_connection_duration"`

	AverageMessageRate float64 `json:"average_message_rate"`
	PeakMessageRate    float64 `json:"peak_message_rate"`

	ConnectionSuccessRate float64 `json:"connection_success_rate"`
	MessageDeliveryRate   float64 `json:"message_delivery_rate"`

	HealthyConnections   int64   `json:"healthy_connections"`
	UnhealthyConnections int64   `json:"unhealthy_connections"`
	AverageQualityScore  float64 `json:"average_quality_score"`
}

// MetricsCollectorConfig contains configuration for metrics collection
type MetricsCollectorConfig struct {
	CollectionInterval time.Duration // How often to collect metrics
	RetentionPeriod    time.Duration // How long to retain metrics
}

// DefaultMetricsCollectorConfig returns default configuration
func DefaultMetricsCollectorConfig() *MetricsCollectorConfig {
	return &MetricsCollectorConfig{
		CollectionInterval: time.Minute * 1, // Collect every minute
		RetentionPeriod:    time.Hour * 24,  // Retain for 24 hours
	}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(
	connRepo *repositories.StreamingConnectionRepository,
	logger *zap.Logger,
	config *MetricsCollectorConfig,
) *MetricsCollector {
	if config == nil {
		config = DefaultMetricsCollectorConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &MetricsCollector{
		connRepo:            connRepo,
		logger:              logger.With(zap.String("component", "metrics_collector")),
		connectionsByState:  make(map[models.ConnectionState]int64),
		errorsByType:        make(map[string]int64),
		connectionDurations: make([]time.Duration, 0),
		messageRates:        make([]float64, 0),
		qualityScores:       make([]float64, 0),
		collectionInterval:  config.CollectionInterval,
		retentionPeriod:     config.RetentionPeriod,
		stopChan:            make(chan struct{}),
	}
}

// Start begins metrics collection
func (mc *MetricsCollector) Start(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.isCollecting {
		return fmt.Errorf("metrics collector is already running")
	}

	mc.isCollecting = true

	// Start collection routine
	go mc.collectionRoutine(ctx)

	mc.logger.Info("metrics collector started",
		zap.Duration("collection_interval", mc.collectionInterval))

	return nil
}

// Stop stops metrics collection
func (mc *MetricsCollector) Stop() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if !mc.isCollecting {
		return nil
	}

	close(mc.stopChan)
	mc.isCollecting = false

	mc.logger.Info("metrics collector stopped")
	return nil
}

// IsCollecting returns whether metrics collection is active
func (mc *MetricsCollector) IsCollecting() bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.isCollecting
}

// collectionRoutine runs the metrics collection loop
func (mc *MetricsCollector) collectionRoutine(ctx context.Context) {
	ticker := time.NewTicker(mc.collectionInterval)
	defer ticker.Stop()

	mc.logger.Info("starting metrics collection routine")

	for {
		select {
		case <-mc.stopChan:
			mc.logger.Info("stopping metrics collection routine")
			return
		case <-ctx.Done():
			mc.logger.Info("context cancelled, stopping metrics collection routine")
			return
		case <-ticker.C:
			if err := mc.collectMetrics(ctx); err != nil {
				mc.logger.Error("failed to collect metrics", zap.Error(err))
			}
		}
	}
}

// collectMetrics collects current metrics from all connections
func (mc *MetricsCollector) collectMetrics(ctx context.Context) error {
	start := time.Now()
	mc.logger.Debug("collecting metrics")

	// Get connection pool statistics
	poolStats, err := mc.connRepo.GetConnectionPool(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection pool stats: %w", err)
	}

	// Get all connections for detailed analysis
	allStates := []models.ConnectionState{
		models.ConnectionStateConnecting,
		models.ConnectionStateConnected,
		models.ConnectionStateIdle,
		models.ConnectionStateClosing,
		models.ConnectionStateClosed,
		models.ConnectionStateError,
	}

	allConnections := make([]models.WebSocketConnection, 0)
	stateCount := make(map[models.ConnectionState]int64)

	for _, state := range allStates {
		conns, err := mc.connRepo.GetConnectionsByState(ctx, state)
		if err != nil {
			mc.logger.Error("failed to get connections by state",
				zap.String("state", string(state)),
				zap.Error(err))
			continue
		}

		allConnections = append(allConnections, conns...)
		stateCount[state] = int64(len(conns))
	}

	// Analyze connections and collect metrics
	mc.analyzeConnections(allConnections, stateCount, poolStats)

	duration := time.Since(start)
	mc.logger.Debug("metrics collection completed",
		zap.Int("total_connections", len(allConnections)),
		zap.Duration("duration", duration))

	return nil
}

// analyzeConnections analyzes connections and updates metrics
func (mc *MetricsCollector) analyzeConnections(
	connections []models.WebSocketConnection,
	stateCount map[models.ConnectionState]int64,
	_ map[string]interface{},
) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Reset aggregated metrics
	mc.totalConnections = int64(len(connections))
	mc.activeConnections = stateCount[models.ConnectionStateConnected] + stateCount[models.ConnectionStateIdle]
	mc.connectionsByState = stateCount

	var totalMessages, totalBytes int64
	var totalLatency time.Duration
	var latencyCount int64
	qualitySum := 0.0
	qualityCount := 0

	connectionDurations := make([]time.Duration, 0)
	messageRates := make([]float64, 0)
	qualityScores := make([]float64, 0)
	errorTypes := make(map[string]int64)

	now := time.Now()

	for _, conn := range connections {
		// Aggregate basic metrics
		totalMessages += conn.Metrics.MessagesReceived + conn.Metrics.MessagesSent
		totalBytes += conn.Metrics.BytesReceived + conn.Metrics.BytesSent

		// Aggregate latency (only if we have ping data)
		if conn.Metrics.PingLatencyMs > 0 {
			totalLatency += time.Duration(conn.Metrics.PingLatencyMs) * time.Millisecond
			latencyCount++
		}

		// Connection duration
		duration := now.Sub(conn.Established)
		connectionDurations = append(connectionDurations, duration)

		// Message rate (messages per minute)
		if duration > 0 {
			messagesPerMinute := float64(conn.Metrics.MessagesReceived+conn.Metrics.MessagesSent) / duration.Minutes()
			messageRates = append(messageRates, messagesPerMinute)
		}

		// Quality score
		if conn.Metrics.ConnectionQuality > 0 {
			qualitySum += conn.Metrics.ConnectionQuality
			qualityCount++
			qualityScores = append(qualityScores, conn.Metrics.ConnectionQuality)
		}

		// Error types
		if conn.Metrics.ErrorCount > 0 && conn.Metrics.LastError != "" {
			errorTypes[conn.Metrics.LastError]++
		}
	}

	// Update aggregated metrics
	mc.totalMessages = totalMessages
	mc.totalBytes = totalBytes
	mc.connectionDurations = connectionDurations
	mc.messageRates = messageRates
	mc.qualityScores = qualityScores
	mc.errorsByType = errorTypes

	// Calculate average latency
	if latencyCount > 0 {
		mc.averageLatency = totalLatency / time.Duration(latencyCount)
	}

	// Log summary
	mc.logger.Info("metrics updated",
		zap.Int64("total_connections", mc.totalConnections),
		zap.Int64("active_connections", mc.activeConnections),
		zap.Int64("total_messages", mc.totalMessages),
		zap.Int64("total_bytes", mc.totalBytes),
		zap.Duration("average_latency", mc.averageLatency))
}

// GetCurrentMetrics returns current connection metrics
func (mc *MetricsCollector) GetCurrentMetrics() ConnectionMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Convert state map to string keys for JSON serialization
	statesByString := make(map[string]int64)
	for state, count := range mc.connectionsByState {
		statesByString[string(state)] = count
	}

	// Calculate rates
	messageRate := float64(0)
	byteRate := float64(0)
	if mc.collectionInterval > 0 {
		intervalSeconds := mc.collectionInterval.Seconds()
		messageRate = float64(mc.totalMessages) / intervalSeconds
		byteRate = float64(mc.totalBytes) / intervalSeconds
	}

	// Calculate utilization
	utilization := float64(0)
	if repositories.MaxTotalConnections > 0 {
		utilization = float64(mc.activeConnections) / float64(repositories.MaxTotalConnections) * 100
	}

	// Calculate average quality
	avgQuality := float64(0)
	if len(mc.qualityScores) > 0 {
		qualitySum := float64(0)
		for _, score := range mc.qualityScores {
			qualitySum += score
		}
		avgQuality = qualitySum / float64(len(mc.qualityScores))
	}

	return ConnectionMetrics{
		Timestamp:             time.Now(),
		TotalConnections:      mc.totalConnections,
		ActiveConnections:     mc.activeConnections,
		ConnectionsByState:    statesByString,
		TotalMessages:         mc.totalMessages,
		TotalBytes:            mc.totalBytes,
		AverageLatency:        mc.averageLatency,
		ErrorsByType:          mc.errorsByType,
		AverageQualityScore:   avgQuality,
		MessageRate:           messageRate,
		ByteRate:              byteRate,
		ConnectionUtilization: utilization,
	}
}

// GetPerformanceMetrics returns detailed performance metrics
func (mc *MetricsCollector) GetPerformanceMetrics() PerformanceMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Calculate connection duration statistics
	avgDuration, medianDuration, p95Duration, p99Duration := mc.calculateDurationStats()

	// Calculate message rate statistics
	avgMessageRate, peakMessageRate := mc.calculateMessageRateStats()

	// Calculate success rates
	connectionSuccessRate := mc.calculateConnectionSuccessRate()
	messageDeliveryRate := mc.calculateMessageDeliveryRate()

	// Calculate health statistics
	healthy, unhealthy := mc.calculateHealthStats()

	// Calculate average quality
	avgQuality := float64(0)
	if len(mc.qualityScores) > 0 {
		qualitySum := float64(0)
		for _, score := range mc.qualityScores {
			qualitySum += score
		}
		avgQuality = qualitySum / float64(len(mc.qualityScores))
	}

	return PerformanceMetrics{
		AverageConnectionDuration: avgDuration,
		MedianConnectionDuration:  medianDuration,
		P95ConnectionDuration:     p95Duration,
		P99ConnectionDuration:     p99Duration,
		AverageMessageRate:        avgMessageRate,
		PeakMessageRate:           peakMessageRate,
		ConnectionSuccessRate:     connectionSuccessRate,
		MessageDeliveryRate:       messageDeliveryRate,
		HealthyConnections:        healthy,
		UnhealthyConnections:      unhealthy,
		AverageQualityScore:       avgQuality,
	}
}

// calculateDurationStats calculates connection duration statistics
func (mc *MetricsCollector) calculateDurationStats() (avg, median, p95, p99 time.Duration) {
	if err := common.ValidateSliceNotEmpty("mc.connectionDurations", mc.connectionDurations); err != nil {
		return
	}

	// Sort durations for percentile calculations (simple bubble sort for small datasets)
	sorted := make([]time.Duration, len(mc.connectionDurations))
	copy(sorted, mc.connectionDurations)

	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	// Calculate average
	totalDuration := time.Duration(0)
	for _, d := range sorted {
		totalDuration += d
	}
	avg = totalDuration / time.Duration(len(sorted))

	// Calculate median
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		median = (sorted[mid-1] + sorted[mid]) / 2
	} else {
		median = sorted[mid]
	}

	// Calculate P95 (95th percentile)
	p95Index := int(float64(len(sorted)) * 0.95)
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	p95 = sorted[p95Index]

	// Calculate P99 (99th percentile)
	p99Index := int(float64(len(sorted)) * 0.99)
	if p99Index >= len(sorted) {
		p99Index = len(sorted) - 1
	}
	p99 = sorted[p99Index]

	return
}

// calculateMessageRateStats calculates message rate statistics
func (mc *MetricsCollector) calculateMessageRateStats() (avg, peak float64) {
	if err := common.ValidateSliceNotEmpty("mc.messageRates", mc.messageRates); err != nil {
		return
	}

	totalRate := float64(0)
	peak = mc.messageRates[0]

	for _, rate := range mc.messageRates {
		totalRate += rate
		if rate > peak {
			peak = rate
		}
	}

	avg = totalRate / float64(len(mc.messageRates))
	return
}

// calculateConnectionSuccessRate calculates the connection success rate
func (mc *MetricsCollector) calculateConnectionSuccessRate() float64 {
	successfulStates := mc.connectionsByState[models.ConnectionStateConnected] +
		mc.connectionsByState[models.ConnectionStateIdle]

	if mc.totalConnections == 0 {
		return 1.0
	}

	return float64(successfulStates) / float64(mc.totalConnections)
}

// calculateMessageDeliveryRate calculates the message delivery success rate
func (mc *MetricsCollector) calculateMessageDeliveryRate() float64 {
	// This is a simplified calculation - in reality, you'd track delivery confirmations
	totalErrors := int64(0)
	for _, count := range mc.errorsByType {
		totalErrors += count
	}

	if mc.totalMessages == 0 {
		return 1.0
	}

	deliveredMessages := mc.totalMessages - totalErrors
	if deliveredMessages < 0 {
		deliveredMessages = 0
	}

	return float64(deliveredMessages) / float64(mc.totalMessages)
}

// calculateHealthStats calculates healthy vs unhealthy connection counts
func (mc *MetricsCollector) calculateHealthStats() (healthy, unhealthy int64) {
	healthyStates := mc.connectionsByState[models.ConnectionStateConnected] +
		mc.connectionsByState[models.ConnectionStateIdle]

	unhealthyStates := mc.connectionsByState[models.ConnectionStateError] +
		mc.connectionsByState[models.ConnectionStateClosing]

	return healthyStates, unhealthyStates
}

// GetMetricsSummary returns a summary of key metrics
func (mc *MetricsCollector) GetMetricsSummary() map[string]interface{} {
	current := mc.GetCurrentMetrics()
	performance := mc.GetPerformanceMetrics()

	return map[string]interface{}{
		"current":     current,
		"performance": performance,
		"collection": map[string]interface{}{
			"is_collecting":       mc.IsCollecting(),
			"collection_interval": mc.collectionInterval.String(),
			"retention_period":    mc.retentionPeriod.String(),
		},
	}
}
