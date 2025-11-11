package monitoring

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// ServerlessHealthMonitor provides on-demand health checking for serverless environments
type ServerlessHealthMonitor struct {
	db               core.DB
	logger           *zap.Logger
	requestID        string
	checkers         map[string]ComponentChecker
	metricsPublisher *CloudWatchMetrics
}

// ComponentChecker defines the interface for checking individual components
type ComponentChecker interface {
	Check(ctx context.Context, identifier string) (*ComponentHealthResult, error)
	GetType() string
}

// NewServerlessHealthMonitor creates a new serverless health monitor
func NewServerlessHealthMonitor(db core.DB, logger *zap.Logger) *ServerlessHealthMonitor {
	monitor := &ServerlessHealthMonitor{
		db:               db,
		logger:           logger,
		requestID:        uuid.New().String(),
		checkers:         make(map[string]ComponentChecker),
		metricsPublisher: nil, // Will be set via SetMetricsPublisher if CloudWatch is configured
	}

	// Register built-in checkers
	monitor.RegisterChecker(&DynamoDBChecker{db: db, logger: logger})
	monitor.RegisterChecker(&LambdaChecker{db: db, logger: logger})
	monitor.RegisterChecker(&SQSChecker{db: db, logger: logger})

	return monitor
}

// SetMetricsPublisher sets the CloudWatch metrics publisher for the health monitor
func (s *ServerlessHealthMonitor) SetMetricsPublisher(publisher *CloudWatchMetrics) {
	s.metricsPublisher = publisher
}

// RegisterChecker registers a component checker
func (s *ServerlessHealthMonitor) RegisterChecker(checker ComponentChecker) {
	s.checkers[checker.GetType()] = checker
}

// ProcessHealthCheckEvent processes a health check event synchronously
func (s *ServerlessHealthMonitor) ProcessHealthCheckEvent(ctx context.Context, event HealthCheckEvent) (*HealthCheckResponse, error) {
	startTime := time.Now()

	// Validate the event
	if err := ValidateHealthCheckEvent(event); err != nil {
		return nil, fmt.Errorf("invalid health check event: %w", err)
	}

	s.logger.Info("Processing health check event",
		zap.String("request_id", s.requestID),
		zap.String("action", event.Action),
		zap.Int("component_count", len(event.Components)),
	)

	// Initialize response
	response := &HealthCheckResponse{
		RequestID:        s.requestID,
		Timestamp:        startTime,
		ComponentResults: make([]ComponentHealthResult, 0, len(event.Components)),
		Summary: HealthCheckSummary{
			TotalComponents: len(event.Components),
		},
	}

	// Process each component check
	ctx, cancel := context.WithTimeout(ctx, time.Duration(event.Options.TimeoutSeconds)*time.Second)
	defer cancel()

	for _, component := range event.Components {
		result := s.checkComponent(ctx, component, event.Options)
		response.ComponentResults = append(response.ComponentResults, *result)

		// Update summary counts
		s.updateSummary(&response.Summary, result.Status)

		// Store result if requested
		if event.Options.StoreResults {
			if err := s.storeHealthCheckResult(ctx, result); err != nil {
				s.logger.Warn("Failed to store health check result",
					zap.String("component", result.Component),
					zap.Error(err),
				)
			}
		}

		// Publish metrics if requested
		if event.Options.PublishMetrics {
			s.publishComponentMetrics(ctx, result)
		}
	}

	// Determine overall status
	response.OverallStatus = s.determineOverallStatus(response.ComponentResults)
	response.ExecutionTime = time.Since(startTime).Milliseconds()

	s.logger.Info("Health check completed",
		zap.String("request_id", s.requestID),
		zap.String("overall_status", string(response.OverallStatus)),
		zap.Int64("execution_time_ms", response.ExecutionTime),
		zap.Int("healthy_components", response.Summary.HealthyComponents),
		zap.Int("warning_components", response.Summary.WarningComponents),
		zap.Int("critical_components", response.Summary.CriticalComponents),
	)

	return response, nil
}

// checkComponent performs a health check on a single component with retries
func (s *ServerlessHealthMonitor) checkComponent(ctx context.Context, config ComponentCheckConfig, options HealthCheckOptions) *ComponentHealthResult {
	checker, exists := s.checkers[config.Type]
	if !exists {
		return &ComponentHealthResult{
			Component:     config.Identifier,
			Type:          config.Type,
			Status:        HealthStatusUnknown,
			CheckTime:     time.Now(),
			Error:         fmt.Sprintf("unsupported component type: %s", config.Type),
			RetryAttempts: 0,
		}
	}

	var lastErr error
	var result *ComponentHealthResult

	// Retry logic
	maxAttempts := options.RetryAttempts + 1 // +1 for initial attempt
	for attempt := 0; attempt < maxAttempts; attempt++ {
		attemptStart := time.Now()

		result, lastErr = checker.Check(ctx, config.Identifier)
		if lastErr == nil && result.Status != HealthStatusCritical {
			// Success or non-critical status, no need to retry
			result.RetryAttempts = attempt
			break
		}

		if attempt < maxAttempts-1 {
			// Wait before retry (exponential backoff)
			backoff := time.Duration(attempt+1) * time.Millisecond * 100
			select {
			case <-ctx.Done():
				result = &ComponentHealthResult{
					Component:     config.Identifier,
					Type:          config.Type,
					Status:        HealthStatusCritical,
					CheckTime:     attemptStart,
					Error:         "timeout during health check",
					RetryAttempts: attempt,
				}
				return result
			case <-time.After(backoff):
				continue
			}
		}
	}

	if result == nil {
		result = &ComponentHealthResult{
			Component:     config.Identifier,
			Type:          config.Type,
			Status:        HealthStatusCritical,
			CheckTime:     time.Now(),
			Error:         fmt.Sprintf("all %d attempts failed: %v", maxAttempts, lastErr),
			RetryAttempts: maxAttempts - 1,
		}
	}

	// Set friendly name if provided
	if config.Name != "" {
		if result.Metadata == nil {
			result.Metadata = make(map[string]interface{})
		}
		result.Metadata["friendly_name"] = config.Name
	}

	return result
}

// updateSummary updates the summary counts based on component status
func (s *ServerlessHealthMonitor) updateSummary(summary *HealthCheckSummary, status HealthStatus) {
	switch status {
	case HealthStatusHealthy:
		summary.HealthyComponents++
	case HealthStatusWarning:
		summary.WarningComponents++
	case HealthStatusCritical:
		summary.CriticalComponents++
	default:
		summary.UnknownComponents++
	}

	if status == HealthStatusCritical {
		summary.FailedChecks++
	}
}

// determineOverallStatus determines the overall system health status
func (s *ServerlessHealthMonitor) determineOverallStatus(results []ComponentHealthResult) HealthStatus {
	if err := common.ValidateSliceNotEmpty("results", results); err != nil {
		return HealthStatusUnknown
	}

	hasCritical := false
	hasWarning := false
	hasUnknown := false

	for _, result := range results {
		switch result.Status {
		case HealthStatusCritical:
			hasCritical = true
		case HealthStatusWarning:
			hasWarning = true
		case HealthStatusUnknown:
			hasUnknown = true
		}
	}

	// Priority: Critical > Warning > Unknown > Healthy
	if hasCritical {
		return HealthStatusCritical
	}
	if hasWarning {
		return HealthStatusWarning
	}
	if hasUnknown {
		return HealthStatusUnknown
	}

	return HealthStatusHealthy
}

// storeHealthCheckResult stores a health check result in DynamoDB
func (s *ServerlessHealthMonitor) storeHealthCheckResult(ctx context.Context, result *ComponentHealthResult) error {
	healthResult := models.NewHealthCheckResult(
		result.Type,
		result.Component,
		string(result.Status),
		s.requestID,
		result.CheckTime,
		result.LatencyMs,
	)

	if result.Error != "" {
		healthResult.Error = result.Error
	}

	if result.Metadata != nil {
		healthResult.Metadata = result.Metadata
	}

	return s.db.WithContext(ctx).Model(healthResult).Create()
}

// publishComponentMetrics publishes health check metrics to CloudWatch
func (s *ServerlessHealthMonitor) publishComponentMetrics(_ context.Context, result *ComponentHealthResult) {
	// Check if CloudWatch metrics publisher is available
	if s.metricsPublisher == nil {
		// Fallback to logging if no metrics publisher configured
		s.logger.Debug("Health check metric (CloudWatch not configured)",
			zap.String("component", result.Component),
			zap.String("type", result.Type),
			zap.String("status", string(result.Status)),
			zap.Int64("latency_ms", result.LatencyMs),
			zap.Int("retry_attempts", result.RetryAttempts),
		)
		return
	}

	// Prepare dimensions for CloudWatch metrics
	dimensions := map[string]string{
		"ComponentType": result.Type,
		"Component":     result.Component,
		"HealthStatus":  string(result.Status),
	}

	// Add friendly name if available
	if result.Metadata != nil {
		if friendlyName, ok := result.Metadata["friendly_name"].(string); ok {
			dimensions["FriendlyName"] = friendlyName
		}
	}

	// Publish latency metric
	s.metricsPublisher.RecordBusinessMetrics(
		"HealthCheck.Latency",
		float64(result.LatencyMs),
		types.StandardUnitMilliseconds,
		dimensions,
	)

	// Publish status metric (1 for healthy, 0 for unhealthy)
	statusValue := 0.0
	if result.Status == HealthStatusHealthy {
		statusValue = 1.0
	}
	s.metricsPublisher.RecordBusinessMetrics(
		"HealthCheck.Status",
		statusValue,
		types.StandardUnitNone,
		dimensions,
	)

	// Publish retry attempts if any
	if result.RetryAttempts > 0 {
		s.metricsPublisher.RecordBusinessMetrics(
			"HealthCheck.RetryAttempts",
			float64(result.RetryAttempts),
			types.StandardUnitCount,
			dimensions,
		)
	}

	// Publish error metric if there was an error
	if result.Error != "" {
		errorDims := make(map[string]string)
		for k, v := range dimensions {
			errorDims[k] = v
		}
		// Classify error type if possible
		errorType := "unknown"
		if strings.Contains(result.Error, "timeout") {
			errorType = "timeout"
		} else if strings.Contains(result.Error, "connection") {
			errorType = "connection"
		} else if strings.Contains(result.Error, "permission") || strings.Contains(result.Error, "unauthorized") {
			errorType = "permission"
		} else if strings.Contains(result.Error, "throttle") || strings.Contains(result.Error, "rate") {
			errorType = "throttling"
		}
		errorDims["ErrorType"] = errorType

		s.metricsPublisher.RecordBusinessMetrics(
			"HealthCheck.Errors",
			1.0,
			types.StandardUnitCount,
			errorDims,
		)
	}

	// Log that metrics were published
	s.logger.Debug("Published health check metrics to CloudWatch",
		zap.String("component", result.Component),
		zap.String("type", result.Type),
		zap.String("status", string(result.Status)),
		zap.Int64("latency_ms", result.LatencyMs),
	)
}

// GetStoredResults retrieves stored health check results for a component
func (s *ServerlessHealthMonitor) GetStoredResults(ctx context.Context, componentType, component string, limit int) ([]models.HealthCheckResult, error) {
	var results []models.HealthCheckResult

	gsi1PK := fmt.Sprintf("COMPONENT#%s#%s", componentType, component)

	query := s.db.WithContext(ctx).Model(&models.HealthCheckResult{}).
		Where("gsi1PK", "=", gsi1PK).
		Limit(limit)

	err := query.All(&results)
	if err != nil {
		return nil, fmt.Errorf("failed to query health check results: %w", err)
	}

	return results, nil
}

// GetComponentHistory retrieves health history for a component
func (s *ServerlessHealthMonitor) GetComponentHistory(ctx context.Context, componentType, component string, limit int) ([]models.ComponentHealthHistory, error) {
	var history []models.ComponentHealthHistory

	pk := fmt.Sprintf("COMPONENT_HISTORY#%s#%s", componentType, component)

	query := s.db.WithContext(ctx).Model(&models.ComponentHealthHistory{}).
		Where("PK", "=", pk).
		Limit(limit)

	err := query.All(&history)
	if err != nil {
		return nil, fmt.Errorf("failed to query component history: %w", err)
	}

	return history, nil
}

// CreateHealthCheckSummary creates or updates hourly summary statistics
func (s *ServerlessHealthMonitor) CreateHealthCheckSummary(ctx context.Context, results []ComponentHealthResult) error {
	now := time.Now()
	date := now.Format(common.DateFormat)
	hour := now.Hour()

	// Try to get existing summary
	summary := models.NewHealthCheckSummaryResult(date, hour)

	err := s.db.WithContext(ctx).Model(summary).
		Where("PK = ? AND SK = ?", summary.PK, summary.SK).
		First(summary)

	if err != nil && err != storage.ErrNotFound {
		return fmt.Errorf("failed to query existing summary: %w", err)
	}

	// Add results to summary
	for _, result := range results {
		summary.AddCheckResult(string(result.Status), result.LatencyMs)
	}

	// Save summary
	if err == storage.ErrNotFound {
		return s.db.WithContext(ctx).Model(summary).Create()
	}
	// Update existing summary
	return s.db.WithContext(ctx).Model(summary).Update()
}
