package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage/core"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/translation"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeTranslationService struct {
	translateHTMLContent string
	translateHTMLLang    string
	translateHTMLErr     error
	translateTextErr     error
	langs                []translation.LanguageInfo
}

func (f *fakeTranslationService) TranslateHTML(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
	return f.translateHTMLContent, f.translateHTMLLang, f.translateHTMLErr
}

func (f *fakeTranslationService) TranslateText(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
	if f.translateTextErr != nil {
		return "", "", f.translateTextErr
	}
	return "translated spoiler", sourceLang, nil
}

func (f *fakeTranslationService) GetSupportedLanguages(ctx context.Context) ([]translation.LanguageInfo, error) {
	return f.langs, nil
}

func TestTranslationHelpers(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	h := &Handler{cfg: cfg, logger: logger}
	require.Equal(t, cfg.BaseURL()+"/objects/status-1", h.normalizeTranslationObjectID("status-1"))
	require.Equal(t, "https://example.com/objects/1", h.normalizeTranslationObjectID("https://example.com/objects/1"))

	content, spoiler, lang := h.extractTranslatableContent(map[string]any{
		"content":  "hello",
		"summary":  "spoiler",
		"language": "es",
	})
	require.Equal(t, "hello", content)
	require.Equal(t, "spoiler", spoiler)
	require.Equal(t, "es", lang)
}

func TestTranslationTargetLanguageFallback(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	registry := &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
				return &accounts.PreferencesResult{Preferences: map[string]any{"language": "invalid-lang"}}, nil
			},
		},
	}

	h := &Handler{cfg: cfg, logger: logger, registry: registry}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/translate", nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "en", h.getTargetLanguage(ctx, "alice"))
}

func TestHandleTranslateStatusLift(t *testing.T) {
	cfg := round10TestConfig()
	cfg.TranslationEnabled = true
	logger := round10TestLogger(t)

	translationSvc := &fakeTranslationService{
		translateHTMLContent: "hola",
		translateHTMLLang:    "es",
	}

	restore := newTranslationService
	newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
		return translationSvc, nil
	}
	defer func() { newTranslationService = restore }()

	registry := &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
				return &accounts.PreferencesResult{Preferences: map[string]any{"language": "en"}}, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
				note := &activitypub.Note{
					BaseObject: activitypub.BaseObject{Summary: "spoiler"},
					Content:    "<p>hello</p>",
				}
				return &storagemodels.Status{
					StatusID: statusID,
					Content:  "<p>hello</p>",
					Note:     note,
					Language: "en",
				}, nil
			},
		},
	}

	h := &Handler{cfg: cfg, logger: logger, registry: registry}
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/translate", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "1"

	requireStatus(t, http.StatusOK)(h.HandleTranslateStatusLift(ctx))
}

func TestHandleGetTranslationLanguagesLift(t *testing.T) {
	cfg := round10TestConfig()
	cfg.TranslationEnabled = true
	logger := round10TestLogger(t)

	translationSvc := &fakeTranslationService{
		langs: []translation.LanguageInfo{
			{Code: "en", Name: "English"},
			{Code: "es", Name: "Spanish"},
		},
	}

	restore := newTranslationService
	newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
		return translationSvc, nil
	}
	defer func() { newTranslationService = restore }()

	h := &Handler{cfg: cfg, logger: logger}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/translation_languages", nil, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusOK)(h.HandleGetTranslationLanguagesLift(ctx))
}

func TestTranslateSpoilerTextFallback(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	h := &Handler{cfg: cfg, logger: logger}
	svc := &fakeTranslationService{translateTextErr: errors.New("boom")}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/translate", nil, nil, nil)
	require.NoError(t, err)

	require.Equal(t, "spoiler", h.translateSpoilerText(ctx, svc, "spoiler", "en", "es"))
	require.Equal(t, "", h.translateSpoilerText(ctx, svc, "", "en", "es"))
}
