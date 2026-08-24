package inmemory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// UploadGrantRepository is a thread-safe in-memory implementation of
// interfaces.UploadGrantRepository. Consume is a compare-and-swap on the
// version, so a concurrent double-finalize race behaves exactly like the
// version-conditioned writer in the production repository.
type UploadGrantRepository struct {
	mu sync.Mutex

	// grantsByOwnerAndID: owner|grantID -> grant
	grants map[string]*models.UploadGrant
}

// NewUploadGrantRepository creates a new in-memory upload grant repository.
func NewUploadGrantRepository() *UploadGrantRepository {
	return &UploadGrantRepository{
		grants: make(map[string]*models.UploadGrant),
	}
}

// SeedUploadGrant inserts a grant without a create, so tests can set up an
// expired or pre-consumed grant deterministically.
func (r *UploadGrantRepository) SeedUploadGrant(grant *models.UploadGrant) {
	if r == nil || grant == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cloned := *grant
	r.grants[grantKey(cloned.Owner, cloned.GrantID)] = &cloned
}

// CreateUploadGrant persists a freshly minted grant, failing on a colliding ID.
func (r *UploadGrantRepository) CreateUploadGrant(_ context.Context, grant *models.UploadGrant) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if grant == nil || strings.TrimSpace(grant.GrantID) == "" || strings.TrimSpace(grant.Owner) == "" {
		return storage.ErrInvalidInput
	}
	if err := grant.UpdateKeys(); err != nil {
		return err
	}
	key := grantKey(grant.Owner, grant.GrantID)
	if _, exists := r.grants[key]; exists {
		return storage.ErrAlreadyExists
	}
	cloned := *grant
	r.grants[key] = &cloned
	return nil
}

// GetUploadGrant loads one grant within the owner's partition.
func (r *UploadGrantRepository) GetUploadGrant(_ context.Context, ownerID, grantID string) (*models.UploadGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ownerID = strings.TrimSpace(ownerID)
	grantID = strings.TrimSpace(grantID)
	if ownerID == "" || grantID == "" {
		return nil, storage.ErrInvalidInput
	}
	grant, exists := r.grants[grantKey(ownerID, grantID)]
	if !exists {
		return nil, storage.ErrNotFound
	}
	cloned := *grant
	return &cloned, nil
}

// ConsumeUploadGrant atomically transitions a MINTED grant to a terminal
// status, failing the loser of a concurrent consume with
// interfaces.ErrUploadGrantConsumed.
func (r *UploadGrantRepository) ConsumeUploadGrant(_ context.Context, grant *models.UploadGrant, status, failureReason string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if grant == nil || strings.TrimSpace(grant.GrantID) == "" {
		return storage.ErrInvalidInput
	}
	switch status {
	case models.UploadGrantStatusUsed, models.UploadGrantStatusFailedDigest:
	default:
		return storage.ErrInvalidInput
	}
	stored, exists := r.grants[grantKey(grant.Owner, grant.GrantID)]
	if !exists {
		return storage.ErrNotFound
	}
	// Compare-and-swap: the consume only wins when the stored row is still
	// MINTED at the version this caller observed.
	if !stored.IsMinted() || stored.Version != grant.Version {
		return errors.Join(interfaces.ErrUploadGrantConsumed, storage.ErrVersionConflict)
	}
	stored.Status = status
	stored.Version++
	switch status {
	case models.UploadGrantStatusUsed:
		usedAt := now.UTC()
		stored.UsedAt = &usedAt
		stored.FailedAt = nil
		stored.FailureReason = ""
	case models.UploadGrantStatusFailedDigest:
		failedAt := now.UTC()
		stored.FailedAt = &failedAt
		stored.FailureReason = strings.TrimSpace(failureReason)
		stored.UsedAt = nil
	}
	// Mirror the production repository: the caller's copy is updated in place.
	grant.Status = stored.Status
	grant.Version = stored.Version
	grant.UsedAt = stored.UsedAt
	grant.FailedAt = stored.FailedAt
	grant.FailureReason = stored.FailureReason
	return nil
}

// Grants returns a copy of all stored grants for direct assertions.
func (r *UploadGrantRepository) Grants() map[string]*models.UploadGrant {
	if r == nil {
		return map[string]*models.UploadGrant{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*models.UploadGrant, len(r.grants))
	for key, grant := range r.grants {
		cloned := *grant
		out[key] = &cloned
	}
	return out
}

func grantKey(owner, grantID string) string {
	return strings.TrimSpace(owner) + "|" + strings.TrimSpace(grantID)
}
