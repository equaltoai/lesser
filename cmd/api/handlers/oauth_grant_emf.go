package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/observability"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

const (
	oauthGrantEMFNamespace       = "Lesser/OAuth"
	oauthGrantEMFOutcomeMetric   = "oauth_grant_outcomes_total"
	oauthGrantEMFExceptionMetric = "oauth_grant_exceptions_total"
	oauthGrantEMFEnabledEnv      = "LESSER_OAUTH_GRANT_EMF_ENABLED"
	oauthGrantUnknown            = "unknown"

	oauthGrantOutcomeSuccess = "success"
	oauthGrantOutcomeFailure = "failure"

	oauthGrantRetryPathNone         = "none"
	oauthGrantRetryPathDirectRotate = "direct_rotate"
	oauthGrantRetryPathRetryRescue  = "retry_rescue"

	oauthGrantReasonSuccess                           = "success"
	oauthGrantReasonInvalidRequest                    = "invalid_request"
	oauthGrantReasonInvalidClient                     = "invalid_client"
	oauthGrantReasonInvalidTarget                     = "invalid_target"
	oauthGrantReasonUnauthorizedClient                = "unauthorized_client"
	oauthGrantReasonTemporarilyUnavailable            = "temporarily_unavailable"
	oauthGrantReasonServerError                       = "server_error"
	oauthGrantReasonAuthorizationCodeAbsent           = "authorization_code_absent"
	oauthGrantReasonAuthorizationCodeClientMismatch   = "authorization_code_client_mismatch"
	oauthGrantReasonAuthorizationCodeRedirectMismatch = "authorization_code_redirect_mismatch"
	oauthGrantReasonAuthorizationCodePKCEMismatch     = "authorization_code_pkce_mismatch"
	oauthGrantReasonAuthorizationCodeScopeInvalid     = "authorization_code_scope_invalid"
	oauthGrantReasonAuthorizationCodeResourceMismatch = "authorization_code_resource_mismatch"
	oauthGrantReasonAuthorizationCodeAuthorityRevoked = "authorization_code_authority_revoked"
	oauthGrantReasonAuthorizationCodeInvalidContext   = "authorization_code_invalid_context"
	oauthGrantReasonAuthorizationCodeConsumed         = "authorization_code_already_consumed"
	oauthGrantReasonTokenGenerationFailed             = "token_generation_failed"
	oauthGrantReasonRefreshTokenAbsent                = "refresh_token_absent"
	oauthGrantReasonRefreshCrossClientReplay          = "refresh_cross_client_replay"
	oauthGrantReasonRefreshShareGrantRevoked          = "refresh_share_grant_revoked"
	oauthGrantReasonRefreshRetryReplayed              = "refresh_retry_replayed"
	oauthGrantReasonRefreshTokenExpired               = "refresh_token_expired"
	oauthGrantReasonRefreshStaleGeneration            = "refresh_stale_generation"
	oauthGrantReasonRefreshResourceMismatch           = "refresh_resource_mismatch"
	oauthGrantReasonRefreshResourceAuthorityRevoked   = "refresh_resource_authority_revoked"
	oauthGrantReasonRefreshOutsideRetryGrace          = "refresh_outside_retry_grace"
	oauthGrantReasonRefreshSuccessorAbsent            = "refresh_successor_absent"
	oauthGrantReasonRefreshAuthorityAbsent            = "refresh_authority_absent"
	oauthGrantReasonRefreshRetryRescueServed          = "refresh_retry_rescue_served"
	oauthGrantReasonRefreshRuntimeInvalid             = "refresh_runtime_invalid"
	oauthGrantReasonRefreshRuntimeReuse               = "refresh_runtime_reuse"
	oauthGrantReasonRefreshRotationInfrastructure     = "refresh_rotation_infrastructure"
	oauthGrantReasonOther                             = "other"
)

type oauthGrantTelemetry struct {
	GrantType    string
	ClientID     string
	Resource     string
	FamilyID     string
	DetailReason string
	RetryPath    string
}

func newOAuthGrantTelemetry(req *oauthTokenRequest) *oauthGrantTelemetry {
	telemetry := &oauthGrantTelemetry{RetryPath: oauthGrantRetryPathNone}
	if req == nil {
		return telemetry
	}
	telemetry.GrantType = strings.TrimSpace(req.grantType)
	telemetry.ClientID = strings.TrimSpace(req.clientID)
	telemetry.Resource = strings.TrimSpace(req.resource)
	return telemetry
}

func oauthGrantTelemetryFromRequest(ctx *apptheory.Context) *oauthGrantTelemetry {
	if ctx == nil || len(ctx.Request.Body) == 0 {
		return nil
	}
	params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
	if err != nil {
		return nil
	}
	grantType := strings.TrimSpace(params["grant_type"])
	if grantType != oauthGrantTypeAuthorizationCode && grantType != oauthGrantTypeRefreshToken {
		return nil
	}
	return &oauthGrantTelemetry{
		GrantType: grantType,
		ClientID:  strings.TrimSpace(params["client_id"]),
		Resource:  strings.TrimSpace(params["resource"]),
		RetryPath: oauthGrantRetryPathNone,
	}
}

func (t *oauthGrantTelemetry) setReason(reason string) {
	if t != nil && strings.TrimSpace(reason) != "" {
		t.DetailReason = strings.TrimSpace(reason)
	}
}

func (t *oauthGrantTelemetry) setResourceIfEmpty(resource string) {
	if t != nil && strings.TrimSpace(t.Resource) == "" {
		t.Resource = strings.TrimSpace(resource)
	}
}

func setOAuthGrantReasonFromError(telemetry *oauthGrantTelemetry, err error) {
	if telemetry == nil || strings.TrimSpace(telemetry.DetailReason) != "" || err == nil {
		return
	}
	switch {
	case errors.Is(err, auth.ErrOAuthTemporarilyUnavailable):
		telemetry.setReason(oauthGrantReasonTemporarilyUnavailable)
	case errors.Is(err, auth.ErrInvalidClient):
		telemetry.setReason(oauthGrantReasonInvalidClient)
	case errors.Is(err, auth.ErrUnauthorizedClient):
		telemetry.setReason(oauthGrantReasonUnauthorizedClient)
	case errors.Is(err, auth.ErrInvalidRequest):
		telemetry.setReason(oauthGrantReasonInvalidRequest)
	case errors.Is(err, auth.ErrInvalidCodeChallenge):
		telemetry.setReason(oauthGrantReasonAuthorizationCodePKCEMismatch)
	case errors.Is(err, auth.ErrInvalidScope):
		telemetry.setReason(oauthGrantReasonAuthorizationCodeScopeInvalid)
	case errors.Is(err, errOAuthInvalidTarget):
		telemetry.setReason(oauthGrantReasonAuthorizationCodeResourceMismatch)
	default:
		telemetry.setReason(oauthGrantReasonServerError)
	}
}

func oauthGrantReasonFromTokenErrorResponse(response *apptheory.Response) string {
	reason := oauthGrantReasonInvalidRequest
	if response == nil || len(response.Body) == 0 {
		return reason
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return reason
	}
	if wireReason := strings.TrimSpace(payload.Error); wireReason != "" {
		return wireReason
	}
	return reason
}

func oauthGrantEMFReasonCode(reason string) string {
	switch strings.TrimSpace(reason) {
	case oauthGrantReasonSuccess,
		oauthGrantReasonInvalidRequest,
		oauthGrantReasonInvalidClient,
		oauthGrantReasonInvalidTarget,
		oauthGrantReasonUnauthorizedClient,
		oauthGrantReasonTemporarilyUnavailable,
		oauthGrantReasonServerError,
		oauthGrantReasonAuthorizationCodeAbsent,
		oauthGrantReasonAuthorizationCodeClientMismatch,
		oauthGrantReasonAuthorizationCodeRedirectMismatch,
		oauthGrantReasonAuthorizationCodePKCEMismatch,
		oauthGrantReasonAuthorizationCodeScopeInvalid,
		oauthGrantReasonAuthorizationCodeResourceMismatch,
		oauthGrantReasonAuthorizationCodeAuthorityRevoked,
		oauthGrantReasonAuthorizationCodeInvalidContext,
		oauthGrantReasonAuthorizationCodeConsumed,
		oauthGrantReasonTokenGenerationFailed,
		oauthGrantReasonRefreshTokenAbsent,
		oauthGrantReasonRefreshCrossClientReplay,
		oauthGrantReasonRefreshShareGrantRevoked,
		oauthGrantReasonRefreshRetryReplayed,
		oauthGrantReasonRefreshTokenExpired,
		oauthGrantReasonRefreshStaleGeneration,
		oauthGrantReasonRefreshResourceMismatch,
		oauthGrantReasonRefreshResourceAuthorityRevoked,
		oauthGrantReasonRefreshOutsideRetryGrace,
		oauthGrantReasonRefreshSuccessorAbsent,
		oauthGrantReasonRefreshAuthorityAbsent,
		oauthGrantReasonRefreshRetryRescueServed,
		oauthGrantReasonRefreshRuntimeInvalid,
		oauthGrantReasonRefreshRuntimeReuse,
		oauthGrantReasonRefreshRotationInfrastructure:
		return strings.TrimSpace(reason)
	default:
		return oauthGrantReasonOther
	}
}

func oauthGrantEMFRetryPath(retryPath string) string {
	switch strings.TrimSpace(retryPath) {
	case oauthGrantRetryPathDirectRotate, oauthGrantRetryPathRetryRescue:
		return strings.TrimSpace(retryPath)
	default:
		return oauthGrantRetryPathNone
	}
}

func oauthGrantExceptionReason(reason string) bool {
	return strings.TrimSpace(reason) == oauthGrantReasonRefreshRetryRescueServed
}

func oauthGrantEMFEnabled() bool {
	raw := strings.TrimSpace(os.Getenv(oauthGrantEMFEnabledEnv))
	if raw == "" {
		return true
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return enabled
}

func configureOAuthGrantEMF(h *Handler) {
	if h == nil {
		return
	}
	h.oauthGrantEMFWriter = os.Stdout
	h.oauthGrantEMFEnabled = oauthGrantEMFEnabled()
}

func oauthGrantResponseStatus(resp *apptheory.Response, err error) int {
	if resp != nil && resp.Status > 0 {
		return resp.Status
	}
	if err != nil {
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}

func oauthGrantRequestID(ctx *apptheory.Context) string {
	if ctx == nil {
		return oauthGrantUnknown
	}
	if requestID := strings.TrimSpace(ctx.RequestID); requestID != "" {
		return requestID
	}
	if requestID := strings.TrimSpace(headerValue(ctx, "X-Request-ID")); requestID != "" {
		return requestID
	}
	return oauthGrantUnknown
}

func oauthGrantTelemetryHash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func (h *Handler) emitOAuthGrantOutcome(ctx *apptheory.Context, resp *apptheory.Response, responseErr error, telemetry *oauthGrantTelemetry) {
	if h == nil || telemetry == nil || !h.oauthGrantEMFEnabled || h.oauthGrantEMFWriter == nil {
		return
	}
	if telemetry.GrantType != oauthGrantTypeAuthorizationCode && telemetry.GrantType != oauthGrantTypeRefreshToken {
		return
	}

	status := oauthGrantResponseStatus(resp, responseErr)
	outcome := oauthGrantOutcomeFailure
	reason := strings.TrimSpace(telemetry.DetailReason)
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		outcome = oauthGrantOutcomeSuccess
		if reason == "" {
			reason = oauthGrantReasonSuccess
		}
	} else if reason == "" {
		if status == http.StatusServiceUnavailable {
			reason = oauthGrantReasonTemporarilyUnavailable
		} else {
			reason = oauthGrantReasonOther
		}
	}

	exception := 0
	if oauthGrantExceptionReason(reason) {
		exception = 1
	}
	dimensions := []string{"outcome", "reason_code", "retry_path", "http_status"}
	payload := map[string]any{
		"_aws": observability.EMFMetadata{
			Timestamp: time.Now().UnixMilli(),
			CloudWatchMetrics: []observability.EMFCloudWatchMetrics{{
				Namespace:  oauthGrantEMFNamespace,
				Dimensions: [][]string{dimensions, {}},
				Metrics: []observability.EMFMetricDefinition{
					{Name: oauthGrantEMFOutcomeMetric, Unit: "Count"},
					{Name: oauthGrantEMFExceptionMetric, Unit: "Count"},
				},
			}},
		},
		"outcome":                    outcome,
		"reason_code":                oauthGrantEMFReasonCode(reason),
		"retry_path":                 oauthGrantEMFRetryPath(telemetry.RetryPath),
		"http_status":                strconv.Itoa(status),
		"grant_type":                 telemetry.GrantType,
		"detail_reason":              reason,
		"request_id":                 oauthGrantRequestID(ctx),
		"client_id_hash":             oauthGrantTelemetryHash(telemetry.ClientID),
		"resource_hash":              oauthGrantTelemetryHash(telemetry.Resource),
		"family_id_hash":             oauthGrantTelemetryHash(telemetry.FamilyID),
		oauthGrantEMFOutcomeMetric:   1,
		oauthGrantEMFExceptionMetric: exception,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if _, err := io.WriteString(h.oauthGrantEMFWriter, string(encoded)+"\n"); err != nil && h.logger != nil {
		h.logger.Warn("failed to emit OAuth grant outcome metric", zap.Error(err))
	}
}
