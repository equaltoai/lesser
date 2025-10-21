package deployment

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	// Deployment environment names
	deploymentBlue  = "blue"
	deploymentGreen = "green"
)

// DeploymentValidator validates deployment strategies
type DeploymentValidator struct {
	rollback     RollbackStrategy
	monitoring   DeploymentMonitoring
	environments []*Environment
	healthChecks []HealthCheck
	config       DeploymentConfig
	mutex        sync.RWMutex
}

// Environment represents a deployment environment
type Environment struct {
	LastChecked time.Time
	Name        string
	URL         string
	Version     string
	Status      EnvironmentStatus
	Config      EnvironmentConfig
	Metrics     EnvironmentMetrics
	mutex       sync.RWMutex
}

// EnvironmentStatus represents the status of an environment
type EnvironmentStatus string

const (
	EnvironmentStatusHealthy     EnvironmentStatus = "healthy"
	EnvironmentStatusUnhealthy   EnvironmentStatus = "unhealthy"
	EnvironmentStatusDeploying   EnvironmentStatus = "deploying"
	EnvironmentStatusRollingBack EnvironmentStatus = "rolling_back"
	EnvironmentStatusMaintenance EnvironmentStatus = "maintenance"
)

// EnvironmentConfig holds environment configuration
type EnvironmentConfig struct {
	RequiredHeaders    map[string]string
	HealthCheckPath    string
	HealthCheckTimeout time.Duration
	ExpectedStatusCode int
	MaxResponseTime    time.Duration
	MinSuccessRate     float64
	TrafficWeight      float64
}

// EnvironmentMetrics tracks environment performance
type EnvironmentMetrics struct {
	LastErrorTime     time.Time
	LastError         string
	ResponseTime      time.Duration
	SuccessRate       float64
	ErrorRate         float64
	RequestCount      int64
	Uptime            time.Duration
	CPUUsage          float64
	MemoryUsage       float64
	ActiveConnections int
}

// DeploymentConfig configures deployment validation
type DeploymentConfig struct {
	HealthCheckInterval time.Duration
	MaxRollbackTime     time.Duration
	TrafficShiftDelay   time.Duration
	ValidationTimeout   time.Duration
	RequiredSuccessRate float64
	MaxErrorRate        float64
}

// HealthCheck represents a health check function
type HealthCheck interface {
	Check(ctx context.Context, env *Environment) error
	Name() string
	Timeout() time.Duration
}

// RollbackStrategy defines rollback behavior
type RollbackStrategy interface {
	ShouldRollback(ctx context.Context, metrics EnvironmentMetrics) bool
	Execute(ctx context.Context, env *Environment) error
	Name() string
}

// DeploymentMonitoring monitors deployment progress
type DeploymentMonitoring interface {
	StartMonitoring(ctx context.Context, env *Environment) error
	StopMonitoring(ctx context.Context, env *Environment) error
	GetMetrics(ctx context.Context, env *Environment) (EnvironmentMetrics, error)
	AlertOnIssue(ctx context.Context, env *Environment, issue string) error
}

// TrafficSplitter manages traffic distribution
type TrafficSplitter interface {
	SetTrafficWeight(ctx context.Context, env *Environment, weight float64) error
	GetTrafficWeight(ctx context.Context, env *Environment) (float64, error)
	SwitchTraffic(ctx context.Context, fromEnv, toEnv *Environment) error
}

// NewDeploymentValidator creates a new deployment validator
func NewDeploymentValidator(config DeploymentConfig) *DeploymentValidator {
	return &DeploymentValidator{
		environments: make([]*Environment, 0),
		healthChecks: make([]HealthCheck, 0),
		rollback:     &DefaultRollbackStrategy{},
		monitoring:   &DefaultDeploymentMonitoring{},
		config:       config,
	}
}

// AddEnvironment adds an environment to validate
func (d *DeploymentValidator) AddEnvironment(env *Environment) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.environments = append(d.environments, env)
}

// AddHealthCheck adds a health check
func (d *DeploymentValidator) AddHealthCheck(check HealthCheck) {
	d.healthChecks = append(d.healthChecks, check)
}

// ValidateEnvironment validates a single environment
func (d *DeploymentValidator) ValidateEnvironment(ctx context.Context, envName string) error {
	env, err := d.getEnvironment(envName)
	if err != nil {
		return err
	}

	// Run all health checks
	for _, check := range d.healthChecks {
		checkCtx, cancel := context.WithTimeout(ctx, check.Timeout())
		err := check.Check(checkCtx, env)
		cancel()

		if err != nil {
			return fmt.Errorf("health check %s failed: %w", check.Name(), err)
		}
	}

	// Update environment status
	env.mutex.Lock()
	env.Status = EnvironmentStatusHealthy
	env.LastChecked = time.Now()
	env.mutex.Unlock()

	return nil
}

// ValidateAllEnvironments validates all environments
func (d *DeploymentValidator) ValidateAllEnvironments(ctx context.Context) error {
	var errors []error

	for _, env := range d.environments {
		if err := d.ValidateEnvironment(ctx, env.Name); err != nil {
			errors = append(errors, fmt.Errorf("environment %s: %w", env.Name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("environment validation failures: %v", errors)
	}

	return nil
}

// getEnvironment retrieves an environment by name
func (d *DeploymentValidator) getEnvironment(name string) (*Environment, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	for i := range d.environments {
		if d.environments[i].Name == name {
			return d.environments[i], nil
		}
	}

	return nil, fmt.Errorf("environment %s not found", name)
}

// BlueGreenDeployment manages blue/green deployments
type BlueGreenDeployment struct {
	blueEnvironment  *Environment
	greenEnvironment *Environment
	trafficSplitter  TrafficSplitter
	validator        *DeploymentValidator
	currentActive    string // "blue" or "green"
	mutex            sync.RWMutex
}

// NewBlueGreenDeployment creates a new blue/green deployment
func NewBlueGreenDeployment(blue, green *Environment, splitter TrafficSplitter, validator *DeploymentValidator) *BlueGreenDeployment {
	return &BlueGreenDeployment{
		blueEnvironment:  blue,
		greenEnvironment: green,
		trafficSplitter:  splitter,
		validator:        validator,
		currentActive:    deploymentBlue, // Default to blue as active
	}
}

// Deploy performs a blue/green deployment
func (bg *BlueGreenDeployment) Deploy(ctx context.Context, newVersion string) error {
	bg.mutex.Lock()
	defer bg.mutex.Unlock()

	// Determine target environment (opposite of current active)
	var targetEnv *Environment
	var targetName string

	if bg.currentActive == deploymentBlue {
		targetEnv = bg.greenEnvironment
		targetName = deploymentGreen
	} else {
		targetEnv = bg.blueEnvironment
		targetName = deploymentBlue
	}

	// Deploy to target environment
	targetEnv.mutex.Lock()
	targetEnv.Status = EnvironmentStatusDeploying
	targetEnv.Version = newVersion
	targetEnv.mutex.Unlock()

	// Simulate deployment (in production, this would trigger actual deployment)
	time.Sleep(5 * time.Second)

	// Validate target environment
	if err := bg.validator.ValidateEnvironment(ctx, targetEnv.Name); err != nil {
		return fmt.Errorf("target environment validation failed: %w", err)
	}

	// Switch traffic to target environment
	if err := bg.trafficSplitter.SwitchTraffic(ctx, bg.getActiveEnvironment(), targetEnv); err != nil {
		return fmt.Errorf("traffic switch failed: %w", err)
	}

	// Update active environment
	bg.currentActive = targetName

	// Mark target as healthy
	targetEnv.mutex.Lock()
	targetEnv.Status = EnvironmentStatusHealthy
	targetEnv.mutex.Unlock()

	return nil
}

// Rollback performs a blue/green rollback
func (bg *BlueGreenDeployment) Rollback(ctx context.Context) error {
	bg.mutex.Lock()
	defer bg.mutex.Unlock()

	// Switch back to the other environment
	var rollbackEnv *Environment
	var rollbackName string

	if bg.currentActive == deploymentBlue {
		rollbackEnv = bg.greenEnvironment
		rollbackName = deploymentGreen
	} else {
		rollbackEnv = bg.blueEnvironment
		rollbackName = deploymentBlue
	}

	// Validate rollback environment
	if err := bg.validator.ValidateEnvironment(ctx, rollbackEnv.Name); err != nil {
		return fmt.Errorf("rollback environment validation failed: %w", err)
	}

	// Switch traffic back
	if err := bg.trafficSplitter.SwitchTraffic(ctx, bg.getActiveEnvironment(), rollbackEnv); err != nil {
		return fmt.Errorf("rollback traffic switch failed: %w", err)
	}

	// Update active environment
	bg.currentActive = rollbackName

	return nil
}

// getActiveEnvironment returns the currently active environment
func (bg *BlueGreenDeployment) getActiveEnvironment() *Environment {
	if bg.currentActive == deploymentBlue {
		return bg.blueEnvironment
	}
	return bg.greenEnvironment
}

// CanaryDeployment manages canary deployments
type CanaryDeployment struct {
	trafficSplitter       TrafficSplitter
	productionEnvironment *Environment
	canaryEnvironment     *Environment
	validator             *DeploymentValidator
	metrics               CanaryMetrics
	config                CanaryConfig
	trafficPercentage     float64
	mutex                 sync.RWMutex
}

// CanaryMetrics tracks canary deployment metrics
type CanaryMetrics struct {
	StartTime              time.Time
	CanarySuccessRate      float64
	ProductionSuccessRate  float64
	CanaryErrorRate        float64
	ProductionErrorRate    float64
	CanaryResponseTime     time.Duration
	ProductionResponseTime time.Duration
	TrafficPercentage      float64
	Duration               time.Duration
}

// CanaryConfig configures canary deployments
type CanaryConfig struct {
	InitialTrafficPercent float64
	MaxTrafficPercent     float64
	TrafficIncrementStep  float64
	StepDuration          time.Duration
	SuccessThreshold      float64
	ErrorThreshold        float64
	AutoPromote           bool
	AutoRollback          bool
}

// NewCanaryDeployment creates a new canary deployment
func NewCanaryDeployment(production, canary *Environment, splitter TrafficSplitter, validator *DeploymentValidator, config CanaryConfig) *CanaryDeployment {
	return &CanaryDeployment{
		productionEnvironment: production,
		canaryEnvironment:     canary,
		trafficSplitter:       splitter,
		validator:             validator,
		trafficPercentage:     0,
		config:                config,
		metrics: CanaryMetrics{
			StartTime: time.Now(),
		},
	}
}

// Deploy performs a canary deployment
func (c *CanaryDeployment) Deploy(ctx context.Context, newVersion string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Deploy to canary environment
	c.canaryEnvironment.mutex.Lock()
	c.canaryEnvironment.Status = EnvironmentStatusDeploying
	c.canaryEnvironment.Version = newVersion
	c.canaryEnvironment.mutex.Unlock()

	// Simulate deployment
	time.Sleep(3 * time.Second)

	// Validate canary environment
	if err := c.validator.ValidateEnvironment(ctx, c.canaryEnvironment.Name); err != nil {
		return fmt.Errorf("canary environment validation failed: %w", err)
	}

	// Start with initial traffic percentage
	c.trafficPercentage = c.config.InitialTrafficPercent
	if err := c.trafficSplitter.SetTrafficWeight(ctx, c.canaryEnvironment, c.trafficPercentage/100); err != nil {
		return fmt.Errorf("failed to set initial canary traffic: %w", err)
	}

	// Mark canary as healthy
	c.canaryEnvironment.mutex.Lock()
	c.canaryEnvironment.Status = EnvironmentStatusHealthy
	c.canaryEnvironment.mutex.Unlock()

	return nil
}

// PromoteCanary gradually increases canary traffic
func (c *CanaryDeployment) PromoteCanary(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for c.trafficPercentage < c.config.MaxTrafficPercent {
		// Wait for step duration
		time.Sleep(c.config.StepDuration)

		// Collect metrics
		if err := c.collectMetrics(ctx); err != nil {
			return fmt.Errorf("failed to collect canary metrics: %w", err)
		}

		// Check if canary is performing well
		if !c.isCanaryHealthy() {
			if c.config.AutoRollback {
				return c.Rollback(ctx)
			}
			return fmt.Errorf("canary metrics below threshold, manual intervention required")
		}

		// Increase traffic
		c.trafficPercentage += c.config.TrafficIncrementStep
		if c.trafficPercentage > c.config.MaxTrafficPercent {
			c.trafficPercentage = c.config.MaxTrafficPercent
		}

		if err := c.trafficSplitter.SetTrafficWeight(ctx, c.canaryEnvironment, c.trafficPercentage/100); err != nil {
			return fmt.Errorf("failed to increase canary traffic: %w", err)
		}
	}

	// Auto-promote if configured and metrics are good
	if c.config.AutoPromote && c.isCanaryHealthy() {
		return c.PromoteToProduction(ctx)
	}

	return nil
}

// PromoteToProduction promotes canary to production
func (c *CanaryDeployment) PromoteToProduction(ctx context.Context) error {
	// Switch all traffic to canary
	if err := c.trafficSplitter.SwitchTraffic(ctx, c.productionEnvironment, c.canaryEnvironment); err != nil {
		return fmt.Errorf("failed to promote canary to production: %w", err)
	}

	// Swap environments (canary becomes production)
	c.productionEnvironment, c.canaryEnvironment = c.canaryEnvironment, c.productionEnvironment
	c.trafficPercentage = 100

	return nil
}

// Rollback rolls back the canary deployment
func (c *CanaryDeployment) Rollback(ctx context.Context) error {
	// Set canary traffic to 0
	if err := c.trafficSplitter.SetTrafficWeight(ctx, c.canaryEnvironment, 0); err != nil {
		return fmt.Errorf("failed to rollback canary traffic: %w", err)
	}

	// Mark canary as rolling back
	c.canaryEnvironment.mutex.Lock()
	c.canaryEnvironment.Status = EnvironmentStatusRollingBack
	c.canaryEnvironment.mutex.Unlock()

	c.trafficPercentage = 0

	return nil
}

// collectMetrics collects canary and production metrics
func (c *CanaryDeployment) collectMetrics(ctx context.Context) error {
	// Get canary metrics
	canaryMetrics, err := c.validator.monitoring.GetMetrics(ctx, c.canaryEnvironment)
	if err != nil {
		return fmt.Errorf("failed to get canary metrics: %w", err)
	}

	// Get production metrics
	prodMetrics, err := c.validator.monitoring.GetMetrics(ctx, c.productionEnvironment)
	if err != nil {
		return fmt.Errorf("failed to get production metrics: %w", err)
	}

	// Update canary metrics
	c.metrics.CanarySuccessRate = canaryMetrics.SuccessRate
	c.metrics.CanaryErrorRate = canaryMetrics.ErrorRate
	c.metrics.CanaryResponseTime = canaryMetrics.ResponseTime
	c.metrics.ProductionSuccessRate = prodMetrics.SuccessRate
	c.metrics.ProductionErrorRate = prodMetrics.ErrorRate
	c.metrics.ProductionResponseTime = prodMetrics.ResponseTime
	c.metrics.TrafficPercentage = c.trafficPercentage
	c.metrics.Duration = time.Since(c.metrics.StartTime)

	return nil
}

// isCanaryHealthy checks if canary metrics meet thresholds
func (c *CanaryDeployment) isCanaryHealthy() bool {
	// Check success rate
	if c.metrics.CanarySuccessRate < c.config.SuccessThreshold {
		return false
	}

	// Check error rate
	if c.metrics.CanaryErrorRate > c.config.ErrorThreshold {
		return false
	}

	// Compare with production (canary should not be significantly worse)
	if c.metrics.CanarySuccessRate < c.metrics.ProductionSuccessRate*0.95 {
		return false
	}

	if c.metrics.CanaryErrorRate > c.metrics.ProductionErrorRate*1.5 {
		return false
	}

	return true
}

// GetMetrics returns current canary metrics
func (c *CanaryDeployment) GetMetrics() CanaryMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.metrics
}

// Default implementations

// DefaultRollbackStrategy provides default rollback behavior
type DefaultRollbackStrategy struct{}

func (d *DefaultRollbackStrategy) ShouldRollback(_ context.Context, metrics EnvironmentMetrics) bool {
	return metrics.ErrorRate > 0.1 || metrics.SuccessRate < 0.9
}

func (d *DefaultRollbackStrategy) Execute(_ context.Context, env *Environment) error {
	// Simulate rollback execution
	env.Status = EnvironmentStatusRollingBack
	time.Sleep(2 * time.Second)
	env.Status = EnvironmentStatusHealthy
	return nil
}

func (d *DefaultRollbackStrategy) Name() string {
	return "default"
}

// DefaultDeploymentMonitoring provides default monitoring
type DefaultDeploymentMonitoring struct{}

func (d *DefaultDeploymentMonitoring) StartMonitoring(_ context.Context, _ *Environment) error {
	return nil
}

func (d *DefaultDeploymentMonitoring) StopMonitoring(_ context.Context, _ *Environment) error {
	return nil
}

func (d *DefaultDeploymentMonitoring) GetMetrics(_ context.Context, _ *Environment) (EnvironmentMetrics, error) {
	// Simulate metrics collection
	return EnvironmentMetrics{
		ResponseTime:      50 * time.Millisecond,
		SuccessRate:       0.99,
		ErrorRate:         0.01,
		RequestCount:      1000,
		Uptime:            24 * time.Hour,
		CPUUsage:          0.3,
		MemoryUsage:       0.4,
		ActiveConnections: 50,
	}, nil
}

func (d *DefaultDeploymentMonitoring) AlertOnIssue(_ context.Context, env *Environment, issue string) error {
	// Simulate alerting
	fmt.Printf("ALERT: Environment %s - %s\n", env.Name, issue)
	return nil
}

// DefaultTrafficSplitter provides default traffic splitting
type DefaultTrafficSplitter struct{}

func (d *DefaultTrafficSplitter) SetTrafficWeight(_ context.Context, _ *Environment, _ float64) error {
	// Simulate traffic weight setting
	// Note: In Go, we need to modify the original struct, not a copy
	// This is a limitation of the interface design - in production, this would modify the actual load balancer
	return nil
}

func (d *DefaultTrafficSplitter) GetTrafficWeight(_ context.Context, env *Environment) (float64, error) {
	return env.Config.TrafficWeight, nil
}

func (d *DefaultTrafficSplitter) SwitchTraffic(_ context.Context, _, _ *Environment) error {
	// Simulate traffic switching
	// Note: In production, this would update the actual load balancer configuration
	// The interface design would need pointers to modify the original structs
	return nil
}

// HTTPHealthCheck performs HTTP health checks
type HTTPHealthCheck struct {
	client  *http.Client
	name    string
	timeout time.Duration
}

// NewHTTPHealthCheck creates a new HTTP health check
func NewHTTPHealthCheck(name string, timeout time.Duration) *HTTPHealthCheck {
	return &HTTPHealthCheck{
		name:    name,
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (h *HTTPHealthCheck) Check(ctx context.Context, env *Environment) error {
	url := env.URL + env.Config.HealthCheckPath

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	// Add required headers
	for key, value := range env.Config.RequiredHeaders {
		req.Header.Set(key, value)
	}

	start := time.Now()
	resp, err := h.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	// Check status code
	if resp.StatusCode != env.Config.ExpectedStatusCode {
		return fmt.Errorf("unexpected status code: got %d, expected %d", resp.StatusCode, env.Config.ExpectedStatusCode)
	}

	// Check response time
	if duration > env.Config.MaxResponseTime {
		return fmt.Errorf("response time too slow: %v > %v", duration, env.Config.MaxResponseTime)
	}

	return nil
}

func (h *HTTPHealthCheck) Name() string {
	return h.name
}

func (h *HTTPHealthCheck) Timeout() time.Duration {
	return h.timeout
}
