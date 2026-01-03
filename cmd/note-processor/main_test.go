package main

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	comprehendTypes "github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/streamer"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeStreamerClient struct {
	connectionID string
	data         []byte
	err          error
}

func (f *fakeStreamerClient) PostToConnection(_ context.Context, connectionID string, data []byte) error {
	f.connectionID = connectionID
	f.data = append([]byte(nil), data...)
	return f.err
}

func (f *fakeStreamerClient) DeleteConnection(_ context.Context, _ string) error { return nil }

func (f *fakeStreamerClient) GetConnection(_ context.Context, _ string) (*streamer.ConnectionInfo, error) {
	return nil, nil
}

func TestNoteProcessor_PureHelpers(t *testing.T) {
	np := &NoteProcessor{
		logger: zap.NewNop(),
		baseURL: "https://example.com",
	}

	t.Run("getStringAttribute", func(t *testing.T) {
		require.Equal(t, "x", getStringAttribute(events.NewStringAttribute("x")))
		require.Equal(t, "", getStringAttribute(events.NewNumberAttribute("1")))
	})

	t.Run("extractUsernameFromActorID", func(t *testing.T) {
		require.Equal(t, "alice", np.extractUsernameFromActorID("https://example.com/users/alice"))
		require.Equal(t, "bob", np.extractUsernameFromActorID("https://example.com/@bob"))
		require.Equal(t, "", np.extractUsernameFromActorID("https://example.com/actors"))
	})

	t.Run("calculateObjectivity", func(t *testing.T) {
		require.Equal(t, 0.5, np.calculateObjectivity(nil))

		neutral := float32(0.8)
		positive := float32(0.2)
		negative := float32(0.2)
		out := np.calculateObjectivity(&comprehend.DetectSentimentOutput{
			SentimentScore: &comprehendTypes.SentimentScore{
				Neutral:  &neutral,
				Positive: &positive,
				Negative: &negative,
			},
		})
		require.InDelta(t, 0.6, out, 0.0001)

		neutral = 0.0
		positive = 1.0
		negative = 1.0
		out = np.calculateObjectivity(&comprehend.DetectSentimentOutput{
			SentimentScore: &comprehendTypes.SentimentScore{
				Neutral:  &neutral,
				Positive: &positive,
				Negative: &negative,
			},
		})
		require.Equal(t, 0.0, out)
	})

	t.Run("source verification and domain scoring", func(t *testing.T) {
		require.Equal(t, 0.3, np.verifySources(context.Background(), nil))
		require.Equal(t, 0.9, np.evaluateSourceDomain("wikipedia.org"))
		require.Equal(t, 0.8, np.evaluateSourceDomain("example.gov"))
		require.Equal(t, 0.5, np.evaluateSourceDomain("example.com"))
		require.Equal(t, 0.3, np.evaluateSourceDomain("%%%%"))

		score := np.verifySources(context.Background(), []string{"https://wikipedia.org/wiki/X", "not a url"})
		require.InDelta(t, 0.6, score, 0.0001)
	})

	t.Run("initial score calculation", func(t *testing.T) {
		score := np.calculateInitialScoreFromAnalysis(nil, &Analysis{Sentiment: 0.2, Objectivity: 0.8}, 0.9, 800)
		require.InDelta(t, 0.7333333, score, 0.0001)
	})

	t.Run("visibility status", func(t *testing.T) {
		require.Equal(t, visibilityProminent, np.determineVisibilityStatus(0.8))
		require.Equal(t, visibilityVisible, np.determineVisibilityStatus(0.6))
		require.Equal(t, visibilityHidden, np.determineVisibilityStatus(0.4))
		require.Equal(t, visibilityDisputed, np.determineVisibilityStatus(0.0))
	})

	t.Run("action mapping", func(t *testing.T) {
		require.Equal(t, "show", np.determineAction(&storage.CommunityNote{VisibilityStatus: visibilityVisible}))
		require.Equal(t, "show", np.determineAction(&storage.CommunityNote{VisibilityStatus: visibilityProminent}))
		require.Equal(t, "hide", np.determineAction(&storage.CommunityNote{VisibilityStatus: visibilityHidden}))
		require.Equal(t, "dispute", np.determineAction(&storage.CommunityNote{VisibilityStatus: visibilityDisputed}))
		require.Equal(t, "pending", np.determineAction(&storage.CommunityNote{VisibilityStatus: "other"}))
	})

	t.Run("note scoring", func(t *testing.T) {
		note := &storage.CommunityNote{Score: 0.2}
		require.Equal(t, 0.2, np.calculateNoteScore(note, nil))

		require.Equal(t, 0.2, np.calculateNoteScore(note, []*storage.CommunityNoteVote{
			{Helpful: true, Weight: 0},
		}))

		score := np.calculateNoteScore(note, []*storage.CommunityNoteVote{
			{Helpful: true, Weight: 1},
			{Helpful: false, Weight: 1},
		})
		require.InDelta(t, 0.38, score, 0.0001)
	})

	t.Run("complexity analysis and scoring", func(t *testing.T) {
		longContent := "research analysis evidence shocking " + string(make([]byte, 600))
		factors := np.analyzeContentComplexity(longContent, []string{"a", "b", "c", "d"})
		require.Contains(t, factors, "long_content")
		require.Contains(t, factors, "multiple_sources")
		require.Contains(t, factors, "research_references")
		require.Contains(t, factors, "technical_language")
		require.Contains(t, factors, "emotional_language")

		shortFactors := np.analyzeContentComplexity("hi", nil)
		require.Contains(t, shortFactors, "brief_content")
		require.Contains(t, shortFactors, "no_sources")

		score := np.calculateComplexityScore([]string{"long_content", "multiple_sources", "research_references", "technical_language", "emotional_language"}, longContent)
		require.True(t, score <= 1.0 && score >= 0.0)

		score = np.calculateComplexityScore([]string{"no_sources"}, "")
		require.True(t, score <= 1.0 && score >= 0.0)
	})

	t.Run("bedrock analysis falls back when client missing", func(t *testing.T) {
		factors := []string{"multiple_sources", "research_references"}
		score := np.performBedrockReputationAnalysis("content", []string{"https://wikipedia.org/wiki/X"}, factors, "alice")
		require.True(t, score >= 0.0 && score <= 1000.0)
	})

	t.Run("getAuthorMetadata defaults", func(t *testing.T) {
		meta := np.getAuthorMetadata("alice")
		require.Equal(t, 30, meta.AccountAge)
	})

	t.Run("minInt", func(t *testing.T) {
		require.Equal(t, 1, minInt(1, 2))
		require.Equal(t, 2, minInt(3, 2))
	})

	t.Run("domain URL passthrough", func(t *testing.T) {
		require.Equal(t, "https://example.com", np.getDomainURL())
	})

	t.Run("language code mapping", func(t *testing.T) {
		require.Equal(t, comprehendTypes.LanguageCodeEs, np.convertToComprehendLanguageCode("español"))
		require.Equal(t, comprehendTypes.LanguageCodeZhTw, np.convertToComprehendLanguageCode("zh-tw"))
		require.Equal(t, comprehendTypes.LanguageCodeEn, np.convertToComprehendLanguageCode("unknown"))
	})
}

func TestNoteProcessor_GenerateID_UsesFallbackWhenRandFails(t *testing.T) {
	prev := randReadFn
	t.Cleanup(func() { randReadFn = prev })

	np := &NoteProcessor{logger: zap.NewNop()}

	randReadFn = func([]byte) (int, error) { return 0, errors.New("fail") }
	fallback := np.generateID()
	require.NotEmpty(t, fallback)
	_, err := strconv.ParseInt(fallback, 10, 64)
	require.NoError(t, err)

	randReadFn = func(b []byte) (int, error) {
		for i := range b {
			b[i] = byte(i)
		}
		return len(b), nil
	}
	id := np.generateID()
	require.Len(t, id, 32)
}

func TestNoteProcessor_SendWebSocketMessage(t *testing.T) {
	fake := &fakeStreamerClient{}
	np := &NoteProcessor{logger: zap.NewNop(), wsClient: fake}

	require.NoError(t, np.sendWebSocketMessage(context.Background(), "c1", []byte("hi")))
	require.Equal(t, "c1", fake.connectionID)
	require.Equal(t, []byte("hi"), fake.data)

	wantErr := errors.New("post failed")
	fake.err = wantErr
	require.ErrorIs(t, np.sendWebSocketMessage(context.Background(), "c2", []byte("x")), wantErr)
}

func TestNoteProcessor_FallbackReputationAnalysis_Bounds(t *testing.T) {
	np := &NoteProcessor{logger: zap.NewNop()}

	score := np.fallbackReputationAnalysis("ok", []string{"https://wikipedia.org/wiki/X"}, []string{"multiple_sources"})
	require.True(t, !math.IsNaN(score))
	require.True(t, score >= 0.0 && score <= 1000.0)
}
