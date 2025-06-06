package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
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
	_, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get the status
	obj, err := h.store.GetObject(ctx, statusID)
	if err != nil {
		h.logger.Error("failed to get status for translation",
			zap.String("status_id", statusID),
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

	// Get user's preferred language (default to "en" if not set)
	// targetLang := "en"
	// TODO: Get user's preferred language from user preferences

	// For now, return a mock translation response
	// TODO: Integrate with a translation service (AWS Translate, Google Translate, etc.)
	translation := TranslationResult{
		Content:          fmt.Sprintf("[Mock translation of: %s]", content),
		SpoilerText:      spoilerText,
		DetectedLanguage: language,
		Provider:         "mock",
	}

	// In a real implementation, you would:
	// 1. Detect the source language if not specified
	// 2. Call a translation API (AWS Translate, Google Translate, DeepL, etc.)
	// 3. Cache the translation for efficiency
	// 4. Handle errors appropriately

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
	// Return a list of supported languages
	// In a real implementation, this would come from your translation service
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
