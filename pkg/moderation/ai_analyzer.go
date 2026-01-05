// Package moderation provides AI-powered content analysis using AWS Comprehend and Rekognition for automated content moderation.
package moderation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rekognitionTypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"

	"github.com/equaltoai/lesser/pkg/common"
)

// AIAnalyzer provides AI-powered content analysis for moderation
type AIAnalyzer struct {
	comprehend  comprehendAPI
	rekognition rekognitionAPI
}

type comprehendAPI interface {
	DetectDominantLanguage(ctx context.Context, params *comprehend.DetectDominantLanguageInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectDominantLanguageOutput, error)
	DetectSentiment(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error)
	DetectEntities(ctx context.Context, params *comprehend.DetectEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectEntitiesOutput, error)
	DetectKeyPhrases(ctx context.Context, params *comprehend.DetectKeyPhrasesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectKeyPhrasesOutput, error)
	DetectPiiEntities(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error)
}

type rekognitionAPI interface {
	DetectModerationLabels(ctx context.Context, params *rekognition.DetectModerationLabelsInput, optFns ...func(*rekognition.Options)) (*rekognition.DetectModerationLabelsOutput, error)
	DetectLabels(ctx context.Context, params *rekognition.DetectLabelsInput, optFns ...func(*rekognition.Options)) (*rekognition.DetectLabelsOutput, error)
	DetectText(ctx context.Context, params *rekognition.DetectTextInput, optFns ...func(*rekognition.Options)) (*rekognition.DetectTextOutput, error)
	DetectFaces(ctx context.Context, params *rekognition.DetectFacesInput, optFns ...func(*rekognition.Options)) (*rekognition.DetectFacesOutput, error)
}

// NewAIAnalyzer creates a new AI analyzer
func NewAIAnalyzer(cfg aws.Config) *AIAnalyzer {
	return &AIAnalyzer{
		comprehend:  comprehend.NewFromConfig(cfg),
		rekognition: rekognition.NewFromConfig(cfg),
	}
}

// AnalyzeText analyzes text content for moderation using AWS Comprehend
func (ai *AIAnalyzer) AnalyzeText(ctx context.Context, content *TextContent) (*TextAnalysis, error) {
	analysis := &TextAnalysis{
		ContentID:  content.ID,
		Text:       content.Text,
		Language:   "en", // Default to English
		AnalyzedAt: time.Now(),
	}

	// Detect language
	if langResult, err := ai.detectLanguage(ctx, content.Text); err == nil {
		analysis.Language = langResult
	}

	// Detect sentiment
	if sentiment, err := ai.detectSentiment(ctx, content.Text, analysis.Language); err == nil {
		analysis.Sentiment = sentiment
	}

	// Detect entities
	if entities, err := ai.detectEntities(ctx, content.Text, analysis.Language); err == nil {
		analysis.Entities = entities
	}

	// Detect key phrases
	if phrases, err := ai.detectKeyPhrases(ctx, content.Text, analysis.Language); err == nil {
		analysis.KeyPhrases = phrases
	}

	// Detect PII (Personally Identifiable Information)
	if pii, err := ai.detectPII(ctx, content.Text, analysis.Language); err == nil {
		analysis.PIIEntities = pii
	}

	// Calculate moderation score based on various factors
	analysis.ModerationScore = ai.calculateTextModerationScore(analysis)
	analysis.RiskLevel = ai.determineRiskLevel(analysis.ModerationScore)

	// Generate recommendations
	analysis.Recommendations = ai.generateTextRecommendations(analysis)

	return analysis, nil
}

// AnalyzeImage analyzes image content for moderation using AWS Rekognition
func (ai *AIAnalyzer) AnalyzeImage(ctx context.Context, content *ImageContent) (*ImageAnalysis, error) {
	analysis := &ImageAnalysis{
		ContentID:  content.ID,
		ImageURL:   content.URL,
		AnalyzedAt: time.Now(),
	}

	// Detect moderation labels
	if modLabels, err := ai.detectModerationLabels(ctx, content.ImageBytes); err == nil {
		analysis.ModerationLabels = modLabels
	}

	// Detect objects and scenes
	if labels, err := ai.detectLabels(ctx, content.ImageBytes); err == nil {
		analysis.Labels = labels
	}

	// Detect text in image
	if text, err := ai.detectTextInImage(ctx, content.ImageBytes); err == nil {
		analysis.DetectedText = text

		// If text was found, analyze it too
		if err := common.ValidateSliceNotEmpty("text", text); err == nil {
			combinedText := strings.Join(text, " ")
			if textAnalysis, err := ai.AnalyzeText(ctx, &TextContent{
				ID:   content.ID + "_extracted_text",
				Text: combinedText,
			}); err == nil {
				analysis.TextAnalysis = textAnalysis
			}
		}
	}

	// Detect faces (for age estimation and emotion detection)
	if faces, err := ai.detectFaces(ctx, content.ImageBytes); err == nil {
		analysis.Faces = faces
	}

	// Calculate moderation score
	analysis.ModerationScore = ai.calculateImageModerationScore(analysis)
	analysis.RiskLevel = ai.determineRiskLevel(analysis.ModerationScore)

	// Generate recommendations
	analysis.Recommendations = ai.generateImageRecommendations(analysis)

	return analysis, nil
}

// Helper methods for text analysis

func (ai *AIAnalyzer) detectLanguage(ctx context.Context, text string) (string, error) {
	input := &comprehend.DetectDominantLanguageInput{
		Text: aws.String(text),
	}

	result, err := ai.comprehend.DetectDominantLanguage(ctx, input)
	if err != nil {
		return "", err
	}

	if err := common.ValidateSliceNotEmpty("result.Languages", result.Languages); err == nil {
		return *result.Languages[0].LanguageCode, nil
	}

	return "en", nil // Default to English
}

func (ai *AIAnalyzer) detectSentiment(ctx context.Context, text, language string) (*SentimentAnalysis, error) {
	input := &comprehend.DetectSentimentInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCode(language),
	}

	result, err := ai.comprehend.DetectSentiment(ctx, input)
	if err != nil {
		return nil, err
	}

	return &SentimentAnalysis{
		Sentiment:     string(result.Sentiment),
		PositiveScore: *result.SentimentScore.Positive,
		NegativeScore: *result.SentimentScore.Negative,
		NeutralScore:  *result.SentimentScore.Neutral,
		MixedScore:    *result.SentimentScore.Mixed,
	}, nil
}

func (ai *AIAnalyzer) detectEntities(ctx context.Context, text, language string) ([]*EntityDetection, error) {
	input := &comprehend.DetectEntitiesInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCode(language),
	}

	result, err := ai.comprehend.DetectEntities(ctx, input)
	if err != nil {
		return nil, err
	}

	entities := make([]*EntityDetection, 0, len(result.Entities))
	for _, entity := range result.Entities {
		entities = append(entities, &EntityDetection{
			Text:        *entity.Text,
			Type:        string(entity.Type),
			Score:       *entity.Score,
			BeginOffset: *entity.BeginOffset,
			EndOffset:   *entity.EndOffset,
		})
	}

	return entities, nil
}

func (ai *AIAnalyzer) detectKeyPhrases(ctx context.Context, text, language string) ([]*KeyPhrase, error) {
	input := &comprehend.DetectKeyPhrasesInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCode(language),
	}

	result, err := ai.comprehend.DetectKeyPhrases(ctx, input)
	if err != nil {
		return nil, err
	}

	phrases := make([]*KeyPhrase, 0, len(result.KeyPhrases))
	for _, phrase := range result.KeyPhrases {
		phrases = append(phrases, &KeyPhrase{
			Text:        *phrase.Text,
			Score:       *phrase.Score,
			BeginOffset: *phrase.BeginOffset,
			EndOffset:   *phrase.EndOffset,
		})
	}

	return phrases, nil
}

func (ai *AIAnalyzer) detectPII(ctx context.Context, text, language string) ([]*PIIEntity, error) {
	input := &comprehend.DetectPiiEntitiesInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCode(language),
	}

	result, err := ai.comprehend.DetectPiiEntities(ctx, input)
	if err != nil {
		return nil, err
	}

	piiEntities := make([]*PIIEntity, 0, len(result.Entities))
	for _, entity := range result.Entities {
		piiEntities = append(piiEntities, &PIIEntity{
			Type:        string(entity.Type),
			Score:       *entity.Score,
			BeginOffset: *entity.BeginOffset,
			EndOffset:   *entity.EndOffset,
		})
	}

	return piiEntities, nil
}

// Helper methods for image analysis

func (ai *AIAnalyzer) detectModerationLabels(ctx context.Context, imageBytes []byte) ([]*ModerationLabel, error) {
	input := &rekognition.DetectModerationLabelsInput{
		Image: &rekognitionTypes.Image{
			Bytes: imageBytes,
		},
		MinConfidence: aws.Float32(50.0), // Minimum confidence threshold
	}

	result, err := ai.rekognition.DetectModerationLabels(ctx, input)
	if err != nil {
		return nil, err
	}

	labels := make([]*ModerationLabel, 0, len(result.ModerationLabels))
	for _, label := range result.ModerationLabels {
		labels = append(labels, &ModerationLabel{
			Name:       *label.Name,
			Confidence: *label.Confidence,
			ParentName: aws.ToString(label.ParentName),
		})
	}

	return labels, nil
}

func (ai *AIAnalyzer) detectLabels(ctx context.Context, imageBytes []byte) ([]*ImageLabel, error) {
	input := &rekognition.DetectLabelsInput{
		Image: &rekognitionTypes.Image{
			Bytes: imageBytes,
		},
		MaxLabels:     aws.Int32(50),
		MinConfidence: aws.Float32(70.0),
	}

	result, err := ai.rekognition.DetectLabels(ctx, input)
	if err != nil {
		return nil, err
	}

	labels := make([]*ImageLabel, 0, len(result.Labels))
	for _, label := range result.Labels {
		labels = append(labels, &ImageLabel{
			Name:       *label.Name,
			Confidence: *label.Confidence,
			Instances:  len(label.Instances),
		})
	}

	return labels, nil
}

func (ai *AIAnalyzer) detectTextInImage(ctx context.Context, imageBytes []byte) ([]string, error) {
	input := &rekognition.DetectTextInput{
		Image: &rekognitionTypes.Image{
			Bytes: imageBytes,
		},
	}

	result, err := ai.rekognition.DetectText(ctx, input)
	if err != nil {
		return nil, err
	}

	var detectedTexts []string
	for _, textDetection := range result.TextDetections {
		if textDetection.Type == rekognitionTypes.TextTypesLine {
			detectedTexts = append(detectedTexts, *textDetection.DetectedText)
		}
	}

	return detectedTexts, nil
}

func (ai *AIAnalyzer) detectFaces(ctx context.Context, imageBytes []byte) ([]*FaceDetection, error) {
	input := &rekognition.DetectFacesInput{
		Image: &rekognitionTypes.Image{
			Bytes: imageBytes,
		},
		Attributes: []rekognitionTypes.Attribute{
			rekognitionTypes.AttributeAll,
		},
	}

	result, err := ai.rekognition.DetectFaces(ctx, input)
	if err != nil {
		return nil, err
	}

	faces := make([]*FaceDetection, 0, len(result.FaceDetails))
	for _, faceDetail := range result.FaceDetails {
		face := &FaceDetection{
			Confidence: *faceDetail.Confidence,
			AgeRange: &AgeRange{
				Low:  *faceDetail.AgeRange.Low,
				High: *faceDetail.AgeRange.High,
			},
		}

		// Extract emotions
		for _, emotion := range faceDetail.Emotions {
			face.Emotions = append(face.Emotions, &Emotion{
				Type:       string(emotion.Type),
				Confidence: *emotion.Confidence,
			})
		}

		faces = append(faces, face)
	}

	return faces, nil
}

// Scoring and risk assessment methods

func (ai *AIAnalyzer) calculateTextModerationScore(analysis *TextAnalysis) float64 {
	score := 0.0

	// Sentiment analysis contribution
	if analysis.Sentiment != nil {
		if analysis.Sentiment.Sentiment == "NEGATIVE" {
			score += float64(analysis.Sentiment.NegativeScore) * 30.0
		}
	}

	// PII detection contribution
	if err := common.ValidateSliceNotEmpty("analysis.PIIEntities", analysis.PIIEntities); err == nil {
		score += float64(len(analysis.PIIEntities)) * 15.0
	}

	// Entity analysis - check for potentially harmful entities
	harmfulEntityTypes := map[string]float64{
		"PERSON":       5.0,
		"LOCATION":     3.0,
		"ORGANIZATION": 2.0,
	}

	for _, entity := range analysis.Entities {
		if weight, exists := harmfulEntityTypes[entity.Type]; exists {
			score += weight * float64(entity.Score)
		}
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

func (ai *AIAnalyzer) calculateImageModerationScore(analysis *ImageAnalysis) float64 {
	score := 0.0

	// Moderation labels contribute the most to score
	for _, label := range analysis.ModerationLabels {
		score += float64(label.Confidence) * 0.8 // Weight moderation labels heavily
	}

	// Text analysis contribution if text was found
	if analysis.TextAnalysis != nil {
		score += analysis.TextAnalysis.ModerationScore * 0.3
	}

	// Face analysis - multiple faces might indicate different content
	if len(analysis.Faces) > 3 {
		score += 10.0 // Slight penalty for many faces
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

func (ai *AIAnalyzer) determineRiskLevel(score float64) string {
	if score >= 80 {
		return "high"
	} else if score >= 50 {
		return "medium"
	} else if score >= 20 {
		return "low"
	}
	return "minimal"
}

func (ai *AIAnalyzer) generateTextRecommendations(analysis *TextAnalysis) []string {
	var recommendations []string

	if analysis.ModerationScore >= 80 {
		recommendations = append(recommendations, "Block content - high risk detected")
	} else if analysis.ModerationScore >= 50 {
		recommendations = append(recommendations, "Flag for manual review")
	}

	if err := common.ValidateSliceNotEmpty("analysis.PIIEntities", analysis.PIIEntities); err == nil {
		recommendations = append(recommendations, "Contains PII - consider redaction")
	}

	if analysis.Sentiment != nil && analysis.Sentiment.Sentiment == "NEGATIVE" && analysis.Sentiment.NegativeScore > 0.8 {
		recommendations = append(recommendations, "Highly negative sentiment - monitor for harmful content")
	}

	return recommendations
}

func (ai *AIAnalyzer) generateImageRecommendations(analysis *ImageAnalysis) []string {
	var recommendations []string

	if analysis.ModerationScore >= 80 {
		recommendations = append(recommendations, "Block image - high risk content detected")
	} else if analysis.ModerationScore >= 50 {
		recommendations = append(recommendations, "Flag for manual review")
	}

	// Check for specific moderation labels
	highRiskLabels := []string{"Explicit Nudity", "Violence", "Hate Symbols"}
	for _, label := range analysis.ModerationLabels {
		for _, riskLabel := range highRiskLabels {
			if strings.Contains(label.Name, riskLabel) && label.Confidence > 75 {
				recommendations = append(recommendations, fmt.Sprintf("Contains %s - immediate action required", riskLabel))
				break
			}
		}
	}

	if err := common.ValidateSliceNotEmpty("analysis.DetectedText", analysis.DetectedText); err == nil {
		recommendations = append(recommendations, "Contains text - analyze extracted text for harmful content")
	}

	return recommendations
}

// Types for AI analysis

// TextContent represents text content to be analyzed
type TextContent struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// ImageContent represents image content to be analyzed
type ImageContent struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	ImageBytes []byte `json:"-"`
}

// TextAnalysis represents the result of text content analysis
type TextAnalysis struct {
	ContentID       string             `json:"content_id"`
	Text            string             `json:"text"`
	Language        string             `json:"language"`
	Sentiment       *SentimentAnalysis `json:"sentiment,omitempty"`
	Entities        []*EntityDetection `json:"entities,omitempty"`
	KeyPhrases      []*KeyPhrase       `json:"key_phrases,omitempty"`
	PIIEntities     []*PIIEntity       `json:"pii_entities,omitempty"`
	ModerationScore float64            `json:"moderation_score"`
	RiskLevel       string             `json:"risk_level"`
	Recommendations []string           `json:"recommendations"`
	AnalyzedAt      time.Time          `json:"analyzed_at"`
}

// ImageAnalysis represents the result of image content analysis
type ImageAnalysis struct {
	ContentID        string             `json:"content_id"`
	ImageURL         string             `json:"image_url"`
	ModerationLabels []*ModerationLabel `json:"moderation_labels,omitempty"`
	Labels           []*ImageLabel      `json:"labels,omitempty"`
	DetectedText     []string           `json:"detected_text,omitempty"`
	Faces            []*FaceDetection   `json:"faces,omitempty"`
	TextAnalysis     *TextAnalysis      `json:"text_analysis,omitempty"`
	ModerationScore  float64            `json:"moderation_score"`
	RiskLevel        string             `json:"risk_level"`
	Recommendations  []string           `json:"recommendations"`
	AnalyzedAt       time.Time          `json:"analyzed_at"`
}

// SentimentAnalysis represents sentiment analysis results
type SentimentAnalysis struct {
	Sentiment     string  `json:"sentiment"`
	PositiveScore float32 `json:"positive_score"`
	NegativeScore float32 `json:"negative_score"`
	NeutralScore  float32 `json:"neutral_score"`
	MixedScore    float32 `json:"mixed_score"`
}

// EntityDetection represents a detected entity in text
type EntityDetection struct {
	Text        string  `json:"text"`
	Type        string  `json:"type"`
	Score       float32 `json:"score"`
	BeginOffset int32   `json:"begin_offset"`
	EndOffset   int32   `json:"end_offset"`
}

// KeyPhrase represents a detected key phrase in text
type KeyPhrase struct {
	Text        string  `json:"text"`
	Score       float32 `json:"score"`
	BeginOffset int32   `json:"begin_offset"`
	EndOffset   int32   `json:"end_offset"`
}

// PIIEntity represents a detected personally identifiable information entity
type PIIEntity struct {
	Type        string  `json:"type"`
	Score       float32 `json:"score"`
	BeginOffset int32   `json:"begin_offset"`
	EndOffset   int32   `json:"end_offset"`
}

// ModerationLabel represents a moderation label detected in content
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific label
type ModerationLabel struct {
	Name       string  `json:"name"`
	Confidence float32 `json:"confidence"`
	ParentName string  `json:"parent_name,omitempty"`
}

// ImageLabel represents a label detected in an image
type ImageLabel struct {
	Name       string  `json:"name"`
	Confidence float32 `json:"confidence"`
	Instances  int     `json:"instances"`
}

// FaceDetection represents a detected face in an image
type FaceDetection struct {
	Confidence float32    `json:"confidence"`
	AgeRange   *AgeRange  `json:"age_range,omitempty"`
	Emotions   []*Emotion `json:"emotions,omitempty"`
}

// AgeRange represents an estimated age range for a detected face
type AgeRange struct {
	Low  int32 `json:"low"`
	High int32 `json:"high"`
}

// Emotion represents a detected emotion in a face
type Emotion struct {
	Type       string  `json:"type"`
	Confidence float32 `json:"confidence"`
}
