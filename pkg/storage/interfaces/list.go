// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ListRepository defines the interface for user list operations.
// This handles Mastodon-style user-created lists for timeline organization.
type ListRepository interface {
	// Core list operations
	CreateList(ctx context.Context, list *models.List) error
	GetList(ctx context.Context, listID string) (*models.List, error)
	UpdateList(ctx context.Context, list *models.List) error
	DeleteList(ctx context.Context, listID string) error

	// User list management
	GetUserLists(ctx context.Context, username string, opts PaginationOptions) (*PaginatedResult[*models.List], error)
	GetListsByMember(ctx context.Context, memberUsername string, opts PaginationOptions) (*PaginatedResult[*models.List], error)
	GetListsForUser(ctx context.Context, username string) ([]*storage.List, error)
	GetListsForUserPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.List, string, error)
	CountUserLists(ctx context.Context, username string) (int, error)

	// List membership operations
	AddListMember(ctx context.Context, listID, memberUsername string) error
	RemoveListMember(ctx context.Context, listID, memberUsername string) error
	GetListMembers(ctx context.Context, listID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)
	IsListMember(ctx context.Context, listID, memberUsername string) (bool, error)
	CountListMembers(ctx context.Context, listID string) (int, error)

	// Account list operations
	GetAccountLists(ctx context.Context, accountID string) ([]*storage.List, error)
	GetAccountListsPaginated(ctx context.Context, accountID string, limit int, cursor string) ([]*storage.List, string, error)
	GetAccountListsForUser(ctx context.Context, accountID, username string) ([]*storage.List, error)
	RemoveAccountFromAllLists(ctx context.Context, accountID string) error

	// Exclusive lists
	GetExclusiveLists(ctx context.Context, username string) ([]*storage.List, error)

	// Batch operations
	AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error
	RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error
	GetListAccounts(ctx context.Context, listID string) ([]string, error)
	GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error)

	// List timeline operations
	GetListTimeline(ctx context.Context, listID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetListStatuses(ctx context.Context, listID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
}
