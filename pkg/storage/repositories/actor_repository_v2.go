package repositories

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ActorRepositoryV2 implements actor operations using BaseRepository
// This demonstrates the code reduction possible with BaseRepository
type ActorRepositoryV2 struct {
	*BaseRepository[*models.Actor]
	logger *zap.Logger
	deps   ActorRepositoryDeps
}

// NewActorRepositoryV2 creates a new actor repository using BaseRepository
func NewActorRepositoryV2(db core.DB, tableName string, logger *zap.Logger) *ActorRepositoryV2 {
	return &ActorRepositoryV2{
		BaseRepository: NewBaseRepository[*models.Actor](db, tableName, logger),
		logger:         logger,
	}
}

// SetDependencies sets the dependencies for cross-repository operations
func (r *ActorRepositoryV2) SetDependencies(deps ActorRepositoryDeps) {
	r.deps = deps
}

// CreateActor creates a new actor in DynamoDB
// BEFORE: 30+ lines of boilerplate
// AFTER: Focused on business logic only
func (r *ActorRepositoryV2) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
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

	// Use BaseRepository Create - saves ~20 lines of boilerplate
	err := r.Create(ctx, actorModel)
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
// BEFORE: 15+ lines of query construction
// AFTER: Single line using BaseRepository
func (r *ActorRepositoryV2) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	actorModel := &models.Actor{}
	
	// Use BaseRepository Get - saves ~15 lines of boilerplate
	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	return actorModel.Actor, nil
}

// GetActorWithMetadata retrieves an actor with metadata
func (r *ActorRepositoryV2) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	actorModel := &models.Actor{}
	
	// Use BaseRepository Get
	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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

// UpdateActor updates an existing actor
// BEFORE: Complex query and update logic
// AFTER: Get + Update using BaseRepository
func (r *ActorRepositoryV2) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	username := actor.PreferredUsername
	if username == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	// Get existing actor first
	actorModel := &models.Actor{}
	err := r.Get(ctx, "ACTOR#"+username, "PROFILE", actorModel)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update the actor data
	actorModel.Actor = actor

	// Use BaseRepository Update - saves ~15 lines of boilerplate
	err = r.Update(ctx, actorModel)
	if err != nil {
		return fmt.Errorf("failed to update actor: %w", err)
	}

	return nil
}

// DeleteActor deletes an actor
// BEFORE: 15+ lines with error handling
// AFTER: Single line using BaseRepository
func (r *ActorRepositoryV2) DeleteActor(ctx context.Context, username string) error {
	// Use BaseRepository Delete - saves ~15 lines of boilerplate
	err := r.Delete(ctx, "ACTOR#"+username, "PROFILE")
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to delete actor: %w", err)
	}

	return nil
}

// SearchAccounts searches for actors by username or display name
// Uses BaseRepository QueryGSI for efficient searches
func (r *ActorRepositoryV2) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	if query == "" {
		return []*activitypub.Actor{}, nil
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	var actors []*models.Actor

	// Try username search first using GSI1
	if len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		// Use BaseRepository QueryGSI - saves ~20 lines of query construction
		gsiActors, err := r.QueryGSI(ctx, "username-search-index", "USERNAME_SEARCH#"+prefix, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to search actors by username: %w", err)
		}
		
		// Filter by prefix match
		for _, actor := range gsiActors {
			if strings.HasPrefix(actor.GSI1SK, normalizedQuery) {
				actors = append(actors, actor)
			}
		}
	}

	// If no results and query could be a display name, try name search
	if len(actors) == 0 && len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		// Use BaseRepository QueryGSI
		gsiActors, err := r.QueryGSI(ctx, "name-search-index", "NAME_SEARCH#"+prefix, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to search actors by name: %w", err)
		}
		
		// Filter by prefix match
		for _, actor := range gsiActors {
			if strings.HasPrefix(actor.GSI2SK, normalizedQuery) {
				actors = append(actors, actor)
			}
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

// GetAccountSuggestions gets suggested accounts using BaseRepository
func (r *ActorRepositoryV2) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
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

// GetCachedRemoteActor retrieves a cached remote actor by handle
func (r *ActorRepositoryV2) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "GetCachedRemoteActor"), zap.String("handle", handle))

	// Note: This would need a separate RemoteActor model with BaseRepository
	// For now, keeping the original implementation
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

// Helper methods remain largely the same...

// getDiscoverableActors returns actors marked as discoverable
func (r *ActorRepositoryV2) getDiscoverableActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
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
func (r *ActorRepositoryV2) extractUsernameFromActorID(actorID string) string {
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

// Helper functions are reused from actor_repository.go
// convertActorFields and getEncryptor are already defined there

// Code Reduction Summary:
// - CreateActor: ~20 lines saved (DynamORM boilerplate)
// - GetActor: ~15 lines saved (query construction)
// - GetActorWithMetadata: ~15 lines saved
// - UpdateActor: ~15 lines saved (query + update logic)
// - DeleteActor: ~15 lines saved
// - SearchAccounts: ~40 lines saved (2 GSI queries)
// - Total: ~120 lines of boilerplate eliminated!
//
// Additional benefits:
// - Consistent error handling across all methods
// - Built-in logging at the BaseRepository level
// - Type safety with generics
// - Easier to test and maintain