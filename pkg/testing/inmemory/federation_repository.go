// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// FederationRepository is a thread-safe in-memory implementation of interfaces.FederationRepository.
type FederationRepository struct {
	mu sync.RWMutex

	// Instance info: key = domain
	instances map[string]*storage.InstanceInfo

	// Federation activities
	activities []*storage.FederationActivity

	// Federation costs
	costs []*storage.FederationCost
}

// NewFederationRepository creates a new in-memory federation repository
func NewFederationRepository() *FederationRepository {
	return &FederationRepository{
		instances:  make(map[string]*storage.InstanceInfo),
		activities: []*storage.FederationActivity{},
		costs:      []*storage.FederationCost{},
	}
}

// GetInstanceInfo retrieves information about a federated instance
func (r *FederationRepository) GetInstanceInfo(_ context.Context, domain string) (*storage.InstanceInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.instances[domain]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return info, nil
}

// UpsertInstanceInfo creates or updates instance information
func (r *FederationRepository) UpsertInstanceInfo(_ context.Context, info *storage.InstanceInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.instances[info.Domain] = info
	return nil
}

// GetKnownInstances retrieves a list of known federated instances
func (r *FederationRepository) GetKnownInstances(_ context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.InstanceInfo
	for _, info := range r.instances {
		result = append(result, info)
	}
	return paginateFederationInstances(result, limit, cursor)
}

// GetFederationStatistics retrieves federation statistics for a time range
func (r *FederationRepository) GetFederationStatistics(_ context.Context, _, _ time.Time) (*storage.FederationStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return &storage.FederationStats{
		ActiveInstances: int64(len(r.instances)),
	}, nil
}

// GetInstanceStats retrieves comprehensive statistics for a specific instance
func (r *FederationRepository) GetInstanceStats(_ context.Context, domain string) (*storage.InstanceStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.instances[domain]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return &storage.InstanceStats{Domain: domain}, nil
}

// RecordFederationActivity records a single federation activity for cost tracking
func (r *FederationRepository) RecordFederationActivity(_ context.Context, activity *storage.FederationActivity) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.activities = append(r.activities, activity)
	return nil
}

// GetFederationCosts retrieves aggregated federation costs
func (r *FederationRepository) GetFederationCosts(_ context.Context, _, _ time.Time, _ int, _ string) ([]*storage.FederationCost, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.costs, "", nil
}

// GetInstanceHealthReport generates a health report for a specific instance
func (r *FederationRepository) GetInstanceHealthReport(_ context.Context, domain string, _ time.Duration) (*storage.InstanceHealthReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return &storage.InstanceHealthReport{Domain: domain}, nil
}

// GetCostProjections generates cost projections based on historical data
func (r *FederationRepository) GetCostProjections(_ context.Context, _ string) (*storage.CostProjection, error) {
	return &storage.CostProjection{}, nil
}

// GetFederationNodes retrieves federation nodes up to a certain depth
func (r *FederationRepository) GetFederationNodes(_ context.Context, _ int) ([]*storage.FederationNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var nodes []*storage.FederationNode
	for domain := range r.instances {
		nodes = append(nodes, &storage.FederationNode{Domain: domain})
	}
	return nodes, nil
}

// GetFederationNodesByHealth retrieves federation nodes filtered by health status
func (r *FederationRepository) GetFederationNodesByHealth(_ context.Context, _ string, _ int) ([]*storage.FederationNode, error) {
	return []*storage.FederationNode{}, nil
}

// GetFederationEdges retrieves edges between specified domains
func (r *FederationRepository) GetFederationEdges(_ context.Context, _ []string) ([]*storage.FederationEdge, error) {
	return []*storage.FederationEdge{}, nil
}

// GetInstanceMetadata retrieves metadata for a specific instance
func (r *FederationRepository) GetInstanceMetadata(_ context.Context, domain string) (*storage.InstanceMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.instances[domain]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return &storage.InstanceMetadata{Domain: domain}, nil
}

// CalculateFederationClusters calculates instance clusters based on connections
func (r *FederationRepository) CalculateFederationClusters(_ context.Context) ([]*storage.InstanceCluster, error) {
	return []*storage.InstanceCluster{}, nil
}

// Helper functions

func paginateFederationInstances(instances []*storage.InstanceInfo, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	startIdx := 0
	if cursor != "" {
		for i, inst := range instances {
			if inst.Domain == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var result []*storage.InstanceInfo
	var nextCursor string

	for i := startIdx; i < len(instances) && len(result) < limit; i++ {
		result = append(result, instances[i])
	}

	if startIdx+limit < len(instances) && len(result) > 0 {
		nextCursor = result[len(result)-1].Domain
	}

	return result, nextCursor, nil
}

// Clear clears all data (test helper)
func (r *FederationRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.instances = make(map[string]*storage.InstanceInfo)
	r.activities = []*storage.FederationActivity{}
	r.costs = []*storage.FederationCost{}
}

// Ensure FederationRepository implements interfaces.FederationRepository
var _ interfaces.FederationRepository = (*FederationRepository)(nil)
