package advanced

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNoOpTextAnalyzer_AnalyzeText(t *testing.T) {
	logger := zap.NewNop()
	config := &ModerationConfig{}
	analyzer := NewNoOpTextAnalyzer(logger, config)
	ctx := context.Background()

	tests := []struct {
		name     string
		text     string
		metadata ContentMetadata
	}{
		{
			name: "normal text returns safe analysis",
			text: "This is normal content that should pass through.",
			metadata: ContentMetadata{
				ContentID: "test-1",
				AuthorID:  "author-1",
			},
		},
		{
			name: "empty text returns safe analysis",
			text: "",
			metadata: ContentMetadata{
				ContentID: "test-2",
			},
		},
		{
			name: "long text returns safe analysis",
			text: string(make([]byte, 10000)),
			metadata: ContentMetadata{
				ContentID: "test-3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := analyzer.AnalyzeText(ctx, tt.text, tt.metadata)
			require.NoError(t, err)

			// Verify safe/neutral outputs
			assert.Equal(t, tt.metadata.ContentID, analysis.ContentID)
			assert.False(t, analysis.Toxicity.IsToxic)
			assert.Equal(t, 0.1, analysis.Toxicity.ToxicityScore)
			assert.Equal(t, 0.5, analysis.Toxicity.Confidence)

			// Verify neutral sentiment
			assert.Equal(t, "NEUTRAL", analysis.Sentiment.Sentiment)
			assert.InDelta(t, 0.33, analysis.Sentiment.Positive, 0.01)
			assert.InDelta(t, 0.33, analysis.Sentiment.Negative, 0.01)
			assert.InDelta(t, 0.34, analysis.Sentiment.Neutral, 0.01)

			// Verify no threats/PII
			assert.Empty(t, analysis.Threats)
			assert.Empty(t, analysis.PII)

			// Verify processing time is set
			assert.True(t, analysis.ProcessingTime > 0)
			assert.False(t, analysis.AnalyzedAt.IsZero())

			// Verify no-op custom flags
			hasNoOpFlag := false
			hasAwsDisabledFlag := false
			for _, flag := range analysis.CustomFlags {
				if flag.Name == "analyzer_type" && flag.Value == "no_op" {
					hasNoOpFlag = true
				}
				if flag.Name == "aws_disabled" {
					hasAwsDisabledFlag = true
				}
			}
			assert.True(t, hasNoOpFlag, "should have analyzer_type=no_op flag")
			assert.True(t, hasAwsDisabledFlag, "should have aws_disabled flag")

			// Verify default language detection
			assert.Equal(t, "en", analysis.Language.LanguageCode)
			assert.Equal(t, 0.5, analysis.Language.Confidence)
		})
	}
}

func TestNoOpImageAnalyzer_AnalyzeImage(t *testing.T) {
	logger := zap.NewNop()
	config := &ModerationConfig{}
	analyzer := NewNoOpImageAnalyzer(logger, config)
	ctx := context.Background()

	tests := []struct {
		name     string
		imageURL string
		metadata ContentMetadata
	}{
		{
			name:     "normal image URL returns safe analysis",
			imageURL: "https://example.com/image.jpg",
			metadata: ContentMetadata{
				ContentID: "img-1",
				AuthorID:  "author-1",
			},
		},
		{
			name:     "S3 URL returns safe analysis",
			imageURL: "s3://bucket/key/image.png",
			metadata: ContentMetadata{
				ContentID: "img-2",
			},
		},
		{
			name:     "empty URL returns safe analysis",
			imageURL: "",
			metadata: ContentMetadata{
				ContentID: "img-3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := analyzer.AnalyzeImage(ctx, tt.imageURL, tt.metadata)
			require.NoError(t, err)

			// Verify correct URL returned
			assert.Equal(t, tt.imageURL, analysis.ImageURL)

			// Verify safe outputs
			assert.False(t, analysis.Explicit.IsExplicit)
			assert.Equal(t, 0.1, analysis.Explicit.NudityScore)
			assert.Equal(t, 0.1, analysis.Explicit.SuggestiveScore)
			assert.Equal(t, 0.1, analysis.Explicit.ViolenceScore)
			assert.Equal(t, 0.5, analysis.Explicit.Confidence)

			// Verify no violence
			assert.False(t, analysis.Violence.HasViolence)
			assert.Equal(t, 0.1, analysis.Violence.ViolenceScore)
			assert.Equal(t, 0.1, analysis.Violence.BloodScore)
			assert.Empty(t, analysis.Violence.WeaponsDetected)

			// Verify no detections
			assert.Empty(t, analysis.Text)
			assert.Empty(t, analysis.Objects)
			assert.Empty(t, analysis.Faces)
			assert.Empty(t, analysis.Celebrities)

			// Verify processing time and timestamp
			assert.True(t, analysis.ProcessingTime > 0)
			assert.False(t, analysis.AnalyzedAt.IsZero())

			// Verify custom labels indicate no-op
			hasNoOpLabel := false
			for _, label := range analysis.CustomLabels {
				if label.Name == "analyzer_type" {
					hasNoOpLabel = true
					assert.Contains(t, label.Parents, "no_op")
				}
			}
			assert.True(t, hasNoOpLabel, "should have analyzer_type label")
		})
	}
}

func TestNoOpVideoAnalyzer_AnalyzeVideo(t *testing.T) {
	logger := zap.NewNop()
	config := &ModerationConfig{}
	analyzer := NewNoOpVideoAnalyzer(logger, config)
	ctx := context.Background()

	tests := []struct {
		name     string
		videoURL string
		metadata ContentMetadata
	}{
		{
			name:     "normal video URL returns safe analysis",
			videoURL: "https://example.com/video.mp4",
			metadata: ContentMetadata{
				ContentID: "vid-1",
				AuthorID:  "author-1",
			},
		},
		{
			name:     "S3 URL returns safe analysis",
			videoURL: "s3://bucket/videos/test.mp4",
			metadata: ContentMetadata{
				ContentID: "vid-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := analyzer.AnalyzeVideo(ctx, tt.videoURL, tt.metadata)
			require.NoError(t, err)

			// Verify correct URL returned
			assert.Equal(t, tt.videoURL, analysis.VideoURL)

			// Verify empty frames (no analysis performed)
			assert.Empty(t, analysis.Frames)

			// Verify audio analysis is empty
			assert.Empty(t, analysis.Audio.Transcription)
			assert.Equal(t, "unknown", analysis.Audio.Language)
			assert.Nil(t, analysis.Audio.TextAnalysis)

			// Verify duration is zero (unknown)
			assert.Equal(t, time.Duration(0), analysis.Duration)

			// Verify processing time and timestamp
			assert.True(t, analysis.ProcessingTime > 0)
			assert.False(t, analysis.AnalyzedAt.IsZero())
		})
	}
}

func TestNoOpAnalyzers_Consistency(t *testing.T) {
	logger := zap.NewNop()
	config := &ModerationConfig{}
	ctx := context.Background()

	// Create all analyzers
	textAnalyzer := NewNoOpTextAnalyzer(logger, config)
	imageAnalyzer := NewNoOpImageAnalyzer(logger, config)
	videoAnalyzer := NewNoOpVideoAnalyzer(logger, config)

	metadata := ContentMetadata{ContentID: "consistency-test"}

	// Run multiple times to ensure consistent results
	for i := 0; i < 5; i++ {
		textResult, err := textAnalyzer.AnalyzeText(ctx, "test", metadata)
		require.NoError(t, err)
		assert.False(t, textResult.Toxicity.IsToxic)
		assert.Equal(t, 0.1, textResult.Toxicity.ToxicityScore)

		imageResult, err := imageAnalyzer.AnalyzeImage(ctx, "http://test.jpg", metadata)
		require.NoError(t, err)
		assert.False(t, imageResult.Explicit.IsExplicit)
		assert.Equal(t, 0.1, imageResult.Explicit.NudityScore)

		videoResult, err := videoAnalyzer.AnalyzeVideo(ctx, "http://test.mp4", metadata)
		require.NoError(t, err)
		assert.Empty(t, videoResult.Frames)
	}
}
