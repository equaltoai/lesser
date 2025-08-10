package services

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// StorageAdapter provides a unified interface to both storage.Storage and core.RepositoryStorage
type StorageAdapter interface {
	// Actor operations
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	
	// Object operations
	CreateObject(ctx context.Context, object interface{}) error
	GetObject(ctx context.Context, objectID string) (interface{}, error)
	TombstoneObject(ctx context.Context, objectID, actorID string) error
	IncrementReplyCount(ctx context.Context, objectID string) error
	
	// Activity operations
	CreateActivity(ctx context.Context, activity *activitypub.Activity) error
	
	// Relationship operations
	CreateRelationship(ctx context.Context, followerUsername, followingID, activityID string) error
	IsFollowing(ctx context.Context, followerUsername, followingID string) (bool, error)
	
	// Like operations
	CreateLike(ctx context.Context, actorID, objectID, activityID string) error
	HasLiked(ctx context.Context, actorID, objectID string) (bool, error)
	
	// Analytics operations
	RecordActivity(ctx context.Context, activityType, actorID string, timestamp time.Time) error
	RecordHashtagUsage(ctx context.Context, hashtag, objectID, actorID string) error
	RecordLinkShare(ctx context.Context, link, objectID, actorID string) error
	RecordStatusEngagement(ctx context.Context, objectID, engagementType, actorID string) error
	
	// Timeline operations
	FanOutPost(ctx context.Context, activity *activitypub.Activity) error
	
	// Federation operations
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	
	// Notification operations
	CreateNotification(ctx context.Context, notification interface{}) error
	DeleteNotificationsByObject(ctx context.Context, objectID string) error
	
	// Database access
	GetDB() interface{}
	GetTableName() string
}

// repositoryStorageAdapter adapts core.RepositoryStorage to StorageAdapter
type repositoryStorageAdapter struct {
	repos core.RepositoryStorage
}

// NewRepositoryStorageAdapter creates an adapter for core.RepositoryStorage
func NewRepositoryStorageAdapter(repos core.RepositoryStorage) StorageAdapter {
	return &repositoryStorageAdapter{repos: repos}
}

// Implement StorageAdapter interface

func (r *repositoryStorageAdapter) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return r.repos.Actor().GetActor(ctx, username)
}

func (r *repositoryStorageAdapter) CreateObject(ctx context.Context, object interface{}) error {
	return r.repos.Object().CreateObject(ctx, object)
}

func (r *repositoryStorageAdapter) GetObject(ctx context.Context, objectID string) (interface{}, error) {
	return r.repos.Object().GetObject(ctx, objectID)
}

func (r *repositoryStorageAdapter) TombstoneObject(ctx context.Context, objectID, actorID string) error {
	return r.repos.Object().TombstoneObject(ctx, objectID, actorID)
}

func (r *repositoryStorageAdapter) IncrementReplyCount(ctx context.Context, objectID string) error {
	return r.repos.Object().IncrementReplyCount(ctx, objectID)
}

func (r *repositoryStorageAdapter) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	return r.repos.Activity().CreateActivity(ctx, activity)
}

func (r *repositoryStorageAdapter) CreateRelationship(ctx context.Context, followerUsername, followingID, activityID string) error {
	return r.repos.Relationship().CreateRelationship(ctx, followerUsername, followingID, activityID)
}

func (r *repositoryStorageAdapter) IsFollowing(ctx context.Context, followerUsername, followingID string) (bool, error) {
	// The repository doesn't have IsFollowing, but we can check if a relationship exists
	relationship, err := r.repos.Relationship().GetRelationship(ctx, followerUsername, followingID)
	if err != nil {
		return false, nil // Not following if we can't find the relationship
	}
	return relationship != nil, nil
}

func (r *repositoryStorageAdapter) CreateLike(ctx context.Context, actorID, objectID, _ string) error {
	// The repository's CreateLike returns a Like model, but we only need the error
	_, err := r.repos.Like().CreateLike(ctx, actorID, objectID)
	return err
}

func (r *repositoryStorageAdapter) HasLiked(ctx context.Context, actorID, objectID string) (bool, error) {
	return r.repos.Like().HasLiked(ctx, actorID, objectID)
}

func (r *repositoryStorageAdapter) RecordActivity(_ context.Context, _, _ string, _ time.Time) error {
	// TODO: TrendingRepository doesn't have RecordActivity method
	// Would need to add this method or use a different approach
	return nil
}

func (r *repositoryStorageAdapter) RecordHashtagUsage(ctx context.Context, hashtag, objectID, actorID string) error {
	// TrendingRepository has RecordHashtagUsage with same parameters
	return r.repos.Analytics().RecordHashtagUsage(ctx, hashtag, objectID, actorID)
}

func (r *repositoryStorageAdapter) RecordLinkShare(_ context.Context, _, _, _ string) error {
	// TODO: TrendingRepository doesn't have RecordLinkShare method
	// Would need to add this method or use a different approach
	return nil
}

func (r *repositoryStorageAdapter) RecordStatusEngagement(ctx context.Context, objectID, engagementType, actorID string) error {
	// TrendingRepository has RecordStatusEngagement with userID instead of actorID
	return r.repos.Analytics().RecordStatusEngagement(ctx, objectID, engagementType, actorID)
}

func (r *repositoryStorageAdapter) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	return r.repos.User().FanOutPost(ctx, activity)
}

func (r *repositoryStorageAdapter) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return r.repos.Relationship().GetFollowers(ctx, username, limit, cursor)
}

func (r *repositoryStorageAdapter) CreateNotification(ctx context.Context, notification interface{}) error {
	// Convert to models.Notification if needed
	var notif *models.Notification
	switch n := notification.(type) {
	case *models.Notification:
		notif = n
	default:
		return fmt.Errorf("invalid notification type: %T", notification)
	}
	
	return r.repos.Notification().CreateNotification(ctx, notif)
}

func (r *repositoryStorageAdapter) DeleteNotificationsByObject(ctx context.Context, objectID string) error {
	return r.repos.Notification().DeleteNotificationsByObject(ctx, objectID)
}

func (r *repositoryStorageAdapter) GetDB() interface{} {
	return r.repos.GetDB()
}

func (r *repositoryStorageAdapter) GetTableName() string {
	return r.repos.GetTableName()
}

// CreateStorageAdapter creates the appropriate adapter based on the storage type
func CreateStorageAdapter(repos interface{}) StorageAdapter {
	switch s := repos.(type) {
	case core.RepositoryStorage:
		return NewRepositoryStorageAdapter(s)
	default:
		// For now, only support RepositoryStorage
		panic("unsupported storage type - only core.RepositoryStorage is supported")
	}
}