package httpclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type errCloseBody struct {
	closed bool
}

func (e *errCloseBody) Read(_ []byte) (int, error) { return 0, io.EOF }
func (e *errCloseBody) Close() error {
	e.closed = true
	return errors.New("close failed")
}

func TestSecureClient_AdditionalOptionsAndHelpers(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	store := pkgtesting.NewMockRepositoryStorage()

	client := NewSecureClient(
		WithLogger(logger),
		WithMaxRedirects(1),
		WithStorage(store),
	)

	require.Same(t, logger, client.logger)
	require.Equal(t, 1, client.maxRedirects)
	require.Same(t, store, client.store)
	require.NotNil(t, client.dnsCache)
	require.Same(t, store, client.dnsCache.store)

	newStore := pkgtesting.NewMockRepositoryStorage()
	WithStorage(newStore)(client)
	require.Same(t, newStore, client.store)
	require.Same(t, newStore, client.dnsCache.store)
}

func TestSecureClient_checkRedirect_TooManyRedirects(t *testing.T) {
	t.Parallel()

	client := NewSecureClient(WithMaxRedirects(1))

	req, err := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	require.NoError(t, err)

	err = client.checkRedirect(req, []*http.Request{req})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many redirects")
}

func TestDNSCacheManager_LocalCachePaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	d := &dnsCacheManager{
		logger: zap.NewNop(),
		local:  make(map[string]*storage.DNSCacheEntry),
	}

	hostname := "example.com"
	publicIP := net.IPv4(93, 184, 216, 34)
	d.setCachedIPs(ctx, hostname, []net.IP{publicIP})

	ips, ok := d.getCachedIPs(ctx, hostname)
	require.True(t, ok)
	require.Len(t, ips, 1)
	assert.Equal(t, publicIP.String(), ips[0].String())

	d.local[hostname].ResolvedAt = time.Now().Add(-dnsCacheTTL - 1*time.Second)
	ips, ok = d.getCachedIPs(ctx, hostname)
	assert.False(t, ok)
	assert.Nil(t, ips)
}

func TestSecureTransport_RoundTrip_DNSLookupError(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	transport := client.client.Transport.(*secureTransport)

	transport.lookupIP = func(string) ([]net.IP, error) {
		return nil, errors.New("lookup failed")
	}

	resp, err := client.Get("http://example.com/")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DNS lookup failed")
}

func TestSecureTransport_RoundTrip_UsesLocalDNSCache(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	transport := client.client.Transport.(*secureTransport)

	transport.lookupIP = func(string) ([]net.IP, error) {
		return []net.IP{net.IPv4(93, 184, 216, 34)}, nil
	}

	callCount := 0
	transport.base = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("OK")),
			Request:    req,
		}, nil
	})

	resp, err := client.Get("http://example.com/")
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()

	transport.lookupIP = func(string) ([]net.IP, error) {
		t.Fatalf("unexpected DNS lookup (expected cache hit)")
		return nil, nil
	}

	resp, err = client.Get("http://example.com/")
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()

	assert.Equal(t, 2, callCount)
}

func TestSecureTransport_RoundTrip_HostnameChangeClosesBody(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	transport := client.client.Transport.(*secureTransport)

	transport.lookupIP = func(string) ([]net.IP, error) {
		return []net.IP{net.IPv4(93, 184, 216, 34)}, nil
	}

	body := &errCloseBody{}
	transport.base = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		evilURL, err := url.Parse("http://evil.example/")
		require.NoError(t, err)

		evilReq := req.Clone(req.Context())
		evilReq.URL = evilURL

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Request:    evilReq,
		}, nil
	})

	resp, err := client.Get("http://example.com/")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hostname changed during request")
	assert.True(t, body.closed)
}

func TestSecureClient_RequestHelpers(t *testing.T) {
	t.Parallel()

	client := DefaultClient()
	transport := client.client.Transport.(*secureTransport)

	transport.lookupIP = func(string) ([]net.IP, error) {
		return []net.IP{net.IPv4(93, 184, 216, 34)}, nil
	}

	var seen []string
	transport.base = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, req.Method)
		switch req.Method {
		case http.MethodPost:
			assert.Equal(t, "text/plain", req.Header.Get("Content-Type"))
		case http.MethodHead:
		default:
			t.Fatalf("unexpected method %s", req.Method)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("OK")),
			Request:    req,
		}, nil
	})

	resp, err := client.Post("http://example.com/", "text/plain", strings.NewReader("body"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()

	resp, err = client.Head("http://example.com/")
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()

	resp, err = client.PostWithContext(context.Background(), "http://example.com/", "text/plain", strings.NewReader("ctx"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()

	resp, err = client.Get("http://[::1")
	assert.Nil(t, resp)
	require.Error(t, err)

	assert.Equal(t, []string{http.MethodPost, http.MethodHead, http.MethodPost}, seen)
}
