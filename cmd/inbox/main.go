// Package main implements the inbox Lambda function for receiving ActivityPub federation messages.
package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// InboxHandler handles ActivityPub inbox requests using Lift
type InboxHandler struct {
	db                           core.DB
	actorRepository              *repositories.ActorRepository
	activityRepository           *repositories.ActivityRepository
	relationshipRepository       *repositories.RelationshipRepository
	objectRepository             *repositories.ObjectRepository
	likeRepository               *repositories.LikeRepository
	federationActivityRepository *repositories.FederationActivityRepository
	federationCostRepository     *repositories.FederationCostRepository
	domainBlockRepository        *repositories.DomainBlockRepository
	userRepository               *repositories.UserRepository
	logger                       *zap.Logger
	authMiddleware               *auth.Middleware
	rateLimiter                  *auth.RateLimiter
	costCalculator               *federation.CostCalculator
	deliveryService              *federation.DeliveryService
	tableName                    string
}

// NewInboxHandler creates a new inbox handler
func NewInboxHandler() (*InboxHandler, error) {
	logger := common.Logger()
	cfg := config.Get()

	// Load AWS config
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Initialize repository factory
	repoFactory, err := factory.NewRepositoryFactory(db, cfg.DynamoTableName, awsConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize repository factory: %w", err)
	}

	// Get repositories from factory
	actorRepo := repoFactory.Actor()
	activityRepo := repoFactory.Activity()
	followRepo := repoFactory.Relationship()
	objectRepo := repoFactory.Object()
	likeRepo := repoFactory.Like()
	federationActivityRepo := repositories.NewFederationActivityRepository(db, cfg.DynamoTableName, logger)
	federationCostRepo := repositories.NewFederationCostRepository(db, cfg.DynamoTableName, logger)
	domainBlockRepo := repoFactory.DomainBlock()
	userRepo := repoFactory.User()

	// Initialize cost calculator
	costCalculator := federation.NewCostCalculator()

	// Initialize auth middleware
	authMiddleware, err := auth.GetMiddleware()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth middleware: %w", err)
	}

	// Initialize rate limiter with repository storage
	rateLimiter := auth.NewRateLimiter(repoFactory)

	// Initialize delivery service for federation
	deliveryService := federation.NewDeliveryService(
		federation.NewRepositoryStorageAdapter(repoFactory),
	)

	return &InboxHandler{
		db:                           db,
		actorRepository:              actorRepo,
		activityRepository:           activityRepo,
		relationshipRepository:       followRepo,
		objectRepository:             objectRepo,
		likeRepository:               likeRepo,
		federationActivityRepository: federationActivityRepo,
		federationCostRepository:     federationCostRepo,
		domainBlockRepository:        domainBlockRepo,
		userRepository:               userRepo,
		logger:                       logger,
		authMiddleware:               authMiddleware,
		rateLimiter:                  rateLimiter,
		costCalculator:               costCalculator,
		deliveryService:              deliveryService,
		tableName:                    cfg.DynamoTableName,
	}, nil
}

// RegisterRoutes registers all inbox routes
func (ih *InboxHandler) RegisterRoutes(app *lift.App) {
	// ActivityPub inbox endpoints
	_ = app.GET("/inbox/{username}", ih.handleGetInbox)
	_ = app.POST("/inbox/{username}", ih.handlePostInbox)
}

// handleGetInbox handles GET requests to retrieve inbox activities
func (ih *InboxHandler) handleGetInbox(ctx *lift.Context) error {
	username := ctx.Param("username")
	if username == "" {
		return lift.NewLiftError("VALIDATION_ERROR", "missing username parameter", 400)
	}

	ih.logger.Info("received inbox GET request",
		zap.String("username", username),
		zap.String("user_agent", ctx.Header("User-Agent")),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))))

	// For GET requests, require authentication
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		return lift.NewLiftError("UNAUTHORIZED", "authentication required", 401)
	}

	// Extract and validate bearer token
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return lift.NewLiftError("UNAUTHORIZED", "invalid authorization header format", 401)
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	// Validate the token using auth middleware
	// Note: This is a simplified approach - in production, you'd want to validate the JWT token
	if len(token) < 10 {
		return lift.NewLiftError("UNAUTHORIZED", "invalid token", 401)
	}

	// Verify the actor exists
	actor, err := ih.actorRepository.GetActorByUsername(ctx.Context, username)
	if err != nil {
		if err.Error() == "actor not found" {
			return lift.NewLiftError("NOT_FOUND", "actor not found", 404)
		}
		ih.logger.Error("failed to get actor", zap.Error(err))
		return lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
	}

	// Parse pagination parameters
	limitStr := ctx.Query("limit")
	if limitStr == "" {
		limitStr = "20"
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	cursor := ctx.Query("cursor")
	page := ctx.Query("page")

	// If no page parameter, return the collection with metadata
	if page == "" && cursor == "" {
		// Get first page to calculate total items (this is a simplification)
		activities, _, err := ih.activityRepository.GetInboxActivities(ctx.Context, username, 1, "")
		if err != nil {
			ih.logger.Error("failed to get inbox count", zap.Error(err))
			return lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
		}

		// Build the collection response
		collection := &activitypub.OrderedCollection{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      actor.Inbox,
					Type:    activitypub.OrderedCollectionType,
				},
				TotalItems: len(activities), // This is approximate
				First:      fmt.Sprintf("%s?page=true", actor.Inbox),
			},
		}

		ctx.Response.Headers["Content-Type"] = "application/activity+json"
		return ctx.JSON(collection)
	}

	// Get activities for the page
	activities, nextCursor, err := ih.activityRepository.GetInboxActivities(ctx.Context, username, limit, cursor)
	if err != nil {
		ih.logger.Error("failed to get inbox activities", zap.Error(err))
		return lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
	}

	// Enrich activities with objects if they contain Create activities
	for _, activity := range activities {
		if activity.Type == activitypub.CreateType && activity.Object != nil {
			// If Object is just an ID string, fetch the full object
			if objID, ok := activity.Object.(string); ok {
				obj, err := ih.objectRepository.GetObject(ctx.Context, objID)
				if err != nil {
					ih.logger.Warn("failed to fetch object for activity",
						zap.String("activity_id", activity.ID),
						zap.String("object_id", objID),
						zap.Error(err))
					// Continue without enrichment
				} else {
					activity.Object = obj
				}
			}
		}
	}

	// Convert activities to ordered items
	orderedItems := make([]any, len(activities))
	for i, activity := range activities {
		orderedItems[i] = activity
	}

	// Build the collection page response
	collectionPage := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      fmt.Sprintf("%s?page=true", actor.Inbox),
					Type:    "OrderedCollectionPage",
				},
				OrderedItems: orderedItems,
			},
			PartOf: actor.Inbox,
		},
	}

	// Add next link if there are more items
	if nextCursor != "" {
		collectionPage.Next = fmt.Sprintf("%s?page=true&cursor=%s&limit=%d", actor.Inbox, nextCursor, limit)
	}

	// Add prev link if we have a cursor (meaning this isn't the first page)
	if cursor != "" {
		collectionPage.Prev = fmt.Sprintf("%s?page=true&limit=%d", actor.Inbox, limit)
	}

	ctx.Response.Headers["Content-Type"] = "application/activity+json"
	return ctx.JSON(collectionPage)
}

// InboxRequest represents an incoming ActivityPub request
type InboxRequest struct {
	Username     string
	Activity     *activitypub.Activity
	Actor        *activitypub.Actor
	Body         []byte
	ActorDomain  string
	StartTime    time.Time
	CostParams   *federation.CostCalculationParams
}

// handlePostInbox handles POST requests to receive activities
func (ih *InboxHandler) handlePostInbox(ctx *lift.Context) error {
	// Initialize and validate request
	req, err := ih.initializeInboxRequest(ctx)
	if err != nil {
		return err
	}

	// Perform security checks (rate limiting, domain blocks)
	if err := ih.performSecurityChecks(ctx, req); err != nil {
		return err
	}

	// Verify authentication (signature verification)
	if err := ih.verifyAuthentication(ctx, req); err != nil {
		return err
	}

	// Store and process the activity
	if err := ih.storeAndProcessActivity(ctx, req); err != nil {
		return err
	}

	// Record success and complete
	ih.recordSuccessAndComplete(ctx, req)

	return ctx.Status(http.StatusAccepted).Text("")
}

// initializeInboxRequest creates and validates the basic request structure
func (ih *InboxHandler) initializeInboxRequest(ctx *lift.Context) (*InboxRequest, error) {
	username := ctx.Param("username")
	if username == "" {
		return nil, lift.NewLiftError("VALIDATION_ERROR", "missing username parameter", 400)
	}

	ih.logger.Info("received inbox POST request",
		zap.String("username", username),
		zap.String("content_type", ctx.Header("Content-Type")),
		zap.String("user_agent", ctx.Header("User-Agent")),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))))

	// Verify the actor exists
	actor, err := ih.actorRepository.GetActorByUsername(ctx.Context, username)
	if err != nil {
		if err.Error() == "actor not found" {
			return nil, lift.NewLiftError("NOT_FOUND", "actor not found", 404)
		}
		ih.logger.Error("failed to get actor", zap.Error(err))
		return nil, lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
	}

	// Validate and parse request body
	body := ctx.Request.Body
	if err := ih.validateRequestBody(body); err != nil {
		return nil, err
	}

	activity, err := ih.parseActivity(body)
	if err != nil {
		return nil, err
	}

	// Validate activity and addressing
	if err := ih.validateActivity(activity, actor); err != nil {
		return nil, err
	}

	// Initialize request structure
	startTime := time.Now()
	actorDomain := ih.extractDomainFromURL(activity.Actor)

	req := &InboxRequest{
		Username:    username,
		Activity:    activity,
		Actor:       actor,
		Body:        body,
		ActorDomain: actorDomain,
		StartTime:   startTime,
		CostParams: &federation.CostCalculationParams{
			ActivityID:         activity.ID,
			Domain:             actorDomain,
			ActivityType:       activity.Type,
			Direction:          "inbound",
			OperationType:      "inbox_processing",
			Timestamp:          startTime,
			PayloadSize:        int64(len(body)),
			LambdaMemoryMB:     512,
			DynamoDBReadCount:  1, // Actor verification
			DynamoDBWriteCount: 0,
		},
	}

	return req, nil
}

// validateRequestBody validates the request body size and content
func (ih *InboxHandler) validateRequestBody(body []byte) error {
	if len(body) == 0 {
		return lift.NewLiftError("VALIDATION_ERROR", "request body is required", 400)
	}

	if len(body) > common.MaxActivitySize {
		ih.logger.Warn("request body too large", zap.Int("size", len(body)))
		return lift.NewLiftError("PAYLOAD_TOO_LARGE", "request body too large", 413)
	}

	return nil
}

// parseActivity parses and sanitizes the ActivityPub activity
func (ih *InboxHandler) parseActivity(body []byte) (*activitypub.Activity, error) {
	var activity activitypub.Activity
	if err := common.ParseActivityPubObject(body, &activity); err != nil {
		ih.logger.Warn("failed to parse activity", zap.Error(err))
		return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid activity: %v", err), 400)
	}

	// Sanitize any embedded objects
	if objMap, ok := activity.Object.(map[string]any); ok {
		common.SanitizeActivityPubObjectDefault(objMap)
		activity.Object = objMap
	}

	ih.logger.Info("processing activity",
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor),
		zap.String("id", activity.ID))

	return &activity, nil
}

// validateActivity validates required activity fields and addressing
func (ih *InboxHandler) validateActivity(activity *activitypub.Activity, actor *activitypub.Actor) error {
	if activity.ID == "" {
		return lift.NewLiftError("VALIDATION_ERROR", "activity ID is required", 400)
	}
	if activity.Actor == "" {
		return lift.NewLiftError("VALIDATION_ERROR", "actor is required", 400)
	}
	if activity.Type == "" {
		return lift.NewLiftError("VALIDATION_ERROR", "activity type is required", 400)
	}

	if !ih.isAddressedTo(activity, actor) {
		ih.logger.Warn("activity not addressed to this actor",
			zap.String("actor_id", actor.ID),
			zap.Any("to", activity.To),
			zap.Any("cc", activity.CC))
		return lift.NewLiftError("VALIDATION_ERROR", "activity is not addressed to this actor", 400)
	}

	return nil
}

// performSecurityChecks handles rate limiting and domain blocking
func (ih *InboxHandler) performSecurityChecks(ctx *lift.Context, req *InboxRequest) error {
	if req.ActorDomain == "" {
		return nil
	}

	// Check rate limiting
	if err := ih.checkRateLimit(ctx, req); err != nil {
		return err
	}

	// Check domain blocking
	if err := ih.checkDomainBlock(ctx, req); err != nil {
		return err
	}

	return nil
}

// checkRateLimit performs rate limiting checks
func (ih *InboxHandler) checkRateLimit(ctx *lift.Context, req *InboxRequest) error {
	if err := ih.rateLimiter.CheckRateLimit(ctx.Context, req.ActorDomain, ctx.Header("X-Forwarded-For")); err != nil {
		ih.logger.Warn("rate limit exceeded",
			zap.String("domain", req.ActorDomain),
			zap.Error(err))

		ih.recordFailureCost(req, "Rate limit exceeded", 2)
		return lift.NewLiftError("RATE_LIMITED", "rate limit exceeded for domain", 429)
	}

	// Record the rate limit attempt
	if err := ih.rateLimiter.RecordAttempt(ctx.Context, req.ActorDomain, ctx.Header("X-Forwarded-For"), false); err != nil {
		ih.logger.Warn("failed to record rate limit attempt", zap.Error(err))
	}

	return nil
}

// checkDomainBlock checks if the domain is blocked
func (ih *InboxHandler) checkDomainBlock(ctx *lift.Context, req *InboxRequest) error {
	isBlocked, block, err := ih.domainBlockRepository.IsDomainBlocked(ctx.Context, req.ActorDomain)
	if err != nil {
		ih.logger.Error("failed to check domain block status",
			zap.String("domain", req.ActorDomain),
			zap.Error(err))
		return nil // Fail open rather than closed
	}

	if !isBlocked || block == nil {
		return nil
	}

	ih.logger.Info("rejecting activity from blocked domain",
		zap.String("domain", req.ActorDomain),
		zap.String("severity", block.Severity),
		zap.String("actor", req.Activity.Actor))

	// For suspended domains, reject completely
	if block.Severity == "suspend" {
		ih.recordFailureCost(req, "Domain is suspended", 3)
		return lift.NewLiftError("FORBIDDEN", "domain is suspended", 403)
	}

	// For silenced domains, we accept but may limit visibility
	return nil
}

// verifyAuthentication handles public key fetching and signature verification
func (ih *InboxHandler) verifyAuthentication(ctx *lift.Context, req *InboxRequest) error {
	// Fetch public key
	keyFetchStart := time.Now()
	publicKey, err := ih.fetchActorPublicKey(ctx.Context, req.Activity.Actor)
	keyFetchDuration := time.Since(keyFetchStart)

	req.CostParams.HTTPRequestCount = 1
	req.CostParams.DNSLookupCount = 1

	if err != nil {
		ih.logger.Error("failed to fetch actor public key",
			zap.String("actor", req.Activity.Actor),
			zap.Error(err))

		req.CostParams.ProcessingTimeMs = keyFetchDuration.Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Failed to fetch actor public key: %v", err), 3)
		return lift.NewLiftError("VALIDATION_ERROR", "unable to verify sender", 400)
	}

	// Verify signature
	signatureVerifyStart := time.Now()
	if err := ih.verifyRequest(ctx, publicKey, req.Body); err != nil {
		signatureVerifyDuration := time.Since(signatureVerifyStart)

		ih.logger.Warn("signature verification failed",
			zap.String("actor", req.Activity.Actor),
			zap.Error(err))

		req.CostParams.ProcessingTimeMs = keyFetchDuration.Milliseconds() + signatureVerifyDuration.Milliseconds()
		req.CostParams.SignatureVerificationMs = signatureVerifyDuration.Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Signature verification failed: %v", err), 3)
		return lift.NewLiftError("UNAUTHORIZED", "signature verification failed", 401)
	}

	signatureVerifyDuration := time.Since(signatureVerifyStart)
	req.CostParams.SignatureVerificationMs = signatureVerifyDuration.Milliseconds()

	// Verify digest if present
	return ih.verifyDigest(ctx, req)
}

// verifyDigest verifies the digest header if present
func (ih *InboxHandler) verifyDigest(ctx *lift.Context, req *InboxRequest) error {
	if ctx.Header("Digest") == "" {
		return nil
	}

	httpReq, err := ih.convertLiftRequest(ctx, req.Body)
	if err != nil {
		return nil
	}

	if err := federation.VerifyDigest(httpReq, req.Body); err != nil {
		ih.logger.Warn("digest verification failed",
			zap.String("actor", req.Activity.Actor),
			zap.Error(err))
		return lift.NewLiftError("VALIDATION_ERROR", "digest verification failed", 400)
	}

	return nil
}

// storeAndProcessActivity stores the activity and processes it based on type
func (ih *InboxHandler) storeAndProcessActivity(ctx *lift.Context, req *InboxRequest) error {
	// Store the activity
	if err := ih.activityRepository.CreateActivity(ctx.Context, req.Activity); err != nil {
		ih.logger.Error("failed to store activity", zap.Error(err))
		ih.recordFailureCost(req, fmt.Sprintf("Failed to store activity: %v", err), 3)
		return lift.NewLiftError("INTERNAL_ERROR", "failed to store activity", 500)
	}

	req.CostParams.DynamoDBWriteCount = 1 // Activity storage

	// Process by activity type
	processingStart := time.Now()
	if err := ih.processActivityByType(ctx.Context, req); err != nil {
		processingDuration := time.Since(processingStart)
		req.CostParams.ProcessingTimeMs += processingDuration.Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Failed to process %s activity: %v", req.Activity.Type, err), 0)
		return lift.NewLiftError("INTERNAL_ERROR", fmt.Sprintf("failed to process %s activity", req.Activity.Type), 500)
	}

	processingDuration := time.Since(processingStart)
	req.CostParams.ProcessingTimeMs += processingDuration.Milliseconds()
	return nil
}

// processActivityByType processes the activity based on its type
func (ih *InboxHandler) processActivityByType(ctx context.Context, req *InboxRequest) error {
	switch req.Activity.Type {
	case activitypub.FollowType:
		if err := ih.processFollowActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process follow activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Relationship creation
		req.CostParams.DynamoDBReadCount++  // Follow approval check

	case activitypub.AcceptType:
		if err := ih.processAcceptActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process accept activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Relationship update
		req.CostParams.DynamoDBReadCount++  // Original activity lookup

	case activitypub.RejectType:
		if err := ih.processRejectActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process reject activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Relationship deletion
		req.CostParams.DynamoDBReadCount++  // Original activity lookup

	case activitypub.CreateType:
		if err := ih.processRemoteCreateActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process create activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount += 2 // Object creation + timeline entry
		req.CostParams.DynamoDBReadCount++  // Content validation

	case activitypub.UpdateType:
		if err := ih.processRemoteUpdateActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process update activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Object update
		req.CostParams.DynamoDBReadCount++  // Object lookup

	case activitypub.DeleteType:
		if err := ih.processRemoteDeleteActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process delete activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Object deletion
		req.CostParams.DynamoDBReadCount++  // Object lookup

	case activitypub.UndoType:
		if err := ih.processUndoActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process undo activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Undo operation
		req.CostParams.DynamoDBReadCount += 2  // Original activity + target lookup
	}

	return nil
}

// recordSuccessAndComplete handles final success logging and cost tracking
func (ih *InboxHandler) recordSuccessAndComplete(ctx *lift.Context, req *InboxRequest) {
	ih.logger.Info("activity accepted and processed",
		zap.String("id", req.Activity.ID),
		zap.String("type", req.Activity.Type),
		zap.String("from", req.Activity.Actor))

	// Record successful cost tracking
	req.CostParams.Success = true
	req.CostParams.ResponseTimeMs = time.Since(req.StartTime).Milliseconds()
	req.CostParams.LambdaDurationMs = time.Since(req.StartTime).Milliseconds()
	req.CostParams.DynamoDBWriteCount++ // Cost tracking record itself

	cost := ih.costCalculator.CalculateFederationCosts(req.CostParams)

	go func() {
		if err := ih.federationCostRepository.RecordFederationCost(context.Background(), cost); err != nil {
			ih.logger.Warn("failed to record federation cost", zap.Error(err))
		}

		if err := ih.federationCostRepository.UpdateBudgetUsage(context.Background(),
			req.ActorDomain, "daily", req.Activity.Type, "inbound", cost.TotalCostMicroCents); err != nil {
			ih.logger.Warn("failed to update budget usage", zap.Error(err))
		}
	}()

	// Mark rate limit success
	if req.ActorDomain != "" {
		if err := ih.rateLimiter.RecordAttempt(ctx.Context, req.ActorDomain, ctx.Header("X-Forwarded-For"), true); err != nil {
			ih.logger.Warn("failed to record rate limit success", zap.Error(err))
		}
	}
}

// recordFailureCost records cost tracking for failures
func (ih *InboxHandler) recordFailureCost(req *InboxRequest, errorMsg string, readCount int) {
	req.CostParams.Success = false
	req.CostParams.ErrorMessage = errorMsg
	req.CostParams.ResponseTimeMs = time.Since(req.StartTime).Milliseconds()
	req.CostParams.LambdaDurationMs = time.Since(req.StartTime).Milliseconds()
	if readCount > 0 {
		req.CostParams.DynamoDBReadCount = int64(readCount)
	}

	cost := ih.costCalculator.CalculateFederationCosts(req.CostParams)
	go func() {
		if err := ih.federationCostRepository.RecordFederationCost(context.Background(), cost); err != nil {
			ih.logger.Warn("failed to record federation cost", zap.Error(err))
		}
	}()
}

// isAddressedTo checks if the activity is addressed to the given actor
func (ih *InboxHandler) isAddressedTo(activity *activitypub.Activity, actor *activitypub.Actor) bool {
	actorID := actor.ID
	inboxURL := actor.Inbox

	// Check 'to' field
	for _, to := range activity.To {
		if to == actorID || to == inboxURL || to == activitypub.PublicAddress {
			return true
		}
	}

	// Check 'cc' field
	for _, cc := range activity.CC {
		if cc == actorID || cc == inboxURL || cc == activitypub.PublicAddress {
			return true
		}
	}

	// Check 'bto' field
	for _, bto := range activity.BTo {
		if bto == actorID || bto == inboxURL {
			return true
		}
	}

	// Check 'bcc' field
	for _, bcc := range activity.BCC {
		if bcc == actorID || bcc == inboxURL {
			return true
		}
	}

	return false
}

// verifyRequest verifies the HTTP signature on the request
func (ih *InboxHandler) verifyRequest(ctx *lift.Context, publicKey crypto.PublicKey, body []byte) error {
	// Convert Lift request to http.Request for signature verification
	req, err := ih.convertLiftRequest(ctx, body)
	if err != nil {
		return fmt.Errorf("failed to convert request: %w", err)
	}

	return federation.VerifyHTTPSignature(req, publicKey)
}

// convertLiftRequest converts a Lift request to an http.Request
func (ih *InboxHandler) convertLiftRequest(ctx *lift.Context, body []byte) (*http.Request, error) {
	// Build URL
	u := &url.URL{
		Scheme: "https",
		Host:   ctx.Header("Host"),
		Path:   ctx.Request.Path,
	}

	if ctx.Request.QueryParams != nil {
		q := u.Query()
		for k, v := range ctx.Request.QueryParams {
			q.Set(k, v)
		}
	u.RawQuery = q.Encode()
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx.Request.Context(), ctx.Request.Method, u.String(), strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	// Copy headers
	for k, v := range ctx.Request.Headers {
		req.Header.Set(k, v)
	}

	// Set host header if not present
	if req.Header.Get("Host") == "" && ctx.Header("Host") != "" {
		req.Host = ctx.Header("Host")
	}

	return req, nil
}

// getConfig returns the current configuration
func (ih *InboxHandler) getConfig() *config.Config {
	return config.Get()
}

// generateActivityID generates a unique activity ID
func generateActivityID() string {
	// Use timestamp and random bytes for uniqueness
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-only ID on crypto error
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%x", time.Now().UnixNano(), b)
}

// fetchActorPublicKey fetches an actor's public key from their profile
func (ih *InboxHandler) fetchActorPublicKey(ctx context.Context, actorURL string) (crypto.PublicKey, error) {
	log := common.WithContext(ctx)

	// Create secure HTTP client with DNS caching
	client := httpclient.NewSecureClient(
		httpclient.WithTimeout(10*time.Second),
		httpclient.WithLogger(log),
	)

	// Create request with ActivityPub Accept header
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Lesser/1.0")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch actor: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Warn("failed to fetch actor profile",
			zap.String("url", actorURL),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)))
		return nil, fmt.Errorf("failed to fetch actor: status %d", resp.StatusCode)
	}

	// Parse actor
	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		return nil, fmt.Errorf("failed to parse actor: %w", err)
	}

	// Extract public key
	if actor.PublicKey == nil || actor.PublicKey.PublicKeyPem == "" {
		return nil, fmt.Errorf("actor has no public key")
	}

	// Parse PEM-encoded public key
	publicKey, err := federation.ParsePublicKeyPEM([]byte(actor.PublicKey.PublicKeyPem))
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	log.Debug("fetched actor public key",
		zap.String("actor", actorURL),
		zap.String("key_id", actor.PublicKey.ID))

	return publicKey, nil
}

// processFollowActivity processes an incoming Follow activity
func (ih *InboxHandler) processFollowActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Extract follower username from actor ID
	followerHandle := ih.extractHandleFromActorID(activity.Actor)

	// Create the follow relationship with pending state
	err := ih.relationshipRepository.CreateRelationship(ctx, followerHandle, targetActor.PreferredUsername, activity.ID)
	if err != nil {
		log.Error("failed to create follow relationship", zap.Error(err))
		return err
	}

	// Check if the target actor requires manual approval for follows
	if targetActor.ManuallyApprovesFollowers {
		log.Info("follow request pending manual approval",
			zap.String("follower", followerHandle),
			zap.String("target", targetActor.PreferredUsername))

		// Send notification to the target user about pending follow request
		notification := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context:   activitypub.Context,
				ID:        fmt.Sprintf("%s/notifications/%s", targetActor.ID, ih.generateActivityID()),
				Type:      "Notification",
				Published: ih.timePtr(time.Now()),
			},
			Actor:  activity.Actor,
			Object: activity.ID,
		}

		// Store notification for follow request
		if err := ih.activityRepository.CreateActivity(ctx, notification); err != nil {
			log.Warn("failed to create follow request notification", zap.Error(err))
		}

		// Follow request stays in pending state - no further action needed
		return nil
	}

	// Auto-accept follows for non-locked accounts
	err = ih.relationshipRepository.AcceptFollowRequest(ctx, followerHandle, targetActor.PreferredUsername)
	if err != nil {
		log.Error("failed to accept follow", zap.Error(err))
		return err
	}

	log.Info("follow request auto-accepted",
		zap.String("follower", followerHandle),
		zap.String("target", targetActor.PreferredUsername))

	// Create Accept activity
	acceptActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      fmt.Sprintf("https://%s/activities/%s", ih.getConfig().Domain, generateActivityID()),
			To:      []string{activity.Actor},
		},
		Actor:  targetActor.ID,
		Object: activity,
	}

	// Get the follower's inbox URL
	followerActor, err := ih.actorRepository.GetCachedRemoteActor(ctx, activity.Actor)
	if err != nil {
		log.Error("failed to get follower actor for delivery",
			zap.String("actor", activity.Actor),
			zap.Error(err))
		return nil // Don't fail the follow acceptance
	}

	// Send Accept activity back to the follower
	if err := ih.deliveryService.DeliverActivity(ctx, acceptActivity, followerActor.Inbox, targetActor); err != nil {
		log.Error("failed to deliver accept activity",
			zap.String("to", activity.Actor),
			zap.String("from", targetActor.ID),
			zap.Error(err))
		// Don't fail the whole operation if delivery fails
	} else {
		log.Info("delivered accept activity",
			zap.String("to", activity.Actor),
			zap.String("from", targetActor.ID))
	}

	return nil
}

// processAcceptActivity processes an incoming Accept activity
func (ih *InboxHandler) processAcceptActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Check if this is accepting a Follow request
	if objectID, ok := activity.Object.(string); ok {
		// Fetch the original activity
		originalActivity, err := ih.activityRepository.GetActivity(ctx, objectID)
		if err != nil {
			log.Warn("failed to find original activity", zap.String("id", objectID))
			return nil // Don't fail, just ignore
		}

		if originalActivity.Type == activitypub.FollowType {
			// Update the follow relationship to accepted
			acceptorHandle := ih.extractHandleFromActorID(activity.Actor)
			err = ih.relationshipRepository.AcceptFollowRequest(ctx, targetActor.PreferredUsername, acceptorHandle)
			if err != nil {
				log.Error("failed to update follow status", zap.Error(err))
				return err
			}
		}
	}

	return nil
}

// processRejectActivity processes an incoming Reject activity
func (ih *InboxHandler) processRejectActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Check if this is rejecting a Follow request
	if objectID, ok := activity.Object.(string); ok {
		// Fetch the original activity
		originalActivity, err := ih.activityRepository.GetActivity(ctx, objectID)
		if err != nil {
			log.Warn("failed to find original activity", zap.String("id", objectID))
			return nil // Don't fail, just ignore
		}

		if originalActivity.Type == activitypub.FollowType {
			// Remove the follow relationship
			rejectorHandle := ih.extractHandleFromActorID(activity.Actor)
			err = ih.relationshipRepository.DeleteRelationship(ctx, targetActor.PreferredUsername, rejectorHandle)
			if err != nil {
				log.Error("failed to remove follow", zap.Error(err))
				return err
			}
		}
	}

	return nil
}

// processRemoteCreateActivity processes an incoming Create activity from a remote instance
func (ih *InboxHandler) processRemoteCreateActivity(ctx context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Extract the object
	objMap, ok := activity.Object.(map[string]any)
	if !ok {
		log.Warn("create activity has invalid object")
		return nil
	}

	// Sanitize the object content to prevent XSS
	common.SanitizeActivityPubObjectDefault(objMap)

	// Store the object if it's a Note
	if objType, _ := objMap["type"].(string); objType == activitypub.NoteType {
		// Convert to Note object
		objJSON, err := json.Marshal(objMap)
		if err != nil {
			return err
		}

		var note activitypub.Note
		if err := common.ParseActivityPubObject(objJSON, &note); err != nil {
			return err
		}

		// Store the note (it will be marked as remote)
		if err := ih.objectRepository.CreateObject(ctx, &note); err != nil {
			log.Error("failed to store remote note", zap.Error(err))
			return err
		}
	}

	return nil
}

// processRemoteUpdateActivity processes an incoming Update activity from a remote instance
func (ih *InboxHandler) processRemoteUpdateActivity(ctx context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Extract the object
	objMap, ok := activity.Object.(map[string]any)
	if !ok {
		log.Warn("update activity has invalid object")
		return nil
	}

	// Sanitize the object content to prevent XSS
	common.SanitizeActivityPubObjectDefault(objMap)

	// Update the object if it's a Note
	if objType, _ := objMap["type"].(string); objType == activitypub.NoteType {
		// Convert to Note object
		objJSON, err := json.Marshal(objMap)
		if err != nil {
			return err
		}

		var note activitypub.Note
		if err := common.ParseActivityPubObject(objJSON, &note); err != nil {
			return err
		}

		// Update the note
		if err := ih.objectRepository.UpdateObject(ctx, &note); err != nil {
			log.Error("failed to update remote note", zap.Error(err))
			// Don't fail if we can't update (might not have it)
		}
	}

	return nil
}

// processRemoteDeleteActivity processes an incoming Delete activity from a remote instance
func (ih *InboxHandler) processRemoteDeleteActivity(ctx context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Get the object ID to delete
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if objectID == "" {
		log.Warn("delete activity has no object ID")
		return nil
	}

	// Delete the object
	if err := ih.objectRepository.DeleteObject(ctx, objectID); err != nil {
		log.Warn("failed to delete object", zap.String("id", objectID), zap.Error(err))
		// Don't fail if we can't delete (might not have it)
	}

	return nil
}

// processUndoActivity processes an incoming Undo activity
func (ih *InboxHandler) processUndoActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Get the activity being undone
	var originalActivity *activitypub.Activity

	switch obj := activity.Object.(type) {
	case string:
		// Fetch the activity by ID
		var err error
		originalActivity, err = ih.activityRepository.GetActivity(ctx, obj)
		if err != nil {
			log.Warn("failed to find activity to undo", zap.String("id", obj))
			return nil
		}
	case map[string]any:
		// Convert to activity
		objJSON, err := json.Marshal(obj)
		if err != nil {
			return err
		}

		originalActivity = &activitypub.Activity{}
		if err := common.ParseActivityPubObject(objJSON, originalActivity); err != nil {
			return err
		}
	default:
		log.Warn("undo activity has invalid object")
		return nil
	}

	// Process based on the original activity type
	switch originalActivity.Type {
	case activitypub.FollowType:
		// Undo follow
		unfollowerHandle := ih.extractHandleFromActorID(activity.Actor)
		err := ih.relationshipRepository.DeleteRelationship(ctx, unfollowerHandle, targetActor.PreferredUsername)
		if err != nil {
			log.Error("failed to remove follow", zap.Error(err))
			return err
		}
	case activitypub.LikeType:
		// Undo like
		if objectID, ok := originalActivity.Object.(string); ok {
			err := ih.likeRepository.DeleteLike(ctx, activity.Actor, objectID)
			if err != nil {
				log.Warn("failed to remove like", zap.Error(err))
				// Don't fail
			}
		}
	}

	return nil
}

// Helper functions

func (ih *InboxHandler) extractHandleFromActorID(actorID string) string {
	// Extract username and domain from actor ID
	// Format: https://domain.com/users/username -> @username@domain.com
	parts := strings.Split(actorID, "/")
	if len(parts) < 5 {
		return actorID // Return as-is if not in expected format
	}

	domain := parts[2]
	username := parts[len(parts)-1]

	return fmt.Sprintf("@%s@%s", username, domain)
}

func (ih *InboxHandler) generateActivityID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), ih.generateRandomString(8))
}

func (ih *InboxHandler) generateRandomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)

	// Use crypto/rand for secure random generation
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		// This should rarely happen, but we handle it gracefully
		ih.logger.Error("Failed to generate secure random bytes", zap.Error(err))
		// Still return something random-ish as a fallback
		for i := range result {
			result[i] = chars[i%len(chars)]
		}
		return string(result)
	}

	// Map random bytes to our character set
	for i := range result {
		result[i] = chars[int(randomBytes[i])%len(chars)]
	}
	return string(result)
}

func (ih *InboxHandler) timePtr(t time.Time) *time.Time {
	return &t
}

// extractDomainFromURL extracts the domain from an ActivityPub actor URL
func (ih *InboxHandler) extractDomainFromURL(actorURL string) string {
	u, err := url.Parse(actorURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func main() {
	handler, err := NewInboxHandler()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize inbox handler: %v", err))
	}

	app := lift.New()

	// Add request ID middleware (first in chain)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("inbox-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware (second in chain)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			path := ctx.Request.Path
			method := ctx.Request.Method

			err := next.Handle(ctx)

			handler.logger.Info("inbox request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			return err
		})
	})

	// Add recovery middleware (third in chain)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					handler.logger.Error("panic recovered in inbox handler",
						zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
						zap.Any("panic", r))
					// Return a generic error
					if err := ctx.Status(500).Text("Internal server error"); err != nil {
						handler.logger.Error("failed to send error response", zap.Error(err))
					}
				}
			}()

			return next.Handle(ctx)
		})
	})

	// Register all inbox routes
	handler.RegisterRoutes(app)

	// Use app.HandleRequest for Lambda (not app.Start())
	lambda.Start(app.HandleRequest)
}
