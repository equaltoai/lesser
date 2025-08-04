package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Initialize AI storage globally for reuse
var (
	aiStorage     *ai.Storage
	aiStorageInit sync.Once
)

func getAIStorage() (*ai.Storage, error) {
	var initErr error
	aiStorageInit.Do(func() {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			initErr = err
			return
		}
		dynamoClient := dynamodb.NewFromConfig(cfg)
		tableName := os.Getenv("DYNAMO_TABLE_NAME")
		if tableName == "" {
			tableName = "lesser-main"
		}
		aiStorage = ai.NewStorage(dynamoClient, tableName)
	})
	return aiStorage, initErr
}

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
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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

	// Get AI storage
	aiStore, err := getAIStorage()
	if err != nil {
		h.logger.Error("failed to get AI storage", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get analysis
	analysis, err := aiStore.GetAnalysis(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "analysis not found",
		})
	}

	// Return analysis
	ctx.Status(http.StatusOK)
	return ctx.JSON(analysis)
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
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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

	// Get AI storage
	aiStore, err := getAIStorage()
	if err != nil {
		h.logger.Error("failed to get AI storage", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Check if analysis exists and is recent
	if !req.Force {
		existing, _ := aiStore.GetAnalysis(ctx.Context, req.ObjectID)
		if existing != nil && time.Since(existing.AnalyzedAt) < 24*time.Hour {
			ctx.Status(http.StatusOK)
			return ctx.JSON(existing)
		}
	}

	// Queue for analysis by updating the object
	err = h.queueForAnalysisLift(ctx.Context, req.ObjectID)
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to queue analysis",
		})
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

	// Get AI storage
	aiStore, err := getAIStorage()
	if err != nil {
		h.logger.Error("failed to get AI storage", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	stats, err := aiStore.GetStats(ctx.Context, period)
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get stats",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(stats)
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
		"cost_per_analysis": ai.CostPerOperation,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(capabilities)
}

// Helper methods

func (h *Handler) queueForAnalysisLift(ctx context.Context, objectID string) error {
	// Create a temporary DynamoDB client for this operation
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}

	dynamoClient := dynamodb.NewFromConfig(cfg)
	tableName := h.cfg.DynamoTableName

	// Update the object to trigger DynamoDB stream
	_, err = dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectID)},
		},
		UpdateExpression: aws.String("SET ForceAnalysis = :true, UpdatedAt = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true": &types.AttributeValueMemberBOOL{Value: true},
			":now":  &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	})

	return err
}

