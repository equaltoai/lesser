package repositories

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
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

// ActorRepositoryDeps interface for dependencies - implemented by the storage adapter
type ActorRepositoryDeps interface {
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetPreference(ctx context.Context, username, key string) (any, error)
	SetPreference(ctx context.Context, username, key string, value any) error
}

// ActorRepository implements actor operations using DynamORM
type ActorRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
	deps      ActorRepositoryDeps
}

// NewActorRepository creates a new actor repository
func NewActorRepository(db core.DB, tableName string, logger *zap.Logger) *ActorRepository {
	return &ActorRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// SetDependencies sets the dependencies for cross-repository operations
func (r *ActorRepository) SetDependencies(deps ActorRepositoryDeps) {
	r.deps = deps
}

// CreateActor creates a new actor in DynamoDB
func (r *ActorRepository) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	if actor.PreferredUsername == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	username := actor.PreferredUsername
	numericID := common.GenerateNumericID(username)

	// Encrypt private key if encryption is available
	encryptedKey := privateKey
	if encryptor, err := getEncryptor(); err == nil {
		if encrypted, err := encryptor.Encrypt([]byte(privateKey)); err == nil {
			encryptedKey = base64.StdEncoding.EncodeToString(encrypted)
		} else {
			common.WithContext(ctx).Warn("failed to encrypt private key", zap.Error(err))
		}
	} else {
		common.WithContext(ctx).Warn("encryption not available, storing private key in plaintext", zap.Error(err))
	}

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
	domain := config.Get().Domain
	if domain != "" {
		actorModel.GSI3PK = "DOMAIN#" + domain
		actorModel.GSI3SK = username
	}

	// Create the actor using DynamORM
	err := r.db.WithContext(ctx).Model(actorModel).Create()
	if err != nil {
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

// GetActor retrieves an actor by username
func (r *ActorRepository) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	return actorModel.Actor, nil
}

// GetActorWithMetadata retrieves an actor with metadata
func (r *ActorRepository) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil, common.ActorNotFoundError{Username: username}
		}
		return nil, nil, fmt.Errorf("failed to get actor: %w", err)
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
	// First get the numeric ID mapping
	var mapping models.NumericIDMapping
	err := r.db.WithContext(ctx).Model(&models.NumericIDMapping{}).
		Where("PK", "=", "NUMERIC_ID#"+numericID).
		Where("SK", "=", "METADATA").
		First(&mapping)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("actor not found: %s", numericID)
		}
		return nil, fmt.Errorf("failed to get numeric ID mapping: %w", err)
	}

	// Now get the actual actor using the username
	return r.GetActor(ctx, mapping.Username)
}

// GetActorPrivateKey retrieves an actor's private key
func (r *ActorRepository) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Select("PrivateKey").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", common.ActorNotFoundError{Username: username}
		}
		return "", fmt.Errorf("failed to get actor private key: %w", err)
	}

	// Decrypt private key if it's encrypted
	privateKey := actorModel.PrivateKey
	if encryptor, err := getEncryptor(); err == nil {
		// Try to decode as base64 - if it fails, assume it's plaintext
		if decoded, err := base64.StdEncoding.DecodeString(privateKey); err == nil {
			if decrypted, err := encryptor.Decrypt(decoded); err == nil {
				privateKey = string(decrypted)
			} else {
				common.WithContext(ctx).Warn("failed to decrypt private key", zap.Error(err))
			}
		}
	}
	return privateKey, nil
}

// UpdateActor updates an existing actor
func (r *ActorRepository) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	username := actor.PreferredUsername
	if username == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update the actor data
	actorModel.Actor = actor

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor: %w", err)
	}

	return nil
}

// UpdateActorLastStatusTime updates the last status timestamp
func (r *ActorRepository) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update last status time
	now := time.Now()
	actorModel.LastStatusAt = &now

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor last status time: %w", err)
	}

	return nil
}

// SetActorFields updates the profile fields for an actor
func (r *ActorRepository) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Convert and update fields
	actorModel.Fields = convertStorageActorFields(fields)

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor fields: %w", err)
	}

	return nil
}

// DeleteActor deletes an actor
func (r *ActorRepository) DeleteActor(ctx context.Context, username string) error {
	// Delete the actor using DynamORM
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to delete actor: %w", err)
	}

	return nil
}

// SearchAccounts searches for actors by username or display name
func (r *ActorRepository) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	if query == "" {
		return []*activitypub.Actor{}, nil
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	var actors []models.Actor

	// Try username search first using GSI1
	if len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("username-search-index").
			Where("GSI1PK", "=", "USERNAME_SEARCH#"+prefix).
			Filter("GSI1SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&actors)
		if err != nil {
			return nil, fmt.Errorf("failed to search actors by username: %w", err)
		}
	}

	// If no results and query could be a display name, try name search
	if len(actors) == 0 && len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("name-search-index").
			Where("GSI2PK", "=", "NAME_SEARCH#"+prefix).
			Filter("GSI2SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&actors)
		if err != nil {
			return nil, fmt.Errorf("failed to search actors by name: %w", err)
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
		Where("GSI1PK", "=", "USERNAME_SEARCH#"+prefixKey).
		Filter("GSI1SK", "BEGINS_WITH", normalizedPrefix).
		Limit(10).
		All(&actors)
	if err != nil {
		return nil, fmt.Errorf("failed to get search suggestions: %w", err)
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
			Name:       field.Name,
			Value:      field.Value,
			VerifiedAt: func() time.Time { if field.VerifiedAt != nil { return *field.VerifiedAt } else { return time.Time{} } }(),
		}
	}
	return result
}

// convertStorageActorFields converts storage.ActorField to models.ActorField
func convertStorageActorFields(fields []storage.ActorField) []models.ActorField {
	result := make([]models.ActorField, len(fields))
	for i, field := range fields {
		result[i] = models.ActorField{
			Name:       field.Name,
			Value:      field.Value,
			VerifiedAt: func() *time.Time { if !field.VerifiedAt.IsZero() { return &field.VerifiedAt } else { return nil } }(),
		}
	}
	return result
}

// getEncryptor returns an AES encryptor for private key encryption
// Falls back gracefully if encryption key is not available
func getEncryptor() (marshalers.Encryptor, error) {
	// First check for KMS (future implementation)
	if kmsKeyID := config.Get().KMSKeyID; kmsKeyID != "" {
		// TODO: Implement KMS encryptor when KMS client is available
		// For now, fall through to AES encryption
	}

	// Check for AES encryption key
	encryptionKey := os.Getenv("DYNAMODB_ENCRYPTION_KEY")
	if encryptionKey == "" {
		// Try alternative env var
		encryptionKey = os.Getenv("ACTOR_PRIVATE_KEY_ENCRYPTION")
	}

	if encryptionKey != "" {
		// Decode base64 key
		key, err := base64.StdEncoding.DecodeString(encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("invalid encryption key format: %w", err)
		}
		return marshalers.NewAESEncryptorWithKey(key)
	}

	return nil, fmt.Errorf("no encryption key available")
}

// GetActorByUsername retrieves an actor by username
func (r *ActorRepository) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	// Query for the actor
	var actorModel models.Actor
	
	query := r.db.Model(&actorModel).
		Where("PK = ? AND SK = ?",
			fmt.Sprintf("ACTOR#%s", username),
			"PROFILE")

	if err := query.First(&actorModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("actor not found")
		}
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Convert to ActivityPub actor
	return r.modelToActivityPubActor(&actorModel)
}

// modelToActivityPubActor converts a model to an ActivityPub actor
func (r *ActorRepository) modelToActivityPubActor(model *models.Actor) (*activitypub.Actor, error) {
	// The actor is stored as a JSON field in the model
	if model.Actor == nil {
		return nil, fmt.Errorf("actor data is missing")
	}

	// Return the stored actor directly
	return model.Actor, nil
}

// GetAccountSuggestions gets suggested accounts for a user based on "friends of friends" algorithm
func (r *ActorRepository) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "GetAccountSuggestions"), zap.String("user_id", userID))

	if r.deps == nil {
		log.Warn("dependencies not set, returning empty suggestions")
		return []*activitypub.Actor{}, nil
	}

	// Step 1: Get users that the current user follows
	following, _, err := r.deps.GetFollowing(ctx, userID, 100, "")
	if err != nil {
		log.Error("failed to get user following for suggestions", zap.Error(err))
		// Fall back to discoverable users if we can't get following
		return r.getDiscoverableActors(ctx, limit)
	}

	suggestionCandidates := make(map[string]int) // actorID -> score
	processedActors := make(map[string]bool)

	// Get who the user already follows to exclude them
	userFollows := make(map[string]bool)
	for _, followedID := range following {
		userFollows[followedID] = true
	}
	userFollows[userID] = true // Exclude self

	// For each user the current user follows, get who they follow
	for i, followedUserID := range following {
		if i >= 20 { // Limit to prevent excessive API calls
			break
		}

		followedUsername := r.extractUsernameFromActorID(followedUserID)
		if followedUsername == "" {
			continue
		}

		// Get who this followed user follows
		theirFollowing, _, err := r.deps.GetFollowing(ctx, followedUsername, 50, "")
		if err != nil {
			continue // Skip if we can't get their following
		}

		// Score each of their follows
		for _, candidate := range theirFollowing {
			if userFollows[candidate] || processedActors[candidate] {
				continue // Skip if user already follows or we've processed
			}

			// Check if user has dismissed this suggestion
			dismissedKey := fmt.Sprintf("dismissed_suggestion:%s", candidate)
			dismissed, _ := r.deps.GetPreference(ctx, userID, dismissedKey)
			if dismissed != nil {
				if isDismissed, ok := dismissed.(bool); ok && isDismissed {
					continue // Skip dismissed suggestions
				}
			}

			suggestionCandidates[candidate]++
			processedActors[candidate] = true
		}
	}

	// Step 2: Get actors with high scores (multiple mutual connections)
	type scoredActor struct {
		actorID string
		score   int
	}

	var scored []scoredActor
	for actorID, score := range suggestionCandidates {
		scored = append(scored, scoredActor{actorID: actorID, score: score})
	}

	// Sort by score (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Step 3: Get actor details for top suggestions
	var suggestions []*activitypub.Actor
	for _, scoredActor := range scored {
		if len(suggestions) >= limit {
			break
		}

		username := r.extractUsernameFromActorID(scoredActor.actorID)
		if username == "" {
			continue
		}

		actor, err := r.GetActor(ctx, username)
		if err != nil {
			continue // Skip if we can't get actor details
		}

		// Only suggest discoverable accounts
		if actor.Discoverable {
			suggestions = append(suggestions, actor)
		}
	}

	// Step 4: Fill remaining slots with discoverable users if needed
	if len(suggestions) < limit {
		remaining := limit - len(suggestions)
		discoverable, err := r.getDiscoverableActors(ctx, remaining*2) // Get more to filter
		if err == nil {
			for _, actor := range discoverable {
				if len(suggestions) >= limit {
					break
				}

				// Skip if already in suggestions or user follows them
				skip := false
				for _, existing := range suggestions {
					if existing.ID == actor.ID {
						skip = true
						break
					}
				}
				if skip || userFollows[actor.ID] {
					continue
				}

				suggestions = append(suggestions, actor)
			}
		}
	}

	log.Info("generated account suggestions",
		zap.Int("requested_limit", limit),
		zap.Int("returned_count", len(suggestions)))

	return suggestions, nil
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
		return fmt.Errorf("dependencies not available")
	}

	// Store the dismissed suggestion in user preferences
	// This prevents the account from being suggested again
	dismissedKey := fmt.Sprintf("dismissed_suggestion:%s", targetID)
	err := r.deps.SetPreference(ctx, userID, dismissedKey, true)
	if err != nil {
		log.Error("failed to store dismissed suggestion preference", zap.Error(err))
		return fmt.Errorf("failed to remove account suggestion: %w", err)
	}

	log.Info("account suggestion removed")

	return nil
}

// Helper functions

// getDiscoverableActors returns actors marked as discoverable
func (r *ActorRepository) getDiscoverableActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
	// Use the existing SearchAccounts method with empty query to get discoverable accounts
	actors, err := r.SearchAccounts(ctx, "", limit*2, false, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get discoverable actors: %w", err)
	}

	// Filter for discoverable only
	var discoverable []*activitypub.Actor
	for _, actor := range actors {
		if actor.Discoverable && len(discoverable) < limit {
			discoverable = append(discoverable, actor)
		}
	}

	return discoverable, nil
}

// extractUsernameFromActorID extracts username from actor ID
func (r *ActorRepository) extractUsernameFromActorID(actorID string) string {
	// Handle local actor IDs like "https://example.com/users/username"
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		username := parts[len(parts)-1]
		// Remove any @ prefix if present
		username = strings.TrimPrefix(username, "@")
		return username
	}
	
	// Handle direct username format
	return strings.TrimPrefix(actorID, "@")
}

// GetCachedRemoteActor retrieves a cached remote actor by handle
func (r *ActorRepository) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "GetCachedRemoteActor"), zap.String("handle", handle))

	var remoteActor models.RemoteActor

	err := r.db.WithContext(ctx).Model(&models.RemoteActor{}).
		Where("PK", "=", fmt.Sprintf("REMOTE_ACTOR#%s", handle)).
		Where("SK", "=", "PROFILE").
		First(&remoteActor)
	if err != nil {
		if errors.IsNotFound(err) {
			// Extract username from handle for error (consistent with legacy)
			username := strings.Split(handle, "@")[0]
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, fmt.Errorf("failed to get cached remote actor: %w", err)
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
