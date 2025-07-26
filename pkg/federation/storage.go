package federation

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
)

// FederationStorage defines the minimal storage interface needed for federation delivery.
// This is a much smaller interface than storage.Storage, focused only on federation needs.
type FederationStorage interface {
	// Actor operations needed for federation
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	// Remote actor caching for federation efficiency
	GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error)
	CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error

	// Federation activity tracking and cost monitoring
	RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error
}