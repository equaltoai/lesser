// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// DomainBlockRepository is a thread-safe in-memory implementation of interfaces.DomainBlockRepository.
type DomainBlockRepository struct {
	mu sync.RWMutex

	// User domain blocks: key = "username:domain"
	userBlocks map[string]bool

	// Instance domain blocks: key = domain
	instanceBlocks map[string]*storage.InstanceDomainBlock

	// Instance domain blocks by ID: key = ID
	instanceBlocksByID map[string]*storage.InstanceDomainBlock

	// Email domain blocks: key = ID
	emailBlocks map[string]*storage.EmailDomainBlock

	// Domain allows: key = ID
	domainAllows map[string]*storage.DomainAllow
}

// NewDomainBlockRepository creates a new in-memory domain block repository
func NewDomainBlockRepository() *DomainBlockRepository {
	return &DomainBlockRepository{
		userBlocks:         make(map[string]bool),
		instanceBlocks:     make(map[string]*storage.InstanceDomainBlock),
		instanceBlocksByID: make(map[string]*storage.InstanceDomainBlock),
		emailBlocks:        make(map[string]*storage.EmailDomainBlock),
		domainAllows:       make(map[string]*storage.DomainAllow),
	}
}

// AddDomainBlock adds a domain to the user's block list
func (r *DomainBlockRepository) AddDomainBlock(_ context.Context, username, domain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + domain
	r.userBlocks[key] = true
	return nil
}

// RemoveDomainBlock removes a domain from the user's block list
func (r *DomainBlockRepository) RemoveDomainBlock(_ context.Context, username, domain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + domain
	delete(r.userBlocks, key)
	return nil
}


// GetUserDomainBlocks retrieves all domains blocked by a user
func (r *DomainBlockRepository) GetUserDomainBlocks(_ context.Context, username string, limit int, cursor string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	var domains []string
	for key := range r.userBlocks {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			domains = append(domains, key[len(prefix):])
		}
	}
	return domains, "", nil
}

// IsBlockedDomain checks if a domain is blocked by a user
func (r *DomainBlockRepository) IsBlockedDomain(_ context.Context, username, domain string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := username + ":" + domain
	return r.userBlocks[key], nil
}

// CreateInstanceDomainBlock creates an instance-level domain block
func (r *DomainBlockRepository) CreateInstanceDomainBlock(_ context.Context, block *storage.InstanceDomainBlock) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.instanceBlocks[block.Domain] = block
	r.instanceBlocksByID[block.ID] = block
	return nil
}

// GetInstanceDomainBlock retrieves a domain block by domain
func (r *DomainBlockRepository) GetInstanceDomainBlock(_ context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	block, exists := r.instanceBlocks[domain]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return block, nil
}

// GetInstanceDomainBlockByID retrieves a domain block by ID
func (r *DomainBlockRepository) GetInstanceDomainBlockByID(_ context.Context, id string) (*storage.InstanceDomainBlock, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	block, exists := r.instanceBlocksByID[id]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return block, nil
}

// ListInstanceDomainBlocks lists all instance domain blocks with pagination
func (r *DomainBlockRepository) ListInstanceDomainBlocks(_ context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.InstanceDomainBlock
	for _, block := range r.instanceBlocks {
		result = append(result, block)
	}
	return result, "", nil
}


// UpdateInstanceDomainBlock updates an existing domain block
func (r *DomainBlockRepository) UpdateInstanceDomainBlock(_ context.Context, domain string, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	block, exists := r.instanceBlocks[domain]
	if !exists {
		return storage.ErrNotFound
	}
	if severity, ok := updates["severity"].(string); ok {
		block.Severity = severity
	}
	return nil
}

// DeleteInstanceDomainBlock deletes a domain block
func (r *DomainBlockRepository) DeleteInstanceDomainBlock(_ context.Context, domain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if block, exists := r.instanceBlocks[domain]; exists {
		delete(r.instanceBlocksByID, block.ID)
	}
	delete(r.instanceBlocks, domain)
	return nil
}

// IsInstanceDomainBlocked checks if a domain is blocked at the instance level
func (r *DomainBlockRepository) IsInstanceDomainBlocked(_ context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	block, exists := r.instanceBlocks[domain]
	return exists, block, nil
}

// GetDomainBlocks retrieves instance-level domain blocks with pagination (alias)
func (r *DomainBlockRepository) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	return r.ListInstanceDomainBlocks(ctx, limit, cursor)
}

// GetDomainBlock retrieves a specific domain block by ID (alias)
func (r *DomainBlockRepository) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	return r.GetInstanceDomainBlockByID(ctx, id)
}

// CreateDomainBlock creates a new instance-level domain block (alias)
func (r *DomainBlockRepository) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	return r.CreateInstanceDomainBlock(ctx, block)
}

// UpdateDomainBlock updates an existing domain block (alias)
func (r *DomainBlockRepository) UpdateDomainBlock(_ context.Context, id string, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	block, exists := r.instanceBlocksByID[id]
	if !exists {
		return storage.ErrNotFound
	}
	if severity, ok := updates["severity"].(string); ok {
		block.Severity = severity
	}
	return nil
}

// DeleteDomainBlock removes a domain block (alias)
func (r *DomainBlockRepository) DeleteDomainBlock(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	block, exists := r.instanceBlocksByID[id]
	if !exists {
		return nil
	}
	delete(r.instanceBlocks, block.Domain)
	delete(r.instanceBlocksByID, id)
	return nil
}


// IsDomainBlocked checks if a domain is blocked at the instance level (alias)
func (r *DomainBlockRepository) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	return r.IsInstanceDomainBlocked(ctx, domain)
}

// CreateEmailDomainBlock creates an email domain block
func (r *DomainBlockRepository) CreateEmailDomainBlock(_ context.Context, block *storage.EmailDomainBlock) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.emailBlocks[block.ID] = block
	return nil
}

// GetEmailDomainBlocks retrieves email domain blocks with pagination
func (r *DomainBlockRepository) GetEmailDomainBlocks(_ context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.EmailDomainBlock
	for _, block := range r.emailBlocks {
		result = append(result, block)
	}
	return result, "", nil
}

// DeleteEmailDomainBlock deletes an email domain block
func (r *DomainBlockRepository) DeleteEmailDomainBlock(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.emailBlocks, id)
	return nil
}

// GetDomainAllows retrieves domain allows (for allowlist mode)
func (r *DomainBlockRepository) GetDomainAllows(_ context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.DomainAllow
	for _, allow := range r.domainAllows {
		result = append(result, allow)
	}
	return result, "", nil
}

// CreateDomainAllow adds a domain to the allowlist
func (r *DomainBlockRepository) CreateDomainAllow(_ context.Context, allow *storage.DomainAllow) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.domainAllows[allow.ID] = allow
	return nil
}

// DeleteDomainAllow removes a domain from the allowlist
func (r *DomainBlockRepository) DeleteDomainAllow(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.domainAllows, id)
	return nil
}

// Clear clears all data (test helper)
func (r *DomainBlockRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.userBlocks = make(map[string]bool)
	r.instanceBlocks = make(map[string]*storage.InstanceDomainBlock)
	r.instanceBlocksByID = make(map[string]*storage.InstanceDomainBlock)
	r.emailBlocks = make(map[string]*storage.EmailDomainBlock)
	r.domainAllows = make(map[string]*storage.DomainAllow)
}

// Ensure DomainBlockRepository implements interfaces.DomainBlockRepository
var _ interfaces.DomainBlockRepository = (*DomainBlockRepository)(nil)
