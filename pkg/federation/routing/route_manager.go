package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// Manager implements the RouteManager interface
type Manager struct {
	logger *zap.Logger
	config *types.RoutingConfig

	// Repository dependencies
	instanceRepo       FederationInstanceRepository
	instanceHealthRepo interface{} // repositories.InstanceHealthRepository
	healthRepo         instanceHealthReader
	circuitBreakerRepo *repositories.CircuitBreakerRepository
	routeOptimRepo     *repositories.RouteOptimizerRepository
	routingMetricsRepo *repositories.RoutingMetricsRepository
	costTrackingRepo   federationCostRecorder

	// Components
	registry         *InstanceRegistry
	optimizer        routeOptimizerEngine
	circuitBreaker   routeCircuitBreakerEngine
	healthChecker    HealthChecker
	loadBalancer     *AdaptiveLoadBalancer
	thresholdManager *RouteThresholdManager

	// HTTP client for federation delivery
	httpClient httpDoer

	// Federation storage for actor keys
	federationStore federation.FederationStorage

	// Route cache with adaptive TTL
	routeCache    sync.Map // domain -> *cachedRoutes
	cacheTTL      time.Duration
	emergencyMode bool
	emergencyMu   sync.RWMutex

	// Metrics
	metrics *RoutingMetrics
}

// ManagerConfig holds configuration for the route manager
type ManagerConfig struct {
	RoutingConfig        *types.RoutingConfig
	OptimizerConfig      *OptimizerConfig
	CircuitBreakerConfig *models.CircuitBreakerConfig
	CacheTTL             time.Duration
	FederationStore      federation.FederationStorage
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type federationCostRecorder interface {
	RecordFederationCost(ctx context.Context, tracker *models.FederationCostTracking) error
}

type instanceHealthReader interface {
	GetLatestHealthCheck(ctx context.Context, domain string) (*models.InstanceHealth, error)
	GetUnhealthyInstances(ctx context.Context, threshold float64) ([]string, error)
}

type noopHealthChecker struct{}

func (noopHealthChecker) CheckHealth(_ *types.Instance) (*types.HealthStatus, error) {
	return &types.HealthStatus{
		Timestamp:    time.Now(),
		Reachable:    false,
		ErrorMessage: "health checker not configured",
	}, nil
}

func (noopHealthChecker) StartMonitoring(_ *types.Instance) error { return nil }
func (noopHealthChecker) StopMonitoring(_ string) error           { return nil }
func (noopHealthChecker) GetHealthHistory(_ string, _ time.Duration) ([]*types.HealthStatus, error) {
	return []*types.HealthStatus{}, nil
}

type noopRouteOptimizer struct{}

func (noopRouteOptimizer) OptimizeRoutes(_ context.Context, routes []*types.Route, _ int64) ([]*types.Route, error) {
	return routes, nil
}

func (noopRouteOptimizer) GetRouteMetrics(_ context.Context, _ string) (*types.RouteMetrics, error) {
	return nil, errors.New("route metrics not available")
}

func (noopRouteOptimizer) RecordDeliveryResult(_ context.Context, _ *types.DeliveryResult) error {
	return nil
}

type noopCircuitBreaker struct{}

func (noopCircuitBreaker) AssessRouteHealthAndAdjustCircuit(_ context.Context, _ string, _ *types.RouteMetrics) error {
	return nil
}

func (noopCircuitBreaker) CanAttempt(_ string) bool { return true }
func (noopCircuitBreaker) Close(_ string) error     { return nil }

func (noopCircuitBreaker) GetBackpressureRules() map[MessagePriority]BackpressureRule {
	return make(map[MessagePriority]BackpressureRule)
}

func (noopCircuitBreaker) GetStatus(_ string) types.CircuitStatus { return types.CircuitClosed }
func (noopCircuitBreaker) Open(_, _ string) error                 { return nil }
func (noopCircuitBreaker) RecordFailure(_ string, _ error) error  { return nil }
func (noopCircuitBreaker) RecordSuccess(_ string) error           { return nil }
func (noopCircuitBreaker) ShouldEnterEmergencyMode(_, _ int) bool { return false }

type routeOptimizerEngine interface {
	OptimizeRoutes(ctx context.Context, routes []*types.Route, messageSize int64) ([]*types.Route, error)
	GetRouteMetrics(ctx context.Context, routeID string) (*types.RouteMetrics, error)
	RecordDeliveryResult(ctx context.Context, result *types.DeliveryResult) error
}

type routeCircuitBreakerEngine interface {
	AssessRouteHealthAndAdjustCircuit(ctx context.Context, routeID string, metrics *types.RouteMetrics) error
	CanAttempt(instanceID string) bool
	Close(instanceID string) error
	GetBackpressureRules() map[MessagePriority]BackpressureRule
	GetStatus(instanceID string) types.CircuitStatus
	Open(instanceID string, reason string) error
	RecordFailure(instanceID string, err error) error
	RecordSuccess(instanceID string) error
	ShouldEnterEmergencyMode(healthyRoutes, totalRoutes int) bool
}

// NewManager creates a new route manager with dependency injection
func NewManager(
	instanceRepo FederationInstanceRepository,
	instanceHealthRepo interface{}, // repositories.InstanceHealthRepository,
	circuitBreakerRepo *repositories.CircuitBreakerRepository,
	routeOptimRepo *repositories.RouteOptimizerRepository,
	routingMetricsRepo *repositories.RoutingMetricsRepository,
	costTrackingRepo *repositories.FederationCostRepository,
	logger *zap.Logger,
	config *ManagerConfig,
) *Manager {
	if config == nil {
		config = &ManagerConfig{
			RoutingConfig: defaultRoutingConfig(),
			CacheTTL:      1 * time.Minute,
		}
	}
	if config.RoutingConfig == nil {
		config.RoutingConfig = defaultRoutingConfig()
	}
	if config.OptimizerConfig == nil {
		config.OptimizerConfig = &OptimizerConfig{
			LatencyWeight:        0.4,
			ReliabilityWeight:    0.4,
			CostWeight:           0.2,
			MaxAcceptableLatency: config.RoutingConfig.DefaultTimeout,
			MinAcceptableSuccess: 0.95,
			HistoryWindow:        24 * time.Hour,
			MinSamplesRequired:   10,
		}
	}
	if config.CircuitBreakerConfig == nil {
		config.CircuitBreakerConfig = &models.CircuitBreakerConfig{
			FailureThreshold:  config.RoutingConfig.CircuitBreakerThreshold,
			SuccessThreshold:  3,
			OpenTimeout:       config.RoutingConfig.CircuitBreakerTimeout,
			HalfOpenTimeout:   10 * time.Second,
			BackoffMultiplier: 2.0,
			MaxBackoff:        5 * time.Minute,
		}
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 1 * time.Minute
	}

	// Create components with injected repositories
	registry := NewInstanceRegistry(instanceRepo, logger)

	// Create threshold manager with guidance document defaults
	thresholdManager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	// Create SmartRouteOptimizer with repository
	var optimizer routeOptimizerEngine = noopRouteOptimizer{}
	if routeOptimRepo != nil {
		optimizer = NewSmartRouteOptimizer(routeOptimRepo, logger, config.OptimizerConfig)
		logger.Info("route optimization repository configured successfully")
	} else {
		logger.Warn("route optimization repository not provided - optimization disabled")
	}

	// Create circuit breaker with threshold manager
	var circuitBreaker routeCircuitBreakerEngine = noopCircuitBreaker{}
	if circuitBreakerRepo != nil {
		circuitBreaker = NewDistributedCircuitBreaker(circuitBreakerRepo, thresholdManager, logger, config.CircuitBreakerConfig)
		logger.Info("circuit breaker configured successfully")
	} else {
		logger.Warn("circuit breaker repository not provided - circuit breaker disabled")
	}

	// Create health checker with repository-backed storage and circuit breaker integration
	var healthRepo instanceHealthReader
	var healthChecker HealthChecker = noopHealthChecker{}
	if instanceHealthRepo != nil {
		repo, ok := instanceHealthRepo.(*repositories.InstanceHealthRepository)
		if ok {
			healthRepo = repo
			healthChecker = NewHealthChecker(repo, logger, config.RoutingConfig)
			logger.Info("health checker configured successfully with repository")
		} else {
			logger.Warn("invalid instance health repository type - health checker disabled")
		}
	} else {
		logger.Warn("instance health repository not provided - health checker disabled")
	}
	loadBalancer := NewAdaptiveLoadBalancer(logger)

	// Initialize HTTP client for federation delivery
	httpClient := httpclient.NewSecureClient(
		httpclient.WithTimeout(config.RoutingConfig.DefaultTimeout),
		httpclient.WithLogger(logger),
		httpclient.WithMaxRedirects(3),
	)

	// Create routing metrics
	metrics := NewRoutingMetrics(nil, "", logger)

	var costRecorder federationCostRecorder
	if costTrackingRepo != nil {
		costRecorder = costTrackingRepo
	}

	return &Manager{
		logger:             logger,
		config:             config.RoutingConfig,
		instanceRepo:       instanceRepo,
		instanceHealthRepo: instanceHealthRepo,
		healthRepo:         healthRepo,
		circuitBreakerRepo: circuitBreakerRepo,
		routeOptimRepo:     routeOptimRepo,
		routingMetricsRepo: routingMetricsRepo,
		costTrackingRepo:   costRecorder,
		registry:           registry,
		optimizer:          optimizer,
		circuitBreaker:     circuitBreaker,
		healthChecker:      healthChecker,
		loadBalancer:       loadBalancer,
		thresholdManager:   thresholdManager,
		httpClient:         httpClient,
		federationStore:    config.FederationStore,
		metrics:            metrics,
		cacheTTL:           config.CacheTTL,
	}
}

// SelectRoute selects the best route for a destination with emergency mode handling
func (m *Manager) SelectRoute(destination string, messageType types.MessageType) (*types.Route, error) {
	// Check emergency mode first
	m.emergencyMu.RLock()
	inEmergencyMode := m.emergencyMode
	m.emergencyMu.RUnlock()

	if inEmergencyMode {
		return m.selectRouteInEmergencyMode(destination, messageType)
	}
	// Get all routes for destination
	routes, err := m.GetRoutes(destination)
	if err != nil {
		return nil, errors.Join(ErrGetRoutesFailed, err)
	}

	if err := common.ValidateSliceNotEmpty("routes", routes); err != nil {
		return nil, types.ErrNoHealthyRoutes
	}

	// Filter by message type support
	supportedRoutes := m.filterByMessageType(routes, messageType)
	if err := common.ValidateSliceNotEmpty("supportedRoutes", supportedRoutes); err != nil {
		m.logger.Error("no routes support message type", zap.String("messageType", string(messageType)))
		return nil, ErrNoMessageTypeSupport
	}

	// Filter by circuit breaker status
	healthyRoutes := m.filterHealthyRoutes(supportedRoutes)

	// Check for emergency mode conditions
	if m.shouldEnterEmergencyMode(len(healthyRoutes), len(supportedRoutes)) {
		m.enterEmergencyMode()
		return m.selectRouteInEmergencyMode(destination, messageType)
	}

	if err := common.ValidateSliceNotEmpty("healthyRoutes", healthyRoutes); err != nil {
		// Try half-open circuits as last resort
		for _, route := range supportedRoutes {
			if m.circuitBreaker != nil && m.circuitBreaker.GetStatus(route.InstanceID) == types.CircuitHalfOpen {
				m.logger.Warn("using half-open circuit",
					zap.String("routeID", route.ID),
					zap.String("destination", destination))
				return route, nil
			}
		}
		return nil, types.ErrNoHealthyRoutes
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

// GetRoutes retrieves all routes for a destination with adaptive caching
func (m *Manager) GetRoutes(destination string) ([]*types.Route, error) {
	// Try to get from cache first
	if routes := m.getRoutesFromCache(destination); routes != nil {
		return routes, nil
	}

	// Build new routes
	routes, err := m.buildRoutesForDestination(destination)
	if err != nil {
		return nil, err
	}

	// Cache the routes
	m.cacheRoutes(destination, routes)

	return routes, nil
}

// getRoutesFromCache attempts to retrieve routes from cache
func (m *Manager) getRoutesFromCache(destination string) []*types.Route {
	cacheKey := fmt.Sprintf("routes:%s", destination)
	cached, ok := m.routeCache.Load(cacheKey)
	if !ok {
		return nil
	}

	cr, ok := cached.(*cachedRoutes)
	if !ok {
		return nil
	}

	// Use adaptive TTL based on route health
	adaptiveTTL := m.getAdaptiveCacheTTL(cr.routes)
	if time.Since(cr.cachedAt) >= adaptiveTTL {
		return nil
	}

	return cr.routes
}

// buildRoutesForDestination builds routes for a given destination
func (m *Manager) buildRoutesForDestination(destination string) ([]*types.Route, error) {
	// Get instances for domain
	instances, err := m.getInstancesForDomain(destination)
	if err != nil {
		return nil, errors.Join(ErrGetInstancesFailed, err)
	}

	// Build routes from instances
	routes := make([]*types.Route, 0, len(instances))
	for _, instance := range instances {
		route := m.buildRouteFromInstance(instance)
		if route != nil {
			routes = append(routes, route)
		}
	}

	// Sort by priority
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Priority < routes[j].Priority
	})

	return routes, nil
}

// buildRouteFromInstance creates a single route from an instance
func (m *Manager) buildRouteFromInstance(instance *types.Instance) *types.Route {
	// Create route from instance
	route, err := m.createRouteFromInstance(instance)
	if err != nil {
		m.logger.Warn("failed to create route",
			zap.String("instanceID", instance.ID),
			zap.Error(err))
		return nil
	}

	// Enhance route with metrics
	m.enhanceRouteWithMetrics(route)

	// Get circuit status
	route.CircuitStatus = m.circuitBreaker.GetStatus(instance.ID)

	return route
}

// enhanceRouteWithMetrics adds performance metrics to a route
func (m *Manager) enhanceRouteWithMetrics(route *types.Route) {
	if m.optimizer == nil {
		return
	}

	metrics, err := m.optimizer.GetRouteMetrics(context.Background(), route.ID)
	if err != nil || metrics.TotalMessages == 0 {
		return
	}

	// Update route metrics
	route.Latency = metrics.AvgLatency
	route.SuccessRate = float64(metrics.SuccessfulCount) / float64(metrics.TotalMessages)
	route.CostPerByte = metrics.TotalCost / float64(metrics.TotalBytes)

	// Assess route health if circuit breaker is available
	if m.circuitBreaker != nil {
		if assessErr := m.circuitBreaker.AssessRouteHealthAndAdjustCircuit(context.Background(), route.ID, metrics); assessErr != nil {
			m.logger.Warn("failed to assess route health",
				zap.String("routeID", route.ID),
				zap.Error(assessErr))
		}
	}
}

// cacheRoutes stores routes in cache with health information
func (m *Manager) cacheRoutes(destination string, routes []*types.Route) {
	cacheKey := fmt.Sprintf("routes:%s", destination)
	cachedData := &cachedRoutes{
		routes:       routes,
		cachedAt:     time.Now(),
		healthStatus: make(map[string]RouteHealthStatus),
	}

	// Store health status for adaptive caching
	m.populateHealthStatus(cachedData, routes)

	m.routeCache.Store(cacheKey, cachedData)
}

// populateHealthStatus fills health status for each route
func (m *Manager) populateHealthStatus(cachedData *cachedRoutes, routes []*types.Route) {
	if m.thresholdManager == nil || m.optimizer == nil {
		return
	}

	for _, route := range routes {
		metrics, err := m.optimizer.GetRouteMetrics(context.Background(), route.ID)
		if err == nil {
			assessment := m.thresholdManager.AssessRouteHealth(context.Background(), route.ID, metrics)
			cachedData.healthStatus[route.ID] = assessment.Status
		}
	}
}

// RegisterInstance registers a new federated instance
func (m *Manager) RegisterInstance(instance *types.Instance) error {
	if err := m.registry.RegisterInstance(context.Background(), instance); err != nil {
		return errors.Join(ErrRegisterInstanceFailed, err)
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
func (m *Manager) UpdateInstanceHealth(instanceID string, health *types.HealthStatus) error {
	// Update registry
	if err := m.registry.UpdateInstanceHealth(context.Background(), instanceID, health); err != nil {
		return errors.Join(ErrUpdateHealthFailed, err)
	}

	// Update circuit breaker based on health
	if !health.Reachable || health.ErrorRate > 0.5 {
		m.logger.Warn("instance unhealthy - recording circuit breaker failure",
			zap.String("instanceID", instanceID),
			zap.Bool("reachable", health.Reachable),
			zap.Float64("errorRate", health.ErrorRate))
		if err := m.circuitBreaker.RecordFailure(instanceID, ErrInstanceUnhealthy); err != nil {
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
func (m *Manager) GetInstance(instanceID string) (*types.Instance, error) {
	return m.registry.GetInstance(context.Background(), instanceID)
}

// ListHealthyInstances lists all healthy instances
func (m *Manager) ListHealthyInstances() ([]*types.Instance, error) {
	return m.registry.ListHealthyInstances(context.Background())
}

// OptimizeRoutes triggers route optimization
func (m *Manager) OptimizeRoutes() error {
	m.logger.Info("starting route optimization")

	// Get all active routes
	instances, err := m.ListHealthyInstances()
	if err != nil {
		return errors.Join(ErrListInstancesFailed, err)
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
func (m *Manager) GetRouteMetrics(destination string) (*types.RouteMetrics, error) {
	routes, err := m.GetRoutes(destination)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateSliceNotEmpty("routes", routes); err != nil {
		m.logger.Warn("no routes available for destination", zap.String("destination", destination))
		return nil, ErrNoRoutesForDestination
	}

	// Aggregate metrics from all routes
	aggregated := &types.RouteMetrics{
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

// Health checker integration methods

// PerformHealthCheck performs a comprehensive health check on an instance
func (m *Manager) PerformHealthCheck(instanceID string) (*types.HealthStatus, error) {
	// Get instance information
	instance, err := m.GetInstance(instanceID)
	if err != nil {
		return nil, errors.Join(ErrGetInstanceFailed, err)
	}

	// Check if circuit breaker allows health check
	if m.circuitBreaker != nil && !m.circuitBreaker.CanAttempt(instanceID) {
		return &types.HealthStatus{
			Timestamp:    time.Now(),
			Reachable:    false,
			ErrorMessage: "circuit breaker open - health check skipped",
		}, nil
	}

	// Perform the actual health check
	health, err := m.healthChecker.CheckHealth(instance)
	if err != nil {
		// Record failure in circuit breaker
		if m.circuitBreaker != nil {
			if cbErr := m.circuitBreaker.RecordFailure(instanceID, err); cbErr != nil {
				m.logger.Error("failed to record circuit breaker failure", zap.Error(cbErr))
			}
		}
		return health, err
	}

	// Update circuit breaker based on health result
	if m.circuitBreaker != nil {
		if health.Reachable && health.StatusCode < 500 {
			if cbErr := m.circuitBreaker.RecordSuccess(instanceID); cbErr != nil {
				m.logger.Error("failed to record circuit breaker success", zap.Error(cbErr))
			}
		} else {
			m.logger.Warn("health check failed - recording circuit breaker failure",
				zap.String("instanceID", instanceID),
				zap.Bool("reachable", health.Reachable),
				zap.Int("statusCode", health.StatusCode))
			cbErr := m.circuitBreaker.RecordFailure(instanceID, ErrHealthCheckFailed)
			if cbErr != nil {
				m.logger.Error("failed to record circuit breaker failure", zap.Error(cbErr))
			}
		}
	}

	// Clear route cache if health changed significantly
	m.clearRouteCache(instance.Domain)

	return health, nil
}

// MonitorInstanceHealth performs continuous health monitoring for all instances
func (m *Manager) MonitorInstanceHealth() error {
	instances, err := m.ListHealthyInstances()
	if err != nil {
		return errors.Join(ErrListInstancesFailed, err)
	}

	if err := common.ValidateSliceNotEmpty("instances", instances); err != nil {
		m.logger.Info("No instances to monitor")
		return nil
	}

	// Perform health checks in parallel with limited concurrency
	semaphore := make(chan struct{}, 10) // Max 10 concurrent health checks
	var wg sync.WaitGroup

	for _, instance := range instances {
		wg.Add(1)
		go func(inst *types.Instance) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			health, err := m.PerformHealthCheck(inst.ID)
			if err != nil {
				m.logger.Error("Health check failed",
					zap.String("instanceID", inst.ID),
					zap.String("domain", inst.Domain),
					zap.Error(err))
				return
			}

			// Update instance health
			if updateErr := m.UpdateInstanceHealth(inst.ID, health); updateErr != nil {
				m.logger.Error("Failed to update instance health",
					zap.String("instanceID", inst.ID),
					zap.Error(updateErr))
			}
		}(instance)
	}

	wg.Wait()

	m.logger.Info("Health monitoring completed",
		zap.Int("instances_checked", len(instances)))

	return nil
}

// DetectUnhealthyInstances identifies instances that should be removed from rotation
func (m *Manager) DetectUnhealthyInstances() ([]string, error) {
	if m.healthRepo == nil {
		return nil, ErrHealthRepositoryNotAvailable
	}

	// Get unhealthy instances with 40% health score threshold
	unhealthy, err := m.healthRepo.GetUnhealthyInstances(context.Background(), 40.0)
	if err != nil {
		return nil, errors.Join(ErrGetUnhealthyInstancesFailed, err)
	}

	// Cross-reference with circuit breaker status
	filteredUnhealthy := make([]string, 0)
	for _, instanceID := range unhealthy {
		// Check if circuit breaker confirms unhealthy state
		if m.circuitBreaker != nil {
			status := m.circuitBreaker.GetStatus(instanceID)
			if status == types.CircuitOpen || status == types.CircuitHalfOpen {
				filteredUnhealthy = append(filteredUnhealthy, instanceID)
			}
		} else {
			// No circuit breaker, trust health repository
			filteredUnhealthy = append(filteredUnhealthy, instanceID)
		}
	}

	m.logger.Info("Detected unhealthy instances",
		zap.Int("total_unhealthy", len(unhealthy)),
		zap.Int("filtered_unhealthy", len(filteredUnhealthy)),
		zap.Strings("instances", filteredUnhealthy))

	return filteredUnhealthy, nil
}

// RecoverInstances attempts to recover instances from unhealthy state
func (m *Manager) RecoverInstances() error {
	// Get instances that might be recoverable (half-open circuits)
	instances, err := m.ListHealthyInstances()
	if err != nil {
		return errors.Join(ErrListInstancesFailed, err)
	}

	recoverableCount := 0
	for _, instance := range instances {
		if m.circuitBreaker != nil {
			status := m.circuitBreaker.GetStatus(instance.ID)
			if status == types.CircuitHalfOpen {
				// Attempt recovery by performing health check
				health, checkErr := m.PerformHealthCheck(instance.ID)
				if checkErr == nil && health.Reachable && health.StatusCode < 400 {
					recoverableCount++
					m.logger.Info("Instance recovery attempted",
						zap.String("instanceID", instance.ID),
						zap.String("domain", instance.Domain),
						zap.Bool("successful", true))
				}
			}
		}
	}

	m.logger.Info("Instance recovery completed",
		zap.Int("recovery_attempts", recoverableCount))

	return nil
}

// GetHealthSummary provides comprehensive health overview
func (m *Manager) GetHealthSummary() (*HealthSummary, error) {
	instances, err := m.ListHealthyInstances()
	if err != nil {
		return nil, errors.Join(ErrListInstancesFailed, err)
	}

	summary := &HealthSummary{
		Timestamp:          time.Now(),
		TotalInstances:     len(instances),
		HealthyInstances:   0,
		DegradedInstances:  0,
		UnhealthyInstances: 0,
		InstanceDetails:    make(map[string]InstanceHealthDetail),
	}

	// Analyze each instance
	for _, instance := range instances {
		detail := InstanceHealthDetail{
			Domain:        instance.Domain,
			LastChecked:   time.Time{},
			HealthScore:   0,
			CircuitStatus: types.CircuitClosed,
		}

		// Get circuit status
		if m.circuitBreaker != nil {
			detail.CircuitStatus = m.circuitBreaker.GetStatus(instance.ID)
		}

		// Get latest health check if available
		if m.healthRepo != nil {
			health, healthErr := m.healthRepo.GetLatestHealthCheck(context.Background(), instance.Domain)
			if healthErr == nil {
				detail.LastChecked = health.Timestamp
				detail.HealthScore = health.GetHealthScore()
				detail.ResponseTime = health.ResponseTime
				detail.ErrorRate = health.ErrorRate
			}
		}

		// Categorize health status
		switch {
		case detail.CircuitStatus == types.CircuitOpen:
			summary.UnhealthyInstances++
		case detail.CircuitStatus == types.CircuitHalfOpen || detail.HealthScore < 70:
			summary.DegradedInstances++
		default:
			summary.HealthyInstances++
		}

		summary.InstanceDetails[instance.ID] = detail
	}

	// Calculate overall health percentage
	if summary.TotalInstances > 0 {
		summary.OverallHealth = float64(summary.HealthyInstances) / float64(summary.TotalInstances) * 100
	}

	return summary, nil
}

// HealthSummary represents overall health status of the federation system
type HealthSummary struct {
	Timestamp          time.Time                       `json:"timestamp"`
	TotalInstances     int                             `json:"total_instances"`
	HealthyInstances   int                             `json:"healthy_instances"`
	DegradedInstances  int                             `json:"degraded_instances"`
	UnhealthyInstances int                             `json:"unhealthy_instances"`
	OverallHealth      float64                         `json:"overall_health_percentage"`
	InstanceDetails    map[string]InstanceHealthDetail `json:"instance_details"`
}

// InstanceHealthDetail represents detailed health information for a single instance
type InstanceHealthDetail struct {
	Domain        string              `json:"domain"`
	LastChecked   time.Time           `json:"last_checked"`
	HealthScore   float64             `json:"health_score"`
	ResponseTime  time.Duration       `json:"response_time"`
	ErrorRate     float64             `json:"error_rate"`
	CircuitStatus types.CircuitStatus `json:"circuit_status"`
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
func (m *Manager) GetCircuitStatus(instanceID string) types.CircuitStatus {
	return m.circuitBreaker.GetStatus(instanceID)
}

// DeliverMessage delivers a federation message using optimal routing
func (m *Manager) DeliverMessage(ctx context.Context, message *types.FederationMessage, options types.DeliveryOptions) (*types.DeliveryResult, error) {
	startTime := time.Now()

	// Select routes for all targets
	routeMap := make(map[string]*types.Route)
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
		return nil, ErrNoRoutesAvailable
	}

	// Group targets by route for batch delivery
	routeTargets := make(map[string][]string)
	for target, route := range routeMap {
		routeTargets[route.ID] = append(routeTargets[route.ID], target)
	}

	// Deliver to each route
	var wg sync.WaitGroup
	results := make([]*types.DeliveryResult, 0, len(routeTargets))
	resultsMu := sync.Mutex{}

	for routeID, targets := range routeTargets {
		wg.Add(1)
		go func(_ string, tgts []string) {
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

			// Record detailed cost tracking
			if m.costTrackingRepo != nil {
				if err := m.recordDetailedCostTracking(ctx, message, route, result); err != nil {
					m.logger.Error("Failed to record detailed cost tracking",
						zap.String("messageID", message.ID),
						zap.String("routeID", route.ID),
						zap.Error(err))
				}
			}

			// Update circuit breaker
			if result.Success {
				if err := m.circuitBreaker.RecordSuccess(route.InstanceID); err != nil {
					m.logger.Error("failed to record circuit breaker success", zap.Error(err))
				}
			} else {
				m.logger.Error("HTTP delivery failed - recording circuit breaker failure",
					zap.String("routeID", route.ID),
					zap.String("instanceID", route.InstanceID),
					zap.String("errorMessage", result.ErrorMessage))
				if err := m.circuitBreaker.RecordFailure(route.InstanceID, ErrHTTPDeliveryFailed); err != nil {
					m.logger.Error("failed to record circuit breaker failure", zap.Error(err))
				}
			}
		}(routeID, targets)
	}

	wg.Wait()

	// Aggregate results
	aggregated := &types.DeliveryResult{
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

func (m *Manager) filterByMessageType(routes []*types.Route, messageType types.MessageType) []*types.Route {
	filtered := make([]*types.Route, 0, len(routes))

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

func (m *Manager) filterHealthyRoutes(routes []*types.Route) []*types.Route {
	healthy := make([]*types.Route, 0, len(routes))

	for _, route := range routes {
		// Check circuit breaker
		if !m.circuitBreaker.CanAttempt(route.InstanceID) {
			continue
		}

		// Check instance status
		instance, err := m.GetInstance(route.InstanceID)
		if err != nil || instance.Status != types.InstanceStatusActive {
			continue
		}

		// Check quota
		if instance.TierLevel != types.TierBlocked && instance.CurrentUsage < instance.MonthlyQuota {
			healthy = append(healthy, route)
		}
	}

	return healthy
}

func (m *Manager) getInstancesForDomain(domain string) ([]*types.Instance, error) {
	// For federation, typically one instance per domain
	// But could have multiple for redundancy

	ctx := context.Background()

	// Query by domain using scan (in production, add GSI for domain lookups)
	instances := []*types.Instance{}

	// Try exact match first
	instance, err := m.registry.GetInstance(ctx, domain)
	if err == nil {
		instances = append(instances, instance)
	}

	// Could also query for backup instances, CDN endpoints, etc.

	return instances, nil
}

func (m *Manager) createRouteFromInstance(instance *types.Instance) (*types.Route, error) {
	endpoint, err := url.Parse(instance.SharedInboxURL)
	if err != nil {
		endpoint, err = url.Parse(instance.InboxURL)
		if err != nil {
			return nil, ErrInvalidInboxURLs
		}
	}

	route := &types.Route{
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

func (m *Manager) deliverToRoute(ctx context.Context, route *types.Route, message *types.FederationMessage, targets []string, options types.DeliveryOptions) *types.DeliveryResult {
	startTime := time.Now()

	// Create the delivery result
	result := &types.DeliveryResult{
		MessageID:  message.ID,
		InstanceID: route.InstanceID,
		RouteID:    route.ID,
		Attempts:   1,
		Timestamp:  startTime,
	}

	// Get instance information for delivery
	instance, err := m.GetInstance(route.InstanceID)
	if err != nil {
		m.logger.Error("failed to get instance for delivery",
			zap.String("instanceID", route.InstanceID),
			zap.Error(err))
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("failed to get instance: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Determine the target inbox URL
	targetInbox := instance.SharedInboxURL
	if err := common.ValidateRequiredParam("targetInbox", targetInbox); err != nil {
		targetInbox = instance.InboxURL
	}
	if err := common.ValidateRequiredParam("targetInbox", targetInbox); err != nil {
		result.Success = false
		result.ErrorMessage = "no inbox URL available for instance"
		result.Duration = time.Since(startTime)
		return result
	}

	// Convert federation message to ActivityPub activity
	activity, signingActor, err := m.prepareActivityForDelivery(ctx, message, targets)
	if err != nil {
		m.logger.Error("failed to prepare activity for delivery",
			zap.String("messageID", message.ID),
			zap.Error(err))
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("failed to prepare activity: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Perform the HTTP delivery with retries
	maxRetries := options.MaxRetries
	if maxRetries <= 0 {
		maxRetries = m.config.MaxRetries
	}

	retryBackoff := options.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = m.config.RetryBackoff
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result.Attempts = attempt

		// Apply exponential backoff for retries
		if attempt > 1 {
			backoffDuration := time.Duration(attempt-1) * retryBackoff
			select {
			case <-ctx.Done():
				result.Success = false
				result.ErrorMessage = "delivery cancelled"
				result.Duration = time.Since(startTime)
				return result
			case <-time.After(backoffDuration):
				// Continue with retry
			}
		}

		// Attempt delivery
		deliveryErr := m.performHTTPDelivery(ctx, activity, targetInbox, signingActor, result)
		if deliveryErr == nil {
			// Success!
			result.Success = true
			result.Duration = time.Since(startTime)
			result.BytesSent = int64(len(message.Payload))
			result.Cost = float64(result.BytesSent) * route.CostPerByte

			m.logger.Info("federation delivery successful",
				zap.String("messageID", message.ID),
				zap.String("routeID", route.ID),
				zap.String("targetInbox", targetInbox),
				zap.Int("attempts", attempt),
				zap.Duration("duration", result.Duration))

			return result
		}

		lastErr = deliveryErr

		// Check if this is a retryable error
		if !m.isRetryableError(result.StatusCode) {
			break
		}

		m.logger.Warn("federation delivery attempt failed",
			zap.String("messageID", message.ID),
			zap.String("routeID", route.ID),
			zap.Int("attempt", attempt),
			zap.Int("maxRetries", maxRetries),
			zap.Error(deliveryErr))
	}

	// All retries exhausted
	result.Success = false
	result.Duration = time.Since(startTime)
	if lastErr != nil {
		result.ErrorMessage = lastErr.Error()
	}

	m.logger.Error("federation delivery failed after retries",
		zap.String("messageID", message.ID),
		zap.String("routeID", route.ID),
		zap.Int("attempts", result.Attempts),
		zap.Error(lastErr))

	return result
}

func (m *Manager) clearRouteCache(domain string) {
	cacheKey := fmt.Sprintf("routes:%s", domain)
	m.routeCache.Delete(cacheKey)
}

type cachedRoutes struct {
	routes       []*types.Route
	cachedAt     time.Time
	healthStatus map[string]RouteHealthStatus
}

// getAdaptiveCacheTTL returns cache TTL based on route health status
func (m *Manager) getAdaptiveCacheTTL(routes []*types.Route) time.Duration {
	if m.thresholdManager == nil {
		return m.cacheTTL
	}

	// Find the worst route health status
	worstHealth := RouteHealthPreferred
	for _, route := range routes {
		if m.optimizer != nil {
			metrics, err := m.optimizer.GetRouteMetrics(context.Background(), route.ID)
			if err == nil {
				assessment := m.thresholdManager.AssessRouteHealth(context.Background(), route.ID, metrics)
				if assessment.Status > worstHealth {
					worstHealth = assessment.Status
				}
			}
		}
	}

	// Return appropriate TTL based on worst health
	switch worstHealth {
	case RouteHealthPreferred, RouteHealthHealthy:
		return m.thresholdManager.config.HealthyRouteTTL
	case RouteHealthDegraded, RouteHealthCritical:
		return m.thresholdManager.config.DegradedRouteTTL
	default:
		return m.thresholdManager.config.UnknownRouteTTL
	}
}

// shouldEnterEmergencyMode checks if emergency mode should be activated
func (m *Manager) shouldEnterEmergencyMode(healthyRoutes, totalRoutes int) bool {
	if m.circuitBreaker == nil {
		return false
	}
	return m.circuitBreaker.ShouldEnterEmergencyMode(healthyRoutes, totalRoutes)
}

// enterEmergencyMode activates emergency mode with proper logging
func (m *Manager) enterEmergencyMode() {
	m.emergencyMu.Lock()
	defer m.emergencyMu.Unlock()

	if !m.emergencyMode {
		m.emergencyMode = true
		m.logger.Error("Entering emergency mode - all routes degraded",
			zap.String("action", "progressive_backpressure_activated"))
	}
}

// selectRouteInEmergencyMode selects routes with emergency mode logic
func (m *Manager) selectRouteInEmergencyMode(destination string, messageType types.MessageType) (*types.Route, error) {
	m.logger.Debug("Selecting route in emergency mode",
		zap.String("destination", destination),
		zap.String("messageType", string(messageType)))

	// Get all routes (including degraded ones)
	routes, err := m.GetRoutes(destination)
	if err != nil {
		return nil, errors.Join(ErrGetRoutesInEmergencyMode, err)
	}

	if err := common.ValidateSliceNotEmpty("routes", routes); err != nil {
		return nil, types.ErrNoHealthyRoutes
	}

	// Apply emergency backpressure rules
	if m.circuitBreaker != nil && m.thresholdManager != nil {
		backpressureRules := m.circuitBreaker.GetBackpressureRules()
		priority := m.thresholdManager.getMessagePriority(messageType)

		rule, exists := backpressureRules[priority]
		if exists {
			// Calculate current health ratio
			healthyCount := 0
			for _, route := range routes {
				if m.circuitBreaker.GetStatus(route.InstanceID) == types.CircuitClosed {
					healthyCount++
				}
			}

			healthRatio := float64(healthyCount) / float64(len(routes))

			// Apply backpressure rules
			switch rule.Action {
			case "queue_if_below_threshold":
				if healthRatio < rule.Threshold {
					m.logger.Warn("message queued due to backpressure",
						zap.Float64("healthRatio", healthRatio),
						zap.Float64("threshold", rule.Threshold),
						zap.String("messageType", string(messageType)))
					return nil, ErrMessageQueuedBackpressure
				}
			case "queue":
				return nil, ErrMessageQueuedEmergency
			case "drop":
				return nil, ErrMessageDroppedEmergency
			}
		}
	}

	// Select best available route (even if degraded)
	for _, route := range routes {
		if m.circuitBreaker == nil || m.circuitBreaker.CanAttempt(route.InstanceID) {
			m.logger.Warn("Using degraded route in emergency mode",
				zap.String("routeID", route.ID),
				zap.String("destination", destination))
			return route, nil
		}
	}

	return nil, types.ErrNoHealthyRoutes
}

// prepareActivityForDelivery converts a federation message to an ActivityPub activity
func (m *Manager) prepareActivityForDelivery(ctx context.Context, message *types.FederationMessage, targets []string) (*activitypub.Activity, *activitypub.Actor, error) {
	// If the message already has a payload, try to parse it as an activity
	if err := common.ValidateSliceNotEmpty("message.Payload", message.Payload); err == nil {
		var activity activitypub.Activity
		if err := json.Unmarshal(message.Payload, &activity); err == nil {
			// Get the signing actor
			signingActor, err := m.getSigningActor(ctx, message.Actor)
			if err != nil {
				return nil, nil, errors.Join(ErrGetSigningActorFailed, err)
			}
			return &activity, signingActor, nil
		}
	}

	// Create a new activity from the message
	publishedTime := message.CreatedAt
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        message.ID,
			Type:      string(message.Type),
			Published: &publishedTime,
		},
		Actor: message.Actor,
	}

	// Set recipients
	if err := common.ValidateSliceNotEmpty("targets", targets); err == nil {
		activity.To = targets
	}

	// Set the object
	if message.Object != nil {
		activity.Object = message.Object
	}

	// Get the signing actor
	signingActor, err := m.getSigningActor(ctx, message.Actor)
	if err != nil {
		return nil, nil, errors.Join(ErrGetSigningActorFailed, err)
	}

	return activity, signingActor, nil
}

// getSigningActor retrieves the actor for signing the request
func (m *Manager) getSigningActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	if m.federationStore == nil {
		return nil, ErrFederationStoreNotConfigured
	}

	// Extract username from actor ID
	username := extractUsernameFromActorID(actorID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		m.logger.Error("could not extract username from actor ID", zap.String("actorID", actorID))
		return nil, ErrExtractUsernameFromActorID
	}

	// Get actor from storage
	actor, err := m.federationStore.GetActor(ctx, username)
	if err != nil {
		m.logger.Error("failed to get actor", zap.String("username", username), zap.Error(err))
		return nil, errors.Join(ErrGetActorFailed, err)
	}

	return actor, nil
}

// performHTTPDelivery performs the actual HTTP delivery
func (m *Manager) performHTTPDelivery(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor, result *types.DeliveryResult) error {
	// Serialize the activity
	body, err := json.Marshal(activity)
	if err != nil {
		return errors.Join(ErrMarshalActivityFailed, err)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetInbox, bytes.NewReader(body))
	if err != nil {
		return errors.Join(ErrCreateRequestFailed, err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("User-Agent", "Lesser/1.0 ActivityPub")

	// Get the actor's private key from storage
	if m.federationStore == nil {
		return ErrFederationStoreNotConfiguredForSigning
	}

	privateKeyPEM, err := m.federationStore.GetActorPrivateKey(ctx, signingActor.PreferredUsername)
	if err != nil {
		return errors.Join(ErrGetPrivateKeyFailed, err)
	}

	// Parse the private key
	privateKey, err := federation.ParsePrivateKeyPEM([]byte(privateKeyPEM))
	if err != nil {
		return errors.Join(ErrParsePrivateKeyFailed, err)
	}

	// Sign the request
	keyID := signingActor.PublicKey.ID
	if err := federation.SignHTTPRequest(req, privateKey, keyID); err != nil {
		return errors.Join(ErrSignRequestFailed, err)
	}

	// Send the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return errors.Join(ErrSendRequestFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Update result with status code
	result.StatusCode = resp.StatusCode

	respBodyBytes, respBodyTruncated, readErr := common.ReadUntrustedHTTPResponseBody(resp.Body, common.MaxUntrustedHTTPResponseBodyBytes)
	if readErr != nil {
		m.logger.Warn("failed to read response body", zap.String("targetInbox", targetInbox), zap.Error(readErr))
	}
	respBody := common.FormatUntrustedHTTPBodySnippet(respBodyBytes, respBodyTruncated)

	// Check the response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		m.logger.Debug("HTTP delivery successful",
			zap.String("targetInbox", targetInbox),
			zap.Int("statusCode", resp.StatusCode),
			zap.Bool("response_truncated", respBodyTruncated),
			zap.String("response", respBody))
		return nil
	}

	// Log the failure
	m.logger.Warn("HTTP delivery failed",
		zap.String("targetInbox", targetInbox),
		zap.Int("statusCode", resp.StatusCode),
		zap.Bool("response_truncated", respBodyTruncated),
		zap.String("response", respBody))

	m.logger.Error("HTTP delivery failed",
		zap.String("targetInbox", targetInbox),
		zap.Int("statusCode", resp.StatusCode),
		zap.Bool("response_truncated", respBodyTruncated),
		zap.String("responseBody", respBody))
	return ErrHTTPDeliveryFailed
}

// isRetryableError determines if an HTTP status code indicates a retryable error
func (m *Manager) isRetryableError(statusCode int) bool {
	switch statusCode {
	case http.StatusInternalServerError, // 500
		http.StatusBadGateway,                    // 502
		http.StatusServiceUnavailable,            // 503
		http.StatusGatewayTimeout,                // 504
		http.StatusInsufficientStorage,           // 507
		http.StatusNetworkAuthenticationRequired: // 511
		return true
	case http.StatusTooManyRequests: // 429
		return true
	default:
		// 4xx errors (except 429) are generally not retryable
		// 2xx and 3xx are success/redirect (shouldn't get here)
		return false
	}
}

// extractUsernameFromActorID extracts username from an ActivityPub actor ID
func extractUsernameFromActorID(actorID string) string {
	// Parse the URL
	u, err := url.Parse(actorID)
	if err != nil {
		return ""
	}

	// Extract from path - typical format: /users/username or /actors/username
	path := u.Path
	if err := common.ValidateRequiredParam("path", path); err != nil {
		return ""
	}

	parts := bytes.Split([]byte(path), []byte("/"))
	if len(parts) >= 3 {
		// Look for users/ or actors/ prefix
		if string(parts[1]) == "users" || string(parts[1]) == "actors" {
			return string(parts[2])
		}
	}

	// Fallback: use the last path segment
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil && len(parts) > 1 {
		return string(parts[len(parts)-1])
	}

	return ""
}

// recordDetailedCostTracking records detailed cost and performance metrics for a delivery
func (m *Manager) recordDetailedCostTracking(ctx context.Context, message *types.FederationMessage, route *types.Route, result *types.DeliveryResult) error {
	if m.costTrackingRepo == nil {
		return nil // No cost tracking configured
	}

	// Extract domain from route or result
	domain := route.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil && route.Endpoint != nil {
		domain = route.Endpoint.Host
	}

	// Create the cost tracking record
	costTracker := &models.FederationCostTracking{
		ActivityID:    message.ID,
		Domain:        domain,
		ActivityType:  string(message.Type),
		Direction:     "outbound",
		OperationType: "outbox_delivery",
		BillingPeriod: time.Now().Format("2006-01"),

		// Basic success/failure tracking
		Success:      result.Success,
		ErrorMessage: result.ErrorMessage,

		// Enhanced delivery attribution
		BytesSent:         result.BytesSent,
		RetryAttempts:     result.Attempts - 1, // First attempt isn't a retry
		DeliveryAttempts:  result.Attempts,
		RouteID:           route.ID,
		DestinationServer: domain,
		FinalRetrySuccess: result.Success,

		// Performance metrics
		ResponseTimeMs:   result.Duration.Milliseconds(),
		ProcessingTimeMs: result.Duration.Milliseconds(), // Simplified - could be more detailed

		// Network costs (simplified calculation)
		HTTPRequestCount:  int64(result.Attempts),
		DataTransferBytes: result.BytesSent,

		// Payload metrics
		PayloadSize: int64(len(message.Payload)),

		// Initialize per-route breakdown
		RouteBreakdown:     make(map[string]int64),
		RouteLatency:       make(map[string]int64),
		RouteErrors:        make(map[string]int),
		RouteSuccessRates:  make(map[string]float64),
		RetryDelaySeconds:  make([]int64, 0),
		RetryErrorMessages: make([]string, 0),
	}

	// Add route-specific metrics
	costTracker.AddRouteDeliveryAttempt(
		route.ID,
		result.BytesSent,
		result.Duration.Milliseconds(),
		result.Success,
		result.ErrorMessage,
	)

	// Add retry delays if multiple attempts were made
	if result.Attempts > 1 {
		// Simplified - in reality would track actual delays
		for i := 1; i < result.Attempts; i++ {
			// Exponential backoff assumption: i + 15 seconds + jitter
			estimatedDelay := int64(i + 15)
			costTracker.AddRetryDelay(estimatedDelay)
		}

		// Add error messages for retries
		if !result.Success && result.ErrorMessage != "" {
			costTracker.RetryErrorMessages = append(costTracker.RetryErrorMessages, result.ErrorMessage)
		}
	}

	// Calculate simplified costs (in microdollars)
	// These would be more sophisticated in production
	costTracker.HTTPRequestCost = int64(result.Attempts) * 100                    // $0.0001 per request = 100 microdollars
	costTracker.DataTransferCost = (result.BytesSent * 90) / (1024 * 1024 * 1024) // $0.09 per GB
	costTracker.RetryCost = int64(result.Attempts-1) * 50                         // Penalty for retries

	// Set timestamps
	now := time.Now()
	costTracker.Timestamp = now
	costTracker.CreatedAt = now
	costTracker.UpdatedAt = now

	// Record the cost tracking
	return m.costTrackingRepo.RecordFederationCost(ctx, costTracker)
}

// Removed unused function: generateRequestID

func defaultRoutingConfig() *types.RoutingConfig {
	return &types.RoutingConfig{
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
