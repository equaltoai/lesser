package federation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// DynamORMFederationStorage implements FederationStorage using DynamORM repositories.
// This provides a clean, minimal interface for federation operations without the
// overhead of the full storage.Storage interface.
type DynamORMFederationStorage struct {
	db                           core.DB
	actorRepository              dynamormFederationActorRepository
	federationActivityRepository dynamormFederationActivityRepository
	relationshipRepository       dynamormFederationRelationshipRepository
}

type dynamormFederationActorRepository interface {
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
}

type dynamormFederationActivityRepository interface {
	Create(ctx context.Context, activity *models.FederationActivity) error
}

type dynamormFederationRelationshipRepository interface {
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

// NewDynamORMFederationStorage creates a new DynamORM-based federation storage implementation.
func NewDynamORMFederationStorage(
	db core.DB,
	tableName string,
	domain string,
	logger *zap.Logger,
) *DynamORMFederationStorage {
	return &DynamORMFederationStorage{
		db:                           db,
		actorRepository:              repositories.NewActorRepository(db, tableName, logger, domain),
		federationActivityRepository: repositories.NewFederationActivityRepository(db, tableName, logger, nil),
		relationshipRepository:       repositories.NewRelationshipRepository(db, tableName, logger),
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
	// Use the relationship repository to get followers
	return s.relationshipRepository.GetFollowers(ctx, username, limit, cursor)
}

// GetCachedRemoteActor retrieves a cached remote actor by actor ID.
func (s *DynamORMFederationStorage) GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	handle := models.NormalizeRemoteActorHandle(actorID)
	if err := common.ValidateRequiredParam("handle", handle); err != nil {
		return nil, storage.ErrNotFound
	}

	lookupHandles := []string{handle}
	if !strings.HasPrefix(handle, "@") {
		lookupHandles = append(lookupHandles, "@"+handle)
	}

	for _, lookupHandle := range lookupHandles {
		var remoteActor models.RemoteActor
		err := s.db.WithContext(ctx).Model(&remoteActor).
			Where("PK", "=", fmt.Sprintf("REMOTE_ACTOR#%s", lookupHandle)).
			Where("SK", "=", "PROFILE").
			First(&remoteActor)

		if err != nil {
			if dynamormErrors.IsNotFound(err) {
				continue
			}
			zap.L().Error("failed to get cached remote actor",
				zap.String("handle", lookupHandle),
				zap.String("actorID", actorID),
				zap.Error(err))
			return nil, errors.Join(ErrRemoteActorCacheRetrieveFailed, err)
		}

		if time.Now().After(remoteActor.ExpiresAt) {
			continue
		}

		actor := normalizeFederationCachedActor(lookupHandle, remoteActor.Actor)
		if err := activitypub.ValidateResolvedActor(actor); err != nil {
			zap.L().Warn("ignoring invalid cached remote actor",
				zap.String("handle", lookupHandle),
				zap.String("actorID", actorID),
				zap.Error(err))
			continue
		}

		return actor, nil
	}

	return nil, storage.ErrNotFound
}

// CacheRemoteActor caches a remote actor for future lookups.
func (s *DynamORMFederationStorage) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	if actor == nil {
		return errors.Join(ErrRemoteActorCacheStoreFailed, fmt.Errorf("remote actor is required"))
	}

	now := time.Now()
	expiresAt := now.Add(ttl)
	canonicalHandle := models.NormalizeRemoteActorHandle(handle)
	cachedActor := normalizeFederationCachedActor(canonicalHandle, actor)

	remoteActor := &models.RemoteActor{
		Handle:    canonicalHandle,
		Actor:     cachedActor,
		CachedAt:  now,
		UpdatedAt: now,
		ExpiresAt: expiresAt,
	}

	// Update keys for DynamORM
	remoteActor.UpdateKeys()

	// Store in database
	err := s.db.WithContext(ctx).Model(remoteActor).Create()
	if err != nil {
		// If already exists, update it
		if dynamormErrors.IsConditionFailed(err) {
			err = s.db.WithContext(ctx).Model(remoteActor).
				Where("PK", "=", remoteActor.PK).
				Where("SK", "=", remoteActor.SK).
				Update()
			if err != nil {
				zap.L().Error("failed to update cached remote actor",
					zap.String("handle", canonicalHandle),
					zap.String("actorID", cachedActor.ID),
					zap.Error(err))
				return errors.Join(ErrRemoteActorCacheUpdateFailed, err)
			}
		} else {
			zap.L().Error("failed to cache remote actor",
				zap.String("handle", canonicalHandle),
				zap.String("actorID", cachedActor.ID),
				zap.Error(err))
			return errors.Join(ErrRemoteActorCacheStoreFailed, err)
		}
	}

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

func normalizeFederationCachedActor(handle string, actor *activitypub.Actor) *activitypub.Actor {
	if actor == nil {
		return nil
	}

	clone := *actor
	if clone.PreferredUsername == "" {
		parts := strings.Split(models.NormalizeRemoteActorHandle(handle), "@")
		if len(parts) >= 1 {
			clone.PreferredUsername = strings.TrimSpace(parts[0])
		}
	}

	return &clone
}
