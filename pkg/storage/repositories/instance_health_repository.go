package repositories

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamoerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// InstanceHealthRepository handles health check data using BaseRepository with DynamORM
type InstanceHealthRepository struct {
	*EnhancedBaseRepository[*models.InstanceHealth]
	summaryRepo *EnhancedBaseRepository[*models.InstanceHealthSummary]
}

// NewInstanceHealthRepository creates a new instance health repository with enhanced functionality
func NewInstanceHealthRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *InstanceHealthRepository {
	return &InstanceHealthRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.InstanceHealth](db, tableName, logger, costService, "InstanceHealthRepository", "instance_health"),
		summaryRepo:           NewEnhancedBaseRepository[*models.InstanceHealthSummary](db, tableName, logger, costService, "InstanceHealthSummaryRepository", "instance_health_summary"),
	}
}


// SaveHealthCheck stores a health check result with health validation and alerting logic
func (r *InstanceHealthRepository) SaveHealthCheck(ctx context.Context, health *models.InstanceHealth) error {
	// Validate health check data before saving
	if health.Domain == "" {
		return ErrorHandler.HandleCreateError(errors.New("missing domain"), EntityInstanceHealth, "health")
	}
	if health.Timestamp.IsZero() {
		health.Timestamp = time.Now().UTC()
	}

	// Calculate health score for monitoring
	healthScore := health.GetHealthScore()

	// Log critical health issues for alerting
	if health.IsCritical() {
		r.logger.Warn("Critical health issue detected",
			zap.String("domain", health.Domain),
			zap.Bool("reachable", health.Reachable),
			zap.Int("status_code", health.StatusCode),
			zap.Float64("error_rate", health.ErrorRate),
			zap.Float64("health_score", healthScore))
	}

	// Save using BaseRepository
	err := r.ValidateAndCreate(ctx, health)
	if err != nil {
		r.logger.Error("Failed to save health check",
			zap.String("domain", health.Domain),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityInstanceHealth, health.Domain)
	}

	r.logger.Debug("Saved health check",
		zap.String("domain", health.Domain),
		zap.Bool("reachable", health.Reachable),
		zap.Int("status_code", health.StatusCode),
		zap.Duration("response_time", health.ResponseTime),
		zap.Float64("health_score", healthScore))

	return nil
}

// SaveHealthChecks saves multiple health checks in batch with health monitoring validation
func (r *InstanceHealthRepository) SaveHealthChecks(ctx context.Context, healthChecks []*models.InstanceHealth) error {
	if err := common.ValidateSliceNotEmpty("healthChecks", healthChecks); err != nil {
		return nil
	}

	// Validate and process each health check for monitoring
	criticalCount := 0
	healthyCount := 0
	for _, health := range healthChecks {
		if health.Domain == "" {
			return ErrorHandler.HandleCreateError(errors.New("missing domain"), EntityInstanceHealth, "health")
		}
		if health.Timestamp.IsZero() {
			health.Timestamp = time.Now().UTC()
		}

		// Track health status for batch monitoring
		if health.IsCritical() {
			criticalCount++
		} else if health.IsHealthy() {
			healthyCount++
		}
	}

	// Use BaseRepository batch create for efficient operations
	err := r.BatchCreate(ctx, healthChecks)
	if err != nil {
		r.logger.Error("Failed to batch save health checks", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityInstanceHealth, "batch")
	}

	r.logger.Info("Batch saved health checks",
		zap.Int("total_count", len(healthChecks)),
		zap.Int("critical_count", criticalCount),
		zap.Int("healthy_count", healthyCount))

	// Log alert if too many critical instances
	if criticalCount > len(healthChecks)/2 {
		r.logger.Warn("High number of critical instances detected",
			zap.Int("critical_count", criticalCount),
			zap.Int("total_count", len(healthChecks)),
			zap.Float64("critical_percentage", float64(criticalCount)/float64(len(healthChecks))*100))
	}

	return nil
}

// GetLatestHealthCheck retrieves the most recent health check for a domain with health status logging
func (r *InstanceHealthRepository) GetLatestHealthCheck(ctx context.Context, domain string) (*models.InstanceHealth, error) {
	var healthChecks []*models.InstanceHealth

	// Use BaseRepository's underlying DB for complex query
	err := r.GetDB().WithContext(ctx).Model(&models.InstanceHealth{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", domain)).
		Where("SK", ">", "HEALTH#").
		OrderBy("SK", "DESC"). // Descending order for most recent first
		Limit(1).
		All(&healthChecks)

	if err != nil {
		r.logger.Error("Failed to get latest health check",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityInstanceHealth, domain)
	}

	if len(healthChecks) == 0 {
		return nil, ErrorHandler.HandleGetError(errors.New("not found"), EntityInstanceHealth, domain)
	}

	health := healthChecks[0]

	// Log health status for monitoring
	healthScore := health.GetHealthScore()
	r.logger.Debug("Retrieved latest health check",
		zap.String("domain", domain),
		zap.Bool("reachable", health.Reachable),
		zap.Bool("is_healthy", health.IsHealthy()),
		zap.Bool("is_critical", health.IsCritical()),
		zap.Float64("health_score", healthScore),
		zap.Time("timestamp", health.Timestamp))

	return health, nil
}

// GetHealthHistory retrieves health history for a domain within a time range with trend analysis
func (r *InstanceHealthRepository) GetHealthHistory(ctx context.Context, domain string, since time.Time, limit int) ([]*models.InstanceHealth, error) {
	var healthChecks []*models.InstanceHealth

	// Build query using BaseRepository's DB
	query := r.GetDB().WithContext(ctx).Model(&models.InstanceHealth{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", domain)).
		OrderBy("SK", "DESC") // Most recent first

	// Add time range filter if specified
	if !since.IsZero() {
		query = query.Where("SK", ">=", fmt.Sprintf("HEALTH#%d", since.UnixNano()))
	} else {
		// Ensure we only get health records
		query = query.Where("SK", ">", "HEALTH#")
	}

	// Add limit if specified
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&healthChecks)
	if err != nil {
		r.logger.Error("Failed to get health history",
			zap.String("domain", domain),
			zap.Time("since", since),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityInstanceHealth, "health history")
	}

	// Analyze health trends for monitoring
	if len(healthChecks) > 0 {
		healthyCount := 0
		criticalCount := 0
		totalScore := 0.0

		for _, health := range healthChecks {
			if health.IsHealthy() {
				healthyCount++
			}
			if health.IsCritical() {
				criticalCount++
			}
			totalScore += health.GetHealthScore()
		}

		avgScore := totalScore / float64(len(healthChecks))
		healthPercentage := float64(healthyCount) / float64(len(healthChecks)) * 100
		criticalPercentage := float64(criticalCount) / float64(len(healthChecks)) * 100

		r.logger.Debug("Health history trend analysis",
			zap.String("domain", domain),
			zap.Int("total_checks", len(healthChecks)),
			zap.Float64("avg_health_score", avgScore),
			zap.Float64("healthy_percentage", healthPercentage),
			zap.Float64("critical_percentage", criticalPercentage))

		// Alert if domain shows concerning trends
		if criticalPercentage > 25.0 {
			r.logger.Warn("Domain showing concerning health trends",
				zap.String("domain", domain),
				zap.Float64("critical_percentage", criticalPercentage),
				zap.Float64("avg_health_score", avgScore))
		}
	}

	return healthChecks, nil
}

// GetDomainsForHealthCheck retrieves a list of domains that need health checking with monitoring prioritization
func (r *InstanceHealthRepository) GetDomainsForHealthCheck(ctx context.Context, limit int) ([]string, error) {
	// Query for recent health summaries to get active domains
	var summaries []*models.InstanceHealthSummary

	// Use BaseRepository's underlying DB for complex query
	query := r.GetDB().WithContext(ctx).Model(&models.InstanceHealthSummary{}).
		Where("SK", "=", "SUMMARY#24h").
		OrderBy("PK", "ASC") // Consistent ordering

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&summaries)
	if err != nil && !dynamoerrors.IsNotFound(err) {
		r.logger.Error("Failed to get domains for health check", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityInstanceHealth, "health check domains")
	}

	// Prioritize domains based on health status for monitoring efficiency
	type domainPriority struct {
		domain       string
		healthScore  float64
		availability float64
	}

	domainPriorities := make([]domainPriority, 0, len(summaries))
	for _, summary := range summaries {
		domainPriorities = append(domainPriorities, domainPriority{
			domain:       summary.Domain,
			healthScore:  summary.HealthScore,
			availability: summary.Availability,
		})
	}

	// Sort by health score (lowest first) to prioritize problematic instances
	sort.Slice(domainPriorities, func(i, j int) bool {
		return domainPriorities[i].healthScore < domainPriorities[j].healthScore
	})

	domains := make([]string, 0, len(domainPriorities))
	lowHealthCount := 0
	for _, dp := range domainPriorities {
		domains = append(domains, dp.domain)
		if dp.healthScore < 80.0 {
			lowHealthCount++
		}
	}

	// Log monitoring status
	r.logger.Info("Retrieved domains for health checking",
		zap.Int("total_domains", len(domains)),
		zap.Int("low_health_domains", lowHealthCount),
		zap.Int("requested_limit", limit))

	if lowHealthCount > 0 {
		r.logger.Info("Prioritizing unhealthy domains for monitoring",
			zap.Int("low_health_count", lowHealthCount),
			zap.Float64("percentage", float64(lowHealthCount)/float64(len(domains))*100))
	}

	// If no summaries found, we could fallback to querying known remote actors
	if len(domains) == 0 {
		r.logger.Info("No domains found for health checking via summaries")
	}

	return domains, nil
}

// SaveHealthSummary saves an aggregated health summary with uptime monitoring alerts
func (r *InstanceHealthRepository) SaveHealthSummary(ctx context.Context, summary *models.InstanceHealthSummary) error {
	// Validate summary data
	if summary.Domain == "" {
		return ErrorHandler.HandleCreateError(errors.New("missing domain"), EntityHealthSummary, "summary")
	}
	if summary.LastUpdated.IsZero() {
		summary.LastUpdated = time.Now().UTC()
	}

	// Log uptime and availability metrics for monitoring
	r.logger.Info("Saving health summary",
		zap.String("domain", summary.Domain),
		zap.Duration("window", summary.Window),
		zap.Float64("availability", summary.Availability),
		zap.Float64("health_score", summary.HealthScore),
		zap.Int("sample_count", summary.SampleCount),
		zap.Duration("avg_response_time", summary.AvgResponseTime))

	// Alert on poor availability or health scores
	if summary.Availability < 0.95 { // Less than 95% availability
		r.logger.Warn("Low availability detected",
			zap.String("domain", summary.Domain),
			zap.Duration("window", summary.Window),
			zap.Float64("availability", summary.Availability),
			zap.Float64("uptime_percentage", summary.Availability*100))
	}

	if summary.HealthScore < 80.0 { // Health score below 80
		r.logger.Warn("Poor health score detected",
			zap.String("domain", summary.Domain),
			zap.Duration("window", summary.Window),
			zap.Float64("health_score", summary.HealthScore),
			zap.Float64("error_rate", summary.ErrorRate))
	}

	// Save using summary repository
	err := r.summaryRepo.Create(ctx, summary)
	if err != nil {
		r.logger.Error("Failed to save health summary",
			zap.String("domain", summary.Domain),
			zap.Duration("window", summary.Window),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityHealthSummary, summary.Domain)
	}

	return nil
}

// GetHealthSummary retrieves an aggregated health summary with uptime status reporting
func (r *InstanceHealthRepository) GetHealthSummary(ctx context.Context, domain string, window time.Duration) (*models.InstanceHealthSummary, error) {
	// Convert window to string identifier
	var windowStr string
	switch window {
	case time.Hour:
		windowStr = "1h"
	case 24 * time.Hour:
		windowStr = "24h"
	case 7 * 24 * time.Hour:
		windowStr = "7d"
	default:
		windowStr = fmt.Sprintf("%ds", int(window.Seconds()))
	}

	pk := fmt.Sprintf("INSTANCE#%s", domain)
	sk := fmt.Sprintf("SUMMARY#%s", windowStr)

	var summary models.InstanceHealthSummary
	err := r.summaryRepo.Get(ctx, pk, sk, &summary)
	if err != nil {
		if dynamoerrors.IsNotFound(err) {
			r.logger.Debug("Health summary not found",
				zap.String("domain", domain),
				zap.Duration("window", window),
				zap.String("window_str", windowStr))
			return nil, nil
		}
		r.logger.Error("Failed to get health summary",
			zap.String("domain", domain),
			zap.Duration("window", window),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityHealthSummary, domain)
	}

	// Log current status for uptime monitoring
	r.logger.Debug("Retrieved health summary",
		zap.String("domain", domain),
		zap.Duration("window", window),
		zap.Float64("availability", summary.Availability),
		zap.Float64("health_score", summary.HealthScore),
		zap.Time("last_updated", summary.LastUpdated),
		zap.Duration("avg_response_time", summary.AvgResponseTime))

	return &summary, nil
}

// CalculateHealthSummary aggregates health data and creates a summary
func (r *InstanceHealthRepository) CalculateHealthSummary(ctx context.Context, domain string, window time.Duration) (*models.InstanceHealthSummary, error) {
	// Get health history for the window
	since := time.Now().UTC().Add(-window)
	history, err := r.GetHealthHistory(ctx, domain, since, 1000) // Reasonable limit
	if err != nil {
		return nil, err
	}

	if err := common.ValidateSliceNotEmpty("history", history); err != nil {
		return nil, ErrorHandler.HandleQueryError(errors.New("no data in window"), EntityInstanceHealth, domain)
	}

	// Create new summary
	summary := models.NewInstanceHealthSummary(domain, window)
	summary.SampleCount = len(history)

	// Calculate aggregated metrics
	var totalResponseTime time.Duration
	reachableCount := 0
	errorCount := 0
	totalBacklog := 0
	var maxResponseTime time.Duration
	statusCodes := make(map[string]int)

	for _, health := range history {
		if health.Reachable {
			reachableCount++
			totalResponseTime += health.ResponseTime
			if health.ResponseTime > maxResponseTime {
				maxResponseTime = health.ResponseTime
			}
		} else {
			errorCount++
		}

		// Track status codes
		if health.StatusCode > 0 {
			statusCodes[fmt.Sprintf("%d", health.StatusCode)]++
		}

		// Accumulate backlog
		totalBacklog += health.InboxBacklog
	}

	// Calculate final metrics
	summary.Availability = float64(reachableCount) / float64(len(history))
	summary.ErrorRate = float64(errorCount) / float64(len(history))
	summary.MaxResponseTime = maxResponseTime

	if reachableCount > 0 {
		summary.AvgResponseTime = totalResponseTime / time.Duration(reachableCount)
	}

	if len(history) > 0 {
		summary.AvgInboxBacklog = totalBacklog / len(history)
	}

	// Find max backlog
	for _, health := range history {
		if health.InboxBacklog > summary.MaxInboxBacklog {
			summary.MaxInboxBacklog = health.InboxBacklog
		}
	}

	// Calculate health score using the same logic as individual health checks
	summary.HealthScore = r.calculateAggregatedHealthScore(summary)

	// Store status code counts as JSON-serializable map
	summary.StatusCodeCounts = statusCodes

	return summary, nil
}

// calculateAggregatedHealthScore calculates health score for aggregated data
func (r *InstanceHealthRepository) calculateAggregatedHealthScore(summary *models.InstanceHealthSummary) float64 {
	score := 100.0

	// Availability penalty (40% weight)
	score -= (1.0 - summary.Availability) * 40.0

	// Response time penalty (30% weight)
	if summary.AvgResponseTime > time.Second {
		penalty := float64(summary.AvgResponseTime.Milliseconds()-1000) / 100.0
		score -= mathMin(penalty, 30.0)
	}

	// Error rate penalty (20% weight)
	score -= summary.ErrorRate * 20.0

	// Backlog penalty (10% weight)
	if summary.MaxInboxBacklog > 1000 {
		penalty := float64(summary.MaxInboxBacklog-1000) / 1000.0
		score -= mathMin(penalty, 10.0)
	}

	return mathMax(score, 0.0)
}

// CleanupOldHealthData removes health data older than the specified duration
func (r *InstanceHealthRepository) CleanupOldHealthData(_ context.Context, olderThan time.Duration) (int, error) {
	// DynamoDB TTL should handle this automatically, but we can implement manual cleanup if needed
	cutoff := time.Now().UTC().Add(-olderThan)

	r.logger.Info("Health data cleanup triggered (TTL should handle this automatically)",
		zap.Time("cutoff", cutoff),
		zap.Duration("older_than", olderThan))

	// For now, rely on TTL. Could implement manual cleanup later if needed.
	return 0, nil
}

// GetUnhealthyInstances returns instances that are currently unhealthy with detailed health status analysis
func (r *InstanceHealthRepository) GetUnhealthyInstances(ctx context.Context, threshold float64) ([]string, error) {
	if threshold <= 0 {
		threshold = 80.0 // Default threshold for unhealthy instances
	}

	// Query recent summaries and check health scores
	var summaries []*models.InstanceHealthSummary

	err := r.GetDB().WithContext(ctx).Model(&models.InstanceHealthSummary{}).
		Where("SK", "=", "SUMMARY#1h"). // Check hourly summaries
		All(&summaries)

	if err != nil && !dynamoerrors.IsNotFound(err) {
		r.logger.Error("Failed to get unhealthy instances", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityInstanceHealth, "unhealthy instances")
	}

	type unhealthyInstance struct {
		domain       string
		healthScore  float64
		availability float64
		errorRate    float64
	}

	var unhealthyInstances []unhealthyInstance
	for _, summary := range summaries {
		if summary.HealthScore < threshold {
			unhealthyInstances = append(unhealthyInstances, unhealthyInstance{
				domain:       summary.Domain,
				healthScore:  summary.HealthScore,
				availability: summary.Availability,
				errorRate:    summary.ErrorRate,
			})
		}
	}

	// Sort by health score (worst first) for monitoring prioritization
	sort.Slice(unhealthyInstances, func(i, j int) bool {
		return unhealthyInstances[i].healthScore < unhealthyInstances[j].healthScore
	})

	// Extract domain names and log detailed unhealthy status
	unhealthyDomains := make([]string, len(unhealthyInstances))
	criticalCount := 0
	lowAvailabilityCount := 0
	highErrorRateCount := 0

	for i, instance := range unhealthyInstances {
		unhealthyDomains[i] = instance.domain

		// Count different types of health issues
		if instance.healthScore < 50.0 {
			criticalCount++
		}
		if instance.availability < 0.90 {
			lowAvailabilityCount++
		}
		if instance.errorRate > 0.10 {
			highErrorRateCount++
		}

		// Log individual unhealthy instances for monitoring
		r.logger.Warn("Unhealthy instance detected",
			zap.String("domain", instance.domain),
			zap.Float64("health_score", instance.healthScore),
			zap.Float64("availability", instance.availability),
			zap.Float64("error_rate", instance.errorRate),
			zap.Float64("threshold", threshold))
	}

	// Log summary for alerting and monitoring
	r.logger.Info("Unhealthy instances summary",
		zap.Int("total_unhealthy", len(unhealthyDomains)),
		zap.Int("critical_count", criticalCount),
		zap.Int("low_availability_count", lowAvailabilityCount),
		zap.Int("high_error_rate_count", highErrorRateCount),
		zap.Float64("threshold", threshold))

	// Alert if too many instances are unhealthy
	if len(unhealthyDomains) > 0 {
		if criticalCount > 0 {
			r.logger.Error("Critical instances require immediate attention",
				zap.Int("critical_count", criticalCount),
				zap.Strings("critical_domains", unhealthyDomains[:minIntForHealth(criticalCount, 10)])) // Limit to first 10 for logging
		}
	}

	return unhealthyDomains, nil
}

// minIntForHealth helper function specific to instance health
func minIntForHealth(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper functions
func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
