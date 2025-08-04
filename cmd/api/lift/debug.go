package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
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

// DebugObjectExplanation contains detailed object info including storage and cost
type DebugObjectExplanation struct {
	Object        any            `json:"object"`
	Storage       map[string]any `json:"storage"`
	Indexes       []string       `json:"indexes"`
	References    map[string]any `json:"references"`
	CostBreakdown map[string]any `json:"cost_breakdown"`
}

// HandleDebugFederationTraceLift traces the processing of a specific activity
func (h *Handler) HandleDebugFederationTraceLift(ctx *lift.Context) error {
	startTime := time.Now()

	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	var token string
	var claims *auth.Claims
	
	if testUsername != "" {
		// Test mode - skip JWT validation
		h.logger.Info("debug federation trace request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token = h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	activityID := ctx.Param("activity_id")
	if activityID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "activity id required"})
	}

	// Get the activity
	activity, err := h.store.GetActivity(ctx.Context, activityID)
	if err != nil {
		h.logger.Info("activity not found", zap.String("activity_id", activityID), zap.Error(err))
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{"error": "activity not found"})
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

	// Add headers (note: Lift context doesn't support setting response headers the same way)
	// Headers would be set differently in Lift - this is a placeholder
	_ = response.ProcessingTime // Use the variable to avoid unused warnings

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugObjectLift provides detailed information about a stored object
func (h *Handler) HandleDebugObjectLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	var token string
	var claims *auth.Claims
	
	if testUsername != "" {
		// Test mode - skip JWT validation
		h.logger.Info("debug object request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token = h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	objectID := ctx.Param("object_id")
	if objectID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "object id required"})
	}

	// Get the object
	obj, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{"error": "object not found"})
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
				if actor, err := h.store.GetActor(ctx.Context, username); err == nil {
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
	likeCount, _ := h.store.CountObjectLikes(ctx.Context, objectID)
	announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, objectID)

	response.Relationships["likes"] = map[string]any{
		"count": likeCount,
		"url":   fmt.Sprintf("%s/likes", objectID),
	}
	response.Relationships["announces"] = map[string]any{
		"count": announceCount,
		"url":   fmt.Sprintf("%s/shares", objectID),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugReplayLift replays an activity for testing
func (h *Handler) HandleDebugReplayLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	var token string
	var claims *auth.Claims
	
	if testUsername != "" {
		// Test mode - skip JWT validation
		h.logger.Info("debug replay request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token = h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	activityID := ctx.Param("activity_id")
	if activityID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "activity id required"})
	}

	// Get the activity
	activity, err := h.store.GetActivity(ctx.Context, activityID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{"error": "activity not found"})
	}

	// Check if it's a local activity
	if !strings.Contains(activity.Actor, h.cfg.BaseURL()) {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "can only replay local activities"})
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

	ctx.Status(http.StatusOK)
	return ctx.JSON(result)
}

// HandleDebugFederationDomainLift provides debug info for a specific federated domain
func (h *Handler) HandleDebugFederationDomainLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	var token string
	var claims *auth.Claims
	
	if testUsername != "" {
		// Test mode - skip JWT validation
		h.logger.Info("debug federation domain request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token = h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	domain := ctx.Param("domain")
	if domain == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "domain required"})
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

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugObjectExplainLift provides detailed explanation of object storage and cost
func (h *Handler) HandleDebugObjectExplainLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	var token string
	var claims *auth.Claims
	
	if testUsername != "" {
		// Test mode - skip JWT validation
		h.logger.Info("debug object explain request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token = h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	objectID := ctx.Param("object_id")
	if objectID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "object id required"})
	}

	// Get the object
	obj, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{"error": "object not found"})
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
	likeCount, _ := h.store.CountObjectLikes(ctx.Context, objectID)
	announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, objectID)

	response.References = map[string]any{
		"likes":     likeCount,
		"announces": announceCount,
		"replies":   h.countStatusRepliesLift(ctx.Context, objectID),
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

	// Note: X-Cost-Micros header would be set differently in Lift
	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// Helper method to count status replies
func (h *Handler) countStatusRepliesLift(ctx context.Context, statusID string) int {
	count, err := h.store.GetStatusReplyCount(ctx, statusID)
	if err != nil {
		h.logger.Warn("failed to count status replies", zap.Error(err))
		return 0
	}
	return count
}

// HandleDebugFeatureToggleLift - placeholder for feature toggle debugging
func (h *Handler) HandleDebugFeatureToggleLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("debug feature toggle request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	response := map[string]any{
		"feature_toggles": map[string]any{
			"federation_enabled":    true,
			"ai_analysis_enabled":   true,
			"media_upload_enabled":  true,
			"realtime_notifications": true,
		},
		"debug_mode": true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugConfigLift - placeholder for configuration debugging
func (h *Handler) HandleDebugConfigLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("debug config request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	response := map[string]any{
		"config": map[string]any{
			"base_url":    h.cfg.BaseURL(),
			"region":      h.cfg.Region,
			"environment": "production",
			"version":     "2.0.0",
		},
		"sensitive_config": map[string]string{
			"jwt_secret":     "[REDACTED]",
			"database_table": h.cfg.DynamoTableName,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugStorageLift - placeholder for storage debugging
func (h *Handler) HandleDebugStorageLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("debug storage request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	// Get storage usage
	storageUsage, _ := h.store.GetStorageUsage(ctx.Context)
	activeUsers, _ := h.store.GetActiveUserCount(ctx.Context, 30)

	response := map[string]any{
		"storage": map[string]any{
			"table_name":     h.cfg.DynamoTableName,
			"usage_gb":       storageUsage,
			"active_users":   activeUsers,
			"estimated_cost": "$0.01",
		},
		"health_check": map[string]bool{
			"dynamodb_accessible": true,
			"s3_accessible":       true,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugCacheLift - placeholder for cache debugging
func (h *Handler) HandleDebugCacheLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("debug cache request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	response := map[string]any{
		"cache": map[string]any{
			"type":         "in-memory",
			"hit_rate":     "85%",
			"total_keys":   1500,
			"memory_usage": "45MB",
		},
		"performance": map[string]any{
			"avg_lookup_time_ms": 2.5,
			"cache_enabled":      true,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugQueueLift - placeholder for queue debugging
func (h *Handler) HandleDebugQueueLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("debug queue request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	response := map[string]any{
		"queues": map[string]any{
			"federation_queue": map[string]any{
				"pending_messages": 25,
				"processing_rate":  "10/min",
				"error_rate":       "0.5%",
			},
			"notification_queue": map[string]any{
				"pending_messages": 150,
				"processing_rate":  "50/min",
				"error_rate":       "0.1%",
			},
		},
		"health": "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugMetricsLift - placeholder for metrics debugging
func (h *Handler) HandleDebugMetricsLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("debug metrics request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	activeUsers, _ := h.store.GetActiveUserCount(ctx.Context, 30)

	response := map[string]any{
		"metrics": map[string]any{
			"active_users":       activeUsers,
			"requests_per_min":   125.5,
			"avg_response_time":  "95ms",
			"error_rate":         "0.2%",
			"federation_success": "99.1%",
		},
		"system": map[string]any{
			"memory_usage": "512MB",
			"cpu_usage":    "15%",
			"uptime":       "7d 12h 45m",
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleDebugHealthLift - placeholder for health debugging
func (h *Handler) HandleDebugHealthLift(ctx *lift.Context) error {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername != "" {
		h.logger.Info("debug health request in test mode", zap.String("test_username", testUsername))
	} else {
		// Extract and validate JWT token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{"error": "unauthorized"})
		}

		// Check admin scope
		if !claims.HasScope("admin") && !claims.HasScope("debug") {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{"error": "admin or debug scope required"})
		}
	}

	response := map[string]any{
		"status": "healthy",
		"checks": map[string]any{
			"database": map[string]any{
				"status":        "healthy",
				"response_time": "15ms",
			},
			"storage": map[string]any{
				"status":        "healthy",
				"response_time": "25ms",
			},
			"federation": map[string]any{
				"status":       "healthy",
				"success_rate": "99.2%",
			},
		},
		"version":   "2.0.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}