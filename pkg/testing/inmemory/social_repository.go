// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// SocialRepository is a thread-safe in-memory implementation of interfaces.SocialRepository.
type SocialRepository struct {
	mu sync.RWMutex

	// Blocks: key = "actor:blockedActor"
	blocks map[string]*storage.Block

	// Mutes: key = "actor:mutedActor"
	mutes map[string]*storage.Mute

	// Announces: key = "actor:object"
	announces map[string]*storage.Announce

	// Account pins: key = "username:pinnedActorID"
	accountPins map[string]*storage.AccountPin

	// Account notes: key = "username:targetActorID"
	accountNotes map[string]*storage.AccountNote

	// Status pins: key = "username:statusID"
	statusPins map[string]*storage.StatusPin
}

// NewSocialRepository creates a new in-memory social repository
func NewSocialRepository() *SocialRepository {
	return &SocialRepository{
		blocks:       make(map[string]*storage.Block),
		mutes:        make(map[string]*storage.Mute),
		announces:    make(map[string]*storage.Announce),
		accountPins:  make(map[string]*storage.AccountPin),
		accountNotes: make(map[string]*storage.AccountNote),
		statusPins:   make(map[string]*storage.StatusPin),
	}
}

// CreateBlock creates a new block relationship
func (r *SocialRepository) CreateBlock(_ context.Context, block *storage.Block) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := block.Actor + ":" + block.BlockedActorID
	r.blocks[key] = block
	return nil
}

// DeleteBlock removes a block relationship
func (r *SocialRepository) DeleteBlock(_ context.Context, actor, blockedActor string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := actor + ":" + blockedActor
	delete(r.blocks, key)
	return nil
}


// GetBlock retrieves a specific block relationship
func (r *SocialRepository) GetBlock(_ context.Context, actor, blockedActor string) (*storage.Block, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := actor + ":" + blockedActor
	block, exists := r.blocks[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return block, nil
}

// IsBlocked checks if targetActor is blocked by actor
func (r *SocialRepository) IsBlocked(_ context.Context, actor, targetActor string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := actor + ":" + targetActor
	_, exists := r.blocks[key]
	return exists, nil
}

// GetBlockedUsers returns a paginated list of actors blocked by the given actor
func (r *SocialRepository) GetBlockedUsers(_ context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := actor + ":"
	var result []*storage.Block
	for key, block := range r.blocks {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, block)
		}
	}
	return result, "", nil
}

// GetBlockedByUsers returns a paginated list of actors who have blocked the given actor
func (r *SocialRepository) GetBlockedByUsers(_ context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	suffix := ":" + actor
	var result []*storage.Block
	for key, block := range r.blocks {
		if len(key) > len(suffix) && key[len(key)-len(suffix):] == suffix {
			result = append(result, block)
		}
	}
	return result, "", nil
}

// CreateMute creates a new mute relationship
func (r *SocialRepository) CreateMute(_ context.Context, mute *storage.Mute) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := mute.Actor + ":" + mute.MutedActorID
	r.mutes[key] = mute
	return nil
}

// DeleteMute removes a mute relationship
func (r *SocialRepository) DeleteMute(_ context.Context, actor, mutedActor string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := actor + ":" + mutedActor
	delete(r.mutes, key)
	return nil
}

// GetMute retrieves a specific mute relationship
func (r *SocialRepository) GetMute(_ context.Context, actor, mutedActor string) (*storage.Mute, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := actor + ":" + mutedActor
	mute, exists := r.mutes[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return mute, nil
}


// IsMuted checks if targetActor is muted by actor
func (r *SocialRepository) IsMuted(_ context.Context, actor, targetActor string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := actor + ":" + targetActor
	_, exists := r.mutes[key]
	return exists, nil
}

// GetMutedUsers returns all actors muted by the given actor
func (r *SocialRepository) GetMutedUsers(_ context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := actor + ":"
	var result []*storage.Mute
	for key, mute := range r.mutes {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, mute)
		}
	}
	return result, "", nil
}

// CreateAnnounce creates a new Announce activity
func (r *SocialRepository) CreateAnnounce(_ context.Context, announce *storage.Announce) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := announce.Actor + ":" + announce.Object
	r.announces[key] = announce
	return nil
}

// DeleteAnnounce removes an Announce activity
func (r *SocialRepository) DeleteAnnounce(_ context.Context, actor, object string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := actor + ":" + object
	delete(r.announces, key)
	return nil
}

// GetAnnounce retrieves a specific Announce by actor and object
func (r *SocialRepository) GetAnnounce(_ context.Context, actor, object string) (*storage.Announce, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := actor + ":" + object
	announce, exists := r.announces[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return announce, nil
}

// GetStatusAnnounces retrieves all announces for a specific object
func (r *SocialRepository) GetStatusAnnounces(_ context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	suffix := ":" + objectID
	var result []*storage.Announce
	for key, announce := range r.announces {
		if len(key) > len(suffix) && key[len(key)-len(suffix):] == suffix {
			result = append(result, announce)
		}
	}
	return result, "", nil
}

// HasUserAnnounced checks if a user has announced a specific object
func (r *SocialRepository) HasUserAnnounced(_ context.Context, actor, object string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := actor + ":" + object
	_, exists := r.announces[key]
	return exists, nil
}


// GetActorAnnounces retrieves all objects announced by a specific actor with pagination
func (r *SocialRepository) GetActorAnnounces(_ context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := actorID + ":"
	var result []*storage.Announce
	for key, announce := range r.announces {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, announce)
		}
	}
	return result, "", nil
}

// CountObjectAnnounces returns the total number of announces for an object
func (r *SocialRepository) CountObjectAnnounces(_ context.Context, objectID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	suffix := ":" + objectID
	count := 0
	for key := range r.announces {
		if len(key) > len(suffix) && key[len(key)-len(suffix):] == suffix {
			count++
		}
	}
	return count, nil
}

// CascadeDeleteAnnounces deletes all announces for an object
func (r *SocialRepository) CascadeDeleteAnnounces(_ context.Context, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	suffix := ":" + objectID
	for key := range r.announces {
		if len(key) > len(suffix) && key[len(key)-len(suffix):] == suffix {
			delete(r.announces, key)
		}
	}
	return nil
}

// CreateAccountPin creates a new account pin (endorsed account)
func (r *SocialRepository) CreateAccountPin(_ context.Context, pin *storage.AccountPin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := pin.Username + ":" + pin.PinnedActorID
	r.accountPins[key] = pin
	return nil
}

// DeleteAccountPin deletes an account pin
func (r *SocialRepository) DeleteAccountPin(_ context.Context, username, pinnedActorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + pinnedActorID
	delete(r.accountPins, key)
	return nil
}

// GetAccountPins retrieves all pinned accounts for a user
func (r *SocialRepository) GetAccountPins(_ context.Context, username string) ([]*storage.AccountPin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	var result []*storage.AccountPin
	for key, pin := range r.accountPins {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, pin)
		}
	}
	return result, nil
}

// GetAccountPinsPaginated retrieves pinned accounts for a user with pagination
func (r *SocialRepository) GetAccountPinsPaginated(_ context.Context, username string, limit int, cursor string) ([]*storage.AccountPin, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	var result []*storage.AccountPin
	for key, pin := range r.accountPins {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, pin)
		}
	}
	return result, "", nil
}


// IsAccountPinned checks if an account is pinned
func (r *SocialRepository) IsAccountPinned(_ context.Context, username, pinnedActorID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := username + ":" + pinnedActorID
	_, exists := r.accountPins[key]
	return exists, nil
}

// CreateAccountNote creates a new private note on an account
func (r *SocialRepository) CreateAccountNote(_ context.Context, note *storage.AccountNote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := note.Username + ":" + note.TargetActorID
	r.accountNotes[key] = note
	return nil
}

// UpdateAccountNote updates an existing private note on an account
func (r *SocialRepository) UpdateAccountNote(_ context.Context, note *storage.AccountNote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := note.Username + ":" + note.TargetActorID
	r.accountNotes[key] = note
	return nil
}

// DeleteAccountNote deletes a private note on an account
func (r *SocialRepository) DeleteAccountNote(_ context.Context, username, targetActorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + targetActorID
	delete(r.accountNotes, key)
	return nil
}

// GetAccountNote retrieves a private note on an account
func (r *SocialRepository) GetAccountNote(_ context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := username + ":" + targetActorID
	note, exists := r.accountNotes[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return note, nil
}

// CreateStatusPin creates a new status pin
func (r *SocialRepository) CreateStatusPin(_ context.Context, pin *storage.StatusPin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := pin.Username + ":" + pin.StatusID
	r.statusPins[key] = pin
	return nil
}

// DeleteStatusPin deletes a status pin
func (r *SocialRepository) DeleteStatusPin(_ context.Context, username, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + statusID
	delete(r.statusPins, key)
	return nil
}

// GetStatusPins retrieves all pinned statuses for a user
func (r *SocialRepository) GetStatusPins(_ context.Context, username string) ([]*storage.StatusPin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	var result []*storage.StatusPin
	for key, pin := range r.statusPins {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, pin)
		}
	}
	return result, nil
}

// GetStatusPinsPaginated retrieves pinned statuses for a user with pagination
func (r *SocialRepository) GetStatusPinsPaginated(_ context.Context, username string, limit int, cursor string) ([]*storage.StatusPin, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	var result []*storage.StatusPin
	for key, pin := range r.statusPins {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, pin)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, "", nil
}

// IsStatusPinned checks if a status is pinned by a user
func (r *SocialRepository) IsStatusPinned(_ context.Context, username, statusID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := username + ":" + statusID
	_, exists := r.statusPins[key]
	return exists, nil
}

// ReorderStatusPins reorders pinned statuses
func (r *SocialRepository) ReorderStatusPins(_ context.Context, username string, statusIDs []string) error {
	// No-op for in-memory implementation
	return nil
}

// CountUserPinnedStatuses counts the number of pinned statuses for a user
func (r *SocialRepository) CountUserPinnedStatuses(_ context.Context, username string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := username + ":"
	count := 0
	for key := range r.statusPins {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			count++
		}
	}
	return count, nil
}

// Clear clears all data (test helper)
func (r *SocialRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.blocks = make(map[string]*storage.Block)
	r.mutes = make(map[string]*storage.Mute)
	r.announces = make(map[string]*storage.Announce)
	r.accountPins = make(map[string]*storage.AccountPin)
	r.accountNotes = make(map[string]*storage.AccountNote)
	r.statusPins = make(map[string]*storage.StatusPin)
}

// Ensure SocialRepository implements interfaces.SocialRepository
var _ interfaces.SocialRepository = (*SocialRepository)(nil)
