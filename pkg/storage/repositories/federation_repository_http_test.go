package repositories

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockRoundTripper implements http.RoundTripper for testing HTTP calls
type mockRoundTripper struct {
	responses    []*http.Response
	errors       []error
	currentIndex int
	requestsLog  []*http.Request
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.requestsLog = append(m.requestsLog, req)

	if m.currentIndex >= len(m.responses) {
		return nil, errors.New("no more mock responses available")
	}

	resp := m.responses[m.currentIndex]
	var err error
	if m.currentIndex < len(m.errors) {
		err = m.errors[m.currentIndex]
	}
	m.currentIndex++

	return resp, err
}

// newMockResponse creates a basic HTTP response for testing
func newMockResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// ============================================================================
// performConnectivityTest Tests
// ============================================================================

func TestPerformConnectivityTest_EmptyDomain(t *testing.T) {
	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.performConnectivityTest(ctx, "")

	require.Error(t, err)
	// Should fail validation
}

func TestPerformConnectivityTest_Success(t *testing.T) {
	// Save and restore default transport
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusOK, ""),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.performConnectivityTest(ctx, "mastodon.social")

	require.NoError(t, err)
	assert.Len(t, mockTransport.requestsLog, 1)
	assert.Equal(t, "HEAD", mockTransport.requestsLog[0].Method)
	assert.Equal(t, "https://mastodon.social", mockTransport.requestsLog[0].URL.String())
}

func TestPerformConnectivityTest_ServerError(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusInternalServerError, ""),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.performConnectivityTest(ctx, "error.social")

	require.Error(t, err)
}

func TestPerformConnectivityTest_TransportError(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{nil},
		errors:    []error{errors.New("connection refused")},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.performConnectivityTest(ctx, "unreachable.social")

	require.Error(t, err)
}

func TestPerformConnectivityTest_ContextCanceled(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{nil},
		errors:    []error{context.Canceled},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// The request may fail due to context cancellation or transport error
	err := repo.performConnectivityTest(ctx, "canceled.social")

	require.Error(t, err)
}

func TestPerformConnectivityTest_Status404Accepted(t *testing.T) {
	// 404 should be accepted as it indicates the server is reachable
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusNotFound, ""),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.performConnectivityTest(ctx, "notfound.social")

	// 404 is < 500, so should succeed
	require.NoError(t, err)
}

func TestPerformConnectivityTest_Status502BadGateway(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusBadGateway, ""),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.performConnectivityTest(ctx, "badgateway.social")

	require.Error(t, err)
	// 502 is a 5xx error, should fail
}

// ============================================================================
// verifyNodeInfo Tests
// ============================================================================

func TestVerifyNodeInfo_EmptyDomain(t *testing.T) {
	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.verifyNodeInfo(ctx, "")

	require.Error(t, err)
}

func TestVerifyNodeInfo_Success(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	nodeInfoResponse := `{
		"links": [
			{
				"rel": "http://nodeinfo.diaspora.software/ns/schema/2.0",
				"href": "https://mastodon.social/nodeinfo/2.0"
			}
		]
	}`

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusOK, nodeInfoResponse),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.verifyNodeInfo(ctx, "mastodon.social")

	require.NoError(t, err)
	assert.Len(t, mockTransport.requestsLog, 1)
	assert.Equal(t, "https://mastodon.social/.well-known/nodeinfo", mockTransport.requestsLog[0].URL.String())
}

func TestVerifyNodeInfo_NodeInfo21(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	nodeInfoResponse := `{
		"links": [
			{
				"rel": "http://nodeinfo.diaspora.software/ns/schema/2.1",
				"href": "https://pleroma.site/nodeinfo/2.1"
			}
		]
	}`

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusOK, nodeInfoResponse),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.verifyNodeInfo(ctx, "pleroma.site")

	require.NoError(t, err)
}

func TestVerifyNodeInfo_Non200Status(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusNotFound, ""),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.verifyNodeInfo(ctx, "nonodeinfo.social")

	require.Error(t, err)
	// 404 means nodeinfo not available
}

func TestVerifyNodeInfo_TransportError(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{nil},
		errors:    []error{errors.New("network error")},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.verifyNodeInfo(ctx, "networkerror.social")

	require.Error(t, err)
}

func TestVerifyNodeInfo_InvalidJSON(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusOK, "not json"),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.verifyNodeInfo(ctx, "invalidjson.social")

	require.Error(t, err)
}

func TestVerifyNodeInfo_NoSupportedVersion(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	// Valid JSON but no recognized nodeinfo version
	nodeInfoResponse := `{
		"links": [
			{
				"rel": "http://nodeinfo.diaspora.software/ns/schema/1.0",
				"href": "https://old.social/nodeinfo/1.0"
			}
		]
	}`

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusOK, nodeInfoResponse),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.verifyNodeInfo(ctx, "oldversion.social")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nodeinfo")
}

// ============================================================================
// testWebFingerResolution Tests
// ============================================================================

func TestWebFingerResolution_EmptyDomain(t *testing.T) {
	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.testWebFingerResolution(ctx, "")

	require.Error(t, err)
}

func TestWebFingerResolution_Success200(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusOK, `{"subject":"acct:test@mastodon.social"}`),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.testWebFingerResolution(ctx, "mastodon.social")

	require.NoError(t, err)
	assert.Len(t, mockTransport.requestsLog, 1)
	assert.Contains(t, mockTransport.requestsLog[0].URL.String(), ".well-known/webfinger")
}

func TestWebFingerResolution_Success404(t *testing.T) {
	// 404 is acceptable - it means the endpoint exists but the test account doesn't
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusNotFound, ""),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.testWebFingerResolution(ctx, "test.social")

	require.NoError(t, err)
}

func TestWebFingerResolution_Error500(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusInternalServerError, ""),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.testWebFingerResolution(ctx, "error.social")

	require.Error(t, err)
}

func TestWebFingerResolution_TransportError(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{nil},
		errors:    []error{errors.New("timeout")},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.testWebFingerResolution(ctx, "timeout.social")

	require.Error(t, err)
}

func TestWebFingerResolution_AcceptHeader(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	mockTransport := &mockRoundTripper{
		responses: []*http.Response{
			newMockResponse(http.StatusOK, "{}"),
		},
	}
	http.DefaultTransport = mockTransport

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}
	ctx := context.Background()

	err := repo.testWebFingerResolution(ctx, "check-headers.social")

	require.NoError(t, err)
	// Verify correct Accept header was set
	req := mockTransport.requestsLog[0]
	assert.Contains(t, req.Header.Get("Accept"), "application/jrd+json")
}

// ============================================================================
// Integration scenarios with timeout
// ============================================================================

func TestHTTPMethods_ContextTimeout(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	// Create context that's already timed out
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // Ensure timeout fires

	logger := zap.NewNop()
	repo := &FederationRepository{logger: logger}

	// All HTTP methods should fail with context deadline exceeded
	t.Run("performConnectivityTest", func(t *testing.T) {
		err := repo.performConnectivityTest(ctx, "timeout-test.social")
		require.Error(t, err)
	})

	t.Run("verifyNodeInfo", func(t *testing.T) {
		err := repo.verifyNodeInfo(ctx, "timeout-test.social")
		require.Error(t, err)
	})

	t.Run("testWebFingerResolution", func(t *testing.T) {
		err := repo.testWebFingerResolution(ctx, "timeout-test.social")
		require.Error(t, err)
	})
}
