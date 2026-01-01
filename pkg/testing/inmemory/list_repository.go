// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
)

// ListRepository is a thread-safe in-memory implementation of interfaces.ListRepository.
type ListRepository struct {
	mu sync.RWMutex

	// Lists: key = listID
	lists map[string]*models.List

	// Index by user: username -> []listID
	byUser map[string][]string

	// List members: key = "listID:memberUsername"
	members map[string]bool

	// Index by list: listID -> []memberUsername
	membersByList map[string][]string

	// Index by member: memberUsername -> []listID
	listsByMember map[string][]string
}

// NewListRepository creates a new in-memory list repository
func NewListRepository() *ListRepository {
	return &ListRepository{
		lists:         make(map[string]*models.List),
		byUser:        make(map[string][]string),
		members:       make(map[string]bool),
		membersByList: make(map[string][]string),
		listsByMember: make(map[string][]string),
	}
}

// memberKey generates a unique key for a list membership
func memberKey(listID, memberUsername string) string {
	return fmt.Sprintf("%s:%s", listID, memberUsername)
}

// CreateList creates a new list
func (r *ListRepository) CreateList(_ context.Context, list *models.List) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if list == nil {
		return fmt.Errorf("list cannot be nil")
	}

	if list.ID == "" {
		list.ID = uuid.New().String()
	}

	if _, exists := r.lists[list.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	list.CreatedAt = now
	list.UpdatedAt = now

	r.lists[list.ID] = list
	r.byUser[list.Username] = append(r.byUser[list.Username], list.ID)

	return nil
}

// GetList retrieves a list by ID
func (r *ListRepository) GetList(_ context.Context, listID string) (*models.List, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list, exists := r.lists[listID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return list, nil
}

// UpdateList updates an existing list
func (r *ListRepository) UpdateList(_ context.Context, list *models.List) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if list == nil {
		return fmt.Errorf("list cannot be nil")
	}

	if _, exists := r.lists[list.ID]; !exists {
		return storage.ErrNotFound
	}

	list.UpdatedAt = time.Now()
	r.lists[list.ID] = list

	return nil
}

// DeleteList removes a list
func (r *ListRepository) DeleteList(_ context.Context, listID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	list, exists := r.lists[listID]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove all members
	for _, member := range r.membersByList[listID] {
		key := memberKey(listID, member)
		delete(r.members, key)
		r.listsByMember[member] = removeListKeyFromSlice(r.listsByMember[member], listID)
	}
	delete(r.membersByList, listID)

	// Remove from user index
	r.byUser[list.Username] = removeListKeyFromSlice(r.byUser[list.Username], listID)

	delete(r.lists, listID)

	return nil
}

// GetUserLists retrieves all lists for a user with pagination
func (r *ListRepository) GetUserLists(_ context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.List], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listIDs := r.byUser[username]
	if len(listIDs) == 0 {
		return &interfaces.PaginatedResult[*models.List]{Items: []*models.List{}}, nil
	}

	// Sort by creation time
	sortedLists := make([]*models.List, 0, len(listIDs))
	for _, id := range listIDs {
		if l, exists := r.lists[id]; exists {
			sortedLists = append(sortedLists, l)
		}
	}
	sort.Slice(sortedLists, func(i, j int) bool {
		return sortedLists[i].CreatedAt.After(sortedLists[j].CreatedAt)
	})

	return paginateListResults(sortedLists, opts), nil
}

// GetListsByMember retrieves all lists containing a member with pagination
func (r *ListRepository) GetListsByMember(_ context.Context, memberUsername string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.List], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listIDs := r.listsByMember[memberUsername]
	if len(listIDs) == 0 {
		return &interfaces.PaginatedResult[*models.List]{Items: []*models.List{}}, nil
	}

	sortedLists := make([]*models.List, 0, len(listIDs))
	for _, id := range listIDs {
		if l, exists := r.lists[id]; exists {
			sortedLists = append(sortedLists, l)
		}
	}
	sort.Slice(sortedLists, func(i, j int) bool {
		return sortedLists[i].CreatedAt.After(sortedLists[j].CreatedAt)
	})

	return paginateListResults(sortedLists, opts), nil
}

// GetListsForUser retrieves all lists for a user (legacy interface)
func (r *ListRepository) GetListsForUser(_ context.Context, username string) ([]*storage.List, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listIDs := r.byUser[username]
	result := make([]*storage.List, 0, len(listIDs))

	for _, id := range listIDs {
		if l, exists := r.lists[id]; exists {
			result = append(result, &storage.List{
				ID:            l.ID,
				Username:      l.Username,
				Title:         l.Title,
				RepliesPolicy: l.RepliesPolicy,
				Exclusive:     l.Exclusive,
				CreatedAt:     l.CreatedAt,
				UpdatedAt:     l.UpdatedAt,
			})
		}
	}

	return result, nil
}

// GetListsForUserPaginated retrieves lists for a user with pagination
func (r *ListRepository) GetListsForUserPaginated(_ context.Context, username string, limit int, cursor string) ([]*storage.List, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listIDs := r.byUser[username]
	if len(listIDs) == 0 {
		return []*storage.List{}, "", nil
	}

	// Sort by ID for consistent pagination
	sortedIDs := make([]string, len(listIDs))
	copy(sortedIDs, listIDs)
	sort.Strings(sortedIDs)

	safeLimit := clampListLimit(limit)

	startIdx := 0
	if cursor != "" {
		for i, id := range sortedIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*storage.List
	var nextCursor string

	for i := startIdx; i < len(sortedIDs) && len(results) < safeLimit; i++ {
		if l, exists := r.lists[sortedIDs[i]]; exists {
			results = append(results, &storage.List{
				ID:            l.ID,
				Username:      l.Username,
				Title:         l.Title,
				RepliesPolicy: l.RepliesPolicy,
				Exclusive:     l.Exclusive,
				CreatedAt:     l.CreatedAt,
				UpdatedAt:     l.UpdatedAt,
			})
		}
	}

	if startIdx+safeLimit < len(sortedIDs) && len(results) > 0 {
		nextCursor = results[len(results)-1].ID
	}

	return results, nextCursor, nil
}

// CountUserLists returns the number of lists owned by a user
func (r *ListRepository) CountUserLists(_ context.Context, username string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.byUser[username]), nil
}

// AddListMember adds a member to a list
func (r *ListRepository) AddListMember(_ context.Context, listID, memberUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.lists[listID]; !exists {
		return storage.ErrNotFound
	}

	key := memberKey(listID, memberUsername)
	if r.members[key] {
		return nil // Already a member
	}

	r.members[key] = true
	r.membersByList[listID] = append(r.membersByList[listID], memberUsername)
	r.listsByMember[memberUsername] = append(r.listsByMember[memberUsername], listID)

	return nil
}

// RemoveListMember removes a member from a list
func (r *ListRepository) RemoveListMember(_ context.Context, listID, memberUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := memberKey(listID, memberUsername)
	if !r.members[key] {
		return nil // Not a member
	}

	delete(r.members, key)
	r.membersByList[listID] = removeListKeyFromSlice(r.membersByList[listID], memberUsername)
	r.listsByMember[memberUsername] = removeListKeyFromSlice(r.listsByMember[memberUsername], listID)

	return nil
}

// GetListMembers retrieves all members of a list with pagination
func (r *ListRepository) GetListMembers(_ context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.lists[listID]; !exists {
		return nil, storage.ErrNotFound
	}

	members := r.membersByList[listID]
	if len(members) == 0 {
		return &interfaces.PaginatedResult[*storage.Account]{Items: []*storage.Account{}}, nil
	}

	// Create placeholder accounts for members
	accounts := make([]*storage.Account, 0, len(members))
	for _, member := range members {
		accounts = append(accounts, &storage.Account{
			User: &storage.User{Username: member},
		})
	}

	return paginateAccountResults(accounts, opts), nil
}

// IsListMember checks if a user is a member of a list
func (r *ListRepository) IsListMember(_ context.Context, listID, memberUsername string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := memberKey(listID, memberUsername)
	return r.members[key], nil
}

// CountListMembers returns the number of members in a list
func (r *ListRepository) CountListMembers(_ context.Context, listID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.membersByList[listID]), nil
}

// GetAccountLists retrieves lists containing an account
func (r *ListRepository) GetAccountLists(_ context.Context, accountID string) ([]*storage.List, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listIDs := r.listsByMember[accountID]
	result := make([]*storage.List, 0, len(listIDs))

	for _, id := range listIDs {
		if l, exists := r.lists[id]; exists {
			result = append(result, &storage.List{
				ID:            l.ID,
				Username:      l.Username,
				Title:         l.Title,
				RepliesPolicy: l.RepliesPolicy,
				Exclusive:     l.Exclusive,
				CreatedAt:     l.CreatedAt,
				UpdatedAt:     l.UpdatedAt,
			})
		}
	}

	return result, nil
}

// GetAccountListsPaginated retrieves lists containing an account with pagination
func (r *ListRepository) GetAccountListsPaginated(_ context.Context, accountID string, limit int, cursor string) ([]*storage.List, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listIDs := r.listsByMember[accountID]
	if len(listIDs) == 0 {
		return []*storage.List{}, "", nil
	}

	sortedIDs := make([]string, len(listIDs))
	copy(sortedIDs, listIDs)
	sort.Strings(sortedIDs)

	safeLimit := clampListLimit(limit)

	startIdx := 0
	if cursor != "" {
		for i, id := range sortedIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*storage.List
	var nextCursor string

	for i := startIdx; i < len(sortedIDs) && len(results) < safeLimit; i++ {
		if l, exists := r.lists[sortedIDs[i]]; exists {
			results = append(results, &storage.List{
				ID:            l.ID,
				Username:      l.Username,
				Title:         l.Title,
				RepliesPolicy: l.RepliesPolicy,
				Exclusive:     l.Exclusive,
				CreatedAt:     l.CreatedAt,
				UpdatedAt:     l.UpdatedAt,
			})
		}
	}

	if startIdx+safeLimit < len(sortedIDs) && len(results) > 0 {
		nextCursor = results[len(results)-1].ID
	}

	return results, nextCursor, nil
}

// GetAccountListsForUser retrieves lists owned by a user that contain an account
func (r *ListRepository) GetAccountListsForUser(_ context.Context, accountID, username string) ([]*storage.List, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listIDs := r.listsByMember[accountID]
	result := make([]*storage.List, 0)

	for _, id := range listIDs {
		if l, exists := r.lists[id]; exists && l.Username == username {
			result = append(result, &storage.List{
				ID:            l.ID,
				Username:      l.Username,
				Title:         l.Title,
				RepliesPolicy: l.RepliesPolicy,
				Exclusive:     l.Exclusive,
				CreatedAt:     l.CreatedAt,
				UpdatedAt:     l.UpdatedAt,
			})
		}
	}

	return result, nil
}

// RemoveAccountFromAllLists removes an account from all lists
func (r *ListRepository) RemoveAccountFromAllLists(_ context.Context, accountID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	listIDs := r.listsByMember[accountID]
	for _, listID := range listIDs {
		key := memberKey(listID, accountID)
		delete(r.members, key)
		r.membersByList[listID] = removeListKeyFromSlice(r.membersByList[listID], accountID)
	}
	delete(r.listsByMember, accountID)

	return nil
}

// GetExclusiveLists retrieves exclusive lists for a user
func (r *ListRepository) GetExclusiveLists(_ context.Context, username string) ([]*storage.List, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	listIDs := r.byUser[username]
	result := make([]*storage.List, 0)

	for _, id := range listIDs {
		if l, exists := r.lists[id]; exists && l.Exclusive {
			result = append(result, &storage.List{
				ID:            l.ID,
				Username:      l.Username,
				Title:         l.Title,
				RepliesPolicy: l.RepliesPolicy,
				Exclusive:     l.Exclusive,
				CreatedAt:     l.CreatedAt,
				UpdatedAt:     l.UpdatedAt,
			})
		}
	}

	return result, nil
}

// AddAccountsToList adds multiple accounts to a list
func (r *ListRepository) AddAccountsToList(_ context.Context, listID string, accountIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.lists[listID]; !exists {
		return storage.ErrNotFound
	}

	for _, accountID := range accountIDs {
		key := memberKey(listID, accountID)
		if !r.members[key] {
			r.members[key] = true
			r.membersByList[listID] = append(r.membersByList[listID], accountID)
			r.listsByMember[accountID] = append(r.listsByMember[accountID], listID)
		}
	}

	return nil
}

// RemoveAccountsFromList removes multiple accounts from a list
func (r *ListRepository) RemoveAccountsFromList(_ context.Context, listID string, accountIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, accountID := range accountIDs {
		key := memberKey(listID, accountID)
		if r.members[key] {
			delete(r.members, key)
			r.membersByList[listID] = removeListKeyFromSlice(r.membersByList[listID], accountID)
			r.listsByMember[accountID] = removeListKeyFromSlice(r.listsByMember[accountID], listID)
		}
	}

	return nil
}

// GetListAccounts retrieves all account IDs in a list
func (r *ListRepository) GetListAccounts(_ context.Context, listID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	members := r.membersByList[listID]
	result := make([]string, len(members))
	copy(result, members)

	return result, nil
}

// GetListsContainingAccount retrieves lists owned by a user that contain an account
func (r *ListRepository) GetListsContainingAccount(_ context.Context, accountID, username string) ([]*storage.List, error) {
	return r.GetAccountListsForUser(context.Background(), accountID, username)
}

// GetListTimeline retrieves statuses from list members (stub implementation)
func (r *ListRepository) GetListTimeline(_ context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.lists[listID]; !exists {
		return nil, storage.ErrNotFound
	}

	// Return empty result - timeline would require status repository integration
	return &interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{}}, nil
}

// GetListStatuses retrieves statuses from list members (stub implementation)
func (r *ListRepository) GetListStatuses(_ context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	return r.GetListTimeline(context.Background(), listID, opts)
}

// Helper functions

func removeListKeyFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func clampListLimit(limit int) int {
	const defaultLimit = 20
	const maxLimit = 100

	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func paginateListResults(lists []*models.List, opts interfaces.PaginationOptions) *interfaces.PaginatedResult[*models.List] {
	limit := clampListLimit(opts.Limit)

	startIdx := 0
	if opts.Cursor != "" {
		for i, l := range lists {
			if l.ID == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*models.List
	var nextCursor string

	for i := startIdx; i < len(lists) && len(results) < limit; i++ {
		results = append(results, lists[i])
	}

	if startIdx+limit < len(lists) && len(results) > 0 {
		nextCursor = results[len(results)-1].ID
	}

	return &interfaces.PaginatedResult[*models.List]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}
}

func paginateAccountResults(accounts []*storage.Account, opts interfaces.PaginationOptions) *interfaces.PaginatedResult[*storage.Account] {
	limit := clampListLimit(opts.Limit)

	startIdx := 0
	if opts.Cursor != "" {
		for i, a := range accounts {
			if a.User != nil && a.User.Username == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*storage.Account
	var nextCursor string

	for i := startIdx; i < len(accounts) && len(results) < limit; i++ {
		results = append(results, accounts[i])
	}

	if startIdx+limit < len(accounts) && len(results) > 0 {
		lastAccount := results[len(results)-1]
		if lastAccount.User != nil {
			nextCursor = lastAccount.User.Username
		}
	}

	return &interfaces.PaginatedResult[*storage.Account]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}
}

// Test helper methods

// Clear clears all data (test helper)
func (r *ListRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lists = make(map[string]*models.List)
	r.byUser = make(map[string][]string)
	r.members = make(map[string]bool)
	r.membersByList = make(map[string][]string)
	r.listsByMember = make(map[string][]string)
}

// GetListCount returns the number of lists (test helper)
func (r *ListRepository) GetListCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.lists)
}

// GetMemberCount returns the total number of memberships (test helper)
func (r *ListRepository) GetMemberCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.members)
}

// Ensure ListRepository implements interfaces.ListRepository
var _ interfaces.ListRepository = (*ListRepository)(nil)
