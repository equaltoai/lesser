package handlers

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

var oauthDeviceUserCodePattern = regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)

func normalizeOAuthDeviceUserCode(input string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(input))
	trimmed = strings.ReplaceAll(trimmed, " ", "")

	withoutHyphen := strings.ReplaceAll(trimmed, "-", "")
	if len(withoutHyphen) == 8 {
		return withoutHyphen[:4] + "-" + withoutHyphen[4:]
	}

	return trimmed
}

func validateOAuthDeviceUserCode(userCode string) bool {
	userCode = strings.TrimSpace(userCode)
	if userCode == "" {
		return false
	}
	return oauthDeviceUserCodePattern.MatchString(userCode)
}

// HandleOAuthDeviceVerifyLift handles POST /oauth/device/verify.
//
// This endpoint is used by the hosted auth UI (verification_uri) to look up the pending device session
// by user_code and display the requesting app + scopes before the user approves.
func (h *Handler) HandleOAuthDeviceVerifyLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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

	userCode := normalizeOAuthDeviceUserCode(params["user_code"])
	if !validateOAuthDeviceUserCode(userCode) {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "invalid user_code",
		})
	}

	session, err := h.repos.Account().GetOAuthDeviceSessionByUserCode(ctx.Context(), userCode)
	if err != nil || session == nil {
		// Avoid leaking existence information.
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "invalid or expired user_code",
		})
	}

	now := time.Now().UTC()
	if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "expired_token",
			ErrorDescription: "the user_code has expired",
		})
	}

	status := strings.ToLower(strings.TrimSpace(session.Status))
	switch status {
	case "pending", "approved", "denied":
		// allowed
	case "consumed":
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "the user_code has already been used",
		})
	default:
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "invalid device session state",
		})
	}

	clientName := session.ClientID
	clientURL := ""
	if client, err := h.repos.Account().GetOAuthClient(ctx.Context(), session.ClientID); err == nil && client != nil {
		if v := strings.TrimSpace(client.Name); v != "" {
			clientName = v
		}
		clientURL = strings.TrimSpace(client.Website)
	} else if err != nil {
		h.logger.Warn("failed to load oauth client for device session",
			zap.Error(err),
			zap.String("client_id", session.ClientID),
		)
	}

	expiresIn := 0
	if !session.ExpiresAt.IsZero() {
		remaining := time.Until(session.ExpiresAt)
		if remaining > 0 {
			expiresIn = int(remaining.Seconds())
		}
	}

	intervalSeconds := session.IntervalSeconds
	if intervalSeconds <= 0 {
		intervalSeconds = oauthDevicePollIntervalSeconds
	}

	return okJSON(apimodels.OAuthDeviceVerifyResponse{
		UserCode:   userCode,
		ClientID:   session.ClientID,
		ClientName: clientName,
		ClientURL:  clientURL,
		Scopes:     session.Scopes,
		Status:     status,
		ExpiresIn:  expiresIn,
		Interval:   intervalSeconds,
	})
}

// HandleOAuthDeviceConsentLift handles POST /oauth/device/consent.
//
// This endpoint requires authentication (wallet/WebAuthn) and is used by the auth UI to approve/deny a device session.
func (h *Handler) HandleOAuthDeviceConsentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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

	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
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

	userCode := normalizeOAuthDeviceUserCode(params["user_code"])
	if !validateOAuthDeviceUserCode(userCode) {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "invalid user_code",
		})
	}

	action := strings.ToLower(strings.TrimSpace(params["action"]))
	if action != "approve" && action != "deny" {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "invalid action",
		})
	}

	session, err := h.repos.Account().GetOAuthDeviceSessionByUserCode(ctx.Context(), userCode)
	if err != nil || session == nil {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "invalid or expired user_code",
		})
	}

	now := time.Now().UTC()
	if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "expired_token",
			ErrorDescription: "the user_code has expired",
		})
	}

	status := strings.ToLower(strings.TrimSpace(session.Status))
	if status == "consumed" {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "the user_code has already been used",
		})
	}

	switch action {
	case "approve":
		session.Status = "approved"
		session.ApprovedUsername = strings.TrimSpace(claims.Username)
		session.ApprovedAt = now
	case "deny":
		session.Status = "denied"
		session.DeniedAt = now
	}

	if err := h.repos.Account().UpdateOAuthDeviceSession(ctx.Context(), session); err != nil {
		h.logger.Error("failed to update oauth device session consent",
			zap.Error(err),
			zap.String("client_id", session.ClientID),
		)
		return apptheory.JSON(http.StatusInternalServerError, apimodels.OAuthErrorResponse{
			Error:            "server_error",
			ErrorDescription: "unable to update device authorization",
		})
	}

	return okJSON(apimodels.OAuthDeviceConsentResponse{
		Status:  session.Status,
		Message: "device authorization updated",
	})
}
