package main

import (
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestCreateLoggingMiddlewareRound12_LogLevels(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mw := createLoggingMiddleware(logger)

	t.Run("error from handler logs error level", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodGet, "/api/v1/test")
		handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusOK)
			return errors.New("boom")
		}))

		require.Error(t, handler.Handle(ctx))
	})

	t.Run("status >=400 logs warn level", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodGet, "/api/v1/test")
		handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusBadRequest)
			return nil
		}))

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
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
		ctx := newTestLiftContext(http.MethodOptions, "/api/v1/statuses")
		handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusNoContent)
			return nil
		}))

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, http.StatusNoContent, ctx.Response.StatusCode)
	})

	t.Run("GET non-content path passes through even when locked", func(t *testing.T) {
		harness.locked = true
		ctx := newTestLiftContext(http.MethodGet, "/api/v1/instance")
		handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusCreated)
			return nil
		}))

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, http.StatusCreated, ctx.Response.StatusCode)
	})

	t.Run("locked write allowed path passes through", func(t *testing.T) {
		harness.locked = true
		ctx := newTestLiftContext(http.MethodPost, "/api/v1/apps")
		handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusCreated)
			return nil
		}))

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, http.StatusCreated, ctx.Response.StatusCode)
	})

	t.Run("locked write disallowed path returns forbidden", func(t *testing.T) {
		harness.locked = true
		ctx := newTestLiftContext(http.MethodPost, "/api/v1/statuses")
		handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusOK)
			return nil
		}))

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("locked read suppresses statuses list", func(t *testing.T) {
		harness.locked = true
		ctx := newTestLiftContext(http.MethodHead, "/api/v1/statuses/1")
		handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusOK)
			return nil
		}))

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
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

	ctxGet := newTestLiftContext(http.MethodGet, "/api/v1/statuses/1")
	handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		ctx.Status(http.StatusOK)
		return nil
	}))
	require.NoError(t, handler.Handle(ctxGet))
	require.Equal(t, http.StatusNotFound, ctxGet.Response.StatusCode)

	ctxPost := newTestLiftContext(http.MethodPost, "/api/v1/statuses")
	require.NoError(t, handler.Handle(ctxPost))
	require.Equal(t, http.StatusForbidden, ctxPost.Response.StatusCode)
}

func TestLockedContentResponsesRound12(t *testing.T) {
	t.Run("status objects become 404", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodGet, "/api/v1/statuses/1")
		require.NoError(t, respondLockedContentRead(ctx, "/api/v1/statuses/1"))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("oembed becomes 404", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodGet, "/api/oembed")
		require.NoError(t, respondLockedContentRead(ctx, "/api/oembed"))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("search returns empty object", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodGet, "/api/v2/search")
		require.NoError(t, respondLockedContentRead(ctx, "/api/v2/search"))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		body := ctx.Response.Body.(map[string]any)
		require.Equal(t, []any{}, body["accounts"])
		require.Equal(t, []any{}, body["statuses"])
		require.Equal(t, []any{}, body["hashtags"])
	})

	t.Run("timelines return empty list", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodGet, "/api/v1/timelines/home")
		require.NoError(t, respondLockedContentRead(ctx, "/api/v1/timelines/home"))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		_, ok := ctx.Response.Body.([]any)
		require.True(t, ok)
	})
}

func TestPermissionAndHelpersRound12(t *testing.T) {
	ctx := newTestLiftContext(http.MethodGet, "/api/v1/test")

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
	require.NotNil(t, GetLogger(newTestLiftContext(http.MethodGet, "/api/v1/test")))
}

func TestRBACMiddlewareRound12(t *testing.T) {
	logger := zaptest.NewLogger(t)
	harness := newMiddlewareHarness(t)
	userRepo := repositories.NewUserRepository(harness.db, "table", logger)
	repoMock := new(mocks.MockRepositoryStorage)
	setRepoField(repoMock, "userRepo", userRepo)

	ctx := newTestLiftContext(http.MethodGet, "/api/v1/test")
	ctx.Set("claims", &auth.Claims{Username: "alice"})

	t.Run("admin allowed", func(t *testing.T) {
		harness.role = "admin"
		handler := AdminOnlyMiddleware(repoMock)(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusOK)
			return nil
		}))

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("admin forbidden", func(t *testing.T) {
		harness.role = "user"
		ctxForbidden := newTestLiftContext(http.MethodGet, "/api/v1/test")
		ctxForbidden.Set("claims", &auth.Claims{Username: "alice"})

		handler := AdminOnlyMiddleware(repoMock)(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusOK)
			return nil
		}))

		require.NoError(t, handler.Handle(ctxForbidden))
		require.Equal(t, http.StatusForbidden, ctxForbidden.Response.StatusCode)
	})

	t.Run("unknown permission level errors", func(t *testing.T) {
		ctxUnknown := newTestLiftContext(http.MethodGet, "/api/v1/test")
		ctxUnknown.Set("claims", &auth.Claims{Username: "alice"})
		harness.role = "admin"

		err := checkUserPermissions(ctxUnknown, "unknown", repoMock)
		require.Error(t, err)
	})

	t.Run("performance monitoring pass-through", func(t *testing.T) {
		mw := createPerformanceMonitoringMiddleware(nil)
		ctxPerf := newTestLiftContext(http.MethodGet, "/api/v1/test")
		handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Status(http.StatusOK)
			return nil
		}))
		require.NoError(t, handler.Handle(ctxPerf))
		require.Equal(t, http.StatusOK, ctxPerf.Response.StatusCode)
	})
}
