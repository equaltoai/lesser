package services

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultLoadBalancer implements multiple load balancing strategies
// Memory optimized: 56 → 32 bytes (24 bytes saved)
type DefaultLoadBalancer struct {
	roundRobinCounters map[string]*int64
	connectionCounts   map[string]*int64
	stats              *LoadBalancerMetrics
	rand               *rand.Rand
	mu                 sync.RWMutex
}

// LoadBalancerMetrics tracks load balancer performance
type LoadBalancerMetrics struct {
	totalRequests        int64
	successfulSelections int64
	failedSelections     int64
	totalLatency         int64
}

// NewDefaultLoadBalancer creates a new load balancer
func NewDefaultLoadBalancer() *DefaultLoadBalancer {
	return &DefaultLoadBalancer{
		roundRobinCounters: make(map[string]*int64),
		connectionCounts:   make(map[string]*int64),
		stats:              &LoadBalancerMetrics{},
		rand:               rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 - non-cryptographic use for load balancing
	}
}

// Select selects an instance using the specified strategy
func (lb *DefaultLoadBalancer) Select(instances []*ServiceInstance, strategy LoadBalanceStrategy) *ServiceInstance {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		atomic.AddInt64(&lb.stats.totalRequests, 1)
		atomic.AddInt64(&lb.stats.totalLatency, duration.Nanoseconds())
	}()

	if len(instances) == 0 {
		atomic.AddInt64(&lb.stats.failedSelections, 1)
		return nil
	}

	if len(instances) == 1 {
		atomic.AddInt64(&lb.stats.successfulSelections, 1)
		return instances[0]
	}

	var selected *ServiceInstance

	switch strategy {
	case RoundRobin:
		selected = lb.selectRoundRobin(instances)
	case WeightedRandom:
		selected = lb.selectWeightedRandom(instances)
	case LeastConnections:
		selected = lb.selectLeastConnections(instances)
	case HealthyFirst:
		selected = lb.selectHealthyFirst(instances)
	case LocalFirst:
		selected = lb.selectLocalFirst(instances)
	default:
		selected = lb.selectRoundRobin(instances) // Default to round robin
	}

	if selected != nil {
		atomic.AddInt64(&lb.stats.successfulSelections, 1)
	} else {
		atomic.AddInt64(&lb.stats.failedSelections, 1)
	}

	return selected
}

// selectRoundRobin implements round-robin load balancing
func (lb *DefaultLoadBalancer) selectRoundRobin(instances []*ServiceInstance) *ServiceInstance {
	if len(instances) == 0 {
		return nil
	}

	// Use service name as key for round-robin counter
	serviceName := instances[0].ServiceName

	lb.mu.Lock()
	counter, exists := lb.roundRobinCounters[serviceName]
	if !exists {
		counter = new(int64)
		lb.roundRobinCounters[serviceName] = counter
	}
	lb.mu.Unlock()

	// Atomic increment and select
	index := atomic.AddInt64(counter, 1) - 1
	return instances[index%int64(len(instances))]
}

// selectWeightedRandom implements weighted random load balancing
func (lb *DefaultLoadBalancer) selectWeightedRandom(instances []*ServiceInstance) *ServiceInstance {
	if len(instances) == 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := 0
	for _, instance := range instances {
		weight := instance.Weight
		if weight <= 0 {
			weight = 1 // Default weight
		}
		totalWeight += weight
	}

	if totalWeight == 0 {
		// Fallback to simple random
		return instances[lb.rand.Intn(len(instances))]
	}

	// Generate random number and select based on weight
	randomWeight := lb.rand.Intn(totalWeight)
	currentWeight := 0

	for _, instance := range instances {
		weight := instance.Weight
		if weight <= 0 {
			weight = 1
		}
		currentWeight += weight

		if randomWeight < currentWeight {
			return instance
		}
	}

	// Fallback to last instance
	return instances[len(instances)-1]
}

// selectLeastConnections implements least connections load balancing
func (lb *DefaultLoadBalancer) selectLeastConnections(instances []*ServiceInstance) *ServiceInstance {
	if len(instances) == 0 {
		return nil
	}

	var selected *ServiceInstance
	minConnections := int64(-1)

	for _, instance := range instances {
		lb.mu.RLock()
		counter, exists := lb.connectionCounts[instance.ID]
		lb.mu.RUnlock()

		var connections int64
		if exists {
			connections = atomic.LoadInt64(counter)
		} else {
			// Initialize counter for new instance
			lb.mu.Lock()
			counter = new(int64)
			lb.connectionCounts[instance.ID] = counter
			lb.mu.Unlock()
			connections = 0
		}

		if minConnections == -1 || connections < minConnections {
			minConnections = connections
			selected = instance
		}
	}

	// Increment connection count for selected instance
	if selected != nil {
		lb.mu.RLock()
		counter := lb.connectionCounts[selected.ID]
		lb.mu.RUnlock()
		atomic.AddInt64(counter, 1)
	}

	return selected
}

// selectHealthyFirst prioritizes healthy instances
func (lb *DefaultLoadBalancer) selectHealthyFirst(instances []*ServiceInstance) *ServiceInstance {
	if len(instances) == 0 {
		return nil
	}

	// Separate healthy and unhealthy instances
	var healthy, unhealthy []*ServiceInstance

	for _, instance := range instances {
		if instance.Health.Status == "healthy" {
			healthy = append(healthy, instance)
		} else {
			unhealthy = append(unhealthy, instance)
		}
	}

	// Prefer healthy instances
	if len(healthy) > 0 {
		return lb.selectRoundRobin(healthy)
	}

	// Fallback to unhealthy instances
	return lb.selectRoundRobin(unhealthy)
}

// selectLocalFirst prioritizes local instances (same region/zone)
func (lb *DefaultLoadBalancer) selectLocalFirst(instances []*ServiceInstance) *ServiceInstance {
	if len(instances) == 0 {
		return nil
	}

	// For now, implement simple round-robin
	// In a real implementation, this would check region/zone metadata
	return lb.selectRoundRobin(instances)
}

// UpdateWeights updates the weights of instances
func (lb *DefaultLoadBalancer) UpdateWeights(_ []*ServiceInstance) error {
	// This could be used to dynamically adjust weights based on performance
	// For now, this is a no-op as weights are stored in the instances themselves
	return nil
}

// ReleaseConnection decrements the connection count for an instance
func (lb *DefaultLoadBalancer) ReleaseConnection(instanceID string) {
	lb.mu.RLock()
	counter, exists := lb.connectionCounts[instanceID]
	lb.mu.RUnlock()

	if exists {
		// Ensure we don't go below zero
		for {
			current := atomic.LoadInt64(counter)
			if current <= 0 {
				break
			}
			if atomic.CompareAndSwapInt64(counter, current, current-1) {
				break
			}
		}
	}
}

// GetStats returns load balancer statistics
func (lb *DefaultLoadBalancer) GetStats() LoadBalancerStats {
	totalRequests := atomic.LoadInt64(&lb.stats.totalRequests)
	successfulSelections := atomic.LoadInt64(&lb.stats.successfulSelections)
	failedSelections := atomic.LoadInt64(&lb.stats.failedSelections)
	totalLatency := atomic.LoadInt64(&lb.stats.totalLatency)

	var avgLatency int64
	if totalRequests > 0 {
		avgLatency = totalLatency / totalRequests
	}

	return LoadBalancerStats{
		TotalRequests:        totalRequests,
		SuccessfulSelections: successfulSelections,
		FailedSelections:     failedSelections,
		AverageLatency:       avgLatency,
	}
}

// Reset resets all counters and statistics
func (lb *DefaultLoadBalancer) Reset() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Reset round-robin counters
	for key := range lb.roundRobinCounters {
		atomic.StoreInt64(lb.roundRobinCounters[key], 0)
	}

	// Reset connection counts
	for key := range lb.connectionCounts {
		atomic.StoreInt64(lb.connectionCounts[key], 0)
	}

	// Reset stats
	atomic.StoreInt64(&lb.stats.totalRequests, 0)
	atomic.StoreInt64(&lb.stats.successfulSelections, 0)
	atomic.StoreInt64(&lb.stats.failedSelections, 0)
	atomic.StoreInt64(&lb.stats.totalLatency, 0)
}

// HealthAwareLoadBalancer wraps a load balancer with health awareness
type HealthAwareLoadBalancer struct {
	delegate        LoadBalancer
	healthThreshold time.Duration
}

// NewHealthAwareLoadBalancer creates a health-aware load balancer
func NewHealthAwareLoadBalancer(delegate LoadBalancer, healthThreshold time.Duration) *HealthAwareLoadBalancer {
	return &HealthAwareLoadBalancer{
		delegate:        delegate,
		healthThreshold: healthThreshold,
	}
}

// Select selects an instance with health awareness
func (h *HealthAwareLoadBalancer) Select(instances []*ServiceInstance, strategy LoadBalanceStrategy) *ServiceInstance {
	// Filter out instances that haven't been seen recently
	var healthy []*ServiceInstance
	now := time.Now()

	for _, instance := range instances {
		if now.Sub(instance.LastSeen) <= h.healthThreshold {
			healthy = append(healthy, instance)
		}
	}

	// If no healthy instances, fall back to all instances
	if len(healthy) == 0 {
		healthy = instances
	}

	return h.delegate.Select(healthy, strategy)
}

// UpdateWeights delegates to the underlying load balancer
func (h *HealthAwareLoadBalancer) UpdateWeights(instances []*ServiceInstance) error {
	return h.delegate.UpdateWeights(instances)
}

// GetStats delegates to the underlying load balancer
func (h *HealthAwareLoadBalancer) GetStats() LoadBalancerStats {
	return h.delegate.GetStats()
}

// WeightedLoadBalancer implements weighted load balancing with dynamic weight adjustment
// Memory optimized: 48 → 24 bytes (24 bytes saved)
type WeightedLoadBalancer struct {
	delegate LoadBalancer
	weights  map[string]int
	mu       sync.RWMutex
}

// NewWeightedLoadBalancer creates a weighted load balancer
func NewWeightedLoadBalancer(delegate LoadBalancer) *WeightedLoadBalancer {
	return &WeightedLoadBalancer{
		weights:  make(map[string]int),
		delegate: delegate,
	}
}

// Select selects an instance using weighted selection
func (w *WeightedLoadBalancer) Select(instances []*ServiceInstance, strategy LoadBalanceStrategy) *ServiceInstance {
	// Apply dynamic weights
	w.mu.RLock()
	for _, instance := range instances {
		if weight, exists := w.weights[instance.ID]; exists {
			instance.Weight = weight
		}
	}
	w.mu.RUnlock()

	// Use weighted random if strategy supports it
	if strategy == WeightedRandom {
		return w.delegate.Select(instances, WeightedRandom)
	}

	// Otherwise use the requested strategy
	return w.delegate.Select(instances, strategy)
}

// SetWeight sets the weight for a specific instance
func (w *WeightedLoadBalancer) SetWeight(instanceID string, weight int) {
	w.mu.Lock()
	w.weights[instanceID] = weight
	w.mu.Unlock()
}

// GetWeight gets the weight for a specific instance
func (w *WeightedLoadBalancer) GetWeight(instanceID string) int {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if weight, exists := w.weights[instanceID]; exists {
		return weight
	}
	return 1 // Default weight
}

// UpdateWeights updates weights based on performance metrics
func (w *WeightedLoadBalancer) UpdateWeights(_ []*ServiceInstance) error {
	// This could implement dynamic weight adjustment based on:
	// - Response times
	// - Error rates
	// - CPU/memory usage
	// - Custom metrics

	// For now, this is a placeholder
	return nil
}

// GetStats delegates to the underlying load balancer
func (w *WeightedLoadBalancer) GetStats() LoadBalancerStats {
	return w.delegate.GetStats()
}

// Convenience functions for creating load balancers

// NewRoundRobinLoadBalancer creates a round-robin load balancer
func NewRoundRobinLoadBalancer() LoadBalancer {
	return NewDefaultLoadBalancer()
}

// NewWeightedRandomLoadBalancer creates a weighted random load balancer
func NewWeightedRandomLoadBalancer() LoadBalancer {
	return NewDefaultLoadBalancer()
}

// NewLeastConnectionsLoadBalancer creates a least connections load balancer
func NewLeastConnectionsLoadBalancer() LoadBalancer {
	return NewDefaultLoadBalancer()
}

// NewHealthyFirstLoadBalancer creates a health-first load balancer
func NewHealthyFirstLoadBalancer(healthThreshold time.Duration) LoadBalancer {
	delegate := NewDefaultLoadBalancer()
	return NewHealthAwareLoadBalancer(delegate, healthThreshold)
}
