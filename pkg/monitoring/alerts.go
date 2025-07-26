package monitoring

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// AlertManager handles production alerting
type AlertManager struct {
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewAlertManager creates a new alert manager
func NewAlertManager(logger *zap.Logger) *AlertManager {
	return &AlertManager{
		logger: logger,
	}
}

// CheckErrorRate triggers alert if error rate is too high
func (am *AlertManager) CheckErrorRate(ctx context.Context, errorRate float64) {
	if errorRate > 5.0 {
		am.logger.Error("high error rate alert",
			zap.Float64("error_rate", errorRate),
			zap.String("severity", "critical"),
		)
	}
}

// CheckLatency triggers alert if latency is too high
func (am *AlertManager) CheckLatency(ctx context.Context, latencyMs float64) {
	if latencyMs > 1000 {
		am.logger.Warn("high latency alert",
			zap.Float64("latency_ms", latencyMs),
			zap.String("severity", "warning"),
		)
	}
}

// CheckCost triggers alert if cost is too high
func (am *AlertManager) CheckCost(ctx context.Context, costMicroCents float64) {
	if costMicroCents > 100000 { // $1 in micro cents
		am.logger.Error("high cost alert",
			zap.Float64("cost_micro_cents", costMicroCents),
			zap.String("severity", "critical"),
		)
	}
}