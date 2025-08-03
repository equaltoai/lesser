package repositories

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// InstanceHealthRepository handles health check data using DynamORM
type InstanceHealthRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewInstanceHealthRepository creates a new instance health repository
func NewInstanceHealthRepository(db core.DB, tableName string, logger *zap.Logger) *InstanceHealthRepository {
	return &InstanceHealthRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// SaveHealthCheck stores a health check result
func (r *InstanceHealthRepository) SaveHealthCheck(ctx context.Context, health *models.InstanceHealth) error {
	health.UpdateKeys()

	err := r.db.WithContext(ctx).Model(health).Create()
	if err != nil {
		r.logger.Error("Failed to save health check",
			zap.String("domain", health.Domain),
			zap.Error(err))
		return fmt.Errorf("failed to save health check for %s: %w", health.Domain, err)
	}

	r.logger.Debug("Saved health check",
		zap.String("domain", health.Domain),
		zap.Bool("reachable", health.Reachable),
		zap.Int("status_code", health.StatusCode),
		zap.Duration("response_time", health.ResponseTime))

	return nil
}

// SaveHealthChecks saves multiple health checks in batch
func (r *InstanceHealthRepository) SaveHealthChecks(ctx context.Context, healthChecks []*models.InstanceHealth) error {
	if len(healthChecks) == 0 {
		return nil
	}

	// Update keys for all health checks
	for _, health := range healthChecks {
		health.UpdateKeys()
	}

	// Save health checks one by one (DynamORM batch operations may not be fully implemented)
	for _, health := range healthChecks {
		err := r.db.WithContext(ctx).Model(health).Create()
		if err != nil {
			r.logger.Error("Failed to save health check in batch",
				zap.String("domain", health.Domain),
				zap.Error(err))
			return fmt.Errorf("failed to save health check for %s: %w", health.Domain, err)
		}
	}

	r.logger.Info("Batch saved health checks",
		zap.Int("count", len(healthChecks)))

	return nil
}

// GetLatestHealthCheck retrieves the most recent health check for a domain
func (r *InstanceHealthRepository) GetLatestHealthCheck(ctx context.Context, domain string) (*models.InstanceHealth, error) {
	var healthChecks []models.InstanceHealth
	
	err := r.db.WithContext(ctx).Model(&models.InstanceHealth{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", domain)).
		Where("SK", ">", "HEALTH#").
		OrderBy("SK", "DESC"). // Descending order for most recent first
		Limit(1).
		All(&healthChecks)

	if err != nil {
		r.logger.Error("Failed to get latest health check",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get latest health check for %s: %w", domain, err)
	}

	if len(healthChecks) == 0 {
		return nil, fmt.Errorf("no health checks found for domain %s", domain)
	}

	return &healthChecks[0], nil
}

// GetHealthHistory retrieves health history for a domain within a time range
func (r *InstanceHealthRepository) GetHealthHistory(ctx context.Context, domain string, since time.Time, limit int) ([]*models.InstanceHealth, error) {
	var healthChecks []models.InstanceHealth
	
	// Build query
	query := r.db.WithContext(ctx).Model(&models.InstanceHealth{}).
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
		return nil, fmt.Errorf("failed to get health history for %s: %w", domain, err)
	}

	// Convert to pointer slice
	result := make([]*models.InstanceHealth, len(healthChecks))
	for i := range healthChecks {
		result[i] = &healthChecks[i]
	}

	return result, nil
}

// GetDomainsForHealthCheck retrieves a list of domains that need health checking
// This queries for all known instances based on recent activity
func (r *InstanceHealthRepository) GetDomainsForHealthCheck(ctx context.Context, limit int) ([]string, error) {
	// Query for recent health summaries to get active domains
	var summaries []models.InstanceHealthSummary
	
	query := r.db.WithContext(ctx).Model(&models.InstanceHealthSummary{}).
		Where("SK", "=", "SUMMARY#24h")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	err := query.All(&summaries)
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("Failed to get domains for health check", zap.Error(err))
		return nil, fmt.Errorf("failed to get domains for health check: %w", err)
	}

	domains := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		domains = append(domains, summary.Domain)
	}

	// If no summaries found, we could fallback to querying known remote actors
	// For now, return what we have
	if len(domains) == 0 {
		r.logger.Info("No domains found for health checking via summaries")
	}

	return domains, nil
}

// SaveHealthSummary saves an aggregated health summary
func (r *InstanceHealthRepository) SaveHealthSummary(ctx context.Context, summary *models.InstanceHealthSummary) error {
	summary.UpdateKeys()

	err := r.db.WithContext(ctx).Model(summary).Create()
	if err != nil {
		r.logger.Error("Failed to save health summary",
			zap.String("domain", summary.Domain),
			zap.Duration("window", summary.Window),
			zap.Error(err))
		return fmt.Errorf("failed to save health summary for %s: %w", summary.Domain, err)
	}

	return nil
}

// GetHealthSummary retrieves an aggregated health summary for a domain and time window
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

	var summary models.InstanceHealthSummary
	err := r.db.WithContext(ctx).Model(&models.InstanceHealthSummary{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", domain)).
		Where("SK", "=", fmt.Sprintf("SUMMARY#%s", windowStr)).
		First(&summary)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		r.logger.Error("Failed to get health summary",
			zap.String("domain", domain),
			zap.Duration("window", window),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get health summary for %s: %w", domain, err)
	}

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

	if len(history) == 0 {
		return nil, fmt.Errorf("no health data available for domain %s in the last %v", domain, window)
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
		score -= min(penalty, 30.0)
	}

	// Error rate penalty (20% weight)
	score -= summary.ErrorRate * 20.0

	// Backlog penalty (10% weight)
	if summary.MaxInboxBacklog > 1000 {
		penalty := float64(summary.MaxInboxBacklog-1000) / 1000.0
		score -= min(penalty, 10.0)
	}

	return max(score, 0.0)
}

// CleanupOldHealthData removes health data older than the specified duration
func (r *InstanceHealthRepository) CleanupOldHealthData(ctx context.Context, olderThan time.Duration) (int, error) {
	// DynamoDB TTL should handle this automatically, but we can implement manual cleanup if needed
	cutoff := time.Now().UTC().Add(-olderThan)
	
	r.logger.Info("Health data cleanup triggered (TTL should handle this automatically)",
		zap.Time("cutoff", cutoff),
		zap.Duration("older_than", olderThan))

	// For now, rely on TTL. Could implement manual cleanup later if needed.
	return 0, nil
}

// GetUnhealthyInstances returns instances that are currently unhealthy
func (r *InstanceHealthRepository) GetUnhealthyInstances(ctx context.Context, threshold float64) ([]string, error) {
	// Query recent summaries and check health scores
	var summaries []models.InstanceHealthSummary
	
	err := r.db.WithContext(ctx).Model(&models.InstanceHealthSummary{}).
		Where("SK", "=", "SUMMARY#1h"). // Check hourly summaries
		All(&summaries)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("Failed to get unhealthy instances", zap.Error(err))
		return nil, fmt.Errorf("failed to get unhealthy instances: %w", err)
	}

	var unhealthy []string
	for _, summary := range summaries {
		if summary.HealthScore < threshold {
			unhealthy = append(unhealthy, summary.Domain)
		}
	}

	// Sort for consistent results
	sort.Strings(unhealthy)

	return unhealthy, nil
}

// Helper functions
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}