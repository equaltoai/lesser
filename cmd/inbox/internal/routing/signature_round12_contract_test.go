package routing

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

func TestInboxHandler_convertRequest_PrefersForwardedOrigin(t *testing.T) {
	env := newInboxTestEnv(t)

	ctx := newAppTheoryContextWithValues(
		http.MethodPost,
		"/users/alice/inbox",
		map[string][]string{
			"host":                      {"internal.execute-api.us-east-1.amazonaws.com"},
			"x-forwarded-host":          {"stale.theory.dev.example.com"},
			"x-forwarded-proto":         {"https"},
			"x-lesser-forwarded-host":   {"theory.dev.example.com"},
			"x-lesser-forwarded-proto":  {"https"},
			"x-custom-signature":        {"sig"},
			"content-type":              {"application/activity+json"},
			"cloudfront-viewer-address": {"198.51.100.20:443"},
		},
		map[string][]string{
			"page": {"1"},
			"via":  {"shared"},
		},
		[]byte(`{"type":"Follow"}`),
	)

	req, err := env.handler.convertRequest(ctx, ctx.Request.Body)
	require.NoError(t, err)
	require.Equal(t, "https://theory.dev.example.com/users/alice/inbox?page=1&via=shared", req.URL.String())
	require.Equal(t, "theory.dev.example.com", req.URL.Host)
	require.Equal(t, "theory.dev.example.com", req.Host)
	require.Equal(t, "theory.dev.example.com", req.Header.Get("Host"))
	require.Equal(t, "sig", req.Header.Get("X-Custom-Signature"))
}

func TestInboxRequestURL_OriginMatrix(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string][]string
		query   map[string][]string
		want    string
	}{
		{
			name: "x-lesser-forwarded host wins",
			headers: map[string][]string{
				"host":                     {"internal.execute-api.us-east-1.amazonaws.com"},
				"x-forwarded-host":         {"stale.theory.dev.example.com"},
				"x-forwarded-proto":        {"https"},
				"x-lesser-forwarded-host":  {"theory.dev.example.com"},
				"x-lesser-forwarded-proto": {"https"},
			},
			query: map[string][]string{
				"page": {"1"},
			},
			want: "https://theory.dev.example.com/inbox?page=1",
		},
		{
			name: "forwarded header fallback",
			headers: map[string][]string{
				"host":      {"internal.execute-api.us-east-1.amazonaws.com"},
				"forwarded": {"for=198.51.100.22;proto=https;host=theory.dev.example.com"},
			},
			query: map[string][]string{
				"cursor": {"next"},
			},
			want: "https://theory.dev.example.com/inbox?cursor=next",
		},
		{
			name: "host fallback with repeated query values",
			headers: map[string][]string{
				"host": {"theory.dev.example.com"},
			},
			query: map[string][]string{
				"tag": {"one", "two"},
			},
			want: "https://theory.dev.example.com/inbox?tag=one&tag=two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newAppTheoryContextWithValues(http.MethodPost, "/inbox", tt.headers, tt.query, nil)
			require.Equal(t, tt.want, inboxRequestURL(ctx).String())
		})
	}
}

func TestInboxHandler_validateInboundSignatureHostBindsInstanceDomain(t *testing.T) {
	env := newInboxTestEnv(t)
	oldDomain := env.cfg.Domain
	env.cfg.Domain = "theory.dev.example.com"
	t.Cleanup(func() { env.cfg.Domain = oldDomain })

	valid := httptest.NewRequest(http.MethodPost, "https://theory.dev.example.com/users/alice/inbox", strings.NewReader(`{}`))
	valid.Host = "theory.dev.example.com"
	require.NoError(t, env.handler.validateInboundSignatureHost(valid))

	forged := httptest.NewRequest(http.MethodPost, "https://attacker.example/users/alice/inbox", strings.NewReader(`{}`))
	forged.Host = "attacker.example"
	require.Error(t, env.handler.validateInboundSignatureHost(forged))
}

func TestInboxHandler_convertRequest_VerifiesClassicDeliveryAfterForwardedReconstruction(t *testing.T) {
	env := newInboxTestEnv(t)

	outboundReq, outboundBody, publicKey := captureClassicFollowDelivery(t)
	sig, err := federation.ParseSignatureHeader(outboundReq.Header.Get(federation.SignatureHeader))
	require.NoError(t, err)

	senderCanonical, err := federation.BuildHTTPSignatureString(outboundReq, sig.Headers)
	require.NoError(t, err)

	ctx := newAppTheoryContextWithValues(
		outboundReq.Method,
		outboundReq.URL.Path,
		lowercaseRequestHeaders(outboundReq.Header, map[string][]string{
			"host":                     {"internal.lambda-url.us-east-1.on.aws"},
			"x-lesser-forwarded-host":  {outboundReq.URL.Host},
			"x-lesser-forwarded-proto": {"https"},
		}),
		outboundReq.URL.Query(),
		outboundBody,
	)

	reconstructed, err := env.handler.convertRequest(ctx, outboundBody)
	require.NoError(t, err)
	require.Equal(t, outboundReq.URL.Host, reconstructed.URL.Host)
	require.Equal(t, outboundReq.URL.Host, reconstructed.Host)

	receiverCanonical, err := federation.BuildHTTPSignatureString(reconstructed, sig.Headers)
	require.NoError(t, err)
	require.Equal(t, senderCanonical, receiverCanonical)
	require.NoError(t, federation.VerifyHTTPSignature(reconstructed, publicKey))
}

func captureClassicFollowDelivery(t *testing.T) (*http.Request, []byte, *rsa.PublicKey) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	signingActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://sim.dev.example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://sim.dev.example.com/users/alice/inbox",
		Outbox:            "https://sim.dev.example.com/users/alice/outbox",
		PublicKey: &activitypub.PublicKey{
			ID:    "https://sim.dev.example.com/users/alice#main-key",
			Owner: "https://sim.dev.example.com/users/alice",
		},
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      "https://sim.dev.example.com/activities/follow-1",
			Type:    activitypub.FollowType,
			To:      []string{"https://theory.dev.example.com/users/steward"},
		},
		Actor:  signingActor.ID,
		Object: "https://theory.dev.example.com/users/steward",
	}

	body, err := json.Marshal(activity)
	require.NoError(t, err)

	req, err := federation.BuildSignedActivityPubRequest(
		context.Background(),
		http.MethodPost,
		"https://theory.dev.example.com/users/steward/inbox?shared=true",
		"Lesser/1.0",
		body,
		privateKey,
		signingActor.PublicKey.ID,
	)
	require.NoError(t, err)

	return req, body, &privateKey.PublicKey
}

func newAppTheoryContextWithValues(method, path string, headers map[string][]string, query map[string][]string, body []byte) *apptheory.Context {
	return &apptheory.Context{
		Request: apptheory.Request{
			Method:  method,
			Path:    path,
			Headers: cloneStringSliceMap(headers),
			Query:   cloneStringSliceMap(query),
			Body:    append([]byte(nil), body...),
		},
		Params: map[string]string{},
	}
}

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, values := range in {
		out[k] = append([]string(nil), values...)
	}
	return out
}

func lowercaseRequestHeaders(header http.Header, extra map[string][]string) map[string][]string {
	out := make(map[string][]string, len(header)+len(extra))
	for key, values := range header {
		out[strings.ToLower(key)] = append([]string(nil), values...)
	}
	for key, values := range extra {
		out[strings.ToLower(key)] = append([]string(nil), values...)
	}
	return out
}
