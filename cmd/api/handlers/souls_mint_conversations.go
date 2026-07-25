package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

const (
	soulPrivateMintConversationDefaultLimit = 20
	soulPrivateMintConversationMaxLimit     = 50
	soulPrivateMintConversationIDMaxLen     = 128
	soulPrivateMintConversationListMaxBytes = 1 * 1024 * 1024
	soulPrivateMintConversationGetMaxBytes  = 2 * 1024 * 1024
)

var soulPrivateMintConversationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type soulPrivateHostReadResult struct {
	status  int
	headers http.Header
	body    []byte
}

// HandleListBoundSoulMintConversationsLift returns compact private mint-conversation metadata
// for the authenticated local principal's bound soul.
func (h *Handler) HandleListBoundSoulMintConversationsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	start := time.Now()
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	limit, resp := parseSoulPrivateMintConversationLimit(ctx)
	if resp != nil {
		h.logSoulPrivateRead(ctx, claims.Username, "", "", "list_mint_conversations", "denied", resp.Status, 0, limit, false, soulPrivateErrorCode(resp), 0, start)
		return resp, nil
	}
	if resp := validateSoulPrivateReadQuery(ctx, map[string]struct{}{"limit": {}}); resp != nil {
		h.logSoulPrivateRead(ctx, claims.Username, "", "", "list_mint_conversations", "denied", resp.Status, 0, limit, soulPrivateCursorPresent(ctx), "SOUL_PRIVATE_QUERY_UNSUPPORTED", 0, start)
		return resp, nil
	}
	if len(ctx.Request.Body) > 0 {
		resp, _ := respondSoulPrivateError(ctx, http.StatusBadRequest, "private soul read requests must not include a body", "SOUL_PRIVATE_REQUEST_BODY_UNSUPPORTED")
		h.logSoulPrivateRead(ctx, claims.Username, "", "", "list_mint_conversations", "denied", resp.Status, 0, limit, false, "SOUL_PRIVATE_REQUEST_BODY_UNSUPPORTED", 0, start)
		return resp, nil
	}

	soul, resp := h.resolveSoulPrivateBoundAgent(ctx, claims.Username)
	if resp != nil {
		h.logSoulPrivateRead(ctx, claims.Username, "", "", "list_mint_conversations", "denied", resp.Status, 0, limit, false, soulPrivateErrorCode(resp), 0, start)
		return resp, nil
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	result, resp := h.getSoulPrivateHostRead(ctx, soul.AgentID, "", query, soulPrivateMintConversationListMaxBytes)
	if resp != nil {
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, "", "list_mint_conversations", "host_unavailable", resp.Status, 0, limit, false, soulPrivateErrorCode(resp), 0, start)
		return resp, nil
	}
	if result.status != http.StatusOK {
		resp, _ := h.translateSoulPrivateHostError(ctx, "list_mint_conversations", result.status, result.headers)
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, "", "list_mint_conversations", "denied", resp.Status, len(result.body), limit, false, soulPrivateErrorCode(resp), result.status, start)
		return resp, nil
	}

	var payload apimodels.SoulMintConversationsResponse
	if err := json.Unmarshal(result.body, &payload); err != nil {
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host response is unavailable", "SOUL_PRIVATE_HOST_RESPONSE_INVALID")
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, "", "list_mint_conversations", "host_unavailable", resp.Status, len(result.body), limit, false, "SOUL_PRIVATE_HOST_RESPONSE_INVALID", result.status, start)
		return resp, nil
	}
	if payload.Version == "" {
		payload.Version = "1"
	}
	for _, conversation := range payload.Conversations {
		if !sameSoulAgentID(conversation.AgentID, soul.AgentID) {
			resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host response is unavailable", "SOUL_PRIVATE_UPSTREAM_SCOPE_MISMATCH")
			h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, conversation.ConversationID, "list_mint_conversations", "denied", resp.Status, len(result.body), limit, false, "SOUL_PRIVATE_UPSTREAM_SCOPE_MISMATCH", result.status, start)
			return resp, nil
		}
	}

	resp, err = okJSON(payload)
	if err == nil {
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, "", "list_mint_conversations", "success", resp.Status, len(result.body), limit, false, "", result.status, start)
	}
	return resp, err
}

// HandleGetBoundSoulMintConversationLift returns one bounded private mint-conversation record
// for the authenticated local principal's bound soul.
func (h *Handler) HandleGetBoundSoulMintConversationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	start := time.Now()
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	conversationID, resp := validateSoulPrivateConversationID(ctx)
	if resp != nil {
		h.logSoulPrivateRead(ctx, claims.Username, "", "", "get_mint_conversation", "denied", resp.Status, 0, 0, false, soulPrivateErrorCode(resp), 0, start)
		return resp, nil
	}
	if resp := validateSoulPrivateReadQuery(ctx, nil); resp != nil {
		h.logSoulPrivateRead(ctx, claims.Username, "", conversationID, "get_mint_conversation", "denied", resp.Status, 0, 0, soulPrivateCursorPresent(ctx), "SOUL_PRIVATE_QUERY_UNSUPPORTED", 0, start)
		return resp, nil
	}
	if len(ctx.Request.Body) > 0 {
		resp, _ := respondSoulPrivateError(ctx, http.StatusBadRequest, "private soul read requests must not include a body", "SOUL_PRIVATE_REQUEST_BODY_UNSUPPORTED")
		h.logSoulPrivateRead(ctx, claims.Username, "", conversationID, "get_mint_conversation", "denied", resp.Status, 0, 0, false, "SOUL_PRIVATE_REQUEST_BODY_UNSUPPORTED", 0, start)
		return resp, nil
	}

	soul, resp := h.resolveSoulPrivateBoundAgent(ctx, claims.Username)
	if resp != nil {
		h.logSoulPrivateRead(ctx, claims.Username, "", conversationID, "get_mint_conversation", "denied", resp.Status, 0, 0, false, soulPrivateErrorCode(resp), 0, start)
		return resp, nil
	}

	result, resp := h.getSoulPrivateHostRead(ctx, soul.AgentID, conversationID, nil, soulPrivateMintConversationGetMaxBytes)
	if resp != nil {
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, conversationID, "get_mint_conversation", "host_unavailable", resp.Status, 0, 0, false, soulPrivateErrorCode(resp), 0, start)
		return resp, nil
	}
	if result.status != http.StatusOK {
		resp, _ := h.translateSoulPrivateHostError(ctx, "get_mint_conversation", result.status, result.headers)
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, conversationID, "get_mint_conversation", "denied", resp.Status, len(result.body), 0, false, soulPrivateErrorCode(resp), result.status, start)
		return resp, nil
	}

	var payload apimodels.SoulMintConversationResponse
	if err := json.Unmarshal(result.body, &payload); err != nil {
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host response is unavailable", "SOUL_PRIVATE_HOST_RESPONSE_INVALID")
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, conversationID, "get_mint_conversation", "host_unavailable", resp.Status, len(result.body), 0, false, "SOUL_PRIVATE_HOST_RESPONSE_INVALID", result.status, start)
		return resp, nil
	}
	if payload.Version == "" {
		payload.Version = "1"
	}
	if !sameSoulAgentID(payload.Conversation.AgentID, soul.AgentID) || strings.TrimSpace(payload.Conversation.ConversationID) != conversationID {
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host response is unavailable", "SOUL_PRIVATE_UPSTREAM_SCOPE_MISMATCH")
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, conversationID, "get_mint_conversation", "denied", resp.Status, len(result.body), 0, false, "SOUL_PRIVATE_UPSTREAM_SCOPE_MISMATCH", result.status, start)
		return resp, nil
	}

	resp, err = okJSON(payload)
	if err == nil {
		h.logSoulPrivateRead(ctx, claims.Username, soul.AgentID, conversationID, "get_mint_conversation", "success", resp.Status, len(result.body), 0, false, "", result.status, start)
	}
	return resp, err
}

func parseSoulPrivateMintConversationLimit(ctx *apptheory.Context) (int, *apptheory.Response) {
	limit, err := common.ParseAndValidateIntWithBounds("limit", firstQueryValue(ctx, "limit"), 0, soulPrivateMintConversationMaxLimit, soulPrivateMintConversationDefaultLimit)
	if err == nil {
		return limit, nil
	}
	resp, _ := respondSoulPrivateError(ctx, http.StatusBadRequest, "limit must be between 1 and 50", "SOUL_PRIVATE_LIMIT_INVALID")
	return 0, resp
}

func validateSoulPrivateReadQuery(ctx *apptheory.Context, allowed map[string]struct{}) *apptheory.Response {
	for key := range ctx.Request.Query {
		normalized := strings.TrimSpace(key)
		if normalized == "" {
			continue
		}
		if strings.EqualFold(normalized, "cursor") {
			resp, _ := respondSoulPrivateError(ctx, http.StatusBadRequest, "cursor is not supported for private soul conversation reads", "SOUL_PRIVATE_CURSOR_UNSUPPORTED")
			return resp
		}
		if _, ok := allowed[normalized]; !ok {
			resp, _ := respondSoulPrivateError(ctx, http.StatusBadRequest, "unsupported private soul conversation query parameter", "SOUL_PRIVATE_QUERY_UNSUPPORTED")
			return resp
		}
	}
	return nil
}

func validateSoulPrivateConversationID(ctx *apptheory.Context) (string, *apptheory.Response) {
	conversationID := strings.TrimSpace(ctx.Param("conversationId"))
	if conversationID == "" {
		resp, _ := respondSoulPrivateError(ctx, http.StatusBadRequest, "conversationId is required", "SOUL_PRIVATE_CONVERSATION_ID_REQUIRED")
		return "", resp
	}
	if len(conversationID) > soulPrivateMintConversationIDMaxLen || !soulPrivateMintConversationIDPattern.MatchString(conversationID) {
		resp, _ := respondSoulPrivateError(ctx, http.StatusBadRequest, "conversationId must be an opaque safe path value", "SOUL_PRIVATE_CONVERSATION_ID_INVALID")
		return "", resp
	}
	return conversationID, nil
}

func (h *Handler) resolveSoulPrivateBoundAgent(ctx *apptheory.Context, username string) (*soulservice.Soul, *apptheory.Response) {
	svc := h.getSoulService()
	if svc == nil {
		resp, _ := common.RespondInternalServerError(ctx)
		return nil, resp
	}

	soul, err := svc.ResolveBoundAgent(ctx.Context(), username)
	if err != nil {
		switch {
		case errors.Is(err, soulservice.ErrTrustNotConfigured):
			resp, _ := respondSoulPrivateError(ctx, http.StatusUnprocessableEntity, "private soul trust is not configured", "SOUL_PRIVATE_TRUST_NOT_CONFIGURED")
			return nil, resp
		case errors.Is(err, soulservice.ErrSoulNotAvailable):
			resp, _ := respondSoulPrivateError(ctx, http.StatusNotFound, "bound soul is not available", "SOUL_BOUND_AGENT_NOT_AVAILABLE")
			return nil, resp
		default:
			resp, _ := common.RespondInternalServerError(ctx)
			return nil, resp
		}
	}
	if soul == nil || !soul.Bound || strings.TrimSpace(soul.AgentID) == "" {
		resp, _ := respondSoulPrivateError(ctx, http.StatusNotFound, "no bound soul for authenticated principal", "SOUL_BOUND_AGENT_NOT_FOUND")
		return nil, resp
	}
	return soul, nil
}

func (h *Handler) getSoulPrivateHostRead(ctx *apptheory.Context, agentID string, conversationID string, query url.Values, maxBytes int64) (*soulPrivateHostReadResult, *apptheory.Response) {
	base := h.effectiveLesserHostTrustBaseURL(ctx.Context())
	if base == "" {
		h.warnTrustProxyMisconfigured("private soul read misconfigured: missing lesser-host base URL", zap.String("route_class", soulPrivateRouteClass(conversationID)))
		resp, _ := respondSoulPrivateError(ctx, http.StatusUnprocessableEntity, "private soul trust is not configured", "SOUL_PRIVATE_TRUST_NOT_CONFIGURED")
		return nil, resp
	}
	if err := validateTrustBaseURL(base); err != nil {
		h.warnTrustProxyMisconfigured("private soul read misconfigured: invalid lesser-host base URL", zap.Error(err), zap.String("route_class", soulPrivateRouteClass(conversationID)))
		resp, _ := respondSoulPrivateError(ctx, http.StatusUnprocessableEntity, "private soul trust is not configured", "SOUL_PRIVATE_TRUST_NOT_CONFIGURED")
		return nil, resp
	}

	instanceKey, err := h.effectiveLesserHostInstanceKey(ctx.Context())
	if err != nil {
		h.warnTrustProxyMisconfigured("private soul read misconfigured: failed to resolve instance key", zap.Error(err), zap.String("route_class", soulPrivateRouteClass(conversationID)))
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host is unavailable", "SOUL_PRIVATE_HOST_UNAVAILABLE")
		return nil, resp
	}
	if strings.TrimSpace(instanceKey) == "" {
		h.warnTrustProxyMisconfigured("private soul read misconfigured: missing instance key", zap.String("route_class", soulPrivateRouteClass(conversationID)))
		resp, _ := respondSoulPrivateError(ctx, http.StatusUnprocessableEntity, "private soul trust is not configured", "SOUL_PRIVATE_TRUST_NOT_CONFIGURED")
		return nil, resp
	}

	upstreamURL, err := soulPrivateHostURL(base, agentID, conversationID, query)
	if err != nil {
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host is unavailable", "SOUL_PRIVATE_HOST_UNAVAILABLE")
		return nil, resp
	}
	if err := validateLesserHostProxyURL(upstreamURL); err != nil {
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host is unavailable", "SOUL_PRIVATE_HOST_UNAVAILABLE")
		return nil, resp
	}

	req, err := http.NewRequestWithContext(ctx.Context(), http.MethodGet, upstreamURL.String(), nil)
	if err != nil {
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host is unavailable", "SOUL_PRIVATE_HOST_UNAVAILABLE")
		return nil, resp
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+instanceKey)

	upstreamResp, err := newLesserHostProxyClient().Do(req)
	if err != nil {
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host is unavailable", "SOUL_PRIVATE_HOST_UNAVAILABLE")
		return nil, resp
	}
	defer func() { _ = upstreamResp.Body.Close() }()

	body, truncated, err := common.ReadUntrustedHTTPResponseBody(upstreamResp.Body, maxBytes)
	if err != nil {
		resp, _ := respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host is unavailable", "SOUL_PRIVATE_HOST_UNAVAILABLE")
		return nil, resp
	}
	if truncated {
		resp, _ := respondSoulPrivateError(ctx, http.StatusRequestEntityTooLarge, "private soul conversation response is too large", "SOUL_PRIVATE_RESPONSE_TOO_LARGE")
		return nil, resp
	}

	return &soulPrivateHostReadResult{status: upstreamResp.StatusCode, headers: upstreamResp.Header, body: body}, nil
}

func soulPrivateHostURL(base string, agentID string, conversationID string, query url.Values) (*url.URL, error) {
	path := "/api/v1/soul/instance/agents/" + url.PathEscape(agentID) + "/mint-conversations"
	if strings.TrimSpace(conversationID) != "" {
		path += "/" + url.PathEscape(conversationID)
	}
	upstreamURL, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		upstreamURL.RawQuery = query.Encode()
	}
	return upstreamURL, nil
}

func (h *Handler) translateSoulPrivateHostError(ctx *apptheory.Context, operation string, upstreamStatus int, upstreamHeaders http.Header) (*apptheory.Response, error) {
	switch upstreamStatus {
	case http.StatusBadRequest:
		return respondSoulPrivateError(ctx, http.StatusBadRequest, "invalid private soul conversation request", "SOUL_PRIVATE_INVALID_REQUEST")
	case http.StatusUnauthorized, http.StatusForbidden:
		return respondSoulPrivateError(ctx, http.StatusConflict, "private soul instance trust was rejected", "SOUL_PRIVATE_INSTANCE_TRUST_REJECTED")
	case http.StatusNotFound:
		if operation == "get_mint_conversation" {
			return respondSoulPrivateError(ctx, http.StatusNotFound, "private soul conversation not found", "SOUL_PRIVATE_CONVERSATION_NOT_FOUND")
		}
		return respondSoulPrivateError(ctx, http.StatusNotFound, "bound soul is not available", "SOUL_BOUND_AGENT_NOT_AVAILABLE")
	case http.StatusRequestEntityTooLarge:
		return respondSoulPrivateError(ctx, http.StatusRequestEntityTooLarge, "private soul conversation response is too large", "SOUL_PRIVATE_RESPONSE_TOO_LARGE")
	case http.StatusTooManyRequests:
		resp, err := respondSoulPrivateError(ctx, http.StatusTooManyRequests, "private soul conversation read rate limited", "SOUL_PRIVATE_RATE_LIMITED")
		if retryAfter := strings.TrimSpace(upstreamHeaders.Get("Retry-After")); retryAfter != "" {
			if resp.Headers == nil {
				resp.Headers = map[string][]string{}
			}
			resp.Headers["retry-after"] = []string{retryAfter}
		}
		return resp, err
	default:
		return respondSoulPrivateError(ctx, http.StatusServiceUnavailable, "private soul conversation host is unavailable", "SOUL_PRIVATE_HOST_UNAVAILABLE")
	}
}

func respondSoulPrivateError(_ *apptheory.Context, status int, message string, code string) (*apptheory.Response, error) {
	return apptheory.JSON(status, common.StandardErrorResponse{
		Error:       message,
		Description: message,
		Code:        code,
	})
}

func firstQueryValue(ctx *apptheory.Context, key string) string {
	if ctx == nil || ctx.Request.Query == nil {
		return ""
	}
	for queryKey, values := range ctx.Request.Query {
		if !strings.EqualFold(queryKey, key) || len(values) == 0 {
			continue
		}
		return strings.TrimSpace(values[0])
	}
	return ""
}

func soulPrivateCursorPresent(ctx *apptheory.Context) bool {
	if ctx == nil || ctx.Request.Query == nil {
		return false
	}
	for key := range ctx.Request.Query {
		if strings.EqualFold(strings.TrimSpace(key), "cursor") {
			return true
		}
	}
	return false
}

func sameSoulAgentID(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func soulPrivateRouteClass(conversationID string) string {
	if strings.TrimSpace(conversationID) == "" {
		return "list_mint_conversations"
	}
	return "get_mint_conversation"
}

func (h *Handler) logSoulPrivateRead(ctx *apptheory.Context, username string, agentID string, conversationID string, operation string, outcome string, status int, responseBytes int, limit int, cursorPresent bool, errorCode string, upstreamStatus int, start time.Time) {
	if h == nil || h.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("event", "soul.private_read"),
		zap.String("request_id", strings.TrimSpace(ctx.RequestID)),
		zap.String("actor_username_hash", soulPrivateHash(username)),
		zap.String("agent_id_hash", soulPrivateHash(agentID)),
		zap.String("conversation_id_hash", soulPrivateHash(conversationID)),
		zap.String("operation", operation),
		zap.Bool("self_scope_verified", strings.TrimSpace(agentID) != ""),
		zap.String("host_route_class", operation),
		zap.String("outcome", outcome),
		zap.Int("status", status),
		zap.Int("upstream_status", upstreamStatus),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.Int("response_bytes", responseBytes),
		zap.Int("limit", limit),
		zap.Bool("cursor_present", cursorPresent),
	}
	if strings.TrimSpace(errorCode) != "" {
		fields = append(fields, zap.String("error_code", errorCode))
	}
	h.logger.Info("soul private read", fields...)
}

func soulPrivateHash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if len(encoded) > 16 {
		return encoded[:16]
	}
	return encoded
}

func soulPrivateErrorCode(resp *apptheory.Response) string {
	if resp == nil || len(resp.Body) == 0 {
		return ""
	}
	var body common.StandardErrorResponse
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.Code)
}
