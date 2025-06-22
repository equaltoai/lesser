package ai

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AIService struct {
	comprehend  *comprehend.Client
	rekognition *rekognition.Client
	bedrock     *bedrockruntime.Client
	s3Client    *s3.Client
	logger      *zap.Logger
	config      *AIConfig
}

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
}

func NewAIService(cfg aws.Config, aiConfig *AIConfig) *AIService {
	return &AIService{
		comprehend:  comprehend.NewFromConfig(cfg),
		rekognition: rekognition.NewFromConfig(cfg),
		bedrock:     bedrockruntime.NewFromConfig(cfg),
		s3Client:    s3.NewFromConfig(cfg),
		logger:      zap.L().Named("ai"),
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
	if len(content.MediaURLs) > 0 && s.config.EnableImageAnalysis {
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
		// For now, skip S3 upload and assume images are already in S3
		// In production, you'd download and upload to S3 first

		// Simulated S3 key (in production, extract from URL or upload first)
		s3Key := extractS3Key(url)
		if s3Key == "" {
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
	if len(analysis.DetectedText) > 0 {
		combinedText := strings.Join(analysis.DetectedText, " ")
		textAnalysis, _ := s.analyzeText(ctx, combinedText)
		if textAnalysis != nil {
			analysis.TextToxicity = textAnalysis.ToxicityScore
		}
	}

	return analysis, nil
}

// analyzeImage uses AWS Rekognition
func (s *AIService) analyzeImage(ctx context.Context, objectID, s3Key string) (*ImageAnalysis, error) {
	analysis := &ImageAnalysis{}

	// Analyze image with Rekognition
	if s.rekognition != nil {
		// Detect moderation labels
		modResp, err := s.rekognition.DetectModerationLabels(ctx, &rekognition.DetectModerationLabelsInput{
			Image: &rekognitiontypes.Image{
				S3Object: &rekognitiontypes.S3Object{
					Bucket: aws.String(s.config.S3Bucket),
					Name:   aws.String(s3Key),
				},
			},
			MinConfidence: aws.Float32(60.0),
		})
		if err == nil {
			for _, label := range modResp.ModerationLabels {
				analysis.ModerationLabels = append(analysis.ModerationLabels, ModerationLabel{
					Name:       *label.Name,
					Confidence: float64(*label.Confidence),
					ParentName: aws.ToString(label.ParentName),
				})

				// Check for NSFW
				if strings.Contains(strings.ToLower(*label.Name), "explicit") ||
					strings.Contains(strings.ToLower(*label.Name), "nudity") {
					analysis.IsNSFW = true
					if float64(*label.Confidence) > analysis.NSFWConfidence {
						analysis.NSFWConfidence = float64(*label.Confidence)
					}
				}

				// Check for violence
				if strings.Contains(strings.ToLower(*label.Name), "violence") ||
					strings.Contains(strings.ToLower(*label.Name), "weapon") {
					analysis.ViolenceScore = maxFloat64(analysis.ViolenceScore, float64(*label.Confidence)/100)
					if strings.Contains(strings.ToLower(*label.Name), "weapon") {
						analysis.WeaponsDetected = true
					}
				}
			}
		}

		// Detect text in images
		textReq := &rekognition.DetectTextInput{
			Image: &rekognitiontypes.Image{
				S3Object: &rekognitiontypes.S3Object{
					Bucket: aws.String(s.config.S3Bucket),
					Name:   aws.String(s3Key),
				},
			},
		}

		textResult, err := s.rekognition.DetectText(ctx, textReq)
		if err == nil && textResult != nil {
			for _, text := range textResult.TextDetections {
				if text.Type == rekognitiontypes.TextTypesLine {
					analysis.DetectedText = append(analysis.DetectedText, aws.ToString(text.DetectedText))
				}
			}
		}

		// Detect celebrities (for impersonation detection)
		celebReq := &rekognition.RecognizeCelebritiesInput{
			Image: &rekognitiontypes.Image{
				S3Object: &rekognitiontypes.S3Object{
					Bucket: aws.String(s.config.S3Bucket),
					Name:   aws.String(s3Key),
				},
			},
		}
		celebResp, err := s.rekognition.RecognizeCelebrities(ctx, celebReq)
		if err == nil {
			for _, celeb := range celebResp.CelebrityFaces {
				analysis.CelebrityFaces = append(analysis.CelebrityFaces, Celebrity{
					Name:       *celeb.Name,
					Confidence: float64(*celeb.Face.Confidence),
					URLs:       celeb.Urls,
				})
			}
		}
	}

	return analysis, nil
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
	requestBody := map[string]interface{}{
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
	var result map[string]interface{}
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
func (s *AIService) analyzeSpam(ctx context.Context, content *Content) (*SpamAnalysis, error) {
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
	if len(analysis.SpamIndicators) > 0 {
		totalSeverity := 0.0
		for _, indicator := range analysis.SpamIndicators {
			totalSeverity += indicator.Severity
		}
		analysis.SpamScore = min(1.0, totalSeverity/float64(len(analysis.SpamIndicators)))
	}

	// Network analysis would require additional data
	// For now, set placeholder values
	analysis.FollowerRatio = 0.5
	analysis.InteractionRate = 0.1
	analysis.AccountAge = 30
	analysis.PostingVelocity = 1.0

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
	if len(sentences) < 2 {
		return 1.0
	}

	// Check if topics shift dramatically between sentences
	// This is a placeholder implementation
	return 0.8
}

func (s *AIService) calculateRepetition(text string) float64 {
	// Simple repetition detection
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
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
	// Extract S3 key from media URL
	// This is a simplified implementation
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}


func min(a, b float64) float64 {
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
