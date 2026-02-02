package moderation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	comprehendTypes "github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeModerationStorage struct {
	mu sync.Mutex

	decisions []*ModerationResult
	queue     []*ModerationQueueItem

	updateCalls int
	updateErr   error
	storeErr    error
}

func (f *fakeModerationStorage) StoreModerationDecision(_ context.Context, decision *ModerationResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.storeErr != nil {
		return f.storeErr
	}
	f.decisions = append(f.decisions, decision)
	return nil
}

func (f *fakeModerationStorage) UpdateModerationDecision(_ context.Context, _ string, _ *ModerationReview) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls++
	return f.updateErr
}

func (f *fakeModerationStorage) GetModerationHistory(_ context.Context, _ string) ([]*ModerationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*ModerationResult(nil), f.decisions...), nil
}

func (f *fakeModerationStorage) GetModerationQueue(_ context.Context, _ *ModerationFilter) ([]*ModerationQueueItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*ModerationQueueItem(nil), f.queue...), nil
}

func TestGenerateTextHash_Deterministic(t *testing.T) {
	h1 := generateTextHash("hello world")
	h2 := generateTextHash("hello world")
	h3 := generateTextHash("different")

	assert.Equal(t, h1, h2)
	assert.NotEqual(t, h1, h3)
	assert.Len(t, h1, 64)
}

func TestNewModerator_WiresDependencies(t *testing.T) {
	store := &fakeModerationStorage{}

	m := NewModerator(store, nil)
	require.NotNil(t, m)
	assert.NotNil(t, m.patternManager)
	assert.Equal(t, store, m.storage)
	assert.Nil(t, m.aiAnalyzer)
}

func TestModerator_EvaluateAndCombineSignals(t *testing.T) {
	m := &Moderator{}

	score, action := m.evaluatePatternMatches([]*PatternMatch{
		{Severity: "low", Confidence: 1.0, Action: actionFlag},
		{Severity: "critical", Confidence: 0.5, Action: actionBlock},
	})
	assert.Equal(t, actionBlock, action)
	assert.InDelta(t, 50.0, score, 0.0001)

	assert.Equal(t, actionAllow, m.determineHighestSeverityAction(nil))
	assert.Equal(t, actionBlock, m.determineHighestSeverityAction([]string{actionFlag, actionBlock}))

	confidence := m.calculateConfidence([]string{actionFlag}, []float64{10})
	assert.InDelta(t, 0.7, confidence, 0.0001)

	confidence = m.calculateConfidence([]string{actionFlag, actionFlag}, []float64{10, 11})
	assert.InDelta(t, 1.0, confidence, 0.0001)

	confidence = m.calculateConfidence([]string{actionFlag, actionBlock}, []float64{0, 100})
	assert.InDelta(t, 0.5, confidence, 0.0001)
}

func TestModerator_moderateWithAI_CollectsTextAndImage(t *testing.T) {
	pos, neg, neu, mix := float32(0.1), float32(0.9), float32(0.0), float32(0.0)
	sentimentScore := &comprehendTypes.SentimentScore{Positive: &pos, Negative: &neg, Neutral: &neu, Mixed: &mix}

	ai := &AIAnalyzer{
		comprehend: &fakeComprehendClient{
			sentimentOut: &comprehend.DetectSentimentOutput{
				Sentiment:      comprehendTypes.SentimentTypeNegative,
				SentimentScore: sentimentScore,
			},
		},
		rekognition: &fakeRekognitionClient{},
	}

	m := &Moderator{aiAnalyzer: ai}
	result, err := m.moderateWithAI(context.Background(), &ContentSubmission{
		ID:         "c1",
		Type:       "post",
		Text:       "hello",
		ImageURL:   "https://example.com/img",
		ImageBytes: []byte("x"),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.TextAnalysis)
	assert.NotNil(t, result.ImageAnalysis)
}

func TestModerator_evaluateAIAnalysis_Thresholds(t *testing.T) {
	m := &Moderator{}

	for _, tt := range []struct {
		name       string
		analysis   *AIAnalysisResult
		wantScore  float64
		wantAction string
	}{
		{
			name:       "block",
			analysis:   &AIAnalysisResult{TextAnalysis: &TextAnalysis{ModerationScore: 80}},
			wantScore:  80,
			wantAction: actionBlock,
		},
		{
			name:       "escalate",
			analysis:   &AIAnalysisResult{TextAnalysis: &TextAnalysis{ModerationScore: 60}},
			wantScore:  60,
			wantAction: actionEscalate,
		},
		{
			name:       "flag",
			analysis:   &AIAnalysisResult{TextAnalysis: &TextAnalysis{ModerationScore: 40}},
			wantScore:  40,
			wantAction: actionFlag,
		},
		{
			name:       "allow",
			analysis:   &AIAnalysisResult{TextAnalysis: &TextAnalysis{ModerationScore: 10}},
			wantScore:  10,
			wantAction: actionAllow,
		},
		{
			name:       "maxScoreFromImage",
			analysis:   &AIAnalysisResult{TextAnalysis: &TextAnalysis{ModerationScore: 10}, ImageAnalysis: &ImageAnalysis{ModerationScore: 70}},
			wantScore:  70,
			wantAction: actionEscalate,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			score, action := m.evaluateAIAnalysis(tt.analysis)
			assert.InDelta(t, tt.wantScore, score, 0.0001)
			assert.Equal(t, tt.wantAction, action)
		})
	}
}

func TestModerator_generateRecommendations_IncludesAIDetailsAndLowConfidence(t *testing.T) {
	m := &Moderator{}

	result := &ModerationResult{
		Action:     actionBlock,
		Confidence: 0.5,
		AIAnalysis: &AIAnalysisResult{
			TextAnalysis: &TextAnalysis{Recommendations: []string{"ai rec 1"}},
			ImageAnalysis: &ImageAnalysis{
				Recommendations: []string{"ai rec 2"},
			},
		},
	}

	recs := m.generateRecommendations(result)
	assert.NotEmpty(t, recs)
	assert.Contains(t, recs, "Content blocked - remove immediately")
	assert.Contains(t, recs, "Low confidence decision - consider manual review")
	assert.Contains(t, recs, "ai rec 1")
	assert.Contains(t, recs, "ai rec 2")
}

func TestModerator_generateRecommendations_ActionCases(t *testing.T) {
	m := &Moderator{}

	for action, expected := range map[string]string{
		actionEscalate: "Escalate to human moderator for review",
		actionFlag:     "Flag for moderator attention",
		actionHide:     "Hide content from public view",
	} {
		recs := m.generateRecommendations(&ModerationResult{Action: action, Confidence: 0.9})
		assert.Contains(t, recs, expected)
	}
}

func TestModerator_moderateWithPatterns_GeneratesTextHash(t *testing.T) {
	storage := newFakePatternStorage()
	pm := &PatternManager{storage: storage}

	text := "hello world"
	hashPattern := &ModerationPattern{
		ID:       "hash-1",
		Name:     "hash",
		Type:     "hash",
		Content:  generateTextHash(text),
		Severity: "high",
		Active:   true,
		Action:   "block",
	}
	require.NoError(t, storage.CreateModerationPattern(context.Background(), hashPattern))

	m := &Moderator{patternManager: pm}
	matches, err := m.moderateWithPatterns(context.Background(), &ContentSubmission{
		ID:          "c1",
		Type:        "post",
		Text:        text,
		Author:      "a1",
		SubmittedAt: time.Now(),
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "hash-1", matches[0].PatternID)
}

func TestModerator_ModerateContent_RecordsDecision(t *testing.T) {
	patternStorage := newFakePatternStorage()
	pm := &PatternManager{storage: patternStorage}
	store := &fakeModerationStorage{}

	m := &Moderator{
		patternManager: pm,
		aiAnalyzer:     nil, // Safe when content has no text/image bytes.
		storage:        store,
	}

	result, err := m.ModerateContent(context.Background(), &ContentSubmission{
		ID:          "c1",
		Type:        "post",
		Text:        "",
		Author:      "a1",
		SubmittedAt: time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	store.mu.Lock()
	assert.Len(t, store.decisions, 1)
	store.mu.Unlock()
}

func TestModerator_GetModerationQueue_AndReviewModerationDecision(t *testing.T) {
	patternStorage := newFakePatternStorage()
	pm := &PatternManager{storage: patternStorage}
	store := &fakeModerationStorage{
		queue: []*ModerationQueueItem{{ContentID: "c1"}},
	}

	pattern := &ModerationPattern{
		ID:       "p1",
		Name:     "p1",
		Type:     "keyword",
		Content:  "forbidden",
		Severity: "medium",
		Active:   true,
	}
	require.NoError(t, patternStorage.CreateModerationPattern(context.Background(), pattern))

	m := &Moderator{patternManager: pm, storage: store}

	queue, err := m.GetModerationQueue(context.Background(), &ModerationFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, queue, 1)

	review := &ModerationReview{
		ContentID:  "c1",
		ReviewerID: "r1",
		Decision:   "modify",
		Action:     actionFlag,
		PatternFeedback: map[string]*PatternFeedback{
			"p1": {WasMatch: true, WasFalsePositive: false},
		},
		ReviewedAt: time.Now(),
	}
	require.NoError(t, m.ReviewModerationDecision(context.Background(), review))

	updated, err := patternStorage.GetModerationPattern(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.MatchCount)
}

func TestModerator_ReviewModerationDecision_ReturnsStorageError(t *testing.T) {
	store := &fakeModerationStorage{updateErr: errors.New("boom")}
	m := &Moderator{patternManager: &PatternManager{storage: newFakePatternStorage()}, storage: store}

	err := m.ReviewModerationDecision(context.Background(), &ModerationReview{ContentID: "c1"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFailedToUpdateModerationDecision)
}
