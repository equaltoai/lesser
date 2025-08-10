package advanced

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// NoOpTextAnalyzer provides basic text analysis without AWS dependencies
// It returns neutral/safe results to avoid blocking content when AWS is disabled
type NoOpTextAnalyzer struct {
	logger *zap.Logger
	config *ModerationConfig
}

// NewNoOpTextAnalyzer creates a no-op text analyzer that doesn't require AWS
func NewNoOpTextAnalyzer(logger *zap.Logger, config *ModerationConfig) *NoOpTextAnalyzer {
	return &NoOpTextAnalyzer{
		logger: logger,
		config: config,
	}
}

// AnalyzeText performs no-op text analysis, returning neutral/safe results
func (n *NoOpTextAnalyzer) AnalyzeText(_ context.Context, text string, metadata ContentMetadata) (*ContentAnalysis, error) {
	n.logger.Debug("performing no-op text analysis (AWS disabled)",
		zap.String("content_id", metadata.ContentID),
		zap.Int("text_length", len(text)))

	// Return neutral analysis that doesn't trigger moderation actions
	analysis := &ContentAnalysis{
		ContentID:      metadata.ContentID,
		AnalyzedAt:     time.Now(),
		ProcessingTime: time.Millisecond, // Very fast since we're doing nothing

		// Neutral sentiment
		Sentiment: SentimentAnalysis{
			Sentiment:  "NEUTRAL",
			Positive:   0.33,
			Negative:   0.33,
			Neutral:    0.34,
			Mixed:      0.0,
			Confidence: 0.5, // Low confidence since we didn't actually analyze
		},

		// No toxicity detected
		Toxicity: ToxicityAnalysis{
			IsToxic:        false,
			ToxicityScore:  0.1, // Very low score
			Categories:     []ToxicCategory{},
			TargetedGroups: []string{},
			Confidence:     0.5, // Low confidence
		},

		// No PII detected
		PII: []PIIEntity{},

		// No specific topics detected
		Topics: []Topic{},

		// Default language detection
		Language: LanguageDetection{
			LanguageCode: "en", // Default to English
			Confidence:   0.5,  // Low confidence
		},

		// No threats detected
		Threats: []ThreatIndicator{},

		// Mark as no-op analysis
		CustomFlags: []CustomFlag{
			{
				Name:       "analyzer_type",
				Value:      "no_op",
				Confidence: 1.0,
			},
			{
				Name:       "aws_disabled",
				Value:      true,
				Confidence: 1.0,
			},
		},
	}

	n.logger.Debug("completed no-op text analysis",
		zap.String("content_id", metadata.ContentID),
		zap.Bool("is_toxic", analysis.Toxicity.IsToxic))

	return analysis, nil
}

// NoOpImageAnalyzer provides no-op image analysis without AWS dependencies
// It returns neutral/safe results to avoid blocking content when AWS is disabled
type NoOpImageAnalyzer struct {
	logger *zap.Logger
	config *ModerationConfig
}

// NewNoOpImageAnalyzer creates a no-op image analyzer that doesn't require AWS
func NewNoOpImageAnalyzer(logger *zap.Logger, config *ModerationConfig) *NoOpImageAnalyzer {
	return &NoOpImageAnalyzer{
		logger: logger,
		config: config,
	}
}

// AnalyzeImage performs no-op image analysis, returning neutral/safe results
func (n *NoOpImageAnalyzer) AnalyzeImage(_ context.Context, imageURL string, metadata ContentMetadata) (*ImageAnalysis, error) {
	n.logger.Debug("performing no-op image analysis (AWS disabled)",
		zap.String("content_id", metadata.ContentID),
		zap.String("image_url", imageURL))

	// Return neutral analysis that doesn't trigger moderation actions
	analysis := &ImageAnalysis{
		ImageURL:       imageURL,
		AnalyzedAt:     time.Now(),
		ProcessingTime: time.Millisecond, // Very fast since we're doing nothing

		// No explicit content detected
		Explicit: ExplicitContent{
			IsExplicit:         false,
			NudityScore:        0.1, // Very low scores
			SuggestiveScore:    0.1,
			ViolenceScore:      0.1,
			VisuallyDisturbing: 0.1,
			Confidence:         0.5, // Low confidence since we didn't analyze
		},

		// No violence detected
		Violence: ViolenceDetection{
			HasViolence:     false,
			WeaponsDetected: []string{},
			BloodScore:      0.1,
			ViolenceScore:   0.1,
			Confidence:      0.5,
		},

		// No text in image detected
		Text: []TextInImage{},

		// No objects detected
		Objects: []ObjectDetection{},

		// No faces detected
		Faces: []FaceAnalysis{},

		// No celebrities detected
		Celebrities: []CelebrityMatch{},

		// Mark as no-op analysis
		CustomLabels: []CustomLabel{
			{
				Name:       "analyzer_type",
				Confidence: 1.0,
				Parents:    []string{"no_op"},
			},
			{
				Name:       "aws_disabled",
				Confidence: 1.0,
				Parents:    []string{"system"},
			},
		},
	}

	n.logger.Debug("completed no-op image analysis",
		zap.String("content_id", metadata.ContentID),
		zap.String("image_url", imageURL),
		zap.Bool("is_explicit", analysis.Explicit.IsExplicit),
		zap.Bool("has_violence", analysis.Violence.HasViolence))

	return analysis, nil
}

// NoOpVideoAnalyzer provides no-op video analysis without AWS dependencies
// It returns neutral/safe results to avoid blocking content when AWS is disabled
type NoOpVideoAnalyzer struct {
	logger *zap.Logger
	config *ModerationConfig
}

// NewNoOpVideoAnalyzer creates a no-op video analyzer that doesn't require AWS
func NewNoOpVideoAnalyzer(logger *zap.Logger, config *ModerationConfig) *NoOpVideoAnalyzer {
	return &NoOpVideoAnalyzer{
		logger: logger,
		config: config,
	}
}

// AnalyzeVideo performs no-op video analysis, returning neutral/safe results
func (n *NoOpVideoAnalyzer) AnalyzeVideo(_ context.Context, videoURL string, metadata ContentMetadata) (*VideoAnalysis, error) {
	n.logger.Debug("performing no-op video analysis (AWS disabled)",
		zap.String("content_id", metadata.ContentID),
		zap.String("video_url", videoURL))

	// Return neutral analysis that doesn't trigger moderation actions
	analysis := &VideoAnalysis{
		VideoURL:       videoURL,
		AnalyzedAt:     time.Now(),
		ProcessingTime: time.Millisecond, // Very fast since we're doing nothing
		Duration:       0,                // Unknown duration
		Frames:         []FrameAnalysis{}, // No frames analyzed

		// No audio transcription
		Audio: AudioAnalysis{
			Transcription: "",
			Language:      "unknown",
			TextAnalysis:  nil,
		},
	}

	n.logger.Debug("completed no-op video analysis",
		zap.String("content_id", metadata.ContentID),
		zap.String("video_url", videoURL))

	return analysis, nil
}