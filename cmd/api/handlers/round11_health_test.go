package lift

import (
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, checker.HandleLivenessCheck(ctxLive))

	ctxReady, err := round10NewLiftContext(http.MethodGet, "/health/ready", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, checker.HandleReadinessCheck(ctxReady))

	ctxDetailed, err := round10NewLiftContext(http.MethodGet, "/health/detailed", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, checker.HandleDetailedHealthCheck(ctxDetailed))
}

func TestHealthMiddleware(t *testing.T) {
	logger := round10TestLogger(t)
	mw := HealthCheckMiddleware(logger)
	ctx, err := round10NewLiftContext(http.MethodGet, "/health", nil, nil, nil)
	require.NoError(t, err)

	handler := mw(lift.HandlerFunc(func(c *lift.Context) error { return nil }))
	require.NoError(t, handler.Handle(ctx))
	require.NotEmpty(t, ctx.Response.Headers["X-Health-Check-Time"])
}
