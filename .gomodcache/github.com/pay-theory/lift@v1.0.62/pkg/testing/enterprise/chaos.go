package enterprise

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ChaosTest manages chaos engineering tests
type ChaosTest struct {
	metrics   *ChaosMetrics
	recovery  *RecoveryValidator
	scenarios []ChaosScenario
	config    ChaosConfig
}

// ChaosConfig is now defined in types.go

// ChaosScenario represents a chaos engineering scenario
type ChaosScenario interface {
	Name() string
	Description() string
	InjectFailure(ctx context.Context) (ChaosFailure, error)
	ExecuteOperations(ctx context.Context) ([]OperationResult, error)
	ValidateRecovery(ctx context.Context, metrics *ChaosMetrics) error
}

// ChaosFailure represents an injected failure
type ChaosFailure interface {
	Type() string
	Severity() ChaosSeverity
	Duration() time.Duration
	Cleanup() error
	IsActive() bool
}

// ChaosSeverity represents the severity of a chaos failure
type ChaosSeverity string

const (
	ChaosSeverityLow      ChaosSeverity = "low"
	ChaosSeverityMedium   ChaosSeverity = "medium"
	ChaosSeverityHigh     ChaosSeverity = "high"
	ChaosSeverityCritical ChaosSeverity = "critical"
)

// OperationResult represents the result of an operation during chaos testing
type OperationResult struct {
	Timestamp time.Time
	Error     error
	Metadata  map[string]any
	Operation string
	Duration  time.Duration
	Success   bool
}

// ChaosMetrics tracks metrics during chaos testing
type ChaosMetrics struct {
	StartTime       time.Time
	EndTime         time.Time
	ResourceUsage   map[string]float64
	CustomMetrics   map[string]any
	TotalOperations int
	SuccessfulOps   int
	FailedOps       int
	AverageLatency  time.Duration
	RecoveryTime    time.Duration
	ErrorRate       float64
	Throughput      float64
	mutex           sync.RWMutex
}

// RecoveryValidator validates system recovery after chaos injection
type RecoveryValidator struct {
	healthChecks []HealthCheck
	thresholds   RecoveryThresholds
}

// HealthCheck represents a health check function
type ChaosHealthCheck func(ctx context.Context) error

// RecoveryThresholds defines thresholds for recovery validation
type RecoveryThresholds struct {
	MaxRecoveryTime    time.Duration
	MinSuccessRate     float64
	MaxErrorRate       float64
	MaxLatencyIncrease float64
}

// NewChaosTest creates a new chaos test
func NewChaosTest(config ChaosConfig) *ChaosTest {
	return &ChaosTest{
		scenarios: make([]ChaosScenario, 0),
		metrics:   NewChaosMetrics(),
		recovery:  NewRecoveryValidator(),
		config:    config,
	}
}

// NewChaosMetrics creates new chaos metrics
func NewChaosMetrics() *ChaosMetrics {
	return &ChaosMetrics{
		ResourceUsage: make(map[string]float64),
		CustomMetrics: make(map[string]any),
	}
}

// NewRecoveryValidator creates a new recovery validator
func NewRecoveryValidator() *RecoveryValidator {
	return &RecoveryValidator{
		healthChecks: make([]HealthCheck, 0),
		thresholds: RecoveryThresholds{
			MaxRecoveryTime:    5 * time.Minute,
			MinSuccessRate:     0.95,
			MaxErrorRate:       0.05,
			MaxLatencyIncrease: 2.0,
		},
	}
}

// AddScenario adds a chaos scenario to the test
func (c *ChaosTest) AddScenario(scenario ChaosScenario) {
	c.scenarios = append(c.scenarios, scenario)
}

// ExecuteChaos executes all chaos scenarios
func (c *ChaosTest) ExecuteChaos(ctx context.Context) error {
	if !c.config.Enabled {
		return fmt.Errorf("chaos testing is disabled")
	}

	var errors []error

	for _, scenario := range c.scenarios {
		if err := c.executeScenario(ctx, scenario); err != nil {
			errors = append(errors, fmt.Errorf("scenario %s failed: %w", scenario.Name(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("chaos test failures: %v", errors)
	}

	return nil
}

// executeScenario executes a single chaos scenario
func (c *ChaosTest) executeScenario(ctx context.Context, scenario ChaosScenario) error {
	// Start metrics collection
	c.metrics.StartCollection()

	// Inject failure
	failure, err := scenario.InjectFailure(ctx)
	if err != nil {
		return fmt.Errorf("failed to inject failure: %w", err)
	}
	defer func() {
		if cleanupErr := failure.Cleanup(); cleanupErr != nil {
			log.Printf("Warning: failed to cleanup chaos failure: %v", cleanupErr)
		}
	}()

	// Execute operations under chaos
	results, err := scenario.ExecuteOperations(ctx)
	if err != nil {
		return fmt.Errorf("operations failed: %w", err)
	}

	// Update metrics with operation results
	c.metrics.UpdateWithResults(results)

	// Wait for failure to end
	time.Sleep(failure.Duration())

	// Validate recovery
	if err := c.recovery.ValidateRecovery(ctx, c.metrics); err != nil {
		return fmt.Errorf("recovery validation failed: %w", err)
	}

	// Validate scenario-specific recovery
	if err := scenario.ValidateRecovery(ctx, c.metrics); err != nil {
		return fmt.Errorf("scenario recovery validation failed: %w", err)
	}

	return nil
}

// StartCollection starts metrics collection
func (m *ChaosMetrics) StartCollection() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.StartTime = time.Now()
	m.TotalOperations = 0
	m.SuccessfulOps = 0
	m.FailedOps = 0
}

// UpdateWithResults updates metrics with operation results
func (m *ChaosMetrics) UpdateWithResults(results []OperationResult) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.TotalOperations += len(results)

	var totalDuration time.Duration
	for _, result := range results {
		if result.Success {
			m.SuccessfulOps++
		} else {
			m.FailedOps++
		}
		totalDuration += result.Duration
	}

	if len(results) > 0 {
		m.AverageLatency = totalDuration / time.Duration(len(results))
	}

	if m.TotalOperations > 0 {
		m.ErrorRate = float64(m.FailedOps) / float64(m.TotalOperations)
	}

	m.EndTime = time.Now()
	if !m.StartTime.IsZero() {
		duration := m.EndTime.Sub(m.StartTime)
		if duration > 0 {
			m.Throughput = float64(m.TotalOperations) / duration.Seconds()
		}
	}
}

// AddHealthCheck adds a health check to the recovery validator
func (r *RecoveryValidator) AddHealthCheck(check HealthCheck) {
	r.healthChecks = append(r.healthChecks, check)
}

// ValidateRecovery validates system recovery after chaos
func (r *RecoveryValidator) ValidateRecovery(ctx context.Context, metrics *ChaosMetrics) error {
	recoveryStart := time.Now()

	// Wait for system to recover
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(r.thresholds.MaxRecoveryTime)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("recovery timeout exceeded: %v", r.thresholds.MaxRecoveryTime)
		case <-ticker.C:
			if r.isSystemHealthy(ctx) {
				recoveryTime := time.Since(recoveryStart)
				metrics.RecoveryTime = recoveryTime
				return r.validateMetrics(metrics)
			}
		}
	}
}

// isSystemHealthy checks if the system is healthy
func (r *RecoveryValidator) isSystemHealthy(ctx context.Context) bool {
	for _, check := range r.healthChecks {
		if err := check(ctx); err != nil {
			return false
		}
	}
	return true
}

// validateMetrics validates recovery metrics against thresholds
func (r *RecoveryValidator) validateMetrics(metrics *ChaosMetrics) error {
	if metrics.ErrorRate > r.thresholds.MaxErrorRate {
		return fmt.Errorf("error rate too high: %.2f%% > %.2f%%",
			metrics.ErrorRate*100, r.thresholds.MaxErrorRate*100)
	}

	successRate := float64(metrics.SuccessfulOps) / float64(metrics.TotalOperations)
	if successRate < r.thresholds.MinSuccessRate {
		return fmt.Errorf("success rate too low: %.2f%% < %.2f%%",
			successRate*100, r.thresholds.MinSuccessRate*100)
	}

	return nil
}

// Concrete chaos scenarios

// NetworkLatencyScenario injects network latency
type NetworkLatencyScenario struct {
	name        string
	description string
	latency     time.Duration
	duration    time.Duration
	active      bool
}

// NewNetworkLatencyScenario creates a network latency scenario
func NewNetworkLatencyScenario(latency, duration time.Duration) *NetworkLatencyScenario {
	return &NetworkLatencyScenario{
		name:        "network_latency",
		description: fmt.Sprintf("Inject %v network latency for %v", latency, duration),
		latency:     latency,
		duration:    duration,
	}
}

func (n *NetworkLatencyScenario) Name() string        { return n.name }
func (n *NetworkLatencyScenario) Description() string { return n.description }

func (n *NetworkLatencyScenario) InjectFailure(_ context.Context) (ChaosFailure, error) {
	failure := &NetworkLatencyFailure{
		latency:  n.latency,
		duration: n.duration,
		active:   true,
	}

	// In a real implementation, this would configure network latency
	// For now, we'll simulate it
	n.active = true

	return failure, nil
}

func (n *NetworkLatencyScenario) ExecuteOperations(_ context.Context) ([]OperationResult, error) {
	var results []OperationResult

	// Simulate operations with injected latency
	for i := 0; i < 10; i++ {
		start := time.Now()

		// Simulate operation with added latency
		if n.active {
			time.Sleep(n.latency)
		}
		time.Sleep(10 * time.Millisecond) // Base operation time

		duration := time.Since(start)

		result := OperationResult{
			Operation: fmt.Sprintf("operation_%d", i),
			Success:   true,
			Duration:  duration,
			Timestamp: time.Now(),
		}

		results = append(results, result)
	}

	return results, nil
}

func (n *NetworkLatencyScenario) ValidateRecovery(_ context.Context, metrics *ChaosMetrics) error {
	// Validate that latency has returned to normal
	if metrics.AverageLatency > n.latency*2 {
		return fmt.Errorf("latency still elevated: %v", metrics.AverageLatency)
	}
	return nil
}

// NetworkLatencyFailure represents a network latency failure
type NetworkLatencyFailure struct {
	latency  time.Duration
	duration time.Duration
	active   bool
}

func (n *NetworkLatencyFailure) Type() string            { return "network_latency" }
func (n *NetworkLatencyFailure) Severity() ChaosSeverity { return ChaosSeverityMedium }
func (n *NetworkLatencyFailure) Duration() time.Duration { return n.duration }
func (n *NetworkLatencyFailure) IsActive() bool          { return n.active }

func (n *NetworkLatencyFailure) Cleanup() error {
	n.active = false
	// In a real implementation, this would remove network latency configuration
	return nil
}

// ServiceUnavailableScenario makes a service unavailable
type ServiceUnavailableScenario struct {
	name        string
	description string
	serviceName string
	duration    time.Duration
	active      bool
}

// NewServiceUnavailableScenario creates a service unavailable scenario
func NewServiceUnavailableScenario(serviceName string, duration time.Duration) *ServiceUnavailableScenario {
	return &ServiceUnavailableScenario{
		name:        "service_unavailable",
		description: fmt.Sprintf("Make %s unavailable for %v", serviceName, duration),
		serviceName: serviceName,
		duration:    duration,
	}
}

func (s *ServiceUnavailableScenario) Name() string        { return s.name }
func (s *ServiceUnavailableScenario) Description() string { return s.description }

func (s *ServiceUnavailableScenario) InjectFailure(_ context.Context) (ChaosFailure, error) {
	failure := &ServiceUnavailableFailure{
		serviceName: s.serviceName,
		duration:    s.duration,
		active:      true,
	}

	// In a real implementation, this would make the service unavailable
	s.active = true

	return failure, nil
}

func (s *ServiceUnavailableScenario) ExecuteOperations(_ context.Context) ([]OperationResult, error) {
	var results []OperationResult

	// Simulate operations with service unavailable
	for i := 0; i < 10; i++ {
		start := time.Now()

		var success bool
		var err error

		if s.active {
			// Simulate service unavailable
			success = false
			err = fmt.Errorf("service %s unavailable", s.serviceName)
		} else {
			success = true
		}

		duration := time.Since(start)

		result := OperationResult{
			Operation: fmt.Sprintf("operation_%d", i),
			Success:   success,
			Duration:  duration,
			Error:     err,
			Timestamp: time.Now(),
		}

		results = append(results, result)
	}

	return results, nil
}

func (s *ServiceUnavailableScenario) ValidateRecovery(_ context.Context, metrics *ChaosMetrics) error {
	// Validate that service is available again
	if metrics.ErrorRate > 0.1 { // Allow 10% error rate during recovery
		return fmt.Errorf("service still experiencing high error rate: %.2f%%", metrics.ErrorRate*100)
	}
	return nil
}

// ServiceUnavailableFailure represents a service unavailable failure
type ServiceUnavailableFailure struct {
	serviceName string
	duration    time.Duration
	active      bool
}

func (s *ServiceUnavailableFailure) Type() string            { return "service_unavailable" }
func (s *ServiceUnavailableFailure) Severity() ChaosSeverity { return ChaosSeverityHigh }
func (s *ServiceUnavailableFailure) Duration() time.Duration { return s.duration }
func (s *ServiceUnavailableFailure) IsActive() bool          { return s.active }

func (s *ServiceUnavailableFailure) Cleanup() error {
	s.active = false
	// In a real implementation, this would restore service availability
	return nil
}
