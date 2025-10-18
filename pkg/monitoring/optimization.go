package monitoring

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// OptimizationTracker tracks Lambda performance optimizations
type OptimizationTracker struct {
	logger     *zap.Logger
	mu         sync.RWMutex
	coldStarts int64
	warmStarts int64
}

// NewOptimizationTracker creates a new optimization tracker
func NewOptimizationTracker(logger *zap.Logger) *OptimizationTracker {
	return &OptimizationTracker{
		logger: logger,
	}
}

// TrackColdStart records a cold start event
func (ot *OptimizationTracker) TrackColdStart(_ context.Context) {
	ot.mu.Lock()
	ot.coldStarts++
	ot.mu.Unlock()

	ot.logger.Info("cold start detected",
		zap.Int64("total_cold_starts", ot.coldStarts),
	)
}

// TrackWarmStart records a warm start event
func (ot *OptimizationTracker) TrackWarmStart(_ context.Context) {
	ot.mu.Lock()
	ot.warmStarts++
	ot.mu.Unlock()

	ot.logger.Debug("warm start",
		zap.Int64("total_warm_starts", ot.warmStarts),
	)
}

// GetColdStartRatio returns the ratio of cold starts to total starts
func (ot *OptimizationTracker) GetColdStartRatio() float64 {
	ot.mu.RLock()
	defer ot.mu.RUnlock()

	total := ot.coldStarts + ot.warmStarts
	if total == 0 {
		return 0
	}
	return float64(ot.coldStarts) / float64(total)
}

// TrackLatency records request latency
func (ot *OptimizationTracker) TrackLatency(_ context.Context, operation string, duration time.Duration) {
	ot.logger.Info("operation latency",
		zap.String("operation", operation),
		zap.Duration("duration", duration),
	)
}

// TrackDBQuery records database query performance
func (ot *OptimizationTracker) TrackDBQuery(_ context.Context, tableName string, operation string, duration time.Duration) {
	ot.logger.Info("db query performance",
		zap.String("table", tableName),
		zap.String("operation", operation),
		zap.Duration("duration", duration),
	)
}
