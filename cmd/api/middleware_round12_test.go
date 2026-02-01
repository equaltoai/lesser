package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap/zaptest"
)

func TestCreateLoggingMiddlewareRound12_LogLevels(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mw := createLoggingMiddleware(logger)

	t.Run("error from handler logs error level", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
		_, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), errors.New("boom")
		})(ctx)
		require.Error(t, err)
	})

	t.Run("status >=400 logs warn level", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusBadRequest, ""), nil
		})(ctx)

		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})
}

func TestInstanceLockMiddlewareRound12_Behavior(t *testing.T) {
	logger := zaptest.NewLogger(t)
	harness := newMiddlewareHarness(t)
	instanceRepo := repositories.NewInstanceRepository(harness.db, "table", logger)
	repoMock := new(mocks.MockRepositoryStorage)
	setRepoField(repoMock, "instanceRepo", instanceRepo)

	mw := createInstanceLockMiddleware(repoMock, logger)

	t.Run("OPTIONS passes through", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodOptions, "/api/v1/statuses")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusNoContent, ""), nil
		})(ctx)

		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, resp.Status)
	})

	t.Run("GET non-content path passes through even when locked", func(t *testing.T) {
		harness.locked = true
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/instance")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusCreated, ""), nil
		})(ctx)

		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.Status)
	})

	t.Run("locked write allowed path passes through", func(t *testing.T) {
		harness.locked = true
		ctx := newTestAppTheoryContext(http.MethodPost, "/api/v1/apps")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusCreated, ""), nil
		})(ctx)

		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.Status)
	})

	t.Run("locked write disallowed path returns forbidden", func(t *testing.T) {
		harness.locked = true
		ctx := newTestAppTheoryContext(http.MethodPost, "/api/v1/statuses")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), nil
		})(ctx)

		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("locked read suppresses statuses list", func(t *testing.T) {
		harness.locked = true
		ctx := newTestAppTheoryContext(http.MethodHead, "/api/v1/statuses/1")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), nil
		})(ctx)

		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})
}

func TestInstanceLockMiddlewareRound12_InstanceStateError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(query).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("First", mock.AnythingOfType("*models.InstanceState")).Return(errors.New("boom")).Maybe()

	instanceRepo := repositories.NewInstanceRepository(db, "table", logger)
	repoMock := new(mocks.MockRepositoryStorage)
	setRepoField(repoMock, "instanceRepo", instanceRepo)

	mw := createInstanceLockMiddleware(repoMock, logger)

	ctxGet := newTestAppTheoryContext(http.MethodGet, "/api/v1/statuses/1")
	handler := mw(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(http.StatusOK, ""), nil
	})
	resp, err := handler(ctxGet)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.Status)

	ctxPost := newTestAppTheoryContext(http.MethodPost, "/api/v1/statuses")
	resp, err = handler(ctxPost)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.Status)
}

func TestLockedContentResponsesRound12(t *testing.T) {
	t.Run("status objects become 404", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/statuses/1")
		resp, err := respondLockedContentRead(ctx, "/api/v1/statuses/1")
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("oembed becomes 404", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/oembed")
		resp, err := respondLockedContentRead(ctx, "/api/oembed")
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("search returns empty object", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v2/search")
		resp, err := respondLockedContentRead(ctx, "/api/v2/search")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, []any{}, body["accounts"])
		require.Equal(t, []any{}, body["statuses"])
		require.Equal(t, []any{}, body["hashtags"])
	})

	t.Run("timelines return empty list", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/timelines/home")
		resp, err := respondLockedContentRead(ctx, "/api/v1/timelines/home")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)

		var out []any
		require.NoError(t, json.Unmarshal(resp.Body, &out))
	})
}

func TestPermissionAndHelpersRound12(t *testing.T) {
	ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")

	require.False(t, isWriteAllowedWhileLocked("  "))
	require.False(t, shouldSuppressContentReadWhileLocked(""))
	require.True(t, shouldSuppressContentReadWhileLocked("/api/v1/accounts/123/statuses"))

	// Cost tracker helpers.
	tracker := cost.New()
	ctx.Set("cost_tracker", tracker)
	require.Equal(t, tracker, GetCostTracker(ctx))
	called := false
	TrackCost(ctx, func(got *cost.Tracker) {
		require.Equal(t, tracker, got)
		called = true
	})
	require.True(t, called)

	ctx.Set("cost_tracker", "not-a-tracker")
	require.Nil(t, GetCostTracker(ctx))

	// Logger helpers.
	require.NotNil(t, GetLogger(newTestAppTheoryContext(http.MethodGet, "/api/v1/test")))
}

func TestRBACMiddlewareRound12(t *testing.T) {
	logger := zaptest.NewLogger(t)
	harness := newMiddlewareHarness(t)
	userRepo := repositories.NewUserRepository(harness.db, "table", logger)
	repoMock := new(mocks.MockRepositoryStorage)
	setRepoField(repoMock, "userRepo", userRepo)

	ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
	ctx.Set("claims", &auth.Claims{Username: "alice"})

	t.Run("admin allowed", func(t *testing.T) {
		harness.role = "admin"
		handler := AdminOnlyMiddleware(repoMock)(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), nil
		})

		resp, err := handler(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("admin forbidden", func(t *testing.T) {
		harness.role = "user"
		ctxForbidden := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
		ctxForbidden.Set("claims", &auth.Claims{Username: "alice"})

		handler := AdminOnlyMiddleware(repoMock)(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), nil
		})

		resp, err := handler(ctxForbidden)
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("unknown permission level errors", func(t *testing.T) {
		ctxUnknown := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
		ctxUnknown.Set("claims", &auth.Claims{Username: "alice"})
		harness.role = "admin"

		err := checkUserPermissions(ctxUnknown, "unknown", repoMock)
		require.Error(t, err)
	})

	t.Run("performance monitoring pass-through", func(t *testing.T) {
		mw := createPerformanceMonitoringMiddleware(nil)
		ctxPerf := newTestAppTheoryContext(http.MethodGet, "/api/v1/test")
		handler := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, ""), nil
		})
		resp, err := handler(ctxPerf)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})
}
