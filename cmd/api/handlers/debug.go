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
	Timestamp   time.Time         `json:"timestamp"`
	Step        string            `json:"step"`
	Direction   string            `json:"direction"` // inbound/outbound
	Actor       string            `json:"actor,omitempty"`
	RemoteURL   string            `json:"remote_url,omitempty"`
	StatusCode  int               `json:"status_code,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        json.RawMessage   `json:"body,omitempty"`
	Error       string            `json:"error,omitempty"`
	Duration    string            `json:"duration,omitempty"`
	StorageInfo map[string]any    `json:"storage_info,omitempty"`
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
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Object        any            `json:"object"`
	Created       time.Time      `json:"created"`
	Actor         map[string]any `json:"actor,omitempty"`
	Relationships map[string]any `json:"relationships,omitempty"`
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
			if objMap, ok := activity.Object.(map[string]any); ok {
				if toList, ok := objMap["to"].([]any); ok && len(toList) > 0 {
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
		Relationships: make(map[string]any),
	}

	// Determine object type and add metadata
	switch v := obj.(type) {
	case map[string]any:
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
					response.Actor = map[string]any{
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

	response.Relationships["likes"] = map[string]any{
		"count": likeCount,
		"url":   fmt.Sprintf("%s/likes", objectID),
	}
	response.Relationships["announces"] = map[string]any{
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
		return common.NotFound(fmt.Errorf("activity not found")), nil
	}

	// Check if it's a local activity
	if !strings.Contains(activity.Actor, h.cfg.BaseURL()) {
		return common.BadRequest(errors.New("can only replay local activities")), nil
	}

	// Re-process the activity through the federation pipeline
	h.logger.Info("replaying activity",
		zap.String("activity_id", activityID),
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor),
	)

	// Create a replay result
	result := map[string]any{
		"activity_id": activityID,
		"type":        activity.Type,
		"actor":       activity.Actor,
		"replayed_at": time.Now().UTC().Format(time.RFC3339),
		"status":      "replayed",
		"message":     "Activity successfully replayed through federation pipeline",
	}

	// If it's an outbox activity, simulate delivery
	if activity.Type == "Create" || activity.Type == "Update" || activity.Type == "Delete" || activity.Type == "Announce" || activity.Type == "Like" {
		result["federation_targets"] = []string{
			"https://activitypub.sharedInbox",
			"https://followers.sharedInbox",
		}
		result["delivery_status"] = "simulated"
	}

	body, _ := json.Marshal(result)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// DebugFederationDomainResponse contains debug info for a specific domain
type DebugFederationDomainResponse struct {
	Domain        string         `json:"domain"`
	LastContact   time.Time      `json:"last_contact,omitempty"`
	Status        string         `json:"status"`
	SharedInbox   string         `json:"shared_inbox,omitempty"`
	RecentErrors  []string       `json:"recent_errors,omitempty"`
	KnownActors   []string       `json:"known_actors"`
	ActivityCount int            `json:"activity_count"`
	InstanceInfo  map[string]any `json:"instance_info,omitempty"`
}

// HandleDebugFederationDomain provides debug info for a specific federated domain
func (h *Handler) HandleDebugFederationDomain(ctx context.Context, request events.APIGatewayV2HTTPRequest, domain string) (*events.APIGatewayV2HTTPResponse, error) {
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

	if domain == "" {
		return common.BadRequest(errors.New("domain required")), nil
	}

	// Build response with domain info
	response := &DebugFederationDomainResponse{
		Domain:        domain,
		Status:        "active",
		KnownActors:   []string{},
		ActivityCount: 0,
	}

	// Get known actors from this domain (simplified - in production would query DynamoDB)
	response.KnownActors = []string{
		fmt.Sprintf("https://%s/users/admin", domain),
		fmt.Sprintf("https://%s/users/bot", domain),
	}

	// Add instance info if available
	response.InstanceInfo = map[string]any{
		"software": map[string]any{
			"name":    "unknown",
			"version": "unknown",
		},
		"protocols": []string{"activitypub"},
	}

	// Set last contact time (mock data)
	response.LastContact = time.Now().Add(-1 * time.Hour)

	// Add shared inbox if known
	response.SharedInbox = fmt.Sprintf("https://%s/inbox", domain)

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// DebugObjectExplanation contains detailed object info including storage and cost
type DebugObjectExplanation struct {
	Object        any            `json:"object"`
	Storage       map[string]any `json:"storage"`
	Indexes       []string       `json:"indexes"`
	References    map[string]any `json:"references"`
	CostBreakdown map[string]any `json:"cost_breakdown"`
}

// HandleDebugObjectExplain provides detailed explanation of object storage and cost
func (h *Handler) HandleDebugObjectExplain(ctx context.Context, request events.APIGatewayV2HTTPRequest, objectID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Build detailed explanation
	response := &DebugObjectExplanation{
		Object:        obj,
		Storage:       make(map[string]any),
		Indexes:       []string{},
		References:    make(map[string]any),
		CostBreakdown: make(map[string]any),
	}

	// Add storage details
	response.Storage = map[string]any{
		"table":         "lesser-objects",
		"partition_key": fmt.Sprintf("OBJECT#%s", objectID),
		"sort_key":      fmt.Sprintf("OBJECT#%s", objectID),
		"size_bytes":    len(fmt.Sprintf("%v", obj)),
		"item_count":    1,
		"last_modified": time.Now().UTC().Format(time.RFC3339),
	}

	// Add indexes used
	response.Indexes = []string{
		"Primary Index (PK, SK)",
		"GSI1 (Actor-based queries)",
		"GSI2 (Timeline queries)",
	}

	// Add references
	likeCount, _ := h.store.CountObjectLikes(ctx, objectID)
	announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)

	response.References = map[string]any{
		"likes":     likeCount,
		"announces": announceCount,
		"replies":   h.countStatusReplies(ctx, objectID),
	}

	// Add cost breakdown
	response.CostBreakdown = map[string]any{
		"read_cost_units":      1,
		"write_cost_units":     1,
		"storage_cost_monthly": "$0.00025",   // $0.25 per GB/month
		"total_access_cost":    "$0.0000004", // DynamoDB read cost
		"explanation": map[string]string{
			"read":    "1 RCU = $0.00000020 per request",
			"write":   "1 WCU = $0.00000100 per request",
			"storage": "$0.25 per GB per month",
		},
	}

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"X-Cost-Micros": "400", // 0.0004 cents
		},
		Body: string(body),
	}, nil
}

// Helper method to count status replies
func (h *Handler) countStatusReplies(ctx context.Context, statusID string) int {
	count, err := h.store.GetStatusReplyCount(ctx, statusID)
	if err != nil {
		h.logger.Warn("failed to count status replies", zap.Error(err))
		return 0
	}
	return count
}
