package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/translation"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// TranslationResult represents the result of translating a status
type TranslationResult struct {
	Content          string `json:"content"`
	SpoilerText      string `json:"spoiler_text,omitempty"`
	DetectedLanguage string `json:"detected_source_language"`
	Provider         string `json:"provider"`
}

// TranslationLanguage represents a supported translation language
type TranslationLanguage struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// HandleTranslateStatus translates a status to the user's preferred language
func (h *Handler) HandleTranslateStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the status
	obj, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		h.logger.Error("failed to get status for translation",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Extract content from the object
	var content string
	var spoilerText string
	var language string

	switch o := obj.(type) {
	case *activitypub.Note:
		content = o.Content
		spoilerText = o.Summary
		// Note: Language is not part of the standard Note type
		// It would need to be stored separately or as object metadata
	case map[string]interface{}:
		if c, ok := o["content"].(string); ok {
			content = c
		}
		if s, ok := o["summary"].(string); ok {
			spoilerText = s
		}
		if l, ok := o["language"].(string); ok {
			language = l
		}
	}

	// Check if translation is needed
	if content == "" {
		return common.UnprocessableEntity(fmt.Errorf("status has no content to translate")), nil
	}

	// Get user's preferred language
	userPrefs, err := h.store.GetUserPreferences(ctx, claims.Username)
	targetLang := "en" // Default to English
	if err == nil && userPrefs != nil && userPrefs.Language != "" {
		targetLang = userPrefs.Language
	}

	// Check if translation is enabled via environment variable
	translationEnabled := os.Getenv("TRANSLATION_ENABLED") == "true"

	if !translationEnabled {
		// Return mock translation if not enabled
		translation := TranslationResult{
			Content:          fmt.Sprintf("[Mock translation of: %s]", content),
			SpoilerText:      spoilerText,
			DetectedLanguage: language,
			Provider:         "mock",
		}

		body, _ := json.Marshal(translation)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(body),
		}, nil
	}

	// Initialize translation service
	translationSvc, err := translation.NewService(ctx, h.store, h.logger, true)
	if err != nil {
		h.logger.Error("failed to initialize translation service", zap.Error(err))
		// Return basic list on error
		languages := []TranslationLanguage{
			{Code: "en", Name: "English"},
			{Code: "es", Name: "Spanish"},
			{Code: "fr", Name: "French"},
		}
		body, _ := json.Marshal(languages)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(body),
		}, nil
	}

	// Translate the content
	translatedContent, detectedLang, err := translationSvc.TranslateHTML(ctx, content, language, targetLang)
	if err != nil {
		h.logger.Error("failed to translate content",
			zap.String("status_id", statusID),
			zap.String("target_lang", targetLang),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("translation failed")), nil
	}

	// Translate spoiler text if present
	translatedSpoiler := spoilerText
	if spoilerText != "" {
		translated, _, err := translationSvc.TranslateText(ctx, spoilerText, language, targetLang)
		if err == nil {
			translatedSpoiler = translated
		}
	}

	// Build response
	translation := TranslationResult{
		Content:          translatedContent,
		SpoilerText:      translatedSpoiler,
		DetectedLanguage: detectedLang,
		Provider:         "AWS Translate",
	}

	body, _ := json.Marshal(translation)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetTranslationLanguages returns the list of supported translation languages
func (h *Handler) HandleGetTranslationLanguages(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check if translation is enabled
	translationEnabled := os.Getenv("TRANSLATION_ENABLED") == "true"

	if !translationEnabled {
		// Return mock languages if not enabled
		languages := []TranslationLanguage{
			{Code: "en", Name: "English"},
			{Code: "es", Name: "Spanish"},
			{Code: "fr", Name: "French"},
			{Code: "de", Name: "German"},
			{Code: "it", Name: "Italian"},
			{Code: "pt", Name: "Portuguese"},
			{Code: "ja", Name: "Japanese"},
			{Code: "ko", Name: "Korean"},
			{Code: "zh", Name: "Chinese"},
			{Code: "ru", Name: "Russian"},
			{Code: "ar", Name: "Arabic"},
			{Code: "hi", Name: "Hindi"},
		}

		body, _ := json.Marshal(languages)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(body),
		}, nil
	}

	// Initialize translation service
	translationSvc, err := translation.NewService(ctx, h.store, h.logger, true)
	if err != nil {
		h.logger.Error("failed to initialize translation service", zap.Error(err))
		// Return basic list on error
		languages := []TranslationLanguage{
			{Code: "en", Name: "English"},
			{Code: "es", Name: "Spanish"},
			{Code: "fr", Name: "French"},
		}
		body, _ := json.Marshal(languages)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(body),
		}, nil
	}

	// Get supported languages from AWS Translate
	supportedLangs, err := translationSvc.GetSupportedLanguages(ctx)
	if err != nil {
		h.logger.Error("failed to get supported languages", zap.Error(err))
		// Return basic list on error
		languages := []TranslationLanguage{
			{Code: "en", Name: "English"},
			{Code: "es", Name: "Spanish"},
			{Code: "fr", Name: "French"},
		}
		body, _ := json.Marshal(languages)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(body),
		}, nil
	}

	// Convert to API format
	languages := make([]TranslationLanguage, 0, len(supportedLangs))
	for _, lang := range supportedLangs {
		// Filter out some less common languages if needed
		languages = append(languages, TranslationLanguage{
			Code: lang.Code,
			Name: lang.Name,
		})
	}

	body, _ := json.Marshal(languages)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// Example AWS Translate integration (commented out for reference):
/*
func (h *Handler) translateWithAWS(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	// Create AWS Translate client
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", "", err
	}

	client := translate.NewFromConfig(cfg)

	// Prepare translation request
	input := &translate.TranslateTextInput{
		Text:               aws.String(text),
		TargetLanguageCode: aws.String(targetLang),
	}

	// Auto-detect source language if not provided
	if sourceLang != "" && sourceLang != "auto" {
		input.SourceLanguageCode = aws.String(sourceLang)
	}

	// Perform translation
	result, err := client.TranslateText(ctx, input)
	if err != nil {
		return "", "", err
	}

	return *result.TranslatedText, *result.SourceLanguageCode, nil
}
*/
