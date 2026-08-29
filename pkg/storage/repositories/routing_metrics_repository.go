package repositories

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// RoutingMetricsRepository handles routing metrics data persistence using BaseRepository pattern
type RoutingMetricsRepository struct {
	routeMetricsRepo    *EnhancedBaseRepository[*models.RouteMetricsWindow]
	globalMetricsRepo   *EnhancedBaseRepository[*models.GlobalMetricsWindow]
	instanceMetricsRepo *EnhancedBaseRepository[*models.InstanceMetricsWindow]
	logger              *zap.Logger
}

// NewRoutingMetricsRepository creates a new routing metrics repository
func NewRoutingMetricsRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *RoutingMetricsRepository {
	routeRepo := NewEnhancedBaseRepository[*models.RouteMetricsWindow](db, tableName, logger, costService, "RoutingMetricsRepository", "route_metrics")

	globalRepo := NewEnhancedBaseRepository[*models.GlobalMetricsWindow](db, tableName, logger, costService, "GlobalMetricsRepository", "global_metrics")

	instanceRepo := NewEnhancedBaseRepository[*models.InstanceMetricsWindow](db, tableName, logger, costService, "InstanceMetricsRepository", "instance_metrics")

	return &RoutingMetricsRepository{
		routeMetricsRepo:    routeRepo,
		globalMetricsRepo:   globalRepo,
		instanceMetricsRepo: instanceRepo,
		logger:              logger,
	}
}

// NewRoutingMetricsRepositoryWithCostTracking creates a new routing metrics repository with cost tracking
func NewRoutingMetricsRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *RoutingMetricsRepository {
	routeRepo := NewEnhancedBaseRepository[*models.RouteMetricsWindow](db, tableName, logger, costService, "RoutingMetricsRepository", "route_metrics")
	globalRepo := NewEnhancedBaseRepository[*models.GlobalMetricsWindow](db, tableName, logger, costService, "GlobalMetricsRepository", "global_metrics")
	instanceRepo := NewEnhancedBaseRepository[*models.InstanceMetricsWindow](db, tableName, logger, costService, "InstanceMetricsRepository", "instance_metrics")

	return &RoutingMetricsRepository{
		routeMetricsRepo:    routeRepo,
		globalMetricsRepo:   globalRepo,
		instanceMetricsRepo: instanceRepo,
		logger:              logger,
	}
}

// StoreRouteMetricsWindow stores aggregated route metrics for a time window
func (r *RoutingMetricsRepository) StoreRouteMetricsWindow(ctx context.Context, window *models.RouteMetricsWindow) error {
	err := r.routeMetricsRepo.ValidateAndCreateOrUpdate(ctx, window)
	if err != nil {
		r.logger.Error("Failed to store route metrics window",
			zap.String("routeID", window.RouteID),
			zap.Time("windowStart", window.WindowStart),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityRoutingMetrics, window.RouteID)
	}

	r.logger.Debug("Stored route metrics window",
		zap.String("routeID", window.RouteID),
		zap.Time("windowStart", window.WindowStart),
		zap.Int64("messageCount", window.MessageCount))

	return nil
}

// getMetricsWindows is a generic function to retrieve metrics windows
func (r *RoutingMetricsRepository) getMetricsWindows(ctx context.Context, repo interface{ GetDB() core.DB }, metricsType, id string, since time.Time, limit int, model interface{}, result interface{}) error {
	pk := fmt.Sprintf("METRICS#%s#%s", metricsType, id)
	sinceKey := fmt.Sprintf("WINDOW#%d", since.Unix())

	// Floor the page size (wave #1469): a limit <= 0 previously compiled
	// Limit(0) — no limit — an unbounded keyed partition read.
	if limit <= 0 {
		limit = 20
	}

	err := repo.GetDB().WithContext(ctx).Model(model).
		Where("PK", "=", pk).
		Where("SK", ">", sinceKey).
		Limit(limit).
		OrderBy("SK", "DESC"). // Most recent first
		All(result)

	if err != nil {
		r.logger.Error(fmt.Sprintf("Failed to get %s metrics windows", strings.ToLower(metricsType)),
			zap.String(fmt.Sprintf("%sID", strings.ToLower(metricsType)), id),
			zap.Time("since", since),
			zap.Error(err))
		return ErrorHandler.HandleQueryError(err, EntityRoutingMetrics, fmt.Sprintf("%s windows", strings.ToLower(metricsType)))
	}

	r.logger.Debug(fmt.Sprintf("Retrieved %s metrics windows", strings.ToLower(metricsType)),
		zap.String(fmt.Sprintf("%sID", strings.ToLower(metricsType)), id),
		zap.Time("since", since),
		zap.Int("count", reflect.ValueOf(result).Elem().Len()))

	return nil
}

// GetRouteMetricsWindows retrieves route metrics for a time range
func (r *RoutingMetricsRepository) GetRouteMetricsWindows(ctx context.Context, routeID string, since time.Time, limit int) ([]*models.RouteMetricsWindow, error) {
	var windows []*models.RouteMetricsWindow
	err := r.getMetricsWindows(ctx, r.routeMetricsRepo, "ROUTE", routeID, since, limit, &models.RouteMetricsWindow{}, &windows)
	if err != nil {
		return nil, err
	}
	return windows, nil
}

// StoreGlobalMetricsWindow stores aggregated global metrics for a time window
func (r *RoutingMetricsRepository) StoreGlobalMetricsWindow(ctx context.Context, window *models.GlobalMetricsWindow) error {
	err := r.globalMetricsRepo.ValidateAndCreateOrUpdate(ctx, window)
	if err != nil {
		r.logger.Error("Failed to store global metrics window",
			zap.Time("windowStart", window.WindowStart),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityRoutingMetrics, "global")
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

	// Floor the page size (wave #1469): a limit <= 0 previously compiled
	// Limit(0) — no limit — an unbounded keyed gsi1 read.
	if limit <= 0 {
		limit = 20
	}

	err := r.globalMetricsRepo.GetDB().WithContext(ctx).Model(&models.GlobalMetricsWindow{}).
		Index("gsi1").
		Where("gsi1PK", "=", "METRICS#GLOBAL").
		Where("gsi1SK", ">", sinceKey).
		Limit(limit).
		OrderBy("gsi1SK", "DESC"). // Most recent first
		All(&windows)

	if err != nil {
		r.logger.Error("Failed to get global metrics windows",
			zap.Time("since", since),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityRoutingMetrics, "global windows")
	}

	r.logger.Debug("Retrieved global metrics windows",
		zap.Time("since", since),
		zap.Int("count", len(windows)))

	return windows, nil
}

// StoreInstanceMetricsWindow stores aggregated instance metrics for a time window
func (r *RoutingMetricsRepository) StoreInstanceMetricsWindow(ctx context.Context, window *models.InstanceMetricsWindow) error {
	err := r.instanceMetricsRepo.ValidateAndCreateOrUpdate(ctx, window)
	if err != nil {
		r.logger.Error("Failed to store instance metrics window",
			zap.String("instanceID", window.InstanceID),
			zap.Time("windowStart", window.WindowStart),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityRoutingMetrics, window.InstanceID)
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
	err := r.getMetricsWindows(ctx, r.instanceMetricsRepo, "INSTANCE", instanceID, since, limit, &models.InstanceMetricsWindow{}, &windows)
	if err != nil {
		return nil, err
	}
	return windows, nil
}

// BatchStoreMetrics stores multiple metrics windows in batch
func (r *RoutingMetricsRepository) BatchStoreMetrics(ctx context.Context,
	routeWindows []*models.RouteMetricsWindow,
	instanceWindows []*models.InstanceMetricsWindow,
	globalWindow *models.GlobalMetricsWindow) error {

	// BatchCreate is conditionally create-only in TableTheory v3.0.5. These
	// deterministic windows are recomputed, so write each as an explicit upsert.
	if len(routeWindows) > 0 {
		if err := validateAndUpsertMetricsWindows(ctx, r.routeMetricsRepo, routeWindows); err != nil {
			r.logger.Error("Failed to batch store route metrics windows",
				zap.Error(err))
			return ErrorHandler.HandleCreateError(err, EntityRoutingMetrics, "route batch")
		}
	}

	// Store instance windows
	if len(instanceWindows) > 0 {
		if err := validateAndUpsertMetricsWindows(ctx, r.instanceMetricsRepo, instanceWindows); err != nil {
			r.logger.Error("Failed to batch store instance metrics windows",
				zap.Error(err))
			return ErrorHandler.HandleCreateError(err, EntityRoutingMetrics, "instance batch")
		}
	}

	// Store global window
	if globalWindow != nil {
		err := r.globalMetricsRepo.ValidateAndCreateOrUpdate(ctx, globalWindow)
		if err != nil {
			r.logger.Error("Failed to store global metrics window in batch",
				zap.Error(err))
			return ErrorHandler.HandleCreateError(err, EntityRoutingMetrics, "global batch")
		}
	}

	r.logger.Info("Batch stored metrics windows",
		zap.Int("routeWindows", len(routeWindows)),
		zap.Int("instanceWindows", len(instanceWindows)),
		zap.Bool("globalWindow", globalWindow != nil))

	return nil
}

func validateAndUpsertMetricsWindows[T BaseModel](ctx context.Context, repo *EnhancedBaseRepository[T], windows []T) error {
	for _, window := range windows {
		if err := repo.ValidateAndCreateOrUpdate(ctx, window); err != nil {
			return err
		}
	}
	return nil
}

// CleanupExpiredMetrics removes old metrics (handled by TTL, but can be called manually)
func (r *RoutingMetricsRepository) CleanupExpiredMetrics(_ context.Context, before time.Time) error {
	// Since we use TTL, this is mainly for manual cleanup if needed
	// In practice, DynamoDB will automatically remove expired items

	r.logger.Info("Metrics cleanup requested - using TTL for automatic cleanup",
		zap.Time("before", before))

	return nil
}
