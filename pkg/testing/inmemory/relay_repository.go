// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// RelayRepository is a thread-safe in-memory implementation of interfaces.RelayRepository.
type RelayRepository struct {
	mu     sync.RWMutex
	relays map[string]*storage.RelayInfo
}

// NewRelayRepository creates a new in-memory relay repository
func NewRelayRepository() *RelayRepository {
	return &RelayRepository{
		relays: make(map[string]*storage.RelayInfo),
	}
}

// StoreRelayInfo stores relay information
func (r *RelayRepository) StoreRelayInfo(_ context.Context, relay *storage.RelayInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.relays[relay.URL] = relay
	return nil
}

// GetRelayInfo retrieves relay information
func (r *RelayRepository) GetRelayInfo(_ context.Context, relayURL string) (*storage.RelayInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	relay, exists := r.relays[relayURL]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return relay, nil
}

// RemoveRelayInfo removes relay information
func (r *RelayRepository) RemoveRelayInfo(_ context.Context, relayURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.relays, relayURL)
	return nil
}

// GetActiveRelays retrieves all active relays
func (r *RelayRepository) GetActiveRelays(_ context.Context) ([]*storage.RelayInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.RelayInfo
	for _, relay := range r.relays {
		if relay.Active {
			result = append(result, relay)
		}
	}
	return result, nil
}


// GetAllRelays retrieves all relays with pagination
func (r *RelayRepository) GetAllRelays(_ context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.RelayInfo
	for _, relay := range r.relays {
		result = append(result, relay)
	}

	// Simple pagination by cursor (URL)
	startIdx := 0
	if cursor != "" {
		for i, relay := range result {
			if relay.URL == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	if startIdx >= len(result) {
		return []*storage.RelayInfo{}, "", nil
	}

	endIdx := startIdx + limit
	if endIdx > len(result) {
		endIdx = len(result)
	}

	nextCursor := ""
	if endIdx < len(result) {
		nextCursor = result[endIdx-1].URL
	}

	return result[startIdx:endIdx], nextCursor, nil
}

// ListRelays retrieves all relays
func (r *RelayRepository) ListRelays(_ context.Context) ([]*storage.RelayInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.RelayInfo
	for _, relay := range r.relays {
		result = append(result, relay)
	}
	return result, nil
}

// UpdateRelayStatus updates the active status of a relay
func (r *RelayRepository) UpdateRelayStatus(_ context.Context, relayURL string, active bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	relay, exists := r.relays[relayURL]
	if !exists {
		return storage.ErrNotFound
	}
	relay.Active = active
	return nil
}

// UpdateRelayState updates multiple relay fields
func (r *RelayRepository) UpdateRelayState(_ context.Context, relayURL string, state storage.RelayState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	relay, exists := r.relays[relayURL]
	if !exists {
		return storage.ErrNotFound
	}
	relay.Active = state.Active
	return nil
}

// CreateRelay creates a new relay
func (r *RelayRepository) CreateRelay(ctx context.Context, relay *storage.RelayInfo) error {
	return r.StoreRelayInfo(ctx, relay)
}

// GetRelay retrieves a relay by URL
func (r *RelayRepository) GetRelay(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	return r.GetRelayInfo(ctx, relayURL)
}

// DeleteRelay removes a relay
func (r *RelayRepository) DeleteRelay(ctx context.Context, relayURL string) error {
	return r.RemoveRelayInfo(ctx, relayURL)
}

// Clear clears all data (test helper)
func (r *RelayRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.relays = make(map[string]*storage.RelayInfo)
}

// Ensure RelayRepository implements interfaces.RelayRepository
var _ interfaces.RelayRepository = (*RelayRepository)(nil)
