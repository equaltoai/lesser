package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

// BulkheadConfig holds configuration for the bulkhead pattern
type BulkheadConfig struct {
	Logger                   observability.StructuredLogger    `json:"-"`
	Metrics                  observability.MetricsCollector    `json:"-"`
	PerTenantLimits          map[string]int                    `json:"per_tenant_limits"`
	PerOperationLimits       map[string]int                    `json:"per_operation_limits"`
	PriorityExtractor        func(*lift.Context) int           `json:"-"`
	RejectionHandler         func(*lift.Context, string) error `json:"-"`
	Name                     string                            `json:"name"`
	MaxWaitTime              time.Duration                     `json:"max_wait_time"`
	DefaultTenantLimit       int                               `json:"default_tenant_limit"`
	MaxConcurrentRequests    int                               `json:"max_concurrent_requests"`
	DefaultOperationLimit    int                               `json:"default_operation_limit"`
	HighPriorityThreshold    int                               `json:"high_priority_threshold"`
	EnableTenantIsolation    bool                              `json:"enable_tenant_isolation"`
	EnablePriority           bool                              `json:"enable_priority"`
	EnableMetrics            bool                              `json:"enable_metrics"`
	EnableOperationIsolation bool                              `json:"enable_operation_isolation"`
}

// BulkheadStats provides statistics about bulkhead performance
type BulkheadStats struct {
	TenantStats         map[string]*ResourceStats `json:"tenant_stats,omitempty"`
	OperationStats      map[string]*ResourceStats `json:"operation_stats,omitempty"`
	Name                string                    `json:"name"`
	ActiveRequests      int                       `json:"active_requests"`
	QueuedRequests      int                       `json:"queued_requests"`
	TotalRequests       int64                     `json:"total_requests"`
	RejectedRequests    int64                     `json:"rejected_requests"`
	CompletedRequests   int64                     `json:"completed_requests"`
	AverageWaitTime     time.Duration             `json:"average_wait_time"`
	MaxWaitTime         time.Duration             `json:"max_wait_time"`
	ResourceUtilization float64                   `json:"resource_utilization"`
}

// ResourceStats provides statistics for a specific resource pool
type ResourceStats struct {
	ActiveRequests   int     `json:"active_requests"`
	QueuedRequests   int     `json:"queued_requests"`
	TotalRequests    int64   `json:"total_requests"`
	RejectedRequests int64   `json:"rejected_requests"`
	Utilization      float64 `json:"utilization"`
	Limit            int     `json:"limit"`
}

// BulkheadMiddleware creates a bulkhead pattern middleware
func BulkheadMiddleware(config BulkheadConfig) lift.Middleware {
	// Apply default configuration
	config = applyBulkheadDefaults(config)

	manager := newBulkheadManager(config)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return manager.handleRequest(ctx, next)
		})
	}
}

// applyBulkheadDefaults applies default values to the configuration
func applyBulkheadDefaults(config BulkheadConfig) BulkheadConfig {
	if config.MaxConcurrentRequests == 0 {
		config.MaxConcurrentRequests = 100
	}
	if config.MaxWaitTime == 0 {
		config.MaxWaitTime = 30 * time.Second
	}
	if config.DefaultTenantLimit == 0 {
		config.DefaultTenantLimit = 10
	}
	if config.DefaultOperationLimit == 0 {
		config.DefaultOperationLimit = 20
	}
	if config.Name == "" {
		config.Name = "default"
	}
	if config.RejectionHandler == nil {
		config.RejectionHandler = defaultRejectionHandler
	}
	if config.PriorityExtractor == nil {
		config.PriorityExtractor = defaultPriorityExtractor
	}
	return config
}

// newBulkheadManager creates a new bulkhead manager
func newBulkheadManager(config BulkheadConfig) *bulkheadManager {
	return &bulkheadManager{
		config:              config,
		globalSemaphore:     newSemaphore(config.MaxConcurrentRequests),
		tenantSemaphores:    make(map[string]*semaphore),
		operationSemaphores: make(map[string]*semaphore),
		stats: &BulkheadStats{
			Name:           config.Name,
			TenantStats:    make(map[string]*ResourceStats),
			OperationStats: make(map[string]*ResourceStats),
		},
	}
}

// handleRequest processes a request through the bulkhead
func (bm *bulkheadManager) handleRequest(ctx *lift.Context, next lift.Handler) error {
	handler := newBulkheadRequestHandler(bm, ctx)
	return handler.handle(next)
}

// bulkheadRequestHandler handles a single request through the bulkhead
type bulkheadRequestHandler struct { //nolint:govet // fieldalignment: accepted tradeoff to keep readable grouping
	tenantID  string
	operation string
	manager   *bulkheadManager
	ctx       *lift.Context
	start     time.Time
	priority  int
}

// newBulkheadRequestHandler creates a new request handler
func newBulkheadRequestHandler(manager *bulkheadManager, ctx *lift.Context) *bulkheadRequestHandler {
	return &bulkheadRequestHandler{
		manager:   manager,
		ctx:       ctx,
		tenantID:  ctx.TenantID(),
		operation: fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path),
		priority:  manager.config.PriorityExtractor(ctx),
		start:     time.Now(),
	}
}

// handle processes the request
func (h *bulkheadRequestHandler) handle(next lift.Handler) error {
	// Acquire resources
	acquired, waitTime, err := h.manager.acquireResources(h.ctx.Context, h.tenantID, h.operation, h.priority)
	if err != nil {
		return h.handleRejection(err, waitTime)
	}

	// Ensure resources are released
	defer h.releaseResources(acquired, waitTime)

	// Record successful acquisition
	h.recordAcquisition(waitTime)

	// Execute the handler
	return next.Handle(h.ctx)
}

// handleRejection handles resource acquisition failure
func (h *bulkheadRequestHandler) handleRejection(err error, waitTime time.Duration) error {
	// Log the rejection
	h.logRejection(waitTime)

	// Record rejection metrics
	h.recordRejection(waitTime)

	// Execute rejection handler
	return h.manager.config.RejectionHandler(h.ctx, err.Error())
}

// logRejection logs when resources cannot be acquired
func (h *bulkheadRequestHandler) logRejection(waitTime time.Duration) {
	if h.manager.config.Logger != nil {
		h.manager.config.Logger.Warn("Bulkhead resource acquisition failed", map[string]any{
			"bulkhead_name": h.manager.config.Name,
			"tenant_id":     h.tenantID,
			"operation":     h.operation,
			"priority":      h.priority,
			"wait_time":     waitTime.String(),
			"error":         "[REDACTED_ERROR_DETAIL]", // Sanitized for security
		})
	}
}

// recordRejection records rejection metrics
func (h *bulkheadRequestHandler) recordRejection(waitTime time.Duration) {
	if h.manager.config.EnableMetrics && h.manager.config.Metrics != nil {
		h.manager.recordRejection(h.tenantID, h.operation, waitTime)
	}
}

// releaseResources ensures resources are properly released
func (h *bulkheadRequestHandler) releaseResources(acquired *acquiredResources, waitTime time.Duration) {
	h.manager.releaseResources(acquired, h.tenantID, h.operation)

	duration := time.Since(h.start)

	// Record completion metrics
	h.recordCompletion(duration, waitTime)

	// Log completion
	h.logCompletion(duration, waitTime)
}

// recordCompletion records completion metrics
func (h *bulkheadRequestHandler) recordCompletion(duration, waitTime time.Duration) {
	if h.manager.config.EnableMetrics && h.manager.config.Metrics != nil {
		h.manager.recordCompletion(h.tenantID, h.operation, duration, waitTime)
	}
}

// logCompletion logs request completion
func (h *bulkheadRequestHandler) logCompletion(duration, waitTime time.Duration) {
	if h.manager.config.Logger != nil {
		h.manager.config.Logger.Debug("Bulkhead request completed", map[string]any{
			"bulkhead_name": h.manager.config.Name,
			"tenant_id":     h.tenantID,
			"operation":     h.operation,
			"duration":      duration.String(),
			"wait_time":     waitTime.String(),
		})
	}
}

// recordAcquisition records successful resource acquisition
func (h *bulkheadRequestHandler) recordAcquisition(waitTime time.Duration) {
	if h.manager.config.EnableMetrics && h.manager.config.Metrics != nil {
		h.manager.recordAcquisition(h.tenantID, h.operation, waitTime)
	}
}

// bulkheadManager manages resource allocation and isolation
type bulkheadManager struct {
	globalSemaphore     *semaphore
	tenantSemaphores    map[string]*semaphore
	operationSemaphores map[string]*semaphore
	stats               *BulkheadStats
	config              BulkheadConfig
	mutex               sync.RWMutex
	statsMutex          sync.RWMutex
}

// acquiredResources tracks which resources were acquired for a request
type acquiredResources struct {
	global    bool
	tenant    bool
	operation bool
}

// acquireResources attempts to acquire all necessary resources
func (bm *bulkheadManager) acquireResources(ctx context.Context, tenantID, operation string, priority int) (*acquiredResources, time.Duration, error) {
	start := time.Now()
	acquired := &acquiredResources{}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, bm.config.MaxWaitTime)
	defer cancel()

	// Acquire global semaphore
	if !bm.globalSemaphore.tryAcquire(timeoutCtx, priority) {
		return nil, time.Since(start), fmt.Errorf("global resource limit exceeded")
	}
	acquired.global = true

	// Acquire tenant semaphore if enabled
	if bm.config.EnableTenantIsolation && tenantID != "" {
		tenantSem := bm.getTenantSemaphore(tenantID)
		if !tenantSem.tryAcquire(timeoutCtx, priority) {
			bm.globalSemaphore.release()
			return nil, time.Since(start), fmt.Errorf("tenant resource limit exceeded")
		}
		acquired.tenant = true
	}

	// Acquire operation semaphore if enabled
	if bm.config.EnableOperationIsolation {
		opSem := bm.getOperationSemaphore(operation)
		if !opSem.tryAcquire(timeoutCtx, priority) {
			if acquired.tenant {
				bm.getTenantSemaphore(tenantID).release()
			}
			bm.globalSemaphore.release()
			return nil, time.Since(start), fmt.Errorf("operation resource limit exceeded")
		}
		acquired.operation = true
	}

	return acquired, time.Since(start), nil
}

// releaseResources releases all acquired resources
func (bm *bulkheadManager) releaseResources(acquired *acquiredResources, tenantID, operation string) {
	if acquired.operation {
		bm.getOperationSemaphore(operation).release()
	}
	if acquired.tenant {
		bm.getTenantSemaphore(tenantID).release()
	}
	if acquired.global {
		bm.globalSemaphore.release()
	}
}

// getSemaphore is a generic helper to get or create a semaphore
func (bm *bulkheadManager) getSemaphore(
	key string,
	semaphoreMap map[string]*semaphore,
	defaultLimit int,
	specificLimits map[string]int,
	statsMap map[string]*ResourceStats,
	statsMutex *sync.RWMutex,
) *semaphore {
	bm.mutex.RLock()
	sem, exists := semaphoreMap[key]
	bm.mutex.RUnlock()

	if exists {
		return sem
	}

	bm.mutex.Lock()
	defer bm.mutex.Unlock()

	// Double-check after acquiring write lock
	if existingSem, exists := semaphoreMap[key]; exists {
		return existingSem
	}

	// Determine limit for this resource
	limit := defaultLimit
	if specificLimit, exists := specificLimits[key]; exists {
		limit = specificLimit
	}

	sem = newSemaphore(limit)
	semaphoreMap[key] = sem

	// Initialize stats
	statsMutex.Lock()
	statsMap[key] = &ResourceStats{
		Limit: limit,
	}
	statsMutex.Unlock()

	return sem
}

// getTenantSemaphore gets or creates a semaphore for a tenant
func (bm *bulkheadManager) getTenantSemaphore(tenantID string) *semaphore {
	return bm.getSemaphore(
		tenantID,
		bm.tenantSemaphores,
		bm.config.DefaultTenantLimit,
		bm.config.PerTenantLimits,
		bm.stats.TenantStats,
		&bm.statsMutex,
	)
}

// getOperationSemaphore gets or creates a semaphore for an operation
func (bm *bulkheadManager) getOperationSemaphore(operation string) *semaphore {
	return bm.getSemaphore(
		operation,
		bm.operationSemaphores,
		bm.config.DefaultOperationLimit,
		bm.config.PerOperationLimits,
		bm.stats.OperationStats,
		&bm.statsMutex,
	)
}

// buildMetricTags creates common metric tags
func (bm *bulkheadManager) buildMetricTags(tenantID, operation, result string) map[string]string {
	tags := map[string]string{
		"bulkhead_name": bm.config.Name,
	}

	if result != "" {
		tags["result"] = result
	}

	if bm.config.EnableTenantIsolation && tenantID != "" {
		tags["tenant_id"] = tenantID
	}

	if bm.config.EnableOperationIsolation && operation != "" {
		tags["operation"] = operation
	}

	return tags
}

// recordAcquisition records successful resource acquisition
func (bm *bulkheadManager) recordAcquisition(tenantID, operation string, waitTime time.Duration) {
	if !bm.config.EnableMetrics || bm.config.Metrics == nil {
		return
	}

	tags := bm.buildMetricTags(tenantID, operation, "acquired")
	metrics := bm.config.Metrics.WithTags(tags)

	// Record acquisition
	counter := metrics.Counter("bulkhead.acquisitions.total")
	counter.Inc()

	// Record wait time
	histogram := metrics.Histogram("bulkhead.wait_time")
	histogram.Observe(float64(waitTime.Milliseconds()))
}

// recordRejection records resource acquisition rejection
func (bm *bulkheadManager) recordRejection(tenantID, operation string, waitTime time.Duration) {
	if !bm.config.EnableMetrics || bm.config.Metrics == nil {
		return
	}

	tags := bm.buildMetricTags(tenantID, operation, "rejected")
	metrics := bm.config.Metrics.WithTags(tags)

	// Record rejection
	counter := metrics.Counter("bulkhead.rejections.total")
	counter.Inc()

	// Record wait time before rejection
	histogram := metrics.Histogram("bulkhead.rejection_wait_time")
	histogram.Observe(float64(waitTime.Milliseconds()))
}

// recordCompletion records request completion
func (bm *bulkheadManager) recordCompletion(tenantID, operation string, duration, waitTime time.Duration) {
	if !bm.config.EnableMetrics || bm.config.Metrics == nil {
		return
	}

	tags := map[string]string{
		"bulkhead_name": bm.config.Name,
	}

	if bm.config.EnableTenantIsolation && tenantID != "" {
		tags["tenant_id"] = tenantID
	}

	if bm.config.EnableOperationIsolation {
		tags["operation"] = operation
	}

	metrics := bm.config.Metrics.WithTags(tags)

	// Record completion
	counter := metrics.Counter("bulkhead.completions.total")
	counter.Inc()

	// Record execution duration
	histogram := metrics.Histogram("bulkhead.execution_duration")
	histogram.Observe(float64(duration.Milliseconds()))

	// Record wait time for completed requests
	waitTimeHist := metrics.Histogram("bulkhead.completion_wait_time")
	waitTimeHist.Observe(float64(waitTime.Milliseconds()))

	// Record resource utilization
	utilization := float64(bm.globalSemaphore.active()) / float64(bm.globalSemaphore.capacity())
	gauge := metrics.Gauge("bulkhead.utilization")
	gauge.Set(utilization)
}

// GetStats returns current bulkhead statistics
func (bm *bulkheadManager) GetStats() BulkheadStats {
	bm.statsMutex.RLock()
	defer bm.statsMutex.RUnlock()

	stats := *bm.stats
	stats.ActiveRequests = bm.globalSemaphore.active()
	stats.ResourceUtilization = float64(stats.ActiveRequests) / float64(bm.config.MaxConcurrentRequests)

	return stats
}

// semaphore implements a priority-aware semaphore
type semaphore struct {
	waitQueue   []*waiter
	maxCapacity int
	activeCount int
	mutex       sync.Mutex
}

// waiter represents a waiting request
type waiter struct {
	ctx      context.Context
	ch       chan bool
	priority int
}

// newSemaphore creates a new semaphore with the given capacity
func newSemaphore(capacity int) *semaphore {
	return &semaphore{
		maxCapacity: capacity,
		waitQueue:   make([]*waiter, 0),
	}
}

// tryAcquire attempts to acquire the semaphore with priority support
func (s *semaphore) tryAcquire(ctx context.Context, priority int) bool {
	s.mutex.Lock()

	// Try immediate acquisition
	if s.activeCount < s.maxCapacity {
		s.activeCount++
		s.mutex.Unlock()
		return true
	}

	// Need to wait - create waiter
	waiter := &waiter{
		priority: priority,
		ch:       make(chan bool, 1),
		ctx:      ctx,
	}

	// Insert waiter in priority order
	s.insertWaiter(waiter)
	s.mutex.Unlock()

	// Wait for acquisition or timeout
	select {
	case acquired := <-waiter.ch:
		return acquired
	case <-ctx.Done():
		// Remove from queue on timeout
		s.removeWaiter(waiter)
		return false
	}
}

// release releases the semaphore and notifies waiting requests
func (s *semaphore) release() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.activeCount > 0 {
		s.activeCount--
	}

	// Notify next waiter
	if len(s.waitQueue) > 0 {
		waiter := s.waitQueue[0]
		s.waitQueue = s.waitQueue[1:]
		s.activeCount++

		select {
		case waiter.ch <- true:
		default:
			// Waiter already timed out, try next
			if s.activeCount > 0 {
				s.activeCount--
			}
			if len(s.waitQueue) > 0 {
				s.release() // Recursive call to try next waiter
			}
		}
	}
}

// insertWaiter inserts a waiter in priority order (higher priority first)
func (s *semaphore) insertWaiter(w *waiter) {
	for i, existing := range s.waitQueue {
		if w.priority > existing.priority {
			// Insert at position i
			s.waitQueue = append(s.waitQueue[:i], append([]*waiter{w}, s.waitQueue[i:]...)...)
			return
		}
	}
	// Append at end
	s.waitQueue = append(s.waitQueue, w)
}

// removeWaiter removes a waiter from the queue
func (s *semaphore) removeWaiter(target *waiter) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for i, w := range s.waitQueue {
		if w == target {
			s.waitQueue = append(s.waitQueue[:i], s.waitQueue[i+1:]...)
			break
		}
	}
}

// active returns the number of active acquisitions
func (s *semaphore) active() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.activeCount
}

// capacity returns the semaphore capacity
func (s *semaphore) capacity() int {
	return s.maxCapacity
}

// Default implementations

// defaultRejectionHandler provides a default rejection response
func defaultRejectionHandler(ctx *lift.Context, reason string) error {
	if err := ctx.Status(503).JSON(map[string]any{
		"error":   "Service temporarily unavailable",
		"message": "Resource limit exceeded",
		"reason":  reason,
		"code":    "BULKHEAD_LIMIT_EXCEEDED",
	}); err != nil {
		return fmt.Errorf("failed to send bulkhead rejection response: %w", err)
	}
	// Return an error to indicate rejection
	return fmt.Errorf("bulkhead limit exceeded: %s", reason)
}

// defaultPriorityExtractor extracts priority from context (default: normal priority)
func defaultPriorityExtractor(ctx *lift.Context) int {
	// Check for priority header
	if priority := ctx.Request.Headers["X-Priority"]; priority != "" {
		switch priority {
		case "high":
			return 10
		case "low":
			return 1
		default:
			return 5
		}
	}
	return 5 // Normal priority
}

// Utility functions for common bulkhead configurations

// NewBasicBulkhead creates a basic bulkhead with sensible defaults
func NewBasicBulkhead(name string, maxConcurrent int) BulkheadConfig {
	return BulkheadConfig{
		Name:                  name,
		MaxConcurrentRequests: maxConcurrent,
		MaxWaitTime:           30 * time.Second,
		DefaultTenantLimit:    maxConcurrent / 10,
		DefaultOperationLimit: maxConcurrent / 5,
		EnableMetrics:         true,
	}
}

// NewTenantBulkhead creates a tenant-isolated bulkhead
func NewTenantBulkhead(name string, maxConcurrent int, tenantLimits map[string]int) BulkheadConfig {
	config := NewBasicBulkhead(name, maxConcurrent)
	config.EnableTenantIsolation = true
	config.PerTenantLimits = tenantLimits
	return config
}

// NewOperationBulkhead creates an operation-isolated bulkhead
func NewOperationBulkhead(name string, maxConcurrent int, operationLimits map[string]int) BulkheadConfig {
	config := NewBasicBulkhead(name, maxConcurrent)
	config.EnableOperationIsolation = true
	config.PerOperationLimits = operationLimits
	return config
}

// NewPriorityBulkhead creates a priority-aware bulkhead
func NewPriorityBulkhead(name string, maxConcurrent int, priorityExtractor func(*lift.Context) int) BulkheadConfig {
	config := NewBasicBulkhead(name, maxConcurrent)
	config.EnablePriority = true
	config.PriorityExtractor = priorityExtractor
	config.HighPriorityThreshold = 8
	return config
}
