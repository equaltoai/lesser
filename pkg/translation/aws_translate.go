// Package translation provides AWS Translate integration with caching for multilingual content support.
package translation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/translate"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// Service provides translation functionality using AWS Translate
type Service struct {
	client       translateAPI
	dynamoClient dynamodbAPI
	tableName    string
	store        core.RepositoryStorage
	logger       *zap.Logger
	cacheEnabled bool
	cacheTTL     time.Duration
}

type translateAPI interface {
	TranslateText(ctx context.Context, params *translate.TranslateTextInput, optFns ...func(*translate.Options)) (*translate.TranslateTextOutput, error)
	ListLanguages(ctx context.Context, params *translate.ListLanguagesInput, optFns ...func(*translate.Options)) (*translate.ListLanguagesOutput, error)
}

type dynamodbAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

// NewService creates a new translation service
func NewService(ctx context.Context, cfg *lesserconfig.Config, store core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (*Service, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Get table name from config
	tableName := cfg.DynamoTableName
	if tableName == "" {
		tableName = "lesser-main" // Default table name
	}

	return &Service{
		client:       translate.NewFromConfig(awsCfg),
		dynamoClient: dynamodb.NewFromConfig(awsCfg),
		tableName:    tableName,
		store:        store,
		logger:       logger,
		cacheEnabled: cacheEnabled,
		cacheTTL:     30 * 24 * time.Hour, // Cache translations for 30 days
	}, nil
}

// TranslateText translates text from source language to target language
func (s *Service) TranslateText(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	// Check cache first
	if s.cacheEnabled {
		cacheKey := s.generateCacheKey(text, sourceLang, targetLang)
		cached, err := s.getCachedTranslation(ctx, cacheKey)
		if err == nil && cached != nil {
			s.logger.Debug("translation cache hit",
				zap.String("cache_key", cacheKey),
				zap.String("target_lang", targetLang))
			return cached.TranslatedText, cached.DetectedLanguage, nil
		}
	}

	// Prepare translation request
	input := &translate.TranslateTextInput{
		Text:               aws.String(text),
		TargetLanguageCode: aws.String(targetLang),
	}

	// Auto-detect source language if not provided or set to "auto"
	if sourceLang != "" && sourceLang != "auto" {
		input.SourceLanguageCode = aws.String(sourceLang)
	}

	// Perform translation
	result, err := s.client.TranslateText(ctx, input)
	if err != nil {
		s.logger.Error("AWS Translate API error",
			zap.String("source_lang", sourceLang),
			zap.String("target_lang", targetLang),
			zap.Error(err))
		return "", "", fmt.Errorf("translation failed: %w", err)
	}

	translatedText := aws.ToString(result.TranslatedText)
	detectedLang := aws.ToString(result.SourceLanguageCode)

	// Cache the result
	if s.cacheEnabled {
		cacheKey := s.generateCacheKey(text, sourceLang, targetLang)
		if err := s.cacheTranslation(ctx, cacheKey, translatedText, detectedLang); err != nil {
			s.logger.Warn("failed to cache translation",
				zap.String("cache_key", cacheKey),
				zap.Error(err))
			// Don't fail the request if caching fails
		}
	}

	return translatedText, detectedLang, nil
}

// GetSupportedLanguages returns the list of supported language pairs
func (s *Service) GetSupportedLanguages(ctx context.Context) ([]LanguageInfo, error) {
	// Check cache for language list
	if s.cacheEnabled {
		cached, err := s.getCachedLanguages(ctx)
		if err == nil && cached != nil {
			return cached, nil
		}
	}

	// Get language list from AWS Translate
	result, err := s.client.ListLanguages(ctx, &translate.ListLanguagesInput{
		MaxResults: aws.Int32(100),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list languages: %w", err)
	}

	languages := make([]LanguageInfo, 0, len(result.Languages))
	for _, lang := range result.Languages {
		languages = append(languages, LanguageInfo{
			Code: aws.ToString(lang.LanguageCode),
			Name: aws.ToString(lang.LanguageName),
		})
	}

	// Cache the language list
	if s.cacheEnabled {
		if err := s.cacheLanguages(ctx, languages); err != nil {
			s.logger.Warn("failed to cache language list", zap.Error(err))
		}
	}

	return languages, nil
}

// DetectLanguage detects the language of the given text
func (s *Service) DetectLanguage(ctx context.Context, text string) (string, float32, error) {
	// AWS Translate doesn't have a separate detect API, it auto-detects during translation
	// We can use a dummy translation to detect language
	input := &translate.TranslateTextInput{
		Text:               aws.String(text),
		TargetLanguageCode: aws.String("en"), // Translate to English to detect source
	}

	result, err := s.client.TranslateText(ctx, input)
	if err != nil {
		return "", 0, fmt.Errorf("language detection failed: %w", err)
	}

	detectedLang := aws.ToString(result.SourceLanguageCode)
	// AWS Translate doesn't provide confidence scores, so we return 1.0
	return detectedLang, 1.0, nil
}

// TranslateHTML translates HTML content while preserving formatting
func (s *Service) TranslateHTML(ctx context.Context, html, sourceLang, targetLang string) (string, string, error) {
	// Strip HTML tags for translation
	textOnly := stripHTMLTags(html)

	// Translate the text
	translatedText, detectedLang, err := s.TranslateText(ctx, textOnly, sourceLang, targetLang)
	if err != nil {
		return "", "", err
	}

	// For now, return translated text without HTML preservation
	// In production, you'd want to use a proper HTML parser to preserve structure
	return translatedText, detectedLang, nil
}

// Helper types and functions

// TranslationCache provides caching for translations
//
//nolint:revive // Translation prefix clarifies this is translation-specific cache
type TranslationCache struct {
	TranslatedText   string
	DetectedLanguage string
	CachedAt         time.Time
}

// LanguageInfo contains information about a language
type LanguageInfo struct {
	Code string
	Name string
}

// generateCacheKey creates a consistent cache key for translations
func (s *Service) generateCacheKey(text, sourceLang, targetLang string) string {
	// Use SHA256 hash of text to keep key size reasonable
	h := sha256.New()
	h.Write([]byte(text))
	textHash := hex.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("translation:%s:%s:%s", textHash, sourceLang, targetLang)
}

// getCachedTranslation retrieves a cached translation from DynamoDB
func (s *Service) getCachedTranslation(ctx context.Context, cacheKey string) (*TranslationCache, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRANSLATION#%s", cacheKey)},
			"SK": &types.AttributeValueMemberS{Value: "RESULT"},
		},
	}

	result, err := s.dynamoClient.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached translation: %w", err)
	}

	if result.Item == nil {
		return nil, nil // Cache miss
	}

	// Parse the cached result
	var cache TranslationCache
	if translatedText, ok := result.Item["TranslatedText"]; ok {
		if textVal, ok := translatedText.(*types.AttributeValueMemberS); ok {
			cache.TranslatedText = textVal.Value
		}
	}
	if detectedLang, ok := result.Item["DetectedLanguage"]; ok {
		if langVal, ok := detectedLang.(*types.AttributeValueMemberS); ok {
			cache.DetectedLanguage = langVal.Value
		}
	}
	if cachedAt, ok := result.Item["CachedAt"]; ok {
		switch timeVal := cachedAt.(type) {
		case *types.AttributeValueMemberS:
			if timestamp, err := time.Parse(time.RFC3339, timeVal.Value); err == nil {
				cache.CachedAt = timestamp
			}
		case *types.AttributeValueMemberN:
			if timestamp, err := time.Parse(time.RFC3339, timeVal.Value); err == nil {
				cache.CachedAt = timestamp
			}
		}
	}

	return &cache, nil
}

// cacheTranslation stores a translation in DynamoDB
func (s *Service) cacheTranslation(ctx context.Context, cacheKey, translatedText, detectedLang string) error {
	ttl := time.Now().Add(s.cacheTTL).Unix()
	cachedAt := time.Now().Format(time.RFC3339)

	item := map[string]types.AttributeValue{
		"PK":               &types.AttributeValueMemberS{Value: fmt.Sprintf("TRANSLATION#%s", cacheKey)},
		"SK":               &types.AttributeValueMemberS{Value: "RESULT"},
		"TranslatedText":   &types.AttributeValueMemberS{Value: translatedText},
		"DetectedLanguage": &types.AttributeValueMemberS{Value: detectedLang},
		"CachedAt":         &types.AttributeValueMemberS{Value: cachedAt},
		"TTL":              &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", ttl)},
	}

	_, err := s.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to cache translation: %w", err)
	}

	return nil
}

// getCachedLanguages retrieves cached language list from DynamoDB
func (s *Service) getCachedLanguages(ctx context.Context) ([]LanguageInfo, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CACHE#LANGUAGES"},
			"SK": &types.AttributeValueMemberS{Value: "SUPPORTED"},
		},
	}

	result, err := s.dynamoClient.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached languages: %w", err)
	}

	if result.Item == nil {
		return nil, nil // Cache miss
	}

	return s.parseLanguagesFromCache(result.Item)
}

// parseLanguagesFromCache parses the language list from DynamoDB cache item
func (s *Service) parseLanguagesFromCache(item map[string]types.AttributeValue) ([]LanguageInfo, error) {
	languageList, ok := item["Languages"]
	if !ok {
		return nil, nil
	}

	listVal, ok := languageList.(*types.AttributeValueMemberL)
	if !ok {
		return nil, nil
	}

	var languages []LanguageInfo
	for _, langItem := range listVal.Value {
		if lang := s.parseLanguageItem(langItem); lang != nil {
			languages = append(languages, *lang)
		}
	}

	return languages, nil
}

// parseLanguageItem parses a single language item from DynamoDB attribute value
func (s *Service) parseLanguageItem(langItem types.AttributeValue) *LanguageInfo {
	langMap, ok := langItem.(*types.AttributeValueMemberM)
	if !ok {
		return nil
	}

	lang := &LanguageInfo{}

	// Extract language code
	if code := s.extractStringValue(langMap.Value, "Code"); code != "" {
		lang.Code = code
	}

	// Extract language name
	if name := s.extractStringValue(langMap.Value, "Name"); name != "" {
		lang.Name = name
	}

	// Return nil if both code and name are empty
	if lang.Code == "" && lang.Name == "" {
		return nil
	}

	return lang
}

// extractStringValue extracts a string value from a DynamoDB attribute map
func (s *Service) extractStringValue(attrMap map[string]types.AttributeValue, key string) string {
	attr, ok := attrMap[key]
	if !ok {
		return ""
	}

	strVal, ok := attr.(*types.AttributeValueMemberS)
	if !ok {
		return ""
	}

	return strVal.Value
}

// cacheLanguages stores the language list in DynamoDB
func (s *Service) cacheLanguages(ctx context.Context, languages []LanguageInfo) error {
	ttl := time.Now().Add(24 * time.Hour).Unix() // 24 hour TTL for language list

	// Convert languages to DynamoDB format
	languageList := make([]types.AttributeValue, 0, len(languages))
	for _, lang := range languages {
		langMap := map[string]types.AttributeValue{
			"Code": &types.AttributeValueMemberS{Value: lang.Code},
			"Name": &types.AttributeValueMemberS{Value: lang.Name},
		}
		languageList = append(languageList, &types.AttributeValueMemberM{Value: langMap})
	}

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: "CACHE#LANGUAGES"},
		"SK":        &types.AttributeValueMemberS{Value: "SUPPORTED"},
		"Languages": &types.AttributeValueMemberL{Value: languageList},
		"CachedAt":  &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", ttl)},
	}

	_, err := s.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to cache languages: %w", err)
	}

	return nil
}

// stripHTMLTags removes HTML tags from text (basic implementation)
func stripHTMLTags(html string) string {
	// This is a very basic implementation
	// In production, use a proper HTML parser
	text := html
	text = strings.ReplaceAll(text, "<br>", " ")
	text = strings.ReplaceAll(text, "<br/>", " ")
	text = strings.ReplaceAll(text, "<br />", " ")
	text = strings.ReplaceAll(text, "</p>", " ")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	// Remove remaining tags
	for strings.Contains(text, "<") && strings.Contains(text, ">") {
		start := strings.Index(text, "<")
		end := strings.Index(text, ">")
		if start != -1 && end != -1 && end > start {
			text = text[:start] + text[end+1:]
		} else {
			break
		}
	}

	return strings.TrimSpace(text)
}
