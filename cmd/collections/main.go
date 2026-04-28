// Package main implements the collections Lambda function for serving ActivityPub federation collections.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/crawler"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
)

var (
	mustInitializeLambdaFn     = common.MustInitializeLambda
	initializeWithDefaultsFn   = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	lambdaStartFn              = lambda.Start
	newCollectionsHandlerFn    = NewCollectionsHandler
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeCollections()
}

func initializeCollections() {
	// Standardized Lambda initialization with automatic service detection
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "collections",
		LambdaType:  common.LambdaTypeAPI,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Initialize with default options for API Lambda type (best-effort; some lambdas still do manual wiring).
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	storage, ok := lambdaCtx.Repos.(core.RepositoryStorage)
	if !ok || storage == nil {
		logger.Warn("lambda context repository missing after initialization, attempting manual storage initialization")
		initializeManualStorage()
		storage, ok = lambdaCtx.Repos.(core.RepositoryStorage)
	}
	if !ok || storage == nil {
		logger.Fatal("lambda context repository is not core.RepositoryStorage")
	}
	repos = storage
}

func initializeManualStorage() {
	if lambdaCtx == nil {
		logger.Fatal("manual storage initialization requires lambda context")
	}

	tableName := strings.TrimSpace(cfg.DynamoTableName)
	if tableName == "" {
		logger.Fatal("DYNAMODB_TABLE environment variable is required for collections lambda")
	}

	db, err := newLambdaOptimizedClientFn(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("failed to initialize DynamORM", zap.Error(err))
	}

	storage, err := newRepositoryFactoryFn(db, tableName, logger)
	if err != nil {
		logger.Fatal("failed to initialize repository factory", zap.Error(err))
	}

	lambdaCtx.DynamoDB = db
	lambdaCtx.Repos = storage
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

// CollectionsHandler handles ActivityPub federation collections.
type CollectionsHandler struct {
	actorRepo        collectionsActorRepo
	relationshipRepo collectionsRelationshipRepo
	likeRepo         collectionsLikeRepo
	instanceRepo     instanceStateGetter
	remoteResolver   collectionsRemoteResolver
}

type collectionsActorRepo interface {
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error)
}

type collectionsRelationshipRepo interface {
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	CountFollowers(ctx context.Context, username string) (int, error)
	CountFollowing(ctx context.Context, username string) (int, error)
}

type collectionsLikeRepo interface {
	GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*storageModels.Like, string, error)
	CountActorLikes(ctx context.Context, actorID string) (int64, error)
}

type instanceStateGetter interface {
	GetInstanceState(ctx context.Context) (*storageModels.InstanceState, error)
}

type collectionsRemoteResolver interface {
	ResolveExactActor(ctx context.Context, input, localDomain string) (*federation.ExactActorResolution, error)
}

// NewCollectionsHandler creates a new collections handler with standardized initialization
func NewCollectionsHandler() *CollectionsHandler {
	var remoteResolver collectionsRemoteResolver
	if repos != nil {
		remoteResolver = federation.NewRemoteSearchService(repos)
	}

	return &CollectionsHandler{
		actorRepo:        repos.Actor(),
		relationshipRepo: repos.Relationship(),
		likeRepo:         repos.Like(),
		instanceRepo:     repos.Instance(),
		remoteResolver:   remoteResolver,
	}
}

// RegisterRoutes registers all collections routes
func (ch *CollectionsHandler) RegisterRoutes(app *apptheory.App) {
	// ActivityPub federation collection endpoints
	_ = app.Get("/users/:username/followers", ch.handleFollowersCollection)
	_ = app.Get("/users/:username/following", ch.handleFollowingCollection)
	_ = app.Get("/users/:username/liked", ch.handleLikedCollection)
}

// handleFollowersCollection handles the followers collection endpoint
func (ch *CollectionsHandler) handleFollowersCollection(ctx *apptheory.Context) (*apptheory.Response, error) {
	return ch.handleCollection(ctx, "followers")
}

// handleFollowingCollection handles the following collection endpoint
func (ch *CollectionsHandler) handleFollowingCollection(ctx *apptheory.Context) (*apptheory.Response, error) {
	return ch.handleCollection(ctx, "following")
}

// handleLikedCollection handles the liked collection endpoint
func (ch *CollectionsHandler) handleLikedCollection(ctx *apptheory.Context) (*apptheory.Response, error) {
	return ch.handleCollection(ctx, "liked")
}

// handleCollection handles ActivityPub collection requests
func (ch *CollectionsHandler) handleCollection(ctx *apptheory.Context, collectionType string) (*apptheory.Response, error) {
	// Extract username from path parameters
	username := ""
	if ctx != nil {
		username = ctx.Param("username")
	}
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return collectionsJSONError(http.StatusUnprocessableEntity, "missing username"), nil
	}

	locked, errResp := ch.checkInstanceState(ctx, username)
	if errResp != nil {
		return errResp, nil
	}

	// Get request ID from context
	requestID := collectionsContextRequestID(ctx)

	logger.Info("processing collections request",
		zap.String("username", username),
		zap.String("collection_type", collectionType),
		zap.String("request_id", requestID),
	)

	// Check if actor exists using DynamORM repository
	actor, err := ch.actorRepo.GetActor(ctx.Context(), username)
	if err != nil {
		if common.IsNotFound(err) {
			logger.Debug("actor not found",
				zap.String("username", username))
			return collectionsJSONError(http.StatusNotFound, "actor not found"), nil
		}
		logger.Error("failed to get actor", zap.Error(err))
		return collectionsJSONError(http.StatusInternalServerError, "database error"), nil
	}

	// Log privacy settings for observability
	logger.Debug("processing collection request",
		zap.String("username", username),
		zap.String("collection_type", collectionType),
		zap.Bool("manually_approves_followers", actor.ManuallyApprovesFollowers),
		zap.String("request_id", requestID),
	)

	// Parse query parameters
	isPage := collectionsQueryValue(ctx, "page") != ""
	cursor := collectionsQueryValue(ctx, "cursor")
	direction := collectionsQueryValue(ctx, "dir") // Check for direction parameter
	limit := 20                                    // default
	if l := collectionsQueryValue(ctx, "limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed >= 1 && parsed <= 100 {
			limit = parsed
		}
	}

	// While locked, collections should be reachable but empty for content-bearing collections.
	if locked && collectionType == collectionTypeLiked {
		return ch.handleLockedCollection(ctx, actor, collectionType, isPage)
	}

	// If not requesting a page, return the collection metadata
	if !isPage {
		return ch.returnCollection(ctx, actor, collectionType)
	}

	// Handle reverse pagination if dir=prev is specified
	if direction == "prev" {
		return ch.handleReverseDirection(ctx, actor, collectionType, username, cursor, limit)
	}

	usernames, likes, nextCursor, err := ch.fetchCollectionItems(ctx.Context(), collectionType, username, actor.ID, limit, cursor)
	if err != nil {
		logger.Error("failed to get relationships",
			zap.String("type", collectionType),
			zap.Error(err))
		return collectionsJSONError(http.StatusInternalServerError, "failed to retrieve collection data"), nil
	}

	// Build and return page
	return ch.returnCollectionPage(ctx, actor, collectionType, usernames, likes, cursor, nextCursor, limit)
}

func (ch *CollectionsHandler) checkInstanceState(ctx *apptheory.Context, username string) (bool, *apptheory.Response) {
	var state *storageModels.InstanceState
	var stateErr error
	if ch.instanceRepo != nil {
		state, stateErr = ch.instanceRepo.GetInstanceState(ctx.Context())
	} else {
		stateErr = errors.New("missing instance repository")
	}
	bootstrapUsername := storageModels.DefaultBootstrapUsername
	if stateErr == nil && state != nil && strings.TrimSpace(state.BootstrapUsername) != "" {
		bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
	}
	locked := stateErr != nil || state == nil || state.Locked
	if locked && strings.EqualFold(username, bootstrapUsername) {
		return false, collectionsJSONError(http.StatusForbidden, "bootstrap actor is not available while instance is locked")
	}
	return locked, nil
}

func (ch *CollectionsHandler) handleLockedCollection(_ *apptheory.Context, actor *activitypub.Actor, collectionType string, isPage bool) (*apptheory.Response, error) {
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
	if !isPage {
		resp, err := collectionsActivityJSON(http.StatusOK, &activitypub.OrderedCollection{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      collectionID,
					Type:    activitypub.OrderedCollectionType,
				},
				TotalItems: 0,
			},
		})
		return resp, err
	}

	resp, err := collectionsActivityJSON(http.StatusOK, &activitypub.OrderedCollectionPage{
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
	return resp, err
}

func (ch *CollectionsHandler) fetchCollectionItems(ctx context.Context, collectionType, username, actorID string, limit int, cursor string) ([]string, []*storage.Like, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var usernames []string
	var likes []*storage.Like
	var nextCursor string
	var err error

	switch collectionType {
	case collectionTypeFollowers:
		usernames, nextCursor, err = ch.relationshipRepo.GetFollowers(ctx, username, limit, cursor)
	case collectionTypeFollowing:
		usernames, nextCursor, err = ch.relationshipRepo.GetFollowing(ctx, username, limit, cursor)
	case collectionTypeLiked:
		// For liked collection, we get Like objects and convert to storage.Like
		var modelLikes []*storageModels.Like
		modelLikes, nextCursor, err = ch.likeRepo.GetActorLikes(ctx, actorID, limit, cursor)
		if err == nil {
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
	return usernames, likes, nextCursor, err
}

// returnCollection returns the collection metadata (not a page)
func (ch *CollectionsHandler) returnCollection(ctx *apptheory.Context, actor *activitypub.Actor, collectionType string) (*apptheory.Response, error) {
	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	// Get total count using proper count methods
	var hasItems bool
	var itemCount int

	switch collectionType {
	case collectionTypeFollowers:
		count, err := ch.relationshipRepo.CountFollowers(runCtx, actor.PreferredUsername)
		if err != nil {
			logger.Error("failed to get followers count", zap.Error(err))
			return collectionsJSONError(http.StatusInternalServerError, "failed to get collection count"), nil
		}
		hasItems = count > 0
		itemCount = count
	case collectionTypeFollowing:
		count, err := ch.relationshipRepo.CountFollowing(runCtx, actor.PreferredUsername)
		if err != nil {
			logger.Error("failed to get following count", zap.Error(err))
			return collectionsJSONError(http.StatusInternalServerError, "failed to get collection count"), nil
		}
		hasItems = count > 0
		itemCount = count
	case collectionTypeLiked:
		count, err := ch.likeRepo.CountActorLikes(runCtx, actor.ID)
		if err != nil {
			logger.Error("failed to get liked count", zap.Error(err))
			return collectionsJSONError(http.StatusInternalServerError, "failed to get collection count"), nil
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

	return collectionsActivityJSON(http.StatusOK, collection)
}

// returnCollectionPage returns a page of the collection
func (ch *CollectionsHandler) returnCollectionPage(ctx *apptheory.Context, actor *activitypub.Actor, collectionType string, usernames []string, likes []*storage.Like, cursor, nextCursor string, limit int) (*apptheory.Response, error) {
	logger.Debug("returning collection page",
		zap.String("actor", actor.ID),
		zap.String("collection_type", collectionType),
		zap.Int("usernames_count", len(usernames)),
		zap.Int("likes_count", len(likes)))

	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	// Build collection and page URLs
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
	pageID := fmt.Sprintf("%s?page=1", collectionID)
	if cursor != "" {
		pageID = fmt.Sprintf("%s&cursor=%s", pageID, cursor)
	}

	orderedItems := ch.collectionOrderedItems(runCtx, collectionType, usernames, likes)

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

	return collectionsActivityJSON(http.StatusOK, page)
}

func (ch *CollectionsHandler) collectionOrderedItems(ctx context.Context, collectionType string, usernames []string, likes []*storage.Like) []any {
	if collectionType == collectionTypeLiked {
		orderedItems := make([]any, len(likes))
		for i, like := range likes {
			orderedItems[i] = like.Object
		}
		return orderedItems
	}

	orderedItems := make([]any, len(usernames))
	for i, username := range usernames {
		orderedItems[i] = ch.collectionActorID(ctx, username)
	}
	return orderedItems
}

func (ch *CollectionsHandler) collectionActorID(ctx context.Context, identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ""
	}

	if strings.Contains(identifier, "://") {
		return identifier
	}

	if strings.Contains(identifier, "@") {
		return ch.remoteCollectionActorID(ctx, identifier)
	}

	return ch.localCollectionActorID(identifier)
}

func (ch *CollectionsHandler) remoteCollectionActorID(ctx context.Context, identifier string) string {
	if ch.actorRepo != nil {
		actor, err := ch.actorRepo.GetCachedRemoteActor(ctx, identifier)
		if actorID := collectionsActorURL(actor); actorID != "" {
			return actorID
		}
		if err != nil {
			logger.Warn("failed to resolve cached remote actor for collection item",
				zap.String("identifier", identifier),
				zap.Error(err))
		}
	}

	if ch.remoteResolver != nil {
		resolution, err := ch.remoteResolver.ResolveExactActor(ctx, identifier, collectionLocalDomain())
		if err == nil && resolution != nil {
			if actorID := collectionsActorURL(resolution.Actor); actorID != "" {
				return actorID
			}
		}
		if err != nil {
			logger.Warn("failed to resolve remote actor for collection item",
				zap.String("identifier", identifier),
				zap.Error(err))
		}
	}

	return ch.remoteCollectionActorURL(identifier)
}

func collectionsActorURL(actor *activitypub.Actor) string {
	if actor == nil {
		return ""
	}
	if actorID := strings.TrimSpace(actor.ID); actorID != "" {
		return actorID
	}
	return strings.TrimSpace(actor.URL)
}

func (ch *CollectionsHandler) localCollectionActorID(identifier string) string {
	domain := ""
	if cfg != nil {
		domain = strings.TrimSpace(cfg.Domain)
	}
	if domain == "" {
		return identifier
	}
	if strings.HasPrefix(domain, "http") {
		return fmt.Sprintf("%s/users/%s", domain, identifier)
	}
	return fmt.Sprintf("https://%s/users/%s", domain, identifier)
}

func (ch *CollectionsHandler) remoteCollectionActorURL(identifier string) string {
	username, domain, ok := parseCollectionHandle(identifier)
	if !ok {
		return identifier
	}

	if localDomain := collectionLocalDomain(); localDomain != "" && strings.EqualFold(domain, localDomain) {
		return ch.localCollectionActorID(username)
	}

	return fmt.Sprintf("https://%s/users/%s", domain, username)
}

func parseCollectionHandle(identifier string) (string, string, bool) {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(identifier, "@")), "@")
	if len(parts) != 2 {
		return "", "", false
	}

	username := strings.TrimSpace(parts[0])
	domain := strings.TrimSpace(parts[1])
	if username == "" || domain == "" {
		return "", "", false
	}

	return username, domain, true
}

func collectionLocalDomain() string {
	if cfg == nil {
		return ""
	}

	domain := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(cfg.Domain, "https://"), "http://"))
	if idx := strings.Index(domain, "/"); idx >= 0 {
		domain = domain[:idx]
	}
	if idx := strings.Index(domain, ":"); idx >= 0 {
		domain = domain[:idx]
	}
	return strings.TrimSpace(domain)
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
func (ch *CollectionsHandler) handleReverseDirection(ctx *apptheory.Context, actor *activitypub.Actor, collectionType, username, cursor string, limit int) (*apptheory.Response, error) {
	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

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
		usernames, nextCursor, err = ch.relationshipRepo.GetFollowers(runCtx, username, limit, actualCursor)
		// Reverse the order of results for reverse pagination
		if err == nil {
			ch.reverseStringSlice(usernames)
		}
	case collectionTypeFollowing:
		usernames, nextCursor, err = ch.relationshipRepo.GetFollowing(runCtx, username, limit, actualCursor)
		if err == nil {
			ch.reverseStringSlice(usernames)
		}
	case collectionTypeLiked:
		modelLikes, likesNextCursor, err := ch.likeRepo.GetActorLikes(runCtx, actor.ID, limit, actualCursor)
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
		return collectionsJSONError(http.StatusInternalServerError, "failed to retrieve collection data"), nil
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
func (ch *CollectionsHandler) returnCollectionPageReverse(ctx *apptheory.Context, actor *activitypub.Actor, collectionType string, usernames []string, likes []*storage.Like, cursor, prevCursor, nextCursor string, limit int) (*apptheory.Response, error) {
	// Similar to returnCollectionPage but with swapped prev/next logic for reverse pagination
	logger.Debug("returning reverse collection page",
		zap.String("actor", actor.ID),
		zap.String("collection_type", collectionType),
		zap.Int("usernames_count", len(usernames)),
		zap.Int("likes_count", len(likes)))

	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	// Build collection and page URLs
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
	pageID := fmt.Sprintf("%s?page=1&dir=prev", collectionID)
	if cursor != "" {
		pageID = fmt.Sprintf("%s&cursor=%s", pageID, cursor)
	}

	orderedItems := ch.collectionOrderedItems(runCtx, collectionType, usernames, likes)

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

	return collectionsActivityJSON(http.StatusOK, page)
}

func main() {
	runCollections()
}

func runCollections() {
	// Create the handler with standardized services
	handler := newCollectionsHandlerFn()

	app := buildApp(handler, logger)

	// Register all collections routes
	handler.RegisterRoutes(app)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, crawler.InjectTrustedSourceIPHeader(event, logger))
	})
}

func buildApp(_ *CollectionsHandler, lambdaLogger *zap.Logger) *apptheory.App {
	app := apptheory.New(
		apptheory.WithCORS(apptheory.CORSConfig{
			AllowedOrigins:   []string{"*"},
			AllowCredentials: false,
			AllowHeaders: []string{
				"Accept",
				"Authorization",
				"Content-Type",
				"Signature",
				"Date",
				"Digest",
			},
		}),
		apptheory.WithLimits(apptheory.Limits{
			MaxRequestBytes:  1024 * 1024,
			MaxResponseBytes: 0,
		}),
	)

	// Panic recovery middleware (MUST be first to catch all panics).
	app.Use(collectionsPanicRecovery(lambdaLogger))

	// Request ID middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				ctx.Set("requestID", collectionsRequestID(ctx, "collections"))
			}
			return next(ctx)
		}
	})

	// Crawler classification middleware (observe-only; configurable via CRAWLER_PROTECTION_MODE).
	app.Use(crawler.NewMiddleware(lambdaLogger))

	// Security headers middleware (federation-friendly).
	app.Use(collectionsActivityPubSecurityHeaders())

	// Logging middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			resp, err := next(ctx)

			requestID := collectionsContextRequestID(ctx)
			hasError := err != nil
			if !hasError && resp != nil && resp.Status >= 400 {
				hasError = true
			}

			method := ""
			path := ""
			if ctx != nil {
				method = ctx.Request.Method
				path = ctx.Request.Path
			}

			logger.Info("collections request completed",
				zap.String("request_id", requestID),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", hasError),
			)

			if hasError {
				logger.Error("collections handler error",
					zap.String("request_id", requestID),
					zap.Error(err),
				)
			}

			return resp, err
		}
	})

	return app
}

func collectionsPanicRecovery(logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (resp *apptheory.Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered",
						zap.String("request_id", collectionsContextRequestID(ctx)),
						zap.Any("panic", r),
					)
					resp = collectionsJSONError(http.StatusInternalServerError, "internal server error")
					err = nil
				}
			}()
			return next(ctx)
		}
	}
}

func collectionsActivityPubSecurityHeaders() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			resp, err := next(ctx)
			if resp == nil {
				return resp, err
			}
			if resp.Headers == nil {
				resp.Headers = map[string][]string{}
			}
			resp.Headers["x-content-type-options"] = []string{"nosniff"}
			resp.Headers["x-frame-options"] = []string{"SAMEORIGIN"}
			resp.Headers["referrer-policy"] = []string{"strict-origin-when-cross-origin"}
			resp.Headers["cross-origin-resource-policy"] = []string{"cross-origin"}
			resp.Headers["x-robots-tag"] = []string{"noindex, nofollow"}
			return resp, err
		}
	}
}

func collectionsQueryValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	values := ctx.Request.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func collectionsRequestID(ctx *apptheory.Context, prefix string) string {
	if ctx != nil && strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "collections"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func collectionsContextRequestID(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}
	if rid, ok := ctx.Get("requestID").(string); ok && strings.TrimSpace(rid) != "" {
		return strings.TrimSpace(rid)
	}
	if strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	return ""
}

func collectionsJSONError(status int, message string) *apptheory.Response {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "internal server error"
	}
	return apptheory.MustJSON(status, map[string]string{"error": message})
}

func collectionsActivityJSON(status int, value any) (*apptheory.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &apptheory.Response{
		Status: status,
		Headers: map[string][]string{
			"content-type":  {contentTypeActivityJSON},
			"cache-control": {cacheControlMaxAge300},
		},
		Body: body,
	}, nil
}
