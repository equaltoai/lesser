// Package repositories provides DynamORM repository implementations for account and user management operations.
package repositories

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/marshalers"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// AccountRepository unifies User and Actor operations into a single repository
// This represents the complete account entity with both authentication and federation aspects
type AccountRepository struct {
	// Use EnhancedBaseRepository for comprehensive functionality
	*EnhancedBaseRepository[*models.User]
	db        core.DB
	logger    *zap.Logger
	tableName string
	domain    string
	encryptor marshalers.Encryptor

	// Dependencies for cross-repository operations
	// Note: storage.Storage dependency removed in Phase 5.6
	statusRepo     interfaces.StatusRepository // For accessing status objects
	actorRepo      *ActorRepository
	bookmarkRepo   *BookmarkRepository
	governanceRepo *AgentGovernanceRepository
}

type userVersionProjection struct {
	Table string `json:"-"`
	PK    string `theorydb:"pk"`
	SK    string `theorydb:"sk"`
	Value *int   `theorydb:"attr:version"`
}

type userCoreProjection struct {
	_ struct{} `theorydb:"naming:camelCase"`

	Table string `json:"-"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Username           string               `theorydb:"attr:username"`
	Email              string               `theorydb:"attr:email"`
	PasswordHash       string               `theorydb:"attr:passwordHash"`
	DisplayName        string               `theorydb:"attr:displayName"`
	Note               string               `theorydb:"attr:note"`
	Avatar             string               `theorydb:"attr:avatar"`
	Header             string               `theorydb:"attr:header"`
	URL                string               `theorydb:"attr:url"`
	Locked             bool                 `theorydb:"attr:locked"`
	Discoverable       bool                 `theorydb:"attr:discoverable"`
	Fields             []map[string]string  `theorydb:"attr:fields"`
	CreatedAt          time.Time            `theorydb:"attr:createdAt"`
	UpdatedAt          time.Time            `theorydb:"attr:updatedAt"`
	Approved           bool                 `theorydb:"attr:approved"`
	Suspended          bool                 `theorydb:"attr:suspended"`
	Silenced           bool                 `theorydb:"attr:silenced"`
	Role               string               `theorydb:"attr:role"`
	Locale             string               `theorydb:"attr:locale"`
	RecoveryMethods    []string             `theorydb:"attr:recoveryMethods"`
	AllowNSFW          bool                 `theorydb:"attr:allowNSFW"`
	RequireNSFWWarning bool                 `theorydb:"attr:requireNSFWWarning"`
	IsAgent            bool                 `theorydb:"attr:isAgent"`
	AgentType          string               `theorydb:"attr:agentType"`
	AgentCapabilities  *agents.Capabilities `theorydb:"json,attr:agentCapabilities"`
	AgentVersion       string               `theorydb:"attr:agentVersion"`
	AgentOwner         string               `theorydb:"attr:agentOwner"`
	AgentCreatedBy     string               `theorydb:"attr:agentCreatedBy"`
	AgentPublicKey     string               `theorydb:"attr:agentPublicKey"`
	AgentKeyType       string               `theorydb:"attr:agentKeyType"`
	Version            int                  `theorydb:"version,attr:version"`
}

func (p userCoreProjection) TableName() string {
	if strings.TrimSpace(p.Table) != "" {
		return p.Table
	}
	return models.MainTableName
}

type userMetadataProjection struct {
	_ struct{} `theorydb:"naming:camelCase"`

	Table string `json:"-"`
	PK    string `theorydb:"pk,attr:PK"`
	SK    string `theorydb:"sk,attr:SK"`

	Metadata map[string]interface{} `theorydb:"attr:metadata"`
}

type userEmailLookupProjection struct {
	_ struct{} `theorydb:"naming:camelCase"`

	Table string `json:"-"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"`

	Username string `theorydb:"attr:username"`
	Email    string `theorydb:"attr:email"`
}

func (p userMetadataProjection) TableName() string {
	if strings.TrimSpace(p.Table) != "" {
		return p.Table
	}
	return models.MainTableName
}

func (p userVersionProjection) TableName() string {
	if strings.TrimSpace(p.Table) != "" {
		return p.Table
	}
	return models.MainTableName
}

func (p userVersionProjection) versionValue() (int, bool) {
	if p.Value == nil {
		return 0, false
	}
	return *p.Value, true
}

func (p userEmailLookupProjection) TableName() string {
	if strings.TrimSpace(p.Table) != "" {
		return p.Table
	}
	return models.MainTableName
}

// NewAccountRepository creates a new unified account repository
func NewAccountRepository(db core.DB, tableName string, domain string, logger *zap.Logger) *AccountRepository {
	// Use enhanced base repository with validation and permissions
	enhancedRepo := NewEnhancedBaseRepository[*models.User](db, tableName, logger, nil, "AccountRepository", "account")

	// Set up default services
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &AccountRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		logger:                 logger,
		tableName:              tableName,
		domain:                 domain,
		actorRepo:              NewActorRepository(db, tableName, logger, domain),
		governanceRepo:         NewAgentGovernanceRepository(db, tableName, logger),
	}
}

// NewAccountRepositoryWithCostTracking creates a new account repository with cost tracking
func NewAccountRepositoryWithCostTracking(db core.DB, tableName string, domain string, logger *zap.Logger, costService *cost.TrackingService) *AccountRepository {
	// Use enhanced base repository with full service integration
	enhancedRepo := NewEnhancedBaseRepository[*models.User](db, tableName, logger, costService, "AccountRepository", "account")

	// Set up enhanced services
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &AccountRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		logger:                 logger,
		tableName:              tableName,
		domain:                 domain,
		actorRepo:              NewActorRepositoryWithCostTracking(db, tableName, logger, costService, domain),
		governanceRepo:         NewAgentGovernanceRepository(db, tableName, logger),
	}
}

// SetStatusRepository sets the status repository dependency for cross-repository operations
func (r *AccountRepository) SetStatusRepository(statusRepo interfaces.StatusRepository) {
	r.statusRepo = statusRepo
}

// SetBookmarkRepository wires the bookmark repository for dual-write operations
func (r *AccountRepository) SetBookmarkRepository(bookmarkRepo *BookmarkRepository) {
	r.bookmarkRepo = bookmarkRepo
}

// SetAgentGovernanceRepository overrides the governance repository dependency.
func (r *AccountRepository) SetAgentGovernanceRepository(governanceRepo *AgentGovernanceRepository) {
	r.governanceRepo = governanceRepo
}

// SetEncryptor overrides the encryptor used for actor private keys.
// When unset, the repository uses KMS based on runtime configuration.
func (r *AccountRepository) SetEncryptor(encryptor marshalers.Encryptor) {
	r.encryptor = encryptor
}

func (r *AccountRepository) getBookmarkRepository() *BookmarkRepository {
	if r.bookmarkRepo == nil {
		r.bookmarkRepo = NewBookmarkRepository(r.db, r.tableName, r.logger)
	}
	return r.bookmarkRepo
}

func (r *AccountRepository) getAgentGovernanceRepository() *AgentGovernanceRepository {
	if r.governanceRepo == nil {
		r.governanceRepo = NewAgentGovernanceRepository(r.db, r.tableName, r.logger)
	}
	return r.governanceRepo
}

// SetStorage is deprecated - storage dependency removed in Phase 5.6
func (r *AccountRepository) SetStorage(_ interface{}) {
	// No-op: storage dependency removed
}

// ===== Core Account Operations =====

// CreateAccount creates both User and Actor entities atomically using enhanced patterns
// This ensures consistency between authentication and federation data
func (r *AccountRepository) CreateAccount(ctx context.Context, account *storage.Account) error {
	return r.createAccount(ctx, account, false)
}

// CreateAccountIfNotExists creates both User and Actor entities while rejecting duplicate user creation.
// This is used by registration flows that must fail loudly under concurrent duplicate submits.
func (r *AccountRepository) CreateAccountIfNotExists(ctx context.Context, account *storage.Account) error {
	return r.createAccount(ctx, account, true)
}

func (r *AccountRepository) createAccount(ctx context.Context, account *storage.Account, createUserIfNotExists bool) error {
	if account == nil || account.User == nil {
		return common.ValidationError{Field: "account", Message: "account and user are required"}
	}

	account.User.Username = r.canonicalUsername(account.User.Username)
	user := account.User
	actor := account.Actor
	if actor != nil {
		actor = r.normalizeLocalActorIdentity(user.Username, actor)
		account.Actor = actor
	}

	// Create User model with enhanced validation and defaults
	userModel := &models.User{
		Username:           user.Username,
		Email:              user.Email,
		PasswordHash:       user.PasswordHash,
		DisplayName:        user.DisplayName,
		Note:               user.Note,
		Avatar:             user.Avatar,
		Header:             user.Header,
		URL:                user.URL,
		Locked:             user.Locked,
		Discoverable:       user.Discoverable,
		Fields:             user.Fields,
		AllowNSFW:          user.AllowNSFW,
		RequireNSFWWarning: user.RequireNSFWWarning,
		Metadata:           user.Metadata,
		IsAgent:            user.IsAgent,
		AgentType:          user.AgentType,
		AgentCapabilities:  user.AgentCapabilities,
		AgentVersion:       user.AgentVersion,
		AgentOwner:         user.AgentOwner,
		AgentCreatedBy:     user.AgentCreatedBy,
		AgentPublicKey:     user.AgentPublicKey,
		AgentKeyType:       user.AgentKeyType,
		Approved:           user.Approved,
		Suspended:          user.Suspended,
		Silenced:           user.Silenced,
		Role:               user.Role,
		Locale:             user.Locale,
		RecoveryMethods:    user.RecoveryMethods,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
	}

	// Set defaults using enhanced patterns
	r.setUserDefaults(userModel)

	// Use enhanced validation and creation with event emission
	if err := r.validateAndCreateUser(ctx, userModel, createUserIfNotExists); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return common.ConflictError{Resource: "user", Message: fmt.Sprintf("user %s already exists", user.Username)}
		}
		return err
	}

	// Create Actor if provided (with rollback on failure)
	if actor != nil {
		// Private key is REQUIRED - must be provided in account.PrivateKey
		privateKey := account.PrivateKey
		if privateKey == "" {
			return fmt.Errorf("private key is required for actor creation but was not provided")
		}

		if err := r.createActorWithRollback(ctx, actor, userModel, privateKey); err != nil {
			// Log the full error for debugging
			r.logger.Error("failed to create actor during account creation",
				zap.String("username", user.Username),
				zap.String("actor_id", actor.ID),
				zap.Error(err))
			return fmt.Errorf("failed to create actor: %w", err)
		}
	}

	// Maintain the O(1) instance counters (best-effort, never fails the
	// create — see instance_counts.go).
	bumpInstanceTotalUsers(ctx, r.db, r.logger, 1)
	if actor != nil {
		recordActorDomain(ctx, r.db, r.logger, domainFromActorID(actor.ID))
	}

	r.logger.Info("created account with enhanced validation",
		zap.String("username", user.Username),
		zap.Bool("with_actor", actor != nil),
		zap.Bool("validation_enabled", r.HasValidation()),
		zap.Bool("events_enabled", r.HasEvents()))

	return nil
}

func (r *AccountRepository) validateAndCreateUser(ctx context.Context, userModel *models.User, ifNotExists bool) error {
	if r.validator != nil {
		if err := r.validator.ValidateRequiredFields(ctx, userModel); err != nil {
			return pkgErrors.ValidationFailed("required fields", err.Error())
		}

		if err := r.validator.ValidateBusinessRules(ctx, userModel, "create"); err != nil {
			return pkgErrors.ValidationFailed("business rules", err.Error())
		}
	}

	if err := r.checkCreatePermissions(ctx, userModel); err != nil {
		return err
	}

	var err error
	if ifNotExists {
		err = r.CreateIfNotExists(ctx, userModel)
	} else {
		err = r.Create(ctx, userModel)
	}
	if err != nil {
		return err
	}

	if r.events != nil {
		event := NewEvent("entity.created", r.entityName, userModel.GetPK(), "create", userModel)
		event.Actor = r.getActorFromContext(ctx)
		_ = r.events.Emit(ctx, event)
	}

	return nil
}

// setUserDefaults sets default values for user model using enhanced patterns
func (r *AccountRepository) setUserDefaults(userModel *models.User) {
	if userModel.Role == "" {
		userModel.Role = "user"
	}
	if userModel.CreatedAt.IsZero() {
		userModel.CreatedAt = time.Now()
	}
	if userModel.UpdatedAt.IsZero() {
		userModel.UpdatedAt = userModel.CreatedAt
	}
}

// createActorWithRollback creates an actor with automatic user rollback on failure
// Private key is REQUIRED for ActivityPub signing
func (r *AccountRepository) createActorWithRollback(ctx context.Context, actor interface{}, userModel *models.User, privateKey string) error {
	if privateKey == "" {
		return common.ValidationError{Field: "privateKey", Message: "private key is required for actor creation"}
	}

	// Type assert the actor to the expected type
	actorPtr, ok := actor.(*activitypub.Actor)
	if !ok {
		return common.ValidationError{Field: "actor", Message: "invalid actor type"}
	}

	if err := r.createActor(ctx, actorPtr, privateKey); err != nil {
		// Enhanced rollback with proper validation and event emission
		pk := fmt.Sprintf("USER#%s", userModel.Username)
		if delErr := r.ValidateAndDelete(ctx, pk, models.SKMetadata); delErr != nil {
			r.logger.Warn("failed to rollback user creation after actor failure",
				zap.Error(delErr),
				zap.String("username", userModel.Username))
			rawDeleteErr := r.db.WithContext(ctx).Model(&models.User{}).
				Where("PK", "=", pk).
				Where("SK", "=", models.SKMetadata).
				Delete()
			if rawDeleteErr != nil {
				r.logger.Warn("failed raw rollback delete after actor failure",
					zap.Error(rawDeleteErr),
					zap.String("username", userModel.Username))
			}
		}
		return ErrorHandler.HandleCreateError(err, EntityActor, userModel.Username)
	}
	return nil
}

// CreateAccountLegacy ensures consistency between authentication and federation data.
func (r *AccountRepository) CreateAccountLegacy(ctx context.Context, username, email, passwordHash string, approved bool, actor *activitypub.Actor, _ string) error {
	// Convert to new interface
	account := &storage.Account{
		User: &storage.User{
			Username:     username,
			Email:        email,
			PasswordHash: passwordHash,
			Approved:     approved,
			Role:         "user",
		},
		Actor: actor,
	}
	return r.CreateAccount(ctx, account)
}

// GetAccount retrieves complete account information (User + Actor)
func (r *AccountRepository) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	canonicalUsername := r.canonicalUsername(username)
	// Get user data
	user, err := r.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}

	// Get actor data
	actor, err := r.GetActor(ctx, username)
	if err != nil && !isAccountNotFound(err) {
		return nil, ErrorHandler.HandleGetError(err, EntityActor, canonicalUsername)
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
	resolvedUsername, err := r.resolveStoredUsername(ctx, username)
	if err != nil {
		return err
	}
	username = resolvedUsername
	// Delete actor first (it's optional)
	if err := r.deleteActor(ctx, username); err != nil && !isAccountNotFound(err) {
		r.logger.Error("failed to delete actor",
			zap.String("username", username),
			zap.Error(err))
	}

	// Delete user using consistent key pattern
	pk := fmt.Sprintf("USER#%s", username)
	if err := r.Delete(ctx, pk, models.SKMetadata); err != nil {
		r.logger.Error("failed to delete user", zap.Error(err), zap.String("username", username))
		return ErrorHandler.HandleDeleteError(err, EntityUser, username)
	}

	// Maintain the O(1) instance TOTAL_USERS counter (best-effort, never
	// fails the delete — see instance_counts.go).
	bumpInstanceTotalUsers(ctx, r.db, r.logger, -1)

	r.logger.Info("deleted account", zap.String("username", username))
	return nil
}

// ===== User Operations (Authentication) =====

// GetUser retrieves user authentication data
func (r *AccountRepository) GetUser(ctx context.Context, username string) (*storage.User, error) {
	user, _, err := r.getCoreUser(ctx, username)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetAgentGovernanceState retrieves typed governance state for an agent account.
func (r *AccountRepository) GetAgentGovernanceState(ctx context.Context, username string) (*storage.AgentGovernanceState, error) {
	return r.getAgentGovernanceRepository().GetAgentGovernanceState(ctx, username)
}

// GetAgentGovernanceStatesByUsernames batch-loads typed governance state for agent accounts.
func (r *AccountRepository) GetAgentGovernanceStatesByUsernames(ctx context.Context, usernames []string) (map[string]*storage.AgentGovernanceState, error) {
	return r.getAgentGovernanceRepository().GetAgentGovernanceStatesByUsernames(ctx, usernames)
}

// PutAgentGovernanceState upserts typed governance state for an agent account.
func (r *AccountRepository) PutAgentGovernanceState(ctx context.Context, state *storage.AgentGovernanceState) error {
	return r.getAgentGovernanceRepository().PutAgentGovernanceState(ctx, state)
}

// DeleteAgentGovernanceState removes typed governance state for an agent account.
func (r *AccountRepository) DeleteAgentGovernanceState(ctx context.Context, username string) error {
	return r.getAgentGovernanceRepository().DeleteAgentGovernanceState(ctx, username)
}

// GetUserByEmail retrieves legacy users by their historical email lookup GSI.
//
// New email-based authentication remains disabled; this method exists only to
// preserve reads for deployed data that still carries gsi2PK=EMAIL#{email}.
func (r *AccountRepository) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if err := common.ValidateRequiredParam("email", normalizedEmail); err != nil {
		return nil, err
	}
	if r.db == nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityUser, normalizedEmail)
	}

	projection := &userEmailLookupProjection{Table: r.tableName}
	err := r.db.WithContext(ctx).Model(projection).
		Index("gsi2").
		Where("gsi2PK", "=", "EMAIL#"+normalizedEmail).
		Limit(1).
		First(projection)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityUser, normalizedEmail)
	}

	username := strings.TrimSpace(projection.Username)
	if username == "" {
		username = strings.TrimPrefix(strings.TrimSpace(projection.PK), "USER#")
	}
	if username == "" {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityUser, normalizedEmail)
	}

	return r.GetUser(ctx, username)
}

// UpdateUser updates user authentication data
func (r *AccountRepository) UpdateUser(ctx context.Context, username string, updates map[string]interface{}) error {
	user, resolvedUsername, err := r.getUserModel(ctx, username)
	if err != nil {
		return err
	}
	username = resolvedUsername
	pk := fmt.Sprintf("USER#%s", username)

	if user.Version == 0 {
		versionProjection := &userVersionProjection{Table: r.tableName}
		if err := r.db.WithContext(ctx).
			Model(versionProjection).
			Where("PK", "=", pk).
			Where("SK", "=", models.SKMetadata).
			ConsistentRead().
			First(versionProjection); err != nil {
			r.logger.Warn("failed to hydrate user version",
				zap.String("username", username),
				zap.Error(err))
		} else if versionValue, ok := versionProjection.versionValue(); ok {
			user.Version = versionValue
			if versionValue == 0 {
				r.logger.Warn("user version is zero; preserving zero for optimistic update",
					zap.String("username", username))
			}
		} else {
			r.logger.Warn("user version attribute missing; proceeding with zero-version optimistic update",
				zap.String("username", username))
		}
	}

	// Apply updates
	if err := r.applyUserUpdates(user, updates); err != nil {
		return err
	}

	r.logger.Info("updating user profile record",
		zap.String("username", username),
		zap.Int("current_version", user.Version))

	// Update using BaseRepository with error handling
	if err := r.Update(ctx, user); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityUser, username)
	}

	return nil
}

// ===== Actor Operations (ActivityPub) =====

// GetActor retrieves an actor by username
func (r *AccountRepository) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	actorModel, _, err := r.getActorModel(ctx, username)
	if err != nil {
		return nil, err
	}

	return r.normalizeLocalActorIdentity(actorModel.Username, actorModel.Actor), nil
}

// GetActorByUsername is an alias for GetActor (for compatibility)
func (r *AccountRepository) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	return r.GetActor(ctx, username)
}

func (r *AccountRepository) canonicalUsername(username string) string {
	return strings.ToLower(r.normalizeUsername(username))
}

func (r *AccountRepository) normalizeUsername(username string) string {
	trimmed := strings.TrimSpace(username)
	if trimmed == "" {
		return trimmed
	}

	trimmed = strings.TrimPrefix(trimmed, "acct:")
	trimmed = strings.TrimPrefix(trimmed, "@")
	trimmed = strings.TrimSuffix(trimmed, "/")

	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") {
		urlWithoutScheme := strings.TrimSuffix(trimmed, "/")
		parsed, _ := url.Parse(urlWithoutScheme)
		remoteDomain := ""
		if parsed != nil && strings.TrimSpace(parsed.Hostname()) != "" && !r.isLocalDomain(parsed.Hostname()) {
			remoteDomain = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		}
		if idx := strings.Index(urlWithoutScheme, "/users/"); idx != -1 && idx+7 < len(urlWithoutScheme) {
			trimmed = urlWithoutScheme[idx+7:]
		} else if idx := strings.LastIndex(urlWithoutScheme, "/@"); idx != -1 && idx+2 < len(urlWithoutScheme) {
			trimmed = urlWithoutScheme[idx+2:]
		} else {
			parts := strings.Split(urlWithoutScheme, "/")
			if len(parts) > 0 {
				trimmed = parts[len(parts)-1]
			}
		}
		trimmed = strings.TrimPrefix(trimmed, "@")
		if remoteDomain != "" && !strings.Contains(trimmed, "@") {
			trimmed = fmt.Sprintf("%s@%s", trimmed, remoteDomain)
		}
	}

	if at := strings.LastIndex(trimmed, "@"); at != -1 {
		localPart := trimmed[:at]
		domainPart := trimmed[at+1:]
		if r.isLocalDomain(domainPart) {
			trimmed = localPart
		}
	}

	return strings.TrimSpace(trimmed)
}

func (r *AccountRepository) usernameLookupCandidates(username string) []string {
	normalized := r.normalizeUsername(username)
	if normalized == "" {
		return nil
	}

	canonical := strings.ToLower(normalized)
	if canonical == normalized {
		return []string{canonical}
	}

	return []string{canonical, normalized}
}

func (r *AccountRepository) getUserModel(ctx context.Context, username string) (*models.User, string, error) {
	canonical := r.canonicalUsername(username)
	for _, candidate := range r.usernameLookupCandidates(username) {
		user := &models.User{}
		pk := fmt.Sprintf("USER#%s", candidate)
		err := r.Get(ctx, pk, models.SKMetadata, user)
		if err == nil {
			return user, candidate, nil
		}
		if !isAccountNotFound(err) {
			r.logger.Error("failed to get user", zap.Error(err), zap.String("username", canonical))
			return nil, "", ErrorHandler.HandleGetError(err, EntityUser, canonical)
		}
	}

	if user, err := r.lookupUserModelByCanonicalHandle(ctx, canonical); err == nil && user != nil {
		return user, strings.TrimSpace(user.Username), nil
	} else if err != nil && !isAccountNotFound(err) {
		r.logger.Error("failed to resolve user by canonical handle",
			zap.String("username", canonical),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleGetError(err, EntityUser, canonical)
	}

	return nil, "", ErrorHandler.HandleGetError(storage.ErrNotFound, EntityUser, canonical)
}

func (r *AccountRepository) getCoreUser(ctx context.Context, username string) (*storage.User, string, error) {
	canonical := r.canonicalUsername(username)
	for _, candidate := range r.usernameLookupCandidates(username) {
		projection, err := r.loadUserCoreProjectionByUsername(ctx, candidate)
		if err == nil {
			user := r.userCoreProjectionToStorageUser(projection)
			r.hydrateOptionalUserMetadata(ctx, candidate, user)
			return user, candidate, nil
		}
		if !isAccountNotFound(err) {
			r.logger.Error("failed to get core user", zap.Error(err), zap.String("username", canonical))
			return nil, "", ErrorHandler.HandleGetError(err, EntityUser, canonical)
		}
	}

	if projection, err := r.lookupUserCoreProjectionByCanonicalHandle(ctx, canonical); err == nil && projection != nil {
		resolvedUsername := strings.TrimSpace(projection.Username)
		user := r.userCoreProjectionToStorageUser(projection)
		r.hydrateOptionalUserMetadata(ctx, resolvedUsername, user)
		return user, resolvedUsername, nil
	} else if err != nil && !isAccountNotFound(err) {
		r.logger.Error("failed to resolve core user by canonical handle",
			zap.String("username", canonical),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleGetError(err, EntityUser, canonical)
	}

	return nil, "", ErrorHandler.HandleGetError(storage.ErrNotFound, EntityUser, canonical)
}

func (r *AccountRepository) getActorModel(ctx context.Context, username string) (*models.Actor, string, error) {
	canonical := r.canonicalUsername(username)
	attemptedUsernames := map[string]struct{}{}
	for _, candidate := range r.usernameLookupCandidates(username) {
		attemptedUsernames[candidate] = struct{}{}
		var actorModel models.Actor
		pk := fmt.Sprintf(models.KeyPatternActor, candidate)

		err := r.db.WithContext(ctx).Model(&actorModel).
			Where("PK", "=", pk).
			Where("SK", "=", models.SKProfile).
			First(&actorModel)
		if err == nil {
			return &actorModel, candidate, nil
		}
		if !isAccountNotFound(err) {
			return nil, "", ErrorHandler.HandleGetError(err, EntityActor, canonical)
		}
	}

	resolvedUsername, err := r.lookupStoredUsernameByCanonicalHandle(ctx, canonical)
	if err != nil && !isAccountNotFound(err) {
		return nil, "", ErrorHandler.HandleGetError(err, EntityActor, canonical)
	}
	if resolvedUsername != "" {
		if _, alreadyTried := attemptedUsernames[resolvedUsername]; alreadyTried {
			return nil, "", ErrorHandler.HandleGetError(storage.ErrNotFound, EntityActor, canonical)
		}

		var actorModel models.Actor
		pk := fmt.Sprintf(models.KeyPatternActor, resolvedUsername)
		err = r.db.WithContext(ctx).Model(&actorModel).
			Where("PK", "=", pk).
			Where("SK", "=", models.SKProfile).
			First(&actorModel)
		if err == nil {
			return &actorModel, resolvedUsername, nil
		}
		if err != nil && !isAccountNotFound(err) {
			return nil, "", ErrorHandler.HandleGetError(err, EntityActor, canonical)
		}
	}

	return nil, "", ErrorHandler.HandleGetError(storage.ErrNotFound, EntityActor, canonical)
}

func (r *AccountRepository) resolveStoredUsername(ctx context.Context, username string) (string, error) {
	_, resolvedUsername, err := r.getCoreUser(ctx, username)
	if err != nil {
		return "", err
	}
	return resolvedUsername, nil
}

func (r *AccountRepository) lookupStoredUsernameByCanonicalHandle(ctx context.Context, username string) (string, error) {
	projection, err := r.lookupUserCoreProjectionByCanonicalHandle(ctx, username)
	if err != nil {
		return "", err
	}
	if projection == nil {
		return "", nil
	}
	return strings.TrimSpace(projection.Username), nil
}

func (r *AccountRepository) lookupUserModelByCanonicalHandle(ctx context.Context, username string) (*models.User, error) {
	if r.db == nil {
		return nil, storage.ErrNotFound
	}

	normalizedUsername := strings.ToLower(strings.TrimSpace(username))
	if normalizedUsername == "" {
		return nil, storage.ErrNotFound
	}

	prefix := normalizedUsername
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}

	var userModel models.User
	err := r.db.WithContext(ctx).Model(&userModel).
		Index("gsi5").
		Where("gsi5PK", "=", fmt.Sprintf("USER_HANDLE_PREFIX#%s", prefix)).
		Where("gsi5SK", "=", normalizedUsername).
		First(&userModel)
	if err != nil {
		return nil, err
	}

	resolvedUsername := strings.TrimSpace(userModel.Username)
	if resolvedUsername == "" || !strings.EqualFold(resolvedUsername, normalizedUsername) {
		return nil, storage.ErrNotFound
	}
	if resolvedUsername == normalizedUsername {
		return nil, storage.ErrNotFound
	}

	return &userModel, nil
}

func (r *AccountRepository) loadUserCoreProjectionByUsername(ctx context.Context, username string) (*userCoreProjection, error) {
	if r.db == nil {
		return nil, storage.ErrNotFound
	}

	projection := &userCoreProjection{Table: r.tableName}
	pk := fmt.Sprintf("USER#%s", username)
	if err := r.db.WithContext(ctx).Model(projection).
		Where("PK", "=", pk).
		Where("SK", "=", models.SKMetadata).
		First(projection); err != nil {
		return nil, err
	}
	return projection, nil
}

func (r *AccountRepository) lookupUserCoreProjectionByCanonicalHandle(ctx context.Context, username string) (*userCoreProjection, error) {
	if r.db == nil {
		return nil, storage.ErrNotFound
	}

	normalizedUsername := strings.ToLower(strings.TrimSpace(username))
	if normalizedUsername == "" {
		return nil, storage.ErrNotFound
	}

	prefix := normalizedUsername
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}

	projection := &userCoreProjection{Table: r.tableName}
	err := r.db.WithContext(ctx).Model(projection).
		Index("gsi5").
		Where("gsi5PK", "=", fmt.Sprintf("USER_HANDLE_PREFIX#%s", prefix)).
		Where("gsi5SK", "=", normalizedUsername).
		Limit(1).
		First(projection)
	if err != nil {
		return nil, err
	}

	return projection, nil
}

func (r *AccountRepository) hydrateOptionalUserMetadata(ctx context.Context, username string, user *storage.User) {
	if user == nil || r.db == nil {
		return
	}

	metadataProjection := &userMetadataProjection{Table: r.tableName}
	pk := fmt.Sprintf("USER#%s", username)
	if err := r.db.WithContext(ctx).Model(metadataProjection).
		Where("PK", "=", pk).
		Where("SK", "=", models.SKMetadata).
		First(metadataProjection); err != nil {
		if !isAccountNotFound(err) {
			r.logger.Warn("optional user metadata decode failed; returning core account data",
				zap.String("username", username),
				zap.Error(err))
		}
		return
	}

	user.Metadata = cloneMetadata(metadataProjection.Metadata)
}

func (r *AccountRepository) isLocalDomain(domain string) bool {
	if domain == "" {
		return false
	}
	normalizedDomain := normalizeDomainValue(domain)
	if normalizedDomain == "lessersoul.ai" || strings.HasSuffix(normalizedDomain, ".lessersoul.ai") {
		return true
	}
	repoDomain := r.domain
	if strings.TrimSpace(repoDomain) == "" {
		repoDomain = config.Get().Domain
	}
	return normalizedDomain == normalizeDomainValue(repoDomain)
}

func normalizeDomainValue(domain string) string {
	normalized := strings.ToLower(strings.TrimSpace(domain))
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")
	normalized = strings.TrimSuffix(normalized, "/")
	return normalized
}

func (r *AccountRepository) ensureNumericIDMapping(ctx context.Context, _ string, username, actorID string) error {
	normalizedNumericID, canonicalUsername, canonicalActorID := normalizedNumericIDMappingValues(username, actorID)
	if canonicalUsername == "" {
		return nil
	}

	mapping := &models.NumericIDMapping{
		NumericID: normalizedNumericID,
		Username:  canonicalUsername,
		ActorID:   canonicalActorID,
	}
	if err := mapping.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "numeric ID mapping", normalizedNumericID)
	}

	err := r.db.WithContext(ctx).Model(mapping).Create()
	if err == nil {
		return nil
	}
	if !dynamormErrors.IsConditionFailed(err) {
		return ErrorHandler.HandleCreateError(err, "numeric ID mapping", normalizedNumericID)
	}

	var existing models.NumericIDMapping
	lookupErr := r.db.WithContext(ctx).Model(&models.NumericIDMapping{}).
		Where("PK", "=", "NUMERIC_ID#"+normalizedNumericID).
		Where("SK", "=", models.SKMetadata).
		First(&existing)
	if lookupErr == nil && strings.EqualFold(existing.Username, mapping.Username) {
		return nil
	}
	if lookupErr == nil {
		return ErrorHandler.HandleCreateError(err, "numeric ID mapping", normalizedNumericID)
	}
	return ErrorHandler.HandleCreateError(lookupErr, "numeric ID mapping", normalizedNumericID)
}

func (r *AccountRepository) deleteNumericIDMapping(ctx context.Context, numericID string) error {
	if strings.TrimSpace(numericID) == "" {
		return nil
	}

	err := r.db.WithContext(ctx).Model(&models.NumericIDMapping{}).
		Where("PK", "=", "NUMERIC_ID#"+numericID).
		Where("SK", "=", models.SKMetadata).
		Delete()
	if err == nil || dynamormErrors.IsNotFound(err) {
		return nil
	}

	return ErrorHandler.HandleDeleteError(err, "numeric ID mapping", numericID)
}

// createActor creates an actor (internal helper)
func (r *AccountRepository) createActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	if err := common.ValidateRequiredParam("preferred_username", actor.PreferredUsername); err != nil {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	username := r.canonicalUsername(actor.PreferredUsername)
	actor = r.normalizeLocalActorIdentity(username, actor)
	numericID := common.GenerateNumericID(username)

	r.logger.Info("creating actor",
		zap.String("username", username),
		zap.String("actor_id", actor.ID))

	// Encrypt private key - REQUIRED for security
	encryptor, err := r.getEncryptor()
	if err != nil {
		r.logger.Error("encryption not available for private key storage",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("encryption is required for private key storage but not configured: %w", err)
	}

	encrypted, err := encryptor.Encrypt([]byte(privateKey))
	if err != nil {
		r.logger.Error("failed to encrypt private key",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to encrypt private key: %w", err)
	}

	encryptedKey := base64.StdEncoding.EncodeToString(encrypted)

	// Create actor model
	actorModel := &models.Actor{
		Username:       username,
		Actor:          actor,
		PrivateKey:     encryptedKey,
		NumericID:      numericID,
		FollowerCount:  0,
		FollowingCount: 0,
		StatusCount:    0,
		CreatedAt:      time.Now().UTC(),
	}
	actorModel.UpdatedAt = actorModel.CreatedAt

	if err := actorModel.UpdateKeys(); err != nil {
		r.logger.Error("failed to update actor keys",
			zap.String("username", username),
			zap.Error(err))
		return err
	}

	if r.domain != "" {
		actorModel.GSI3PK = fmt.Sprintf("DOMAIN#%s", r.domain)
		actorModel.GSI3SK = username
	}

	// Create using DynamORM
	if err := r.db.WithContext(ctx).Model(actorModel).Create(); err != nil {
		r.logger.Error("DynamORM Create failed for actor",
			zap.String("username", username),
			zap.String("pk", actorModel.PK),
			zap.String("sk", actorModel.SK),
			zap.Error(err))
		if dynamormErrors.IsConditionFailed(err) {
			return common.ConflictError{Resource: "actor", Message: fmt.Sprintf("actor %s already exists", username)}
		}
		return ErrorHandler.HandleCreateError(err, EntityActor, username)
	}

	if err := r.ensureNumericIDMapping(ctx, numericID, username, actor.ID); err != nil {
		_ = r.db.WithContext(ctx).Model(&models.Actor{}).
			Where("PK", "=", actorModel.PK).
			Where("SK", "=", actorModel.SK).
			Delete()
		return err
	}

	r.logger.Info("actor created successfully",
		zap.String("username", username),
		zap.String("pk", actorModel.PK))

	return nil
}

// deleteActor deletes an actor (internal helper)
func (r *AccountRepository) deleteActor(ctx context.Context, username string) error {
	// Capture the actor's domain before the row is gone so the O(1)
	// TOTAL_DOMAINS counter can be released on delete (best-effort read).
	var actorModel models.Actor
	_ = r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", models.SKProfile).
		First(&actorModel)
	domain := ""
	if actorModel.Actor != nil {
		domain = domainFromActorID(actorModel.Actor.ID)
	}

	// Use key utilities for consistent key generation
	pk := fmt.Sprintf(models.KeyPatternActor, username)

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", pk).
		Where("SK", "=", models.SKProfile).
		Delete()

	// Use error utility for consistent error handling (delete doesn't fail on not found)
	if handled := ErrorHandler.HandleDeleteError(err, EntityActor, username); handled != nil {
		return handled
	}

	// Maintain the O(1) instance TOTAL_DOMAINS counter (best-effort, never
	// fails the delete — see instance_counts.go).
	if domain != "" {
		releaseActorDomain(ctx, r.db, r.logger, domain)
	}

	return r.deleteNumericIDMapping(ctx, common.GenerateNumericID(username))
}

// GetActorPrivateKey retrieves an actor's private key
func (r *AccountRepository) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	actorModel, _, err := r.getActorModel(ctx, username)
	if err != nil {
		return "", err
	}

	// Decrypt private key - REQUIRED
	encryptor, err := r.getEncryptor()
	if err != nil {
		r.logger.Error("encryption not available for private key retrieval",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("encryption is required for private key retrieval but not configured: %w", err)
	}

	privateKey := actorModel.PrivateKey
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		r.logger.Error("failed to decode private key (not base64)",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("private key is not in expected encrypted format: %w", err)
	}

	// Decrypt
	decrypted, err := encryptor.Decrypt(decoded)
	if err != nil {
		r.logger.Error("failed to decrypt private key",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("failed to decrypt private key: %w", err)
	}

	return string(decrypted), nil
}

// ===== Helper Methods =====

// modelToStorageUser converts User model to storage type
func (r *AccountRepository) modelToStorageUser(model *models.User) *storage.User {
	id := common.GenerateNumericID(model.Username)

	baseURL := strings.TrimSpace(r.domain)
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	user := &storage.User{
		ID:                 id,
		Username:           model.Username,
		Email:              "", // Email is forbidden - always empty
		PasswordHash:       model.PasswordHash,
		DisplayName:        model.DisplayName,
		Note:               model.Note,
		Avatar:             model.Avatar,
		Header:             model.Header,
		URL:                model.URL,
		Locked:             model.Locked,
		Discoverable:       model.Discoverable,
		Fields:             model.Fields,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
		Approved:           model.Approved,
		Suspended:          model.Suspended,
		Silenced:           model.Silenced,
		Role:               model.Role,
		Locale:             model.Locale,
		RecoveryMethods:    model.RecoveryMethods,
		AllowNSFW:          model.AllowNSFW,
		RequireNSFWWarning: model.RequireNSFWWarning,
		Metadata:           model.Metadata,
		IsAgent:            model.IsAgent,
		AgentType:          model.AgentType,
		AgentCapabilities:  model.AgentCapabilities,
		AgentVersion:       model.AgentVersion,
		AgentOwner:         model.AgentOwner,
		AgentCreatedBy:     model.AgentCreatedBy,
		AgentPublicKey:     model.AgentPublicKey,
		AgentKeyType:       model.AgentKeyType,
		Version:            model.Version,
	}

	if baseURL != "" && strings.TrimSpace(user.URL) == "" {
		user.URL = fmt.Sprintf("%s/@%s", baseURL, model.Username)
	}

	return user
}

func (r *AccountRepository) userCoreProjectionToStorageUser(projection *userCoreProjection) *storage.User {
	if projection == nil {
		return nil
	}

	return r.modelToStorageUser(&models.User{
		Username:           projection.Username,
		Email:              projection.Email,
		PasswordHash:       projection.PasswordHash,
		DisplayName:        projection.DisplayName,
		Note:               projection.Note,
		Avatar:             projection.Avatar,
		Header:             projection.Header,
		URL:                projection.URL,
		Locked:             projection.Locked,
		Discoverable:       projection.Discoverable,
		Fields:             cloneFields(projection.Fields),
		CreatedAt:          projection.CreatedAt,
		UpdatedAt:          projection.UpdatedAt,
		Approved:           projection.Approved,
		Suspended:          projection.Suspended,
		Silenced:           projection.Silenced,
		Role:               projection.Role,
		Locale:             projection.Locale,
		RecoveryMethods:    append([]string(nil), projection.RecoveryMethods...),
		AllowNSFW:          projection.AllowNSFW,
		RequireNSFWWarning: projection.RequireNSFWWarning,
		IsAgent:            projection.IsAgent,
		AgentType:          projection.AgentType,
		AgentCapabilities:  projection.AgentCapabilities,
		AgentVersion:       projection.AgentVersion,
		AgentOwner:         projection.AgentOwner,
		AgentCreatedBy:     projection.AgentCreatedBy,
		AgentPublicKey:     projection.AgentPublicKey,
		AgentKeyType:       projection.AgentKeyType,
		Version:            projection.Version,
	})
}

func (r *AccountRepository) normalizeLocalActorIdentity(username string, actor *activitypub.Actor) *activitypub.Actor {
	canonical := r.canonicalUsername(username)
	if canonical == "" && actor != nil {
		canonical = r.canonicalUsername(actor.PreferredUsername)
	}
	return normalizeLocalActorIdentityForStorage(canonical, r.actorBaseURL(), actor)
}

// UserUpdatePayload captures mutable account fields accepted from federation updates.
type UserUpdatePayload struct {
	Email              *string
	Note               *string
	Avatar             *string
	Header             *string
	URL                *string
	PasswordHash       *string
	DisplayName        *string
	Approved           *bool
	Suspended          *bool
	Silenced           *bool
	Role               *string
	Locked             *bool
	Discoverable       *bool
	Locale             *string
	AllowNSFW          *bool
	RequireNSFWWarning *bool
	RecoveryMethods    *[]string
	Fields             *[]map[string]string
	Metadata           map[string]interface{}
}

func decodeUserUpdatePayload(updates map[string]interface{}) (*UserUpdatePayload, error) {
	payload := &UserUpdatePayload{}
	var err error

	if payload.Email, err = stringPtrFromMap(updates, "email"); err != nil {
		return nil, err
	}
	if payload.Note, err = stringPtrFromMap(updates, "note"); err != nil {
		return nil, err
	}
	if payload.Avatar, err = stringPtrFromMap(updates, "avatar"); err != nil {
		return nil, err
	}
	if payload.Header, err = stringPtrFromMap(updates, "header"); err != nil {
		return nil, err
	}
	if payload.URL, err = stringPtrFromMap(updates, "url"); err != nil {
		return nil, err
	}
	if payload.PasswordHash, err = stringPtrFromMap(updates, "password_hash"); err != nil {
		return nil, err
	}
	if payload.DisplayName, err = stringPtrFromMap(updates, "display_name"); err != nil {
		return nil, err
	}
	if payload.Approved, err = boolPtrFromMap(updates, "approved"); err != nil {
		return nil, err
	}
	if payload.Suspended, err = boolPtrFromMap(updates, AccountStatusSuspended); err != nil {
		return nil, err
	}
	if payload.Silenced, err = boolPtrFromMap(updates, "silenced"); err != nil {
		return nil, err
	}
	if payload.Role, err = stringPtrFromMap(updates, "role"); err != nil {
		return nil, err
	}
	if payload.Locked, err = boolPtrFromMap(updates, "locked"); err != nil {
		return nil, err
	}
	if payload.Discoverable, err = boolPtrFromMap(updates, "discoverable"); err != nil {
		return nil, err
	}
	if payload.Locale, err = stringPtrFromMap(updates, "locale"); err != nil {
		return nil, err
	}
	if payload.AllowNSFW, err = boolPtrFromMap(updates, "allow_nsfw"); err != nil {
		return nil, err
	}
	if payload.RequireNSFWWarning, err = boolPtrFromMap(updates, "require_nsfw_warning"); err != nil {
		return nil, err
	}

	if payload.RecoveryMethods, err = stringSlicePtrFromValue(updates["recovery_methods"]); err != nil {
		return nil, err
	}
	if payload.Fields, err = fieldSlicePtrFromValue(updates["fields"]); err != nil {
		return nil, err
	}
	if payload.Metadata, err = metadataFromValue(updates["metadata"]); err != nil {
		return nil, err
	}

	return payload, nil
}

func stringPtrFromMap(m map[string]interface{}, key string) (*string, error) {
	if m == nil {
		return nil, nil
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil, nil
	}
	str, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("field %s must be a string", key)
	}
	value := str
	return &value, nil
}

func boolPtrFromMap(m map[string]interface{}, key string) (*bool, error) {
	if m == nil {
		return nil, nil
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return nil, fmt.Errorf("field %s must be a boolean", key)
	}
	value := b
	return &value, nil
}

func stringSlicePtrFromValue(value interface{}) (*[]string, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case []string:
		cloned := append([]string(nil), v...)
		return &cloned, nil
	case []interface{}:
		result := make([]string, 0, len(v))
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("field recovery_methods[%d] must be a string", i)
			}
			result = append(result, str)
		}
		return &result, nil
	default:
		return nil, fmt.Errorf("field recovery_methods must be an array of strings")
	}
}

func fieldSlicePtrFromValue(value interface{}) (*[]map[string]string, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case []map[string]string:
		cloned := cloneFields(v)
		return &cloned, nil
	case []interface{}:
		fields := make([]map[string]string, 0, len(v))
		for idx, item := range v {
			switch fieldMap := item.(type) {
			case map[string]string:
				fields = append(fields, cloneStringMap(fieldMap))
			case map[string]interface{}:
				normalized := make(map[string]string, len(fieldMap))
				for key, raw := range fieldMap {
					str, ok := raw.(string)
					if !ok {
						return nil, fmt.Errorf("field fields[%d][%s] must be a string", idx, key)
					}
					normalized[key] = str
				}
				fields = append(fields, normalized)
			default:
				return nil, fmt.Errorf("field fields[%d] must be an object", idx)
			}
		}
		return &fields, nil
	default:
		return nil, fmt.Errorf("field fields must be an array of objects")
	}
}

func metadataFromValue(value interface{}) (map[string]interface{}, error) {
	if value == nil {
		return nil, nil
	}
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("field metadata must be an object")
	}
	return cloneMetadata(m), nil
}

func cloneFields(fields []map[string]string) []map[string]string {
	if fields == nil {
		return nil
	}
	cloned := make([]map[string]string, 0, len(fields))
	for _, field := range fields {
		cloned = append(cloned, cloneStringMap(field))
	}
	return cloned
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneMetadata(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func (p *UserUpdatePayload) applyTo(user *models.User) {
	if p == nil {
		return
	}
	if p.Email != nil {
		user.Email = *p.Email
	}
	if p.Note != nil {
		user.Note = *p.Note
	}
	if p.Avatar != nil {
		user.Avatar = *p.Avatar
	}
	if p.Header != nil {
		user.Header = *p.Header
	}
	if p.URL != nil {
		user.URL = *p.URL
	}
	if p.PasswordHash != nil {
		user.PasswordHash = *p.PasswordHash
	}
	if p.DisplayName != nil {
		user.DisplayName = *p.DisplayName
	}
	if p.Approved != nil {
		user.Approved = *p.Approved
	}
	if p.Suspended != nil {
		user.Suspended = *p.Suspended
	}
	if p.Silenced != nil {
		user.Silenced = *p.Silenced
	}
	if p.Role != nil {
		user.Role = *p.Role
	}
	if p.Locked != nil {
		user.Locked = *p.Locked
	}
	if p.Discoverable != nil {
		user.Discoverable = *p.Discoverable
	}
	if p.Locale != nil {
		user.Locale = *p.Locale
	}
	if p.AllowNSFW != nil {
		user.AllowNSFW = *p.AllowNSFW
	}
	if p.RequireNSFWWarning != nil {
		user.RequireNSFWWarning = *p.RequireNSFWWarning
	}
	if p.RecoveryMethods != nil {
		user.RecoveryMethods = append([]string(nil), (*p.RecoveryMethods)...)
	}
	if p.Fields != nil {
		user.Fields = cloneFields(*p.Fields)
	}
	if p.Metadata != nil {
		user.Metadata = cloneMetadata(p.Metadata)
	}
}

// applyUserUpdates applies a map of updates to a user model
func (r *AccountRepository) applyUserUpdates(user *models.User, updates map[string]interface{}) error {
	payload, err := decodeUserUpdatePayload(updates)
	if err != nil {
		return err
	}

	payload.applyTo(user)

	user.UpdatedAt = time.Now()
	return nil
}

// getEncryptor returns an encryptor for actor private keys using KMS
func (r *AccountRepository) getEncryptor() (marshalers.Encryptor, error) {
	if r != nil && r.encryptor != nil {
		return r.encryptor, nil
	}

	cfg := config.Get()

	kmsKeyID := cfg.KMSKeyID
	if kmsKeyID == "" {
		return nil, errors.New("KMS_KEY_ID not configured")
	}

	return marshalers.NewKMSEncryptor(kmsKeyID)
}

// isAccountNotFound checks only stable item-absence sentinels and codes. Resource
// failures such as a missing table or index must propagate to callers.
func isAccountNotFound(err error) bool {
	var actorNotFound common.ActorNotFoundError
	return errors.Is(err, storage.ErrNotFound) ||
		dynamormErrors.IsNotFound(err) ||
		IsRepositoryNotFoundError(err) ||
		errors.As(err, &actorNotFound)
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
		if dynamormErrors.IsConditionFailed(err) {
			// Already pinned, not an error
			return nil
		}
		return ErrorHandler.HandleCreateError(err, "account pin", fmt.Sprintf("%s:%s", pin.Username, pin.PinnedActorID))
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
	if err := pin.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	err := r.db.WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", pin.PK).
		Where("SK", "=", pin.SK).
		Delete()

	if err != nil {
		return ErrorHandler.HandleDeleteError(err, "account pin", targetActorID)
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
		if dynamormErrors.IsNotFound(err) {
			return false, nil
		}
		return false, ErrorHandler.HandleQueryError(err, "account pin", "check pinned")
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
	if err := noteModel.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	err := r.db.WithContext(ctx).Model(&existing).
		Where("PK", "=", noteModel.PK).
		Where("SK", "=", noteModel.SK).
		First(&existing)

	if err != nil && !dynamormErrors.IsNotFound(err) {
		return ErrorHandler.HandleQueryError(err, "account note", "check existing")
	}

	if dynamormErrors.IsNotFound(err) {
		// Create new note
		if err := r.db.WithContext(ctx).Model(noteModel).Create(); err != nil {
			return ErrorHandler.HandleCreateError(err, "account note", note.TargetActorID)
		}
	} else {
		// Update existing note
		existing.Note = noteModel.Note
		existing.UpdatedAt = now
		if err := r.db.WithContext(ctx).Model(&existing).Update(); err != nil {
			return ErrorHandler.HandleUpdateError(err, "account note", note.TargetActorID)
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
		if dynamormErrors.IsNotFound(err) {
			return "", nil // Return empty string for not found, not an error
		}
		return "", ErrorHandler.HandleGetError(err, "preference", key)
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
		if dynamormErrors.IsNotFound(err) {
			return "", nil // Return empty string for not found, not an error
		}
		return "", ErrorHandler.HandleGetError(err, "follow request state", requesterID)
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
		if dynamormErrors.IsNotFound(err) {
			return false, nil
		}
		return false, ErrorHandler.HandleQueryError(err, "domain block", domain)
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
		if dynamormErrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "field verification", username)
		}
		return nil, ErrorHandler.HandleGetError(err, "field verification", username)
	}

	// Check if verification has expired
	if verification.IsExpired() {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "field verification", username)
	}

	// Convert to storage.ActorField
	return &storage.ActorField{
		Name:       verification.FieldName,
		Value:      verification.FieldValue,
		VerifiedAt: verification.VerifiedAt,
	}, nil
}

// ===== Account Moderation Operations =====

// ApproveAccount approves a pending user account
func (r *AccountRepository) ApproveAccount(ctx context.Context, username string) error {
	updates := map[string]interface{}{
		"approved": true,
	}
	return r.UpdateUser(ctx, username, updates)
}

// SuspendAccount suspends a user account with a reason
func (r *AccountRepository) SuspendAccount(ctx context.Context, username string, reason string) error {
	updates := map[string]interface{}{
		"suspended": true,
	}
	if reason != "" {
		updates["suspension_reason"] = reason
	}
	return r.UpdateUser(ctx, username, updates)
}

// UnsuspendAccount removes suspension from a user account
func (r *AccountRepository) UnsuspendAccount(ctx context.Context, username string) error {
	updates := map[string]interface{}{
		"suspended":         false,
		"suspension_reason": "",
	}
	return r.UpdateUser(ctx, username, updates)
}

// SilenceAccount silences a user account with a reason
func (r *AccountRepository) SilenceAccount(ctx context.Context, username string, reason string) error {
	updates := map[string]interface{}{
		"silenced": true,
	}
	if reason != "" {
		updates["silence_reason"] = reason
	}
	return r.UpdateUser(ctx, username, updates)
}

// UnsilenceAccount removes silence from a user account
func (r *AccountRepository) UnsilenceAccount(ctx context.Context, username string) error {
	updates := map[string]interface{}{
		"silenced":       false,
		"silence_reason": "",
	}
	return r.UpdateUser(ctx, username, updates)
}

// ===== Account Discovery Operations =====

// GetAccountByURL retrieves an account by its ActivityPub URL
func (r *AccountRepository) GetAccountByURL(ctx context.Context, actorURL string) (*storage.Account, error) {
	// First, try to find the actor by URL in the Actor model
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&actorModel).
		Where("ActivityPubID", "=", actorURL).
		First(&actorModel)

	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityUser, actorURL)
	}

	// Get the full account using the username
	return r.GetAccount(ctx, actorModel.Username)
}

// GetAccountByEmail retrieves an account by email address (updated to match interface)
func (r *AccountRepository) GetAccountByEmail(ctx context.Context, email string) (*storage.Account, error) {
	user, err := r.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil || strings.TrimSpace(user.Username) == "" {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityUser, strings.TrimSpace(email))
	}

	return r.GetAccount(ctx, user.Username)
}

// UpdateAccount updates account data (updated to match interface)
func (r *AccountRepository) UpdateAccount(ctx context.Context, account *storage.Account) error {
	if account == nil || account.User == nil {
		return ErrorHandler.HandleUpdateError(errors.New("invalid input"), EntityUser, "account")
	}

	username := strings.TrimSpace(account.User.Username)
	if username == "" {
		return ErrorHandler.HandleUpdateError(errors.New("username is required"), EntityUser, "account")
	}

	userModel, resolvedUsername, err := r.getUserModel(ctx, username)
	if err != nil {
		return err
	}
	username = resolvedUsername
	pk := fmt.Sprintf("USER#%s", username)

	// Ensure we have the current optimistic locking version
	versionExistsInDB := userModel.Version > 0 // If Version > 0 after Get(), it exists in DB
	if userModel.Version == 0 {
		versionProjection := &userVersionProjection{Table: r.tableName}
		if err := r.db.WithContext(ctx).
			Model(versionProjection).
			Where("PK", "=", pk).
			Where("SK", "=", models.SKMetadata).
			ConsistentRead().
			First(versionProjection); err != nil {
			r.logger.Warn("failed to hydrate user version for update account, defaulting to 1",
				zap.String("username", username),
				zap.Error(err))
			userModel.Version = 1     // Default to 1 if hydration fails
			versionExistsInDB = false // Version doesn't exist in DB
		} else if versionValue, ok := versionProjection.versionValue(); ok && versionValue > 0 {
			userModel.Version = versionValue
			versionExistsInDB = true // Version exists in DB
		} else {
			// Version attribute is missing or zero. Keep UpdateAccount's existing
			// bootstrap behavior: initialize it without a condition check.
			userModel.Version = 1
			versionExistsInDB = false // Version doesn't exist in DB
			r.logger.Warn("user version attribute missing or zero during update; will initialize to 1",
				zap.String("username", username))
		}
	}

	currentVersion := userModel.Version
	now := time.Now().UTC()

	// Apply incoming changes
	userModel.DisplayName = account.User.DisplayName
	userModel.Note = account.User.Note
	userModel.Avatar = account.User.Avatar
	userModel.Header = account.User.Header
	userModel.URL = strings.TrimSpace(account.User.URL)
	userModel.Locked = account.User.Locked
	userModel.Discoverable = account.User.Discoverable
	userModel.Fields = account.User.Fields
	userModel.Email = strings.TrimSpace(account.User.Email)
	userModel.Locale = strings.TrimSpace(account.User.Locale)
	userModel.IsAgent = account.User.IsAgent
	userModel.AgentType = strings.TrimSpace(account.User.AgentType)
	userModel.AgentCapabilities = account.User.AgentCapabilities
	userModel.AgentVersion = strings.TrimSpace(account.User.AgentVersion)
	userModel.AgentOwner = strings.TrimSpace(account.User.AgentOwner)
	userModel.AgentCreatedBy = strings.TrimSpace(account.User.AgentCreatedBy)
	userModel.AgentPublicKey = strings.TrimSpace(account.User.AgentPublicKey)
	userModel.AgentKeyType = strings.TrimSpace(account.User.AgentKeyType)
	userModel.Approved = account.User.Approved
	userModel.Suspended = account.User.Suspended
	userModel.Silenced = account.User.Silenced
	userModel.Role = strings.TrimSpace(account.User.Role)
	userModel.RecoveryMethods = account.User.RecoveryMethods
	userModel.AllowNSFW = account.User.AllowNSFW
	userModel.RequireNSFWWarning = account.User.RequireNSFWWarning
	if account.User.Metadata != nil {
		userModel.Metadata = account.User.Metadata
	}
	userModel.UpdatedAt = now

	if strings.TrimSpace(userModel.Username) == "" {
		userModel.Username = username
	}

	// Ensure primary and secondary keys reflect the latest state before persisting.
	// DynamORM does not automatically invoke UpdateKeys during Update(), so we do it explicitly.
	if err := userModel.UpdateKeys(); err != nil {
		r.logger.Error("failed to refresh user keys prior to update",
			zap.String("username", username),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityUser, username)
	}

	// Use UpdateBuilder for versioned updates to ensure optimistic locking works correctly
	updateBuilder := r.db.WithContext(ctx).Model(userModel).
		Where("PK", "=", userModel.PK).
		Where("SK", "=", userModel.SK).
		UpdateBuilder()

	// Set all fields that need updating
	updateBuilder.Set("DisplayName", userModel.DisplayName)
	updateBuilder.Set("Note", userModel.Note)
	updateBuilder.Set("Avatar", userModel.Avatar)
	updateBuilder.Set("Header", userModel.Header)
	updateBuilder.Set("URL", userModel.URL)
	updateBuilder.Set("Locked", userModel.Locked)
	updateBuilder.Set("Discoverable", userModel.Discoverable)
	updateBuilder.Set("Fields", userModel.Fields)
	updateBuilder.Set("Email", userModel.Email)
	updateBuilder.Set("Locale", userModel.Locale)
	updateBuilder.Set("IsAgent", userModel.IsAgent)
	updateBuilder.Set("AgentType", userModel.AgentType)
	updateBuilder.Set("AgentCapabilities", userModel.AgentCapabilities)
	updateBuilder.Set("AgentVersion", userModel.AgentVersion)
	updateBuilder.Set("AgentOwner", userModel.AgentOwner)
	updateBuilder.Set("AgentCreatedBy", userModel.AgentCreatedBy)
	updateBuilder.Set("AgentPublicKey", userModel.AgentPublicKey)
	updateBuilder.Set("AgentKeyType", userModel.AgentKeyType)
	updateBuilder.Set("Approved", userModel.Approved)
	updateBuilder.Set("Suspended", userModel.Suspended)
	updateBuilder.Set("Silenced", userModel.Silenced)
	updateBuilder.Set("Role", userModel.Role)
	updateBuilder.Set("RecoveryMethods", userModel.RecoveryMethods)
	updateBuilder.Set("AllowNSFW", userModel.AllowNSFW)
	updateBuilder.Set("RequireNSFWWarning", userModel.RequireNSFWWarning)
	if userModel.Metadata != nil {
		updateBuilder.Set("Metadata", userModel.Metadata)
	}
	updateBuilder.Set("UpdatedAt", now)

	// Handle version for optimistic locking
	// If version exists in DB (> 0), use condition check for optimistic locking
	// If version doesn't exist (was 0), set it without condition (first time initialization)
	if versionExistsInDB && currentVersion > 0 {
		// Version exists in DB - use optimistic locking
		updateBuilder.ConditionVersion(int64(currentVersion))
		updateBuilder.Set("Version", currentVersion+1)
	} else {
		// Version doesn't exist yet - set it without condition (first time initialization)
		updateBuilder.Set("Version", 1)
	}

	if err := updateBuilder.Execute(); err != nil {
		r.logger.Error("failed to update user profile record",
			zap.String("username", username),
			zap.Int("version", currentVersion),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityUser, username)
	}

	newVersion := currentVersion + 1
	if newVersion == 0 {
		newVersion = 1
	}
	account.User.UpdatedAt = now
	account.User.Version = newVersion
	userModel.Version = newVersion

	r.logger.Info("updated user profile record",
		zap.String("username", username),
		zap.Int("previous_version", currentVersion),
		zap.Int("new_version", newVersion))

	if err := r.updateAccountActorProfile(ctx, username, account); err != nil {
		return err
	}

	return nil
}

func (r *AccountRepository) updateAccountActorProfile(ctx context.Context, username string, account *storage.Account) error {
	if account.Actor == nil || r.actorRepo == nil {
		return nil
	}

	var existingActor *activitypub.Actor
	actorMissing := false
	if storedActor, err := r.actorRepo.GetActor(ctx, username); err == nil {
		existingActor = storedActor
	} else if isAccountNotFound(err) {
		actorMissing = true
	} else {
		r.logger.Error("failed to load existing actor profile record",
			zap.String("username", username),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityActor, username)
	}

	account.Actor = r.mergeActorDataForUpdate(username, existingActor, account.Actor)
	if actorMissing {
		return r.createRecoveredActorProfile(ctx, username, account.Actor)
	}

	if err := r.actorRepo.UpdateActor(ctx, account.Actor); err != nil {
		r.logger.Error("failed to update actor profile record",
			zap.String("username", username),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityActor, username)
	}

	return nil
}

func (r *AccountRepository) createRecoveredActorProfile(ctx context.Context, username string, actor *activitypub.Actor) error {
	if actor == nil || r.actorRepo == nil {
		return nil
	}

	username = r.canonicalUsername(username)
	actor = r.normalizeLocalActorIdentity(username, actor)
	now := time.Now().UTC()
	actorModel := &models.Actor{
		Username:       username,
		Actor:          actor,
		NumericID:      common.GenerateNumericID(username),
		CreatedAt:      now,
		UpdatedAt:      now,
		FollowerCount:  0,
		FollowingCount: 0,
		StatusCount:    0,
		Version:        1,
	}
	if domain := r.actorRepo.localActorDomain(); domain != "" {
		actorModel.GSI3PK = "DOMAIN#" + domain
		actorModel.GSI3SK = username
	}

	r.logger.Warn("actor profile record missing during account update; repairing public actor profile row without private key material",
		zap.String("username", username),
		zap.String("actor_id", actor.ID))

	if err := r.actorRepo.CreateIfNotExists(ctx, actorModel); err != nil {
		if isActorCreateConflict(err) {
			r.logger.Info("actor profile repair detected concurrent actor row; merging update into existing actor record",
				zap.String("username", username),
				zap.String("actor_id", actor.ID))
			return r.updateRecoveredActorAfterCreateConflict(ctx, username, actor)
		}

		r.logger.Error("failed to repair missing actor profile record",
			zap.String("username", username),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityActor, username)
	}

	r.logger.Info("repaired missing actor profile record during account update",
		zap.String("username", username),
		zap.String("actor_id", actor.ID))

	return nil
}

func (r *AccountRepository) updateRecoveredActorAfterCreateConflict(ctx context.Context, username string, incoming *activitypub.Actor) error {
	existing, err := r.getActorForRepairMerge(ctx, username)
	if err != nil {
		r.logger.Error("failed to load actor profile record after conditional repair conflict",
			zap.String("username", username),
			zap.Error(err))
		return ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	merged := r.mergeActorDataForUpdate(username, existing, incoming)
	if err := r.actorRepo.UpdateActor(ctx, merged); err != nil {
		r.logger.Error("failed to update actor profile record after conditional repair conflict",
			zap.String("username", username),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityActor, username)
	}

	return nil
}

func (r *AccountRepository) getActorForRepairMerge(ctx context.Context, username string) (*activitypub.Actor, error) {
	actorModel := &models.Actor{}
	err := r.actorRepo.db.WithContext(ctx).Model(actorModel).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", models.SKProfile).
		ConsistentRead().
		First(actorModel)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, err
	}

	return r.actorRepo.canonicalLocalActorFromModel(ctx, username, actorModel)
}

func isActorCreateConflict(err error) bool {
	return dynamormErrors.IsConditionFailed(err) || errors.Is(err, storage.ErrAlreadyExists)
}

func (r *AccountRepository) mergeActorDataForUpdate(username string, existing, incoming *activitypub.Actor) *activitypub.Actor {
	if incoming == nil {
		return existing
	}

	result := existing
	if result == nil {
		result = &activitypub.Actor{}
	}

	// Copy over identifying fields when provided
	if incoming.ID != "" {
		result.ID = incoming.ID
	}
	if incoming.URL != "" {
		result.URL = incoming.URL
	}
	if incoming.Inbox != "" {
		result.Inbox = incoming.Inbox
	}
	if incoming.Outbox != "" {
		result.Outbox = incoming.Outbox
	}
	if incoming.Followers != "" {
		result.Followers = incoming.Followers
	}
	if incoming.Following != "" {
		result.Following = incoming.Following
	}
	if incoming.Liked != "" {
		result.Liked = incoming.Liked
	}

	if incoming.PreferredUsername != "" {
		result.PreferredUsername = incoming.PreferredUsername
	} else if result.PreferredUsername == "" {
		result.PreferredUsername = username
	}

	if incoming.Name != "" {
		result.Name = incoming.Name
	}
	if incoming.Summary != "" {
		result.Summary = incoming.Summary
	}

	// Booleans should reflect the incoming state explicitly
	result.ManuallyApprovesFollowers = incoming.ManuallyApprovesFollowers
	result.Discoverable = incoming.Discoverable

	if incoming.Type != "" {
		result.Type = incoming.Type
	} else if result.Type == "" {
		result.Type = activitypub.PersonType
	}

	if len(incoming.Attachment) > 0 {
		result.Attachment = incoming.Attachment
	}

	if incoming.Icon != nil {
		if result.Icon == nil {
			result.Icon = &activitypub.Image{}
		}
		mergeActivityPubImage(result.Icon, incoming.Icon)
	}

	if incoming.Image != nil {
		if result.Image == nil {
			result.Image = &activitypub.Image{}
		}
		mergeActivityPubImage(result.Image, incoming.Image)
	}

	if incoming.PublicKey != nil {
		result.PublicKey = incoming.PublicKey
	}

	if incoming.Endpoints != nil {
		result.Endpoints = incoming.Endpoints
	}

	r.ensureActorIdentifiers(username, result, incoming)

	return result
}

func mergeActivityPubImage(dest *activitypub.Image, src *activitypub.Image) {
	if src == nil || dest == nil {
		return
	}

	if src.Type != "" {
		dest.Type = src.Type
	}
	if src.URL != "" {
		dest.URL = src.URL
	}
	if src.MediaType != "" {
		dest.MediaType = src.MediaType
	}
	if src.Width != 0 {
		dest.Width = src.Width
	}
	if src.Height != 0 {
		dest.Height = src.Height
	}
}

func (r *AccountRepository) ensureActorIdentifiers(username string, actor *activitypub.Actor, source *activitypub.Actor) {
	if actor == nil {
		return
	}

	resolvedUsername := activitypubutil.DerivePreferredUsername(actor, username)
	if resolvedUsername == "" {
		resolvedUsername = strings.TrimSpace(username)
	}

	baseURL := r.actorBaseURL()
	if baseURL == "" {
		baseURL = deriveActorBaseURL(actor, resolvedUsername)
		if baseURL == "" {
			baseURL = deriveActorBaseURL(source, resolvedUsername)
		}
	}

	sanitized := activitypubutil.BuildLocalActor(resolvedUsername, baseURL, nil, actor)
	if sanitized == nil {
		return
	}

	if source != nil {
		activitypubutil.MergeActorMetadata(sanitized, source)
	}

	*actor = *sanitized

	if actor.PublicKey != nil && actor.ID != "" {
		if actor.PublicKey.Owner == "" {
			actor.PublicKey.Owner = actor.ID
		}
		if actor.PublicKey.ID == "" {
			actor.PublicKey.ID = actor.ID + "#main-key"
		}
	}
}

func deriveActorBaseURL(actor *activitypub.Actor, username string) string {
	if actor == nil {
		return ""
	}
	if actor.URL != "" {
		if base := extractBaseURL(actor.URL, "/@"+username); base != "" {
			return base
		}
	}
	if actor.ID != "" {
		if base := extractBaseURL(actor.ID, "/users/"+username); base != "" {
			return base
		}
	}
	return ""
}

func (r *AccountRepository) actorBaseURL() string {
	base := strings.TrimSpace(r.domain)
	if base == "" {
		return ""
	}

	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}

	return strings.TrimSuffix(base, "/")
}

func extractBaseURL(value, marker string) string {
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, marker); idx > 0 {
		return value[:idx]
	}
	return strings.TrimSuffix(value, marker)
}

// SearchAccounts searches for accounts matching a query
func (r *AccountRepository) SearchAccounts(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return &interfaces.PaginatedResult[*storage.Account]{
			Items:      []*storage.Account{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	limit := opts.Limit
	if err := common.ValidateQueryLimit(limit, 100, "user search"); err != nil {
		limit = 20 // Default limit on validation error
	}

	searchLimit := limit
	if searchLimit <= 0 {
		searchLimit = 20
	}

	if len(normalizedQuery) == 1 {
		users, nextCursor, hasMore, err := r.searchAccountsByShortHandlePrefix(ctx, normalizedQuery, searchLimit, opts.Cursor)
		if err != nil {
			return nil, err
		}

		return &interfaces.PaginatedResult[*storage.Account]{
			Items:      r.usersToAccounts(users),
			NextCursor: nextCursor,
			HasMore:    hasMore,
			Total:      -1,
		}, nil
	}

	prefix := normalizedQuery
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	prefixKey := fmt.Sprintf("USER_HANDLE_PREFIX#%s", prefix)

	var users []models.User
	queryBuilder := r.db.WithContext(ctx).Model(&models.User{}).
		Index("gsi5").
		Where("gsi5PK", "=", prefixKey).
		Where("gsi5SK", "BEGINS_WITH", normalizedQuery).
		OrderBy("gsi5SK", "ASC").
		Limit(searchLimit + 1)

	if opts.Cursor != "" {
		pkCursor, skCursor, err := Utils.Pagination.DecodeCursor(opts.Cursor)
		if err != nil {
			r.logger.Warn("invalid search cursor provided",
				zap.String("cursor", opts.Cursor),
				zap.Error(err))
		} else if skCursor != "" {
			if pkCursor != "" && pkCursor != prefixKey {
				r.logger.Info("search cursor prefix mismatch - resetting to new prefix",
					zap.String("expected_prefix", prefixKey),
					zap.String("cursor_prefix", pkCursor))
			} else {
				queryBuilder = queryBuilder.Where("gsi5SK", ">", skCursor)
			}
		}
	}

	if err := queryBuilder.All(&users); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityUser, "search")
	}

	hasMore := len(users) > searchLimit
	if hasMore {
		users = users[:searchLimit]
	}

	var nextCursor string
	if hasMore && len(users) > 0 {
		lastUser := users[len(users)-1]
		nextCursor = Utils.Pagination.EncodeCursor(lastUser.GSI5PK, lastUser.GSI5SK)
	}

	return &interfaces.PaginatedResult[*storage.Account]{
		Items:      r.usersToAccounts(users),
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

const accountHandleSecondChars = "-0123456789_abcdefghijklmnopqrstuvwxyz"

func (r *AccountRepository) searchAccountsByShortHandlePrefix(ctx context.Context, normalizedQuery string, searchLimit int, cursor string) ([]models.User, string, bool, error) {
	prefixKeys := handlePrefixKeysForShortSearch(normalizedQuery)
	if len(prefixKeys) == 0 {
		return nil, "", false, nil
	}

	skCursor := r.decodeShortHandleSearchCursor(prefixKeys, cursor)
	seen := make(map[string]struct{})
	users := make([]models.User, 0, searchLimit+1)
	for _, prefixKey := range prefixKeys {
		partitionUsers, err := r.queryShortHandlePrefixPartition(ctx, prefixKey, normalizedQuery, searchLimit, skCursor)
		if err != nil {
			return nil, "", false, ErrorHandler.HandleQueryError(err, EntityUser, "search")
		}
		users = appendUniqueShortSearchUsers(users, seen, partitionUsers, normalizedQuery, skCursor)
	}

	sortUsersBySearchKey(users)
	users, nextCursor, hasMore := paginateSearchUsers(users, searchLimit)
	return users, nextCursor, hasMore, nil
}

func (r *AccountRepository) decodeShortHandleSearchCursor(prefixKeys []string, cursor string) string {
	if cursor == "" {
		return ""
	}

	pkCursor, skCursor, err := Utils.Pagination.DecodeCursor(cursor)
	if err != nil {
		r.logger.Warn("invalid search cursor provided",
			zap.String("cursor", cursor),
			zap.Error(err))
		return ""
	}
	if skCursor == "" {
		return ""
	}
	if pkCursor != "" && !containsString(prefixKeys, pkCursor) {
		r.logger.Info("search cursor prefix mismatch - resetting to new prefix",
			zap.String("expected_prefix", prefixKeys[0]),
			zap.String("cursor_prefix", pkCursor))
		return ""
	}
	return skCursor
}

func (r *AccountRepository) queryShortHandlePrefixPartition(ctx context.Context, prefixKey, normalizedQuery string, searchLimit int, skCursor string) ([]models.User, error) {
	var users []models.User
	queryBuilder := r.db.WithContext(ctx).Model(&models.User{}).
		Index("gsi5").
		Where("gsi5PK", "=", prefixKey).
		Where("gsi5SK", "BEGINS_WITH", normalizedQuery).
		OrderBy("gsi5SK", "ASC").
		Limit(searchLimit + 1)

	if skCursor != "" {
		queryBuilder = queryBuilder.Where("gsi5SK", ">", skCursor)
	}

	if err := queryBuilder.All(&users); err != nil {
		return nil, err
	}
	return users, nil
}

func appendUniqueShortSearchUsers(result []models.User, seen map[string]struct{}, users []models.User, normalizedQuery, skCursor string) []models.User {
	for _, user := range users {
		gsiSK := userSearchSortKey(user)
		if !strings.HasPrefix(gsiSK, normalizedQuery) {
			continue
		}
		if skCursor != "" && gsiSK <= skCursor {
			continue
		}
		identity := userSearchIdentity(user)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		result = append(result, user)
	}
	return result
}

func sortUsersBySearchKey(users []models.User) {
	sort.SliceStable(users, func(i, j int) bool {
		left := userSearchSortKey(users[i])
		right := userSearchSortKey(users[j])
		if left == right {
			return users[i].GSI5PK < users[j].GSI5PK
		}
		return left < right
	})
}

func paginateSearchUsers(users []models.User, searchLimit int) ([]models.User, string, bool) {
	hasMore := len(users) > searchLimit
	if hasMore {
		users = users[:searchLimit]
	}

	nextCursor := ""
	if hasMore && len(users) > 0 {
		lastUser := users[len(users)-1]
		nextCursor = Utils.Pagination.EncodeCursor(lastUser.GSI5PK, lastUser.GSI5SK)
	}

	return users, nextCursor, hasMore
}

func userSearchSortKey(user models.User) string {
	if user.GSI5SK != "" {
		return user.GSI5SK
	}
	return strings.ToLower(user.Username)
}

func userSearchIdentity(user models.User) string {
	if user.PK != "" {
		return user.PK
	}
	return strings.ToLower(user.Username)
}

func handlePrefixKeysForShortSearch(normalizedQuery string) []string {
	if len(normalizedQuery) != 1 {
		return nil
	}

	keys := make([]string, 0, len(accountHandleSecondChars)+1)
	keys = append(keys, fmt.Sprintf("USER_HANDLE_PREFIX#%s", normalizedQuery))
	for _, second := range accountHandleSecondChars {
		keys = append(keys, fmt.Sprintf("USER_HANDLE_PREFIX#%s%c", normalizedQuery, second))
	}
	return keys
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (r *AccountRepository) usersToAccounts(users []models.User) []*storage.Account {
	accounts := make([]*storage.Account, 0, len(users))
	for _, user := range users {
		account := &storage.Account{
			User: r.modelToStorageUser(&user),
		}
		accounts = append(accounts, account)
	}
	return accounts
}

// GetSuggestedAccounts retrieves suggested accounts to follow
func (r *AccountRepository) GetSuggestedAccounts(ctx context.Context, _ string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.AccountSuggestion], error) {
	// For now, return popular accounts as suggestions
	// In a full implementation, this would use recommendation algorithms

	var users []models.User
	queryBuilder := r.db.WithContext(ctx).Model(&models.User{}).
		Index(models.IndexGSI1).
		Where("gsi1PK", "=", "USERS")

	limit := opts.Limit
	if err := common.ValidateQueryLimit(limit, 50, "user suggestions"); err != nil {
		limit = 10 // Default limit for suggestions on validation error
	}
	queryBuilder = queryBuilder.Limit(limit + 1)

	if opts.Cursor != "" {
		_, sk, err := Utils.Pagination.DecodeCursor(opts.Cursor)
		if err == nil && sk != "" {
			queryBuilder = queryBuilder.Where("gsi1SK", ">", sk)
		}
	}

	err := queryBuilder.Scan(&users)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityUser, "suggestions")
	}

	// Convert to account suggestions
	var suggestions []*storage.AccountSuggestion
	for i, user := range users {
		if i >= limit {
			break
		}

		// Get actor data
		actor, _ := r.GetActor(ctx, user.Username)

		suggestion := &storage.AccountSuggestion{
			Actor:  actor,
			Reason: "popular",                // Simple reason for now
			Score:  1.0 - (float64(i) * 0.1), // Decreasing score
		}
		suggestions = append(suggestions, suggestion)
	}

	hasMore := len(users) > limit
	var nextCursor string
	if hasMore && len(suggestions) > 0 && len(users) > 0 {
		lastUser := users[len(suggestions)-1]
		nextCursor = Utils.Pagination.EncodeCursor(lastUser.PK, lastUser.GSI1SK)
	}

	return &interfaces.PaginatedResult[*storage.AccountSuggestion]{
		Items:      suggestions,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

// GetFeaturedAccounts retrieves featured accounts
func (r *AccountRepository) GetFeaturedAccounts(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	// Featured accounts are typically admins and moderators
	var users []models.User
	queryBuilder := r.db.WithContext(ctx).Model(&models.User{}).
		Index(models.IndexGSI3).
		Where("gsi3PK", "IN", []string{"ROLE#admin", "ROLE#moderator"})

	limit := opts.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	queryBuilder = queryBuilder.Limit(limit + 1)

	if opts.Cursor != "" {
		_, sk, err := Utils.Pagination.DecodeCursor(opts.Cursor)
		if err == nil && sk != "" {
			queryBuilder = queryBuilder.Where("gsi3SK", ">", sk)
		}
	}

	err := queryBuilder.Scan(&users)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityUser, "featured")
	}

	// Convert to accounts
	var accounts []*storage.Account
	for i, user := range users {
		if i >= limit {
			break
		}

		// Get actor data
		actor, _ := r.GetActor(ctx, user.Username)

		account := &storage.Account{
			User:  r.modelToStorageUser(&user),
			Actor: actor,
		}
		accounts = append(accounts, account)
	}

	hasMore := len(users) > limit
	var nextCursor string
	if hasMore && len(accounts) > 0 && len(users) > 0 {
		lastUser := users[len(accounts)-1]
		nextCursor = Utils.Pagination.EncodeCursor(lastUser.PK, lastUser.GSI3SK)
	}

	return &interfaces.PaginatedResult[*storage.Account]{
		Items:      accounts,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

// ===== Account Preferences Operations =====

// UpdateAccountPreferences updates user preferences
func (r *AccountRepository) UpdateAccountPreferences(ctx context.Context, username string, preferences map[string]interface{}) error {
	// Store each preference as a separate record
	for key, value := range preferences {
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case bool:
			if v {
				valueStr = "true"
			} else {
				valueStr = "false"
			}
		default:
			valueStr = fmt.Sprintf("%v", v)
		}

		pref := &models.UserPreference{
			Username: username,
			Key:      key,
			Value:    valueStr,
		}

		// Try to get existing preference first
		var existing models.UserPreference
		pref.UpdateKeys()
		err := r.db.WithContext(ctx).Model(&existing).
			Where("PK", "=", pref.PK).
			Where("SK", "=", pref.SK).
			First(&existing)

		if err != nil && !dynamormErrors.IsNotFound(err) {
			return ErrorHandler.HandleQueryError(err, "preference", "check existing")
		}

		if dynamormErrors.IsNotFound(err) {
			// Create new preference
			if err := r.db.WithContext(ctx).Model(pref).Create(); err != nil {
				return ErrorHandler.HandleCreateError(err, "preference", key)
			}
		} else {
			// Update existing preference
			existing.Value = valueStr
			existing.UpdatedAt = time.Now()
			if err := r.db.WithContext(ctx).Model(&existing).Update(); err != nil {
				return ErrorHandler.HandleUpdateError(err, "preference", key)
			}
		}
	}

	return nil
}

// GetAccountPreferences retrieves all preferences for an account
func (r *AccountRepository) GetAccountPreferences(ctx context.Context, username string) (map[string]interface{}, error) {
	var preferences []models.UserPreference
	err := r.db.WithContext(ctx).Model(&models.UserPreference{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "begins_with", "PREFERENCE#").
		Scan(&preferences)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityUser, "preferences")
	}

	result := make(map[string]interface{})
	for _, pref := range preferences {
		// Try to parse boolean values
		switch pref.Value {
		case "true":
			result[pref.Key] = true
		case "false":
			result[pref.Key] = false
		default:
			result[pref.Key] = pref.Value
		}
	}

	return result, nil
}

// ===== Account Features Operations =====

// UpdateAccountFeatures updates account feature flags
func (r *AccountRepository) UpdateAccountFeatures(ctx context.Context, username string, features map[string]bool) error {
	// Store features as preferences with special prefix
	preferences := make(map[string]interface{})
	for key, value := range features {
		preferences["feature_"+key] = value
	}
	return r.UpdateAccountPreferences(ctx, username, preferences)
}

// GetAccountFeatures retrieves account feature flags
func (r *AccountRepository) GetAccountFeatures(ctx context.Context, username string) (map[string]bool, error) {
	allPrefs, err := r.GetAccountPreferences(ctx, username)
	if err != nil {
		return nil, err
	}

	features := make(map[string]bool)
	for key, value := range allPrefs {
		if strings.HasPrefix(key, "feature_") {
			featureKey := strings.TrimPrefix(key, "feature_")
			if boolValue, ok := value.(bool); ok {
				features[featureKey] = boolValue
			}
		}
	}

	return features, nil
}

// ===== Authentication Operations =====

// ValidateCredentials validates username and password credentials
func (r *AccountRepository) ValidateCredentials(ctx context.Context, username, password string) (*storage.Account, error) {
	// Get user
	user, err := r.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}

	// Check if user has a password hash
	if err := common.ValidateRequiredParam("password_hash", user.PasswordHash); err != nil {
		return nil, ErrorHandler.HandleGetError(errors.New("password auth unavailable"), EntityUser, username)
	}

	// Verify password using bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrorHandler.HandleGetError(errors.New("authentication failed"), EntityUser, username)
	}

	// Get the full account
	return r.GetAccount(ctx, username)
}

// UpdatePassword updates a user's password hash
func (r *AccountRepository) UpdatePassword(ctx context.Context, username, newPasswordHash string) error {
	updates := map[string]interface{}{
		"password_hash": newPasswordHash,
	}
	return r.UpdateUser(ctx, username, updates)
}

// CreatePasswordReset creates a password reset request
func (r *AccountRepository) CreatePasswordReset(ctx context.Context, reset *storage.PasswordReset) error {
	resetModel := &models.PasswordReset{
		Username:  reset.Username,
		Token:     reset.Token,
		Email:     reset.Email,
		CreatedAt: reset.CreatedAt,
		ExpiresAt: reset.ExpiresAt,
		Used:      reset.Used,
	}

	if resetModel.CreatedAt.IsZero() {
		resetModel.CreatedAt = time.Now()
	}
	if resetModel.ExpiresAt.IsZero() {
		resetModel.ExpiresAt = resetModel.CreatedAt.Add(24 * time.Hour) // 24 hour expiry
	}

	if err := r.db.WithContext(ctx).Model(resetModel).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityPasswordReset, reset.Token)
	}

	r.logger.Info("created password reset",
		zap.String("username", reset.Username),
		zap.String("email", reset.Email))

	return nil
}

// GetPasswordReset retrieves a password reset by token
func (r *AccountRepository) GetPasswordReset(ctx context.Context, token string) (*storage.PasswordReset, error) {
	var resetModel models.PasswordReset
	err := r.db.WithContext(ctx).Model(&resetModel).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("RESET_TOKEN#%s", token)).
		First(&resetModel)

	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityPasswordReset, token)
	}

	// Check if expired
	if time.Now().After(resetModel.ExpiresAt) {
		return nil, ErrorHandler.HandleGetError(errors.New("token expired"), EntityPasswordReset, token)
	}

	// Check if already used
	if resetModel.Used {
		return nil, ErrorHandler.HandleGetError(errors.New("token already used"), EntityPasswordReset, token)
	}

	return &storage.PasswordReset{
		Username:  resetModel.Username,
		Token:     resetModel.Token,
		Email:     resetModel.Email,
		CreatedAt: resetModel.CreatedAt,
		ExpiresAt: resetModel.ExpiresAt,
		Used:      resetModel.Used,
	}, nil
}

// UsePasswordReset marks a password reset token as used
func (r *AccountRepository) UsePasswordReset(ctx context.Context, token string) error {
	// Get the reset record first
	resetModel := &models.PasswordReset{}
	resetModel.Token = token

	err := r.db.WithContext(ctx).Model(resetModel).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("RESET_TOKEN#%s", token)).
		First(resetModel)

	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityPasswordReset, token)
	}

	// Mark as used
	resetModel.Used = true
	resetModel.UsedAt = time.Now()

	if err := r.db.WithContext(ctx).Model(resetModel).Update(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityPasswordReset, token)
	}

	return nil
}

// ===== Activity Tracking Operations =====

// RecordLogin records a login attempt
func (r *AccountRepository) RecordLogin(ctx context.Context, attempt *storage.LoginAttempt) error {
	loginModel := &models.UserLogin{
		Username:  attempt.Username,
		Timestamp: attempt.Timestamp,
		Success:   attempt.Success,
		IPAddress: attempt.IPAddress,
		UserAgent: attempt.UserAgent,
	}

	if loginModel.Timestamp.IsZero() {
		loginModel.Timestamp = time.Now()
	}

	if err := r.db.WithContext(ctx).Model(loginModel).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, "login attempt", attempt.Username)
	}

	return nil
}

// GetLoginHistory retrieves login history for a user
func (r *AccountRepository) GetLoginHistory(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.LoginAttempt], error) {
	var logins []models.UserLogin
	queryBuilder := r.db.WithContext(ctx).Model(&models.UserLogin{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "begins_with", "LOGIN#")

	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	queryBuilder = queryBuilder.Limit(limit + 1)

	if opts.Cursor != "" {
		_, sk, err := Utils.Pagination.DecodeCursor(opts.Cursor)
		if err == nil && sk != "" {
			queryBuilder = queryBuilder.Where("SK", "<", sk) // Reverse order for recent first
		}
	}

	// Sort by SK descending to get most recent first
	queryBuilder = queryBuilder.OrderBy("SK", "DESC")

	err := queryBuilder.Scan(&logins)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityUser, "login_history")
	}

	// Convert to storage type
	var attempts []*storage.LoginAttempt
	for i, login := range logins {
		if i >= limit {
			break
		}

		attempts = append(attempts, &storage.LoginAttempt{
			Username:  login.Username,
			Timestamp: login.Timestamp,
			Success:   login.Success,
			IPAddress: login.IPAddress,
			UserAgent: login.UserAgent,
		})
	}

	hasMore := len(logins) > limit
	var nextCursor string
	if hasMore && len(attempts) > 0 {
		lastLogin := logins[len(attempts)-1]
		nextCursor = Utils.Pagination.EncodeCursor(lastLogin.PK, lastLogin.SK)
	}

	return &interfaces.PaginatedResult[*storage.LoginAttempt]{
		Items:      attempts,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

// UpdateLastActivity updates the last activity timestamp for a user
func (r *AccountRepository) UpdateLastActivity(ctx context.Context, username string, activity time.Time) error {
	updates := map[string]interface{}{
		"last_activity": activity,
	}
	return r.UpdateUser(ctx, username, updates)
}

// ===== Batch Operations =====

// GetAccountsByUsernames retrieves multiple accounts by their usernames
func (r *AccountRepository) GetAccountsByUsernames(ctx context.Context, usernames []string) ([]*storage.Account, error) {
	if err := common.ValidateSliceNotEmpty("usernames", usernames); err != nil {
		return []*storage.Account{}, nil
	}

	var accounts []*storage.Account

	// Fetch accounts one by one (batch get would be more efficient but requires more complex implementation)
	for _, username := range usernames {
		account, err := r.GetAccount(ctx, username)
		if err != nil {
			// Skip accounts that don't exist rather than failing entirely
			if !isAccountNotFound(err) {
				r.logger.Warn("failed to get account in batch",
					zap.String("username", username),
					zap.Error(err))
			}
			continue
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

// GetAccountsCount retrieves the total number of accounts
func (r *AccountRepository) GetAccountsCount(ctx context.Context) (int64, error) {
	// Count users using GSI1 (user listing index)
	var users []models.User
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index(models.IndexGSI1).
		Where("gsi1PK", "=", "USERS").
		Scan(&users)

	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, EntityUser, "count")
	}

	return int64(len(users)), nil
}

// Note: This is the core file. Additional methods will be organized into:
// - account_repository_auth.go (authentication methods)
// - account_repository_social.go (follows, blocks, mutes)
// - account_repository_timeline.go (timeline operations)
// - account_repository_search.go (search and discovery)
