package httpclient

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFederationClient_DefaultsAndRedirectChecks(t *testing.T) {
	fc := NewFederationClient(nil, zap.NewNop())
	require.NotNil(t, fc)
	assert.Equal(t, 30*time.Second, fc.client.Timeout)

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	require.NoError(t, err)

	via := make([]*http.Request, DefaultFederationClientConfig().MaxRedirects)
	err = fc.client.CheckRedirect(req, via)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many redirects")

	localhostReq, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	require.NoError(t, err)
	err = fc.client.CheckRedirect(localhostReq, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect URL blocked")
}

func TestFederationClient_URLAndIPGuards(t *testing.T) {
	assert.Error(t, validateFederationURL("http://127.0.0.1/", nil, nil))
	assert.Error(t, validateFederationURL("https://metadata.google.internal/", nil, nil))
	assert.NoError(t, validateFederationURL("https://example.com/users/alice", nil, nil))

	assert.True(t, isBlockedIP(net.ParseIP("127.0.0.1")))
	assert.True(t, isBlockedIP(net.ParseIP("10.0.0.1")))
	assert.True(t, isBlockedIP(net.ParseIP("169.254.169.254")))
	assert.False(t, isBlockedIP(net.ParseIP("8.8.8.8")))
}

func TestFederationClient_RequestsAndHeaders(t *testing.T) {
	fc := NewFederationClient(DefaultFederationClientConfig(), zap.NewNop())

	ctx := context.Background()
	fc.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		assert.NotEmpty(t, req.Header.Get("Accept"))
		assert.NotEmpty(t, req.Header.Get("Date"))
		assert.NotEmpty(t, req.Header.Get("Host"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	resp, err := fc.Get(ctx, "https://example.com/users/alice")
	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = fc.Post(ctx, "https://example.com/inbox", "application/activity+json", []byte(`{"type":"Create"}`))
	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Cleanup(func() { _ = resp.Body.Close() })

	fc.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		assert.NotEmpty(t, req.Header.Get("Accept"))
		assert.Equal(t, "TestAgent/1.0", req.Header.Get("User-Agent"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	resp, err = fc.GetWithUserAgent(ctx, "https://example.com/users/alice", "TestAgent/1.0")
	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, "TestAgent/1.0", resp.Request.Header.Get("User-Agent"))
}

func TestFederationClient_SetTimeoutCloseAndClientAccessors(t *testing.T) {
	fc := NewFederationClient(DefaultFederationClientConfig(), zap.NewNop())
	fc.SetTimeout(123 * time.Millisecond)
	assert.Equal(t, 123*time.Millisecond, fc.client.Timeout)
	assert.Same(t, fc.client, fc.GetClient())

	// Exercise Close with a standard *http.Transport.
	fc.client.Transport = &http.Transport{}
	fc.Close()
}

func TestFederationClient_setActivityPubHeaders(t *testing.T) {
	fc := &FederationClient{logger: zap.NewNop()}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/users/alice", nil)
	require.NoError(t, err)

	fc.setActivityPubHeaders(req)
	assert.Contains(t, req.Header.Get("Accept"), "application/activity+json")
	assert.NotEmpty(t, req.Header.Get("Date"))
	assert.Equal(t, req.URL.Host, req.Header.Get("Host"))
}

func TestNewFederationClient_AllowsInsecureTLSOption(t *testing.T) {
	cfg := DefaultFederationClientConfig()
	cfg.AllowInsecureTLS = true
	fc := NewFederationClient(cfg, zap.NewNop())
	require.NotNil(t, fc)

	transport, ok := fc.client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestFederationClient_validateRedirectURL(t *testing.T) {
	fc := NewFederationClient(DefaultFederationClientConfig(), zap.NewNop())
	u, err := url.Parse("http://localhost")
	require.NoError(t, err)

	req := &http.Request{URL: u}
	err = fc.client.CheckRedirect(req, nil)
	require.Error(t, err)
}
