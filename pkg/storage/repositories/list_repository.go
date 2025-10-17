package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	dmerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ListRepository handles list-related database operations using enhanced patterns
type ListRepository struct {
	*EnhancedBaseRepository[*models.List]
	// Helper for ListMember operations
	memberRepo *EnhancedBaseRepository[*models.ListMember]
}

// NewListRepository creates a new list repository with enhanced functionality
func NewListRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *ListRepository {
	// Create enhanced repository optimized for list operations
	enhancedRepo := NewEnhancedBaseRepository[*models.List](db, tableName, logger, costService, "ListRepository", "list")

	// Set up enhanced services for list operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Lists cached for timeline performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for list events

	return &ListRepository{
		EnhancedBaseRepository: enhancedRepo,
		memberRepo: NewEnhancedBaseRepository[*models.ListMember](
			db, tableName, logger, costService, "ListMemberRepository", "listmember"),
	}
}

// CreateList creates a new list
func (r *ListRepository) CreateList(ctx context.Context, list *models.List) error {
	// Validate replies policy
	if err := common.ValidateRequiredParam("list.RepliesPolicy", list.RepliesPolicy); err != nil {
		list.RepliesPolicy = RepliesPolicyList // default
	}
	if list.RepliesPolicy != RepliesPolicyFollowed && list.RepliesPolicy != RepliesPolicyList && list.RepliesPolicy != RepliesPolicyNone {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityList, list.RepliesPolicy)
	}

	// Generate ID if not provided
	if err := common.ValidateRequiredParam("list.ID", list.ID); err != nil {
		list.ID = uuid.New().String()
	}

	// Use enhanced validation and creation with automatic permission checking and event emission
	if err := r.ValidateAndCreate(ctx, list); err != nil {
		r.logger.Error("failed to create list with enhanced validation",
			zap.String("list_id", list.ID),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return err
	}

	return nil
}

// GetList retrieves a list by ID
func (r *ListRepository) GetList(ctx context.Context, listID string) (*models.List, error) {
	list := &models.List{}
	pk := fmt.Sprintf("LIST#%s", listID)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, list)
	if err != nil {
		if dmerrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityList, listID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityList, listID)
	}

	return list, nil
}

// UpdateList updates a list's properties
func (r *ListRepository) UpdateList(ctx context.Context, list *models.List) error {
	// Validate replies policy if provided
	if list.RepliesPolicy != "" {
		if list.RepliesPolicy != RepliesPolicyFollowed && list.RepliesPolicy != RepliesPolicyList && list.RepliesPolicy != RepliesPolicyNone {
			return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityList, list.RepliesPolicy)
		}
	}

	// Use BaseRepository Update method
	return r.Update(ctx, list)
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
		SK: models.SKMetadata,
	}
	if err := r.db.WithContext(ctx).Model(listModel).Delete(); err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityList, listID)
	}

	// Query and delete all list memberships
	var members []models.ListMember
	err = r.db.WithContext(ctx).Model(&models.ListMember{}).
		Where("PK", "=", fmt.Sprintf("LIST_MEMBERS#%s", listID)).
		Scan(&members)
	if err != nil && !dmerrors.IsNotFound(err) {
		return ErrorHandler.HandleQueryError(err, EntityList, "member deletion")
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

// GetListsForUser retrieves all lists owned by a user (for backward compatibility)
func (r *ListRepository) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	// Use paginated version with reasonable default limit
	lists, _, err := r.GetListsForUserPaginated(ctx, username, 100, "")
	return lists, err
}

// GetListsForUserPaginated retrieves lists owned by a user with pagination
func (r *ListRepository) GetListsForUserPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.List, string, error) {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	query := r.db.WithContext(ctx).Model(&models.List{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER_LISTS#%s", username)).
		OrderBy("GSI1SK", "ASC")

	// Resume from the provided cursor when set
	if cursor != "" {
		query = query.Where("GSI1SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var listModels []models.List
	err := query.Scan(&listModels)
	if err != nil {
		if dmerrors.IsNotFound(err) {
			return []*storage.List{}, "", nil
		}
		return nil, "", ErrorHandler.HandleQueryError(err, EntityList, "user list pagination")
	}

	// Generate next cursor
	var nextCursor string
	hasMore := len(listModels) > limit
	if hasMore {
		// We got more results than requested, so there are more pages
		nextCursor = listModels[limit-1].GSI1SK
		// Trim results to requested limit
		listModels = listModels[:limit]
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

	return lists, nextCursor, nil
}

// GetUserLists is an alias for GetListsForUser
// GetUserLists gets all lists owned by a user with pagination
func (r *ListRepository) GetUserLists(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.List], error) {
	// Query lists with pagination
	query := r.db.WithContext(ctx).Model(&models.List{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER_LISTS#%s", username))

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	query = query.Limit(opts.Limit)

	if opts.Cursor != "" {
		query = query.Where("ID", ">", opts.Cursor)
	}

	var lists []models.List
	err := query.All(&lists)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityList, "user list management")
	}

	// Convert to models.List pointers
	result := &interfaces.PaginatedResult[*models.List]{
		Items: make([]*models.List, len(lists)),
	}

	for i := range lists {
		result.Items[i] = &lists[i]
	}

	// Set next cursor if we have more results
	if len(lists) == opts.Limit {
		result.NextCursor = lists[len(lists)-1].ID
	}

	return result, nil
}

// GetListsByMember gets all lists that contain a specific member
func (r *ListRepository) GetListsByMember(ctx context.Context, memberUsername string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.List], error) {
	// First get list IDs where user is a member
	query := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Index("GSI2"). // Assuming GSI2 is AccountID-based index
		Where("GSI2PK", "=", fmt.Sprintf("ACCOUNT_LISTS#%s", memberUsername))

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	query = query.Limit(opts.Limit)

	if opts.Cursor != "" {
		query = query.Where("ID", ">", opts.Cursor)
	}

	var members []models.ListMember
	err := query.All(&members)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityList, "member query")
	}

	// Now get the actual lists
	result := &interfaces.PaginatedResult[*models.List]{
		Items: make([]*models.List, 0, len(members)),
	}

	for _, member := range members {
		list, err := r.GetList(ctx, member.ListID)
		if err != nil {
			r.logger.Warn("failed to get list", zap.String("list_id", member.ListID), zap.Error(err))
			continue
		}
		result.Items = append(result.Items, list)
	}

	// Set next cursor if we have more results
	if len(members) == opts.Limit && len(members) > 0 {
		result.NextCursor = members[len(members)-1].ListID
	}

	return result, nil
}

// CountUserLists counts the number of lists owned by a user
func (r *ListRepository) CountUserLists(ctx context.Context, username string) (int, error) {
	// For GSI counts, we still need to use the direct query approach
	// BaseRepository.Count works with main table PK only
	count, err := r.db.WithContext(ctx).Model(&models.List{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER_LISTS#%s", username)).
		Count()

	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, EntityList, "user list counting")
	}

	return int(count), nil
}

// AddListMember adds a member to a list
func (r *ListRepository) AddListMember(ctx context.Context, listID, memberUsername string) error {
	// Get the list to verify it exists and get the owner
	list, err := r.GetList(ctx, listID)
	if err != nil {
		return err
	}

	// Check if already in list
	exists, err := r.IsListMember(ctx, listID, memberUsername)
	if err != nil {
		return err
	}
	if exists {
		return nil // Already in list, no error
	}

	// Create membership
	member := &models.ListMember{
		ListID:       listID,
		AccountID:    memberUsername,
		ListUsername: list.Username,
	}

	// Use enhanced repository for validation and creation
	return r.memberRepo.ValidateAndCreate(ctx, member)
}

// RemoveListMember removes a member from a list
func (r *ListRepository) RemoveListMember(ctx context.Context, listID, memberUsername string) error {
	// Use BaseRepository Delete method
	pk := fmt.Sprintf("LIST_MEMBERS#%s", listID)
	sk := memberUsername
	return r.memberRepo.Delete(ctx, pk, sk)
}

// GetListMembers retrieves paginated list members
// GetListMembers gets members of a list with pagination
func (r *ListRepository) GetListMembers(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Where("PK", "=", fmt.Sprintf("LIST_MEMBERS#%s", listID))

	if opts.Cursor != "" {
		query = query.Where("SK", ">", opts.Cursor)
	}

	var members []models.ListMember
	err := query.Limit(opts.Limit).Scan(&members)
	if err != nil {
		if dmerrors.IsNotFound(err) {
			return &interfaces.PaginatedResult[*storage.Account]{
				Items: []*storage.Account{},
			}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityList, "member timeline query")
	}

	// Fetch actual Account objects for each member
	result := &interfaces.PaginatedResult[*storage.Account]{
		Items: make([]*storage.Account, 0, len(members)),
	}

	for _, member := range members {
		// Get the user data
		var user models.User
		err := r.db.WithContext(ctx).Model(&models.User{}).
			Where("PK", "=", fmt.Sprintf("USER#%s", member.AccountID)).
			Where("SK", "=", models.SKMetadata).
			First(&user)
		if err != nil {
			r.logger.Warn("failed to get user for list member",
				zap.String("account_id", member.AccountID),
				zap.Error(err))
			continue
		}

		// Get actor data if exists
		var actor models.Actor
		err = r.db.WithContext(ctx).Model(&models.Actor{}).
			Where("PK", "=", fmt.Sprintf("ACTOR#%s", member.AccountID)).
			Where("SK", "=", "PROFILE").
			First(&actor)
		// Actor may not exist for local users, that's ok

		// Convert to storage.Account
		storageAccount := &storage.Account{
			User: &storage.User{
				Username:    user.Username,
				Email:       user.Email,
				DisplayName: user.DisplayName,
				CreatedAt:   user.CreatedAt,
				UpdatedAt:   user.UpdatedAt,
				Approved:    user.Approved,
				Suspended:   user.Suspended,
				Silenced:    user.Silenced,
				Role:        user.Role,
				Locale:      user.Locale,
			},
		}

		// Add actor data if available
		if err == nil && actor.Actor != nil {
			storageAccount.Actor = actor.Actor
		}

		result.Items = append(result.Items, storageAccount)
	}

	// Extract cursor from last member if we have results
	if len(members) > 0 && len(members) == opts.Limit {
		result.NextCursor = members[len(members)-1].AccountID
	}

	return result, nil
}

// IsListMember checks if a user is a member of a list
func (r *ListRepository) IsListMember(ctx context.Context, listID, memberUsername string) (bool, error) {
	pk := fmt.Sprintf("LIST_MEMBERS#%s", listID)
	sk := memberUsername
	return r.memberRepo.Exists(ctx, pk, sk)
}

// GetAccountLists retrieves all lists that contain an account (for backward compatibility)
func (r *ListRepository) GetAccountLists(ctx context.Context, accountID string) ([]*storage.List, error) {
	// Use paginated version with reasonable default limit
	lists, _, err := r.GetAccountListsPaginated(ctx, accountID, 100, "")
	return lists, err
}

// GetAccountListsPaginated retrieves lists that contain an account with pagination
func (r *ListRepository) GetAccountListsPaginated(ctx context.Context, accountID string, limit int, cursor string) ([]*storage.List, string, error) {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	// Query the reverse index
	query := r.db.WithContext(ctx).Model(&models.ListMember{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("ACCOUNT_LISTS#%s", accountID)).
		OrderBy("GSI1SK", "ASC")

	// Resume from the provided cursor when set
	if cursor != "" {
		query = query.Where("GSI1SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var members []models.ListMember
	err := query.Scan(&members)
	if err != nil {
		if dmerrors.IsNotFound(err) {
			return []*storage.List{}, "", nil
		}
		return nil, "", ErrorHandler.HandleQueryError(err, EntityList, "account lists query")
	}

	// Generate next cursor
	var nextCursor string
	hasMore := len(members) > limit
	if hasMore {
		// We got more results than requested, so there are more pages
		nextCursor = members[limit-1].GSI1SK
		// Trim results to requested limit
		members = members[:limit]
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
		// Convert models.List to storage.List
		storageList := &storage.List{
			ID:            list.ID,
			Username:      list.Username,
			Title:         list.Title,
			RepliesPolicy: list.RepliesPolicy,
			CreatedAt:     list.CreatedAt,
			UpdatedAt:     list.UpdatedAt,
		}
		lists = append(lists, storageList)
	}

	return lists, nextCursor, nil
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
		return nil, ErrorHandler.HandleQueryError(err, EntityList, "account lists query")
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
		// Convert models.List to storage.List
		storageList := &storage.List{
			ID:            list.ID,
			Username:      list.Username,
			Title:         list.Title,
			RepliesPolicy: list.RepliesPolicy,
			CreatedAt:     list.CreatedAt,
			UpdatedAt:     list.UpdatedAt,
		}
		lists = append(lists, storageList)
	}

	return lists, nil
}

// CountListMembers counts the number of members in a list
func (r *ListRepository) CountListMembers(ctx context.Context, listID string) (int, error) {
	pk := fmt.Sprintf("LIST_MEMBERS#%s", listID)
	return r.memberRepo.Count(ctx, pk)
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
		return ErrorHandler.HandleQueryError(err, EntityList, "member removal query")
	}

	// Remove from each list
	for _, member := range members {
		if err := r.RemoveListMember(ctx, member.ListID, accountID); err != nil {
			r.logger.Warn("failed to remove account from list",
				zap.String("list_id", member.ListID),
				zap.String("account_id", accountID),
				zap.Error(err))
		}
	}

	return nil
}

// GetExclusiveLists retrieves all exclusive lists for a user
func (r *ListRepository) GetExclusiveLists(ctx context.Context, username string) ([]*storage.List, error) {
	// Get all user lists using the paginated version
	lists, _, err := r.GetListsForUserPaginated(ctx, username, 100, "")
	if err != nil {
		return nil, err
	}

	// Filter for exclusive lists
	exclusive := make([]*storage.List, 0)
	// Note: The legacy code doesn't have an Exclusive field, so this would need to be added
	// For now, return empty array
	_ = lists // Avoid unused variable warning

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
		exists, err := r.IsListMember(ctx, listID, accountID)
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

		// Use enhanced repository for validation and creation
		if err := r.memberRepo.ValidateAndCreate(ctx, member); err != nil {
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
		if err := r.RemoveListMember(ctx, listID, accountID); err != nil {
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
	pk := fmt.Sprintf("LIST_MEMBERS#%s", listID)
	members, err := r.memberRepo.FindByPK(ctx, pk)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityList, "member accounts query")
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

// GetListTimeline retrieves the timeline for a list
func (r *ListRepository) GetListTimeline(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	// First get all members of the list
	membersResult, err := r.GetListMembers(ctx, listID, interfaces.PaginationOptions{Limit: 100})
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityList, listID)
	}

	if err := common.ValidateSliceNotEmpty("membersResult.Items", membersResult.Items); err != nil {
		return &interfaces.PaginatedResult[*models.Status]{
			Items: []*models.Status{},
		}, nil
	}

	// Build query for statuses from list members
	usernames := make([]string, len(membersResult.Items))
	for i, account := range membersResult.Items {
		if account.User != nil {
			usernames[i] = account.User.Username
		}
	}

	// Query statuses from these users
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	result := &interfaces.PaginatedResult[*models.Status]{
		Items: make([]*models.Status, 0, opts.Limit),
	}

	// Get statuses from each user and merge
	for _, username := range usernames {
		var statuses []models.Status
		query := r.db.WithContext(ctx).Model(&models.Status{}).
			Index("GSI1").
			Where("GSI1PK", "=", fmt.Sprintf("USER_TIMELINE#%s", username)).
			Limit(opts.Limit)

		if opts.Cursor != "" {
			query = query.Where("ID", ">", opts.Cursor)
		}

		err := query.All(&statuses)
		if err != nil {
			r.logger.Warn("failed to get user timeline", zap.String("username", username), zap.Error(err))
			continue
		}

		for i := range statuses {
			result.Items = append(result.Items, &statuses[i])
			if len(result.Items) >= opts.Limit {
				break
			}
		}

		if len(result.Items) >= opts.Limit {
			break
		}
	}

	// Set cursor if we have max results
	if len(result.Items) == opts.Limit && len(result.Items) > 0 {
		// Use the StatusID which should be the primary identifier
		lastStatus := result.Items[len(result.Items)-1]
		result.NextCursor = lastStatus.StatusID
	}

	return result, nil
}

// GetListStatuses is an alias for GetListTimeline (both return statuses from list members)
func (r *ListRepository) GetListStatuses(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	return r.GetListTimeline(ctx, listID, opts)
}
