package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// DebugFederationTrace represents a single step in federation processing
type DebugFederationTrace struct {
	Timestamp   time.Time              `json:"timestamp"`
	Step        string                 `json:"step"`
	Direction   string                 `json:"direction"` // inbound/outbound
	Actor       string                 `json:"actor,omitempty"`
	RemoteURL   string                 `json:"remote_url,omitempty"`
	StatusCode  int                    `json:"status_code,omitempty"`
	Headers     map[string]string      `json:"headers,omitempty"`
	Body        json.RawMessage        `json:"body,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Duration    string                 `json:"duration,omitempty"`
	StorageInfo map[string]interface{} `json:"storage_info,omitempty"`
}

// DebugFederationResponse contains the complete trace of an activity
type DebugFederationResponse struct {
	ActivityID        string                 `json:"activity_id"`
	Type              string                 `json:"type"`
	Actor             string                 `json:"actor"`
	Created           time.Time              `json:"created"`
	Traces            []DebugFederationTrace `json:"traces"`
	ProcessingTime    string                 `json:"processing_time"`
	StorageLocations  map[string]string      `json:"storage_locations"`
	RelatedActivities []string               `json:"related_activities,omitempty"`
}

// DebugObjectResponse contains detailed information about a stored object
type DebugObjectResponse struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Object        interface{}            `json:"object"`
	Created       time.Time              `json:"created"`
	Actor         map[string]interface{} `json:"actor,omitempty"`
	Relationships map[string]interface{} `json:"relationships,omitempty"`
}

// HandleDebugFederationTrace traces the processing of a specific activity
func (h *Handler) HandleDebugFederationTrace(ctx context.Context, request events.APIGatewayV2HTTPRequest, activityID string) (*events.APIGatewayV2HTTPResponse, error) {
	startTime := time.Now()

	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check admin scope
	if !claims.HasScope("admin") && !claims.HasScope("debug") {
		return common.Forbidden(errors.New("admin or debug scope required")), nil
	}

	if activityID == "" {
		return common.BadRequest(errors.New("activity id required")), nil
	}

	// Get the activity
	activity, err := h.store.GetActivity(ctx, activityID)
	if err != nil {
		h.logger.Info("activity not found", zap.String("activity_id", activityID), zap.Error(err))
		return common.NotFound(fmt.Errorf("activity not found")), nil
	}

	// Build the trace response
	response := &DebugFederationResponse{
		ActivityID:       activityID,
		Type:             activity.Type,
		Actor:            activity.Actor,
		Traces:           []DebugFederationTrace{},
		StorageLocations: make(map[string]string),
	}

	// Set created time
	if activity.Published != nil {
		response.Created = *activity.Published
	}

	// Marshal activity to JSON for body
	activityJSON, _ := json.Marshal(activity)

	// Add basic trace information
	publishedTime := time.Now()
	if activity.Published != nil {
		publishedTime = *activity.Published
	}

	response.Traces = append(response.Traces, DebugFederationTrace{
		Timestamp: publishedTime,
		Step:      "activity_created",
		Direction: "inbound",
		Actor:     activity.Actor,
		Body:      json.RawMessage(activityJSON),
	})

	// Check if it's in inbox or outbox
	if strings.Contains(activity.Actor, h.cfg.BaseURL()) {
		// Local activity - check outbox
		response.Traces = append(response.Traces, DebugFederationTrace{
			Timestamp: publishedTime,
			Step:      "stored_in_outbox",
			Direction: "outbound",
		})
		response.StorageLocations["outbox"] = fmt.Sprintf("%s/outbox", activity.Actor)
	} else {
		// Remote activity - check inbox
		response.Traces = append(response.Traces, DebugFederationTrace{
			Timestamp: publishedTime,
			Step:      "stored_in_inbox",
			Direction: "inbound",
		})
		// Extract username from activity object/target
		if activity.Object != nil {
			if objMap, ok := activity.Object.(map[string]interface{}); ok {
				if toList, ok := objMap["to"].([]interface{}); ok && len(toList) > 0 {
					if toStr, ok := toList[0].(string); ok && strings.Contains(toStr, h.cfg.BaseURL()) {
						response.StorageLocations["inbox"] = toStr + "/inbox"
					}
				}
			}
		}
	}

	// Calculate processing time
	response.ProcessingTime = time.Since(startTime).String()

	// Add headers
	headers := map[string]string{
		"Content-Type":      "application/json",
		"X-Processing-Time": response.ProcessingTime,
		"X-Debug-Traces":    fmt.Sprintf("%d", len(response.Traces)),
	}

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleDebugObject provides detailed information about a stored object
func (h *Handler) HandleDebugObject(ctx context.Context, request events.APIGatewayV2HTTPRequest, objectID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check admin scope
	if !claims.HasScope("admin") && !claims.HasScope("debug") {
		return common.Forbidden(errors.New("admin or debug scope required")), nil
	}

	if objectID == "" {
		return common.BadRequest(errors.New("object id required")), nil
	}

	// Get the object
	obj, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("object not found")), nil
	}

	// Build response
	response := &DebugObjectResponse{
		ID:            objectID,
		Object:        obj,
		Relationships: make(map[string]interface{}),
	}

	// Determine object type and add metadata
	switch v := obj.(type) {
	case map[string]interface{}:
		if typeStr, ok := v["type"].(string); ok {
			response.Type = typeStr
		}
		if published, ok := v["published"].(string); ok {
			if t, err := time.Parse(time.RFC3339, published); err == nil {
				response.Created = t
			}
		}
		if actorStr, ok := v["attributedTo"].(string); ok {
			// Try to get actor info
			parts := strings.Split(actorStr, "/")
			if len(parts) > 0 {
				username := parts[len(parts)-1]
				if actor, err := h.store.GetActor(ctx, username); err == nil {
					response.Actor = map[string]interface{}{
						"id":       actor.ID,
						"username": actor.PreferredUsername,
						"name":     actor.Name,
						"type":     actor.Type,
					}
				}
			}
		}
	}

	// Check relationships
	likeCount, _ := h.store.CountObjectLikes(ctx, objectID)
	announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)

	response.Relationships["likes"] = map[string]interface{}{
		"count": likeCount,
		"url":   fmt.Sprintf("%s/likes", objectID),
	}
	response.Relationships["announces"] = map[string]interface{}{
		"count": announceCount,
		"url":   fmt.Sprintf("%s/shares", objectID),
	}

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleDebugReplay replays an activity for testing
func (h *Handler) HandleDebugReplay(ctx context.Context, request events.APIGatewayV2HTTPRequest, activityID string) (*events.APIGatewayV2HTTPResponse, error) {
	// TODO: Implement activity replay - would require storing raw activities and re-processing them
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusNotImplemented,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: `{"error":"Activity replay coming soon"}`,
	}, nil
}
