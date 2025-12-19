package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// RouteOptimizerRepository handles route optimizer data persistence
type RouteOptimizerRepository struct {
	*EnhancedBaseRepository[*models.RouteDeliveryResult]
	optimizationDecisionRepo *EnhancedBaseRepository[*models.OptimizationDecision]
	logger                   *zap.Logger
}

// NewRouteOptimizerRepository creates a new route optimizer repository with enhanced functionality
func NewRouteOptimizerRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *RouteOptimizerRepository {
	// Create enhanced repository optimized for route optimization operations
	baseRepo := NewEnhancedBaseRepository[*models.RouteDeliveryResult](db, tableName, logger, costService, "RouteOptimizerRepository", "route_optimizer")

	// Set up enhanced services for route optimization operations
	baseRepo.SetValidationService(NewDefaultValidationService())
	baseRepo.SetPermissionService(NewDefaultPermissionService())
	baseRepo.SetCachingService(NewInMemoryCachingService()) // Route results cached
	baseRepo.SetEventService(NewDefaultEventService())      // Route optimization events

	optDecisionRepo := NewEnhancedBaseRepository[*models.OptimizationDecision](db, tableName, logger, costService, "OptimizationDecisionRepository", "optimization_decision")

	return &RouteOptimizerRepository{
		EnhancedBaseRepository:   baseRepo,
		optimizationDecisionRepo: optDecisionRepo,
		logger:                   logger,
	}
}

// recordDeliveryResultInternal stores a delivery result for route learning
func (r *RouteOptimizerRepository) recordDeliveryResultInternal(ctx context.Context, result *models.RouteDeliveryResult) error {
	err := r.ValidateAndCreate(ctx, result)
	if err != nil {
		r.logger.Error("Failed to record delivery result",
			zap.String("routeID", result.RouteID),
			zap.String("messageID", result.MessageID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "route optimizer", result.RouteID)
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

	query := r.GetDB().WithContext(ctx).Model(&models.RouteDeliveryResult{}).
		Where("PK", "=", pk).
		Where("SK", "begins_with", "RESULT#").
		OrderBy("SK", "DESC"). // Most recent first
		Limit(limit)

	err := query.All(&results)

	if err != nil {
		r.logger.Error("Failed to get route results",
			zap.String("routeID", routeID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "route optimizer", "route results")
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

	query := r.GetDB().WithContext(ctx).Model(&models.RouteDeliveryResult{}).
		Index("gsi1").
		Where("gsi1PK", "=", "RESULTS").
		Where("gsi1SK", ">", sinceKey).
		OrderBy("gsi1SK", "DESC"). // Most recent first
		Limit(limit)

	err := query.All(&results)

	if err != nil {
		r.logger.Error("Failed to get recent results",
			zap.Time("since", since),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "route optimizer", "recent results")
	}

	r.logger.Debug("Retrieved recent results",
		zap.Time("since", since),
		zap.Int("count", len(results)))

	return results, nil
}

// storeOptimizationDecisionInternal records an optimization decision for analysis
func (r *RouteOptimizerRepository) storeOptimizationDecisionInternal(ctx context.Context, decision *models.OptimizationDecision) error {
	_ = decision.UpdateKeys() // Ignore error as this is internal model operation

	err := r.db.WithContext(ctx).Model(decision).Create()
	if err != nil {
		r.logger.Error("Failed to store optimization decision",
			zap.Time("timestamp", decision.Timestamp),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "optimization decision", "decision")
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

	query := r.optimizationDecisionRepo.GetDB().WithContext(ctx).Model(&models.OptimizationDecision{}).
		Where("PK", "=", "OPTIMIZATION").
		Where("SK", ">", sinceKey).
		OrderBy("SK", "DESC"). // Most recent first
		Limit(limit)

	err := query.All(&decisions)

	if err != nil {
		r.logger.Error("Failed to get optimization decisions",
			zap.Time("since", since),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "optimization decision", "decisions")
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

	if err := common.ValidateSliceNotEmpty("results", results); err != nil {
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
		return ErrorHandler.HandleCreateError(err, "optimization decision", "marshal data")
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

// GetMetricsInRange retrieves delivery results for a specific route within a time range
func (r *RouteOptimizerRepository) GetMetricsInRange(ctx context.Context, routeID string, start, end time.Time, limit int) ([]*types.DeliveryResult, error) {
	r.logger.Debug("Getting metrics in range",
		zap.String("routeID", routeID),
		zap.Time("start", start),
		zap.Time("end", end),
		zap.Int("limit", limit))

	var results []*models.RouteDeliveryResult

	// Query strategy depends on whether we're filtering by specific route
	if routeID != "" {
		// Query by specific route using primary key
		query := r.GetDB().WithContext(ctx).Model(&models.RouteDeliveryResult{}).
			Where("PK", "=", fmt.Sprintf("ROUTE#%s", routeID)).
			Where("SK", ">=", fmt.Sprintf("RESULT#%d", start.UnixNano()))

		if !end.IsZero() {
			query = query.Where("SK", "<=", fmt.Sprintf("RESULT#%d", end.UnixNano()))
		}

		query = query.OrderBy("SK", "DESC").Limit(limit)

		err := query.All(&results)
		if err != nil {
			r.logger.Error("Failed to get route-specific metrics",
				zap.String("routeID", routeID),
				zap.Error(err))
			return nil, ErrorHandler.HandleQueryError(err, "route optimizer", "route metrics")
		}
	} else {
		// Query across all routes using GSI1
		startKey := fmt.Sprintf("%d", start.Unix())
		query := r.GetDB().WithContext(ctx).Model(&models.RouteDeliveryResult{}).
			Index("gsi1").
			Where("gsi1PK", "=", "RESULTS").
			Where("gsi1SK", ">=", startKey)

		if !end.IsZero() {
			endKey := fmt.Sprintf("%d", end.Unix())
			query = query.Where("gsi1SK", "<=", endKey)
		}

		query = query.OrderBy("gsi1SK", "DESC").Limit(limit)

		err := query.All(&results)
		if err != nil {
			r.logger.Error("Failed to get all route metrics",
				zap.Time("start", start),
				zap.Time("end", end),
				zap.Error(err))
			return nil, ErrorHandler.HandleQueryError(err, "route optimizer", "all route metrics")
		}
	}

	// Convert RouteDeliveryResult to DeliveryResult
	deliveryResults := make([]*types.DeliveryResult, 0, len(results))
	for _, result := range results {
		// Extract instance ID from route if possible (routes often encode instance info)
		instanceID := r.extractInstanceFromRoute(result.RouteID)

		deliveryResult := &types.DeliveryResult{
			MessageID:    result.MessageID,
			InstanceID:   instanceID,
			RouteID:      result.RouteID,
			Success:      result.Success,
			StatusCode:   result.StatusCode,
			ErrorMessage: result.ErrorMessage,
			Attempts:     r.estimateAttempts(result.Success, result.StatusCode),
			Duration:     time.Duration(result.Duration) * time.Millisecond,
			BytesSent:    result.BytesSent,
			Cost:         result.Cost,
			Timestamp:    result.Timestamp,
		}
		deliveryResults = append(deliveryResults, deliveryResult)
	}

	r.logger.Debug("Retrieved route metrics",
		zap.String("routeID", routeID),
		zap.Int("results", len(deliveryResults)))

	return deliveryResults, nil
}

// extractInstanceFromRoute attempts to extract instance ID from route ID
func (r *RouteOptimizerRepository) extractInstanceFromRoute(routeID string) string {
	// Route IDs often contain instance information
	// This is a heuristic - adjust based on your route ID format
	if strings.Contains(routeID, "@") {
		parts := strings.Split(routeID, "@")
		if len(parts) > 1 {
			return parts[len(parts)-1] // Return domain part
		}
	}

	// Try to extract from URL-like route IDs
	if strings.Contains(routeID, "://") {
		if parsed, err := url.Parse(routeID); err == nil {
			return parsed.Host
		}
	}

	return "" // No instance ID extractable
}

// estimateAttempts estimates attempt count based on success and status code
func (r *RouteOptimizerRepository) estimateAttempts(success bool, statusCode int) int {
	if success {
		return 1 // Successful on first try
	}

	// Estimate attempts based on status code patterns
	switch {
	case statusCode >= 500: // Server errors typically get more retries
		return 3
	case statusCode >= 400: // Client errors typically get fewer retries
		return 2
	case statusCode == 0: // Network/timeout errors
		return 3
	default:
		return 1
	}
}
