package routing

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"go.uber.org/zap"
)

// Manager implements the RouteManager interface
type Manager struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger
	config    *RoutingConfig

	// Components
	registry       *InstanceRegistry
	optimizer      *SmartRouteOptimizer
	circuitBreaker *DistributedCircuitBreaker
	healthChecker  *InstanceHealthChecker
	loadBalancer   *AdaptiveLoadBalancer

	// Route cache
	routeCache sync.Map // domain -> []*Route
	cacheTTL   time.Duration

	// Metrics
	metrics *RoutingMetrics
}

// NewManager creates a new route manager
func NewManager(db *dynamodb.Client, tableName string, logger *zap.Logger, config *RoutingConfig) *Manager {
	if config == nil {
		config = defaultRoutingConfig()
	}

	// Create components
	registry := NewInstanceRegistry(db, tableName, logger)

	optimizerConfig := &OptimizerConfig{
		LatencyWeight:        0.4,
		ReliabilityWeight:    0.4,
		CostWeight:           0.2,
		MaxAcceptableLatency: config.DefaultTimeout,
		MinAcceptableSuccess: 0.95,
		HistoryWindow:        24 * time.Hour,
		MinSamplesRequired:   10,
	}
	optimizer := NewSmartRouteOptimizer(db, tableName, logger, optimizerConfig)

	cbConfig := &CircuitBreakerConfig{
		FailureThreshold:  config.CircuitBreakerThreshold,
		SuccessThreshold:  3,
		OpenTimeout:       config.CircuitBreakerTimeout,
		HalfOpenTimeout:   10 * time.Second,
		BackoffMultiplier: 2.0,
		MaxBackoff:        5 * time.Minute,
	}
	circuitBreaker := NewDistributedCircuitBreaker(db, tableName, logger, cbConfig)

	healthChecker := NewHealthChecker(db, tableName, logger, config)
	loadBalancer := NewAdaptiveLoadBalancer(logger)
	metrics := NewRoutingMetrics(db, tableName, logger)

	return &Manager{
		db:             db,
		tableName:      tableName,
		logger:         logger,
		config:         config,
		registry:       registry,
		optimizer:      optimizer,
		circuitBreaker: circuitBreaker,
		healthChecker:  healthChecker,
		loadBalancer:   loadBalancer,
		metrics:        metrics,
		cacheTTL:       1 * time.Minute,
	}
}

// SelectRoute selects the best route for a destination
func (m *Manager) SelectRoute(destination string, messageType MessageType) (*Route, error) {
	// Get all routes for destination
	routes, err := m.GetRoutes(destination)
	if err != nil {
		return nil, fmt.Errorf("get routes: %w", err)
	}

	if len(routes) == 0 {
		return nil, ErrNoHealthyRoutes
	}

	// Filter by message type support
	supportedRoutes := m.filterByMessageType(routes, messageType)
	if len(supportedRoutes) == 0 {
		return nil, fmt.Errorf("no routes support message type: %s", messageType)
	}

	// Filter by circuit breaker status
	healthyRoutes := m.filterHealthyRoutes(supportedRoutes)
	if len(healthyRoutes) == 0 {
		// Try half-open circuits as last resort
		for _, route := range supportedRoutes {
			if m.circuitBreaker.GetStatus(route.InstanceID) == CircuitHalfOpen {
				m.logger.Warn("using half-open circuit",
					zap.String("routeID", route.ID),
					zap.String("destination", destination))
				return route, nil
			}
		}
		return nil, ErrNoHealthyRoutes
	}

	// Optimize route selection
	optimized, err := m.optimizer.OptimizeRoutes(context.Background(), healthyRoutes, 1024) // Assume 1KB message
	if err != nil {
		m.logger.Warn("route optimization failed, using fallback",
			zap.Error(err))
		// Fallback to first healthy route
		return healthyRoutes[0], nil
	}

	// Select best route
	bestRoute := optimized[0]

	// Update metrics
	m.metrics.RecordRouteSelection(bestRoute.ID, destination, messageType)

	m.logger.Debug("route selected",
		zap.String("routeID", bestRoute.ID),
		zap.String("destination", destination),
		zap.String("messageType", string(messageType)),
		zap.Int("priority", bestRoute.Priority))

	return bestRoute, nil
}

// GetRoutes retrieves all routes for a destination
func (m *Manager) GetRoutes(destination string) ([]*Route, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("routes:%s", destination)
	if cached, ok := m.routeCache.Load(cacheKey); ok {
		if cr, ok := cached.(*cachedRoutes); ok && time.Since(cr.cachedAt) < m.cacheTTL {
			return cr.routes, nil
		}
	}

	// Get instance for domain
	instances, err := m.getInstancesForDomain(destination)
	if err != nil {
		return nil, fmt.Errorf("get instances: %w", err)
	}

	// Build routes from instances
	routes := make([]*Route, 0, len(instances))
	for _, instance := range instances {
		// Create route from instance
		route, err := m.createRouteFromInstance(instance)
		if err != nil {
			m.logger.Warn("failed to create route",
				zap.String("instanceID", instance.ID),
				zap.Error(err))
			continue
		}

		// Get performance metrics
		metrics, err := m.optimizer.GetRouteMetrics(context.Background(), route.ID)
		if err == nil && metrics.TotalMessages > 0 {
			route.Latency = metrics.AvgLatency
			route.SuccessRate = float64(metrics.SuccessfulCount) / float64(metrics.TotalMessages)
			route.CostPerByte = metrics.TotalCost / float64(metrics.TotalBytes)
		}

		// Get circuit status
		route.CircuitStatus = m.circuitBreaker.GetStatus(instance.ID)

		routes = append(routes, route)
	}

	// Sort by priority
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Priority < routes[j].Priority
	})

	// Cache routes
	m.routeCache.Store(cacheKey, &cachedRoutes{
		routes:   routes,
		cachedAt: time.Now(),
	})

	return routes, nil
}

// RegisterInstance registers a new federated instance
func (m *Manager) RegisterInstance(instance *Instance) error {
	if err := m.registry.RegisterInstance(context.Background(), instance); err != nil {
		return fmt.Errorf("register instance: %w", err)
	}

	// Start health monitoring
	if err := m.healthChecker.StartMonitoring(instance); err != nil {
		m.logger.Warn("failed to start health monitoring",
			zap.String("instanceID", instance.ID),
			zap.Error(err))
	}

	// Initialize circuit breaker
	if err := m.circuitBreaker.Close(instance.ID); err != nil {
		m.logger.Error("Failed to close circuit breaker",
			zap.String("instanceID", instance.ID),
			zap.Error(err))
	}

	// Clear route cache for the domain
	m.clearRouteCache(instance.Domain)

	m.logger.Info("instance registered",
		zap.String("instanceID", instance.ID),
		zap.String("domain", instance.Domain),
		zap.String("tier", string(instance.TierLevel)))

	return nil
}

// UpdateInstanceHealth updates instance health metrics
func (m *Manager) UpdateInstanceHealth(instanceID string, health *HealthStatus) error {
	// Update registry
	if err := m.registry.UpdateInstanceHealth(context.Background(), instanceID, health); err != nil {
		return fmt.Errorf("update health: %w", err)
	}

	// Update circuit breaker based on health
	if !health.Reachable || health.ErrorRate > 0.5 {
		if err := m.circuitBreaker.RecordFailure(instanceID, fmt.Errorf("unhealthy: reachable=%v, errorRate=%.2f",
			health.Reachable, health.ErrorRate)); err != nil {
			m.logger.Error("failed to record circuit breaker failure", zap.Error(err))
		}
	} else {
		if err := m.circuitBreaker.RecordSuccess(instanceID); err != nil {
			m.logger.Error("failed to record circuit breaker success", zap.Error(err))
		}
	}

	// Clear relevant caches
	instance, err := m.registry.GetInstance(context.Background(), instanceID)
	if err == nil {
		m.clearRouteCache(instance.Domain)
	}

	return nil
}

// GetInstance retrieves instance information
func (m *Manager) GetInstance(instanceID string) (*Instance, error) {
	return m.registry.GetInstance(context.Background(), instanceID)
}

// ListHealthyInstances lists all healthy instances
func (m *Manager) ListHealthyInstances() ([]*Instance, error) {
	return m.registry.ListHealthyInstances(context.Background())
}

// OptimizeRoutes triggers route optimization
func (m *Manager) OptimizeRoutes() error {
	m.logger.Info("starting route optimization")

	// Get all active routes
	instances, err := m.ListHealthyInstances()
	if err != nil {
		return fmt.Errorf("list instances: %w", err)
	}

	optimizedCount := 0
	for _, instance := range instances {
		routes, err := m.GetRoutes(instance.Domain)
		if err != nil {
			continue
		}

		// Optimize routes for this domain
		optimized, err := m.optimizer.OptimizeRoutes(context.Background(), routes, 1024)
		if err != nil {
			m.logger.Warn("optimization failed for domain",
				zap.String("domain", instance.Domain),
				zap.Error(err))
			continue
		}

		// Update priorities
		for i, route := range optimized {
			route.Priority = i + 1
		}

		optimizedCount++
	}

	m.logger.Info("route optimization completed",
		zap.Int("domainsOptimized", optimizedCount))

	return nil
}

// GetRouteMetrics retrieves metrics for a destination
func (m *Manager) GetRouteMetrics(destination string) (*RouteMetrics, error) {
	routes, err := m.GetRoutes(destination)
	if err != nil {
		return nil, err
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("no routes for destination: %s", destination)
	}

	// Aggregate metrics from all routes
	aggregated := &RouteMetrics{
		LastUpdated: time.Now(),
	}

	for _, route := range routes {
		metrics, err := m.optimizer.GetRouteMetrics(context.Background(), route.ID)
		if err != nil {
			continue
		}

		aggregated.TotalMessages += metrics.TotalMessages
		aggregated.SuccessfulCount += metrics.SuccessfulCount
		aggregated.FailedCount += metrics.FailedCount
		aggregated.RetryCount += metrics.RetryCount
		aggregated.TotalBytes += metrics.TotalBytes
		aggregated.TotalCost += metrics.TotalCost

		// Use worst-case latencies
		if metrics.P95Latency > aggregated.P95Latency {
			aggregated.P95Latency = metrics.P95Latency
		}
		if metrics.P99Latency > aggregated.P99Latency {
			aggregated.P99Latency = metrics.P99Latency
		}
	}

	// Calculate average latency
	if aggregated.SuccessfulCount > 0 {
		// This is simplified - in reality would need weighted average
		aggregated.AvgLatency = aggregated.P95Latency / 2
	}

	return aggregated, nil
}

// Circuit breaker methods

// OpenCircuit opens the circuit for an instance
func (m *Manager) OpenCircuit(instanceID string, reason string) error {
	if err := m.circuitBreaker.Open(instanceID, reason); err != nil {
		return err
	}

	// Clear route cache
	instance, err := m.GetInstance(instanceID)
	if err == nil {
		m.clearRouteCache(instance.Domain)
	}

	return nil
}

// CloseCircuit closes the circuit for an instance
func (m *Manager) CloseCircuit(instanceID string) error {
	if err := m.circuitBreaker.Close(instanceID); err != nil {
		return err
	}

	// Clear route cache
	instance, err := m.GetInstance(instanceID)
	if err == nil {
		m.clearRouteCache(instance.Domain)
	}

	return nil
}

// GetCircuitStatus returns the circuit status for an instance
func (m *Manager) GetCircuitStatus(instanceID string) CircuitStatus {
	return m.circuitBreaker.GetStatus(instanceID)
}

// DeliverMessage delivers a federation message using optimal routing
func (m *Manager) DeliverMessage(ctx context.Context, message *FederationMessage, options DeliveryOptions) (*DeliveryResult, error) {
	startTime := time.Now()

	// Select routes for all targets
	routeMap := make(map[string]*Route)
	for _, target := range message.Target {
		route, err := m.SelectRoute(target, message.Type)
		if err != nil {
			m.logger.Warn("no route for target",
				zap.String("target", target),
				zap.Error(err))
			continue
		}
		routeMap[target] = route
	}

	if len(routeMap) == 0 {
		return nil, fmt.Errorf("no routes available for any target")
	}

	// Group targets by route for batch delivery
	routeTargets := make(map[string][]string)
	for target, route := range routeMap {
		routeTargets[route.ID] = append(routeTargets[route.ID], target)
	}

	// Deliver to each route
	var wg sync.WaitGroup
	results := make([]*DeliveryResult, 0, len(routeTargets))
	resultsMu := sync.Mutex{}

	for routeID, targets := range routeTargets {
		wg.Add(1)
		go func(rid string, tgts []string) {
			defer wg.Done()

			route := routeMap[tgts[0]] // Get route from first target
			result := m.deliverToRoute(ctx, route, message, tgts, options)

			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()

			// Record result for learning
			if err := m.optimizer.RecordDeliveryResult(ctx, result); err != nil {
				m.logger.Error("Failed to record delivery result",
					zap.String("instanceID", route.InstanceID),
					zap.Error(err))
			}

			// Update circuit breaker
			if result.Success {
				if err := m.circuitBreaker.RecordSuccess(route.InstanceID); err != nil {
					m.logger.Error("failed to record circuit breaker success", zap.Error(err))
				}
			} else {
				if err := m.circuitBreaker.RecordFailure(route.InstanceID, fmt.Errorf(result.ErrorMessage)); err != nil {
					m.logger.Error("failed to record circuit breaker failure", zap.Error(err))
				}
			}
		}(routeID, targets)
	}

	wg.Wait()

	// Aggregate results
	aggregated := &DeliveryResult{
		MessageID: message.ID,
		Success:   true,
		Duration:  time.Since(startTime),
		Timestamp: time.Now(),
	}

	for _, result := range results {
		if !result.Success {
			aggregated.Success = false
			aggregated.ErrorMessage = result.ErrorMessage
		}
		aggregated.BytesSent += result.BytesSent
		aggregated.Cost += result.Cost
		aggregated.Attempts += result.Attempts
	}

	// Update metrics
	m.metrics.RecordDelivery(aggregated)

	return aggregated, nil
}

// Helper methods

func (m *Manager) filterByMessageType(routes []*Route, messageType MessageType) []*Route {
	filtered := make([]*Route, 0, len(routes))

	for _, route := range routes {
		// Get instance to check supported types
		instance, err := m.GetInstance(route.InstanceID)
		if err != nil {
			continue
		}

		// Check if message type is supported
		supported := false
		for _, t := range instance.SupportedTypes {
			if t == messageType {
				supported = true
				break
			}
		}

		if supported || len(instance.SupportedTypes) == 0 { // Empty means all types
			filtered = append(filtered, route)
		}
	}

	return filtered
}

func (m *Manager) filterHealthyRoutes(routes []*Route) []*Route {
	healthy := make([]*Route, 0, len(routes))

	for _, route := range routes {
		// Check circuit breaker
		if !m.circuitBreaker.CanAttempt(route.InstanceID) {
			continue
		}

		// Check instance status
		instance, err := m.GetInstance(route.InstanceID)
		if err != nil || instance.Status != InstanceStatusActive {
			continue
		}

		// Check quota
		if instance.TierLevel != TierBlocked && instance.CurrentUsage < instance.MonthlyQuota {
			healthy = append(healthy, route)
		}
	}

	return healthy
}

func (m *Manager) getInstancesForDomain(domain string) ([]*Instance, error) {
	// For federation, typically one instance per domain
	// But could have multiple for redundancy

	ctx := context.Background()

	// Query by domain using scan (in production, add GSI for domain lookups)
	instances := []*Instance{}

	// Try exact match first
	instance, err := m.registry.GetInstance(ctx, domain)
	if err == nil {
		instances = append(instances, instance)
	}

	// Could also query for backup instances, CDN endpoints, etc.

	return instances, nil
}

func (m *Manager) createRouteFromInstance(instance *Instance) (*Route, error) {
	endpoint, err := url.Parse(instance.SharedInboxURL)
	if err != nil {
		endpoint, err = url.Parse(instance.InboxURL)
		if err != nil {
			return nil, fmt.Errorf("invalid inbox URLs")
		}
	}

	route := &Route{
		ID:         fmt.Sprintf("%s-primary", instance.ID),
		InstanceID: instance.ID,
		Domain:     instance.Domain,
		Endpoint:   endpoint,
		Priority:   1,

		// Default values - will be updated with real metrics
		Latency:     200 * time.Millisecond,
		Bandwidth:   1000000, // 1 MB/s
		SuccessRate: 0.99,

		CostPerMessage: 0.001,
		CostPerByte:    0.0000001,
	}

	return route, nil
}

func (m *Manager) deliverToRoute(ctx context.Context, route *Route, message *FederationMessage, _ []string, _ DeliveryOptions) *DeliveryResult {
	// This is where actual HTTP delivery would happen
	// For now, return a simulated result

	result := &DeliveryResult{
		MessageID:  message.ID,
		InstanceID: route.InstanceID,
		RouteID:    route.ID,
		Success:    true,
		StatusCode: 200,
		Attempts:   1,
		Duration:   route.Latency,
		BytesSent:  message.PayloadSize,
		Cost:       float64(message.PayloadSize) * route.CostPerByte,
		Timestamp:  time.Now(),
	}

	// Simulate failures based on success rate
	if time.Now().UnixNano()%100 > int64(route.SuccessRate*100) {
		result.Success = false
		result.StatusCode = 500
		result.ErrorMessage = "simulated failure"
	}

	return result
}

func (m *Manager) clearRouteCache(domain string) {
	cacheKey := fmt.Sprintf("routes:%s", domain)
	m.routeCache.Delete(cacheKey)
}

type cachedRoutes struct {
	routes   []*Route
	cachedAt time.Time
}

func defaultRoutingConfig() *RoutingConfig {
	return &RoutingConfig{
		HealthCheckInterval:     1 * time.Minute,
		HealthCheckTimeout:      5 * time.Second,
		UnhealthyThreshold:      3,
		HealthyThreshold:        2,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,
		HalfOpenMaxAttempts:     3,
		MaxRoutesPerInstance:    10,
		RouteSelectionAlgorithm: "weighted_random",
		EnableLoadBalancing:     true,
		EnableCostOptimization:  true,
		DefaultTimeout:          10 * time.Second,
		MaxRetries:              3,
		RetryBackoff:            1 * time.Second,
		MaxQueueDepth:           10000,
		EnableCompression:       true,
		BatchDeliverySize:       100,
		ParallelDeliveries:      10,
	}
}
