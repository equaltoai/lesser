package repositories

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	neturl "net/url"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/marshalers"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ActorRepositoryDeps interface for dependencies - implemented by the storage adapter
type ActorRepositoryDeps interface {
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetPreference(ctx context.Context, username, key string) (any, error)
	SetPreference(ctx context.Context, username, key string, value any) error
}

// ActorRepository implements actor operations using enhanced DynamORM patterns
type ActorRepository struct {
	*EnhancedBaseRepository[*models.Actor]
	deps ActorRepositoryDeps
}

// NewActorRepository creates a new actor repository with enhanced functionality
func NewActorRepository(db core.DB, tableName string, logger *zap.Logger) *ActorRepository {
	// Create enhanced repository optimized for actor operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Actor](db, tableName, logger, nil, "ActorRepository", "actor")

	// Set up enhanced services for actor operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Actors frequently accessed for federation
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for federation events

	return &ActorRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// NewActorRepositoryWithCostTracking creates a new actor repository with cost tracking
func NewActorRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *ActorRepository {
	// Create enhanced repository with cost tracking
	enhancedRepo := NewEnhancedBaseRepository[*models.Actor](db, tableName, logger, costService, "ActorRepository", "actor")

	// Set up enhanced services for actor operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Actors frequently accessed for federation
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for federation events

	return &ActorRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// SetDependencies sets the dependencies for cross-repository operations
func (r *ActorRepository) SetDependencies(deps ActorRepositoryDeps) {
	r.deps = deps
}

// CreateActor creates a new actor in DynamoDB
func (r *ActorRepository) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	// Validate actor entity using centralized validation
	if err := common.ValidateActorEntity(actor.ID, actor.PreferredUsername); err != nil {
		return err
	}

	username := actor.PreferredUsername
	numericID := common.GenerateNumericID(username)

	// Encrypt private key - REQUIRED for security
	encryptor, err := getEncryptor()
	if err != nil {
		common.WithContext(ctx).Error("encryption not available for private key storage",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("encryption is required for private key storage but not configured: %w", err)
	}

	encrypted, err := encryptor.Encrypt([]byte(privateKey))
	if err != nil {
		common.WithContext(ctx).Error("failed to encrypt private key",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to encrypt private key: %w", err)
	}

	encryptedKey := base64.StdEncoding.EncodeToString(encrypted)

	// Create the DynamORM model
	actorModel := &models.Actor{
		Username:       username,
		Actor:          actor,
		PrivateKey:     encryptedKey,
		NumericID:      numericID,
		FollowerCount:  0,
		FollowingCount: 0,
		StatusCount:    0,
	}

	// Set domain for GSI3 if available
	domain := lesserconfig.Get().Domain
	if domain != "" {
		actorModel.GSI3PK = "DOMAIN#" + domain
		actorModel.GSI3SK = username
	}

	// Create the actor using BaseRepository
	err = r.Create(ctx, actorModel)
	if err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return common.ConflictError{
				Resource: "actor",
				Message:  fmt.Sprintf("actor %s already exists", username),
			}
		}
		return ErrorHandler.HandleCreateError(err, "actor", username)
	}

	return nil
}

// GetActor retrieves an actor by username
func (r *ActorRepository) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	actorModel := &models.Actor{}

	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	return actorModel.Actor, nil
}

// GetActorWithMetadata retrieves an actor with metadata
func (r *ActorRepository) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	actorModel := &models.Actor{}

	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, nil, common.ActorNotFoundError{Username: username}
		}
		return nil, nil, ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	metadata := &storage.ActorMetadata{
		CreatedAt:    actorModel.CreatedAt,
		UpdatedAt:    actorModel.UpdatedAt,
		LastStatusAt: actorModel.LastStatusAt,
		Fields:       convertActorFields(actorModel.Fields),
	}

	return actorModel.Actor, metadata, nil
}

// GetActorByNumericID retrieves an actor by numeric ID
func (r *ActorRepository) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	// First get the numeric ID mapping using BaseRepository pattern
	// Note: This uses a different model type, so we use direct query
	var mapping models.NumericIDMapping
	err := r.db.WithContext(ctx).Model(&models.NumericIDMapping{}).
		Where("PK", "=", "NUMERIC_ID#"+numericID).
		Where("SK", "=", "METADATA").
		First(&mapping)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, EntityActor, numericID)
		}
		return nil, ErrorHandler.HandleGetError(err, "numeric ID mapping", numericID)
	}

	// Now get the actual actor using the username
	return r.GetActor(ctx, mapping.Username)
}

// GetActorPrivateKey retrieves an actor's private key
func (r *ActorRepository) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	actorModel := &models.Actor{}

	// Use direct query for Select functionality (BaseRepository doesn't support Select)
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Select("PrivateKey").
		First(actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return "", common.ActorNotFoundError{Username: username}
		}
		return "", ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Decrypt private key - REQUIRED
	encryptor, err := getEncryptor()
	if err != nil {
		common.WithContext(ctx).Error("encryption not available for private key retrieval",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("encryption is required for private key retrieval but not configured: %w", err)
	}

	privateKey := actorModel.PrivateKey
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		common.WithContext(ctx).Error("failed to decode private key (not base64)",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("private key is not in expected encrypted format: %w", err)
	}

	// Decrypt
	decrypted, err := encryptor.Decrypt(decoded)
	if err != nil {
		common.WithContext(ctx).Error("failed to decrypt private key",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("failed to decrypt private key: %w", err)
	}

	return string(decrypted), nil
}

// UpdateActor updates an existing actor
func (r *ActorRepository) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	// Validate actor entity using centralized validation
	if err := common.ValidateActorEntity(actor.ID, actor.PreferredUsername); err != nil {
		return err
	}

	username := actor.PreferredUsername

	// Get existing actor first
	actorModel := &models.Actor{}
	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	if actorModel.Version == 0 {
		seedBuilder := r.db.WithContext(ctx).
			Model(&models.Actor{}).
			Where("PK", "=", actorModel.PK).
			Where("SK", "=", actorModel.SK).
			UpdateBuilder()

		if err := seedBuilder.SetIfNotExists("Version", nil, 0).Execute(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "conditionalcheckfailed") {
			r.logger.Warn("failed to seed actor version attribute",
				zap.String("username", username),
				zap.Error(err))
		}
	}

	now := time.Now()
	actorModel.Actor = actor
	actorModel.UpdatedAt = now

	updateBuilder := r.db.WithContext(ctx).
		Model(&models.Actor{}).
		Where("PK", "=", actorModel.PK).
		Where("SK", "=", actorModel.SK).
		UpdateBuilder()

	actorBytes, err := json.Marshal(actor)
	if err != nil {
		return fmt.Errorf("failed to marshal actor: %w", err)
	}

	var actorMap map[string]interface{}
	if err := json.Unmarshal(actorBytes, &actorMap); err != nil {
		return fmt.Errorf("failed to unmarshal actor into map: %w", err)
	}

	updateBuilder.Set("Actor", actorMap)
	updateBuilder.Set("UpdatedAt", now)

	gsi1PK, gsi1SK := buildActorGSI1Keys(username)
	updateBuilder.Set("gsi1PK", gsi1PK)
	updateBuilder.Set("gsi1SK", gsi1SK)

	displayName := strings.TrimSpace(actor.Name)
	if displayName != "" {
		gsi2PK, gsi2SK := buildActorGSI2Keys(displayName, username)
		updateBuilder.Set("gsi2PK", gsi2PK)
		updateBuilder.Set("gsi2SK", gsi2SK)
	} else {
		updateBuilder.Remove("gsi2PK")
		updateBuilder.Remove("gsi2SK")
	}

	domainName := r.resolveActorDomain(actor)
	if domainName != "" {
		updateBuilder.Set("gsi3PK", fmt.Sprintf("DOMAIN#%s", domainName))
		updateBuilder.Set("gsi3SK", username)
	} else {
		updateBuilder.Remove("gsi3PK")
		updateBuilder.Remove("gsi3SK")
	}

	gsi4PK, gsi4SK := buildActorGSI4Keys(actorModel.FollowerCount, username)
	updateBuilder.Set("gsi4PK", gsi4PK)
	updateBuilder.Set("gsi4SK", gsi4SK)

	gsi5PK, gsi5SK := buildActorGSI5Keys(now, username)
	updateBuilder.Set("gsi5PK", gsi5PK)
	updateBuilder.Set("gsi5SK", gsi5SK)

	if actorModel.Version > 0 {
		updateBuilder.ConditionVersion(int64(actorModel.Version))
		updateBuilder.Set("Version", actorModel.Version+1)
	} else {
		updateBuilder.Set("Version", 1)
	}

	if err := updateBuilder.Execute(); err != nil {
		r.logger.Error("failed to update actor record",
			zap.String("username", username),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityActor, username)
	}

	actorModel.Version++
	return nil
}

// UpdateActorLastStatusTime updates the last status timestamp
func (r *ActorRepository) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	// Get existing actor first
	actorModel := &models.Actor{}
	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Update last status time
	now := time.Now()
	actorModel.LastStatusAt = &now

	// Update using BaseRepository
	err = r.Update(ctx, actorModel)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityActor, username)
	}

	return nil
}

// SetActorFields updates the profile fields for an actor
func (r *ActorRepository) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	// Get existing actor first
	actorModel := &models.Actor{}
	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Convert and update fields
	actorModel.Fields = convertStorageActorFields(fields)

	// Update using BaseRepository
	err = r.Update(ctx, actorModel)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityActor, username)
	}

	return nil
}

// DeleteActor deletes an actor
func (r *ActorRepository) DeleteActor(ctx context.Context, username string) error {
	// Delete the actor using BaseRepository
	err := r.Delete(ctx, "ACTOR#"+username, "PROFILE")
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return ErrorHandler.HandleDeleteError(err, EntityActor, username)
	}

	return nil
}

// SearchAccounts searches for actors by username or display name
func (r *ActorRepository) SearchAccounts(ctx context.Context, query string, limit int, _ bool, _ int) ([]*activitypub.Actor, error) {
	// Handle empty query gracefully
	if query == "" {
		return []*activitypub.Actor{}, nil
	}

	if err := common.ValidateRequiredParam("query", query); err != nil {
		// For empty query, return recent active discoverable actors
		return r.getRecentActiveActors(ctx, limit)
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	var actors []models.Actor

	// Try username search first using GSI1
	if len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("username-search-index").
			Where("gsi1PK", "=", "USERNAME_SEARCH#"+prefix).
			Filter("gsi1SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&actors)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, EntityActor, "search by username")
		}
	}

	// If no results and query could be a display name, try name search
	if len(actors) == 0 && len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("name-search-index").
			Where("gsi2PK", "=", "NAME_SEARCH#"+prefix).
			Filter("gsi2SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&actors)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, EntityActor, "search by name")
		}
	}

	// Convert to activitypub.Actor slice
	result := make([]*activitypub.Actor, 0, len(actors))
	for _, actor := range actors {
		if actor.Actor != nil {
			result = append(result, actor.Actor)
		}
	}

	return result, nil
}

// GetSearchSuggestions returns search suggestions for autocomplete
func (r *ActorRepository) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	if len(prefix) < 2 {
		return []storage.SearchSuggestion{}, nil
	}

	normalizedPrefix := strings.ToLower(strings.TrimSpace(prefix))
	prefixKey := normalizedPrefix[:2]

	var actors []models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("username-search-index").
		Where("gsi1PK", "=", "USERNAME_SEARCH#"+prefixKey).
		Filter("gsi1SK", "BEGINS_WITH", normalizedPrefix).
		Limit(10).
		All(&actors)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityActor, "search suggestions")
	}

	suggestions := make([]storage.SearchSuggestion, 0, len(actors))
	for _, actor := range actors {
		suggestions = append(suggestions, storage.SearchSuggestion{
			Type:  "account",
			Value: actor.Username,
			Score: 100, // Could be based on follower count or activity
		})
	}

	return suggestions, nil
}

// Helper functions

// convertActorFields converts models.ActorField to storage.ActorField
func convertActorFields(fields []models.ActorField) []storage.ActorField {
	result := make([]storage.ActorField, len(fields))
	for i, field := range fields {
		result[i] = storage.ActorField{
			Name:  field.Name,
			Value: field.Value,
			VerifiedAt: func() time.Time {
				if field.VerifiedAt != nil {
					return *field.VerifiedAt
				}
				return time.Time{}
			}(),
		}
	}
	return result
}

// convertStorageActorFields converts storage.ActorField to models.ActorField
func convertStorageActorFields(fields []storage.ActorField) []models.ActorField {
	result := make([]models.ActorField, len(fields))
	for i, field := range fields {
		result[i] = models.ActorField{
			Name:  field.Name,
			Value: field.Value,
			VerifiedAt: func() *time.Time {
				if !field.VerifiedAt.IsZero() {
					return &field.VerifiedAt
				}
				return nil
			}(),
		}
	}
	return result
}

func (r *ActorRepository) resolveActorDomain(actor *activitypub.Actor) string {
	if actor == nil {
		return ""
	}

	scrub := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return ""
		}
		parsed, err := neturl.Parse(raw)
		if err != nil {
			return ""
		}
		return parsed.Host
	}

	if host := scrub(actor.URL); host != "" {
		return host
	}
	if host := scrub(actor.ID); host != "" {
		return host
	}

	return ""
}

func buildActorGSI1Keys(username string) (string, string) {
	lower := strings.ToLower(username)
	prefix := lower
	if len(prefix) >= 2 {
		prefix = prefix[:2]
	}
	return "USERNAME_SEARCH#" + prefix, lower
}

func buildActorGSI2Keys(displayName, username string) (string, string) {
	lower := strings.ToLower(displayName)
	prefix := lower
	if len(prefix) >= 2 {
		prefix = prefix[:2]
	}
	return "NAME_SEARCH#" + prefix, lower + "#" + username
}

func buildActorGSI4Keys(followerCount int, username string) (string, string) {
	bucket := followerCountBucket(followerCount)
	return "ACTOR_RANK#" + bucket, fmt.Sprintf("%010d#%s", followerCount, username)
}

func buildActorGSI5Keys(ts time.Time, username string) (string, string) {
	return "ACTIVE#" + ts.Format(common.DateFormat), fmt.Sprintf("%d#%s", ts.Unix(), username)
}

func followerCountBucket(count int) string {
	switch {
	case count >= 10000:
		return "10K+"
	case count >= 1000:
		return "1K+"
	case count >= 100:
		return "100+"
	case count >= 10:
		return "10+"
	default:
		return "0-9"
	}
}

// getEncryptor returns an encryptor for actor private keys using KMS
func getEncryptor() (marshalers.Encryptor, error) {
	cfg := lesserconfig.Get()
	
	kmsKeyID := cfg.KMSKeyID
	if kmsKeyID == "" {
		return nil, errors.New("KMS_KEY_ID not configured")
	}
	
	return marshalers.NewKMSEncryptor(kmsKeyID)
}

// GetActorByUsername retrieves an actor by username
func (r *ActorRepository) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	// Query for the actor using BaseRepository
	actorModel := &models.Actor{}

	err := r.Get(ctx, fmt.Sprintf("ACTOR#%s", username), "PROFILE", actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, EntityActor, username)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Convert to ActivityPub actor
	return r.modelToActivityPubActor(actorModel)
}

// modelToActivityPubActor converts a model to an ActivityPub actor
func (r *ActorRepository) modelToActivityPubActor(model *models.Actor) (*activitypub.Actor, error) {
	// The actor is stored as a JSON field in the model
	if model.Actor == nil {
		err := errors.New("actor data is missing")
		return nil, ErrorHandler.HandleGetError(err, EntityActor, "actor data")
	}

	// Return the stored actor directly
	return model.Actor, nil
}

// GetAccountSuggestions gets suggested accounts for a user based on "friends of friends" algorithm
//
//nolint:dupl // Account suggestion algorithms are shared between actor repositories
func (r *ActorRepository) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "GetAccountSuggestions"), zap.String("user_id", userID))

	if r.deps == nil {
		log.Warn("dependencies not set, returning empty suggestions")
		return []*activitypub.Actor{}, nil
	}

	// Get user's following list
	following, err := r.getUserFollowing(ctx, userID, log)
	if err != nil {
		return r.getDiscoverableActors(ctx, limit)
	}

	// Build exclusion set
	userFollows := r.buildExclusionSet(following, userID)

	// Collect suggestion candidates from mutual connections
	candidates := r.collectSuggestionCandidates(ctx, userID, following, userFollows)

	// Score and sort candidates
	scored := r.scoreCandidates(candidates)

	// Get actor details for top suggestions
	suggestions := r.buildSuggestions(ctx, scored, limit)

	// Fill remaining slots if needed
	suggestions = r.fillRemainingSuggestions(ctx, suggestions, userFollows, limit)

	log.Info("generated account suggestions",
		zap.Int("requested_limit", limit),
		zap.Int("returned_count", len(suggestions)))

	return suggestions, nil
}

// getUserFollowing gets the list of users that the current user follows
func (r *ActorRepository) getUserFollowing(ctx context.Context, userID string, log *zap.Logger) ([]string, error) {
	following, _, err := r.deps.GetFollowing(ctx, userID, 100, "")
	if err != nil {
		log.Error("failed to get user following for suggestions", zap.Error(err))
		return nil, err
	}
	return following, nil
}

// buildExclusionSet creates a set of actor IDs to exclude from suggestions
func (r *ActorRepository) buildExclusionSet(following []string, userID string) map[string]bool {
	userFollows := make(map[string]bool)
	for _, followedID := range following {
		userFollows[followedID] = true
	}
	userFollows[userID] = true // Exclude self
	return userFollows
}

// suggestionCandidate holds information about a potential suggestion
type suggestionCandidate struct {
	actorID string
	score   int
}

// collectSuggestionCandidates collects candidates from users that the current user follows
func (r *ActorRepository) collectSuggestionCandidates(ctx context.Context, userID string, following []string, userFollows map[string]bool) map[string]int {
	candidates := make(map[string]int) // actorID -> score
	processedActors := make(map[string]bool)

	for i, followedUserID := range following {
		if i >= 20 { // Limit to prevent excessive API calls
			break
		}

		r.processMutualConnections(ctx, userID, followedUserID, userFollows, processedActors, candidates)
	}

	return candidates
}

// processMutualConnections processes mutual connections for a single followed user
func (r *ActorRepository) processMutualConnections(ctx context.Context, userID, followedUserID string, userFollows, processedActors map[string]bool, candidates map[string]int) {
	followedUsername := r.extractUsernameFromActorID(followedUserID)
	if err := common.ValidateRequiredParam("followed_username", followedUsername); err != nil {
		return
	}

	// Get who this followed user follows
	theirFollowing, _, err := r.deps.GetFollowing(ctx, followedUsername, 50, "")
	if err != nil {
		return // Skip if we can't get their following
	}

	// Score each of their follows
	for _, candidate := range theirFollowing {
		if r.shouldSkipCandidate(ctx, userID, candidate, userFollows, processedActors) {
			continue
		}

		candidates[candidate]++
		processedActors[candidate] = true
	}
}

// shouldSkipCandidate checks if a candidate should be skipped
func (r *ActorRepository) shouldSkipCandidate(ctx context.Context, userID, candidate string, userFollows, processedActors map[string]bool) bool {
	// Skip if user already follows or we've processed
	if userFollows[candidate] || processedActors[candidate] {
		return true
	}

	// Check if user has dismissed this suggestion
	return r.isSuggestionDismissed(ctx, userID, candidate)
}

// isSuggestionDismissed checks if a suggestion has been dismissed by the user
func (r *ActorRepository) isSuggestionDismissed(ctx context.Context, userID, candidate string) bool {
	dismissedKey := fmt.Sprintf("dismissed_suggestion:%s", candidate)
	dismissed, _ := r.deps.GetPreference(ctx, userID, dismissedKey)
	if dismissed != nil {
		if isDismissed, ok := dismissed.(bool); ok && isDismissed {
			return true
		}
	}
	return false
}

// scoreCandidates converts candidates map to sorted slice
func (r *ActorRepository) scoreCandidates(candidates map[string]int) []suggestionCandidate {
	scored := make([]suggestionCandidate, 0, len(candidates))
	for actorID, score := range candidates {
		scored = append(scored, suggestionCandidate{actorID: actorID, score: score})
	}

	// Sort by score (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return scored
}

// buildSuggestions builds actor suggestions from scored candidates
func (r *ActorRepository) buildSuggestions(ctx context.Context, scored []suggestionCandidate, limit int) []*activitypub.Actor {
	var suggestions []*activitypub.Actor

	for _, candidate := range scored {
		if len(suggestions) >= limit {
			break
		}

		actor := r.loadActorIfDiscoverable(ctx, candidate.actorID)
		if actor != nil {
			suggestions = append(suggestions, actor)
		}
	}

	return suggestions
}

// loadActorIfDiscoverable loads an actor if it's discoverable
func (r *ActorRepository) loadActorIfDiscoverable(ctx context.Context, actorID string) *activitypub.Actor {
	// Validate actor ID using centralized validation (returns nil for backward compatibility)
	if err := common.ValidateEntityID(actorID, "actor"); err != nil {
		return nil
	}

	username := r.extractUsernameFromActorID(actorID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil
	}

	actor, err := r.GetActor(ctx, username)
	if err != nil {
		return nil
	}

	// Only suggest discoverable accounts
	if !actor.Discoverable {
		return nil
	}

	return actor
}

// fillRemainingSuggestions fills remaining slots with discoverable users
func (r *ActorRepository) fillRemainingSuggestions(ctx context.Context, suggestions []*activitypub.Actor, userFollows map[string]bool, limit int) []*activitypub.Actor {
	if len(suggestions) >= limit {
		return suggestions
	}

	remaining := limit - len(suggestions)
	discoverable, err := r.getDiscoverableActors(ctx, remaining*2) // Get more to filter
	if err != nil {
		return suggestions
	}

	for _, actor := range discoverable {
		if len(suggestions) >= limit {
			break
		}

		if !r.shouldIncludeDiscoverable(actor, suggestions, userFollows) {
			continue
		}

		suggestions = append(suggestions, actor)
	}

	return suggestions
}

// shouldIncludeDiscoverable checks if a discoverable actor should be included
func (r *ActorRepository) shouldIncludeDiscoverable(actor *activitypub.Actor, suggestions []*activitypub.Actor, userFollows map[string]bool) bool {
	// Skip if user already follows
	if userFollows[actor.ID] {
		return false
	}

	// Skip if already in suggestions
	for _, existing := range suggestions {
		if existing.ID == actor.ID {
			return false
		}
	}

	return true
}

// MigrationInfo represents account migration information
type MigrationInfo struct {
	AlsoKnownAs []string `json:"also_known_as"`
	MovedTo     string   `json:"moved_to,omitempty"`
}

// UpdateAlsoKnownAs updates the AlsoKnownAs field for an actor
func (r *ActorRepository) UpdateAlsoKnownAs(ctx context.Context, username string, alsoKnownAs []string) error {
	log := r.logger.With(
		zap.String("method", "UpdateAlsoKnownAs"),
		zap.String("username", username),
		zap.Int("also_known_as_count", len(alsoKnownAs)),
	)

	// Get existing actor first
	actorModel := &models.Actor{}
	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			log.Error("actor not found for alsoKnownAs update")
			return common.ActorNotFoundError{Username: username}
		}
		log.Error("failed to get existing actor", zap.Error(err))
		return ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Update the alsoKnownAs field in the embedded actor
	if actorModel.Actor == nil {
		log.Error("actor data is nil")
		err := errors.New("actor data is missing")
		return ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	actorModel.Actor.AlsoKnownAs = alsoKnownAs

	// Update using BaseRepository
	err = r.Update(ctx, actorModel)
	if err != nil {
		log.Error("failed to update actor alsoKnownAs", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityActor, username)
	}

	log.Info("updated actor alsoKnownAs successfully")
	return nil
}

// UpdateMovedTo updates the MovedTo field for an actor
func (r *ActorRepository) UpdateMovedTo(ctx context.Context, username string, movedTo string) error {
	log := r.logger.With(
		zap.String("method", "UpdateMovedTo"),
		zap.String("username", username),
		zap.String("moved_to", movedTo),
	)

	// Get existing actor first
	actorModel := &models.Actor{}
	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			log.Error("actor not found for movedTo update")
			return common.ActorNotFoundError{Username: username}
		}
		log.Error("failed to get existing actor", zap.Error(err))
		return ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Update the movedTo field in the embedded actor
	if actorModel.Actor == nil {
		log.Error("actor data is nil")
		err := errors.New("actor data is missing")
		return ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	actorModel.Actor.MovedTo = movedTo

	// Update using BaseRepository
	err = r.Update(ctx, actorModel)
	if err != nil {
		log.Error("failed to update actor movedTo", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityActor, username)
	}

	log.Info("updated actor movedTo successfully")
	return nil
}

// CheckAlsoKnownAs checks if targetActorID is in the AlsoKnownAs slice for the given username
func (r *ActorRepository) CheckAlsoKnownAs(ctx context.Context, username string, targetActorID string) (bool, error) {
	log := r.logger.With(
		zap.String("method", "CheckAlsoKnownAs"),
		zap.String("username", username),
		zap.String("target_actor_id", targetActorID),
	)

	// Get existing actor
	actorModel := &models.Actor{}
	// Use direct query for Select functionality (BaseRepository doesn't support Select)
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Select("Actor"). // Only select the Actor field for efficiency
		First(actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			log.Debug("actor not found for alsoKnownAs check")
			return false, common.ActorNotFoundError{Username: username}
		}
		log.Error("failed to get actor", zap.Error(err))
		return false, ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Check if the embedded actor data exists
	if actorModel.Actor == nil {
		log.Debug("actor data is nil")
		return false, nil
	}

	// Check if targetActorID is in the AlsoKnownAs slice
	for _, actorID := range actorModel.Actor.AlsoKnownAs {
		if actorID == targetActorID {
			log.Debug("target actor ID found in alsoKnownAs")
			return true, nil
		}
	}

	log.Debug("target actor ID not found in alsoKnownAs")
	return false, nil
}

// GetActorMigrationInfo returns migration information for an actor
func (r *ActorRepository) GetActorMigrationInfo(ctx context.Context, username string) (*MigrationInfo, error) {
	log := r.logger.With(
		zap.String("method", "GetActorMigrationInfo"),
		zap.String("username", username),
	)

	// Get existing actor
	actorModel := &models.Actor{}
	// Use direct query for Select functionality (BaseRepository doesn't support Select)
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Select("Actor"). // Only select the Actor field for efficiency
		First(actorModel)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			log.Error("actor not found for migration info")
			return nil, common.ActorNotFoundError{Username: username}
		}
		log.Error("failed to get actor", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Check if the embedded actor data exists
	if actorModel.Actor == nil {
		log.Error("actor data is nil")
		err := errors.New("actor data is missing")
		return nil, ErrorHandler.HandleGetError(err, EntityActor, username)
	}

	// Create and return migration info
	migrationInfo := &MigrationInfo{
		AlsoKnownAs: actorModel.Actor.AlsoKnownAs,
		MovedTo:     actorModel.Actor.MovedTo,
	}

	log.Debug("retrieved actor migration info",
		zap.Int("also_known_as_count", len(migrationInfo.AlsoKnownAs)),
		zap.String("moved_to", migrationInfo.MovedTo),
	)

	return migrationInfo, nil
}

// RemoveAccountSuggestion removes an account from suggestions for a user
func (r *ActorRepository) RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error {
	log := r.logger.With(
		zap.String("method", "RemoveAccountSuggestion"),
		zap.String("user_id", userID),
		zap.String("target_id", targetID),
	)

	if r.deps == nil {
		log.Error("dependencies not set")
		err := errors.New("dependencies not available")
		return ErrorHandler.HandleGetError(err, "dependencies", userID)
	}

	// Store the dismissed suggestion in user preferences
	// This prevents the account from being suggested again
	dismissedKey := fmt.Sprintf("dismissed_suggestion:%s", targetID)
	err := r.deps.SetPreference(ctx, userID, dismissedKey, true)
	if err != nil {
		log.Error("failed to store dismissed suggestion preference", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "suggestion preference", targetID)
	}

	log.Info("account suggestion removed")

	return nil
}

// Helper functions

// getDiscoverableActors returns actors marked as discoverable
func (r *ActorRepository) getDiscoverableActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
	return r.getRecentActiveActors(ctx, limit)
}

// getRecentActiveActors returns recently active actors using the activity index
func (r *ActorRepository) getRecentActiveActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "getRecentActiveActors"))

	// Query using the activity index (GSI5) to get recently active actors
	// Try multiple days to get enough results
	var allActors []models.Actor
	now := time.Now()

	for days := 0; days < 7 && len(allActors) < limit*2; days++ {
		searchDate := now.AddDate(0, 0, -days)
		dateKey := "ACTIVE#" + searchDate.Format(common.DateFormat)

		var actors []models.Actor
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("activity-index").
			Where("gsi5PK", "=", dateKey).
			OrderBy("gsi5SK", "DESC"). // Get most recent first
			Limit(limit).
			All(&actors)
		if err != nil {
			log.Warn("failed to query activity index", zap.String("date", dateKey), zap.Error(err))
			continue
		}

		allActors = append(allActors, actors...)
	}

	// If no recent activity found, fall back to popularity index
	if err := common.ValidateSliceNotEmpty("all_actors", allActors); err != nil {
		log.Debug("no recent activity found, falling back to popularity index")
		return r.getPopularActors(ctx, limit)
	}

	// Convert to activitypub.Actor slice and filter for discoverable
	result := make([]*activitypub.Actor, 0, limit)
	seen := make(map[string]bool)

	for _, actor := range allActors {
		if len(result) >= limit {
			break
		}

		// Avoid duplicates
		if seen[actor.Username] {
			continue
		}
		seen[actor.Username] = true

		if actor.Actor != nil && actor.Actor.Discoverable {
			result = append(result, actor.Actor)
		}
	}

	log.Debug("retrieved recent active actors", zap.Int("count", len(result)))
	return result, nil
}

// getPopularActors returns actors sorted by popularity (follower count)
func (r *ActorRepository) getPopularActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "getPopularActors"))

	// Query popularity buckets starting from highest
	buckets := []string{"10K+", "1K+", "100+", "10+", "0-9"}
	var allActors []models.Actor

	for _, bucket := range buckets {
		if len(allActors) >= limit {
			break
		}

		var actors []models.Actor
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("popularity-index").
			Where("gsi4PK", "=", "ACTOR_RANK#"+bucket).
			OrderBy("gsi4SK", "DESC"). // Highest follower count first
			Limit(limit - len(allActors)).
			All(&actors)
		if err != nil {
			log.Warn("failed to query popularity index", zap.String("bucket", bucket), zap.Error(err))
			continue
		}

		allActors = append(allActors, actors...)
	}

	// Convert to activitypub.Actor slice
	result := make([]*activitypub.Actor, 0, len(allActors))
	for _, actor := range allActors {
		if len(result) >= limit {
			break
		}

		if actor.Actor != nil && actor.Actor.Discoverable {
			result = append(result, actor.Actor)
		}
	}

	log.Debug("retrieved popular actors", zap.Int("count", len(result)))
	return result, nil
}

// extractUsernameFromActorID extracts username from actor ID
func (r *ActorRepository) extractUsernameFromActorID(actorID string) string {
	// Handle local actor IDs like "https://example.com/users/username"
	parts := strings.Split(actorID, "/")
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
		username := parts[len(parts)-1]
		// Remove any @ prefix if present
		username = strings.TrimPrefix(username, "@")
		return username
	}

	// Handle direct username format
	return strings.TrimPrefix(actorID, "@")
}

// GetCachedRemoteActor retrieves a cached remote actor by handle
//
//nolint:dupl // Remote actor caching patterns are shared between actor repositories
func (r *ActorRepository) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "GetCachedRemoteActor"), zap.String("handle", handle))

	// Use direct query for RemoteActor (different model type)
	var remoteActor models.RemoteActor

	err := r.db.WithContext(ctx).Model(&models.RemoteActor{}).
		Where("PK", "=", fmt.Sprintf("REMOTE_ACTOR#%s", handle)).
		Where("SK", "=", "PROFILE").
		First(&remoteActor)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			// Extract username from handle for error (consistent with legacy)
			username := strings.Split(handle, "@")[0]
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, ErrorHandler.HandleGetError(err, "remote actor", handle)
	}

	// Check if the cache has expired (consistent with legacy behavior)
	if time.Now().After(remoteActor.ExpiresAt) {
		log.Debug("cached remote actor expired",
			zap.Time("expired_at", remoteActor.ExpiresAt))
		// Extract username from handle for error (consistent with legacy)
		username := strings.Split(handle, "@")[0]
		return nil, common.ActorNotFoundError{Username: username}
	}

	log.Debug("retrieved cached remote actor",
		zap.String("actor_id", remoteActor.Actor.ID))

	return remoteActor.Actor, nil
}
