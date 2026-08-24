package routing

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

type round14MetricsTimer struct {
	finished bool
	errored  bool
}

func (t *round14MetricsTimer) Finish(_ interface{}, _ bool) {
	t.finished = true
}

func (t *round14MetricsTimer) FinishWithError(_ interface{}, _ string) {
	t.errored = true
}

func TestRound14_AppendHeaderValue(t *testing.T) {
	require.Equal(t, []string{"a"}, appendHeaderValue([]string{"a"}, ""))
	require.Equal(t, []string{"a"}, appendHeaderValue([]string{"a"}, " A "))
	require.Equal(t, []string{"a", "b"}, appendHeaderValue([]string{"a"}, "b"))
}

func TestRound14_PanicRecovery(t *testing.T) {
	mw := panicRecovery(zap.NewNop())
	next := func(_ *apptheory.Context) (*apptheory.Response, error) {
		panic("boom")
	}

	ctx := &apptheory.Context{
		Request: apptheory.Request{Method: http.MethodGet, Path: "/users/alice/inbox"},
	}
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.NotEmpty(t, ctx.RequestID)
	require.Contains(t, string(resp.Body), ctx.RequestID)
}

func TestRound14_FederationSecurityMiddleware(t *testing.T) {
	mw := federationSecurityMiddleware()

	t.Run("rejects large request by content-length", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodPost,
				Path:    "/users/alice/inbox",
				Headers: map[string][]string{"content-length": {"1048577"}},
				Body:    []byte("x"),
			},
		}

		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusRequestEntityTooLarge, resp.Status)
		require.Equal(t, "*", resp.Headers["access-control-allow-origin"][0])
		require.Contains(t, strings.Join(resp.Headers["vary"], ","), "Origin")
	})

	t.Run("rejects large request by body length", func(t *testing.T) {
		tooBig := make([]byte, 1024*1024+1)
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/users/alice/inbox",
				Body:   tooBig,
			},
		}

		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusRequestEntityTooLarge, resp.Status)
		require.Equal(t, "*", resp.Headers["access-control-allow-origin"][0])
	})

	t.Run("handles preflight options", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodOptions,
				Path:   "/users/alice/inbox",
			},
		}

		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNoContent, resp.Status)
		require.Equal(t, "*", resp.Headers["access-control-allow-origin"][0])
		require.NotEmpty(t, resp.Headers["access-control-allow-methods"])
		require.NotEmpty(t, resp.Headers["access-control-allow-headers"])
	})

	t.Run("applies headers to downstream response", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/inbox",
			},
		}

		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: http.StatusOK}, nil
		})(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "*", resp.Headers["access-control-allow-origin"][0])
		require.Equal(t, "nosniff", resp.Headers["x-content-type-options"][0])
	})
}

func TestRound14_RemoteInstanceAndMetricsHelpers(t *testing.T) {
	ih := &InboxHandler{
		emfMetrics: observability.NewEMFMetrics(zap.NewNop(), "Lesser/Test", "inbox"),
	}

	require.Equal(t, "unknown", ih.parseRemoteInstance(""))
	require.Equal(t, "remote.example", ih.parseRemoteInstance("https://remote.example/users/alice"))
	require.Equal(t, "mastodon/4.0.0", ih.parseRemoteInstance("Mastodon/4.0.0 (compatible)"))

	require.Equal(t, observability.ErrorTypeAuthentication, ih.categorizeErrorType(401))
	require.Equal(t, observability.ErrorTypeAuthentication, ih.categorizeErrorType(403))
	require.Equal(t, observability.ErrorTypeRateLimit, ih.categorizeErrorType(429))
	require.Equal(t, observability.ErrorTypeValidation, ih.categorizeErrorType(404))
	require.Equal(t, observability.ErrorTypeFederation, ih.categorizeErrorType(500))

	fedCtx := &federationContext{
		remoteInstance: "remote.example",
		hasSignature:   true,
		dimensions:     map[string]string{observability.DimensionInstance: "remote.example"},
	}

	okTimer := &round14MetricsTimer{}
	ih.recordFederationMetrics(fedCtx, okTimer, nil)
	require.True(t, okTimer.finished)

	failTimer := &round14MetricsTimer{}
	ih.recordFederationMetrics(fedCtx, failTimer, errors.New("boom"))
	require.True(t, failTimer.errored)
}
