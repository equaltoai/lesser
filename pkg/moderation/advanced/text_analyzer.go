package advanced

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// TextAnalyzer handles text content analysis using AWS Comprehend
type TextAnalyzer struct {
	client interface {
		DetectDominantLanguage(ctx context.Context, params *comprehend.DetectDominantLanguageInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectDominantLanguageOutput, error)
		DetectSentiment(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error)
		DetectPiiEntities(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error)
		DetectEntities(ctx context.Context, params *comprehend.DetectEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectEntitiesOutput, error)
		DetectKeyPhrases(ctx context.Context, params *comprehend.DetectKeyPhrasesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectKeyPhrasesOutput, error)
	}
	logger      *zap.Logger
	config      *ModerationConfig
	costTracker CostTracker

	// Cache for language detection
	languageCache sync.Map
	cacheTTL      time.Duration
}

// CostTracker interface for tracking AWS costs
type CostTracker interface {
	TrackComprehendRequest(operation string, units int)
	TrackTranscribeRequest(jobName string, estimatedMinutes int)
}

// NewTextAnalyzer creates a new text analyzer
func NewTextAnalyzer(client *comprehend.Client, logger *zap.Logger, config *ModerationConfig, costTracker CostTracker) *TextAnalyzer {
	if logger == nil {
		logger = zap.NewNop()
	}

	var clientInterface interface {
		DetectDominantLanguage(ctx context.Context, params *comprehend.DetectDominantLanguageInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectDominantLanguageOutput, error)
		DetectSentiment(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error)
		DetectPiiEntities(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error)
		DetectEntities(ctx context.Context, params *comprehend.DetectEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectEntitiesOutput, error)
		DetectKeyPhrases(ctx context.Context, params *comprehend.DetectKeyPhrasesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectKeyPhrasesOutput, error)
	}
	if client != nil {
		clientInterface = client
	}

	return &TextAnalyzer{
		client:      clientInterface,
		logger:      logger,
		config:      config,
		costTracker: costTracker,
		cacheTTL:    5 * time.Minute,
	}
}

// AnalyzeText performs comprehensive text analysis
func (ta *TextAnalyzer) AnalyzeText(ctx context.Context, text string, metadata ContentMetadata) (*ContentAnalysis, error) {
	startTime := time.Now()

	if err := common.ValidateRequiredParam("text", text); err != nil {
		return &ContentAnalysis{
			ContentID:      metadata.ContentID,
			AnalyzedAt:     time.Now(),
			ProcessingTime: time.Since(startTime),
		}, nil
	}

	// Truncate text if too long (Comprehend has limits)
	if len(text) > 5000 {
		text = text[:5000]
	}

	analysis := &ContentAnalysis{
		ContentID:  metadata.ContentID,
		AnalyzedAt: time.Now(),
	}

	// Detect language if not provided
	language := metadata.Language
	if err := common.ValidateRequiredParam("language", language); err != nil {
		detectedLang, err := ta.detectLanguage(ctx, text)
		if err != nil {
			ta.logger.Warn("language detection failed", zap.Error(err))
			language = "en" // Default to English
		} else {
			language = detectedLang
			analysis.Language = LanguageDetection{
				LanguageCode: language,
				Confidence:   0.99, // Comprehend is very accurate
			}
		}
	}

	// Run analyses in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Sentiment analysis
	wg.Add(1)
	go func() {
		defer wg.Done()
		sentiment, err := ta.analyzeSentiment(ctx, text, language)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("sentiment analysis: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		analysis.Sentiment = *sentiment
		mu.Unlock()
	}()

	// Toxicity detection
	wg.Add(1)
	go func() {
		defer wg.Done()
		toxicity, err := ta.detectToxicity(ctx, text, language)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("toxicity detection: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		analysis.Toxicity = *toxicity
		mu.Unlock()
	}()

	// PII detection
	if ta.config.EnableTextAnalysis {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pii, err := ta.detectPII(ctx, text, language)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("PII detection: %w", err))
				mu.Unlock()
				return
			}
			mu.Lock()
			analysis.PII = pii
			mu.Unlock()
		}()
	}

	// Entity recognition for topics
	wg.Add(1)
	go func() {
		defer wg.Done()
		topics, err := ta.extractTopics(ctx, text, language)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("topic extraction: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		analysis.Topics = topics
		mu.Unlock()
	}()

	// Threat detection
	wg.Add(1)
	go func() {
		defer wg.Done()
		threats := ta.detectThreats(text, metadata)
		mu.Lock()
		analysis.Threats = threats
		mu.Unlock()
	}()

	wg.Wait()

	// Check for critical errors
	if err := common.ValidateSliceNotEmpty("errors", errors); err == nil && len(errors) == 5 {
		// All analyses failed
		return nil, fmt.Errorf("all analyses failed: %v", errors)
	}

	// Log non-critical errors
	for _, err := range errors {
		ta.logger.Warn("analysis error", zap.Error(err))
	}

	analysis.ProcessingTime = time.Since(startTime)

	// Add custom flags based on analysis
	analysis.CustomFlags = ta.generateCustomFlags(analysis, metadata)

	return analysis, nil
}

// detectLanguage detects the dominant language
func (ta *TextAnalyzer) detectLanguage(ctx context.Context, text string) (string, error) {
	// Check cache
	cacheKey := fmt.Sprintf("lang:%x", hashText(text))
	if cached, ok := ta.languageCache.Load(cacheKey); ok {
		if lang, ok := cached.(*cachedLanguage); ok && time.Since(lang.cachedAt) < ta.cacheTTL {
			return lang.language, nil
		}
	}

	input := &comprehend.DetectDominantLanguageInput{
		Text: aws.String(text),
	}

	result, err := ta.client.DetectDominantLanguage(ctx, input)
	if err != nil {
		return "", fmt.Errorf("detect language: %w", err)
	}

	if ta.costTracker != nil {
		ta.costTracker.TrackComprehendRequest("DetectDominantLanguage", len(text))
	}

	if err := common.ValidateSliceNotEmpty("result.Languages", result.Languages); err != nil {
		return "en", nil // Default to English
	}

	// Find the highest scoring language
	var bestLang types.DominantLanguage
	for _, lang := range result.Languages {
		if bestLang.Score == nil || *lang.Score > *bestLang.Score {
			bestLang = lang
		}
	}

	language := aws.ToString(bestLang.LanguageCode)

	// Cache the result
	ta.languageCache.Store(cacheKey, &cachedLanguage{
		language: language,
		cachedAt: time.Now(),
	})

	return language, nil
}

// analyzeSentiment performs sentiment analysis
func (ta *TextAnalyzer) analyzeSentiment(ctx context.Context, text, language string) (*SentimentAnalysis, error) {
	input := &comprehend.DetectSentimentInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCode(language),
	}

	result, err := ta.client.DetectSentiment(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("detect sentiment: %w", err)
	}

	if ta.costTracker != nil {
		ta.costTracker.TrackComprehendRequest("DetectSentiment", len(text))
	}

	sentiment := &SentimentAnalysis{
		Sentiment: string(result.Sentiment),
	}

	if result.SentimentScore != nil {
		sentiment.Positive = float64(*result.SentimentScore.Positive)
		sentiment.Negative = float64(*result.SentimentScore.Negative)
		sentiment.Neutral = float64(*result.SentimentScore.Neutral)
		sentiment.Mixed = float64(*result.SentimentScore.Mixed)

		// Calculate confidence as the highest score
		sentiment.Confidence = maxFloat64(maxFloat64(sentiment.Positive, sentiment.Negative), maxFloat64(sentiment.Neutral, sentiment.Mixed))
	}

	return sentiment, nil
}

// detectToxicity uses various methods to detect toxic content
func (ta *TextAnalyzer) detectToxicity(ctx context.Context, text, language string) (*ToxicityAnalysis, error) {
	// AWS Comprehend doesn't have direct toxicity detection, so we use multiple signals

	toxicity := &ToxicityAnalysis{
		Categories: []ToxicCategory{},
	}

	// Use sentiment as a proxy
	sentiment, err := ta.analyzeSentiment(ctx, text, language)
	if err == nil && sentiment.Negative > 0.8 {
		toxicity.ToxicityScore = sentiment.Negative
	}

	// Check for key phrases that indicate toxicity
	keyPhrases, err := ta.detectKeyPhrases(ctx, text, language)
	if err == nil {
		toxicPhrases := ta.checkToxicPhrases(keyPhrases)
		if len(toxicPhrases) > 0 {
			toxicity.IsToxic = true
			toxicity.ToxicityScore = maxFloat64(toxicity.ToxicityScore, 0.7)

			for category, phrases := range toxicPhrases {
				toxicity.Categories = append(toxicity.Categories, ToxicCategory{
					Category:   category,
					Score:      float64(len(phrases)) / float64(len(keyPhrases)),
					Confidence: 0.8,
				})
			}
		}
	}

	// Check for targeted harassment
	entities, err := ta.detectEntities(ctx, text, language)
	if err == nil {
		targeted := ta.checkTargetedHarassment(text, entities)
		if err := common.ValidateSliceNotEmpty("targeted", targeted); err == nil {
			toxicity.TargetedGroups = targeted
			toxicity.IsToxic = true
			toxicity.ToxicityScore = maxFloat64(toxicity.ToxicityScore, 0.8)
		}
	}

	// Set overall confidence
	if toxicity.ToxicityScore > 0 {
		toxicity.Confidence = minFloat64(toxicity.ToxicityScore+0.1, 1.0)
	}

	return toxicity, nil
}

// detectPII detects personally identifiable information
func (ta *TextAnalyzer) detectPII(ctx context.Context, text, language string) ([]PIIEntity, error) {
	input := &comprehend.DetectPiiEntitiesInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCode(language),
	}

	result, err := ta.client.DetectPiiEntities(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("detect PII: %w", err)
	}

	if ta.costTracker != nil {
		ta.costTracker.TrackComprehendRequest("DetectPiiEntities", len(text))
	}

	piiEntities := make([]PIIEntity, 0, len(result.Entities))
	for _, entity := range result.Entities {
		// Extract the actual text
		beginOffset := int(*entity.BeginOffset)
		endOffset := int(*entity.EndOffset)
		entityText := ""
		if beginOffset < len(text) && endOffset <= len(text) {
			entityText = text[beginOffset:endOffset]
		}

		piiEntities = append(piiEntities, PIIEntity{
			Type:       string(entity.Type),
			Text:       entityText,
			BeginIndex: beginOffset,
			EndIndex:   endOffset,
			Confidence: float64(*entity.Score),
		})
	}

	return piiEntities, nil
}

// extractTopics extracts topics using entity recognition
func (ta *TextAnalyzer) extractTopics(ctx context.Context, text, language string) ([]Topic, error) {
	entities, err := ta.detectEntities(ctx, text, language)
	if err != nil {
		return nil, err
	}

	// Group entities by type to create topics
	topicMap := make(map[string]*Topic)

	for _, entity := range entities {
		entityType := string(entity.Type)
		key := fmt.Sprintf("%s:%s", entityType, *entity.Text)

		if topic, exists := topicMap[key]; exists {
			topic.Score += float64(*entity.Score)
		} else {
			topicMap[key] = &Topic{
				Name:     *entity.Text,
				Category: entityType,
				Score:    float64(*entity.Score),
			}
		}
	}

	// Convert map to slice
	topics := make([]Topic, 0, len(topicMap))
	for _, topic := range topicMap {
		topics = append(topics, *topic)
	}

	return topics, nil
}

// detectEntities detects named entities
func (ta *TextAnalyzer) detectEntities(ctx context.Context, text, language string) ([]types.Entity, error) {
	input := &comprehend.DetectEntitiesInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCode(language),
	}

	result, err := ta.client.DetectEntities(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("detect entities: %w", err)
	}

	if ta.costTracker != nil {
		ta.costTracker.TrackComprehendRequest("DetectEntities", len(text))
	}

	return result.Entities, nil
}

// detectKeyPhrases detects key phrases
func (ta *TextAnalyzer) detectKeyPhrases(ctx context.Context, text, language string) ([]types.KeyPhrase, error) {
	input := &comprehend.DetectKeyPhrasesInput{
		Text:         aws.String(text),
		LanguageCode: types.LanguageCode(language),
	}

	result, err := ta.client.DetectKeyPhrases(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("detect key phrases: %w", err)
	}

	if ta.costTracker != nil {
		ta.costTracker.TrackComprehendRequest("DetectKeyPhrases", len(text))
	}

	return result.KeyPhrases, nil
}

// detectThreats performs basic threat detection
func (ta *TextAnalyzer) detectThreats(text string, metadata ContentMetadata) []ThreatIndicator {
	threats := []ThreatIndicator{}
	lowerText := strings.ToLower(text)

	// Violence threats
	violenceKeywords := []string{"kill", "hurt", "attack", "bomb", "shoot", "stab", "murder"}
	violenceCount := 0
	violenceEvidence := []string{}

	for _, keyword := range violenceKeywords {
		if strings.Contains(lowerText, keyword) {
			violenceCount++
			violenceEvidence = append(violenceEvidence, keyword)
		}
	}

	if violenceCount > 0 {
		severity := SeverityLow
		if violenceCount > 2 {
			severity = SeverityMedium
		}
		if violenceCount > 4 {
			severity = SeverityHigh
		}

		threats = append(threats, ThreatIndicator{
			Type:        "VIOLENCE",
			Severity:    severity,
			Confidence:  minFloat64(float64(violenceCount)*0.2, 0.9),
			Evidence:    violenceEvidence,
			ActionItems: []string{"Review context", "Check user history"},
		})
	}

	// Self-harm detection
	selfHarmKeywords := []string{"suicide", "kill myself", "end it all", "self harm", "cut myself"}
	for _, keyword := range selfHarmKeywords {
		if strings.Contains(lowerText, keyword) {
			threats = append(threats, ThreatIndicator{
				Type:        "SELF_HARM",
				Severity:    SeverityCritical,
				Confidence:  0.9,
				Evidence:    []string{keyword},
				ActionItems: []string{"Immediate review", "Provide help resources", "Notify safety team"},
			})
			break
		}
	}

	// Doxxing detection
	if len(metadata.URLs) > 2 && containsPersonalInfo(text) {
		threats = append(threats, ThreatIndicator{
			Type:        "DOXXING",
			Severity:    SeverityHigh,
			Confidence:  0.7,
			Evidence:    []string{"Multiple URLs", "Personal information"},
			ActionItems: []string{"Review URLs", "Check for PII"},
		})
	}

	return threats
}

// checkToxicPhrases checks key phrases for toxic content
func (ta *TextAnalyzer) checkToxicPhrases(phrases []types.KeyPhrase) map[string][]string {
	toxicCategories := make(map[string][]string)

	// Simple keyword matching - in production, use a more sophisticated approach
	profanityList := []string{"fuck", "shit", "damn", "hell", "ass", "bitch"}
	hateTerms := []string{"hate", "despise", "loathe", "detest"}

	for _, phrase := range phrases {
		phraseText := strings.ToLower(*phrase.Text)

		for _, profanity := range profanityList {
			if strings.Contains(phraseText, profanity) {
				toxicCategories["PROFANITY"] = append(toxicCategories["PROFANITY"], *phrase.Text)
				break
			}
		}

		for _, hateTerm := range hateTerms {
			if strings.Contains(phraseText, hateTerm) {
				toxicCategories["HATE_SPEECH"] = append(toxicCategories["HATE_SPEECH"], *phrase.Text)
				break
			}
		}
	}

	return toxicCategories
}

// checkTargetedHarassment checks if entities are being targeted
func (ta *TextAnalyzer) checkTargetedHarassment(text string, entities []types.Entity) []string {
	targeted := []string{}
	lowerText := strings.ToLower(text)

	harassmentTerms := []string{"hate", "kill", "die", "stupid", "idiot", "moron"}

	for _, entity := range entities {
		if entity.Type == types.EntityTypePerson || entity.Type == types.EntityTypeOrganization {
			entityText := strings.ToLower(*entity.Text)

			// Check if harassment terms appear near the entity
			for _, term := range harassmentTerms {
				if strings.Contains(lowerText, entityText+" "+term) ||
					strings.Contains(lowerText, term+" "+entityText) {
					targeted = append(targeted, *entity.Text)
					break
				}
			}
		}
	}

	return targeted
}

// generateCustomFlags generates custom flags based on analysis
func (ta *TextAnalyzer) generateCustomFlags(analysis *ContentAnalysis, metadata ContentMetadata) []CustomFlag {
	flags := []CustomFlag{}

	// All caps detection
	if isAllCaps(metadata.ContentID) {
		flags = append(flags, CustomFlag{
			Name:       "ALL_CAPS",
			Value:      true,
			Confidence: 1.0,
		})
	}

	// Excessive punctuation
	punctCount := countExcessivePunctuation(metadata.ContentID)
	if punctCount > 5 {
		flags = append(flags, CustomFlag{
			Name:       "EXCESSIVE_PUNCTUATION",
			Value:      punctCount,
			Confidence: 1.0,
		})
	}

	// Spam patterns
	if len(metadata.URLs) > 3 {
		flags = append(flags, CustomFlag{
			Name:       "MANY_URLS",
			Value:      len(metadata.URLs),
			Confidence: 0.8,
		})
	}

	// Context-based flags
	if metadata.Context == "comment" && analysis.Sentiment.Negative > 0.8 {
		flags = append(flags, CustomFlag{
			Name:       "NEGATIVE_COMMENT",
			Value:      true,
			Confidence: analysis.Sentiment.Confidence,
		})
	}

	return flags
}

// Helper functions

type cachedLanguage struct {
	language string
	cachedAt time.Time
}

func hashText(text string) string {
	// Use SHA-256 to create a cryptographically secure hash of the text for caching
	hash := sha256.Sum256([]byte(text))
	// Return first 16 characters of hex-encoded hash for cache key efficiency
	hexHash := hex.EncodeToString(hash[:])
	if len(hexHash) > 16 {
		return hexHash[:16]
	}
	return hexHash
}

func isAllCaps(text string) bool {
	if len(text) < 10 {
		return false
	}

	upperCount := 0
	letterCount := 0

	for _, r := range text {
		if 'A' <= r && r <= 'Z' {
			upperCount++
			letterCount++
		} else if 'a' <= r && r <= 'z' {
			letterCount++
		}
	}

	if letterCount == 0 {
		return false
	}

	return float64(upperCount)/float64(letterCount) > 0.8
}

func countExcessivePunctuation(text string) int {
	count := 0
	for _, r := range text {
		if r == '!' || r == '?' {
			count++
		}
	}
	return count
}

func containsPersonalInfo(text string) bool {
	lowerText := strings.ToLower(text)
	personalKeywords := []string{"address", "phone", "email", "ssn", "social security", "credit card"}

	for _, keyword := range personalKeywords {
		if strings.Contains(lowerText, keyword) {
			return true
		}
	}

	return false
}
