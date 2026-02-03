package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage/core"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/translation"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type translationServiceStub struct {
	TranslateHTMLFunc          func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error)
	TranslateTextFunc          func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error)
	GetSupportedLanguagesFunc  func(ctx context.Context) ([]translation.LanguageInfo, error)
}

func (s *translationServiceStub) TranslateHTML(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
	if s != nil && s.TranslateHTMLFunc != nil {
		return s.TranslateHTMLFunc(ctx, content, sourceLang, targetLang)
	}
	return "", "", errors.New("TranslateHTML stub not set")
}

func (s *translationServiceStub) TranslateText(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
	if s != nil && s.TranslateTextFunc != nil {
		return s.TranslateTextFunc(ctx, content, sourceLang, targetLang)
	}
	return "", "", errors.New("TranslateText stub not set")
}

func (s *translationServiceStub) GetSupportedLanguages(ctx context.Context) ([]translation.LanguageInfo, error) {
	if s != nil && s.GetSupportedLanguagesFunc != nil {
		return s.GetSupportedLanguagesFunc(ctx)
	}
	return nil, errors.New("GetSupportedLanguages stub not set")
}

func TestTranslationRound13_HandleTranslateStatus_AndLanguages(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.TranslationEnabled = true

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	t.Run("translation disabled returns 422", func(t *testing.T) {
		disabledCfg := *cfg
		disabledCfg.TranslationEnabled = false
		h, _, _ := round11NewHandler(t, &disabledCfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status1/translate", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status1"

		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleTranslateStatusLift(ctx))
	})

	t.Run("invalid status id rejected", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/bad id/translate", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "bad id"

		requireStatus(t, http.StatusBadRequest)(h.HandleTranslateStatusLift(ctx))
	})

	t.Run("missing token rejected", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status1/translate", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status1"

		requireStatus(t, http.StatusUnauthorized)(h.HandleTranslateStatusLift(ctx))
	})

	t.Run("status not found returns 404", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
					return nil, errors.New("not found")
				},
			},
			AccountsSvc: &AccountsServiceStub{
				GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
					return &accounts.PreferencesResult{Preferences: map[string]any{"language": "en"}}, nil
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status1/translate", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status1"

		requireStatus(t, http.StatusNotFound)(h.HandleTranslateStatusLift(ctx))
	})

	t.Run("status with empty content returns 422", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
					return &storagemodels.Status{StatusID: statusID, Content: "", Note: &activitypub.Note{BaseObject: activitypub.BaseObject{Summary: "spoiler"}}}, nil
				},
			},
			AccountsSvc: &AccountsServiceStub{
				GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
					return &accounts.PreferencesResult{Preferences: map[string]any{"language": "en"}}, nil
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status1/translate", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status1"

		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleTranslateStatusLift(ctx))
	})

	t.Run("invalid source language rejected", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
					return &storagemodels.Status{StatusID: statusID, Content: "<p>hello</p>", Language: "english"}, nil
				},
			},
			AccountsSvc: &AccountsServiceStub{
				GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
					return &accounts.PreferencesResult{Preferences: map[string]any{"language": "en"}}, nil
				},
			},
		}

		svc := &translationServiceStub{
			TranslateHTMLFunc: func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
				return "<p>hola</p>", "en", nil
			},
			TranslateTextFunc: func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
				return "aviso", "en", nil
			},
		}

		oldFactory := newTranslationService
		newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
			return svc, nil
		}
		t.Cleanup(func() { newTranslationService = oldFactory })

		h, _, _ := round11NewHandler(t, cfg, state, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status1/translate", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status1"

		requireStatus(t, http.StatusBadRequest)(h.HandleTranslateStatusLift(ctx))
	})

	t.Run("translation service initialization failure returns 500", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
					return &storagemodels.Status{StatusID: statusID, Content: "<p>hello</p>"}, nil
				},
			},
			AccountsSvc: &AccountsServiceStub{
				GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
					return &accounts.PreferencesResult{Preferences: map[string]any{"language": "en"}}, nil
				},
			},
		}

		oldFactory := newTranslationService
		newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
			return nil, errors.New("boom")
		}
		t.Cleanup(func() { newTranslationService = oldFactory })

		h, _, _ := round11NewHandler(t, cfg, state, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status1/translate", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleTranslateStatusLift(ctx))
	})

	t.Run("translation failure returns 500", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
					return &storagemodels.Status{StatusID: statusID, Content: "<p>hello</p>", Note: &activitypub.Note{BaseObject: activitypub.BaseObject{Summary: "spoiler"}}}, nil
				},
			},
			AccountsSvc: &AccountsServiceStub{
				GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
					return &accounts.PreferencesResult{Preferences: map[string]any{"language": "en"}}, nil
				},
			},
		}

		svc := &translationServiceStub{
			TranslateHTMLFunc: func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
				return "", "", errors.New("translate failed")
			},
			TranslateTextFunc: func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
				return "ignored", "en", nil
			},
		}

		oldFactory := newTranslationService
		newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
			return svc, nil
		}
		t.Cleanup(func() { newTranslationService = oldFactory })

		h, _, _ := round11NewHandler(t, cfg, state, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status1/translate", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleTranslateStatusLift(ctx))
	})

	t.Run("success returns translated content and supports spoiler fallback", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
					return &storagemodels.Status{
						StatusID:  statusID,
						Content:   "<p>Hello</p>",
						Language:  "en",
						Note:      &activitypub.Note{BaseObject: activitypub.BaseObject{Summary: "Spoiler"}},
					}, nil
				},
			},
			AccountsSvc: &AccountsServiceStub{
				GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
					return &accounts.PreferencesResult{Preferences: map[string]any{"language": "es"}}, nil
				},
			},
		}

		svc := &translationServiceStub{
			TranslateHTMLFunc: func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
				require.Equal(t, "es", targetLang)
				return "<p>Hola</p>", "en", nil
			},
			TranslateTextFunc: func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
				return "", "", errors.New("spoiler translation failed")
			},
		}

		oldFactory := newTranslationService
		newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
			return svc, nil
		}
		t.Cleanup(func() { newTranslationService = oldFactory })

		h, _, _ := round11NewHandler(t, cfg, state, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status1/translate", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status1"

		resp := requireStatus(t, http.StatusOK)(h.HandleTranslateStatusLift(ctx))
		var out TranslationResult
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "<p>Hola</p>", out.Content)
		require.Equal(t, "Spoiler", out.SpoilerText)
		require.Equal(t, "AWS Translate", out.Provider)
	})

	t.Run("target language validation errors", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
					return &storagemodels.Status{StatusID: statusID, Content: "<p>Hello</p>", Language: "en"}, nil
				},
			},
			AccountsSvc: &AccountsServiceStub{
				GetPreferencesFunc: func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
					return &accounts.PreferencesResult{Preferences: map[string]any{"language": "en"}}, nil
				},
			},
		}
		svc := &translationServiceStub{
			TranslateHTMLFunc: func(ctx context.Context, content, sourceLang, targetLang string) (string, string, error) {
				return "<p>Hola</p>", "en", nil
			},
		}
		oldFactory := newTranslationService
		newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
			return svc, nil
		}
		t.Cleanup(func() { newTranslationService = oldFactory })

		h, _, _ := round11NewHandler(t, cfg, state, reg)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/status1/translate", headers, nil, nil)
		require.NoError(t, err)

		_, resp, err := h.performTranslation(ctx, "status1", "<p>hello</p>", "", "en", "zzz")
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("get translation languages", func(t *testing.T) {
		t.Run("disabled returns 422", func(t *testing.T) {
			disabledCfg := *cfg
			disabledCfg.TranslationEnabled = false
			h, _, _ := round11NewHandler(t, &disabledCfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/translation_languages", nil, nil, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusUnprocessableEntity)(h.HandleGetTranslationLanguagesLift(ctx))
		})

		t.Run("service init error returns 500", func(t *testing.T) {
			oldFactory := newTranslationService
			newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
				return nil, errors.New("boom")
			}
			t.Cleanup(func() { newTranslationService = oldFactory })

			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/translation_languages", nil, nil, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusInternalServerError)(h.HandleGetTranslationLanguagesLift(ctx))
		})

		t.Run("languages error returns 500", func(t *testing.T) {
			svc := &translationServiceStub{
				GetSupportedLanguagesFunc: func(ctx context.Context) ([]translation.LanguageInfo, error) {
					return nil, errors.New("boom")
				},
			}
			oldFactory := newTranslationService
			newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
				return svc, nil
			}
			t.Cleanup(func() { newTranslationService = oldFactory })

			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/translation_languages", nil, nil, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusInternalServerError)(h.HandleGetTranslationLanguagesLift(ctx))
		})

		t.Run("success returns languages", func(t *testing.T) {
			svc := &translationServiceStub{
				GetSupportedLanguagesFunc: func(ctx context.Context) ([]translation.LanguageInfo, error) {
					return []translation.LanguageInfo{
						{Code: "en", Name: "English"},
						{Code: "es", Name: "Spanish"},
					}, nil
				},
			}
			oldFactory := newTranslationService
			newTranslationService = func(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, cacheEnabled bool) (translationService, error) {
				return svc, nil
			}
			t.Cleanup(func() { newTranslationService = oldFactory })

			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/translation_languages", nil, nil, nil)
			require.NoError(t, err)

			resp := requireStatus(t, http.StatusOK)(h.HandleGetTranslationLanguagesLift(ctx))
			var langs []TranslationLanguage
			require.NoError(t, json.Unmarshal(resp.Body, &langs))
			require.Len(t, langs, 2)
		})
	})
}
