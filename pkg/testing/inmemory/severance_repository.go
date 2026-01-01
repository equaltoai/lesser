// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// SeveranceRepository is a thread-safe in-memory implementation of interfaces.SeveranceRepository.
type SeveranceRepository struct {
	mu sync.RWMutex

	// Severed relationships: id -> SeveredRelationship
	severances map[string]*models.SeveredRelationship

	// Severed relationships by local instance: localInstance -> []SeveredRelationship
	severancesByInstance map[string][]*models.SeveredRelationship

	// Affected relationships: severanceID -> []AffectedRelationship
	affectedBySeverance map[string][]*models.AffectedRelationship

	// Reconnection attempts: severanceID -> attemptID -> SeveranceReconnectionAttempt
	reconnectionAttempts map[string]map[string]*models.SeveranceReconnectionAttempt
}

// NewSeveranceRepository creates a new in-memory severance repository
func NewSeveranceRepository() *SeveranceRepository {
	return &SeveranceRepository{
		severances:           make(map[string]*models.SeveredRelationship),
		severancesByInstance: make(map[string][]*models.SeveredRelationship),
		affectedBySeverance:  make(map[string][]*models.AffectedRelationship),
		reconnectionAttempts: make(map[string]map[string]*models.SeveranceReconnectionAttempt),
	}
}

// ===== Severed Relationship Operations =====

// CreateSeveredRelationship creates a new severed relationship record
func (r *SeveranceRepository) CreateSeveredRelationship(_ context.Context, severance *models.SeveredRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.severances[severance.ID] = severance
	r.severancesByInstance[severance.LocalInstance] = append(r.severancesByInstance[severance.LocalInstance], severance)

	return nil
}

// GetSeveredRelationship retrieves a severed relationship by ID
func (r *SeveranceRepository) GetSeveredRelationship(_ context.Context, id string) (*models.SeveredRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	severance, exists := r.severances[id]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return severance, nil
}

// ListSeveredRelationships retrieves severed relationships with filters and pagination
func (r *SeveranceRepository) ListSeveredRelationships(_ context.Context, localInstance string, filters interfaces.SeveranceFilters, limit int, cursor string) ([]*models.SeveredRelationship, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	severances := r.severancesByInstance[localInstance]

	// Apply filters
	var filtered []*models.SeveredRelationship
	for _, s := range severances {
		if filters.Instance != "" && s.RemoteInstance != filters.Instance {
			continue
		}
		if filters.Status != "" && s.Status != filters.Status {
			continue
		}
		if filters.Reason != "" && s.Reason != filters.Reason {
			continue
		}
		filtered = append(filtered, s)
	}

	// Sort by created at descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	// Apply cursor
	startIdx := 0
	if cursor != "" {
		for i, s := range filtered {
			if s.SK == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(filtered) {
		endIdx = len(filtered)
	}

	result := filtered[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(filtered) && len(result) > 0 {
		nextCursor = result[len(result)-1].SK
	}

	return result, nextCursor, nil
}

// UpdateSeveranceStatus updates the status of a severed relationship
func (r *SeveranceRepository) UpdateSeveranceStatus(_ context.Context, id string, status models.SeveranceStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	severance, exists := r.severances[id]
	if !exists {
		return storage.ErrNotFound
	}

	severance.Status = status
	severance.UpdatedAt = time.Now()
	if status == models.SeveranceStatusAcknowledged {
		severance.Acknowledge()
	}

	return nil
}

// ===== Affected Relationship Operations =====

// CreateAffectedRelationship creates a new affected relationship record
func (r *SeveranceRepository) CreateAffectedRelationship(_ context.Context, affected *models.AffectedRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.affectedBySeverance[affected.SeveranceID] = append(r.affectedBySeverance[affected.SeveranceID], affected)
	return nil
}

// GetAffectedRelationships retrieves affected relationships for a severance
func (r *SeveranceRepository) GetAffectedRelationships(_ context.Context, severanceID string, limit int, cursor string) ([]*models.AffectedRelationship, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	affected := r.affectedBySeverance[severanceID]

	// Apply cursor
	startIdx := 0
	if cursor != "" {
		for i, a := range affected {
			if a.SK == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(affected) {
		endIdx = len(affected)
	}

	result := affected[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(affected) && len(result) > 0 {
		nextCursor = result[len(result)-1].SK
	}

	return result, nextCursor, nil
}

// ===== Reconnection Attempt Operations =====

// CreateReconnectionAttempt creates a new reconnection attempt record
func (r *SeveranceRepository) CreateReconnectionAttempt(_ context.Context, attempt *models.SeveranceReconnectionAttempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.reconnectionAttempts[attempt.SeveranceID] == nil {
		r.reconnectionAttempts[attempt.SeveranceID] = make(map[string]*models.SeveranceReconnectionAttempt)
	}
	r.reconnectionAttempts[attempt.SeveranceID][attempt.ID] = attempt

	return nil
}

// UpdateReconnectionAttempt updates a reconnection attempt record
func (r *SeveranceRepository) UpdateReconnectionAttempt(_ context.Context, attempt *models.SeveranceReconnectionAttempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.reconnectionAttempts[attempt.SeveranceID] == nil {
		return storage.ErrNotFound
	}
	if _, exists := r.reconnectionAttempts[attempt.SeveranceID][attempt.ID]; !exists {
		return storage.ErrNotFound
	}

	r.reconnectionAttempts[attempt.SeveranceID][attempt.ID] = attempt
	return nil
}

// GetReconnectionAttempt retrieves a reconnection attempt by ID
func (r *SeveranceRepository) GetReconnectionAttempt(_ context.Context, severanceID, attemptID string) (*models.SeveranceReconnectionAttempt, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	attempts := r.reconnectionAttempts[severanceID]
	if attempts == nil {
		return nil, storage.ErrNotFound
	}

	attempt, exists := attempts[attemptID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return attempt, nil
}

// GetReconnectionAttempts retrieves all reconnection attempts for a severance
func (r *SeveranceRepository) GetReconnectionAttempts(_ context.Context, severanceID string) ([]*models.SeveranceReconnectionAttempt, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	attempts := r.reconnectionAttempts[severanceID]
	if attempts == nil {
		return []*models.SeveranceReconnectionAttempt{}, nil
	}

	var result []*models.SeveranceReconnectionAttempt
	for _, attempt := range attempts {
		result = append(result, attempt)
	}

	// Sort by created at descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// Clear clears all data (test helper)
func (r *SeveranceRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.severances = make(map[string]*models.SeveredRelationship)
	r.severancesByInstance = make(map[string][]*models.SeveredRelationship)
	r.affectedBySeverance = make(map[string][]*models.AffectedRelationship)
	r.reconnectionAttempts = make(map[string]map[string]*models.SeveranceReconnectionAttempt)
}

// Ensure SeveranceRepository implements interfaces.SeveranceRepository
var _ interfaces.SeveranceRepository = (*SeveranceRepository)(nil)
