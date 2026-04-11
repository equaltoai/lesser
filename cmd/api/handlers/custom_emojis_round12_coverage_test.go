package handlers

import (
	"context"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func round12UserState(t *testing.T, username, role string) *round10QueryState {
	t.Helper()

	now := time.Now()
	user := storagemodels.User{
		Username:  username,
		Role:      role,
		Approved:  true,
		Version:   1,
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-24 * time.Hour),
	}
	require.NoError(t, user.UpdateKeys())

	return &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			username: user,
		},
	}
}

func TestCustomEmojis_Round12_GetCustomEmojis_Errors(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("service_unavailable", func(t *testing.T) {
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/custom_emojis", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusServiceUnavailable)(handler.HandleGetCustomEmojisLift(ctx))
	})

	t.Run("list_error", func(t *testing.T) {
		handler.registry = &RegistryStub{
			EmojiSvc: &EmojiServiceStub{
				ListEmojisFunc: func(context.Context, *emoji.ListEmojisQuery) (*emoji.ListResult, error) {
					return nil, stdErrors.New("boom")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/custom_emojis", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetCustomEmojisLift(ctx))
	})
}

func TestCustomEmojis_Round12_AdminEndpoints_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"read"})
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken}

	adminAccountSvc := &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
			return &storage.Account{
				User: &storage.User{Username: username, Role: roleAdmin},
			}, nil
		},
	}

	t.Run("create_unauthorized_missing_token", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			AccountsSvc: adminAccountSvc,
			EmojiSvc:    &EmojiServiceStub{},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", nil, nil, models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png"})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleCreateCustomEmojiLift(ctx))
	})

	t.Run("create_unauthorized_invalid_token", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			AccountsSvc: adminAccountSvc,
			EmojiSvc:    &EmojiServiceStub{},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", map[string]string{"Authorization": "Bearer bad"}, nil, models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png"})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleCreateCustomEmojiLift(ctx))
	})

	t.Run("create_forbidden_not_admin", func(t *testing.T) {
		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
		userHeaders := map[string]string{"Authorization": "Bearer " + userToken}
		handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "alice", "user"))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", userHeaders, nil, models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png"})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(handler.HandleCreateCustomEmojiLift(ctx))
	})

	t.Run("create_account_lookup_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
		handler.registry = &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
					return nil, stdErrors.New("boom")
				},
			},
			EmojiSvc: &EmojiServiceStub{},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", adminHeaders, nil, models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png"})
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleCreateCustomEmojiLift(ctx))
	})

	t.Run("create_invalid_request_body", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
		handler.registry = &RegistryStub{
			AccountsSvc: adminAccountSvc,
			EmojiSvc:    &EmojiServiceStub{},
		}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/custom_emojis", adminHeaders, nil, []byte(`{invalid}`))

		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateCustomEmojiLift(ctx))
	})

	t.Run("create_parse_fallback_success_and_validation_errors", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
		handler.registry = &RegistryStub{
			AccountsSvc: adminAccountSvc,
			EmojiSvc: &EmojiServiceStub{
				CreateEmojiFunc: func(ctx context.Context, cmd *emoji.CreateEmojiCommand) (*emoji.Result, error) {
					return &emoji.Result{
						Emoji: &storage.CustomEmoji{Shortcode: cmd.Shortcode, URL: cmd.ImageURL, StaticURL: cmd.ImageURL, VisibleInPicker: true, Category: cmd.Category},
					}, nil
				},
			},
		}

		// Force ctx.ParseRequest to fail by setting a non-JSON content-type, but keep JSON body so fallback succeeds.
		headers := map[string]string{
			"Authorization": adminHeaders["Authorization"],
			"Content-Type":  "application/x-www-form-urlencoded",
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/custom_emojis", headers, nil, []byte(`{"shortcode":"","url":"https://example.com/wave.png"}`))
		requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleCreateCustomEmojiLift(ctx))

		ctx2 := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/custom_emojis", headers, nil, []byte(`{"shortcode":"wave","url":""}`))
		requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleCreateCustomEmojiLift(ctx2))
	})

	t.Run("create_service_unavailable", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
		handler.registry = &RegistryStub{AccountsSvc: adminAccountSvc}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", adminHeaders, nil, models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png"})
		require.NoError(t, err)

		requireStatus(t, http.StatusServiceUnavailable)(handler.HandleCreateCustomEmojiLift(ctx))
	})

	t.Run("create_conflict_and_internal_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
		handler.registry = &RegistryStub{
			AccountsSvc: adminAccountSvc,
			EmojiSvc: &EmojiServiceStub{
				CreateEmojiFunc: func(ctx context.Context, cmd *emoji.CreateEmojiCommand) (*emoji.Result, error) {
					return nil, stdErrors.New("emoji with shortcode " + cmd.Shortcode + " already exists")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", adminHeaders, nil, models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png"})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleCreateCustomEmojiLift(ctx))

		handler.registry = &RegistryStub{
			AccountsSvc: adminAccountSvc,
			EmojiSvc: &EmojiServiceStub{
				CreateEmojiFunc: func(context.Context, *emoji.CreateEmojiCommand) (*emoji.Result, error) {
					return nil, stdErrors.New("boom")
				},
			},
		}
		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", adminHeaders, nil, models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png"})
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleCreateCustomEmojiLift(ctx2))
	})

	t.Run("update_validation_and_not_found", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
		handler.registry = &RegistryStub{
			AccountsSvc: adminAccountSvc,
			EmojiSvc: &EmojiServiceStub{
				UpdateEmojiFunc: func(ctx context.Context, cmd *emoji.UpdateEmojiCommand) (*emoji.Result, error) {
					return nil, stdErrors.New("emoji not found: " + cmd.Shortcode)
				},
			},
		}

		ctxMissing, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/", adminHeaders, nil, models.UpdateCustomEmojiRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleUpdateCustomEmojiLift(ctxMissing))

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, models.UpdateCustomEmojiRequest{})
		require.NoError(t, err)
		ctx.Params["shortcode"] = "wave"
		requireStatus(t, http.StatusNotFound)(handler.HandleUpdateCustomEmojiLift(ctx))
	})

	t.Run("update_additional_branches", func(t *testing.T) {
		t.Run("unauthorized", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			handler.registry = &RegistryStub{AccountsSvc: adminAccountSvc, EmojiSvc: &EmojiServiceStub{}}

			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", nil, nil, models.UpdateCustomEmojiRequest{})
			require.NoError(t, err)
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusUnauthorized)(handler.HandleUpdateCustomEmojiLift(ctx))
		})

		t.Run("forbidden_not_admin", func(t *testing.T) {
			userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
			userHeaders := map[string]string{"Authorization": "Bearer " + userToken}
			handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "alice", "user"))

			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", userHeaders, nil, models.UpdateCustomEmojiRequest{})
			require.NoError(t, err)
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusForbidden)(handler.HandleUpdateCustomEmojiLift(ctx))
		})

		t.Run("account_lookup_error", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
			handler.registry = &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
						return nil, stdErrors.New("boom")
					},
				},
				EmojiSvc: &EmojiServiceStub{},
			}

			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, models.UpdateCustomEmojiRequest{})
			require.NoError(t, err)
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusInternalServerError)(handler.HandleUpdateCustomEmojiLift(ctx))
		})

		t.Run("invalid_body_responds_400", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
			handler.registry = &RegistryStub{AccountsSvc: adminAccountSvc, EmojiSvc: &EmojiServiceStub{}}

			ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, []byte(`{invalid}`))
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusBadRequest)(handler.HandleUpdateCustomEmojiLift(ctx))
		})

		t.Run("emoji_service_unavailable", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
			handler.registry = &RegistryStub{AccountsSvc: adminAccountSvc}

			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, models.UpdateCustomEmojiRequest{})
			require.NoError(t, err)
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusServiceUnavailable)(handler.HandleUpdateCustomEmojiLift(ctx))
		})

		t.Run("internal_error_and_success", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
			handler.registry = &RegistryStub{
				AccountsSvc: adminAccountSvc,
				EmojiSvc: &EmojiServiceStub{
					UpdateEmojiFunc: func(context.Context, *emoji.UpdateEmojiCommand) (*emoji.Result, error) {
						return nil, stdErrors.New("boom")
					},
				},
			}

			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, models.UpdateCustomEmojiRequest{})
			require.NoError(t, err)
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusInternalServerError)(handler.HandleUpdateCustomEmojiLift(ctx))

			handler.registry = &RegistryStub{
				AccountsSvc: adminAccountSvc,
				EmojiSvc: &EmojiServiceStub{
					UpdateEmojiFunc: func(ctx context.Context, cmd *emoji.UpdateEmojiCommand) (*emoji.Result, error) {
						return &emoji.Result{
							Emoji: &storage.CustomEmoji{Shortcode: cmd.Shortcode, URL: "https://example.com/wave.png", StaticURL: "https://example.com/wave.png", VisibleInPicker: true},
						}, nil
					},
				},
			}

			ctx2, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, models.UpdateCustomEmojiRequest{})
			require.NoError(t, err)
			ctx2.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusOK)(handler.HandleUpdateCustomEmojiLift(ctx2))
		})
	})

	t.Run("delete_not_found_and_internal_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
		handler.registry = &RegistryStub{
			AccountsSvc: adminAccountSvc,
			EmojiSvc: &EmojiServiceStub{
				DeleteEmojiFunc: func(ctx context.Context, cmd *emoji.DeleteEmojiCommand) error {
					if cmd.Shortcode == "missing" {
						return stdErrors.New("emoji not found: " + cmd.Shortcode)
					}
					return stdErrors.New("boom")
				},
			},
		}

		ctxMissing, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/missing", adminHeaders, nil, nil)
		require.NoError(t, err)
		ctxMissing.Params["shortcode"] = "missing"
		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteCustomEmojiLift(ctxMissing))

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["shortcode"] = "wave"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleDeleteCustomEmojiLift(ctx))
	})

	t.Run("delete_additional_branches", func(t *testing.T) {
		t.Run("missing_shortcode_param", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
			handler.registry = &RegistryStub{AccountsSvc: adminAccountSvc, EmojiSvc: &EmojiServiceStub{}}

			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/", adminHeaders, nil, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteCustomEmojiLift(ctx))
		})

		t.Run("unauthorized", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			handler.registry = &RegistryStub{AccountsSvc: adminAccountSvc, EmojiSvc: &EmojiServiceStub{}}

			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/wave", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusUnauthorized)(handler.HandleDeleteCustomEmojiLift(ctx))
		})

		t.Run("forbidden_not_admin", func(t *testing.T) {
			userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
			userHeaders := map[string]string{"Authorization": "Bearer " + userToken}
			handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "alice", "user"))

			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/wave", userHeaders, nil, nil)
			require.NoError(t, err)
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusForbidden)(handler.HandleDeleteCustomEmojiLift(ctx))
		})

		t.Run("emoji_service_unavailable", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
			handler.registry = &RegistryStub{AccountsSvc: adminAccountSvc}

			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, nil)
			require.NoError(t, err)
			ctx.Params["shortcode"] = "wave"

			requireStatus(t, http.StatusServiceUnavailable)(handler.HandleDeleteCustomEmojiLift(ctx))
		})
	})
}

func TestCustomEmojis_Round12_parseEmojiRequest_BodyEmpty(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/custom_emojis", map[string]string{"Content-Type": "application/json"}, nil, []byte{})

	var req models.CreateCustomEmojiRequest
	require.Error(t, handler.parseEmojiRequest(ctx, &req))
}

func TestCustomEmojis_Round12_AdminAuthHeaderFallback(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"read"})
	headers := map[string]string{"Authorization": "Bearer " + token}

	handler, _, _ := round11NewHandler(t, cfg, round12UserState(t, "admin", roleAdmin))
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{User: &storage.User{Username: username, Role: roleAdmin}}, nil
			},
		},
		EmojiSvc: &EmojiServiceStub{
			DeleteEmojiFunc: func(context.Context, *emoji.DeleteEmojiCommand) error { return nil },
		},
	}

	ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/wave", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["shortcode"] = "wave"

	ctx.Request.Headers = map[string][]string{"authorization": {headers["Authorization"]}}

	requireStatus(t, http.StatusOK)(handler.HandleDeleteCustomEmojiLift(ctx))
}
