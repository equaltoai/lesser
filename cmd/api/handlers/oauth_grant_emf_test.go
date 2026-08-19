package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/observability"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

type oauthGrantEMFPayload struct {
	AWS          observability.EMFMetadata `json:"_aws"`
	Outcome      string                    `json:"outcome"`
	ReasonCode   string                    `json:"reason_code"`
	RetryPath    string                    `json:"retry_path"`
	HTTPStatus   string                    `json:"http_status"`
	GrantType    string                    `json:"grant_type"`
	DetailReason string                    `json:"detail_reason"`
	RequestID    string                    `json:"request_id"`
	ClientHash   string                    `json:"client_id_hash"`
	ResourceHash string                    `json:"resource_hash"`
	FamilyHash   string                    `json:"family_id_hash"`
	Outcomes     int                       `json:"oauth_grant_outcomes_total"`
	Exceptions   int                       `json:"oauth_grant_exceptions_total"`
}

func decodeOAuthGrantEMFLines(t *testing.T, raw string) []oauthGrantEMFPayload {
	t.Helper()
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	result := make([]oauthGrantEMFPayload, 0, len(lines))
	for _, line := range lines {
		var payload oauthGrantEMFPayload
		require.NoError(t, json.Unmarshal([]byte(line), &payload))
		result = append(result, payload)
	}
	return result
}

func TestOAuthGrantEMFContractClampsDimensionsAndKeepsCorrelationAsProperties(t *testing.T) {
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", map[string]string{"X-Request-ID": "request-123"}, nil, nil)
	ctx.RequestID = "lambda-request-123"
	var output bytes.Buffer
	h := &Handler{oauthGrantEMFWriter: &output, oauthGrantEMFEnabled: true, logger: round10TestLogger(t)}
	telemetry := &oauthGrantTelemetry{
		GrantType:    oauthGrantTypeRefreshToken,
		ClientID:     "client-secret-id",
		Resource:     "https://api.example.com/mcp/alice",
		FamilyID:     "family-sensitive-id",
		DetailReason: "future_unmapped_reason",
		RetryPath:    "future_retry_path",
	}

	h.emitOAuthGrantOutcome(ctx, responseWithStatus{Status: http.StatusBadRequest}.response(), nil, telemetry)

	lines := decodeOAuthGrantEMFLines(t, output.String())
	require.Len(t, lines, 1)
	payload := lines[0]
	require.Equal(t, oauthGrantOutcomeFailure, payload.Outcome)
	require.Equal(t, oauthGrantReasonOther, payload.ReasonCode)
	require.Equal(t, "future_unmapped_reason", payload.DetailReason)
	require.Equal(t, oauthGrantRetryPathNone, payload.RetryPath)
	require.Equal(t, "400", payload.HTTPStatus)
	require.Equal(t, "lambda-request-123", payload.RequestID)
	require.Equal(t, oauthGrantTelemetryHash(telemetry.ClientID), payload.ClientHash)
	require.Equal(t, oauthGrantTelemetryHash(telemetry.Resource), payload.ResourceHash)
	require.Equal(t, oauthGrantTelemetryHash(telemetry.FamilyID), payload.FamilyHash)
	require.NotContains(t, output.String(), telemetry.ClientID)
	require.NotContains(t, output.String(), telemetry.FamilyID)
	require.Equal(t, 1, payload.Outcomes)
	require.Zero(t, payload.Exceptions)

	require.Len(t, payload.AWS.CloudWatchMetrics, 1)
	directive := payload.AWS.CloudWatchMetrics[0]
	require.Equal(t, oauthGrantEMFNamespace, directive.Namespace)
	require.Equal(t, [][]string{{"outcome", "reason_code", "retry_path", "http_status"}, {}}, directive.Dimensions)
	require.Equal(t, []observability.EMFMetricDefinition{
		{Name: oauthGrantEMFOutcomeMetric, Unit: "Count"},
		{Name: oauthGrantEMFExceptionMetric, Unit: "Count"},
	}, directive.Metrics)
	for _, dimensionSet := range directive.Dimensions {
		require.NotContains(t, dimensionSet, "request_id")
		require.NotContains(t, dimensionSet, "client_id_hash")
		require.NotContains(t, dimensionSet, "resource_hash")
		require.NotContains(t, dimensionSet, "family_id_hash")
	}
}

func TestOAuthGrantEMFExceptionMetricCountsOnlyRescueServes(t *testing.T) {
	var output bytes.Buffer
	h := &Handler{oauthGrantEMFWriter: &output, oauthGrantEMFEnabled: true}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, nil)

	h.emitOAuthGrantOutcome(ctx, responseWithStatus{Status: http.StatusOK}.response(), nil, &oauthGrantTelemetry{
		GrantType: oauthGrantTypeRefreshToken, DetailReason: oauthGrantReasonSuccess, RetryPath: oauthGrantRetryPathDirectRotate,
	})
	h.emitOAuthGrantOutcome(ctx, responseWithStatus{Status: http.StatusOK}.response(), nil, &oauthGrantTelemetry{
		GrantType: oauthGrantTypeRefreshToken, DetailReason: oauthGrantReasonRefreshRetryRescueServed, RetryPath: oauthGrantRetryPathRetryRescue,
	})

	lines := decodeOAuthGrantEMFLines(t, output.String())
	require.Len(t, lines, 2)
	require.Zero(t, lines[0].Exceptions)
	require.Equal(t, 1, lines[1].Exceptions)
	require.Equal(t, oauthGrantReasonRefreshRetryRescueServed, lines[1].ReasonCode)
}

func TestOAuthGrantEMFClosedReasonVocabulary(t *testing.T) {
	reasons := []string{
		oauthGrantReasonSuccess,
		oauthGrantReasonInvalidRequest,
		oauthGrantReasonInvalidClient,
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
		oauthGrantReasonRefreshRotationInfrastructure,
	}
	for _, reason := range reasons {
		require.Equal(t, reason, oauthGrantEMFReasonCode(reason))
	}
	require.Equal(t, oauthGrantReasonOther, oauthGrantEMFReasonCode("not_declared"))
}

func TestOAuthGrantEMFTransientOutcomeUsesRetryableClass(t *testing.T) {
	var output bytes.Buffer
	h := &Handler{oauthGrantEMFWriter: &output, oauthGrantEMFEnabled: true}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, nil)

	h.emitOAuthGrantOutcome(ctx, responseWithStatus{Status: http.StatusServiceUnavailable}.response(), nil, &oauthGrantTelemetry{
		GrantType: oauthGrantTypeAuthorizationCode,
	})

	lines := decodeOAuthGrantEMFLines(t, output.String())
	require.Len(t, lines, 1)
	require.Equal(t, oauthGrantOutcomeFailure, lines[0].Outcome)
	require.Equal(t, oauthGrantReasonTemporarilyUnavailable, lines[0].ReasonCode)
	require.Equal(t, "503", lines[0].HTTPStatus)
	require.Zero(t, lines[0].Exceptions)
}

func TestOAuthGrantEMFGatesAndNilWriterAreSilent(t *testing.T) {
	t.Setenv(oauthGrantEMFEnabledEnv, "false")
	require.False(t, oauthGrantEMFEnabled())
	var output bytes.Buffer
	disabled := &Handler{}
	configureOAuthGrantEMF(disabled)
	require.False(t, disabled.oauthGrantEMFEnabled)
	disabled.oauthGrantEMFWriter = &output
	disabled.emitOAuthGrantOutcome(
		round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, nil),
		responseWithStatus{Status: http.StatusOK}.response(), nil,
		&oauthGrantTelemetry{GrantType: oauthGrantTypeRefreshToken},
	)
	require.Empty(t, output.String())

	t.Setenv(oauthGrantEMFEnabledEnv, "true")
	require.True(t, oauthGrantEMFEnabled())
	t.Setenv(oauthGrantEMFEnabledEnv, "not-a-bool")
	require.True(t, oauthGrantEMFEnabled())

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, nil)
	telemetry := &oauthGrantTelemetry{GrantType: oauthGrantTypeAuthorizationCode}
	(&Handler{oauthGrantEMFEnabled: true}).emitOAuthGrantOutcome(ctx, responseWithStatus{Status: http.StatusOK}.response(), nil, telemetry)

	output.Reset()
	(&Handler{oauthGrantEMFWriter: &output, oauthGrantEMFEnabled: false}).emitOAuthGrantOutcome(ctx, responseWithStatus{Status: http.StatusOK}.response(), nil, telemetry)
	require.Empty(t, output.String())
}

type failingOAuthGrantEMFWriter struct{}

func (failingOAuthGrantEMFWriter) Write([]byte) (int, error) {
	return 0, errors.New("stdout unavailable")
}

func TestOAuthGrantEMFFailingWriterDoesNotAlterGrantResponse(t *testing.T) {
	h, _, _ := round11NewHandler(t, &round10QueryState{})
	request := []byte("grant_type=refresh_token&client_id=client-1")
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, request)

	h.oauthGrantEMFWriter = nil
	expected := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
	h.oauthGrantEMFWriter = failingOAuthGrantEMFWriter{}
	h.oauthGrantEMFEnabled = true
	actual := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, request)))

	require.Equal(t, expected.Status, actual.Status)
	require.Equal(t, expected.Headers, actual.Headers)
	require.JSONEq(t, string(expected.Body), string(actual.Body))
}

func TestOAuthTokenGrantArmsEmitExactlyOneOutcome(t *testing.T) {
	h, _, _ := round11NewHandler(t, &round10QueryState{})
	var output bytes.Buffer
	h.oauthGrantEMFWriter = &output
	h.oauthGrantEMFEnabled = true

	requests := [][]byte{
		[]byte("grant_type=authorization_code&client_id=client-1&redirect_uri=https%3A%2F%2Fclient.example%2Fcallback"),
		[]byte("grant_type=refresh_token&client_id=client-1"),
	}
	for _, request := range requests {
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, request)))
		requireOAuthTokenError(t, resp.Status, "invalid_request", resp.Body)
	}

	lines := decodeOAuthGrantEMFLines(t, output.String())
	require.Len(t, lines, 2)
	require.Equal(t, oauthGrantTypeAuthorizationCode, lines[0].GrantType)
	require.Equal(t, oauthGrantTypeRefreshToken, lines[1].GrantType)
	for _, payload := range lines {
		require.Equal(t, oauthGrantReasonInvalidRequest, payload.ReasonCode)
		require.Equal(t, 1, payload.Outcomes)
	}
}

func TestOAuthRefreshRescueEmitsExceptionOutcome(t *testing.T) {
	now := time.Now().UTC()
	stale := oauthRefreshReliabilityToken("telemetry-stale", "client-1", now)
	stale.Current = false
	stale.Revoked = true
	stale.RevokedAt = now.Add(-time.Second)
	stale.RevokedReason = "rotated"
	stale.ReplacedByHash = ""
	head := oauthRefreshReliabilityToken("telemetry-head", "client-1", now)
	head.Generation = 2
	require.NoError(t, head.UpdateKeys())
	state := &round10QueryState{
		oauthClientsByID:     map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityClient("client-1")},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{stale.Token: stale, head.Token: head},
		disableAuditRepo:     true,
	}
	h, _, _ := round11NewHandler(t, state)
	var output bytes.Buffer
	h.oauthGrantEMFWriter = &output
	h.oauthGrantEMFEnabled = true

	request := []byte("grant_type=refresh_token&refresh_token=telemetry-stale&client_id=client-1")
	resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, request)))
	require.NotEmpty(t, resp.Body)

	lines := decodeOAuthGrantEMFLines(t, output.String())
	require.Len(t, lines, 1)
	require.Equal(t, oauthGrantOutcomeSuccess, lines[0].Outcome)
	require.Equal(t, oauthGrantReasonRefreshRetryRescueServed, lines[0].ReasonCode)
	require.Equal(t, oauthGrantRetryPathRetryRescue, lines[0].RetryPath)
	require.Equal(t, 1, lines[0].Exceptions)
	require.NotEmpty(t, lines[0].FamilyHash)
}

func TestOAuthGrantTelemetryMapsRefreshTerminalAndTransientReasons(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name      string
		status    int
		reason    string
		clientID  string
		resource  string
		refresh   string
		configure func(*round10QueryState)
	}{
		{
			name: "authoritative absence", status: http.StatusBadRequest,
			reason: oauthGrantReasonRefreshTokenAbsent, clientID: "client-1", refresh: "missing",
			configure: func(state *round10QueryState) {
				state.notFoundPKs = map[string]bool{"REFRESHTOKEN#missing": true}
			},
		},
		{
			name: "expired", status: http.StatusBadRequest,
			reason: oauthGrantReasonRefreshTokenExpired, clientID: "client-1", refresh: "expired",
			configure: func(state *round10QueryState) {
				token := oauthRefreshReliabilityToken("expired", "client-1", now)
				token.ExpiresAt = now.Add(-time.Second)
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{token.Token: token}
			},
		},
		{
			name: "outside retry grace", status: http.StatusBadRequest,
			reason: oauthGrantReasonRefreshOutsideRetryGrace, clientID: "client-1", refresh: "stale",
			configure: func(state *round10QueryState) {
				token := oauthRefreshReliabilityToken("stale", "client-1", now)
				token.Current = false
				token.Revoked = true
				token.RevokedAt = now.Add(-time.Minute)
				token.RevokedReason = standardRefreshRevokedReasonRotated
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{token.Token: token}
			},
		},
		{
			name: "resource mismatch", status: http.StatusBadRequest,
			reason: oauthGrantReasonRefreshResourceMismatch, clientID: "client-1", refresh: "resource-bound",
			resource: "https://api.example.com/mcp/bob",
			configure: func(state *round10QueryState) {
				token := oauthRefreshReliabilityToken("resource-bound", "client-1", now)
				token.Resource = "https://api.example.com/mcp/alice"
				require.NoError(t, token.UpdateKeys())
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{token.Token: token}
			},
		},
		{
			name: "cross-client containment", status: http.StatusBadRequest,
			reason: oauthGrantReasonRefreshCrossClientReplay, clientID: "client-2", refresh: "cross-client",
			configure: func(state *round10QueryState) {
				token := oauthRefreshReliabilityToken("cross-client", "client-1", now)
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{token.Token: token}
			},
		},
		{
			name: "storage fault", status: http.StatusServiceUnavailable,
			reason: oauthGrantReasonTemporarilyUnavailable, clientID: "client-1", refresh: "fault",
			configure: func(state *round10QueryState) {
				state.firstErrorByType = map[string]error{"*models.RefreshToken": errors.New("storage unavailable")}
			},
		},
		{
			name: "direct success", status: http.StatusOK,
			reason: oauthGrantReasonSuccess, clientID: "client-1", refresh: "active",
			configure: func(state *round10QueryState) {
				token := oauthRefreshReliabilityToken("active", "client-1", now)
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{token.Token: token}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &round10QueryState{
				oauthClientsByID: map[string]storagemodels.OAuthClient{
					"client-1": oauthRefreshReliabilityClient("client-1"),
					"client-2": oauthRefreshReliabilityClient("client-2"),
				},
				disableAuditRepo: true,
			}
			tc.configure(state)
			h, _, _ := round11NewHandler(t, state)
			var output bytes.Buffer
			h.oauthGrantEMFWriter = &output
			h.oauthGrantEMFEnabled = true

			body := "grant_type=refresh_token&refresh_token=" + tc.refresh + "&client_id=" + tc.clientID
			if tc.resource != "" {
				body += "&resource=" + url.QueryEscape(tc.resource)
			}
			resp := requireStatus(t, tc.status)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(body))))
			require.NotEmpty(t, resp.Body)

			lines := decodeOAuthGrantEMFLines(t, output.String())
			require.Len(t, lines, 1)
			require.Equal(t, tc.reason, lines[0].ReasonCode)
			require.Equal(t, 1, lines[0].Outcomes)
			require.Zero(t, lines[0].Exceptions)
			if tc.status == http.StatusOK {
				require.NotEmpty(t, lines[0].FamilyHash)
			}
		})
	}
}

// responseWithStatus keeps the focused EMF tests independent of response-body
// construction while still exercising the production response-status reader.
type responseWithStatus struct{ Status int }

func (r responseWithStatus) response() *apptheory.Response {
	return &apptheory.Response{Status: r.Status}
}

var _ io.Writer = failingOAuthGrantEMFWriter{}
