package moderation

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAIAnalyzer_Constructs(t *testing.T) {
	ai := NewAIAnalyzer(aws.Config{Region: "us-east-1"})
	require.NotNil(t, ai)
}

func TestAIAnalyzer_TextScoringRiskAndRecommendations(t *testing.T) {
	ai := &AIAnalyzer{}

	analysis := &TextAnalysis{
		Sentiment: &SentimentAnalysis{
			Sentiment:     "NEGATIVE",
			NegativeScore: 0.5,
		},
		PIIEntities: []*PIIEntity{
			{Type: "SSN", Score: 0.9},
			{Type: "PHONE", Score: 0.9},
		},
		Entities: []*EntityDetection{
			{Type: "PERSON", Score: 0.8},
			{Type: "LOCATION", Score: 1.0},
		},
	}

	score := ai.calculateTextModerationScore(analysis)
	assert.InDelta(t, 52.0, score, 0.01)
	assert.Equal(t, "medium", ai.determineRiskLevel(score))

	analysis.ModerationScore = 85
	analysis.Sentiment.NegativeScore = 0.9
	recs := ai.generateTextRecommendations(analysis)
	assert.NotEmpty(t, recs)
	assert.Contains(t, recs, "Block content - high risk detected")
	assert.Contains(t, recs, "Contains PII - consider redaction")
	assert.Contains(t, recs, "Highly negative sentiment - monitor for harmful content")

	analysis.ModerationScore = 10
	recs = ai.generateTextRecommendations(analysis)
	assert.NotContains(t, recs, "Flag for manual review")
}

func TestAIAnalyzer_ImageScoringRiskAndRecommendations(t *testing.T) {
	ai := &AIAnalyzer{}

	analysis := &ImageAnalysis{
		ModerationLabels: []*ModerationLabel{
			{Name: "Explicit Nudity", Confidence: 90},
		},
		TextAnalysis: &TextAnalysis{ModerationScore: 50},
		DetectedText: []string{"some text"},
		Faces: []*FaceDetection{
			{}, {}, {}, {},
		},
	}

	score := ai.calculateImageModerationScore(analysis)
	assert.Greater(t, score, 0.0)
	assert.Equal(t, "high", ai.determineRiskLevel(score))

	analysis.ModerationScore = score
	recs := ai.generateImageRecommendations(analysis)
	assert.NotEmpty(t, recs)
	assert.Contains(t, recs, "Contains Explicit Nudity - immediate action required")
	assert.Contains(t, recs, "Contains text - analyze extracted text for harmful content")
}

func TestAIAnalyzer_ScoringCapsAt100(t *testing.T) {
	ai := &AIAnalyzer{}

	analysis := &ImageAnalysis{
		ModerationLabels: []*ModerationLabel{
			{Name: "Violence", Confidence: 200},
		},
		TextAnalysis: &TextAnalysis{ModerationScore: 200},
		Faces:        make([]*FaceDetection, 10),
	}

	assert.Equal(t, 100.0, ai.calculateImageModerationScore(analysis))
}
