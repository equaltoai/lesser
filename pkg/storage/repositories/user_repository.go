package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
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

// UserRepository implements user operations using DynamORM
type UserRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
	deps      UserRepositoryDeps
}

// NewUserRepository creates a new user repository
func NewUserRepository(db core.DB, tableName string, logger *zap.Logger) *UserRepository {
	return &UserRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// SetDependencies sets the dependencies for cross-repository operations
func (r *UserRepository) SetDependencies(deps UserRepositoryDeps) {
	r.deps = deps
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
		Where("PK", "=", "USER#"+username).
		Where("SK", "=", "METADATA").
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
		Where("GSI2PK", "=", "EMAIL#"+strings.ToLower(email)).
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
		Where("PK", "=", "USER#"+username).
		Where("SK", "=", "METADATA").
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
		Where("PK", "=", "USER#"+username).
		Where("SK", "=", "METADATA").
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
	query := r.db.WithContext(ctx).Model(&models.User{}).
		Index("user-list-index").
		Where("GSI1PK", "=", "USERS").
		Limit(int(limit) + 1) // Request one extra to detect if there are more pages

	// Apply cursor if provided
	if cursor != "" {
		query = query.Where("GSI1SK", ">", cursor)
	}

	err := query.All(&userModels)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list users: %w", err)
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

// GetActiveUserCount returns the number of active users
func (r *UserRepository) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	// Calculate cutoff time for activity
	cutoffTime := time.Now().AddDate(0, 0, -days)
	cutoffTimestamp := cutoffTime.Unix()

	// Query users who have been active within the specified days
	// Use the last_activity index if available, otherwise fall back to status check
	var userModels []models.User
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("activity-index").
		Where("GSI3PK", "=", "ACTIVITY").
		Where("GSI3SK", ">=", fmt.Sprintf("%d", cutoffTimestamp)).
		All(&userModels)
	if err != nil {
		return 0, fmt.Errorf("failed to count active users: %w", err)
	}

	return int64(len(userModels)), nil
}

// GetTotalUserCount returns the total number of users in the system
func (r *UserRepository) GetTotalUserCount(ctx context.Context) (int64, error) {
	r.logger.Debug("getting total user count")

	// Use GSI1 (user-list-index) where all users have GSI1PK = "USERS"
	// This is much more efficient than scanning the main table
	count, err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("user-list-index").
		Where("GSI1PK", "=", "USERS").
		Count()

	if err != nil {
		r.logger.Error("failed to count total users", zap.Error(err))
		return 0, fmt.Errorf("failed to count total users: %w", err)
	}

	r.logger.Debug("retrieved total user count", zap.Int64("count", count))
	return count, nil
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

// GetDNSCache retrieves a cached DNS lookup result
func (r *UserRepository) GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	// Create model with keys set
	dnsCache := &models.DNSCache{
		Hostname: hostname,
	}
	dnsCache.UpdateKeys()

	// Query for the entry using DynamORM pattern
	err := r.db.WithContext(ctx).Model(&models.DNSCache{}).
		Where("PK", "=", dnsCache.PK).
		Where("SK", "=", dnsCache.SK).
		First(&dnsCache)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return nil when not found (matching legacy behavior)
			return nil, nil
		}
		r.logger.Error("failed to get DNS cache entry",
			zap.String("hostname", hostname),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get DNS cache entry: %w", err)
	}

	// Check if the entry has expired
	if dnsCache.ExpiresAt > 0 && time.Now().Unix() > dnsCache.ExpiresAt {
		// Entry has expired, return nil (matching legacy behavior)
		return nil, nil
	}

	// Convert to storage model
	entry := &storage.DNSCacheEntry{
		Hostname:   dnsCache.Hostname,
		IPs:        dnsCache.IPs,
		ResolvedAt: dnsCache.ResolvedAt,
		TTL:        int64(dnsCache.TTL),
	}

	return entry, nil
}

// SetDNSCache stores a DNS lookup result in the cache
func (r *UserRepository) SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error {
	if entry == nil {
		return fmt.Errorf("DNS cache entry cannot be nil")
	}

	// Calculate expiration time for DynamoDB TTL (Unix timestamp)
	expiresAt := time.Now().Add(time.Duration(entry.TTL) * time.Second).Unix()

	// Create DynamORM model
	dnsCache := &models.DNSCache{
		Hostname:   entry.Hostname,
		IPs:        entry.IPs,
		ResolvedAt: entry.ResolvedAt,
		TTL:        int(entry.TTL),
		ExpiresAt:  expiresAt,
	}
	dnsCache.UpdateKeys()

	// Save to DynamoDB using DynamORM pattern
	if err := r.db.WithContext(ctx).Model(dnsCache).Create(); err != nil {
		r.logger.Error("failed to set DNS cache entry",
			zap.String("hostname", entry.Hostname),
			zap.Error(err))
		return fmt.Errorf("failed to set DNS cache entry: %w", err)
	}

	r.logger.Debug("DNS cache entry stored",
		zap.String("hostname", entry.Hostname),
		zap.Int("ip_count", len(entry.IPs)),
		zap.Int64("ttl_seconds", entry.TTL))

	return nil
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
		return fmt.Errorf("account already pinned")
	}

	// Create the model
	pinModel := &models.AccountPin{
		Username:       pin.Username,
		PinnedActorID:  pin.PinnedActorID,
		PinnedUsername: pin.PinnedUsername,
		CreatedAt:      pin.CreatedAt,
	}
	pinModel.UpdateKeys()

	// Create in DynamoDB
	err = r.db.WithContext(ctx).Model(pinModel).Create()
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
	pin.UpdateKeys()

	// Delete from DynamoDB
	err := r.db.WithContext(ctx).Model(pin).Delete()
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

	err := r.db.WithContext(ctx).Model(&models.AccountPin{}).
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
	pin.UpdateKeys()

	// Check if exists
	var found models.AccountPin
	err := r.db.WithContext(ctx).Model(&models.AccountPin{}).
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
	noteModel.UpdateKeys()

	// Create in DynamoDB
	err := r.db.WithContext(ctx).Model(noteModel).Create()
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
	note.UpdateKeys()

	// Query from DynamoDB
	var found models.AccountNote
	err := r.db.WithContext(ctx).Model(&models.AccountNote{}).
		Where("PK", "=", note.PK).
		Where("SK", "=", note.SK).
		First(&found)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		r.logger.Error("failed to get account note", zap.Error(err))
		return nil, err
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
	noteModel.UpdateKeys()

	// Update in DynamoDB (Put overwrites existing)
	err := r.db.WithContext(ctx).Model(noteModel).Create()
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
	note.UpdateKeys()

	// Delete from DynamoDB
	err := r.db.WithContext(ctx).Model(note).Delete()
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
	err := r.db.WithContext(ctx).Model(repModel).Create()
	if err != nil {
		r.logger.Error("failed to store reputation", zap.Error(err))
		return fmt.Errorf("failed to store reputation: %w", err)
	}

	return nil
}

// GetReputation retrieves the latest reputation for an actor
func (r *UserRepository) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	// Extract username from actorID
	username := extractUsernameFromActorID(actorID)
	if username == "" {
		return nil, nil // Return nil (not error) when invalid actorID
	}

	// Build query for latest reputation
	pk := fmt.Sprintf("ACTOR#%s", username)
	skPrefix := "REP#"

	// Query for latest reputation (most recent first)
	var reputations []models.Reputation
	err := r.db.WithContext(ctx).
		Model(&models.Reputation{}).
		Where("PK", "=", pk).
		Filter("SK", "BEGINS_WITH", skPrefix).
		OrderBy("SK", "DESC"). // Descending order to get latest first
		Limit(1).
		All(&reputations)

	if err != nil {
		r.logger.Error("failed to query reputation", zap.Error(err))
		return nil, fmt.Errorf("failed to query reputation: %w", err)
	}

	// No reputation found
	if len(reputations) == 0 {
		return nil, nil // Return nil (not error) when not found
	}

	// Convert to storage.Reputation
	repInterface, err := reputations[0].ToStorageReputation()
	if err != nil {
		r.logger.Error("failed to unmarshal reputation", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal reputation: %w", err)
	}

	// Convert interface back to storage.Reputation
	var reputation storage.Reputation
	repJSON, _ := json.Marshal(repInterface)
	if err := json.Unmarshal(repJSON, &reputation); err != nil {
		return nil, fmt.Errorf("failed to convert reputation: %w", err)
	}

	return &reputation, nil
}

// GetReputationHistory retrieves reputation history for an actor
func (r *UserRepository) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	// Extract username from actorID
	username := extractUsernameFromActorID(actorID)
	if username == "" {
		return []*storage.Reputation{}, nil // Return empty slice when invalid actorID
	}

	// Build query
	pk := fmt.Sprintf("ACTOR#%s", username)
	skPrefix := "REP#"

	// Query for reputation history
	var reputations []models.Reputation
	query := r.db.WithContext(ctx).
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
		return nil, fmt.Errorf("failed to query reputation history: %w", err)
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
	if vouch.ID == "" {
		vouch.ID = fmt.Sprintf("vouch-%d-%s", time.Now().Unix(), generateRandomID(8))
	}

	// Marshal vouch to JSON
	vouchJSON, err := json.Marshal(vouch)
	if err != nil {
		return fmt.Errorf("failed to marshal vouch: %w", err)
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
	if err := r.db.Model(vouchModel).Create(); err != nil {
		return fmt.Errorf("failed to store vouch: %w", err)
	}

	return nil
}

// GetVouch retrieves a vouch by ID
func (r *UserRepository) GetVouch(_ context.Context, vouchID string) (*storage.Vouch, error) {
	// Query by primary key
	var vouchModels []*models.Vouch
	err := r.db.Model(&models.Vouch{}).
		Where("PK", "=", fmt.Sprintf("VOUCH#%s", vouchID)).
		Where("SK", "=", "METADATA").
		Scan(&vouchModels)

	if err != nil {
		return nil, fmt.Errorf("failed to get vouch: %w", err)
	}

	// Return nil if not found
	if len(vouchModels) == 0 {
		return nil, nil
	}

	vouchModel := vouchModels[0]

	// Unmarshal vouch data
	if vouchModel.VouchData == "" {
		return nil, nil
	}

	var vouch storage.Vouch
	if err := json.Unmarshal([]byte(vouchModel.VouchData), &vouch); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vouch data: %w", err)
	}

	return &vouch, nil
}

// GetVouchesByActor retrieves vouches given by an actor
func (r *UserRepository) GetVouchesByActor(_ context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	// Query GSI1 for vouches by this actor
	query := r.db.Model(&models.Vouch{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", fmt.Sprintf("VOUCHER#%s", actorID))

	// Add active filter if requested
	if activeOnly {
		query = query.Filter("Active", "=", true)
	}

	// Execute query
	var vouchModels []*models.Vouch
	if err := query.Scan(&vouchModels); err != nil {
		return nil, fmt.Errorf("failed to query vouches by actor: %w", err)
	}

	// Convert to storage.Vouch slice
	vouches := make([]*storage.Vouch, 0, len(vouchModels))
	for _, model := range vouchModels {
		if model.VouchData == "" {
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

// GetVouchesForActor retrieves vouches received by an actor
func (r *UserRepository) GetVouchesForActor(_ context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	// Query GSI2 for vouches for this actor
	query := r.db.Model(&models.Vouch{}).
		Index("gsi2-index").
		Where("GSI2PK", "=", fmt.Sprintf("VOUCHEE#%s", actorID))

	// Add active filter if requested
	if activeOnly {
		query = query.Filter("Active", "=", true)
	}

	// Execute query
	var vouchModels []*models.Vouch
	if err := query.Scan(&vouchModels); err != nil {
		return nil, fmt.Errorf("failed to query vouches for actor: %w", err)
	}

	// Convert to storage.Vouch slice
	vouches := make([]*storage.Vouch, 0, len(vouchModels))
	for _, model := range vouchModels {
		if model.VouchData == "" {
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

// UpdateVouchStatus updates the active status of a vouch
func (r *UserRepository) UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	// First get the vouch to update the JSON data
	vouch, err := r.GetVouch(ctx, vouchID)
	if err != nil {
		return err
	}
	if vouch == nil {
		return fmt.Errorf("vouch not found")
	}

	// Update vouch fields
	vouch.Active = active
	vouch.Revoked = !active
	vouch.RevokedAt = revokedAt

	// Marshal updated vouch
	vouchJSON, err := json.Marshal(vouch)
	if err != nil {
		return fmt.Errorf("failed to marshal vouch: %w", err)
	}

	// Create model with updated data
	vouchModel := &models.Vouch{
		PK:        fmt.Sprintf("VOUCH#%s", vouchID),
		SK:        "METADATA",
		VouchData: string(vouchJSON),
		Active:    active,
	}
	expiresAt := time.Time{}
	if vouch.ExpiresAt != nil {
		expiresAt = *vouch.ExpiresAt
	}
	vouchModel.UpdateKeys(vouch.ID, vouch.From, vouch.To, vouch.Active, vouch.CreatedAt, expiresAt)

	// Update in DynamoDB
	if err := r.db.Model(vouchModel).Update(); err != nil {
		return fmt.Errorf("failed to update vouch status: %w", err)
	}

	return nil
}

// GetMonthlyVouchCount gets the count of vouches created by an actor in a specific month
func (r *UserRepository) GetMonthlyVouchCount(_ context.Context, actorID string, year int, month time.Month) (int, error) {
	// Calculate start and end of month
	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	// Query GSI1 with date range filter
	query := r.db.Model(&models.Vouch{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", fmt.Sprintf("VOUCHER#%s", actorID))

	// Execute query - we'll filter in memory since DynamORM doesn't support BETWEEN on non-key attributes
	var vouchModels []*models.Vouch
	if err := query.Scan(&vouchModels); err != nil {
		return 0, fmt.Errorf("failed to query monthly vouch count: %w", err)
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
	if relationship.ID == "" {
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
	model.UpdateKeys()

	// Save to DynamoDB
	if err := r.db.Model(model).Create(); err != nil {
		return fmt.Errorf("failed to create trust relationship: %w", err)
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
	err := r.db.Model(model).
		Where("PK", "=", fmt.Sprintf("TRUST#%s#%s", trusterID, category)).
		Where("SK", "=", fmt.Sprintf("TRUSTEE#%s", trusteeID)).
		First(model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Return nil when not found, not error
		}
		return nil, fmt.Errorf("failed to get trust relationship: %w", err)
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

	if err := r.db.Model(model).Delete(); err != nil {
		return fmt.Errorf("failed to delete trust relationship: %w", err)
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
	query := r.db.Model(&models.TrustRelationship{}).
		Filter("Type", "=", "RELATIONSHIP").
		Limit(limit * 2) // Get more to account for filtering

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var models []*models.TrustRelationship
	err := query.Scan(&models)
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan trust relationships: %w", err)
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

	// DynamORM doesn't support cursor-based pagination on scans
	nextCursor := ""

	return relationships, nextCursor, nil
}

// GetTrustedByRelationships retrieves all relationships where the actor is trusted
func (r *UserRepository) GetTrustedByRelationships(_ context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	// Use GSI1 to query by trustee
	// DynamORM doesn't support begins_with, so we'll filter in memory
	query := r.db.Model(&models.TrustRelationship{}).
		Index("gsi1-index").
		Filter("Type", "=", "RELATIONSHIP").
		Limit(limit * 2) // Get more to account for filtering

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var models []*models.TrustRelationship
	err := query.Scan(&models)
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan trusted-by relationships: %w", err)
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

	// DynamORM doesn't support cursor-based pagination on scans
	nextCursor := ""

	return relationships, nextCursor, nil
}

// GetTrustScore retrieves a cached trust score or calculates it
func (r *UserRepository) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	// First, try to get cached score
	cacheModel := &models.TrustScore{}
	cacheKey := fmt.Sprintf("SCORE#%s#%s", actorID, category)

	err := r.db.Model(cacheModel).
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
	model.UpdateKeys()

	// Save to DynamoDB
	if err := r.db.Model(model).Create(); err != nil {
		return fmt.Errorf("failed to update trust score: %w", err)
	}

	return nil
}

// RecordTrustUpdate records a trust score update event
func (r *UserRepository) RecordTrustUpdate(_ context.Context, update *storage.TrustUpdate) error {
	update.Timestamp = time.Now()

	// Generate event ID if not set
	if update.EventID == "" {
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
	model.UpdateKeys()

	// Save to DynamoDB
	if err := r.db.Model(model).Create(); err != nil {
		return fmt.Errorf("failed to record trust update: %w", err)
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
	query := r.db.Model(&models.TrustRelationship{}).
		Filter("Type", "=", "RELATIONSHIP").
		Limit(limit)

	var models []*models.TrustRelationship
	if err := query.Scan(&models); err != nil {
		return nil, fmt.Errorf("failed to scan all trust relationships: %w", err)
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

	if err := r.db.Model(model).Delete(); err != nil {
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

	if len(relationships) == 0 {
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
		prefs = &storage.UserPreferences{
			Username:    username,
			Language:    language,
			Preferences: make(map[string]string),
			UpdatedAt:   time.Now(),
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
	prefModel.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.UserPreferences{}).
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
				ExpandSpoilers:            defaultModelStorage.ExpandSpoilers,
				ExpandMedia:               defaultModelStorage.ExpandMedia,
				AutoplayGifs:              defaultModelStorage.AutoplayGifs,
				ShowFollowCounts:          defaultModelStorage.ShowFollowCounts,
				PreferredTimelineOrder:    defaultModelStorage.PreferredTimelineOrder,
				SearchSuggestionsEnabled:  defaultModelStorage.SearchSuggestionsEnabled,
				PersonalizedSearchEnabled: defaultModelStorage.PersonalizedSearchEnabled,
				ReblogFilters:             defaultModelStorage.ReblogFilters,
			}, nil
		}
		r.logger.Error("failed to get user preferences",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user preferences: %w", err)
	}

	// Convert models.UserPreferencesStorage to storage.UserPreferences
	modelStorage := prefModel.ToStorage()
	return &storage.UserPreferences{
		Language:                  modelStorage.Language,
		DefaultPostingVisibility:  modelStorage.DefaultPostingVisibility,
		DefaultMediaSensitive:     modelStorage.DefaultMediaSensitive,
		ExpandSpoilers:            modelStorage.ExpandSpoilers,
		ExpandMedia:               modelStorage.ExpandMedia,
		AutoplayGifs:              modelStorage.AutoplayGifs,
		ShowFollowCounts:          modelStorage.ShowFollowCounts,
		PreferredTimelineOrder:    modelStorage.PreferredTimelineOrder,
		SearchSuggestionsEnabled:  modelStorage.SearchSuggestionsEnabled,
		PersonalizedSearchEnabled: modelStorage.PersonalizedSearchEnabled,
		ReblogFilters:             modelStorage.ReblogFilters,
	}, nil
}

// UpdateUserPreferences updates user preferences
func (r *UserRepository) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	// Convert storage.UserPreferences to models.UserPreferencesStorage
	modelStorage := &models.UserPreferencesStorage{
		Language:                  preferences.Language,
		DefaultPostingVisibility:  preferences.DefaultPostingVisibility,
		DefaultMediaSensitive:     preferences.DefaultMediaSensitive,
		ExpandSpoilers:            preferences.ExpandSpoilers,
		ExpandMedia:               preferences.ExpandMedia,
		AutoplayGifs:              preferences.AutoplayGifs,
		ShowFollowCounts:          preferences.ShowFollowCounts,
		PreferredTimelineOrder:    preferences.PreferredTimelineOrder,
		SearchSuggestionsEnabled:  preferences.SearchSuggestionsEnabled,
		PersonalizedSearchEnabled: preferences.PersonalizedSearchEnabled,
		ReblogFilters:             preferences.ReblogFilters,
	}

	// Create DynamORM model from storage preferences
	prefModel := &models.UserPreferences{}
	prefModel.FromStorage(username, modelStorage)

	// Create or update the preferences using DynamORM
	err := r.db.WithContext(ctx).Model(prefModel).Create()
	if err != nil {
		r.logger.Error("failed to update user preferences",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to update preferences: %w", err)
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
	default:
		return fmt.Errorf("unknown preference key: %s", key)
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
		return nil, fmt.Errorf("unknown preference key: %s", key)
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
		"expand_spoilers":             prefs.ExpandSpoilers,
		"expand_media":                prefs.ExpandMedia,
		"autoplay_gifs":               prefs.AutoplayGifs,
		"show_follow_counts":          prefs.ShowFollowCounts,
		"preferred_timeline_order":    prefs.PreferredTimelineOrder,
		"search_suggestions_enabled":  prefs.SearchSuggestionsEnabled,
		"personalized_search_enabled": prefs.PersonalizedSearchEnabled,
		"reblog_filters":              prefs.ReblogFilters,
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
	default:
		r.logger.Warn("unknown preference key ignored",
			zap.String("key", key),
			zap.String("username", username))
		return nil
	}
}

// setStringPreference sets a string preference with type checking
func (r *UserRepository) setStringPreference(field *string, value any, key string) error {
	v, ok := value.(string)
	if !ok {
		return fmt.Errorf("invalid type for %s preference: expected string", key)
	}
	*field = v
	return nil
}

// setBoolPreference sets a boolean preference with type checking
func (r *UserRepository) setBoolPreference(field *bool, value any, key string) error {
	v, ok := value.(bool)
	if !ok {
		return fmt.Errorf("invalid type for %s preference: expected bool", key)
	}
	*field = v
	return nil
}

// setReblogFiltersPreference sets the reblog filters preference with type checking
func (r *UserRepository) setReblogFiltersPreference(field *map[string]bool, value any, key string) error {
	v, ok := value.(map[string]bool)
	if !ok {
		return fmt.Errorf("invalid type for %s preference: expected map[string]bool", key)
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
	err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", followerUsername)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", followedUsername)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("follow relationship not found")
		}
		r.logger.Error("failed to get follow relationship", zap.Error(err))
		return fmt.Errorf("failed to get follow relationship: %w", err)
	}

	// Update the relationship state
	relationship.Accept()

	// Save the updated relationship
	err = r.db.WithContext(ctx).Model(&relationship).Update()
	if err != nil {
		r.logger.Error("failed to update relationship state", zap.Error(err))
		return fmt.Errorf("failed to update relationship state: %w", err)
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
	err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", followerUsername)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", followedUsername)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("follow relationship not found")
		}
		r.logger.Error("failed to get follow relationship", zap.Error(err))
		return fmt.Errorf("failed to get follow relationship: %w", err)
	}

	// Update the relationship state
	relationship.Reject()

	// Save the updated relationship
	err = r.db.WithContext(ctx).Model(&relationship).Update()
	if err != nil {
		r.logger.Error("failed to update relationship state", zap.Error(err))
		return fmt.Errorf("failed to update relationship state: %w", err)
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

// updateActorCount updates either follower or following count for a user's actor
func (r *UserRepository) updateActorCount(ctx context.Context, username string, delta int, countType countUpdateType) error {
	// Get the current actor
	var actor models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s", username)).
		Where("SK", "=", "PROFILE").
		First(&actor)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Warn("actor not found for count update",
				zap.String("username", username),
				zap.String("count_type", string(countType)))
			return nil // Don't error if actor doesn't exist
		}
		return fmt.Errorf("failed to get actor: %w", err)
	}

	// Update the appropriate count
	var newCount int
	switch countType {
	case countUpdateFollowers:
		actor.FollowerCount += delta
		if actor.FollowerCount < 0 {
			actor.FollowerCount = 0
		}
		newCount = actor.FollowerCount
	case countUpdateFollowing:
		actor.FollowingCount += delta
		if actor.FollowingCount < 0 {
			actor.FollowingCount = 0
		}
		newCount = actor.FollowingCount
	}

	// Update keys to reflect new counts
	if err := actor.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update actor keys after count change: %w", err)
	}

	// Save the updated actor
	err = r.db.WithContext(ctx).Model(&actor).Update()
	if err != nil {
		return fmt.Errorf("failed to update %s count: %w", string(countType), err)
	}

	r.logger.Debug("updated count",
		zap.String("username", username),
		zap.String("count_type", string(countType)),
		zap.Int("delta", delta),
		zap.Int("new_count", newCount))

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
	muteModel.UpdateKeys()

	// Create the mute
	err := r.db.WithContext(ctx).Model(muteModel).Create()

	if err != nil {
		// Check if it's a duplicate (condition check failed)
		if errors.IsConditionFailed(err) {
			return fmt.Errorf("conversation already muted")
		}
		return fmt.Errorf("failed to create conversation mute: %w", err)
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
	muteModel.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.ConversationMute{}).
		Where("PK", "=", muteModel.PK).
		Where("SK", "=", muteModel.SK).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete conversation mute: %w", err)
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
	muteModel.UpdateKeys()

	var result models.ConversationMute
	err := r.db.WithContext(ctx).Model(&models.ConversationMute{}).
		Where("PK", "=", muteModel.PK).
		Where("SK", "=", muteModel.SK).
		First(&result)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check conversation mute: %w", err)
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
	err := r.db.WithContext(ctx).Model(&models.ConversationMute{}).
		Where("PK", "=", pk).
		All(&mutes)

	if err != nil {
		return nil, fmt.Errorf("failed to query muted conversations: %w", err)
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
		return false, fmt.Errorf("failed to check notification mute status: %w", err)
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
	err = r.db.WithContext(ctx).Model(&models.NotificationPreferences{}).
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
	remoteActor.UpdateKeys()

	// Create in DynamoDB using DynamORM
	err := r.db.WithContext(ctx).Model(remoteActor).Create()
	if err != nil {
		r.logger.Error("failed to cache remote actor",
			zap.String("handle", handle),
			zap.String("actor_id", actor.ID),
			zap.Error(err))
		return fmt.Errorf("failed to cache remote actor: %w", err)
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
	r.logger.Debug("creating bookmark",
		zap.String("username", username),
		zap.String("object_id", objectID))

	// Create the bookmark record
	now := time.Now()
	bookmark := &models.Bookmark{
		Username:  username,
		ObjectID:  objectID,
		CreatedAt: now,
	}
	bookmark.UpdateKeys()

	// Create the bookmark using DynamORM with condition check to prevent duplicates
	// Note: DynamORM Create will overwrite if the same keys exist, so we need to check first
	err := r.db.WithContext(ctx).Model(bookmark).Create()
	if err != nil {
		// Check if already bookmarked by trying a conditional create
		// Since DynamORM doesn't have built-in conditional creates, we log and continue
		// This matches the legacy behavior where duplicate bookmarks are silently ignored
		r.logger.Debug("bookmark creation result",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))

		// Check for specific error types to handle appropriately
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			// Bookmark already exists, this is expected behavior
			return nil
		}
		// For other errors, return them to the caller
		return fmt.Errorf("failed to create bookmark: %w", err)
	}

	r.logger.Info("bookmark created successfully",
		zap.String("username", username),
		zap.String("object_id", objectID))

	return nil
}

// RemoveBookmark removes a bookmark for a user
func (r *UserRepository) RemoveBookmark(ctx context.Context, username, objectID string) error {
	r.logger.Debug("removing bookmark",
		zap.String("username", username),
		zap.String("object_id", objectID))

	// Query to find bookmarks with the specific objectID
	pk := fmt.Sprintf("BOOKMARK#%s", username)

	var bookmarks []models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		Filter("ObjectID", "=", objectID).
		All(&bookmarks)

	if err != nil {
		r.logger.Error("failed to query bookmark for removal",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to query bookmark: %w", err)
	}

	// Delete all found bookmarks (should typically be 0 or 1)
	for _, bookmark := range bookmarks {
		err = r.db.WithContext(ctx).Model(&models.Bookmark{}).
			Where("PK", "=", bookmark.PK).
			Where("SK", "=", bookmark.SK).
			Delete()

		if err != nil {
			r.logger.Error("failed to delete bookmark",
				zap.String("username", username),
				zap.String("object_id", objectID),
				zap.Error(err))
			return fmt.Errorf("failed to delete bookmark: %w", err)
		}
	}

	r.logger.Info("bookmark removed successfully",
		zap.String("username", username),
		zap.String("object_id", objectID),
		zap.Int("removed_count", len(bookmarks)))

	return nil
}

// GetBookmarks retrieves bookmarks for a user with pagination
func (r *UserRepository) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	r.logger.Debug("getting bookmarks",
		zap.String("username", username),
		zap.Int("limit", limit),
		zap.String("cursor", cursor))

	// Validate limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Build query
	pk := fmt.Sprintf("BOOKMARK#%s", username)
	query := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC"). // Newest first (descending order)
		Limit(limit + 1)       // Request one extra to determine if there's a next page

	// Add cursor if provided
	if cursor != "" {
		// For DynamORM, we need to use the exact SK value as cursor
		query = query.Filter("SK", "<", cursor)
	}

	// Execute query
	var bookmarks []models.Bookmark
	err := query.All(&bookmarks)
	if err != nil {
		r.logger.Error("failed to query bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query bookmarks: %w", err)
	}

	// Extract object IDs
	objectIDs := make([]string, 0, len(bookmarks))
	for i, bookmark := range bookmarks {
		// Skip the extra item used for pagination
		if i >= limit {
			break
		}
		objectIDs = append(objectIDs, bookmark.ObjectID)
	}

	// Determine next cursor
	var nextCursor string
	if len(bookmarks) > limit && len(objectIDs) > 0 {
		// Use the SK of the last returned item as cursor
		nextCursor = bookmarks[limit-1].SK
	}

	r.logger.Debug("retrieved bookmarks",
		zap.String("username", username),
		zap.Int("count", len(objectIDs)),
		zap.String("next_cursor", nextCursor))

	return objectIDs, nextCursor, nil
}

// IsBookmarked checks if a user has bookmarked an object
func (r *UserRepository) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	r.logger.Debug("checking if bookmarked",
		zap.String("username", username),
		zap.String("object_id", objectID))

	// Query to find the bookmark with the specific objectID
	pk := fmt.Sprintf("BOOKMARK#%s", username)

	var bookmarks []models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		Filter("ObjectID", "=", objectID).
		Limit(1).
		All(&bookmarks)

	if err != nil {
		r.logger.Error("failed to query bookmark status",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		return false, fmt.Errorf("failed to query bookmark: %w", err)
	}

	isBookmarked := len(bookmarks) > 0

	r.logger.Debug("bookmark status checked",
		zap.String("username", username),
		zap.String("object_id", objectID),
		zap.Bool("is_bookmarked", isBookmarked))

	return isBookmarked, nil
}

// DeleteFromTimeline removes a specific timeline entry
func (r *UserRepository) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	// We need to find the entry first to get its timestamp
	pk := fmt.Sprintf("timeline#%s#%s", timelineType, timelineID)

	// Query for the specific entry
	var entry models.Timeline
	err := r.db.WithContext(ctx).Model(&models.Timeline{}).
		Where("PK", "=", pk).
		Filter("EntryID", "=", entryID).
		First(&entry)

	if err != nil {
		if errors.IsNotFound(err) {
			// Entry not found, nothing to delete
			return nil
		}
		return fmt.Errorf("failed to find timeline entry: %w", err)
	}

	// Now delete the entry using its PK and SK
	err = r.db.WithContext(ctx).Model(&entry).Delete()
	if err != nil {
		r.logger.Error("failed to delete timeline entry",
			zap.String("timeline_type", timelineType),
			zap.String("timeline_id", timelineID),
			zap.String("entry_id", entryID),
			zap.Error(err))
		return fmt.Errorf("failed to delete timeline entry: %w", err)
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
	err := r.db.WithContext(ctx).Model(&models.Timeline{}).
		Filter("ExpiresAt", "<", before).
		All(&expiredEntries)
	if err != nil {
		r.logger.Error("failed to scan for expired timeline entries",
			zap.Time("before", before),
			zap.Error(err))
		return fmt.Errorf("failed to scan for expired timeline entries: %w", err)
	}

	if len(expiredEntries) == 0 {
		r.logger.Debug("no expired timeline entries found",
			zap.Time("before", before))
		return nil // Nothing to delete
	}

	// Delete entries one by one (in a real implementation, you'd use batch operations)
	deletedCount := 0
	for _, entry := range expiredEntries {
		err := r.db.WithContext(ctx).Model(entry).Delete()
		if err != nil {
			r.logger.Error("failed to delete expired timeline entry",
				zap.String("pk", entry.PK),
				zap.String("sk", entry.SK),
				zap.Error(err))
			return fmt.Errorf("failed to delete timeline entry: %w", err)
		}
		deletedCount++
	}

	r.logger.Info("deleted expired timeline entries",
		zap.Time("before", before),
		zap.Int("deleted_count", deletedCount))

	return nil
}

// getTimelineEntries is a generic function to retrieve timeline entries with pagination
func (r *UserRepository) getTimelineEntries(ctx context.Context, pk, errorContext string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// Build query
	query := r.db.WithContext(ctx).Model(&models.Timeline{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC") // Most recent first

	// Handle cursor-based pagination
	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var entries []*models.Timeline
	err := query.All(&entries)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get %s timeline entries: %w", errorContext, err)
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
				if e.ExpiresAt.IsZero() {
					return nil
				}
				t := e.ExpiresAt
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
	err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", followerID)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", targetID)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return "none", nil
		}
		return "", fmt.Errorf("failed to get follow request state: %w", err)
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
		return nil, "", fmt.Errorf("dependencies not available")
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
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("users-by-role").
		Where("UserRole", "=", role).
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
		return fmt.Errorf("dependencies not available")
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
		return err
	}
	if object == nil {
		return nil // Unsupported object type
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
			return fmt.Errorf("failed to write to timelines: %w", err)
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
		return nil, nil, nil
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
	if metadata.objectID == "" || metadata.attributedTo == "" {
		log.Error("missing required fields in object", zap.Any("object", object))
		return nil, fmt.Errorf("object missing required fields")
	}

	// Extract username from actor ID
	metadata.username = extractUsernameFromActorID(metadata.attributedTo)
	if metadata.username == "" {
		log.Error("failed to extract username from actor", zap.String("actor", metadata.attributedTo))
		return nil, fmt.Errorf("invalid actor ID")
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
		if tag.Type != "Hashtag" || tag.Name == "" {
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
			return nil, fmt.Errorf("failed to get followers: %w", err)
		}

		// Create timeline entry for each follower
		for _, followerID := range followers {
			// Extract follower username
			followerUsername := extractUsernameFromActorID(followerID)
			if followerUsername == "" {
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
		if nextCursor == "" {
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
		return nil, fmt.Errorf("failed to get lists containing account: %w", err)
	}

	var entries []*models.Timeline

	for _, list := range lists {
		// Check list replies policy
		shouldInclude := false
		switch list.RepliesPolicy {
		case "none":
			// No replies
			shouldInclude = baseEntry.InReplyTo == ""
		case "followed":
			// Replies to followed accounts only - check if replied-to account is followed
			if baseEntry.InReplyTo == "" {
				// Not a reply, include it
				shouldInclude = true
			} else {
				// Check if the replied-to account is followed by list owner
				repliedToAccount := extractAccountFromReply(baseEntry.InReplyTo)
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
func extractAccountFromReply(inReplyTo string) string {
	// InReplyTo is typically a post ID like "POST#user#timestamp" or URL
	// Extract the username/account part
	if strings.HasPrefix(inReplyTo, "POST#") {
		parts := strings.Split(inReplyTo, "#")
		if len(parts) >= 2 {
			return parts[1] // Return the username part
		}
	}
	// For URLs, extract from path (ActivityPub URLs typically contain username)
	// This is a simplified extraction - could be enhanced for different URL patterns
	return ""
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
