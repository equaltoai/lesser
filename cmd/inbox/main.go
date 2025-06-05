package main

import (
	"context"
	"crypto"
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

	// Parse the activity
	body := []byte(request.Body)
	var activity activitypub.Activity
	if err := json.Unmarshal(body, &activity); err != nil {
		log.Warn("failed to parse activity", zap.Error(err))
		return common.BadRequest(common.ValidationError{Field: "body", Message: "invalid JSON"}), nil
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

	// Fetch the sender's public key
	publicKey, err := fetchActorPublicKey(ctx, activity.Actor)
	if err != nil {
		log.Error("failed to fetch actor public key",
			zap.String("actor", activity.Actor),
			zap.Error(err))
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

	log.Info("activity accepted",
		zap.String("id", activity.ID),
		zap.String("type", activity.Type),
		zap.String("from", activity.Actor))

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

	// Create HTTP client
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

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
	if err := json.NewDecoder(resp.Body).Decode(&actor); err != nil {
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

func main() {
	lambda.Start(handler)
}
