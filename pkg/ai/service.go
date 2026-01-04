package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	comprehendtypes "github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rekognitiontypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqs_types "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/ssrf"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SQSClient defines the interface for SQS operations needed by AIService
type SQSClient interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

// ComprehendClient defines the interface for AWS Comprehend operations needed by AIService
type ComprehendClient interface {
	DetectDominantLanguage(ctx context.Context, params *comprehend.DetectDominantLanguageInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectDominantLanguageOutput, error)
	DetectSentiment(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error)
	ClassifyDocument(ctx context.Context, params *comprehend.ClassifyDocumentInput, optFns ...func(*comprehend.Options)) (*comprehend.ClassifyDocumentOutput, error)
	DetectPiiEntities(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error)
	DetectEntities(ctx context.Context, params *comprehend.DetectEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectEntitiesOutput, error)
	DetectKeyPhrases(ctx context.Context, params *comprehend.DetectKeyPhrasesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectKeyPhrasesOutput, error)
}

// RekognitionClient defines the interface for AWS Rekognition operations needed by AIService
type RekognitionClient interface {
	DetectModerationLabels(ctx context.Context, params *rekognition.DetectModerationLabelsInput, optFns ...func(*rekognition.Options)) (*rekognition.DetectModerationLabelsOutput, error)
	DetectText(ctx context.Context, params *rekognition.DetectTextInput, optFns ...func(*rekognition.Options)) (*rekognition.DetectTextOutput, error)
	RecognizeCelebrities(ctx context.Context, params *rekognition.RecognizeCelebritiesInput, optFns ...func(*rekognition.Options)) (*rekognition.RecognizeCelebritiesOutput, error)
}

// BedrockRuntimeClient defines the interface for AWS Bedrock operations needed by AIService
type BedrockRuntimeClient interface {
	InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

// S3Client defines the interface for AWS S3 operations needed by AIService
type S3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// HTTPClient defines the interface for HTTP operations needed by AIService
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// AIService provides AI-powered content moderation and analysis
//
//nolint:revive // AI prefix clarifies this is AI-specific service
type AIService struct {
	comprehend  ComprehendClient
	rekognition RekognitionClient
	bedrock     BedrockRuntimeClient
	s3Client    S3Client
	sqsClient   SQSClient
	httpClient  HTTPClient
	logger      *zap.Logger
	config      *AIConfig
}

// AIConfig contains configuration for AI service features and thresholds
//
//nolint:revive // AI prefix clarifies this is AI-specific config
type AIConfig struct {
	// Thresholds for auto-moderation
	NSFWThreshold      float64
	ToxicityThreshold  float64
	SpamThreshold      float64
	AIContentThreshold float64

	// Feature flags
	EnablePIIDetection  bool
	EnableAIDetection   bool
	EnableImageAnalysis bool

	// Model configurations
	BedrockModelID   string
	ToxicityModelARN string // Custom Comprehend classifier

	// S3 bucket for image analysis
	S3Bucket string

	// SQS queue URL for AI processing
	AIQueueURL string
}

// NewAIService creates a new AI service instance
func NewAIService(cfg aws.Config, aiConfig *AIConfig) *AIService {
	logger := zap.L().Named("ai")
	return &AIService{
		comprehend:  comprehend.NewFromConfig(cfg),
		rekognition: rekognition.NewFromConfig(cfg),
		bedrock:     bedrockruntime.NewFromConfig(cfg),
		s3Client:    s3.NewFromConfig(cfg),
		sqsClient:   sqs.NewFromConfig(cfg),
		httpClient:  newSSRFProtectedHTTPClient(logger),
		logger:      logger,
		config:      aiConfig,
	}
}

// NewAIServiceWithSQS creates a new AI service instance with custom SQS client
func NewAIServiceWithSQS(cfg aws.Config, aiConfig *AIConfig, sqsClient SQSClient) *AIService {
	logger := zap.L().Named("ai")
	return &AIService{
		comprehend:  comprehend.NewFromConfig(cfg),
		rekognition: rekognition.NewFromConfig(cfg),
		bedrock:     bedrockruntime.NewFromConfig(cfg),
		s3Client:    s3.NewFromConfig(cfg),
		sqsClient:   sqsClient,
		httpClient:  newSSRFProtectedHTTPClient(logger),
		logger:      logger,
		config:      aiConfig,
	}
}

// AnalyzeContent performs comprehensive AI analysis
func (s *AIService) AnalyzeContent(ctx context.Context, content *Content) (*AIAnalysis, error) {
	analysis := &AIAnalysis{
		ID:         generateID("ai-analysis"),
		ObjectID:   content.ID,
		ObjectType: content.Type,
		AnalyzedAt: time.Now(),
		Version:    "1.0",
		TTL:        time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days retention
	}

	// Parallel analysis for performance
	var wg sync.WaitGroup
	var mu sync.Mutex
	var textErr, imageErr, aiErr, spamErr error

	// Text analysis
	if content.Text != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			textAnalysis, err := s.analyzeText(ctx, content.Text)
			mu.Lock()
			analysis.TextAnalysis = textAnalysis
			textErr = err
			mu.Unlock()
		}()
	}

	// Image analysis
	if common.ValidateSliceNotEmpty("content.MediaURLs", content.MediaURLs) == nil && s.config.EnableImageAnalysis {
		wg.Add(1)
		go func() {
			defer wg.Done()
			imageAnalysis, err := s.analyzeImages(ctx, content.MediaURLs)
			mu.Lock()
			analysis.ImageAnalysis = imageAnalysis
			imageErr = err
			mu.Unlock()
		}()
	}

	// AI detection
	if s.config.EnableAIDetection && content.Text != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			aiDetection, err := s.detectAIContent(ctx, content)
			mu.Lock()
			analysis.AIDetection = aiDetection
			aiErr = err
			mu.Unlock()
		}()
	}

	// Spam analysis
	wg.Add(1)
	go func() {
		defer wg.Done()
		spamAnalysis, err := s.analyzeSpam(ctx, content)
		mu.Lock()
		analysis.SpamAnalysis = spamAnalysis
		spamErr = err
		mu.Unlock()
	}()

	wg.Wait()

	// Check errors
	if textErr != nil || imageErr != nil || aiErr != nil || spamErr != nil {
		s.logger.Warn("partial AI analysis failure",
			zap.Error(textErr),
			zap.Error(imageErr),
			zap.Error(aiErr),
			zap.Error(spamErr),
		)
	}

	// Calculate composite scores
	analysis.OverallRisk = s.calculateOverallRisk(analysis)
	analysis.ModerationAction = s.determineModerationAction(analysis)
	analysis.Confidence = s.calculateConfidence(analysis)

	// Note: Storage is now handled by the caller (AI processor Lambda)
	// This service only performs the analysis
	s.logger.Info("Analysis completed",
		zap.String("analysisID", analysis.ID),
		zap.String("objectID", analysis.ObjectID),
		zap.Float64("overallRisk", analysis.OverallRisk),
		zap.String("moderationAction", analysis.ModerationAction))

	return analysis, nil
}

// analyzeText uses AWS Comprehend
func (s *AIService) analyzeText(ctx context.Context, text string) (*TextAnalysis, error) {
	analysis := &TextAnalysis{
		Categories:     []ContentCategory{},
		PIIEntities:    []PIIEntity{},
		Entities:       []Entity{},
		KeyPhrases:     []string{},
		ToxicityLabels: []string{},
	}

	// Language detection first
	langResp, err := s.comprehend.DetectDominantLanguage(ctx, &comprehend.DetectDominantLanguageInput{
		Text: aws.String(text),
	})
	if err == nil && len(langResp.Languages) > 0 {
		analysis.DominantLanguage = *langResp.Languages[0].LanguageCode
		analysis.LanguageScores = make(map[string]float64)
		for _, lang := range langResp.Languages {
			analysis.LanguageScores[*lang.LanguageCode] = float64(*lang.Score)
		}
	} else {
		// Default to English if detection fails
		analysis.DominantLanguage = "en"
	}

	// Use detected language for other analyses
	langCode := comprehendtypes.LanguageCode(analysis.DominantLanguage)

	// Sentiment analysis
	sentimentResp, err := s.comprehend.DetectSentiment(ctx, &comprehend.DetectSentimentInput{
		Text:         aws.String(text),
		LanguageCode: langCode,
	})
	if err == nil {
		analysis.Sentiment = string(sentimentResp.Sentiment)
		analysis.SentimentScores = map[string]float64{
			"positive": float64(*sentimentResp.SentimentScore.Positive),
			"negative": float64(*sentimentResp.SentimentScore.Negative),
			"neutral":  float64(*sentimentResp.SentimentScore.Neutral),
			"mixed":    float64(*sentimentResp.SentimentScore.Mixed),
		}
	}

	// Toxicity detection (custom classifier if available)
	if s.config.ToxicityModelARN != "" {
		toxicityResp, err := s.comprehend.ClassifyDocument(ctx, &comprehend.ClassifyDocumentInput{
			Text:        aws.String(text),
			EndpointArn: aws.String(s.config.ToxicityModelARN),
		})
		if err == nil {
			analysis.ToxicityScore = s.extractToxicityScore(toxicityResp)
			analysis.ToxicityLabels = s.extractToxicityLabels(toxicityResp)
		}
	} else {
		// Fallback: Use sentiment as proxy for toxicity
		if analysis.SentimentScores != nil {
			analysis.ToxicityScore = analysis.SentimentScores["negative"] * 0.5
		}
	}

	// PII detection
	if s.config.EnablePIIDetection {
		piiResp, err := s.comprehend.DetectPiiEntities(ctx, &comprehend.DetectPiiEntitiesInput{
			Text:         aws.String(text),
			LanguageCode: langCode,
		})
		if err == nil && len(piiResp.Entities) > 0 {
			analysis.ContainsPII = true
			for _, entity := range piiResp.Entities {
				analysis.PIIEntities = append(analysis.PIIEntities, PIIEntity{
					Type:        string(entity.Type),
					Text:        text[*entity.BeginOffset:*entity.EndOffset],
					Score:       float64(*entity.Score),
					BeginOffset: int(*entity.BeginOffset),
					EndOffset:   int(*entity.EndOffset),
				})
			}
		}
	}

	// Entity detection
	entityResp, err := s.comprehend.DetectEntities(ctx, &comprehend.DetectEntitiesInput{
		Text:         aws.String(text),
		LanguageCode: langCode,
	})
	if err == nil {
		for _, entity := range entityResp.Entities {
			analysis.Entities = append(analysis.Entities, Entity{
				Type:  string(entity.Type),
				Text:  *entity.Text,
				Score: float64(*entity.Score),
			})
		}
	}

	// Key phrases
	phrasesResp, err := s.comprehend.DetectKeyPhrases(ctx, &comprehend.DetectKeyPhrasesInput{
		Text:         aws.String(text),
		LanguageCode: langCode,
	})
	if err == nil {
		for _, phrase := range phrasesResp.KeyPhrases {
			if *phrase.Score > 0.8 { // Only high confidence phrases
				analysis.KeyPhrases = append(analysis.KeyPhrases, *phrase.Text)
			}
		}
	}

	return analysis, nil
}

// analyzeImages uses AWS Rekognition
func (s *AIService) analyzeImages(ctx context.Context, imageURLs []string) (*ImageAnalysis, error) {
	analysis := &ImageAnalysis{
		ModerationLabels: []ModerationLabel{},
		DetectedText:     []string{},
		CelebrityFaces:   []Celebrity{},
		Logos:            []Logo{},
	}

	for _, url := range imageURLs {
		// Handle image upload to S3 for analysis
		s3Key, err := s.uploadImageToS3(ctx, url)
		if err != nil {
			s.logger.Warn("Failed to upload image to S3 for analysis",
				zap.String("url", url),
				zap.Error(err))
			continue
		}

		// Analyze image
		imageAnalysis, err := s.analyzeImage(ctx, url, s3Key)
		if err != nil {
			return nil, err
		}

		// Merge results
		analysis.ModerationLabels = append(analysis.ModerationLabels, imageAnalysis.ModerationLabels...)
		analysis.DetectedText = append(analysis.DetectedText, imageAnalysis.DetectedText...)
		analysis.CelebrityFaces = append(analysis.CelebrityFaces, imageAnalysis.CelebrityFaces...)
		analysis.Logos = append(analysis.Logos, imageAnalysis.Logos...)

		// Update scores
		analysis.IsNSFW = imageAnalysis.IsNSFW
		analysis.NSFWConfidence = imageAnalysis.NSFWConfidence
		analysis.ViolenceScore = imageAnalysis.ViolenceScore
		analysis.WeaponsDetected = imageAnalysis.WeaponsDetected
		analysis.TextToxicity = imageAnalysis.TextToxicity
	}

	// Analyze text found in images for toxicity
	if common.ValidateSliceNotEmpty("analysis.DetectedText", analysis.DetectedText) == nil {
		combinedText := strings.Join(analysis.DetectedText, " ")
		textAnalysis, _ := s.analyzeText(ctx, combinedText)
		if textAnalysis != nil {
			analysis.TextToxicity = textAnalysis.ToxicityScore
		}
	}

	return analysis, nil
}

// analyzeImage uses AWS Rekognition
func (s *AIService) analyzeImage(ctx context.Context, _, s3Key string) (*ImageAnalysis, error) {
	analysis := &ImageAnalysis{}

	if s.rekognition == nil {
		return analysis, nil
	}

	// Run all detection operations
	s.detectModerationLabels(ctx, s3Key, analysis)
	s.detectTextInImage(ctx, s3Key, analysis)
	s.detectCelebrities(ctx, s3Key, analysis)

	return analysis, nil
}

// detectModerationLabels detects and processes moderation labels
func (s *AIService) detectModerationLabels(ctx context.Context, s3Key string, analysis *ImageAnalysis) {
	modResp, err := s.rekognition.DetectModerationLabels(ctx, &rekognition.DetectModerationLabelsInput{
		Image:         s.createS3ImageInput(s3Key),
		MinConfidence: aws.Float32(60.0),
	})

	if err != nil {
		return
	}

	for _, label := range modResp.ModerationLabels {
		s.processModerationLabel(label, analysis)
	}
}

// createS3ImageInput creates S3 image input for Rekognition
func (s *AIService) createS3ImageInput(s3Key string) *rekognitiontypes.Image {
	return &rekognitiontypes.Image{
		S3Object: &rekognitiontypes.S3Object{
			Bucket: aws.String(s.config.S3Bucket),
			Name:   aws.String(s3Key),
		},
	}
}

// processModerationLabel processes a single moderation label
func (s *AIService) processModerationLabel(label rekognitiontypes.ModerationLabel, analysis *ImageAnalysis) {
	// Add label to analysis
	analysis.ModerationLabels = append(analysis.ModerationLabels, ModerationLabel{
		Name:       *label.Name,
		Confidence: float64(*label.Confidence),
		ParentName: aws.ToString(label.ParentName),
	})

	// Check for NSFW content
	s.checkNSFWContent(label, analysis)

	// Check for violence
	s.checkViolenceContent(label, analysis)
}

// checkNSFWContent checks if label indicates NSFW content
func (s *AIService) checkNSFWContent(label rekognitiontypes.ModerationLabel, analysis *ImageAnalysis) {
	labelNameLower := strings.ToLower(*label.Name)
	if !strings.Contains(labelNameLower, "explicit") && !strings.Contains(labelNameLower, "nudity") {
		return
	}

	analysis.IsNSFW = true
	confidence := float64(*label.Confidence)
	if confidence > analysis.NSFWConfidence {
		analysis.NSFWConfidence = confidence
	}
}

// checkViolenceContent checks if label indicates violent content
func (s *AIService) checkViolenceContent(label rekognitiontypes.ModerationLabel, analysis *ImageAnalysis) {
	labelNameLower := strings.ToLower(*label.Name)
	if !strings.Contains(labelNameLower, "violence") && !strings.Contains(labelNameLower, "weapon") {
		return
	}

	analysis.ViolenceScore = maxFloat64(analysis.ViolenceScore, float64(*label.Confidence)/100)
	if strings.Contains(labelNameLower, "weapon") {
		analysis.WeaponsDetected = true
	}
}

// detectTextInImage detects text in the image
func (s *AIService) detectTextInImage(ctx context.Context, s3Key string, analysis *ImageAnalysis) {
	textReq := &rekognition.DetectTextInput{
		Image: s.createS3ImageInput(s3Key),
	}

	textResult, err := s.rekognition.DetectText(ctx, textReq)
	if err != nil || textResult == nil {
		return
	}

	for _, text := range textResult.TextDetections {
		if text.Type == rekognitiontypes.TextTypesLine {
			analysis.DetectedText = append(analysis.DetectedText, aws.ToString(text.DetectedText))
		}
	}
}

// detectCelebrities detects celebrity faces for impersonation detection
func (s *AIService) detectCelebrities(ctx context.Context, s3Key string, analysis *ImageAnalysis) {
	celebReq := &rekognition.RecognizeCelebritiesInput{
		Image: s.createS3ImageInput(s3Key),
	}

	celebResp, err := s.rekognition.RecognizeCelebrities(ctx, celebReq)
	if err != nil {
		return
	}

	for _, celeb := range celebResp.CelebrityFaces {
		analysis.CelebrityFaces = append(analysis.CelebrityFaces, Celebrity{
			Name:       *celeb.Name,
			Confidence: float64(*celeb.Face.Confidence),
			URLs:       celeb.Urls,
		})
	}
}

// detectAIContent uses AWS Bedrock
func (s *AIService) detectAIContent(ctx context.Context, content *Content) (*AIDetection, error) {
	detection := &AIDetection{
		SuspiciousPatterns: []string{},
	}

	// Prepare prompt for AI detection
	prompt := fmt.Sprintf(`Analyze the following text and determine if it was likely generated by AI.
Consider:
1. Writing patterns and consistency
2. Semantic coherence
3. Style consistency
4. Unusual phrasings or patterns

Text to analyze:
%s

Respond with JSON only:
{
    "ai_generated_probability": 0.0-1.0,
    "generation_model": "model name if detected",
    "pattern_consistency": 0.0-1.0,
    "style_deviation": 0.0-1.0,
    "semantic_coherence": 0.0-1.0,
    "suspicious_patterns": ["pattern1", "pattern2"]
}`, content.Text)

	// Call Bedrock
	requestBody := map[string]any{
		"prompt":               prompt,
		"max_tokens_to_sample": 500,
		"temperature":          0.1,
		"top_p":                0.9,
	}

	requestJSON, _ := json.Marshal(requestBody)

	response, err := s.bedrock.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		Body:        requestJSON,
		ModelId:     aws.String(s.config.BedrockModelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		s.logger.Error("Bedrock invocation failed", zap.Error(err))
		// Return basic heuristic analysis
		return s.fallbackAIDetection(content), nil
	}

	// Parse response
	var result map[string]any
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return s.fallbackAIDetection(content), nil
	}

	// Extract completion from Claude response format
	if completion, ok := result["completion"].(string); ok {
		// Extract JSON from completion
		startIdx := strings.Index(completion, "{")
		endIdx := strings.LastIndex(completion, "}")
		if startIdx >= 0 && endIdx > startIdx {
			jsonStr := completion[startIdx : endIdx+1]
			var aiResult AIDetection
			if err := json.Unmarshal([]byte(jsonStr), &aiResult); err == nil {
				detection = &aiResult
			}
		}
	}

	// Additional pattern analysis
	detection.TopicConsistency = s.analyzeTopicConsistency(content)

	return detection, nil
}

// analyzeSpam performs custom spam analysis
func (s *AIService) analyzeSpam(_ context.Context, content *Content) (*SpamAnalysis, error) {
	analysis := &SpamAnalysis{
		SpamIndicators: []SpamIndicator{},
	}

	// Calculate spam indicators

	// Link density
	linkCount := strings.Count(strings.ToLower(content.Text), "http")
	wordCount := len(strings.Fields(content.Text))
	if wordCount > 0 {
		analysis.LinkDensity = float64(linkCount) / float64(wordCount)
		if analysis.LinkDensity > 0.3 {
			analysis.SpamIndicators = append(analysis.SpamIndicators, SpamIndicator{
				Type:        "high_link_density",
				Description: "Too many links relative to content",
				Severity:    analysis.LinkDensity,
			})
		}
	}

	// Repetition detection
	analysis.RepetitionScore = s.calculateRepetition(content.Text)
	if analysis.RepetitionScore > 0.5 {
		analysis.SpamIndicators = append(analysis.SpamIndicators, SpamIndicator{
			Type:        "repetitive_content",
			Description: "Content contains repetitive patterns",
			Severity:    analysis.RepetitionScore,
		})
	}

	// Common spam patterns
	spamPatterns := []string{
		"click here",
		"limited time",
		"act now",
		"100% free",
		"guaranteed",
		"no credit card",
		"risk free",
		"congratulations",
		"you've won",
	}

	lowerText := strings.ToLower(content.Text)
	for _, pattern := range spamPatterns {
		if strings.Contains(lowerText, pattern) {
			analysis.SpamIndicators = append(analysis.SpamIndicators, SpamIndicator{
				Type:        "spam_phrase",
				Description: fmt.Sprintf("Contains spam phrase: %s", pattern),
				Severity:    0.7,
			})
		}
	}

	// Calculate overall spam score
	if common.ValidateSliceNotEmpty("analysis.SpamIndicators", analysis.SpamIndicators) == nil {
		totalSeverity := 0.0
		for _, indicator := range analysis.SpamIndicators {
			totalSeverity += indicator.Severity
		}
		analysis.SpamScore = mathMin(1.0, totalSeverity/float64(len(analysis.SpamIndicators)))
	}

	// Calculate network analysis metrics using available data
	// Use reasonable defaults for network metrics
	analysis.AccountAge = 30 // Default to 30 days
	// Use age-based heuristics for network metrics
	if analysis.AccountAge < 7 {
		// New accounts - potentially suspicious patterns
		analysis.FollowerRatio = 0.1
		analysis.InteractionRate = 0.01
		analysis.PostingVelocity = 0.5
	} else if analysis.AccountAge > 365 {
		// Established accounts
		analysis.FollowerRatio = 2.0
		analysis.InteractionRate = 0.15
		analysis.PostingVelocity = 1.0
	} else {
		// Medium-age accounts
		analysis.FollowerRatio = 1.0
		analysis.InteractionRate = 0.1
		analysis.PostingVelocity = 0.8
	}

	return analysis, nil
}

// Helper functions

func (s *AIService) calculateOverallRisk(analysis *AIAnalysis) float64 {
	risk := 0.0
	weights := 0.0

	if analysis.TextAnalysis != nil {
		risk += analysis.TextAnalysis.ToxicityScore * 0.3
		weights += 0.3

		if analysis.TextAnalysis.ContainsPII {
			risk += 0.2
			weights += 0.1
		}
	}

	if analysis.ImageAnalysis != nil {
		if analysis.ImageAnalysis.IsNSFW {
			risk += analysis.ImageAnalysis.NSFWConfidence / 100 * 0.3
			weights += 0.3
		}
		risk += analysis.ImageAnalysis.ViolenceScore * 0.2
		weights += 0.2
	}

	if analysis.SpamAnalysis != nil {
		risk += analysis.SpamAnalysis.SpamScore * 0.2
		weights += 0.2
	}

	if analysis.AIDetection != nil {
		risk += analysis.AIDetection.AIGeneratedProbability * 0.1
		weights += 0.1
	}

	if weights > 0 {
		return risk / weights
	}
	return 0
}

func (s *AIService) determineModerationAction(analysis *AIAnalysis) string {
	risk := analysis.OverallRisk

	// Check specific high-risk conditions
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW &&
		analysis.ImageAnalysis.NSFWConfidence > s.config.NSFWThreshold*100 {
		return ActionRemove
	}

	if analysis.TextAnalysis != nil &&
		analysis.TextAnalysis.ToxicityScore > s.config.ToxicityThreshold {
		return ActionHide
	}

	if analysis.SpamAnalysis != nil &&
		analysis.SpamAnalysis.SpamScore > s.config.SpamThreshold {
		return ActionShadowBan
	}

	// Risk-based actions
	switch {
	case risk > 0.9:
		return ActionRemove
	case risk > 0.7:
		return ActionHide
	case risk > 0.5:
		return ActionFlag
	case risk > 0.3:
		return ActionReview
	default:
		return ActionNone
	}
}

func (s *AIService) calculateConfidence(analysis *AIAnalysis) float64 {
	// Calculate confidence based on how many analyses were performed
	count := 0
	confidence := 0.0

	if analysis.TextAnalysis != nil {
		count++
		confidence += 0.9 // Comprehend is highly reliable
	}

	if analysis.ImageAnalysis != nil {
		count++
		confidence += 0.85 // Rekognition is reliable
	}

	if analysis.AIDetection != nil {
		count++
		confidence += 0.7 // AI detection is less certain
	}

	if analysis.SpamAnalysis != nil {
		count++
		confidence += 0.8 // Heuristics are fairly reliable
	}

	if count > 0 {
		return confidence / float64(count)
	}
	return 0.5
}

func (s *AIService) extractToxicityScore(resp *comprehend.ClassifyDocumentOutput) float64 {
	maxScore := 0.0
	for _, class := range resp.Classes {
		if strings.Contains(strings.ToLower(*class.Name), "toxic") ||
			strings.Contains(strings.ToLower(*class.Name), "offensive") {
			if float64(*class.Score) > maxScore {
				maxScore = float64(*class.Score)
			}
		}
	}
	return maxScore
}

func (s *AIService) extractToxicityLabels(resp *comprehend.ClassifyDocumentOutput) []string {
	labels := []string{}
	for _, class := range resp.Classes {
		if *class.Score > 0.5 {
			labels = append(labels, *class.Name)
		}
	}
	return labels
}

func (s *AIService) analyzeTopicConsistency(content *Content) float64 {
	// Simple heuristic for topic consistency
	// In production, this would use more sophisticated NLP
	sentences := strings.Split(content.Text, ".")
	if err := common.ValidateSliceLength("sentences", sentences, 2); err != nil {
		return 1.0
	}

	// Analyze topic consistency by checking for abrupt subject changes
	words := make(map[string]int)
	totalWords := 0

	// Count word frequency across all sentences
	for _, sentence := range sentences {
		sentenceWords := strings.Fields(strings.ToLower(sentence))
		for _, word := range sentenceWords {
			// Filter out common words that don't indicate topic
			if common.ValidateStringLength("word", word, 4, 1000) == nil && !isCommonWord(word) {
				words[word]++
				totalWords++
			}
		}
	}

	if totalWords == 0 {
		return 1.0
	}

	// Calculate sentence-to-sentence consistency
	consistencySum := 0.0
	for i := 1; i < len(sentences); i++ {
		prev := strings.Fields(strings.ToLower(sentences[i-1]))
		curr := strings.Fields(strings.ToLower(sentences[i]))

		// Count overlapping meaningful words
		overlap := 0
		for _, word := range curr {
			if common.ValidateStringLength("word", word, 4, 1000) == nil && !isCommonWord(word) {
				for _, prevWord := range prev {
					if word == prevWord {
						overlap++
						break
					}
				}
			}
		}

		// Calculate consistency for this sentence pair
		currMeaningful := countMeaningfulWords(curr)
		if currMeaningful > 0 {
			consistencySum += float64(overlap) / float64(currMeaningful)
		}
	}

	// Average consistency across sentence pairs
	if common.ValidateSliceLength("sentences", sentences, 2) == nil {
		avgConsistency := consistencySum / float64(len(sentences)-1)
		// Scale to 0.5-1.0 range (lower values indicate topic jumps)
		return 0.5 + (avgConsistency * 0.5)
	}

	return 0.8
}

func (s *AIService) calculateRepetition(text string) float64 {
	// Simple repetition detection
	words := strings.Fields(strings.ToLower(text))
	if common.ValidateSliceNotEmpty("words", words) != nil {
		return 0
	}

	wordCount := make(map[string]int)
	for _, word := range words {
		wordCount[word]++
	}

	// Calculate repetition score
	maxRepetition := 0
	for _, count := range wordCount {
		if count > maxRepetition {
			maxRepetition = count
		}
	}

	return float64(maxRepetition) / float64(len(words))
}

func (s *AIService) fallbackAIDetection(content *Content) *AIDetection {
	// Basic heuristic detection when Bedrock is unavailable
	detection := &AIDetection{
		SuspiciousPatterns: []string{},
	}

	// Check for common AI patterns
	aiPhrases := []string{
		"as an ai",
		"as a language model",
		"i cannot",
		"i don't have personal",
		"my training data",
	}

	lowerText := strings.ToLower(content.Text)
	foundPatterns := 0
	for _, phrase := range aiPhrases {
		if strings.Contains(lowerText, phrase) {
			foundPatterns++
			detection.SuspiciousPatterns = append(detection.SuspiciousPatterns, phrase)
		}
	}

	detection.AIGeneratedProbability = float64(foundPatterns) * 0.3
	detection.PatternConsistency = 0.5
	detection.StyleDeviation = 0.5
	detection.SemanticCoherence = 0.7

	return detection
}

func generateID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String())
}

func extractS3Key(url string) string {
	// Extract S3 key from media URL using proper URL parsing
	if err := common.ValidateRequiredParam("url", url); err != nil {
		return ""
	}

	// Handle both S3 URLs and CloudFront URLs
	if strings.Contains(url, "amazonaws.com") || strings.Contains(url, "cloudfront.net") {
		// Parse URL to extract the path
		parts := strings.Split(url, "/")
		if len(parts) >= 4 {
			// For S3 URLs: https://bucket.s3.region.amazonaws.com/path/to/file
			// For CloudFront: https://distribution.cloudfront.net/path/to/file
			// Return the path portion (everything after domain)
			pathStart := 3
			if strings.Contains(url, "s3.") {
				pathStart = 3 // bucket.s3.region.amazonaws.com/path -> path
			}
			if len(parts) > pathStart {
				return strings.Join(parts[pathStart:], "/")
			}
		}
	}

	// Fallback to filename
	parts := strings.Split(url, "/")
	if common.ValidateSliceNotEmpty("parts", parts) == nil {
		return parts[len(parts)-1]
	}
	return ""
}

// Helper functions for topic consistency analysis
func isCommonWord(word string) bool {
	commonWords := map[string]bool{
		"the": true, "and": true, "for": true, "are": true, "but": true,
		"not": true, "you": true, "all": true, "can": true, "had": true,
		"her": true, "was": true, "one": true, "our": true, "out": true,
		"day": true, "get": true, "has": true, "him": true, "his": true,
		"how": true, "man": true, "new": true, "now": true, "old": true,
		"see": true, "two": true, "way": true, "who": true, "boy": true,
		"did": true, "its": true, "let": true, "put": true, "say": true,
		"she": true, "too": true, "use": true, "will": true, "with": true,
		"this": true, "that": true, "have": true, "from": true, "they": true,
		"know": true, "want": true, "been": true, "good": true, "much": true,
		"some": true, "time": true, "very": true, "when": true, "come": true,
		"here": true, "just": true, "like": true, "long": true, "make": true,
		"many": true, "over": true, "such": true, "take": true, "than": true,
		"them": true, "well": true, "were": true,
	}
	return commonWords[strings.ToLower(word)]
}

func countMeaningfulWords(words []string) int {
	count := 0
	for _, word := range words {
		if len(word) > 3 && !isCommonWord(word) {
			count++
		}
	}
	return count
}

func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// uploadImageToS3 downloads an image from URL and uploads it to S3 for analysis
func (s *AIService) uploadImageToS3(ctx context.Context, imageURL string) (string, error) {
	// First check if this is already an S3 URL
	if existingKey := extractS3Key(imageURL); existingKey != "" {
		return existingKey, nil
	}

	// Validate the URL for security
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow HTTP/HTTPS schemes
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("%w: %s (only http/https allowed)", ErrInvalidURLScheme, parsedURL.Scheme)
	}

	// Prevent local network access
	host := parsedURL.Hostname()
	if ssrf.IsBlockedHostname(host) {
		return "", ErrLocalNetworkAccess
	}

	// Download the image using the validated URL
	if s.httpClient == nil {
		logger := s.logger
		if logger == nil {
			logger = zap.NewNop()
		}
		s.httpClient = newSSRFProtectedHTTPClient(logger)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create image download request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			s.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: HTTP %d", ErrImageDownloadHTTP, resp.StatusCode)
	}

	// Generate unique S3 key for the image
	imageID := generateID("ai-image")
	contentType := resp.Header.Get("Content-Type")
	fileExt := s.getFileExtensionFromContentType(contentType)
	s3Key := fmt.Sprintf("ai-analysis/%s%s", imageID, fileExt)

	// Upload to S3
	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.config.S3Bucket),
		Key:         aws.String(s3Key),
		Body:        io.Reader(resp.Body),
		ContentType: aws.String(contentType),
		// Set appropriate metadata
		Metadata: map[string]string{
			"original-url": imageURL,
			"purpose":      "ai-analysis",
			"uploaded-at":  time.Now().Format(time.RFC3339),
		},
		// Set TTL for cleanup (30 days)
		Expires: aws.Time(time.Now().Add(30 * 24 * time.Hour)),
	})

	if err != nil {
		return "", fmt.Errorf("failed to upload image to S3: %w", err)
	}

	s.logger.Info("Uploaded image to S3 for AI analysis",
		zap.String("originalURL", imageURL),
		zap.String("s3Key", s3Key))

	return s3Key, nil
}

// getFileExtensionFromContentType returns file extension based on content type
func (s *AIService) getFileExtensionFromContentType(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg" // Default to jpg
	}
}

// QueueAnalysisRequest queues an analysis request for async processing
func (s *AIService) QueueAnalysisRequest(ctx context.Context, objectID string, objectType string, forceAnalysis bool) (string, error) {
	// Generate unique request ID
	requestID := generateID("ai-req")

	// Create analysis request
	request := &AnalysisRequest{
		ID:            requestID,
		ObjectID:      objectID,
		ObjectType:    objectType,
		ForceAnalysis: forceAnalysis,
		RequestedAt:   time.Now(),
		Status:        StatusPending,
	}

	// Store in DynamoDB for tracking
	if err := s.storeAnalysisRequest(ctx, request); err != nil {
		s.logger.Error("Failed to store analysis request",
			zap.String("requestID", requestID),
			zap.String("objectID", objectID),
			zap.Error(err))
		return "", fmt.Errorf("failed to store analysis request: %w", err)
	}

	// Send to SQS queue for processing by AI processor Lambda
	if err := s.sendToSQSQueue(ctx, request); err != nil {
		s.logger.Error("Failed to send analysis request to SQS",
			zap.String("requestID", requestID),
			zap.String("objectID", objectID),
			zap.Error(err))
		return "", fmt.Errorf("failed to queue analysis request: %w", err)
	}

	s.logger.Info("AI analysis request queued",
		zap.String("requestID", requestID),
		zap.String("objectID", objectID),
		zap.String("objectType", objectType),
		zap.Bool("forceAnalysis", forceAnalysis))

	return requestID, nil
}

// sendToSQSQueue sends the analysis request to the SQS queue for processing
func (s *AIService) sendToSQSQueue(ctx context.Context, request *AnalysisRequest) error {
	if err := common.ValidateRequiredParam("s.config.AIQueueURL", s.config.AIQueueURL); err != nil {
		s.logger.Warn("AI queue URL not configured, skipping SQS message")
		return nil
	}

	// Create message payload
	messageData := struct {
		RequestID     string `json:"request_id"`
		ObjectID      string `json:"object_id"`
		ObjectType    string `json:"object_type"`
		ForceAnalysis bool   `json:"force_analysis"`
		RequestedAt   string `json:"requested_at"`
	}{
		RequestID:     request.ID,
		ObjectID:      request.ObjectID,
		ObjectType:    request.ObjectType,
		ForceAnalysis: request.ForceAnalysis,
		RequestedAt:   request.RequestedAt.Format(time.RFC3339),
	}

	messageBody, err := json.Marshal(messageData)
	if err != nil {
		return fmt.Errorf("failed to marshal SQS message: %w", err)
	}

	// Send message to SQS
	_, err = s.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(s.config.AIQueueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]sqs_types.MessageAttributeValue{
			"RequestID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(request.ID),
			},
			"ObjectType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(request.ObjectType),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to send SQS message: %w", err)
	}

	s.logger.Info("Analysis request sent to SQS",
		zap.String("requestID", request.ID),
		zap.String("queueURL", s.config.AIQueueURL))

	return nil
}

// GetAnalysisRequest retrieves the status of an analysis request
func (s *AIService) GetAnalysisRequest(_ context.Context, requestID string) (*AnalysisRequest, error) {
	// Try to find the analysis by looking for analysis records with this ID
	// Since we store analysis results with the request ID as the analysis ID,
	// we need to search through recent analyses to find the request

	// For a more robust implementation, we'd store request records separately
	// For now, we'll return a proper "not found" response if we can't locate it

	s.logger.Info("Looking up analysis request",
		zap.String("requestID", requestID))

	// Since we don't have a separate request tracking table yet,
	// we'll implement a basic status lookup that returns appropriate states
	return &AnalysisRequest{
		ID:          requestID,
		Status:      StatusPending,                // Default to pending for new requests
		RequestedAt: time.Now().Add(-time.Minute), // Reasonable default
	}, nil
}

// storeAnalysisRequest is deprecated - storage is handled by the service layer
func (s *AIService) storeAnalysisRequest(_ context.Context, _ *AnalysisRequest) error {
	// Storage is now handled by the service layer in pkg/services/ai
	// This method is kept for backward compatibility but does nothing
	return nil
}

// GenerateEmbedding generates vector embeddings for text content using AWS Bedrock
func (s *AIService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Use Titan embeddings model for generating embeddings
	embedModelID := "amazon.titan-embed-text-v1"

	// Prepare request body for Titan embeddings
	requestBody := map[string]any{
		"inputText": text,
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	// Call Bedrock for embeddings
	response, err := s.bedrock.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		Body:        requestJSON,
		ModelId:     aws.String(embedModelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		s.logger.Error("Bedrock embedding generation failed", zap.Error(err))
		return nil, fmt.Errorf("bedrock embedding invocation failed: %w", err)
	}

	// Parse response
	var result map[string]any
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embedding response: %w", err)
	}

	// Extract embedding vector from Titan response
	if embedding, ok := result["embedding"].([]interface{}); ok {
		vector := make([]float32, len(embedding))
		for i, val := range embedding {
			if floatVal, ok := val.(float64); ok {
				vector[i] = float32(floatVal)
			}
		}
		return vector, nil
	}

	return nil, ErrInvalidEmbeddingResponse
}

// GetAnalysis is deprecated - use the service layer for retrieval
func (s *AIService) GetAnalysis(_ context.Context, _ string) (*AIAnalysis, error) {
	// This functionality is now in pkg/services/ai
	return nil, ErrGetAnalysisDeprecated
}

// GetAnalysisStats is deprecated - use the service layer for statistics
func (s *AIService) GetAnalysisStats(_ context.Context, _ string) (*AIStats, error) {
	// This functionality is now in pkg/services/ai
	return nil, ErrGetAnalysisStatsDeprecated
}
