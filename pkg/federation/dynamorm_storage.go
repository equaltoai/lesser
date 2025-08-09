package federation

import (
	"context"
	"fmt"
	"github.com/equaltoai/lesser/pkg/storage"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
)

// DynamORMFederationStorage implements FederationStorage using DynamORM repositories.
// This provides a clean, minimal interface for federation operations without the
// overhead of the full storage.Storage interface.
type DynamORMFederationStorage struct {
	db                           core.DB
	actorRepository              *repositories.ActorRepository
	federationActivityRepository *repositories.FederationActivityRepository
	relationshipRepository       *repositories.RelationshipRepository
}

// NewDynamORMFederationStorage creates a new DynamORM-based federation storage implementation.
func NewDynamORMFederationStorage(
	db core.DB,
	tableName string,
) *DynamORMFederationStorage {
	return &DynamORMFederationStorage{
		db:                           db,
		actorRepository:              repositories.NewActorRepository(db, tableName, nil),
		federationActivityRepository: repositories.NewFederationActivityRepository(db, tableName, nil),
		relationshipRepository:       repositories.NewRelationshipRepository(db, tableName, nil),
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
	if handle == "" {
		return nil, storage.ErrNotFound
	}

	// Query for cached remote actor
	var remoteActor models.RemoteActor
	err := s.db.WithContext(ctx).Model(&remoteActor).
		Where("PK", "=", fmt.Sprintf("REMOTE_ACTOR#%s", handle)).
		Where("SK", "=", "PROFILE").
		First(&remoteActor)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get cached remote actor: %w", err)
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
		if errors.IsConditionFailed(err) {
			err = s.db.WithContext(ctx).Model(remoteActor).
				Where("PK", "=", remoteActor.PK).
				Where("SK", "=", remoteActor.SK).
				Update()
			if err != nil {
				return fmt.Errorf("failed to update cached remote actor: %w", err)
			}
		} else {
			return fmt.Errorf("failed to cache remote actor: %w", err)
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
	// Remove protocol
	withoutProtocol := strings.TrimPrefix(actorID, "https://")
	withoutProtocol = strings.TrimPrefix(withoutProtocol, "http://")

	// Split by /
	parts := strings.Split(withoutProtocol, "/")
	if len(parts) < 3 {
		return ""
	}

	// Extract domain and username
	domain := parts[0]

	// Find users part and get username
	for i, part := range parts {
		if part == "users" && i+1 < len(parts) {
			username := parts[i+1]
			return fmt.Sprintf("%s@%s", username, domain)
		}
	}

	return ""
}
