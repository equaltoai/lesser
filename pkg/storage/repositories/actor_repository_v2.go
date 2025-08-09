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
func (r *ActorRepositoryV2) SearchAccounts(ctx context.Context, query string, limit int, _ bool, _ int) ([]*activitypub.Actor, error) {
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

	// Get user's following list
	following, err := r.getUserFollowingV2(ctx, userID, log)
	if err != nil {
		return r.getDiscoverableActors(ctx, limit)
	}

	// Build exclusion set
	userFollows := r.buildExclusionSetV2(following, userID)

	// Collect suggestion candidates from mutual connections
	candidates := r.collectSuggestionCandidatesV2(ctx, userID, following, userFollows)

	// Score and sort candidates
	scored := r.scoreCandidatesV2(candidates)

	// Get actor details for top suggestions
	suggestions := r.buildSuggestionsV2(ctx, scored, limit)

	// Fill remaining slots if needed
	suggestions = r.fillRemainingSuggestionsV2(ctx, suggestions, userFollows, limit)

	log.Info("generated account suggestions",
		zap.Int("requested_limit", limit),
		zap.Int("returned_count", len(suggestions)))

	return suggestions, nil
}

// getUserFollowingV2 gets the list of users that the current user follows
func (r *ActorRepositoryV2) getUserFollowingV2(ctx context.Context, userID string, log *zap.Logger) ([]string, error) {
	following, _, err := r.deps.GetFollowing(ctx, userID, 100, "")
	if err != nil {
		log.Error("failed to get user following for suggestions", zap.Error(err))
		return nil, err
	}
	return following, nil
}

// buildExclusionSetV2 creates a set of actor IDs to exclude from suggestions
func (r *ActorRepositoryV2) buildExclusionSetV2(following []string, userID string) map[string]bool {
	userFollows := make(map[string]bool)
	for _, followedID := range following {
		userFollows[followedID] = true
	}
	userFollows[userID] = true // Exclude self
	return userFollows
}

// suggestionCandidateV2 holds information about a potential suggestion
type suggestionCandidateV2 struct {
	actorID string
	score   int
}

// collectSuggestionCandidatesV2 collects candidates from users that the current user follows
func (r *ActorRepositoryV2) collectSuggestionCandidatesV2(ctx context.Context, userID string, following []string, userFollows map[string]bool) map[string]int {
	candidates := make(map[string]int) // actorID -> score
	processedActors := make(map[string]bool)

	for i, followedUserID := range following {
		if i >= 20 { // Limit to prevent excessive API calls
			break
		}

		r.processMutualConnectionsV2(ctx, userID, followedUserID, userFollows, processedActors, candidates)
	}

	return candidates
}

// processMutualConnectionsV2 processes mutual connections for a single followed user
func (r *ActorRepositoryV2) processMutualConnectionsV2(ctx context.Context, userID, followedUserID string, userFollows, processedActors map[string]bool, candidates map[string]int) {
	followedUsername := r.extractUsernameFromActorID(followedUserID)
	if followedUsername == "" {
		return
	}

	// Get who this followed user follows
	theirFollowing, _, err := r.deps.GetFollowing(ctx, followedUsername, 50, "")
	if err != nil {
		return // Skip if we can't get their following
	}

	// Score each of their follows
	for _, candidate := range theirFollowing {
		if r.shouldSkipCandidateV2(ctx, userID, candidate, userFollows, processedActors) {
			continue
		}

		candidates[candidate]++
		processedActors[candidate] = true
	}
}

// shouldSkipCandidateV2 checks if a candidate should be skipped
func (r *ActorRepositoryV2) shouldSkipCandidateV2(ctx context.Context, userID, candidate string, userFollows, processedActors map[string]bool) bool {
	// Skip if user already follows or we've processed
	if userFollows[candidate] || processedActors[candidate] {
		return true
	}

	// Check if user has dismissed this suggestion
	return r.isSuggestionDismissedV2(ctx, userID, candidate)
}

// isSuggestionDismissedV2 checks if a suggestion has been dismissed by the user
func (r *ActorRepositoryV2) isSuggestionDismissedV2(ctx context.Context, userID, candidate string) bool {
	dismissedKey := fmt.Sprintf("dismissed_suggestion:%s", candidate)
	dismissed, _ := r.deps.GetPreference(ctx, userID, dismissedKey)
	if dismissed != nil {
		if isDismissed, ok := dismissed.(bool); ok && isDismissed {
			return true
		}
	}
	return false
}

// scoreCandidatesV2 converts candidates map to sorted slice
func (r *ActorRepositoryV2) scoreCandidatesV2(candidates map[string]int) []suggestionCandidateV2 {
	scored := make([]suggestionCandidateV2, 0, len(candidates))
	for actorID, score := range candidates {
		scored = append(scored, suggestionCandidateV2{actorID: actorID, score: score})
	}

	// Sort by score (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return scored
}

// buildSuggestionsV2 builds actor suggestions from scored candidates
func (r *ActorRepositoryV2) buildSuggestionsV2(ctx context.Context, scored []suggestionCandidateV2, limit int) []*activitypub.Actor {
	var suggestions []*activitypub.Actor

	for _, candidate := range scored {
		if len(suggestions) >= limit {
			break
		}

		actor := r.loadActorIfDiscoverableV2(ctx, candidate.actorID)
		if actor != nil {
			suggestions = append(suggestions, actor)
		}
	}

	return suggestions
}

// loadActorIfDiscoverableV2 loads an actor if it's discoverable
func (r *ActorRepositoryV2) loadActorIfDiscoverableV2(ctx context.Context, actorID string) *activitypub.Actor {
	username := r.extractUsernameFromActorID(actorID)
	if username == "" {
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

// fillRemainingSuggestionsV2 fills remaining slots with discoverable users
func (r *ActorRepositoryV2) fillRemainingSuggestionsV2(ctx context.Context, suggestions []*activitypub.Actor, userFollows map[string]bool, limit int) []*activitypub.Actor {
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

		if !r.shouldIncludeDiscoverableV2(actor, suggestions, userFollows) {
			continue
		}

		suggestions = append(suggestions, actor)
	}

	return suggestions
}

// shouldIncludeDiscoverableV2 checks if a discoverable actor should be included
func (r *ActorRepositoryV2) shouldIncludeDiscoverableV2(actor *activitypub.Actor, suggestions []*activitypub.Actor, userFollows map[string]bool) bool {
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
