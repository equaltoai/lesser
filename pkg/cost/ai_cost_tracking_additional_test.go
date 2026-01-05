package cost

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeAIService struct {
	embedding    []float32
	embedErr     error
	analyze      *ai.AIAnalysis
	analyzeErr   error
	embedCalls   int
	analyzeCalls int
}

func (f *fakeAIService) GenerateEmbedding(_ context.Context, _ string) ([]float32, error) {
	f.embedCalls++
	if f.embedErr != nil {
		return nil, f.embedErr
	}
	return f.embedding, nil
}

func (f *fakeAIService) AnalyzeContent(_ context.Context, _ *ai.Content) (*ai.AIAnalysis, error) {
	f.analyzeCalls++
	if f.analyzeErr != nil {
		return nil, f.analyzeErr
	}
	return f.analyze, nil
}

func TestAIServiceWithCostTracking_GenerateEmbeddingWithCostTracking(t *testing.T) {
	t.Parallel()

	tracker := New()
	tracker.circuitBreaker = nil

	aiSvc := &fakeAIService{embedding: []float32{1, 2, 3}}
	service := NewAIServiceWithCostTracking(aiSvc, tracker, zap.NewNop())

	embedding, costData, err := service.GenerateEmbeddingWithCostTracking(context.Background(), "hello world", "user-1")
	require.NoError(t, err)
	require.Equal(t, []float32{1, 2, 3}, embedding)
	require.Equal(t, "user-1", costData.UserID)
	require.Equal(t, "semantic_search", costData.OperationType)
	require.Equal(t, 1, costData.BedrockRequests)
	require.Equal(t, 3, costData.EmbeddingDimension)
	require.Equal(t, 1, costData.ResultCount)
	require.NotEmpty(t, costData.PK)
	require.NotEmpty(t, costData.SK)
}

func TestAIServiceWithCostTracking_GenerateEmbeddingWithCostTracking_NilAIService(t *testing.T) {
	t.Parallel()

	service := NewAIServiceWithCostTracking(nil, nil, zap.NewNop())
	_, costData, err := service.GenerateEmbeddingWithCostTracking(context.Background(), "hello", "user-1")
	require.Error(t, err)
	require.NotNil(t, costData)
}

func TestAIServiceWithCostTracking_SemanticSearchWithCostTracking(t *testing.T) {
	t.Parallel()

	tracker := New()
	tracker.circuitBreaker = nil

	aiSvc := &fakeAIService{embedding: []float32{1, 2}}
	service := NewAIServiceWithCostTracking(aiSvc, tracker, zap.NewNop())

	embedding, results, costData, err := service.SemanticSearchWithCostTracking(context.Background(), "q", "user-1", 10, 0.7)
	require.NoError(t, err)
	require.Equal(t, []float32{1, 2}, embedding)
	require.Nil(t, results)
	require.Equal(t, 10*500, costData.VectorComparisons)
	require.Equal(t, int64(500), costData.DynamoReads)
	require.Equal(t, 1, costData.DynamoQueries)
	require.Equal(t, 1, costData.ScanOperations)
	require.Greater(t, costData.TotalCostMicros, int64(0))
}

func TestAIServiceWithCostTracking_AnalyzeContentWithCostTracking(t *testing.T) {
	t.Parallel()

	tracker := New()
	tracker.circuitBreaker = nil

	aiSvc := &fakeAIService{analyze: &ai.AIAnalysis{ID: "id"}}
	service := NewAIServiceWithCostTracking(aiSvc, tracker, zap.NewNop())

	analysis, costData, err := service.AnalyzeContentWithCostTracking(context.Background(), &ai.Content{Text: "hello"}, "user-1")
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.Equal(t, "ai_analysis", costData.OperationType)
	require.Equal(t, 2, costData.BedrockRequests)
	require.Equal(t, 1, costData.ResultCount)
}

func TestAIServiceWithCostTracking_BulkEmbeddingGenerationWithCostTracking(t *testing.T) {
	tracker := New()
	tracker.circuitBreaker = nil

	aiSvc := &fakeAIService{embedding: []float32{1, 2, 3}}
	service := NewAIServiceWithCostTracking(aiSvc, tracker, zap.NewNop())

	texts := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}
	embeddings, costData, err := service.BulkEmbeddingGenerationWithCostTracking(context.Background(), texts, "user-1")
	require.NoError(t, err)
	require.Len(t, embeddings, len(texts))
	require.Equal(t, 3, costData.EmbeddingDimension)
	require.Equal(t, len(texts), costData.ResultCount)

	aiSvc.embedErr = errors.New("boom")
	embeddings, costData, err = service.BulkEmbeddingGenerationWithCostTracking(context.Background(), texts, "user-1")
	require.NoError(t, err)
	require.Len(t, embeddings, 0)
	require.Equal(t, 0, costData.ResultCount)
}
