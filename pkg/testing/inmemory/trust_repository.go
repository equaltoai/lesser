// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// TrustRepository is a thread-safe in-memory implementation of interfaces.TrustRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type TrustRepository struct {
	mu sync.RWMutex

	// Trust relationships: key = "trusterID:trusteeID:category"
	relationships map[string]*storage.TrustRelationship

	// Index by truster: trusterID -> []relationshipKey
	byTruster map[string][]string

	// Index by trustee: trusteeID -> []relationshipKey
	byTrustee map[string][]string

	// Trust scores: key = "actorID:category"
	scores map[string]*storage.TrustScore

	// Trust updates: key = "actorID:eventID"
	updates map[string]*storage.TrustUpdate

	// Updates by actor: actorID -> []updateKey
	updatesByActor map[string][]string
}

// NewTrustRepository creates a new in-memory trust repository
func NewTrustRepository() *TrustRepository {
	return &TrustRepository{
		relationships:  make(map[string]*storage.TrustRelationship),
		byTruster:      make(map[string][]string),
		byTrustee:      make(map[string][]string),
		scores:         make(map[string]*storage.TrustScore),
		updates:        make(map[string]*storage.TrustUpdate),
		updatesByActor: make(map[string][]string),
	}
}

// relationshipKey generates a unique key for a trust relationship
func relationshipKey(trusterID, trusteeID, category string) string {
	return fmt.Sprintf("%s:%s:%s", trusterID, trusteeID, category)
}

// scoreKey generates a unique key for a trust score
func scoreKey(actorID, category string) string {
	return fmt.Sprintf("%s:%s", actorID, category)
}

// updateKey generates a unique key for a trust update
func updateKey(actorID, eventID string) string {
	return fmt.Sprintf("%s:%s", actorID, eventID)
}

// ===== Trust Relationship Operations =====

// CreateTrustRelationship creates or updates a trust relationship between two actors
func (r *TrustRepository) CreateTrustRelationship(_ context.Context, relationship *storage.TrustRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if relationship == nil {
		return fmt.Errorf("relationship cannot be nil")
	}

	key := relationshipKey(relationship.TrusterID, relationship.TrusteeID, string(relationship.Category))

	// Check if relationship already exists
	if _, exists := r.relationships[key]; exists {
		return storage.ErrAlreadyExists
	}

	// Store the relationship
	r.relationships[key] = relationship

	// Update indexes
	r.byTruster[relationship.TrusterID] = append(r.byTruster[relationship.TrusterID], key)
	r.byTrustee[relationship.TrusteeID] = append(r.byTrustee[relationship.TrusteeID], key)

	return nil
}

// GetTrustRelationship retrieves a specific trust relationship
func (r *TrustRepository) GetTrustRelationship(_ context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := relationshipKey(trusterID, trusteeID, category)
	relationship, exists := r.relationships[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return relationship, nil
}

// UpdateTrustRelationship updates an existing trust relationship
func (r *TrustRepository) UpdateTrustRelationship(_ context.Context, relationship *storage.TrustRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if relationship == nil {
		return fmt.Errorf("relationship cannot be nil")
	}

	key := relationshipKey(relationship.TrusterID, relationship.TrusteeID, string(relationship.Category))

	// Check if relationship exists
	if _, exists := r.relationships[key]; !exists {
		return storage.ErrNotFound
	}

	// Update the relationship
	relationship.Updated = time.Now()
	r.relationships[key] = relationship

	return nil
}

// DeleteTrustRelationship removes a trust relationship
func (r *TrustRepository) DeleteTrustRelationship(_ context.Context, trusterID, trusteeID, category string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := relationshipKey(trusterID, trusteeID, category)

	relationship, exists := r.relationships[key]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from main storage
	delete(r.relationships, key)

	// Remove from truster index
	r.byTruster[relationship.TrusterID] = removeTrustKeyFromSlice(r.byTruster[relationship.TrusterID], key)

	// Remove from trustee index
	r.byTrustee[relationship.TrusteeID] = removeTrustKeyFromSlice(r.byTrustee[relationship.TrusteeID], key)

	return nil
}

// GetTrustRelationships retrieves all trust relationships for a truster with pagination
func (r *TrustRepository) GetTrustRelationships(_ context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := r.byTruster[trusterID]
	if len(keys) == 0 {
		return []*storage.TrustRelationship{}, "", nil
	}

	// Sort keys for consistent pagination
	sortedKeys := make([]string, len(keys))
	copy(sortedKeys, keys)
	sort.Strings(sortedKeys)

	// Apply safe limit
	safeLimit := clampTrustLimit(limit)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, k := range sortedKeys {
			if k == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*storage.TrustRelationship
	var nextCursor string

	for i := startIdx; i < len(sortedKeys) && len(results) < safeLimit; i++ {
		if rel, exists := r.relationships[sortedKeys[i]]; exists {
			results = append(results, rel)
		}
	}

	// Set next cursor if there are more results
	if startIdx+safeLimit < len(sortedKeys) {
		nextCursor = sortedKeys[startIdx+safeLimit-1]
	}

	return results, nextCursor, nil
}

// GetTrustedByRelationships retrieves all relationships where the actor is trusted with pagination
func (r *TrustRepository) GetTrustedByRelationships(_ context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := r.byTrustee[trusteeID]
	if len(keys) == 0 {
		return []*storage.TrustRelationship{}, "", nil
	}

	// Sort keys for consistent pagination
	sortedKeys := make([]string, len(keys))
	copy(sortedKeys, keys)
	sort.Strings(sortedKeys)

	// Apply safe limit
	safeLimit := clampTrustLimit(limit)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, k := range sortedKeys {
			if k == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*storage.TrustRelationship
	var nextCursor string

	for i := startIdx; i < len(sortedKeys) && len(results) < safeLimit; i++ {
		if rel, exists := r.relationships[sortedKeys[i]]; exists {
			results = append(results, rel)
		}
	}

	// Set next cursor if there are more results
	if startIdx+safeLimit < len(sortedKeys) {
		nextCursor = sortedKeys[startIdx+safeLimit-1]
	}

	return results, nextCursor, nil
}

// GetAllTrustRelationships retrieves all trust relationships for admin visualization
func (r *TrustRepository) GetAllTrustRelationships(_ context.Context, limit int) ([]*storage.TrustRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	safeLimit := clampTrustLimit(limit)

	var results []*storage.TrustRelationship
	for _, rel := range r.relationships {
		results = append(results, rel)
		if len(results) >= safeLimit {
			break
		}
	}

	return results, nil
}

// ===== Trust Score Operations =====

// GetTrustScore retrieves a cached trust score or calculates it
func (r *TrustRepository) GetTrustScore(_ context.Context, actorID, category string) (*storage.TrustScore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := scoreKey(actorID, category)
	score, exists := r.scores[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	// Check if cache is expired
	if score.CacheTTL.Before(time.Now()) {
		return nil, storage.ErrNotFound
	}

	return score, nil
}

// UpdateTrustScore updates a cached trust score
func (r *TrustRepository) UpdateTrustScore(_ context.Context, score *storage.TrustScore) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if score == nil {
		return fmt.Errorf("score cannot be nil")
	}

	key := scoreKey(score.ActorID, string(score.Category))
	r.scores[key] = score

	return nil
}

// GetUserTrustScore retrieves the trust score for a user
func (r *TrustRepository) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	// Get general trust score
	score, err := r.GetTrustScore(ctx, userID, string(storage.TrustCategoryGeneral))
	if err != nil {
		if err == storage.ErrNotFound {
			return 0.5, nil // Default neutral score
		}
		return 0.0, err
	}

	return score.Score, nil
}

// ===== Trust Update Operations =====

// RecordTrustUpdate records a trust score update event
func (r *TrustRepository) RecordTrustUpdate(_ context.Context, update *storage.TrustUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if update == nil {
		return fmt.Errorf("update cannot be nil")
	}

	key := updateKey(update.ActorID, update.EventID)

	// Store the update
	r.updates[key] = update

	// Update index
	r.updatesByActor[update.ActorID] = append(r.updatesByActor[update.ActorID], key)

	return nil
}

// ===== Helper Functions =====

// removeTrustKeyFromSlice removes a string from a slice
func removeTrustKeyFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// clampTrustLimit ensures limit is within valid bounds
func clampTrustLimit(limit int) int {
	const defaultLimit = 20
	const maxLimit = 100

	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// ===== Test Helper Methods =====

// Clear clears all data (test helper)
func (r *TrustRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.relationships = make(map[string]*storage.TrustRelationship)
	r.byTruster = make(map[string][]string)
	r.byTrustee = make(map[string][]string)
	r.scores = make(map[string]*storage.TrustScore)
	r.updates = make(map[string]*storage.TrustUpdate)
	r.updatesByActor = make(map[string][]string)
}

// GetRelationshipCount returns the number of relationships (test helper)
func (r *TrustRepository) GetRelationshipCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.relationships)
}

// GetScoreCount returns the number of scores (test helper)
func (r *TrustRepository) GetScoreCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.scores)
}

// GetUpdateCount returns the number of updates (test helper)
func (r *TrustRepository) GetUpdateCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.updates)
}

// Ensure TrustRepository implements interfaces.TrustRepository
var _ interfaces.TrustRepository = (*TrustRepository)(nil)
