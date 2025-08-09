// Package main implements the collections Lambda function for serving ActivityPub federation collections.
package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Collection type constants
const (
	collectionTypeFollowers = "followers"
	collectionTypeFollowing = "following"
	collectionTypeLiked     = "liked"
)

// CollectionsHandler handles ActivityPub federation collections using Lift
type CollectionsHandler struct {
	actorRepo        *repositories.ActorRepository
	relationshipRepo *repositories.RelationshipRepository
	likeRepo         *repositories.LikeRepository
	logger           *zap.Logger
	cfg              *config.Config
}

// NewCollectionsHandler creates a new collections handler with DynamORM repositories
func NewCollectionsHandler() (*CollectionsHandler, error) {
	logger := common.Logger()
	cfg := config.Get()

	// Initialize DynamORM database connection using the established pattern
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM database: %w", err)
	}

	// Initialize repositories
	tableName := cfg.DynamoTableName
	if tableName == "" {
		tableName = "lesser-main" // Default table name
	}
	actorRepo := repositories.NewActorRepository(db, tableName, logger)
	relationshipRepo := repositories.NewRelationshipRepository(db, tableName, logger)
	likeRepo := repositories.NewLikeRepository(db, tableName, logger)

	return &CollectionsHandler{
		actorRepo:        actorRepo,
		relationshipRepo: relationshipRepo,
		likeRepo:         likeRepo,
		logger:           logger,
		cfg:              cfg,
	}, nil
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
	if username == "" {
		return lift.ValidationError("missing username")
	}

	// Get request ID from context
	requestID := ctx.Get("requestID")
	if requestID == nil {
		requestID = "unknown"
	}

	ch.logger.Info("processing collections request",
		zap.String("username", username),
		zap.String("collection_type", collectionType),
		zap.Any("request_id", requestID))

	// Check if actor exists using DynamORM repository
	actor, err := ch.actorRepo.GetActor(ctx.Context, username)
	if err != nil {
		if common.IsNotFound(err) {
			ch.logger.Debug("actor not found",
				zap.String("username", username))
			return lift.NotFound("actor not found")
		}
		ch.logger.Error("failed to get actor", zap.Error(err))
		return lift.NewLiftError("DATABASE_ERROR", "database error", 500)
	}

	// Parse query parameters
	isPage := ctx.Query("page") == "true"
	cursor := ctx.Query("cursor")
	limit := 20 // default
	if l := ctx.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed >= 1 && parsed <= 100 {
			limit = parsed
		}
	}

	// If not requesting a page, return the collection metadata
	if !isPage {
		return ch.returnCollection(ctx, actor, collectionType)
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
		ch.logger.Error("failed to get relationships",
			zap.String("type", collectionType),
			zap.Error(err))
		return lift.NewLiftError("DATABASE_ERROR", "failed to retrieve collection data", 500)
	}

	// Build and return page
	return ch.returnCollectionPage(ctx, actor, collectionType, usernames, likes, cursor, nextCursor, limit)
}

// returnCollection returns the collection metadata (not a page)
func (ch *CollectionsHandler) returnCollection(ctx *lift.Context, actor *activitypub.Actor, collectionType string) error {
	// Get total count (we'll get a small sample to determine if there are any items)
	// In a production system, you might want to add a count method to storage
	var hasItems bool
	var itemCount int

	switch collectionType {
	case collectionTypeFollowers:
		usernames, _, err := ch.relationshipRepo.GetFollowers(ctx.Context, actor.PreferredUsername, 1, "")
		if err != nil {
			ch.logger.Error("failed to get count", zap.Error(err))
			return lift.NewLiftError("DATABASE_ERROR", "failed to get collection count", 500)
		}
		hasItems = len(usernames) > 0
		itemCount = len(usernames)
	case collectionTypeFollowing:
		usernames, _, err := ch.relationshipRepo.GetFollowing(ctx.Context, actor.PreferredUsername, 1, "")
		if err != nil {
			ch.logger.Error("failed to get count", zap.Error(err))
			return lift.NewLiftError("DATABASE_ERROR", "failed to get collection count", 500)
		}
		hasItems = len(usernames) > 0
		itemCount = len(usernames)
	case collectionTypeLiked:
		likes, _, err := ch.likeRepo.GetActorLikes(ctx.Context, actor.ID, 1, "")
		if err != nil {
			ch.logger.Error("failed to get count", zap.Error(err))
			return lift.NewLiftError("DATABASE_ERROR", "failed to get collection count", 500)
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

	// Set ActivityPub content type and caching headers
	ctx.Response.Headers["Content-Type"] = "application/activity+json"
	ctx.Response.Headers["Cache-Control"] = "max-age=300"
	return ctx.JSON(collection)
}

// returnCollectionPage returns a page of the collection
func (ch *CollectionsHandler) returnCollectionPage(ctx *lift.Context, actor *activitypub.Actor, collectionType string, usernames []string, likes []*storage.Like, cursor, nextCursor string, limit int) error {
	ch.logger.Debug("returning collection page",
		zap.String("actor", actor.ID),
		zap.String("collection_type", collectionType),
		zap.Int("usernames_count", len(usernames)),
		zap.Int("likes_count", len(likes)))

	// Build collection and page URLs
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
	pageID := fmt.Sprintf("%s?page=true", collectionID)
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
			orderedItems[i] = fmt.Sprintf("%s/users/%s", ch.cfg.Domain, username)
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

	// Set ActivityPub content type and caching headers
	ctx.Response.Headers["Content-Type"] = "application/activity+json"
	ctx.Response.Headers["Cache-Control"] = "max-age=300"
	return ctx.JSON(page)
}

func main() {
	// Create the handler
	handler, err := NewCollectionsHandler()
	if err != nil {
		panic(fmt.Sprintf("failed to create handler: %v", err))
	}

	// Create new Lift app
	app := lift.New()

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

			handler.logger.Info("collections request completed",
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
					handler.logger.Error("panic recovered in collections handler",
						zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
						zap.Any("panic", r))
					if err := ctx.Status(500).Text("Internal server error"); err != nil {
						handler.logger.Error("failed to send error response", zap.Error(err))
					}
				}
			}()

			return next.Handle(ctx)
		})
	})

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

	// Use app.HandleRequest for Lambda (not app.Start())
	lambda.Start(app.HandleRequest)
}
