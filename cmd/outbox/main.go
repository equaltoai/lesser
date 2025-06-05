package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
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
		return handleGetOutbox(ctx, log, username, request.QueryStringParameters)
	case http.MethodPost:
		return handlePostOutbox(ctx, log, username, request)
	default:
		return common.BadRequest(fmt.Errorf("method %s not allowed", request.RequestContext.HTTP.Method)), nil
	}
}

// handleGetOutbox handles GET requests to retrieve outbox activities
func handleGetOutbox(ctx context.Context, log *zap.Logger, username string, queryParams map[string]string) (*events.APIGatewayV2HTTPResponse, error) {
	log.Info("received outbox GET request",
		zap.String("username", username),
		zap.Any("query_params", queryParams))

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
	limitStr := queryParams["limit"]
	if limitStr == "" {
		limitStr = "20"
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	cursor := queryParams["cursor"]
	page := queryParams["page"]

	// If no page parameter, return the collection with metadata
	if page == "" && cursor == "" {
		// Get first page to calculate total items (this is a simplification)
		activities, _, err := store.GetOutboxActivities(ctx, username, 1, "")
		if err != nil {
			log.Error("failed to get outbox count", zap.Error(err))
			return common.InternalServerError(err), nil
		}

		// Build the collection response
		collection := &activitypub.OrderedCollection{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      actor.Outbox,
					Type:    activitypub.OrderedCollectionType,
				},
				TotalItems: len(activities), // This is approximate
				First:      fmt.Sprintf("%s?page=true", actor.Outbox),
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
	activities, nextCursor, err := store.GetOutboxActivities(ctx, username, limit, cursor)
	if err != nil {
		log.Error("failed to get outbox activities", zap.Error(err))
		return common.InternalServerError(err), nil
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
					ID:      fmt.Sprintf("%s?page=true", actor.Outbox),
					Type:    "OrderedCollectionPage",
				},
				OrderedItems: orderedItems,
			},
			PartOf: actor.Outbox,
		},
	}

	// Add next link if there are more items
	if nextCursor != "" {
		collectionPage.Next = fmt.Sprintf("%s?page=true&cursor=%s&limit=%d", actor.Outbox, nextCursor, limit)
	}

	// Add prev link if we have a cursor (meaning this isn't the first page)
	if cursor != "" {
		collectionPage.Prev = fmt.Sprintf("%s?page=true&limit=%d", actor.Outbox, limit)
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

// handlePostOutbox handles POST requests to create activities
func handlePostOutbox(ctx context.Context, log *zap.Logger, username string, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	log.Info("received outbox POST request",
		zap.String("username", username),
		zap.String("content_type", request.Headers["Content-Type"]))

	// Verify authentication
	claims, err := authMiddleware.RequireAuth(ctx, request)
	if err != nil {
		log.Warn("authentication failed", zap.Error(err))
		return common.Unauthorized(err), nil
	}

	// Verify the authenticated user matches the username in the path
	if err := authMiddleware.RequireUser(claims, username); err != nil {
		log.Warn("user mismatch",
			zap.String("authenticated_user", claims.Username),
			zap.String("path_username", username))
		return common.Forbidden(err), nil
	}

	// Verify write scope
	if err := authMiddleware.RequireScope(claims, auth.ScopeWrite); err != nil {
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
		zap.String("id", activity.ID),
		zap.String("authenticated_user", claims.Username))

	// Verify that the activity's actor matches the authenticated user
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

	// Handle Create activities with embedded objects
	if activity.Type == activitypub.CreateType {
		if err := processCreateActivity(ctx, &activity, actor); err != nil {
			log.Warn("failed to process Create activity", zap.Error(err))
			return common.BadRequest(err), nil
		}
	}

	// Handle Like activities
	if activity.Type == activitypub.LikeType {
		if err := processLikeActivity(ctx, &activity, actor); err != nil {
			log.Warn("failed to process Like activity", zap.Error(err))
			return common.BadRequest(err), nil
		}
	}

	// Handle Announce activities
	if activity.Type == activitypub.AnnounceType {
		if err := processAnnounceActivity(ctx, &activity, actor); err != nil {
			log.Warn("failed to process Announce activity", zap.Error(err))
			return common.BadRequest(err), nil
		}
	}

	// Handle Delete activities
	if activity.Type == activitypub.DeleteType {
		if err := processDeleteActivity(ctx, &activity, actor); err != nil {
			log.Warn("failed to process Delete activity", zap.Error(err))
			return common.BadRequest(err), nil
		}
	}

	// Handle Update activities
	if activity.Type == activitypub.UpdateType {
		if err := processUpdateActivity(ctx, &activity, actor); err != nil {
			log.Warn("failed to process Update activity", zap.Error(err))
			return common.BadRequest(err), nil
		}
	}

	// Handle Undo activities
	if activity.Type == activitypub.UndoType {
		if err := processUndoActivity(ctx, &activity, actor); err != nil {
			log.Warn("failed to process Undo activity", zap.Error(err))
			return common.BadRequest(err), nil
		}
	}

	// Handle Block activities
	if activity.Type == activitypub.BlockType {
		if err := processBlockActivity(ctx, &activity, actor); err != nil {
			log.Warn("failed to process Block activity", zap.Error(err))
			return common.BadRequest(err), nil
		}
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

	// Fan out posts to timelines (for Create activities)
	if activity.Type == activitypub.CreateType {
		if err := store.FanOutPost(ctx, &activity); err != nil {
			// Log the error but don't fail the request
			log.Error("failed to fan out post to timelines", zap.Error(err))
		}
	}

	log.Info("activity created",
		zap.String("id", activity.ID),
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor))

	// Return 201 Created with the activity
	response := &events.APIGatewayV2HTTPResponse{
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

// processCreateActivity processes a Create activity and its embedded object
func processCreateActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Extract the object
	objMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		return common.ValidationError{Field: "object", Message: "Create activity must have an object"}
	}

	// Generate object ID if not provided
	if objMap["id"] == nil || objMap["id"] == "" {
		objType, _ := objMap["type"].(string)
		if objType == "" {
			objType = "Note"         // Default to Note
			objMap["type"] = objType // Set the type in the object
		}
		objMap["id"] = generateObjectID(actor.ID, objType)
	}

	// Set required fields
	objMap["attributedTo"] = actor.ID
	if objMap["published"] == nil {
		objMap["published"] = time.Now().UTC().Format(time.RFC3339)
	}

	// Copy addressing from activity if not set on object
	if objMap["to"] == nil && activity.To != nil {
		objMap["to"] = activity.To
	}
	if objMap["cc"] == nil && activity.CC != nil {
		objMap["cc"] = activity.CC
	}

	// Default addressing if none provided
	if objMap["to"] == nil {
		objMap["to"] = []string{activitypub.PublicAddress}
	}
	if objMap["cc"] == nil {
		// Only add followers if the actor has a Followers collection
		if actor.Followers != "" {
			objMap["cc"] = []string{actor.Followers}
		} else {
			objMap["cc"] = []string{}
		}
	}

	// Validate the object
	if err := validateObject(objMap); err != nil {
		return err
	}

	// Update the activity with the processed object
	activity.Object = objMap

	// Set activity published time if not set
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Copy addressing from object to activity if not set
	if activity.To == nil && objMap["to"] != nil {
		activity.To = convertToStringSlice(objMap["to"])
	}
	if activity.CC == nil && objMap["cc"] != nil {
		cc := convertToStringSlice(objMap["cc"])
		// Only set CC if it's not empty
		if len(cc) > 0 {
			activity.CC = cc
		}
	}

	return nil
}

// processLikeActivity processes a Like activity and validates its object
func processLikeActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Ensure the activity has an object
	if activity.Object == nil {
		return common.ValidationError{Field: "object", Message: "Like activity must have an object"}
	}

	// The object should be either a string (ID) or an object with an ID
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		} else {
			return common.ValidationError{Field: "object.id", Message: "Like object must have an ID"}
		}
	default:
		return common.ValidationError{Field: "object", Message: "Like object must be a string or object"}
	}

	// Validate the object ID is a valid URL
	if !isValidURL(objectID) {
		return common.ValidationError{Field: "object", Message: "Like object must be a valid URL"}
	}

	// Set activity published time if not set
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Default addressing if none provided
	if activity.To == nil {
		// Like activities are typically addressed to the object's actor
		// and optionally to public
		activity.To = []string{activitypub.PublicAddress}
	}

	return nil
}

// processAnnounceActivity processes an Announce activity and validates its object
func processAnnounceActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Ensure the activity has an object
	if activity.Object == nil {
		return common.ValidationError{Field: "object", Message: "Announce activity must have an object"}
	}

	// The object should be either a string (ID) or an object with an ID
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		} else {
			return common.ValidationError{Field: "object.id", Message: "Announce object must have an ID"}
		}
	default:
		return common.ValidationError{Field: "object", Message: "Announce object must be a string or object"}
	}

	// Validate the object ID is a valid URL
	if !isValidURL(objectID) {
		return common.ValidationError{Field: "object", Message: "Announce object must be a valid URL"}
	}

	// Set activity published time if not set
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Default addressing if none provided
	if activity.To == nil {
		// Announce activities are typically addressed to public and followers
		activity.To = []string{activitypub.PublicAddress}
	}
	if activity.CC == nil && actor.Followers != "" {
		activity.CC = []string{actor.Followers}
	}

	return nil
}

// processDeleteActivity processes a Delete activity and validates its object
func processDeleteActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Ensure the activity has an object
	if activity.Object == nil {
		return common.ValidationError{Field: "object", Message: "Delete activity must have an object"}
	}

	// The object should be either a string (ID) or a tombstone object
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		} else {
			return common.ValidationError{Field: "object.id", Message: "Delete object must have an ID"}
		}
	default:
		return common.ValidationError{Field: "object", Message: "Delete object must be a string or object"}
	}

	// Validate the object ID is a valid URL
	if !isValidURL(objectID) {
		return common.ValidationError{Field: "object", Message: "Delete object must be a valid URL"}
	}

	// Check that the object exists and belongs to the actor
	existingObj, err := store.GetObject(ctx, objectID)
	if err != nil {
		return common.ValidationError{Field: "object", Message: fmt.Sprintf("object not found: %s", objectID)}
	}

	// Check if it's already a tombstone
	if _, ok := existingObj.(*storage.Tombstone); ok {
		return common.ValidationError{Field: "object", Message: "object is already deleted"}
	}

	// Verify the actor owns the object
	var attributedTo string
	switch v := existingObj.(type) {
	case *dynamodb.Object:
		attributedTo = v.AttributedTo
	case map[string]interface{}:
		if attr, ok := v["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo != actor.ID {
		return common.ValidationError{Field: "object", Message: "you can only delete your own objects"}
	}

	// Create the tombstone
	if err := store.TombstoneObject(ctx, objectID, actor.ID); err != nil {
		return fmt.Errorf("failed to tombstone object: %w", err)
	}

	// Update the activity object to be a Tombstone
	activity.Object = map[string]interface{}{
		"id":         objectID,
		"type":       "Tombstone",
		"formerType": getObjectType(existingObj),
		"deleted":    time.Now().UTC().Format(time.RFC3339),
	}

	// Set activity published time if not set
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Default addressing if none provided
	if activity.To == nil {
		// Delete activities are typically addressed to followers and public
		activity.To = []string{activitypub.PublicAddress}
	}
	if activity.CC == nil && actor.Followers != "" {
		activity.CC = []string{actor.Followers}
	}

	return nil
}

// processUpdateActivity processes an Update activity and validates its object
func processUpdateActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Ensure the activity has an object
	if activity.Object == nil {
		return common.ValidationError{Field: "object", Message: "Update activity must have an object"}
	}

	// The object should be a map with all the object properties
	objMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		return common.ValidationError{Field: "object", Message: "Update object must be a map"}
	}

	// Extract object ID
	objectID, ok := objMap["id"].(string)
	if !ok || objectID == "" {
		return common.ValidationError{Field: "object.id", Message: "Update object must have an ID"}
	}

	// Validate the object ID is a valid URL
	if !isValidURL(objectID) {
		return common.ValidationError{Field: "object.id", Message: "Update object ID must be a valid URL"}
	}

	// Check that the object exists and belongs to the actor
	existingObj, err := store.GetObject(ctx, objectID)
	if err != nil {
		return common.ValidationError{Field: "object", Message: fmt.Sprintf("object not found: %s", objectID)}
	}

	// Check if it's already a tombstone
	if _, ok := existingObj.(*storage.Tombstone); ok {
		return common.ValidationError{Field: "object", Message: "cannot update a deleted object"}
	}

	// Verify the actor owns the object
	var attributedTo string
	switch v := existingObj.(type) {
	case *dynamodb.Object:
		attributedTo = v.AttributedTo
	case map[string]interface{}:
		if attr, ok := v["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo != actor.ID {
		return common.ValidationError{Field: "object", Message: "you can only update your own objects"}
	}

	// Set the attributedTo field to ensure consistency
	objMap["attributedTo"] = actor.ID

	// Set updated timestamp
	objMap["updated"] = time.Now().UTC().Format(time.RFC3339)

	// Preserve the original published time
	switch v := existingObj.(type) {
	case *dynamodb.Object:
		objMap["published"] = v.Published.Format(time.RFC3339)
	case map[string]interface{}:
		// Keep existing published time
		if _, hasPublished := objMap["published"]; !hasPublished {
			if pub, ok := v["published"]; ok {
				objMap["published"] = pub
			}
		}
	}

	// Validate the updated object
	if err := validateObject(objMap); err != nil {
		return err
	}

	// Update the object in storage (this will save history)
	if err := store.UpdateObject(ctx, objMap); err != nil {
		return fmt.Errorf("failed to update object: %w", err)
	}

	// Set activity published time if not set
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Default addressing if none provided
	if activity.To == nil {
		// Update activities are typically addressed to followers and public
		activity.To = []string{activitypub.PublicAddress}
	}
	if activity.CC == nil && actor.Followers != "" {
		activity.CC = []string{actor.Followers}
	}

	return nil
}

// processUndoActivity processes an Undo activity and validates its object
func processUndoActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Ensure the activity has an object
	if activity.Object == nil {
		return common.ValidationError{Field: "object", Message: "Undo activity must have an object"}
	}

	// The object should be a map representing the activity being undone
	objMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		return common.ValidationError{Field: "object", Message: "Undo object must be an activity"}
	}

	// Extract the type of the activity being undone
	undoType, ok := objMap["type"].(string)
	if !ok || undoType == "" {
		return common.ValidationError{Field: "object.type", Message: "Undo object must have a type"}
	}

	// Handle different undo types
	switch undoType {
	case activitypub.FollowType:
		return processUndoFollowActivity(ctx, activity, objMap, actor)
	case activitypub.LikeType:
		return processUndoLikeActivity(ctx, activity, objMap, actor)
	case activitypub.AnnounceType:
		return processUndoAnnounceActivity(ctx, activity, objMap, actor)
	default:
		return common.ValidationError{Field: "object.type", Message: fmt.Sprintf("cannot undo activity of type %s", undoType)}
	}
}

func processUndoFollowActivity(ctx context.Context, activity *activitypub.Activity, followObj map[string]interface{}, actor *activitypub.Actor) error {
	// Ensure the follow has required fields
	followActor, ok := followObj["actor"].(string)
	if !ok || followActor == "" {
		// If actor is not set in the follow object, set it to the current actor
		followActor = actor.ID
		followObj["actor"] = actor.ID
	}

	followObject, ok := followObj["object"].(string)
	if !ok || followObject == "" {
		return common.ValidationError{Field: "object.object", Message: "Follow activity must have an object"}
	}

	// Verify the actor matches
	if followActor != actor.ID {
		return common.ValidationError{Field: "object.actor", Message: "you can only undo your own follows"}
	}

	// Extract usernames
	followerUsername := extractUsernameFromActorID(followActor)
	followedUsername := extractUsernameFromActorID(followObject)

	if followerUsername == "" || followedUsername == "" {
		return common.ValidationError{Field: "object", Message: "invalid actor IDs in follow activity"}
	}

	// Check if the follow relationship exists
	isFollowing, err := store.IsFollowing(ctx, followerUsername, followedUsername)
	if err != nil {
		return fmt.Errorf("failed to check follow relationship: %w", err)
	}
	if !isFollowing {
		return common.ValidationError{Field: "object", Message: "you are not following this user"}
	}

	// Remove the follow relationship
	if err := store.RemoveFollow(ctx, followerUsername, followedUsername); err != nil {
		return fmt.Errorf("failed to remove follow: %w", err)
	}

	// Set the object with all required fields
	activity.Object = followObj

	// Set activity published time if not set
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Default addressing - send to the followed user
	if activity.To == nil {
		activity.To = []string{followObject}
	}

	return nil
}

func processUndoLikeActivity(ctx context.Context, activity *activitypub.Activity, likeObj map[string]interface{}, actor *activitypub.Actor) error {
	// Ensure the like has required fields
	likeActor, ok := likeObj["actor"].(string)
	if !ok || likeActor == "" {
		// If actor is not set in the like object, set it to the current actor
		likeActor = actor.ID
		likeObj["actor"] = actor.ID
	}

	var likeObject string
	switch obj := likeObj["object"].(type) {
	case string:
		likeObject = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			likeObject = id
		}
	}

	if likeObject == "" {
		return common.ValidationError{Field: "object.object", Message: "Like activity must have an object"}
	}

	// Verify the actor matches
	if likeActor != actor.ID {
		return common.ValidationError{Field: "object.actor", Message: "you can only undo your own likes"}
	}

	// Check if the like exists
	_, err := store.GetLike(ctx, likeActor, likeObject)
	if err != nil {
		return common.ValidationError{Field: "object", Message: "like not found"}
	}

	// Delete the like
	if err := store.DeleteLike(ctx, likeActor, likeObject); err != nil {
		return fmt.Errorf("failed to delete like: %w", err)
	}

	// Set the object with all required fields
	activity.Object = likeObj

	// Set activity published time if not set
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Default addressing if none provided
	if activity.To == nil {
		activity.To = []string{activitypub.PublicAddress}
	}

	return nil
}

func processUndoAnnounceActivity(ctx context.Context, activity *activitypub.Activity, announceObj map[string]interface{}, actor *activitypub.Actor) error {
	// Ensure the announce has required fields
	announceActor, ok := announceObj["actor"].(string)
	if !ok || announceActor == "" {
		// If actor is not set in the announce object, set it to the current actor
		announceActor = actor.ID
		announceObj["actor"] = actor.ID
	}

	var announceObject string
	switch obj := announceObj["object"].(type) {
	case string:
		announceObject = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			announceObject = id
		}
	}

	if announceObject == "" {
		return common.ValidationError{Field: "object.object", Message: "Announce activity must have an object"}
	}

	// Verify the actor matches
	if announceActor != actor.ID {
		return common.ValidationError{Field: "object.actor", Message: "you can only undo your own announces"}
	}

	// Check if the announce exists
	_, err := store.GetAnnounce(ctx, announceActor, announceObject)
	if err != nil {
		return common.ValidationError{Field: "object", Message: "announce not found"}
	}

	// Delete the announce
	if err := store.DeleteAnnounce(ctx, announceActor, announceObject); err != nil {
		return fmt.Errorf("failed to delete announce: %w", err)
	}

	// Set the object with all required fields
	activity.Object = announceObj

	// Set activity published time if not set
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Default addressing if none provided
	if activity.To == nil {
		activity.To = []string{activitypub.PublicAddress}
	}
	if activity.CC == nil && actor.Followers != "" {
		activity.CC = []string{actor.Followers}
	}

	return nil
}

// getObjectType extracts the type from an object
func getObjectType(obj interface{}) string {
	switch v := obj.(type) {
	case *dynamodb.Object:
		return v.Type
	case map[string]interface{}:
		if t, ok := v["type"].(string); ok {
			return t
		}
	}
	return "Object"
}

// validateObject validates the object within a Create activity
func validateObject(obj map[string]interface{}) error {
	// Check required fields
	if obj["type"] == nil || obj["type"] == "" {
		return common.ValidationError{Field: "object.type", Message: "object type is required"}
	}

	objType, _ := obj["type"].(string)

	// Validate based on type
	switch objType {
	case activitypub.NoteType:
		if obj["content"] == nil || obj["content"] == "" {
			return common.ValidationError{Field: "object.content", Message: "Note must have content"}
		}
		// Content length validation
		content, _ := obj["content"].(string)
		if len(content) > 500 {
			return common.ValidationError{Field: "object.content", Message: "Note content must not exceed 500 characters"}
		}
	case activitypub.ArticleType:
		if obj["name"] == nil || obj["name"] == "" {
			return common.ValidationError{Field: "object.name", Message: "Article must have a name"}
		}
		if obj["content"] == nil || obj["content"] == "" {
			return common.ValidationError{Field: "object.content", Message: "Article must have content"}
		}
		// Title length validation
		name, _ := obj["name"].(string)
		if len(name) > 200 {
			return common.ValidationError{Field: "object.name", Message: "Article name must not exceed 200 characters"}
		}
		// Content length validation
		content, _ := obj["content"].(string)
		if len(content) > 50000 {
			return common.ValidationError{Field: "object.content", Message: "Article content must not exceed 50000 characters"}
		}
	default:
		// Allow other object types but ensure they have at least an ID
		if obj["id"] == nil || obj["id"] == "" {
			return common.ValidationError{Field: "object.id", Message: "object must have an ID"}
		}
	}

	// Validate attachments if present
	if attachments, ok := obj["attachment"].([]interface{}); ok {
		for i, att := range attachments {
			if attMap, ok := att.(map[string]interface{}); ok {
				// Validate attachment URL
				if url, ok := attMap["url"].(string); ok {
					if !isValidURL(url) {
						return common.ValidationError{Field: fmt.Sprintf("object.attachment[%d].url", i), Message: "invalid attachment URL"}
					}
				} else {
					return common.ValidationError{Field: fmt.Sprintf("object.attachment[%d].url", i), Message: "attachment must have a URL"}
				}

				// Validate media type
				if mediaType, ok := attMap["mediaType"].(string); ok {
					if !isValidMediaType(mediaType) {
						return common.ValidationError{Field: fmt.Sprintf("object.attachment[%d].mediaType", i), Message: "unsupported media type"}
					}
				}
			}
		}
	}

	// Validate contentMap if present
	if contentMap, ok := obj["contentMap"].(map[string]interface{}); ok {
		for lang, content := range contentMap {
			// Validate language code (simple check for 2 or 2-2 format)
			if !isValidLanguageCode(lang) {
				return common.ValidationError{Field: fmt.Sprintf("object.contentMap.%s", lang), Message: "invalid language code"}
			}
			// Validate content length based on object type
			if contentStr, ok := content.(string); ok {
				if objType == activitypub.NoteType && len(contentStr) > 500 {
					return common.ValidationError{Field: fmt.Sprintf("object.contentMap.%s", lang), Message: "Note content must not exceed 500 characters"}
				} else if objType == activitypub.ArticleType && len(contentStr) > 50000 {
					return common.ValidationError{Field: fmt.Sprintf("object.contentMap.%s", lang), Message: "Article content must not exceed 50000 characters"}
				}
			}
		}
	}

	// Validate tags if present
	if tags, ok := obj["tag"].([]interface{}); ok {
		for i, tag := range tags {
			if tagMap, ok := tag.(map[string]interface{}); ok {
				tagType, _ := tagMap["type"].(string)
				name, _ := tagMap["name"].(string)

				switch tagType {
				case "Hashtag":
					if !strings.HasPrefix(name, "#") {
						return common.ValidationError{Field: fmt.Sprintf("object.tag[%d].name", i), Message: "Hashtag name must start with #"}
					}
					if len(name) < 2 || len(name) > 100 {
						return common.ValidationError{Field: fmt.Sprintf("object.tag[%d].name", i), Message: "Hashtag name must be between 2 and 100 characters"}
					}
				case "Mention":
					if href, ok := tagMap["href"].(string); !ok || href == "" {
						return common.ValidationError{Field: fmt.Sprintf("object.tag[%d].href", i), Message: "Mention must have an href"}
					}
				}
			}
		}
	}

	return nil
}

// generateObjectID generates a unique object ID for the given actor and object type
func generateObjectID(actorID, objectType string) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	random := generateRandomString(8)

	// Extract base URL from actor ID
	parts := strings.Split(actorID, "/users/")
	if len(parts) > 0 {
		return fmt.Sprintf("%s/objects/%s-%s", parts[0], timestamp, random)
	}
	return fmt.Sprintf("https://%s/objects/%s-%s", cfg.Domain, timestamp, random)
}

// convertToStringSlice converts an interface{} to []string
func convertToStringSlice(v interface{}) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return []string{val}
	default:
		return nil
	}
}

// Helper validation functions
func isValidURL(urlStr string) bool {
	if urlStr == "" {
		return false
	}
	// Simple check for HTTP(S) URLs
	return strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://")
}

func isValidMediaType(mediaType string) bool {
	// List of supported media types
	supportedTypes := []string{
		"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp",
		"video/mp4", "video/webm", "video/ogg",
		"audio/mp3", "audio/ogg", "audio/wav",
		"application/pdf",
	}

	for _, supported := range supportedTypes {
		if mediaType == supported {
			return true
		}
	}
	return false
}

func isValidLanguageCode(code string) bool {
	// Simple validation for ISO 639-1 (2 letters) or with region (e.g., en-US)
	if len(code) == 2 {
		return true
	}
	if len(code) == 5 && code[2] == '-' {
		return true
	}
	return false
}

// extractUsernameFromActorID extracts username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func extractUsernameFromActorID(actorID string) string {
	parts := strings.Split(actorID, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// processBlockActivity processes a Block activity and validates its object
func processBlockActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Ensure object is a string (actor ID)
	blockedActor, ok := activity.Object.(string)
	if !ok {
		// Try to extract from object map
		if objMap, ok := activity.Object.(map[string]interface{}); ok {
			if id, ok := objMap["id"].(string); ok {
				blockedActor = id
				activity.Object = id // Normalize to string
			} else {
				return common.ValidationError{Field: "object", Message: "Block object must be an actor ID"}
			}
		} else {
			return common.ValidationError{Field: "object", Message: "Block object must be an actor ID"}
		}
	}

	// Validate it's a URL
	if err := activitypub.ValidateURL(blockedActor, "object"); err != nil {
		return err
	}

	// Set default addressing if not provided
	if len(activity.To) == 0 && len(activity.CC) == 0 {
		// Block activities are typically not public
		// They're usually addressed to the blocked actor
		activity.To = []string{blockedActor}
	}

	// Generate ID if not provided
	if activity.ID == "" {
		activity.ID = generateActivityID(actor.ID, activity.Type)
	}

	// Set published time if not provided
	if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	// Store the block locally
	block := &storage.Block{
		Actor:     actor.ID,
		Object:    blockedActor,
		ID:        activity.ID,
		Published: *activity.Published,
	}

	if err := store.CreateBlock(ctx, block); err != nil {
		// Check if already blocked
		if strings.Contains(err.Error(), "already exists") {
			return common.ValidationError{Field: "object", Message: "already blocked"}
		}
		return fmt.Errorf("failed to create block: %w", err)
	}

	return nil
}

func main() {
	lambda.Start(handler)
}
