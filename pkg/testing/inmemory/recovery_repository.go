// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// RecoveryRepository is a thread-safe in-memory implementation of interfaces.RecoveryRepository.
type RecoveryRepository struct {
	mu sync.RWMutex

	// Trustees: key = "username:trusteeActorID"
	trustees map[string]*storage.TrusteeConfig

	// Recovery requests: key = requestID
	requests map[string]*storage.SocialRecoveryRequest

	// Recovery codes: key = "username:codeHash"
	codes map[string]*storage.RecoveryCodeItem

	// Recovery tokens: key = token key
	tokens map[string]map[string]any
}

// NewRecoveryRepository creates a new in-memory recovery repository
func NewRecoveryRepository() *RecoveryRepository {
	return &RecoveryRepository{
		trustees: make(map[string]*storage.TrusteeConfig),
		requests: make(map[string]*storage.SocialRecoveryRequest),
		codes:    make(map[string]*storage.RecoveryCodeItem),
		tokens:   make(map[string]map[string]any),
	}
}

// StoreTrustee stores a trustee configuration for social recovery
func (r *RecoveryRepository) StoreTrustee(_ context.Context, username string, trustee *storage.TrusteeConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + trustee.TrusteeActorID
	r.trustees[key] = trustee
	return nil
}

// GetTrustees retrieves all trustees for a user
func (r *RecoveryRepository) GetTrustees(_ context.Context, username string) ([]*storage.TrusteeConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	var result []*storage.TrusteeConfig
	for key, trustee := range r.trustees {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, trustee)
		}
	}
	return result, nil
}

// DeleteTrustee removes a trustee
func (r *RecoveryRepository) DeleteTrustee(_ context.Context, username, trusteeActorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + trusteeActorID
	delete(r.trustees, key)
	return nil
}


// UpdateTrusteeConfirmed updates the confirmed status of a trustee
func (r *RecoveryRepository) UpdateTrusteeConfirmed(_ context.Context, username, trusteeActorID string, confirmed bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + trusteeActorID
	if trustee, exists := r.trustees[key]; exists {
		trustee.Confirmed = confirmed
	}
	return nil
}

// StoreRecoveryRequest stores a social recovery request
func (r *RecoveryRepository) StoreRecoveryRequest(_ context.Context, request *storage.SocialRecoveryRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests[request.RequestID] = request
	return nil
}

// GetRecoveryRequest retrieves a recovery request by ID
func (r *RecoveryRepository) GetRecoveryRequest(_ context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	request, exists := r.requests[requestID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return request, nil
}

// UpdateRecoveryRequest updates a recovery request
func (r *RecoveryRepository) UpdateRecoveryRequest(_ context.Context, request *storage.SocialRecoveryRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.requests[request.RequestID]; !exists {
		return storage.ErrNotFound
	}
	r.requests[request.RequestID] = request
	return nil
}

// DeleteRecoveryRequest deletes a recovery request
func (r *RecoveryRepository) DeleteRecoveryRequest(_ context.Context, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.requests, requestID)
	return nil
}

// GetActiveRecoveryRequests gets all active recovery requests for a user
func (r *RecoveryRepository) GetActiveRecoveryRequests(_ context.Context, username string) ([]*storage.SocialRecoveryRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.SocialRecoveryRequest
	for _, req := range r.requests {
		if req.Username == username && req.Status == "active" {
			result = append(result, req)
		}
	}
	return result, nil
}

// StoreRecoveryCode stores a recovery code
func (r *RecoveryRepository) StoreRecoveryCode(_ context.Context, username string, code *storage.RecoveryCodeItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + code.CodeHash
	r.codes[key] = code
	return nil
}


// GetRecoveryCodes retrieves all recovery codes for a user
func (r *RecoveryRepository) GetRecoveryCodes(_ context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	var result []*storage.RecoveryCodeItem
	for key, code := range r.codes {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, code)
		}
	}
	return result, nil
}

// MarkRecoveryCodeUsed marks a recovery code as used
func (r *RecoveryRepository) MarkRecoveryCodeUsed(_ context.Context, username, codeHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + codeHash
	if code, exists := r.codes[key]; exists {
		code.Used = true
	}
	return nil
}

// DeleteAllRecoveryCodes deletes all recovery codes for a user
func (r *RecoveryRepository) DeleteAllRecoveryCodes(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := username + ":"
	for key := range r.codes {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(r.codes, key)
		}
	}
	return nil
}

// CountUnusedRecoveryCodes counts how many unused recovery codes the user has
func (r *RecoveryRepository) CountUnusedRecoveryCodes(_ context.Context, username string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	count := 0
	for key, code := range r.codes {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix && !code.Used {
			count++
		}
	}
	return count, nil
}

// StoreRecoveryToken stores a generic recovery token with data
func (r *RecoveryRepository) StoreRecoveryToken(_ context.Context, key string, data map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tokens[key] = data
	return nil
}

// GetRecoveryToken retrieves a recovery token by key
func (r *RecoveryRepository) GetRecoveryToken(_ context.Context, key string) (map[string]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, exists := r.tokens[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return data, nil
}

// DeleteRecoveryToken deletes a recovery token
func (r *RecoveryRepository) DeleteRecoveryToken(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tokens, key)
	return nil
}

// Clear clears all data (test helper)
func (r *RecoveryRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.trustees = make(map[string]*storage.TrusteeConfig)
	r.requests = make(map[string]*storage.SocialRecoveryRequest)
	r.codes = make(map[string]*storage.RecoveryCodeItem)
	r.tokens = make(map[string]map[string]any)
}

// Ensure RecoveryRepository implements interfaces.RecoveryRepository
var _ interfaces.RecoveryRepository = (*RecoveryRepository)(nil)
