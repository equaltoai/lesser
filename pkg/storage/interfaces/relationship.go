// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ConcreteRelationshipRepository defines the interface for relationship operations
// that matches the concrete repositories.RelationshipRepository implementation.
// This handles follow relationships, blocks, mutes, endorsements, moves, and collections.
type ConcreteRelationshipRepository interface {
	// ===== Core Follow Relationship Operations =====

	// CreateRelationship creates a new follow relationship
	CreateRelationship(ctx context.Context, followerUsername, followingUsername, activityID string) error

	// DeleteRelationship removes a follow relationship
	DeleteRelationship(ctx context.Context, followerUsername, followingUsername string) error

	// GetRelationship retrieves a specific follow relationship
	GetRelationship(ctx context.Context, followerUsername, followingUsername string) (*models.RelationshipRecord, error)

	// UpdateRelationship updates relationship settings
	UpdateRelationship(ctx context.Context, followerUsername, followingUsername string, updates map[string]interface{}) error

	// IsFollowing checks if followerUsername is following the targetActorID
	IsFollowing(ctx context.Context, followerUsername, targetActorID string) (bool, error)

	// ===== Follow Request Operations =====

	// GetFollowRequest gets a follow request by follower and target IDs
	GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error)

	// HasFollowRequest checks if there's a follow request between two users
	HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error)

	// HasPendingFollowRequest checks if there's a pending follow request between two users
	HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error)

	// GetPendingFollowRequests retrieves pending follow requests for a user
	GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	// AcceptFollowRequest accepts a follow request
	AcceptFollowRequest(ctx context.Context, followerUsername, followingUsername string) error

	// RejectFollowRequest rejects a follow request
	RejectFollowRequest(ctx context.Context, followerUsername, followingUsername string) error

	// ===== Follower/Following List Operations =====

	// GetFollowers retrieves all followers for a user
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	// GetFollowing retrieves all users that a user is following
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	// CountFollowers returns the number of followers for a user
	CountFollowers(ctx context.Context, username string) (int, error)

	// CountFollowing returns the number of users that a user is following
	CountFollowing(ctx context.Context, username string) (int, error)

	// GetFollowerCount returns the number of followers for a user (int64 version)
	GetFollowerCount(ctx context.Context, userID string) (int64, error)

	// GetFollowingCount returns the number of users that a user is following (int64 version)
	GetFollowingCount(ctx context.Context, userID string) (int64, error)

	// CountRelationshipsByDomain counts follower/following relationships involving a remote domain
	CountRelationshipsByDomain(ctx context.Context, domain string) (followers, following int, err error)

	// Unfollow removes a follow relationship (wrapper for DeleteRelationship)
	Unfollow(ctx context.Context, followerID, followingID string) error

	// ===== Block Operations =====

	// CreateBlock creates a new block relationship
	CreateBlock(ctx context.Context, blockerActor, blockedActor, activityID string) error

	// DeleteBlock removes a block relationship
	DeleteBlock(ctx context.Context, blockerActor, blockedActor string) error

	// BlockUser blocks another user
	BlockUser(ctx context.Context, blockerID, blockedID string) error

	// UnblockUser removes a block relationship
	UnblockUser(ctx context.Context, blockerID, blockedID string) error

	// IsBlocked checks if one actor has blocked another
	IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error)

	// IsBlockedBidirectional checks if either actor has blocked the other
	IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error)

	// GetBlockedUsers returns a list of users blocked by the given actor
	GetBlockedUsers(ctx context.Context, blockerActor string, limit int, cursor string) ([]string, string, error)

	// GetUsersWhoBlocked returns a list of users who have blocked the given actor
	GetUsersWhoBlocked(ctx context.Context, blockedActor string, limit int, cursor string) ([]string, string, error)

	// GetBlock retrieves a specific block relationship
	GetBlock(ctx context.Context, blockerActor, blockedActor string) (*storage.Block, error)

	// CountBlockedUsers returns the number of users blocked by the given actor
	CountBlockedUsers(ctx context.Context, blockerActor string) (int, error)

	// CountUsersWhoBlocked returns the number of users who have blocked the given actor
	CountUsersWhoBlocked(ctx context.Context, blockedActor string) (int, error)

	// ===== Mute Operations =====

	// CreateMute creates a new mute relationship
	CreateMute(ctx context.Context, muterActor, mutedActor, activityID string, hideNotifications bool, duration *time.Duration) error

	// DeleteMute removes a mute relationship
	DeleteMute(ctx context.Context, muterActor, mutedActor string) error

	// UnmuteUser removes a mute relationship
	UnmuteUser(ctx context.Context, muterID, mutedID string) error

	// IsMuted checks if one actor has muted another
	IsMuted(ctx context.Context, muterActor, mutedActor string) (bool, error)

	// GetMutedUsers returns a list of users muted by the given actor
	GetMutedUsers(ctx context.Context, muterActor string, limit int, cursor string) ([]string, string, error)

	// GetUsersWhoMuted returns a list of users who have muted the given actor
	GetUsersWhoMuted(ctx context.Context, mutedActor string, limit int, cursor string) ([]string, string, error)

	// GetMute retrieves a specific mute relationship
	GetMute(ctx context.Context, muterActor, mutedActor string) (*storage.Mute, error)

	// CountMutedUsers returns the number of users muted by the given actor
	CountMutedUsers(ctx context.Context, muterActor string) (int, error)

	// CountUsersWhoMuted returns the number of users who have muted the given actor
	CountUsersWhoMuted(ctx context.Context, mutedActor string) (int, error)

	// ===== Endorsement Operations =====

	// IsEndorsed checks if a user has endorsed (pinned) a target account
	IsEndorsed(ctx context.Context, userID, targetID string) (bool, error)

	// CreateEndorsement creates a new endorsement (account pin) relationship
	CreateEndorsement(ctx context.Context, endorsement *storage.AccountPin) error

	// DeleteEndorsement removes an endorsement (account pin) relationship
	DeleteEndorsement(ctx context.Context, endorserID, endorsedID string) error

	// GetEndorsements retrieves all endorsements (account pins) for a user
	GetEndorsements(ctx context.Context, userID string, limit int, cursor string) ([]*storage.AccountPin, string, error)

	// ===== Relationship Note Operations =====

	// GetRelationshipNote retrieves a private note on an account
	GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error)

	// ===== Move Operations =====

	// CreateMove creates a new move record
	CreateMove(ctx context.Context, move *storage.Move) error

	// GetMove retrieves the most recent move for an actor
	GetMove(ctx context.Context, actor string) (*storage.Move, error)

	// GetAccountMoves retrieves all moves for an account (as actor)
	GetAccountMoves(ctx context.Context, actor string) ([]*storage.Move, error)

	// UpdateMoveProgress updates move migration progress
	UpdateMoveProgress(ctx context.Context, actor, target string, progress map[string]interface{}) error

	// VerifyMove marks a move as verified
	VerifyMove(ctx context.Context, actor, target string) error

	// GetPendingMoves retrieves moves that haven't been fully processed
	GetPendingMoves(ctx context.Context, limit int) ([]*storage.Move, error)

	// GetMoveByTarget retrieves all moves to a specific target account
	GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error)

	// HasMovedFrom checks if newActor has moved from oldActor
	HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error)

	// ===== Collection Operations =====

	// AddToCollection adds an item to a collection
	AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error

	// RemoveFromCollection removes an item from a collection
	RemoveFromCollection(ctx context.Context, collection, itemID string) error

	// GetCollectionItems retrieves items from a collection with pagination
	GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error)

	// IsInCollection checks if an item is in a collection
	IsInCollection(ctx context.Context, collection, itemID string) (bool, error)

	// CountCollectionItems returns the count of items in a collection
	CountCollectionItems(ctx context.Context, collection string) (int, error)

	// ClearCollection removes all items from a collection
	ClearCollection(ctx context.Context, collection string) error
}
