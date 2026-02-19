package repositories

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/batch"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// UserRepositoryDeps interface for dependencies - implemented by the storage adapter
type UserRepositoryDeps interface {
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error)
	CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error
	GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	RemoveFollow(ctx context.Context, followerUsername, username string) error
}

// UserRepository implements user operations using enhanced DynamORM patterns
type UserRepository struct {
	*EnhancedBaseRepository[*models.User]
	deps         UserRepositoryDeps
	urlValidator *URLValidator
	logger       *zap.Logger // Keep reference for complex business logic
	tableName    string      // Keep reference for cost tracking
	bookmarkRepo *BookmarkRepository
}

// NewUserRepository creates a new user repository with enhanced functionality
func NewUserRepository(db core.DB, tableName string, logger *zap.Logger) *UserRepository {
	// Create enhanced repository optimized for user operations
	enhancedRepo := NewEnhancedBaseRepository[*models.User](db, tableName, logger, nil, "UserRepository", "user")

	// Set up enhanced services for user operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Users frequently accessed
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for user activity events

	return &UserRepository{
		EnhancedBaseRepository: enhancedRepo,
		urlValidator:           NewURLValidator(logger),
		logger:                 logger,
		tableName:              tableName,
	}
}

// NewUserRepositoryWithCostTracking creates a new user repository with cost tracking
func NewUserRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *UserRepository {
	// Create enhanced repository with cost tracking
	enhancedRepo := NewEnhancedBaseRepository[*models.User](db, tableName, logger, costService, "UserRepository", "user")

	// Set up enhanced services for user operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Users frequently accessed
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for user activity events

	return &UserRepository{
		EnhancedBaseRepository: enhancedRepo,
		urlValidator:           NewURLValidator(logger),
		logger:                 logger,
		tableName:              tableName,
	}
}

// SetDependencies sets the dependencies for cross-repository operations
func (r *UserRepository) SetDependencies(deps UserRepositoryDeps) {
	r.deps = deps
}

// SetBookmarkRepository injects the bookmark repository dependency.
func (r *UserRepository) SetBookmarkRepository(bookmarkRepo *BookmarkRepository) {
	r.bookmarkRepo = bookmarkRepo
}

func (r *UserRepository) getBookmarkRepository() *BookmarkRepository {
	if r.bookmarkRepo == nil {
		r.bookmarkRepo = NewBookmarkRepository(r.GetDB(), r.tableName, r.logger)
	}
	return r.bookmarkRepo
}

// CreateUser creates a new user in DynamoDB using BaseRepository pattern
func (r *UserRepository) CreateUser(ctx context.Context, user *storage.User) error {
	// Validate user entity using centralized validation
	if err := common.ValidateUserEntity(user.Username, user.Email); err != nil {
		return err
	}

	// Ensure timestamps are set before enhanced validation runs.
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = user.CreatedAt
	}

	// Create the DynamORM model
	userModel := &models.User{
		Username:          user.Username,
		Email:             user.Email,
		PasswordHash:      user.PasswordHash,
		DisplayName:       user.DisplayName,
		IsAgent:           user.IsAgent,
		AgentType:         user.AgentType,
		AgentCapabilities: user.AgentCapabilities,
		AgentVersion:      user.AgentVersion,
		AgentOwner:        user.AgentOwner,
		AgentCreatedBy:    user.AgentCreatedBy,
		AgentPublicKey:    user.AgentPublicKey,
		AgentKeyType:      user.AgentKeyType,
		Approved:          user.Approved,
		Suspended:         user.Suspended,
		Silenced:          user.Silenced,
		Role:              user.Role,
		Locale:            user.Locale,
		RecoveryMethods:   user.RecoveryMethods,
		CreatedAt:         user.CreatedAt,
		UpdatedAt:         user.UpdatedAt,
	}

	// Use enhanced validation and creation with automatic permission checking and event emission
	err := r.ValidateAndCreate(ctx, userModel)
	if err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Debug("user already exists",
				zap.String("username", user.Username),
				zap.Bool("validation_enabled", r.HasValidation()),
				zap.Bool("events_enabled", r.HasEvents()))
			return ErrorHandler.HandleCreateError(common.ConflictError{
				Resource: "user",
				Message:  fmt.Sprintf("user %s already exists", user.Username),
			}, EntityUser, user.Username)
		}
		r.logger.Error("failed to create user with enhanced validation",
			zap.String("username", user.Username),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityUser, user.Username)
	}

	r.logger.Info("created user with enhanced patterns",
		zap.String("username", user.Username),
		zap.String("role", user.Role))

	// Update the original user with timestamps
	user.CreatedAt = userModel.CreatedAt
	user.UpdatedAt = userModel.UpdatedAt

	return nil
}

// GetUser retrieves a user by username using BaseRepository pattern
func (r *UserRepository) GetUser(ctx context.Context, username string) (*storage.User, error) {
	userModel := &models.User{}
	pk := "USER#" + username
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, userModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(common.UserNotFoundError{Username: username}, EntityUser, username)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityUser, username)
	}

	return r.modelToStorage(userModel), nil
}

// GetUserByEmail retrieves a user by email address
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	// Validate email parameter using centralized validation
	if err := common.ValidateRequiredParam("email", email); err != nil {
		return nil, err
	}

	var userModels []models.User
	err := r.GetDB().WithContext(ctx).Model(&models.User{}).
		Index("gsi2").
		Where("gsi2PK", "=", "EMAIL#"+strings.ToLower(email)).
		Limit(1).
		All(&userModels)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityUser, "by email")
	}

	if err := common.ValidateSliceNotEmpty("user_models", userModels); err != nil {
		return nil, ErrorHandler.HandleGetError(common.UserNotFoundError{Username: email}, EntityUser, email)
	}

	return r.modelToStorage(&userModels[0]), nil
}

// UpdateUser updates an existing user using BaseRepository pattern
func (r *UserRepository) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
	// Validate username parameter
	if err := common.ValidateUsernameParamID(username); err != nil {
		return err
	}

	// Validate updates map
	if len(updates) == 0 {
		return ErrorHandler.HandleUpdateError(common.ValidationError{Field: "Updates", Message: "no updates provided"}, EntityUser, "updates validation")
	}

	// Get existing user first using BaseRepository
	userModel := &models.User{}
	pk := "USER#" + username
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, userModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(common.UserNotFoundError{Username: username}, EntityUser, username)
		}
		return ErrorHandler.HandleGetError(err, EntityUser, username)
	}

	// Apply updates to the model
	r.applyUpdates(userModel, updates)

	// Update using BaseRepository
	err = r.Update(ctx, userModel)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityUser, userModel.Username)
	}

	return nil
}

// DeleteUser deletes a user using BaseRepository pattern
func (r *UserRepository) DeleteUser(ctx context.Context, username string) error {
	pk := "USER#" + username
	sk := models.SKMetadata

	err := r.Delete(ctx, pk, sk)
	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(common.UserNotFoundError{Username: username}, EntityUser, username)
		}
		return ErrorHandler.HandleDeleteError(err, EntityUser, username)
	}

	return nil
}

// ListUsers retrieves a paginated list of users
func (r *UserRepository) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	// Validate limit using centralized validation
	if err := common.ValidateQueryLimit(int(limit), 100, "user listing"); err != nil {
		limit = 20
	}

	var userModels []models.User
	query := r.GetDB().WithContext(ctx).Model(&models.User{}).
		Index("gsi1").
		Where("gsi1PK", "=", "USERS").
		Limit(int(limit) + 1) // Request one extra to detect if there are more pages

	// Apply cursor if provided
	if cursor != "" {
		query = query.Where("gsi1SK", ">", cursor)
	}

	err := query.All(&userModels)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityUser, "list")
	}

	// Convert to storage.User slice
	users := make([]*storage.User, 0, len(userModels))
	for _, userModel := range userModels {
		users = append(users, r.modelToStorage(&userModel))
	}

	// Implement pagination using DynamORM's pagination features
	var nextCursor string

	// If we got more results than requested, there are more pages
	if len(userModels) > int(limit) {
		// Remove the extra item and create cursor from the last item
		userModels = userModels[:limit]
		if len(userModels) > 0 {
			lastUser := userModels[len(userModels)-1]
			// Create cursor from last user's sort key
			nextCursor = lastUser.SK
		}
	}

	return users, nextCursor, nil
}

// ListAgents retrieves a paginated list of local agent accounts.
func (r *UserRepository) ListAgents(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	if err := common.ValidateQueryLimit(int(limit), 100, "agent listing"); err != nil {
		limit = 20
	}

	var userModels []models.User
	query := r.GetDB().WithContext(ctx).Model(&models.User{}).
		Index("gsi6").
		Where("gsi6PK", "=", "ACCOUNT_TYPE#AGENT").
		Limit(int(limit) + 1)

	if cursor != "" {
		query = query.Where("gsi6SK", ">", cursor)
	}

	if err := query.All(&userModels); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityUser, "list agents")
	}

	users := make([]*storage.User, 0, len(userModels))
	for _, userModel := range userModels {
		users = append(users, r.modelToStorage(&userModel))
	}

	var nextCursor string
	if len(userModels) > int(limit) {
		userModels = userModels[:limit]
		if len(userModels) > 0 {
			nextCursor = userModels[len(userModels)-1].GSI6SK
		}
	}

	return users, nextCursor, nil
}

// GetActiveUserCount returns the number of active users
func (r *UserRepository) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	// Calculate cutoff time for activity
	cutoffTime := time.Now().AddDate(0, 0, -days)
	cutoffTimestamp := cutoffTime.Unix()

	// Query users who have been active within the specified days
	// Use the last_activity index if available, otherwise fall back to status check
	var userModels []models.User
	err := r.GetDB().WithContext(ctx).Model(&models.User{}).
		Index("gsi3").
		Where("gsi3PK", "=", "ACTIVITY").
		Where("gsi3SK", ">=", fmt.Sprintf("%d", cutoffTimestamp)).
		All(&userModels)
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, EntityUser, "count active")
	}

	return int64(len(userModels)), nil
}

// GetTotalUserCount returns the total number of users in the system
func (r *UserRepository) GetTotalUserCount(ctx context.Context) (int64, error) {
	r.logger.Debug("getting total user count")

	// Use GSI1 (user listing index) where all users have GSI1PK = "USERS"
	// This is much more efficient than scanning the main table
	count, err := r.GetDB().WithContext(ctx).Model(&models.User{}).
		Index("gsi1").
		Where("gsi1PK", "=", "USERS").
		Count()

	if err != nil {
		r.logger.Error("failed to count total users", zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityUser, "count total")
	}

	r.logger.Debug("retrieved total user count", zap.Int64("count", count))
	return count, nil
}

// GetUserByProviderID gets a user by their OAuth provider ID
func (r *UserRepository) GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error) {
	// Query the ProviderAccount by provider and providerID using GSI1
	var providerAccounts []models.ProviderAccount
	err := r.GetDB().WithContext(ctx).Model(&models.ProviderAccount{}).
		Index("gsi1").
		Where("gsi1PK", "=", "PROVIDER#"+provider).
		Where("gsi1SK", "=", providerID+"#").
		Limit(1).
		All(&providerAccounts)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "provider account", "query")
	}

	if err := common.ValidateSliceNotEmpty("provider_accounts", providerAccounts); err != nil {
		return nil, ErrorHandler.HandleGetError(common.UserNotFoundError{Username: fmt.Sprintf("%s:%s", provider, providerID)}, EntityUser, providerID)
	}

	// Now get the user by UserID
	return r.GetUser(ctx, providerAccounts[0].UserID)
}

// LinkProviderAccount links an OAuth provider account to a user
func (r *UserRepository) LinkProviderAccount(ctx context.Context, username, provider, providerID string) error {
	// First verify the user exists
	_, err := r.GetUser(ctx, username)
	if err != nil {
		return err
	}

	// Create the provider account link
	providerAccount := &models.ProviderAccount{
		UserID:     username,
		Provider:   provider,
		ProviderID: providerID,
		IsActive:   true,
	}

	// Create the provider account using DynamORM
	err = r.GetDB().WithContext(ctx).Model(providerAccount).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			return ErrorHandler.HandleCreateError(common.ConflictError{
				Resource: "provider_account",
				Message:  fmt.Sprintf("provider account %s:%s already linked", provider, providerID),
			}, "provider account", providerID)
		}
		return ErrorHandler.HandleCreateError(err, "provider account", providerID)
	}

	return nil
}

// UnlinkProviderAccount unlinks an OAuth provider account from a user
func (r *UserRepository) UnlinkProviderAccount(ctx context.Context, username, provider string) error {
	// Find the provider account for this user and provider
	// First get all provider accounts for this user
	var allProviderAccounts []models.ProviderAccount
	err := r.GetDB().WithContext(ctx).Model(&models.ProviderAccount{}).
		Index("gsi2").
		Where("gsi2PK", "=", "USER_PROVIDERS#"+username).
		All(&allProviderAccounts)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, "provider account", "query")
	}

	// Filter by provider manually since DynamORM might not support begins_with
	var providerAccounts []models.ProviderAccount
	for _, pa := range allProviderAccounts {
		if pa.Provider == provider {
			providerAccounts = append(providerAccounts, pa)
		}
	}

	if err := common.ValidateSliceNotEmpty("provider_accounts", providerAccounts); err != nil {
		return ErrorHandler.HandleGetError(common.ValidationError{Field: "provider account", Message: fmt.Sprintf("not found for user %s and provider %s", username, provider)}, "provider account", username)
	}

	// Delete the provider account(s) for this provider
	for _, pa := range providerAccounts {
		err = r.GetDB().WithContext(ctx).Model(&pa).Delete()
		if err != nil {
			return ErrorHandler.HandleDeleteError(err, "provider account", username)
		}
	}

	return nil
}

// GetLinkedProviders gets all linked OAuth providers for a user
func (r *UserRepository) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	// Query all provider accounts for this user
	var providerAccounts []models.ProviderAccount
	err := r.GetDB().WithContext(ctx).Model(&models.ProviderAccount{}).
		Index("gsi2").
		Where("gsi2PK", "=", "USER_PROVIDERS#"+username).
		All(&providerAccounts)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "provider account", "query")
	}

	// Extract unique provider names
	providers := make([]string, 0, len(providerAccounts))
	providerSet := make(map[string]bool)

	for _, pa := range providerAccounts {
		if pa.IsActive && !providerSet[pa.Provider] {
			providers = append(providers, pa.Provider)
			providerSet[pa.Provider] = true
		}
	}

	return providers, nil
}

// Helper methods

// modelToStorage converts a models.User to storage.User
func (r *UserRepository) modelToStorage(userModel *models.User) *storage.User {
	user := &storage.User{
		ID:                 common.GenerateNumericID(userModel.Username),
		Username:           userModel.Username,
		Email:              userModel.Email,
		PasswordHash:       userModel.PasswordHash,
		DisplayName:        userModel.DisplayName,
		Note:               userModel.Note,
		Avatar:             userModel.Avatar,
		Header:             userModel.Header,
		URL:                userModel.URL,
		Locked:             userModel.Locked,
		Discoverable:       userModel.Discoverable,
		Fields:             userModel.Fields,
		CreatedAt:          userModel.CreatedAt,
		UpdatedAt:          userModel.UpdatedAt,
		Approved:           userModel.Approved,
		Suspended:          userModel.Suspended,
		Silenced:           userModel.Silenced,
		Role:               userModel.Role,
		Locale:             userModel.Locale,
		RecoveryMethods:    userModel.RecoveryMethods,
		AllowNSFW:          userModel.AllowNSFW,
		RequireNSFWWarning: userModel.RequireNSFWWarning,
		Metadata:           userModel.Metadata,
		IsAgent:            userModel.IsAgent,
		AgentType:          userModel.AgentType,
		AgentCapabilities:  userModel.AgentCapabilities,
		AgentVersion:       userModel.AgentVersion,
		AgentOwner:         userModel.AgentOwner,
		AgentCreatedBy:     userModel.AgentCreatedBy,
		AgentPublicKey:     userModel.AgentPublicKey,
		AgentKeyType:       userModel.AgentKeyType,
		Version:            userModel.Version,
	}

	baseURL := strings.TrimSpace(config.Get().Domain)
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if baseURL != "" && strings.TrimSpace(user.URL) == "" {
		user.URL = fmt.Sprintf("%s/@%s", baseURL, userModel.Username)
	}

	return user
}

type userUpdateApplier func(*models.User, any)

var userUpdateAppliers = map[string]userUpdateApplier{
	"email": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.Email = v
		}
	},
	"password_hash": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.PasswordHash = v
		}
	},
	"display_name": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.DisplayName = v
		}
	},
	"approved": func(user *models.User, value any) {
		if v, ok := value.(bool); ok {
			user.Approved = v
		}
	},
	"suspended": func(user *models.User, value any) {
		if v, ok := value.(bool); ok {
			user.Suspended = v
		}
	},
	"silenced": func(user *models.User, value any) {
		if v, ok := value.(bool); ok {
			user.Silenced = v
		}
	},
	"role": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.Role = v
		}
	},
	"locale": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.Locale = v
		}
	},
	"recovery_methods": func(user *models.User, value any) {
		if v, ok := value.([]string); ok {
			user.RecoveryMethods = v
		}
	},
	"is_agent": func(user *models.User, value any) {
		if v, ok := value.(bool); ok {
			user.IsAgent = v
		}
	},
	"agent_type": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.AgentType = v
		}
	},
	"agent_version": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.AgentVersion = v
		}
	},
	"agent_owner": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.AgentOwner = v
		}
	},
	"agent_created_by": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.AgentCreatedBy = v
		}
	},
	"agent_public_key": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.AgentPublicKey = v
		}
	},
	"agent_key_type": func(user *models.User, value any) {
		if v, ok := value.(string); ok {
			user.AgentKeyType = v
		}
	},
	"agent_capabilities": func(user *models.User, value any) {
		if v, ok := value.(*agents.Capabilities); ok {
			user.AgentCapabilities = v
			return
		}
		if v, ok := value.(agents.Capabilities); ok {
			capsCopy := v
			user.AgentCapabilities = &capsCopy
		}
	},
}

// applyUpdates applies the updates map to the user model.
func (r *UserRepository) applyUpdates(userModel *models.User, updates map[string]any) {
	if userModel == nil || len(updates) == 0 {
		return
	}

	for key, value := range updates {
		if apply, ok := userUpdateAppliers[key]; ok {
			apply(userModel, value)
		}
	}
}

// CreateAccountPin creates a new account pin (endorsed account)
func (r *UserRepository) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	if pin.CreatedAt.IsZero() {
		pin.CreatedAt = time.Now()
	}

	// Check if already pinned
	exists, err := r.IsAccountPinned(ctx, pin.Username, pin.PinnedActorID)
	if err != nil {
		return err
	}
	if exists {
		return ErrorHandler.HandleCreateError(common.ConflictError{Resource: "account pin", Message: "account already pinned"}, "account pin", "already exists")
	}

	// Create the model
	pinModel := &models.AccountPin{
		Username:       pin.Username,
		PinnedActorID:  pin.PinnedActorID,
		PinnedUsername: pin.PinnedUsername,
		CreatedAt:      pin.CreatedAt,
	}
	_ = pinModel.UpdateKeys() // Ignore error as this is internal model operation

	// Create in DynamoDB
	err = r.GetDB().WithContext(ctx).Model(pinModel).Create()
	if err != nil {
		r.logger.Error("failed to create account pin", zap.Error(err))
		return err
	}

	return nil
}

// DeleteAccountPin deletes an account pin
func (r *UserRepository) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	// Create the pin to delete
	pin := &models.AccountPin{
		Username:      username,
		PinnedActorID: pinnedActorID,
	}
	_ = pin.UpdateKeys() // Ignore error as this is internal model operation

	// Delete from DynamoDB
	err := r.GetDB().WithContext(ctx).Model(pin).Delete()
	if err != nil {
		r.logger.Error("failed to delete account pin", zap.Error(err))
		return err
	}

	return nil
}

// GetAccountPins retrieves all pinned accounts for a user
func (r *UserRepository) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	// Query for all pins for this user
	var pins []models.AccountPin
	pk := fmt.Sprintf("ACCOUNT_PIN#%s", username)

	err := r.GetDB().WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", pk).
		Filter("SK", "BEGINS_WITH", "PIN#").
		All(&pins)
	if err != nil {
		r.logger.Error("failed to query account pins", zap.Error(err))
		return nil, err
	}

	// Convert to storage models
	result := make([]*storage.AccountPin, 0, len(pins))
	for _, pin := range pins {
		result = append(result, &storage.AccountPin{
			Username:       pin.Username,
			PinnedActorID:  pin.PinnedActorID,
			PinnedUsername: pin.PinnedUsername,
			CreatedAt:      pin.CreatedAt,
		})
	}

	return result, nil
}

// IsAccountPinned checks if an account is pinned
func (r *UserRepository) IsAccountPinned(ctx context.Context, username, actorID string) (bool, error) {
	// Create the pin to check
	pin := &models.AccountPin{
		Username:      username,
		PinnedActorID: actorID,
	}
	_ = pin.UpdateKeys() // Ignore error as this is internal model operation

	// Check if exists
	var found models.AccountPin
	err := r.GetDB().WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", pin.PK).
		Where("SK", "=", pin.SK).
		First(&found)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check account pin", zap.Error(err))
		return false, err
	}

	return true, nil
}

// CreateAccountNote creates a new private note on an account
func (r *UserRepository) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now()
	}
	if note.UpdatedAt.IsZero() {
		note.UpdatedAt = note.CreatedAt
	}

	// Create the model
	noteModel := &models.AccountNote{
		Username:      note.Username,
		TargetActorID: note.TargetActorID,
		Note:          note.Note,
		CreatedAt:     note.CreatedAt,
		UpdatedAt:     note.UpdatedAt,
	}
	_ = noteModel.UpdateKeys() // Ignore error as this is internal model operation

	// Create in DynamoDB
	err := r.GetDB().WithContext(ctx).Model(noteModel).Create()
	if err != nil {
		r.logger.Error("failed to create account note", zap.Error(err))
		return err
	}

	return nil
}

// GetAccountNote retrieves a private note on an account
func (r *UserRepository) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	// Create the note to find
	note := &models.AccountNote{
		Username:      username,
		TargetActorID: targetActorID,
	}
	_ = note.UpdateKeys() // Ignore error as this is internal model operation

	// Query from DynamoDB
	var found models.AccountNote
	err := r.GetDB().WithContext(ctx).Model(&models.AccountNote{}).
		Where("PK", "=", note.PK).
		Where("SK", "=", note.SK).
		First(&found)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityAccountNote, fmt.Sprintf("%s->%s", username, targetActorID))
		}
		r.logger.Error("failed to get account note", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityAccountNote, fmt.Sprintf("%s->%s", username, targetActorID))
	}

	// Convert to storage model
	return &storage.AccountNote{
		Username:      found.Username,
		TargetActorID: found.TargetActorID,
		Note:          found.Note,
		CreatedAt:     found.CreatedAt,
		UpdatedAt:     found.UpdatedAt,
	}, nil
}

// UpdateAccountNote updates an existing private note on an account
func (r *UserRepository) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	note.UpdatedAt = time.Now()

	// Create the model
	noteModel := &models.AccountNote{
		Username:      note.Username,
		TargetActorID: note.TargetActorID,
		Note:          note.Note,
		CreatedAt:     note.CreatedAt,
		UpdatedAt:     note.UpdatedAt,
	}
	_ = noteModel.UpdateKeys() // Ignore error as this is internal model operation

	// Update in DynamoDB (Put overwrites existing)
	err := r.GetDB().WithContext(ctx).Model(noteModel).Create()
	if err != nil {
		r.logger.Error("failed to update account note", zap.Error(err))
		return err
	}

	return nil
}

// DeleteAccountNote deletes a private note on an account
func (r *UserRepository) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	// Create the note to delete
	note := &models.AccountNote{
		Username:      username,
		TargetActorID: targetActorID,
	}
	_ = note.UpdateKeys() // Ignore error as this is internal model operation

	// Delete from DynamoDB
	err := r.GetDB().WithContext(ctx).Model(note).Delete()
	if err != nil {
		r.logger.Error("failed to delete account note", zap.Error(err))
		return err
	}

	return nil
}

// StoreReputation stores or updates a reputation record
func (r *UserRepository) StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error {
	// Create the model
	repModel := &models.Reputation{}

	// Update keys and fields
	if err := repModel.UpdateKeys(actorID, reputation); err != nil {
		r.logger.Error("failed to update reputation keys", zap.Error(err))
		return err
	}

	// Store in DynamoDB
	err := r.GetDB().WithContext(ctx).Model(repModel).Create()
	if err != nil {
		r.logger.Error("failed to store reputation", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "reputation", actorID)
	}

	return nil
}

// GetReputation retrieves the latest reputation for an actor
func (r *UserRepository) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	// Validate and extract username from actorID
	if err := common.ValidateEntityID(actorID, "actor"); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "reputation", actorID)
	}

	username := extractUsernameFromActorID(actorID)
	if err := common.ValidateEntityID(username, "user"); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "reputation", actorID)
	}

	// Build query for latest reputation
	pk := fmt.Sprintf("ACTOR#%s", username)
	skPrefix := "REP#"

	// Query for latest reputation (most recent first)
	var reputations []models.Reputation
	err := r.GetDB().WithContext(ctx).
		Model(&models.Reputation{}).
		Where("PK", "=", pk).
		Filter("SK", "BEGINS_WITH", skPrefix).
		OrderBy("SK", "DESC"). // Descending order to get latest first
		Limit(1).
		All(&reputations)

	if err != nil {
		r.logger.Error("failed to query reputation", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "reputation", "query")
	}

	// No reputation found
	if err := common.ValidateSliceNotEmpty("reputations", reputations); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "reputation", actorID)
	}

	// Convert to storage.Reputation
	repInterface, err := reputations[0].ToStorageReputation()
	if err != nil {
		r.logger.Error("failed to unmarshal reputation", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "reputation", "unmarshal")
	}

	// Convert interface back to storage.Reputation
	var reputation storage.Reputation
	repJSON, _ := json.Marshal(repInterface)
	if err := json.Unmarshal(repJSON, &reputation); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "reputation", "convert")
	}

	return &reputation, nil
}

// GetReputationHistory retrieves reputation history for an actor
func (r *UserRepository) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	// Extract username from actorID
	username := extractUsernameFromActorID(actorID)
	if err := common.ValidateEntityID(username, "user"); err != nil {
		return []*storage.Reputation{}, nil // Return empty slice when invalid actorID
	}

	// Build query
	pk := fmt.Sprintf("ACTOR#%s", username)
	skPrefix := "REP#"

	// Query for reputation history
	var reputations []models.Reputation
	query := r.GetDB().WithContext(ctx).
		Model(&models.Reputation{}).
		Where("PK", "=", pk).
		Filter("SK", "BEGINS_WITH", skPrefix).
		OrderBy("SK", "DESC") // Descending order (most recent first)

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&reputations)
	if err != nil {
		r.logger.Error("failed to query reputation history", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "reputation", "history")
	}

	// Convert to storage.Reputation slice
	history := make([]*storage.Reputation, 0, len(reputations))
	for _, rep := range reputations {
		repInterface, err := rep.ToStorageReputation()
		if err != nil {
			r.logger.Warn("Failed to unmarshal reputation data", zap.Error(err))
			continue
		}

		// Convert interface back to storage.Reputation
		var reputation storage.Reputation
		repJSON, _ := json.Marshal(repInterface)
		if err := json.Unmarshal(repJSON, &reputation); err != nil {
			r.logger.Warn("Failed to convert reputation", zap.Error(err))
			continue
		}
		history = append(history, &reputation)
	}

	return history, nil
}

// GetUserTrustScore retrieves the trust score for a user
func (r *UserRepository) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	// Get the latest reputation
	reputation, err := r.GetReputation(ctx, userID)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return 0.0, nil
		}
		return 0.0, err
	}

	// Return 0 if no reputation found
	if reputation == nil {
		return 0.0, nil
	}

	// Return the total score as a float
	return float64(reputation.TotalScore), nil
}

// extractUsernameFromActorID extracts username from actor ID
// This helper matches the legacy implementation
func extractUsernameFromActorID(actorID string) string {
	parts := strings.Split(actorID, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// generateRandomID generates a random ID string of the specified length
func generateRandomID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// CreateVouch creates a new vouch
func (r *UserRepository) CreateVouch(_ context.Context, vouch *storage.Vouch) error {
	// Generate vouch ID if not set
	if common.ValidateRequiredParam("vouch.ID", vouch.ID) != nil {
		vouch.ID = fmt.Sprintf("vouch-%d-%s", time.Now().Unix(), generateRandomID(8))
	}

	// Marshal vouch to JSON
	vouchJSON, err := json.Marshal(vouch)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "vouch", "marshal")
	}

	// Create DynamORM model
	vouchModel := &models.Vouch{
		VouchData: string(vouchJSON),
	}
	expiresAt := time.Time{}
	if vouch.ExpiresAt != nil {
		expiresAt = *vouch.ExpiresAt
	}
	vouchModel.UpdateKeys(vouch.ID, vouch.From, vouch.To, vouch.Active, vouch.CreatedAt, expiresAt)

	// Create in DynamoDB
	if err := r.GetDB().Model(vouchModel).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, "vouch", "store")
	}

	return nil
}

// GetVouch retrieves a vouch by ID
func (r *UserRepository) GetVouch(_ context.Context, vouchID string) (*storage.Vouch, error) {
	// Query by primary key
	var vouchModels []*models.Vouch
	err := r.GetDB().Model(&models.Vouch{}).
		Where("PK", "=", fmt.Sprintf("VOUCH#%s", vouchID)).
		Where("SK", "=", models.SKMetadata).
		Scan(&vouchModels)

	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, "vouch", "get")
	}

	// Return nil if not found
	if err := common.ValidateSliceNotEmpty("vouch_models", vouchModels); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "vouch", vouchID)
	}

	vouchModel := vouchModels[0]

	// Unmarshal vouch data
	if common.ValidateRequiredParam("vouchData", vouchModel.VouchData) != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "vouch", vouchID)
	}

	var vouch storage.Vouch
	if err := json.Unmarshal([]byte(vouchModel.VouchData), &vouch); err != nil {
		return nil, ErrorHandler.HandleGetError(err, "vouch", "unmarshal")
	}

	return &vouch, nil
}

// queryVouchesByGSI is a helper function to query vouches using a specific GSI
func (r *UserRepository) queryVouchesByGSI(actorID string, activeOnly bool, gsiIndex, keyPrefix, errorContext string) ([]*storage.Vouch, error) {
	// Query the specified GSI for vouches
	query := r.GetDB().Model(&models.Vouch{}).
		Index(gsiIndex).
		Where(fmt.Sprintf("%sPK", strings.ToLower(gsiIndex[:4])), "=", fmt.Sprintf("%s#%s", keyPrefix, actorID))

	// Add active filter if requested
	if activeOnly {
		query = query.Filter("Active", "=", true)
	}

	// Execute query
	var vouchModels []*models.Vouch
	if err := query.Scan(&vouchModels); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "vouch", errorContext)
	}

	// Convert to storage.Vouch slice
	vouches := make([]*storage.Vouch, 0, len(vouchModels))
	for _, model := range vouchModels {
		if common.ValidateRequiredParam("vouchData", model.VouchData) != nil {
			continue
		}

		var vouch storage.Vouch
		if err := json.Unmarshal([]byte(model.VouchData), &vouch); err != nil {
			r.logger.Warn("Failed to unmarshal vouch data", zap.Error(err))
			continue
		}
		vouches = append(vouches, &vouch)
	}

	return vouches, nil
}

// GetVouchesByActor retrieves vouches given by an actor
func (r *UserRepository) GetVouchesByActor(_ context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	return r.queryVouchesByGSI(actorID, activeOnly, "gsi1", "VOUCHER", "by actor")
}

// GetVouchesForActor retrieves vouches received by an actor
func (r *UserRepository) GetVouchesForActor(_ context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	return r.queryVouchesByGSI(actorID, activeOnly, "gsi2", "VOUCHEE", "for actor")
}

// UpdateVouchStatus updates the active status of a vouch
func (r *UserRepository) UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	// First get the vouch to update the JSON data
	vouch, err := r.GetVouch(ctx, vouchID)
	if err != nil {
		return err
	}
	if vouch == nil {
		return ErrorHandler.HandleGetError(common.ValidationError{Field: "vouch", Message: "not found"}, "vouch", vouchID)
	}

	// Update vouch fields
	vouch.Active = active
	vouch.Revoked = !active
	vouch.RevokedAt = revokedAt

	// Marshal updated vouch
	vouchJSON, err := json.Marshal(vouch)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "vouch", "marshal")
	}

	// Create model with updated data
	vouchModel := &models.Vouch{
		PK:        fmt.Sprintf("VOUCH#%s", vouchID),
		SK:        models.SKMetadata,
		VouchData: string(vouchJSON),
		Active:    active,
	}
	expiresAt := time.Time{}
	if vouch.ExpiresAt != nil {
		expiresAt = *vouch.ExpiresAt
	}
	vouchModel.UpdateKeys(vouch.ID, vouch.From, vouch.To, vouch.Active, vouch.CreatedAt, expiresAt)

	// Update in DynamoDB
	if err := r.GetDB().Model(vouchModel).Update(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "vouch", "status")
	}

	return nil
}

// GetMonthlyVouchCount gets the count of vouches created by an actor in a specific month
func (r *UserRepository) GetMonthlyVouchCount(_ context.Context, actorID string, year int, month time.Month) (int, error) {
	// Calculate start and end of month
	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	// Query GSI1 with date range filter
	query := r.GetDB().Model(&models.Vouch{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("VOUCHER#%s", actorID))

	// Execute query - we'll filter in memory since DynamORM doesn't support BETWEEN on non-key attributes
	var vouchModels []*models.Vouch
	if err := query.Scan(&vouchModels); err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "vouch", "monthly count")
	}

	// Count vouches in the specified month
	count := 0
	for _, model := range vouchModels {
		if model.CreatedAt.After(startOfMonth) && model.CreatedAt.Before(endOfMonth) {
			count++
		}
	}

	return count, nil
}

// Trust Relationship Methods

// CreateTrustRelationship creates or updates a trust relationship
func (r *UserRepository) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	// Generate ID if not set
	if common.ValidateRequiredParam("relationship.ID", relationship.ID) != nil {
		relationship.ID = fmt.Sprintf("trust_%s", generateRandomID(12))
	}

	// Set timestamps
	now := time.Now()
	relationship.Created = now
	relationship.Updated = now

	// Set TTL if not specified (1 year default)
	if relationship.TTL == 0 {
		relationship.TTL = now.Add(365 * 24 * time.Hour).Unix()
	}

	// Create the model
	model := &models.TrustRelationship{
		ID:         relationship.ID,
		TrusterID:  relationship.TrusterID,
		TrusteeID:  relationship.TrusteeID,
		Category:   relationship.Category,
		Score:      relationship.Score,
		Confidence: relationship.Confidence,
		Evidence:   convertToModelEvidence(relationship.Evidence),
		TTL:        relationship.TTL,
		Created:    relationship.Created,
		Updated:    relationship.Updated,
	}

	// Update all keys
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Save to DynamoDB
	if err := r.GetDB().Model(model).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, "trust relationship", "create")
	}

	r.logger.Debug("Created trust relationship",
		zap.String("id", relationship.ID),
		zap.String("truster", relationship.TrusterID),
		zap.String("trustee", relationship.TrusteeID),
		zap.Float64("score", relationship.Score),
	)

	// Invalidate cached trust scores
	r.invalidateTrustScoreCache(ctx, relationship.TrusteeID, string(relationship.Category))

	return nil
}

// GetTrustRelationship retrieves a specific trust relationship
func (r *UserRepository) GetTrustRelationship(_ context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	model := &models.TrustRelationship{}

	// Query using primary key
	err := r.GetDB().Model(model).
		Where("PK", "=", fmt.Sprintf("TRUST#%s#%s", trusterID, category)).
		Where("SK", "=", fmt.Sprintf("TRUSTEE#%s", trusteeID)).
		First(model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "trust relationship", fmt.Sprintf("%s->%s#%s", trusterID, trusteeID, category))
		}
		return nil, ErrorHandler.HandleGetError(err, "trust relationship", "get")
	}

	// Convert to storage type
	return r.modelToTrustRelationship(model), nil
}

// UpdateTrustRelationship updates an existing trust relationship
func (r *UserRepository) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	// Just use CreateTrustRelationship as it's an upsert operation
	relationship.Updated = time.Now()
	return r.CreateTrustRelationship(ctx, relationship)
}

// DeleteTrustRelationship removes a trust relationship
func (r *UserRepository) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	model := &models.TrustRelationship{
		PK: fmt.Sprintf("TRUST#%s#%s", trusterID, category),
		SK: fmt.Sprintf("TRUSTEE#%s", trusteeID),
	}

	if err := r.GetDB().Model(model).Delete(); err != nil {
		return ErrorHandler.HandleDeleteError(err, "trust relationship", "delete")
	}

	r.logger.Debug("Deleted trust relationship",
		zap.String("truster", trusterID),
		zap.String("trustee", trusteeID),
		zap.String("category", category),
	)

	// Invalidate cached trust scores
	r.invalidateTrustScoreCache(ctx, trusteeID, category)

	return nil
}

// GetTrustRelationships retrieves all trust relationships for a truster
func (r *UserRepository) GetTrustRelationships(_ context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	// We need to scan with filter since we want all categories
	// DynamORM doesn't support begins_with, so we'll filter in memory
	query := r.GetDB().Model(&models.TrustRelationship{}).
		Filter("Type", "=", "RELATIONSHIP").
		Limit(limit * 2) // Get more to account for filtering

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var models []*models.TrustRelationship
	err := query.Scan(&models)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "trust relationship", "scan")
	}

	// Filter by truster ID in memory
	relationships := make([]*storage.TrustRelationship, 0)
	expectedPrefix := fmt.Sprintf("TRUST#%s#", trusterID)
	for _, model := range models {
		if strings.HasPrefix(model.PK, expectedPrefix) {
			relationships = append(relationships, r.modelToTrustRelationship(model))
			if len(relationships) >= limit {
				break
			}
		}
	}

	// Scans do not expose native cursors in DynamORM, so we stop here
	nextCursor := ""

	return relationships, nextCursor, nil
}

// GetTrustedByRelationships retrieves all relationships where the actor is trusted
func (r *UserRepository) GetTrustedByRelationships(_ context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	// Use GSI1 to query by trustee
	// DynamORM doesn't support begins_with, so we'll filter in memory
	query := r.GetDB().Model(&models.TrustRelationship{}).
		Index("gsi1").
		Filter("Type", "=", "RELATIONSHIP").
		Limit(limit * 2) // Get more to account for filtering

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var models []*models.TrustRelationship
	err := query.Scan(&models)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "trust relationship", "scan trusted-by")
	}

	// Filter by trustee ID in memory
	relationships := make([]*storage.TrustRelationship, 0)
	expectedPrefix := fmt.Sprintf("TRUSTED#%s#", trusteeID)
	for _, model := range models {
		if strings.HasPrefix(model.GSI1PK, expectedPrefix) {
			relationships = append(relationships, r.modelToTrustRelationship(model))
			if len(relationships) >= limit {
				break
			}
		}
	}

	// Scans do not expose native cursors in DynamORM, so we stop here
	nextCursor := ""

	return relationships, nextCursor, nil
}

// GetTrustScore retrieves a cached trust score or calculates it
func (r *UserRepository) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	// First, try to get cached score
	cacheModel := &models.TrustScore{}
	cacheKey := fmt.Sprintf("SCORE#%s#%s", actorID, category)

	err := r.GetDB().Model(cacheModel).
		Where("PK", "=", cacheKey).
		Where("SK", "=", "CURRENT").
		First(cacheModel)

	if err == nil && cacheModel.CacheTTL.After(time.Now()) {
		// Cache hit and still valid
		return r.modelToTrustScore(cacheModel), nil
	}

	// Cache miss or expired, calculate new score
	score, err := r.calculateTrustScore(ctx, actorID, category)
	if err != nil {
		return nil, err
	}

	// Cache the score
	if err := r.UpdateTrustScore(ctx, score); err != nil {
		// Log the error but return the calculated score as it's still valid
		r.logger.Warn("failed to cache updated trust score",
			zap.String("actorID", actorID),
			zap.String("category", category),
			zap.Error(err))
	}

	return score, nil
}

// UpdateTrustScore updates a cached trust score
func (r *UserRepository) UpdateTrustScore(_ context.Context, score *storage.TrustScore) error {
	score.LastCalculated = time.Now()
	score.CacheTTL = score.LastCalculated.Add(2 * time.Hour) // 2 hour cache

	// Create the model
	model := &models.TrustScore{
		ActorID:         score.ActorID,
		Category:        score.Category,
		Score:           score.Score,
		DirectScore:     score.DirectScore,
		PropagatedScore: score.PropagatedScore,
		Confidence:      score.Confidence,
		TrusterCount:    score.TrusterCount,
		CategoryScores:  score.CategoryScores,
		LastCalculated:  score.LastCalculated,
		CacheTTL:        score.CacheTTL,
	}

	// Update keys
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Save to DynamoDB
	if err := r.GetDB().Model(model).Create(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "trust score", "update")
	}

	return nil
}

// RecordTrustUpdate records a trust score update event
func (r *UserRepository) RecordTrustUpdate(_ context.Context, update *storage.TrustUpdate) error {
	update.Timestamp = time.Now()

	// Generate event ID if not set
	if common.ValidateRequiredParam("update.EventID", update.EventID) != nil {
		update.EventID = generateRandomID(12)
	}

	// Create the model
	model := &models.TrustUpdate{
		ActorID:   update.ActorID,
		EventID:   update.EventID,
		Category:  update.Category,
		Delta:     update.Delta,
		Reason:    update.Reason,
		Timestamp: update.Timestamp,
	}

	// Update keys
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Save to DynamoDB
	if err := r.GetDB().Model(model).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, "trust update", "record")
	}

	r.logger.Debug("Recorded trust update",
		zap.String("actor", update.ActorID),
		zap.String("category", string(update.Category)),
		zap.Float64("delta", update.Delta),
		zap.String("reason", update.Reason),
	)

	return nil
}

// GetAllTrustRelationships retrieves all trust relationships for admin visualization
func (r *UserRepository) GetAllTrustRelationships(_ context.Context, limit int) ([]*storage.TrustRelationship, error) {
	// Scan with filter for type
	query := r.GetDB().Model(&models.TrustRelationship{}).
		Filter("Type", "=", "RELATIONSHIP").
		Limit(limit)

	var models []*models.TrustRelationship
	if err := query.Scan(&models); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "trust relationship", "scan all")
	}

	// Convert to storage types
	relationships := make([]*storage.TrustRelationship, len(models))
	for i, model := range models {
		relationships[i] = r.modelToTrustRelationship(model)
	}

	r.logger.Debug("Retrieved all trust relationships",
		zap.Int("count", len(relationships)),
		zap.Int("limit", limit),
	)

	return relationships, nil
}

// Helper methods

// invalidateTrustScoreCache invalidates cached trust scores for an actor
func (r *UserRepository) invalidateTrustScoreCache(_ context.Context, actorID, category string) {
	// Delete cached score
	cacheKey := fmt.Sprintf("SCORE#%s#%s", actorID, category)

	model := &models.TrustScore{
		PK: cacheKey,
		SK: "CURRENT",
	}

	if err := r.GetDB().Model(model).Delete(); err != nil {
		r.logger.Warn("Failed to invalidate trust score cache",
			zap.String("actor", actorID),
			zap.String("category", category),
			zap.Error(err),
		)
	}
}

// calculateTrustScore calculates the trust score for an actor using PageRank-inspired algorithm
func (r *UserRepository) calculateTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	score := r.initTrustScore(actorID, category)

	// Get direct trust relationships
	relationships, _, err := r.GetTrustedByRelationships(ctx, actorID, 100, "")
	if err != nil {
		return nil, err
	}

	if err := common.ValidateSliceNotEmpty("relationships", relationships); err != nil {
		return score, nil // No trust relationships
	}

	// Calculate direct trust and get truster scores for propagation
	trusterScores := r.calculateDirectTrustScore(score, relationships, category)

	// Calculate propagated trust through the network
	r.calculatePropagatedTrustScore(ctx, score, actorID, category, trusterScores)

	// Combine and finalize scores
	r.combineTrustScores(score)

	r.logTrustCalculation(actorID, category, score)

	return score, nil
}

// initTrustScore initializes a new trust score with defaults
func (r *UserRepository) initTrustScore(actorID, category string) *storage.TrustScore {
	return &storage.TrustScore{
		ActorID:         actorID,
		Category:        storage.TrustCategory(category),
		Score:           0.0,
		DirectScore:     0.0,
		PropagatedScore: 0.0,
		Confidence:      0.0,
		TrusterCount:    0,
		CategoryScores:  make(map[string]float64),
	}
}

// calculateDirectTrustScore calculates direct trust from immediate relationships
func (r *UserRepository) calculateDirectTrustScore(score *storage.TrustScore, relationships []*storage.TrustRelationship, category string) map[string]float64 {
	var totalWeight float64
	trusterScores := make(map[string]float64)

	for _, rel := range relationships {
		if r.isRelevantCategory(rel.Category, category) {
			weight := rel.Confidence
			score.DirectScore += rel.Score * weight
			totalWeight += weight
			score.TrusterCount++
			trusterScores[rel.TrusterID] = rel.Score * weight
		}
	}

	if totalWeight > 0 {
		score.DirectScore /= totalWeight
		score.Confidence = totalWeight / float64(score.TrusterCount)
	}

	return trusterScores
}

// isRelevantCategory checks if a trust category is relevant for scoring
func (r *UserRepository) isRelevantCategory(relCategory trust.TrustCategory, targetCategory string) bool {
	return string(relCategory) == targetCategory || relCategory == trust.TrustCategoryGeneral
}

// userTrustPropagationConfig contains configuration for trust propagation
type userTrustPropagationConfig struct {
	dampingFactor   float64
	maxDepth        int
	minTrustScore   float64
	propagationRate float64
	maxVisited      int
}

// defaultPropagationConfig returns default propagation configuration
func (r *UserRepository) defaultPropagationConfig() userTrustPropagationConfig {
	return userTrustPropagationConfig{
		dampingFactor:   0.85, // How much trust propagates through the network
		maxDepth:        3,    // Maximum depth of trust propagation
		minTrustScore:   0.1,  // Minimum trust score to propagate
		propagationRate: 0.5,  // How much of the trust score propagates to next level
		maxVisited:      100,  // Maximum number of actors to examine
	}
}

// propagationNode represents a node in the trust propagation graph
type propagationNode struct {
	actorID   string
	trustPath float64 // Accumulated trust along the path
	depth     int
}

// calculatePropagatedTrustScore calculates trust propagated through the network
func (r *UserRepository) calculatePropagatedTrustScore(ctx context.Context, score *storage.TrustScore, actorID, category string, trusterScores map[string]float64) {
	config := r.defaultPropagationConfig()

	visited := make(map[string]bool)
	visited[actorID] = true

	queue := r.initializePropagationQueue(trusterScores, config.minTrustScore)

	propagatedTrust, propagatedWeight := r.processPropagationQueue(ctx, queue, visited, category, config)

	if propagatedWeight > 0 {
		score.PropagatedScore = propagatedTrust / propagatedWeight
	}
}

// initializePropagationQueue creates initial queue from direct trusters
func (r *UserRepository) initializePropagationQueue(trusterScores map[string]float64, minTrustScore float64) []propagationNode {
	queue := make([]propagationNode, 0)

	for trusterID, trustValue := range trusterScores {
		if trustValue >= minTrustScore {
			queue = append(queue, propagationNode{
				actorID:   trusterID,
				trustPath: trustValue,
				depth:     1,
			})
		}
	}

	return queue
}

// processPropagationQueue processes the BFS queue for trust propagation
func (r *UserRepository) processPropagationQueue(ctx context.Context, queue []propagationNode, visited map[string]bool, category string, config userTrustPropagationConfig) (float64, float64) {
	propagatedTrust := 0.0
	propagatedWeight := 0.0

	for len(queue) > 0 && len(visited) < config.maxVisited {
		node := queue[0]
		queue = queue[1:]

		if !r.shouldProcessNode(node, visited, config.maxDepth) {
			continue
		}
		visited[node.actorID] = true

		contribution, newNodes := r.processNode(ctx, node, category, visited, config)
		if contribution > 0 {
			propagatedTrust += contribution
			propagatedWeight += r.calculateNodeWeight(node, config.propagationRate)
		}

		queue = append(queue, newNodes...)
	}

	return propagatedTrust, propagatedWeight
}

// shouldProcessNode determines if a node should be processed
func (r *UserRepository) shouldProcessNode(node propagationNode, visited map[string]bool, maxDepth int) bool {
	return !visited[node.actorID] && node.depth <= maxDepth
}

// processNode processes a single node in the propagation graph
func (r *UserRepository) processNode(ctx context.Context, node propagationNode, category string, visited map[string]bool, config userTrustPropagationConfig) (float64, []propagationNode) {
	// Get trust score of the current node
	nodeScore, err := r.GetTrustScore(ctx, node.actorID, category)
	if err != nil {
		r.logger.Warn("Failed to get trust score for propagation",
			zap.String("actor", node.actorID),
			zap.Error(err))
		return 0, nil
	}

	if nodeScore.Score < config.minTrustScore {
		return 0, nil
	}

	// Calculate contribution
	contribution := r.calculateContribution(node, nodeScore.Score, config)

	// Get next level nodes if not at max depth
	var newNodes []propagationNode
	if node.depth < config.maxDepth {
		newNodes = r.expandPropagation(ctx, node, contribution, category, visited)
	}

	return contribution, newNodes
}

// calculateContribution calculates trust contribution for a node
func (r *UserRepository) calculateContribution(node propagationNode, nodeScore float64, config userTrustPropagationConfig) float64 {
	propagationFactor := r.calculatePropagationFactor(node.depth, config.propagationRate)
	return node.trustPath * nodeScore * propagationFactor * config.dampingFactor
}

// calculatePropagationFactor calculates the propagation factor for a given depth
func (r *UserRepository) calculatePropagationFactor(depth int, propagationRate float64) float64 {
	factor := 1.0
	for i := 1; i < depth; i++ {
		factor *= propagationRate
	}
	return factor
}

// calculateNodeWeight calculates the weight for a node in propagation
func (r *UserRepository) calculateNodeWeight(node propagationNode, propagationRate float64) float64 {
	return node.trustPath * r.calculatePropagationFactor(node.depth, propagationRate)
}

// expandPropagation expands the propagation to the next level
func (r *UserRepository) expandPropagation(ctx context.Context, node propagationNode, contribution float64, category string, visited map[string]bool) []propagationNode {
	nodeRelationships, _, err := r.GetTrustedByRelationships(ctx, node.actorID, 50, "")
	if err != nil {
		return nil
	}

	var newNodes []propagationNode
	for _, rel := range nodeRelationships {
		if r.shouldAddToPropagation(rel, category, visited) {
			newNodes = append(newNodes, propagationNode{
				actorID:   rel.TrusterID,
				trustPath: contribution * rel.Score * rel.Confidence,
				depth:     node.depth + 1,
			})
		}
	}

	return newNodes
}

// shouldAddToPropagation determines if a relationship should be added to propagation
func (r *UserRepository) shouldAddToPropagation(rel *storage.TrustRelationship, category string, visited map[string]bool) bool {
	return !visited[rel.TrusterID] && r.isRelevantCategory(rel.Category, category)
}

// combineTrustScores combines direct and propagated scores into final score
func (r *UserRepository) combineTrustScores(score *storage.TrustScore) {
	const directWeight = 0.7
	const propagatedWeightFactor = 0.3

	if score.DirectScore > 0 && score.PropagatedScore > 0 {
		score.Score = (score.DirectScore * directWeight) + (score.PropagatedScore * propagatedWeightFactor)
	} else if score.DirectScore > 0 {
		score.Score = score.DirectScore
	} else {
		score.Score = score.PropagatedScore
	}

	// Apply bounds
	if score.Score > 1.0 {
		score.Score = 1.0
	} else if score.Score < 0.0 {
		score.Score = 0.0
	}
}

// logTrustCalculation logs debug information about trust calculation
func (r *UserRepository) logTrustCalculation(actorID, category string, score *storage.TrustScore) {
	r.logger.Debug("Calculated trust score with propagation",
		zap.String("actor", actorID),
		zap.String("category", category),
		zap.Float64("direct_score", score.DirectScore),
		zap.Float64("propagated_score", score.PropagatedScore),
		zap.Float64("final_score", score.Score))
}

// Model conversion helpers

func (r *UserRepository) modelToTrustRelationship(model *models.TrustRelationship) *storage.TrustRelationship {
	return &storage.TrustRelationship{
		ID:         model.ID,
		TrusterID:  model.TrusterID,
		TrusteeID:  model.TrusteeID,
		Category:   model.Category,
		Score:      model.Score,
		Confidence: model.Confidence,
		Evidence:   convertFromModelEvidence(model.Evidence),
		TTL:        model.TTL,
		Created:    model.Created,
		Updated:    model.Updated,
	}
}

func (r *UserRepository) modelToTrustScore(model *models.TrustScore) *storage.TrustScore {
	return &storage.TrustScore{
		ActorID:         model.ActorID,
		Category:        model.Category,
		Score:           model.Score,
		DirectScore:     model.DirectScore,
		PropagatedScore: model.PropagatedScore,
		Confidence:      model.Confidence,
		TrusterCount:    model.TrusterCount,
		CategoryScores:  model.CategoryScores,
		LastCalculated:  model.LastCalculated,
		CacheTTL:        model.CacheTTL,
	}
}

// User Preferences Methods

// GetUserLanguagePreference retrieves a user's preferred language
func (r *UserRepository) GetUserLanguagePreference(ctx context.Context, username string) (string, error) {
	prefs, err := r.GetUserPreferences(ctx, username)
	if err != nil {
		return "", err
	}

	if prefs != nil && prefs.Language != "" {
		return prefs.Language, nil
	}

	// Default to English if no preference set
	return "en", nil
}

// SetUserLanguagePreference updates a user's preferred language
func (r *UserRepository) SetUserLanguagePreference(ctx context.Context, username string, language string) error {
	// Get existing preferences or create new ones
	prefs, err := r.GetUserPreferences(ctx, username)
	if err != nil {
		// If preferences don't exist, create new ones with defaults
		defaultPrefs := models.GetDefaultPreferences()
		prefs = &storage.UserPreferences{
			Username:                  username,
			Language:                  language,
			DefaultPostingVisibility:  defaultPrefs.DefaultPostingVisibility,
			DefaultMediaSensitive:     defaultPrefs.DefaultMediaSensitive,
			ExpandSpoilers:            defaultPrefs.ExpandSpoilers,
			ExpandMedia:               defaultPrefs.ExpandMedia,
			AutoplayGifs:              defaultPrefs.AutoplayGifs,
			ShowFollowCounts:          defaultPrefs.ShowFollowCounts,
			PreferredTimelineOrder:    defaultPrefs.PreferredTimelineOrder,
			SearchSuggestionsEnabled:  defaultPrefs.SearchSuggestionsEnabled,
			PersonalizedSearchEnabled: defaultPrefs.PersonalizedSearchEnabled,
			ReblogFilters:             defaultPrefs.ReblogFilters,
			StreamingDefaultQuality:   defaultPrefs.StreamingDefaultQuality,
			StreamingAutoQuality:      defaultPrefs.StreamingAutoQuality,
			StreamingPreloadNext:      defaultPrefs.StreamingPreloadNext,
			StreamingDataSaver:        defaultPrefs.StreamingDataSaver,
			Preferences:               make(map[string]string),
			UpdatedAt:                 time.Now(),
		}
	}

	// Update language
	prefs.Language = language

	return r.UpdateUserPreferences(ctx, username, prefs)
}

// GetUserPreferences retrieves all user preferences
func (r *UserRepository) GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error) {
	var prefModel models.UserPreferences

	// Set the keys and query for preferences
	prefModel.Username = username
	prefModel.UpdateKeys() // Internal model operation

	err := r.GetDB().WithContext(ctx).Model(&models.UserPreferences{}).
		Where("PK", "=", prefModel.PK).
		Where("SK", "=", prefModel.SK).
		First(&prefModel)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return default preferences if none exist
			defaultModelStorage := models.GetDefaultPreferences()
			return &storage.UserPreferences{
				Language:                  defaultModelStorage.Language,
				DefaultPostingVisibility:  defaultModelStorage.DefaultPostingVisibility,
				DefaultMediaSensitive:     defaultModelStorage.DefaultMediaSensitive,
				DirectMessagesFrom:        defaultModelStorage.DirectMessagesFrom,
				ExpandSpoilers:            defaultModelStorage.ExpandSpoilers,
				ExpandMedia:               defaultModelStorage.ExpandMedia,
				AutoplayGifs:              defaultModelStorage.AutoplayGifs,
				ShowFollowCounts:          defaultModelStorage.ShowFollowCounts,
				PreferredTimelineOrder:    defaultModelStorage.PreferredTimelineOrder,
				SearchSuggestionsEnabled:  defaultModelStorage.SearchSuggestionsEnabled,
				PersonalizedSearchEnabled: defaultModelStorage.PersonalizedSearchEnabled,
				ReblogFilters:             defaultModelStorage.ReblogFilters,
				StreamingDefaultQuality:   defaultModelStorage.StreamingDefaultQuality,
				StreamingAutoQuality:      defaultModelStorage.StreamingAutoQuality,
				StreamingPreloadNext:      defaultModelStorage.StreamingPreloadNext,
				StreamingDataSaver:        defaultModelStorage.StreamingDataSaver,
			}, nil
		}
		r.logger.Error("failed to get user preferences",
			zap.String("username", username),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "user preferences", "get")
	}

	// Convert models.UserPreferencesStorage to storage.UserPreferences
	modelStorage := prefModel.ToStorage()
	return &storage.UserPreferences{
		Language:                  modelStorage.Language,
		DefaultPostingVisibility:  modelStorage.DefaultPostingVisibility,
		DefaultMediaSensitive:     modelStorage.DefaultMediaSensitive,
		DirectMessagesFrom:        modelStorage.DirectMessagesFrom,
		ExpandSpoilers:            modelStorage.ExpandSpoilers,
		ExpandMedia:               modelStorage.ExpandMedia,
		AutoplayGifs:              modelStorage.AutoplayGifs,
		ShowFollowCounts:          modelStorage.ShowFollowCounts,
		PreferredTimelineOrder:    modelStorage.PreferredTimelineOrder,
		SearchSuggestionsEnabled:  modelStorage.SearchSuggestionsEnabled,
		PersonalizedSearchEnabled: modelStorage.PersonalizedSearchEnabled,
		ReblogFilters:             modelStorage.ReblogFilters,
		StreamingDefaultQuality:   modelStorage.StreamingDefaultQuality,
		StreamingAutoQuality:      modelStorage.StreamingAutoQuality,
		StreamingPreloadNext:      modelStorage.StreamingPreloadNext,
		StreamingDataSaver:        modelStorage.StreamingDataSaver,
	}, nil
}

// UpdateUserPreferences updates user preferences
func (r *UserRepository) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	// Convert storage.UserPreferences to models.UserPreferencesStorage
	modelStorage := &models.UserPreferencesStorage{
		Language:                  preferences.Language,
		DefaultPostingVisibility:  preferences.DefaultPostingVisibility,
		DefaultMediaSensitive:     preferences.DefaultMediaSensitive,
		DirectMessagesFrom:        preferences.DirectMessagesFrom,
		ExpandSpoilers:            preferences.ExpandSpoilers,
		ExpandMedia:               preferences.ExpandMedia,
		AutoplayGifs:              preferences.AutoplayGifs,
		ShowFollowCounts:          preferences.ShowFollowCounts,
		PreferredTimelineOrder:    preferences.PreferredTimelineOrder,
		SearchSuggestionsEnabled:  preferences.SearchSuggestionsEnabled,
		PersonalizedSearchEnabled: preferences.PersonalizedSearchEnabled,
		ReblogFilters:             preferences.ReblogFilters,
		StreamingDefaultQuality:   preferences.StreamingDefaultQuality,
		StreamingAutoQuality:      preferences.StreamingAutoQuality,
		StreamingPreloadNext:      preferences.StreamingPreloadNext,
		StreamingDataSaver:        preferences.StreamingDataSaver,
	}

	// Create DynamORM model from storage preferences
	prefModel := &models.UserPreferences{}
	prefModel.FromStorage(username, modelStorage)

	// Create or update the preferences using DynamORM
	err := r.GetDB().WithContext(ctx).Model(prefModel).Create()
	if err != nil {
		r.logger.Error("failed to update user preferences",
			zap.String("username", username),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "user preferences", "update")
	}

	r.logger.Debug("updated user preferences",
		zap.String("username", username),
		zap.String("language", preferences.Language))

	return nil
}

// SetPreference sets a specific preference key-value pair
func (r *UserRepository) SetPreference(ctx context.Context, username, key string, value any) error {
	// Get existing preferences
	prefs, err := r.GetUserPreferences(ctx, username)
	if err != nil {
		return err
	}

	// Update the specific preference field
	if err := r.updatePreferenceField(prefs, key, value); err != nil {
		return err
	}

	// Update the preferences
	return r.UpdateUserPreferences(ctx, username, prefs)
}

// updatePreferenceField updates a specific preference field based on the key
func (r *UserRepository) updatePreferenceField(prefs *storage.UserPreferences, key string, value any) error {
	switch key {
	case PrefKeyLanguage:
		return r.setStringPreference(&prefs.Language, value, key)
	case PrefKeyDefaultPostingVisibility:
		return r.setStringPreference(&prefs.DefaultPostingVisibility, value, key)
	case PrefKeyDefaultMediaSensitive:
		return r.setBoolPreference(&prefs.DefaultMediaSensitive, value, key)
	case PrefKeyDirectMessagesFrom:
		return r.setStringPreference(&prefs.DirectMessagesFrom, value, key)
	case PrefKeyExpandSpoilers:
		return r.setBoolPreference(&prefs.ExpandSpoilers, value, key)
	case PrefKeyExpandMedia:
		return r.setStringPreference(&prefs.ExpandMedia, value, key)
	case PrefKeyAutoplayGifs:
		return r.setBoolPreference(&prefs.AutoplayGifs, value, key)
	case PrefKeyShowFollowCounts:
		return r.setBoolPreference(&prefs.ShowFollowCounts, value, key)
	case PrefKeyPreferredTimelineOrder:
		return r.setStringPreference(&prefs.PreferredTimelineOrder, value, key)
	case PrefKeySearchSuggestionsEnabled:
		return r.setBoolPreference(&prefs.SearchSuggestionsEnabled, value, key)
	case PrefKeyPersonalizedSearchEnabled:
		return r.setBoolPreference(&prefs.PersonalizedSearchEnabled, value, key)
	case PrefKeyReblogFilters:
		return r.setReblogFiltersPreference(&prefs.ReblogFilters, value, key)
	case PrefKeyStreamingDefaultQuality:
		return r.setStringPreference(&prefs.StreamingDefaultQuality, value, key)
	case PrefKeyStreamingAutoQuality:
		return r.setBoolPreference(&prefs.StreamingAutoQuality, value, key)
	case PrefKeyStreamingPreloadNext:
		return r.setBoolPreference(&prefs.StreamingPreloadNext, value, key)
	case PrefKeyStreamingDataSaver:
		return r.setBoolPreference(&prefs.StreamingDataSaver, value, key)
	default:
		// Store unknown preferences in the generic Preferences map
		if prefs.Preferences == nil {
			prefs.Preferences = make(map[string]string)
		}
		prefs.Preferences[key] = fmt.Sprintf("%v", value)
		return nil
	}
}

// GetPreference gets a specific preference value
func (r *UserRepository) GetPreference(ctx context.Context, username, key string) (any, error) {
	prefs, err := r.GetUserPreferences(ctx, username)
	if err != nil {
		return nil, err
	}

	switch key {
	case PrefKeyLanguage:
		return prefs.Language, nil
	case PrefKeyDefaultPostingVisibility:
		return prefs.DefaultPostingVisibility, nil
	case PrefKeyDefaultMediaSensitive:
		return prefs.DefaultMediaSensitive, nil
	case PrefKeyDirectMessagesFrom:
		return prefs.DirectMessagesFrom, nil
	case PrefKeyExpandSpoilers:
		return prefs.ExpandSpoilers, nil
	case PrefKeyExpandMedia:
		return prefs.ExpandMedia, nil
	case PrefKeyAutoplayGifs:
		return prefs.AutoplayGifs, nil
	case PrefKeyShowFollowCounts:
		return prefs.ShowFollowCounts, nil
	case PrefKeyPreferredTimelineOrder:
		return prefs.PreferredTimelineOrder, nil
	case PrefKeySearchSuggestionsEnabled:
		return prefs.SearchSuggestionsEnabled, nil
	case PrefKeyPersonalizedSearchEnabled:
		return prefs.PersonalizedSearchEnabled, nil
	case PrefKeyReblogFilters:
		return prefs.ReblogFilters, nil
	default:
		return nil, ErrorHandler.HandleGetError(common.ValidationError{Field: "preference key", Message: fmt.Sprintf("unknown key: %s", key)}, "user preferences", key)
	}
}

// GetAllPreferences gets all preferences as a map
func (r *UserRepository) GetAllPreferences(ctx context.Context, username string) (map[string]any, error) {
	prefs, err := r.GetUserPreferences(ctx, username)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"language":                    prefs.Language,
		"default_posting_visibility":  prefs.DefaultPostingVisibility,
		"default_media_sensitive":     prefs.DefaultMediaSensitive,
		"direct_messages_from":        prefs.DirectMessagesFrom,
		"expand_spoilers":             prefs.ExpandSpoilers,
		"expand_media":                prefs.ExpandMedia,
		"autoplay_gifs":               prefs.AutoplayGifs,
		"show_follow_counts":          prefs.ShowFollowCounts,
		"preferred_timeline_order":    prefs.PreferredTimelineOrder,
		"search_suggestions_enabled":  prefs.SearchSuggestionsEnabled,
		"personalized_search_enabled": prefs.PersonalizedSearchEnabled,
		"reblog_filters":              prefs.ReblogFilters,
		"streaming_default_quality":   prefs.StreamingDefaultQuality,
		"streaming_auto_quality":      prefs.StreamingAutoQuality,
		"streaming_preload_next":      prefs.StreamingPreloadNext,
		"streaming_data_saver":        prefs.StreamingDataSaver,
	}, nil
}

// UpdatePreferences updates multiple preferences at once
func (r *UserRepository) UpdatePreferences(ctx context.Context, username string, preferences map[string]any) error {
	// Get existing preferences
	prefs, err := r.GetUserPreferences(ctx, username)
	if err != nil {
		return err
	}

	// Update each preference that's provided
	for key, value := range preferences {
		if err := r.updateSinglePreference(prefs, key, value, username); err != nil {
			return err
		}
	}

	// Save the updated preferences
	return r.UpdateUserPreferences(ctx, username, prefs)
}

// updateSinglePreference updates a single preference field
func (r *UserRepository) updateSinglePreference(prefs *storage.UserPreferences, key string, value any, username string) error {
	switch key {
	case PrefKeyLanguage:
		return r.setStringPreference(&prefs.Language, value, key)
	case PrefKeyDefaultPostingVisibility:
		return r.setStringPreference(&prefs.DefaultPostingVisibility, value, key)
	case PrefKeyDefaultMediaSensitive:
		return r.setBoolPreference(&prefs.DefaultMediaSensitive, value, key)
	case PrefKeyDirectMessagesFrom:
		return r.setStringPreference(&prefs.DirectMessagesFrom, value, key)
	case PrefKeyExpandSpoilers:
		return r.setBoolPreference(&prefs.ExpandSpoilers, value, key)
	case PrefKeyExpandMedia:
		return r.setStringPreference(&prefs.ExpandMedia, value, key)
	case PrefKeyAutoplayGifs:
		return r.setBoolPreference(&prefs.AutoplayGifs, value, key)
	case PrefKeyShowFollowCounts:
		return r.setBoolPreference(&prefs.ShowFollowCounts, value, key)
	case PrefKeyPreferredTimelineOrder:
		return r.setStringPreference(&prefs.PreferredTimelineOrder, value, key)
	case PrefKeySearchSuggestionsEnabled:
		return r.setBoolPreference(&prefs.SearchSuggestionsEnabled, value, key)
	case PrefKeyPersonalizedSearchEnabled:
		return r.setBoolPreference(&prefs.PersonalizedSearchEnabled, value, key)
	case PrefKeyReblogFilters:
		return r.setReblogFiltersPreference(&prefs.ReblogFilters, value, key)
	case PrefKeyStreamingDefaultQuality:
		return r.setStringPreference(&prefs.StreamingDefaultQuality, value, key)
	case PrefKeyStreamingAutoQuality:
		return r.setBoolPreference(&prefs.StreamingAutoQuality, value, key)
	case PrefKeyStreamingPreloadNext:
		return r.setBoolPreference(&prefs.StreamingPreloadNext, value, key)
	case PrefKeyStreamingDataSaver:
		return r.setBoolPreference(&prefs.StreamingDataSaver, value, key)
	default:
		// Store unknown preferences in the generic Preferences map
		if prefs.Preferences == nil {
			prefs.Preferences = make(map[string]string)
		}
		prefs.Preferences[key] = fmt.Sprintf("%v", value)
		r.logger.Debug("stored custom preference",
			zap.String("key", key),
			zap.String("username", username))
		return nil
	}
}

// setStringPreference sets a string preference with type checking using centralized validation
func (r *UserRepository) setStringPreference(field *string, value any, key string) error {
	// Use centralized preference validation
	if err := common.ValidatePreferenceValue(key, value); err != nil {
		return err
	}

	v, ok := value.(string)
	if !ok {
		return ErrorHandler.HandleUpdateError(common.ValidationError{Field: "preference type", Message: fmt.Sprintf("expected string for %s", key)}, "user preferences", key)
	}
	*field = v
	return nil
}

// setBoolPreference sets a boolean preference with type checking using centralized validation
func (r *UserRepository) setBoolPreference(field *bool, value any, key string) error {
	// Use centralized preference validation
	if err := common.ValidatePreferenceValue(key, value); err != nil {
		return err
	}

	v, ok := value.(bool)
	if !ok {
		return ErrorHandler.HandleUpdateError(common.ValidationError{Field: "preference type", Message: fmt.Sprintf("expected bool for %s", key)}, "user preferences", key)
	}
	*field = v
	return nil
}

// setReblogFiltersPreference sets the reblog filters preference with type checking using centralized validation
func (r *UserRepository) setReblogFiltersPreference(field *map[string]bool, value any, key string) error {
	// Use centralized preference validation
	if err := common.ValidatePreferenceValue(key, value); err != nil {
		return err
	}

	v, ok := value.(map[string]bool)
	if !ok {
		return ErrorHandler.HandleUpdateError(common.ValidationError{Field: "preference type", Message: fmt.Sprintf("expected map[string]bool for %s", key)}, "user preferences", key)
	}
	*field = v
	return nil
}

// AcceptFollow accepts a follow request and updates both the relationship state and follower counts
func (r *UserRepository) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	r.logger.Info("accepting follow request",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	// 1. Update the relationship state to "accepted"
	var relationship models.RelationshipRecord
	err := r.GetDB().WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", followerUsername)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", followedUsername)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(common.ValidationError{Field: "follow relationship", Message: "not found"}, "follow relationship", followerUsername)
		}
		r.logger.Error("failed to get follow relationship", zap.Error(err))
		return ErrorHandler.HandleGetError(err, "follow relationship", followerUsername)
	}

	// Update the relationship state
	relationship.Accept()

	// Save the updated relationship
	err = r.GetDB().WithContext(ctx).Model(&relationship).Update()
	if err != nil {
		r.logger.Error("failed to update relationship state", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "follow relationship", "state")
	}

	// 2. Update follower count for the followed user (increment)
	if err := r.updateFollowerCount(ctx, followedUsername, 1); err != nil {
		r.logger.Error("failed to update follower count",
			zap.String("followed_user", followedUsername),
			zap.Error(err))
		// Note: We don't return error here to avoid partial state, but we log it
	}

	// 3. Update following count for the follower user (increment)
	if err := r.updateFollowingCount(ctx, followerUsername, 1); err != nil {
		r.logger.Error("failed to update following count",
			zap.String("follower_user", followerUsername),
			zap.Error(err))
		// Note: We don't return error here to avoid partial state, but we log it
	}

	r.logger.Info("successfully accepted follow request",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	return nil
}

// RejectFollow rejects a follow request by updating the relationship state to "rejected"
func (r *UserRepository) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	r.logger.Info("rejecting follow request",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	// Update the relationship state to "rejected"
	var relationship models.RelationshipRecord
	err := r.GetDB().WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", followerUsername)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", followedUsername)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(common.ValidationError{Field: "follow relationship", Message: "not found"}, "follow relationship", followerUsername)
		}
		r.logger.Error("failed to get follow relationship", zap.Error(err))
		return ErrorHandler.HandleGetError(err, "follow relationship", followerUsername)
	}

	// Update the relationship state
	relationship.Reject()

	// Save the updated relationship
	err = r.GetDB().WithContext(ctx).Model(&relationship).Update()
	if err != nil {
		r.logger.Error("failed to update relationship state", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "follow relationship", "state")
	}

	r.logger.Info("successfully rejected follow request",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	return nil
}

// countUpdateType represents the type of count being updated
type countUpdateType string

const (
	countUpdateFollowers countUpdateType = "followers"
	countUpdateFollowing countUpdateType = "following"
)

// updateActorCount updates either follower or following count for a user's actor using atomic operations
func (r *UserRepository) updateActorCount(ctx context.Context, username string, delta int, countType countUpdateType) error {
	pk := fmt.Sprintf("ACTOR#%s", username)
	sk := "PROFILE"

	// Determine which field to update
	var fieldName string
	switch countType {
	case countUpdateFollowers:
		fieldName = "FollowerCount"
	case countUpdateFollowing:
		fieldName = "FollowingCount"
	default:
		return ErrorHandler.HandleUpdateError(fmt.Errorf("unknown count type: %s", countType), EntityActor, "count")
	}

	// Use atomic UpdateBuilder().Add() to prevent race conditions
	err := r.GetDB().WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder().
		Add(fieldName, delta).
		Condition(fieldName, ">=", -delta). // Prevent negative counts: only allow if count + delta >= 0
		Execute()

	if err != nil {
		// Check if actor doesn't exist
		if errors.IsNotFound(err) {
			r.logger.Warn("actor not found for count update",
				zap.String("username", username),
				zap.String("count_type", string(countType)))
			return nil // Don't error if actor doesn't exist
		}
		return ErrorHandler.HandleUpdateError(err, EntityActor, string(countType))
	}

	r.logger.Debug("updated count atomically",
		zap.String("username", username),
		zap.String("count_type", string(countType)),
		zap.Int("delta", delta),
		zap.String("field", fieldName))

	return nil
}

// updateFollowerCount updates the follower count for a user's actor
func (r *UserRepository) updateFollowerCount(ctx context.Context, username string, delta int) error {
	return r.updateActorCount(ctx, username, delta, countUpdateFollowers)
}

// updateFollowingCount updates the following count for a user's actor
func (r *UserRepository) updateFollowingCount(ctx context.Context, username string, delta int) error {
	return r.updateActorCount(ctx, username, delta, countUpdateFollowing)
}

// CreateConversationMute creates a new conversation mute
func (r *UserRepository) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	r.logger.Info("creating conversation mute",
		zap.String("username", mute.Username),
		zap.String("conversation_id", mute.ConversationID))

	// Set timestamp if not provided
	if mute.CreatedAt.IsZero() {
		mute.CreatedAt = time.Now()
	}

	// Create the model
	muteModel := &models.ConversationMute{
		Username:       mute.Username,
		ConversationID: mute.ConversationID,
		CreatedAt:      mute.CreatedAt,
		ExpiresAt:      mute.ExpiresAt,
	}
	_ = muteModel.UpdateKeys() // Ignore error as this is internal model operation

	// Create the mute
	err := r.GetDB().WithContext(ctx).Model(muteModel).Create()

	if err != nil {
		// Check if it's a duplicate (condition check failed)
		if errors.IsConditionFailed(err) {
			return ErrorHandler.HandleCreateError(common.ConflictError{Resource: "conversation mute", Message: "conversation already muted"}, "conversation mute", mute.ConversationID)
		}
		return ErrorHandler.HandleCreateError(err, "conversation mute", mute.ConversationID)
	}

	return nil
}

// DeleteConversationMute removes a conversation mute
func (r *UserRepository) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	r.logger.Info("deleting conversation mute",
		zap.String("username", username),
		zap.String("conversation_id", conversationID))

	// Create model with keys for deletion
	muteModel := &models.ConversationMute{
		Username:       username,
		ConversationID: conversationID,
	}
	_ = muteModel.UpdateKeys() // Ignore error as this is internal model operation

	err := r.GetDB().WithContext(ctx).Model(&models.ConversationMute{}).
		Where("PK", "=", muteModel.PK).
		Where("SK", "=", muteModel.SK).
		Delete()

	if err != nil {
		return ErrorHandler.HandleDeleteError(err, "conversation mute", conversationID)
	}

	return nil
}

// IsConversationMuted checks if a conversation is muted by a user
func (r *UserRepository) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	// Create model with keys for lookup
	muteModel := &models.ConversationMute{
		Username:       username,
		ConversationID: conversationID,
	}
	_ = muteModel.UpdateKeys() // Ignore error as this is internal model operation

	var result models.ConversationMute
	err := r.GetDB().WithContext(ctx).Model(&models.ConversationMute{}).
		Where("PK", "=", muteModel.PK).
		Where("SK", "=", muteModel.SK).
		First(&result)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, ErrorHandler.HandleQueryError(err, "conversation mute", "check")
	}

	// Check if the mute has expired
	if !result.ExpiresAt.IsZero() && result.ExpiresAt.Before(time.Now()) {
		return false, nil
	}

	return true, nil
}

// GetMutedConversations retrieves all muted conversations for a user
func (r *UserRepository) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	r.logger.Info("getting muted conversations",
		zap.String("username", username))

	// Query all muted conversations for the user
	pk := fmt.Sprintf("USER#%s#CONV_MUTES", username)

	var mutes []models.ConversationMute
	err := r.GetDB().WithContext(ctx).Model(&models.ConversationMute{}).
		Where("PK", "=", pk).
		All(&mutes)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "conversation mute", "query")
	}

	// Filter out expired mutes and extract conversation IDs
	conversationIDs := make([]string, 0, len(mutes))
	now := time.Now()
	for _, mute := range mutes {
		// Skip expired mutes
		if !mute.ExpiresAt.IsZero() && mute.ExpiresAt.Before(now) {
			continue
		}
		conversationIDs = append(conversationIDs, mute.ConversationID)
	}

	return conversationIDs, nil
}

// IsNotificationMuted checks if notifications from a target user are muted
func (r *UserRepository) IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error) {
	// Check user preferences for notification muting
	// This is a simple implementation that checks if the target is in a muted notifications list
	// In a more sophisticated implementation, this could check various notification preference settings

	prefs, err := r.GetUserPreferences(ctx, userID)
	if err != nil {
		// If preferences don't exist, notifications aren't muted
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to get user preferences for notification mute check",
			zap.String("userID", userID),
			zap.String("targetID", targetID),
			zap.Error(err))
		return false, ErrorHandler.HandleQueryError(err, "notification mute", "check")
	}

	// Check if target is in muted notifications list
	r.logger.Debug("checking notification mute status",
		zap.String("userID", userID),
		zap.String("targetID", targetID))

	if prefs == nil {
		// No preferences set, default to not muted
		return false, nil
	}

	// Check for dedicated notification preferences first
	var notifPrefs models.NotificationPreferences
	err = r.GetDB().WithContext(ctx).Model(&models.NotificationPreferences{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", "NOTIFICATION_PREFS").
		First(&notifPrefs)

	if err == nil {
		// Use notification preferences to determine mute status
		// Check if all notification types are disabled (effectively muted)
		if !notifPrefs.MentionEnabled && !notifPrefs.ReblogEnabled &&
			!notifPrefs.FavoriteEnabled && !notifPrefs.FollowEnabled {
			r.logger.Debug("all notifications disabled for user",
				zap.String("userID", userID))
			return true, nil
		}
	} else if !errors.IsNotFound(err) {
		// Log non-not-found errors but don't fail
		r.logger.Debug("failed to get notification preferences, checking reblog filters",
			zap.String("userID", userID),
			zap.Error(err))
	}

	// Fallback to reblog filters for backwards compatibility
	if prefs.ReblogFilters != nil {
		if showReblogs, exists := prefs.ReblogFilters[targetID]; exists && !showReblogs {
			// If reblogs are muted for this user, consider notifications muted too
			r.logger.Debug("user has reblogs muted, considering notifications muted",
				zap.String("userID", userID),
				zap.String("targetID", targetID))
			return true, nil
		}
	}

	return false, nil
}

// CacheRemoteActor caches a remote actor with a TTL using DynamORM patterns
func (r *UserRepository) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	r.logger.Debug("caching remote actor",
		zap.String("handle", handle),
		zap.String("actor_id", actor.ID),
		zap.Duration("ttl", ttl))

	now := time.Now()
	expiresAt := now.Add(ttl)

	// Create the DynamORM model following the exact legacy pattern
	remoteActor := &models.RemoteActor{
		Handle:    handle,
		Actor:     actor,
		CachedAt:  now,
		UpdatedAt: now,
		ExpiresAt: expiresAt,
	}

	// UpdateKeys sets the PK, SK, Domain, and TTL fields based on the legacy pattern
	remoteActor.UpdateKeys() // Internal model operation

	// Create in DynamoDB using DynamORM
	err := r.GetDB().WithContext(ctx).Model(remoteActor).Create()
	if err != nil {
		r.logger.Error("failed to cache remote actor",
			zap.String("handle", handle),
			zap.String("actor_id", actor.ID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "remote actor cache", actor.ID)
	}

	r.logger.Debug("remote actor cached successfully",
		zap.String("handle", handle),
		zap.String("actor_id", actor.ID),
		zap.Duration("ttl", ttl),
		zap.Time("expires_at", expiresAt))

	return nil
}

// Bookmark methods

// CreateBookmark creates a new bookmark for a user
func (r *UserRepository) CreateBookmark(ctx context.Context, username, objectID string) error {
	repo := r.getBookmarkRepository()
	if repo == nil {
		return ErrorHandler.HandleCreateError(fmt.Errorf("bookmark repository not configured"), EntityBookmark, objectID)
	}
	_, err := repo.CreateBookmark(ctx, username, objectID)
	if err != nil {
		r.logger.Error("failed to create bookmark",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityBookmark, objectID)
	}
	return nil
}

// RemoveBookmark removes a bookmark for a user
func (r *UserRepository) RemoveBookmark(ctx context.Context, username, objectID string) error {
	repo := r.getBookmarkRepository()
	if repo == nil {
		return ErrorHandler.HandleDeleteError(fmt.Errorf("bookmark repository not configured"), EntityBookmark, objectID)
	}
	if err := repo.DeleteBookmark(ctx, username, objectID); err != nil {
		r.logger.Error("failed to delete bookmark",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityBookmark, objectID)
	}
	return nil
}

// GetBookmarks retrieves bookmarks for a user with pagination
func (r *UserRepository) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	repo := r.getBookmarkRepository()
	if repo == nil {
		return nil, "", ErrorHandler.HandleQueryError(fmt.Errorf("bookmark repository not configured"), EntityBookmark, "query")
	}

	if err := common.ValidateQueryLimit(int(limit), 100, "user listing"); err != nil {
		limit = 20
	}

	bookmarks, nextCursor, err := repo.GetUserBookmarks(ctx, username, limit, cursor)
	if err != nil {
		r.logger.Error("failed to query bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityBookmark, "query")
	}

	objectIDs := make([]string, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		objectIDs = append(objectIDs, bookmark.ObjectID)
	}

	return objectIDs, nextCursor, nil
}

// IsBookmarked checks if a user has bookmarked an object
func (r *UserRepository) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	repo := r.getBookmarkRepository()
	if repo == nil {
		return false, ErrorHandler.HandleQueryError(fmt.Errorf("bookmark repository not configured"), EntityBookmark, "query")
	}
	return repo.IsBookmarked(ctx, username, objectID)
}

// DeleteFromTimeline removes a specific timeline entry
func (r *UserRepository) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	// We need to find the entry first to get its timestamp
	pk := fmt.Sprintf("timeline#%s#%s", timelineType, timelineID)

	// Query for the specific entry
	var entry models.Timeline
	err := r.GetDB().WithContext(ctx).Model(&models.Timeline{}).
		Where("PK", "=", pk).
		Filter("EntryID", "=", entryID).
		First(&entry)

	if err != nil {
		if errors.IsNotFound(err) {
			// Entry not found, nothing to delete
			return nil
		}
		return ErrorHandler.HandleGetError(err, EntityTimelineEntry, entryID)
	}

	// Now delete the entry using its PK and SK
	err = r.GetDB().WithContext(ctx).Model(&entry).Delete()
	if err != nil {
		r.logger.Error("failed to delete timeline entry",
			zap.String("timeline_type", timelineType),
			zap.String("timeline_id", timelineID),
			zap.String("entry_id", entryID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityTimelineEntry, entryID)
	}

	r.logger.Debug("deleted timeline entry",
		zap.String("timeline_type", timelineType),
		zap.String("timeline_id", timelineID),
		zap.String("entry_id", entryID))

	return nil
}

// DeleteExpiredTimelineEntries deletes timeline entries that have expired
func (r *UserRepository) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	r.logger.Debug("deleting expired timeline entries",
		zap.Time("before", before))

	// Use TTL-based approach for efficiency. This method handles cleanup
	// of entries that didn't get auto-deleted due to DynamoDB TTL delays

	// Note: DynamoDB TTL typically deletes within 48 hours, but we may need
	// manual cleanup for immediate consistency requirements

	var expiredEntries []*models.Timeline

	// Scan for expired entries (this is expensive - consider using TTL instead)
	err := r.GetDB().WithContext(ctx).Model(&models.Timeline{}).
		Filter("ExpiresAt", "<", before).
		All(&expiredEntries)
	if err != nil {
		r.logger.Error("failed to scan for expired timeline entries",
			zap.Time("before", before),
			zap.Error(err))
		return ErrorHandler.HandleQueryError(err, EntityTimelineEntry, "scan expired")
	}

	if err := common.ValidateSliceNotEmpty("expired_entries", expiredEntries); err != nil {
		r.logger.Debug("no expired timeline entries found",
			zap.Time("before", before))
		return nil // Nothing to delete
	}

	// Use batch deletion for efficient processing
	// Convert timeline entries to keys for batch deletion
	keys := make([]any, len(expiredEntries))
	for i, entry := range expiredEntries {
		// DynamORM expects the actual model as the key for batch delete
		keys[i] = entry
	}

	// Perform batch deletion using DynamORM batch operations
	// Use centralized cost tracker if available, fallback to legacy tracker
	var costTracker batch.CostTracker
	if r.GetCostService() != nil {
		costTracker = &centralizedCostTracker{
			costService: r.GetCostService(),
			tableName:   r.tableName,
			logger:      r.logger,
		}
	} else {
		costTracker = &timelineCostTracker{logger: r.logger}
	}

	result, err := batch.BatchDeleteWithCostTracking(ctx, r.GetDB(), keys, costTracker, r.logger)
	if err != nil {
		r.logger.Error("failed to batch delete expired timeline entries",
			zap.Time("before", before),
			zap.Int("total_entries", len(expiredEntries)),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityTimelineEntry, "batch")
	}

	deletedCount := result.ProcessedItems
	if result.FailedItems > 0 {
		r.logger.Warn("some timeline entries failed to delete in batch operation",
			zap.Int("total_entries", len(expiredEntries)),
			zap.Int("deleted_count", deletedCount),
			zap.Int("failed_count", result.FailedItems))
	}

	r.logger.Info("deleted expired timeline entries",
		zap.Time("before", before),
		zap.Int("deleted_count", deletedCount))

	return nil
}

// getTimelineEntries is a generic function to retrieve timeline entries with pagination
func (r *UserRepository) getTimelineEntries(ctx context.Context, pk, errorContext string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// Build query
	query := r.GetDB().WithContext(ctx).Model(&models.Timeline{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC") // Most recent first

	// Resume from the supplied cursor value when available
	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityTimelineEntry, errorContext)
	}

	// Generate next cursor
	var nextCursor string
	if len(entries) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = entries[limit-1].SK
		entries = entries[:limit] // Trim to requested limit
	}

	// Convert to storage.TimelineEntry
	result := make([]*storage.TimelineEntry, len(entries))
	for i, e := range entries {
		result[i] = &storage.TimelineEntry{
			TimelineType: e.TimelineType,
			TimelineID:   e.TimelineID,
			EntryID:      e.EntryID,
			PostID:       e.PostID,
			ActorID:      e.ActorID,
			ActorHandle:  e.ActorHandle,
			Content:      e.Content,
			ContentType:  e.ContentType,
			HasMedia:     e.HasMedia,
			IsReply:      e.IsReply,
			IsBoost:      e.IsBoost,
			Language:     e.Language,
			Visibility:   e.Visibility,
			TimelineAt:   e.TimelineAt,
			ExpiresAt: func() *time.Time {
				if e.TTL == 0 {
					return nil
				}
				t := time.Unix(e.TTL, 0)
				return &t
			}(),
			CreatedAt: e.CreatedAt,
		}
	}

	return result, nextCursor, nil
}

// GetDirectTimeline retrieves direct message timeline entries for a user
func (r *UserRepository) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// Direct messages are stored in a special timeline type
	pk := fmt.Sprintf("timeline#DIRECT#%s", username)
	return r.getTimelineEntries(ctx, pk, "direct", limit, cursor)
}

// GetFollowRequestState returns the state of a follow request between two users
func (r *UserRepository) GetFollowRequestState(ctx context.Context, followerID, targetID string) (string, error) {
	// Check if there's a pending follow request from follower to target
	if r.deps != nil {
		requests, _, err := r.deps.GetPendingFollowRequests(ctx, targetID, 100, "")
		if err != nil {
			r.logger.Debug("failed to get pending follow requests",
				zap.String("targetID", targetID),
				zap.Error(err))
			return "none", nil //nolint:goconst // This "none" is for relationship status
		}
		for _, req := range requests {
			if req == followerID {
				return "pending", nil
			}
		}
		return "none", nil //nolint:goconst // This "none" is for relationship status, not replies policy
	}

	// Check if there's a follow relationship
	var relationship models.RelationshipRecord
	err := r.GetDB().WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", followerID)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", targetID)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return "none", nil
		}
		return "", ErrorHandler.HandleGetError(err, "follow request", "state")
	}

	// Return the relationship state
	return relationship.State, nil
}

// GetHashtagTimeline retrieves timeline entries for a specific hashtag
func (r *UserRepository) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// Hashtag timelines are stored with special timeline type
	timelineID := hashtag
	if local {
		timelineID = hashtag + "#LOCAL"
	}

	pk := fmt.Sprintf("timeline#HASHTAG#%s", timelineID)
	return r.getTimelineEntries(ctx, pk, "hashtag", limit, cursor)
}

// GetListTimeline retrieves timeline entries for a specific list
func (r *UserRepository) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// List timelines use LIST timeline type
	pk := fmt.Sprintf("timeline#LIST#%s", listID)
	return r.getTimelineEntries(ctx, pk, "list", limit, cursor)
}

// GetPendingFollowRequests retrieves pending follow requests for a user
func (r *UserRepository) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	log := r.logger.With(
		zap.String("method", "GetPendingFollowRequests"),
		zap.String("username", username),
		zap.Int("limit", limit),
	)

	if r.deps == nil {
		log.Error("dependencies not set")
		return nil, "", ErrorHandler.HandleGetError(common.ValidationError{Field: "dependencies", Message: "not available"}, "dependencies", "get")
	}

	// Delegate to RelationshipRepository through deps
	return r.deps.GetPendingFollowRequests(ctx, username, limit, cursor)
}

// ListUsersByRole lists users by their role
func (r *UserRepository) ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error) {
	log := r.logger.With(
		zap.String("method", "ListUsersByRole"),
		zap.String("role", role),
	)

	// Query for users by role using GSI
	var userModels []models.User
	err := r.GetDB().WithContext(ctx).Model(&models.User{}).
		Index("gsi3").
		Where("gsi3PK", "=", "ROLE#"+role).
		All(&userModels)
	if err != nil {
		// If the GSI doesn't exist or no users found, return empty list
		if errors.IsNotFound(err) {
			return []*storage.User{}, nil
		}
		log.Warn("failed to query users by role", zap.Error(err))
		return []*storage.User{}, nil
	}

	// Convert to storage.User slice
	users := make([]*storage.User, 0, len(userModels))
	for _, userModel := range userModels {
		users = append(users, r.modelToStorage(&userModel))
	}

	log.Info("retrieved users by role",
		zap.String("role", role),
		zap.Int("count", len(users)))

	return users, nil
}

// RemoveFromFollowers removes a follower from the current user's followers list
func (r *UserRepository) RemoveFromFollowers(ctx context.Context, username, followerUsername string) error {
	log := r.logger.With(
		zap.String("method", "RemoveFromFollowers"),
		zap.String("username", username),
		zap.String("follower_username", followerUsername),
	)

	if r.deps == nil {
		log.Error("dependencies not set")
		return ErrorHandler.HandleCreateError(common.ValidationError{Field: "dependencies", Message: "not available"}, "dependencies", "create")
	}

	// This is an alias for RemoveFollow with parameters in the order expected by the interface
	// RemoveFollow expects (follower, followed), so we swap the parameters
	return r.deps.RemoveFollow(ctx, followerUsername, username)
}

// FanOutPost distributes a post to all relevant timelines (followers' home timelines, public timeline, etc.)
func (r *UserRepository) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	log := r.logger.With(
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
		zap.String("actor", activity.Actor),
	)

	// Only fan out Create activities
	if activity.Type != activitypub.CreateType {
		return nil
	}

	// Extract and validate object from activity
	object, tags, err := r.extractActivityObject(activity, log)
	if err != nil {
		if stdErrors.Is(err, storage.ErrInvalidInput) {
			return nil
		}
		return err
	}

	// Extract and validate metadata
	metadata, err := r.extractObjectMetadata(object, log)
	if err != nil {
		return err
	}

	// Create base timeline entry
	baseEntry := r.createBaseTimelineEntry(metadata, object)

	// Build all timeline entries
	entries := r.buildTimelineEntries(ctx, metadata, baseEntry, tags, log)

	// Write all entries to timelines
	if len(entries) > 0 {
		if err := r.deps.CreateTimelineEntries(ctx, entries); err != nil {
			log.Error("failed to write to timelines", zap.Error(err), zap.Int("entry_count", len(entries)))
			return ErrorHandler.HandleCreateError(err, EntityTimelineEntry, "write")
		}
	}

	log.Info("successfully fanned out post", zap.Int("timeline_count", len(entries)))
	return nil
}

// extractActivityObject extracts and normalizes the object from an activity
func (r *UserRepository) extractActivityObject(activity *activitypub.Activity, log *zap.Logger) (map[string]interface{}, []activitypub.Tag, error) {
	var object map[string]interface{}
	var tags []activitypub.Tag

	switch obj := activity.Object.(type) {
	case map[string]interface{}:
		object = obj
		tags = r.extractTagsFromMap(obj)
	case *activitypub.Note:
		object = r.noteToMap(obj)
		tags = obj.Tag
	default:
		log.Warn("unsupported object type for fan-out", zap.Any("object", activity.Object))
		return nil, nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "activity object", fmt.Sprintf("type %T", activity.Object))
	}

	return object, tags, nil
}

// extractTagsFromMap extracts tags from an object map
func (r *UserRepository) extractTagsFromMap(obj map[string]interface{}) []activitypub.Tag {
	var tags []activitypub.Tag
	tagList, ok := obj["tag"].([]interface{})
	if !ok {
		return tags
	}

	for _, t := range tagList {
		tagMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		tag := activitypub.Tag{
			Type: getStringFromMap(tagMap, "type"),
			Name: getStringFromMap(tagMap, "name"),
			Href: getStringFromMap(tagMap, "href"),
		}
		tags = append(tags, tag)
	}
	return tags
}

// noteToMap converts an ActivityPub Note to a map
func (r *UserRepository) noteToMap(note *activitypub.Note) map[string]interface{} {
	return map[string]interface{}{
		"id":           note.ID,
		"type":         note.Type,
		"content":      note.Content,
		"attributedTo": note.AttributedTo,
		"to":           note.To,
		"cc":           note.CC,
		"inReplyTo":    note.InReplyTo,
		"sensitive":    note.Sensitive,
		"summary":      note.Summary,
	}
}

// objectMetadata holds extracted metadata from an object
type objectMetadata struct {
	objectID     string
	objectType   string
	content      string
	attributedTo string
	inReplyTo    string
	sensitive    bool
	summary      string
	username     string
	visibility   string
}

// extractObjectMetadata extracts and validates metadata from an object
func (r *UserRepository) extractObjectMetadata(object map[string]interface{}, log *zap.Logger) (*objectMetadata, error) {
	metadata := &objectMetadata{
		objectID:     r.getStringField(object, "id"),
		objectType:   r.getStringField(object, "type"),
		content:      r.getStringField(object, "content"),
		attributedTo: r.getStringField(object, "attributedTo"),
		inReplyTo:    r.getStringField(object, "inReplyTo"),
		sensitive:    r.getBoolField(object, "sensitive"),
		summary:      r.getStringField(object, "summary"),
	}

	// Validate required fields
	if common.ValidateRequiredParam("objectID", metadata.objectID) != nil || common.ValidateRequiredParam("attributedTo", metadata.attributedTo) != nil {
		log.Error("missing required fields in object", zap.Any("object", object))
		return nil, ErrorHandler.HandleGetError(common.ValidationError{Field: "object", Message: "missing required fields"}, EntityObject, "validation")
	}

	// Extract username from actor ID
	metadata.username = extractUsernameFromActorID(metadata.attributedTo)
	if common.ValidateRequiredParam("username", metadata.username) != nil {
		log.Error("failed to extract username from actor", zap.String("actor", metadata.attributedTo))
		return nil, ErrorHandler.HandleGetError(common.ValidationError{Field: "actor ID", Message: "invalid format"}, EntityActor, "validation")
	}

	// Determine visibility
	metadata.visibility = r.determineVisibility(object)

	return metadata, nil
}

// getStringField safely extracts a string field from a map
func (r *UserRepository) getStringField(obj map[string]interface{}, field string) string {
	val, _ := obj[field].(string)
	return val
}

// getBoolField safely extracts a bool field from a map
func (r *UserRepository) getBoolField(obj map[string]interface{}, field string) bool {
	val, _ := obj[field].(bool)
	return val
}

// createBaseTimelineEntry creates the base timeline entry from metadata
func (r *UserRepository) createBaseTimelineEntry(metadata *objectMetadata, object map[string]interface{}) *models.Timeline {
	return &models.Timeline{
		PostID:      metadata.objectID,
		ActorID:     metadata.attributedTo,
		ActorHandle: metadata.username,
		Content:     truncateContent(metadata.content, 500),
		ContentType: metadata.objectType,
		HasMedia:    hasMediaAttachments(object),
		IsReply:     metadata.inReplyTo != "",
		InReplyTo:   metadata.inReplyTo,
		IsBoost:     false,
		Visibility:  metadata.visibility,
		Language:    extractLanguage(object),
		Sensitive:   metadata.sensitive,
		SpoilerText: metadata.summary,
		CreatedAt:   extractPublishedTime(object),
		TimelineAt:  time.Now(),
	}
}

// buildTimelineEntries builds all timeline entries for the post
func (r *UserRepository) buildTimelineEntries(ctx context.Context, metadata *objectMetadata, baseEntry *models.Timeline, tags []activitypub.Tag, log *zap.Logger) []*models.Timeline {
	var entries []*models.Timeline

	// Add follower timeline entries for non-direct messages
	if metadata.visibility != "direct" { //nolint:goconst // "direct" is a visibility level from models/status.go, not a preference key
		entries = r.addFollowerEntries(ctx, metadata.username, baseEntry, entries, log)
	}

	// Add public timeline entries
	if r.shouldAddToPublicTimelines(metadata.visibility) {
		entries = r.addPublicTimelineEntries(metadata.attributedTo, baseEntry, entries)
	}

	// Add hashtag timeline entries
	if metadata.visibility == "public" && len(tags) > 0 {
		entries = r.addHashtagTimelineEntries(baseEntry, tags, entries)
	}

	// Add list timeline entries for non-direct messages
	if metadata.visibility != "direct" {
		entries = r.addListEntries(ctx, metadata.username, baseEntry, entries, log)
	}

	return entries
}

// shouldAddToPublicTimelines checks if the post should be added to public timelines
func (r *UserRepository) shouldAddToPublicTimelines(visibility string) bool {
	return visibility == models.VisibilityPublic || visibility == models.VisibilityUnlisted
}

// addFollowerEntries adds entries for followers' home timelines
func (r *UserRepository) addFollowerEntries(ctx context.Context, username string, baseEntry *models.Timeline, entries []*models.Timeline, log *zap.Logger) []*models.Timeline {
	followerEntries, err := r.createFollowerTimelineEntries(ctx, username, baseEntry)
	if err != nil {
		log.Error("failed to create follower timeline entries", zap.Error(err))
		return entries
	}
	return append(entries, followerEntries...)
}

// addPublicTimelineEntries adds entries for public timelines
func (r *UserRepository) addPublicTimelineEntries(attributedTo string, baseEntry *models.Timeline, entries []*models.Timeline) []*models.Timeline {
	// Add to federated public timeline
	publicEntry := *baseEntry
	publicEntry.TimelineType = "PUBLIC"
	publicEntry.TimelineID = "FEDERATED"
	publicEntry.EntryID = r.timelineSK(publicEntry.TimelineAt, publicEntry.PostID)
	entries = append(entries, &publicEntry)

	// Add to local public timeline if it's a local user
	if strings.HasPrefix(attributedTo, config.Get().BaseURL()) {
		localEntry := *baseEntry
		localEntry.TimelineType = "PUBLIC"
		localEntry.TimelineID = "LOCAL"
		localEntry.EntryID = r.timelineSK(localEntry.TimelineAt, localEntry.PostID)
		entries = append(entries, &localEntry)
	}

	return entries
}

// addHashtagTimelineEntries adds entries for hashtag timelines
func (r *UserRepository) addHashtagTimelineEntries(baseEntry *models.Timeline, tags []activitypub.Tag, entries []*models.Timeline) []*models.Timeline {
	for _, tag := range tags {
		if tag.Type != "Hashtag" || common.ValidateRequiredParam("tag.Name", tag.Name) != nil {
			continue
		}

		// Extract and normalize hashtag name
		hashtagName := strings.TrimPrefix(tag.Name, "#")
		hashtagName = strings.ToLower(hashtagName)

		hashtagEntry := *baseEntry
		hashtagEntry.TimelineType = "HASHTAG"
		hashtagEntry.TimelineID = hashtagName
		hashtagEntry.EntryID = r.timelineSK(hashtagEntry.TimelineAt, hashtagEntry.PostID)
		entries = append(entries, &hashtagEntry)
	}
	return entries
}

// addListEntries adds entries for list timelines
func (r *UserRepository) addListEntries(ctx context.Context, username string, baseEntry *models.Timeline, entries []*models.Timeline, log *zap.Logger) []*models.Timeline {
	listEntries, err := r.createListTimelineEntries(ctx, username, baseEntry)
	if err != nil {
		log.Error("failed to create list timeline entries", zap.Error(err))
		return entries
	}
	return append(entries, listEntries...)
}

// createFollowerTimelineEntries creates timeline entries for all followers
func (r *UserRepository) createFollowerTimelineEntries(ctx context.Context, username string, baseEntry *models.Timeline) ([]*models.Timeline, error) {
	log := r.logger.With(zap.String("username", username))

	var entries []*models.Timeline
	cursor := ""

	// Paginate through all followers
	for {
		followers, nextCursor, err := r.deps.GetFollowers(ctx, username, 100, cursor)
		if err != nil {
			log.Error("failed to get followers", zap.Error(err))
			return nil, ErrorHandler.HandleGetError(err, EntityFollow, "followers")
		}

		// Create timeline entry for each follower
		for _, followerID := range followers {
			// Extract follower username
			followerUsername := extractUsernameFromActorID(followerID)
			if common.ValidateRequiredParam("followerUsername", followerUsername) != nil {
				log.Warn("invalid follower ID", zap.String("follower_id", followerID))
				continue
			}

			// Create timeline entry for this follower
			entry := *baseEntry
			entry.TimelineType = "HOME"
			entry.TimelineID = followerUsername
			entry.EntryID = r.timelineSK(entry.TimelineAt, entry.PostID)
			entries = append(entries, &entry)
		}

		// Check if there are more followers
		if common.ValidateRequiredParam("nextCursor", nextCursor) != nil {
			break
		}
		cursor = nextCursor
	}

	log.Debug("created follower timeline entries", zap.Int("count", len(entries)))
	return entries, nil
}

// createListTimelineEntries creates timeline entries for lists containing the actor
func (r *UserRepository) createListTimelineEntries(ctx context.Context, username string, baseEntry *models.Timeline) ([]*models.Timeline, error) {
	log := r.logger.With(zap.String("username", username))

	// Get all lists that contain this account
	lists, err := r.deps.GetListsContainingAccount(ctx, username, "")
	if err != nil {
		log.Error("failed to get lists containing account", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityList, "account lists")
	}

	var entries []*models.Timeline

	for _, list := range lists {
		// Check list replies policy
		shouldInclude := false
		switch list.RepliesPolicy {
		case "none":
			// No replies
			shouldInclude = common.ValidateRequiredParam("inReplyTo", baseEntry.InReplyTo) != nil
		case "followed":
			// Replies to followed accounts only - check if replied-to account is followed
			if common.ValidateRequiredParam("inReplyTo", baseEntry.InReplyTo) != nil {
				// Not a reply, include it
				shouldInclude = true
			} else {
				// Check if the replied-to account is followed by list owner
				repliedToAccount := r.extractAccountFromReply(ctx, baseEntry.InReplyTo)
				if repliedToAccount != "" && r.deps != nil {
					// Use GetFollowers to check if the list owner follows the replied-to account
					followers, _, err := r.deps.GetFollowers(ctx, repliedToAccount, 1000, "")
					if err == nil {
						for _, follower := range followers {
							if follower == list.Username {
								shouldInclude = true
								break
							}
						}
					}
				}
			}
		case "list":
			// All posts including replies
			shouldInclude = true
		default:
			// Default to list policy
			shouldInclude = true
		}

		if shouldInclude {
			// Create timeline entry for this list
			entry := *baseEntry
			entry.TimelineType = "LIST"
			entry.TimelineID = list.ID
			entry.EntryID = r.timelineSK(entry.TimelineAt, entry.PostID)
			entries = append(entries, &entry)
		}
	}

	log.Debug("created list timeline entries", zap.Int("count", len(entries)))
	return entries, nil
}

// extractAccountFromReply extracts the account ID/username from an InReplyTo field
func (r *UserRepository) extractAccountFromReply(ctx context.Context, inReplyTo string) string {
	// InReplyTo is typically a post ID like "POST#user#timestamp" or URL
	// Extract the username/account part
	if strings.HasPrefix(inReplyTo, "POST#") {
		parts := strings.Split(inReplyTo, "#")
		if len(parts) >= 2 {
			return parts[1] // Return the username part
		}
	}

	// Use enhanced URL extraction for ActivityPub URLs and other patterns
	if r.urlValidator != nil {
		if username, err := r.urlValidator.EnhancedExtractAccountFromReply(ctx, inReplyTo); err == nil && username != "" {
			return username
		}
	}

	// Fallback to legacy path extraction
	return r.extractUsernameFromURLPath(inReplyTo)
}

// extractUsernameFromURLPath provides fallback URL path parsing
func (r *UserRepository) extractUsernameFromURLPath(inReplyTo string) string {
	// Basic URL parsing for ActivityPub URLs
	if !strings.HasPrefix(inReplyTo, "http") {
		return ""
	}

	// Try to extract username from common ActivityPub URL patterns
	parts := strings.Split(inReplyTo, "/")
	if len(parts) < 3 {
		return ""
	}

	// Look for patterns like /users/username, /@username, /actors/username
	for i, part := range parts {
		if (part == "users" || part == "actors" || part == "profile") && i+1 < len(parts) {
			return parts[i+1]
		}
		if strings.HasPrefix(part, "@") && len(part) > 1 {
			return strings.TrimPrefix(part, "@")
		}
	}

	// If no pattern matches, try the last path segment if it looks like a username
	lastPart := parts[len(parts)-1]
	if len(lastPart) > 0 && len(lastPart) <= 50 {
		// Basic sanity check for username-like strings
		if !strings.Contains(lastPart, ".") && !strings.Contains(lastPart, "=") {
			return lastPart
		}
	}

	return ""
}

// ValidateAndNormalizeUserFields validates and normalizes URLs in user profile fields
func (r *UserRepository) ValidateAndNormalizeUserFields(ctx context.Context, fields []map[string]string) ([]map[string]string, []string, error) {
	if r.urlValidator == nil {
		r.logger.Warn("URL validator not initialized, returning fields unchanged")
		return fields, nil, nil
	}

	return r.urlValidator.ValidateAndNormalizeProfileURLs(ctx, fields)
}

// ExtractProfileURLs extracts and validates all URLs from user profile fields
func (r *UserRepository) ExtractProfileURLs(ctx context.Context, fields []map[string]string) ([]*URLExtractionResult, error) {
	if r.urlValidator == nil {
		r.logger.Warn("URL validator not initialized")
		return nil, ErrorHandler.HandleGetError(common.ValidationError{Field: "URL validator", Message: "not available"}, "URL validator", "get")
	}

	return r.urlValidator.ExtractProfileURLs(ctx, fields)
}

// ValidateUserURL validates and normalizes a single URL (for main profile URL field)
func (r *UserRepository) ValidateUserURL(ctx context.Context, rawURL string) (*URLExtractionResult, error) {
	if r.urlValidator == nil {
		r.logger.Warn("URL validator not initialized")
		return nil, ErrorHandler.HandleGetError(common.ValidationError{Field: "URL validator", Message: "not available"}, "URL validator", "get")
	}

	return r.urlValidator.ExtractAndValidateURL(ctx, rawURL)
}

// generateURLWarnings converts validation tags to user-friendly warning messages
func generateURLWarnings(validationTags []string) []string {
	var warnings []string
	for _, tag := range validationTags {
		switch tag {
		case "insecure_http":
			warnings = append(warnings, "Profile URL uses insecure HTTP protocol")
		case "suspicious_tld":
			warnings = append(warnings, "Profile URL uses suspicious domain")
		case "url_shortener":
			warnings = append(warnings, "Profile URL is a shortened URL")
		}
	}
	return warnings
}

// validateAndUpdateProfileURL validates and normalizes a profile URL
func (r *UserRepository) validateAndUpdateProfileURL(ctx context.Context, updates map[string]any) ([]string, error) {
	var warnings []string

	rawURL, exists := updates["url"]
	if !exists {
		return warnings, nil
	}

	urlStr, ok := rawURL.(string)
	if !ok || common.ValidateRequiredParam("urlStr", urlStr) != nil {
		return warnings, nil
	}

	if r.urlValidator == nil {
		return warnings, nil
	}

	result, err := r.urlValidator.ExtractAndValidateURL(ctx, urlStr)
	if err != nil {
		return warnings, ErrorHandler.HandleGetError(err, "profile URL", "validation")
	}

	if !result.IsValid {
		return warnings, ErrorHandler.HandleGetError(common.ValidationError{Field: "profile URL", Message: "invalid format"}, "profile URL", "validation")
	}

	updates["url"] = result.NormalizedURL
	warnings = append(warnings, generateURLWarnings(result.ValidationTags)...)

	return warnings, nil
}

// validateAndUpdateProfileFields validates and normalizes profile fields containing URLs
func (r *UserRepository) validateAndUpdateProfileFields(ctx context.Context, updates map[string]any) ([]string, error) {
	var warnings []string

	rawFields, exists := updates["fields"]
	if !exists {
		return warnings, nil
	}

	fields, ok := rawFields.([]map[string]string)
	if !ok {
		return warnings, nil
	}
	if err := common.ValidateSliceNotEmpty("fields", fields); err != nil {
		return warnings, nil
	}

	if r.urlValidator == nil {
		return warnings, nil
	}

	normalizedFields, fieldWarnings, err := r.urlValidator.ValidateAndNormalizeProfileURLs(ctx, fields)
	if err != nil {
		r.logger.Error("failed to validate profile fields", zap.Error(err))
		// Continue with original fields rather than failing
		return warnings, nil
	}

	updates["fields"] = normalizedFields
	warnings = append(warnings, fieldWarnings...)

	return warnings, nil
}

// UpdateUserWithURLValidation updates user profile with URL validation and normalization
func (r *UserRepository) UpdateUserWithURLValidation(ctx context.Context, username string, updates map[string]any) ([]string, error) {
	var allWarnings []string

	// Validate and normalize profile URL if present
	urlWarnings, err := r.validateAndUpdateProfileURL(ctx, updates)
	if err != nil {
		return allWarnings, err
	}
	allWarnings = append(allWarnings, urlWarnings...)

	// Validate and normalize fields if present
	fieldWarnings, err := r.validateAndUpdateProfileFields(ctx, updates)
	if err != nil {
		return allWarnings, err
	}
	allWarnings = append(allWarnings, fieldWarnings...)

	// Perform the actual update
	err = r.UpdateUser(ctx, username, updates)
	if err != nil {
		return allWarnings, ErrorHandler.HandleUpdateError(err, EntityUser, username)
	}

	return allWarnings, nil
}

// determineVisibility determines the visibility of a post based on addressing
func (r *UserRepository) determineVisibility(object map[string]interface{}) string {
	to := convertToStringSlice(object["to"])
	cc := convertToStringSlice(object["cc"])

	// Direct message - no public addressing
	if !containsPublicAddress(to) && !containsPublicAddress(cc) {
		return "direct"
	}

	// Public - addressed to public in 'to'
	if containsPublicAddress(to) {
		return "public"
	}

	// Unlisted - public in 'cc'
	if containsPublicAddress(cc) {
		return "unlisted"
	}

	// Private - followers only
	return "private"
}

// timelineSK generates a sort key for timeline entries using reverse timestamp
func (r *UserRepository) timelineSK(timestamp time.Time, postID string) string {
	// Use reverse timestamp for newest-first ordering
	reverseTimestamp := 9999999999 - timestamp.Unix()
	return fmt.Sprintf("%010d#%s", reverseTimestamp, postID)
}

// Helper functions

func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen]
}

func hasMediaAttachments(object map[string]interface{}) bool {
	attachments, ok := object["attachment"].([]interface{})
	return ok && len(attachments) > 0
}

func extractLanguage(object map[string]interface{}) string {
	if lang, ok := object["language"].(string); ok {
		return lang
	}
	if langMap, ok := object["contentMap"].(map[string]interface{}); ok && len(langMap) > 0 {
		// Return the first language found
		for lang := range langMap {
			return lang
		}
	}
	return "en" // Default to English
}

func extractPublishedTime(object map[string]interface{}) time.Time {
	if published, ok := object["published"].(string); ok {
		if t, err := time.Parse(time.RFC3339, published); err == nil {
			return t
		}
	}
	return time.Now()
}

func convertToStringSlice(v interface{}) []string {
	if v == nil {
		return []string{}
	}

	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case string:
		return []string{val}
	default:
		return []string{}
	}
}

func containsPublicAddress(slice []string) bool {
	for _, s := range slice {
		if s == activitypub.PublicAddress {
			return true
		}
	}
	return false
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// centralizedCostTracker implements the CostTracker interface using the centralized cost tracking framework
type centralizedCostTracker struct {
	costService *cost.TrackingService
	tableName   string
	logger      *zap.Logger
	reads       int64
	writes      int64
}

// CalculateCost returns the current cost metrics
func (c *centralizedCostTracker) CalculateCost() batch.CostMetrics {
	return batch.CostMetrics{
		DynamoDBReads:  c.reads,
		DynamoDBWrites: c.writes,
	}
}

// TrackDynamoWrite tracks DynamoDB write operations through centralized service
func (c *centralizedCostTracker) TrackDynamoWrite(items int) {
	c.writes += int64(items)

	if c.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "BatchWriteItem",
			TableName:          c.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: int64(items),
			ItemCount:          int64(items),
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("user_batch_delete_%d", time.Now().UnixNano()),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.costService.TrackDynamoOperation(ctx, operation); err != nil {
			c.logger.Warn("failed to track batch delete cost through centralized service",
				zap.Int("items", items),
				zap.Error(err))
		}
		cancel()
	}

	if c.logger != nil {
		c.logger.Debug("tracked timeline delete operations via centralized framework",
			zap.Int("deleted_items", items),
			zap.Int64("total_writes", c.writes))
	}
}

// TrackDynamoRead tracks DynamoDB read operations through centralized service
func (c *centralizedCostTracker) TrackDynamoRead(items int) {
	c.reads += int64(items)

	if c.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "BatchGetItem",
			TableName:          c.tableName,
			ConsumedReadUnits:  int64(items),
			ConsumedWriteUnits: 0,
			ItemCount:          int64(items),
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("user_batch_read_%d", time.Now().UnixNano()),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.costService.TrackDynamoOperation(ctx, operation); err != nil {
			c.logger.Warn("failed to track batch read cost through centralized service",
				zap.Int("items", items),
				zap.Error(err))
		}
		cancel()
	}
}

// timelineCostTracker implements the CostTracker interface for timeline deletion operations (LEGACY)
type timelineCostTracker struct {
	logger *zap.Logger
	reads  int64
	writes int64
}

// CalculateCost returns the current cost metrics
func (t *timelineCostTracker) CalculateCost() batch.CostMetrics {
	return batch.CostMetrics{
		DynamoDBReads:  t.reads,
		DynamoDBWrites: t.writes,
	}
}

// TrackDynamoWrite tracks DynamoDB write operations (deletes)
func (t *timelineCostTracker) TrackDynamoWrite(items int) {
	t.writes += int64(items)
	if t.logger != nil {
		t.logger.Debug("tracked timeline delete operations",
			zap.Int("deleted_items", items),
			zap.Int64("total_writes", t.writes))
	}
}

// TrackDynamoRead tracks DynamoDB read operations
func (t *timelineCostTracker) TrackDynamoRead(items int) {
	t.reads += int64(items)
}

// === COST TRACKING UTILITY METHODS ===

// SetCostService allows setting or updating the cost service
func (r *UserRepository) SetCostService(costService *cost.TrackingService) {
	r.BaseRepository.SetCostService(costService)
}

// TrackRead provides a simple way to track read operations
func (r *UserRepository) TrackRead(ctx context.Context, operationType string, readUnits int64) error {
	return r.BaseRepository.TrackRead(ctx, operationType, readUnits)
}

// TrackWrite provides a simple way to track write operations
func (r *UserRepository) TrackWrite(ctx context.Context, operationType string, writeUnits int64) error {
	return r.BaseRepository.TrackWrite(ctx, operationType, writeUnits)
}
