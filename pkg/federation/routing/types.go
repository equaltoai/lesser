package routing

import (
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
)

// RouteManager manages federation message routing
type RouteManager interface {
	// Route selection
	SelectRoute(destination string, messageType types.MessageType) (*types.Route, error)
	GetRoutes(destination string) ([]*types.Route, error)

	// Instance management
	RegisterInstance(instance *types.Instance) error
	UpdateInstanceHealth(instanceID string, health *types.HealthStatus) error
	GetInstance(instanceID string) (*types.Instance, error)
	ListHealthyInstances() ([]*types.Instance, error)

	// Route optimization
	OptimizeRoutes() error
	GetRouteMetrics(destination string) (*types.RouteMetrics, error)

	// Circuit breaker
	OpenCircuit(instanceID string, reason string) error
	CloseCircuit(instanceID string) error
	GetCircuitStatus(instanceID string) types.CircuitStatus
}

// RouteSelector implements routing algorithms
type RouteSelector interface {
	SelectBestRoute(routes []*types.Route, options types.SelectionOptions) (*types.Route, error)
	RankRoutes(routes []*types.Route) []*types.Route
}

// HealthChecker monitors instance health
type HealthChecker interface {
	CheckHealth(instance *types.Instance) (*types.HealthStatus, error)
	StartMonitoring(instance *types.Instance) error
	StopMonitoring(instanceID string) error
	GetHealthHistory(instanceID string, duration time.Duration) ([]*types.HealthStatus, error)
}

// CircuitBreaker implements circuit breaker pattern
type CircuitBreaker interface {
	// Circuit control
	Open(instanceID string, reason string) error
	Close(instanceID string) error
	HalfOpen(instanceID string) error

	// Status checks
	IsOpen(instanceID string) bool
	CanAttempt(instanceID string) bool
	RecordSuccess(instanceID string) error
	RecordFailure(instanceID string, err error) error

	// Configuration
	SetThreshold(instanceID string, threshold int) error
	SetTimeout(instanceID string, timeout time.Duration) error
}

// DeliveryQueue manages message delivery
type DeliveryQueue interface {
	Enqueue(message *types.FederationMessage, options types.DeliveryOptions) error
	Dequeue(count int) ([]*types.QueuedMessage, error)

	// Retry management
	ScheduleRetry(messageID string, after time.Duration) error
	GetRetryMessages() ([]*types.QueuedMessage, error)

	// Dead letter queue
	MoveToDLQ(messageID string, reason string) error
	GetDLQMessages(limit int) ([]*types.QueuedMessage, error)

	// Metrics
	GetQueueDepth() (int64, error)
	GetQueueMetrics() (*types.QueueMetrics, error)
}

// RouteOptimizer optimizes routing decisions
type RouteOptimizer interface {
	Optimize(routes []*types.Route, history []*types.DeliveryResult) ([]*types.Route, error)
	PredictLatency(route *types.Route, messageSize int64) time.Duration
	EstimateCost(route *types.Route, messageSize int64) float64
	RecommendBatchSize(route *types.Route) int
}

// LoadBalancer distributes load across routes
type LoadBalancer interface {
	Balance(routes []*types.Route, load int) map[string]int
	UpdateWeights(metrics map[string]*types.RouteMetrics) error
	GetCurrentWeights() map[string]float64
}
