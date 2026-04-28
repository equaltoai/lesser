package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func allowLocalLesserHostProxyForTests(t *testing.T) {
	t.Helper()

	prevValidate := validateLesserHostProxyURL
	prevClient := newLesserHostProxyClient
	validateLesserHostProxyURL = func(*url.URL) error { return nil }
	newLesserHostProxyClient = func() lesserHostProxyHTTPClient {
		return &http.Client{Timeout: lesserHostProxyTimeout}
	}
	t.Cleanup(func() {
		validateLesserHostProxyURL = prevValidate
		newLesserHostProxyClient = prevClient
	})
}

func TestLesserHostTrustProxyRound20(t *testing.T) {
	allowLocalLesserHostProxyForTests(t)

	t.Run("forwards_instance_auth_and_rewrites_attestation_url", func(t *testing.T) {
		const (
			instanceKey    = "instance-key-raw"
			attestationID  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			upstreamStatus = `{"status":"ok","cached":false,"job_id":"j1","attestation_id":"` + attestationID + `","attestation_url":"https://lesser.host/attestations/` + attestationID + `"}`
		)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/v1/ai/claims/verify", r.URL.Path)
			require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))

			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NotEmpty(t, body)

			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(upstreamStatus))
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/ai/claims/verify", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, []byte(`{"text":"hello","evidence":[{"source_id":"s1","text":"x"}]}`))

		resp := requireStatus(t, http.StatusOK)(h.HandleTrustAIClaimVerifyLift(ctx))

		var got map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &got))
		require.Equal(t, "/api/v1/trust/attestations/"+attestationID, got["attestation_url"])
	})

	t.Run("forwards_query_params_for_attestation_lookup", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "/attestations", r.URL.Path)

			q := r.URL.Query()
			require.Equal(t, "https://example.com/users/alice", q.Get("actor_uri"))
			require.Equal(t, "https://example.com/objects/1", q.Get("object_uri"))
			require.Equal(t, "0xdeadbeef", q.Get("content_hash"))
			require.Equal(t, "ai_claim_verify_llm", q.Get("module"))
			require.Equal(t, "v1", q.Get("policy_version"))

			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, _ = w.Write([]byte(`{"id":"x","jws":"y","payload":{}}`))
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostAttestationsURL = upstream.URL

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/attestations", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, map[string]string{
			"actor_uri":      "https://example.com/users/alice",
			"object_uri":     "https://example.com/objects/1",
			"content_hash":   "0xdeadbeef",
			"module":         "ai_claim_verify_llm",
			"policy_version": "v1",
		}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleTrustLookupAttestationLift(ctx))
		require.Equal(t, []string{"public, max-age=3600"}, resp.Headers["cache-control"])
	})

	t.Run("missing_instance_key_returns_409", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.LesserHostURL = "https://example.com"
		cfg.LesserHostInstanceKey = ""

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/ai/claims/verify", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, []byte(`{"text":"hello","evidence":[{"source_id":"s1","text":"x"}]}`))

		requireStatus(t, http.StatusConflict)(h.HandleTrustAIClaimVerifyLift(ctx))
	})

	t.Run("rewrites_image_and_render_urls_with_nested_payload", func(t *testing.T) {
		const (
			instanceKey    = "instance-key-raw"
			attestationID  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			imageID        = "img123"
			renderID       = "render123"
			upstreamStatus = `{"status":"ok","attestation_id":"` + attestationID + `","image_id":"` + imageID + `","render_id":"` + renderID + `","thumbnail_url":"https://lesser.host/renders/` + renderID + `/thumbnail","snapshot_url":"https://lesser.host/renders/` + renderID + `/snapshot","nested":[{"attestation_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}`
		)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/v1/renders", r.URL.Path)
			require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
			require.Equal(t, "application/json", r.Header.Get("Accept"))

			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(upstreamStatus))
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/renders", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, []byte(`{"url":"https://example.com"}`))

		resp := requireStatus(t, http.StatusOK)(h.HandleTrustCreateRenderLift(ctx))

		var got map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &got))
		require.Equal(t, "/api/v1/trust/attestations/"+attestationID, got["attestation_url"])
		require.Equal(t, "/api/v1/trust/previews/images/"+imageID, got["image_url"])
		require.Equal(t, "/api/v1/trust/renders/"+renderID+"/thumbnail", got["thumbnail_url"])
		require.Equal(t, "/api/v1/trust/renders/"+renderID+"/snapshot", got["snapshot_url"])

		nested, ok := got["nested"].([]any)
		require.True(t, ok)
		require.Len(t, nested, 1)
		nestedMap, ok := nested[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "/api/v1/trust/attestations/"+nestedMap["attestation_id"].(string), nestedMap["attestation_url"])
	})

	t.Run("adds_default_content_type_and_rewrites_when_upstream_omits_it", func(t *testing.T) {
		const (
			instanceKey   = "instance-key-raw"
			attestationID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/v1/ai/claims/verify", r.URL.Path)
			require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))

			// Intentionally omit Content-Type to exercise defaulting behavior.
			_, _ = w.Write([]byte(`{"attestation_id":"` + attestationID + `"}`))
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/ai/claims/verify", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, []byte(`{"text":"hello","evidence":[{"source_id":"s1","text":"x"}]}`))

		resp := requireStatus(t, http.StatusOK)(h.HandleTrustAIClaimVerifyLift(ctx))
		require.Equal(t, []string{"application/json; charset=utf-8"}, resp.Headers["content-type"])

		var got map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &got))
		require.Equal(t, "/api/v1/trust/attestations/"+attestationID, got["attestation_url"])
	})

	t.Run("preview_image_requires_user_auth_and_does_not_forward_it", func(t *testing.T) {
		const imageID = "img123"

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "/api/v1/previews/images/"+imageID, r.URL.Path)
			require.Empty(t, r.Header.Get("Authorization"))
			require.Equal(t, "*/*", r.Header.Get("Accept"))

			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xde, 0xad, 0xbe, 0xef})
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = ""

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/previews/images/"+imageID, nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["imageId"] = imageID

		proxyResp, err := h.HandleTrustGetLinkPreviewImageLift(ctx)
		require.Nil(t, proxyResp)
		require.Error(t, err)

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/trust/previews/images/"+imageID, map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["imageId"] = imageID

		resp := requireStatus(t, http.StatusOK)(h.HandleTrustGetLinkPreviewImageLift(ctx))
		require.Equal(t, []string{"image/jpeg"}, resp.Headers["content-type"])
		require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, resp.Body)
	})

	t.Run("request_body_too_large_returns_413", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.LesserHostURL = "https://example.com"
		cfg.LesserHostInstanceKey = "instance-key-raw"

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		body := make([]byte, lesserHostProxyMaxRequestBytes+1)
		for i := range body {
			body[i] = 'a'
		}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/previews", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, body)

		requireStatus(t, http.StatusRequestEntityTooLarge)(h.HandleTrustCreateLinkPreviewLift(ctx))
	})

	t.Run("missing_render_id_returns_400", func(t *testing.T) {
		cfg := round11TestConfig()
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/renders//thumbnail", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleTrustGetRenderThumbnailLift(ctx))
	})

	t.Run("missing_asset_ids_return_400_before_proxy", func(t *testing.T) {
		cfg := round11TestConfig()
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		cases := []struct {
			name    string
			path    string
			handler func(*apptheory.Context) (*apptheory.Response, error)
		}{
			{name: "preview", path: "/api/v1/trust/previews/", handler: h.HandleTrustGetLinkPreviewLift},
			{name: "preview_image", path: "/api/v1/trust/previews/images/", handler: h.HandleTrustGetLinkPreviewImageLift},
			{name: "publish_job", path: "/api/v1/trust/publish/jobs/", handler: h.HandleTrustGetPublishJobLift},
			{name: "render", path: "/api/v1/trust/renders/", handler: h.HandleTrustGetRenderLift},
			{name: "render_snapshot", path: "/api/v1/trust/renders//snapshot", handler: h.HandleTrustGetRenderSnapshotLift},
			{name: "attestation", path: "/api/v1/trust/attestations/", handler: h.HandleTrustGetAttestationLift},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, err := round10NewLiftContext(http.MethodGet, tc.path, nil, nil, nil)
				require.NoError(t, err)

				requireStatus(t, http.StatusBadRequest)(tc.handler(ctx))
			})
		}
	})

	t.Run("missing_job_id_returns_400", func(t *testing.T) {
		cfg := round11TestConfig()
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/ai/jobs/", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleTrustGetAIJobLift(ctx))
	})

	t.Run("create_and_get_preview_proxy_and_rewrite_image_url", func(t *testing.T) {
		const (
			instanceKey = "instance-key-raw"
			previewID   = "p1"
			imageID     = "img123"
		)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
			require.Equal(t, "application/json", r.Header.Get("Accept"))

			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/previews":
				// Use a whitespace content-type that will be ignored by the proxy copier.
				w.Header().Set("Content-Type", "   ")
				_, _ = w.Write([]byte(`{"preview_id":"` + previewID + `","image_id":"` + imageID + `"}`))
			case r.Method == http.MethodGet && r.URL.Path == "/api/v1/previews/"+previewID:
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"preview_id":"` + previewID + `","image_id":"` + imageID + `"}`))
			default:
				require.FailNow(t, "unexpected upstream request", "%s %s", r.Method, r.URL.Path)
			}
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		createCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/previews", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, []byte(`{"url":"https://example.com"}`))

		createResp := requireStatus(t, http.StatusOK)(h.HandleTrustCreateLinkPreviewLift(createCtx))
		require.Equal(t, []string{"application/json; charset=utf-8"}, createResp.Headers["content-type"])

		var created map[string]any
		require.NoError(t, json.Unmarshal(createResp.Body, &created))
		require.Equal(t, "/api/v1/trust/previews/images/"+imageID, created["image_url"])

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		getCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/previews/"+previewID, map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		getCtx.Params["id"] = previewID

		getResp := requireStatus(t, http.StatusOK)(h.HandleTrustGetLinkPreviewLift(getCtx))
		var got map[string]any
		require.NoError(t, json.Unmarshal(getResp.Body, &got))
		require.Equal(t, "/api/v1/trust/previews/images/"+imageID, got["image_url"])
	})

	t.Run("thumbnail_and_snapshot_require_user_auth_for_binary_responses", func(t *testing.T) {
		const (
			instanceKey = "instance-key-raw"
			renderID    = "render123"
		)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "*/*", r.Header.Get("Accept"))

			switch r.URL.Path {
			case "/api/v1/renders/" + renderID + "/thumbnail":
				require.Empty(t, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write([]byte{0x01, 0x02, 0x03})
			case "/api/v1/renders/" + renderID + "/snapshot":
				require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "image/png")
				w.Header().Set("ETag", "\"abc\"")
				_, _ = w.Write([]byte{0x04, 0x05, 0x06})
			default:
				require.FailNow(t, "unexpected upstream request", "%s %s", r.Method, r.URL.Path)
			}
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		thumbCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/renders/"+renderID+"/thumbnail", nil, nil, nil)
		require.NoError(t, err)
		thumbCtx.Params["renderId"] = renderID

		proxyResp, err := h.HandleTrustGetRenderThumbnailLift(thumbCtx)
		require.Nil(t, proxyResp)
		require.Error(t, err)

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		thumbCtx, err = round10NewLiftContext(http.MethodGet, "/api/v1/trust/renders/"+renderID+"/thumbnail", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		thumbCtx.Params["renderId"] = renderID

		thumbResp := requireStatus(t, http.StatusOK)(h.HandleTrustGetRenderThumbnailLift(thumbCtx))
		require.Equal(t, []string{"image/png"}, thumbResp.Headers["content-type"])
		require.Equal(t, []byte{0x01, 0x02, 0x03}, thumbResp.Body)

		snapCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/renders/"+renderID+"/snapshot", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		snapCtx.Params["renderId"] = renderID

		snapResp := requireStatus(t, http.StatusOK)(h.HandleTrustGetRenderSnapshotLift(snapCtx))
		require.Equal(t, []string{"image/png"}, snapResp.Headers["content-type"])
		require.Equal(t, []string{"\"abc\""}, snapResp.Headers["etag"])
		require.Equal(t, []byte{0x04, 0x05, 0x06}, snapResp.Body)
	})

	t.Run("other_trust_endpoints_proxy_to_lesser_host", func(t *testing.T) {
		const (
			instanceKey   = "instance-key-raw"
			attestationID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "application/json", r.Header.Get("Accept"))

			switch r.URL.Path {
			case "/api/v1/publish/jobs":
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"job_id":"j1"}`))
			case "/api/v1/publish/jobs/j1":
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"job_id":"j1","status":"done"}`))
			case "/api/v1/renders/r1":
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"render_id":"r1"}`))
			case "/api/v1/ai/jobs/j1":
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"job_id":"j1","attestation_id":"` + attestationID + `"}`))
			case "/.well-known/jwks.json":
				require.Equal(t, http.MethodGet, r.Method)
				require.Empty(t, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"keys":[]}`))
			case "/attestations/" + attestationID:
				require.Equal(t, http.MethodGet, r.Method)
				require.Empty(t, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"id":"` + attestationID + `","jws":"x","payload":{}}`))
			default:
				require.FailNow(t, "unexpected upstream request", "%s %s", r.Method, r.URL.Path)
			}
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey
		cfg.LesserHostAttestationsURL = ""

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		createPublishCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/publish/jobs", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, []byte(`{"object_uri":"https://example.com/objects/1"}`))
		requireStatus(t, http.StatusOK)(h.HandleTrustCreatePublishJobLift(createPublishCtx))

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

		getPublishCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/publish/jobs/j1", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		getPublishCtx.Params["jobId"] = "j1"
		requireStatus(t, http.StatusOK)(h.HandleTrustGetPublishJobLift(getPublishCtx))

		getRenderCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/renders/r1", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		getRenderCtx.Params["renderId"] = "r1"
		requireStatus(t, http.StatusOK)(h.HandleTrustGetRenderLift(getRenderCtx))

		getAIJobCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/ai/jobs/j1", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		getAIJobCtx.Params["jobId"] = "j1"
		requireStatus(t, http.StatusOK)(h.HandleTrustGetAIJobLift(getAIJobCtx))

		jwksCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/jwks.json", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleTrustJWKSJSONLift(jwksCtx))

		attCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/attestations/"+attestationID, map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		attCtx.Params["id"] = attestationID
		requireStatus(t, http.StatusOK)(h.HandleTrustGetAttestationLift(attCtx))
	})

	t.Run("requires_user_auth_for_scoped_endpoints", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.LesserHostURL = "https://example.com"
		cfg.LesserHostInstanceKey = "instance-key-raw"

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/publish/jobs", nil, nil, []byte(`{"object_uri":"https://example.com/objects/1"}`))

		resp, err := h.HandleTrustCreatePublishJobLift(ctx)
		require.Nil(t, resp)
		require.Error(t, err)

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/jwks.json", nil, nil, nil)
		require.NoError(t, err)
		resp, err = h.HandleTrustJWKSJSONLift(ctx2)
		require.Nil(t, resp)
		require.Error(t, err)

		ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/attestations", nil, nil, nil)
		require.NoError(t, err)
		resp, err = h.HandleTrustLookupAttestationLift(ctx3)
		require.Nil(t, resp)
		require.Error(t, err)

		ctx4, err := round10NewLiftContext(http.MethodGet, "/api/v1/trust/attestations/att-1", nil, nil, nil)
		require.NoError(t, err)
		ctx4.Params["id"] = "att-1"
		resp, err = h.HandleTrustGetAttestationLift(ctx4)
		require.Nil(t, resp)
		require.Error(t, err)
	})

	t.Run("upstream_connection_failure_returns_503", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.LesserHostURL = "http://127.0.0.1:0"
		cfg.LesserHostInstanceKey = "instance-key-raw"

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/ai/claims/verify", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, []byte(`{"text":"hello","evidence":[{"source_id":"s1","text":"x"}]}`))

		requireStatus(t, http.StatusServiceUnavailable)(h.HandleTrustAIClaimVerifyLift(ctx))
	})

	t.Run("upstream_unauthorized_maps_to_409", func(t *testing.T) {
		const instanceKey = "instance-key-raw"

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/v1/ai/claims/verify", r.URL.Path)
			require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/ai/claims/verify", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, []byte(`{"text":"hello","evidence":[{"source_id":"s1","text":"x"}]}`))

		requireStatus(t, http.StatusConflict)(h.HandleTrustAIClaimVerifyLift(ctx))
	})

	t.Run("upstream_response_too_large_returns_503", func(t *testing.T) {
		const instanceKey = "instance-key-raw"

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/v1/ai/claims/verify", r.URL.Path)
			require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))

			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write(make([]byte, lesserHostProxyMaxResponseBytes+1))
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/ai/claims/verify", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, []byte(`{"text":"hello","evidence":[{"source_id":"s1","text":"x"}]}`))

		requireStatus(t, http.StatusServiceUnavailable)(h.HandleTrustAIClaimVerifyLift(ctx))
	})

	t.Run("non_json_upstream_response_is_not_rewritten", func(t *testing.T) {
		const instanceKey = "instance-key-raw"

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/v1/renders", r.URL.Path)
			require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
			require.Equal(t, "application/json", r.Header.Get("Accept"))

			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("hello"))
		}))
		t.Cleanup(upstream.Close)

		cfg := round11TestConfig()
		cfg.LesserHostURL = upstream.URL
		cfg.LesserHostInstanceKey = instanceKey

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/trust/renders", map[string]string{
			"Authorization": "Bearer " + userToken,
		}, nil, []byte(`{"url":"https://example.com"}`))

		resp := requireStatus(t, http.StatusOK)(h.HandleTrustCreateRenderLift(ctx))
		require.Equal(t, []string{"text/plain; charset=utf-8"}, resp.Headers["content-type"])
		require.Equal(t, []byte("hello"), resp.Body)
	})
}

func TestLesserHostTrustProxyHelpersRound20(t *testing.T) {
	allowLocalLesserHostProxyForTests(t)

	t.Run("looks_like_json", func(t *testing.T) {
		require.False(t, looksLikeJSON(nil))
		require.False(t, looksLikeJSON([]byte(" \n\t")))
		require.True(t, looksLikeJSON([]byte("  \n\t{ \"k\": \"v\" }")))
		require.True(t, looksLikeJSON([]byte("[1,2,3]")))
		require.False(t, looksLikeJSON([]byte("x{")))
	})

	t.Run("accept_defaulting", func(t *testing.T) {
		require.Equal(t, "application/json", resolveLesserHostProxyAccept(""))
		require.Equal(t, "text/plain", resolveLesserHostProxyAccept(" text/plain "))
	})

	t.Run("target_validation", func(t *testing.T) {
		require.False(t, isValidLesserHostProxyTarget(lesserHostProxyTarget{method: http.MethodGet, path: "/x"}, ""))
		require.False(t, isValidLesserHostProxyTarget(lesserHostProxyTarget{method: " ", path: "/x"}, "https://example.com"))
		require.False(t, isValidLesserHostProxyTarget(lesserHostProxyTarget{method: http.MethodGet, path: "x"}, "https://example.com"))
		require.True(t, isValidLesserHostProxyTarget(lesserHostProxyTarget{method: http.MethodGet, path: "/x"}, "https://example.com"))
	})

	t.Run("header_helpers", func(t *testing.T) {
		require.False(t, isJSONContentType([]string{"text/plain"}))
		require.True(t, isJSONContentType([]string{"application/json"}))

		require.Equal(t, "", firstHeaderValue(nil, "content-type"))
		require.Equal(t, "", firstHeaderValue(map[string][]string{}, "content-type"))
		require.Equal(t, "application/json", firstHeaderValue(map[string][]string{"content-type": {" application/json "}}, "Content-Type"))

		copyHeaderIfPresent(nil, http.Header{"Content-Type": []string{"application/json"}}, "content-type")

		dst := map[string][]string{}
		copyHeaderIfPresent(dst, nil, "content-type")
		copyHeaderIfPresent(dst, http.Header{}, "content-type")
		copyHeaderIfPresent(dst, http.Header{"Content-Type": []string{" ", ""}}, "content-type")
		copyHeaderIfPresent(dst, http.Header{"Content-Type": []string{"application/json"}}, "content-type")
		require.Equal(t, []string{"application/json"}, dst["content-type"])
	})

	t.Run("error_branches", func(t *testing.T) {
		_, ok := rewriteLesserHostURLsToInstanceProxy([]byte("not-json"))
		require.False(t, ok)

		ctx, err := round10NewLiftContext(http.MethodGet, "/x", nil, nil, nil)
		require.NoError(t, err)

		u, resp := buildLesserHostProxyURL(ctx, "http://[::1", "/api/v1/previews")
		require.Nil(t, u)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)

		u, resp = buildLesserHostProxyURL(ctx, "https://example.com", "/api/v1/previews")
		require.NotNil(t, u)
		require.Nil(t, resp)

		req, resp := buildLesserHostProxyRequest(ctx, lesserHostProxyTarget{method: " "}, u, nil, "")
		require.Nil(t, req)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
	})

	t.Run("empty_scope_and_nil_logger_helpers", func(t *testing.T) {
		cfg := round11TestConfig()
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/x", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.ensureLesserHostProxyScope(ctx, "  "))

		var nilHandler *Handler
		nilHandler.warnTrustProxyMisconfigured("ignored")
		(&Handler{}).warnTrustProxyMisconfigured("ignored")
	})

	t.Run("content_type_normalization", func(t *testing.T) {
		headers := map[string][]string{"content-type": {"text/plain"}}
		normalizeLesserHostProxyContentType(headers, []byte(`{"k":"v"}`))
		require.Equal(t, []string{"text/plain"}, headers["content-type"])

		headers = map[string][]string{}
		normalizeLesserHostProxyContentType(headers, []byte("not-json"))
		_, ok := headers["content-type"]
		require.False(t, ok)

		headers = map[string][]string{}
		normalizeLesserHostProxyContentType(headers, []byte("  {\"k\":\"v\"}"))
		require.Equal(t, []string{"application/json; charset=utf-8"}, headers["content-type"])
	})

	t.Run("nil_receiver_helpers", func(t *testing.T) {
		var h *Handler
		require.Equal(t, "", h.lesserHostTrustBaseURL())
		require.Equal(t, "", h.lesserHostAttestationsBaseURL())
		key, err := h.resolveLegacyLesserHostInstanceKey()
		require.NoError(t, err)
		require.Equal(t, "", key)

		h = &Handler{}
		require.Equal(t, "", h.lesserHostTrustBaseURL())
		require.Equal(t, "", h.lesserHostAttestationsBaseURL())
		key, err = h.resolveLegacyLesserHostInstanceKey()
		require.NoError(t, err)
		require.Equal(t, "", key)
	})

	t.Run("proxy_validation_failures_return_503", func(t *testing.T) {
		cfg := round11TestConfig()
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/x", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusUnprocessableEntity)(h.proxyToLesserHost(ctx, lesserHostProxyTarget{
			method:  http.MethodGet,
			baseURL: "",
			path:    "/api/v1/previews",
		}))
		require.NotNil(t, resp)
	})
}
