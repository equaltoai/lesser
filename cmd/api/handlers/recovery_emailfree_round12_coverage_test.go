package lift

import (
	"context"
	stdErrors "errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRecoveryEmailFree_Round12_GetRecoveryOptions_Coverage(t *testing.T) {
	t.Run("missing_username", func(t *testing.T) {
		cfg := round11TestConfig()
		_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/recovery/options", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetRecoveryOptionsLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("user_not_found_returns_generic_ok", func(t *testing.T) {
		cfg := round11TestConfig()
		state := &round10QueryState{
			notFoundPKs: map[string]bool{"USER#missing": true},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/recovery/options", nil, map[string]string{"username": "missing"}, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetRecoveryOptionsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("development_env_forces_all_options", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Environment = "development"

		now := time.Now()
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/recovery/options", nil, map[string]string{"username": "alice"}, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetRecoveryOptionsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		body, ok := ctx.Response.Body.(map[string]any)
		require.True(t, ok)
		opts, ok := body["options"].([]string)
		if !ok {
			// Lift JSON encoder may decode as []interface{} in tests depending on adapter.
			raw, ok := body["options"].([]any)
			require.True(t, ok)
			for _, v := range raw {
				if s, ok := v.(string); ok {
					opts = append(opts, s)
				}
			}
		}
		require.ElementsMatch(t, []string{"passkey", "wallet", "oauth_github", "social", "recovery_code"}, opts)
	})
}

func TestRecoveryEmailFree_Round12_SocialRecoveryInitiateAndConfirm_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	t.Run("initiate_invalid_body", func(t *testing.T) {
		_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/social/initiate", nil, nil, []byte(`{invalid}`))

		require.NoError(t, handler.HandleInitiateSocialRecoveryLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("initiate_insufficient_trustees_returns_generic_ok", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
			trusteesByUser: map[string][]storagemodels.Trustee{
				"alice": {{Username: "alice", ActorID: "@trustee1@example.com", AddedAt: now.Add(-2 * time.Hour), Confirmed: true}},
			},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/social/initiate", nil, nil, map[string]string{"username": "alice"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleInitiateSocialRecoveryLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("initiate_development_returns_details_with_fallback_parser", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Environment = "development"

		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
			trusteesByUser: map[string][]storagemodels.Trustee{
				"alice": {
					{Username: "alice", ActorID: "@trustee1@example.com", AddedAt: now.Add(-2 * time.Hour), Confirmed: true},
					{Username: "alice", ActorID: "@trustee2@example.com", AddedAt: now.Add(-2 * time.Hour), Confirmed: true},
				},
			},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/social/initiate", headers, nil, []byte(`{"username":"alice"}`))

		require.NoError(t, handler.HandleInitiateSocialRecoveryLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		body, ok := ctx.Response.Body.(map[string]any)
		require.True(t, ok)
		_, ok = body["request_id"].(string)
		require.True(t, ok)
	})

	t.Run("confirm_invalid_body", func(t *testing.T) {
		_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/social/confirm", nil, nil, []byte(`{invalid}`))

		require.NoError(t, handler.HandleConfirmSocialRecoveryLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("confirm_missing_request", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{"RECOVERY#missing#REQUEST": true},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/social/confirm", nil, nil, map[string]string{"request_id": "missing", "trustee_id": "@trustee@example.com"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleConfirmSocialRecoveryLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})
}

func TestRecoveryEmailFree_Round12_RecoveryCodes_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	t.Run("generate_unauthorized", func(t *testing.T) {
		_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/codes/generate", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGenerateRecoveryCodesLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("generate_internal_error", func(t *testing.T) {
		state := &round10QueryState{
			createErrorOnce: stdErrors.New("create failed"),
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
		}
		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/codes/generate", nil, nil, nil)
		require.NoError(t, err)
		ctx.Context = context.WithValue(ctx.Context, "jwt_claims", map[string]any{"sub": "alice"})

		require.NoError(t, handler.HandleGenerateRecoveryCodesLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("use_invalid_body", func(t *testing.T) {
		_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/codes/use", nil, nil, []byte(`{invalid}`))

		require.NoError(t, handler.HandleUseRecoveryCodeLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("use_invalid_code", func(t *testing.T) {
		code := "ABCD-EFGH-IJKL-MNOP"
		normalized := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
		hash, err := auth.HashPassword(normalized)
		require.NoError(t, err)

		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
			recoveryCodesByUser: map[string][]storagemodels.RecoveryCode{
				"alice": {{Username: "alice", CodeHash: hash, CreatedAt: now.Add(-2 * time.Hour), Position: 0}},
			},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/codes/use", nil, nil, map[string]string{"username": "alice", "code": "WRONG-CODE-0000"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleUseRecoveryCodeLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("use_validate_error_and_token_generation_error", func(t *testing.T) {
		allErrType := reflect.TypeOf(&[]storagemodels.RecoveryCode{}).String()
		state := &round10QueryState{
			allErrorByType: map[string]error{allErrType: stdErrors.New("query failed")},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/codes/use", nil, nil, map[string]string{"username": "alice", "code": "ABCD"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleUseRecoveryCodeLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)

		// Now force recovery token storage to fail after a valid code.
		code := "ABCD-EFGH-IJKL-MNOP"
		normalized := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
		hash, err := auth.HashPassword(normalized)
		require.NoError(t, err)
		state2 := &round10QueryState{
			createErrorOnce: stdErrors.New("create failed"),
			recoveryCodesByUser: map[string][]storagemodels.RecoveryCode{
				"alice": {{Username: "alice", CodeHash: hash, CreatedAt: now.Add(-2 * time.Hour), Position: 0}},
			},
		}
		_, repos2, _ := round11NewHandler(t, cfg, state2)
		authService2, err := auth.NewAuthService(cfg, repos2)
		require.NoError(t, err)
		handler2 := NewEmailFreeRecoveryHandler(authService2)

		ctx2, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/codes/use", nil, nil, map[string]string{"username": "alice", "code": code})
		require.NoError(t, err)
		require.NoError(t, handler2.HandleUseRecoveryCodeLift(ctx2))
		require.Equal(t, http.StatusInternalServerError, ctx2.Response.StatusCode)
	})
}

func TestRecoveryEmailFree_Round12_TrusteesAndDeviceRecovery_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("add_trustee_unauthorized_and_invalid_format", func(t *testing.T) {
		_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/trustees/add", nil, nil, map[string]string{"trustee_actor_id": "@bob@example.com"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleAddTrusteeLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/trustees/add", nil, nil, map[string]string{"trustee_actor_id": "no-at"})
		require.NoError(t, err)
		ctx2.Context = context.WithValue(ctx2.Context, "jwt_claims", map[string]any{"sub": "alice"})

		require.NoError(t, handler.HandleAddTrusteeLift(ctx2))
		require.Equal(t, http.StatusBadRequest, ctx2.Response.StatusCode)
	})

	t.Run("add_trustee_invalid_body_and_storage_error", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: stdErrors.New("create failed")}
		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/trustees/add", nil, nil, []byte(`{invalid}`))
		ctxBad.Context = context.WithValue(ctxBad.Context, "jwt_claims", map[string]any{"sub": "alice"})
		require.NoError(t, handler.HandleAddTrusteeLift(ctxBad))
		require.Equal(t, http.StatusBadRequest, ctxBad.Response.StatusCode)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/trustees/add", nil, nil, map[string]string{"trustee_actor_id": "@bob@example.com"})
		require.NoError(t, err)
		ctx.Context = context.WithValue(ctx.Context, "jwt_claims", map[string]any{"sub": "alice"})

		require.NoError(t, handler.HandleAddTrusteeLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("list_trustees_unauthorized_and_internal_error", func(t *testing.T) {
		allErrType := reflect.TypeOf(&[]storagemodels.Trustee{}).String()
		state := &round10QueryState{
			allErrorByType: map[string]error{allErrType: stdErrors.New("query failed")},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/recovery/trustees", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleListTrusteesLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodGet, "/auth/recovery/trustees", nil, nil, nil)
		require.NoError(t, err)
		ctx2.Context = context.WithValue(ctx2.Context, "jwt_claims", map[string]any{"sub": "alice"})
		require.NoError(t, handler.HandleListTrusteesLift(ctx2))
		require.Equal(t, http.StatusInternalServerError, ctx2.Response.StatusCode)
	})

	t.Run("remove_trustee_errors", func(t *testing.T) {
		state := &round10QueryState{deleteErrorOnce: stdErrors.New("delete failed")}
		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodDelete, "/auth/recovery/trustees/", nil, nil, nil)
		require.NoError(t, err)
		ctx.Context = context.WithValue(ctx.Context, "jwt_claims", map[string]any{"sub": "alice"})

		require.NoError(t, handler.HandleRemoveTrusteeLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodDelete, "/auth/recovery/trustees/trustee-1", nil, nil, nil)
		require.NoError(t, err)
		ctx2.Context = context.WithValue(ctx2.Context, "jwt_claims", map[string]any{"sub": "alice"})
		ctx2.SetParam("trustee_id", "@bob@example.com")

		require.NoError(t, handler.HandleRemoveTrusteeLift(ctx2))
		require.Equal(t, http.StatusBadRequest, ctx2.Response.StatusCode)
	})

	t.Run("device_recovery_invalid_body_and_not_trusted", func(t *testing.T) {
		state := &round10QueryState{
			devicesByID: map[string]storagemodels.Device{
				"device-1": {DeviceID: "device-1", Username: "alice", DeviceName: "iPhone", TrustLevel: "untrusted"},
			},
		}

		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/device", nil, nil, []byte(`{invalid}`))
		require.NoError(t, handler.HandleDeviceRecoveryLift(ctxBad))
		require.Equal(t, http.StatusBadRequest, ctxBad.Response.StatusCode)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/device", nil, nil, map[string]string{"username": "alice", "device_id": "device-1"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeviceRecoveryLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("device_recovery_token_storage_error", func(t *testing.T) {
		state := &round10QueryState{
			createErrorOnce: stdErrors.New("create failed"),
			devicesByID: map[string]storagemodels.Device{
				"device-1": {DeviceID: "device-1", Username: "alice", DeviceName: "iPhone", TrustLevel: "trusted"},
			},
		}
		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/device", nil, nil, map[string]string{"username": "alice", "device_id": "device-1"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeviceRecoveryLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})
}
