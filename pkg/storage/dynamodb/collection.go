package dynamodb

import (
	"context"
	"fmt"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"go.uber.org/zap"
)

// GetCollection retrieves a collection for an actor (followers, following, etc.)
func (s *dynamoDBStorage) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	log := common.WithContext(ctx).With(
		zap.String("username", username),
		zap.String("collection_type", collectionType),
		zap.Int("limit", limit),
	)

	// Get the base URL for constructing IDs
	baseURL := config.Get().BaseURL()
	actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
	collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

	// Create the base collection page
	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      collectionID,
					Type:    activitypub.OrderedCollectionType,
				},
			},
			PartOf: collectionID,
		},
	}

	// Get the actual collection items based on type
	switch collectionType {
	case activitypub.FollowersCollection:
		followers, nextCursor, err := s.GetFollowers(ctx, username, limit, cursor)
		if err != nil {
			log.Error("failed to get followers", zap.Error(err))
			return nil, fmt.Errorf("failed to get followers: %w", err)
		}

		// Convert usernames to actor IDs
		items := make([]any, len(followers))
		for i, follower := range followers {
			items[i] = fmt.Sprintf("%s/users/%s", baseURL, follower)
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		// Get total count (approximate)
		// In a real implementation, you might want to cache this
		page.TotalItems = len(items)

	case activitypub.FollowingCollection:
		following, nextCursor, err := s.GetFollowing(ctx, username, limit, cursor)
		if err != nil {
			log.Error("failed to get following", zap.Error(err))
			return nil, fmt.Errorf("failed to get following: %w", err)
		}

		// Convert usernames to actor IDs
		items := make([]any, len(following))
		for i, followed := range following {
			items[i] = fmt.Sprintf("%s/users/%s", baseURL, followed)
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		// Get total count (approximate)
		page.TotalItems = len(items)

	case activitypub.LikedCollection:
		// Get the liked posts for this user
		likes, nextCursor, err := s.GetActorLikes(ctx, fmt.Sprintf("%s/users/%s", baseURL, username), limit, cursor)
		if err != nil {
			log.Error("failed to get liked posts", zap.Error(err))
			return nil, fmt.Errorf("failed to get liked posts: %w", err)
		}

		// Convert likes to object IDs
		items := make([]any, len(likes))
		for i, like := range likes {
			items[i] = like.Object
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		// Get total count (approximate)
		page.TotalItems = len(items)

	case activitypub.InboxCollection, activitypub.OutboxCollection:
		// For inbox/outbox collections, get activities
		var activities []*activitypub.Activity
		var nextCursor string
		var err error

		if collectionType == activitypub.InboxCollection {
			activities, nextCursor, err = s.GetInboxActivities(ctx, username, limit, cursor)
		} else {
			activities, nextCursor, err = s.GetOutboxActivities(ctx, username, limit, cursor)
		}

		if err != nil {
			log.Error("failed to get activities", zap.Error(err))
			return nil, fmt.Errorf("failed to get activities: %w", err)
		}

		// Convert activities to interfaces
		items := make([]any, len(activities))
		for i, activity := range activities {
			items[i] = activity
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		// Get total count (approximate)
		page.TotalItems = len(items)

	default:
		// For unknown collection types, return empty
		log.Warn("unknown collection type requested", zap.String("type", collectionType))
		page.OrderedItems = []any{}
		page.TotalItems = 0
	}

	log.Info("retrieved collection",
		zap.String("type", collectionType),
		zap.Int("items", len(page.OrderedItems.([]any))),
	)

	return page, nil
}
