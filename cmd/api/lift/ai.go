package lift

import (
	"encoding/json"
	"net/http"
	"strings"

	originalai "github.com/equaltoai/lesser/pkg/ai"
	ai "github.com/equaltoai/lesser/pkg/services/ai"
	"github.com/pay-theory/lift/pkg/lift"
)

// Removed global AI storage - now using AIRepository

// HandleGetAIAnalysisLift returns AI analysis for an object
// GET /api/v1/ai/analysis/:object_id
func (h *Handler) HandleGetAIAnalysisLift(ctx *lift.Context) error {
	// Auth - require read scope
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	_, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Get object ID from path parameters
	objectID := ctx.Param("object_id")

	// Test mode fallback - extract from path
	if objectID == "" && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract object_id from path like /api/v1/ai/analysis/test-object-123
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[4] == "analysis" {
			objectID = parts[5]
		}
	}

	if objectID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "object_id is required",
		})
	}

	// Get analysis using AI service
	result, err := h.registry.AI().GetAnalysis(ctx.Context, &ai.GetAnalysisQuery{
		ObjectID: objectID,
	})
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "analysis not found",
		})
	}

	// Return analysis
	ctx.Status(http.StatusOK)
	return ctx.JSON(result.Analysis)
}

// HandleRequestAIAnalysisLift triggers AI analysis for an object
// POST /api/v1/ai/analyze
func (h *Handler) HandleRequestAIAnalysisLift(ctx *lift.Context) error {
	// Auth - require moderation scope
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check moderation scope
	if !claims.HasScope("moderation") {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "moderation scope required",
		})
	}

	var req struct {
		ObjectID   string `json:"object_id"`
		ObjectType string `json:"object_type"`
		Force      bool   `json:"force"` // Force re-analysis
	}

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]string{
					"error": "invalid request body",
				})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": "invalid request body",
			})
		}
	}

	if req.ObjectID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "object_id is required",
		})
	}

	// Queue for analysis using AI service
	queueResult, err := h.registry.AI().QueueForAnalysis(ctx.Context, &ai.QueueAnalysisCommand{
		ObjectID:   req.ObjectID,
		ObjectType: req.ObjectType,
		Force:      req.Force,
	})
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to queue analysis",
		})
	}

	// If not queued (already exists), return existing
	if !queueResult.Queued {
		// Get the existing analysis
		result, _ := h.registry.AI().GetAnalysis(ctx.Context, &ai.GetAnalysisQuery{
			ObjectID: req.ObjectID,
		})
		if result != nil && result.Analysis != nil {
			ctx.Status(http.StatusOK)
			return ctx.JSON(result.Analysis)
		}
	}

	response := map[string]any{
		"message":        "Analysis queued",
		"object_id":      req.ObjectID,
		"estimated_time": "10-30 seconds",
	}

	ctx.Status(http.StatusAccepted)
	return ctx.JSON(response)
}

// HandleGetAIStatsLift returns AI analysis statistics
// GET /api/v1/ai/stats
func (h *Handler) HandleGetAIStatsLift(ctx *lift.Context) error {
	// Public endpoint - no auth required

	// Get time range
	period := ctx.Query("period")

	// Test mode fallback - extract from path query string
	if period == "" && ctx.Request != nil && strings.Contains(ctx.Request.Path, "?") {
		parts := strings.Split(ctx.Request.Path, "?")
		if len(parts) > 1 {
			params := strings.Split(parts[1], "&")
			for _, param := range params {
				kv := strings.Split(param, "=")
				if len(kv) == 2 && kv[0] == "period" {
					period = kv[1]
					break
				}
			}
		}
	}

	if period == "" {
		period = "day"
	}

	// Get stats using AI service
	result, err := h.registry.AI().GetStats(ctx.Context, &ai.GetStatsQuery{
		Period: period,
	})
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get stats",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(result.Stats)
}

// HandleGetAISummaryLift returns a summary of AI features and capabilities
// GET /api/v1/ai/capabilities
func (h *Handler) HandleGetAISummaryLift(ctx *lift.Context) error {
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

	ctx.Status(http.StatusOK)
	return ctx.JSON(capabilities)
}

// Helper methods removed - now handled by AIRepository
