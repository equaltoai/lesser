package main

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap/zaptest"
)

func TestCreateCostTrackingMiddleware_Round15_SetsUnifiedTracker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mw := createCostTrackingMiddleware(logger)

	ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
	ctx.RequestID = "rid"
	ctx.Set("claims", &auth.Claims{Username: "alice"})

	resp, err := mw(func(ctx *apptheory.Context) (*apptheory.Response, error) {
		// Ensure tracker is visible to handlers.
		tracker, ok := ctx.Get("unified_cost_tracker").(*cost.UnifiedTracker)
		require.True(t, ok)
		require.NotNil(t, tracker)
		require.Same(t, tracker, ctx.Get("cost_tracker"))
		return apptheory.Text(http.StatusOK, "ok"), nil
	})(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)

	tracker, ok := ctx.Get("unified_cost_tracker").(*cost.UnifiedTracker)
	require.True(t, ok)
	require.Greater(t, tracker.GetCurrentCostMicroCents(), int64(0))
}

func TestRBACWrappers_Round15(t *testing.T) {
	logger := zaptest.NewLogger(t)
	harness := newMiddlewareHarness(t)
	userRepo := repositories.NewUserRepository(harness.db, "table", logger)
	repoMock := new(mocks.MockRepositoryStorage)
	setRepoField(repoMock, "userRepo", userRepo)

	t.Run("viewer allows viewer", func(t *testing.T) {
		harness.role = "viewer"
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
		ctx.Set("claims", &auth.Claims{Username: "alice"})

		handler := ViewerOrHigherMiddleware(repoMock)(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), nil
		})
		resp, err := handler(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("moderator allows moderator", func(t *testing.T) {
		harness.role = "moderator"
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
		ctx.Set("claims", &auth.Claims{Username: "alice"})

		handler := ModeratorOrHigherMiddleware(repoMock)(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), nil
		})
		resp, err := handler(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("missing claims returns forbidden", func(t *testing.T) {
		harness.role = "admin"
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")

		handler := ViewerOrHigherMiddleware(repoMock)(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), nil
		})
		resp, err := handler(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})
}
