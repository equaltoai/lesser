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
		items := make([]interface{}, len(followers))
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
		items := make([]interface{}, len(following))
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

	default:
		// For other collection types (liked, etc.), return empty for now
		page.OrderedItems = []interface{}{}
		page.TotalItems = 0
	}

	log.Info("retrieved collection",
		zap.String("type", collectionType),
		zap.Int("items", len(page.OrderedItems.([]interface{}))),
	)

	return page, nil
}
