package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg    *config.Config
	store  storage.Storage
	logger *zap.Logger
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}
}

func handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	log := common.WithContext(ctx)

	// Ensure this is a GET request
	if request.RequestContext.HTTP.Method != http.MethodGet {
		return common.MethodNotAllowed(fmt.Errorf("method %s not allowed", request.RequestContext.HTTP.Method)), nil
	}

	username := request.PathParameters["username"]
	if username == "" {
		return common.BadRequest(errors.New("missing username")), nil
	}

	// Extract collection type from path
	// Path should be /users/{username}/followers or /users/{username}/following
	path := request.RawPath
	// Remove stage prefix if present
	if request.RequestContext.Stage != "" && strings.HasPrefix(path, "/"+request.RequestContext.Stage) {
		path = strings.TrimPrefix(path, "/"+request.RequestContext.Stage)
	}

	var collectionType string
	if strings.HasSuffix(path, "/followers") {
		collectionType = "followers"
	} else if strings.HasSuffix(path, "/following") {
		collectionType = "following"
	} else if strings.HasSuffix(path, "/liked") {
		collectionType = "liked"
	} else {
		return common.NotFound(errors.New("unknown collection")), nil
	}

	// Check if actor exists
	actor, err := store.GetActor(ctx, username)
	if err != nil {
		if common.IsNotFound(err) {
			return common.NotFound(err), nil
		}
		log.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Parse query parameters
	isPage := request.QueryStringParameters["page"] == "true"
	cursor := request.QueryStringParameters["cursor"]
	limit := 20 // default
	if l := request.QueryStringParameters["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed >= 1 && parsed <= 100 {
			limit = parsed
		}
	}

	// If not requesting a page, return the collection metadata
	if !isPage {
		return returnCollection(ctx, actor, collectionType)
	}

	// Get relationships based on type
	var usernames []string
	var likes []*storage.Like
	var nextCursor string

	if collectionType == "followers" {
		usernames, nextCursor, err = store.GetFollowers(ctx, username, limit, cursor)
	} else if collectionType == "following" {
		usernames, nextCursor, err = store.GetFollowing(ctx, username, limit, cursor)
	} else {
		// For liked collection, we get Like objects
		likes, nextCursor, err = store.GetActorLikes(ctx, actor.ID, limit, cursor)
	}

	if err != nil {
		log.Error("failed to get relationships",
			zap.String("type", collectionType),
			zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Build and return page
	return returnCollectionPage(ctx, actor, collectionType, usernames, likes, cursor, nextCursor, limit)
}

// returnCollection returns the collection metadata (not a page)
func returnCollection(ctx context.Context, actor *activitypub.Actor, collectionType string) (*events.APIGatewayV2HTTPResponse, error) {
	log := common.WithContext(ctx)

	// Get total count (we'll get a small sample to determine if there are any items)
	// In a production system, you might want to add a count method to storage
	var hasItems bool
	var itemCount int

	if collectionType == "followers" {
		usernames, _, err := store.GetFollowers(ctx, actor.PreferredUsername, 1, "")
		if err != nil {
			log.Error("failed to get count", zap.Error(err))
			return common.InternalServerError(err), nil
		}
		hasItems = len(usernames) > 0
		itemCount = len(usernames)
	} else if collectionType == "following" {
		usernames, _, err := store.GetFollowing(ctx, actor.PreferredUsername, 1, "")
		if err != nil {
			log.Error("failed to get count", zap.Error(err))
			return common.InternalServerError(err), nil
		}
		hasItems = len(usernames) > 0
		itemCount = len(usernames)
	} else {
		likes, _, err := store.GetActorLikes(ctx, actor.ID, 1, "")
		if err != nil {
			log.Error("failed to get count", zap.Error(err))
			return common.InternalServerError(err), nil
		}
		hasItems = len(likes) > 0
		itemCount = len(likes)
	}

	// Build collection URL
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)

	// Build the collection
	collection := &activitypub.OrderedCollection{
		Collection: activitypub.Collection{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				ID:      collectionID,
				Type:    activitypub.OrderedCollectionType,
			},
			TotalItems: itemCount,
		},
	}

	// Only add first page link if there are items
	if hasItems {
		collection.First = fmt.Sprintf("%s?page=true", collectionID)
	}

	return common.JSONResponse(http.StatusOK, collection), nil
}

// returnCollectionPage returns a page of the collection
func returnCollectionPage(ctx context.Context, actor *activitypub.Actor, collectionType string, usernames []string, likes []*storage.Like, cursor, nextCursor string, limit int) (*events.APIGatewayV2HTTPResponse, error) {
	log := common.WithContext(ctx)
	log.Debug("returning collection page",
		zap.String("actor", actor.ID),
		zap.String("collection_type", collectionType),
		zap.Int("item_count", len(usernames)))

	// Build collection and page URLs
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
	pageID := fmt.Sprintf("%s?page=true", collectionID)
	if cursor != "" {
		pageID = fmt.Sprintf("%s&cursor=%s", pageID, cursor)
	}

	// Convert to appropriate URLs based on collection type
	var orderedItems []interface{}

	if collectionType == "liked" {
		// For liked collection, we use the object IDs from likes
		orderedItems = make([]interface{}, len(likes))
		for i, like := range likes {
			orderedItems[i] = like.Object
		}
	} else {
		// For followers/following, convert usernames to actor URLs
		orderedItems = make([]interface{}, len(usernames))
		for i, username := range usernames {
			orderedItems[i] = fmt.Sprintf("%s/users/%s", cfg.Domain, username)
		}
	}

	// Build the page
	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      pageID,
					Type:    "OrderedCollectionPage", // The constant doesn't exist, use string literal
				},
				OrderedItems: orderedItems,
			},
			PartOf: collectionID,
		},
	}

	// Add next link if there are more items
	if nextCursor != "" {
		page.Next = fmt.Sprintf("%s?page=true&cursor=%s&limit=%d", collectionID, nextCursor, limit)
	}

	// Add prev link if this is not the first page
	if cursor != "" {
		// In a real implementation, you'd need to implement reverse pagination
		// For now, we'll just indicate that there are previous items
		page.Prev = fmt.Sprintf("%s?page=true", collectionID)
	}

	return common.JSONResponse(http.StatusOK, page), nil
}

func main() {
	lambda.Start(handler)
}
