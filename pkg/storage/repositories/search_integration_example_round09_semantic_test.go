package repositories

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSearchServiceIntegration_Round09_SemanticSearchBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	makeAIService := func(t *testing.T, body string) *ai.AIService {
		t.Helper()

		cfg := aws.Config{
			Region:      "us-east-1",
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "TOKEN")),
			HTTPClient: &http.Client{
				Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(body)),
						Request:    r,
					}, nil
				}),
			},
		}

		return ai.NewAIService(cfg, &ai.Config{})
	}

	setupBudgetOK := func(mockQuery *mocks.MockQuery) {
		mockQuery.
			On("First", mockMatchedByType[*models.SearchBudget]()).
			Run(func(args mock.Arguments) {
				b := args.Get(0).(*models.SearchBudget)
				b.UserID = "user-1"
				b.BudgetLimitMicros = 1_000_000_000
				b.UsedBudgetMicros = 0
				b.SearchBudgetMicros = 1_000_000_000
				b.SemanticBudgetMicros = 1_000_000_000
				b.MaxRequestsPerHour = 1000
				b.MaxSemanticPerHour = 1000
			}).
			Return(nil).
			Maybe()
	}

	t.Run("semantic success populates embedding and cost breakdown", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupBudgetOK(mockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		s := NewSearchServiceIntegration(mockDB, makeAIService(t, `{"embedding":[0.1,0.2,0.3]}`), "req-1", zap.NewNop())

		got, err := s.ComprehensiveSearchExample(ctx, "user-1", "hello", true)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NotEmpty(t, got.QueryEmbedding)
		require.NotNil(t, got.CostBreakdown)
		require.GreaterOrEqual(t, got.CostBreakdown["semantic_search"], int64(0))
	})

	t.Run("semantic error is tolerated", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupBudgetOK(mockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		s := NewSearchServiceIntegration(mockDB, makeAIService(t, `{}`), "req-1", zap.NewNop())

		got, err := s.ComprehensiveSearchExample(ctx, "user-1", "hello", true)
		require.NoError(t, err)
		require.NotNil(t, got)
		// Semantic results may be empty due to error; request still succeeds.
	})
}
