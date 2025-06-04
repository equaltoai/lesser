package main

import (
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
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
	cfg    *config.Config
	store  storage.Storage
	logger *zap.Logger
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
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log := common.WithContext(ctx)

	// Only accept POST requests
	if request.HTTPMethod != http.MethodPost {
		return common.BadRequest(fmt.Errorf("method %s not allowed", request.HTTPMethod)), nil
	}

	// Extract username from path
	username := request.PathParameters["username"]
	if username == "" {
		return common.BadRequest(common.ValidationError{Field: "username", Message: "missing username"}), nil
	}

	log.Info("received inbox request",
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
	return events.APIGatewayProxyResponse{
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
func verifyRequest(request *events.APIGatewayProxyRequest, publicKey crypto.PublicKey, body []byte) error {
	// Convert Lambda request to http.Request for signature verification
	req, err := convertLambdaRequest(request, body)
	if err != nil {
		return fmt.Errorf("failed to convert request: %w", err)
	}

	return federation.VerifyHTTPSignature(req, publicKey)
}

// convertLambdaRequest converts a Lambda API Gateway request to an http.Request
func convertLambdaRequest(request *events.APIGatewayProxyRequest, body []byte) (*http.Request, error) {
	// Build URL
	u := &url.URL{
		Scheme: "https",
		Host:   request.Headers["Host"],
		Path:   request.Path,
	}
	if request.QueryStringParameters != nil {
		q := u.Query()
		for k, v := range request.QueryStringParameters {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	// Create request
	req, err := http.NewRequest(request.HTTPMethod, u.String(), strings.NewReader(string(body)))
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
