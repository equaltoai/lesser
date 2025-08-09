package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ===== Social Features =====
// This file contains social interaction methods for the AccountRepository

// Follow creates a follow relationship between users
func (r *AccountRepository) Follow(ctx context.Context, followerUsername, followedUsername string) error {
	// Validate usernames using validation utility
	if !Utils.Validation.IsValidUsername(followerUsername) {
		return common.ValidationError{Field: "follower", Message: "invalid username"}
	}
	if !Utils.Validation.IsValidUsername(followedUsername) {
		return common.ValidationError{Field: "followed", Message: "invalid username"}
	}

	// Verify both actors exist
	follower, err := r.GetActor(ctx, followerUsername)
	if err != nil {
		return ErrorHandler.HandleNotFound(err, EntityActor, followerUsername)
	}

	followed, err := r.GetActor(ctx, followedUsername)
	if err != nil {
		return ErrorHandler.HandleNotFound(err, EntityActor, followedUsername)
	}

	// Check if already following
	isFollowing, err := r.IsFollowing(ctx, followerUsername, followedUsername)
	if err != nil {
		return err
	}
	if isFollowing {
		return common.AlreadyFollowingError{
			Follower: followerUsername,
			Followee: followedUsername,
		}
	}

	// Create follow relationship using NewFollow constructor
	activityID := fmt.Sprintf("%s/activities/follow-%d", follower.ID, time.Now().Unix())
	follow := models.NewFollow(followerUsername, followedUsername, activityID)

	// Auto-approve if account is not locked (check ManuallyApprovesFollowers)
	if !followed.ManuallyApprovesFollowers {
		follow.Accept()
	}

	err = r.db.WithContext(ctx).Model(follow).Create()
	if err != nil {
		r.logger.Error("failed to create follow relationship",
			zap.String("follower", followerUsername),
			zap.String("followed", followedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to create follow: %w", err)
	}

	// Update follower/followed counts
	r.updateFollowCounts(ctx, followerUsername, followedUsername, 1)

	return nil
}

// Unfollow removes a follow relationship between users
func (r *AccountRepository) Unfollow(ctx context.Context, followerUsername, followedUsername string) error {
	// Delete follow relationship
	err := r.db.WithContext(ctx).Model(&models.Follow{}).
		Where("PK", "=", fmt.Sprintf("follow#%s", followerUsername)).
		Where("SK", "=", fmt.Sprintf("following#%s", followedUsername)).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to delete follow relationship",
			zap.String("follower", followerUsername),
			zap.String("followed", followedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to unfollow: %w", err)
	}

	// Update follower/followed counts
	r.updateFollowCounts(ctx, followerUsername, followedUsername, -1)

	return nil
}

// IsFollowing checks if one user follows another
func (r *AccountRepository) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	var follow models.Follow

	err := r.db.WithContext(ctx).Model(&follow).
		Where("PK", "=", fmt.Sprintf("follow#%s", followerUsername)).
		Where("SK", "=", fmt.Sprintf("following#%s", followedUsername)).
		First(&follow)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check follow relationship",
			zap.String("follower", followerUsername),
			zap.String("followed", followedUsername),
			zap.Error(err))
		return false, fmt.Errorf("failed to check follow: %w", err)
	}

	return follow.State == models.FollowStateAccepted, nil
}

// GetFollowers retrieves paginated list of followers for a user
func (r *AccountRepository) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Actor, string, error) {
	var follows []models.Follow

	// Build query using GSI1 for followed's perspective
	query := r.db.WithContext(ctx).Model(&models.Follow{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", fmt.Sprintf("follow#%s", username)).
		Limit(limit)

	if cursor != "" {
		query = query.Where("GSI1SK", ">", cursor)
	}

	err := query.All(&follows)
	if err != nil {
		r.logger.Error("failed to get followers",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get followers: %w", err)
	}

	// Get actor details for each follower
	actors := make([]*activitypub.Actor, 0, len(follows))
	for _, follow := range follows {
		if follow.State == models.FollowStateAccepted {
			actor, err := r.GetActor(ctx, follow.FollowerUsername)
			if err != nil {
				r.logger.Warn("failed to get follower actor",
					zap.String("username", follow.FollowerUsername),
					zap.Error(err))
				continue
			}
			actors = append(actors, actor)
		}
	}

	// Determine next cursor
	nextCursor := ""
	if len(follows) == limit {
		lastFollow := follows[len(follows)-1]
		nextCursor = lastFollow.GSI1SK
	}

	return actors, nextCursor, nil
}

// GetFollowing retrieves paginated list of users that a user follows
func (r *AccountRepository) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Actor, string, error) {
	var follows []models.Follow

	// Build query using primary key
	query := r.db.WithContext(ctx).Model(&models.Follow{}).
		Where("PK", "=", fmt.Sprintf("follow#%s", username)).
		Where("SK", "BEGINS_WITH", "following#").
		Limit(limit)

	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	err := query.All(&follows)
	if err != nil {
		r.logger.Error("failed to get following",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get following: %w", err)
	}

	// Get actor details for each followed user
	actors := make([]*activitypub.Actor, 0, len(follows))
	for _, follow := range follows {
		if follow.State == models.FollowStateAccepted {
			actor, err := r.GetActor(ctx, follow.FollowedUsername)
			if err != nil {
				r.logger.Warn("failed to get followed actor",
					zap.String("username", follow.FollowedUsername),
					zap.Error(err))
				continue
			}
			actors = append(actors, actor)
		}
	}

	// Determine next cursor
	nextCursor := ""
	if len(follows) == limit {
		lastFollow := follows[len(follows)-1]
		nextCursor = lastFollow.SK
	}

	return actors, nextCursor, nil
}

// Block creates a block relationship
func (r *AccountRepository) Block(ctx context.Context, blockerUsername, blockedUsername string) error {
	// Get actor IDs
	blocker, err := r.GetActor(ctx, blockerUsername)
	if err != nil {
		return fmt.Errorf("blocker not found: %w", err)
	}
	blocked, err := r.GetActor(ctx, blockedUsername)
	if err != nil {
		return fmt.Errorf("blocked user not found: %w", err)
	}

	// Create block using model fields
	block := &models.Block{
		Actor:     blocker.ID,
		Object:    blocked.ID,
		Published: time.Now(),
		CreatedAt: time.Now(),
	}

	err = r.db.WithContext(ctx).Model(block).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			// Already blocked
			return nil
		}
		r.logger.Error("failed to create block",
			zap.String("blocker", blockerUsername),
			zap.String("blocked", blockedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to create block: %w", err)
	}

	// Remove any existing follow relationships in both directions
	if err := r.Unfollow(ctx, blockerUsername, blockedUsername); err != nil {
		r.logger.Warn("failed to unfollow after block", 
			zap.String("blocker", blockerUsername),
			zap.String("blocked", blockedUsername),
			zap.Error(err))
	}
	if err := r.Unfollow(ctx, blockedUsername, blockerUsername); err != nil {
		r.logger.Warn("failed to unfollow reverse after block",
			zap.String("blocked", blockedUsername), 
			zap.String("blocker", blockerUsername),
			zap.Error(err))
	}

	return nil
}

// Unblock removes a block relationship
func (r *AccountRepository) Unblock(ctx context.Context, blockerUsername, blockedUsername string) error {
	err := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("PK", "=", fmt.Sprintf(storage.ActorBlocksKey, blockerUsername)).
		Where("SK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to remove block",
			zap.String("blocker", blockerUsername),
			zap.String("blocked", blockedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to unblock: %w", err)
	}

	return nil
}

// IsBlocked checks if one user has blocked another
func (r *AccountRepository) IsBlocked(ctx context.Context, blockerUsername, blockedUsername string) (bool, error) {
	var block models.Block

	err := r.db.WithContext(ctx).Model(&block).
		Where("PK", "=", fmt.Sprintf(storage.ActorBlocksKey, blockerUsername)).
		Where("SK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		First(&block)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check block",
			zap.String("blocker", blockerUsername),
			zap.String("blocked", blockedUsername),
			zap.Error(err))
		return false, fmt.Errorf("failed to check block: %w", err)
	}

	return true, nil
}

// GetBlocks retrieves all users blocked by a user
//nolint:dupl // Social relationship query patterns are shared between repositories
func (r *AccountRepository) GetBlocks(ctx context.Context, username string) ([]*storage.Block, error) {
	var blocks []models.Block

	err := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s#BLOCKS", username)).
		Where("SK", "BEGINS_WITH", "BLOCKED#").
		All(&blocks)

	if err != nil {
		r.logger.Error("failed to get blocks",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get blocks: %w", err)
	}

	// Convert to storage type and extract usernames from actor IDs
	result := make([]*storage.Block, len(blocks))
	for i, block := range blocks {
		result[i] = &storage.Block{
			Actor:     block.Actor,
			Object:    block.Object,
			ID:        block.ID,
			Published: block.Published,
			CreatedAt: block.CreatedAt,
		}
	}

	return result, nil
}

// Mute creates a mute relationship
func (r *AccountRepository) Mute(ctx context.Context, muterUsername, mutedUsername string, notifications bool, _ time.Duration) error {
	// Get actor IDs
	muter, err := r.GetActor(ctx, muterUsername)
	if err != nil {
		return fmt.Errorf("muter not found: %w", err)
	}
	muted, err := r.GetActor(ctx, mutedUsername)
	if err != nil {
		return fmt.Errorf("muted user not found: %w", err)
	}

	mute := &models.Mute{
		Actor:             muter.ID,
		Object:            muted.ID,
		HideNotifications: notifications,
		Published:         time.Now(),
		CreatedAt:         time.Now(),
	}

	err = r.db.WithContext(ctx).Model(mute).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			// Update existing mute
			return r.updateMute(ctx, muterUsername, mutedUsername, notifications)
		}
		r.logger.Error("failed to create mute",
			zap.String("muter", muterUsername),
			zap.String("muted", mutedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to create mute: %w", err)
	}

	return nil
}

// Unmute removes a mute relationship
func (r *AccountRepository) Unmute(ctx context.Context, muterUsername, mutedUsername string) error {
	err := r.db.WithContext(ctx).Model(&models.Mute{}).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", muterUsername)).
		Where("SK", "=", fmt.Sprintf("MUTED#%s", mutedUsername)).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to remove mute",
			zap.String("muter", muterUsername),
			zap.String("muted", mutedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to unmute: %w", err)
	}

	return nil
}

// IsMuted checks if one user has muted another
func (r *AccountRepository) IsMuted(ctx context.Context, muterUsername, mutedUsername string) (bool, bool, error) {
	var mute models.Mute

	err := r.db.WithContext(ctx).Model(&mute).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", muterUsername)).
		Where("SK", "=", fmt.Sprintf("MUTED#%s", mutedUsername)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, false, nil
		}
		r.logger.Error("failed to check mute",
			zap.String("muter", muterUsername),
			zap.String("muted", mutedUsername),
			zap.Error(err))
		return false, false, fmt.Errorf("failed to check mute: %w", err)
	}

	return true, mute.HideNotifications, nil
}

// GetMutes retrieves all users muted by a user
func (r *AccountRepository) GetMutes(ctx context.Context, username string) ([]*storage.Mute, error) {
	var mutes []models.Mute

	err := r.db.WithContext(ctx).Model(&models.Mute{}).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", username)).
		Where("SK", "BEGINS_WITH", "MUTED#").
		All(&mutes)

	if err != nil {
		r.logger.Error("failed to get mutes",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get mutes: %w", err)
	}

	// Convert to storage type and extract usernames from actor IDs
	result := make([]*storage.Mute, len(mutes))
	for i, mute := range mutes {
		result[i] = &storage.Mute{
			Actor:             mute.Actor,
			Object:            mute.Object,
			ID:                mute.ID,
			HideNotifications: mute.HideNotifications,
			Published:         mute.Published,
			CreatedAt:         mute.CreatedAt,
		}
	}

	return result, nil
}

// AddBookmark bookmarks an object for a user
func (r *AccountRepository) AddBookmark(ctx context.Context, username, objectID string) error {
	bookmark := &models.Bookmark{
		Username:  username,
		ObjectID:  objectID,
		CreatedAt: time.Now(),
	}

	// Update keys using the model's UpdateKeys method
	bookmark.UpdateKeys()

	err := r.db.WithContext(ctx).Model(bookmark).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			// Already bookmarked
			return nil
		}
		r.logger.Error("failed to add bookmark",
			zap.String("username", username),
			zap.String("objectID", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to add bookmark: %w", err)
	}

	return nil
}

// RemoveBookmark removes a bookmark
func (r *AccountRepository) RemoveBookmark(ctx context.Context, username, objectID string) error {
	// We need to find the bookmark first since SK includes timestamp
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", fmt.Sprintf("BOOKMARK#%s", username)).
		Where("SK", "CONTAINS", objectID).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to remove bookmark",
			zap.String("username", username),
			zap.String("objectID", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to remove bookmark: %w", err)
	}

	return nil
}

// GetBookmarks retrieves paginated bookmarks for a user
func (r *AccountRepository) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error) {
	var bookmarks []models.Bookmark

	query := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", fmt.Sprintf("BOOKMARK#%s", username)).
		Limit(limit)

	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	err := query.All(&bookmarks)
	if err != nil {
		r.logger.Error("failed to get bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get bookmarks: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.Bookmark, len(bookmarks))
	for i, bookmark := range bookmarks {
		result[i] = &storage.Bookmark{
			Username:  bookmark.Username,
			ObjectID:  bookmark.ObjectID,
			CreatedAt: bookmark.CreatedAt,
		}
	}

	// Determine next cursor
	nextCursor := ""
	if len(bookmarks) == limit {
		lastBookmark := bookmarks[len(bookmarks)-1]
		nextCursor = lastBookmark.SK
	}

	return result, nextCursor, nil
}

// PinAccount pins an account for a user
func (r *AccountRepository) PinAccount(ctx context.Context, username, pinnedUsername string) error {
	// Get the pinned actor to get their ID
	pinnedActor, err := r.GetActor(ctx, pinnedUsername)
	if err != nil {
		return ErrorHandler.HandleNotFound(err, EntityActor, pinnedUsername)
	}

	// Create account pin using existing method
	pin := &storage.AccountPin{
		Username:       username,
		PinnedActorID:  pinnedActor.ID,
		PinnedUsername: pinnedUsername,
		CreatedAt:      time.Now(),
	}

	return r.CreateAccountPin(ctx, pin)
}

// UnpinAccount unpins an account for a user
func (r *AccountRepository) UnpinAccount(ctx context.Context, username, pinnedUsername string) error {
	// Get the pinned actor to get their ID
	pinnedActor, err := r.GetActor(ctx, pinnedUsername)
	if err != nil {
		return ErrorHandler.HandleNotFound(err, EntityActor, pinnedUsername)
	}

	// Delete the account pin
	err = r.db.WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_PIN#%s", username)).
		Where("SK", "=", fmt.Sprintf("PIN#%s", pinnedActor.ID)).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to remove account pin",
			zap.String("username", username),
			zap.String("pinnedUsername", pinnedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to unpin account: %w", err)
	}

	return nil
}

// GetPinnedAccounts retrieves pinned accounts for a user
func (r *AccountRepository) GetPinnedAccounts(ctx context.Context, username string) ([]*activitypub.Actor, error) {
	// Get account pins
	pins, err := r.GetAccountPins(ctx, username)
	if err != nil {
		return nil, err
	}

	// Convert to actors
	actors := make([]*activitypub.Actor, 0, len(pins))
	for _, pin := range pins {
		// Get actor by username if available
		if pin.PinnedUsername != "" {
			actor, err := r.GetActor(ctx, pin.PinnedUsername)
			if err != nil {
				r.logger.Warn("failed to get pinned actor",
					zap.String("username", username),
					zap.String("pinnedUsername", pin.PinnedUsername),
					zap.Error(err))
				continue
			}
			actors = append(actors, actor)
		}
		// If no username, we can't retrieve the actor easily
		// This should not happen in normal operation since we store both
	}

	return actors, nil
}

// GetAccountPins retrieves all account pins for a user
func (r *AccountRepository) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	var pins []models.AccountPin

	err := r.db.WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_PIN#%s", username)).
		Where("SK", "BEGINS_WITH", "PIN#").
		All(&pins)

	if err != nil {
		r.logger.Error("failed to get account pins",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get account pins: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.AccountPin, len(pins))
	for i, pin := range pins {
		result[i] = &storage.AccountPin{
			Username:       pin.Username,
			PinnedActorID:  pin.PinnedActorID,
			PinnedUsername: pin.PinnedUsername,
			CreatedAt:      pin.CreatedAt,
		}
	}

	return result, nil
}

// GetAccountPin retrieves a specific account pin by actor ID
func (r *AccountRepository) GetAccountPin(ctx context.Context, username, targetActorID string) (*storage.AccountPin, error) {
	var pin models.AccountPin

	err := r.db.WithContext(ctx).Model(&pin).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_PIN#%s", username)).
		Where("SK", "=", fmt.Sprintf("PIN#%s", targetActorID)).
		First(&pin)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		r.logger.Error("failed to get account pin",
			zap.String("username", username),
			zap.String("targetActorID", targetActorID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get account pin: %w", err)
	}

	return &storage.AccountPin{
		Username:       pin.Username,
		PinnedActorID:  pin.PinnedActorID,
		PinnedUsername: pin.PinnedUsername,
		CreatedAt:      pin.CreatedAt,
	}, nil
}

// Helper methods

// updateMute updates an existing mute
func (r *AccountRepository) updateMute(ctx context.Context, muterUsername, mutedUsername string, notifications bool) error {
	// Get existing mute
	var mute models.Mute
	err := r.db.WithContext(ctx).Model(&mute).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", muterUsername)).
		Where("SK", "=", fmt.Sprintf("MUTED#%s", mutedUsername)).
		First(&mute)

	if err != nil {
		return fmt.Errorf("failed to get existing mute: %w", err)
	}

	// Update fields
	mute.HideNotifications = notifications

	err = r.db.WithContext(ctx).Model(&mute).Update()
	if err != nil {
		r.logger.Error("failed to update mute",
			zap.String("muter", muterUsername),
			zap.String("muted", mutedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to update mute: %w", err)
	}

	return nil
}

// updateFollowCounts updates follower/following counts (best effort)
func (r *AccountRepository) updateFollowCounts(ctx context.Context, followerUsername, followedUsername string, delta int) {
	// Update follower's following count
	r.updateActorCount(ctx, followerUsername, "FollowingCount", delta)

	// Update followed user's follower count
	r.updateActorCount(ctx, followedUsername, "FollowerCount", delta)
}

// updateActorCount updates a numeric count field on an actor
func (r *AccountRepository) updateActorCount(ctx context.Context, username, field string, delta int) {
	// This is a best-effort update, don't fail the operation if it fails
	// Get the actor first
	var actor models.Actor
	err := r.db.WithContext(ctx).Model(&actor).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s", username)).
		Where("SK", "=", "PROFILE").
		First(&actor)

	if err != nil {
		r.logger.Warn("failed to get actor for count update",
			zap.String("username", username),
			zap.Error(err))
		return
	}

	// Update the count field
	switch field {
	case "FollowerCount":
		actor.FollowerCount += delta
		if actor.FollowerCount < 0 {
			actor.FollowerCount = 0
		}
	case "FollowingCount":
		actor.FollowingCount += delta
		if actor.FollowingCount < 0 {
			actor.FollowingCount = 0
		}
	case "StatusCount":
		actor.StatusCount += delta
		if actor.StatusCount < 0 {
			actor.StatusCount = 0
		}
	}

	err = r.db.WithContext(ctx).Model(&actor).Update()
	if err != nil {
		r.logger.Warn("failed to update actor count",
			zap.String("username", username),
			zap.String("field", field),
			zap.Int("delta", delta),
			zap.Error(err))
	}
}
