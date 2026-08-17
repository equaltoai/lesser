package handlers

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/go-webauthn/webauthn/protocol"
	libwebauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

const (
	webauthnSignupFixtureChallenge = "W8GzFU8pGjhoRbWrLDlamAfq_y4S1CZG1VuoeRLARrE"
	webauthnSignupFixtureResponse  = `{
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
)

func TestWebAuthn_Round12_SignupBeginAndFinish_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("begin_signup_parse_error_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/auth/webauthn/signup/begin", nil, nil, []byte("{"))
		requireStatus(t, http.StatusBadRequest)(handler.HandleBeginWebAuthnSignupLift(ctx))
	})

	t.Run("begin_signup_username_required", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/signup/begin", nil, nil, apimodels.WebAuthnBeginLoginRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleBeginWebAuthnSignupLift(ctx))
	})

	t.Run("begin_signup_success", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/signup/begin", nil, nil, apimodels.WebAuthnBeginLoginRequest{Username: "alice"})
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(handler.HandleBeginWebAuthnSignupLift(ctx))

		var body apimodels.WebAuthnBeginResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.Challenge)
		require.NotEmpty(t, body.PublicKey)
	})

	t.Run("begin_signup_storage_error_maps_to_500", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			createErrorOnce: stdErrors.New("create failed"),
		}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/signup/begin", nil, nil, apimodels.WebAuthnBeginLoginRequest{Username: "alice"})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleBeginWebAuthnSignupLift(ctx))
	})

	t.Run("finish_signup_parse_error_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/auth/webauthn/signup/finish", nil, nil, []byte("{"))
		requireStatus(t, http.StatusBadRequest)(handler.HandleFinishWebAuthnSignupLift(ctx))
	})

	t.Run("finish_signup_required_fields", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctxMissingUser, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/signup/finish", nil, nil, webAuthnSignupFinishRequest{
			Challenge: "c1",
			Response:  map[string]any{},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleFinishWebAuthnSignupLift(ctxMissingUser))

		ctxMissingChallenge, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/signup/finish", nil, nil, webAuthnSignupFinishRequest{
			Username: "alice",
			Response: map[string]any{},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleFinishWebAuthnSignupLift(ctxMissingChallenge))
	})

	t.Run("finish_signup_missing_or_expired_challenge_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			notFoundPKs: map[string]bool{"CHALLENGE#missing": true},
		}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/signup/finish", nil, nil, webAuthnSignupFinishRequest{
			Username:  "alice",
			Challenge: "missing",
			Response:  map[string]any{},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleFinishWebAuthnSignupLift(ctx))
	})

	t.Run("finish_signup_success_returns_passkey_registration_proof", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Domain = "webauthn.io"

		sessionDataBytes, err := json.Marshal(libwebauthn.SessionData{
			Challenge:        webauthnSignupFixtureChallenge,
			RelyingPartyID:   cfg.Domain,
			UserID:           []byte("alice"),
			CredParams:       libwebauthn.CredentialParametersDefault(),
			Expires:          time.Now().Add(5 * time.Minute),
			UserVerification: protocol.VerificationPreferred,
		})
		require.NoError(t, err)

		var responseMap map[string]any
		require.NoError(t, json.Unmarshal([]byte(webauthnSignupFixtureResponse), &responseMap))

		state := &round10QueryState{
			webAuthnChallengesByID: map[string]storagemodels.WebAuthnChallenge{
				webauthnSignupFixtureChallenge: {
					Challenge:   webauthnSignupFixtureChallenge,
					UserID:      "alice",
					Type:        "signup",
					ExpiresAt:   time.Now().Add(5 * time.Minute),
					SessionData: sessionDataBytes,
				},
			},
		}

		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/signup/finish", nil, nil, webAuthnSignupFinishRequest{
			Username:  "alice",
			Challenge: webauthnSignupFixtureChallenge,
			Response:  responseMap,
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(handler.HandleFinishWebAuthnSignupLift(ctx))

		var body webAuthnSignupFinishResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.PasskeyRegistrationProof)
	})
}
