package handlers

import (
	"encoding/json"
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

		requireStatus(t, http.StatusBadRequest)(handler.HandleGetRecoveryOptionsLift(ctx))
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

		requireStatus(t, http.StatusOK)(handler.HandleGetRecoveryOptionsLift(ctx))
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

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetRecoveryOptionsLift(ctx))

		var body struct {
			Options []string `json:"options"`
		}
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		opts := body.Options
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

		requireStatus(t, http.StatusBadRequest)(handler.HandleInitiateSocialRecoveryLift(ctx))
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

		requireStatus(t, http.StatusOK)(handler.HandleInitiateSocialRecoveryLift(ctx))
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

		resp := requireStatus(t, http.StatusOK)(handler.HandleInitiateSocialRecoveryLift(ctx))

		var body struct {
			RequestID string `json:"request_id"`
		}
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.RequestID)
	})

	t.Run("confirm_invalid_body", func(t *testing.T) {
		_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/social/confirm", nil, nil, []byte(`{invalid}`))

		requireStatus(t, http.StatusBadRequest)(handler.HandleConfirmSocialRecoveryLift(ctx))
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

		requireStatus(t, http.StatusBadRequest)(handler.HandleConfirmSocialRecoveryLift(ctx))
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

		requireStatus(t, http.StatusUnauthorized)(handler.HandleGenerateRecoveryCodesLift(ctx))
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
		ctx.Set("jwt_claims", map[string]any{"sub": "alice"})

		requireStatus(t, http.StatusInternalServerError)(handler.HandleGenerateRecoveryCodesLift(ctx))
	})

	t.Run("use_invalid_body", func(t *testing.T) {
		_, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/codes/use", nil, nil, []byte(`{invalid}`))

		requireStatus(t, http.StatusBadRequest)(handler.HandleUseRecoveryCodeLift(ctx))
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

		requireStatus(t, http.StatusBadRequest)(handler.HandleUseRecoveryCodeLift(ctx))
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

		requireStatus(t, http.StatusBadRequest)(handler.HandleUseRecoveryCodeLift(ctx))

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
		requireStatus(t, http.StatusInternalServerError)(handler2.HandleUseRecoveryCodeLift(ctx2))
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

		requireStatus(t, http.StatusUnauthorized)(handler.HandleAddTrusteeLift(ctx))

		ctx2, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/trustees/add", nil, nil, map[string]string{"trustee_actor_id": "no-at"})
		require.NoError(t, err)
		ctx2.Set("jwt_claims", map[string]any{"sub": "alice"})

		requireStatus(t, http.StatusBadRequest)(handler.HandleAddTrusteeLift(ctx2))
	})

	t.Run("add_trustee_invalid_body_and_storage_error", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: stdErrors.New("create failed")}
		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPost, "/auth/recovery/trustees/add", nil, nil, []byte(`{invalid}`))
		ctxBad.Set("jwt_claims", map[string]any{"sub": "alice"})
		requireStatus(t, http.StatusBadRequest)(handler.HandleAddTrusteeLift(ctxBad))

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/trustees/add", nil, nil, map[string]string{"trustee_actor_id": "@bob@example.com"})
		require.NoError(t, err)
		ctx.Set("jwt_claims", map[string]any{"sub": "alice"})

		requireStatus(t, http.StatusBadRequest)(handler.HandleAddTrusteeLift(ctx))
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
		requireStatus(t, http.StatusUnauthorized)(handler.HandleListTrusteesLift(ctx))

		ctx2, err := round10NewLiftContext(http.MethodGet, "/auth/recovery/trustees", nil, nil, nil)
		require.NoError(t, err)
		ctx2.Set("jwt_claims", map[string]any{"sub": "alice"})
		requireStatus(t, http.StatusInternalServerError)(handler.HandleListTrusteesLift(ctx2))
	})

	t.Run("remove_trustee_errors", func(t *testing.T) {
		state := &round10QueryState{deleteErrorOnce: stdErrors.New("delete failed")}
		_, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		handler := NewEmailFreeRecoveryHandler(authService)

		ctx, err := round10NewLiftContext(http.MethodDelete, "/auth/recovery/trustees/", nil, nil, nil)
		require.NoError(t, err)
		ctx.Set("jwt_claims", map[string]any{"sub": "alice"})

		requireStatus(t, http.StatusBadRequest)(handler.HandleRemoveTrusteeLift(ctx))

		ctx2, err := round10NewLiftContext(http.MethodDelete, "/auth/recovery/trustees/trustee-1", nil, nil, nil)
		require.NoError(t, err)
		ctx2.Set("jwt_claims", map[string]any{"sub": "alice"})
		ctx2.Params["trustee_id"] = "@bob@example.com"

		requireStatus(t, http.StatusBadRequest)(handler.HandleRemoveTrusteeLift(ctx2))
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
		requireStatus(t, http.StatusBadRequest)(handler.HandleDeviceRecoveryLift(ctxBad))

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/device", nil, nil, map[string]string{"username": "alice", "device_id": "device-1"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleDeviceRecoveryLift(ctx))
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
		requireStatus(t, http.StatusInternalServerError)(handler.HandleDeviceRecoveryLift(ctx))
	})
}
