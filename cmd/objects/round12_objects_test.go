package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormMocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	pkgtypes "github.com/theory-cloud/tabletheory/pkg/types"
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
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/objects/"},
			Params:  map[string]string{},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 422, resp.Status)
	})

	t.Run("locked when instance state lookup fails", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{err: errors.New("db down")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
	})

	t.Run("locked when instance is locked", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: true}},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
	})

	t.Run("authorized fetch conversion failure returns 400", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: "bad method",
				Path:   "/objects/123",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 400, resp.Status)
	})

	t.Run("authorized fetch missing signature returns 401", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/objects/123",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
	})

	t.Run("authorized fetch invalid signature returns 403", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("bad signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/objects/123",
				Headers: map[string][]string{
					"accept":    {"application/activity+json"},
					"host":      {"example.com"},
					"signature": {"sig"},
				},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 403, resp.Status)
	})

	t.Run("authorized fetch missing signature returns 401 when accept is missing", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"host": {"example.com"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
	})

	t.Run("authorized fetch missing signature returns 401 for accept */*", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/objects/123",
				Headers: map[string][]string{
					"accept": {"*/*"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
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
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
		require.Contains(t, objRepo.gotID, "https://example.com/objects/123")
	})

	t.Run("internal fetch error returns 500", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{err: errors.New("boom")},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/https://example.com/objects/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "https://example.com/objects/123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 500, resp.Status)
	})

	t.Run("html response for browsers", func(t *testing.T) {
		now := time.Now().UTC()
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://example.com/objects/123",
				Type:      "Note",
				Summary:   "cw",
				Sensitive: true,
				Published: &now,
				Updated:   &now,
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
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"text/html; charset=utf-8"}, resp.Headers["content-type"])
		body := string(resp.Body)
		require.Contains(t, body, "<!DOCTYPE html>")
		require.Contains(t, body, "Content Warning")
		require.Contains(t, body, "@alice")
	})

	t.Run("authorized fetch enabled suppresses HTML for non-public objects", func(t *testing.T) {
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/objects/123",
				Type: "Note",
			},
			Content:      "secret",
			AttributedTo: "https://example.com/users/alice",
		}

		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: note},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
	})

	t.Run("authorized fetch enabled still allows HTML for public objects", func(t *testing.T) {
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/objects/123",
				Type: "Note",
				To:   []string{activitypub.PublicAddress},
			},
			Content:      "hello",
			AttributedTo: "https://example.com/users/alice",
		}

		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: note},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"text/html; charset=utf-8"}, resp.Headers["content-type"])
		require.Contains(t, string(resp.Body), "<!DOCTYPE html>")
	})

	t.Run("activitypub json response", func(t *testing.T) {
		objRepo := &fakeObjectRepo{obj: map[string]any{"id": "x", "type": "Note"}}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"application/activity+json"}, resp.Headers["content-type"])
	})

	t.Run("canonical status route resolves published status url", func(t *testing.T) {
		now := time.Now().UTC()
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://example.com/users/alice/statuses/123",
				Type:      activitypub.NoteType,
				Published: &now,
			},
			Content:      "hello from canonical route",
			AttributedTo: "https://example.com/users/alice",
		}

		objRepo := &fakeObjectRepo{obj: note}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/users/alice/statuses/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{
				"username": "alice",
				"id":       "123",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, "https://example.com/users/alice/statuses/123", objRepo.gotID)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "https://example.com/users/alice/statuses/123", body["id"])
	})

	t.Run("canonical status route preserves authorized fetch behavior", func(t *testing.T) {
		objRepo := &fakeObjectRepo{obj: map[string]any{"id": "https://example.com/users/alice/statuses/123", "type": "Note"}}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/statuses/123",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{
				"username": "alice",
				"id":       "123",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
		require.Empty(t, objRepo.gotID)
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
				Config:    cfg,
				Logger:    nil,
				Repos:     repos,
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
			fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
			require.True(t, ok)

			event := events.APIGatewayV2HTTPRequest{
				Version:  "2.0",
				RouteKey: "GET /objects/123",
				RawPath:  "/objects/123",
				Headers:  map[string]string{"accept": "application/activity+json"},
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					RequestID: "req",
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: "GET",
						Path:   "/objects/123",
					},
				},
			}

			raw, err := json.Marshal(event)
			require.NoError(t, err)
			resp, err := fn(context.Background(), raw)
			require.NoError(t, err)
			lambdaResp, ok := resp.(events.APIGatewayV2HTTPResponse)
			require.True(t, ok)
			require.Equal(t, 200, lambdaResp.StatusCode)
			require.Equal(t, "application/activity+json", lambdaResp.Headers["content-type"])
		}

		main()
		require.True(t, called)
		require.Contains(t, fakeRepo.gotID, "https://example.com/objects/123")
	})

	t.Run("build app serves canonical status routes", func(t *testing.T) {
		fakeRepo := &fakeObjectRepo{obj: map[string]any{"id": "https://example.com/users/alice/statuses/123", "type": "Note"}}
		app := buildApp(&Handler{
			objectRepo:             fakeRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		}, zap.NewNop())

		event := events.APIGatewayV2HTTPRequest{
			Version:  "2.0",
			RouteKey: "GET /users/alice/statuses/123",
			RawPath:  "/users/alice/statuses/123",
			Headers:  map[string]string{"accept": "application/activity+json"},
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				RequestID: "req-canonical",
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: "GET",
					Path:   "/users/alice/statuses/123",
				},
			},
		}

		raw, err := json.Marshal(event)
		require.NoError(t, err)

		resp, err := app.HandleLambda(context.Background(), raw)
		require.NoError(t, err)
		lambdaResp, ok := resp.(events.APIGatewayV2HTTPResponse)
		require.True(t, ok)
		require.Equal(t, 200, lambdaResp.StatusCode)
		require.Equal(t, "https://example.com/users/alice/statuses/123", fakeRepo.gotID)
		require.Equal(t, "application/activity+json", lambdaResp.Headers["content-type"])
	})
}

func TestObjectHelpers_UncoveredBranches_Round12(t *testing.T) {
	h := &Handler{}
	require.True(t, h.parseDateTime(time.Now()).After(time.Time{}))
	require.True(t, h.parseDateTime("not-a-time").IsZero())
	require.Equal(t, "", h.generateWarningHTML(false, "cw"))
	require.Equal(t, "", h.generateUpdatedHTML(time.Time{}))

	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodGet,
			Path:   "/objects/123",
			Headers: map[string][]string{
				"host":                     {"internal.execute-api.us-east-1.amazonaws.com"},
				"x-lesser-forwarded-host":  {"example.com"},
				"x-lesser-forwarded-proto": {"https"},
				"some":                     {"Header"},
			},
			Query: map[string][]string{"q": {"1"}},
		},
	}
	req, err := h.convertAppTheoryRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/objects/123?q=1", req.URL.String())
	require.Equal(t, "example.com", req.Host)
	require.Equal(t, "example.com", req.Header.Get("Host"))
}

func TestObjectsSecurityHeaders_Round12(t *testing.T) {
	mw := objectsActivityPubSecurityHeaders()
	wrapped := mw(func(_ *apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{
			Status: 200,
			Headers: map[string][]string{
				"content-type": {"text/html; charset=utf-8"},
			},
			Body: []byte("<!DOCTYPE html><html></html>"),
		}, nil
	})

	resp, err := wrapped(&apptheory.Context{
		Request: apptheory.Request{Method: http.MethodGet, Path: "/objects/123"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Headers["content-security-policy"])
	require.Contains(t, resp.Headers["content-security-policy"][0], "script-src 'none'")
}
