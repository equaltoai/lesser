package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	originalai "github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	ai "github.com/equaltoai/lesser/pkg/services/ai"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

// Removed global AI storage - now using AIRepository

// HandleGetAIAnalysisLift returns AI analysis for an object
// GET /api/v1/ai/analysis/:object_id
func (h *Handler) HandleGetAIAnalysisLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Auth - require read scope
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	_, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid token",
		})
	}

	// Get object ID from path parameters
	objectID := ctx.Param("object_id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("object_id", objectID); err != nil && ctx.Request.Path != "" {
		// Extract object_id from path like /api/v1/ai/analysis/test-object-123
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[4] == "analysis" {
			objectID = parts[5]
		}
	}

	if err := common.ValidateRequiredParam("object_id", objectID); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Get analysis using AI service
	result, err := h.registry.AI().GetAnalysis(ctx.Context(), &ai.GetAnalysisQuery{
		ObjectID: objectID,
	})
	if err != nil {
		return apptheory.JSON(http.StatusNotFound, map[string]string{
			"error": "analysis not found",
		})
	}

	// Return analysis
	return okJSON(result.Analysis)
}

// HandleRequestAIAnalysisLift triggers AI analysis for an object
// POST /api/v1/ai/analyze
func (h *Handler) HandleRequestAIAnalysisLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Auth - require moderation scope
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid token",
		})
	}

	// Check moderation scope
	if !claims.HasScope("moderation") {
		return apptheory.JSON(http.StatusForbidden, map[string]string{
			"error": "moderation scope required",
		})
	}

	var req struct {
		ObjectID   string `json:"object_id"`
		ObjectType string `json:"object_type"`
		Force      bool   `json:"force"` // Force re-analysis
	}

	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		// Fallback for test environments / missing content-type
		if len(ctx.Request.Body) == 0 {
			return apptheory.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
			return apptheory.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
	}

	if err := common.ValidateRequiredParam("object_id", req.ObjectID); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Queue for analysis using AI service
	queueResult, err := h.registry.AI().QueueForAnalysis(ctx.Context(), &ai.QueueAnalysisCommand{
		ObjectID:   req.ObjectID,
		ObjectType: req.ObjectType,
		Force:      req.Force,
	})
	if err != nil {
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to queue analysis",
		})
	}

	// If not queued (already exists), return existing
	if !queueResult.Queued {
		// Get the existing analysis
		result, _ := h.registry.AI().GetAnalysis(ctx.Context(), &ai.GetAnalysisQuery{
			ObjectID: req.ObjectID,
		})
		if result != nil && result.Analysis != nil {
			return okJSON(result.Analysis)
		}
	}

	response := map[string]any{
		"message":        "Analysis queued",
		"object_id":      req.ObjectID,
		"estimated_time": "10-30 seconds",
	}

	return apptheory.JSON(http.StatusAccepted, response)
}

// HandleGetAIStatsLift returns AI analysis statistics
// GET /api/v1/ai/stats
func (h *Handler) HandleGetAIStatsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Public endpoint - no auth required

	// Get time range
	period := queryValue(ctx, "period")

	if err := common.ValidateRequiredParam("period", period); err != nil {
		period = "day"
	}

	// Get stats using AI service
	result, err := h.registry.AI().GetStats(ctx.Context(), &ai.GetStatsQuery{
		Period: period,
	})
	if err != nil {
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to get stats",
		})
	}

	return okJSON(result.Stats)
}

// HandleGetAISummaryLift returns a summary of AI features and capabilities
// GET /api/v1/ai/capabilities
func (h *Handler) HandleGetAISummaryLift(_ *apptheory.Context) (*apptheory.Response, error) {
	capabilities := map[string]any{
		"text_analysis": map[string]any{
			"sentiment_analysis": true,
			"toxicity_detection": true,
			"spam_detection":     true,
			"pii_detection":      true,
			"entity_extraction":  true,
			"language_detection": true,
		},
		"image_analysis": map[string]any{
			"nsfw_detection":        true,
			"violence_detection":    true,
			"text_extraction":       true,
			"celebrity_recognition": true,
			"deepfake_detection":    false, // Future feature
		},
		"ai_detection": map[string]any{
			"ai_generated_content": true,
			"pattern_analysis":     true,
			"style_consistency":    true,
		},
		"moderation_actions": []string{
			"none",
			"flag",
			"hide",
			"remove",
			"shadow_ban",
			"review",
		},
		"cost_per_analysis": originalai.CostPerOperation,
	}

	return okJSON(capabilities)
}

// Helper methods removed - now handled by AIRepository
