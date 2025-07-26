package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

// HandleGetAIAnalysis returns AI analysis for an object
// GET /api/v1/ai/analysis/:object_id
func (h *Handler) HandleGetAIAnalysis(ctx context.Context, request events.APIGatewayV2HTTPRequest, objectID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Auth - require read scope
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	_, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	if objectID == "" {
		return common.BadRequest(errors.New("object_id is required")), nil
	}

	// Get AI storage
	aiStore, err := getAIStorage()
	if err != nil {
		h.logger.Error("failed to get AI storage", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get analysis
	analysis, err := aiStore.GetAnalysis(ctx, objectID)
	if err != nil {
		return common.NotFound(errors.New("analysis not found")), nil
	}

	// Return analysis
	body, _ := json.Marshal(analysis)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleRequestAIAnalysis triggers AI analysis for an object
// POST /api/v1/ai/analyze
func (h *Handler) HandleRequestAIAnalysis(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Auth - require moderation scope
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check moderation scope
	if !claims.HasScope("moderation") {
		return common.Forbidden(errors.New("moderation scope required")), nil
	}

	var req struct {
		ObjectID   string `json:"object_id"`
		ObjectType string `json:"object_type"`
		Force      bool   `json:"force"` // Force re-analysis
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	if req.ObjectID == "" {
		return common.BadRequest(errors.New("object_id is required")), nil
	}

	// Get AI storage
	aiStore, err := getAIStorage()
	if err != nil {
		h.logger.Error("failed to get AI storage", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Check if analysis exists and is recent
	if !req.Force {
		existing, _ := aiStore.GetAnalysis(ctx, req.ObjectID)
		if existing != nil && time.Since(existing.AnalyzedAt) < 24*time.Hour {
			body, _ := json.Marshal(existing)
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: 200,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: string(body),
			}, nil
		}
	}

	// Queue for analysis by updating the object
	err = h.queueForAnalysis(ctx, req.ObjectID)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	response := map[string]any{
		"message":        "Analysis queued",
		"object_id":      req.ObjectID,
		"estimated_time": "10-30 seconds",
	}

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 202,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetAIStats returns AI analysis statistics
// GET /api/v1/ai/stats
func (h *Handler) HandleGetAIStats(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Public endpoint - no auth required

	// Get time range
	period := request.QueryStringParameters["period"]
	if period == "" {
		period = "day"
	}

	// Get AI storage
	aiStore, err := getAIStorage()
	if err != nil {
		h.logger.Error("failed to get AI storage", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	stats, err := aiStore.GetStats(ctx, period)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	body, _ := json.Marshal(stats)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetAISummary returns a summary of AI features and capabilities
// GET /api/v1/ai/capabilities
func (h *Handler) HandleGetAISummary(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	body, _ := json.Marshal(capabilities)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// Helper methods

func (h *Handler) queueForAnalysis(ctx context.Context, objectID string) error {
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
