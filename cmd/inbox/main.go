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
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
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
	domainBlockRepository        *repositories.DomainBlockRepository
	userRepository               *repositories.UserRepository
	logger                       *zap.Logger
	authMiddleware               *auth.Middleware
	rateLimiter                  *auth.RateLimiter
	tableName                    string
}

// NewInboxHandler creates a new inbox handler
func NewInboxHandler() (*InboxHandler, error) {
	logger := common.Logger()
	cfg := config.Get()

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Initialize repositories
	actorRepo := repositories.NewActorRepository(db, cfg.DynamoTableName, logger)
	activityRepo := repositories.NewActivityRepository(db, cfg.DynamoTableName, logger)
	followRepo := repositories.NewRelationshipRepository(db, cfg.DynamoTableName, logger)
	objectRepo := repositories.NewObjectRepository(db, cfg.DynamoTableName, logger)
	likeRepo := repositories.NewLikeRepository(db, cfg.DynamoTableName, logger)
	federationActivityRepo := repositories.NewFederationActivityRepository(db, cfg.DynamoTableName, logger)
	domainBlockRepo := repositories.NewDomainBlockRepository(db, cfg.DynamoTableName, logger)
	userRepo := repositories.NewUserRepository(db, cfg.DynamoTableName, logger)

	// Initialize auth middleware
	authMiddleware, err := auth.GetMiddleware()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth middleware: %w", err)
	}

	// Initialize rate limiter for federation instances
	// TODO: Update rate limiter to use DynamORM
	rateLimiter := auth.NewRateLimiter(nil)

	return &InboxHandler{
		db:                           db,
		actorRepository:              actorRepo,
		activityRepository:           activityRepo,
		relationshipRepository:       followRepo,
		objectRepository:             objectRepo,
		likeRepository:               likeRepo,
		federationActivityRepository: federationActivityRepo,
		domainBlockRepository:        domainBlockRepo,
		userRepository:               userRepo,
		logger:                       logger,
		authMiddleware:               authMiddleware,
		rateLimiter:                  rateLimiter,
		tableName:                    cfg.DynamoTableName,
	}, nil
}

// RegisterRoutes registers all inbox routes
func (ih *InboxHandler) RegisterRoutes(app *lift.App) {
	// ActivityPub inbox endpoints
	app.GET("/inbox/{username}", ih.handleGetInbox)
	app.POST("/inbox/{username}", ih.handlePostInbox)
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

// handlePostInbox handles POST requests to receive activities
func (ih *InboxHandler) handlePostInbox(ctx *lift.Context) error {
	username := ctx.Param("username")
	if username == "" {
		return lift.NewLiftError("VALIDATION_ERROR", "missing username parameter", 400)
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
			return lift.NewLiftError("NOT_FOUND", "actor not found", 404)
		}
		ih.logger.Error("failed to get actor", zap.Error(err))
		return lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
	}

	// Parse the activity with size limit
	body := ctx.Request.Body
	if len(body) == 0 {
		return lift.NewLiftError("VALIDATION_ERROR", "request body is required", 400)
	}

	if len(body) > common.MaxActivitySize {
		ih.logger.Warn("request body too large", zap.Int("size", len(body)))
		return lift.NewLiftError("PAYLOAD_TOO_LARGE", "request body too large", 413)
	}

	// Safe JSON parsing for ActivityPub objects
	var activity activitypub.Activity
	if err := common.ParseActivityPubObject(body, &activity); err != nil {
		ih.logger.Warn("failed to parse activity", zap.Error(err))
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid activity: %v", err), 400)
	}

	// Sanitize any embedded objects in the activity
	if objMap, ok := activity.Object.(map[string]any); ok {
		common.SanitizeActivityPubObjectDefault(objMap)
		activity.Object = objMap
	}

	ih.logger.Info("processing activity",
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor),
		zap.String("id", activity.ID))

	// Verify required fields
	if activity.ID == "" {
		return lift.NewLiftError("VALIDATION_ERROR", "activity ID is required", 400)
	}
	if activity.Actor == "" {
		return lift.NewLiftError("VALIDATION_ERROR", "actor is required", 400)
	}
	if activity.Type == "" {
		return lift.NewLiftError("VALIDATION_ERROR", "activity type is required", 400)
	}

	// Check if activity is addressed to this actor
	if !ih.isAddressedTo(&activity, actor) {
		ih.logger.Warn("activity not addressed to this actor",
			zap.String("actor_id", actor.ID),
			zap.Any("to", activity.To),
			zap.Any("cc", activity.CC))
		return lift.NewLiftError("VALIDATION_ERROR", "activity is not addressed to this actor", 400)
	}

	// Track start time for response time measurement
	startTime := time.Now()

	// Extract domain from actor URL
	actorDomain := ih.extractDomainFromURL(activity.Actor)

	// Record federation activity for cost tracking
	federationActivity := &models.FederationActivity{
		Domain:       actorDomain,
		ActivityType: activity.Type,
		InboundSize:  int64(len(body)),
		Timestamp:    startTime,
	}

	// Rate limiting per ActivityPub instance
	if actorDomain != "" {
		// Use domain as the "username" for rate limiting purposes
		if err := ih.rateLimiter.CheckRateLimit(ctx.Context, actorDomain, ctx.Header("X-Forwarded-For")); err != nil {
			ih.logger.Warn("rate limit exceeded",
				zap.String("domain", actorDomain),
				zap.Error(err))

			federationActivity.Success = false
			federationActivity.ErrorMessage = "Rate limit exceeded"
			federationActivity.ResponseTime = float64(time.Since(startTime).Milliseconds())
			go func() {
				if err := ih.federationActivityRepository.Create(context.Background(), federationActivity); err != nil {
					ih.logger.Warn("failed to record federation activity", zap.Error(err))
				}
			}()

			return lift.NewLiftError("RATE_LIMITED", "rate limit exceeded for domain", 429)
		}

		// Record the rate limit attempt
		if err := ih.rateLimiter.RecordAttempt(ctx.Context, actorDomain, ctx.Header("X-Forwarded-For"), false); err != nil {
			ih.logger.Warn("failed to record rate limit attempt", zap.Error(err))
		}

		// Check if the domain is blocked at the instance level
		isBlocked, block, err := ih.domainBlockRepository.IsDomainBlocked(ctx.Context, actorDomain)
		if err != nil {
			ih.logger.Error("failed to check domain block status",
				zap.String("domain", actorDomain),
				zap.Error(err))
			// Continue processing on error - fail open rather than closed
		} else if isBlocked && block != nil {
			ih.logger.Info("rejecting activity from blocked domain",
				zap.String("domain", actorDomain),
				zap.String("severity", block.Severity),
				zap.String("actor", activity.Actor))

			// For suspended domains, reject completely
			if block.Severity == "suspend" {
				federationActivity.Success = false
				federationActivity.ErrorMessage = "Domain is suspended"
				federationActivity.ResponseTime = float64(time.Since(startTime).Milliseconds())
				go func() {
					if err := ih.federationActivityRepository.Create(context.Background(), federationActivity); err != nil {
						ih.logger.Warn("failed to record federation activity", zap.Error(err))
					}
				}()

				return lift.NewLiftError("FORBIDDEN", "domain is suspended", 403)
			}

			// For silenced domains, we accept but may limit visibility
			// This is handled later in processing
		}
	}

	// Fetch the sender's public key
	publicKey, err := ih.fetchActorPublicKey(ctx.Context, activity.Actor)
	if err != nil {
		ih.logger.Error("failed to fetch actor public key",
			zap.String("actor", activity.Actor),
			zap.Error(err))

		federationActivity.Success = false
		federationActivity.ErrorMessage = fmt.Sprintf("Failed to fetch actor public key: %v", err)
		federationActivity.ResponseTime = float64(time.Since(startTime).Milliseconds())
		go func() {
			if err := ih.federationActivityRepository.Create(context.Background(), federationActivity); err != nil {
				ih.logger.Warn("failed to record federation activity", zap.Error(err))
			}
		}()

		return lift.NewLiftError("VALIDATION_ERROR", "unable to verify sender", 400)
	}

	// Verify HTTP signature
	if err := ih.verifyRequest(ctx, publicKey, body); err != nil {
		ih.logger.Warn("signature verification failed",
			zap.String("actor", activity.Actor),
			zap.Error(err))

		federationActivity.Success = false
		federationActivity.ErrorMessage = fmt.Sprintf("Signature verification failed: %v", err)
		federationActivity.ResponseTime = float64(time.Since(startTime).Milliseconds())
		go func() {
			if err := ih.federationActivityRepository.Create(context.Background(), federationActivity); err != nil {
				ih.logger.Warn("failed to record federation activity", zap.Error(err))
			}
		}()

		return lift.NewLiftError("UNAUTHORIZED", "signature verification failed", 401)
	}

	// Verify digest if present
	if ctx.Header("Digest") != "" {
		httpReq, err := ih.convertLiftRequest(ctx, body)
		if err == nil {
			if err := federation.VerifyDigest(httpReq, body); err != nil {
				ih.logger.Warn("digest verification failed",
					zap.String("actor", activity.Actor),
					zap.Error(err))
				return lift.NewLiftError("VALIDATION_ERROR", "digest verification failed", 400)
			}
		}
	}

	// Store in inbox (the storage layer will automatically put it in the inbox based on actor)
	err = ih.activityRepository.CreateActivity(ctx.Context, &activity)
	if err != nil {
		ih.logger.Error("failed to store activity", zap.Error(err))
		return lift.NewLiftError("INTERNAL_ERROR", "failed to store activity", 500)
	}

	// Process different activity types
	switch activity.Type {
	case activitypub.FollowType:
		if err := ih.processFollowActivity(ctx.Context, &activity, actor); err != nil {
			ih.logger.Error("failed to process follow activity", zap.Error(err))
			return lift.NewLiftError("INTERNAL_ERROR", "failed to process follow activity", 500)
		}
	case activitypub.AcceptType:
		if err := ih.processAcceptActivity(ctx.Context, &activity, actor); err != nil {
			ih.logger.Error("failed to process accept activity", zap.Error(err))
			return lift.NewLiftError("INTERNAL_ERROR", "failed to process accept activity", 500)
		}
	case activitypub.RejectType:
		if err := ih.processRejectActivity(ctx.Context, &activity, actor); err != nil {
			ih.logger.Error("failed to process reject activity", zap.Error(err))
			return lift.NewLiftError("INTERNAL_ERROR", "failed to process reject activity", 500)
		}
	case activitypub.CreateType:
		if err := ih.processRemoteCreateActivity(ctx.Context, &activity, actor); err != nil {
			ih.logger.Error("failed to process create activity", zap.Error(err))
			return lift.NewLiftError("INTERNAL_ERROR", "failed to process create activity", 500)
		}
	case activitypub.UpdateType:
		if err := ih.processRemoteUpdateActivity(ctx.Context, &activity, actor); err != nil {
			ih.logger.Error("failed to process update activity", zap.Error(err))
			return lift.NewLiftError("INTERNAL_ERROR", "failed to process update activity", 500)
		}
	case activitypub.DeleteType:
		if err := ih.processRemoteDeleteActivity(ctx.Context, &activity, actor); err != nil {
			ih.logger.Error("failed to process delete activity", zap.Error(err))
			return lift.NewLiftError("INTERNAL_ERROR", "failed to process delete activity", 500)
		}
	case activitypub.UndoType:
		if err := ih.processUndoActivity(ctx.Context, &activity, actor); err != nil {
			ih.logger.Error("failed to process undo activity", zap.Error(err))
			return lift.NewLiftError("INTERNAL_ERROR", "failed to process undo activity", 500)
		}
	}

	ih.logger.Info("activity accepted and processed",
		zap.String("id", activity.ID),
		zap.String("type", activity.Type),
		zap.String("from", activity.Actor))

	// Record successful federation activity and rate limit success
	federationActivity.Success = true
	federationActivity.ResponseTime = float64(time.Since(startTime).Milliseconds())
	go ih.federationActivityRepository.Create(context.Background(), federationActivity)

	if actorDomain != "" {
		// Mark rate limit attempt as successful
		if err := ih.rateLimiter.RecordAttempt(ctx.Context, actorDomain, ctx.Header("X-Forwarded-For"), true); err != nil {
			ih.logger.Warn("failed to record rate limit success", zap.Error(err))
		}
	}

	// Return 202 Accepted
	return ctx.Status(http.StatusAccepted).Text("")
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

	// Create request
	req, err := http.NewRequest(ctx.Request.Method, u.String(), strings.NewReader(string(body)))
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

// fetchActorPublicKey fetches an actor's public key from their profile
func (ih *InboxHandler) fetchActorPublicKey(ctx context.Context, actorURL string) (crypto.PublicKey, error) {
	log := common.WithContext(ctx)

	// Create secure HTTP client with DNS caching
	client := httpclient.NewSecureClient(
		httpclient.WithTimeout(10*time.Second),
		httpclient.WithLogger(log),
		// TODO: Update httpclient to use DynamORM if needed
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

	// Send Accept activity back to the follower
	// TODO: Update delivery service to use DynamORM
	log.Info("would deliver accept activity",
		zap.String("to", activity.Actor),
		zap.String("from", targetActor.ID))

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
					ctx.Status(500).Text("Internal server error")
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
