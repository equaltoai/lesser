package lift

import (
	"fmt"
	"os"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
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
		return ctx.Status(422).JSON(map[string]string{"error": "translation service is not enabled"})
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
	if content == "" {
		return ctx.Status(422).JSON(map[string]string{"error": "status has no content to translate"})
	}

	// Get target language
	targetLang := h.getTargetLanguage(ctx, username)

	// Perform translation
	translationResult, err := h.performTranslation(ctx, statusID, content, spoilerText, language, targetLang)
	if err != nil {
		return err
	}

	return ctx.Status(200).JSON(translationResult)
}

// isTranslationEnabled checks if translation service is enabled
func (h *Handler) isTranslationEnabled() bool {
	return os.Getenv("TRANSLATION_ENABLED") == boolTrue
}

// getTranslationStatusID gets and validates the status ID
func (h *Handler) getTranslationStatusID(ctx *lift.Context) (string, error) {
	statusID := ctx.Param("id")
	if statusID == "" {
		return "", ctx.Status(400).JSON(map[string]string{"error": "missing status ID"})
	}
	return statusID, nil
}

// authenticateTranslationRequest handles authentication for translation
func (h *Handler) authenticateTranslationRequest(ctx *lift.Context) (string, error) {
	// Check for test username
	testUsername := h.getTranslationTestUsername(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Normal authentication flow
	return h.authenticateTranslationWithToken(ctx)
}

// getTranslationTestUsername extracts test username from headers
func (h *Handler) getTranslationTestUsername(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// authenticateTranslationWithToken authenticates using bearer token
func (h *Handler) authenticateTranslationWithToken(ctx *lift.Context) (string, error) {
	// Extract auth header
	authHeader := h.extractTranslationAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	return claims.Username, nil
}

// extractTranslationAuthHeader extracts authorization header
func (h *Handler) extractTranslationAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
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
	obj, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		h.logger.Error("failed to get status for translation",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil, ctx.Status(404).JSON(map[string]string{"error": "status not found"})
	}
	return obj, nil
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
	userPrefs, err := h.repos.User().GetUserPreferences(ctx.Context, username)
	if err == nil && userPrefs != nil && userPrefs.Language != "" {
		return userPrefs.Language
	}
	return "en" // Default to English
}

// performTranslation performs the actual translation
func (h *Handler) performTranslation(ctx *lift.Context, statusID, content, spoilerText, sourceLang, targetLang string) (*TranslationResult, error) {
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
	translationSvc, err := translation.NewService(ctx.Context, h.repos, h.logger, true)
	if err != nil {
		h.logger.Error("failed to initialize translation service", zap.Error(err))
		return nil, ctx.Status(500).JSON(map[string]string{"error": "translation service initialization failed"})
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
		return "", "", ctx.Status(500).JSON(map[string]string{"error": "translation failed"})
	}
	return translatedContent, detectedLang, nil
}

// translateSpoilerText translates the spoiler text if present
func (h *Handler) translateSpoilerText(ctx *lift.Context, svc *translation.Service, spoilerText, sourceLang, targetLang string) string {
	if spoilerText == "" {
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
	translationEnabled := os.Getenv("TRANSLATION_ENABLED") == boolTrue
	if !translationEnabled {
		return ctx.Status(422).JSON(map[string]string{"error": "translation service is not enabled"})
	}

	// Initialize translation service
	translationSvc, err := translation.NewService(ctx.Context, h.repos, h.logger, true)
	if err != nil {
		h.logger.Error("failed to initialize translation service", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "translation service initialization failed"})
	}

	// Get supported languages from AWS Translate
	supportedLangs, err := translationSvc.GetSupportedLanguages(ctx.Context)
	if err != nil {
		h.logger.Error("failed to get supported languages", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get supported languages"})
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

	return ctx.Status(200).JSON(languages)
}
