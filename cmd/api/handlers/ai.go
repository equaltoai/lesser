package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	originalai "github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	ai "github.com/equaltoai/lesser/pkg/services/ai"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

type aiAnalysisLiftRequest struct {
	ObjectID   string `json:"object_id"`
	ObjectType string `json:"object_type"`
	Force      bool   `json:"force"` // Force re-analysis
}

// Removed global AI storage - now using AIRepository

// HandleGetAIAnalysisLift returns AI analysis for an object
// GET /api/v1/ai/analysis/:object_id
func (h *Handler) HandleGetAIAnalysisLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Auth - require read scope
	claims, resp, err := h.authenticatedClaimsWithResponder(ctx, common.RespondMissingAuth, common.RespondInvalidToken)
	if resp != nil || err != nil {
		return resp, err
	}
	if !claims.HasScope(auth.ScopeRead) && !claims.HasScope(auth.ScopeAdmin) {
		return common.RespondInsufficientScope(ctx, auth.ScopeRead)
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

	adminViewer, resp, err := h.authorizeAIAnalysisViewer(ctx, claims, objectID)
	if resp != nil || err != nil {
		return resp, err
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
	analysis := result.Analysis
	if !adminViewer {
		analysis = originalai.RedactPIIFromAnalysis(analysis)
	}
	return okJSON(analysis)
}

func (h *Handler) authorizeAIAnalysisViewer(ctx *apptheory.Context, claims *auth.Claims, objectID string) (bool, *apptheory.Response, error) {
	adminViewer, err := h.claimsUserHasAdminRole(ctx.Context(), claims)
	if err != nil {
		resp, respErr := common.RespondInternalServerError(ctx)
		return false, resp, respErr
	}
	if adminViewer {
		return true, nil, nil
	}

	ownerUsername, err := h.aiAnalysisOwnerUsername(ctx.Context(), objectID)
	if err != nil {
		resp, respErr := common.RespondInternalServerError(ctx)
		return false, resp, respErr
	}
	if ownerUsername != "" && strings.EqualFold(ownerUsername, strings.TrimSpace(claims.Username)) {
		return false, nil, nil
	}
	resp, respErr := common.RespondForbidden(ctx, "not authorized to view AI analysis")
	return false, resp, respErr
}

func (h *Handler) claimsUserHasAdminRole(ctx context.Context, claims *auth.Claims) (bool, error) {
	if claims == nil || strings.TrimSpace(claims.Username) == "" {
		return false, nil
	}
	if h == nil || h.repos == nil || h.repos.Account() == nil {
		return false, nil
	}
	user, err := h.repos.Account().GetUser(ctx, claims.Username)
	if err != nil {
		return false, err
	}
	return user != nil && strings.EqualFold(user.Role, roleAdmin), nil
}

func (h *Handler) aiAnalysisOwnerUsername(ctx context.Context, objectID string) (string, error) {
	if h == nil || h.repos == nil || h.repos.Status() == nil {
		return "", nil
	}
	status, err := h.repos.Status().GetStatus(ctx, objectID)
	if err != nil {
		if common.IsNotFound(err) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return "", nil
		}
		return "", err
	}
	if status == nil {
		return "", nil
	}
	if owner := strings.TrimSpace(status.AuthorUsername); owner != "" {
		return owner, nil
	}
	return h.localUsernameForStoredActorCandidate(status.AuthorID), nil
}

// HandleRequestAIAnalysisLift triggers AI analysis for an object
// POST /api/v1/ai/analyze
func (h *Handler) HandleRequestAIAnalysisLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Auth - require moderation scope
	claims, resp, err := h.authenticatedClaimsWithResponder(ctx, common.RespondMissingAuth, common.RespondInvalidToken)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check moderation scope
	if !claims.HasScope("moderation") {
		return common.RespondInsufficientScope(ctx, "moderation")
	}

	if resp, err := h.ensureAIAnalysisEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	req, resp, err := parseAIAnalysisLiftRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
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
		if resp, err := h.existingAIAnalysisResponse(ctx, claims, req.ObjectID); resp != nil || err != nil {
			return resp, err
		}
	}

	response := map[string]any{
		"message":        "Analysis queued",
		"object_id":      req.ObjectID,
		"estimated_time": "10-30 seconds",
	}

	return apptheory.JSON(http.StatusAccepted, response)
}

func (h *Handler) ensureAIAnalysisEnabled(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h.repos != nil && h.repos.Instance() != nil {
		exists, err := h.repos.Instance().AIConfigExists(ctx.Context())
		if err != nil {
			return apptheory.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to resolve AI configuration",
			})
		}
		if exists {
			effectiveAI, err := h.repos.Instance().EffectiveAIConfig(ctx.Context())
			if err != nil {
				return apptheory.JSON(http.StatusInternalServerError, map[string]string{
					"error": "failed to resolve AI configuration",
				})
			}
			if effectiveAI != nil && !effectiveAI.AIEnabled {
				return apptheory.JSON(http.StatusUnprocessableEntity, map[string]string{
					"error": "AI analysis is not enabled on this instance",
				})
			}
		}
	}
	return nil, nil
}

func parseAIAnalysisLiftRequest(ctx *apptheory.Context) (*aiAnalysisLiftRequest, *apptheory.Response, error) {
	var req aiAnalysisLiftRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		// Fallback for test environments / missing content-type
		if len(ctx.Request.Body) == 0 {
			resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return nil, resp, respErr
		}
		if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
			resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return nil, resp, respErr
		}
	}

	if err := common.ValidateRequiredParam("object_id", req.ObjectID); err != nil {
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return nil, resp, respErr
	}

	return &req, nil, nil
}

func (h *Handler) existingAIAnalysisResponse(ctx *apptheory.Context, claims *auth.Claims, objectID string) (*apptheory.Response, error) {
	result, _ := h.registry.AI().GetAnalysis(ctx.Context(), &ai.GetAnalysisQuery{
		ObjectID: objectID,
	})
	if result == nil || result.Analysis == nil {
		return nil, nil
	}

	adminViewer, resp, err := h.authorizeAIAnalysisViewer(ctx, claims, objectID)
	if resp != nil || err != nil {
		return resp, err
	}
	analysis := result.Analysis
	if !adminViewer {
		analysis = originalai.RedactPIIFromAnalysis(analysis)
	}
	return okJSON(analysis)
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
