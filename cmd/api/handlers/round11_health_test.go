package handlers

import (
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

type round11RoundTrip func(req *http.Request) (*http.Response, error)

func (rt round11RoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return rt(req) }

func TestHealthCheckerHandlers(t *testing.T) {
	os.Setenv("PRIVATE_KEY_SECRET", "secret")
	defer os.Unsetenv("PRIVATE_KEY_SECRET")
	os.Setenv("DOMAIN_NAME", "example.com")
	defer os.Unsetenv("DOMAIN_NAME")

	h, repos, _ := round11NewHandler(t, nil, nil)
	checker := NewHealthChecker(h.logger, repos)
	checker.httpClient = &http.Client{
		Transport: round11RoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(http.NoBody)}, nil
		}),
		Timeout: 2 * time.Second,
	}

	ctxLive, err := round10NewLiftContext(http.MethodGet, "/health/live", nil, nil, nil)
	require.NoError(t, err)
	respLive, err := checker.HandleLivenessCheck(ctxLive)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, respLive.Status)

	ctxReady, err := round10NewLiftContext(http.MethodGet, "/health/ready", nil, nil, nil)
	require.NoError(t, err)
	respReady, err := checker.HandleReadinessCheck(ctxReady)
	require.NoError(t, err)
	require.NotNil(t, respReady)

	ctxDetailed, err := round10NewLiftContext(http.MethodGet, "/health/detailed", nil, nil, nil)
	require.NoError(t, err)
	respDetailed, err := checker.HandleDetailedHealthCheck(ctxDetailed)
	require.NoError(t, err)
	require.NotNil(t, respDetailed)
}

func TestHealthMiddleware(t *testing.T) {
	logger := round10TestLogger(t)
	mw := HealthCheckMiddleware(logger)
	ctx, err := round10NewLiftContext(http.MethodGet, "/health", nil, nil, nil)
	require.NoError(t, err)

	handler := mw(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(http.StatusOK, "ok"), nil
	})
	resp := requireStatus(t, http.StatusOK)(handler(ctx))
	require.NotEmpty(t, firstStringValue(resp.Headers, "x-health-check-time"))
}
