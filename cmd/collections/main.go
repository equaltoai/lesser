// Package main implements the collections Lambda function for serving ActivityPub federation collections.
package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	// Standardized Lambda initialization with automatic service detection
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "collections",
		LambdaType:  common.LambdaTypeAPI,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	repos = lambdaCtx.Repos.(core.RepositoryStorage)

	// Initialize with default options for API Lambda type
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		logger.Warn("failed to initialize with defaults, some features may be limited", zap.Error(err))
	}
}

// Collection type constants
const (
	collectionTypeFollowers = "followers"
	collectionTypeFollowing = "following"
	collectionTypeLiked     = "liked"
)

// HTTP constants
const (
	contentTypeActivityJSON = "application/activity+json"
	cacheControlMaxAge300   = "max-age=300"
)

// CollectionsHandler handles ActivityPub federation collections using Lift
type CollectionsHandler struct {
	actorRepo        interfaces.ActorRepository
	relationshipRepo interfaces.ConcreteRelationshipRepository
	likeRepo         *repositories.LikeRepository
}

// NewCollectionsHandler creates a new collections handler with standardized initialization
func NewCollectionsHandler() *CollectionsHandler {
	return &CollectionsHandler{
		actorRepo:        repos.Actor(),
		relationshipRepo: repos.Relationship(),
		likeRepo:         repos.Like(),
	}
}

// RegisterRoutes registers all collections routes
func (ch *CollectionsHandler) RegisterRoutes(app *lift.App) {
	// ActivityPub federation collection endpoints
	_ = app.GET("/users/:username/followers", ch.handleFollowersCollection)
	_ = app.GET("/users/:username/following", ch.handleFollowingCollection)
	_ = app.GET("/users/:username/liked", ch.handleLikedCollection)
}

// handleFollowersCollection handles the followers collection endpoint
func (ch *CollectionsHandler) handleFollowersCollection(ctx *lift.Context) error {
	return ch.handleCollection(ctx, "followers")
}

// handleFollowingCollection handles the following collection endpoint
func (ch *CollectionsHandler) handleFollowingCollection(ctx *lift.Context) error {
	return ch.handleCollection(ctx, "following")
}

// handleLikedCollection handles the liked collection endpoint
func (ch *CollectionsHandler) handleLikedCollection(ctx *lift.Context) error {
	return ch.handleCollection(ctx, "liked")
}

// handleCollection handles ActivityPub collection requests
func (ch *CollectionsHandler) handleCollection(ctx *lift.Context, collectionType string) error {
	// Extract username from path parameters
	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return lift.ValidationError("missing username")
	}

	state, stateErr := repos.Instance().GetInstanceState(ctx.Context)
	bootstrapUsername := storageModels.DefaultBootstrapUsername
	if stateErr == nil && strings.TrimSpace(state.BootstrapUsername) != "" {
		bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
	}
	locked := stateErr != nil || state.Locked
	if locked && strings.EqualFold(username, bootstrapUsername) {
		return lift.NewLiftError("FORBIDDEN", "bootstrap actor is not available while instance is locked", 403)
	}

	// Get request ID from context
	requestID := ctx.Get("requestID")
	if requestID == nil {
		requestID = "unknown"
	}

	logger.Info("processing collections request",
		zap.String("username", username),
		zap.String("collection_type", collectionType),
		zap.Any("request_id", requestID))

	// Check if actor exists using DynamORM repository
	actor, err := ch.actorRepo.GetActor(ctx.Context, username)
	if err != nil {
		if common.IsNotFound(err) {
			logger.Debug("actor not found",
				zap.String("username", username))
			return lift.NotFound("actor not found")
		}
		logger.Error("failed to get actor", zap.Error(err))
		return lift.NewLiftError("DATABASE_ERROR", "database error", 500)
	}

	// Log privacy settings for observability
	logger.Debug("processing collection request",
		zap.String("username", username),
		zap.String("collection_type", collectionType),
		zap.Bool("manually_approves_followers", actor.ManuallyApprovesFollowers),
		zap.Any("request_id", requestID))

	// Parse query parameters
	isPage := ctx.Query("page") != ""
	cursor := ctx.Query("cursor")
	direction := ctx.Query("dir") // Check for direction parameter
	limit := 20                   // default
	if l := ctx.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed >= 1 && parsed <= 100 {
			limit = parsed
		}
	}

	// While locked, collections should be reachable but empty for content-bearing collections.
	if locked && collectionType == collectionTypeLiked {
		collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
		if !isPage {
			ctx.Response.Headers["Content-Type"] = contentTypeActivityJSON
			ctx.Response.Headers["Cache-Control"] = cacheControlMaxAge300
			return ctx.JSON(&activitypub.OrderedCollection{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      collectionID,
						Type:    activitypub.OrderedCollectionType,
					},
					TotalItems: 0,
				},
			})
		}

		ctx.Response.Headers["Content-Type"] = contentTypeActivityJSON
		ctx.Response.Headers["Cache-Control"] = cacheControlMaxAge300
		return ctx.JSON(&activitypub.OrderedCollectionPage{
			CollectionPage: activitypub.CollectionPage{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      fmt.Sprintf("%s?page=1", collectionID),
						Type:    activitypub.OrderedCollectionPageType,
					},
					OrderedItems: []any{},
				},
				PartOf: collectionID,
			},
		})
	}

	// If not requesting a page, return the collection metadata
	if !isPage {
		return ch.returnCollection(ctx, actor, collectionType)
	}

	// Handle reverse pagination if dir=prev is specified
	if direction == "prev" {
		return ch.handleReverseDirection(ctx, actor, collectionType, username, cursor, limit)
	}

	// Get relationships based on type
	var usernames []string
	var likes []*storage.Like
	var nextCursor string

	switch collectionType {
	case collectionTypeFollowers:
		usernames, nextCursor, err = ch.relationshipRepo.GetFollowers(ctx.Context, username, limit, cursor)
	case collectionTypeFollowing:
		usernames, nextCursor, err = ch.relationshipRepo.GetFollowing(ctx.Context, username, limit, cursor)
	case collectionTypeLiked:
		// For liked collection, we get Like objects and convert to storage.Like
		modelLikes, likesNextCursor, err := ch.likeRepo.GetActorLikes(ctx.Context, actor.ID, limit, cursor)
		if err == nil {
			nextCursor = likesNextCursor
			// Convert models.Like to storage.Like
			likes = make([]*storage.Like, len(modelLikes))
			for i, modelLike := range modelLikes {
				likes[i] = &storage.Like{
					ID:        modelLike.ID,
					Actor:     modelLike.Actor,
					Object:    modelLike.Object,
					CreatedAt: modelLike.CreatedAt,
				}
			}
		}
	}

	if err != nil {
		logger.Error("failed to get relationships",
			zap.String("type", collectionType),
			zap.Error(err))
		return lift.NewLiftError("DATABASE_ERROR", "failed to retrieve collection data", 500)
	}

	// Build and return page
	return ch.returnCollectionPage(ctx, actor, collectionType, usernames, likes, cursor, nextCursor, limit)
}

// returnCollection returns the collection metadata (not a page)
func (ch *CollectionsHandler) returnCollection(ctx *lift.Context, actor *activitypub.Actor, collectionType string) error {
	// Get total count using proper count methods
	var hasItems bool
	var itemCount int

	switch collectionType {
	case collectionTypeFollowers:
		count, err := ch.relationshipRepo.CountFollowers(ctx.Context, actor.PreferredUsername)
		if err != nil {
			logger.Error("failed to get followers count", zap.Error(err))
			return lift.NewLiftError("DATABASE_ERROR", "failed to get collection count", 500)
		}
		hasItems = count > 0
		itemCount = count
	case collectionTypeFollowing:
		count, err := ch.relationshipRepo.CountFollowing(ctx.Context, actor.PreferredUsername)
		if err != nil {
			logger.Error("failed to get following count", zap.Error(err))
			return lift.NewLiftError("DATABASE_ERROR", "failed to get collection count", 500)
		}
		hasItems = count > 0
		itemCount = count
	case collectionTypeLiked:
		count, err := ch.likeRepo.CountActorLikes(ctx.Context, actor.ID)
		if err != nil {
			logger.Error("failed to get liked count", zap.Error(err))
			return lift.NewLiftError("DATABASE_ERROR", "failed to get collection count", 500)
		}
		hasItems = count > 0
		itemCount = int(count)
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
		collection.First = fmt.Sprintf("%s?page=1", collectionID)
	}

	// Set ActivityPub content type and caching headers
	ctx.Response.Headers["Content-Type"] = contentTypeActivityJSON
	ctx.Response.Headers["Cache-Control"] = cacheControlMaxAge300
	return ctx.JSON(collection)
}

// returnCollectionPage returns a page of the collection
func (ch *CollectionsHandler) returnCollectionPage(ctx *lift.Context, actor *activitypub.Actor, collectionType string, usernames []string, likes []*storage.Like, cursor, nextCursor string, limit int) error {
	logger.Debug("returning collection page",
		zap.String("actor", actor.ID),
		zap.String("collection_type", collectionType),
		zap.Int("usernames_count", len(usernames)),
		zap.Int("likes_count", len(likes)))

	// Build collection and page URLs
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
	pageID := fmt.Sprintf("%s?page=1", collectionID)
	if cursor != "" {
		pageID = fmt.Sprintf("%s&cursor=%s", pageID, cursor)
	}

	// Convert to appropriate URLs based on collection type
	var orderedItems []any

	if collectionType == "liked" {
		// For liked collection, we use the object IDs from likes
		orderedItems = make([]any, len(likes))
		for i, like := range likes {
			orderedItems[i] = like.Object
		}
	} else {
		// For followers/following, convert usernames to actor URLs
		orderedItems = make([]any, len(usernames))
		for i, username := range usernames {
			// Use full HTTPS URL format
			if strings.HasPrefix(cfg.Domain, "http") {
				orderedItems[i] = fmt.Sprintf("%s/users/%s", cfg.Domain, username)
			} else {
				orderedItems[i] = fmt.Sprintf("https://%s/users/%s", cfg.Domain, username)
			}
		}
	}

	// Build the page
	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      pageID,
					Type:    activitypub.OrderedCollectionPageType,
				},
				OrderedItems: orderedItems,
			},
			PartOf: collectionID,
		},
	}

	// Add next link if there are more items
	if nextCursor != "" {
		page.Next = fmt.Sprintf("%s?page=1&cursor=%s&limit=%d", collectionID, nextCursor, limit)
	}

	// Add prev link if this is not the first page
	if cursor != "" {
		// Generate previous page cursor for reverse pagination
		prevCursor := ch.generatePreviousCursor(cursor, collectionType, usernames, likes)
		if prevCursor != "" {
			page.Prev = fmt.Sprintf("%s?page=1&cursor=%s&limit=%d&dir=prev", collectionID, prevCursor, limit)
		} else {
			// If we can't generate a proper reverse cursor, link to the first page
			page.Prev = fmt.Sprintf("%s?page=1", collectionID)
		}
	}

	// Set ActivityPub content type and caching headers
	ctx.Response.Headers["Content-Type"] = contentTypeActivityJSON
	ctx.Response.Headers["Cache-Control"] = cacheControlMaxAge300
	return ctx.JSON(page)
}

// generatePreviousCursor generates a cursor for reverse pagination
func (ch *CollectionsHandler) generatePreviousCursor(_ string, collectionType string, usernames []string, likes []*storage.Like) string {
	// For reverse pagination, we need to create a cursor that points to the item before the first item in the current page
	// This is collection-type specific

	if err := common.ValidateSliceNotEmpty("usernames", usernames); err != nil && common.ValidateSliceNotEmpty("likes", likes) != nil {
		return ""
	}

	switch collectionType {
	case collectionTypeFollowers, collectionTypeFollowing:
		if err := common.ValidateSliceNotEmpty("usernames", usernames); err == nil {
			// Use the first username as the reverse cursor
			// This assumes the cursor format is based on username ordering
			firstUsername := usernames[0]
			// Create a reverse cursor that would fetch items before this username
			return fmt.Sprintf("before_%s", firstUsername)
		}
	case collectionTypeLiked:
		if err := common.ValidateSliceNotEmpty("likes", likes); err == nil {
			// Use the first like's ID or timestamp for reverse cursor
			firstLike := likes[0]
			// Create a reverse cursor that would fetch items before this like
			return fmt.Sprintf("before_%s", firstLike.ID)
		}
	}

	return ""
}

// handleReverseDirection handles reverse pagination when dir=prev is specified
func (ch *CollectionsHandler) handleReverseDirection(ctx *lift.Context, actor *activitypub.Actor, collectionType, username, cursor string, limit int) error {
	// For reverse pagination, we need to fetch items in reverse order
	// This would typically involve modifying the query to sort in the opposite direction
	// and then reversing the results

	var usernames []string
	var likes []*storage.Like
	var nextCursor string
	var err error

	// Extract the actual cursor value (remove the "before_" prefix if present)
	actualCursor := strings.TrimPrefix(cursor, "before_")

	switch collectionType {
	case collectionTypeFollowers:
		// For reverse pagination, we'd need a special method that fetches in reverse order
		// Since we don't have that, we'll simulate it by fetching forward with a modified cursor
		usernames, nextCursor, err = ch.relationshipRepo.GetFollowers(ctx.Context, username, limit, actualCursor)
		// Reverse the order of results for reverse pagination
		if err == nil {
			ch.reverseStringSlice(usernames)
		}
	case collectionTypeFollowing:
		usernames, nextCursor, err = ch.relationshipRepo.GetFollowing(ctx.Context, username, limit, actualCursor)
		if err == nil {
			ch.reverseStringSlice(usernames)
		}
	case collectionTypeLiked:
		modelLikes, likesNextCursor, err := ch.likeRepo.GetActorLikes(ctx.Context, actor.ID, limit, actualCursor)
		if err == nil {
			nextCursor = likesNextCursor
			// Convert and reverse
			likes = make([]*storage.Like, len(modelLikes))
			for i, modelLike := range modelLikes {
				likes[i] = &storage.Like{
					ID:        modelLike.ID,
					Actor:     modelLike.Actor,
					Object:    modelLike.Object,
					CreatedAt: modelLike.CreatedAt,
				}
			}
			ch.reverseLikeSlice(likes)
		}
	}

	if err != nil {
		logger.Error("failed to get relationships for reverse pagination",
			zap.String("type", collectionType),
			zap.Error(err))
		return lift.NewLiftError("DATABASE_ERROR", "failed to retrieve collection data", 500)
	}

	// For reverse pagination, the "next" cursor becomes previous, and we generate a new next cursor
	prevCursor := nextCursor
	nextCursor = ch.generateNextCursorForReverse(collectionType, usernames, likes)

	// Build and return page with swapped navigation
	return ch.returnCollectionPageReverse(ctx, actor, collectionType, usernames, likes, cursor, prevCursor, nextCursor, limit)
}

// reverseStringSlice reverses a slice of strings in place
func (ch *CollectionsHandler) reverseStringSlice(slice []string) {
	for i, j := 0, len(slice)-1; i < j; i, j = i+1, j-1 {
		slice[i], slice[j] = slice[j], slice[i]
	}
}

// reverseLikeSlice reverses a slice of likes in place
func (ch *CollectionsHandler) reverseLikeSlice(slice []*storage.Like) {
	for i, j := 0, len(slice)-1; i < j; i, j = i+1, j-1 {
		slice[i], slice[j] = slice[j], slice[i]
	}
}

// generateNextCursorForReverse generates a next cursor when doing reverse pagination
func (ch *CollectionsHandler) generateNextCursorForReverse(collectionType string, usernames []string, likes []*storage.Like) string {
	switch collectionType {
	case collectionTypeFollowers, collectionTypeFollowing:
		if err := common.ValidateSliceNotEmpty("usernames", usernames); err == nil {
			return fmt.Sprintf("after_%s", usernames[len(usernames)-1])
		}
	case collectionTypeLiked:
		if err := common.ValidateSliceNotEmpty("likes", likes); err == nil {
			return fmt.Sprintf("after_%s", likes[len(likes)-1].ID)
		}
	}
	return ""
}

// returnCollectionPageReverse returns a page with reverse pagination links
func (ch *CollectionsHandler) returnCollectionPageReverse(ctx *lift.Context, actor *activitypub.Actor, collectionType string, usernames []string, likes []*storage.Like, cursor, prevCursor, nextCursor string, limit int) error {
	// Similar to returnCollectionPage but with swapped prev/next logic for reverse pagination
	logger.Debug("returning reverse collection page",
		zap.String("actor", actor.ID),
		zap.String("collection_type", collectionType),
		zap.Int("usernames_count", len(usernames)),
		zap.Int("likes_count", len(likes)))

	// Build collection and page URLs
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
	pageID := fmt.Sprintf("%s?page=1&dir=prev", collectionID)
	if cursor != "" {
		pageID = fmt.Sprintf("%s&cursor=%s", pageID, cursor)
	}

	// Convert to appropriate URLs based on collection type
	var orderedItems []any

	if collectionType == "liked" {
		orderedItems = make([]any, len(likes))
		for i, like := range likes {
			orderedItems[i] = like.Object
		}
	} else {
		orderedItems = make([]any, len(usernames))
		for i, username := range usernames {
			if strings.HasPrefix(cfg.Domain, "http") {
				orderedItems[i] = fmt.Sprintf("%s/users/%s", cfg.Domain, username)
			} else {
				orderedItems[i] = fmt.Sprintf("https://%s/users/%s", cfg.Domain, username)
			}
		}
	}

	// Build the page
	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      pageID,
					Type:    activitypub.OrderedCollectionPageType,
				},
				OrderedItems: orderedItems,
			},
			PartOf: collectionID,
		},
	}

	// For reverse pagination: next goes forward, prev goes further back
	if nextCursor != "" {
		page.Next = fmt.Sprintf("%s?page=1&cursor=%s&limit=%d", collectionID, nextCursor, limit)
	}

	if prevCursor != "" {
		page.Prev = fmt.Sprintf("%s?page=1&cursor=%s&limit=%d&dir=prev", collectionID, prevCursor, limit)
	}

	// Set ActivityPub content type and caching headers
	ctx.Response.Headers["Content-Type"] = contentTypeActivityJSON
	ctx.Response.Headers["Cache-Control"] = cacheControlMaxAge300
	return ctx.JSON(page)
}

func main() {
	// Create the handler with standardized services
	handler := NewCollectionsHandler()

	// Create new Lift app
	app := lift.New()

	// Panic recovery middleware (MUST be first to catch all panics)
	app.Use(middleware.PanicRecovery(lambdaCtx.Logger))

	// Add request ID middleware (first - generates request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("collections-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware (second - logs with request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			path := ctx.Request.Path
			method := ctx.Request.Method

			err := next.Handle(ctx)

			logger.Info("collections request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			return err
		})
	})

	// Add recovery middleware (third - catches panics)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered in collections handler",
						zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
						zap.Any("panic", r))
					if err := ctx.Status(500).Text("Internal server error"); err != nil {
						logger.Error("failed to send error response", zap.Error(err))
					}
				}
			}()

			return next.Handle(ctx)
		})
	})

	// Note: Federation rate limiting removed - using Limited library approach in API service only
	// Federation endpoints rely on ActivityPub HTTP signatures for authentication

	// Add CORS middleware for ActivityPub federation compatibility
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers for ActivityPub federation
			ctx.Response.Headers["Access-Control-Allow-Origin"] = "*"
			ctx.Response.Headers["Access-Control-Allow-Methods"] = "GET, HEAD, OPTIONS"
			ctx.Response.Headers["Access-Control-Allow-Headers"] = "Accept, Authorization, Content-Type, Signature, Date, Digest"

			// Handle preflight requests
			if ctx.Request.Method == "OPTIONS" {
				return ctx.Status(http.StatusNoContent).JSON(nil)
			}

			return next.Handle(ctx)
		})
	})

	// Register all collections routes
	handler.RegisterRoutes(app)

	// Use standardized Lambda handler with observability
	standardHandler := lambdaCtx.CreateStandardizedLambdaHandler(func(ctx context.Context, event interface{}) (interface{}, error) {
		return app.HandleRequest(ctx, event)
	})

	lambda.Start(standardHandler)
}
