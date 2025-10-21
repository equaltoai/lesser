package performance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// ConnectionPoolConfig holds configuration for DynamoDB connection pooling
type ConnectionPoolConfig struct {
	Region              string        `json:"region"`
	Endpoint            string        `json:"endpoint,omitempty"`
	MaxConnections      int           `json:"max_connections"`
	MinConnections      int           `json:"min_connections"`
	MaxIdleTime         time.Duration `json:"max_idle_time"`
	ConnectionTimeout   time.Duration `json:"connection_timeout"`
	MaxRetries          int           `json:"max_retries"`
	RetryDelay          time.Duration `json:"retry_delay"`
	BackoffMultiplier   float64       `json:"backoff_multiplier"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	HealthCheckTimeout  time.Duration `json:"health_check_timeout"`
	EnableMetrics       bool          `json:"enable_metrics"`
}

// DefaultConnectionPoolConfig returns a default configuration optimized for DynamORM
func DefaultConnectionPoolConfig() *ConnectionPoolConfig {
	return &ConnectionPoolConfig{
		MaxConnections:      50,
		MinConnections:      5,
		MaxIdleTime:         10 * time.Minute,
		ConnectionTimeout:   30 * time.Second,
		MaxRetries:          3,
		RetryDelay:          100 * time.Millisecond,
		BackoffMultiplier:   2.0,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  5 * time.Second,
		Region:              "us-east-1",
		EnableMetrics:       true,
	}
}

// ProductionConnectionPoolConfig returns configuration optimized for production workloads
func ProductionConnectionPoolConfig() *ConnectionPoolConfig {
	config := DefaultConnectionPoolConfig()
	config.MaxConnections = 100
	config.MinConnections = 10
	config.MaxIdleTime = 5 * time.Minute
	config.ConnectionTimeout = 10 * time.Second
	config.HealthCheckInterval = 15 * time.Second
	return config
}

// HighThroughputConnectionPoolConfig returns configuration for high-throughput scenarios
func HighThroughputConnectionPoolConfig() *ConnectionPoolConfig {
	config := DefaultConnectionPoolConfig()
	config.MaxConnections = 200
	config.MinConnections = 20
	config.MaxIdleTime = 2 * time.Minute
	config.ConnectionTimeout = 5 * time.Second
	config.MaxRetries = 5
	config.HealthCheckInterval = 10 * time.Second
	return config
}

// LowLatencyConnectionPoolConfig returns configuration optimized for low latency
func LowLatencyConnectionPoolConfig() *ConnectionPoolConfig {
	config := DefaultConnectionPoolConfig()
	config.MaxConnections = 150
	config.MinConnections = 25
	config.MaxIdleTime = 1 * time.Minute
	config.ConnectionTimeout = 2 * time.Second
	config.RetryDelay = 50 * time.Millisecond
	config.HealthCheckInterval = 5 * time.Second
	return config
}

// ConnectionPool manages a pool of DynamoDB connections
type ConnectionPool struct {
	state *poolState
}

type poolState struct {
	mu               *sync.RWMutex
	resources        *poolResources
	metrics          *PoolMetrics
	totalConnections int
	closed           bool
}

type poolResources struct {
	ctx       context.Context
	config    *ConnectionPoolConfig
	clients   chan *dynamodb.Client
	cancel    context.CancelFunc
	awsConfig *aws.Config
}

// PoolMetrics tracks connection pool performance
type PoolMetrics struct {
	LastHealthCheck      time.Time     `json:"last_health_check"`
	ActiveConnections    int64         `json:"active_connections"`
	IdleConnections      int64         `json:"idle_connections"`
	TotalRequests        int64         `json:"total_requests"`
	FailedRequests       int64         `json:"failed_requests"`
	AverageWaitTime      time.Duration `json:"average_wait_time"`
	ConnectionsCreated   int64         `json:"connections_created"`
	ConnectionsDestroyed int64         `json:"connections_destroyed"`
	HealthChecksPassed   int64         `json:"health_checks_passed"`
	HealthChecksFailed   int64         `json:"health_checks_failed"`
	mu                   sync.RWMutex
}

// NewConnectionPool creates a new connection pool with the given configuration
func NewConnectionPool(ctx context.Context, cfg *ConnectionPoolConfig) (*ConnectionPool, error) {
	if cfg == nil {
		cfg = DefaultConnectionPoolConfig()
	}

	// Load AWS configuration
	awsConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithRetryMaxAttempts(cfg.MaxRetries),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Override endpoint if specified
	if cfg.Endpoint != "" {
		awsConfig.BaseEndpoint = aws.String(cfg.Endpoint)
	}

	poolCtx, cancel := context.WithCancel(ctx)

	awsCfg := awsConfig
	pool := &ConnectionPool{
		state: &poolState{
			mu: &sync.RWMutex{},
			resources: &poolResources{
				ctx:       poolCtx,
				config:    cfg,
				clients:   make(chan *dynamodb.Client, cfg.MaxConnections),
				cancel:    cancel,
				awsConfig: &awsCfg,
			},
			metrics: &PoolMetrics{},
		},
	}

	// Create minimum number of connections
	for i := 0; i < cfg.MinConnections; i++ {
		client := pool.newClient()
		pool.state.resources.clients <- client
		pool.state.metrics.ConnectionsCreated++
		pool.state.metrics.IdleConnections++
		pool.state.totalConnections++
	}

	// Start health check routine
	if cfg.HealthCheckInterval > 0 {
		go pool.healthCheckRoutine()
	}

	return pool, nil
}

// GetClient retrieves a client from the pool
func (p *ConnectionPool) GetClient(ctx context.Context) (*dynamodb.Client, error) {
	if p.state.closed {
		return nil, fmt.Errorf("connection pool is closed")
	}

	start := time.Now()
	defer func() {
		waitTime := time.Since(start)
		p.state.metrics.mu.Lock()
		p.state.metrics.TotalRequests++
		p.state.metrics.AverageWaitTime = (p.state.metrics.AverageWaitTime + waitTime) / 2
		p.state.metrics.mu.Unlock()
	}()

	// Try to get an existing client
	select {
	case client := <-p.state.resources.clients:
		p.state.metrics.mu.Lock()
		p.state.metrics.ActiveConnections++
		p.state.metrics.IdleConnections--
		p.state.metrics.mu.Unlock()
		return client, nil
	case <-time.After(p.state.resources.config.ConnectionTimeout):
		p.state.metrics.mu.Lock()
		p.state.metrics.FailedRequests++
		p.state.metrics.mu.Unlock()
		return nil, fmt.Errorf("connection timeout after %v", p.state.resources.config.ConnectionTimeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		// Create new client if pool not at max capacity
		if p.reserveConnectionSlot() {
			client := p.newClient()
			p.state.metrics.mu.Lock()
			p.state.metrics.ConnectionsCreated++
			p.state.metrics.ActiveConnections++
			p.state.metrics.mu.Unlock()
			return client, nil
		}

		// Wait for available client
		select {
		case client := <-p.state.resources.clients:
			p.state.metrics.mu.Lock()
			p.state.metrics.ActiveConnections++
			p.state.metrics.IdleConnections--
			p.state.metrics.mu.Unlock()
			return client, nil
		case <-time.After(p.state.resources.config.ConnectionTimeout):
			p.state.metrics.mu.Lock()
			p.state.metrics.FailedRequests++
			p.state.metrics.mu.Unlock()
			return nil, fmt.Errorf("connection timeout after %v", p.state.resources.config.ConnectionTimeout)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// ReturnClient returns a client to the pool
func (p *ConnectionPool) ReturnClient(client *dynamodb.Client) {
	if p.state.closed || client == nil {
		return
	}

	// Return client to pool if there's space
	select {
	case p.state.resources.clients <- client:
		p.state.metrics.mu.Lock()
		p.state.metrics.ActiveConnections--
		p.state.metrics.IdleConnections++
		p.state.metrics.mu.Unlock()
	default:
		// Pool is full, client will be garbage collected
		p.state.metrics.mu.Lock()
		p.state.metrics.ConnectionsDestroyed++
		p.state.metrics.ActiveConnections--
		p.state.metrics.mu.Unlock()
		p.releaseConnectionSlot()
	}
}

// Close closes the connection pool and all clients
func (p *ConnectionPool) Close() error {
	p.state.mu.Lock()
	if p.state.closed {
		p.state.mu.Unlock()
		return nil
	}

	p.state.closed = true
	if p.state.resources.cancel != nil {
		p.state.resources.cancel()
	}

	close(p.state.resources.clients)
	p.state.mu.Unlock()

	for client := range p.state.resources.clients {
		// DynamoDB clients don't need explicit closing
		_ = client
		p.state.metrics.mu.Lock()
		p.state.metrics.ConnectionsDestroyed++
		p.state.metrics.mu.Unlock()
		p.releaseConnectionSlot()
	}

	p.state.mu.Lock()
	p.state.totalConnections = 0
	p.state.mu.Unlock()

	return nil
}

// GetMetrics returns current pool metrics
func (p *ConnectionPool) GetMetrics() *PoolMetrics {
	p.state.metrics.mu.RLock()
	defer p.state.metrics.mu.RUnlock()

	// Return a copy of metrics without the mutex
	return &PoolMetrics{
		ActiveConnections:    p.state.metrics.ActiveConnections,
		IdleConnections:      p.state.metrics.IdleConnections,
		TotalRequests:        p.state.metrics.TotalRequests,
		FailedRequests:       p.state.metrics.FailedRequests,
		AverageWaitTime:      p.state.metrics.AverageWaitTime,
		ConnectionsCreated:   p.state.metrics.ConnectionsCreated,
		ConnectionsDestroyed: p.state.metrics.ConnectionsDestroyed,
		HealthChecksPassed:   p.state.metrics.HealthChecksPassed,
		HealthChecksFailed:   p.state.metrics.HealthChecksFailed,
		LastHealthCheck:      p.state.metrics.LastHealthCheck,
	}
}

// newClient creates a new DynamoDB client
func (p *ConnectionPool) newClient() *dynamodb.Client {
	if p.state == nil || p.state.resources.awsConfig == nil {
		return nil
	}

	return dynamodb.NewFromConfig(*p.state.resources.awsConfig)
}

// healthCheckRoutine performs periodic health checks on pool connections
func (p *ConnectionPool) healthCheckRoutine() {
	ticker := time.NewTicker(p.state.resources.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performHealthCheck()
		case <-p.state.resources.ctx.Done():
			return
		}
	}
}

// performHealthCheck checks the health of connections in the pool
func (p *ConnectionPool) performHealthCheck() {
	ctx, cancel := context.WithTimeout(p.state.resources.ctx, p.state.resources.config.HealthCheckTimeout)
	defer cancel()

	// Sample a few connections for health check
	healthyCount := 0
	totalChecked := 0
	maxCheck := 5 // Check up to 5 connections

healthLoop:
	for i := 0; i < maxCheck && len(p.state.resources.clients) > 0; i++ {
		select {
		case client := <-p.state.resources.clients:
			totalChecked++

			// Perform a simple operation to check health
			_, err := client.DescribeEndpoints(ctx, &dynamodb.DescribeEndpointsInput{})
			if err == nil {
				healthyCount++
				p.state.resources.clients <- client // Return healthy client
			} else {
				// Create a new client to replace the unhealthy one
				newClient := p.newClient()
				p.state.resources.clients <- newClient
				p.state.metrics.mu.Lock()
				p.state.metrics.ConnectionsDestroyed++
				p.state.metrics.ConnectionsCreated++
				p.state.metrics.mu.Unlock()
			}
		default:
			break healthLoop
		}
	}

	// Update health check metrics
	p.state.metrics.mu.Lock()
	p.state.metrics.HealthChecksPassed += int64(healthyCount)
	p.state.metrics.HealthChecksFailed += int64(totalChecked - healthyCount)
	p.state.metrics.LastHealthCheck = time.Now()
	p.state.metrics.mu.Unlock()
}

// PoolStats returns formatted statistics about the pool
func (p *ConnectionPool) PoolStats() map[string]interface{} {
	metrics := p.GetMetrics()

	return map[string]interface{}{
		"active_connections":    metrics.ActiveConnections,
		"idle_connections":      metrics.IdleConnections,
		"total_requests":        metrics.TotalRequests,
		"failed_requests":       metrics.FailedRequests,
		"success_rate":          calculateSuccessRate(metrics.TotalRequests, metrics.FailedRequests),
		"average_wait_time_ms":  metrics.AverageWaitTime.Milliseconds(),
		"connections_created":   metrics.ConnectionsCreated,
		"connections_destroyed": metrics.ConnectionsDestroyed,
		"health_checks_passed":  metrics.HealthChecksPassed,
		"health_checks_failed":  metrics.HealthChecksFailed,
		"last_health_check":     metrics.LastHealthCheck,
		"pool_utilization":      p.calculateUtilization(),
	}
}

func calculateSuccessRate(totalRequests, failedRequests int64) float64 {
	if totalRequests == 0 {
		return 0
	}

	return float64(totalRequests-failedRequests) / float64(totalRequests) * 100
}

func (p *ConnectionPool) calculateUtilization() float64 {
	maxConnections := p.state.resources.config.MaxConnections
	if maxConnections <= 0 {
		return 0
	}

	total := p.currentTotalConnections()
	if total == 0 {
		return 0
	}

	return float64(total) / float64(maxConnections) * 100
}

func (p *ConnectionPool) currentTotalConnections() int {
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	return p.state.totalConnections
}

func (p *ConnectionPool) reserveConnectionSlot() bool {
	p.state.mu.Lock()
	defer p.state.mu.Unlock()

	if p.state.closed {
		return false
	}

	if p.state.resources.config.MaxConnections > 0 && p.state.totalConnections >= p.state.resources.config.MaxConnections {
		return false
	}

	p.state.totalConnections++
	return true
}

func (p *ConnectionPool) releaseConnectionSlot() {
	p.state.mu.Lock()
	if p.state.totalConnections > 0 {
		p.state.totalConnections--
	}
	p.state.mu.Unlock()
}

// OptimizeForWorkload adjusts pool configuration based on workload characteristics
func (p *ConnectionPool) OptimizeForWorkload(workloadType string) error {
	p.state.mu.Lock()
	defer p.state.mu.Unlock()

	var newConfig *ConnectionPoolConfig

	switch workloadType {
	case "high-throughput":
		newConfig = HighThroughputConnectionPoolConfig()
	case "low-latency":
		newConfig = LowLatencyConnectionPoolConfig()
	case "production":
		newConfig = ProductionConnectionPoolConfig()
	default:
		return fmt.Errorf("unknown workload type: %s", workloadType)
	}

	// Apply new configuration (simplified - in practice would need gradual transition)
	newConfig.Region = p.state.resources.config.Region
	newConfig.Endpoint = p.state.resources.config.Endpoint
	p.state.resources.config = newConfig

	return nil
}
