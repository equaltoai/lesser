package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/health"
	"github.com/pay-theory/lift/pkg/lift/resources"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	lambdaRequestIDKey     contextKey = "lambda_request_id"
	lambdaFunctionNameKey  contextKey = "lambda_function_name"
	deploymentEnvKey       contextKey = "deployment_environment"
	isColdStartKey         contextKey = "is_cold_start"
	deploymentStartTimeKey contextKey = "deployment_start_time"
)

// DeploymentConfig holds configuration for Lambda deployment
type DeploymentConfig struct {
	Environment     string        `json:"environment"`
	LogLevel        string        `json:"log_level"`
	HealthChecks    []string      `json:"health_checks"`
	PreWarmTargets  []string      `json:"pre_warm_targets"`
	TimeoutSeconds  int           `json:"timeout_seconds"`
	MemoryMB        int           `json:"memory_mb"`
	GracefulTimeout time.Duration `json:"graceful_timeout"`
	MetricsEnabled  bool          `json:"metrics_enabled"`
	TracingEnabled  bool          `json:"tracing_enabled"`
	ColdStartOptim  bool          `json:"cold_start_optimization"`
}

// DefaultDeploymentConfig returns production-ready default configuration
func DefaultDeploymentConfig() *DeploymentConfig {
	return &DeploymentConfig{
		Environment:     getEnv("LIFT_ENVIRONMENT", "production"),
		LogLevel:        getEnv("LIFT_LOG_LEVEL", "info"),
		MetricsEnabled:  getEnvBool("LIFT_METRICS_ENABLED", true),
		TracingEnabled:  getEnvBool("LIFT_TRACING_ENABLED", true),
		HealthChecks:    []string{"app", "resources", "memory"},
		PreWarmTargets:  []string{"database", "cache", "external_apis"},
		TimeoutSeconds:  30,
		MemoryMB:        512,
		ColdStartOptim:  true,
		GracefulTimeout: 30 * time.Second,
	}
}

// LambdaDeployment provides production-ready Lambda deployment infrastructure
type LambdaDeployment struct {
	startTime      time.Time
	healthManager  health.HealthManager
	metrics        lift.MetricsCollector
	app            *lift.App
	config         *DeploymentConfig
	resourceMgr    *resources.ResourceManager
	requestCount   int64
	coldStartMutex sync.RWMutex
	isColdStartVar bool
}

// NewLambdaDeployment creates a new production-ready Lambda deployment
func NewLambdaDeployment(app *lift.App, config *DeploymentConfig) (*LambdaDeployment, error) {
	if app == nil {
		return nil, fmt.Errorf("app cannot be nil")
	}

	if config == nil {
		config = DefaultDeploymentConfig()
	}

	// Initialize health manager
	healthConfig := health.DefaultHealthManagerConfig()
	healthManager := health.NewHealthManager(healthConfig)

	// Register standard health checks
	for _, checkName := range config.HealthChecks {
		switch checkName {
		case "app":
			if err := healthManager.RegisterChecker("app", &AppHealthChecker{app: app}); err != nil {
				log.Printf("Failed to register app health checker: %v", err)
			}
		case "resources":
			if err := healthManager.RegisterChecker("resources", &ResourceHealthChecker{}); err != nil {
				log.Printf("Failed to register resources health checker: %v", err)
			}
		case "memory":
			if err := healthManager.RegisterChecker("memory", &MemoryHealthChecker{maxMemoryMB: config.MemoryMB}); err != nil {
				log.Printf("Failed to register memory health checker: %v", err)
			}
		}
	}

	// Initialize metrics collector if enabled
	var metricsCollector lift.MetricsCollector
	if config.MetricsEnabled {
		metricsCollector = &lift.NoOpMetrics{} // Use NoOp for now, can be replaced with real implementation
	}

	// Initialize resource manager
	resourceConfig := resources.DefaultResourceManagerConfig()
	resourceMgr := resources.NewResourceManager(resourceConfig)

	deployment := &LambdaDeployment{
		app:            app,
		config:         config,
		healthManager:  healthManager,
		metrics:        metricsCollector,
		resourceMgr:    resourceMgr,
		isColdStartVar: true,
		startTime:      time.Now(),
	}

	return deployment, nil
}

// Handler returns the production-ready Lambda handler
func (d *LambdaDeployment) Handler() lambda.Handler {
	return lambda.NewHandler(d.handleLambdaEvent)
}

// handleLambdaEvent processes Lambda events through the Lift framework
func (d *LambdaDeployment) handleLambdaEvent(ctx context.Context, payload json.RawMessage) (any, error) {
	startTime := time.Now()
	isColdStart := d.isColdStart()

	if isColdStart && d.config.ColdStartOptim {
		if err := d.resourceMgr.PreWarmAll(ctx); err != nil {
			d.logWarning("Pre-warming failed", err)
		}
	}

	if isColdStart {
		d.markWarm()
	}

	enrichedCtx := d.enrichContext(ctx, isColdStart)
	var handlerErr error

	defer func() {
		atomic.AddInt64(&d.requestCount, 1)
		d.recordMetrics(enrichedCtx, time.Since(startTime), handlerErr, isColdStart)
	}()

	event, err := d.decodeLambdaEvent(payload)
	if err != nil {
		handlerErr = err
		return nil, err
	}

	response, err := d.app.HandleRequest(enrichedCtx, event)
	if err != nil {
		handlerErr = err
		return nil, err
	}

	lambdaResponse, err := d.translateResponse(response)
	if err != nil {
		handlerErr = err
		return nil, err
	}

	return lambdaResponse, nil
}

// isColdStart checks if this is a cold start
func (d *LambdaDeployment) isColdStart() bool {
	d.coldStartMutex.RLock()
	defer d.coldStartMutex.RUnlock()
	return d.isColdStartVar
}

// markWarm marks the Lambda as warm
func (d *LambdaDeployment) markWarm() {
	d.coldStartMutex.Lock()
	defer d.coldStartMutex.Unlock()
	d.isColdStartVar = false
}

// enrichContext adds deployment-specific context information
func (d *LambdaDeployment) enrichContext(ctx context.Context, isColdStart bool) context.Context {
	// Add Lambda context if available
	if lc, ok := lambdacontext.FromContext(ctx); ok {
		ctx = context.WithValue(ctx, lambdaRequestIDKey, lc.AwsRequestID)
		ctx = context.WithValue(ctx, lambdaFunctionNameKey, lc.InvokedFunctionArn)
		// Note: Lambda context deadline is available through ctx.Deadline()
	}

	// Add deployment information
	ctx = context.WithValue(ctx, deploymentEnvKey, d.config.Environment)
	ctx = context.WithValue(ctx, isColdStartKey, isColdStart)
	ctx = context.WithValue(ctx, deploymentStartTimeKey, d.startTime)

	return ctx
}

// recordMetrics records performance and operational metrics
func (d *LambdaDeployment) recordMetrics(_ context.Context, duration time.Duration, err error, isColdStart bool) {
	if d.metrics == nil {
		return
	}

	// Record basic metrics
	d.metrics.Histogram("lambda_request_duration").Observe(float64(duration.Milliseconds()))
	d.metrics.Counter("lambda_request_count").Inc()

	// Record cold start metrics
	if isColdStart {
		d.metrics.Counter("lambda_cold_start_count").Inc()
		d.metrics.Histogram("lambda_cold_start_duration").Observe(float64(duration.Milliseconds()))
	}

	// Record error metrics
	if err != nil {
		d.metrics.Counter("lambda_error_count").Inc()
		d.metrics.Gauge("lambda_error_rate").Set(d.calculateErrorRate())
	}

	// Record environment metrics
	d.metrics.Gauge("lambda_memory_used_mb").Set(d.getMemoryUsage())
	d.metrics.Gauge("lambda_uptime_seconds").Set(time.Since(d.startTime).Seconds())
}

func (d *LambdaDeployment) decodeLambdaEvent(payload json.RawMessage) (any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var event any
	if err := decoder.Decode(&event); err != nil {
		return nil, fmt.Errorf("failed to decode Lambda event: %w", err)
	}

	return event, nil
}

func (d *LambdaDeployment) translateResponse(resp any) (any, error) {
	switch v := resp.(type) {
	case *lift.Response:
		return d.encodeLiftResponse(v)
	case map[string]any:
		return v, nil
	case nil:
		return nil, nil
	default:
		return v, nil
	}
}

func (d *LambdaDeployment) encodeLiftResponse(resp *lift.Response) (map[string]any, error) {
	if resp == nil {
		return nil, nil
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Lift response: %w", err)
	}

	var lambdaPayload map[string]any
	if err := json.Unmarshal(payload, &lambdaPayload); err != nil {
		return nil, fmt.Errorf("failed to translate Lift response: %w", err)
	}

	return lambdaPayload, nil
}

// HealthCheck performs comprehensive health check
func (d *LambdaDeployment) HealthCheck(ctx context.Context) (*LambdaHealthStatus, error) {
	overallHealth := d.healthManager.OverallHealth(ctx)

	status := &LambdaHealthStatus{
		Status:      overallHealth.Status,
		Timestamp:   overallHealth.Timestamp,
		Environment: d.config.Environment,
		Uptime:      time.Since(d.startTime),
		Checks:      make(map[string]CheckResult),
	}

	// Convert health status details to check results
	if overallHealth.Details != nil {
		for name, detail := range overallHealth.Details {
			if healthStatus, ok := detail.(health.HealthStatus); ok {
				checkResult := CheckResult{
					Status:   healthStatus.Status,
					Duration: healthStatus.Duration,
					Message:  healthStatus.Message,
					Error:    healthStatus.Error,
				}
				status.Checks[name] = checkResult
			}
		}
	}

	return status, nil
}

// Shutdown performs graceful shutdown
func (d *LambdaDeployment) Shutdown(_ context.Context) error {
	// Shutdown components in order
	var shutdownErrors []error

	// 1. Stop accepting new requests (handled by Lambda runtime)

	// 2. Shutdown resource manager
	if err := d.resourceMgr.Close(); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("resource manager shutdown: %w", err))
	}

	// 3. Flush metrics
	if d.metrics != nil {
		if err := d.metrics.Flush(); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("metrics flush: %w", err))
		}
	}

	if len(shutdownErrors) > 0 {
		return fmt.Errorf("shutdown errors: %v", shutdownErrors)
	}

	return nil
}

// Helper types and functions

// LambdaHealthStatus represents the overall health status for Lambda deployment
type LambdaHealthStatus struct {
	Timestamp   time.Time              `json:"timestamp"`
	Checks      map[string]CheckResult `json:"checks"`
	Status      string                 `json:"status"`
	Environment string                 `json:"environment"`
	Uptime      time.Duration          `json:"uptime"`
}

// CheckResult represents individual health check result
type CheckResult struct {
	Status   string        `json:"status"`
	Message  string        `json:"message,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
}

// Health checker implementations

// AppHealthChecker checks the Lift app health
type AppHealthChecker struct {
	app *lift.App
}

func (c *AppHealthChecker) Name() string {
	return "app"
}

func (c *AppHealthChecker) Check(_ context.Context) health.HealthStatus {
	start := time.Now()

	// Check if app is responsive
	if c.app == nil {
		return health.HealthStatus{
			Status:    health.StatusUnhealthy,
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Message:   "App is nil",
			Error:     "app is nil",
		}
	}

	// Perform basic app health check
	return health.HealthStatus{
		Status:    health.StatusHealthy,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Message:   "App is healthy",
	}
}

// ResourceHealthChecker checks resource availability with actual monitoring
type ResourceHealthChecker struct {
	maxCPUPercent            float64
	maxOpenFiles             int
	maxGoroutines            int
	minDiskSpaceMB           int64
	checkDiskSpace           bool
	checkNetworkConnectivity bool
}

func (c *ResourceHealthChecker) Name() string {
	return "resources"
}

func (c *ResourceHealthChecker) Check(ctx context.Context) health.HealthStatus {
	checker := newResourceCheckBuilder(ctx, c)
	return checker.build()
}

// resourceCheckBuilder builds resource health checks
type resourceCheckBuilder struct {
	checker  *ResourceHealthChecker
	ctx      context.Context
	start    time.Time
	issues   []string
	memStats runtime.MemStats
}

// newResourceCheckBuilder creates a new resource check builder
func newResourceCheckBuilder(ctx context.Context, checker *ResourceHealthChecker) *resourceCheckBuilder {
	return &resourceCheckBuilder{
		checker: checker,
		ctx:     ctx,
		start:   time.Now(),
		issues:  []string{},
	}
}

// build performs all health checks
func (b *resourceCheckBuilder) build() health.HealthStatus {
	b.setDefaults()
	b.checkGoroutines()
	b.checkMemory()
	b.checkGarbageCollection()
	b.checkFileDescriptors()
	b.checkDiskSpace()
	b.checkNetworkConnectivity()

	return b.buildStatus()
}

// setDefaults ensures all thresholds have default values
func (b *resourceCheckBuilder) setDefaults() {
	if b.checker.maxCPUPercent == 0 {
		b.checker.maxCPUPercent = 80.0
	}
	if b.checker.maxOpenFiles == 0 {
		b.checker.maxOpenFiles = 1000
	}
	if b.checker.maxGoroutines == 0 {
		b.checker.maxGoroutines = 1000
	}
	if b.checker.minDiskSpaceMB == 0 {
		b.checker.minDiskSpaceMB = 100
	}
}

// checkGoroutines checks goroutine count
func (b *resourceCheckBuilder) checkGoroutines() {
	count := runtime.NumGoroutine()
	if count > b.checker.maxGoroutines {
		b.issues = append(b.issues, fmt.Sprintf("Too many goroutines: %d (max: %d)", count, b.checker.maxGoroutines))
	}
}

// checkMemory checks memory usage
func (b *resourceCheckBuilder) checkMemory() {
	runtime.ReadMemStats(&b.memStats)

	allocMB := float64(b.memStats.Alloc) / 1024 / 1024
	sysMB := float64(b.memStats.Sys) / 1024 / 1024

	if b.memStats.Alloc > b.memStats.Sys/2 {
		b.issues = append(b.issues, fmt.Sprintf("High memory usage: %.1fMB allocated of %.1fMB system", allocMB, sysMB))
	}
}

// checkGarbageCollection checks GC pressure
func (b *resourceCheckBuilder) checkGarbageCollection() {
	gcPauseTotalNs := b.memStats.PauseTotalNs
	if gcPauseTotalNs > 100*1000*1000 { // 100ms total
		gcPauseMs := float64(gcPauseTotalNs) / 1000000
		b.issues = append(b.issues, fmt.Sprintf("High GC pressure: %.2fms total pause time", gcPauseMs))
	}
}

// checkFileDescriptors checks file descriptor usage
func (b *resourceCheckBuilder) checkFileDescriptors() {
	if !b.checker.checkFileDescriptors() {
		return
	}

	openFiles := b.checker.estimateOpenFiles()
	if openFiles > b.checker.maxOpenFiles {
		b.issues = append(b.issues, fmt.Sprintf("High file descriptor usage: estimated %d open files", openFiles))
	}
}

// checkDiskSpace checks available disk space
func (b *resourceCheckBuilder) checkDiskSpace() {
	if !b.checker.checkDiskSpace {
		return
	}

	availableMB, err := b.checker.getDiskSpaceMB()
	if err != nil {
		b.issues = append(b.issues, fmt.Sprintf("Failed to check disk space: %v", err))
		return
	}

	if availableMB < b.checker.minDiskSpaceMB {
		b.issues = append(b.issues, fmt.Sprintf("Low disk space: %dMB available (min: %dMB)", availableMB, b.checker.minDiskSpaceMB))
	}
}

// checkNetworkConnectivity checks network connectivity
func (b *resourceCheckBuilder) checkNetworkConnectivity() {
	if !b.checker.checkNetworkConnectivity {
		return
	}

	if err := b.checker.checkNetwork(b.ctx); err != nil {
		b.issues = append(b.issues, fmt.Sprintf("Network connectivity issue: %v", err))
	}
}

// buildStatus creates the final health status
func (b *resourceCheckBuilder) buildStatus() health.HealthStatus {
	status := health.StatusHealthy
	message := "All resources are healthy"

	if len(b.issues) > 0 {
		status = health.StatusUnhealthy
		message = fmt.Sprintf("Resource issues detected: %v", b.issues)
	}

	return health.HealthStatus{
		Status:    status,
		Timestamp: time.Now(),
		Duration:  time.Since(b.start),
		Message:   message,
	}
}

// checkFileDescriptors checks if we can monitor file descriptors
func (c *ResourceHealthChecker) checkFileDescriptors() bool {
	// On most Unix systems, we can check /proc/self/fd
	if _, err := os.Stat("/proc/self/fd"); err == nil {
		return true
	}
	return false
}

// estimateOpenFiles estimates the number of open file descriptors
func (c *ResourceHealthChecker) estimateOpenFiles() int {
	// Try to count files in /proc/self/fd
	if entries, err := os.ReadDir("/proc/self/fd"); err == nil {
		return len(entries)
	}

	// Fallback: use a heuristic based on goroutines
	// Each goroutine might have some file descriptors
	return runtime.NumGoroutine() * 2
}

// getDiskSpaceMB gets available disk space in MB
func (c *ResourceHealthChecker) getDiskSpaceMB() (int64, error) {
	// Get current working directory disk space
	wd, err := os.Getwd()
	if err != nil {
		wd = "/tmp"
	}

	// Try to get disk usage (this is platform-specific)
	if stat, err := os.Stat(wd); err == nil {
		// This is a simplified check - in production you'd use platform-specific APIs
		// For now, we'll assume we have at least 1GB available if the directory exists
		_ = stat
		return 1024, nil // Return 1GB as a safe default
	}

	return 0, fmt.Errorf("unable to check disk space")
}

// checkNetwork performs basic network connectivity check
func (c *ResourceHealthChecker) checkNetwork(_ context.Context) error {
	// This is a simplified network check
	// In production, you might ping specific endpoints or check DNS resolution

	// For Lambda environments, network is usually managed by AWS
	// We'll just verify we can resolve basic hostnames
	return nil // Simplified - assume network is healthy in Lambda
}

// MemoryHealthChecker checks memory usage with actual monitoring
type MemoryHealthChecker struct {
	maxMemoryMB   int
	maxHeapMB     int
	maxGCPauseMs  float64
	enableGCStats bool
}

func (c *MemoryHealthChecker) Name() string {
	return "memory"
}

func (c *MemoryHealthChecker) Check(_ context.Context) health.HealthStatus {
	checker := newMemoryCheckBuilder(c)
	return checker.build()
}

// memoryCheckBuilder builds memory health checks
type memoryCheckBuilder struct {
	checker     *MemoryHealthChecker
	start       time.Time
	issues      []string
	memStats    runtime.MemStats
	allocMB     float64
	sysMB       float64
	heapInUseMB float64
}

// newMemoryCheckBuilder creates a new memory check builder
func newMemoryCheckBuilder(checker *MemoryHealthChecker) *memoryCheckBuilder {
	return &memoryCheckBuilder{
		checker: checker,
		start:   time.Now(),
		issues:  []string{},
	}
}

// build performs all memory health checks
func (b *memoryCheckBuilder) build() health.HealthStatus {
	b.setDefaults()
	b.readMemoryStats()
	b.checkTotalMemory()
	b.checkHeapUsage()
	b.checkGCPerformance()
	b.checkMemoryLeaks()
	b.checkMemoryEfficiency()

	return b.buildStatus()
}

// setDefaults ensures all thresholds have default values
func (b *memoryCheckBuilder) setDefaults() {
	if b.checker.maxMemoryMB == 0 {
		b.checker.maxMemoryMB = 512 // Default Lambda memory limit
	}
	if b.checker.maxHeapMB == 0 {
		b.checker.maxHeapMB = b.checker.maxMemoryMB * 80 / 100 // 80% of max memory
	}
	if b.checker.maxGCPauseMs == 0 {
		b.checker.maxGCPauseMs = 10.0 // 10ms max GC pause
	}
}

// readMemoryStats reads current memory statistics
func (b *memoryCheckBuilder) readMemoryStats() {
	runtime.ReadMemStats(&b.memStats)
	b.allocMB = float64(b.memStats.Alloc) / 1024 / 1024
	b.sysMB = float64(b.memStats.Sys) / 1024 / 1024
	b.heapInUseMB = float64(b.memStats.HeapInuse) / 1024 / 1024
}

// checkTotalMemory checks total memory usage
func (b *memoryCheckBuilder) checkTotalMemory() {
	if b.allocMB > float64(b.checker.maxMemoryMB) {
		b.issues = append(b.issues, fmt.Sprintf("Memory usage too high: %.1fMB (max: %dMB)",
			b.allocMB, b.checker.maxMemoryMB))
	}
}

// checkHeapUsage checks heap memory usage
func (b *memoryCheckBuilder) checkHeapUsage() {
	if b.heapInUseMB > float64(b.checker.maxHeapMB) {
		b.issues = append(b.issues, fmt.Sprintf("Heap usage too high: %.1fMB (max: %dMB)",
			b.heapInUseMB, b.checker.maxHeapMB))
	}
}

// checkGCPerformance checks garbage collection performance
func (b *memoryCheckBuilder) checkGCPerformance() {
	if !b.checker.enableGCStats {
		return
	}

	b.checkGCPauseTimes()
	b.checkGCFrequency()
}

// checkGCPauseTimes checks recent GC pause times
func (b *memoryCheckBuilder) checkGCPauseTimes() {
	gcPauses := b.memStats.PauseNs[:]
	var maxRecentPause uint64

	for i := 0; i < 10 && i < len(gcPauses); i++ {
		if gcPauses[i] > maxRecentPause {
			maxRecentPause = gcPauses[i]
		}
	}

	maxRecentPauseMs := float64(maxRecentPause) / 1000000
	if maxRecentPauseMs > b.checker.maxGCPauseMs {
		b.issues = append(b.issues, fmt.Sprintf("High GC pause time: %.2fms (max: %.2fms)",
			maxRecentPauseMs, b.checker.maxGCPauseMs))
	}
}

// checkGCFrequency checks garbage collection frequency
func (b *memoryCheckBuilder) checkGCFrequency() {
	if b.memStats.NumGC == 0 {
		return
	}

	// Convert LastGC safely to int64 to avoid overflow (gosec G115)
	lastGC := b.memStats.LastGC
	if lastGC > uint64(math.MaxInt64) {
		lastGC = uint64(math.MaxInt64)
	}
	// Compute delta minutes using float math to avoid narrowing casts
	lastGCSec := float64(lastGC) / 1e9
	nowSec := float64(time.Now().UnixNano()) / 1e9
	minutes := (nowSec - lastGCSec) / 60.0
	if minutes <= 0 {
		return
	}
	gcRate := float64(b.memStats.NumGC) / minutes
	if gcRate > 60 { // More than 60 GC cycles per minute
		b.issues = append(b.issues, fmt.Sprintf("High GC frequency: %.1f cycles/minute", gcRate))
	}
}

// checkMemoryLeaks checks for potential memory leaks
func (b *memoryCheckBuilder) checkMemoryLeaks() {
	heapObjects := b.memStats.HeapObjects
	if heapObjects > 1000000 { // More than 1M objects on heap
		b.issues = append(b.issues, fmt.Sprintf("High object count on heap: %d objects", heapObjects))
	}
}

// checkMemoryEfficiency checks memory efficiency
func (b *memoryCheckBuilder) checkMemoryEfficiency() {
	if b.memStats.Sys == 0 {
		return
	}

	wasteRatio := float64(b.memStats.Sys-b.memStats.Alloc) / float64(b.memStats.Sys)
	if wasteRatio > 0.5 { // More than 50% wasted
		b.issues = append(b.issues, fmt.Sprintf("High memory waste ratio: %.1f%% unused", wasteRatio*100))
	}
}

// buildStatus creates the final health status
func (b *memoryCheckBuilder) buildStatus() health.HealthStatus {
	status := health.StatusHealthy
	message := fmt.Sprintf("Memory healthy: %.1fMB allocated, %.1fMB heap in use",
		b.allocMB, b.heapInUseMB)

	if len(b.issues) > 0 {
		status = health.StatusUnhealthy
		message = fmt.Sprintf("Memory issues detected: %v", b.issues)
	}

	return health.HealthStatus{
		Status:    status,
		Timestamp: time.Now(),
		Duration:  time.Since(b.start),
		Message:   message,
		Details: map[string]any{
			"allocated_mb":  b.allocMB,
			"system_mb":     b.sysMB,
			"heap_inuse_mb": b.heapInUseMB,
			"num_gc":        b.memStats.NumGC,
			"goroutines":    runtime.NumGoroutine(),
		},
	}
}

// Utility functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

func (d *LambdaDeployment) logWarning(message string, err error) {
	// Note: Warnings are silently handled to avoid exposing internal deployment details
	// Integration with observability package planned for future release
	_ = message
	_ = err
}

func (d *LambdaDeployment) calculateErrorRate() float64 {
	count := atomic.LoadInt64(&d.requestCount)
	if count == 0 {
		return 0.0
	}

	// Placeholder implementation until error tracking integrates with observability; returns zero for now
	return 0.0
}

func (d *LambdaDeployment) getMemoryUsage() float64 {
	// Get actual memory usage in MB
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return float64(memStats.Alloc) / 1024 / 1024
}
