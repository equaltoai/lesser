// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// RelationshipRepository is a thread-safe in-memory implementation of interfaces.RelationshipRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type RelationshipRepository struct {
	mu sync.RWMutex

	// Follow relationships: follower -> following -> record
	relationships map[string]map[string]*models.RelationshipRecord

	// Reverse index: following -> follower -> record (for GetFollowers)
	reverseRelationships map[string]map[string]*models.RelationshipRecord

	// Block relationships: blocker -> blocked -> block
	blocks map[string]map[string]*storage.Block

	// Reverse block index: blocked -> blocker -> block
	reverseBlocks map[string]map[string]*storage.Block

	// Mute relationships: muter -> muted -> mute
	mutes map[string]map[string]*storage.Mute

	// Reverse mute index: muted -> muter -> mute
	reverseMutes map[string]map[string]*storage.Mute

	// Endorsements: endorser -> endorsed -> pin
	endorsements map[string]map[string]*storage.AccountPin

	// Relationship notes: user -> target -> note
	notes map[string]map[string]*storage.AccountNote

	// Moves: actor -> target -> move
	moves map[string]map[string]*storage.Move

	// Moves by target: target -> actor -> move
	movesByTarget map[string]map[string]*storage.Move

	// Collections: collection -> itemID -> item
	collections map[string]map[string]*storage.CollectionItem
}

// NewRelationshipRepository creates a new in-memory relationship repository
func NewRelationshipRepository() *RelationshipRepository {
	return &RelationshipRepository{
		relationships:        make(map[string]map[string]*models.RelationshipRecord),
		reverseRelationships: make(map[string]map[string]*models.RelationshipRecord),
		blocks:               make(map[string]map[string]*storage.Block),
		reverseBlocks:        make(map[string]map[string]*storage.Block),
		mutes:                make(map[string]map[string]*storage.Mute),
		reverseMutes:         make(map[string]map[string]*storage.Mute),
		endorsements:         make(map[string]map[string]*storage.AccountPin),
		notes:                make(map[string]map[string]*storage.AccountNote),
		moves:                make(map[string]map[string]*storage.Move),
		movesByTarget:        make(map[string]map[string]*storage.Move),
		collections:          make(map[string]map[string]*storage.CollectionItem),
	}
}

// ===== Core Follow Relationship Operations =====

// CreateRelationship creates a new follow relationship
func (r *RelationshipRepository) CreateRelationship(_ context.Context, followerUsername, followingUsername, activityID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize maps if needed
	if r.relationships[followerUsername] == nil {
		r.relationships[followerUsername] = make(map[string]*models.RelationshipRecord)
	}
	if r.reverseRelationships[followingUsername] == nil {
		r.reverseRelationships[followingUsername] = make(map[string]*models.RelationshipRecord)
	}

	// Check if relationship already exists
	if _, exists := r.relationships[followerUsername][followingUsername]; exists {
		return nil // Already exists, no error
	}

	now := time.Now()
	record := &models.RelationshipRecord{
		PK:         fmt.Sprintf("FOLLOW#%s", followerUsername),
		SK:         fmt.Sprintf("FOLLOWING#%s", followingUsername),
		GSI1PK:     fmt.Sprintf("FOLLOW#%s", followingUsername),
		GSI1SK:     fmt.Sprintf("FOLLOWER#%s", followerUsername),
		ActivityID: activityID,
		State:      models.RelationshipPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	r.relationships[followerUsername][followingUsername] = record
	r.reverseRelationships[followingUsername][followerUsername] = record

	return nil
}

// DeleteRelationship removes a follow relationship
func (r *RelationshipRepository) DeleteRelationship(_ context.Context, followerUsername, followingUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.relationships[followerUsername] != nil {
		delete(r.relationships[followerUsername], followingUsername)
	}
	if r.reverseRelationships[followingUsername] != nil {
		delete(r.reverseRelationships[followingUsername], followerUsername)
	}

	return nil
}

// GetRelationship retrieves a specific follow relationship
func (r *RelationshipRepository) GetRelationship(_ context.Context, followerUsername, followingUsername string) (*models.RelationshipRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.relationships[followerUsername] == nil {
		return nil, storage.ErrNotFound
	}

	record, exists := r.relationships[followerUsername][followingUsername]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyRelationshipRecord(record), nil
}

// UpdateRelationship updates relationship settings
func (r *RelationshipRepository) UpdateRelationship(_ context.Context, followerUsername, followingUsername string, updates map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.relationships[followerUsername] == nil {
		return storage.ErrNotFound
	}

	record, exists := r.relationships[followerUsername][followingUsername]
	if !exists {
		return storage.ErrNotFound
	}

	// Apply updates
	for key, value := range updates {
		switch key {
		case "state", "State":
			if v, ok := value.(string); ok {
				record.State = v
			}
		case "notifying", "Notifying":
			if v, ok := value.(bool); ok {
				record.Notifying = v
			}
		case "showing_reblogs", "ShowingReblogs":
			if v, ok := value.(bool); ok {
				record.ShowingReblogs = v
			}
		case "note", "Note":
			if v, ok := value.(string); ok {
				record.Note = v
			}
		}
	}

	record.UpdatedAt = time.Now()
	return nil
}

// IsFollowing checks if followerUsername is following the targetActorID
func (r *RelationshipRepository) IsFollowing(_ context.Context, followerUsername, targetActorID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.relationships[followerUsername] == nil {
		return false, nil
	}

	record, exists := r.relationships[followerUsername][targetActorID]
	if !exists {
		return false, nil
	}

	return record.State == models.RelationshipAccepted, nil
}

// ===== Follow Request Operations =====

// GetFollowRequest gets a follow request by follower and target IDs
func (r *RelationshipRepository) GetFollowRequest(_ context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.relationships[followerID] == nil {
		return nil, storage.ErrNotFound
	}

	record, exists := r.relationships[followerID][targetID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return &storage.RelationshipRecord{
		PK:         record.PK,
		SK:         record.SK,
		GSI1PK:     record.GSI1PK,
		GSI1SK:     record.GSI1SK,
		ActivityID: record.ActivityID,
		State:      record.State,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}, nil
}

// HasFollowRequest checks if there's a follow request between two users
func (r *RelationshipRepository) HasFollowRequest(_ context.Context, requesterID, targetID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.relationships[requesterID] == nil {
		return false, nil
	}

	_, exists := r.relationships[requesterID][targetID]
	return exists, nil
}

// HasPendingFollowRequest checks if there's a pending follow request between two users
func (r *RelationshipRepository) HasPendingFollowRequest(_ context.Context, requesterID, targetID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.relationships[requesterID] == nil {
		return false, nil
	}

	record, exists := r.relationships[requesterID][targetID]
	if !exists {
		return false, nil
	}

	return record.State == models.RelationshipPending, nil
}

// GetPendingFollowRequests retrieves pending follow requests for a user
func (r *RelationshipRepository) GetPendingFollowRequests(_ context.Context, username string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 40
	}

	var results []string
	if r.reverseRelationships[username] != nil {
		for follower, record := range r.reverseRelationships[username] {
			if record.State == models.RelationshipPending {
				results = append(results, follower)
				if len(results) >= limit {
					break
				}
			}
		}
	}

	return results, "", nil
}

// AcceptFollowRequest accepts a follow request
func (r *RelationshipRepository) AcceptFollowRequest(_ context.Context, followerUsername, followingUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.relationships[followerUsername] == nil {
		return storage.ErrNotFound
	}

	record, exists := r.relationships[followerUsername][followingUsername]
	if !exists {
		return storage.ErrNotFound
	}

	record.State = models.RelationshipAccepted
	record.UpdatedAt = time.Now()
	return nil
}

// RejectFollowRequest rejects a follow request
func (r *RelationshipRepository) RejectFollowRequest(_ context.Context, followerUsername, followingUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.relationships[followerUsername] == nil {
		return storage.ErrNotFound
	}

	record, exists := r.relationships[followerUsername][followingUsername]
	if !exists {
		return storage.ErrNotFound
	}

	record.State = models.RelationshipRejected
	record.UpdatedAt = time.Now()
	return nil
}

// ===== Follower/Following List Operations =====

// GetFollowers retrieves all followers for a user
func (r *RelationshipRepository) GetFollowers(_ context.Context, username string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 40
	}

	var results []string
	if r.reverseRelationships[username] != nil {
		for follower, record := range r.reverseRelationships[username] {
			if record.State == models.RelationshipAccepted {
				results = append(results, follower)
				if len(results) >= limit {
					break
				}
			}
		}
	}

	return results, "", nil
}

// GetFollowing retrieves all users that a user is following
func (r *RelationshipRepository) GetFollowing(_ context.Context, username string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 40
	}

	var results []string
	if r.relationships[username] != nil {
		for following, record := range r.relationships[username] {
			if record.State == models.RelationshipAccepted {
				results = append(results, following)
				if len(results) >= limit {
					break
				}
			}
		}
	}

	return results, "", nil
}

// CountFollowers returns the number of followers for a user
func (r *RelationshipRepository) CountFollowers(_ context.Context, username string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	if r.reverseRelationships[username] != nil {
		for _, record := range r.reverseRelationships[username] {
			if record.State == models.RelationshipAccepted {
				count++
			}
		}
	}

	return count, nil
}

// CountFollowing returns the number of users that a user is following
func (r *RelationshipRepository) CountFollowing(_ context.Context, username string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	if r.relationships[username] != nil {
		for _, record := range r.relationships[username] {
			if record.State == models.RelationshipAccepted {
				count++
			}
		}
	}

	return count, nil
}

// GetFollowerCount returns the number of followers for a user (int64 version)
func (r *RelationshipRepository) GetFollowerCount(ctx context.Context, userID string) (int64, error) {
	count, err := r.CountFollowers(ctx, userID)
	return int64(count), err
}

// GetFollowingCount returns the number of users that a user is following (int64 version)
func (r *RelationshipRepository) GetFollowingCount(ctx context.Context, userID string) (int64, error) {
	count, err := r.CountFollowing(ctx, userID)
	return int64(count), err
}

// CountRelationshipsByDomain counts follower/following relationships involving a remote domain
func (r *RelationshipRepository) CountRelationshipsByDomain(_ context.Context, _ string) (followers, following int, err error) {
	// Simplified implementation - in real usage this would filter by domain
	return 0, 0, nil
}

// Unfollow removes a follow relationship (wrapper for DeleteRelationship)
func (r *RelationshipRepository) Unfollow(ctx context.Context, followerID, followingID string) error {
	return r.DeleteRelationship(ctx, followerID, followingID)
}

// ===== Block Operations =====

// CreateBlock creates a new block relationship
func (r *RelationshipRepository) CreateBlock(_ context.Context, blockerActor, blockedActor, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.blocks[blockerActor] == nil {
		r.blocks[blockerActor] = make(map[string]*storage.Block)
	}
	if r.reverseBlocks[blockedActor] == nil {
		r.reverseBlocks[blockedActor] = make(map[string]*storage.Block)
	}

	now := time.Now()
	block := &storage.Block{
		ID:             fmt.Sprintf("block_%s_%s", blockerActor, blockedActor),
		Username:       blockerActor,
		BlockedActorID: blockedActor,
		CreatedAt:      now,
		Published:      now,
	}

	r.blocks[blockerActor][blockedActor] = block
	r.reverseBlocks[blockedActor][blockerActor] = block

	return nil
}

// DeleteBlock removes a block relationship
func (r *RelationshipRepository) DeleteBlock(_ context.Context, blockerActor, blockedActor string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.blocks[blockerActor] != nil {
		delete(r.blocks[blockerActor], blockedActor)
	}
	if r.reverseBlocks[blockedActor] != nil {
		delete(r.reverseBlocks[blockedActor], blockerActor)
	}

	return nil
}

// BlockUser blocks another user
func (r *RelationshipRepository) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	activityID := fmt.Sprintf("block_%s_%s_%d", blockerID, blockedID, time.Now().Unix())
	return r.CreateBlock(ctx, blockerID, blockedID, activityID)
}

// UnblockUser removes a block relationship
func (r *RelationshipRepository) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	return r.DeleteBlock(ctx, blockerID, blockedID)
}

// IsBlocked checks if one actor has blocked another
func (r *RelationshipRepository) IsBlocked(_ context.Context, blockerActor, blockedActor string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.blocks[blockerActor] == nil {
		return false, nil
	}

	_, exists := r.blocks[blockerActor][blockedActor]
	return exists, nil
}

// IsBlockedBidirectional checks if either actor has blocked the other
func (r *RelationshipRepository) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	blocked1, err := r.IsBlocked(ctx, actor1, actor2)
	if err != nil {
		return false, err
	}
	if blocked1 {
		return true, nil
	}

	return r.IsBlocked(ctx, actor2, actor1)
}

// GetBlockedUsers returns a list of users blocked by the given actor
func (r *RelationshipRepository) GetBlockedUsers(_ context.Context, blockerActor string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 40
	}

	var results []string
	if r.blocks[blockerActor] != nil {
		for blocked := range r.blocks[blockerActor] {
			results = append(results, blocked)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, "", nil
}

// GetUsersWhoBlocked returns a list of users who have blocked the given actor
func (r *RelationshipRepository) GetUsersWhoBlocked(_ context.Context, blockedActor string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 40
	}

	var results []string
	if r.reverseBlocks[blockedActor] != nil {
		for blocker := range r.reverseBlocks[blockedActor] {
			results = append(results, blocker)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, "", nil
}

// GetBlock retrieves a specific block relationship
func (r *RelationshipRepository) GetBlock(_ context.Context, blockerActor, blockedActor string) (*storage.Block, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.blocks[blockerActor] == nil {
		return nil, storage.ErrNotFound
	}

	block, exists := r.blocks[blockerActor][blockedActor]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyBlock(block), nil
}

// CountBlockedUsers returns the number of users blocked by the given actor
func (r *RelationshipRepository) CountBlockedUsers(_ context.Context, blockerActor string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.blocks[blockerActor] == nil {
		return 0, nil
	}

	return len(r.blocks[blockerActor]), nil
}

// CountUsersWhoBlocked returns the number of users who have blocked the given actor
func (r *RelationshipRepository) CountUsersWhoBlocked(_ context.Context, blockedActor string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.reverseBlocks[blockedActor] == nil {
		return 0, nil
	}

	return len(r.reverseBlocks[blockedActor]), nil
}

// ===== Mute Operations =====

// CreateMute creates a new mute relationship
func (r *RelationshipRepository) CreateMute(_ context.Context, muterActor, mutedActor, _ string, hideNotifications bool, duration *time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mutes[muterActor] == nil {
		r.mutes[muterActor] = make(map[string]*storage.Mute)
	}
	if r.reverseMutes[mutedActor] == nil {
		r.reverseMutes[mutedActor] = make(map[string]*storage.Mute)
	}

	now := time.Now()
	var expiresAt *time.Time
	if duration != nil {
		exp := now.Add(*duration)
		expiresAt = &exp
	}

	mute := &storage.Mute{
		ID:                fmt.Sprintf("mute_%s_%s", muterActor, mutedActor),
		Username:          muterActor,
		MutedActorID:      mutedActor,
		HideNotifications: hideNotifications,
		CreatedAt:         now,
		Published:         now,
		ExpiresAt:         expiresAt,
	}

	r.mutes[muterActor][mutedActor] = mute
	r.reverseMutes[mutedActor][muterActor] = mute

	return nil
}

// DeleteMute removes a mute relationship
func (r *RelationshipRepository) DeleteMute(_ context.Context, muterActor, mutedActor string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mutes[muterActor] != nil {
		delete(r.mutes[muterActor], mutedActor)
	}
	if r.reverseMutes[mutedActor] != nil {
		delete(r.reverseMutes[mutedActor], muterActor)
	}

	return nil
}

// UnmuteUser removes a mute relationship
func (r *RelationshipRepository) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	return r.DeleteMute(ctx, muterID, mutedID)
}

// IsMuted checks if one actor has muted another
func (r *RelationshipRepository) IsMuted(_ context.Context, muterActor, mutedActor string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.mutes[muterActor] == nil {
		return false, nil
	}

	mute, exists := r.mutes[muterActor][mutedActor]
	if !exists {
		return false, nil
	}

	// Check if mute has expired
	if mute.ExpiresAt != nil && time.Now().After(*mute.ExpiresAt) {
		return false, nil
	}

	return true, nil
}

// GetMutedUsers returns a list of users muted by the given actor
func (r *RelationshipRepository) GetMutedUsers(_ context.Context, muterActor string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 40
	}

	var results []string
	now := time.Now()
	if r.mutes[muterActor] != nil {
		for muted, mute := range r.mutes[muterActor] {
			// Skip expired mutes
			if mute.ExpiresAt != nil && now.After(*mute.ExpiresAt) {
				continue
			}
			results = append(results, muted)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, "", nil
}

// GetUsersWhoMuted returns a list of users who have muted the given actor
func (r *RelationshipRepository) GetUsersWhoMuted(_ context.Context, mutedActor string, limit int, _ string) ([]string, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 40
	}

	var results []string
	now := time.Now()
	if r.reverseMutes[mutedActor] != nil {
		for muter, mute := range r.reverseMutes[mutedActor] {
			// Skip expired mutes
			if mute.ExpiresAt != nil && now.After(*mute.ExpiresAt) {
				continue
			}
			results = append(results, muter)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, "", nil
}

// GetMute retrieves a specific mute relationship
func (r *RelationshipRepository) GetMute(_ context.Context, muterActor, mutedActor string) (*storage.Mute, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.mutes[muterActor] == nil {
		return nil, storage.ErrNotFound
	}

	mute, exists := r.mutes[muterActor][mutedActor]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyMute(mute), nil
}

// CountMutedUsers returns the number of users muted by the given actor
func (r *RelationshipRepository) CountMutedUsers(_ context.Context, muterActor string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.mutes[muterActor] == nil {
		return 0, nil
	}

	count := 0
	now := time.Now()
	for _, mute := range r.mutes[muterActor] {
		if mute.ExpiresAt == nil || now.Before(*mute.ExpiresAt) {
			count++
		}
	}

	return count, nil
}

// CountUsersWhoMuted returns the number of users who have muted the given actor
func (r *RelationshipRepository) CountUsersWhoMuted(_ context.Context, mutedActor string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.reverseMutes[mutedActor] == nil {
		return 0, nil
	}

	count := 0
	now := time.Now()
	for _, mute := range r.reverseMutes[mutedActor] {
		if mute.ExpiresAt == nil || now.Before(*mute.ExpiresAt) {
			count++
		}
	}

	return count, nil
}

// ===== Endorsement Operations =====

// IsEndorsed checks if a user has endorsed (pinned) a target account
func (r *RelationshipRepository) IsEndorsed(_ context.Context, userID, targetID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.endorsements[userID] == nil {
		return false, nil
	}

	_, exists := r.endorsements[userID][targetID]
	return exists, nil
}

// CreateEndorsement creates a new endorsement (account pin) relationship
func (r *RelationshipRepository) CreateEndorsement(_ context.Context, endorsement *storage.AccountPin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if endorsement == nil {
		return fmt.Errorf("endorsement is required")
	}

	if r.endorsements[endorsement.Username] == nil {
		r.endorsements[endorsement.Username] = make(map[string]*storage.AccountPin)
	}

	// Check endorsement limit (typically 4)
	if len(r.endorsements[endorsement.Username]) >= 4 {
		return fmt.Errorf("endorsement limit reached")
	}

	r.endorsements[endorsement.Username][endorsement.PinnedActorID] = copyAccountPin(endorsement)
	return nil
}

// DeleteEndorsement removes an endorsement (account pin) relationship
func (r *RelationshipRepository) DeleteEndorsement(_ context.Context, endorserID, endorsedID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.endorsements[endorserID] != nil {
		delete(r.endorsements[endorserID], endorsedID)
	}

	return nil
}

// GetEndorsements retrieves all endorsements (account pins) for a user
func (r *RelationshipRepository) GetEndorsements(_ context.Context, userID string, limit int, _ string) ([]*storage.AccountPin, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	var results []*storage.AccountPin
	if r.endorsements[userID] != nil {
		for _, pin := range r.endorsements[userID] {
			results = append(results, copyAccountPin(pin))
			if len(results) >= limit {
				break
			}
		}
	}

	return results, "", nil
}

// ===== Relationship Note Operations =====

// GetRelationshipNote retrieves a private note on an account
func (r *RelationshipRepository) GetRelationshipNote(_ context.Context, userID, targetID string) (*storage.AccountNote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.notes[userID] == nil {
		return nil, storage.ErrNotFound
	}

	note, exists := r.notes[userID][targetID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyAccountNote(note), nil
}

// SetRelationshipNote sets a private note on an account (test helper)
func (r *RelationshipRepository) SetRelationshipNote(userID, targetID, noteText string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.notes[userID] == nil {
		r.notes[userID] = make(map[string]*storage.AccountNote)
	}

	now := time.Now()
	r.notes[userID][targetID] = &storage.AccountNote{
		Username:      userID,
		TargetActorID: targetID,
		Note:          noteText,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// ===== Move Operations =====

// CreateMove creates a new move record
func (r *RelationshipRepository) CreateMove(_ context.Context, move *storage.Move) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if move == nil {
		return fmt.Errorf("move is required")
	}

	if r.moves[move.Actor] == nil {
		r.moves[move.Actor] = make(map[string]*storage.Move)
	}
	if r.movesByTarget[move.Target] == nil {
		r.movesByTarget[move.Target] = make(map[string]*storage.Move)
	}

	// Check if move already exists
	if _, exists := r.moves[move.Actor][move.Target]; exists {
		return storage.ErrAlreadyExists
	}

	moveCopy := copyMove(move)
	r.moves[move.Actor][move.Target] = moveCopy
	r.movesByTarget[move.Target][move.Actor] = moveCopy

	return nil
}

// GetMove retrieves the most recent move for an actor
func (r *RelationshipRepository) GetMove(_ context.Context, actor string) (*storage.Move, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.moves[actor] == nil || len(r.moves[actor]) == 0 {
		return nil, storage.ErrNotFound
	}

	// Return the first move (in real implementation, would return most recent)
	for _, move := range r.moves[actor] {
		return copyMove(move), nil
	}

	return nil, storage.ErrNotFound
}

// GetAccountMoves retrieves all moves for an account (as actor)
func (r *RelationshipRepository) GetAccountMoves(_ context.Context, actor string) ([]*storage.Move, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.Move
	if r.moves[actor] != nil {
		for _, move := range r.moves[actor] {
			results = append(results, copyMove(move))
		}
	}

	return results, nil
}

// UpdateMoveProgress updates move migration progress
func (r *RelationshipRepository) UpdateMoveProgress(_ context.Context, actor, target string, _ map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.moves[actor] == nil {
		return storage.ErrNotFound
	}

	_, exists := r.moves[actor][target]
	if !exists {
		return storage.ErrNotFound
	}

	// In a real implementation, would apply progress updates
	return nil
}

// VerifyMove marks a move as verified
func (r *RelationshipRepository) VerifyMove(ctx context.Context, actor, target string) error {
	return r.UpdateMoveProgress(ctx, actor, target, map[string]interface{}{
		"Verified": true,
	})
}

// GetPendingMoves retrieves moves that haven't been fully processed
func (r *RelationshipRepository) GetPendingMoves(_ context.Context, limit int) ([]*storage.Move, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	var results []*storage.Move
	for _, actorMoves := range r.moves {
		for _, move := range actorMoves {
			results = append(results, copyMove(move))
			if len(results) >= limit {
				return results, nil
			}
		}
	}

	return results, nil
}

// GetMoveByTarget retrieves all moves to a specific target account
func (r *RelationshipRepository) GetMoveByTarget(_ context.Context, target string) ([]*storage.Move, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.Move
	if r.movesByTarget[target] != nil {
		for _, move := range r.movesByTarget[target] {
			results = append(results, copyMove(move))
		}
	}

	return results, nil
}

// HasMovedFrom checks if newActor has moved from oldActor
func (r *RelationshipRepository) HasMovedFrom(_ context.Context, oldActor, newActor string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.moves[oldActor] == nil {
		return false, nil
	}

	_, exists := r.moves[oldActor][newActor]
	return exists, nil
}

// ===== Collection Operations =====

// AddToCollection adds an item to a collection
func (r *RelationshipRepository) AddToCollection(_ context.Context, collection string, item *storage.CollectionItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil {
		return fmt.Errorf("item is required")
	}

	if r.collections[collection] == nil {
		r.collections[collection] = make(map[string]*storage.CollectionItem)
	}

	r.collections[collection][item.ItemID] = copyCollectionItem(item)
	return nil
}

// RemoveFromCollection removes an item from a collection
func (r *RelationshipRepository) RemoveFromCollection(_ context.Context, collection, itemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.collections[collection] != nil {
		delete(r.collections[collection], itemID)
	}

	return nil
}

// GetCollectionItems retrieves items from a collection with pagination
func (r *RelationshipRepository) GetCollectionItems(_ context.Context, collection string, limit int, _ string) ([]*storage.CollectionItem, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var results []*storage.CollectionItem
	if r.collections[collection] != nil {
		for _, item := range r.collections[collection] {
			results = append(results, copyCollectionItem(item))
			if len(results) >= limit {
				break
			}
		}
	}

	return results, "", nil
}

// IsInCollection checks if an item is in a collection
func (r *RelationshipRepository) IsInCollection(_ context.Context, collection, itemID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.collections[collection] == nil {
		return false, nil
	}

	_, exists := r.collections[collection][itemID]
	return exists, nil
}

// CountCollectionItems returns the count of items in a collection
func (r *RelationshipRepository) CountCollectionItems(_ context.Context, collection string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.collections[collection] == nil {
		return 0, nil
	}

	return len(r.collections[collection]), nil
}

// ClearCollection removes all items from a collection
func (r *RelationshipRepository) ClearCollection(_ context.Context, collection string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.collections, collection)
	return nil
}

// ===== Helper Functions =====

// copyRelationshipRecord creates a deep copy of a RelationshipRecord
func copyRelationshipRecord(record *models.RelationshipRecord) *models.RelationshipRecord {
	if record == nil {
		return nil
	}

	recordCopy := *record

	// Copy slices
	if record.Languages != nil {
		recordCopy.Languages = make([]string, len(record.Languages))
		copy(recordCopy.Languages, record.Languages)
	}

	return &recordCopy
}

// copyBlock creates a deep copy of a Block
func copyBlock(block *storage.Block) *storage.Block {
	if block == nil {
		return nil
	}

	blockCopy := *block
	return &blockCopy
}

// copyMute creates a deep copy of a Mute
func copyMute(mute *storage.Mute) *storage.Mute {
	if mute == nil {
		return nil
	}

	muteCopy := *mute

	// Copy pointer fields
	if mute.ExpiresAt != nil {
		exp := *mute.ExpiresAt
		muteCopy.ExpiresAt = &exp
	}

	return &muteCopy
}

// copyAccountPin creates a deep copy of an AccountPin
func copyAccountPin(pin *storage.AccountPin) *storage.AccountPin {
	if pin == nil {
		return nil
	}

	pinCopy := *pin
	return &pinCopy
}

// copyAccountNote creates a deep copy of an AccountNote
func copyAccountNote(note *storage.AccountNote) *storage.AccountNote {
	if note == nil {
		return nil
	}

	noteCopy := *note
	return &noteCopy
}

// copyMove creates a deep copy of a Move
func copyMove(move *storage.Move) *storage.Move {
	if move == nil {
		return nil
	}

	moveCopy := *move
	return &moveCopy
}

// copyCollectionItem creates a deep copy of a CollectionItem
func copyCollectionItem(item *storage.CollectionItem) *storage.CollectionItem {
	if item == nil {
		return nil
	}

	itemCopy := *item
	return &itemCopy
}
