package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm"
	dmerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ListRepository handles list-related database operations
type ListRepository struct {
	db     dynamorm.DB
	logger *zap.Logger
}

// NewListRepository creates a new list repository
func NewListRepository(db dynamorm.DB, logger *zap.Logger) *ListRepository {
	return &ListRepository{
		db:     db,
		logger: logger,
	}
}

// CreateList creates a new list for a user
func (r *ListRepository) CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error) {
	// Validate replies policy
	if repliesPolicy == "" {
		repliesPolicy = "list" // default
	}
	if repliesPolicy != "followed" && repliesPolicy != "list" && repliesPolicy != "none" {
		return nil, fmt.Errorf("invalid replies policy: %s", repliesPolicy)
	}

	// Create the list model
	list := &models.List{
		ID:            uuid.New().String(),
		Username:      username,
		Title:         title,
		RepliesPolicy: repliesPolicy,
	}

	// Save to DynamoDB
	if err := r.db.WithContext(ctx).Model(list).Create(); err != nil {
		return nil, fmt.Errorf("failed to create list: %w", err)
	}

	// Convert to storage.List
	return &storage.List{
		ID:            list.ID,
		Username:      list.Username,
		Title:         list.Title,
		RepliesPolicy: list.RepliesPolicy,
		CreatedAt:     list.CreatedAt,
		UpdatedAt:     list.UpdatedAt,
	}, nil
}

// GetList retrieves a list by ID
func (r *ListRepository) GetList(ctx context.Context, listID string) (*storage.List, error) {
	var list models.List
	err := r.db.WithContext(ctx).Model(&models.List{}).
		Where("PK", "=", fmt.Sprintf("LIST#%s", listID)).
		Where("SK", "=", "METADATA").
		First(&list)

	if err != nil {
		if dmerrors.IsNotFound(err) {
			return nil, fmt.Errorf("list not found: %s", listID)
		}
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	// Convert to storage.List
	return &storage.List{
		ID:            list.ID,
		Username:      list.Username,
		Title:         list.Title,
		RepliesPolicy: list.RepliesPolicy,
		CreatedAt:     list.CreatedAt,
		UpdatedAt:     list.UpdatedAt,
	}, nil
}

// UpdateList updates a list's properties
func (r *ListRepository) UpdateList(ctx context.Context, listID string, updates map[string]any) error {
	// Get the existing list first
	list, err := r.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Create model from existing list
	model := &models.List{
		ID:            list.ID,
		Username:      list.Username,
		Title:         list.Title,
		RepliesPolicy: list.RepliesPolicy,
		CreatedAt:     list.CreatedAt,
		UpdatedAt:     list.UpdatedAt,
	}

	// Apply updates
	if title, ok := updates["title"].(string); ok {
		model.Title = title
	}

	if repliesPolicy, ok := updates["replies_policy"].(string); ok {
		// Validate replies policy
		if repliesPolicy != "followed" && repliesPolicy != "list" && repliesPolicy != "none" {
			return fmt.Errorf("invalid replies policy: %s", repliesPolicy)
		}
		model.RepliesPolicy = repliesPolicy
	}

	// Update in database
	if err := r.db.WithContext(ctx).Model(model).Update(); err != nil {
		return fmt.Errorf("failed to update list: %w", err)
	}

	return nil
}

// DeleteList deletes a list and all its memberships
func (r *ListRepository) DeleteList(ctx context.Context, listID string) error {
	// Get the list first to find the owner
	_, err := r.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Delete list metadata
	listModel := &models.List{
		PK: fmt.Sprintf("LIST#%s", listID),
		SK: "METADATA",
	}
	if err := r.db.WithContext(ctx).Model(listModel).Delete(); err != nil {
		return fmt.Errorf("failed to delete list: %w", err)
	}

	// Query and delete all list memberships
	var members []models.ListMember
	err = r.db.WithContext(ctx).Model(&models.ListMember{}).
		Where("PK", "=", fmt.Sprintf("LIST_MEMBERS#%s", listID)).
		Scan(&members)
	if err != nil && !dmerrors.IsNotFound(err) {
		return fmt.Errorf("failed to query list members: %w", err)
	}

	// Delete each membership
	for _, member := range members {
		if err := r.db.WithContext(ctx).Model(&member).Delete(); err != nil {
			r.logger.Warn("failed to delete list member",
				zap.String("list_id", listID),
				zap.String("account_id", member.AccountID),
				zap.Error(err))
		}
	}

	return nil
}

// GetListsForUser retrieves all lists owned by a user
func (r *ListRepository) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	var listModels []models.List
	err := r.db.WithContext(ctx).Model(&models.List{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER_LISTS#%s", username)).
		Scan(&listModels)

	if err != nil {
		if dmerrors.IsNotFound(err) {
			return []*storage.List{}, nil
		}
		return nil, fmt.Errorf("failed to query user lists: %w", err)
	}

	// Convert to storage.List
	lists := make([]*storage.List, len(listModels))
	for i, model := range listModels {
		lists[i] = &storage.List{
			ID:            model.ID,
			Username:      model.Username,
			Title:         model.Title,
			RepliesPolicy: model.RepliesPolicy,
			CreatedAt:     model.CreatedAt,
			UpdatedAt:     model.UpdatedAt,
		}
	}

	return lists, nil
}

// GetUserLists is an alias for GetListsForUser
func (r *ListRepository) GetUserLists(ctx context.Context, username string) ([]*storage.List, error) {
	return r.GetListsForUser(ctx, username)
}

// CountUserLists counts the number of lists owned by a user
func (r *ListRepository) CountUserLists(ctx context.Context, username string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.List{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER_LISTS#%s", username)).
		Count()

	if err != nil {
		return 0, fmt.Errorf("failed to count user lists: %w", err)
	}

	return int(count), nil
}

// AddAccountToList adds an account to a list
func (r *ListRepository) AddAccountToList(ctx context.Context, listID, accountID string) error {
	// Get the list to verify it exists and get the owner
	list, err := r.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Check if already in list
	exists, err := r.IsAccountInList(ctx, listID, accountID)
	if err != nil {
		return err
	}
	if exists {
		return nil // Already in list, no error
	}

	// Create membership
	member := &models.ListMember{
		ListID:       listID,
		AccountID:    accountID,
		ListUsername: list.Username,
	}

	// Save to DynamoDB
	if err := r.db.WithContext(ctx).Model(member).Create(); err != nil {
		return fmt.Errorf("failed to add account to list: %w", err)
	}

	return nil
}

// RemoveAccountFromList removes an account from a list
func (r *ListRepository) RemoveAccountFromList(ctx context.Context, listID, accountID string) error {
	// Get the list to verify it exists and get the owner
	list, err := r.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Delete membership
	member := &models.ListMember{
		PK: fmt.Sprintf("LIST_MEMBERS#%s", listID),
		SK: accountID,
		// Need to set GSI keys for deletion
		ListID:       listID,
		AccountID:    accountID,
		ListUsername: list.Username,
	}
	member.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(member).Delete(); err != nil {
		return fmt.Errorf("failed to remove account from list: %w", err)
	}

	return nil
}

// GetListMembers retrieves paginated list members
func (r *ListRepository) GetListMembers(ctx context.Context, listID string, limit int, cursor string) ([]*storage.ListMember, string, error) {
	if limit <= 0 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Where("PK", "=", fmt.Sprintf("LIST_MEMBERS#%s", listID))

	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	var members []models.ListMember
	err := query.Limit(limit).Scan(&members)
	if err != nil {
		if dmerrors.IsNotFound(err) {
			return []*storage.ListMember{}, "", nil
		}
		return nil, "", fmt.Errorf("failed to query list members: %w", err)
	}

	// Convert to storage.ListMember
	result := make([]*storage.ListMember, len(members))
	for i, member := range members {
		result[i] = &storage.ListMember{
			ListID:    member.ListID,
			AccountID: member.AccountID,
			AddedAt:   member.AddedAt,
		}
	}

	// Extract cursor from last member if we have results
	var nextCursorStr string
	if len(members) > 0 && len(members) == limit {
		nextCursorStr = members[len(members)-1].AccountID
	}

	return result, nextCursorStr, nil
}

// IsAccountInList checks if an account is in a list
func (r *ListRepository) IsAccountInList(ctx context.Context, listID, accountID string) (bool, error) {
	var member models.ListMember
	err := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Where("PK", "=", fmt.Sprintf("LIST_MEMBERS#%s", listID)).
		Where("SK", "=", accountID).
		First(&member)

	if err != nil {
		if dmerrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check list membership: %w", err)
	}

	return true, nil
}

// GetAccountLists retrieves all lists that contain an account
func (r *ListRepository) GetAccountLists(ctx context.Context, accountID string) ([]*storage.List, error) {
	// Query the reverse index
	var members []models.ListMember
	err := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("ACCOUNT_LISTS#%s", accountID)).
		Scan(&members)

	if err != nil {
		if dmerrors.IsNotFound(err) {
			return []*storage.List{}, nil
		}
		return nil, fmt.Errorf("failed to query account lists: %w", err)
	}

	// Get full list details for each membership
	lists := make([]*storage.List, 0, len(members))
	for _, member := range members {
		list, err := r.GetList(ctx, member.ListID)
		if err != nil {
			r.logger.Warn("failed to get list details",
				zap.String("list_id", member.ListID),
				zap.Error(err))
			continue
		}
		lists = append(lists, list)
	}

	return lists, nil
}

// GetAccountListsForUser retrieves all lists (for a specific user) that contain an account
func (r *ListRepository) GetAccountListsForUser(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	// Query the reverse index
	var members []models.ListMember
	err := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("ACCOUNT_LISTS#%s", accountID)).
		Scan(&members)

	if err != nil {
		if dmerrors.IsNotFound(err) {
			return []*storage.List{}, nil
		}
		return nil, fmt.Errorf("failed to query account lists: %w", err)
	}

	// Filter by username and get full list details
	lists := make([]*storage.List, 0)
	for _, member := range members {
		// Filter by username if specified
		if username != "" && member.ListUsername != username {
			continue
		}

		list, err := r.GetList(ctx, member.ListID)
		if err != nil {
			r.logger.Warn("failed to get list details",
				zap.String("list_id", member.ListID),
				zap.Error(err))
			continue
		}
		lists = append(lists, list)
	}

	return lists, nil
}

// CountListMembers counts the number of members in a list
func (r *ListRepository) CountListMembers(ctx context.Context, listID string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Where("PK", "=", fmt.Sprintf("LIST_MEMBERS#%s", listID)).
		Count()

	if err != nil {
		return 0, fmt.Errorf("failed to count list members: %w", err)
	}

	return int(count), nil
}

// RemoveAccountFromAllLists removes an account from all lists
func (r *ListRepository) RemoveAccountFromAllLists(ctx context.Context, accountID string) error {
	// Query all lists the account is in
	var members []models.ListMember
	err := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("ACCOUNT_LISTS#%s", accountID)).
		Scan(&members)

	if err != nil {
		if dmerrors.IsNotFound(err) {
			return nil // No lists to remove from
		}
		return fmt.Errorf("failed to query account lists: %w", err)
	}

	// Remove from each list
	for _, member := range members {
		if err := r.RemoveAccountFromList(ctx, member.ListID, accountID); err != nil {
			r.logger.Warn("failed to remove account from list",
				zap.String("list_id", member.ListID),
				zap.String("account_id", accountID),
				zap.Error(err))
		}
	}

	return nil
}

// GetListTimeline retrieves timeline entries for a list
func (r *ListRepository) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// This would typically query a timeline table filtered by list members
	// For now, return empty as timeline functionality is handled elsewhere
	return []*storage.TimelineEntry{}, "", nil
}

// GetExclusiveLists retrieves all exclusive lists for a user
func (r *ListRepository) GetExclusiveLists(ctx context.Context, username string) ([]*storage.List, error) {
	// Get all user lists
	lists, err := r.GetUserLists(ctx, username)
	if err != nil {
		return nil, err
	}

	// Filter for exclusive lists
	exclusive := make([]*storage.List, 0)
	for range lists {
		// Note: The legacy code doesn't have an Exclusive field, so this would need to be added
		// For now, return empty array
	}

	return exclusive, nil
}

// Batch operations for efficiency

// AddAccountsToList adds multiple accounts to a list
func (r *ListRepository) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	// Get the list to verify it exists and get the owner
	list, err := r.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Add each account
	for _, accountID := range accountIDs {
		// Check if already in list
		exists, err := r.IsAccountInList(ctx, listID, accountID)
		if err != nil {
			r.logger.Warn("failed to check if account in list",
				zap.String("list_id", listID),
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}
		if exists {
			continue // Skip if already in list
		}

		// Create membership
		member := &models.ListMember{
			ListID:       listID,
			AccountID:    accountID,
			ListUsername: list.Username,
		}

		// Save to DynamoDB
		if err := r.db.WithContext(ctx).Model(member).Create(); err != nil {
			r.logger.Error("failed to add account to list",
				zap.String("list_id", listID),
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// RemoveAccountsFromList removes multiple accounts from a list
func (r *ListRepository) RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error {
	// Get the list to verify it exists
	_, err := r.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Remove each account
	for _, accountID := range accountIDs {
		if err := r.RemoveAccountFromList(ctx, listID, accountID); err != nil {
			r.logger.Error("failed to remove account from list",
				zap.String("list_id", listID),
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// GetListAccounts retrieves all account IDs in a list
func (r *ListRepository) GetListAccounts(ctx context.Context, listID string) ([]string, error) {
	var members []models.ListMember
	err := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Where("PK", "=", fmt.Sprintf("LIST_MEMBERS#%s", listID)).
		Scan(&members)

	if err != nil {
		if dmerrors.IsNotFound(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to query list members: %w", err)
	}

	// Extract account IDs
	accountIDs := make([]string, len(members))
	for i, member := range members {
		accountIDs[i] = member.AccountID
	}

	return accountIDs, nil
}

// GetListsContainingAccount is an alias for GetAccountListsForUser
func (r *ListRepository) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	return r.GetAccountListsForUser(ctx, accountID, username)
}