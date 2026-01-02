package federation

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
)

// RepositoryStorageAdapter adapts RepositoryStorage to implement FederationStorage
// This is a temporary adapter to ease the migration from custom interfaces
type RepositoryStorageAdapter struct {
	actorRepo        storageAdapterActorRepository
	relationshipRepo storageAdapterRelationshipRepository
	userRepo         storageAdapterUserRepository
	federationRepo   storageAdapterFederationRepository
}

// NewRepositoryStorageAdapter creates a new adapter
func NewRepositoryStorageAdapter(repos core.RepositoryStorage) FederationStorage {
	return &RepositoryStorageAdapter{
		actorRepo:        repos.Actor(),
		relationshipRepo: repos.Relationship(),
		userRepo:         repos.User(),
		federationRepo:   repos.Federation(),
	}
}

type storageAdapterActorRepository interface {
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
	GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error)
}

type storageAdapterRelationshipRepository interface {
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

type storageAdapterUserRepository interface {
	CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error
}

type storageAdapterFederationRepository interface {
	RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error
}

// GetActorPrivateKey retrieves the private key for an actor by username
func (a *RepositoryStorageAdapter) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	return a.actorRepo.GetActorPrivateKey(ctx, username)
}

// GetActor retrieves an actor by username
func (a *RepositoryStorageAdapter) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return a.actorRepo.GetActorByUsername(ctx, username)
}

// GetFollowers retrieves the list of follower usernames for an actor
func (a *RepositoryStorageAdapter) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return a.relationshipRepo.GetFollowers(ctx, username, limit, cursor)
}

// GetCachedRemoteActor retrieves a cached remote actor by handle
func (a *RepositoryStorageAdapter) GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	return a.actorRepo.GetCachedRemoteActor(ctx, actorID)
}

// CacheRemoteActor caches a remote actor with a TTL
func (a *RepositoryStorageAdapter) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	return a.userRepo.CacheRemoteActor(ctx, handle, actor, ttl)
}

// RecordFederationActivity records a federation activity
func (a *RepositoryStorageAdapter) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	// Check if there's a federation repository method for this
	return a.federationRepo.RecordFederationActivity(ctx, activity)
}
