package main

import (
	"net/http"
	"reflect"
	"testing"
	"unsafe"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
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

func newTestAppTheoryContext(method, path string) *apptheory.Context {
	return &apptheory.Context{
		Request: apptheory.Request{
			Method: method,
			Path:   path,
			Headers: map[string][]string{
				"user-agent":      {"test"},
				"x-forwarded-for": {"127.0.0.1"},
			},
		},
	}
}

func setRepoField(repo *mocks.MockRepositoryStorage, field string, value any) {
	rv := reflect.ValueOf(repo).Elem().FieldByName(field)
	reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func TestMiddlewareHelpers_Round11(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/timelines/home")
	ctx.Set("claims", &auth.Claims{Username: "alice"})
	ctx.TenantID = "tenant-1"

	mw := createLoggingMiddleware(logger)
	_, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(http.StatusOK, ""), nil
	})(ctx)
	require.NoError(t, err)
	require.NotNil(t, GetLogger(ctx))

	harness := newMiddlewareHarness(t)
	instanceRepo := repositories.NewInstanceRepository(harness.db, "table", logger)
	userRepo := repositories.NewUserRepository(harness.db, "table", logger)
	repoMock := new(mocks.MockRepositoryStorage)
	setRepoField(repoMock, "instanceRepo", instanceRepo)
	setRepoField(repoMock, "userRepo", userRepo)

	mwLock := createInstanceLockMiddleware(repoMock, logger)
	handlerLocked := mwLock(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(http.StatusOK, ""), nil
	})

	harness.locked = true
	ctxLocked := newTestAppTheoryContext(http.MethodGet, "/api/v1/timelines/home")
	resp, err := handlerLocked(ctxLocked)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)

	harness.locked = false
	// Use a fresh repository to avoid reusing cached instance lock state.
	instanceRepo = repositories.NewInstanceRepository(harness.db, "table", logger)
	setRepoField(repoMock, "instanceRepo", instanceRepo)
	ctxUnlocked := newTestAppTheoryContext(http.MethodPost, "/api/v1/statuses")
	resp, err = handlerLocked(ctxUnlocked)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)

	require.True(t, shouldSuppressContentReadWhileLocked("/api/v1/statuses/1"))
	require.False(t, shouldSuppressContentReadWhileLocked("/api/v1/instance"))
	require.True(t, isWriteAllowedWhileLocked("/api/v1/apps"))
	require.False(t, isWriteAllowedWhileLocked("/api/v1/statuses"))

	trackerCtx := newTestAppTheoryContext(http.MethodPost, "/api/v1/statuses")
	trackerCtx.Set("claims", &auth.Claims{Username: "alice"})
	harness.role = "admin"
	require.NoError(t, checkUserPermissions(trackerCtx, PermissionAdmin, repoMock))
	harness.role = "moderator"
	require.NoError(t, checkUserPermissions(trackerCtx, PermissionModerator, repoMock))
	harness.role = "viewer"
	require.NoError(t, checkUserPermissions(trackerCtx, PermissionViewer, repoMock))

	noClaimsCtx := newTestAppTheoryContext(http.MethodGet, "/api/v1/statuses")
	permErr := checkUserPermissions(noClaimsCtx, PermissionAdmin, repoMock)
	require.Error(t, permErr)

	require.NotNil(t, createPerformanceMonitoringMiddleware(nil))
}
