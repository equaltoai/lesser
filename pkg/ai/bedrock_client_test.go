package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeBedrockRuntime struct {
	calls     int
	lastInput *bedrockruntime.InvokeModelInput
	fn        func(ctx context.Context, in *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error)
}

func (f *fakeBedrockRuntime) InvokeModel(ctx context.Context, in *bedrockruntime.InvokeModelInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	f.calls++
	f.lastInput = in
	return f.fn(ctx, in)
}

type fakeBedrockService struct {
	calls int
	fn    func(ctx context.Context, in *bedrock.ListFoundationModelsInput) (*bedrock.ListFoundationModelsOutput, error)
}

func (f *fakeBedrockService) ListFoundationModels(ctx context.Context, in *bedrock.ListFoundationModelsInput, _ ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error) {
	f.calls++
	return f.fn(ctx, in)
}

func TestBedrockClient_isClaudeModel(t *testing.T) {
	t.Parallel()

	require.True(t, (&BedrockClient{modelID: "anthropic.claude-3-haiku"}).isClaudeModel())
	require.True(t, (&BedrockClient{modelID: "claude-v2"}).isClaudeModel())
	require.False(t, (&BedrockClient{modelID: "ai21.j2-ultra"}).isClaudeModel())
	require.False(t, (&BedrockClient{modelID: ""}).isClaudeModel())
}

func TestBedrockClient_buildReputationAnalysisPrompt_IncludesInputs(t *testing.T) {
	t.Parallel()

	c := &BedrockClient{logger: zap.NewNop(), modelID: "anthropic.claude-3-haiku"}
	req := ReputationAnalysisRequest{
		Content:           "hello",
		Sources:           []string{"https://example.com"},
		ComplexityFactors: []string{"factor"},
	}
	req.AuthorMetadata.AccountAge = 10
	req.AuthorMetadata.FollowerCount = 2
	req.AuthorMetadata.PostHistory = 3
	req.AuthorMetadata.EngagementRate = 1.23

	prompt := c.buildReputationAnalysisPrompt(req)
	require.Contains(t, prompt, "hello")
	require.Contains(t, prompt, "example.com")
	require.Contains(t, prompt, "factor")
	require.Contains(t, prompt, "Account age")
}

func TestBedrockClient_parseReputationResponse_ExtractsJSONAndValidatesRanges(t *testing.T) {
	t.Parallel()

	c := &BedrockClient{logger: zap.NewNop(), modelID: "anthropic.claude-3-haiku"}

	out, err := c.parseReputationResponse(`prefix {"reputation_score":2000,"confidence_level":2} suffix`)
	require.NoError(t, err)
	require.Equal(t, 500.0, out.ReputationScore)
	require.Equal(t, 0.5, out.ConfidenceLevel)

	_, err = c.parseReputationResponse("no json here")
	require.ErrorIs(t, err, ErrNoJSONFoundInResponse)
}

func TestBedrockClient_AnalyzeReputation_ParsesOrFallsBack(t *testing.T) {
	t.Parallel()

	t.Run("parsed result", func(t *testing.T) {
		runtime := &fakeBedrockRuntime{
			fn: func(_ context.Context, in *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
				require.Equal(t, "application/json", aws.ToString(in.ContentType))
				require.Equal(t, "application/json", aws.ToString(in.Accept))
				require.Equal(t, "anthropic.claude-3-haiku", aws.ToString(in.ModelId))

				var payload map[string]any
				require.NoError(t, json.Unmarshal(in.Body, &payload))
				require.Contains(t, payload["prompt"].(string), "Human:")
				require.Contains(t, payload["prompt"].(string), "Assistant:")
				require.Equal(t, float64(0.3), payload["temperature"])

				resp := map[string]any{
					"completion":  `{"reputation_score":600,"confidence_level":0.8}`,
					"stop_reason": "end",
				}
				body, _ := json.Marshal(resp)
				return &bedrockruntime.InvokeModelOutput{Body: body}, nil
			},
		}

		client := &BedrockClient{
			runtime: runtime,
			bedrock: &fakeBedrockService{fn: func(_ context.Context, _ *bedrock.ListFoundationModelsInput) (*bedrock.ListFoundationModelsOutput, error) {
				return &bedrock.ListFoundationModelsOutput{}, nil
			}},
			logger:  zap.NewNop(),
			modelID: "anthropic.claude-3-haiku",
		}

		resp, err := client.AnalyzeReputation(context.Background(), ReputationAnalysisRequest{Content: "hello"})
		require.NoError(t, err)
		require.Equal(t, 600.0, resp.ReputationScore)
		require.Equal(t, 0.8, resp.ConfidenceLevel)
		require.Equal(t, 1, runtime.calls)
	})

	t.Run("fallback when response isn't parseable", func(t *testing.T) {
		runtime := &fakeBedrockRuntime{
			fn: func(_ context.Context, _ *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
				resp := map[string]any{
					"completion":  "no json",
					"stop_reason": "end",
				}
				body, _ := json.Marshal(resp)
				return &bedrockruntime.InvokeModelOutput{Body: body}, nil
			},
		}

		client := &BedrockClient{
			runtime: runtime,
			bedrock: &fakeBedrockService{fn: func(_ context.Context, _ *bedrock.ListFoundationModelsInput) (*bedrock.ListFoundationModelsOutput, error) {
				return &bedrock.ListFoundationModelsOutput{}, nil
			}},
			logger:  zap.NewNop(),
			modelID: "anthropic.claude-3-haiku",
		}

		req := ReputationAnalysisRequest{Content: "hello"}
		req.AuthorMetadata.AccountAge = 400
		resp, err := client.AnalyzeReputation(context.Background(), req)
		require.NoError(t, err)
		require.Contains(t, resp.RiskFactors, "fallback_analysis_used")
		require.GreaterOrEqual(t, resp.ReputationScore, 0.0)
		require.LessOrEqual(t, resp.ReputationScore, 1000.0)
	})
}

func TestBedrockClient_invokeModel_NonClaudeModel(t *testing.T) {
	t.Parallel()

	runtime := &fakeBedrockRuntime{
		fn: func(_ context.Context, in *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
			var payload BedrockInvokeRequest
			require.NoError(t, json.Unmarshal(in.Body, &payload))
			require.Contains(t, payload.Prompt, "CONTENT TO ANALYZE")

			body, _ := json.Marshal(BedrockInvokeResponse{Completion: `{"reputation_score":500,"confidence_level":0.5}`, StopReason: "end"})
			return &bedrockruntime.InvokeModelOutput{Body: body}, nil
		},
	}

	client := &BedrockClient{
		runtime: runtime,
		bedrock: &fakeBedrockService{fn: func(_ context.Context, _ *bedrock.ListFoundationModelsInput) (*bedrock.ListFoundationModelsOutput, error) {
			return &bedrock.ListFoundationModelsOutput{}, nil
		}},
		logger:  zap.NewNop(),
		modelID: "ai21.j2-ultra",
	}

	resp, err := client.AnalyzeReputation(context.Background(), ReputationAnalysisRequest{Content: "hello"})
	require.NoError(t, err)
	require.Equal(t, 500.0, resp.ReputationScore)
}

func TestBedrockClient_invokeModel_PropagatesRuntimeErrors(t *testing.T) {
	t.Parallel()

	client := &BedrockClient{
		runtime: &fakeBedrockRuntime{
			fn: func(_ context.Context, _ *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
				return nil, errors.New("boom")
			},
		},
		bedrock: &fakeBedrockService{fn: func(_ context.Context, _ *bedrock.ListFoundationModelsInput) (*bedrock.ListFoundationModelsOutput, error) {
			return &bedrock.ListFoundationModelsOutput{}, nil
		}},
		logger:  zap.NewNop(),
		modelID: "ai21.j2-ultra",
	}

	_, err := client.invokeModel(context.Background(), BedrockInvokeRequest{Prompt: "p"})
	require.Error(t, err)
}

func TestBedrockClient_testConnectivity_SurfacesErrors(t *testing.T) {
	t.Parallel()

	client := &BedrockClient{
		runtime: &fakeBedrockRuntime{fn: func(_ context.Context, _ *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
			return nil, nil
		}},
		bedrock: &fakeBedrockService{fn: func(_ context.Context, _ *bedrock.ListFoundationModelsInput) (*bedrock.ListFoundationModelsOutput, error) {
			return nil, errors.New("boom")
		}},
		logger:  zap.NewNop(),
		modelID: "anthropic.claude-3-haiku",
	}

	require.Error(t, client.testConnectivity(context.Background()))

	client.bedrock = &fakeBedrockService{fn: func(_ context.Context, _ *bedrock.ListFoundationModelsInput) (*bedrock.ListFoundationModelsOutput, error) {
		return &bedrock.ListFoundationModelsOutput{}, nil
	}}
	require.NoError(t, client.testConnectivity(context.Background()))
}

func TestBedrockClient_fallbackAnalysis_ScoringHeuristics(t *testing.T) {
	t.Parallel()

	client := &BedrockClient{logger: zap.NewNop()}
	req := ReputationAnalysisRequest{
		Content:           strings.Repeat("x", 200),
		Sources:           []string{"https://example.com/source"},
		ComplexityFactors: []string{"a", "b", "c"},
	}
	req.AuthorMetadata.AccountAge = 10
	resp := client.fallbackAnalysis(req)
	require.Contains(t, resp.RiskFactors, "fallback_analysis_used")
	require.GreaterOrEqual(t, resp.ReputationScore, 0.0)
	require.LessOrEqual(t, resp.ReputationScore, 1000.0)
}

