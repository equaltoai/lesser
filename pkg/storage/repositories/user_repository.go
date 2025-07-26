package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
)

// UserRepository implements user operations using DynamORM
type UserRepository struct {
	db core.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db core.DB) *UserRepository {
	return &UserRepository{
		db: db,
	}
}

// CreateUser creates a new user in DynamoDB
func (r *UserRepository) CreateUser(ctx context.Context, user *storage.User) error {
	if user.Username == "" {
		return common.ValidationError{Field: "Username", Message: "username is required"}
	}

	// Create the DynamORM model
	userModel := &models.User{
		Username:        user.Username,
		Email:           user.Email,
		PasswordHash:    user.PasswordHash,
		DisplayName:     user.DisplayName,
		Approved:        user.Approved,
		Suspended:       user.Suspended,
		Silenced:        user.Silenced,
		Role:            user.Role,
		Locale:          user.Locale,
		RecoveryMethods: user.RecoveryMethods,
	}

	// Create the user using DynamORM
	err := r.db.WithContext(ctx).Model(userModel).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			return common.ConflictError{
				Resource: "user",
				Message:  fmt.Sprintf("user %s already exists", user.Username),
			}
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Update the original user with timestamps
	user.CreatedAt = userModel.CreatedAt
	user.UpdatedAt = userModel.UpdatedAt

	return nil
}

// GetUser retrieves a user by username
func (r *UserRepository) GetUser(ctx context.Context, username string) (*storage.User, error) {
	var userModel models.User

	err := r.db.WithContext(ctx).Model(&models.User{}).
		Where("PK", "=", "user#"+username).
		Where("SK", "=", "user#"+username).
		First(&userModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("user not found: %s", username)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return r.modelToStorage(&userModel), nil
}

// GetUserByEmail retrieves a user by email address
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	if email == "" {
		return nil, common.ValidationError{Field: "Email", Message: "email is required"}
	}

	var userModels []models.User
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("email-index").
		Where("GSI1PK", "=", "EMAIL#"+strings.ToLower(email)).
		Limit(1).
		All(&userModels)
	if err != nil {
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}

	if len(userModels) == 0 {
		return nil, fmt.Errorf("user not found with email: %s", email)
	}

	return r.modelToStorage(&userModels[0]), nil
}

// UpdateUser updates an existing user
func (r *UserRepository) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
	if len(updates) == 0 {
		return common.ValidationError{Field: "Updates", Message: "no updates provided"}
	}

	// Get existing user first
	var userModel models.User
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Where("PK", "=", "user#"+username).
		Where("SK", "=", "user#"+username).
		First(&userModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("user not found: %s", username)
		}
		return fmt.Errorf("failed to get existing user: %w", err)
	}

	// Apply updates to the model
	r.applyUpdates(&userModel, updates)

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&userModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// DeleteUser deletes a user
func (r *UserRepository) DeleteUser(ctx context.Context, username string) error {
	// Delete the user using DynamORM
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Where("PK", "=", "user#"+username).
		Where("SK", "=", "user#"+username).
		Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("user not found: %s", username)
		}
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// ListUsers retrieves a paginated list of users
func (r *UserRepository) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var userModels []models.User
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("user-list-index").
		Where("GSI2PK", "=", "USERS").
		Limit(int(limit)).
		All(&userModels)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list users: %w", err)
	}

	// Convert to storage.User slice
	users := make([]*storage.User, 0, len(userModels))
	for _, userModel := range userModels {
		users = append(users, r.modelToStorage(&userModel))
	}

	// For now, we don't support pagination cursor
	// In a real implementation, this would use DynamORM's pagination features
	var nextCursor string

	return users, nextCursor, nil
}

// GetActiveUserCount returns the number of active users
func (r *UserRepository) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	// For now, return count of all active users
	// In a real implementation, this would query based on activity within the specified days
	var userModels []models.User
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("status-index").
		Where("GSI4PK", "=", "STATUS#active").
		All(&userModels)
	if err != nil {
		return 0, fmt.Errorf("failed to count active users: %w", err)
	}

	return int64(len(userModels)), nil
}

// GetUserByProviderID gets a user by their OAuth provider ID
func (r *UserRepository) GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error) {
	// Query the ProviderAccount by provider and providerID using GSI1
	var providerAccounts []models.ProviderAccount
	err := r.db.WithContext(ctx).Model(&models.ProviderAccount{}).
		Index("provider-index").
		Where("GSI1PK", "=", "PROVIDER#"+provider).
		Where("GSI1SK", "=", providerID+"#").
		Limit(1).
		All(&providerAccounts)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider account: %w", err)
	}

	if len(providerAccounts) == 0 {
		return nil, fmt.Errorf("user not found with provider %s:%s", provider, providerID)
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
	err = r.db.WithContext(ctx).Model(providerAccount).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			return common.ConflictError{
				Resource: "provider_account",
				Message:  fmt.Sprintf("provider account %s:%s already linked", provider, providerID),
			}
		}
		return fmt.Errorf("failed to link provider account: %w", err)
	}

	return nil
}

// UnlinkProviderAccount unlinks an OAuth provider account from a user
func (r *UserRepository) UnlinkProviderAccount(ctx context.Context, username, provider string) error {
	// Find the provider account for this user and provider
	// First get all provider accounts for this user
	var allProviderAccounts []models.ProviderAccount
	err := r.db.WithContext(ctx).Model(&models.ProviderAccount{}).
		Index("user-providers-index").
		Where("GSI2PK", "=", "USER_PROVIDERS#"+username).
		All(&allProviderAccounts)
	if err != nil {
		return fmt.Errorf("failed to query provider accounts: %w", err)
	}

	// Filter by provider manually since DynamORM might not support begins_with
	var providerAccounts []models.ProviderAccount
	for _, pa := range allProviderAccounts {
		if pa.Provider == provider {
			providerAccounts = append(providerAccounts, pa)
		}
	}

	if len(providerAccounts) == 0 {
		return fmt.Errorf("provider account not found for user %s and provider %s", username, provider)
	}

	// Delete the provider account(s) for this provider
	for _, pa := range providerAccounts {
		err = r.db.WithContext(ctx).Model(&pa).Delete()
		if err != nil {
			return fmt.Errorf("failed to unlink provider account: %w", err)
		}
	}

	return nil
}

// GetLinkedProviders gets all linked OAuth providers for a user
func (r *UserRepository) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	// Query all provider accounts for this user
	var providerAccounts []models.ProviderAccount
	err := r.db.WithContext(ctx).Model(&models.ProviderAccount{}).
		Index("user-providers-index").
		Where("GSI2PK", "=", "USER_PROVIDERS#"+username).
		All(&providerAccounts)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider accounts: %w", err)
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
	return &storage.User{
		Username:        userModel.Username,
		Email:           userModel.Email,
		PasswordHash:    userModel.PasswordHash,
		DisplayName:     userModel.DisplayName,
		CreatedAt:       userModel.CreatedAt,
		UpdatedAt:       userModel.UpdatedAt,
		Approved:        userModel.Approved,
		Suspended:       userModel.Suspended,
		Silenced:        userModel.Silenced,
		Role:            userModel.Role,
		Locale:          userModel.Locale,
		RecoveryMethods: userModel.RecoveryMethods,
	}
}

// applyUpdates applies the updates map to the user model
func (r *UserRepository) applyUpdates(userModel *models.User, updates map[string]any) {
	for key, value := range updates {
		switch key {
		case "email":
			if v, ok := value.(string); ok {
				userModel.Email = v
			}
		case "password_hash":
			if v, ok := value.(string); ok {
				userModel.PasswordHash = v
			}
		case "display_name":
			if v, ok := value.(string); ok {
				userModel.DisplayName = v
			}
		case "approved":
			if v, ok := value.(bool); ok {
				userModel.Approved = v
			}
		case "suspended":
			if v, ok := value.(bool); ok {
				userModel.Suspended = v
			}
		case "silenced":
			if v, ok := value.(bool); ok {
				userModel.Silenced = v
			}
		case "role":
			if v, ok := value.(string); ok {
				userModel.Role = v
			}
		case "locale":
			if v, ok := value.(string); ok {
				userModel.Locale = v
			}
		case "recovery_methods":
			if v, ok := value.([]string); ok {
				userModel.RecoveryMethods = v
			}
		}
	}
}
