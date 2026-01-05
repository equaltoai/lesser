package advanced

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeComprehendClient struct {
	mu sync.Mutex

	dominantLangOut *comprehend.DetectDominantLanguageOutput
	dominantLangErr error

	sentimentOut *comprehend.DetectSentimentOutput
	sentimentErr error

	piiOut *comprehend.DetectPiiEntitiesOutput
	piiErr error

	entitiesOut *comprehend.DetectEntitiesOutput
	entitiesErr error

	keyPhrasesOut *comprehend.DetectKeyPhrasesOutput
	keyPhrasesErr error

	calls map[string]int
}

func (c *fakeComprehendClient) bump(op string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calls == nil {
		c.calls = make(map[string]int)
	}
	c.calls[op]++
}

func (c *fakeComprehendClient) DetectDominantLanguage(context.Context, *comprehend.DetectDominantLanguageInput, ...func(*comprehend.Options)) (*comprehend.DetectDominantLanguageOutput, error) {
	c.bump("DetectDominantLanguage")
	return c.dominantLangOut, c.dominantLangErr
}

func (c *fakeComprehendClient) DetectSentiment(context.Context, *comprehend.DetectSentimentInput, ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
	c.bump("DetectSentiment")
	return c.sentimentOut, c.sentimentErr
}

func (c *fakeComprehendClient) DetectPiiEntities(context.Context, *comprehend.DetectPiiEntitiesInput, ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
	c.bump("DetectPiiEntities")
	return c.piiOut, c.piiErr
}

func (c *fakeComprehendClient) DetectEntities(context.Context, *comprehend.DetectEntitiesInput, ...func(*comprehend.Options)) (*comprehend.DetectEntitiesOutput, error) {
	c.bump("DetectEntities")
	return c.entitiesOut, c.entitiesErr
}

func (c *fakeComprehendClient) DetectKeyPhrases(context.Context, *comprehend.DetectKeyPhrasesInput, ...func(*comprehend.Options)) (*comprehend.DetectKeyPhrasesOutput, error) {
	c.bump("DetectKeyPhrases")
	return c.keyPhrasesOut, c.keyPhrasesErr
}

type textAnalyzerCostTracker struct {
	mu sync.Mutex

	comprehendOps []string
	units         []int
}

func (f *textAnalyzerCostTracker) TrackComprehendRequest(operation string, units int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comprehendOps = append(f.comprehendOps, operation)
	f.units = append(f.units, units)
}

func (f *textAnalyzerCostTracker) TrackTranscribeRequest(string, int) {}

func TestTextAnalyzer_AnalyzeText_EarlyReturnOnEmptyText(t *testing.T) {
	ta := &TextAnalyzer{
		logger: zap.NewNop(),
		config: &ModerationConfig{EnableTextAnalysis: true},
	}

	out, err := ta.AnalyzeText(context.Background(), "", ContentMetadata{ContentID: "id-1"})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "id-1", out.ContentID)
}

func TestTextAnalyzer_AnalyzeText_FullPipelineAndBranches(t *testing.T) {
	now := time.Now()
	privateText := "bob die - please send my address and phone. kill attack bomb shoot stab murder suicide."

	langEN := "en"
	langES := "es"
	scoreEN := float32(0.6)
	scoreES := float32(0.9)

	positive := float32(0.05)
	negative := float32(0.92)
	neutral := float32(0.02)
	mixed := float32(0.01)

	begin := int32(14)
	end := int32(21)
	outOfRangeBegin := int32(100)
	outOfRangeEnd := int32(200)
	piiScore := float32(0.99)

	entityScore := float32(0.9)
	keyPhraseScore := float32(0.9)

	client := &fakeComprehendClient{
		dominantLangOut: &comprehend.DetectDominantLanguageOutput{
			Languages: []types.DominantLanguage{
				{LanguageCode: &langEN, Score: &scoreEN},
				{LanguageCode: &langES, Score: &scoreES},
			},
		},
		sentimentOut: &comprehend.DetectSentimentOutput{
			Sentiment: types.SentimentTypeNegative,
			SentimentScore: &types.SentimentScore{
				Positive: &positive,
				Negative: &negative,
				Neutral:  &neutral,
				Mixed:    &mixed,
			},
		},
		keyPhrasesOut: &comprehend.DetectKeyPhrasesOutput{
			KeyPhrases: []types.KeyPhrase{
				{Text: aws.String("You are a bitch"), Score: &keyPhraseScore},
				{Text: aws.String("I hate you"), Score: &keyPhraseScore},
			},
		},
		entitiesOut: &comprehend.DetectEntitiesOutput{
			Entities: []types.Entity{
				{Type: types.EntityTypePerson, Text: aws.String("bob"), Score: &entityScore},
				{Type: types.EntityTypePerson, Text: aws.String("bob"), Score: &entityScore},
			},
		},
		piiOut: &comprehend.DetectPiiEntitiesOutput{
			Entities: []types.PiiEntity{
				{Type: types.PiiEntityTypePhone, BeginOffset: &begin, EndOffset: &end, Score: &piiScore},
				{Type: types.PiiEntityTypeName, BeginOffset: &outOfRangeBegin, EndOffset: &outOfRangeEnd, Score: &piiScore},
			},
		},
	}

	costTracker := &textAnalyzerCostTracker{}

	ta := &TextAnalyzer{
		client:      client,
		logger:      zap.NewNop(),
		config:      &ModerationConfig{EnableTextAnalysis: true},
		costTracker: costTracker,
		cacheTTL:    time.Minute,
	}

	meta := ContentMetadata{
		ContentID: "THIS IS ALL CAPS!!!!!!!!",
		Context:   "comment",
		URLs:      []string{"https://a", "https://b", "https://c", "https://d"},
		Timestamp: now,
	}

	out, err := ta.AnalyzeText(context.Background(), privateText, meta)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.Equal(t, meta.ContentID, out.ContentID)
	require.NotEmpty(t, out.Language.LanguageCode)
	require.NotEmpty(t, out.Sentiment.Sentiment)
	require.True(t, out.Toxicity.IsToxic)
	require.NotEmpty(t, out.Toxicity.Categories)
	require.NotEmpty(t, out.Toxicity.TargetedGroups)
	require.NotEmpty(t, out.PII)
	require.NotEmpty(t, out.Topics)
	require.NotEmpty(t, out.Threats)
	require.NotEmpty(t, out.CustomFlags)

	costTracker.mu.Lock()
	defer costTracker.mu.Unlock()
	require.NotEmpty(t, costTracker.comprehendOps)
}

func TestTextAnalyzer_DetectLanguage_CacheAndFallbacks(t *testing.T) {
	lang := "en"
	score := float32(0.8)

	client := &fakeComprehendClient{
		dominantLangOut: &comprehend.DetectDominantLanguageOutput{
			Languages: []types.DominantLanguage{
				{LanguageCode: &lang, Score: &score},
			},
		},
	}

	ta := &TextAnalyzer{
		client:   client,
		logger:   zap.NewNop(),
		config:   &ModerationConfig{EnableTextAnalysis: true},
		cacheTTL: time.Hour,
	}

	ctx := context.Background()
	text := "hello world"

	first, err := ta.detectLanguage(ctx, text)
	require.NoError(t, err)
	require.Equal(t, "en", first)

	second, err := ta.detectLanguage(ctx, text)
	require.NoError(t, err)
	require.Equal(t, "en", second)

	client.mu.Lock()
	calls := client.calls["DetectDominantLanguage"]
	client.mu.Unlock()
	require.Equal(t, 1, calls, "expected cache hit on second call")

	// Empty language list defaults to English.
	client2 := &fakeComprehendClient{dominantLangOut: &comprehend.DetectDominantLanguageOutput{Languages: nil}}
	ta2 := &TextAnalyzer{client: client2, logger: zap.NewNop(), config: &ModerationConfig{EnableTextAnalysis: true}, cacheTTL: time.Minute}
	lang2, err := ta2.detectLanguage(ctx, text)
	require.NoError(t, err)
	require.Equal(t, "en", lang2)

	// Error surfaces to caller.
	client3 := &fakeComprehendClient{dominantLangErr: errors.New("boom")}
	ta3 := &TextAnalyzer{client: client3, logger: zap.NewNop(), config: &ModerationConfig{EnableTextAnalysis: true}, cacheTTL: time.Minute}
	_, err = ta3.detectLanguage(ctx, text)
	require.Error(t, err)
}

func TestTextAnalyzer_HelperFunctions_Coverage(t *testing.T) {
	require.NotEmpty(t, hashText("anything"))
	require.False(t, isAllCaps("short"))
	require.True(t, isAllCaps("THIS IS MOSTLY CAPS"))
	require.Equal(t, 6, countExcessivePunctuation("!!!!!!"))
	require.True(t, containsPersonalInfo("my address is ..."))
	require.False(t, containsPersonalInfo("nothing to see here"))
}

func TestTextAnalyzer_NewTextAnalyzer_Defaults(t *testing.T) {
	ta := NewTextAnalyzer(nil, nil, &ModerationConfig{}, nil)
	require.NotNil(t, ta)
	require.NotNil(t, ta.logger)
}

func TestTextAnalyzer_AnalyzeText_TruncationAndLanguageBehavior(t *testing.T) {
	ctx := context.Background()

	lang := "en"
	score := float32(0.9)
	positive := float32(0.1)
	negative := float32(0.2)
	neutral := float32(0.7)
	mixed := float32(0.0)

	client := &fakeComprehendClient{
		dominantLangOut: &comprehend.DetectDominantLanguageOutput{
			Languages: []types.DominantLanguage{{LanguageCode: &lang, Score: &score}},
		},
		sentimentOut: &comprehend.DetectSentimentOutput{
			Sentiment: types.SentimentTypeNeutral,
			SentimentScore: &types.SentimentScore{
				Positive: &positive,
				Negative: &negative,
				Neutral:  &neutral,
				Mixed:    &mixed,
			},
		},
		keyPhrasesOut: &comprehend.DetectKeyPhrasesOutput{KeyPhrases: nil},
		entitiesOut:   &comprehend.DetectEntitiesOutput{Entities: nil},
	}

	tracker := &textAnalyzerCostTracker{}

	ta := &TextAnalyzer{
		client:      client,
		logger:      zap.NewNop(),
		config:      &ModerationConfig{EnableTextAnalysis: false},
		costTracker: tracker,
		cacheTTL:    time.Minute,
	}

	// Long text triggers truncation to 5000 chars and should *not* run language detection when provided.
	longText := strings.Repeat("a", 6000)
	meta := ContentMetadata{
		ContentID: "THIS IS ALL CAPS!!!!!!!!",
		Language:  "en",
		URLs:      []string{},
		Timestamp: time.Now(),
	}

	out, err := ta.AnalyzeText(ctx, longText, meta)
	require.NoError(t, err)
	require.NotNil(t, out)

	client.mu.Lock()
	detectLangCalls := client.calls["DetectDominantLanguage"]
	client.mu.Unlock()
	require.Equal(t, 0, detectLangCalls)

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.NotEmpty(t, tracker.units)
	for _, units := range tracker.units {
		require.LessOrEqual(t, units, 5000)
	}
}

func TestTextAnalyzer_AnalyzeText_LanguageDetectionFailureAndNonCriticalErrors(t *testing.T) {
	ctx := context.Background()

	client := &fakeComprehendClient{
		dominantLangErr: errors.New("no lang"),
		sentimentErr:    errors.New("no sentiment"),
		piiErr:          errors.New("no pii"),
		entitiesErr:     errors.New("no entities"),
		keyPhrasesErr:   errors.New("no phrases"),
	}

	ta := &TextAnalyzer{
		client:   client,
		logger:   zap.NewNop(),
		config:   &ModerationConfig{EnableTextAnalysis: true},
		cacheTTL: time.Minute,
	}

	out, err := ta.AnalyzeText(ctx, "some text", ContentMetadata{ContentID: "id-1", Timestamp: time.Now()})
	require.NoError(t, err)
	require.NotNil(t, out)

	// Language detection failed, so AnalyzeText defaults to English without populating LanguageDetection struct.
	require.Empty(t, out.Language.LanguageCode)
}
