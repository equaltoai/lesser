package middleware

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

// RetryStrategy defines different retry strategies
type RetryStrategy string

const (
	RetryStrategyFixed       RetryStrategy = "fixed"       // Fixed delay between retries
	RetryStrategyLinear      RetryStrategy = "linear"      // Linear backoff
	RetryStrategyExponential RetryStrategy = "exponential" // Exponential backoff
	RetryStrategyCustom      RetryStrategy = "custom"      // Custom backoff function
)

// RetryConfig holds configuration for the retry middleware
type RetryConfig struct {
	Metrics                 observability.MetricsCollector                           `json:"-"`
	Logger                  observability.StructuredLogger                           `json:"-"`
	RetryCondition          func(error) bool                                         `json:"-"`
	OnGiveUp                func(attempts int, lastErr error)                        `json:"-"`
	OnRetry                 func(attempt int, err error, delay time.Duration)        `json:"-"`
	CustomBackoff           func(attempt int, lastDelay time.Duration) time.Duration `json:"-"`
	Name                    string                                                   `json:"name"`
	Strategy                RetryStrategy                                            `json:"strategy"`
	NonRetryableErrors      []string                                                 `json:"non_retryable_errors"`
	RetryableErrors         []string                                                 `json:"retryable_errors"`
	RetryableStatusCodes    []int                                                    `json:"retryable_status_codes"`
	NonRetryableStatusCodes []int                                                    `json:"non_retryable_status_codes"`
	MaxAttempts             int                                                      `json:"max_attempts"`
	PerAttemptTimeout       time.Duration                                            `json:"per_attempt_timeout"`
	TotalTimeout            time.Duration                                            `json:"total_timeout"`
	JitterRange             float64                                                  `json:"jitter_range"`
	BackoffMultiplier       float64                                                  `json:"backoff_multiplier"`
	MaxDelay                time.Duration                                            `json:"max_delay"`
	InitialDelay            time.Duration                                            `json:"initial_delay"`
	Jitter                  bool                                                     `json:"jitter"`
	EnableMetrics           bool                                                     `json:"enable_metrics"`
}

// RetryStats provides statistics about retry performance
type RetryStats struct {
	Name              string        `json:"name"`
	TotalRequests     int64         `json:"total_requests"`
	RetriedRequests   int64         `json:"retried_requests"`
	SuccessfulRetries int64         `json:"successful_retries"`
	FailedRetries     int64         `json:"failed_retries"`
	TotalAttempts     int64         `json:"total_attempts"`
	AverageAttempts   float64       `json:"average_attempts"`
	MaxAttempts       int           `json:"max_attempts"`
	AverageDelay      time.Duration `json:"average_delay"`
	TotalDelay        time.Duration `json:"total_delay"`
}

// RetryMiddleware creates a retry middleware
func RetryMiddleware(config RetryConfig) lift.Middleware {
	// Set defaults
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay == 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay == 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.Strategy == "" {
		config.Strategy = RetryStrategyExponential
	}
	if config.BackoffMultiplier == 0 {
		config.BackoffMultiplier = 2.0
	}
	if config.JitterRange == 0 {
		config.JitterRange = 0.1 // 10% jitter
	}
	if config.PerAttemptTimeout == 0 {
		config.PerAttemptTimeout = 30 * time.Second
	}
	if config.TotalTimeout == 0 {
		config.TotalTimeout = 5 * time.Minute
	}
	if config.Name == "" {
		config.Name = defaultName
	}
	if config.RetryCondition == nil {
		config.RetryCondition = defaultRetryCondition
	}

	// Initialize default retryable status codes if not provided
	if len(config.RetryableStatusCodes) == 0 {
		config.RetryableStatusCodes = []int{500, 502, 503, 504, 429}
	}

	// Initialize default non-retryable status codes if not provided
	if len(config.NonRetryableStatusCodes) == 0 {
		config.NonRetryableStatusCodes = []int{400, 401, 403, 404, 422}
	}

	retrier := &retryManager{
		config: config,
		stats: &RetryStats{
			Name:        config.Name,
			MaxAttempts: config.MaxAttempts,
		},
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return retrier.executeWithRetry(ctx, next)
		})
	}
}

// retryManager manages retry logic and statistics
type retryManager struct {
	stats  *RetryStats
	config RetryConfig
	mu     sync.RWMutex
}

// executeWithRetry executes the handler with retry logic
func (rm *retryManager) executeWithRetry(ctx *lift.Context, handler lift.Handler) error {
	execution := newRetryExecution(rm, ctx, handler)
	return execution.execute()
}

// shouldRetry determines if an error should be retried
func (rm *retryManager) shouldRetry(err error, attempt int) bool {
	// Attempt-based retry logic could be added here in the future
	// For example: different rules for first attempt vs subsequent attempts
	_ = attempt
	// Check custom retry condition first
	if !rm.config.RetryCondition(err) {
		return false
	}

	// Check non-retryable errors
	for _, nonRetryableError := range rm.config.NonRetryableErrors {
		if fmt.Sprintf("%T", err) == nonRetryableError {
			return false
		}
	}

	// Check retryable errors (if specified)
	if len(rm.config.RetryableErrors) > 0 {
		for _, retryableError := range rm.config.RetryableErrors {
			if fmt.Sprintf("%T", err) == retryableError {
				return true
			}
		}
		return false // Not in retryable list
	}

	// For HTTP errors, check status codes
	if liftErr, ok := err.(*lift.LiftError); ok {
		// Check non-retryable status codes
		for _, code := range rm.config.NonRetryableStatusCodes {
			if liftErr.StatusCode == code {
				return false
			}
		}

		// Check retryable status codes
		for _, code := range rm.config.RetryableStatusCodes {
			if liftErr.StatusCode == code {
				return true
			}
		}

		// Default: don't retry HTTP errors not in retryable list
		return false
	}

	// Default: retry non-HTTP errors
	return true
}

// calculateDelay calculates the delay for the next retry attempt
func (rm *retryManager) calculateDelay(attempt int, totalDelay time.Duration) time.Duration {
	var delay time.Duration

	switch rm.config.Strategy {
	case RetryStrategyFixed:
		delay = rm.config.InitialDelay

	case RetryStrategyLinear:
		delay = time.Duration(attempt) * rm.config.InitialDelay

	case RetryStrategyExponential:
		delay = time.Duration(float64(rm.config.InitialDelay) * math.Pow(rm.config.BackoffMultiplier, float64(attempt-1)))

	case RetryStrategyCustom:
		if rm.config.CustomBackoff != nil {
			delay = rm.config.CustomBackoff(attempt, totalDelay)
		} else {
			delay = rm.config.InitialDelay
		}

	default:
		delay = rm.config.InitialDelay
	}

	// Apply maximum delay limit
	if delay > rm.config.MaxDelay {
		delay = rm.config.MaxDelay
	}

	// Apply jitter if enabled
	if rm.config.Jitter {
		jitterAmount := float64(delay) * rm.config.JitterRange
		jitter := (rand.Float64() - 0.5) * 2 * jitterAmount // #nosec G404 - non-cryptographic use for retry jitter
		delay = time.Duration(float64(delay) + jitter)

		// Ensure delay is not negative
		if delay < 0 {
			delay = time.Millisecond
		}
	}

	return delay
}

// recordAttempt records metrics for an individual attempt
func (rm *retryManager) recordAttempt(attempt int, err error, duration time.Duration) {
	if !rm.config.EnableMetrics || rm.config.Metrics == nil {
		return
	}

	tags := map[string]string{
		"retry_name": rm.config.Name,
		"attempt":    fmt.Sprintf("%d", attempt),
		"result":     map[bool]string{true: "success", false: "failure"}[err == nil],
	}

	metrics := rm.config.Metrics.WithTags(tags)

	// Record attempt
	counter := metrics.Counter("retry.attempts.total")
	counter.Inc()

	// Record attempt duration
	histogram := metrics.Histogram("retry.attempt.duration")
	histogram.Observe(float64(duration.Milliseconds()))
}

// recordSuccess records metrics for a successful request
func (rm *retryManager) recordSuccess(attempts int, totalDuration, totalDelay time.Duration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.stats.TotalRequests++
	rm.stats.TotalAttempts += int64(attempts)
	rm.stats.TotalDelay += totalDelay

	if attempts > 1 {
		rm.stats.RetriedRequests++
		rm.stats.SuccessfulRetries++
	}

	rm.stats.AverageAttempts = float64(rm.stats.TotalAttempts) / float64(rm.stats.TotalRequests)
	if rm.stats.TotalRequests > 0 {
		rm.stats.AverageDelay = time.Duration(int64(rm.stats.TotalDelay) / rm.stats.TotalRequests)
	}

	if !rm.config.EnableMetrics || rm.config.Metrics == nil {
		return
	}

	tags := map[string]string{
		"retry_name": rm.config.Name,
		"result":     "success",
	}

	metrics := rm.config.Metrics.WithTags(tags)

	// Record success
	counter := metrics.Counter("retry.requests.total")
	counter.Inc()

	// Record total duration
	histogram := metrics.Histogram("retry.total.duration")
	histogram.Observe(float64(totalDuration.Milliseconds()))

	// Record total delay
	delayHistogram := metrics.Histogram("retry.total.delay")
	delayHistogram.Observe(float64(totalDelay.Milliseconds()))

	// Record attempt count
	attemptHistogram := metrics.Histogram("retry.attempts.count")
	attemptHistogram.Observe(float64(attempts))
}

// recordFailure records metrics for a failed request
func (rm *retryManager) recordFailure(attempts int, totalDuration, totalDelay time.Duration, err error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.stats.TotalRequests++
	rm.stats.TotalAttempts += int64(attempts)
	rm.stats.TotalDelay += totalDelay

	if attempts > 1 {
		rm.stats.RetriedRequests++
		rm.stats.FailedRetries++
	}

	rm.stats.AverageAttempts = float64(rm.stats.TotalAttempts) / float64(rm.stats.TotalRequests)
	if rm.stats.TotalRequests > 0 {
		rm.stats.AverageDelay = time.Duration(int64(rm.stats.TotalDelay) / rm.stats.TotalRequests)
	}

	if !rm.config.EnableMetrics || rm.config.Metrics == nil {
		return
	}

	tags := map[string]string{
		"retry_name": rm.config.Name,
		"result":     "failure",
		"error_type": fmt.Sprintf("%T", err),
	}

	metrics := rm.config.Metrics.WithTags(tags)

	// Record failure
	counter := metrics.Counter("retry.requests.total")
	counter.Inc()

	// Record total duration
	histogram := metrics.Histogram("retry.total.duration")
	histogram.Observe(float64(totalDuration.Milliseconds()))

	// Record total delay
	delayHistogram := metrics.Histogram("retry.total.delay")
	delayHistogram.Observe(float64(totalDelay.Milliseconds()))

	// Record attempt count
	attemptHistogram := metrics.Histogram("retry.attempts.count")
	attemptHistogram.Observe(float64(attempts))
}

// GetStats returns current retry statistics
func (rm *retryManager) GetStats() RetryStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return *rm.stats
}

// retryExecution manages a single retry execution
type retryExecution struct {
	startTime  time.Time
	handler    lift.Handler
	lastErr    error
	manager    *retryManager
	ctx        *lift.Context
	totalDelay time.Duration
}

// newRetryExecution creates a new retry execution
func newRetryExecution(manager *retryManager, ctx *lift.Context, handler lift.Handler) *retryExecution {
	return &retryExecution{
		manager:   manager,
		ctx:       ctx,
		handler:   handler,
		startTime: time.Now(),
	}
}

// execute runs the retry execution
func (re *retryExecution) execute() error {
	totalCtx := re.createTotalTimeoutContext()

	for attempt := 1; attempt <= re.manager.config.MaxAttempts; attempt++ {
		result := re.executeAttempt(totalCtx, attempt)

		if result.shouldReturn() {
			return result.err
		}

		// Prepare for next attempt
		if attempt < re.manager.config.MaxAttempts {
			if err := re.waitForNextAttempt(totalCtx, attempt); err != nil {
				return err
			}
		}
	}

	return re.lastErr
}

// createTotalTimeoutContext creates a context with total timeout if configured
func (re *retryExecution) createTotalTimeoutContext() context.Context {
	if re.manager.config.TotalTimeout <= 0 {
		return re.ctx.Context
	}

	ctx, cancel := context.WithTimeout(re.ctx.Context, re.manager.config.TotalTimeout)
	// Store cancel func in a goroutine-safe way would be needed in production
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}

// executeAttempt executes a single retry attempt
func (re *retryExecution) executeAttempt(totalCtx context.Context, attempt int) *attemptResult {
	// Create attempt context
	attemptCtx := re.createAttemptContext(totalCtx)

	// Execute handler
	executor := newAttemptExecutor(attemptCtx, re.ctx, re.handler)
	duration, err := executor.execute()

	// Record metrics
	re.recordAttemptMetrics(attempt, err, duration)

	// Handle result
	resultHandler := newAttemptResultHandler(re.manager, re)
	return resultHandler.handleResult(attempt, err, duration)
}

// createAttemptContext creates a context for a single attempt
func (re *retryExecution) createAttemptContext(totalCtx context.Context) context.Context {
	if re.manager.config.PerAttemptTimeout <= 0 {
		return totalCtx
	}

	ctx, cancel := context.WithTimeout(totalCtx, re.manager.config.PerAttemptTimeout)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}

// recordAttemptMetrics records metrics for an attempt
func (re *retryExecution) recordAttemptMetrics(attempt int, err error, duration time.Duration) {
	if re.manager.config.EnableMetrics && re.manager.config.Metrics != nil {
		re.manager.recordAttempt(attempt, err, duration)
	}
}

// waitForNextAttempt waits before the next retry attempt
func (re *retryExecution) waitForNextAttempt(totalCtx context.Context, attempt int) error {
	delay := re.manager.calculateDelay(attempt, re.totalDelay)
	re.totalDelay += delay

	// Check if delay would exceed total timeout
	if re.manager.config.TotalTimeout > 0 && time.Since(re.startTime)+delay > re.manager.config.TotalTimeout {
		return re.handleTimeoutExceeded(attempt)
	}

	// Log retry
	re.logRetry(attempt, delay)

	// Call retry callback
	if re.manager.config.OnRetry != nil {
		re.manager.config.OnRetry(attempt, re.lastErr, delay)
	}

	// Wait for delay
	select {
	case <-time.After(delay):
		return nil
	case <-totalCtx.Done():
		return re.handleContextCanceled(attempt, totalCtx.Err())
	}
}

// handleTimeoutExceeded handles when total timeout would be exceeded
func (re *retryExecution) handleTimeoutExceeded(attempt int) error {
	totalDuration := time.Since(re.startTime)
	re.manager.recordFailure(attempt, totalDuration, re.totalDelay, re.lastErr)

	if re.manager.config.Logger != nil {
		re.manager.config.Logger.Error("Request failed due to total timeout", map[string]any{
			"retry_name":     re.manager.config.Name,
			"attempt":        attempt,
			"total_timeout":  re.manager.config.TotalTimeout.String(),
			"total_duration": totalDuration.String(),
		})
	}

	if re.manager.config.OnGiveUp != nil {
		re.manager.config.OnGiveUp(attempt, re.lastErr)
	}

	return re.lastErr
}

// handleContextCanceled handles context cancellation during delay
func (re *retryExecution) handleContextCanceled(attempt int, err error) error {
	totalDuration := time.Since(re.startTime)
	re.manager.recordFailure(attempt, totalDuration, re.totalDelay, err)
	return err
}

// logRetry logs a retry attempt
func (re *retryExecution) logRetry(attempt int, delay time.Duration) {
	if re.manager.config.Logger != nil {
		re.manager.config.Logger.Warn("Request failed, retrying", map[string]any{
			"retry_name":   re.manager.config.Name,
			"attempt":      attempt,
			"next_attempt": attempt + 1,
			"delay":        delay.String(),
			"error":        "[SANITIZED_ERROR]",
		})
	}
}

// attemptExecutor executes a single attempt
type attemptExecutor struct {
	ctx        *lift.Context
	handler    lift.Handler
	attemptCtx context.Context
}

// newAttemptExecutor creates a new attempt executor
func newAttemptExecutor(attemptCtx context.Context, ctx *lift.Context, handler lift.Handler) *attemptExecutor {
	return &attemptExecutor{
		ctx:        ctx,
		handler:    handler,
		attemptCtx: attemptCtx,
	}
}

// execute runs the attempt and returns error and duration
func (ae *attemptExecutor) execute() (time.Duration, error) {
	// Save original context
	originalCtx := ae.ctx.Context
	ae.ctx.Context = ae.attemptCtx

	// Execute handler
	start := time.Now()
	err := ae.handler.Handle(ae.ctx)
	duration := time.Since(start)

	// Restore original context
	ae.ctx.Context = originalCtx

	return duration, err
}

// attemptResult represents the result of an attempt
type attemptResult struct {
	err          error
	shouldRetry  bool
	finalFailure bool
}

// shouldReturn determines if the execution should return with this result
func (ar *attemptResult) shouldReturn() bool {
	return ar.err == nil || ar.finalFailure
}

// attemptResultHandler handles attempt results
type attemptResultHandler struct {
	manager   *retryManager
	execution *retryExecution
}

// newAttemptResultHandler creates a new result handler
func newAttemptResultHandler(manager *retryManager, execution *retryExecution) *attemptResultHandler {
	return &attemptResultHandler{
		manager:   manager,
		execution: execution,
	}
}

// handleResult processes the result of an attempt
func (arh *attemptResultHandler) handleResult(attempt int, err error, _ time.Duration) *attemptResult {
	totalDuration := time.Since(arh.execution.startTime)

	if err == nil {
		// Success
		arh.handleSuccess(attempt, totalDuration)
		return &attemptResult{err: nil, shouldRetry: false, finalFailure: false}
	}

	// Error occurred
	arh.execution.lastErr = err

	// Check if we should retry
	if !arh.manager.shouldRetry(err, attempt) {
		arh.handleNonRetryableError(attempt, totalDuration, err)
		return &attemptResult{err: err, shouldRetry: false, finalFailure: true}
	}

	// Check if max attempts reached
	if attempt >= arh.manager.config.MaxAttempts {
		arh.handleMaxAttemptsReached(attempt, totalDuration, err)
		return &attemptResult{err: err, shouldRetry: false, finalFailure: true}
	}

	// Will retry
	return &attemptResult{err: err, shouldRetry: true, finalFailure: false}
}

// handleSuccess handles a successful attempt
func (arh *attemptResultHandler) handleSuccess(attempt int, totalDuration time.Duration) {
	arh.manager.recordSuccess(attempt, totalDuration, arh.execution.totalDelay)

	if arh.manager.config.Logger != nil {
		arh.manager.config.Logger.Info("Request succeeded", map[string]any{
			"retry_name":     arh.manager.config.Name,
			"attempt":        attempt,
			"total_duration": totalDuration.String(),
			"total_delay":    arh.execution.totalDelay.String(),
		})
	}
}

// handleNonRetryableError handles errors that should not be retried
func (arh *attemptResultHandler) handleNonRetryableError(attempt int, totalDuration time.Duration, err error) {
	arh.manager.recordFailure(attempt, totalDuration, arh.execution.totalDelay, err)

	if arh.manager.config.Logger != nil {
		arh.manager.config.Logger.Error("Request failed (not retryable)", map[string]any{
			"retry_name":     arh.manager.config.Name,
			"attempt":        attempt,
			"error":          "[SANITIZED_ERROR]",
			"total_duration": totalDuration.String(),
		})
	}

	if arh.manager.config.OnGiveUp != nil {
		arh.manager.config.OnGiveUp(attempt, err)
	}
}

// handleMaxAttemptsReached handles when max attempts have been reached
func (arh *attemptResultHandler) handleMaxAttemptsReached(attempt int, totalDuration time.Duration, err error) {
	arh.manager.recordFailure(attempt, totalDuration, arh.execution.totalDelay, err)

	if arh.manager.config.Logger != nil {
		arh.manager.config.Logger.Error("Request failed after max attempts", map[string]any{
			"retry_name":     arh.manager.config.Name,
			"max_attempts":   arh.manager.config.MaxAttempts,
			"error":          "[SANITIZED_ERROR]",
			"total_duration": totalDuration.String(),
			"total_delay":    arh.execution.totalDelay.String(),
		})
	}

	if arh.manager.config.OnGiveUp != nil {
		arh.manager.config.OnGiveUp(attempt, err)
	}
}

// Default implementations

// defaultRetryCondition determines if an error should be retried by default
func defaultRetryCondition(err error) bool {
	// Don't retry context cancellation or timeout errors
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Don't retry validation errors
	if liftErr, ok := err.(*lift.LiftError); ok && liftErr.Code == "VALIDATION_ERROR" {
		return false
	}

	// Retry other errors by default
	return true
}

// Utility functions for common retry configurations

// NewBasicRetry creates a basic retry configuration with exponential backoff
func NewBasicRetry(name string, maxAttempts int) RetryConfig {
	return RetryConfig{
		Name:              name,
		MaxAttempts:       maxAttempts,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          30 * time.Second,
		Strategy:          RetryStrategyExponential,
		BackoffMultiplier: 2.0,
		Jitter:            true,
		JitterRange:       0.1,
		PerAttemptTimeout: 30 * time.Second,
		TotalTimeout:      5 * time.Minute,
		EnableMetrics:     true,
	}
}

// NewHTTPRetry creates a retry configuration optimized for HTTP requests
func NewHTTPRetry(name string, maxAttempts int) RetryConfig {
	config := NewBasicRetry(name, maxAttempts)
	config.RetryableStatusCodes = []int{500, 502, 503, 504, 429}
	config.NonRetryableStatusCodes = []int{400, 401, 403, 404, 422}
	return config
}

// NewDatabaseRetry creates a retry configuration optimized for database operations
func NewDatabaseRetry(name string, maxAttempts int) RetryConfig {
	config := NewBasicRetry(name, maxAttempts)
	config.InitialDelay = 50 * time.Millisecond
	config.MaxDelay = 5 * time.Second
	config.BackoffMultiplier = 1.5
	return config
}

// NewCustomRetry creates a retry configuration with custom backoff
func NewCustomRetry(name string, maxAttempts int, backoffFunc func(int, time.Duration) time.Duration) RetryConfig {
	config := NewBasicRetry(name, maxAttempts)
	config.Strategy = RetryStrategyCustom
	config.CustomBackoff = backoffFunc
	return config
}
