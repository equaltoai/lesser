package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift/health"
	"github.com/pay-theory/lift/pkg/observability"
)

// ServiceRegistry manages service registration and discovery
type ServiceRegistry struct {
	services      map[string]*ServiceConfig
	discovery     ServiceDiscovery
	loadBalancer  LoadBalancer
	healthChecker health.HealthManager
	metrics       observability.MetricsCollector
	cache         ServiceCache
	mu            sync.RWMutex
	config        RegistryConfig
}

// RegistryConfig configures the service registry
type RegistryConfig struct {
	CacheTTL            time.Duration `json:"cache_ttl"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	MaxRetries          int           `json:"max_retries"`
	RetryBackoff        time.Duration `json:"retry_backoff"`
	EnableCaching       bool          `json:"enable_caching"`
	EnableMetrics       bool          `json:"enable_metrics"`
	TenantIsolation     bool          `json:"tenant_isolation"`
}

// ServiceConfig represents a service configuration
type ServiceConfig struct {
	Created     time.Time         `json:"created"`
	LastSeen    time.Time         `json:"last_seen"`
	Metadata    map[string]string `json:"metadata"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	TenantID    string            `json:"tenant_id,omitempty"`
	Region      string            `json:"region"`
	Environment string            `json:"environment"`
	Endpoints   []ServiceEndpoint `json:"endpoints"`
	Tags        []string          `json:"tags"`
	HealthCheck HealthCheckConfig `json:"health_check"`
	Weight      int               `json:"weight"`
}

// ServiceEndpoint represents a service endpoint
type ServiceEndpoint struct {
	Metadata map[string]string `json:"metadata"`
	Protocol string            `json:"protocol"`
	Host     string            `json:"host"`
	Path     string            `json:"path"`
	Port     int               `json:"port"`
}

// ServiceInstance represents a discovered service instance
type ServiceInstance struct {
	LastSeen    time.Time         `json:"last_seen"`
	Metadata    map[string]string `json:"metadata"`
	Health      HealthStatus      `json:"health"`
	ID          string            `json:"id"`
	ServiceName string            `json:"service_name"`
	Version     string            `json:"version"`
	TenantID    string            `json:"tenant_id,omitempty"`
	Endpoint    ServiceEndpoint   `json:"endpoint"`
	Weight      int               `json:"weight"`
}

// HealthStatus represents the health status of a service
type HealthStatus struct {
	Timestamp time.Time              `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks"`
	Status    string                 `json:"status"`
	Message   string                 `json:"message"`
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Latency int64  `json:"latency_ms"`
}

// HealthCheckConfig configures health checking for a service
type HealthCheckConfig struct {
	Path             string        `json:"path"`
	Interval         time.Duration `json:"interval"`
	Timeout          time.Duration `json:"timeout"`
	Retries          int           `json:"retries"`
	FailureThreshold int           `json:"failure_threshold"`
	SuccessThreshold int           `json:"success_threshold"`
	Enabled          bool          `json:"enabled"`
}

// DiscoveryOptions configures service discovery behavior
type DiscoveryOptions struct {
	TenantID         string              `json:"tenant_id,omitempty"`
	Strategy         LoadBalanceStrategy `json:"strategy"`
	Version          string              `json:"version,omitempty"`
	Region           string              `json:"region,omitempty"`
	Tags             []string            `json:"tags"`
	MaxInstances     int                 `json:"max_instances"`
	IncludeUnhealthy bool                `json:"include_unhealthy"`
	PreferLocal      bool                `json:"prefer_local"`
}

// LoadBalanceStrategy defines load balancing strategies
type LoadBalanceStrategy string

const (
	RoundRobin       LoadBalanceStrategy = "round_robin"
	WeightedRandom   LoadBalanceStrategy = "weighted_random"
	LeastConnections LoadBalanceStrategy = "least_connections"
	HealthyFirst     LoadBalanceStrategy = "healthy_first"
	LocalFirst       LoadBalanceStrategy = "local_first"
)

// ServiceDiscovery defines the interface for service discovery backends
type ServiceDiscovery interface {
	Register(ctx context.Context, config *ServiceConfig) error
	Deregister(ctx context.Context, serviceID string) error
	Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error)
	Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error)
	HealthCheck(ctx context.Context, instance *ServiceInstance) (*HealthStatus, error)
}

// LoadBalancer defines the interface for load balancing
type LoadBalancer interface {
	Select(instances []*ServiceInstance, strategy LoadBalanceStrategy) *ServiceInstance
	UpdateWeights(instances []*ServiceInstance) error
	GetStats() LoadBalancerStats
}

// CircuitBreaker defines the interface for circuit breaking
type CircuitBreaker interface {
	Execute(fn func() (any, error)) (any, error)
	GetState() CircuitBreakerState
	GetStats() CircuitBreakerStats
}

// ServiceCache defines the interface for service discovery caching
type ServiceCache interface {
	Get(key string) ([]*ServiceInstance, bool)
	Set(key string, instances []*ServiceInstance, ttl time.Duration)
	Delete(key string)
	Clear()
	Stats() CacheStats
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry(config RegistryConfig, discovery ServiceDiscovery, loadBalancer LoadBalancer) *ServiceRegistry {
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute
	}

	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	if config.RetryBackoff == 0 {
		config.RetryBackoff = 1 * time.Second
	}

	registry := &ServiceRegistry{
		services:     make(map[string]*ServiceConfig),
		discovery:    discovery,
		loadBalancer: loadBalancer,
		config:       config,
	}

	if config.EnableCaching {
		registry.cache = NewMemoryServiceCache()
	}

	return registry
}

// Register registers a service with the registry
func (r *ServiceRegistry) Register(ctx context.Context, config *ServiceConfig) error {
	start := time.Now()

	// Validate service configuration
	if err := r.validateConfig(config); err != nil {
		return fmt.Errorf("invalid service config: %w", err)
	}

	// Set timestamps
	config.Created = time.Now()
	config.LastSeen = time.Now()

	// Register with discovery backend
	if err := r.discovery.Register(ctx, config); err != nil {
		if r.config.EnableMetrics && r.metrics != nil {
			r.metrics.Counter("service.registration.failures", map[string]string{
				"service": config.Name,
				"tenant":  config.TenantID,
			}).Inc()
		}
		return fmt.Errorf("failed to register service: %w", err)
	}

	// Store in local registry
	r.mu.Lock()
	r.services[config.Name] = config
	r.mu.Unlock()

	// Start health monitoring if enabled
	if config.HealthCheck.Enabled && r.healthChecker != nil {
		r.startHealthMonitoring(config)
	}

	// Record metrics
	if r.config.EnableMetrics && r.metrics != nil {
		r.metrics.Counter("service.registrations", map[string]string{
			"service": config.Name,
			"tenant":  config.TenantID,
		}).Inc()

		r.metrics.Histogram("service.registration.duration", map[string]string{
			"service": config.Name,
		}).Observe(float64(time.Since(start).Milliseconds()))
	}

	return nil
}

// Discover discovers service instances
func (r *ServiceRegistry) Discover(ctx context.Context, serviceName string, opts DiscoveryOptions) (*ServiceInstance, error) {
	start := time.Now()

	// Check cache first
	cacheKey := r.generateCacheKey(serviceName, opts)
	if r.config.EnableCaching && r.cache != nil {
		if cached, found := r.cache.Get(cacheKey); found {
			if r.config.EnableMetrics && r.metrics != nil {
				r.metrics.Counter("service.discovery.cache_hits", map[string]string{
					"service": serviceName,
				}).Inc()
			}

			// Apply load balancing to cached results
			selected := r.loadBalancer.Select(cached, opts.Strategy)
			return selected, nil
		}
	}

	// Discover from backend
	instances, err := r.discovery.Discover(ctx, serviceName)
	if err != nil {
		if r.config.EnableMetrics && r.metrics != nil {
			r.metrics.Counter("service.discovery.failures", map[string]string{
				"service": serviceName,
			}).Inc()
		}
		return nil, fmt.Errorf("service discovery failed: %w", err)
	}

	// Filter instances
	filtered := r.filterInstances(instances, opts)
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no suitable instances found for service %s", serviceName)
	}

	// Cache results
	if r.config.EnableCaching && r.cache != nil {
		r.cache.Set(cacheKey, filtered, r.config.CacheTTL)
	}

	// Load balance selection
	selected := r.loadBalancer.Select(filtered, opts.Strategy)

	// Record metrics
	if r.config.EnableMetrics && r.metrics != nil {
		r.metrics.Counter("service.discoveries", map[string]string{
			"service": serviceName,
			"tenant":  opts.TenantID,
		}).Inc()

		r.metrics.Histogram("service.discovery.duration", map[string]string{
			"service": serviceName,
		}).Observe(float64(time.Since(start).Milliseconds()))

		r.metrics.Gauge("service.instances.available", map[string]string{
			"service": serviceName,
		}).Set(float64(len(filtered)))
	}

	return selected, nil
}

// DiscoverAll discovers all instances of a service
func (r *ServiceRegistry) DiscoverAll(ctx context.Context, serviceName string, opts DiscoveryOptions) ([]*ServiceInstance, error) {
	instances, err := r.discovery.Discover(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("service discovery failed: %w", err)
	}

	// Filter instances
	filtered := r.filterInstances(instances, opts)

	return filtered, nil
}

// Deregister removes a service from the registry
func (r *ServiceRegistry) Deregister(ctx context.Context, serviceID string) error {
	// Remove from discovery backend
	if err := r.discovery.Deregister(ctx, serviceID); err != nil {
		return fmt.Errorf("failed to deregister service: %w", err)
	}

	// Remove from local registry
	r.mu.Lock()
	delete(r.services, serviceID)
	r.mu.Unlock()

	// Clear cache
	if r.config.EnableCaching && r.cache != nil {
		r.cache.Clear()
	}

	// Record metrics
	if r.config.EnableMetrics && r.metrics != nil {
		r.metrics.Counter("service.deregistrations", map[string]string{
			"service": serviceID,
		}).Inc()
	}

	return nil
}

// Watch watches for changes to a service
func (r *ServiceRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	return r.discovery.Watch(ctx, serviceName)
}

// GetService returns a service configuration
func (r *ServiceRegistry) GetService(serviceName string) (*ServiceConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.services[serviceName]
	return config, exists
}

// ListServices returns all registered services
func (r *ServiceRegistry) ListServices() []*ServiceConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	services := make([]*ServiceConfig, 0, len(r.services))
	for _, config := range r.services {
		services = append(services, config)
	}

	return services
}

// validateConfig validates a service configuration
func (r *ServiceRegistry) validateConfig(config *ServiceConfig) error {
	if config.Name == "" {
		return fmt.Errorf("service name is required")
	}

	if config.Version == "" {
		return fmt.Errorf("service version is required")
	}

	if len(config.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint is required")
	}

	// Validate endpoints
	for i, endpoint := range config.Endpoints {
		if endpoint.Host == "" {
			return fmt.Errorf("endpoint %d: host is required", i)
		}

		if endpoint.Port <= 0 || endpoint.Port > 65535 {
			return fmt.Errorf("endpoint %d: invalid port %d", i, endpoint.Port)
		}
	}

	// Validate tenant isolation
	if r.config.TenantIsolation && config.TenantID == "" {
		return fmt.Errorf("tenant ID is required when tenant isolation is enabled")
	}

	return nil
}

// filterInstances filters service instances based on discovery options
func (r *ServiceRegistry) filterInstances(instances []*ServiceInstance, opts DiscoveryOptions) []*ServiceInstance {
	filtered := make([]*ServiceInstance, 0, len(instances))

	for _, instance := range instances {
		// Filter by tenant
		if r.config.TenantIsolation && opts.TenantID != "" {
			if instance.TenantID != opts.TenantID {
				continue
			}
		}

		// Filter by health status
		if !opts.IncludeUnhealthy && instance.Health.Status != "healthy" {
			continue
		}

		// Filter by version
		if opts.Version != "" && instance.Version != opts.Version {
			continue
		}

		// Filter by tags
		if len(opts.Tags) > 0 {
			if !r.hasAllTags(instance, opts.Tags) {
				continue
			}
		}

		filtered = append(filtered, instance)

		// Limit results
		if opts.MaxInstances > 0 && len(filtered) >= opts.MaxInstances {
			break
		}
	}

	return filtered
}

// hasAllTags checks if an instance has all required tags
func (r *ServiceRegistry) hasAllTags(instance *ServiceInstance, requiredTags []string) bool {
	instanceTags := make(map[string]bool)
	if tagsStr, exists := instance.Metadata["tags"]; exists {
		// Parse tags from metadata (simplified)
		for _, tag := range []string{tagsStr} {
			instanceTags[tag] = true
		}
	}

	for _, tag := range requiredTags {
		if !instanceTags[tag] {
			return false
		}
	}

	return true
}

// generateCacheKey generates a cache key for discovery options
func (r *ServiceRegistry) generateCacheKey(serviceName string, opts DiscoveryOptions) string {
	return fmt.Sprintf("%s:%s:%s:%s", serviceName, opts.TenantID, opts.Version, opts.Strategy)
}

// startHealthMonitoring starts health monitoring for a service
func (r *ServiceRegistry) startHealthMonitoring(_ *ServiceConfig) {
	// This would integrate with the existing health monitoring system
	// For now, this is a placeholder
}

// GetStats returns registry statistics
func (r *ServiceRegistry) GetStats() RegistryStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := RegistryStats{
		RegisteredServices: len(r.services),
		Timestamp:          time.Now(),
	}

	if r.cache != nil {
		stats.CacheStats = r.cache.Stats()
	}

	if r.loadBalancer != nil {
		stats.LoadBalancerStats = r.loadBalancer.GetStats()
	}

	return stats
}

// RegistryStats provides registry performance metrics
type RegistryStats struct {
	Timestamp          time.Time         `json:"timestamp"`
	CacheStats         CacheStats        `json:"cache_stats"`
	LoadBalancerStats  LoadBalancerStats `json:"load_balancer_stats"`
	RegisteredServices int               `json:"registered_services"`
}

// LoadBalancerStats provides load balancer metrics
type LoadBalancerStats struct {
	TotalRequests        int64 `json:"total_requests"`
	SuccessfulSelections int64 `json:"successful_selections"`
	FailedSelections     int64 `json:"failed_selections"`
	AverageLatency       int64 `json:"average_latency_ns"`
}

// CircuitBreakerState represents circuit breaker states
type CircuitBreakerState string

const (
	CircuitBreakerClosed   CircuitBreakerState = "closed"
	CircuitBreakerOpen     CircuitBreakerState = "open"
	CircuitBreakerHalfOpen CircuitBreakerState = "half_open"
)

// CircuitBreakerStats provides circuit breaker metrics
type CircuitBreakerStats struct {
	LastStateChange    time.Time           `json:"last_state_change"`
	State              CircuitBreakerState `json:"state"`
	TotalRequests      int64               `json:"total_requests"`
	SuccessfulRequests int64               `json:"successful_requests"`
	FailedRequests     int64               `json:"failed_requests"`
}

// CacheStats provides cache performance metrics (reusing from features package)
type CacheStats struct {
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	Sets       int64   `json:"sets"`
	Deletes    int64   `json:"deletes"`
	Errors     int64   `json:"errors"`
	HitRate    float64 `json:"hit_rate"`
	AvgLatency int64   `json:"avg_latency_ns"`
	Size       int64   `json:"size"`
	Memory     int64   `json:"memory_bytes"`
}
