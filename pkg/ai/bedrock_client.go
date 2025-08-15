package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"go.uber.org/zap"
)

// BedrockClient provides AI analysis using AWS Bedrock
type BedrockClient struct {
	runtime *bedrockruntime.Client
	bedrock *bedrock.Client
	logger  *zap.Logger
	modelID string
}

// ReputationAnalysisRequest represents the input for reputation analysis
type ReputationAnalysisRequest struct {
	Content           string   `json:"content"`
	Sources           []string `json:"sources"`
	ComplexityFactors []string `json:"complexity_factors"`
	AuthorMetadata    struct {
		AccountAge      int     `json:"account_age_days"`
		FollowerCount   int     `json:"follower_count"`
		PostHistory     int     `json:"post_count"`
		EngagementRate  float64 `json:"engagement_rate"`
	} `json:"author_metadata"`
}

// ReputationAnalysisResponse represents the AI analysis result
type ReputationAnalysisResponse struct {
	ReputationScore    float64 `json:"reputation_score"`
	ConfidenceLevel    float64 `json:"confidence_level"`
	QualityIndicators  struct {
		SourceCredibility  float64 `json:"source_credibility"`
		ContentCoherence   float64 `json:"content_coherence"`
		FactualAccuracy    float64 `json:"factual_accuracy"`
		LanguageQuality    float64 `json:"language_quality"`
	} `json:"quality_indicators"`
	RiskFactors        []string `json:"risk_factors"`
	Reasoning          string   `json:"reasoning"`
}

// BedrockInvokeRequest represents the structure for Bedrock API calls
type BedrockInvokeRequest struct {
	Prompt     string  `json:"prompt"`
	MaxTokens  int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TopP       float64 `json:"top_p"`
}

// BedrockInvokeResponse represents the response from Bedrock
type BedrockInvokeResponse struct {
	Completion string `json:"completion"`
	StopReason string `json:"stop_reason"`
}

// NewBedrockClient creates a new AWS Bedrock client for AI analysis
func NewBedrockClient(ctx context.Context, logger *zap.Logger) (*BedrockClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(getAWSRegion()),
		config.WithRetryMaxAttempts(3),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create Bedrock clients
	bedrockRuntime := bedrockruntime.NewFromConfig(cfg)
	bedrockService := bedrock.NewFromConfig(cfg)

	// Determine the model ID to use
	modelID := getBedrockModelID()

	client := &BedrockClient{
		runtime: bedrockRuntime,
		bedrock: bedrockService,
		logger:  logger,
		modelID: modelID,
	}

	// Test connectivity
	if err := client.testConnectivity(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Bedrock: %w", err)
	}

	return client, nil
}

// AnalyzeReputation performs AI-powered reputation analysis using AWS Bedrock
func (c *BedrockClient) AnalyzeReputation(ctx context.Context, req ReputationAnalysisRequest) (*ReputationAnalysisResponse, error) {
	// Create a structured prompt for the AI model
	prompt := c.buildReputationAnalysisPrompt(req)

	// Prepare the Bedrock request
	bedrockReq := BedrockInvokeRequest{
		Prompt:      prompt,
		MaxTokens:   1000,
		Temperature: 0.3, // Lower temperature for more consistent analysis
		TopP:        0.9,
	}

	// Invoke the model
	response, err := c.invokeModel(ctx, bedrockReq)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke Bedrock model: %w", err)
	}

	// Parse the AI response
	analysisResult, err := c.parseReputationResponse(response.Completion)
	if err != nil {
		c.logger.Warn("failed to parse AI response, using fallback analysis",
			zap.Error(err),
			zap.String("raw_response", response.Completion))
		
		// Fall back to a basic analysis if parsing fails
		return c.fallbackAnalysis(req), nil
	}

	return analysisResult, nil
}

// buildReputationAnalysisPrompt creates a structured prompt for reputation analysis
func (c *BedrockClient) buildReputationAnalysisPrompt(req ReputationAnalysisRequest) string {
	prompt := fmt.Sprintf(`You are an expert content analyst for a social media platform. Analyze the following community note and provide a reputation score.

CONTENT TO ANALYZE:
%s

SOURCES PROVIDED:
%s

COMPLEXITY FACTORS:
%s

AUTHOR METADATA:
- Account age: %d days
- Followers: %d
- Total posts: %d
- Engagement rate: %.2f%%

ANALYSIS REQUIREMENTS:
1. Provide a reputation score from 0-1000 (500 = neutral)
2. Assess source credibility, content coherence, factual accuracy, and language quality (0-1.0 each)
3. Identify any risk factors (bias, misinformation, low quality sources, etc.)
4. Provide clear reasoning for your assessment

RESPONSE FORMAT (JSON):
{
  "reputation_score": <number 0-1000>,
  "confidence_level": <number 0-1.0>,
  "quality_indicators": {
    "source_credibility": <number 0-1.0>,
    "content_coherence": <number 0-1.0>,
    "factual_accuracy": <number 0-1.0>,
    "language_quality": <number 0-1.0>
  },
  "risk_factors": [<array of strings>],
  "reasoning": "<detailed explanation>"
}

Analyze now:`,
		req.Content,
		fmt.Sprintf("%v", req.Sources),
		fmt.Sprintf("%v", req.ComplexityFactors),
		req.AuthorMetadata.AccountAge,
		req.AuthorMetadata.FollowerCount,
		req.AuthorMetadata.PostHistory,
		req.AuthorMetadata.EngagementRate,
	)

	return prompt
}

// invokeModel calls the AWS Bedrock model with the given request
func (c *BedrockClient) invokeModel(ctx context.Context, req BedrockInvokeRequest) (*BedrockInvokeResponse, error) {
	// Prepare the payload based on the model type
	var payload []byte
	var err error

	if c.isClaudeModel() {
		payload, err = json.Marshal(map[string]interface{}{
			"prompt":               fmt.Sprintf("\n\nHuman: %s\n\nAssistant:", req.Prompt),
			"max_tokens_to_sample": req.MaxTokens,
			"temperature":          req.Temperature,
			"top_p":                req.TopP,
		})
	} else {
		// For other models (Jurassic, etc.)
		payload, err = json.Marshal(req)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Invoke the model
	result, err := c.runtime.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        payload,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke model: %w", err)
	}

	// Parse the response
	var response BedrockInvokeResponse
	if c.isClaudeModel() {
		var claudeResp struct {
			Completion string `json:"completion"`
			StopReason string `json:"stop_reason"`
		}
		if err := json.Unmarshal(result.Body, &claudeResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Claude response: %w", err)
		}
		response.Completion = claudeResp.Completion
		response.StopReason = claudeResp.StopReason
	} else {
		if err := json.Unmarshal(result.Body, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return &response, nil
}

// parseReputationResponse parses the AI model's response into structured data
func (c *BedrockClient) parseReputationResponse(completion string) (*ReputationAnalysisResponse, error) {
	// Try to extract JSON from the completion
	// The response might have extra text, so look for JSON block
	startIdx := -1
	endIdx := -1
	
	for i := 0; i < len(completion); i++ {
		if completion[i] == '{' && startIdx == -1 {
			startIdx = i
		}
		if completion[i] == '}' {
			endIdx = i + 1
		}
	}

	if startIdx == -1 || endIdx == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}

	jsonStr := completion[startIdx:endIdx]
	
	var result ReputationAnalysisResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Validate the response
	if result.ReputationScore < 0 || result.ReputationScore > 1000 {
		result.ReputationScore = 500 // Default to neutral
	}
	
	if result.ConfidenceLevel < 0 || result.ConfidenceLevel > 1 {
		result.ConfidenceLevel = 0.5 // Default confidence
	}

	return &result, nil
}

// fallbackAnalysis provides a basic analysis when AI parsing fails
func (c *BedrockClient) fallbackAnalysis(req ReputationAnalysisRequest) *ReputationAnalysisResponse {
	// Basic scoring based on available metadata
	score := 500.0 // Start neutral

	// Account age factor
	if req.AuthorMetadata.AccountAge > 365 {
		score += 50 // Established account bonus
	} else if req.AuthorMetadata.AccountAge < 30 {
		score -= 30 // New account penalty
	}

	// Source quality (basic heuristic)
	if len(req.Sources) > 0 {
		score += 25 // Has sources
		for _, source := range req.Sources {
			if len(source) > 20 { // Reasonable URL length
				score += 5
			}
		}
	}

	// Content length and complexity
	if len(req.Content) > 100 {
		score += 15 // Detailed content
	}
	if len(req.ComplexityFactors) > 2 {
		score += 10 // Complex analysis
	}

	// Clamp score
	if score > 1000 {
		score = 1000
	}
	if score < 0 {
		score = 0
	}

	return &ReputationAnalysisResponse{
		ReputationScore: score,
		ConfidenceLevel: 0.6, // Moderate confidence for fallback
		QualityIndicators: struct {
			SourceCredibility  float64 `json:"source_credibility"`
			ContentCoherence   float64 `json:"content_coherence"`
			FactualAccuracy    float64 `json:"factual_accuracy"`
			LanguageQuality    float64 `json:"language_quality"`
		}{
			SourceCredibility: 0.7,
			ContentCoherence:  0.7,
			FactualAccuracy:   0.6,
			LanguageQuality:   0.8,
		},
		RiskFactors: []string{"fallback_analysis_used"},
		Reasoning:   "Analysis performed using fallback heuristics due to AI parsing failure",
	}
}

// testConnectivity tests the connection to AWS Bedrock
func (c *BedrockClient) testConnectivity(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// List foundation models to test connectivity
	_, err := c.bedrock.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return fmt.Errorf("failed to list foundation models: %w", err)
	}

	return nil
}

// isClaudeModel checks if the configured model is a Claude model
func (c *BedrockClient) isClaudeModel() bool {
	return len(c.modelID) > 0 && (c.modelID[:8] == "anthropic" || c.modelID[:6] == "claude")
}

// Helper functions for configuration

func getAWSRegion() string {
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region
	}
	return "us-east-1" // Default region
}

func getBedrockModelID() string {
	if modelID := os.Getenv("BEDROCK_MODEL_ID"); modelID != "" {
		return modelID
	}
	// Default to Claude 3 Haiku for cost-effective analysis
	return "anthropic.claude-3-haiku-20240307-v1:0"
}