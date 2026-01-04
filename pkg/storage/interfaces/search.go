// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
)

// SearchRepository defines the interface for search operations.
// This handles account search, status search, and search with privacy enforcement.
type SearchRepository interface {
	// ===== Account Search Operations =====

	// SearchAccounts searches for accounts matching the given query
	SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error)

	// SearchAccountsWithPrivacy searches for accounts with privacy enforcement
	SearchAccountsWithPrivacy(ctx context.Context, query string, limit int, followingOnly bool, offset int, searcherActorID string) ([]*activitypub.Actor, error)

	// SearchAccountsAdvanced searches for accounts with advanced filtering
	SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error)

	// ===== Status Search Operations =====

	// SearchStatuses searches for statuses matching the given query
	SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error)

	// SearchStatusesWithOptions searches for statuses with advanced options
	SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error)

	// SearchStatusesWithOptionsPaginated searches for statuses with advanced options and pagination support
	SearchStatusesWithOptionsPaginated(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, *PaginationResult, error)

	// SearchStatusesWithPrivacy searches for statuses with privacy enforcement
	SearchStatusesWithPrivacy(ctx context.Context, query string, options storage.StatusSearchOptions, searcherActorID string) ([]*storage.StatusSearchResult, error)

	// SearchStatusesWithPrivacyPaginated searches for statuses with privacy enforcement and pagination
	SearchStatusesWithPrivacyPaginated(ctx context.Context, query string, options storage.StatusSearchOptions, searcherActorID string) ([]*storage.StatusSearchResult, *PaginationResult, error)

	// SearchStatusesAdvanced searches for statuses with advanced filtering
	SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error)

	// ===== Dependency Management =====

	// SetDependencies sets the dependencies for cross-repository operations
	SetDependencies(deps SearchRepositoryDeps)
}

// SearchRepositoryDeps interface for dependencies - implemented by the storage adapter
type SearchRepositoryDeps interface {
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error)
	IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error)
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

// PaginationResult represents pagination information for search results
type PaginationResult struct {
	HasNextPage bool
	NextCursor  string
	TotalCount  int
}
