// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// SocialRepository defines the interface for social interaction operations.
// This handles blocks, mutes, announces (boosts), account pins, account notes, and status pins.
type SocialRepository interface {
	// Block operations

	// CreateBlock creates a new block relationship
	CreateBlock(ctx context.Context, block *storage.Block) error

	// DeleteBlock removes a block relationship
	DeleteBlock(ctx context.Context, actor, blockedActor string) error

	// GetBlock retrieves a specific block relationship
	GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error)

	// IsBlocked checks if targetActor is blocked by actor
	IsBlocked(ctx context.Context, actor, targetActor string) (bool, error)

	// GetBlockedUsers returns a paginated list of actors blocked by the given actor
	GetBlockedUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error)

	// GetBlockedByUsers returns a paginated list of actors who have blocked the given actor
	GetBlockedByUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error)

	// Mute operations

	// CreateMute creates a new mute relationship
	CreateMute(ctx context.Context, mute *storage.Mute) error

	// DeleteMute removes a mute relationship
	DeleteMute(ctx context.Context, actor, mutedActor string) error

	// GetMute retrieves a specific mute relationship
	GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error)

	// IsMuted checks if targetActor is muted by actor
	IsMuted(ctx context.Context, actor, targetActor string) (bool, error)

	// GetMutedUsers returns all actors muted by the given actor
	GetMutedUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error)

	// Announce (boost) operations

	// CreateAnnounce creates a new Announce activity
	CreateAnnounce(ctx context.Context, announce *storage.Announce) error

	// DeleteAnnounce removes an Announce activity
	DeleteAnnounce(ctx context.Context, actor, object string) error

	// GetAnnounce retrieves a specific Announce by actor and object
	GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error)

	// GetStatusAnnounces retrieves all announces for a specific object
	GetStatusAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error)

	// HasUserAnnounced checks if a user has announced a specific object
	HasUserAnnounced(ctx context.Context, actor, object string) (bool, error)

	// GetActorAnnounces retrieves all objects announced by a specific actor with pagination
	GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error)

	// CountObjectAnnounces returns the total number of announces for an object
	CountObjectAnnounces(ctx context.Context, objectID string) (int, error)

	// CascadeDeleteAnnounces deletes all announces for an object
	CascadeDeleteAnnounces(ctx context.Context, objectID string) error

	// Account pin operations

	// CreateAccountPin creates a new account pin (endorsed account)
	CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error

	// DeleteAccountPin deletes an account pin
	DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error

	// GetAccountPins retrieves all pinned accounts for a user
	GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error)

	// GetAccountPinsPaginated retrieves pinned accounts for a user with pagination
	GetAccountPinsPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.AccountPin, string, error)

	// IsAccountPinned checks if an account is pinned
	IsAccountPinned(ctx context.Context, username, pinnedActorID string) (bool, error)

	// Account note operations

	// CreateAccountNote creates a new private note on an account
	CreateAccountNote(ctx context.Context, note *storage.AccountNote) error

	// UpdateAccountNote updates an existing private note on an account
	UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error

	// DeleteAccountNote deletes a private note on an account
	DeleteAccountNote(ctx context.Context, username, targetActorID string) error

	// GetAccountNote retrieves a private note on an account
	GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error)

	// Status pin operations

	// CreateStatusPin creates a new status pin
	CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error

	// DeleteStatusPin deletes a status pin
	DeleteStatusPin(ctx context.Context, username, statusID string) error

	// GetStatusPins retrieves all pinned statuses for a user
	GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error)

	// GetStatusPinsPaginated retrieves pinned statuses for a user with pagination
	GetStatusPinsPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.StatusPin, string, error)

	// IsStatusPinned checks if a status is pinned by a user
	IsStatusPinned(ctx context.Context, username, statusID string) (bool, error)

	// ReorderStatusPins reorders pinned statuses
	ReorderStatusPins(ctx context.Context, username string, statusIDs []string) error

	// CountUserPinnedStatuses counts the number of pinned statuses for a user
	CountUserPinnedStatuses(ctx context.Context, username string) (int, error)
}
