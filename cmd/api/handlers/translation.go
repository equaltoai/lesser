package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/translation"
	apptheory "github.com/theory-cloud/apptheory/runtime"
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

type translationService interface {
	TranslateHTML(ctx context.Context, content, sourceLang, targetLang string) (string, string, error)
	TranslateText(ctx context.Context, content, sourceLang, targetLang string) (string, string, error)
	GetSupportedLanguages(ctx context.Context) ([]translation.LanguageInfo, error)
}

type translationServiceFactory func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error)

var newTranslationService translationServiceFactory = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
	return translation.NewService(ctx, cfg, repos, logger, cacheEnabled)
}

// HandleTranslateStatusLift handles POST /api/v1/statuses/:id/translate
func (h *Handler) HandleTranslateStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Check if translation is enabled
	if !h.isTranslationEnabled() {
		return common.RespondUnprocessableEntity(ctx, "translation service is not enabled")
	}

	// Get and validate status ID
	statusID, resp, err := h.getTranslationStatusID(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Authenticate user
	username, resp, err := h.authenticateTranslationRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get the status object
	objectID := h.normalizeTranslationObjectID(statusID)
	obj, resp, err := h.getStatusForTranslation(ctx, statusID, objectID)
	if resp != nil || err != nil {
		return resp, err
	}

	// Extract content from the object
	content, spoilerText, language := h.extractTranslatableContent(obj)
	if err := common.ValidateRequiredParam("content", content); err != nil {
		return common.RespondUnprocessableEntity(ctx, "status has no content to translate")
	}

	// Get target language
	targetLang := h.getTargetLanguage(ctx, username)

	// Perform translation
	translationResult, resp, err := h.performTranslation(ctx, statusID, content, spoilerText, language, targetLang)
	if resp != nil || err != nil {
		return resp, err
	}

	return common.RespondSuccess(ctx, translationResult)
}

// isTranslationEnabled checks if translation service is enabled
func (h *Handler) isTranslationEnabled() bool {
	return h.cfg.TranslationEnabled
}

// getTranslationStatusID gets and validates the status ID
func (h *Handler) getTranslationStatusID(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		resp, respErr := common.RespondValidationError(ctx, err)
		return "", resp, respErr
	}
	return statusID, nil, nil
}

// authenticateTranslationRequest handles authentication for translation
func (h *Handler) authenticateTranslationRequest(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	// Normal authentication flow
	return h.authenticateTranslationWithToken(ctx)
}

// authenticateTranslationWithToken authenticates using bearer token
func (h *Handler) authenticateTranslationWithToken(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	// Extract auth header
	authHeader := h.extractTranslationAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	return claims.Username, nil, nil
}

// extractTranslationAuthHeader extracts authorization header
func (h *Handler) extractTranslationAuthHeader(ctx *apptheory.Context) string {
	return headerValue(ctx, "Authorization")
}

// normalizeTranslationObjectID normalizes the status ID to a full URL
func (h *Handler) normalizeTranslationObjectID(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// getStatusForTranslation retrieves the status object
func (h *Handler) getStatusForTranslation(ctx *apptheory.Context, statusID, objectID string) (any, *apptheory.Response, error) {
	// Use Notes service to get the status
	note, err := h.registry.Notes().GetNote(ctx.Context(), statusID)
	if err != nil {
		h.logger.Error("failed to get status for translation",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
		resp, respErr := common.RespondNotFound(ctx, "status not found")
		return nil, resp, respErr
	}

	// Convert to generic object for translation
	summary := ""
	if note.Note != nil {
		summary = note.Note.Summary
	}
	return map[string]any{
		"content":  note.Content,
		"summary":  summary,
		"language": note.Language,
	}, nil, nil
}

// extractTranslatableContent extracts content from the status object
func (h *Handler) extractTranslatableContent(obj any) (content, spoilerText, language string) {
	switch o := obj.(type) {
	case *activitypub.Note:
		content = o.Content
		spoilerText = o.Summary
		// Note: Language is not part of the standard Note type
	case map[string]any:
		content = h.extractStringField(o, "content")
		spoilerText = h.extractStringField(o, "summary")
		language = h.extractStringField(o, "language")
	}
	return
}

// extractStringField extracts a string field from a map
func (h *Handler) extractStringField(m map[string]any, field string) string {
	if val, ok := m[field].(string); ok {
		return val
	}
	return ""
}

// getTargetLanguage gets the user's preferred language for translation
func (h *Handler) getTargetLanguage(ctx *apptheory.Context, username string) string {
	// Use Accounts service to get preferences
	result, err := h.registry.Accounts().GetPreferences(ctx.Context(), &accounts.GetPreferencesQuery{
		Username: username,
	})
	if err == nil && result != nil && result.Preferences != nil {
		if lang, ok := result.Preferences["language"].(string); ok && lang != "" {
			if err := common.ValidateLanguageCode(lang); err != nil {
				h.logger.Warn("invalid language code in user preferences",
					zap.String("username", username),
					zap.String("lang", lang),
					zap.Error(err))
				return "en" // Fall back to English for invalid codes
			}
			return lang
		}
	}
	return "en" // Default to English
}

// performTranslation performs the actual translation
func (h *Handler) performTranslation(ctx *apptheory.Context, statusID, content, spoilerText, sourceLang, targetLang string) (*TranslationResult, *apptheory.Response, error) {
	// Validate language codes
	if sourceLang != "" {
		if err := common.ValidateLanguageCode(sourceLang); err != nil {
			resp, respErr := common.RespondValidationError(ctx, errors.Join(invalidSourceLanguageCode(), err))
			return nil, resp, respErr
		}
	}
	if err := common.ValidateLanguageCode(targetLang); err != nil {
		resp, respErr := common.RespondValidationError(ctx, errors.Join(invalidTargetLanguageCode(), err))
		return nil, resp, respErr
	}

	// Initialize translation service
	translationSvc, resp, err := h.initializeTranslationService(ctx)
	if resp != nil || err != nil {
		return nil, resp, err
	}

	// Translate main content
	translatedContent, detectedLang, resp, err := h.translateContent(ctx, translationSvc, statusID, content, sourceLang, targetLang)
	if resp != nil || err != nil {
		return nil, resp, err
	}

	// Translate spoiler text if present
	translatedSpoiler := h.translateSpoilerText(ctx, translationSvc, spoilerText, sourceLang, targetLang)

	// Build response
	return &TranslationResult{
		Content:          translatedContent,
		SpoilerText:      translatedSpoiler,
		DetectedLanguage: detectedLang,
		Provider:         "AWS Translate",
	}, nil, nil
}

// initializeTranslationService initializes the translation service
func (h *Handler) initializeTranslationService(ctx *apptheory.Context) (translationService, *apptheory.Response, error) {
	translationSvc, err := newTranslationService(ctx.Context(), h.cfg, h.repos, h.logger, true)
	if err != nil {
		h.logger.Error("failed to initialize translation service", zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "translation service initialization failed")
		return nil, resp, respErr
	}
	return translationSvc, nil, nil
}

// translateContent translates the main content
func (h *Handler) translateContent(ctx *apptheory.Context, svc translationService, statusID, content, sourceLang, targetLang string) (string, string, *apptheory.Response, error) {
	translatedContent, detectedLang, err := svc.TranslateHTML(ctx.Context(), content, sourceLang, targetLang)
	if err != nil {
		h.logger.Error("failed to translate content",
			zap.String("status_id", statusID),
			zap.String("target_lang", targetLang),
			zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "translation failed")
		return "", "", resp, respErr
	}
	return translatedContent, detectedLang, nil, nil
}

// translateSpoilerText translates the spoiler text if present
func (h *Handler) translateSpoilerText(ctx *apptheory.Context, svc translationService, spoilerText, sourceLang, targetLang string) string {
	if err := common.ValidateRequiredParam("spoiler_text", spoilerText); err != nil {
		return ""
	}

	translated, _, err := svc.TranslateText(ctx.Context(), spoilerText, sourceLang, targetLang)
	if err == nil {
		return translated
	}

	// Return original if translation failed
	return spoilerText
}

// HandleGetTranslationLanguagesLift handles GET /api/v1/instance/translation_languages
func (h *Handler) HandleGetTranslationLanguagesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Check if translation is enabled
	translationEnabled := h.cfg.TranslationEnabled
	if !translationEnabled {
		return common.RespondUnprocessableEntity(ctx, "translation service is not enabled")
	}

	// Initialize translation service
	translationSvc, err := newTranslationService(ctx.Context(), h.cfg, h.repos, h.logger, true)
	if err != nil {
		h.logger.Error("failed to initialize translation service", zap.Error(err))
		return common.RespondInternalServerError(ctx, "translation service initialization failed")
	}

	// Get supported languages from AWS Translate
	supportedLangs, err := translationSvc.GetSupportedLanguages(ctx.Context())
	if err != nil {
		h.logger.Error("failed to get supported languages", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to get supported languages")
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

	return common.RespondSuccess(ctx, languages)
}
