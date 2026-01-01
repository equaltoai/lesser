// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// ActorRepository is a thread-safe in-memory implementation of interfaces.ActorRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type ActorRepository struct {
	mu sync.RWMutex

	// Core actor data
	actors            map[string]*actorEntry // keyed by username
	actorsByNumericID map[string]string      // numericID -> username mapping

	// Remote actor cache
	remoteActors map[string]*remoteActorEntry // keyed by handle

	// Dismissed suggestions
	dismissedSuggestions map[string]map[string]bool // userID -> targetID -> dismissed

	// Search indexes
	usernameIndex map[string][]string // prefix -> usernames
	nameIndex     map[string][]string // prefix -> usernames
}

// actorEntry stores an actor with its metadata
type actorEntry struct {
	actor      *activitypub.Actor
	privateKey string
	metadata   *storage.ActorMetadata
	numericID  string
	fields     []storage.ActorField
}

// remoteActorEntry stores a cached remote actor
type remoteActorEntry struct {
	actor     *activitypub.Actor
	expiresAt time.Time
}

// NewActorRepository creates a new in-memory actor repository
func NewActorRepository() *ActorRepository {
	return &ActorRepository{
		actors:               make(map[string]*actorEntry),
		actorsByNumericID:    make(map[string]string),
		remoteActors:         make(map[string]*remoteActorEntry),
		dismissedSuggestions: make(map[string]map[string]bool),
		usernameIndex:        make(map[string][]string),
		nameIndex:            make(map[string][]string),
	}
}

// Core actor operations

// CreateActor creates a new actor
func (r *ActorRepository) CreateActor(_ context.Context, actor *activitypub.Actor, privateKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if actor == nil {
		return fmt.Errorf("actor is required")
	}

	username := actor.PreferredUsername
	if username == "" {
		return fmt.Errorf("actor preferred username is required")
	}

	if _, exists := r.actors[username]; exists {
		return storage.ErrAlreadyExists
	}

	// Generate a numeric ID
	numericID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Make a copy to avoid external mutations
	actorCopy := copyActorPub(actor)
	now := time.Now()

	entry := &actorEntry{
		actor:      actorCopy,
		privateKey: privateKey,
		numericID:  numericID,
		metadata: &storage.ActorMetadata{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	r.actors[username] = entry
	r.actorsByNumericID[numericID] = username

	// Update search indexes
	r.updateSearchIndexes(username, actor.Name)

	return nil
}

// GetActor retrieves an actor by username
func (r *ActorRepository) GetActor(_ context.Context, username string) (*activitypub.Actor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.actors[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyActorPub(entry.actor), nil
}

// GetActorByUsername retrieves an actor by username (alias for GetActor)
func (r *ActorRepository) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	return r.GetActor(ctx, username)
}

// GetActorByNumericID retrieves an actor by numeric ID
func (r *ActorRepository) GetActorByNumericID(_ context.Context, numericID string) (*activitypub.Actor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	username, exists := r.actorsByNumericID[numericID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	entry, exists := r.actors[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyActorPub(entry.actor), nil
}

// GetActorWithMetadata retrieves an actor with metadata
func (r *ActorRepository) GetActorWithMetadata(_ context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.actors[username]
	if !exists {
		return nil, nil, storage.ErrNotFound
	}

	actorCopy := copyActorPub(entry.actor)
	metadataCopy := &storage.ActorMetadata{
		CreatedAt:    entry.metadata.CreatedAt,
		UpdatedAt:    entry.metadata.UpdatedAt,
		LastStatusAt: entry.metadata.LastStatusAt,
		Fields:       copyActorFields(entry.fields),
	}

	return actorCopy, metadataCopy, nil
}

// GetActorPrivateKey retrieves an actor's private key
func (r *ActorRepository) GetActorPrivateKey(_ context.Context, username string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.actors[username]
	if !exists {
		return "", storage.ErrNotFound
	}

	return entry.privateKey, nil
}

// UpdateActor updates an existing actor
func (r *ActorRepository) UpdateActor(_ context.Context, actor *activitypub.Actor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if actor == nil {
		return fmt.Errorf("actor is required")
	}

	username := actor.PreferredUsername
	entry, exists := r.actors[username]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove old search indexes
	r.removeSearchIndexes(username, entry.actor.Name)

	// Update actor
	entry.actor = copyActorPub(actor)
	entry.metadata.UpdatedAt = time.Now()

	// Update search indexes
	r.updateSearchIndexes(username, actor.Name)

	return nil
}

// UpdateActorLastStatusTime updates the last status timestamp
func (r *ActorRepository) UpdateActorLastStatusTime(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.actors[username]
	if !exists {
		return storage.ErrNotFound
	}

	now := time.Now()
	entry.metadata.LastStatusAt = &now
	entry.metadata.UpdatedAt = now

	return nil
}

// SetActorFields updates the profile fields for an actor
func (r *ActorRepository) SetActorFields(_ context.Context, username string, fields []storage.ActorField) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.actors[username]
	if !exists {
		return storage.ErrNotFound
	}

	entry.fields = copyActorFields(fields)
	entry.metadata.Fields = copyActorFields(fields)
	entry.metadata.UpdatedAt = time.Now()

	return nil
}

// DeleteActor deletes an actor
func (r *ActorRepository) DeleteActor(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.actors[username]
	if !exists {
		return storage.ErrNotFound
	}

	// Clean up indexes
	delete(r.actorsByNumericID, entry.numericID)
	r.removeSearchIndexes(username, entry.actor.Name)
	delete(r.actors, username)

	return nil
}

// Search and discovery

// SearchAccounts searches for actors by username or display name
func (r *ActorRepository) SearchAccounts(_ context.Context, query string, limit int, _ bool, _ int) ([]*activitypub.Actor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if query == "" {
		return []*activitypub.Actor{}, nil
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if len(normalizedQuery) < 2 {
		return []*activitypub.Actor{}, nil
	}

	seen := make(map[string]bool)
	var results []*activitypub.Actor

	// Search by username prefix
	prefix := normalizedQuery[:2]
	results = r.searchByUsernamePrefix(prefix, normalizedQuery, limit, seen, results)

	// Search by display name prefix if not enough results
	if len(results) < limit {
		results = r.searchByNamePrefix(prefix, normalizedQuery, limit, seen, results)
	}

	return results, nil
}

// searchByUsernamePrefix searches actors by username prefix
func (r *ActorRepository) searchByUsernamePrefix(prefix, normalizedQuery string, limit int, seen map[string]bool, results []*activitypub.Actor) []*activitypub.Actor {
	for _, username := range r.usernameIndex[prefix] {
		if len(results) >= limit {
			break
		}
		if seen[username] {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(username), normalizedQuery) {
			continue
		}
		if entry, exists := r.actors[username]; exists {
			results = append(results, copyActorPub(entry.actor))
			seen[username] = true
		}
	}
	return results
}

// searchByNamePrefix searches actors by display name prefix
func (r *ActorRepository) searchByNamePrefix(prefix, normalizedQuery string, limit int, seen map[string]bool, results []*activitypub.Actor) []*activitypub.Actor {
	for _, username := range r.nameIndex[prefix] {
		if len(results) >= limit {
			break
		}
		if seen[username] {
			continue
		}
		entry, exists := r.actors[username]
		if !exists {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(entry.actor.Name), normalizedQuery) {
			continue
		}
		results = append(results, copyActorPub(entry.actor))
		seen[username] = true
	}
	return results
}

// GetSearchSuggestions returns search suggestions for autocomplete
func (r *ActorRepository) GetSearchSuggestions(_ context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(prefix) < 2 {
		return []storage.SearchSuggestion{}, nil
	}

	normalizedPrefix := strings.ToLower(strings.TrimSpace(prefix))
	prefixKey := normalizedPrefix[:2]

	var suggestions []storage.SearchSuggestion
	seen := make(map[string]bool)

	for _, username := range r.usernameIndex[prefixKey] {
		if seen[username] {
			continue
		}
		if strings.HasPrefix(strings.ToLower(username), normalizedPrefix) {
			suggestions = append(suggestions, storage.SearchSuggestion{
				Type:  "account",
				Value: username,
				Score: 100,
			})
			seen[username] = true
		}
		if len(suggestions) >= 10 {
			break
		}
	}

	return suggestions, nil
}

// GetAccountSuggestions gets suggested accounts for a user
func (r *ActorRepository) GetAccountSuggestions(_ context.Context, _ string, limit int) ([]*activitypub.Actor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*activitypub.Actor
	for _, entry := range r.actors {
		if entry.actor.Discoverable {
			results = append(results, copyActorPub(entry.actor))
		}
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// Migration operations

// UpdateAlsoKnownAs updates the AlsoKnownAs field for an actor
func (r *ActorRepository) UpdateAlsoKnownAs(_ context.Context, username string, alsoKnownAs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.actors[username]
	if !exists {
		return storage.ErrNotFound
	}

	// Make a copy of the slice
	entry.actor.AlsoKnownAs = make([]string, len(alsoKnownAs))
	copy(entry.actor.AlsoKnownAs, alsoKnownAs)
	entry.metadata.UpdatedAt = time.Now()

	return nil
}

// UpdateMovedTo updates the MovedTo field for an actor
func (r *ActorRepository) UpdateMovedTo(_ context.Context, username string, movedTo string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.actors[username]
	if !exists {
		return storage.ErrNotFound
	}

	entry.actor.MovedTo = movedTo
	entry.metadata.UpdatedAt = time.Now()

	return nil
}

// CheckAlsoKnownAs checks if targetActorID is in the AlsoKnownAs slice for the given username
func (r *ActorRepository) CheckAlsoKnownAs(_ context.Context, username string, targetActorID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.actors[username]
	if !exists {
		return false, storage.ErrNotFound
	}

	if entry.actor == nil {
		return false, nil
	}

	for _, actorID := range entry.actor.AlsoKnownAs {
		if actorID == targetActorID {
			return true, nil
		}
	}

	return false, nil
}

// GetActorMigrationInfo returns migration information for an actor
func (r *ActorRepository) GetActorMigrationInfo(_ context.Context, username string) (*interfaces.MigrationInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.actors[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	if entry.actor == nil {
		return nil, fmt.Errorf("actor data is missing")
	}

	// Make a copy of the AlsoKnownAs slice
	var alsoKnownAs []string
	if entry.actor.AlsoKnownAs != nil {
		alsoKnownAs = make([]string, len(entry.actor.AlsoKnownAs))
		copy(alsoKnownAs, entry.actor.AlsoKnownAs)
	}

	return &interfaces.MigrationInfo{
		AlsoKnownAs: alsoKnownAs,
		MovedTo:     entry.actor.MovedTo,
	}, nil
}

// RemoveAccountSuggestion removes an account from suggestions for a user
func (r *ActorRepository) RemoveAccountSuggestion(_ context.Context, userID, targetID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.dismissedSuggestions[userID] == nil {
		r.dismissedSuggestions[userID] = make(map[string]bool)
	}
	r.dismissedSuggestions[userID][targetID] = true

	return nil
}

// GetCachedRemoteActor retrieves a cached remote actor by handle
func (r *ActorRepository) GetCachedRemoteActor(_ context.Context, handle string) (*activitypub.Actor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.remoteActors[handle]
	if !exists {
		return nil, storage.ErrNotFound
	}

	// Check if the cache has expired
	if time.Now().After(entry.expiresAt) {
		return nil, storage.ErrNotFound
	}

	return copyActorPub(entry.actor), nil
}

// SetCachedRemoteActor stores a remote actor in the cache (test helper)
func (r *ActorRepository) SetCachedRemoteActor(handle string, actor *activitypub.Actor, ttl time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.remoteActors[handle] = &remoteActorEntry{
		actor:     copyActorPub(actor),
		expiresAt: time.Now().Add(ttl),
	}
}

// Helper functions

// updateSearchIndexes adds an actor to search indexes
func (r *ActorRepository) updateSearchIndexes(username, displayName string) {
	// Index by username prefix
	lowerUsername := strings.ToLower(username)
	if len(lowerUsername) >= 2 {
		prefix := lowerUsername[:2]
		r.usernameIndex[prefix] = append(r.usernameIndex[prefix], username)
	}

	// Index by display name prefix
	if displayName != "" {
		lowerName := strings.ToLower(displayName)
		if len(lowerName) >= 2 {
			prefix := lowerName[:2]
			r.nameIndex[prefix] = append(r.nameIndex[prefix], username)
		}
	}
}

// removeSearchIndexes removes an actor from search indexes
func (r *ActorRepository) removeSearchIndexes(username, displayName string) {
	// Remove from username index
	lowerUsername := strings.ToLower(username)
	if len(lowerUsername) >= 2 {
		prefix := lowerUsername[:2]
		r.usernameIndex[prefix] = removeFromSlice(r.usernameIndex[prefix], username)
	}

	// Remove from name index
	if displayName != "" {
		lowerName := strings.ToLower(displayName)
		if len(lowerName) >= 2 {
			prefix := lowerName[:2]
			r.nameIndex[prefix] = removeFromSlice(r.nameIndex[prefix], username)
		}
	}
}

// removeFromSlice removes a string from a slice
func removeFromSlice(slice []string, item string) []string {
	for i, s := range slice {
		if s == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// copyActorPub creates a deep copy of an activitypub.Actor
func copyActorPub(actor *activitypub.Actor) *activitypub.Actor {
	if actor == nil {
		return nil
	}

	actorCopy := *actor

	// Copy slices
	if actor.AlsoKnownAs != nil {
		actorCopy.AlsoKnownAs = make([]string, len(actor.AlsoKnownAs))
		copy(actorCopy.AlsoKnownAs, actor.AlsoKnownAs)
	}

	return &actorCopy
}

// copyActorFields creates a deep copy of actor fields
func copyActorFields(fields []storage.ActorField) []storage.ActorField {
	if fields == nil {
		return nil
	}

	result := make([]storage.ActorField, len(fields))
	copy(result, fields)
	return result
}
