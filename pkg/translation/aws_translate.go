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
	"github.com/aws/aws-sdk-go-v2/service/translate"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/theory-cloud/tabletheory/v3"
	theorydb "github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"go.uber.org/zap"
)

// Service provides translation functionality using AWS Translate
type Service struct {
	client translateAPI
	db     theorydb.DB

	logger       *zap.Logger
	cacheEnabled bool
	cacheTTL     time.Duration
}

type translateAPI interface {
	TranslateText(ctx context.Context, params *translate.TranslateTextInput, optFns ...func(*translate.Options)) (*translate.TranslateTextOutput, error)
	ListLanguages(ctx context.Context, params *translate.ListLanguagesInput, optFns ...func(*translate.Options)) (*translate.ListLanguagesOutput, error)
}

type translationCacheItem struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	TranslatedText   string    `theorydb:"attr:translatedText"`
	DetectedLanguage string    `theorydb:"attr:detectedLanguage"`
	CachedAt         time.Time `theorydb:"attr:cachedAt"`
	TTL              int64     `theorydb:"ttl,attr:ttl"`
}

func (translationCacheItem) TableName() string {
	return lesserconfig.GetMainTableName()
}

type supportedLanguagesCacheItem struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Languages []LanguageInfo `theorydb:"attr:languages"`
	CachedAt  time.Time      `theorydb:"attr:cachedAt"`
	TTL       int64          `theorydb:"ttl,attr:ttl"`
}

func (supportedLanguagesCacheItem) TableName() string {
	return lesserconfig.GetMainTableName()
}

// NewService creates a new translation service.
func NewService(ctx context.Context, cfg *lesserconfig.Config, store storagecore.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (*Service, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var db theorydb.DB
	if store != nil {
		db = store.GetDB()
	}

	if db == nil {
		if tabletheory.IsLambdaEnvironment() {
			lambdaDB, dbErr := tabletheory.NewLambdaOptimized()
			if dbErr != nil {
				return nil, fmt.Errorf("initialize TableTheory LambdaDB: %w", dbErr)
			}
			db = lambdaDB
		} else {
			region := cfg.Region
			if region == "" {
				region = "us-east-1"
			}
			standardDB, dbErr := tabletheory.New(session.Config{Region: region})
			if dbErr != nil {
				return nil, fmt.Errorf("initialize TableTheory DB: %w", dbErr)
			}
			db = standardDB
		}
	}

	return &Service{
		client:       translate.NewFromConfig(awsCfg),
		db:           db,
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
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("translation service database is not initialized")
	}

	item := &translationCacheItem{
		PK: fmt.Sprintf("TRANSLATION#%s", cacheKey),
		SK: "RESULT",
	}

	err := s.db.Model(item).
		WithContext(ctx).
		Where("PK", "=", item.PK).
		Where("SK", "=", item.SK).
		First(item)
	if err != nil {
		if theorydbErrors.IsNotFound(err) {
			return nil, nil // Cache miss
		}
		return nil, fmt.Errorf("failed to get cached translation: %w", err)
	}

	return &TranslationCache{
		TranslatedText:   item.TranslatedText,
		DetectedLanguage: item.DetectedLanguage,
		CachedAt:         item.CachedAt,
	}, nil
}

// cacheTranslation stores a translation in DynamoDB
func (s *Service) cacheTranslation(ctx context.Context, cacheKey, translatedText, detectedLang string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("translation service database is not initialized")
	}

	now := time.Now().UTC()

	item := &translationCacheItem{
		PK: fmt.Sprintf("TRANSLATION#%s", cacheKey),
		SK: "RESULT",

		TranslatedText:   translatedText,
		DetectedLanguage: detectedLang,
		CachedAt:         now,
		TTL:              now.Add(s.cacheTTL).Unix(),
	}

	if err := s.db.Model(item).WithContext(ctx).CreateOrUpdate(); err != nil {
		return fmt.Errorf("failed to cache translation: %w", err)
	}

	return nil
}

// getCachedLanguages retrieves cached language list from DynamoDB
func (s *Service) getCachedLanguages(ctx context.Context) ([]LanguageInfo, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("translation service database is not initialized")
	}

	item := &supportedLanguagesCacheItem{
		PK: "CACHE#LANGUAGES",
		SK: "SUPPORTED",
	}

	err := s.db.Model(item).
		WithContext(ctx).
		Where("PK", "=", item.PK).
		Where("SK", "=", item.SK).
		First(item)
	if err != nil {
		if theorydbErrors.IsNotFound(err) {
			return nil, nil // Cache miss
		}
		return nil, fmt.Errorf("failed to get cached languages: %w", err)
	}

	return item.Languages, nil
}

// cacheLanguages stores the language list in DynamoDB
func (s *Service) cacheLanguages(ctx context.Context, languages []LanguageInfo) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("translation service database is not initialized")
	}

	now := time.Now().UTC()

	item := &supportedLanguagesCacheItem{
		PK: "CACHE#LANGUAGES",
		SK: "SUPPORTED",

		Languages: languages,
		CachedAt:  now,
		TTL:       now.Add(24 * time.Hour).Unix(),
	}

	if err := s.db.Model(item).WithContext(ctx).CreateOrUpdate(); err != nil {
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
