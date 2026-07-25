package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

const (
	oauthDeviceCodeTTLSeconds      = 10 * 60
	oauthDevicePollIntervalSeconds = 10
)

// HandleOAuthDeviceCodeLift handles POST /oauth/device/code.
//
// This is a headless-friendly OAuth bootstrap flow used by the Lesser CLI.
// Wallet authentication remains in the web UI; the CLI polls /oauth/token using the device_code grant.
func (h *Handler) HandleOAuthDeviceCodeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || ctx == nil {
		return common.RespondInternalServerError(ctx)
	}
	if h.cfg == nil || !h.cfg.AllowDeviceFlow {
		return apptheory.JSON(http.StatusForbidden, apimodels.OAuthErrorResponse{
			Error:            "access_denied",
			ErrorDescription: "device authorization is disabled",
		})
	}

	if h.repos == nil || h.repos.Account() == nil {
		return apptheory.JSON(http.StatusServiceUnavailable, apimodels.OAuthErrorResponse{
			Error:            "server_error",
			ErrorDescription: "storage is not initialized",
		})
	}

	if err := common.ValidateSliceNotEmpty("request_body", ctx.Request.Body); err != nil {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "empty request body",
		})
	}

	params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
	if err != nil {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "unable to parse form data",
		})
	}

	clientID := strings.TrimSpace(params["client_id"])
	if err := common.ValidateRequiredParam("client_id", clientID); err != nil {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "client_id is required",
		})
	}

	// Validate that the client exists (public clients are allowed; secret is not required here).
	client, err := h.repos.Account().GetOAuthClient(ctx.Context(), clientID)
	if err != nil || client == nil {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_client",
			ErrorDescription: "unknown client",
		})
	}
	if !oauthClientSupportsGrantType(client, oauthDeviceCodeGrantType) {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "unauthorized_client",
			ErrorDescription: "device_code is not allowed for this client",
		})
	}

	scopes, err := h.normalizeAuthorizeScopes(params["scope"])
	if err != nil {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_scope",
			ErrorDescription: err.Error(),
		})
	}

	deviceCode, err := generateOAuthDeviceCode()
	if err != nil {
		h.logger.Error("failed to generate device_code", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	userCode, err := generateOAuthUserCode()
	if err != nil {
		h.logger.Error("failed to generate user_code", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	now := time.Now().UTC()
	session := &storage.OAuthDeviceSession{
		DeviceCodeHash:  oauthDeviceCodeHash(deviceCode),
		UserCode:        userCode,
		ClientID:        clientID,
		Scopes:          scopes,
		Status:          oauthDeviceSessionStatusPending,
		IntervalSeconds: oauthDevicePollIntervalSeconds,
		PollCount:       0,
		CreatedAt:       now,
		UpdatedAt:       now,
		ExpiresAt:       oauthDeviceCodeExpiresAt(now),
	}

	if err := h.repos.Account().CreateOAuthDeviceSession(ctx.Context(), session); err != nil {
		// Never log the device_code or user_code; the device_code is a secret, and the user_code is short.
		h.logger.Error("failed to create oauth device session",
			zap.Error(err),
			zap.String("client_id", clientID),
		)
		return apptheory.JSON(http.StatusInternalServerError, apimodels.OAuthErrorResponse{
			Error:            "server_error",
			ErrorDescription: "unable to start device authorization",
		})
	}

	base := strings.TrimRight(h.cfg.BaseURL(), "/")
	verificationURI := base + "/auth/device"

	verificationComplete := verificationURI
	if encoded := url.QueryEscape(userCode); encoded != "" {
		verificationComplete = verificationURI + "?user_code=" + encoded
	}

	return okJSON(apimodels.OAuthDeviceCodeResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationComplete,
		ExpiresIn:               oauthDeviceCodeTTLSeconds,
		Interval:                oauthDevicePollIntervalSeconds,
	})
}

func generateOAuthDeviceCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateOAuthUserCode() (string, error) {
	// Avoid ambiguous characters for manual entry.
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	out := make([]byte, 0, 9)
	for i := 0; i < 8; i++ {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, alphabet[int(b[i])%len(alphabet)])
	}
	return string(out), nil
}

func oauthDeviceCodeHash(deviceCode string) string {
	sum := sha256.Sum256([]byte(deviceCode))
	return hex.EncodeToString(sum[:])
}

// oauthDeviceCodeExpiresAt returns the TTL cutoff for device sessions.
//
// NOTE: this is only used once we persist device sessions (M1); it is kept here so the constants stay close.
func oauthDeviceCodeExpiresAt(now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	return now.Add(time.Duration(oauthDeviceCodeTTLSeconds) * time.Second)
}
