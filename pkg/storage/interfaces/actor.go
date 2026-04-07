// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
)

// MigrationInfo represents account migration information
type MigrationInfo struct {
	AlsoKnownAs []string `json:"also_known_as"`
	MovedTo     string   `json:"moved_to,omitempty"`
}

// ActorRepository defines the interface for actor operations.
// Local actor read methods return canonical local actors by contract so
// federation callers do not need to rehydrate top-level actor identifiers
// after crossing this repository boundary.
type ActorRepository interface {
	// Core actor operations
	CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error
	// GetActor returns a canonical local actor for the requested username.
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	// GetActorByUsername returns the same canonical local actor contract as GetActor.
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
	GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error)
	// GetActorWithMetadata returns the same canonical local actor plus repository metadata.
	GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error)
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	UpdateActor(ctx context.Context, actor *activitypub.Actor) error
	UpdateActorLastStatusTime(ctx context.Context, username string) error
	SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error
	DeleteActor(ctx context.Context, username string) error

	// Search and discovery
	SearchAccounts(ctx context.Context, query string, limit int, resolve bool, offset int) ([]*activitypub.Actor, error)
	GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error)
	GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error)
	RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error

	// Remote actor caching
	GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error)

	// Migration operations
	UpdateAlsoKnownAs(ctx context.Context, username string, alsoKnownAs []string) error
	UpdateMovedTo(ctx context.Context, username string, movedTo string) error
	CheckAlsoKnownAs(ctx context.Context, username string, targetActorID string) (bool, error)
	GetActorMigrationInfo(ctx context.Context, username string) (*MigrationInfo, error)
}
