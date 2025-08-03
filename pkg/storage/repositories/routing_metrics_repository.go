package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// RoutingMetricsRepository handles routing metrics data persistence
type RoutingMetricsRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewRoutingMetricsRepository creates a new routing metrics repository
func NewRoutingMetricsRepository(db core.DB, tableName string, logger *zap.Logger) *RoutingMetricsRepository {
	return &RoutingMetricsRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// StoreRouteMetricsWindow stores aggregated route metrics for a time window
func (r *RoutingMetricsRepository) StoreRouteMetricsWindow(ctx context.Context, window *models.RouteMetricsWindow) error {
	window.UpdateKeys()
	
	err := r.db.WithContext(ctx).Model(window).Create()
	if err != nil {
		r.logger.Error("Failed to store route metrics window",
			zap.String("routeID", window.RouteID),
			zap.Time("windowStart", window.WindowStart),
			zap.Error(err))
		return fmt.Errorf("store route metrics window: %w", err)
	}

	r.logger.Debug("Stored route metrics window",
		zap.String("routeID", window.RouteID),
		zap.Time("windowStart", window.WindowStart),
		zap.Int64("messageCount", window.MessageCount))

	return nil
}

// GetRouteMetricsWindows retrieves route metrics for a time range
func (r *RoutingMetricsRepository) GetRouteMetricsWindows(ctx context.Context, routeID string, since time.Time, limit int) ([]*models.RouteMetricsWindow, error) {
	var windows []*models.RouteMetricsWindow
	
	pk := fmt.Sprintf("METRICS#ROUTE#%s", routeID)
	sinceKey := fmt.Sprintf("WINDOW#%d", since.Unix())
	
	err := r.db.WithContext(ctx).Model(&models.RouteMetricsWindow{}).
		Where("PK", "=", pk).
		Where("SK", ">", sinceKey).
		Limit(limit).
		OrderBy("SK", "DESC"). // Most recent first
		All(&windows)

	if err != nil {
		r.logger.Error("Failed to get route metrics windows",
			zap.String("routeID", routeID),
			zap.Time("since", since),
			zap.Error(err))
		return nil, fmt.Errorf("get route metrics windows: %w", err)
	}

	r.logger.Debug("Retrieved route metrics windows",
		zap.String("routeID", routeID),
		zap.Time("since", since),
		zap.Int("count", len(windows)))

	return windows, nil
}

// StoreGlobalMetricsWindow stores aggregated global metrics for a time window
func (r *RoutingMetricsRepository) StoreGlobalMetricsWindow(ctx context.Context, window *models.GlobalMetricsWindow) error {
	window.UpdateKeys()
	
	err := r.db.WithContext(ctx).Model(window).Create()
	if err != nil {
		r.logger.Error("Failed to store global metrics window",
			zap.Time("windowStart", window.WindowStart),
			zap.Error(err))
		return fmt.Errorf("store global metrics window: %w", err)
	}

	r.logger.Debug("Stored global metrics window",
		zap.Time("windowStart", window.WindowStart),
		zap.Int64("totalMessages", window.TotalMessages))

	return nil
}

// GetGlobalMetricsWindows retrieves global metrics for a time range
func (r *RoutingMetricsRepository) GetGlobalMetricsWindows(ctx context.Context, since time.Time, limit int) ([]*models.GlobalMetricsWindow, error) {
	var windows []*models.GlobalMetricsWindow
	
	sinceKey := fmt.Sprintf("%d", since.Unix())
	
	err := r.db.WithContext(ctx).Model(&models.GlobalMetricsWindow{}).
		Index("GSI1").
		Where("GSI1PK", "=", "METRICS#GLOBAL").
		Where("GSI1SK", ">", sinceKey).
		Limit(limit).
		OrderBy("GSI1SK", "DESC"). // Most recent first
		All(&windows)

	if err != nil {
		r.logger.Error("Failed to get global metrics windows",
			zap.Time("since", since),
			zap.Error(err))
		return nil, fmt.Errorf("get global metrics windows: %w", err)
	}

	r.logger.Debug("Retrieved global metrics windows",
		zap.Time("since", since),
		zap.Int("count", len(windows)))

	return windows, nil
}

// StoreInstanceMetricsWindow stores aggregated instance metrics for a time window
func (r *RoutingMetricsRepository) StoreInstanceMetricsWindow(ctx context.Context, window *models.InstanceMetricsWindow) error {
	window.UpdateKeys()
	
	err := r.db.WithContext(ctx).Model(window).Create()
	if err != nil {
		r.logger.Error("Failed to store instance metrics window",
			zap.String("instanceID", window.InstanceID),
			zap.Time("windowStart", window.WindowStart),
			zap.Error(err))
		return fmt.Errorf("store instance metrics window: %w", err)
	}

	r.logger.Debug("Stored instance metrics window",
		zap.String("instanceID", window.InstanceID),
		zap.Time("windowStart", window.WindowStart),
		zap.Int64("totalMessages", window.TotalMessages))

	return nil
}

// GetInstanceMetricsWindows retrieves instance metrics for a time range
func (r *RoutingMetricsRepository) GetInstanceMetricsWindows(ctx context.Context, instanceID string, since time.Time, limit int) ([]*models.InstanceMetricsWindow, error) {
	var windows []*models.InstanceMetricsWindow
	
	pk := fmt.Sprintf("METRICS#INSTANCE#%s", instanceID)
	sinceKey := fmt.Sprintf("WINDOW#%d", since.Unix())
	
	err := r.db.WithContext(ctx).Model(&models.InstanceMetricsWindow{}).
		Where("PK", "=", pk).
		Where("SK", ">", sinceKey).
		Limit(limit).
		OrderBy("SK", "DESC"). // Most recent first
		All(&windows)

	if err != nil {
		r.logger.Error("Failed to get instance metrics windows",
			zap.String("instanceID", instanceID),
			zap.Time("since", since),
			zap.Error(err))
		return nil, fmt.Errorf("get instance metrics windows: %w", err)
	}

	r.logger.Debug("Retrieved instance metrics windows",
		zap.String("instanceID", instanceID),
		zap.Time("since", since),
		zap.Int("count", len(windows)))

	return windows, nil
}

// BatchStoreMetrics stores multiple metrics windows in batch
func (r *RoutingMetricsRepository) BatchStoreMetrics(ctx context.Context, 
	routeWindows []*models.RouteMetricsWindow,
	instanceWindows []*models.InstanceMetricsWindow,
	globalWindow *models.GlobalMetricsWindow) error {
	
	// Update keys for all items
	for _, window := range routeWindows {
		window.UpdateKeys()
	}
	for _, window := range instanceWindows {
		window.UpdateKeys()
	}
	if globalWindow != nil {
		globalWindow.UpdateKeys()
	}

	// Store route windows
	for _, window := range routeWindows {
		if err := r.db.WithContext(ctx).Model(window).Create(); err != nil {
			r.logger.Error("Failed to store route metrics window in batch",
				zap.String("routeID", window.RouteID),
				zap.Error(err))
			return fmt.Errorf("batch store route metrics: %w", err)
		}
	}

	// Store instance windows
	for _, window := range instanceWindows {
		if err := r.db.WithContext(ctx).Model(window).Create(); err != nil {
			r.logger.Error("Failed to store instance metrics window in batch",
				zap.String("instanceID", window.InstanceID),
				zap.Error(err))
			return fmt.Errorf("batch store instance metrics: %w", err)
		}
	}

	// Store global window
	if globalWindow != nil {
		if err := r.db.WithContext(ctx).Model(globalWindow).Create(); err != nil {
			r.logger.Error("Failed to store global metrics window in batch",
				zap.Error(err))
			return fmt.Errorf("batch store global metrics: %w", err)
		}
	}

	r.logger.Info("Batch stored metrics windows",
		zap.Int("routeWindows", len(routeWindows)),
		zap.Int("instanceWindows", len(instanceWindows)),
		zap.Bool("globalWindow", globalWindow != nil))

	return nil
}

// CleanupExpiredMetrics removes old metrics (handled by TTL, but can be called manually)
func (r *RoutingMetricsRepository) CleanupExpiredMetrics(ctx context.Context, before time.Time) error {
	// Since we use TTL, this is mainly for manual cleanup if needed
	// In practice, DynamoDB will automatically remove expired items
	
	r.logger.Info("Metrics cleanup requested - using TTL for automatic cleanup",
		zap.Time("before", before))
	
	return nil
}