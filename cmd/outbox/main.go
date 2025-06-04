package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

	log.Info("received outbox request",
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

	log.Info("processing outbox activity",
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor),
		zap.String("id", activity.ID))

	// Verify that the activity's actor matches the username
	// The actor should be set to the local user's ID
	if activity.Actor == "" {
		// If actor is not set, set it to the local actor
		activity.Actor = actor.ID
	} else if activity.Actor != actor.ID {
		// Ensure the activity's actor matches the authenticated user
		log.Warn("activity actor does not match authenticated user",
			zap.String("activity_actor", activity.Actor),
			zap.String("user_actor", actor.ID))
		return common.BadRequest(common.ValidationError{
			Field:   "actor",
			Message: "activity actor must match the authenticated user",
		}), nil
	}

	// Generate activity ID if not provided
	if activity.ID == "" {
		activity.ID = generateActivityID(actor.ID, activity.Type)
	}

	// Validate the activity
	if err := activitypub.ValidateActivity(&activity); err != nil {
		log.Warn("activity validation failed",
			zap.String("actor", activity.Actor),
			zap.Error(err))
		return common.BadRequest(err), nil
	}

	// Store in outbox (storage layer will automatically put it in the outbox based on actor)
	err = store.CreateActivity(ctx, &activity)
	if err != nil {
		log.Error("failed to store activity", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	log.Info("activity created",
		zap.String("id", activity.ID),
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor))

	// Return 201 Created with the activity
	response := events.APIGatewayProxyResponse{
		StatusCode: http.StatusCreated,
		Headers: map[string]string{
			"Content-Type": "application/activity+json",
			"Location":     activity.ID,
		},
	}

	// Serialize the activity for the response
	responseBody, err := json.Marshal(activity)
	if err != nil {
		log.Error("failed to serialize response", zap.Error(err))
		return common.InternalServerError(err), nil
	}
	response.Body = string(responseBody)

	return response, nil
}

// generateActivityID generates a unique activity ID for the given actor and activity type
func generateActivityID(actorID, activityType string) string {
	// Generate a timestamp-based ID
	timestamp := time.Now().UTC().Format("20060102-150405") + "-" + generateRandomString(8)

	// Extract base URL from actor ID
	// e.g., "https://example.com/users/alice" -> "https://example.com"
	parts := strings.Split(actorID, "/users/")
	if len(parts) < 1 {
		// Fallback to using the full actor ID
		return fmt.Sprintf("%s/activities/%s", actorID, timestamp)
	}

	baseURL := parts[0]
	return fmt.Sprintf("%s/activities/%s", baseURL, timestamp)
}

// generateRandomString generates a random string of the specified length
func generateRandomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return string(result)
}

func main() {
	lambda.Start(handler)
}
