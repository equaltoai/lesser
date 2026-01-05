package main

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"unsafe"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type middlewareHarness struct {
	db     *dynamormmocks.MockDB
	query  *dynamormmocks.MockQuery
	locked bool
	role   string
}

func newMiddlewareHarness(t *testing.T) *middlewareHarness {
	t.Helper()

	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)

	h := &middlewareHarness{
		db:    db,
		query: query,
		role:  "admin",
	}

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(query).Maybe()

	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Limit", mock.Anything).Return(query).Maybe()
	query.On("All", mock.Anything).Return(nil).Maybe()
	query.On("Delete").Return(nil).Maybe()

	query.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0)
		switch d := dest.(type) {
		case *models.InstanceState:
			*d = models.InstanceState{Locked: h.locked}
		case *models.User:
			*d = models.User{Username: "alice", Role: h.role}
		}
	}).Maybe()

	return h
}

func newTestLiftContext(method, path string) *lift.Context {
	req := lift.NewRequest(&adapters.Request{
		Method: method,
		Path:   path,
		Headers: map[string]string{
			"User-Agent":      "test",
			"X-Forwarded-For": "127.0.0.1",
		},
		QueryParams: map[string]string{},
		Body:        nil,
	})
	return lift.NewContext(context.Background(), req)
}

func setRepoField(repo *mocks.MockRepositoryStorage, field string, value any) {
	rv := reflect.ValueOf(repo).Elem().FieldByName(field)
	reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func TestMiddlewareHelpers_Round11(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := newTestLiftContext(http.MethodGet, "/api/v1/timelines/home")
	ctx.Set("claims", &auth.Claims{Username: "alice"})
	ctx.Set("tenantID", "tenant-1")

	mw := createLoggingMiddleware(logger)
	handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		ctx.Status(http.StatusOK)
		return nil
	}))
	require.NoError(t, handler.Handle(ctx))
	require.NotNil(t, GetLogger(ctx))

	harness := newMiddlewareHarness(t)
	instanceRepo := repositories.NewInstanceRepository(harness.db, "table", logger)
	userRepo := repositories.NewUserRepository(harness.db, "table", logger)
	repoMock := new(mocks.MockRepositoryStorage)
	setRepoField(repoMock, "instanceRepo", instanceRepo)
	setRepoField(repoMock, "userRepo", userRepo)

	mwLock := createInstanceLockMiddleware(repoMock, logger)
	handlerLocked := mwLock(lift.HandlerFunc(func(ctx *lift.Context) error {
		ctx.Status(http.StatusOK)
		return nil
	}))

	harness.locked = true
	ctxLocked := newTestLiftContext(http.MethodGet, "/api/v1/timelines/home")
	require.NoError(t, handlerLocked.Handle(ctxLocked))
	require.Equal(t, http.StatusOK, ctxLocked.Response.StatusCode)

	harness.locked = false
	// Use a fresh repository to avoid reusing cached instance lock state.
	instanceRepo = repositories.NewInstanceRepository(harness.db, "table", logger)
	setRepoField(repoMock, "instanceRepo", instanceRepo)
	ctxUnlocked := newTestLiftContext(http.MethodPost, "/api/v1/statuses")
	require.NoError(t, handlerLocked.Handle(ctxUnlocked))
	require.Equal(t, http.StatusOK, ctxUnlocked.Response.StatusCode)

	require.True(t, shouldSuppressContentReadWhileLocked("/api/v1/statuses/1"))
	require.False(t, shouldSuppressContentReadWhileLocked("/api/v1/instance"))
	require.True(t, isWriteAllowedWhileLocked("/api/v1/apps"))
	require.False(t, isWriteAllowedWhileLocked("/api/v1/statuses"))

	trackerCtx := newTestLiftContext(http.MethodPost, "/api/v1/statuses")
	trackerCtx.Set("claims", &auth.Claims{Username: "alice"})
	mwCost := createCostTrackingMiddleware(logger)
	handlerCost := mwCost(lift.HandlerFunc(func(ctx *lift.Context) error {
		TrackCost(ctx, func(_ *cost.Tracker) {})
		ctx.Status(http.StatusOK)
		return nil
	}))
	require.NoError(t, handlerCost.Handle(trackerCtx))

	trackerCtx.Set("claims", &auth.Claims{Username: "alice"})
	harness.role = "admin"
	require.NoError(t, checkUserPermissions(trackerCtx, PermissionAdmin, repoMock))
	harness.role = "moderator"
	require.NoError(t, checkUserPermissions(trackerCtx, PermissionModerator, repoMock))
	harness.role = "viewer"
	require.NoError(t, checkUserPermissions(trackerCtx, PermissionViewer, repoMock))

	noClaimsCtx := newTestLiftContext(http.MethodGet, "/api/v1/statuses")
	err := checkUserPermissions(noClaimsCtx, PermissionAdmin, repoMock)
	require.Error(t, err)

	require.NotNil(t, createPerformanceMonitoringMiddleware(nil))
}
