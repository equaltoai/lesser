package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// ServerlessHealthChecker performs stateless health checks triggered by EventBridge
type ServerlessHealthChecker struct {
	// Dependencies
	healthRepo *repositories.InstanceHealthRepository
	logger     *zap.Logger
	httpClient *http.Client

	// Configuration
	config CheckerConfig
}

// CheckerConfig contains configuration for the health checker
type CheckerConfig struct {
	// HTTP configuration
	RequestTimeout      time.Duration `json:"request_timeout"`
	MaxIdleConns        int           `json:"max_idle_conns"`
	MaxIdleConnsPerHost int           `json:"max_idle_conns_per_host"`
	IdleConnTimeout     time.Duration `json:"idle_conn_timeout"`
	FollowRedirects     bool          `json:"follow_redirects"`
	UserAgent           string        `json:"user_agent"`

	// Concurrency configuration
	MaxConcurrentChecks int `json:"max_concurrent_checks"`

	// Retry configuration
	MaxRetries   int           `json:"max_retries"`
	RetryBackoff time.Duration `json:"retry_backoff"`

	// Validation configuration
	RequiredHeaders  map[string]string `json:"required_headers"`
	ValidStatusCodes []int             `json:"valid_status_codes"`
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() CheckerConfig {
	return CheckerConfig{
		RequestTimeout:      30 * time.Second,
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		FollowRedirects:     true,
		UserAgent:           "Lesser/1.0 (Federation Health Check)",
		MaxConcurrentChecks: 10,
		MaxRetries:          2,
		RetryBackoff:        time.Second,
		RequiredHeaders: map[string]string{
			"Accept": "application/activity+json, application/ld+json",
		},
		ValidStatusCodes: []int{200, 201, 202, 204, 301, 302, 303, 307, 308},
	}
}

// NewServerlessHealthChecker creates a new serverless health checker
func NewServerlessHealthChecker(db core.DB, tableName string, logger *zap.Logger, config CheckerConfig) *ServerlessHealthChecker {
	// Create HTTP client with optimized settings for Lambda
	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		DisableCompression:  true, // Reduce CPU usage in Lambda
	}

	// Configure redirect policy
	client := &http.Client{
		Transport: transport,
		Timeout:   config.RequestTimeout,
	}

	if !config.FollowRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return &ServerlessHealthChecker{
		healthRepo: repositories.NewInstanceHealthRepository(db, tableName, logger),
		logger:     logger,
		httpClient: client,
		config:     config,
	}
}

// ProcessHealthCheckEvent processes an EventBridge health check event
func (c *ServerlessHealthChecker) ProcessHealthCheckEvent(ctx context.Context, event *HealthCheckEvent) (*HealthCheckResult, error) {
	startTime := time.Now()

	c.logger.Info("Processing health check event",
		zap.String("action", event.Detail.Action),
		zap.Int("domain_count", len(event.Detail.Domains)),
		zap.Int("batch_size", event.Detail.BatchSize))

	// Validate event
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid health check event: %w", err)
	}

	var result *HealthCheckResult
	var err error

	switch event.Detail.Action {
	case "check_health":
		result, err = c.checkInstanceHealth(ctx, event.Detail.Domains)
	case "aggregate_summary":
		err = c.aggregateHealthSummaries(ctx, event.Detail.Domains, event.Detail.SummaryWindows)
		if err == nil {
			result = &HealthCheckResult{
				EventID:   fmt.Sprintf("agg-%d", time.Now().UnixNano()),
				Source:    "lesser.federation.health",
				Timestamp: time.Now().UTC(),
			}
		}
	case "cleanup":
		count, cleanupErr := c.cleanupOldData(ctx, event.Detail.RetentionDays)
		if cleanupErr != nil {
			err = cleanupErr
		} else {
			result = &HealthCheckResult{
				EventID:          fmt.Sprintf("cleanup-%d", time.Now().UnixNano()),
				Source:           "lesser.federation.health",
				Timestamp:        time.Now().UTC(),
				SuccessfulChecks: count,
			}
		}
	default:
		err = fmt.Errorf("unknown action: %s", event.Detail.Action)
	}

	if err != nil {
		return nil, err
	}

	// Update performance metrics
	if result != nil {
		result.TotalDuration = time.Since(startTime)
		if len(result.Results) > 0 {
			var totalDuration time.Duration
			for _, domainResult := range result.Results {
				totalDuration += domainResult.ResponseTime
			}
			result.AvgDuration = totalDuration / time.Duration(len(result.Results))
		}
	}

	c.logger.Info("Completed health check event processing",
		zap.String("action", event.Detail.Action),
		zap.Duration("total_duration", result.TotalDuration),
		zap.Int("successful_checks", result.SuccessfulChecks),
		zap.Int("failed_checks", result.FailedChecks))

	return result, nil
}

// checkInstanceHealth performs health checks on the specified domains
func (c *ServerlessHealthChecker) checkInstanceHealth(ctx context.Context, domains []string) (*HealthCheckResult, error) {
	if err := common.ValidateSliceNotEmpty("domains", domains); err != nil {
		return nil, fmt.Errorf("no domains specified for health check")
	}

	result := &HealthCheckResult{
		EventID:        fmt.Sprintf("health-%d", time.Now().UnixNano()),
		Source:         "lesser.federation.health",
		Timestamp:      time.Now().UTC(),
		CheckedDomains: domains,
		Results:        make([]DomainHealthCheckResult, 0, len(domains)),
		Errors:         make([]HealthCheckError, 0),
	}

	// Create a semaphore to limit concurrent checks
	semaphore := make(chan struct{}, c.config.MaxConcurrentChecks)
	var wg sync.WaitGroup

	// Channel to collect results
	resultChan := make(chan DomainHealthCheckResult, len(domains))
	errorChan := make(chan HealthCheckError, len(domains))

	// Launch health checks concurrently
	for _, domain := range domains {
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			domainResult, err := c.checkSingleDomain(ctx, domain)
			if err != nil {
				errorChan <- HealthCheckError{
					Domain:       domain,
					ErrorType:    "check_failed",
					ErrorMessage: err.Error(),
					Timestamp:    time.Now().UTC(),
				}
			} else {
				resultChan <- *domainResult
			}
		}(domain)
	}

	// Wait for all checks to complete
	wg.Wait()
	close(resultChan)
	close(errorChan)

	// Collect results
	for domainResult := range resultChan {
		result.Results = append(result.Results, domainResult)
		if domainResult.Success {
			result.SuccessfulChecks++
		} else {
			result.FailedChecks++
		}
	}

	// Collect errors
	for error := range errorChan {
		result.Errors = append(result.Errors, error)
		result.FailedChecks++
	}

	// Save health check results to database
	if len(result.Results) > 0 {
		healthChecks := make([]*models.InstanceHealth, 0, len(result.Results))
		for _, domainResult := range result.Results {
			health := c.convertToHealthModel(domainResult)
			healthChecks = append(healthChecks, health)
		}

		if err := c.healthRepo.SaveHealthChecks(ctx, healthChecks); err != nil {
			c.logger.Error("Failed to save health check results", zap.Error(err))
			// Don't fail the entire operation, but log the error
		}
	}

	return result, nil
}

// checkSingleDomain performs a health check on a single domain
func (c *ServerlessHealthChecker) checkSingleDomain(ctx context.Context, domain string) (*DomainHealthCheckResult, error) {
	startTime := time.Now()

	result := &DomainHealthCheckResult{
		Domain:    domain,
		CheckedAt: startTime.UTC(),
		Success:   false,
		Reachable: false,
	}

	// Construct inbox URL (ActivityPub standard)
	inboxURL := fmt.Sprintf("https://%s/inbox", domain)

	// Create HTTP request with context for timeout
	req, err := http.NewRequestWithContext(ctx, "GET", inboxURL, nil)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("invalid URL: %v", err)
		return result, nil
	}

	// Set required headers
	req.Header.Set("User-Agent", c.config.UserAgent)
	for key, value := range c.config.RequiredHeaders {
		req.Header.Set(key, value)
	}

	// Perform the request with retries
	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			time.Sleep(c.config.RetryBackoff * time.Duration(attempt))
		}

		resp, lastErr = c.httpClient.Do(req)
		if lastErr == nil {
			break
		}

		c.logger.Debug("HTTP request failed, retrying",
			zap.String("domain", domain),
			zap.Int("attempt", attempt+1),
			zap.Error(lastErr))
	}

	if lastErr != nil {
		result.ErrorMessage = fmt.Sprintf("request failed after %d attempts: %v", c.config.MaxRetries+1, lastErr)
		result.ResponseTime = time.Since(startTime)
		return result, nil
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Warn("Failed to close HTTP response body",
				zap.String("domain", domain),
				zap.Error(closeErr))
		}
	}()

	// Update result with response data
	result.Reachable = true
	result.StatusCode = resp.StatusCode
	result.ResponseTime = time.Since(startTime)

	// Check if status code is valid
	isValidStatus := false
	for _, validCode := range c.config.ValidStatusCodes {
		if resp.StatusCode == validCode {
			isValidStatus = true
			break
		}
	}

	if !isValidStatus {
		result.ErrorMessage = fmt.Sprintf("invalid status code: %d", resp.StatusCode)
		result.Success = false
	} else {
		result.Success = true
	}

	// Parse additional federation metrics from headers
	if backlog := resp.Header.Get("X-Inbox-Backlog"); backlog != "" {
		if _, err := fmt.Sscanf(backlog, "%d", &result.InboxBacklog); err != nil {
			c.logger.Debug("Failed to parse X-Inbox-Backlog header",
				zap.String("domain", domain),
				zap.String("value", backlog),
				zap.Error(err))
		}
	}

	if delay := resp.Header.Get("X-Processing-Delay"); delay != "" {
		if duration, err := time.ParseDuration(delay); err == nil {
			result.ProcessingDelay = duration
		} else {
			c.logger.Debug("Failed to parse X-Processing-Delay header",
				zap.String("domain", domain),
				zap.String("value", delay),
				zap.Error(err))
		}
	}

	// Calculate health score
	health := c.convertToHealthModel(*result)
	result.HealthScore = health.GetHealthScore()

	return result, nil
}

// convertToHealthModel converts a domain result to a health model
func (c *ServerlessHealthChecker) convertToHealthModel(result DomainHealthCheckResult) *models.InstanceHealth {
	health := models.NewInstanceHealth(result.Domain)
	health.Timestamp = result.CheckedAt
	health.Reachable = result.Reachable
	health.ResponseTime = result.ResponseTime
	health.StatusCode = result.StatusCode
	health.ErrorMessage = result.ErrorMessage
	health.InboxBacklog = result.InboxBacklog
	health.ProcessingDelay = result.ProcessingDelay

	// Calculate error rate based on success
	if !result.Success && result.Reachable {
		health.ErrorRate = 1.0
	} else if result.Success {
		health.ErrorRate = 0.0
	} else {
		health.ErrorRate = 1.0 // Unreachable = 100% error rate
	}

	return health
}

// aggregateHealthSummaries creates aggregated health summaries for domains
func (c *ServerlessHealthChecker) aggregateHealthSummaries(ctx context.Context, domains []string, windows []string) error {
	if len(domains) == 0 || len(windows) == 0 {
		return fmt.Errorf("domains and windows are required for aggregation")
	}

	c.logger.Info("Aggregating health summaries",
		zap.Int("domain_count", len(domains)),
		zap.Strings("windows", windows))

	// Convert window strings to durations
	windowDurations := make(map[string]time.Duration)
	for _, window := range windows {
		switch window {
		case "1h":
			windowDurations[window] = time.Hour
		case "24h":
			windowDurations[window] = 24 * time.Hour
		case "7d":
			windowDurations[window] = 7 * 24 * time.Hour
		default:
			return fmt.Errorf("unsupported window: %s", window)
		}
	}

	// Process each domain
	for _, domain := range domains {
		for windowStr, windowDuration := range windowDurations {
			summary, err := c.healthRepo.CalculateHealthSummary(ctx, domain, windowDuration)
			if err != nil {
				c.logger.Error("Failed to calculate health summary",
					zap.String("domain", domain),
					zap.String("window", windowStr),
					zap.Error(err))
				continue
			}

			if err := c.healthRepo.SaveHealthSummary(ctx, summary); err != nil {
				c.logger.Error("Failed to save health summary",
					zap.String("domain", domain),
					zap.String("window", windowStr),
					zap.Error(err))
				continue
			}

			c.logger.Debug("Saved health summary",
				zap.String("domain", domain),
				zap.String("window", windowStr),
				zap.Float64("health_score", summary.HealthScore),
				zap.Float64("availability", summary.Availability))
		}
	}

	return nil
}

// cleanupOldData removes old health data beyond retention period
func (c *ServerlessHealthChecker) cleanupOldData(ctx context.Context, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		retentionDays = 7 // Default to 7 days
	}

	retentionDuration := time.Duration(retentionDays) * 24 * time.Hour

	c.logger.Info("Starting health data cleanup",
		zap.Int("retention_days", retentionDays),
		zap.Duration("retention_duration", retentionDuration))

	// DynamoDB TTL should handle most cleanup automatically
	// This is a placeholder for manual cleanup if needed
	count, err := c.healthRepo.CleanupOldHealthData(ctx, retentionDuration)
	if err != nil {
		return 0, fmt.Errorf("cleanup failed: %w", err)
	}

	c.logger.Info("Completed health data cleanup",
		zap.Int("cleaned_records", count))

	return count, nil
}

// GetHealthStatus retrieves the latest health status for a domain
func (c *ServerlessHealthChecker) GetHealthStatus(ctx context.Context, domain string) (*models.InstanceHealth, error) {
	return c.healthRepo.GetLatestHealthCheck(ctx, domain)
}

// GetHealthHistory retrieves health history for a domain
func (c *ServerlessHealthChecker) GetHealthHistory(ctx context.Context, domain string, since time.Time, limit int) ([]*models.InstanceHealth, error) {
	return c.healthRepo.GetHealthHistory(ctx, domain, since, limit)
}

// GetHealthSummary retrieves an aggregated health summary
func (c *ServerlessHealthChecker) GetHealthSummary(ctx context.Context, domain string, window time.Duration) (*models.InstanceHealthSummary, error) {
	return c.healthRepo.GetHealthSummary(ctx, domain, window)
}

// GetUnhealthyInstances returns a list of currently unhealthy instances
func (c *ServerlessHealthChecker) GetUnhealthyInstances(ctx context.Context, threshold float64) ([]string, error) {
	if threshold <= 0 {
		threshold = 50.0 // Default threshold of 50% health score
	}

	return c.healthRepo.GetUnhealthyInstances(ctx, threshold)
}
