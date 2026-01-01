// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ObjectRepository defines the interface for ActivityPub object operations.
// This handles object lifecycle, collections, quotes, threads, tombstones, and update history.
type ObjectRepository interface {
	// ===== Core Object Operations =====

	// CreateObject stores a generic ActivityPub object
	CreateObject(ctx context.Context, object any) error

	// GetObject retrieves an object by ID
	GetObject(ctx context.Context, id string) (any, error)

	// UpdateObject updates an existing object
	UpdateObject(ctx context.Context, object any) error

	// UpdateObjectWithHistory updates an object and tracks the edit history
	UpdateObjectWithHistory(ctx context.Context, object any, updatedBy string) error

	// DeleteObject deletes an object by ID
	DeleteObject(ctx context.Context, objectID string) error

	// GetObjectsByActor retrieves objects created by a specific actor
	GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error)

	// ===== Status Operations =====

	// GetStatus retrieves a status by ID (alias for GetObject)
	GetStatus(ctx context.Context, statusID string) (any, error)

	// GetUserStatusCount counts the number of statuses by a user
	GetUserStatusCount(ctx context.Context, userID string) (int, error)

	// GetStatusReplyCount counts replies to a specific status
	GetStatusReplyCount(ctx context.Context, statusID string) (int, error)

	// ===== Reply Operations =====

	// CountObjectReplies counts the number of replies to an object
	CountObjectReplies(ctx context.Context, objectID string) (int, error)

	// CountReplies counts the number of replies to an object using GSI6
	CountReplies(ctx context.Context, objectID string) (int, error)

	// GetReplies retrieves replies to an object with pagination
	GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error)

	// IncrementReplyCount increments the reply count for an object
	IncrementReplyCount(ctx context.Context, objectID string) error

	// GetReplyCount gets the reply count for a status
	GetReplyCount(ctx context.Context, statusID string) (int64, error)

	// ===== Tombstone Operations =====

	// TombstoneObject marks an object as deleted by creating a tombstone
	TombstoneObject(ctx context.Context, objectID string, deletedBy string) error

	// CreateTombstone creates a tombstone for a deleted object
	CreateTombstone(ctx context.Context, tombstone *models.Tombstone) error

	// GetTombstone retrieves a tombstone by object ID
	GetTombstone(ctx context.Context, objectID string) (*models.Tombstone, error)

	// IsTombstoned checks if an object has been tombstoned (deleted)
	IsTombstoned(ctx context.Context, objectID string) (bool, error)

	// GetTombstonesByActor retrieves all tombstones created by a specific actor
	GetTombstonesByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Tombstone, string, error)

	// GetTombstonesByType retrieves tombstones by their former type
	GetTombstonesByType(ctx context.Context, formerType string, limit int, cursor string) ([]*models.Tombstone, string, error)

	// CleanupExpiredTombstones removes tombstones that have exceeded their TTL
	CleanupExpiredTombstones(ctx context.Context, batchSize int) (int, error)

	// ReplaceObjectWithTombstone atomically replaces an object with a tombstone
	ReplaceObjectWithTombstone(ctx context.Context, objectID, formerType, deletedBy string) error

	// ===== Update History Operations =====

	// CreateUpdateHistory creates a new update history entry for an object
	CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error

	// GetUpdateHistory retrieves update history for an object
	GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error)

	// GetObjectHistory retrieves the version history of an object
	GetObjectHistory(ctx context.Context, objectID string) ([]*storage.UpdateHistory, error)

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

	// ===== Quote Operations =====

	// CountQuotes counts the number of quotes for a specific note
	CountQuotes(ctx context.Context, noteID string) (int, error)

	// CountWithdrawnQuotes counts the number of withdrawn quotes for a specific note
	CountWithdrawnQuotes(ctx context.Context, noteID string) (int, error)

	// CreateQuoteRelationship creates a new quote relationship between notes
	CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error

	// GetQuotesForNote retrieves quotes for a specific note with pagination
	GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error)

	// IsQuoted checks if a note is quoted by a specific actor
	IsQuoted(ctx context.Context, actorID, noteID string) (bool, error)

	// WithdrawQuote withdraws a quote by marking it as withdrawn
	WithdrawQuote(ctx context.Context, quoteNoteID string) error

	// WithdrawStatusFromQuotes withdraws a status from being quoted with proper cascade effects
	WithdrawStatusFromQuotes(ctx context.Context, statusID string) error

	// UpdateQuotePermissions updates the quote permissions for a status
	UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error

	// IsQuoteAllowed checks if a quote is allowed for a status by a quoter
	IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error)

	// GetQuoteType returns the quote type for a status
	GetQuoteType(ctx context.Context, statusID string) (string, error)

	// IsWithdrawnFromQuotes checks if a status is withdrawn from quotes
	IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error)

	// GetQuotesOfStatus retrieves quotes of a specific status
	GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error)

	// ===== Thread Operations =====

	// GetMissingReplies returns a list of known missing replies in a thread
	GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error)

	// MarkThreadAsSynced marks a thread as successfully synced
	MarkThreadAsSynced(ctx context.Context, statusID string) error

	// SyncThreadFromRemote syncs a thread from a remote server
	SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error)

	// SyncMissingRepliesFromRemote syncs missing replies from remote servers
	SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error)

	// GetThreadContext retrieves the thread context for a status with full hierarchy
	GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error)
}
