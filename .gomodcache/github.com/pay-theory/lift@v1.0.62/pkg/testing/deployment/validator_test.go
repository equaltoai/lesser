package deployment

import (
	"context"
	"testing"
	"time"
)

const (
	envBlue  = "blue"
	envGreen = "green"
)

func TestDeploymentValidator(t *testing.T) {
	config := DeploymentConfig{
		HealthCheckInterval: 5 * time.Second,
		MaxRollbackTime:     60 * time.Second,
		TrafficShiftDelay:   10 * time.Second,
		ValidationTimeout:   30 * time.Second,
		RequiredSuccessRate: 0.95,
		MaxErrorRate:        0.05,
	}

	validator := NewDeploymentValidator(config)

	// Add test environment
	env := Environment{
		Name:    "test-env",
		URL:     "http://localhost:8080",
		Version: "v1.0.0",
		Status:  EnvironmentStatusHealthy,
		Config: EnvironmentConfig{
			HealthCheckPath:    "/health",
			HealthCheckTimeout: 5 * time.Second,
			ExpectedStatusCode: 200,
			MaxResponseTime:    100 * time.Millisecond,
			MinSuccessRate:     0.95,
			TrafficWeight:      1.0,
		},
	}

	validator.AddEnvironment(&env)

	// Add HTTP health check
	healthCheck := NewHTTPHealthCheck("http-health", 5*time.Second)
	validator.AddHealthCheck(healthCheck)

	// Test environment validation
	ctx := context.Background()
	err := validator.ValidateEnvironment(ctx, "test-env")

	// Note: This will fail in test environment since localhost:8080 isn't running
	// In a real test, you'd mock the HTTP client or use a test server
	if err == nil {
		t.Error("Expected health check to fail since no server is running")
	}
}

func TestBlueGreenDeployment(t *testing.T) {
	// Create test environments
	blueEnv := Environment{
		Name:    envBlue,
		URL:     "http://blue.example.com",
		Version: "v1.0.0",
		Status:  EnvironmentStatusHealthy,
		Config: EnvironmentConfig{
			HealthCheckPath:    "/health",
			HealthCheckTimeout: 5 * time.Second,
			ExpectedStatusCode: 200,
			MaxResponseTime:    100 * time.Millisecond,
			TrafficWeight:      1.0,
		},
	}

	greenEnv := Environment{
		Name:    envGreen,
		URL:     "http://green.example.com",
		Version: "v1.0.0",
		Status:  EnvironmentStatusHealthy,
		Config: EnvironmentConfig{
			HealthCheckPath:    "/health",
			HealthCheckTimeout: 5 * time.Second,
			ExpectedStatusCode: 200,
			MaxResponseTime:    100 * time.Millisecond,
			TrafficWeight:      0.0,
		},
	}

	// Create validator
	config := DeploymentConfig{
		HealthCheckInterval: 5 * time.Second,
		MaxRollbackTime:     60 * time.Second,
		TrafficShiftDelay:   10 * time.Second,
		ValidationTimeout:   30 * time.Second,
		RequiredSuccessRate: 0.95,
		MaxErrorRate:        0.05,
	}
	validator := NewDeploymentValidator(config)

	// Create traffic splitter
	splitter := &DefaultTrafficSplitter{}

	// Create blue/green deployment
	bgDeployment := NewBlueGreenDeployment(&blueEnv, &greenEnv, splitter, validator)

	// Test initial state
	if bgDeployment.currentActive != envBlue {
		t.Errorf("Expected initial active environment to be 'blue', got '%s'", bgDeployment.currentActive)
	}

	// Test deployment (this will fail health checks but we can test the logic)
	ctx := context.Background()
	err := bgDeployment.Deploy(ctx, "v2.0.0")

	// We expect this to fail since we don't have real servers
	if err == nil {
		t.Error("Expected deployment to fail due to health check failures")
	}

	// Test rollback
	err = bgDeployment.Rollback(ctx)
	if err == nil {
		t.Error("Expected rollback to fail due to health check failures")
	}
}

func TestCanaryDeployment(t *testing.T) {
	// Create test environments
	prodEnv := Environment{
		Name:    "production",
		URL:     "http://prod.example.com",
		Version: "v1.0.0",
		Status:  EnvironmentStatusHealthy,
		Config: EnvironmentConfig{
			HealthCheckPath:    "/health",
			HealthCheckTimeout: 5 * time.Second,
			ExpectedStatusCode: 200,
			MaxResponseTime:    100 * time.Millisecond,
			TrafficWeight:      1.0,
		},
	}

	canaryEnv := Environment{
		Name:    "canary",
		URL:     "http://canary.example.com",
		Version: "v1.0.0",
		Status:  EnvironmentStatusHealthy,
		Config: EnvironmentConfig{
			HealthCheckPath:    "/health",
			HealthCheckTimeout: 5 * time.Second,
			ExpectedStatusCode: 200,
			MaxResponseTime:    100 * time.Millisecond,
			TrafficWeight:      0.0,
		},
	}

	// Create validator
	validatorConfig := DeploymentConfig{
		HealthCheckInterval: 5 * time.Second,
		MaxRollbackTime:     60 * time.Second,
		TrafficShiftDelay:   10 * time.Second,
		ValidationTimeout:   30 * time.Second,
		RequiredSuccessRate: 0.95,
		MaxErrorRate:        0.05,
	}
	validator := NewDeploymentValidator(validatorConfig)

	// Create traffic splitter
	splitter := &DefaultTrafficSplitter{}

	// Create canary config
	canaryConfig := CanaryConfig{
		InitialTrafficPercent: 5.0,
		MaxTrafficPercent:     50.0,
		TrafficIncrementStep:  5.0,
		StepDuration:          30 * time.Second,
		SuccessThreshold:      0.95,
		ErrorThreshold:        0.05,
		AutoPromote:           false,
		AutoRollback:          true,
	}

	// Create canary deployment
	canaryDeployment := NewCanaryDeployment(&prodEnv, &canaryEnv, splitter, validator, canaryConfig)

	// Test initial state
	if canaryDeployment.trafficPercentage != 0 {
		t.Errorf("Expected initial traffic percentage to be 0, got %f", canaryDeployment.trafficPercentage)
	}

	// Test deployment
	ctx := context.Background()
	err := canaryDeployment.Deploy(ctx, "v2.0.0")

	// We expect this to fail since we don't have real servers
	if err == nil {
		t.Error("Expected canary deployment to fail due to health check failures")
	}

	// Test metrics
	metrics := canaryDeployment.GetMetrics()
	if metrics.StartTime.IsZero() {
		t.Error("Expected metrics start time to be set")
	}

	// Test rollback
	err = canaryDeployment.Rollback(ctx)
	if err != nil {
		t.Errorf("Rollback should not fail: %v", err)
	}

	if canaryDeployment.trafficPercentage != 0 {
		t.Errorf("Expected traffic percentage to be 0 after rollback, got %f", canaryDeployment.trafficPercentage)
	}
}

func TestHTTPHealthCheck(t *testing.T) {
	healthCheck := NewHTTPHealthCheck("test-health", 5*time.Second)

	if healthCheck.Name() != "test-health" {
		t.Errorf("Expected health check name 'test-health', got '%s'", healthCheck.Name())
	}

	if healthCheck.Timeout() != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", healthCheck.Timeout())
	}

	// Test health check against non-existent server
	env := Environment{
		Name: "test",
		URL:  "http://localhost:9999",
		Config: EnvironmentConfig{
			HealthCheckPath:    "/health",
			ExpectedStatusCode: 200,
			MaxResponseTime:    100 * time.Millisecond,
		},
	}

	ctx := context.Background()
	err := healthCheck.Check(ctx, &env)

	if err == nil {
		t.Error("Expected health check to fail against non-existent server")
	}
}

func TestDefaultRollbackStrategy(t *testing.T) {
	strategy := &DefaultRollbackStrategy{}

	if strategy.Name() != "default" {
		t.Errorf("Expected strategy name 'default', got '%s'", strategy.Name())
	}

	// Test should rollback with high error rate
	metrics := EnvironmentMetrics{
		ErrorRate:   0.15, // Above 0.1 threshold
		SuccessRate: 0.85,
	}

	ctx := context.Background()
	shouldRollback := strategy.ShouldRollback(ctx, metrics)
	if !shouldRollback {
		t.Error("Expected rollback with high error rate")
	}

	// Test should rollback with low success rate
	metrics = EnvironmentMetrics{
		ErrorRate:   0.05,
		SuccessRate: 0.85, // Below 0.9 threshold
	}

	shouldRollback = strategy.ShouldRollback(ctx, metrics)
	if !shouldRollback {
		t.Error("Expected rollback with low success rate")
	}

	// Test should not rollback with good metrics
	metrics = EnvironmentMetrics{
		ErrorRate:   0.02,
		SuccessRate: 0.98,
	}

	shouldRollback = strategy.ShouldRollback(ctx, metrics)
	if shouldRollback {
		t.Error("Expected no rollback with good metrics")
	}
}

func TestDefaultDeploymentMonitoring(t *testing.T) {
	monitoring := &DefaultDeploymentMonitoring{}

	env := Environment{
		Name: "test",
		URL:  "http://test.example.com",
	}

	ctx := context.Background()

	// Test start monitoring
	err := monitoring.StartMonitoring(ctx, &env)
	if err != nil {
		t.Errorf("Start monitoring should not fail: %v", err)
	}

	// Test get metrics
	metrics, err := monitoring.GetMetrics(ctx, &env)
	if err != nil {
		t.Errorf("Get metrics should not fail: %v", err)
	}

	if metrics.SuccessRate != 0.99 {
		t.Errorf("Expected success rate 0.99, got %f", metrics.SuccessRate)
	}

	if metrics.ErrorRate != 0.01 {
		t.Errorf("Expected error rate 0.01, got %f", metrics.ErrorRate)
	}

	// Test alert
	err = monitoring.AlertOnIssue(ctx, &env, "test issue")
	if err != nil {
		t.Errorf("Alert should not fail: %v", err)
	}

	// Test stop monitoring
	err = monitoring.StopMonitoring(ctx, &env)
	if err != nil {
		t.Errorf("Stop monitoring should not fail: %v", err)
	}
}

func TestDefaultTrafficSplitter(t *testing.T) {
	splitter := &DefaultTrafficSplitter{}

	env := Environment{
		Name: "test",
		Config: EnvironmentConfig{
			TrafficWeight: 0.5,
		},
	}

	ctx := context.Background()

	// Test get traffic weight
	weight, err := splitter.GetTrafficWeight(ctx, &env)
	if err != nil {
		t.Errorf("Get traffic weight should not fail: %v", err)
	}

	if weight != 0.5 {
		t.Errorf("Expected traffic weight 0.5, got %f", weight)
	}

	// Test set traffic weight (mock implementation doesn't modify struct)
	err = splitter.SetTrafficWeight(ctx, &env, 0.8)
	if err != nil {
		t.Errorf("Set traffic weight should not fail: %v", err)
	}

	// Note: Mock implementation doesn't actually modify the struct
	// In production, this would update the load balancer configuration

	// Test switch traffic (mock implementation)
	env1 := Environment{
		Name: "env1",
		Config: EnvironmentConfig{
			TrafficWeight: 1.0,
		},
	}

	env2 := Environment{
		Name: "env2",
		Config: EnvironmentConfig{
			TrafficWeight: 0.0,
		},
	}

	err = splitter.SwitchTraffic(ctx, &env1, &env2)
	if err != nil {
		t.Errorf("Switch traffic should not fail: %v", err)
	}

	// Note: Mock implementation doesn't actually modify the structs
	// In production, this would update the actual load balancer
}

func TestCanaryMetricsHealthCheck(t *testing.T) {
	canaryConfig := CanaryConfig{
		SuccessThreshold: 0.95,
		ErrorThreshold:   0.05,
	}

	canary := &CanaryDeployment{
		config: canaryConfig,
		metrics: CanaryMetrics{
			CanarySuccessRate:     0.98,
			ProductionSuccessRate: 0.97,
			CanaryErrorRate:       0.02,
			ProductionErrorRate:   0.03,
		},
	}

	// Test healthy canary
	if !canary.isCanaryHealthy() {
		t.Error("Expected canary to be healthy with good metrics")
	}

	// Test unhealthy canary - low success rate
	canary.metrics.CanarySuccessRate = 0.90
	if canary.isCanaryHealthy() {
		t.Error("Expected canary to be unhealthy with low success rate")
	}

	// Reset and test unhealthy canary - high error rate
	canary.metrics.CanarySuccessRate = 0.98
	canary.metrics.CanaryErrorRate = 0.10
	if canary.isCanaryHealthy() {
		t.Error("Expected canary to be unhealthy with high error rate")
	}

	// Reset and test unhealthy canary - worse than production
	canary.metrics.CanaryErrorRate = 0.02
	canary.metrics.CanarySuccessRate = 0.90 // Much worse than production's 0.97
	if canary.isCanaryHealthy() {
		t.Error("Expected canary to be unhealthy when significantly worse than production")
	}
}
