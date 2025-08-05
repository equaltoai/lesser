package routing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// InstanceHealthChecker monitors instance health using DynamORM
type InstanceHealthChecker struct {
	healthRepo *repositories.InstanceHealthRepository
	logger     *zap.Logger
	config     *types.RoutingConfig
	httpClient *http.Client
}


// NewHealthChecker creates a new health checker using DynamORM
func NewHealthChecker(healthRepo *repositories.InstanceHealthRepository, logger *zap.Logger, config *types.RoutingConfig) *InstanceHealthChecker {
	return &InstanceHealthChecker{
		healthRepo: healthRepo,
		logger:     logger,
		config:     config,
		httpClient: &http.Client{
			Timeout: config.HealthCheckTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// StartMonitoring is deprecated - use serverless health checking instead
// This method is kept for compatibility but does nothing
func (hc *InstanceHealthChecker) StartMonitoring(instance *types.Instance) error {
	hc.logger.Warn("StartMonitoring called on DynamORM health checker - use serverless health checking instead")
	return nil
}

// StopMonitoring is deprecated - use serverless health checking instead  
// This method is kept for compatibility but does nothing
func (hc *InstanceHealthChecker) StopMonitoring(instanceID string) error {
	hc.logger.Warn("StopMonitoring called on DynamORM health checker - use serverless health checking instead")
	return nil
}

// CheckHealth performs a health check on an instance and stores the result
func (hc *InstanceHealthChecker) CheckHealth(instance *types.Instance) (*types.HealthStatus, error) {
	ctx := context.Background()
	startTime := time.Now()

	health := &types.HealthStatus{
		Timestamp:    startTime,
		Reachable:    false,
		ResponseTime: 0,
		StatusCode:   0,
	}

	// Perform HTTP health check
	req, err := http.NewRequestWithContext(ctx, "GET", instance.InboxURL, nil)
	if err != nil {
		health.ErrorMessage = fmt.Sprintf("invalid URL: %v", err)
		// Store the failed health check
		hc.storeHealthCheck(ctx, instance.Domain, health)
		return health, err
	}

	// Add headers
	req.Header.Set("User-Agent", "Lesser/1.0 (Federation Health Check)")
	req.Header.Set("Accept", "application/activity+json")

	// Perform request
	resp, err := hc.httpClient.Do(req)
	if err != nil {
		health.ErrorMessage = fmt.Sprintf("request failed: %v", err)
		// Store the failed health check
		hc.storeHealthCheck(ctx, instance.Domain, health)
		return health, nil
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			hc.logger.Warn("Failed to close HTTP response body",
				zap.String("instanceID", instance.ID),
				zap.Error(closeErr))
		}
	}()

	// Update health status
	health.Reachable = true
	health.ResponseTime = time.Since(startTime)
	health.StatusCode = resp.StatusCode

	// Check status code
	if resp.StatusCode >= 500 {
		health.ErrorMessage = fmt.Sprintf("server error: %d", resp.StatusCode)
		health.ErrorRate = 1.0
	} else if resp.StatusCode >= 400 {
		health.ErrorMessage = fmt.Sprintf("client error: %d", resp.StatusCode)
		health.ErrorRate = 0.5
	}

	// Parse additional health info from headers if available
	if backlog := resp.Header.Get("X-Inbox-Backlog"); backlog != "" {
		if _, err := fmt.Sscanf(backlog, "%d", &health.InboxBacklog); err != nil {
			hc.logger.Warn("failed to parse X-Inbox-Backlog header",
				zap.String("value", backlog),
				zap.Error(err))
		}
	}
	if delay := resp.Header.Get("X-Processing-Delay"); delay != "" {
		duration, err := time.ParseDuration(delay)
		if err != nil {
			hc.logger.Warn("failed to parse X-Processing-Delay header",
				zap.String("value", delay),
				zap.Error(err))
		}
		health.ProcessingDelay = duration
	}

	// Store the health check result
	hc.storeHealthCheck(ctx, instance.Domain, health)

	return health, nil
}

// GetHealthHistory retrieves health history for an instance using DynamORM
func (hc *InstanceHealthChecker) GetHealthHistory(instanceID string, duration time.Duration) ([]*types.HealthStatus, error) {
	ctx := context.Background()
	since := time.Now().Add(-duration)
	
	// Use the repository to get health history
	healthRecords, err := hc.healthRepo.GetHealthHistory(ctx, instanceID, since, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get health history: %w", err)
	}

	// Convert models to HealthStatus
	history := make([]*types.HealthStatus, 0, len(healthRecords))
	for _, record := range healthRecords {
		health := &types.HealthStatus{
			Timestamp:       record.Timestamp,
			Reachable:       record.Reachable,
			ResponseTime:    record.ResponseTime,
			StatusCode:      record.StatusCode,
			ErrorMessage:    record.ErrorMessage,
			InboxBacklog:    record.InboxBacklog,
			ProcessingDelay: record.ProcessingDelay,
			ErrorRate:       record.ErrorRate,
		}
		history = append(history, health)
	}

	return history, nil
}

// GetAggregatedHealth returns aggregated health metrics using DynamORM
func (hc *InstanceHealthChecker) GetAggregatedHealth(instanceID string, window time.Duration) (*AggregatedHealth, error) {
	ctx := context.Background()
	
	// Try to get cached summary first
	summary, err := hc.healthRepo.GetHealthSummary(ctx, instanceID, window)
	if err == nil && summary != nil {
		// Convert summary to AggregatedHealth format
		return &AggregatedHealth{
			InstanceID:      instanceID,
			Window:          window,
			SampleCount:     summary.SampleCount,
			LastCheck:       summary.LastUpdated,
			Availability:    summary.Availability,
			AvgResponseTime: summary.AvgResponseTime,
			ErrorRate:       summary.ErrorRate,
			AvgBacklog:      summary.AvgInboxBacklog,
			MaxBacklog:      summary.MaxInboxBacklog,
			HealthScore:     summary.HealthScore,
			StatusCodes:     convertStatusCodes(summary.StatusCodeCounts),
		}, nil
	}
	
	// Fallback to calculating from raw history
	history, err := hc.GetHealthHistory(instanceID, window)
	if err != nil {
		return nil, err
	}

	if len(history) == 0 {
		return nil, fmt.Errorf("no health data available")
	}

	agg := &AggregatedHealth{
		InstanceID:  instanceID,
		Window:      window,
		SampleCount: len(history),
		LastCheck:   history[0].Timestamp,
	}

	// Calculate aggregates
	var totalResponseTime time.Duration
	reachableCount := 0
	errorCount := 0

	for _, h := range history {
		if h.Reachable {
			reachableCount++
			totalResponseTime += h.ResponseTime
		} else {
			errorCount++
		}

		// Track response codes
		if h.StatusCode > 0 {
			if agg.StatusCodes == nil {
				agg.StatusCodes = make(map[int]int)
			}
			agg.StatusCodes[h.StatusCode]++
		}

		// Update backlog stats
		if h.InboxBacklog > agg.MaxBacklog {
			agg.MaxBacklog = h.InboxBacklog
		}
		agg.AvgBacklog += h.InboxBacklog
	}

	// Calculate final metrics
	agg.Availability = float64(reachableCount) / float64(len(history))
	agg.ErrorRate = float64(errorCount) / float64(len(history))

	if reachableCount > 0 {
		agg.AvgResponseTime = totalResponseTime / time.Duration(reachableCount)
	}

	if len(history) > 0 {
		agg.AvgBacklog = agg.AvgBacklog / len(history)
	}

	// Determine health score (0-100)
	agg.HealthScore = hc.calculateHealthScore(agg)

	return agg, nil
}

// Helper methods

// storeHealthCheck stores a health check result using DynamORM
func (hc *InstanceHealthChecker) storeHealthCheck(ctx context.Context, domain string, health *types.HealthStatus) {
	// Convert HealthStatus to InstanceHealth model
	healthModel := &models.InstanceHealth{
		Domain:          domain,
		Timestamp:       health.Timestamp,
		Reachable:       health.Reachable,
		ResponseTime:    health.ResponseTime,
		StatusCode:      health.StatusCode,
		ErrorMessage:    health.ErrorMessage,
		InboxBacklog:    health.InboxBacklog,
		ProcessingDelay: health.ProcessingDelay,
		ErrorRate:       health.ErrorRate,
		CheckerVersion:  "routing-v1",
		UserAgent:       "Lesser/1.0 (Federation Health Check)",
	}

	err := hc.healthRepo.SaveHealthCheck(ctx, healthModel)
	if err != nil {
		hc.logger.Error("Failed to store health check result",
			zap.String("domain", domain),
			zap.Error(err))
	} else {
		hc.logger.Debug("Stored health check result",
			zap.String("domain", domain),
			zap.Bool("reachable", health.Reachable),
			zap.Int("status_code", health.StatusCode))
	}
}

// convertStatusCodes converts map[string]int to map[int]int for compatibility
func convertStatusCodes(statusCodes map[string]int) map[int]int {
	if statusCodes == nil {
		return nil
	}
	
	result := make(map[int]int)
	for codeStr, count := range statusCodes {
		var code int
		if _, err := fmt.Sscanf(codeStr, "%d", &code); err == nil {
			result[code] = count
		}
	}
	return result
}

func (hc *InstanceHealthChecker) calculateHealthScore(agg *AggregatedHealth) float64 {
	score := 100.0

	// Availability (40% weight)
	score -= (1.0 - agg.Availability) * 40.0

	// Response time (30% weight)
	if agg.AvgResponseTime > 1*time.Second {
		penalty := float64(agg.AvgResponseTime.Milliseconds()-1000) / 100.0 // -1 point per 100ms over 1s
		score -= min(penalty, 30.0)
	}

	// Error rate (20% weight)
	score -= agg.ErrorRate * 20.0

	// Backlog (10% weight)
	if agg.MaxBacklog > 1000 {
		penalty := float64(agg.MaxBacklog-1000) / 1000.0 // -1 point per 1000 messages
		score -= min(penalty, 10.0)
	}

	return max(score, 0.0)
}

// AggregatedHealth represents aggregated health metrics
type AggregatedHealth struct {
	InstanceID  string
	Window      time.Duration
	SampleCount int
	LastCheck   time.Time

	Availability    float64
	AvgResponseTime time.Duration
	ErrorRate       float64
	AvgBacklog      int
	MaxBacklog      int
	StatusCodes     map[int]int

	HealthScore float64 // 0-100
}

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
