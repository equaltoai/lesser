package lift

import (
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/translation"
	"github.com/pay-theory/lift/pkg/lift"
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

// HandleTranslateStatusLift handles POST /api/v1/statuses/:id/translate
func (h *Handler) HandleTranslateStatusLift(ctx *lift.Context) error {
	// Check if translation is enabled
	if !h.isTranslationEnabled() {
		return common.RespondUnprocessableEntity(ctx, "translation service is not enabled")
	}

	// Get and validate status ID
	statusID, err := h.getTranslationStatusID(ctx)
	if err != nil {
		return err
	}

	// Authenticate user
	username, err := h.authenticateTranslationRequest(ctx)
	if err != nil {
		return err
	}

	// Get the status object
	objectID := h.normalizeTranslationObjectID(statusID)
	obj, err := h.getStatusForTranslation(ctx, statusID, objectID)
	if err != nil {
		return err
	}

	// Extract content from the object
	content, spoilerText, language := h.extractTranslatableContent(obj)
	if err := common.ValidateRequiredParam("content", content); err != nil {
		return common.RespondUnprocessableEntity(ctx, "status has no content to translate")
	}

	// Get target language
	targetLang := h.getTargetLanguage(ctx, username)

	// Perform translation
	translationResult, err := h.performTranslation(ctx, statusID, content, spoilerText, language, targetLang)
	if err != nil {
		return err
	}

	return common.RespondSuccess(ctx, translationResult)
}

// isTranslationEnabled checks if translation service is enabled
func (h *Handler) isTranslationEnabled() bool {
	return h.cfg.TranslationEnabled
}

// getTranslationStatusID gets and validates the status ID
func (h *Handler) getTranslationStatusID(ctx *lift.Context) (string, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return "", common.RespondValidationError(ctx, err)
	}
	return statusID, nil
}

// authenticateTranslationRequest handles authentication for translation
func (h *Handler) authenticateTranslationRequest(ctx *lift.Context) (string, error) {
	// Normal authentication flow
	return h.authenticateTranslationWithToken(ctx)
}

// authenticateTranslationWithToken authenticates using bearer token
func (h *Handler) authenticateTranslationWithToken(ctx *lift.Context) (string, error) {
	// Extract auth header
	authHeader := h.extractTranslationAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", common.RespondUnauthorized(ctx)
	}

	return claims.Username, nil
}

// extractTranslationAuthHeader extracts authorization header
func (h *Handler) extractTranslationAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("Authorization", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if err := common.ValidateRequiredParam("authorization", authHeader); err != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// normalizeTranslationObjectID normalizes the status ID to a full URL
func (h *Handler) normalizeTranslationObjectID(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// getStatusForTranslation retrieves the status object
func (h *Handler) getStatusForTranslation(ctx *lift.Context, statusID, objectID string) (any, error) {
	// Use Notes service to get the status
	note, err := h.registry.Notes().GetNote(ctx.Context, statusID)
	if err != nil {
		h.logger.Error("failed to get status for translation",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil, common.RespondNotFound(ctx, "status not found")
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
	}, nil
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
func (h *Handler) getTargetLanguage(ctx *lift.Context, username string) string {
	// Use Accounts service to get preferences
	result, err := h.registry.Accounts().GetPreferences(ctx.Context, &accounts.GetPreferencesQuery{
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
func (h *Handler) performTranslation(ctx *lift.Context, statusID, content, spoilerText, sourceLang, targetLang string) (*TranslationResult, error) {
	// Validate language codes
	if sourceLang != "" {
		if err := common.ValidateLanguageCode(sourceLang); err != nil {
			return nil, errors.Join(invalidSourceLanguageCode(), err)
		}
	}
	if err := common.ValidateLanguageCode(targetLang); err != nil {
		return nil, errors.Join(invalidTargetLanguageCode(), err)
	}

	// Initialize translation service
	translationSvc, err := h.initializeTranslationService(ctx)
	if err != nil {
		return nil, err
	}

	// Translate main content
	translatedContent, detectedLang, err := h.translateContent(ctx, translationSvc, statusID, content, sourceLang, targetLang)
	if err != nil {
		return nil, err
	}

	// Translate spoiler text if present
	translatedSpoiler := h.translateSpoilerText(ctx, translationSvc, spoilerText, sourceLang, targetLang)

	// Build response
	return &TranslationResult{
		Content:          translatedContent,
		SpoilerText:      translatedSpoiler,
		DetectedLanguage: detectedLang,
		Provider:         "AWS Translate",
	}, nil
}

// initializeTranslationService initializes the translation service
func (h *Handler) initializeTranslationService(ctx *lift.Context) (*translation.Service, error) {
	translationSvc, err := translation.NewService(ctx.Context, h.cfg, h.repos, h.logger, true)
	if err != nil {
		h.logger.Error("failed to initialize translation service", zap.Error(err))
		return nil, common.RespondInternalServerError(ctx, "translation service initialization failed")
	}
	return translationSvc, nil
}

// translateContent translates the main content
func (h *Handler) translateContent(ctx *lift.Context, svc *translation.Service, statusID, content, sourceLang, targetLang string) (string, string, error) {
	translatedContent, detectedLang, err := svc.TranslateHTML(ctx.Context, content, sourceLang, targetLang)
	if err != nil {
		h.logger.Error("failed to translate content",
			zap.String("status_id", statusID),
			zap.String("target_lang", targetLang),
			zap.Error(err))
		return "", "", common.RespondInternalServerError(ctx, "translation failed")
	}
	return translatedContent, detectedLang, nil
}

// translateSpoilerText translates the spoiler text if present
func (h *Handler) translateSpoilerText(ctx *lift.Context, svc *translation.Service, spoilerText, sourceLang, targetLang string) string {
	if err := common.ValidateRequiredParam("spoiler_text", spoilerText); err != nil {
		return ""
	}

	translated, _, err := svc.TranslateText(ctx.Context, spoilerText, sourceLang, targetLang)
	if err == nil {
		return translated
	}

	// Return original if translation failed
	return spoilerText
}

// HandleGetTranslationLanguagesLift handles GET /api/v1/instance/translation_languages
func (h *Handler) HandleGetTranslationLanguagesLift(ctx *lift.Context) error {
	// Check if translation is enabled
	translationEnabled := h.cfg.TranslationEnabled
	if !translationEnabled {
		return common.RespondUnprocessableEntity(ctx, "translation service is not enabled")
	}

	// Initialize translation service
	translationSvc, err := translation.NewService(ctx.Context, h.cfg, h.repos, h.logger, true)
	if err != nil {
		h.logger.Error("failed to initialize translation service", zap.Error(err))
		return common.RespondInternalServerError(ctx, "translation service initialization failed")
	}

	// Get supported languages from AWS Translate
	supportedLangs, err := translationSvc.GetSupportedLanguages(ctx.Context)
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
