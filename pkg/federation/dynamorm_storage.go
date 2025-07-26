package federation

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// DynamORMFederationStorage implements FederationStorage using DynamORM repositories.
// This provides a clean, minimal interface for federation operations without the
// overhead of the full storage.Storage interface.
type DynamORMFederationStorage struct {
	actorRepository              *repositories.ActorRepository
	federationActivityRepository *repositories.FederationActivityRepository
}

// NewDynamORMFederationStorage creates a new DynamORM-based federation storage implementation.
func NewDynamORMFederationStorage(
	db core.DB,
	tableName string,
) *DynamORMFederationStorage {
	return &DynamORMFederationStorage{
		actorRepository:              repositories.NewActorRepository(db),
		federationActivityRepository: repositories.NewFederationActivityRepository(db, tableName, nil),
	}
}

// GetActorPrivateKey retrieves the private key for an actor by username.
func (s *DynamORMFederationStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	return s.actorRepository.GetActorPrivateKey(ctx, username)
}

// GetActor retrieves an actor by username.
func (s *DynamORMFederationStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return s.actorRepository.GetActorByUsername(ctx, username)
}

// GetFollowers retrieves the list of follower usernames for an actor.
func (s *DynamORMFederationStorage) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	// For now, return empty list since this is primarily used for federation delivery
	// In a full implementation, this would query the follows table
	return []string{}, "", nil
}

// GetCachedRemoteActor retrieves a cached remote actor by actor ID.
func (s *DynamORMFederationStorage) GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	// For now, return not found to force fresh fetches
	// In a full implementation, this would check a cache table
	return nil, storage.ErrNotFound
}

// CacheRemoteActor caches a remote actor for future lookups.
func (s *DynamORMFederationStorage) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	// For now, this is a no-op
	// In a full implementation, this would store in a cache table with TTL
	return nil
}

// RecordFederationActivity records federation activity for cost tracking and metrics.
func (s *DynamORMFederationStorage) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	// Convert legacy storage type to DynamORM model
	modelActivity := &models.FederationActivity{
		Domain:       activity.Domain,
		ActivityType: activity.ActivityType,
		Success:      activity.Success,
		ResponseTime: float64(activity.ResponseTime),
		ErrorMessage: activity.ErrorMessage,
		Timestamp:    activity.Timestamp,
	}

	// Set the byte size in the appropriate field based on activity type
	if activity.Type == "egress" {
		modelActivity.OutboundSize = activity.ByteSize
	} else {
		modelActivity.InboundSize = activity.ByteSize
	}

	return s.federationActivityRepository.Create(ctx, modelActivity)
}