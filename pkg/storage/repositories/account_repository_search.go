package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ===== Search and Discovery =====
// This file contains search and discovery methods for the AccountRepository

// SearchActors searches for actors by username or display name
func (r *AccountRepository) SearchActors(ctx context.Context, query string, limit int, offset int, following bool, username string) ([]*activitypub.Actor, error) {
	// Normalize query for search
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return []*activitypub.Actor{}, nil
	}

	var actors []*activitypub.Actor

	// If following filter is enabled, search only among followed accounts
	if following && username != "" {
		actors = r.searchFollowedActors(ctx, username, query, limit, offset)
	} else {
		// Search all actors
		actors = r.searchAllActors(ctx, query, limit, offset)
	}

	return actors, nil
}

// searchAllActors searches all actors in the system
func (r *AccountRepository) searchAllActors(ctx context.Context, query string, limit int, offset int) []*activitypub.Actor {
	var actors []*activitypub.Actor

	// First try exact username match
	if actor, err := r.GetActor(ctx, query); err == nil {
		actors = append(actors, actor)
	}

	// Then search by partial match using GSI
	var actorModels []models.Actor

	// Search local actors
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("domain-index").
		Where("GSI3PK", "=", fmt.Sprintf("DOMAIN#%s", r.domain)).
		Limit(limit + offset).
		All(&actorModels)

	if err != nil {
		r.logger.Error("failed to search actors",
			zap.String("query", query),
			zap.Error(err))
		return actors
	}

	// Filter by query and apply offset
	matched := 0
	skipped := 0
	for _, model := range actorModels {
		// Check if username or display name contains query
		if strings.Contains(strings.ToLower(model.Username), query) ||
			(model.Actor != nil && strings.Contains(strings.ToLower(model.Actor.Name), query)) {
			if skipped < offset {
				skipped++
				continue
			}

			if matched >= limit {
				break
			}

			actors = append(actors, model.Actor)
			matched++
		}
	}

	return actors
}

// searchFollowedActors searches only among actors the user follows
func (r *AccountRepository) searchFollowedActors(ctx context.Context, username, query string, limit int, offset int) []*activitypub.Actor {
	var actors []*activitypub.Actor

	// Get all following relationships
	var follows []models.Follow

	err := r.db.WithContext(ctx).Model(&models.Follow{}).
		Where("PK", "=", fmt.Sprintf("FOLLOWER#%s", username)).
		Where("SK", "BEGINS_WITH", "FOLLOWS#").
		All(&follows)

	if err != nil {
		r.logger.Error("failed to get following for search",
			zap.String("username", username),
			zap.Error(err))
		return actors
	}

	// Search among followed actors
	matched := 0
	skipped := 0
	for _, follow := range follows {
		if follow.State != models.FollowStateAccepted {
			continue
		}

		// Get actor details
		actor, err := r.GetActor(ctx, follow.FollowedUsername)
		if err != nil {
			continue
		}

		// Check if matches query
		if strings.Contains(strings.ToLower(actor.PreferredUsername), query) ||
			strings.Contains(strings.ToLower(actor.Name), query) {
			if skipped < offset {
				skipped++
				continue
			}

			if matched >= limit {
				break
			}

			actors = append(actors, actor)
			matched++
		}
	}

	return actors
}

// GetAccountSuggestions returns suggested accounts for a user to follow
func (r *AccountRepository) GetAccountSuggestions(ctx context.Context, username string, limit int) ([]*storage.AccountSuggestion, error) {
	suggestions := make([]*storage.AccountSuggestion, 0)

	// Get user's existing follows to exclude
	following := r.getFollowingUsernames(ctx, username)
	followingSet := make(map[string]bool)
	for _, f := range following {
		followingSet[f] = true
	}

	// Strategy 1: Popular accounts (high follower count)
	popularSuggestions := r.getPopularAccountSuggestions(ctx, username, followingSet, limit/2)
	suggestions = append(suggestions, popularSuggestions...)

	// Strategy 2: Accounts followed by people you follow
	friendOfFriendSuggestions := r.getFriendOfFriendSuggestions(ctx, username, followingSet, limit/2)
	suggestions = append(suggestions, friendOfFriendSuggestions...)

	// Deduplicate and limit
	seen := make(map[string]bool)
	result := make([]*storage.AccountSuggestion, 0, limit)
	for _, suggestion := range suggestions {
		if !seen[suggestion.Actor.ID] && len(result) < limit {
			seen[suggestion.Actor.ID] = true
			result = append(result, suggestion)
		}
	}

	return result, nil
}

// getPopularAccountSuggestions returns suggestions based on follower count
func (r *AccountRepository) getPopularAccountSuggestions(ctx context.Context, username string, excludeSet map[string]bool, limit int) []*storage.AccountSuggestion {
	suggestions := make([]*storage.AccountSuggestion, 0)

	// Get actors sorted by follower count
	var actors []models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("follower-count-index").
		Where("GSI5PK", "=", "ACTORS_BY_FOLLOWERS").
		Limit(limit * 2). // Get extra to account for filtering
		All(&actors)

	if err != nil {
		r.logger.Error("failed to get popular actors", zap.Error(err))
		return suggestions
	}

	// Filter and convert
	for _, actor := range actors {
		if len(suggestions) >= limit {
			break
		}

		// Skip if already following or is self
		if excludeSet[actor.Username] || actor.Username == username {
			continue
		}

		suggestions = append(suggestions, &storage.AccountSuggestion{
			Actor:         actor.Actor,
			Reason:        "popular",
			Score:         float64(actor.FollowerCount),
			FollowerCount: actor.FollowerCount,
		})
	}

	return suggestions
}

// getFriendOfFriendSuggestions returns accounts followed by people you follow
func (r *AccountRepository) getFriendOfFriendSuggestions(ctx context.Context, username string, excludeSet map[string]bool, limit int) []*storage.AccountSuggestion {
	suggestions := make([]*storage.AccountSuggestion, 0)

	// Get people the user follows
	following := r.getFollowingUsernames(ctx, username)
	if len(following) == 0 {
		return suggestions
	}

	// Count how many of your follows follow each account
	followCounts := make(map[string]int)

	for _, followedUsername := range following {
		// Get who they follow
		theirFollowing := r.getFollowingUsernames(ctx, followedUsername)
		for _, theirFollow := range theirFollowing {
			if !excludeSet[theirFollow] && theirFollow != username {
				followCounts[theirFollow]++
			}
		}
	}

	// Convert to suggestions
	for actorUsername, count := range followCounts {
		if len(suggestions) >= limit {
			break
		}

		if count < 2 { // Require at least 2 mutual connections
			continue
		}

		actor, err := r.GetActor(ctx, actorUsername)
		if err != nil {
			continue
		}

		suggestions = append(suggestions, &storage.AccountSuggestion{
			Actor:           actor,
			Reason:          "friend_of_friend",
			Score:           float64(count),
			MutualFollowers: count,
		})
	}

	return suggestions
}

// GetTrendingActors returns actors that are currently trending
func (r *AccountRepository) GetTrendingActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
	// Get actors with recent high activity
	var actors []models.Actor

	// Use activity index to find trending actors
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("activity-index").
		Where("GSI6PK", "=", "TRENDING_ACTORS").
		Where("GSI6SK", ">", time.Now().Add(-24*time.Hour).Format(time.RFC3339)).
		Limit(limit).
		All(&actors)

	if err != nil {
		r.logger.Error("failed to get trending actors", zap.Error(err))
		return nil, fmt.Errorf("failed to get trending actors: %w", err)
	}

	// Convert to activitypub actors
	result := make([]*activitypub.Actor, len(actors))
	for i, actor := range actors {
		result[i] = actor.Actor
	}

	return result, nil
}

// SearchByWebfinger searches for an actor by webfinger address
func (r *AccountRepository) SearchByWebfinger(ctx context.Context, webfinger string) (*activitypub.Actor, error) {
	// Parse webfinger address (user@domain)
	parts := strings.Split(webfinger, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid webfinger address: %s", webfinger)
	}

	username := parts[0]
	domain := parts[1]

	// If local domain, search locally
	if domain == r.domain {
		return r.GetActor(ctx, username)
	}

	// Search for remote actor
	var actor models.Actor

	err := r.db.WithContext(ctx).Model(&actor).
		Index("webfinger-index").
		Where("GSI7PK", "=", fmt.Sprintf("WEBFINGER#%s", webfinger)).
		First(&actor)

	if err != nil {
		if errors.IsNotFound(err) {
			// Try to fetch from remote if not cached
			// Remote actor lookup is available via federation.RemoteSearchService
			// For now, return not found to maintain current behavior
			return nil, fmt.Errorf("actor not found: %s", webfinger)
		}
		return nil, fmt.Errorf("failed to search by webfinger: %w", err)
	}

	return actor.Actor, nil
}

// CacheRemoteActor caches a remote actor for search
func (r *AccountRepository) CacheRemoteActor(ctx context.Context, actor *activitypub.Actor) error {
	// Extract username and domain from actor ID
	username := actor.PreferredUsername
	domain := extractDomainFromActorID(actor.ID)
	webfinger := fmt.Sprintf("%s@%s", username, domain)

	// Create actor model with remote actor data
	actorModel := &models.Actor{
		Username: webfinger, // Use webfinger as username for remote actors
		Actor:    actor,
		// Remote actors don't have private keys
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set domain for GSI3 (domain-based queries)
	actorModel.GSI3PK = fmt.Sprintf("DOMAIN#%s", domain)
	actorModel.GSI3SK = username

	// Create or update
	err := r.db.WithContext(ctx).Model(actorModel).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			// Update existing
			var existingActor models.Actor
			err = r.db.WithContext(ctx).Model(&existingActor).
				Where("PK", "=", fmt.Sprintf("ACTOR#%s", webfinger)).
				Where("SK", "=", "PROFILE").
				First(&existingActor)
			if err == nil {
				existingActor.Actor = actor
				existingActor.UpdatedAt = time.Now()
				err = r.db.WithContext(ctx).Model(&existingActor).Update()
			}
		}
		if err != nil {
			r.logger.Error("failed to cache remote actor",
				zap.String("webfinger", webfinger),
				zap.Error(err))
			return fmt.Errorf("failed to cache remote actor: %w", err)
		}
	}

	return nil
}

// UpdateLastSeen updates the last seen timestamp for a user
func (r *AccountRepository) UpdateLastSeen(ctx context.Context, username string) error {
	return r.UpdateUser(ctx, username, map[string]interface{}{
		"last_seen_at": time.Now(),
	})
}

// GetActiveUsers returns recently active users
func (r *AccountRepository) GetActiveUsers(ctx context.Context, since time.Time, limit int) ([]*storage.User, error) {
	var users []models.User

	// Use activity index
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("activity-index").
		Where("GSI5PK", "=", "ACTIVE_USERS").
		Where("GSI5SK", ">", since.Format(time.RFC3339)).
		Limit(limit).
		All(&users)

	if err != nil {
		r.logger.Error("failed to get active users", zap.Error(err))
		return nil, fmt.Errorf("failed to get active users: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.User, len(users))
	for i, user := range users {
		result[i] = r.modelToStorageUser(&user)
	}

	return result, nil
}

// GetInactiveUsers returns users who haven't been active recently
func (r *AccountRepository) GetInactiveUsers(ctx context.Context, inactiveSince time.Time, limit int) ([]*storage.User, error) {
	var users []models.User

	// Use activity index (reverse query)
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("activity-index").
		Where("GSI5PK", "=", "ACTIVE_USERS").
		Where("GSI5SK", "<", inactiveSince.Format(time.RFC3339)).
		Limit(limit).
		All(&users)

	if err != nil {
		r.logger.Error("failed to get inactive users", zap.Error(err))
		return nil, fmt.Errorf("failed to get inactive users: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.User, len(users))
	for i, user := range users {
		result[i] = r.modelToStorageUser(&user)
	}

	return result, nil
}

// Helper methods

// getFollowingUsernames returns list of usernames that a user follows
func (r *AccountRepository) getFollowingUsernames(ctx context.Context, username string) []string {
	var follows []models.Follow
	usernames := make([]string, 0)

	err := r.db.WithContext(ctx).Model(&models.Follow{}).
		Where("PK", "=", fmt.Sprintf("FOLLOWER#%s", username)).
		Where("SK", "BEGINS_WITH", "FOLLOWS#").
		All(&follows)

	if err != nil {
		r.logger.Error("failed to get following usernames",
			zap.String("username", username),
			zap.Error(err))
		return usernames
	}

	for _, follow := range follows {
		if follow.State == models.FollowStateAccepted {
			usernames = append(usernames, follow.FollowedUsername)
		}
	}

	return usernames
}

// extractDomainFromActorID extracts domain from an actor ID URL
func extractDomainFromActorID(actorID string) string {
	// Parse URL to extract domain
	// Example: https://mastodon.social/users/alice -> mastodon.social
	parts := strings.Split(actorID, "/")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}
