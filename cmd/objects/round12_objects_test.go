package main

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	pkgtypes "github.com/pay-theory/dynamorm/pkg/types"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeObjectRepo struct {
	obj   any
	err   error
	gotID string
}

func (f *fakeObjectRepo) GetObject(_ context.Context, id string) (any, error) {
	f.gotID = id
	if f.err != nil {
		return nil, f.err
	}
	return f.obj, nil
}

type fakeAuthorizedFetch struct {
	enabled   bool
	verifyErr error
}

func (f *fakeAuthorizedFetch) IsAuthorizedFetchEnabled(context.Context) bool { return f.enabled }

func (f *fakeAuthorizedFetch) VerifyAuthorizedFetch(context.Context, *http.Request) (*activitypub.Actor, error) {
	return nil, f.verifyErr
}

type fakeInstanceRepo struct {
	state *storageModels.InstanceState
	err   error
}

func (f *fakeInstanceRepo) GetInstanceState(context.Context) (*storageModels.InstanceState, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.state, nil
}

func TestHandleGetObject_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	t.Run("missing object id", func(t *testing.T) {
		h := &Handler{}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/",
		}))
		err := h.HandleGetObject(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 422, liftErr.StatusCode)
	})

	t.Run("locked when instance state lookup fails", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{err: errors.New("db down")},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/123",
			Headers:    map[string]string{"Accept": "text/html"},
		}))
		ctx.SetParam("id", "123")
		err := h.HandleGetObject(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 404, liftErr.StatusCode)
	})

	t.Run("locked when instance is locked", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: true}},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/123",
			Headers:    map[string]string{"Accept": "text/html"},
		}))
		ctx.SetParam("id", "123")
		err := h.HandleGetObject(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 404, liftErr.StatusCode)
	})

	t.Run("authorized fetch conversion failure returns 400", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     "bad method",
			Path:       "/objects/123",
			Headers:    map[string]string{"Accept": "application/activity+json", "Host": "example.com"},
		}))
		ctx.SetParam("id", "123")
		err := h.HandleGetObject(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 400, liftErr.StatusCode)
		require.Equal(t, "REQUEST_CONVERSION_ERROR", liftErr.Code)
	})

	t.Run("authorized fetch missing signature returns 401", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/123",
			Headers:    map[string]string{"Accept": "application/activity+json", "Host": "example.com"},
		}))
		ctx.SetParam("id", "123")
		err := h.HandleGetObject(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 401, liftErr.StatusCode)
		require.Equal(t, "UNAUTHORIZED", liftErr.Code)
	})

	t.Run("authorized fetch invalid signature returns 403", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("bad signature")},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/123",
			Headers:    map[string]string{"Accept": "application/activity+json", "Host": "example.com", "Signature": "sig"},
		}))
		ctx.SetParam("id", "123")
		err := h.HandleGetObject(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 403, liftErr.StatusCode)
		require.Equal(t, "FORBIDDEN", liftErr.Code)
	})
}

func TestHandleGetObject_FetchResponses_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	instanceRepo := &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}}

	t.Run("not found error returns 404", func(t *testing.T) {
		objRepo := &fakeObjectRepo{err: errors.New("not found")}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/123",
			Headers:    map[string]string{"Accept": "application/activity+json"},
		}))
		ctx.SetParam("id", "123")
		err := h.HandleGetObject(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 404, liftErr.StatusCode)
		require.Contains(t, objRepo.gotID, "https://example.com/objects/123")
	})

	t.Run("internal fetch error returns 500", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{err: errors.New("boom")},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/https://example.com/objects/123",
			Headers:    map[string]string{"accept": "application/activity+json"},
		}))
		ctx.SetParam("id", "https://example.com/objects/123")
		err := h.HandleGetObject(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 500, liftErr.StatusCode)
		require.Equal(t, "OBJECT_FETCH_ERROR", liftErr.Code)
	})

	t.Run("html response for browsers", func(t *testing.T) {
		now := time.Now().UTC()
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:           "https://example.com/objects/123",
				Type:         "Note",
				Summary:      "cw",
				Sensitive:    true,
				Published:    &now,
				Updated:      &now,
			},
			Content:      "Hello <b>world</b>",
			AttributedTo: "https://example.com/users/alice",
			Attachment: []activitypub.Attachment{
				{Type: "Image", URL: "https://example.com/img.png", Name: "img"},
			},
			Tag: []activitypub.Tag{
				{Type: "Hashtag", Href: "https://example.com/tags/cats", Name: "#cats"},
			},
		}

		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: note},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/123",
			Headers:    map[string]string{"Accept": "text/html"},
		}))
		ctx.SetParam("id", "123")
		require.NoError(t, h.HandleGetObject(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Equal(t, "text/html; charset=utf-8", ctx.Response.Headers["Content-Type"])
		body, ok := ctx.Response.Body.(string)
		require.True(t, ok)
		require.Contains(t, body, "<!DOCTYPE html>")
		require.Contains(t, body, "Content Warning")
		require.Contains(t, body, "@alice")
	})

	t.Run("activitypub json response", func(t *testing.T) {
		objRepo := &fakeObjectRepo{obj: map[string]any{"id": "x", "type": "Note"}}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:     http.MethodGet,
			Path:       "/objects/123",
			Headers:    map[string]string{"Accept": "application/activity+json"},
		}))
		ctx.SetParam("id", "123")
		require.NoError(t, h.HandleGetObject(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Equal(t, "application/activity+json", ctx.Response.Headers["Content-Type"])
	})
}

func TestExtractObjectData_Round12(t *testing.T) {
	h := &Handler{}

	data := h.extractObjectData(123)
	require.Equal(t, "Object", data.objectType)

	now := time.Now().UTC()
	data = h.extractObjectData(map[string]any{
		"type":         "Article",
		"id":           "https://example.com/objects/1",
		"content":      "<p>content</p>",
		"name":         "Title",
		"summary":      "Summary",
		"published":    now.Format(time.RFC3339),
		"updated":      now,
		"sensitive":    true,
		"attributedTo": "https://example.com/users/alice",
		"attachment":   []map[string]any{{"type": "Image", "url": "https://example.com/img.png", "name": "img"}},
		"tag":          []map[string]any{{"type": "Hashtag", "href": "https://example.com/tags/cats", "name": "#cats"}},
	})
	require.Equal(t, "Article", data.objectType)
	require.Equal(t, "Title", data.name)
	require.True(t, data.sensitive)
	require.Len(t, data.attachments, 1)
	require.Len(t, data.tags, 1)
}

type extendedMockDB struct {
	inner *dynamormMocks.MockDB
}

var _ dynamormCore.ExtendedDB = (*extendedMockDB)(nil)

func (db *extendedMockDB) Model(model any) dynamormCore.Query {
	return db.inner.Model(model)
}

func (db *extendedMockDB) Transaction(fn func(tx *dynamormCore.Tx) error) error {
	// Transaction behavior isn't relevant for these tests.
	return fn(nil)
}

func (db *extendedMockDB) Migrate() error { return nil }

func (db *extendedMockDB) AutoMigrate(models ...any) error { return nil }

func (db *extendedMockDB) Close() error { return nil }

func (db *extendedMockDB) WithContext(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) AutoMigrateWithOptions(_ any, _ ...any) error { return nil }

func (db *extendedMockDB) RegisterTypeConverter(_ reflect.Type, _ pkgtypes.CustomConverter) error {
	return nil
}

func (db *extendedMockDB) CreateTable(_ any, _ ...any) error { return nil }

func (db *extendedMockDB) EnsureTable(_ any) error { return nil }

func (db *extendedMockDB) DeleteTable(_ any) error { return nil }

func (db *extendedMockDB) DescribeTable(_ any) (any, error) { return nil, nil }

func (db *extendedMockDB) WithLambdaTimeout(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) WithLambdaTimeoutBuffer(_ time.Duration) dynamormCore.DB { return db }

func (db *extendedMockDB) TransactionFunc(fn func(tx any) error) error { return fn(nil) }

func (db *extendedMockDB) Transact() dynamormCore.TransactionBuilder { return nil }

func (db *extendedMockDB) TransactWrite(_ context.Context, fn func(dynamormCore.TransactionBuilder) error) error {
	return fn(nil)
}

func TestNewHandler_MainAndInit_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	origRepos := repos
	origLambdaCtx := lambdaCtx
	origMust := mustInitializeLambdaFn
	origDefaults := initializeWithDefaultsFn
	origStart := lambdaStartFn
	origNewHandler := newHandlerFn
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
		repos = origRepos
		lambdaCtx = origLambdaCtx
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origDefaults
		lambdaStartFn = origStart
		newHandlerFn = origNewHandler
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	innerDB := new(dynamormMocks.MockDB)
	db := &extendedMockDB{inner: innerDB}
	repoFactory, err := factory.NewRepositoryFactory(db, "test-table", zap.NewNop())
	require.NoError(t, err)
	repos = repoFactory

	t.Run("NewHandler wires dependencies", func(t *testing.T) {
		h := NewHandler()
		require.NotNil(t, h.objectRepo)
		require.NotNil(t, h.authorizedFetchService)
		require.NotNil(t, h.instanceRepo)
	})

	t.Run("initializeObjects + main register routes", func(t *testing.T) {
		mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
			return &common.LambdaContext{
				Config:   cfg,
				Logger:   nil,
				Repos:    repos,
				StartTime: time.Now(),
			}
		}
		initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }

		initializeObjects()
		require.NotNil(t, lambdaCtx)
		require.NotNil(t, repos)

		fakeRepo := &fakeObjectRepo{obj: map[string]any{"id": "x", "type": "Note"}}
		newHandlerFn = func() *Handler {
			return &Handler{
				objectRepo:             fakeRepo,
				authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
				instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			}
		}

		called := false
		lambdaStartFn = func(handler any) {
			called = true
			fn, ok := handler.(func(context.Context, interface{}) (interface{}, error))
			require.True(t, ok)

			event := map[string]any{
				"version":  "2.0",
				"routeKey": "GET /objects/123",
				"rawPath":  "/objects/123",
				"headers": map[string]any{
					"accept": "application/activity+json",
				},
				"requestContext": map[string]any{
					"requestId": "req",
					"http": map[string]any{
						"method": "GET",
						"path":   "/objects/123",
					},
				},
			}

			resp, err := fn(context.Background(), event)
			require.NoError(t, err)
			liftResp, ok := resp.(*lift.Response)
			require.True(t, ok)
			require.Equal(t, 200, liftResp.StatusCode)
			require.Equal(t, "application/activity+json", liftResp.Headers["Content-Type"])
		}

		main()
		require.True(t, called)
		require.Contains(t, fakeRepo.gotID, "https://example.com/objects/123")
	})
}

func TestObjectHelpers_UncoveredBranches_Round12(t *testing.T) {
	h := &Handler{}
	require.True(t, h.parseDateTime(time.Now()).After(time.Time{}))
	require.True(t, h.parseDateTime("not-a-time").IsZero())
	require.Equal(t, "", h.generateWarningHTML(false, "cw"))
	require.Equal(t, "", h.generateUpdatedHTML(time.Time{}))

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      http.MethodGet,
		Path:        "/objects/123",
		Headers:     map[string]string{"Some": "Header"},
		QueryParams: map[string]string{"q": "1"},
	}))
	ctx.Request.Headers["Host"] = "example.com"
	req, err := h.convertLiftRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/objects/123?q=1", req.URL.String())
}
