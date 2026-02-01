package federation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
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
	logger *zap.Logger,
) *DynamORMFederationStorage {
	return &DynamORMFederationStorage{
		db:                           db,
		actorRepository:              repositories.NewActorRepository(db, tableName, logger),
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
	// For GetCachedRemoteActor, we need to extract the handle from just the actor ID
	// Since we don't have preferredUsername here, we'll extract it from the URL
	handle := extractHandleFromURL(actorID)
	if err := common.ValidateRequiredParam("handle", handle); err != nil {
		return nil, storage.ErrNotFound
	}

	// Query for cached remote actor
	var remoteActor models.RemoteActor
	err := s.db.WithContext(ctx).Model(&remoteActor).
		Where("PK", "=", fmt.Sprintf("REMOTE_ACTOR#%s", handle)).
		Where("SK", "=", "PROFILE").
		First(&remoteActor)

	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		zap.L().Error("failed to get cached remote actor",
			zap.String("handle", handle),
			zap.String("actorID", actorID),
			zap.Error(err))
		return nil, errors.Join(ErrRemoteActorCacheRetrieveFailed, err)
	}

	// Check if cache has expired
	if time.Now().After(remoteActor.ExpiresAt) {
		return nil, storage.ErrNotFound
	}

	return remoteActor.Actor, nil
}

// CacheRemoteActor caches a remote actor for future lookups.
func (s *DynamORMFederationStorage) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	now := time.Now()
	expiresAt := now.Add(ttl)

	remoteActor := &models.RemoteActor{
		Handle:    handle,
		Actor:     actor,
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
					zap.String("handle", handle),
					zap.String("actorID", actor.ID),
					zap.Error(err))
				return errors.Join(ErrRemoteActorCacheUpdateFailed, err)
			}
		} else {
			zap.L().Error("failed to cache remote actor",
				zap.String("handle", handle),
				zap.String("actorID", actor.ID),
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

// extractHandleFromURL extracts the handle (user@domain) from an actor ID URL
// e.g., "https://example.com/users/alice" -> "alice@example.com"
func extractHandleFromURL(actorID string) string {
	if actorID == "" {
		return ""
	}

	parsed, err := url.Parse(actorID)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}

	host := parsed.Hostname()
	if host == "" {
		return ""
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, part := range parts {
		if part == "users" && i+1 < len(parts) && parts[i+1] != "" {
			return fmt.Sprintf("%s@%s", parts[i+1], host)
		}
	}

	return ""
}
