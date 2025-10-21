package middleware

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

const (
	defaultName  = "default"
	priorityHigh = "high"
	priorityLow  = "low"
)

// LoadSheddingStrategy defines different load shedding strategies
type LoadSheddingStrategy string

const (
	LoadSheddingRandom   LoadSheddingStrategy = "random"   // Random shedding based on probability
	LoadSheddingPriority LoadSheddingStrategy = "priority" // Priority-based shedding
	LoadSheddingAdaptive LoadSheddingStrategy = "adaptive" // Adaptive shedding based on system metrics
	LoadSheddingCircuit  LoadSheddingStrategy = "circuit"  // Circuit breaker style shedding
	LoadSheddingCustom   LoadSheddingStrategy = "custom"   // Custom shedding algorithm

	// Backward compatibility aliases
	LoadSheddingStrategyRandom   = LoadSheddingRandom
	LoadSheddingStrategyPriority = LoadSheddingPriority
	LoadSheddingStrategyAdaptive = LoadSheddingAdaptive
	LoadSheddingStrategyCircuit  = LoadSheddingCircuit
	LoadSheddingStrategyCustom   = LoadSheddingCustom
)

// LoadSheddingConfig holds configuration for load shedding
type LoadSheddingConfig struct {
	Metrics                  observability.MetricsCollector         `json:"-"`
	Logger                   observability.StructuredLogger         `json:"-"`
	SheddingHandler          func(*lift.Context) error              `json:"-"`
	CustomShedder            func(*lift.Context, *LoadMetrics) bool `json:"-"`
	PriorityExtractor        func(*lift.Context) int                `json:"-"`
	LifecycleContext         context.Context                        `json:"-"`
	RegisterStop             func(func())                           `json:"-"`
	PriorityThresholds       map[int]float64                        `json:"priority_thresholds"`
	Strategy                 LoadSheddingStrategy                   `json:"strategy"`
	Name                     string                                 `json:"name"`
	SheddingMessage          string                                 `json:"shedding_message"`
	TargetLatency            time.Duration                          `json:"target_latency"`
	MetricsWindow            time.Duration                          `json:"metrics_window"`
	MetricsCollectorInterval time.Duration                          `json:"-"`
	LatencyThreshold         time.Duration                          `json:"latency_threshold"`
	AdaptationRate           float64                                `json:"adaptation_rate"`
	ErrorRateThreshold       float64                                `json:"error_rate_threshold"`
	MinSheddingRate          float64                                `json:"min_shedding_rate"`
	MaxSheddingRate          float64                                `json:"max_shedding_rate"`
	SamplingRate             float64                                `json:"sampling_rate"`
	SheddingRate             float64                                `json:"shedding_rate"`
	MemoryThreshold          float64                                `json:"memory_threshold"`
	CPUThreshold             float64                                `json:"cpu_threshold"`
	SheddingStatusCode       int                                    `json:"shedding_status_code"`
	EnableMetrics            bool                                   `json:"enable_metrics"`
	Enabled                  bool                                   `json:"enabled"`
}

// ConfigureLoadSheddingForApp wires lifecycle management into the provided
// LoadSheddingConfig using the application's lifecycle context and shutdown
// hooks when they have not already been supplied.
func ConfigureLoadSheddingForApp(app *lift.App, config LoadSheddingConfig) LoadSheddingConfig {
	if app == nil {
		return config
	}

	if config.LifecycleContext == nil {
		config.LifecycleContext = app.LifecycleContext()
	}

	if config.RegisterStop == nil {
		config.RegisterStop = app.RegisterShutdownHook
	}

	return config
}

// LoadMetrics provides real-time system and application metrics
type LoadMetrics struct {
	LastUpdated         time.Time     `json:"last_updated"`
	WindowStart         time.Time     `json:"window_start"`
	P99Latency          time.Duration `json:"p99_latency"`
	RequestRate         float64       `json:"request_rate"`
	AverageLatency      time.Duration `json:"average_latency"`
	P95Latency          time.Duration `json:"p95_latency"`
	CPUUsage            float64       `json:"cpu_usage"`
	ErrorRate           float64       `json:"error_rate"`
	CurrentSheddingRate float64       `json:"current_shedding_rate"`
	TotalRequests       int64         `json:"total_requests"`
	ShedRequests        int64         `json:"shed_requests"`
	ActiveRequests      int64         `json:"active_requests"`
	MemoryUsage         float64       `json:"memory_usage"`
}

// LoadSheddingStats provides statistics about load shedding performance
type LoadSheddingStats struct {
	Strategy            LoadSheddingStrategy `json:"strategy"`
	Name                string               `json:"name"`
	SystemMetrics       LoadMetrics          `json:"system_metrics"`
	TotalRequests       int64                `json:"total_requests"`
	ShedRequests        int64                `json:"shed_requests"`
	AverageLatency      time.Duration        `json:"average_latency"`
	CurrentSheddingRate float64              `json:"current_shedding_rate"`
	SheddingRatio       float64              `json:"shedding_ratio"`
	Enabled             bool                 `json:"enabled"`
}

// LoadSheddingMiddleware creates a load shedding middleware
func LoadSheddingMiddleware(config LoadSheddingConfig) lift.Middleware {
	// Apply default configuration
	config = applyLoadSheddingDefaults(config)

	manager := newLoadSheddingManager(config)

	manager.startMetricsCollector(config.LifecycleContext)
	if config.RegisterStop != nil {
		config.RegisterStop(manager.Stop)
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return manager.handleRequest(ctx, next)
		})
	}
}

// applyLoadSheddingDefaults applies default values to the configuration
func applyLoadSheddingDefaults(config LoadSheddingConfig) LoadSheddingConfig {
	builder := newLoadSheddingDefaultsBuilder(config)
	return builder.build()
}

// loadSheddingDefaultsBuilder applies defaults to load shedding configuration
type loadSheddingDefaultsBuilder struct {
	config LoadSheddingConfig
}

// newLoadSheddingDefaultsBuilder creates a new defaults builder
func newLoadSheddingDefaultsBuilder(config LoadSheddingConfig) *loadSheddingDefaultsBuilder {
	return &loadSheddingDefaultsBuilder{config: config}
}

// build applies all defaults to the configuration
func (b *loadSheddingDefaultsBuilder) build() LoadSheddingConfig {
	b.applyPerformanceDefaults()
	b.applySheddingDefaults()
	b.applyMetricsDefaults()
	b.applyResponseDefaults()
	b.applyFunctionDefaults()

	return b.config
}

// applyPerformanceDefaults sets performance-related defaults
func (b *loadSheddingDefaultsBuilder) applyPerformanceDefaults() {
	if b.config.CPUThreshold == 0 {
		b.config.CPUThreshold = 0.8 // 80% CPU
	}
	if b.config.MemoryThreshold == 0 {
		b.config.MemoryThreshold = 0.85 // 85% Memory
	}
	if b.config.LatencyThreshold == 0 {
		b.config.LatencyThreshold = 5 * time.Second
	}
	if b.config.ErrorRateThreshold == 0 {
		b.config.ErrorRateThreshold = 0.1 // 10% error rate
	}
	if b.config.TargetLatency == 0 {
		b.config.TargetLatency = 100 * time.Millisecond
	}
}

// applySheddingDefaults sets shedding-related defaults
func (b *loadSheddingDefaultsBuilder) applySheddingDefaults() {
	if b.config.MaxSheddingRate == 0 {
		b.config.MaxSheddingRate = 0.9 // Max 90% shedding
	}
	if b.config.MinSheddingRate == 0 {
		b.config.MinSheddingRate = 0.0 // Min 0% shedding
	}
	if b.config.AdaptationRate == 0 {
		b.config.AdaptationRate = 0.1 // 10% adaptation rate
	}
}

// applyMetricsDefaults sets metrics-related defaults
func (b *loadSheddingDefaultsBuilder) applyMetricsDefaults() {
	if b.config.MetricsWindow == 0 {
		b.config.MetricsWindow = 30 * time.Second
	}
	if b.config.SamplingRate == 0 {
		b.config.SamplingRate = 1.0 // Sample all requests by default
	}
}

// applyResponseDefaults sets response-related defaults
func (b *loadSheddingDefaultsBuilder) applyResponseDefaults() {
	if b.config.SheddingStatusCode == 0 {
		b.config.SheddingStatusCode = 503 // Service Unavailable
	}
	if b.config.SheddingMessage == "" {
		b.config.SheddingMessage = "Service temporarily overloaded"
	}
	if b.config.Name == "" {
		b.config.Name = defaultName
	}
}

// applyFunctionDefaults sets function-related defaults
func (b *loadSheddingDefaultsBuilder) applyFunctionDefaults() {
	if b.config.PriorityExtractor == nil {
		b.config.PriorityExtractor = defaultLoadSheddingPriorityExtractor
	}
	if b.config.SheddingHandler == nil {
		b.config.SheddingHandler = defaultSheddingHandler(b.config.SheddingStatusCode, b.config.SheddingMessage)
	}
}

// newLoadSheddingManager creates a new load shedding manager
func newLoadSheddingManager(config LoadSheddingConfig) *loadSheddingManager {
	cfg := config
	return &loadSheddingManager{
		config:         &cfg,
		metrics:        &LoadMetrics{LastUpdated: time.Now(), WindowStart: time.Now()},
		latencyHistory: &latencyHistory{values: make([]time.Duration, 0, 1000)},
		stats: &LoadSheddingStats{
			Name:     cfg.Name,
			Strategy: cfg.Strategy,
			Enabled:  cfg.Enabled,
		},
		stopCh: make(chan struct{}),
	}
}

func (lsm *loadSheddingManager) startMetricsCollector(ctx context.Context) {
	if !lsm.config.EnableMetrics || lsm.config.Metrics == nil {
		return
	}

	if ctx == nil {
		ctx = context.Background()
	}

	lsm.wg.Add(1)
	go func() {
		defer lsm.wg.Done()
		lsm.metricsCollector(ctx)
	}()
}

func (lsm *loadSheddingManager) metricsCollectorInterval() time.Duration {
	if lsm.config.MetricsCollectorInterval > 0 {
		return lsm.config.MetricsCollectorInterval
	}
	return time.Second
}

// Stop terminates background processing started by the load shedding manager.
func (lsm *loadSheddingManager) Stop() {
	lsm.stopOnce.Do(func() {
		close(lsm.stopCh)
	})
	lsm.wg.Wait()
}

// handleRequest processes a single request with load shedding logic
func (lsm *loadSheddingManager) handleRequest(ctx *lift.Context, next lift.Handler) error {
	if !lsm.config.Enabled {
		return next.Handle(ctx)
	}

	handler := newLoadSheddingHandler(lsm, ctx)
	return handler.handle(next)
}

// loadSheddingHandler handles a single request with load shedding
type loadSheddingHandler struct {
	manager *loadSheddingManager
	ctx     *lift.Context
	start   time.Time
}

// newLoadSheddingHandler creates a new load shedding handler
func newLoadSheddingHandler(manager *loadSheddingManager, ctx *lift.Context) *loadSheddingHandler {
	return &loadSheddingHandler{
		manager: manager,
		ctx:     ctx,
		start:   time.Now(),
	}
}

// handle processes the request with load shedding logic
func (h *loadSheddingHandler) handle(next lift.Handler) error {
	// Update active request count
	h.incrementActiveRequests()
	defer h.decrementActiveRequests()

	// Check if request should be shed
	if h.shouldShedRequest() {
		return h.handleShedding()
	}

	// Execute request and record metrics
	return h.executeAndRecordMetrics(next)
}

// incrementActiveRequests increments the active request counter
func (h *loadSheddingHandler) incrementActiveRequests() {
	atomic.AddInt64(&h.manager.metrics.ActiveRequests, 1)
}

// decrementActiveRequests decrements the active request counter
func (h *loadSheddingHandler) decrementActiveRequests() {
	atomic.AddInt64(&h.manager.metrics.ActiveRequests, -1)
}

// shouldShedRequest determines if the request should be shed
func (h *loadSheddingHandler) shouldShedRequest() bool {
	return h.manager.shouldShedRequest(h.ctx)
}

// handleShedding handles a shed request
func (h *loadSheddingHandler) handleShedding() error {
	// Record shedding metrics
	h.recordSheddingMetrics()

	// Log shedding event
	h.logSheddingEvent()

	// Record shedding in metrics system
	if h.manager.config.EnableMetrics && h.manager.config.Metrics != nil {
		h.manager.recordShedding(h.ctx)
	}

	return h.manager.config.SheddingHandler(h.ctx)
}

// recordSheddingMetrics updates shedding counters
func (h *loadSheddingHandler) recordSheddingMetrics() {
	atomic.AddInt64(&h.manager.metrics.ShedRequests, 1)
	atomic.AddInt64(&h.manager.metrics.TotalRequests, 1)
}

// logSheddingEvent logs the shedding event
func (h *loadSheddingHandler) logSheddingEvent() {
	if h.manager.config.Logger != nil {
		priority := h.manager.config.PriorityExtractor(h.ctx)
		h.manager.config.Logger.Warn("Request shed due to load", map[string]any{
			"load_shedding_name": h.manager.config.Name,
			"strategy":           string(h.manager.config.Strategy),
			"priority":           priority,
			"shedding_rate":      h.manager.getCurrentSheddingRate(),
			"active_requests":    atomic.LoadInt64(&h.manager.metrics.ActiveRequests),
		})
	}
}

// executeAndRecordMetrics executes the request and records metrics
func (h *loadSheddingHandler) executeAndRecordMetrics(next lift.Handler) error {
	// Execute request
	err := next.Handle(h.ctx)
	duration := time.Since(h.start)

	// Record request metrics
	h.recordRequestMetrics(duration, err)

	return err
}

// recordRequestMetrics records metrics for the completed request
func (h *loadSheddingHandler) recordRequestMetrics(duration time.Duration, err error) {
	atomic.AddInt64(&h.manager.metrics.TotalRequests, 1)
	h.manager.recordLatency(duration)

	if err != nil {
		h.manager.recordError()
	}

	// Record success metrics in metrics system
	if h.manager.config.EnableMetrics && h.manager.config.Metrics != nil {
		h.manager.recordSuccess(h.ctx, duration)
	}
}

// loadSheddingManager manages load shedding logic and metrics
type loadSheddingManager struct {
	config         *LoadSheddingConfig
	latencyHistory *latencyHistory
	metrics        *LoadMetrics
	stats          *LoadSheddingStats
	stopCh         chan struct{}
	wg             sync.WaitGroup
	mutex          sync.RWMutex
	stopOnce       sync.Once
	errorCount     int64
}

type latencyHistory struct {
	values []time.Duration
}

// shouldShedRequest determines if a request should be shed
func (lsm *loadSheddingManager) shouldShedRequest(ctx *lift.Context) bool {
	switch lsm.config.Strategy {
	case LoadSheddingRandom:
		return lsm.randomShedding()
	case LoadSheddingPriority:
		return lsm.priorityShedding(ctx)
	case LoadSheddingAdaptive:
		return lsm.adaptiveShedding()
	case LoadSheddingCircuit:
		return lsm.circuitShedding()
	case LoadSheddingCustom:
		if lsm.config.CustomShedder != nil {
			return lsm.config.CustomShedder(ctx, lsm.metrics)
		}
		return false
	default:
		return false
	}
}

// randomShedding implements random load shedding based on current load
func (lsm *loadSheddingManager) randomShedding() bool {
	sheddingRate := lsm.calculateSheddingRate()
	return rand.Float64() < sheddingRate // #nosec G404 - non-cryptographic use for load shedding
}

// priorityShedding implements priority-based load shedding
func (lsm *loadSheddingManager) priorityShedding(ctx *lift.Context) bool {
	priority := lsm.config.PriorityExtractor(ctx)

	// Get shedding rate for this priority level
	sheddingRate := lsm.calculateSheddingRate()

	// Apply priority-specific adjustments
	if threshold, exists := lsm.config.PriorityThresholds[priority]; exists {
		sheddingRate = math.Min(sheddingRate, threshold)
	}

	// Higher priority requests are less likely to be shed
	priorityMultiplier := 1.0 / (1.0 + float64(priority)*0.1)
	adjustedRate := sheddingRate * priorityMultiplier

	return rand.Float64() < adjustedRate // #nosec G404 - non-cryptographic use for load shedding
}

// adaptiveShedding implements adaptive load shedding based on target latency
func (lsm *loadSheddingManager) adaptiveShedding() bool {
	currentLatency := lsm.metrics.AverageLatency
	targetLatency := lsm.config.TargetLatency

	if currentLatency <= targetLatency {
		// Performance is good, reduce shedding
		currentRate := lsm.getCurrentSheddingRate()
		newRate := math.Max(currentRate-lsm.config.AdaptationRate, lsm.config.MinSheddingRate)
		lsm.setCurrentSheddingRate(newRate)
		return rand.Float64() < newRate // #nosec G404 - non-cryptographic use for load shedding
	}

	// Performance is poor, increase shedding
	latencyRatio := float64(currentLatency) / float64(targetLatency)
	desiredRate := math.Min((latencyRatio-1.0)*0.5, lsm.config.MaxSheddingRate)

	currentRate := lsm.getCurrentSheddingRate()
	newRate := math.Min(currentRate+lsm.config.AdaptationRate, desiredRate)
	lsm.setCurrentSheddingRate(newRate)

	return rand.Float64() < newRate // #nosec G404 - non-cryptographic use for load shedding
}

// circuitShedding implements circuit breaker style load shedding
func (lsm *loadSheddingManager) circuitShedding() bool {
	// Check multiple thresholds
	cpuOverload := lsm.metrics.CPUUsage > lsm.config.CPUThreshold
	memoryOverload := lsm.metrics.MemoryUsage > lsm.config.MemoryThreshold
	latencyOverload := lsm.metrics.AverageLatency > lsm.config.LatencyThreshold
	errorOverload := lsm.metrics.ErrorRate > lsm.config.ErrorRateThreshold

	overloadCount := 0
	if cpuOverload {
		overloadCount++
	}
	if memoryOverload {
		overloadCount++
	}
	if latencyOverload {
		overloadCount++
	}
	if errorOverload {
		overloadCount++
	}

	// Shed based on number of overloaded metrics
	sheddingRate := float64(overloadCount) * 0.25 // 25% per overloaded metric
	sheddingRate = math.Min(sheddingRate, lsm.config.MaxSheddingRate)

	lsm.setCurrentSheddingRate(sheddingRate)
	return rand.Float64() < sheddingRate // #nosec G404 - non-cryptographic use for load shedding
}

// calculateSheddingRate calculates the current shedding rate based on system metrics
func (lsm *loadSheddingManager) calculateSheddingRate() float64 {
	lsm.mutex.RLock()
	defer lsm.mutex.RUnlock()

	// Base shedding rate on multiple factors
	cpuFactor := math.Max(0, (lsm.metrics.CPUUsage-lsm.config.CPUThreshold)/(1.0-lsm.config.CPUThreshold))
	memoryFactor := math.Max(0, (lsm.metrics.MemoryUsage-lsm.config.MemoryThreshold)/(1.0-lsm.config.MemoryThreshold))

	latencyFactor := 0.0
	if lsm.config.LatencyThreshold > 0 {
		latencyFactor = math.Max(0, (float64(lsm.metrics.AverageLatency)-float64(lsm.config.LatencyThreshold))/float64(lsm.config.LatencyThreshold))
	}

	errorFactor := math.Max(0, (lsm.metrics.ErrorRate-lsm.config.ErrorRateThreshold)/(1.0-lsm.config.ErrorRateThreshold))

	// Combine factors (weighted average)
	combinedFactor := (cpuFactor*0.3 + memoryFactor*0.3 + latencyFactor*0.3 + errorFactor*0.1)

	// Apply bounds
	sheddingRate := math.Min(combinedFactor, lsm.config.MaxSheddingRate)
	sheddingRate = math.Max(sheddingRate, lsm.config.MinSheddingRate)

	return sheddingRate
}

// getCurrentSheddingRate returns the current shedding rate
func (lsm *loadSheddingManager) getCurrentSheddingRate() float64 {
	lsm.mutex.RLock()
	defer lsm.mutex.RUnlock()
	return lsm.metrics.CurrentSheddingRate
}

// setCurrentSheddingRate sets the current shedding rate
func (lsm *loadSheddingManager) setCurrentSheddingRate(rate float64) {
	lsm.mutex.Lock()
	defer lsm.mutex.Unlock()
	lsm.metrics.CurrentSheddingRate = rate
}

// recordLatency records request latency
func (lsm *loadSheddingManager) recordLatency(duration time.Duration) {
	lsm.mutex.Lock()
	defer lsm.mutex.Unlock()

	// Add to history
	lsm.latencyHistory.values = append(lsm.latencyHistory.values, duration)

	// Keep only recent history
	if len(lsm.latencyHistory.values) > 1000 {
		lsm.latencyHistory.values = lsm.latencyHistory.values[len(lsm.latencyHistory.values)-1000:]
	}

	// Update metrics
	lsm.updateLatencyMetrics()
}

// recordError records request error
func (lsm *loadSheddingManager) recordError() {
	atomic.AddInt64(&lsm.errorCount, 1)
}

// updateLatencyMetrics calculates latency percentiles
func (lsm *loadSheddingManager) updateLatencyMetrics() {
	if len(lsm.latencyHistory.values) == 0 {
		return
	}

	// Calculate average
	var total time.Duration
	for _, duration := range lsm.latencyHistory.values {
		total += duration
	}
	lsm.metrics.AverageLatency = total / time.Duration(len(lsm.latencyHistory.values))

	// Calculate percentiles (simplified)
	if len(lsm.latencyHistory.values) >= 20 {
		sorted := make([]time.Duration, len(lsm.latencyHistory.values))
		copy(sorted, lsm.latencyHistory.values)

		// Simple sort for percentiles
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[i] > sorted[j] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}

		p95Index := int(float64(len(sorted)) * 0.95)
		p99Index := int(float64(len(sorted)) * 0.99)

		lsm.metrics.P95Latency = sorted[p95Index]
		lsm.metrics.P99Latency = sorted[p99Index]
	}
}

// metricsCollector runs in background to collect system metrics
func (lsm *loadSheddingManager) metricsCollector(ctx context.Context) {
	ticker := time.NewTicker(lsm.metricsCollectorInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lsm.updateMetrics()
		case <-ctx.Done():
			return
		case <-lsm.stopCh:
			return
		}
	}
}

// updateMetrics updates system and application metrics
func (lsm *loadSheddingManager) updateMetrics() {
	lsm.mutex.Lock()
	defer lsm.mutex.Unlock()

	now := time.Now()

	// Update request rate
	windowDuration := now.Sub(lsm.metrics.WindowStart)
	totalRequests := atomic.LoadInt64(&lsm.metrics.TotalRequests)
	if windowDuration > 0 {
		lsm.metrics.RequestRate = float64(totalRequests) / windowDuration.Seconds()
	}

	// Update error rate
	if totalRequests > 0 {
		lsm.metrics.ErrorRate = float64(atomic.LoadInt64(&lsm.errorCount)) / float64(totalRequests)
	}

	// Reset window if needed
	if windowDuration > lsm.config.MetricsWindow {
		lsm.metrics.WindowStart = now
		atomic.StoreInt64(&lsm.metrics.TotalRequests, 0)
		atomic.StoreInt64(&lsm.metrics.ShedRequests, 0)
		atomic.StoreInt64(&lsm.errorCount, 0)
	}

	lsm.metrics.LastUpdated = now

	// Update stats
	lsm.stats.CurrentSheddingRate = lsm.metrics.CurrentSheddingRate
	lsm.stats.TotalRequests = totalRequests
	shedRequests := atomic.LoadInt64(&lsm.metrics.ShedRequests)
	lsm.stats.ShedRequests = shedRequests
	if totalRequests > 0 {
		lsm.stats.SheddingRatio = float64(shedRequests) / float64(totalRequests)
	}
	lsm.stats.AverageLatency = lsm.metrics.AverageLatency

	// Create a safe copy of metrics with atomic values
	lsm.stats.SystemMetrics = LoadMetrics{
		CPUUsage:            lsm.metrics.CPUUsage,
		MemoryUsage:         lsm.metrics.MemoryUsage,
		ActiveRequests:      atomic.LoadInt64(&lsm.metrics.ActiveRequests),
		RequestRate:         lsm.metrics.RequestRate,
		AverageLatency:      lsm.metrics.AverageLatency,
		P95Latency:          lsm.metrics.P95Latency,
		P99Latency:          lsm.metrics.P99Latency,
		ErrorRate:           lsm.metrics.ErrorRate,
		CurrentSheddingRate: lsm.metrics.CurrentSheddingRate,
		TotalRequests:       totalRequests,
		ShedRequests:        shedRequests,
		LastUpdated:         lsm.metrics.LastUpdated,
		WindowStart:         lsm.metrics.WindowStart,
	}
}

// recordShedding records metrics for shed requests
func (lsm *loadSheddingManager) recordShedding(ctx *lift.Context) {
	if !lsm.config.EnableMetrics || lsm.config.Metrics == nil {
		return
	}

	priority := lsm.config.PriorityExtractor(ctx)

	tags := map[string]string{
		"load_shedding_name": lsm.config.Name,
		"strategy":           string(lsm.config.Strategy),
		"result":             "shed",
		"priority":           fmt.Sprintf("%d", priority),
	}

	metrics := lsm.config.Metrics.WithTags(tags)

	// Record shedding
	counter := metrics.Counter("load_shedding.requests.total")
	counter.Inc()

	// Record current shedding rate
	gauge := metrics.Gauge("load_shedding.rate")
	gauge.Set(lsm.getCurrentSheddingRate())
}

// recordSuccess records metrics for successful requests
func (lsm *loadSheddingManager) recordSuccess(ctx *lift.Context, duration time.Duration) {
	if !lsm.config.EnableMetrics || lsm.config.Metrics == nil {
		return
	}

	priority := lsm.config.PriorityExtractor(ctx)

	tags := map[string]string{
		"load_shedding_name": lsm.config.Name,
		"strategy":           string(lsm.config.Strategy),
		"result":             "success",
		"priority":           fmt.Sprintf("%d", priority),
	}

	metrics := lsm.config.Metrics.WithTags(tags)

	// Record success
	counter := metrics.Counter("load_shedding.requests.total")
	counter.Inc()

	// Record latency
	histogram := metrics.Histogram("load_shedding.latency")
	histogram.Observe(float64(duration.Milliseconds()))

	// Record active requests
	gauge := metrics.Gauge("load_shedding.active_requests")
	gauge.Set(float64(atomic.LoadInt64(&lsm.metrics.ActiveRequests)))
}

// GetStats returns current load shedding statistics
func (lsm *loadSheddingManager) GetStats() LoadSheddingStats {
	lsm.mutex.RLock()
	defer lsm.mutex.RUnlock()
	return *lsm.stats
}

// Default implementations

// defaultLoadSheddingPriorityExtractor extracts priority from context
func defaultLoadSheddingPriorityExtractor(ctx *lift.Context) int {
	// Check for priority header
	if priority := ctx.Request.Headers["X-Priority"]; priority != "" {
		switch priority {
		case "critical":
			return 10
		case priorityHigh:
			return 8
		case "normal":
			return 5
		case priorityLow:
			return 2
		case "background":
			return 1
		default:
			return 5
		}
	}
	return 5 // Normal priority
}

// defaultSheddingHandler creates a default shedding response handler
func defaultSheddingHandler(statusCode int, message string) func(*lift.Context) error {
	return func(ctx *lift.Context) error {
		return ctx.Status(statusCode).JSON(map[string]any{
			"error":       "Service Overloaded",
			"message":     message,
			"code":        "LOAD_SHED",
			"retry_after": "5",
		})
	}
}

// Utility functions for common load shedding configurations

// NewBasicLoadShedding creates a basic load shedding configuration
func NewBasicLoadShedding(name string) LoadSheddingConfig {
	return LoadSheddingConfig{
		Name:               name,
		Strategy:           LoadSheddingAdaptive,
		Enabled:            true,
		CPUThreshold:       0.8,
		MemoryThreshold:    0.85,
		LatencyThreshold:   5 * time.Second,
		ErrorRateThreshold: 0.1,
		TargetLatency:      100 * time.Millisecond,
		MaxSheddingRate:    0.9,
		MinSheddingRate:    0.0,
		AdaptationRate:     0.1,
		MetricsWindow:      30 * time.Second,
		SamplingRate:       1.0,
		EnableMetrics:      true,
	}
}

// NewPriorityLoadShedding creates a priority-based load shedding configuration
func NewPriorityLoadShedding(name string, priorityThresholds map[int]float64) LoadSheddingConfig {
	config := NewBasicLoadShedding(name)
	config.Strategy = LoadSheddingPriority
	config.PriorityThresholds = priorityThresholds
	return config
}

// NewAdaptiveLoadShedding creates an adaptive load shedding configuration
func NewAdaptiveLoadShedding(name string, targetLatency time.Duration) LoadSheddingConfig {
	config := NewBasicLoadShedding(name)
	config.Strategy = LoadSheddingAdaptive
	config.TargetLatency = targetLatency
	return config
}

// NewCustomLoadShedding creates a custom load shedding configuration
func NewCustomLoadShedding(name string, customShedder func(*lift.Context, *LoadMetrics) bool) LoadSheddingConfig {
	config := NewBasicLoadShedding(name)
	config.Strategy = LoadSheddingCustom
	config.CustomShedder = customShedder
	return config
}
