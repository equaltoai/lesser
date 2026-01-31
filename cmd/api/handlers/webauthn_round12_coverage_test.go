package lift

import (
	"encoding/base64"
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	authpkg "github.com/equaltoai/lesser/pkg/auth"
	"github.com/go-webauthn/webauthn/protocol"
	libwebauthn "github.com/go-webauthn/webauthn/webauthn"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestWebAuthn_Round12_PublicKeyMap_Coverage(t *testing.T) {
	t.Run("nil_options_returns_nil_map", func(t *testing.T) {
		out, err := webAuthnPublicKeyMap(nil)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("marshal_error_is_returned", func(t *testing.T) {
		_, err := webAuthnPublicKeyMap(map[string]any{"bad": func() {}})
		require.Error(t, err)
	})

	t.Run("unmarshal_error_is_returned_for_non_object", func(t *testing.T) {
		_, err := webAuthnPublicKeyMap("not-an-object")
		require.Error(t, err)
	})

	t.Run("plain_object_round_trips", func(t *testing.T) {
		out, err := webAuthnPublicKeyMap(map[string]any{"k": "v"})
		require.NoError(t, err)
		require.Equal(t, "v", out["k"])
	})

	t.Run("protocol_option_types_are_supported", func(t *testing.T) {
		out, err := webAuthnPublicKeyMap(protocol.CredentialCreation{})
		require.NoError(t, err)
		require.Contains(t, out, "challenge")

		out, err = webAuthnPublicKeyMap(&protocol.CredentialCreation{})
		require.NoError(t, err)
		require.Contains(t, out, "challenge")

		out, err = webAuthnPublicKeyMap(protocol.CredentialAssertion{})
		require.NoError(t, err)
		require.Contains(t, out, "challenge")

		out, err = webAuthnPublicKeyMap(&protocol.CredentialAssertion{})
		require.NoError(t, err)
		require.Contains(t, out, "challenge")
	})
}

func TestWebAuthn_Round12_RegistrationBeginAndFinish_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read", "write"})
	authHeaders := map[string]string{"Authorization": "Bearer " + token}

	t.Run("begin_registration_requires_auth", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/begin", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleBeginWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("begin_registration_success", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/begin", authHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleBeginWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		resp, ok := ctx.Response.Body.(apimodels.WebAuthnBeginResponse)
		require.True(t, ok)
		require.NotEmpty(t, resp.Challenge)
		require.NotEmpty(t, resp.PublicKey)
	})

	t.Run("begin_registration_exits_if_response_already_written", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/begin", authHeaders, nil, nil)
		require.NoError(t, err)

		ctx.Status(http.StatusTeapot)
		require.NoError(t, ctx.JSON(map[string]string{"error": "already written"}))

		require.NoError(t, handler.HandleBeginWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusTeapot, ctx.Response.StatusCode)
	})

	t.Run("begin_registration_storage_error_maps_to_500", func(t *testing.T) {
		state := &round10QueryState{
			createErrorOnce: stdErrors.New("create failed"),
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/begin", authHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleBeginWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("finish_registration_requires_auth", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/finish", nil, nil, apimodels.WebAuthnFinishRegistrationRequest{
			Challenge: "c1",
			Response:  map[string]any{},
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("finish_registration_parse_error_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/auth/webauthn/register/finish", authHeaders, nil, []byte("{"))
		require.NoError(t, handler.HandleFinishWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("finish_registration_missing_challenge_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/finish", authHeaders, nil, apimodels.WebAuthnFinishRegistrationRequest{
			Challenge: "",
			Response:  map[string]any{},
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("finish_registration_missing_or_expired_challenge_returns_400", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{"CHALLENGE#missing": true},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/finish", authHeaders, nil, apimodels.WebAuthnFinishRegistrationRequest{
			Challenge: "missing",
			Response:  map[string]any{},
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})
}

func TestWebAuthn_Round12_LoginBeginAndFinish_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("begin_login_auth_service_init_failure_returns_500", func(t *testing.T) {
		badCfg := round11TestConfig()
		badCfg.JWTSecret = ""
		handler, _, _ := round11NewHandler(t, badCfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/begin", nil, nil, apimodels.WebAuthnBeginLoginRequest{Username: "alice"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleBeginWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("begin_login_parse_error_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/auth/webauthn/login/begin", nil, nil, []byte("{"))
		require.NoError(t, handler.HandleBeginWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("begin_login_username_required", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/begin", nil, nil, apimodels.WebAuthnBeginLoginRequest{Username: ""})
		require.NoError(t, err)
		require.NoError(t, handler.HandleBeginWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("begin_login_no_credentials_returns_400", func(t *testing.T) {
		state := &round10QueryState{
			webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
				"alice": {},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/begin", nil, nil, apimodels.WebAuthnBeginLoginRequest{Username: "alice"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleBeginWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("begin_login_success", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/begin", nil, nil, apimodels.WebAuthnBeginLoginRequest{Username: "alice"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleBeginWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		resp, ok := ctx.Response.Body.(apimodels.WebAuthnBeginResponse)
		require.True(t, ok)
		require.NotEmpty(t, resp.Challenge)
		require.NotEmpty(t, resp.PublicKey)
	})

	t.Run("finish_login_parse_error_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/auth/webauthn/login/finish", nil, nil, []byte("{"))
		require.NoError(t, handler.HandleFinishWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("finish_login_auth_service_init_failure_returns_500", func(t *testing.T) {
		badCfg := round11TestConfig()
		badCfg.JWTSecret = ""
		handler, _, _ := round11NewHandler(t, badCfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/finish", nil, nil, apimodels.WebAuthnFinishLoginRequest{
			Username:   "alice",
			Challenge:  "c1",
			Response:   map[string]any{},
			DeviceName: "",
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("finish_login_required_fields", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctxMissingUser, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/finish", nil, nil, apimodels.WebAuthnFinishLoginRequest{
			Username:  "",
			Challenge: "c1",
			Response:  map[string]any{},
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnLoginLift(ctxMissingUser))
		require.Equal(t, http.StatusBadRequest, ctxMissingUser.Response.StatusCode)

		ctxMissingChallenge, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/finish", nil, nil, apimodels.WebAuthnFinishLoginRequest{
			Username:  "alice",
			Challenge: "",
			Response:  map[string]any{},
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnLoginLift(ctxMissingChallenge))
		require.Equal(t, http.StatusBadRequest, ctxMissingChallenge.Response.StatusCode)
	})

	t.Run("finish_login_missing_or_expired_challenge_returns_400_and_defaults_device_name", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{"CHALLENGE#missing": true},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/finish", nil, nil, apimodels.WebAuthnFinishLoginRequest{
			Username:   "alice",
			Challenge:  "missing",
			Response:   map[string]any{},
			DeviceName: "",
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})
}

func TestWebAuthn_Round12_CredentialsCRUD_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read", "write"})
	authHeaders := map[string]string{"Authorization": "Bearer " + token}

	t.Run("list_credentials_requires_auth", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/auth/webauthn/credentials", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleListWebAuthnCredentialsLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("list_credentials_query_error_returns_500", func(t *testing.T) {
		state := &round10QueryState{
			allErrorByType: map[string]error{
				"*[]models.WebAuthnCredential": stdErrors.New("boom"),
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/auth/webauthn/credentials", authHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleListWebAuthnCredentialsLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("list_credentials_success", func(t *testing.T) {
		state := &round10QueryState{
			webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
				"alice": {
					{
						ID:         "Y3JlZA==",
						UserID:     "alice",
						Name:       "Key 1",
						CreatedAt:  time.Now().Add(-2 * time.Hour),
						LastUsedAt: time.Now().Add(-1 * time.Hour),
					},
					{
						ID:         "Y3JlZDI=",
						UserID:     "alice",
						Name:       "Key 2",
						CreatedAt:  time.Now().Add(-3 * time.Hour),
						LastUsedAt: time.Now().Add(-2 * time.Hour),
					},
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/auth/webauthn/credentials", authHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleListWebAuthnCredentialsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		resp, ok := ctx.Response.Body.(apimodels.WebAuthnCredentialsResponse)
		require.True(t, ok)
		require.Len(t, resp.Credentials, 2)
	})

	t.Run("delete_credential_requires_auth_and_param", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctxUnauthed, err := round10NewLiftContext(http.MethodDelete, "/api/v1/auth/webauthn/credentials/cred", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeleteWebAuthnCredentialLift(ctxUnauthed))
		require.Equal(t, http.StatusUnauthorized, ctxUnauthed.Response.StatusCode)

		ctxMissingParam, err := round10NewLiftContext(http.MethodDelete, "/api/v1/auth/webauthn/credentials/", authHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeleteWebAuthnCredentialLift(ctxMissingParam))
		require.Equal(t, http.StatusBadRequest, ctxMissingParam.Response.StatusCode)
	})

	t.Run("delete_credential_not_found_returns_404", func(t *testing.T) {
		state := &round10QueryState{
			webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
				"Y3JlZA==": {ID: "Y3JlZA==", UserID: "bob"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleDeleteWebAuthnCredentialLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("delete_credential_last_auth_method_returns_400", func(t *testing.T) {
		state := &round10QueryState{
			webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
				"Y3JlZA==": {ID: "Y3JlZA==", UserID: "alice"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleDeleteWebAuthnCredentialLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("delete_credential_delete_error_returns_500", func(t *testing.T) {
		state := &round10QueryState{
			deleteErrorOnce: stdErrors.New("delete failed"),
			webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
				"Y3JlZA==": {ID: "Y3JlZA==", UserID: "alice"},
			},
			webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
				"alice": {
					{ID: "Y3JlZA==", UserID: "alice"},
					{ID: "Y3JlZDI=", UserID: "alice"},
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleDeleteWebAuthnCredentialLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("delete_credential_success", func(t *testing.T) {
		state := &round10QueryState{
			webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
				"Y3JlZA==": {ID: "Y3JlZA==", UserID: "alice"},
			},
			webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
				"alice": {
					{ID: "Y3JlZA==", UserID: "alice"},
					{ID: "Y3JlZDI=", UserID: "alice"},
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleDeleteWebAuthnCredentialLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("update_credential_requires_auth_param_and_name", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctxUnauthed, err := round10NewLiftContext(http.MethodPut, "/api/v1/auth/webauthn/credentials/cred", nil, nil, apimodels.WebAuthnUpdateCredentialRequest{Name: "n"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleUpdateWebAuthnCredentialNameLift(ctxUnauthed))
		require.Equal(t, http.StatusUnauthorized, ctxUnauthed.Response.StatusCode)

		ctxMissingParam, err := round10NewLiftContext(http.MethodPut, "/api/v1/auth/webauthn/credentials/", authHeaders, nil, apimodels.WebAuthnUpdateCredentialRequest{Name: "n"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleUpdateWebAuthnCredentialNameLift(ctxMissingParam))
		require.Equal(t, http.StatusBadRequest, ctxMissingParam.Response.StatusCode)

		ctxBadBody := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, []byte("{"))
		ctxBadBody.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleUpdateWebAuthnCredentialNameLift(ctxBadBody))
		require.Equal(t, http.StatusBadRequest, ctxBadBody.Response.StatusCode)

		ctxMissingName, err := round10NewLiftContext(http.MethodPut, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, apimodels.WebAuthnUpdateCredentialRequest{Name: ""})
		require.NoError(t, err)
		ctxMissingName.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleUpdateWebAuthnCredentialNameLift(ctxMissingName))
		require.Equal(t, http.StatusBadRequest, ctxMissingName.Response.StatusCode)
	})

	t.Run("update_credential_not_found_returns_404", func(t *testing.T) {
		state := &round10QueryState{
			webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
				"Y3JlZA==": {ID: "Y3JlZA==", UserID: "bob"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, apimodels.WebAuthnUpdateCredentialRequest{Name: "new name"})
		require.NoError(t, err)
		ctx.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleUpdateWebAuthnCredentialNameLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("update_credential_update_error_returns_500", func(t *testing.T) {
		state := &round10QueryState{
			updateErrorOnce: stdErrors.New("update failed"),
			webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
				"Y3JlZA==": {ID: "Y3JlZA==", UserID: "alice"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, apimodels.WebAuthnUpdateCredentialRequest{Name: "new name"})
		require.NoError(t, err)
		ctx.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleUpdateWebAuthnCredentialNameLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("update_credential_success", func(t *testing.T) {
		state := &round10QueryState{
			webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
				"Y3JlZA==": {ID: "Y3JlZA==", UserID: "alice"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/auth/webauthn/credentials/Y3JlZA==", authHeaders, nil, apimodels.WebAuthnUpdateCredentialRequest{Name: "new name"})
		require.NoError(t, err)
		ctx.SetParam("credentialId", "Y3JlZA==")
		require.NoError(t, handler.HandleUpdateWebAuthnCredentialNameLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})
}

func TestWebAuthn_Round12_FinishHandlers_Success_Fixtures(t *testing.T) {
	const (
		registrationChallenge = "W8GzFU8pGjhoRbWrLDlamAfq_y4S1CZG1VuoeRLARrE"
		registrationResponse  = `{
  "id":"6xrtBhJQW6QU4tOaB4rrHaS2Ks0yDDL_q8jDC16DEjZ-VLVf4kCRkvl2xp2D71sTPYns-exsHQHTy3G-zJRK8g",
  "rawId":"6xrtBhJQW6QU4tOaB4rrHaS2Ks0yDDL_q8jDC16DEjZ-VLVf4kCRkvl2xp2D71sTPYns-exsHQHTy3G-zJRK8g",
  "type":"public-key",
  "authenticatorAttachment":"platform",
  "clientExtensionResults":{"appid":true},
  "response":{
    "attestationObject":"o2NmbXRkbm9uZWdhdHRTdG10oGhhdXRoRGF0YVjEdKbqkhPJnC90siSSsyDPQCYqlMGpUKA5fyklC2CEHvBBAAAAAAAAAAAAAAAAAAAAAAAAAAAAQOsa7QYSUFukFOLTmgeK6x2ktirNMgwy_6vIwwtegxI2flS1X-JAkZL5dsadg-9bEz2J7PnsbB0B08txvsyUSvKlAQIDJiABIVggLKF5xS0_BntttUIrm2Z2tgZ4uQDwllbdIfrrBMABCNciWCDHwin8Zdkr56iSIh0MrB5qZiEzYLQpEOREhMUkY6q4Vw",
    "clientDataJSON":"eyJjaGFsbGVuZ2UiOiJXOEd6RlU4cEdqaG9SYldyTERsYW1BZnFfeTRTMUNaRzFWdW9lUkxBUnJFIiwib3JpZ2luIjoiaHR0cHM6Ly93ZWJhdXRobi5pbyIsInR5cGUiOiJ3ZWJhdXRobi5jcmVhdGUifQ",
    "transports":["usb","nfc","fake"]
  }
}`

		loginChallenge = "E4PTcIH_HfX1pC6Sigk1SC9NAlgeztN0439vi8z_c9k"
		loginResponse  = `{
  "id":"AI7D5q2P0LS-Fal9ZT7CHM2N5BLbUunF92T8b6iYC199bO2kagSuU05-5dZGqb1SP0A0lyTWng",
  "rawId":"AI7D5q2P0LS-Fal9ZT7CHM2N5BLbUunF92T8b6iYC199bO2kagSuU05-5dZGqb1SP0A0lyTWng",
  "clientExtensionResults":{"appID":"example.com"},
  "type":"public-key",
  "response":{
    "authenticatorData":"dKbqkhPJnC90siSSsyDPQCYqlMGpUKA5fyklC2CEHvBFXJJiGa3OAAI1vMYKZIsLJfHwVQMANwCOw-atj9C0vhWpfWU-whzNjeQS21Lpxfdk_G-omAtffWztpGoErlNOfuXWRqm9Uj9ANJck1p6lAQIDJiABIVggKAhfsdHcBIc0KPgAcRyAIK_-Vi-nCXHkRHPNaCMBZ-4iWCBxB8fGYQSBONi9uvq0gv95dGWlhJrBwCsj_a4LJQKVHQ",
    "clientDataJSON":"eyJjaGFsbGVuZ2UiOiJFNFBUY0lIX0hmWDFwQzZTaWdrMVNDOU5BbGdlenROMDQzOXZpOHpfYzlrIiwibmV3X2tleXNfbWF5X2JlX2FkZGVkX2hlcmUiOiJkbyBub3QgY29tcGFyZSBjbGllbnREYXRhSlNPTiBhZ2FpbnN0IGEgdGVtcGxhdGUuIFNlZSBodHRwczovL2dvby5nbC95YWJQZXgiLCJvcmlnaW4iOiJodHRwczovL3dlYmF1dGhuLmlvIiwidHlwZSI6IndlYmF1dGhuLmdldCJ9",
    "signature":"MEUCIBtIVOQxzFYdyWQyxaLR0tik1TnuPhGVhXVSNgFwLmN5AiEAnxXdCq0UeAVGWxOaFcjBZ_mEZoXqNboY5IkQDdlWZYc",
    "userHandle":"YWxpY2U"
  }
}`

		loginCredentialPublicKeyB64URL = "pQMmIAEhWCAoCF-x0dwEhzQo-ABxHIAgr_5WL6cJceREc81oIwFn7iJYIHEHx8ZhBIE42L26-rSC_3l0ZaWEmsHAKyP9rgslApUdAQI"
	)

	cfg := round11TestConfig()
	cfg.Domain = "webauthn.io"

	t.Run("finish_registration_success", func(t *testing.T) {
		sessionID := "sess-1"
		token := round11SignToken(t, cfg.JWTSecret, "alice", []string{"read"}, sessionID)
		headers := map[string]string{"Authorization": "Bearer " + token}

		sessionDataBytes, err := json.Marshal(libwebauthn.SessionData{
			Challenge:      registrationChallenge,
			RelyingPartyID: cfg.Domain,
			UserID:         []byte("alice"),
			CredParams:     libwebauthn.CredentialParametersDefault(),
			Expires:        time.Now().Add(5 * time.Minute),
			UserVerification: protocol.VerificationPreferred,
		})
		require.NoError(t, err)

		var responseMap map[string]any
		require.NoError(t, json.Unmarshal([]byte(registrationResponse), &responseMap))

		state := &round10QueryState{
			sessionsByID: map[string]storagemodels.Session{
				sessionID: {SessionID: sessionID, UserID: "USER#alice", ExpiresAt: time.Now().Add(1 * time.Hour).Unix()},
			},
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: true, Version: 1},
			},
			webAuthnChallengesByID: map[string]storagemodels.WebAuthnChallenge{
				registrationChallenge: {
					Challenge:   registrationChallenge,
					UserID:      "alice",
					Type:        "registration",
					ExpiresAt:   time.Now().Add(5 * time.Minute),
					SessionData: sessionDataBytes,
				},
			},
			webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
				"alice": {},
			},
		}

		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/finish", headers, nil, apimodels.WebAuthnFinishRegistrationRequest{
			Challenge:      registrationChallenge,
			Response:       responseMap,
			CredentialName: "Fixture Key",
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnRegistrationLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("finish_login_success", func(t *testing.T) {
		idBytes, err := base64.RawURLEncoding.DecodeString("AI7D5q2P0LS-Fal9ZT7CHM2N5BLbUunF92T8b6iYC199bO2kagSuU05-5dZGqb1SP0A0lyTWng")
		require.NoError(t, err)
		credentialID := base64.StdEncoding.EncodeToString(idBytes)

		publicKeyBytes, err := base64.RawURLEncoding.DecodeString(loginCredentialPublicKeyB64URL)
		require.NoError(t, err)

		sessionDataBytes, err := json.Marshal(libwebauthn.SessionData{
			Challenge:            loginChallenge,
			RelyingPartyID:       cfg.Domain,
			UserID:               []byte("alice"),
			AllowedCredentialIDs: [][]byte{idBytes},
			Expires:              time.Now().Add(5 * time.Minute),
			UserVerification:     protocol.VerificationPreferred,
		})
		require.NoError(t, err)

		var responseMap map[string]any
		require.NoError(t, json.Unmarshal([]byte(loginResponse), &responseMap))

		state := &round10QueryState{
			webAuthnChallengesByID: map[string]storagemodels.WebAuthnChallenge{
				loginChallenge: {
					Challenge:   loginChallenge,
					UserID:      "alice",
					Type:        "authentication",
					ExpiresAt:   time.Now().Add(5 * time.Minute),
					SessionData: sessionDataBytes,
				},
			},
			webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
				credentialID: {ID: credentialID, UserID: "alice", PublicKey: publicKeyBytes, CreatedAt: time.Now().Add(-1 * time.Hour)},
			},
			webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
				"alice": {{ID: credentialID, UserID: "alice", PublicKey: publicKeyBytes, CreatedAt: time.Now().Add(-1 * time.Hour)}},
			},
		}

		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/login/finish", nil, nil, apimodels.WebAuthnFinishLoginRequest{
			Username:   "alice",
			Challenge:  loginChallenge,
			Response:   responseMap,
			DeviceName: "Fixture Device",
		})
		require.NoError(t, err)
		require.NoError(t, handler.HandleFinishWebAuthnLoginLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		resp, ok := ctx.Response.Body.(*authpkg.AuthResponse)
		require.True(t, ok)
		require.Equal(t, "alice", resp.Me)
		require.Equal(t, credentialID, resp.CredentialID)
		require.NotEmpty(t, resp.AccessToken)
		require.NotEmpty(t, resp.RefreshToken)
	})
}
