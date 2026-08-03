package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

func TestHealthRound19_MoreCoverage(t *testing.T) {
	t.Run("httpClientOrDefault handles nil receiver", func(t *testing.T) {
		var checker *HealthChecker
		require.NotNil(t, checker.httpClientOrDefault())

		checker = &HealthChecker{}
		require.NotNil(t, checker.httpClientOrDefault())
	})

	t.Run("checkSecrets missing secret is unhealthy", func(t *testing.T) {
		checker := NewHealthChecker(zap.NewNop(), nil)
		t.Setenv("PRIVATE_KEY_SECRET", "")

		checks := make(map[string]CheckResult)
		checker.checkSecrets(context.Background(), checks)
		require.Equal(t, HealthStatusUnhealthy, checks["secrets"].Status)
	})

	t.Run("middleware passes through nil responses", func(t *testing.T) {
		mw := HealthCheckMiddleware(zap.NewNop())
		ctx, err := round10NewLiftContext(http.MethodGet, "/health", nil, nil, nil)
		require.NoError(t, err)

		handler := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return nil, nil
		})

		resp, respErr := handler(ctx)
		require.NoError(t, respErr)
		require.Nil(t, resp)
	})

	t.Run("external dependency checks handle transport errors", func(t *testing.T) {
		checker := NewHealthChecker(zap.NewNop(), nil)
		checker.httpClient = &http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			}),
		}

		wk := checker.checkWellKnownEndpoint(context.Background(), "example.com")
		require.Equal(t, HealthStatusUnhealthy, wk.Status)
		require.NotEmpty(t, wk.Error)

		fed := checker.checkFederationConnectivity(context.Background())
		require.Equal(t, HealthStatusDegraded, fed.Status)
		require.NotEmpty(t, fed.Error)
	})
}
