package repositories

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/marshalers"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// AccountRepository unifies User and Actor operations into a single repository
// This represents the complete account entity with both authentication and federation aspects
type AccountRepository struct {
	// Use BaseRepository for User model as primary
	*BaseRepository[*models.User]
	db        core.DB
	logger    *zap.Logger
	tableName string
	domain    string
	
	// Dependencies for cross-repository operations
	// Note: storage.Storage dependency removed in Phase 5.6
}

// NewAccountRepository creates a new unified account repository
func NewAccountRepository(db core.DB, tableName string, domain string, logger *zap.Logger) *AccountRepository {
	return &AccountRepository{
		BaseRepository: NewBaseRepository[*models.User](db, tableName, logger),
		db:             db,
		logger:         logger,
		tableName:      tableName,
		domain:         domain,
	}
}

// SetStorage is deprecated - storage dependency removed in Phase 5.6
func (r *AccountRepository) SetStorage(storage interface{}) {
	// No-op: storage dependency removed
}

// ===== Core Account Operations =====

// CreateAccount creates both User and Actor entities atomically
// This ensures consistency between authentication and federation data
func (r *AccountRepository) CreateAccount(ctx context.Context, username, email, passwordHash string, approved bool, actor *activitypub.Actor, privateKey string) error {
	// Validate inputs
	if username == "" {
		return common.ValidationError{Field: "username", Message: "username is required"}
	}
	
	// Create User model
	user := &models.User{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Approved:     approved,
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	// Create user using BaseRepository
	if err := r.Create(ctx, user); err != nil {
		if errors.IsConditionFailed(err) {
			return common.ConflictError{
				Resource: "user",
				Message:  fmt.Sprintf("user %s already exists", username),
			}
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	
	// Create Actor
	if actor != nil {
		if err := r.createActor(ctx, actor, privateKey); err != nil {
			// Rollback user creation (best effort)
			r.Delete(ctx, fmt.Sprintf("USER#%s", username), "METADATA")
			return fmt.Errorf("failed to create actor: %w", err)
		}
	}
	
	r.logger.Info("created account",
		zap.String("username", username),
		zap.Bool("with_actor", actor != nil))
	
	return nil
}

// GetAccount retrieves complete account information (User + Actor)
func (r *AccountRepository) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	// Get user data
	user, err := r.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}
	
	// Get actor data
	actor, err := r.GetActor(ctx, username)
	if err != nil && !isAccountNotFound(err) {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}
	
	// Combine into account
	account := &storage.Account{
		User:  user,
		Actor: actor,
	}
	
	return account, nil
}

// DeleteAccount removes both User and Actor entities
func (r *AccountRepository) DeleteAccount(ctx context.Context, username string) error {
	// Delete actor first (it's optional)
	if err := r.deleteActor(ctx, username); err != nil && !isAccountNotFound(err) {
		r.logger.Error("failed to delete actor", 
			zap.String("username", username),
			zap.Error(err))
	}
	
	// Delete user using key utilities
	pk := Utils.Keys.UserKey(username)
	if err := r.Delete(ctx, pk, SKMetadata); err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityUser, username)
	}
	
	r.logger.Info("deleted account", zap.String("username", username))
	return nil
}

// ===== User Operations (Authentication) =====

// GetUser retrieves user authentication data
func (r *AccountRepository) GetUser(ctx context.Context, username string) (*storage.User, error) {
	user := &models.User{}
	
	// Use key utility for consistent key generation
	pk := Utils.Keys.UserKey(username)
	
	err := r.Get(ctx, pk, SKMetadata, user)
	if err != nil {
		// Use error utility for consistent error handling
		return nil, ErrorHandler.HandleGetError(err, EntityUser, username)
	}
	
	return r.modelToStorageUser(user), nil
}

// GetUserByEmail retrieves a user by email address
func (r *AccountRepository) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	var user models.User
	
	// Use GSI key utility for consistent key generation
	emailKey := Utils.GSI.EmailIndexKey(email)
	
	err := r.db.WithContext(ctx).Model(&user).
		Index("email-index").
		Where("GSI2PK", "=", emailKey).
		First(&user)
		
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityUser, email)
	}
	
	return r.modelToStorageUser(&user), nil
}

// UpdateUser updates user authentication data
func (r *AccountRepository) UpdateUser(ctx context.Context, username string, updates map[string]interface{}) error {
	// Get existing user using utilities
	user := &models.User{}
	pk := Utils.Keys.UserKey(username)
	err := r.Get(ctx, pk, SKMetadata, user)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityUser, username)
	}
	
	// Apply updates
	if err := r.applyUserUpdates(user, updates); err != nil {
		return err
	}
	
	// Update using BaseRepository with error handling
	if err := r.Update(ctx, user); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityUser, username)
	}
	
	return nil
}

// ===== Actor Operations (ActivityPub) =====

// GetActor retrieves an actor by username
func (r *AccountRepository) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	var actorModel models.Actor
	
	// Use key utilities for consistent key generation
	pk := Utils.Keys.ActorKey(username)
	
	err := r.db.WithContext(ctx).Model(&actorModel).
		Where("PK", "=", pk).
		Where("SK", "=", SKProfile).
		First(&actorModel)
		
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityActor, username)
	}
	
	return actorModel.Actor, nil
}

// GetActorByUsername is an alias for GetActor (for compatibility)
func (r *AccountRepository) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	return r.GetActor(ctx, username)
}

// createActor creates an actor (internal helper)
func (r *AccountRepository) createActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	if actor.PreferredUsername == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}
	
	username := actor.PreferredUsername
	numericID := common.GenerateNumericID(username)
	
	// Encrypt private key if available
	encryptedKey := privateKey
	if encryptor, err := r.getEncryptor(); err == nil {
		if encrypted, err := encryptor.Encrypt([]byte(privateKey)); err == nil {
			encryptedKey = base64.StdEncoding.EncodeToString(encrypted)
		}
	}
	
	// Create actor model
	actorModel := &models.Actor{
		Username:       username,
		Actor:          actor,
		PrivateKey:     encryptedKey,
		NumericID:      numericID,
		FollowerCount:  0,
		FollowingCount: 0,
		StatusCount:    0,
	}
	
	// Set domain for GSI3
	if r.domain != "" {
		actorModel.GSI3PK = fmt.Sprintf("DOMAIN#%s", r.domain)
		actorModel.GSI3SK = username
	}
	
	// Create using DynamORM
	if err := r.db.WithContext(ctx).Model(actorModel).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			return common.ConflictError{
				Resource: "actor",
				Message:  fmt.Sprintf("actor %s already exists", username),
			}
		}
		return fmt.Errorf("failed to create actor: %w", err)
	}
	
	return nil
}

// deleteActor deletes an actor (internal helper)
func (r *AccountRepository) deleteActor(ctx context.Context, username string) error {
	// Use key utilities for consistent key generation
	pk := Utils.Keys.ActorKey(username)
	
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", pk).
		Where("SK", "=", SKProfile).
		Delete()
		
	// Use error utility for consistent error handling (delete doesn't fail on not found)
	return ErrorHandler.HandleDeleteError(err, EntityActor, username)
}

// GetActorPrivateKey retrieves an actor's private key
func (r *AccountRepository) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	var actorModel models.Actor
	
	// Use key utilities for consistent key generation
	pk := Utils.Keys.ActorKey(username)
	
	err := r.db.WithContext(ctx).Model(&actorModel).
		Where("PK", "=", pk).
		Where("SK", "=", SKProfile).
		Select("PrivateKey").
		First(&actorModel)
		
	if err != nil {
		return "", ErrorHandler.HandleGetError(err, EntityActor, username)
	}
	
	// Decrypt private key if encrypted
	privateKey := actorModel.PrivateKey
	if encryptor, err := r.getEncryptor(); err == nil {
		if decoded, err := base64.StdEncoding.DecodeString(privateKey); err == nil {
			if decrypted, err := encryptor.Decrypt(decoded); err == nil {
				privateKey = string(decrypted)
			}
		}
	}
	
	return privateKey, nil
}

// ===== Helper Methods =====

// modelToStorageUser converts User model to storage type
func (r *AccountRepository) modelToStorageUser(model *models.User) *storage.User {
	return &storage.User{
		Username:        model.Username,
		Email:           model.Email,
		PasswordHash:    model.PasswordHash,
		DisplayName:     model.DisplayName,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
		Approved:        model.Approved,
		Suspended:       model.Suspended,
		Silenced:        model.Silenced,
		Role:            model.Role,
		Locale:          model.Locale,
		RecoveryMethods: model.RecoveryMethods,
	}
}

// applyUserUpdates applies a map of updates to a user model
func (r *AccountRepository) applyUserUpdates(user *models.User, updates map[string]interface{}) error {
	for key, value := range updates {
		switch key {
		case "email":
			if v, ok := value.(string); ok {
				user.Email = v
			}
		case "password_hash":
			if v, ok := value.(string); ok {
				user.PasswordHash = v
			}
		case "display_name":
			if v, ok := value.(string); ok {
				user.DisplayName = v
			}
		case "approved":
			if v, ok := value.(bool); ok {
				user.Approved = v
			}
		case "suspended":
			if v, ok := value.(bool); ok {
				user.Suspended = v
			}
		case "silenced":
			if v, ok := value.(bool); ok {
				user.Silenced = v
			}
		case "role":
			if v, ok := value.(string); ok {
				user.Role = v
			}
		case "locale":
			if v, ok := value.(string); ok {
				user.Locale = v
			}
		}
	}
	
	user.UpdatedAt = time.Now()
	return nil
}

// getEncryptor returns an encryptor for private keys
func (r *AccountRepository) getEncryptor() (marshalers.Encryptor, error) {
	// For now, use the JWT secret as encryption key
	// In production, you'd want a dedicated encryption key
	jwtSecret := config.Get().JWTSecret
	if jwtSecret == "" {
		return nil, fmt.Errorf("no JWT secret available for encryption")
	}
	
	// Use first 32 bytes of JWT secret as AES key
	key := []byte(jwtSecret)
	if len(key) > 32 {
		key = key[:32]
	}
	for len(key) < 32 {
		key = append(key, 0) // Pad with zeros if needed
	}
	
	return marshalers.NewAESEncryptorWithKey(key)
}

// isAccountNotFound checks if an error is a not found error
func isAccountNotFound(err error) bool {
	return errors.IsNotFound(err) || strings.Contains(err.Error(), "not found")
}

// ===== Account Pin Operations =====

// CreateAccountPin creates a new account pin
func (r *AccountRepository) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	// Create model
	pinModel := &models.AccountPin{
		Username:       pin.Username,
		PinnedActorID:  pin.PinnedActorID,
		PinnedUsername: pin.PinnedUsername,
		CreatedAt:      pin.CreatedAt,
	}
	
	// Set timestamp if not provided
	if pinModel.CreatedAt.IsZero() {
		pinModel.CreatedAt = time.Now()
	}

	// Create using DynamORM
	if err := r.db.WithContext(ctx).Model(pinModel).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			// Already pinned, not an error
			return nil
		}
		return fmt.Errorf("failed to create account pin: %w", err)
	}

	r.logger.Info("created account pin",
		zap.String("username", pin.Username),
		zap.String("pinned_actor_id", pin.PinnedActorID))

	return nil
}

// DeleteAccountPin removes an account pin
func (r *AccountRepository) DeleteAccountPin(ctx context.Context, username, targetActorID string) error {
	pin := &models.AccountPin{
		Username:      username,
		PinnedActorID: targetActorID,
	}
	pin.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", pin.PK).
		Where("SK", "=", pin.SK).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete account pin: %w", err)
	}

	r.logger.Info("deleted account pin",
		zap.String("username", username),
		zap.String("target_actor_id", targetActorID))

	return nil
}

// IsAccountPinned checks if an account is pinned by a user
func (r *AccountRepository) IsAccountPinned(ctx context.Context, username, targetActorID string) (bool, error) {
	var pin models.AccountPin
	err := r.db.WithContext(ctx).Model(&pin).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_PIN#%s", username)).
		Where("SK", "=", fmt.Sprintf("PIN#%s", targetActorID)).
		First(&pin)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if account is pinned: %w", err)
	}

	return true, nil
}

// ===== Account Note Operations =====

// CreateAccountNote creates or updates a private note on an account
func (r *AccountRepository) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	// Create model
	noteModel := &models.AccountNote{
		Username:      note.Username,
		TargetActorID: note.TargetActorID,
		Note:          note.Note,
		CreatedAt:     note.CreatedAt,
		UpdatedAt:     note.UpdatedAt,
	}
	
	// Set timestamps if not provided
	now := time.Now()
	if noteModel.CreatedAt.IsZero() {
		noteModel.CreatedAt = now
	}
	if noteModel.UpdatedAt.IsZero() {
		noteModel.UpdatedAt = now
	}

	// Use upsert pattern - try to get existing first
	var existing models.AccountNote
	noteModel.UpdateKeys()
	err := r.db.WithContext(ctx).Model(&existing).
		Where("PK", "=", noteModel.PK).
		Where("SK", "=", noteModel.SK).
		First(&existing)

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing note: %w", err)
	}

	if errors.IsNotFound(err) {
		// Create new note
		if err := r.db.WithContext(ctx).Model(noteModel).Create(); err != nil {
			return fmt.Errorf("failed to create account note: %w", err)
		}
	} else {
		// Update existing note
		existing.Note = noteModel.Note
		existing.UpdatedAt = now
		if err := r.db.WithContext(ctx).Model(&existing).Update(); err != nil {
			return fmt.Errorf("failed to update account note: %w", err)
		}
	}

	r.logger.Info("created/updated account note",
		zap.String("username", note.Username),
		zap.String("target_actor_id", note.TargetActorID))

	return nil
}

// ===== Preference Operations =====

// GetPreference retrieves a specific user preference
func (r *AccountRepository) GetPreference(ctx context.Context, username, key string) (string, error) {
	var pref models.UserPreference
	err := r.db.WithContext(ctx).Model(&pref).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("PREFERENCE#%s", key)).
		First(&pref)

	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil // Return empty string for not found, not an error
		}
		return "", fmt.Errorf("failed to get preference: %w", err)
	}

	return pref.Value, nil
}

// ===== Follow Request Operations =====

// GetFollowRequestState retrieves the state of a follow request
func (r *AccountRepository) GetFollowRequestState(ctx context.Context, requesterID, targetID string) (string, error) {
	var request models.FollowRequestState
	err := r.db.WithContext(ctx).Model(&request).
		Where("PK", "=", fmt.Sprintf("FOLLOW_REQUEST#%s", requesterID)).
		Where("SK", "=", fmt.Sprintf("TARGET#%s", targetID)).
		First(&request)

	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil // Return empty string for not found, not an error
		}
		return "", fmt.Errorf("failed to get follow request state: %w", err)
	}

	return request.State, nil
}

// ===== Domain Block Operations =====

// IsBlockedDomain checks if a domain is blocked by a user
func (r *AccountRepository) IsBlockedDomain(ctx context.Context, userID, domain string) (bool, error) {
	var block models.UserDomainBlock
	err := r.db.WithContext(ctx).Model(&block).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		First(&block)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if domain is blocked: %w", err)
	}

	return true, nil
}

// ===== Field Verification Operations =====

// GetFieldVerification retrieves field verification info for a user
func (r *AccountRepository) GetFieldVerification(ctx context.Context, username, fieldName string) (*storage.ActorField, error) {
	var verification models.FieldVerification
	err := r.db.WithContext(ctx).Model(&verification).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("FIELD_VERIFICATION#%s", fieldName)).
		First(&verification)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Return nil for not found, not an error
		}
		return nil, fmt.Errorf("failed to get field verification: %w", err)
	}

	// Check if verification has expired
	if verification.IsExpired() {
		return nil, nil
	}

	// Convert to storage.ActorField
	return &storage.ActorField{
		Name:       verification.FieldName,
		Value:      verification.FieldValue,
		VerifiedAt: verification.VerifiedAt,
	}, nil
}

// Note: This is the core file. Additional methods will be organized into:
// - account_repository_auth.go (authentication methods)
// - account_repository_social.go (follows, blocks, mutes)
// - account_repository_timeline.go (timeline operations)
// - account_repository_search.go (search and discovery)