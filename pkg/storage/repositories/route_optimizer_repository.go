package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// RouteOptimizerRepository handles route optimizer data persistence
type RouteOptimizerRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewRouteOptimizerRepository creates a new route optimizer repository
func NewRouteOptimizerRepository(db core.DB, tableName string, logger *zap.Logger) *RouteOptimizerRepository {
	return &RouteOptimizerRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// recordDeliveryResultInternal stores a delivery result for route learning
func (r *RouteOptimizerRepository) recordDeliveryResultInternal(ctx context.Context, result *models.RouteDeliveryResult) error {
	result.UpdateKeys()

	err := r.db.WithContext(ctx).Model(result).Create()
	if err != nil {
		r.logger.Error("Failed to record delivery result",
			zap.String("routeID", result.RouteID),
			zap.String("messageID", result.MessageID),
			zap.Error(err))
		return fmt.Errorf("record delivery result: %w", err)
	}

	r.logger.Debug("Recorded delivery result",
		zap.String("routeID", result.RouteID),
		zap.String("messageID", result.MessageID),
		zap.Bool("success", result.Success))

	return nil
}

// GetRouteResults retrieves recent delivery results for a route
func (r *RouteOptimizerRepository) GetRouteResults(ctx context.Context, routeID string, limit int) ([]*models.RouteDeliveryResult, error) {
	var results []*models.RouteDeliveryResult

	pk := fmt.Sprintf("ROUTE#%s", routeID)

	query := r.db.WithContext(ctx).Model(&models.RouteDeliveryResult{}).
		Where("PK", "=", pk).
		Where("SK", "begins_with", "RESULT#").
		OrderBy("SK", "DESC"). // Most recent first
		Limit(limit)

	err := query.All(&results)

	if err != nil {
		r.logger.Error("Failed to get route results",
			zap.String("routeID", routeID),
			zap.Error(err))
		return nil, fmt.Errorf("get route results: %w", err)
	}

	r.logger.Debug("Retrieved route results",
		zap.String("routeID", routeID),
		zap.Int("count", len(results)))

	return results, nil
}

// GetRecentResults retrieves recent delivery results across all routes
func (r *RouteOptimizerRepository) GetRecentResults(ctx context.Context, since time.Time, limit int) ([]*models.RouteDeliveryResult, error) {
	var results []*models.RouteDeliveryResult

	sinceKey := fmt.Sprintf("%d", since.Unix())

	query := r.db.WithContext(ctx).Model(&models.RouteDeliveryResult{}).
		Index("GSI1").
		Where("GSI1PK", "=", "RESULTS").
		Where("GSI1SK", ">", sinceKey).
		OrderBy("GSI1SK", "DESC"). // Most recent first
		Limit(limit)

	err := query.All(&results)

	if err != nil {
		r.logger.Error("Failed to get recent results",
			zap.Time("since", since),
			zap.Error(err))
		return nil, fmt.Errorf("get recent results: %w", err)
	}

	r.logger.Debug("Retrieved recent results",
		zap.Time("since", since),
		zap.Int("count", len(results)))

	return results, nil
}

// storeOptimizationDecisionInternal records an optimization decision for analysis
func (r *RouteOptimizerRepository) storeOptimizationDecisionInternal(ctx context.Context, decision *models.OptimizationDecision) error {
	decision.UpdateKeys()

	err := r.db.WithContext(ctx).Model(decision).Create()
	if err != nil {
		r.logger.Error("Failed to store optimization decision",
			zap.Time("timestamp", decision.Timestamp),
			zap.Error(err))
		return fmt.Errorf("store optimization decision: %w", err)
	}

	r.logger.Debug("Stored optimization decision",
		zap.Time("timestamp", decision.Timestamp),
		zap.Int64("messageSize", decision.MessageSize),
		zap.Int("routeCount", len(decision.RouteIDs)))

	return nil
}

// GetOptimizationDecisions retrieves recent optimization decisions
func (r *RouteOptimizerRepository) GetOptimizationDecisions(ctx context.Context, since time.Time, limit int) ([]*models.OptimizationDecision, error) {
	var decisions []*models.OptimizationDecision

	sinceKey := fmt.Sprintf("DECISION#%d", since.UnixNano())

	query := r.db.WithContext(ctx).Model(&models.OptimizationDecision{}).
		Where("PK", "=", "OPTIMIZATION").
		Where("SK", ">", sinceKey).
		OrderBy("SK", "DESC"). // Most recent first
		Limit(limit)

	err := query.All(&decisions)

	if err != nil {
		r.logger.Error("Failed to get optimization decisions",
			zap.Time("since", since),
			zap.Error(err))
		return nil, fmt.Errorf("get optimization decisions: %w", err)
	}

	r.logger.Debug("Retrieved optimization decisions",
		zap.Time("since", since),
		zap.Int("count", len(decisions)))

	return decisions, nil
}

// RecordDeliveryResult converts federation DeliveryResult and stores it (implements interface)
func (r *RouteOptimizerRepository) RecordDeliveryResult(ctx context.Context, result *types.DeliveryResult) error {
	deliveryResult := &models.RouteDeliveryResult{
		MessageID:    result.MessageID,
		RouteID:      result.RouteID,
		Success:      result.Success,
		StatusCode:   result.StatusCode,
		Duration:     result.Duration.Milliseconds(),
		BytesSent:    result.BytesSent,
		Cost:         result.Cost,
		ErrorMessage: result.ErrorMessage,
		Timestamp:    result.Timestamp,
	}

	return r.recordDeliveryResultInternal(ctx, deliveryResult)
}

// GetRouteMetricsForFederation calculates route metrics for federation types
func (r *RouteOptimizerRepository) GetRouteMetricsForFederation(ctx context.Context, routeID string) (*types.RouteMetrics, error) {
	results, err := r.GetRouteResults(ctx, routeID, 1000) // Get last 1000 results
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return &types.RouteMetrics{
			LastUpdated: time.Now(),
		}, nil
	}

	metrics := &types.RouteMetrics{
		LastUpdated: time.Now(),
	}

	latencies := []time.Duration{}

	for _, result := range results {
		metrics.TotalMessages++
		if result.Success {
			metrics.SuccessfulCount++
			latencies = append(latencies, time.Duration(result.Duration)*time.Millisecond)
		} else {
			metrics.FailedCount++
		}
		metrics.TotalBytes += result.BytesSent
		metrics.TotalCost += result.Cost
	}

	// Calculate latency percentiles
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})

		// Average
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		metrics.AvgLatency = total / time.Duration(len(latencies))

		// P95
		p95Index := int(float64(len(latencies)) * 0.95)
		if p95Index >= len(latencies) {
			p95Index = len(latencies) - 1
		}
		metrics.P95Latency = latencies[p95Index]

		// P99
		p99Index := int(float64(len(latencies)) * 0.99)
		if p99Index >= len(latencies) {
			p99Index = len(latencies) - 1
		}
		metrics.P99Latency = latencies[p99Index]
	}

	return metrics, nil
}

// GetRoutePerformanceData returns internal performance data for optimization
func (r *RouteOptimizerRepository) GetRoutePerformanceData(ctx context.Context, routeID string) (interface{}, error) {
	results, err := r.GetRouteResults(ctx, routeID, 1000)
	if err != nil {
		return nil, err
	}

	// Return the raw results for internal processing
	// The optimizer can process these results for its internal needs
	return results, nil
}

// StoreOptimizationDecision stores optimization decision from route array (implements interface)
func (r *RouteOptimizerRepository) StoreOptimizationDecision(ctx context.Context, routes []*types.Route, messageSize int64) error {
	routeIDs := make([]string, len(routes))
	for i, route := range routes {
		routeIDs[i] = route.ID
	}

	// Create decision data
	decisionData := map[string]interface{}{
		"timestamp":   time.Now(),
		"messageSize": messageSize,
		"routes":      routeIDs,
	}

	decisionJSON, err := json.Marshal(decisionData)
	if err != nil {
		return fmt.Errorf("marshal decision data: %w", err)
	}

	decision := &models.OptimizationDecision{
		Timestamp:   time.Now(),
		MessageSize: messageSize,
		RouteIDs:    routeIDs,
		Decision:    string(decisionJSON),
	}

	return r.storeOptimizationDecisionInternal(ctx, decision)
}

// GetRouteMetrics implements RouteOptimizationRepository interface (delegates to GetRouteMetricsForFederation)
func (r *RouteOptimizerRepository) GetRouteMetrics(ctx context.Context, routeID string) (*types.RouteMetrics, error) {
	return r.GetRouteMetricsForFederation(ctx, routeID)
}

// GetRoutePerformance implements RouteOptimizationRepository interface (delegates to GetRoutePerformanceData)
func (r *RouteOptimizerRepository) GetRoutePerformance(ctx context.Context, routeID string) (interface{}, error) {
	return r.GetRoutePerformanceData(ctx, routeID)
}

// CleanupExpiredResults removes old delivery results (handled by TTL, but can be called manually)
func (r *RouteOptimizerRepository) CleanupExpiredResults(_ context.Context, before time.Time) error {
	// Since we use TTL, this is mainly for manual cleanup if needed
	// In practice, DynamoDB will automatically remove expired items

	r.logger.Info("Cleanup requested - using TTL for automatic cleanup",
		zap.Time("before", before))

	return nil
}
