package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const (
	lesserHostProxyTimeout          = 15 * time.Second
	lesserHostProxyMaxRequestBytes  = 1 * 1024 * 1024  // 1MB
	lesserHostProxyMaxResponseBytes = 10 * 1024 * 1024 // 10MB
)

type lesserHostProxyTarget struct {
	method string
	// baseURL is required and must not have a trailing slash.
	baseURL string
	// path must start with a leading slash.
	path string

	// accept sets the Accept header for the upstream request.
	// Defaults to application/json when empty.
	accept string

	// If set, require user auth with this scope.
	requiredScope string

	// If true, send instance auth to lesser-host (server-side only).
	useInstanceAuth bool

	// If true, attempt to rewrite embedded lesser-host URLs to instance proxy URLs.
	rewriteResponseURLs bool
}

func (h *Handler) lesserHostTrustBaseURL() string {
	if h == nil || h.cfg == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(h.cfg.LesserHostURL), "/")
}

func (h *Handler) lesserHostAttestationsBaseURL() string {
	if h == nil || h.cfg == nil {
		return ""
	}
	if v := strings.TrimRight(strings.TrimSpace(h.cfg.LesserHostAttestationsURL), "/"); v != "" {
		return v
	}
	return h.lesserHostTrustBaseURL()
}

func (h *Handler) lesserHostInstanceKey() string {
	if h == nil || h.cfg == nil {
		return ""
	}
	return strings.TrimSpace(h.cfg.LesserHostInstanceKey)
}

func (h *Handler) proxyToLesserHost(ctx *apptheory.Context, target lesserHostProxyTarget) (*apptheory.Response, error) {
	if h == nil || ctx == nil {
		return common.RespondInternalServerError(ctx)
	}

	base := strings.TrimRight(strings.TrimSpace(target.baseURL), "/")
	if base == "" {
		h.warnTrustProxyMisconfigured("trust proxy misconfigured: missing LESSER_HOST_URL", zap.String("upstream_path", target.path))
		return common.RespondServiceUnavailable(ctx, "lesser-host")
	}
	if !strings.HasPrefix(base, "https://") && !strings.HasPrefix(base, "http://") {
		h.warnTrustProxyMisconfigured("trust proxy misconfigured: LESSER_HOST_URL must include scheme (https://)", zap.String("lesser_host_url", base))
		return common.RespondServiceUnavailable(ctx, "lesser-host")
	}
	if !isValidLesserHostProxyTarget(target, base) {
		return common.RespondServiceUnavailable(ctx, "lesser-host")
	}

	if err := h.ensureLesserHostProxyScope(ctx, target.requiredScope); err != nil {
		return nil, err
	}

	instanceKey, resp := h.resolveLesserHostProxyInstanceKey(ctx, target)
	if resp != nil {
		return resp, nil
	}

	upstreamURL, resp := buildLesserHostProxyURL(ctx, base, target.path)
	if resp != nil {
		return resp, nil
	}

	body, resp := buildLesserHostProxyBody(ctx)
	if resp != nil {
		return resp, nil
	}

	req, resp := buildLesserHostProxyRequest(ctx, target, upstreamURL, body, instanceKey)
	if resp != nil {
		return resp, nil
	}

	status, upstreamHeaders, respBody, resp := executeLesserHostProxyRequest(ctx, req)
	if resp != nil {
		return resp, nil
	}

	return buildLesserHostProxyResponse(target, status, upstreamHeaders, respBody), nil
}

func (h *Handler) ensureLesserHostProxyScope(ctx *apptheory.Context, requiredScope string) error {
	requiredScope = strings.TrimSpace(requiredScope)
	if requiredScope == "" {
		return nil
	}
	_, err := h.authenticateWithScope(ctx, requiredScope)
	return err
}

func isValidLesserHostProxyTarget(target lesserHostProxyTarget, base string) bool {
	if base == "" {
		return false
	}
	if strings.TrimSpace(target.method) == "" {
		return false
	}
	return strings.HasPrefix(target.path, "/")
}

func (h *Handler) resolveLesserHostProxyInstanceKey(ctx *apptheory.Context, target lesserHostProxyTarget) (string, *apptheory.Response) {
	if !target.useInstanceAuth {
		return "", nil
	}

	key := h.lesserHostInstanceKey()
	if key == "" {
		h.warnTrustProxyMisconfigured("trust proxy misconfigured: missing LESSER_HOST_INSTANCE_KEY/LESSER_HOST_INSTANCE_KEY_ARN", zap.String("upstream_path", target.path))
		resp, _ := common.RespondServiceUnavailable(ctx, "lesser-host")
		return "", resp
	}
	return key, nil
}

func (h *Handler) warnTrustProxyMisconfigured(message string, fields ...zap.Field) {
	if h == nil || h.logger == nil {
		return
	}
	h.logger.Warn(message, fields...)
}

func buildLesserHostProxyURL(ctx *apptheory.Context, base, path string) (*url.URL, *apptheory.Response) {
	upstreamURL, err := url.Parse(base + path)
	if err != nil {
		resp, _ := common.RespondServiceUnavailable(ctx, "lesser-host")
		return nil, resp
	}

	q := upstreamURL.Query()
	appendLesserHostProxyQuery(q, ctx.Request.Query)
	upstreamURL.RawQuery = q.Encode()
	return upstreamURL, nil
}

func appendLesserHostProxyQuery(dst url.Values, src map[string][]string) {
	for k, values := range src {
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

func buildLesserHostProxyBody(ctx *apptheory.Context) (io.Reader, *apptheory.Response) {
	if len(ctx.Request.Body) == 0 {
		return nil, nil
	}
	if int64(len(ctx.Request.Body)) > lesserHostProxyMaxRequestBytes {
		resp, _ := common.SendError(ctx, http.StatusRequestEntityTooLarge, "request body too large")
		return nil, resp
	}
	return bytes.NewReader(ctx.Request.Body), nil
}

func buildLesserHostProxyRequest(ctx *apptheory.Context, target lesserHostProxyTarget, upstreamURL *url.URL, body io.Reader, instanceKey string) (*http.Request, *apptheory.Response) {
	req, err := http.NewRequestWithContext(ctx.Context(), target.method, upstreamURL.String(), body)
	if err != nil {
		resp, _ := common.RespondServiceUnavailable(ctx, "lesser-host")
		return nil, resp
	}

	if ct := firstHeaderValue(ctx.Request.Headers, "content-type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Accept", resolveLesserHostProxyAccept(target.accept))

	// Never forward the user token; always use instance auth when required.
	if instanceKey != "" {
		req.Header.Set("Authorization", "Bearer "+instanceKey)
	}

	return req, nil
}

func resolveLesserHostProxyAccept(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "application/json"
	}
	return value
}

func executeLesserHostProxyRequest(ctx *apptheory.Context, req *http.Request) (int, http.Header, []byte, *apptheory.Response) {
	client := &http.Client{Timeout: lesserHostProxyTimeout}
	upstreamResp, err := client.Do(req)
	if err != nil {
		resp, _ := common.RespondServiceUnavailable(ctx, "lesser-host")
		return 0, nil, nil, resp
	}
	defer func() { _ = upstreamResp.Body.Close() }()

	limited := io.LimitReader(upstreamResp.Body, lesserHostProxyMaxResponseBytes+1)
	respBody, err := io.ReadAll(limited)
	if err != nil || int64(len(respBody)) > lesserHostProxyMaxResponseBytes {
		resp, _ := common.RespondServiceUnavailable(ctx, "lesser-host")
		return 0, nil, nil, resp
	}

	return upstreamResp.StatusCode, upstreamResp.Header, respBody, nil
}

func buildLesserHostProxyResponse(target lesserHostProxyTarget, status int, upstreamHeaders http.Header, body []byte) *apptheory.Response {
	headers := map[string][]string{}
	copyHeaderIfPresent(headers, upstreamHeaders, "content-type")
	copyHeaderIfPresent(headers, upstreamHeaders, "cache-control")
	copyHeaderIfPresent(headers, upstreamHeaders, "etag")
	copyHeaderIfPresent(headers, upstreamHeaders, "last-modified")

	normalizeLesserHostProxyContentType(headers, body)
	body = maybeRewriteLesserHostProxyResponse(target, headers, body)

	return &apptheory.Response{
		Status:  status,
		Headers: headers,
		Body:    body,
	}
}

func normalizeLesserHostProxyContentType(headers map[string][]string, body []byte) {
	if len(headers["content-type"]) != 0 {
		return
	}
	if looksLikeJSON(body) {
		headers["content-type"] = []string{"application/json; charset=utf-8"}
	}
}

func maybeRewriteLesserHostProxyResponse(target lesserHostProxyTarget, headers map[string][]string, body []byte) []byte {
	if !target.rewriteResponseURLs || len(body) == 0 {
		return body
	}

	isJSON := isJSONContentType(headers["content-type"])
	if !isJSON && !looksLikeJSON(body) {
		return body
	}

	rewritten, ok := rewriteLesserHostURLsToInstanceProxy(body)
	if !ok {
		return body
	}

	if !isJSON {
		headers["content-type"] = []string{"application/json; charset=utf-8"}
	}
	return rewritten
}

func isJSONContentType(values []string) bool {
	for _, v := range values {
		v = strings.ToLower(strings.TrimSpace(v))
		if strings.HasPrefix(v, "application/json") {
			return true
		}
	}
	return false
}

func firstHeaderValue(headers map[string][]string, name string) string {
	if headers == nil {
		return ""
	}
	values := headers[strings.ToLower(strings.TrimSpace(name))]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func copyHeaderIfPresent(dst map[string][]string, src http.Header, name string) {
	if dst == nil || src == nil {
		return
	}
	values := src.Values(name)
	if len(values) == 0 {
		return
	}
	key := strings.ToLower(strings.TrimSpace(name))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return
	}
	dst[key] = out
}

func looksLikeJSON(body []byte) bool {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', '[':
			return true
		default:
			return false
		}
	}
	return false
}

func rewriteLesserHostURLsToInstanceProxy(body []byte) ([]byte, bool) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, false
	}

	changed := rewriteURLsInJSON(&v)
	if !changed {
		return nil, false
	}

	rewritten, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return rewritten, true
}

func rewriteURLsInJSON(v *any) bool {
	switch x := (*v).(type) {
	case map[string]any:
		changed := false

		attID, _ := x["attestation_id"].(string)
		attID = strings.TrimSpace(attID)
		if attID != "" {
			x["attestation_url"] = "/api/v1/trust/attestations/" + attID
			changed = true
		}

		imageID, _ := x["image_id"].(string)
		imageID = strings.TrimSpace(imageID)
		if imageID != "" {
			x["image_url"] = "/api/v1/trust/previews/images/" + imageID
			changed = true
		}

		renderID, _ := x["render_id"].(string)
		renderID = strings.TrimSpace(renderID)
		if renderID != "" {
			if _, ok := x["thumbnail_url"]; ok {
				x["thumbnail_url"] = "/api/v1/trust/renders/" + renderID + "/thumbnail"
				changed = true
			}
			if _, ok := x["snapshot_url"]; ok {
				x["snapshot_url"] = "/api/v1/trust/renders/" + renderID + "/snapshot"
				changed = true
			}
		}

		for k, child := range x {
			childAny := any(child)
			if rewriteURLsInJSON(&childAny) {
				x[k] = childAny
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i := range x {
			childAny := any(x[i])
			if rewriteURLsInJSON(&childAny) {
				x[i] = childAny
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

// Trust proxy endpoints (Issue #56).

// HandleTrustCreateLinkPreviewLift proxies POST /api/v1/trust/previews to lesser-host.
func (h *Handler) HandleTrustCreateLinkPreviewLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodPost,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/previews",
		requiredScope:       auth.ScopeWrite,
		useInstanceAuth:     true,
		rewriteResponseURLs: true,
	})
}

// HandleTrustGetLinkPreviewLift proxies GET /api/v1/trust/previews/{id} to lesser-host.
func (h *Handler) HandleTrustGetLinkPreviewLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	id := strings.TrimSpace(ctx.Param("id"))
	if id == "" {
		return common.RespondMissingParameter(ctx, "id")
	}
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/previews/" + url.PathEscape(id),
		requiredScope:       auth.ScopeRead,
		useInstanceAuth:     true,
		rewriteResponseURLs: true,
	})
}

// HandleTrustGetLinkPreviewImageLift proxies GET /api/v1/trust/previews/images/{imageId} to lesser-host.
// This endpoint is public (no user auth) so that image URLs can be loaded by <img> tags.
func (h *Handler) HandleTrustGetLinkPreviewImageLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	imageID := strings.TrimSpace(ctx.Param("imageId"))
	if imageID == "" {
		return common.RespondMissingParameter(ctx, "imageId")
	}
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/previews/images/" + url.PathEscape(imageID),
		accept:              "*/*",
		requiredScope:       "",
		useInstanceAuth:     false,
		rewriteResponseURLs: false,
	})
}

// HandleTrustCreatePublishJobLift proxies POST /api/v1/trust/publish/jobs to lesser-host.
func (h *Handler) HandleTrustCreatePublishJobLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodPost,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/publish/jobs",
		requiredScope:       auth.ScopeWrite,
		useInstanceAuth:     true,
		rewriteResponseURLs: true,
	})
}

// HandleTrustGetPublishJobLift proxies GET /api/v1/trust/publish/jobs/{jobId} to lesser-host.
func (h *Handler) HandleTrustGetPublishJobLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	jobID := strings.TrimSpace(ctx.Param("jobId"))
	if jobID == "" {
		return common.RespondMissingParameter(ctx, "jobId")
	}
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/publish/jobs/" + url.PathEscape(jobID),
		requiredScope:       auth.ScopeRead,
		useInstanceAuth:     true,
		rewriteResponseURLs: true,
	})
}

// HandleTrustCreateRenderLift proxies POST /api/v1/trust/renders to lesser-host.
func (h *Handler) HandleTrustCreateRenderLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodPost,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/renders",
		requiredScope:       auth.ScopeWrite,
		useInstanceAuth:     true,
		rewriteResponseURLs: true,
	})
}

// HandleTrustGetRenderLift proxies GET /api/v1/trust/renders/{renderId} to lesser-host.
func (h *Handler) HandleTrustGetRenderLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	renderID := strings.TrimSpace(ctx.Param("renderId"))
	if renderID == "" {
		return common.RespondMissingParameter(ctx, "renderId")
	}
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/renders/" + url.PathEscape(renderID),
		requiredScope:       auth.ScopeRead,
		useInstanceAuth:     true,
		rewriteResponseURLs: true,
	})
}

// HandleTrustGetRenderThumbnailLift proxies GET /api/v1/trust/renders/{renderId}/thumbnail to lesser-host.
// This endpoint is public (no user auth) so thumbnail URLs can be loaded by <img> tags.
func (h *Handler) HandleTrustGetRenderThumbnailLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	renderID := strings.TrimSpace(ctx.Param("renderId"))
	if renderID == "" {
		return common.RespondMissingParameter(ctx, "renderId")
	}
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/renders/" + url.PathEscape(renderID) + "/thumbnail",
		accept:              "*/*",
		requiredScope:       "",
		useInstanceAuth:     false,
		rewriteResponseURLs: false,
	})
}

// HandleTrustGetRenderSnapshotLift proxies GET /api/v1/trust/renders/{renderId}/snapshot to lesser-host.
func (h *Handler) HandleTrustGetRenderSnapshotLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	renderID := strings.TrimSpace(ctx.Param("renderId"))
	if renderID == "" {
		return common.RespondMissingParameter(ctx, "renderId")
	}
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/renders/" + url.PathEscape(renderID) + "/snapshot",
		accept:              "*/*",
		requiredScope:       auth.ScopeRead,
		useInstanceAuth:     true,
		rewriteResponseURLs: false,
	})
}

// HandleTrustAIClaimVerifyLift proxies POST /api/v1/trust/ai/claims/verify to lesser-host.
func (h *Handler) HandleTrustAIClaimVerifyLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodPost,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/ai/claims/verify",
		requiredScope:       auth.ScopeWrite,
		useInstanceAuth:     true,
		rewriteResponseURLs: true,
	})
}

// HandleTrustGetAIJobLift proxies GET /api/v1/trust/ai/jobs/{jobId} to lesser-host.
func (h *Handler) HandleTrustGetAIJobLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	jobID := strings.TrimSpace(ctx.Param("jobId"))
	if jobID == "" {
		return common.RespondMissingParameter(ctx, "jobId")
	}
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostTrustBaseURL(),
		path:                "/api/v1/ai/jobs/" + url.PathEscape(jobID),
		requiredScope:       auth.ScopeRead,
		useInstanceAuth:     true,
		rewriteResponseURLs: true,
	})
}

// HandleTrustJWKSJSONLift proxies GET /api/v1/trust/jwks.json to lesser-host.
func (h *Handler) HandleTrustJWKSJSONLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostAttestationsBaseURL(),
		path:                "/.well-known/jwks.json",
		requiredScope:       auth.ScopeRead,
		useInstanceAuth:     false,
		rewriteResponseURLs: false,
	})
}

// HandleTrustLookupAttestationLift proxies GET /api/v1/trust/attestations to lesser-host.
func (h *Handler) HandleTrustLookupAttestationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostAttestationsBaseURL(),
		path:                "/attestations",
		requiredScope:       auth.ScopeRead,
		useInstanceAuth:     false,
		rewriteResponseURLs: false,
	})
}

// HandleTrustGetAttestationLift proxies GET /api/v1/trust/attestations/{id} to lesser-host.
func (h *Handler) HandleTrustGetAttestationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	id := strings.TrimSpace(ctx.Param("id"))
	if id == "" {
		return common.RespondMissingParameter(ctx, "id")
	}
	return h.proxyToLesserHost(ctx, lesserHostProxyTarget{
		method:              http.MethodGet,
		baseURL:             h.lesserHostAttestationsBaseURL(),
		path:                "/attestations/" + url.PathEscape(id),
		requiredScope:       auth.ScopeRead,
		useInstanceAuth:     false,
		rewriteResponseURLs: false,
	})
}
