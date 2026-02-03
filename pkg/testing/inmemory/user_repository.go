// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
)

// UserRepository is a thread-safe in-memory implementation of interfaces.UserRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type UserRepository struct {
	mu sync.RWMutex

	// Core user data
	users        map[string]*storage.User // keyed by username
	usersByEmail map[string]string        // email -> username mapping

	// OAuth provider data
	providerAccounts map[string]map[string]string // username -> provider -> providerID
	providerToUser   map[string]string            // "provider:providerID" -> username

	// Account pins
	accountPins map[string][]*storage.AccountPin // username -> pins

	// Account notes
	accountNotes map[string]map[string]*storage.AccountNote // username -> targetActorID -> note

	// Reputation data
	reputations map[string][]*storage.Reputation // actorID -> reputation history (newest first)

	// Vouch data
	vouches         map[string]*storage.Vouch   // vouchID -> vouch
	vouchesByActor  map[string][]*storage.Vouch // actorID -> vouches given
	vouchesForActor map[string][]*storage.Vouch // actorID -> vouches received

	// Trust relationships
	trustRelationships map[string]*storage.TrustRelationship // "trusterID:trusteeID:category" -> relationship
	trustScores        map[string]*storage.TrustScore        // "actorID:category" -> score
	trustUpdates       []*storage.TrustUpdate

	// User preferences
	userPreferences map[string]*storage.UserPreferences // username -> preferences

	// Conversation mutes
	conversationMutes map[string]map[string]*storage.ConversationMute // username -> conversationID -> mute

	// Bookmarks
	bookmarks map[string][]string // username -> objectIDs

	// Timeline entries
	timelineEntries map[string][]*storage.TimelineEntry // "type:id" -> entries

	// Remote actor cache
	remoteActorCache map[string]*cachedActor // handle -> cached actor
}

// cachedActor represents a cached remote actor with expiration
type cachedActor struct {
	actor     *activitypub.Actor
	expiresAt time.Time
}

// NewUserRepository creates a new in-memory user repository
func NewUserRepository() *UserRepository {
	return &UserRepository{
		users:              make(map[string]*storage.User),
		usersByEmail:       make(map[string]string),
		providerAccounts:   make(map[string]map[string]string),
		providerToUser:     make(map[string]string),
		accountPins:        make(map[string][]*storage.AccountPin),
		accountNotes:       make(map[string]map[string]*storage.AccountNote),
		reputations:        make(map[string][]*storage.Reputation),
		vouches:            make(map[string]*storage.Vouch),
		vouchesByActor:     make(map[string][]*storage.Vouch),
		vouchesForActor:    make(map[string][]*storage.Vouch),
		trustRelationships: make(map[string]*storage.TrustRelationship),
		trustScores:        make(map[string]*storage.TrustScore),
		trustUpdates:       make([]*storage.TrustUpdate, 0),
		userPreferences:    make(map[string]*storage.UserPreferences),
		conversationMutes:  make(map[string]map[string]*storage.ConversationMute),
		bookmarks:          make(map[string][]string),
		timelineEntries:    make(map[string][]*storage.TimelineEntry),
		remoteActorCache:   make(map[string]*cachedActor),
	}
}

// Core CRUD operations

// CreateUser creates a new user
func (r *UserRepository) CreateUser(_ context.Context, user *storage.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[user.Username]; exists {
		return storage.ErrAlreadyExists
	}

	if user.Email != "" {
		if _, exists := r.usersByEmail[user.Email]; exists {
			return storage.ErrAlreadyExists
		}
		r.usersByEmail[user.Email] = user.Username
	}

	// Make a copy to avoid external mutations
	userCopy := *user
	if userCopy.CreatedAt.IsZero() {
		userCopy.CreatedAt = time.Now()
	}
	if userCopy.UpdatedAt.IsZero() {
		userCopy.UpdatedAt = userCopy.CreatedAt
	}
	r.users[user.Username] = &userCopy

	return nil
}

// GetUser retrieves a user by username
func (r *UserRepository) GetUser(_ context.Context, username string) (*storage.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	// Return a copy to avoid external mutations
	userCopy := *user
	return &userCopy, nil
}

// GetUserByEmail retrieves a user by email
func (r *UserRepository) GetUserByEmail(_ context.Context, email string) (*storage.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	username, exists := r.usersByEmail[email]
	if !exists {
		return nil, storage.ErrNotFound
	}

	user, exists := r.users[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	userCopy := *user
	return &userCopy, nil
}

// UpdateUser updates an existing user
func (r *UserRepository) UpdateUser(_ context.Context, username string, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[username]
	if !exists {
		return storage.ErrNotFound
	}

	r.applyUserUpdates(user, username, updates)
	user.UpdatedAt = time.Now()
	return nil
}

// applyUserUpdates applies the update map to a user (must be called with lock held)
func (r *UserRepository) applyUserUpdates(user *storage.User, username string, updates map[string]any) {
	for key, value := range updates {
		r.applyUserUpdate(user, username, key, value)
	}
}

// applyUserUpdate applies a single update to a user (must be called with lock held)
func (r *UserRepository) applyUserUpdate(user *storage.User, username, key string, value any) {
	switch key {
	case "email":
		if v, ok := value.(string); ok {
			r.updateUserEmail(user, username, v)
		}
	case "display_name":
		if v, ok := value.(string); ok {
			user.DisplayName = v
		}
	case "approved":
		if v, ok := value.(bool); ok {
			user.Approved = v
		}
	case "suspended":
		if v, ok := value.(bool); ok {
			user.Suspended = v
		}
	case "silenced":
		if v, ok := value.(bool); ok {
			user.Silenced = v
		}
	case "role":
		if v, ok := value.(string); ok {
			user.Role = v
		}
	case "locale":
		if v, ok := value.(string); ok {
			user.Locale = v
		}
	}
}

// updateUserEmail updates the user's email and maintains the email index (must be called with lock held)
func (r *UserRepository) updateUserEmail(user *storage.User, username, newEmail string) {
	if user.Email != "" {
		delete(r.usersByEmail, user.Email)
	}
	user.Email = newEmail
	if newEmail != "" {
		r.usersByEmail[newEmail] = username
	}
}

// DeleteUser deletes a user
func (r *UserRepository) DeleteUser(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[username]
	if !exists {
		return storage.ErrNotFound
	}

	// Clean up email index
	if user.Email != "" {
		delete(r.usersByEmail, user.Email)
	}

	delete(r.users, username)
	return nil
}

// ListUsers lists users with pagination
func (r *UserRepository) ListUsers(_ context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect all users
	users := make([]*storage.User, 0, len(r.users))
	for _, user := range r.users {
		userCopy := *user
		users = append(users, &userCopy)
	}

	// Simple pagination (not sorted, just for testing)
	start := 0
	if cursor != "" {
		for i, u := range users {
			if u.Username == cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + int(limit)
	if end > len(users) {
		end = len(users)
	}

	result := users[start:end]
	nextCursor := ""
	if end < len(users) {
		nextCursor = users[end-1].Username
	}

	return result, nextCursor, nil
}

// ListAgents lists agent accounts with pagination.
func (r *UserRepository) ListAgents(_ context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*storage.User, 0)
	for _, user := range r.users {
		if !user.IsAgent {
			continue
		}
		userCopy := *user
		agents = append(agents, &userCopy)
	}

	start := 0
	if cursor != "" {
		for i, u := range agents {
			if u.Username == cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + int(limit)
	if end > len(agents) {
		end = len(agents)
	}

	result := agents[start:end]
	nextCursor := ""
	if end < len(agents) && len(result) > 0 {
		nextCursor = result[len(result)-1].Username
	}

	return result, nextCursor, nil
}

// ListUsersByRole lists users by role
func (r *UserRepository) ListUsersByRole(_ context.Context, role string) ([]*storage.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]*storage.User, 0)
	for _, user := range r.users {
		if user.Role == role {
			userCopy := *user
			users = append(users, &userCopy)
		}
	}

	return users, nil
}

// Count operations

// GetActiveUserCount returns the count of active users
func (r *UserRepository) GetActiveUserCount(_ context.Context, days int) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cutoff := time.Now().AddDate(0, 0, -days)
	count := int64(0)
	for _, user := range r.users {
		if user.UpdatedAt.After(cutoff) && !user.Suspended {
			count++
		}
	}
	return count, nil
}

// GetTotalUserCount returns the total count of users
func (r *UserRepository) GetTotalUserCount(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.users)), nil
}

// OAuth provider operations

// GetUserByProviderID gets a user by OAuth provider ID
func (r *UserRepository) GetUserByProviderID(_ context.Context, provider, providerID string) (*storage.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", provider, providerID)
	username, exists := r.providerToUser[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	user, exists := r.users[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	userCopy := *user
	return &userCopy, nil
}

// LinkProviderAccount links an OAuth provider account to a user
func (r *UserRepository) LinkProviderAccount(_ context.Context, username, provider, providerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[username]; !exists {
		return storage.ErrNotFound
	}

	key := fmt.Sprintf("%s:%s", provider, providerID)
	if _, exists := r.providerToUser[key]; exists {
		return storage.ErrAlreadyExists
	}

	if r.providerAccounts[username] == nil {
		r.providerAccounts[username] = make(map[string]string)
	}
	r.providerAccounts[username][provider] = providerID
	r.providerToUser[key] = username

	return nil
}

// UnlinkProviderAccount unlinks an OAuth provider account from a user
func (r *UserRepository) UnlinkProviderAccount(_ context.Context, username, provider string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	providers, exists := r.providerAccounts[username]
	if !exists {
		return storage.ErrNotFound
	}

	providerID, exists := providers[provider]
	if !exists {
		return storage.ErrNotFound
	}

	key := fmt.Sprintf("%s:%s", provider, providerID)
	delete(r.providerToUser, key)
	delete(providers, provider)

	return nil
}

// GetLinkedProviders gets all linked OAuth providers for a user
func (r *UserRepository) GetLinkedProviders(_ context.Context, username string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers, exists := r.providerAccounts[username]
	if !exists {
		return []string{}, nil
	}

	result := make([]string, 0, len(providers))
	for provider := range providers {
		result = append(result, provider)
	}
	return result, nil
}

// Account pins

// CreateAccountPin creates an account pin
func (r *UserRepository) CreateAccountPin(_ context.Context, pin *storage.AccountPin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already pinned
	pins := r.accountPins[pin.Username]
	for _, p := range pins {
		if p.PinnedActorID == pin.PinnedActorID {
			return storage.ErrAlreadyExists
		}
	}

	pinCopy := *pin
	if pinCopy.CreatedAt.IsZero() {
		pinCopy.CreatedAt = time.Now()
	}
	r.accountPins[pin.Username] = append(r.accountPins[pin.Username], &pinCopy)
	return nil
}

// DeleteAccountPin deletes an account pin
func (r *UserRepository) DeleteAccountPin(_ context.Context, username, pinnedActorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pins := r.accountPins[username]
	for i, p := range pins {
		if p.PinnedActorID == pinnedActorID {
			r.accountPins[username] = append(pins[:i], pins[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

// GetAccountPins gets all account pins for a user
func (r *UserRepository) GetAccountPins(_ context.Context, username string) ([]*storage.AccountPin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pins := r.accountPins[username]
	result := make([]*storage.AccountPin, len(pins))
	for i, p := range pins {
		pinCopy := *p
		result[i] = &pinCopy
	}
	return result, nil
}

// IsAccountPinned checks if an account is pinned
func (r *UserRepository) IsAccountPinned(_ context.Context, username, actorID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pins := r.accountPins[username]
	for _, p := range pins {
		if p.PinnedActorID == actorID {
			return true, nil
		}
	}
	return false, nil
}

// Account notes

// CreateAccountNote creates an account note
func (r *UserRepository) CreateAccountNote(_ context.Context, note *storage.AccountNote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.accountNotes[note.Username] == nil {
		r.accountNotes[note.Username] = make(map[string]*storage.AccountNote)
	}

	noteCopy := *note
	if noteCopy.CreatedAt.IsZero() {
		noteCopy.CreatedAt = time.Now()
	}
	if noteCopy.UpdatedAt.IsZero() {
		noteCopy.UpdatedAt = noteCopy.CreatedAt
	}
	r.accountNotes[note.Username][note.TargetActorID] = &noteCopy
	return nil
}

// GetAccountNote gets an account note
func (r *UserRepository) GetAccountNote(_ context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	notes, exists := r.accountNotes[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	note, exists := notes[targetActorID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	noteCopy := *note
	return &noteCopy, nil
}

// UpdateAccountNote updates an account note
func (r *UserRepository) UpdateAccountNote(_ context.Context, note *storage.AccountNote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.accountNotes[note.Username] == nil {
		r.accountNotes[note.Username] = make(map[string]*storage.AccountNote)
	}

	noteCopy := *note
	noteCopy.UpdatedAt = time.Now()
	r.accountNotes[note.Username][note.TargetActorID] = &noteCopy
	return nil
}

// DeleteAccountNote deletes an account note
func (r *UserRepository) DeleteAccountNote(_ context.Context, username, targetActorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	notes, exists := r.accountNotes[username]
	if !exists {
		return storage.ErrNotFound
	}

	if _, exists := notes[targetActorID]; !exists {
		return storage.ErrNotFound
	}

	delete(notes, targetActorID)
	return nil
}

// Reputation operations

// StoreReputation stores a reputation record
func (r *UserRepository) StoreReputation(_ context.Context, actorID string, reputation *storage.Reputation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	repCopy := *reputation
	// Prepend to keep newest first
	r.reputations[actorID] = append([]*storage.Reputation{&repCopy}, r.reputations[actorID]...)
	return nil
}

// GetReputation gets the latest reputation for an actor
func (r *UserRepository) GetReputation(_ context.Context, actorID string) (*storage.Reputation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reps := r.reputations[actorID]
	if len(reps) == 0 {
		return nil, storage.ErrNotFound
	}

	repCopy := *reps[0]
	return &repCopy, nil
}

// GetReputationHistory gets reputation history for an actor
func (r *UserRepository) GetReputationHistory(_ context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reps := r.reputations[actorID]
	if limit > 0 && limit < len(reps) {
		reps = reps[:limit]
	}

	result := make([]*storage.Reputation, len(reps))
	for i, rep := range reps {
		repCopy := *rep
		result[i] = &repCopy
	}
	return result, nil
}

// GetUserTrustScore gets the trust score for a user
func (r *UserRepository) GetUserTrustScore(_ context.Context, userID string) (float64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reps := r.reputations[userID]
	if len(reps) == 0 {
		return 0.0, nil
	}

	return float64(reps[0].TotalScore), nil
}

// Vouch operations

// CreateVouch creates a vouch
func (r *UserRepository) CreateVouch(_ context.Context, vouch *storage.Vouch) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if vouch.ID == "" {
		vouch.ID = fmt.Sprintf("vouch-%d", time.Now().UnixNano())
	}

	vouchCopy := *vouch
	r.vouches[vouch.ID] = &vouchCopy
	r.vouchesByActor[vouch.From] = append(r.vouchesByActor[vouch.From], &vouchCopy)
	r.vouchesForActor[vouch.To] = append(r.vouchesForActor[vouch.To], &vouchCopy)
	return nil
}

// GetVouch gets a vouch by ID
func (r *UserRepository) GetVouch(_ context.Context, vouchID string) (*storage.Vouch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vouch, exists := r.vouches[vouchID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	vouchCopy := *vouch
	return &vouchCopy, nil
}

// GetVouchesByActor gets vouches given by an actor
func (r *UserRepository) GetVouchesByActor(_ context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vouches := r.vouchesByActor[actorID]
	result := make([]*storage.Vouch, 0, len(vouches))
	for _, v := range vouches {
		if !activeOnly || v.Active {
			vouchCopy := *v
			result = append(result, &vouchCopy)
		}
	}
	return result, nil
}

// GetVouchesForActor gets vouches received by an actor
func (r *UserRepository) GetVouchesForActor(_ context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vouches := r.vouchesForActor[actorID]
	result := make([]*storage.Vouch, 0, len(vouches))
	for _, v := range vouches {
		if !activeOnly || v.Active {
			vouchCopy := *v
			result = append(result, &vouchCopy)
		}
	}
	return result, nil
}

// UpdateVouchStatus updates the status of a vouch
func (r *UserRepository) UpdateVouchStatus(_ context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	vouch, exists := r.vouches[vouchID]
	if !exists {
		return storage.ErrNotFound
	}

	vouch.Active = active
	vouch.Revoked = !active
	vouch.RevokedAt = revokedAt
	return nil
}

// GetMonthlyVouchCount gets the count of vouches in a month
func (r *UserRepository) GetMonthlyVouchCount(_ context.Context, actorID string, year int, month time.Month) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	count := 0
	for _, v := range r.vouchesByActor[actorID] {
		if v.CreatedAt.After(startOfMonth) && v.CreatedAt.Before(endOfMonth) {
			count++
		}
	}
	return count, nil
}

// Trust relationship operations

// CreateTrustRelationship creates a trust relationship
func (r *UserRepository) CreateTrustRelationship(_ context.Context, relationship *storage.TrustRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", relationship.TrusterID, relationship.TrusteeID, relationship.Category)
	relCopy := *relationship
	if relCopy.Created.IsZero() {
		relCopy.Created = time.Now()
	}
	relCopy.Updated = time.Now()
	r.trustRelationships[key] = &relCopy
	return nil
}

// GetTrustRelationship gets a trust relationship
func (r *UserRepository) GetTrustRelationship(_ context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s:%s", trusterID, trusteeID, category)
	rel, exists := r.trustRelationships[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	relCopy := *rel
	return &relCopy, nil
}

// UpdateTrustRelationship updates a trust relationship
func (r *UserRepository) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	return r.CreateTrustRelationship(ctx, relationship)
}

// DeleteTrustRelationship deletes a trust relationship
func (r *UserRepository) DeleteTrustRelationship(_ context.Context, trusterID, trusteeID, category string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", trusterID, trusteeID, category)
	if _, exists := r.trustRelationships[key]; !exists {
		return storage.ErrNotFound
	}

	delete(r.trustRelationships, key)
	return nil
}

// GetTrustRelationships gets trust relationships for a truster
func (r *UserRepository) GetTrustRelationships(_ context.Context, trusterID string, limit int, _ string) ([]*storage.TrustRelationship, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*storage.TrustRelationship, 0)
	for key, rel := range r.trustRelationships {
		if len(key) > len(trusterID) && key[:len(trusterID)+1] == trusterID+":" {
			relCopy := *rel
			result = append(result, &relCopy)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, "", nil
}

// GetTrustedByRelationships gets relationships where the actor is trusted
func (r *UserRepository) GetTrustedByRelationships(_ context.Context, trusteeID string, limit int, _ string) ([]*storage.TrustRelationship, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*storage.TrustRelationship, 0)
	for _, rel := range r.trustRelationships {
		if rel.TrusteeID == trusteeID {
			relCopy := *rel
			result = append(result, &relCopy)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, "", nil
}

// GetAllTrustRelationships gets all trust relationships
func (r *UserRepository) GetAllTrustRelationships(_ context.Context, limit int) ([]*storage.TrustRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*storage.TrustRelationship, 0, len(r.trustRelationships))
	for _, rel := range r.trustRelationships {
		relCopy := *rel
		result = append(result, &relCopy)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

// Trust score operations

// GetTrustScore gets a trust score
func (r *UserRepository) GetTrustScore(_ context.Context, actorID, category string) (*storage.TrustScore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", actorID, category)
	score, exists := r.trustScores[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	scoreCopy := *score
	return &scoreCopy, nil
}

// UpdateTrustScore updates a trust score
func (r *UserRepository) UpdateTrustScore(_ context.Context, score *storage.TrustScore) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s", score.ActorID, score.Category)
	scoreCopy := *score
	scoreCopy.LastCalculated = time.Now()
	r.trustScores[key] = &scoreCopy
	return nil
}

// RecordTrustUpdate records a trust update
func (r *UserRepository) RecordTrustUpdate(_ context.Context, update *storage.TrustUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	updateCopy := *update
	updateCopy.Timestamp = time.Now()
	r.trustUpdates = append(r.trustUpdates, &updateCopy)
	return nil
}

// User preferences operations

// GetUserLanguagePreference gets the language preference for a user
func (r *UserRepository) GetUserLanguagePreference(_ context.Context, username string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefs, exists := r.userPreferences[username]
	if !exists || prefs == nil {
		return "", nil
	}
	return prefs.Language, nil
}

// SetUserLanguagePreference sets the language preference for a user
func (r *UserRepository) SetUserLanguagePreference(_ context.Context, username string, language string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.userPreferences[username] == nil {
		r.userPreferences[username] = &storage.UserPreferences{}
	}
	r.userPreferences[username].Language = language
	return nil
}

// GetUserPreferences gets all preferences for a user
func (r *UserRepository) GetUserPreferences(_ context.Context, username string) (*storage.UserPreferences, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefs, exists := r.userPreferences[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	prefsCopy := *prefs
	return &prefsCopy, nil
}

// UpdateUserPreferences updates preferences for a user
func (r *UserRepository) UpdateUserPreferences(_ context.Context, username string, preferences *storage.UserPreferences) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefsCopy := *preferences
	r.userPreferences[username] = &prefsCopy
	return nil
}

// SetPreference sets a single preference for a user
func (r *UserRepository) SetPreference(_ context.Context, username, key string, value any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.userPreferences[username] == nil {
		r.userPreferences[username] = &storage.UserPreferences{
			Preferences: make(map[string]string),
		}
	}
	if r.userPreferences[username].Preferences == nil {
		r.userPreferences[username].Preferences = make(map[string]string)
	}
	r.userPreferences[username].Preferences[key] = fmt.Sprintf("%v", value)
	return nil
}

// GetPreference gets a single preference for a user
func (r *UserRepository) GetPreference(_ context.Context, username, key string) (any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefs, exists := r.userPreferences[username]
	if !exists || prefs == nil || prefs.Preferences == nil {
		return nil, storage.ErrNotFound
	}

	value, exists := prefs.Preferences[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return value, nil
}

// GetAllPreferences gets all preferences as a map for a user
func (r *UserRepository) GetAllPreferences(_ context.Context, username string) (map[string]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefs, exists := r.userPreferences[username]
	if !exists || prefs == nil || prefs.Preferences == nil {
		return make(map[string]any), nil
	}

	result := make(map[string]any, len(prefs.Preferences))
	for k, v := range prefs.Preferences {
		result[k] = v
	}
	return result, nil
}

// UpdatePreferences updates multiple preferences for a user
func (r *UserRepository) UpdatePreferences(_ context.Context, username string, preferences map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.userPreferences[username] == nil {
		r.userPreferences[username] = &storage.UserPreferences{
			Preferences: make(map[string]string),
		}
	}
	if r.userPreferences[username].Preferences == nil {
		r.userPreferences[username].Preferences = make(map[string]string)
	}

	for k, v := range preferences {
		r.userPreferences[username].Preferences[k] = fmt.Sprintf("%v", v)
	}
	return nil
}

// Follow operations

// followRequests stores follow request state: "follower:followed" -> state
var followRequests = make(map[string]string)

// AcceptFollow accepts a follow request
func (r *UserRepository) AcceptFollow(_ context.Context, followerUsername, followedUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s", followerUsername, followedUsername)
	followRequests[key] = "accepted"
	return nil
}

// RejectFollow rejects a follow request
func (r *UserRepository) RejectFollow(_ context.Context, followerUsername, followedUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s", followerUsername, followedUsername)
	followRequests[key] = "rejected"
	return nil
}

// GetFollowRequestState gets the state of a follow request
func (r *UserRepository) GetFollowRequestState(_ context.Context, followerID, targetID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", followerID, targetID)
	state, exists := followRequests[key]
	if !exists {
		return "", nil
	}
	return state, nil
}

// pendingFollowRequests stores pending follow requests: username -> []followerUsernames
var pendingFollowRequests = make(map[string][]string)

// GetPendingFollowRequests gets pending follow requests for a user
func (r *UserRepository) GetPendingFollowRequests(_ context.Context, username string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	requests := pendingFollowRequests[username]
	if limit > 0 && limit < len(requests) {
		return requests[:limit], requests[limit-1], nil
	}
	return requests, "", nil
}

// followers stores follower relationships: username -> []followerUsernames
var followers = make(map[string][]string)

// RemoveFromFollowers removes a follower from a user's followers list
func (r *UserRepository) RemoveFromFollowers(_ context.Context, username, followerUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	followerList := followers[username]
	for i, f := range followerList {
		if f == followerUsername {
			followers[username] = append(followerList[:i], followerList[i+1:]...)
			return nil
		}
	}
	return nil
}

// Conversation mute operations

// CreateConversationMute creates a conversation mute
func (r *UserRepository) CreateConversationMute(_ context.Context, mute *storage.ConversationMute) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conversationMutes[mute.Username] == nil {
		r.conversationMutes[mute.Username] = make(map[string]*storage.ConversationMute)
	}

	muteCopy := *mute
	r.conversationMutes[mute.Username][mute.ConversationID] = &muteCopy
	return nil
}

// DeleteConversationMute deletes a conversation mute
func (r *UserRepository) DeleteConversationMute(_ context.Context, username, conversationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mutes, exists := r.conversationMutes[username]
	if !exists {
		return storage.ErrNotFound
	}

	if _, exists := mutes[conversationID]; !exists {
		return storage.ErrNotFound
	}

	delete(mutes, conversationID)
	return nil
}

// IsConversationMuted checks if a conversation is muted
func (r *UserRepository) IsConversationMuted(_ context.Context, username, conversationID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mutes, exists := r.conversationMutes[username]
	if !exists {
		return false, nil
	}

	_, muted := mutes[conversationID]
	return muted, nil
}

// GetMutedConversations gets all muted conversations for a user
func (r *UserRepository) GetMutedConversations(_ context.Context, username string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mutes := r.conversationMutes[username]
	result := make([]string, 0, len(mutes))
	for conversationID := range mutes {
		result = append(result, conversationID)
	}
	return result, nil
}

// Notification operations

// notificationMutes stores notification mute state: "userID:targetID" -> muted
var notificationMutes = make(map[string]bool)

// IsNotificationMuted checks if notifications from a target are muted
func (r *UserRepository) IsNotificationMuted(_ context.Context, userID, targetID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", userID, targetID)
	return notificationMutes[key], nil
}

// Remote actor caching

// CacheRemoteActor caches a remote actor
func (r *UserRepository) CacheRemoteActor(_ context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.remoteActorCache[handle] = &cachedActor{
		actor:     actor,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// Bookmark operations

// CreateBookmark creates a bookmark
func (r *UserRepository) CreateBookmark(_ context.Context, username, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already bookmarked
	for _, id := range r.bookmarks[username] {
		if id == objectID {
			return storage.ErrAlreadyExists
		}
	}

	r.bookmarks[username] = append(r.bookmarks[username], objectID)
	return nil
}

// RemoveBookmark removes a bookmark
func (r *UserRepository) RemoveBookmark(_ context.Context, username, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	bookmarkList := r.bookmarks[username]
	for i, id := range bookmarkList {
		if id == objectID {
			r.bookmarks[username] = append(bookmarkList[:i], bookmarkList[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

// GetBookmarks gets bookmarks for a user
func (r *UserRepository) GetBookmarks(_ context.Context, username string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	bookmarkList := r.bookmarks[username]
	if limit > 0 && limit < len(bookmarkList) {
		return bookmarkList[:limit], bookmarkList[limit-1], nil
	}
	return bookmarkList, "", nil
}

// IsBookmarked checks if an object is bookmarked
func (r *UserRepository) IsBookmarked(_ context.Context, username, objectID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, id := range r.bookmarks[username] {
		if id == objectID {
			return true, nil
		}
	}
	return false, nil
}

// Timeline operations

// DeleteFromTimeline deletes an entry from a timeline
func (r *UserRepository) DeleteFromTimeline(_ context.Context, timelineType, timelineID, entryID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s", timelineType, timelineID)
	entries := r.timelineEntries[key]
	for i, entry := range entries {
		if entry.ObjectID == entryID {
			r.timelineEntries[key] = append(entries[:i], entries[i+1:]...)
			return nil
		}
	}
	return nil
}

// DeleteExpiredTimelineEntries deletes expired timeline entries
func (r *UserRepository) DeleteExpiredTimelineEntries(_ context.Context, before time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, entries := range r.timelineEntries {
		filtered := make([]*storage.TimelineEntry, 0, len(entries))
		for _, entry := range entries {
			if entry.CreatedAt.After(before) {
				filtered = append(filtered, entry)
			}
		}
		r.timelineEntries[key] = filtered
	}
	return nil
}

// GetDirectTimeline gets the direct message timeline for a user
func (r *UserRepository) GetDirectTimeline(_ context.Context, username string, limit int, _ string) ([]*storage.TimelineEntry, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("direct:%s", username)
	entries := r.timelineEntries[key]

	if limit > 0 && limit < len(entries) {
		return entries[:limit], entries[limit-1].ObjectID, nil
	}
	return entries, "", nil
}

// GetHashtagTimeline gets the timeline for a hashtag
func (r *UserRepository) GetHashtagTimeline(_ context.Context, hashtag string, _ bool, limit int, _ string) ([]*storage.TimelineEntry, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("hashtag:%s", hashtag)
	entries := r.timelineEntries[key]

	if limit > 0 && limit < len(entries) {
		return entries[:limit], entries[limit-1].ObjectID, nil
	}
	return entries, "", nil
}

// GetListTimeline gets the timeline for a list
func (r *UserRepository) GetListTimeline(_ context.Context, listID string, limit int, _ string) ([]*storage.TimelineEntry, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("list:%s", listID)
	entries := r.timelineEntries[key]

	if limit > 0 && limit < len(entries) {
		return entries[:limit], entries[limit-1].ObjectID, nil
	}
	return entries, "", nil
}

// Fan-out operations

// FanOutPost fans out a post to followers' timelines
func (r *UserRepository) FanOutPost(_ context.Context, _ *activitypub.Activity) error {
	// In-memory implementation is a no-op for fan-out
	// Real implementation would distribute to follower timelines
	return nil
}
