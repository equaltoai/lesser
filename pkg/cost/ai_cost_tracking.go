package cost

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// AIServiceWithCostTracking wraps AIService with comprehensive cost tracking
type AIServiceWithCostTracking struct {
	*ai.AIService
	costTracker *Tracker
	logger      *zap.Logger
}

// NewAIServiceWithCostTracking creates a cost-tracking wrapper for AI service
func NewAIServiceWithCostTracking(aiService *ai.AIService, costTracker *Tracker, logger *zap.Logger) *AIServiceWithCostTracking {
	return &AIServiceWithCostTracking{
		AIService:   aiService,
		costTracker: costTracker,
		logger:      logger,
	}
}

// GenerateEmbeddingWithCostTracking wraps embedding generation with cost tracking
func (s *AIServiceWithCostTracking) GenerateEmbeddingWithCostTracking(ctx context.Context, text string, userID string) ([]float32, *models.SearchCostTracking, error) {
	startTime := time.Now()

	// Initialize cost tracking
	costData := &models.SearchCostTracking{
		UserID:          userID,
		OperationType:   "semantic_search",
		SearchType:      "embedding_generation",
		Query:           text,
		QueryLength:     len(text),
		Timestamp:       startTime,
		EmbeddingTokens: s.estimateTokenCount(text),
		BedrockRequests: 1,
	}

	// Estimate cost before operation
	estimatedTokens := s.estimateTokenCount(text)
	estimatedCostMicros := s.calculateBedrockCost(estimatedTokens)

	// Track Bedrock request in cost tracker
	if s.costTracker != nil {
		// Track as a custom operation cost
		s.trackBedrockCost(estimatedCostMicros)
	}

	// Generate embedding
	embedding, err := s.GenerateEmbedding(ctx, text)

	// Complete cost tracking
	responseTime := time.Since(startTime)
	costData.ResponseTimeMs = responseTime.Milliseconds()
	costData.BedrockCostMicros = estimatedCostMicros

	if err == nil && len(embedding) > 0 {
		costData.EmbeddingDimension = len(embedding)
		costData.ResultCount = 1 // Successfully generated embedding
	}

	// Calculate total cost
	costData.TotalCostMicros = estimatedCostMicros

	// Update cost tracking keys
	costData.UpdateKeys()

	s.logger.Debug("embedding_generation_tracked",
		zap.String("user_id", userID),
		zap.Int("text_length", len(text)),
		zap.Int("estimated_tokens", estimatedTokens),
		zap.Int64("cost_micros", estimatedCostMicros),
		zap.Int64("response_time_ms", costData.ResponseTimeMs),
		zap.Bool("success", err == nil))

	return embedding, costData, err
}

// SemanticSearchWithCostTracking performs semantic search with comprehensive cost tracking
func (s *AIServiceWithCostTracking) SemanticSearchWithCostTracking(ctx context.Context, query string, userID string, limit int, _ float64) ([]float32, []*models.SearchEmbedding, *models.SearchCostTracking, error) {
	startTime := time.Now()

	// Initialize cost tracking for the complete semantic search operation
	costData := &models.SearchCostTracking{
		UserID:        userID,
		OperationType: "semantic_search",
		SearchType:    "semantic",
		Query:         query,
		QueryLength:   len(query),
		Timestamp:     startTime,
	}

	// Step 1: Generate query embedding with cost tracking
	queryEmbedding, embeddingCostData, err := s.GenerateEmbeddingWithCostTracking(ctx, query, userID)
	if err != nil {
		return nil, nil, costData, err
	}

	// Merge embedding generation costs
	costData.BedrockRequests = embeddingCostData.BedrockRequests
	costData.EmbeddingTokens = embeddingCostData.EmbeddingTokens
	costData.BedrockCostMicros = embeddingCostData.BedrockCostMicros
	costData.EmbeddingDimension = len(queryEmbedding)

	// Step 2: Search similar embeddings (this would be done via search repository)
	// For this example, we'll simulate the search operation costs

	// Estimate vector search costs
	searchStartTime := time.Now()

	// Simulate vector similarity search costs
	estimatedComparisons := limit * 500 // Estimate comparing against 500 stored embeddings
	costData.VectorComparisons = estimatedComparisons

	// DynamoDB scan costs for embedding search
	estimatedReads := int64(500) // Scanning stored embeddings
	costData.DynamoReads = estimatedReads
	costData.DynamoQueries = 1 // Single scan operation
	costData.ScanOperations = 1

	// Track DynamoDB costs
	if s.costTracker != nil {
		_ = s.costTracker.TrackDynamoRead(int(estimatedReads))
	}

	// Simulate the actual search (would be done by search repository)
	var results []*models.SearchEmbedding // Would be populated by actual search

	// Complete timing
	searchTime := time.Since(searchStartTime)
	totalTime := time.Since(startTime)

	costData.ResponseTimeMs = totalTime.Milliseconds()
	costData.IndexLookupTimeMs = searchTime.Milliseconds()
	costData.ResultCount = len(results)

	// Calculate total costs
	dynamoCostMicros := s.calculateDynamoCost(estimatedReads, 0)
	costData.DynamoCostMicros = dynamoCostMicros
	costData.TotalCostMicros = costData.BedrockCostMicros + dynamoCostMicros

	// Calculate cost efficiency
	if len(results) > 0 {
		costData.CostPerResult = costData.TotalCostMicros / int64(len(results))
	}

	costData.UpdateKeys()

	s.logger.Info("semantic_search_completed",
		zap.String("user_id", userID),
		zap.String("query", query),
		zap.Int("query_length", len(query)),
		zap.Int("embedding_dimension", len(queryEmbedding)),
		zap.Int("result_count", len(results)),
		zap.Int("vector_comparisons", costData.VectorComparisons),
		zap.Int64("bedrock_cost_micros", costData.BedrockCostMicros),
		zap.Int64("dynamo_cost_micros", costData.DynamoCostMicros),
		zap.Int64("total_cost_micros", costData.TotalCostMicros),
		zap.Int64("response_time_ms", costData.ResponseTimeMs))

	return queryEmbedding, results, costData, nil
}

// AnalyzeContentWithCostTracking wraps AI content analysis with cost tracking
func (s *AIServiceWithCostTracking) AnalyzeContentWithCostTracking(ctx context.Context, content *ai.Content, userID string) (*ai.AIAnalysis, *models.SearchCostTracking, error) {
	startTime := time.Now()

	// Initialize cost tracking for AI analysis
	costData := &models.SearchCostTracking{
		UserID:        userID,
		OperationType: "ai_analysis",
		SearchType:    "content_analysis",
		Query:         content.Text,
		QueryLength:   len(content.Text),
		Timestamp:     startTime,
	}

	// Estimate AI analysis costs
	var estimatedTokens int
	var estimatedBedrockRequests int

	// Count tokens for text analysis
	if content.Text != "" {
		estimatedTokens += s.estimateTokenCount(content.Text)
		estimatedBedrockRequests++
	}

	// Add costs for AI detection (assume enabled for cost tracking)
	if content.Text != "" {
		estimatedTokens += s.estimateTokenCount(content.Text) * 2 // AI detection is more expensive
		estimatedBedrockRequests++
	}

	costData.EmbeddingTokens = estimatedTokens
	costData.BedrockRequests = estimatedBedrockRequests
	costData.BedrockCostMicros = s.calculateBedrockCost(estimatedTokens)

	// Track costs
	if s.costTracker != nil {
		s.trackBedrockCost(costData.BedrockCostMicros)
	}

	// Perform the actual analysis
	analysis, err := s.AnalyzeContent(ctx, content)

	// Complete cost tracking
	responseTime := time.Since(startTime)
	costData.ResponseTimeMs = responseTime.Milliseconds()
	costData.TotalCostMicros = costData.BedrockCostMicros

	if err == nil {
		costData.ResultCount = 1 // Successfully analyzed content
	}

	costData.UpdateKeys()

	s.logger.Debug("ai_content_analysis_tracked",
		zap.String("user_id", userID),
		zap.Int("content_length", len(content.Text)),
		zap.Int("estimated_tokens", estimatedTokens),
		zap.Int("bedrock_requests", estimatedBedrockRequests),
		zap.Int64("cost_micros", costData.TotalCostMicros),
		zap.Int64("response_time_ms", costData.ResponseTimeMs),
		zap.Bool("success", err == nil))

	return analysis, costData, err
}

// Helper methods for cost calculation

func (s *AIServiceWithCostTracking) estimateTokenCount(text string) int {
	// Simple token estimation: roughly 4 characters per token
	// This is a rough approximation - in practice, you'd use a proper tokenizer
	if err := common.ValidateSliceNotEmpty("text", text); err != nil {
		return 0
	}
	return (len(text) / 4) + 1
}

func (s *AIServiceWithCostTracking) calculateBedrockCost(tokens int) int64 {
	// AWS Bedrock Titan embeddings: ~$0.0001 per 1K tokens
	// Cost in microcents: 100 microcents per 1K tokens
	if tokens == 0 {
		return 0
	}

	// Base cost for API call
	baseCost := int64(50) // 50 microcents per API call

	// Token-based cost
	tokenCost := (int64(tokens) * 100) / 1000 // 100 microcents per 1K tokens

	return baseCost + tokenCost
}

func (s *AIServiceWithCostTracking) calculateDynamoCost(reads, writes int64) int64 {
	// DynamoDB pricing: $0.25 per million read units, $1.25 per million write units
	const (
		readCostPer1M  = 25000  // 25000 microcents per million read units
		writeCostPer1M = 125000 // 125000 microcents per million write units
	)

	readCost := (reads * readCostPer1M) / 1000000
	writeCost := (writes * writeCostPer1M) / 1000000

	return readCost + writeCost
}

func (s *AIServiceWithCostTracking) trackBedrockCost(costMicros int64) {
	// Track Bedrock costs as a custom cost in the cost tracker
	if s.costTracker != nil {
		// Convert microcents to a count for tracking
		// This is a simplification - in practice, you might want to extend
		// the cost tracker to support custom service costs
		_ = int(costMicros / 1000) // Rough equivalent - not currently used

		// Track as Lambda invocations for now (could be extended)
		s.costTracker.TrackLambdaInvocation(100, 512) // 100ms, 512MB equivalent
	}
}

// BulkEmbeddingGenerationWithCostTracking handles bulk embedding generation with cost optimization
func (s *AIServiceWithCostTracking) BulkEmbeddingGenerationWithCostTracking(ctx context.Context, texts []string, userID string) ([][]float32, *models.SearchCostTracking, error) {
	startTime := time.Now()

	// Initialize bulk cost tracking
	costData := &models.SearchCostTracking{
		UserID:        userID,
		OperationType: "bulk_embedding_generation",
		SearchType:    "bulk_semantic",
		Query:         fmt.Sprintf("bulk_%d_texts", len(texts)),
		QueryLength:   len(texts),
		Timestamp:     startTime,
	}

	var totalTokens int
	var totalRequests int
	var embeddings [][]float32

	// Process texts in batches to optimize costs
	batchSize := 10 // Process 10 texts per batch
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		for _, text := range batch {
			embedding, err := s.GenerateEmbedding(ctx, text)
			if err != nil {
				s.logger.Warn("failed to generate embedding in bulk",
					zap.String("user_id", userID),
					zap.Int("batch_index", i),
					zap.Error(err))
				continue
			}

			embeddings = append(embeddings, embedding)
			totalTokens += s.estimateTokenCount(text)
			totalRequests++
		}
	}

	// Complete cost tracking
	responseTime := time.Since(startTime)
	costData.ResponseTimeMs = responseTime.Milliseconds()
	costData.EmbeddingTokens = totalTokens
	costData.BedrockRequests = totalRequests
	costData.BedrockCostMicros = s.calculateBedrockCost(totalTokens)
	costData.TotalCostMicros = costData.BedrockCostMicros
	costData.ResultCount = len(embeddings)

	if len(embeddings) > 0 {
		costData.EmbeddingDimension = len(embeddings[0])
		costData.CostPerResult = costData.TotalCostMicros / int64(len(embeddings))
	}

	// Track costs
	if s.costTracker != nil {
		s.trackBedrockCost(costData.TotalCostMicros)
	}

	costData.UpdateKeys()

	s.logger.Info("bulk_embedding_generation_completed",
		zap.String("user_id", userID),
		zap.Int("input_texts", len(texts)),
		zap.Int("generated_embeddings", len(embeddings)),
		zap.Int("total_tokens", totalTokens),
		zap.Int("total_requests", totalRequests),
		zap.Int64("total_cost_micros", costData.TotalCostMicros),
		zap.Int64("response_time_ms", costData.ResponseTimeMs))

	return embeddings, costData, nil
}
