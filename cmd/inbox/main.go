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

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/httpclient"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg            *config.Config
	store          storage.Storage
	logger         *zap.Logger
	authMiddleware *auth.Middleware
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	// Initialize storage
	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Initialize auth middleware
	authMiddleware, err = auth.GetMiddleware()
	if err != nil {
		logger.Fatal("failed to initialize auth middleware", zap.Error(err))
	}
}

func handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	log := common.WithContext(ctx)

	// Extract username from path
	username := request.PathParameters["username"]
	if username == "" {
		return common.BadRequest(common.ValidationError{Field: "username", Message: "missing username"}), nil
	}

	// Route based on HTTP method
	switch request.RequestContext.HTTP.Method {
	case http.MethodGet:
		return handleGetInbox(ctx, log, username, request)
	case http.MethodPost:
		return handlePostInbox(ctx, log, username, request)
	default:
		return common.BadRequest(fmt.Errorf("method %s not allowed", request.RequestContext.HTTP.Method)), nil
	}
}

// handleGetInbox handles GET requests to retrieve inbox activities
func handleGetInbox(ctx context.Context, log *zap.Logger, username string, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	log.Info("received inbox GET request",
		zap.String("username", username),
		zap.Any("query_params", request.QueryStringParameters))

	// Require authentication
	claims, err := authMiddleware.RequireAuth(ctx, request)
	if err != nil {
		log.Warn("authentication failed", zap.Error(err))
		return common.Unauthorized(err), nil
	}

	// Verify user owns this inbox
	if err := authMiddleware.RequireUser(claims, username); err != nil {
		log.Warn("user mismatch",
			zap.String("authenticated_user", claims.Username),
			zap.String("inbox_owner", username))
		return common.Forbidden(err), nil
	}

	// Verify read scope
	if err := authMiddleware.RequireScope(claims, auth.ScopeRead); err != nil {
		log.Warn("insufficient scope", zap.Error(err))
		return common.Forbidden(err), nil
	}

	// Verify the actor exists
	actor, err := store.GetActor(ctx, username)
	if err != nil {
		if common.IsNotFound(err) {
			return common.NotFound(err), nil
		}
		log.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Parse pagination parameters
	limitStr := request.QueryStringParameters["limit"]
	if limitStr == "" {
		limitStr = "20"
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	cursor := request.QueryStringParameters["cursor"]
	page := request.QueryStringParameters["page"]

	// If no page parameter, return the collection with metadata
	if page == "" && cursor == "" {
		// Get first page to calculate total items (this is a simplification)
		activities, _, err := store.GetInboxActivities(ctx, username, 1, "")
		if err != nil {
			log.Error("failed to get inbox count", zap.Error(err))
			return common.InternalServerError(err), nil
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

		// Serialize the collection
		responseBody, err := json.Marshal(collection)
		if err != nil {
			log.Error("failed to serialize collection", zap.Error(err))
			return common.InternalServerError(err), nil
		}

		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"Content-Type": "application/activity+json",
			},
			Body: string(responseBody),
		}, nil
	}

	// Get activities for the page
	activities, nextCursor, err := store.GetInboxActivities(ctx, username, limit, cursor)
	if err != nil {
		log.Error("failed to get inbox activities", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Enrich activities with objects if they contain Create activities
	for _, activity := range activities {
		if activity.Type == activitypub.CreateType && activity.Object != nil {
			// If Object is just an ID string, fetch the full object
			if objID, ok := activity.Object.(string); ok {
				obj, err := store.GetObject(ctx, objID)
				if err != nil {
					log.Warn("failed to fetch object for activity",
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
	orderedItems := make([]interface{}, len(activities))
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

	// Serialize the page
	responseBody, err := json.Marshal(collectionPage)
	if err != nil {
		log.Error("failed to serialize page", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/activity+json",
		},
		Body: string(responseBody),
	}, nil
}

// handlePostInbox handles POST requests to receive activities
func handlePostInbox(ctx context.Context, log *zap.Logger, username string, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	log.Info("received inbox POST request",
		zap.String("username", username),
		zap.String("content_type", request.Headers["Content-Type"]))

	// Verify the actor exists
	actor, err := store.GetActor(ctx, username)
	if err != nil {
		if common.IsNotFound(err) {
			return common.NotFound(err), nil
		}
		log.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Parse the activity with size limit
	body, err := common.ReadRequestBody(strings.NewReader(request.Body), common.MaxActivitySize)
	if err != nil {
		if strings.Contains(err.Error(), "too large") {
			log.Warn("request body too large", zap.Error(err))
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: 413, // Payload Too Large
				Body:       fmt.Sprintf(`{"error": "%s"}`, err.Error()),
			}, nil
		}
		log.Warn("failed to read request body", zap.Error(err))
		return common.BadRequest(common.ValidationError{Field: "body", Message: "failed to read request body"}), nil
	}

	// Safe JSON parsing for ActivityPub objects
	var activity activitypub.Activity
	if err := common.ParseActivityPubObject(body, &activity); err != nil {
		log.Warn("failed to parse activity", zap.Error(err))
		return common.BadRequest(common.ValidationError{Field: "body", Message: err.Error()}), nil
	}

	// Sanitize any embedded objects in the activity
	if objMap, ok := activity.Object.(map[string]interface{}); ok {
		common.SanitizeActivityPubObjectDefault(objMap)
		activity.Object = objMap
	}

	log.Info("processing activity",
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor),
		zap.String("id", activity.ID))

	// Verify required fields
	if activity.ID == "" {
		return common.BadRequest(common.ValidationError{Field: "id", Message: "activity ID is required"}), nil
	}
	if activity.Actor == "" {
		return common.BadRequest(common.ValidationError{Field: "actor", Message: "actor is required"}), nil
	}
	if activity.Type == "" {
		return common.BadRequest(common.ValidationError{Field: "type", Message: "activity type is required"}), nil
	}

	// Check if activity is addressed to this actor
	if !isAddressedTo(&activity, actor) {
		log.Warn("activity not addressed to this actor",
			zap.String("actor_id", actor.ID),
			zap.Any("to", activity.BaseObject.To),
			zap.Any("cc", activity.BaseObject.CC))
		return common.BadRequest(common.ValidationError{
			Field:   "addressing",
			Message: "activity is not addressed to this actor",
		}), nil
	}

	// Track start time for response time measurement
	startTime := time.Now()

	// Extract domain from actor URL
	actorDomain := extractDomainFromURL(activity.Actor)
	
	// Record federation activity for cost tracking
	federationActivity := &storage.FederationActivity{
		Domain:       actorDomain,
		Type:         "ingress",
		ActivityType: activity.Type,
		ByteSize:     int64(len(body)),
		Timestamp:    startTime,
	}
	
	if actorDomain != "" {
		// Check if the domain is blocked at the instance level
		isBlocked, block, err := store.IsDomainBlocked(ctx, actorDomain)
		if err != nil {
			log.Error("failed to check domain block status",
				zap.String("domain", actorDomain),
				zap.Error(err))
			// Continue processing on error - fail open rather than closed
		} else if isBlocked && block != nil {
			log.Info("rejecting activity from blocked domain",
				zap.String("domain", actorDomain),
				zap.String("severity", block.Severity),
				zap.String("actor", activity.Actor))

			// For suspended domains, reject completely
			if block.Severity == "suspend" {
				federationActivity.Success = false
				federationActivity.ErrorMessage = "Domain is suspended"
				federationActivity.ResponseTime = time.Since(startTime).Milliseconds()
				go store.RecordFederationActivity(context.Background(), federationActivity)
				
				return &events.APIGatewayV2HTTPResponse{
					StatusCode: http.StatusForbidden,
					Body:       `{"error": "Domain is suspended"}`,
				}, nil
			}

			// For silenced domains, we accept but may limit visibility
			// This is handled later in processing
		}
	}

	// Fetch the sender's public key
	publicKey, err := fetchActorPublicKey(ctx, activity.Actor)
	if err != nil {
		log.Error("failed to fetch actor public key",
			zap.String("actor", activity.Actor),
			zap.Error(err))
		
		federationActivity.Success = false
		federationActivity.ErrorMessage = fmt.Sprintf("Failed to fetch actor public key: %v", err)
		federationActivity.ResponseTime = time.Since(startTime).Milliseconds()
		go store.RecordFederationActivity(context.Background(), federationActivity)
		
		return common.BadRequest(common.ValidationError{
			Field:   "actor",
			Message: "unable to verify sender",
		}), nil
	}

	// Verify HTTP signature
	if err := verifyRequest(&request, publicKey, body); err != nil {
		log.Warn("signature verification failed",
			zap.String("actor", activity.Actor),
			zap.Error(err))
		
		federationActivity.Success = false
		federationActivity.ErrorMessage = fmt.Sprintf("Signature verification failed: %v", err)
		federationActivity.ResponseTime = time.Since(startTime).Milliseconds()
		go store.RecordFederationActivity(context.Background(), federationActivity)
		
		return common.Unauthorized(err), nil
	}

	// Verify digest if present
	if request.Headers["Digest"] != "" {
		httpReq, err := convertLambdaRequest(&request, body)
		if err == nil {
			if err := federation.VerifyDigest(httpReq, body); err != nil {
				log.Warn("digest verification failed",
					zap.String("actor", activity.Actor),
					zap.Error(err))
				return common.BadRequest(common.ValidationError{
					Field:   "digest",
					Message: "digest verification failed",
				}), nil
			}
		}
	}

	// Store in inbox (the storage layer will automatically put it in the inbox based on actor)
	err = store.CreateActivity(ctx, &activity)
	if err != nil {
		log.Error("failed to store activity", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Process different activity types
	switch activity.Type {
	case activitypub.FollowType:
		if err := processFollowActivity(ctx, &activity, actor); err != nil {
			log.Error("failed to process follow activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	case activitypub.AcceptType:
		if err := processAcceptActivity(ctx, &activity, actor); err != nil {
			log.Error("failed to process accept activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	case activitypub.RejectType:
		if err := processRejectActivity(ctx, &activity, actor); err != nil {
			log.Error("failed to process reject activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	case activitypub.CreateType:
		if err := processRemoteCreateActivity(ctx, &activity, actor); err != nil {
			log.Error("failed to process create activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	case activitypub.UpdateType:
		if err := processRemoteUpdateActivity(ctx, &activity, actor); err != nil {
			log.Error("failed to process update activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	case activitypub.DeleteType:
		if err := processRemoteDeleteActivity(ctx, &activity, actor); err != nil {
			log.Error("failed to process delete activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	case activitypub.UndoType:
		if err := processUndoActivity(ctx, &activity, actor); err != nil {
			log.Error("failed to process undo activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	}

	log.Info("activity accepted and processed",
		zap.String("id", activity.ID),
		zap.String("type", activity.Type),
		zap.String("from", activity.Actor))

	// Record successful federation activity
	federationActivity.Success = true
	federationActivity.ResponseTime = time.Since(startTime).Milliseconds()
	go store.RecordFederationActivity(context.Background(), federationActivity)

	// Return 202 Accepted
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusAccepted,
	}, nil
}

// isAddressedTo checks if the activity is addressed to the given actor
func isAddressedTo(activity *activitypub.Activity, actor *activitypub.Actor) bool {
	actorID := actor.ID
	inboxURL := actor.Inbox

	// Check 'to' field
	for _, to := range activity.BaseObject.To {
		if to == actorID || to == inboxURL || to == activitypub.PublicAddress {
			return true
		}
	}

	// Check 'cc' field
	for _, cc := range activity.BaseObject.CC {
		if cc == actorID || cc == inboxURL || cc == activitypub.PublicAddress {
			return true
		}
	}

	// Check 'bto' field
	for _, bto := range activity.BaseObject.BTo {
		if bto == actorID || bto == inboxURL {
			return true
		}
	}

	// Check 'bcc' field
	for _, bcc := range activity.BaseObject.BCC {
		if bcc == actorID || bcc == inboxURL {
			return true
		}
	}

	return false
}

// verifyRequest verifies the HTTP signature on the request
func verifyRequest(request *events.APIGatewayV2HTTPRequest, publicKey crypto.PublicKey, body []byte) error {
	// Convert Lambda request to http.Request for signature verification
	req, err := convertLambdaRequest(request, body)
	if err != nil {
		return fmt.Errorf("failed to convert request: %w", err)
	}

	return federation.VerifyHTTPSignature(req, publicKey)
}

// convertLambdaRequest converts a Lambda API Gateway request to an http.Request
func convertLambdaRequest(request *events.APIGatewayV2HTTPRequest, body []byte) (*http.Request, error) {
	// Get the path, removing the stage prefix if present
	path := request.RawPath
	if request.RequestContext.Stage != "" && strings.HasPrefix(path, "/"+request.RequestContext.Stage) {
		path = strings.TrimPrefix(path, "/"+request.RequestContext.Stage)
	}

	// Build URL
	u := &url.URL{
		Scheme: "https",
		Host:   request.Headers["Host"],
		Path:   path,
	}
	if request.QueryStringParameters != nil {
		q := u.Query()
		for k, v := range request.QueryStringParameters {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	// Create request
	req, err := http.NewRequest(request.RequestContext.HTTP.Method, u.String(), strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	// Copy headers (normalize to canonical form)
	for k, v := range request.Headers {
		req.Header.Set(k, v)
	}

	// Set host header if not present
	if req.Header.Get("Host") == "" && request.Headers["Host"] != "" {
		req.Host = request.Headers["Host"]
	}

	return req, nil
}

// fetchActorPublicKey fetches an actor's public key from their profile
func fetchActorPublicKey(ctx context.Context, actorURL string) (crypto.PublicKey, error) {
	log := common.WithContext(ctx)

	// Create secure HTTP client with DNS caching
	client := httpclient.NewSecureClient(
		httpclient.WithTimeout(10*time.Second),
		httpclient.WithLogger(log),
		httpclient.WithStorage(store),
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
	defer resp.Body.Close()

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
func processFollowActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Extract follower username from actor ID
	followerHandle := extractHandleFromActorID(activity.Actor)

	// Create the follow relationship with pending state
	err := store.CreateFollow(ctx, followerHandle, targetActor.PreferredUsername, activity.ID)
	if err != nil {
		log.Error("failed to create follow relationship", zap.Error(err))
		return err
	}

	// For now, auto-accept all follows (TODO: implement manual approval option)
	err = store.AcceptFollow(ctx, followerHandle, targetActor.PreferredUsername)
	if err != nil {
		log.Error("failed to accept follow", zap.Error(err))
		return err
	}

	// Send Accept activity back to the follower
	acceptActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        fmt.Sprintf("%s/activities/%s", targetActor.ID, generateActivityID()),
			Type:      activitypub.AcceptType,
			Published: timePtr(time.Now()),
		},
		Actor:  targetActor.ID,
		Object: activity.ID, // Reference the original Follow activity
	}

	// Deliver the Accept activity
	deliveryService := federation.NewDeliveryService(store)
	if err := deliveryService.DeliverActivity(ctx, acceptActivity, activity.Actor, targetActor); err != nil {
		log.Error("failed to deliver accept activity", zap.Error(err))
		// Don't fail the operation if delivery fails
	}

	return nil
}

// processAcceptActivity processes an incoming Accept activity
func processAcceptActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Check if this is accepting a Follow request
	if objectID, ok := activity.Object.(string); ok {
		// Fetch the original activity
		originalActivity, err := store.GetActivity(ctx, objectID)
		if err != nil {
			log.Warn("failed to find original activity", zap.String("id", objectID))
			return nil // Don't fail, just ignore
		}

		if originalActivity.Type == activitypub.FollowType {
			// Update the follow relationship to accepted
			acceptorHandle := extractHandleFromActorID(activity.Actor)
			err = store.AcceptFollow(ctx, targetActor.PreferredUsername, acceptorHandle)
			if err != nil {
				log.Error("failed to update follow status", zap.Error(err))
				return err
			}
		}
	}

	return nil
}

// processRejectActivity processes an incoming Reject activity
func processRejectActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Check if this is rejecting a Follow request
	if objectID, ok := activity.Object.(string); ok {
		// Fetch the original activity
		originalActivity, err := store.GetActivity(ctx, objectID)
		if err != nil {
			log.Warn("failed to find original activity", zap.String("id", objectID))
			return nil // Don't fail, just ignore
		}

		if originalActivity.Type == activitypub.FollowType {
			// Remove the follow relationship
			rejectorHandle := extractHandleFromActorID(activity.Actor)
			err = store.RemoveFollow(ctx, targetActor.PreferredUsername, rejectorHandle)
			if err != nil {
				log.Error("failed to remove follow", zap.Error(err))
				return err
			}
		}
	}

	return nil
}

// processRemoteCreateActivity processes an incoming Create activity from a remote instance
func processRemoteCreateActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Extract the object
	objMap, ok := activity.Object.(map[string]interface{})
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
		if err := store.CreateObject(ctx, &note); err != nil {
			log.Error("failed to store remote note", zap.Error(err))
			return err
		}
	}

	return nil
}

// processRemoteUpdateActivity processes an incoming Update activity from a remote instance
func processRemoteUpdateActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Extract the object
	objMap, ok := activity.Object.(map[string]interface{})
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
		if err := store.UpdateObject(ctx, &note); err != nil {
			log.Error("failed to update remote note", zap.Error(err))
			// Don't fail if we can't update (might not have it)
		}
	}

	return nil
}

// processRemoteDeleteActivity processes an incoming Delete activity from a remote instance
func processRemoteDeleteActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Get the object ID to delete
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if objectID == "" {
		log.Warn("delete activity has no object ID")
		return nil
	}

	// Delete the object
	if err := store.DeleteObject(ctx, objectID); err != nil {
		log.Warn("failed to delete object", zap.String("id", objectID), zap.Error(err))
		// Don't fail if we can't delete (might not have it)
	}

	return nil
}

// processUndoActivity processes an incoming Undo activity
func processUndoActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Get the activity being undone
	var originalActivity *activitypub.Activity

	switch obj := activity.Object.(type) {
	case string:
		// Fetch the activity by ID
		var err error
		originalActivity, err = store.GetActivity(ctx, obj)
		if err != nil {
			log.Warn("failed to find activity to undo", zap.String("id", obj))
			return nil
		}
	case map[string]interface{}:
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
		unfollowerHandle := extractHandleFromActorID(activity.Actor)
		err := store.RemoveFollow(ctx, unfollowerHandle, targetActor.PreferredUsername)
		if err != nil {
			log.Error("failed to remove follow", zap.Error(err))
			return err
		}
	case activitypub.LikeType:
		// Undo like
		if objectID, ok := originalActivity.Object.(string); ok {
			err := store.DeleteLike(ctx, activity.Actor, objectID)
			if err != nil {
				log.Warn("failed to remove like", zap.Error(err))
				// Don't fail
			}
		}
	}

	return nil
}

// Helper functions

func extractHandleFromActorID(actorID string) string {
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

func extractUsernameFromHandle(handle string) string {
	// Extract username from handle @username@domain
	parts := strings.Split(handle, "@")
	if len(parts) >= 2 {
		return parts[1]
	}
	return handle
}

func generateActivityID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), generateRandomString(8))
}

func generateRandomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)

	// Use crypto/rand for secure random generation
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		// This should rarely happen, but we handle it gracefully
		logger.Error("Failed to generate secure random bytes", zap.Error(err))
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

func timePtr(t time.Time) *time.Time {
	return &t
}

// extractDomainFromURL extracts the domain from an ActivityPub actor URL
func extractDomainFromURL(actorURL string) string {
	u, err := url.Parse(actorURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func main() {
	lambda.Start(handler)
}
