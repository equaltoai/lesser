package ai

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/stretchr/testify/require"
)

func TestService_isHighPriority(t *testing.T) {
	svc := &Service{}

	require.True(t, svc.isHighPriority(&ai.AIAnalysis{ModerationAction: ai.ActionRemove, OverallRisk: 0.1}))
	require.True(t, svc.isHighPriority(&ai.AIAnalysis{ModerationAction: ai.ActionHide, OverallRisk: 0.1}))
	require.True(t, svc.isHighPriority(&ai.AIAnalysis{ModerationAction: ai.ActionNone, OverallRisk: 0.81}))
	require.False(t, svc.isHighPriority(&ai.AIAnalysis{ModerationAction: ai.ActionNone, OverallRisk: 0.8}))
}

func TestService_createAnalysisEvent_StreamAndPayload(t *testing.T) {
	svc := &Service{}
	analyzedAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	before := time.Now()
	ev := svc.createAnalysisEvent(&ai.AIAnalysis{
		ID:               "analysis-1",
		ObjectID:         "obj-1",
		ObjectType:       "status",
		ModerationAction: ai.ActionRemove,
		OverallRisk:      0.1,
		Confidence:       0.9,
		AnalyzedAt:       analyzedAt,
		Version:          "v1",
	}, "user-1")

	require.Equal(t, "ai.analysis.completed", ev.Type)
	require.Equal(t, "ai_analysis_urgent", ev.Stream)
	require.NotNil(t, ev.Payload)
	require.Equal(t, "analysis-1", ev.Payload["analysis_id"])
	require.Equal(t, "obj-1", ev.Payload["content_id"])
	require.Equal(t, "status", ev.Payload["content_type"])
	require.Equal(t, "user-1", ev.Payload["user_id"])
	require.Equal(t, analyzedAt, ev.Payload["processed_at"])
	require.True(t, ev.Timestamp.After(before) || ev.Timestamp.Equal(before))
}

func TestService_convertAnalysisToResults(t *testing.T) {
	svc := &Service{}
	analysis := &ai.AIAnalysis{
		OverallRisk:      0.42,
		ModerationAction: ai.ActionFlag,
		Confidence:       0.8,
		TextAnalysis: &ai.TextAnalysis{
			Sentiment:        ai.SentimentPositive,
			SentimentScores:  map[string]float64{"positive": 0.9},
			ToxicityScore:    0.1,
			ToxicityLabels:   []string{"low"},
			ContainsPII:      true,
			DominantLanguage: "en",
			KeyPhrases:       []string{"hello"},
		},
		ImageAnalysis: &ai.ImageAnalysis{
			IsNSFW:           true,
			NSFWConfidence:   0.95,
			ViolenceScore:    0.2,
			WeaponsDetected:  false,
			DeepfakeScore:    0.01,
			DetectedText:     []string{"text"},
			ModerationLabels: []ai.ModerationLabel{{Name: "Explicit", Confidence: 0.8}},
		},
	}

	results := svc.convertAnalysisToResults(analysis)
	require.Contains(t, results, "text")
	require.Contains(t, results, "image")
	require.Contains(t, results, "overall")

	overall, ok := results["overall"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, analysis.OverallRisk, overall["risk_score"])
	require.Equal(t, analysis.ModerationAction, overall["moderation_action"])
	require.Equal(t, analysis.Confidence, overall["confidence"])
}

func TestService_SaveAnalysis_validation(t *testing.T) {
	svc := &Service{}

	_, err := svc.SaveAnalysis(context.Background(), &SaveAnalysisCommand{Analysis: nil})
	require.Error(t, err)

	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	require.Equal(t, "analysis", ve.Field)
}

func TestService_GetAnalysis_validation(t *testing.T) {
	svc := &Service{}

	_, err := svc.GetAnalysis(context.Background(), &GetAnalysisQuery{ObjectID: ""})
	require.Error(t, err)

	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	require.Equal(t, "object_id", ve.Field)
}

func TestService_QueueForAnalysis_validation(t *testing.T) {
	svc := &Service{}

	_, err := svc.QueueForAnalysis(context.Background(), &QueueAnalysisCommand{ObjectID: ""})
	require.Error(t, err)

	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	require.Equal(t, "object_id", ve.Field)
}
