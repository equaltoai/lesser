package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"github.com/theory-cloud/tabletheory/v2"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/streaming"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
)

type fakeStreamQueue struct{}

func (f *fakeStreamQueue) QueueEventForUser(context.Context, string, string, map[string]interface{}) error {
	return nil
}
func (f *fakeStreamQueue) QueueEventForStream(context.Context, string, string, map[string]interface{}) error {
	return nil
}
func (f *fakeStreamQueue) QueueEventForConversation(context.Context, string, string, map[string]interface{}) error {
	return nil
}
func (f *fakeStreamQueue) QueueEventForFollowers(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func TestCreateAuthMiddlewareWithService_Round12(t *testing.T) {
	originalLogger := logger
	t.Cleanup(func() { logger = originalLogger })
	logger = zaptest.NewLogger(t)

	secret := "secret"
	oauthService := auth.NewOAuthService(secret, &config.Config{}, nil, nil)
	mw := createAuthMiddlewareWithService(oauthService, logger)

	claims := &auth.Claims{
		Username: "alice",
		Scopes:   []string{"read"},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)

	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodPost,
			Path:   "/graphql",
			Headers: map[string][]string{
				"authorization": {"Bearer " + token},
			},
		},
	}
	resp, err := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
		require.NotNil(t, c.AuthPrincipal)
		require.Equal(t, "alice", c.AuthIdentity)
		require.Equal(t, "alice", auth.GetAuthenticatedUsername(c))
		return &apptheory.Response{Status: http.StatusOK}, nil
	})(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)

	ctx = &apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodPost,
			Path:   "/graphql",
			Headers: map[string][]string{
				"authorization": {"Bearer not-a-token"},
			},
		},
	}
	resp, err = mw(func(c *apptheory.Context) (*apptheory.Response, error) {
		require.Nil(t, c.AuthPrincipal)
		require.False(t, auth.IsAuthenticated(c))
		return &apptheory.Response{Status: http.StatusOK}, nil
	})(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
}

func TestGraphQLResponseWriter_AndBytesReader_Round12(t *testing.T) {
	w := newGraphQLResponseWriter()
	w.Header().Set("X-Test", "1")
	_, err := w.Write([]byte("body"))
	require.NoError(t, err)
	resp := w.Response()
	require.Equal(t, 200, resp.Status)
	require.Equal(t, []string{"1"}, resp.Headers["x-test"])
	require.Equal(t, []byte("body"), resp.Body)

	w2 := newGraphQLResponseWriter()
	w2.WriteHeader(201)
	_, err = w2.Write([]byte("created"))
	require.NoError(t, err)
	resp2 := w2.Response()
	require.Equal(t, 201, resp2.Status)
	require.Equal(t, []byte("created"), resp2.Body)

	r := &bytesReader{data: []byte("abc")}
	buf := make([]byte, 2)
	n, err := r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, "ab", string(buf[:n]))
	n, err = r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "c", string(buf[:n]))
	_, err = r.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, r.Close())
}

func TestCreateMiddlewares_Round12(t *testing.T) {
	originalRepos := repos
	originalLogger := logger
	originalCfg := cfg
	originalCostTracker := costTracker
	originalCostSvc := costTrackingService
	t.Cleanup(func() {
		repos = originalRepos
		logger = originalLogger
		cfg = originalCfg
		costTracker = originalCostTracker
		costTrackingService = originalCostSvc
	})

	logger = zap.NewNop()
	repos = nil
	cfg = &config.Config{JWTSecret: "secret"}
	costTracker = cost.New()
	costTrackingService = nil

	t.Run("data_loader_sets_loaders", func(t *testing.T) {
		mw := createDataLoaderMiddleware()
		called := false
		next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
			called = true
			val := ctx.Get("loaders")
			_, ok := val.(*graph.Loaders)
			require.True(t, ok)
			return &apptheory.Response{Status: 200}, nil
		}
		ctx := &apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/graphql"},
		}
		_, err := mw(next)(ctx)
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("cost_tracking_sets_tracker", func(t *testing.T) {
		mw := createCostTrackingMiddleware()
		next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
			require.Same(t, costTracker, ctx.Get("cost_tracker"))
			return &apptheory.Response{Status: 200}, nil
		}
		ctx := &apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/graphql"},
		}
		_, err := mw(next)(ctx)
		require.NoError(t, err)
	})

	t.Run("auth_middleware_constructs", func(t *testing.T) {
		mw := createAuthMiddleware()
		require.NotNil(t, mw)
	})
}

func TestInitializeGraphQL_Branches_Round12(t *testing.T) {
	originalRunningUnitTests := runningUnitTestsFn
	originalMustInitialize := mustInitializeLambdaFn
	originalInitializeWithDefaults := initializeWithDefaultsFn
	originalExtract := extractStandardizedServicesFn
	originalManual := initializeManualServicesFn
	originalSpecific := initializeGraphQLSpecificServicesFn
	originalNewClient := newLambdaOptimizedClientFn
	originalNewFactory := newRepositoryFactoryFn
	originalLambdaCtx := lambdaCtx
	originalInitTime := initTime

	t.Cleanup(func() {
		runningUnitTestsFn = originalRunningUnitTests
		mustInitializeLambdaFn = originalMustInitialize
		initializeWithDefaultsFn = originalInitializeWithDefaults
		extractStandardizedServicesFn = originalExtract
		initializeManualServicesFn = originalManual
		initializeGraphQLSpecificServicesFn = originalSpecific
		newLambdaOptimizedClientFn = originalNewClient
		newRepositoryFactoryFn = originalNewFactory
		lambdaCtx = originalLambdaCtx
		initTime = originalInitTime
	})

	runningUnitTestsFn = func() bool { return false }

	var mustInitCalls int
	mustInitializeLambdaFn = func(cfg common.LambdaConfig) *common.LambdaContext {
		mustInitCalls++
		require.Equal(t, "graphql", cfg.ServiceName)
		require.Equal(t, common.LambdaTypeAPI, cfg.LambdaType)
		require.Equal(t, 30*time.Second, cfg.RequestTimeout)
		return &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &config.Config{DynamoTableName: "tbl", Region: "us-east-1"},
		}
	}

	t.Run("defaults_success_uses_standardized_extraction", func(t *testing.T) {
		var extracted, manual, specific int
		initializeWithDefaultsFn = func(*common.LambdaContext) error { return nil }
		newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
			return &tabletheory.LambdaDB{}, nil
		}
		newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (storagecore.RepositoryStorage, error) {
			return &testingmocks.MockRepositoryStorage{}, nil
		}
		extractStandardizedServicesFn = func() { extracted++ }
		initializeManualServicesFn = func() { manual++ }
		initializeGraphQLSpecificServicesFn = func() { specific++ }

		initializeGraphQL()
		require.Equal(t, 1, mustInitCalls)
		require.Equal(t, 1, extracted)
		require.Equal(t, 0, manual)
		require.Equal(t, 1, specific)
		require.False(t, initTime.IsZero())
		require.NotNil(t, lambdaCtx)
	})

	t.Run("defaults_error_falls_back_to_manual_init", func(t *testing.T) {
		var extracted, manual, specific int
		initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
		newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
			return nil, errors.New("boom")
		}
		extractStandardizedServicesFn = func() { extracted++ }
		initializeManualServicesFn = func() { manual++ }
		initializeGraphQLSpecificServicesFn = func() { specific++ }

		initializeGraphQL()
		require.Equal(t, 2, mustInitCalls)
		require.Equal(t, 0, extracted)
		require.Equal(t, 1, manual)
		require.Equal(t, 1, specific)
	})

	t.Run("on_start_respects_running_unit_tests_flag", func(t *testing.T) {
		mustInitCalls = 0
		initializeWithDefaultsFn = func(*common.LambdaContext) error { return nil }
		newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
			return &tabletheory.LambdaDB{}, nil
		}
		newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (storagecore.RepositoryStorage, error) {
			return &testingmocks.MockRepositoryStorage{}, nil
		}
		extractStandardizedServicesFn = func() {}
		initializeManualServicesFn = func() {}
		initializeGraphQLSpecificServicesFn = func() {}

		runningUnitTestsFn = func() bool { return false }
		initializeGraphQLOnStart()
		require.Equal(t, 1, mustInitCalls)

		mustInitCalls = 0
		runningUnitTestsFn = func() bool { return true }
		initializeGraphQLOnStart()
		require.Equal(t, 0, mustInitCalls)
	})
}

type fakeInvocationTracker struct {
	called chan cost.LambdaOperation
}

func (f *fakeInvocationTracker) TrackLambdaInvocation(_ context.Context, op cost.LambdaOperation) error {
	f.called <- op
	return nil
}

func TestCreateCostTrackingMiddleware_CentralizedTrackingBranch_Round12(t *testing.T) {
	originalLogger := logger
	originalTracker := costTracker
	originalService := costTrackingService
	originalEnv := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
	t.Cleanup(func() {
		logger = originalLogger
		costTracker = originalTracker
		costTrackingService = originalService
		_ = os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", originalEnv)
	})

	logger = zap.NewNop()
	costTracker = cost.New()
	inv := &fakeInvocationTracker{called: make(chan cost.LambdaOperation, 1)}
	costTrackingService = inv
	require.NoError(t, os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "256"))

	mw := createCostTrackingMiddleware()
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{Status: 200}, nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/ready"}}
	_, err := mw(next)(ctx)
	require.NoError(t, err)

	select {
	case op := <-inv.called:
		require.Equal(t, "graphql", op.FunctionName)
		require.Equal(t, int64(256), op.MemoryMB)
	case <-time.After(time.Second):
		t.Fatal("expected TrackLambdaInvocation to be called")
	}
}

func TestHandlePlayground_AndHandleGraphQL_Round12(t *testing.T) {
	originalCfg := cfg
	originalLogger := logger
	originalHandler := graphQLHandler
	t.Cleanup(func() {
		cfg = originalCfg
		logger = originalLogger
		graphQLHandler = originalHandler
	})

	logger = zap.NewNop()
	cfg = &config.Config{EnablePlayground: false}

	resp, err := handlePlayground(&apptheory.Context{
		Request: apptheory.Request{Method: http.MethodGet, Path: "/playground"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 404, resp.Status)

	cfg.EnablePlayground = true
	resp, err = handlePlayground(&apptheory.Context{
		Request: apptheory.Request{Method: http.MethodGet, Path: "/playground"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.NotEmpty(t, resp.Body)

	costTracker = cost.New()
	fakeClaims := &auth.Claims{Username: "claims-user", Scopes: []string{"read"}}
	graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		require.Equal(t, `{"query":"{ me { id } }"}`, string(body))
		require.Equal(t, "claims-user", r.Context().Value(contextKeyUser).(string))
		claimsVal := r.Context().Value(common.ContextKeyClaims)
		claims, ok := claimsVal.(common.Claims)
		require.True(t, ok)
		require.Equal(t, "claims-user", claims.GetUsername())
		require.Equal(t, "ct", r.Context().Value(contextKeyCostTracker))
		w.Header().Set("X-GraphQL", "ok")
		w.WriteHeader(202)
		_, _ = w.Write([]byte("graphql-ok"))
	})

	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodPost,
			Path:   "/graphql",
			Body:   []byte(`{"query":"{ me { id } }"}`),
			Headers: map[string][]string{
				"authorization": {"Bearer test"},
			},
		},
	}
	ctx.Set("user", "ignored")
	ctx.Set("username", "ignored-too")
	ctx.Set("claims", fakeClaims)
	ctx.Set("cost_tracker", "ct")
	ctx.Set("loaders", &graph.Loaders{})
	resp, err = handleGraphQL(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 202, resp.Status)
	require.Equal(t, []string{"ok"}, resp.Headers["x-graphql"])
	require.Equal(t, []byte("graphql-ok"), resp.Body)
}

func TestHandleGraphQL_AdditionalBranches_Round12(t *testing.T) {
	originalLogger := logger
	originalHandler := graphQLHandler
	t.Cleanup(func() {
		logger = originalLogger
		graphQLHandler = originalHandler
	})

	logger = zap.NewNop()

	t.Run("claims_wrong_type_and_loaders_wrong_type", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "bad-loaders", r.Context().Value(contextKeyLoaders))
			require.Nil(t, r.Context().Value(common.ContextKeyClaims))
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/graphql",
				Body:   []byte(`{}`),
			},
		}
		ctx.Set("is_authenticated", true)
		ctx.Set("username", "alice")
		ctx.Set("claims", "not-claims")
		ctx.Set("loaders", "bad-loaders")
		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
	})

	t.Run("claims_empty_username", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claimsVal := r.Context().Value(common.ContextKeyClaims)
			claims, ok := claimsVal.(common.Claims)
			require.True(t, ok)
			require.Equal(t, "", claims.GetUsername())
			require.Nil(t, r.Context().Value(contextKeyUser))
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/graphql", Body: []byte(`{}`)},
		}
		ctx.Set("is_authenticated", true)
		ctx.Set("claims", &auth.Claims{})
		_, err := handleGraphQL(ctx)
		require.NoError(t, err)
	})

	t.Run("no_claims_no_loaders", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/graphql", Body: []byte(`{}`)},
		}
		ctx.Set("is_authenticated", true)
		ctx.Set("username", "alice")
		_, err := handleGraphQL(ctx)
		require.NoError(t, err)
	})

	t.Run("unauthenticated_rejected", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			require.Fail(t, "graphql handler should not be invoked for unauthenticated requests")
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/graphql", Body: []byte(`{}`)},
		}
		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})
}

func TestHandleGraphQL_AnonymousPublicQueryAllowlist(t *testing.T) {
	originalLogger := logger
	originalHandler := graphQLHandler
	t.Cleanup(func() {
		logger = originalLogger
		graphQLHandler = originalHandler
	})

	logger = zap.NewNop()

	t.Run("allows_public_query_without_auth", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, readErr := io.ReadAll(r.Body)
			require.NoError(t, readErr)
			require.Equal(t, `{"query":"query PublicSurface { instance { domain } }","operationName":"PublicSurface"}`, string(body))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/graphql",
				Body:   []byte(`{"query":"query PublicSurface { instance { domain } }","operationName":"PublicSurface"}`),
			},
		}

		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, []byte("ok"), resp.Body)
	})

	t.Run("allows_public_query_fragments_without_auth", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/graphql",
				Body:   []byte(`{"query":"query PublicObject { ...VisibleFields } fragment VisibleFields on Query { object(id: \"status-1\") { id } }"}`),
			},
		}

		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("rejects_non_public_query_without_auth", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			require.Fail(t, "graphql handler should not be invoked for disallowed anonymous requests")
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/graphql",
				Body:   []byte(`{"query":"{ viewer { id } }"}`),
			},
		}

		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("rejects_account_quote_permissions_without_auth", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			require.Fail(t, "graphql handler should not be invoked for anonymous quote permission reads")
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/graphql",
				Body:   []byte(`{"query":"{ accountQuotePermissions(username: \"alice\") { username allowPublic } }"}`),
			},
		}

		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("rejects_mutation_without_auth", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			require.Fail(t, "graphql handler should not be invoked for anonymous mutations")
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/graphql",
				Body:   []byte(`{"query":"mutation { dismissAnnouncement(id: \"ann-1\") }"}`),
			},
		}

		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})
}

func TestGraphQLAnonymousRequestHelpers_Round12(t *testing.T) {
	originalLogger := logger
	t.Cleanup(func() { logger = originalLogger })
	logger = zap.NewNop()

	t.Run("graphqlFirstRequestValue", func(t *testing.T) {
		require.Empty(t, graphqlFirstRequestValue(nil, "query"))
		require.Empty(t, graphqlFirstRequestValue(map[string][]string{"other": {"x"}}, "query"))
		require.Equal(t, "query-value", graphqlFirstRequestValue(map[string][]string{"query": {"query-value", "ignored"}}, "query"))
	})

	t.Run("graphqlExtractOperation", func(t *testing.T) {
		ctxQuery := &apptheory.Context{
			Request: apptheory.Request{
				Query: map[string][]string{
					"query":         {" query FromQuery { instance { domain } } "},
					"operationName": {" FromQuery "},
				},
			},
		}
		query, operationName := graphqlExtractOperation(ctxQuery)
		require.Equal(t, "query FromQuery { instance { domain } }", query)
		require.Equal(t, "FromQuery", operationName)

		ctxJSON := &apptheory.Context{
			Request: apptheory.Request{
				Body: []byte(`{"query":"query FromBody { instance { domain } }","operationName":"FromBody"}`),
			},
		}
		query, operationName = graphqlExtractOperation(ctxJSON)
		require.Equal(t, "query FromBody { instance { domain } }", query)
		require.Equal(t, "FromBody", operationName)

		ctxRaw := &apptheory.Context{
			Request: apptheory.Request{
				Body: []byte(`query RawBody { instance { domain } }`),
			},
		}
		query, operationName = graphqlExtractOperation(ctxRaw)
		require.Equal(t, "query RawBody { instance { domain } }", query)
		require.Empty(t, operationName)

		query, operationName = graphqlExtractOperation(&apptheory.Context{Request: apptheory.Request{Body: []byte("   ")}})
		require.Empty(t, query)
		require.Empty(t, operationName)
	})

	t.Run("graphqlSelectOperation", func(t *testing.T) {
		require.Nil(t, graphqlSelectOperation(nil, ""))

		singleDoc, err := parser.ParseQuery(&ast.Source{Input: `query { instance { domain } }`})
		require.NoError(t, err)
		operation := graphqlSelectOperation(singleDoc, "")
		require.NotNil(t, operation)
		require.Equal(t, ast.Query, operation.Operation)

		multiDoc, err := parser.ParseQuery(&ast.Source{Input: `
			query First { instance { domain } }
			query Second { announcements { id } }
		`})
		require.NoError(t, err)
		require.Nil(t, graphqlSelectOperation(multiDoc, ""))
		require.NotNil(t, graphqlSelectOperation(multiDoc, "Second"))
		require.Nil(t, graphqlSelectOperation(multiDoc, "Missing"))
	})

	t.Run("graphqlCollectTopLevelFields_and_find_fragment", func(t *testing.T) {
		doc, err := parser.ParseQuery(&ast.Source{Input: `
			query PublicSurface {
				...VisibleFields
				...VisibleFields
				... on Query { customEmojis { shortcode } }
				__typename
			}

			fragment VisibleFields on Query {
				instance { domain }
				announcements { id }
			}
		`})
		require.NoError(t, err)

		operation := graphqlSelectOperation(doc, "PublicSurface")
		require.NotNil(t, operation)

		fields, ok := graphqlCollectTopLevelFields(doc, operation.SelectionSet, map[string]struct{}{})
		require.True(t, ok)
		require.Equal(t, []string{"instance", "announcements", "customEmojis", "__typename"}, fields)

		require.NotNil(t, graphqlFindFragment(doc, "VisibleFields"))
		require.Nil(t, graphqlFindFragment(doc, "Missing"))
		require.Nil(t, graphqlFindFragment(nil, "VisibleFields"))

		missingFragmentFields, ok := graphqlCollectTopLevelFields(&ast.QueryDocument{}, ast.SelectionSet{
			&ast.FragmentSpread{Name: "Missing"},
		}, map[string]struct{}{})
		require.False(t, ok)
		require.Nil(t, missingFragmentFields)

		blankFieldFields, ok := graphqlCollectTopLevelFields(doc, ast.SelectionSet{
			&ast.Field{Name: "   "},
		}, map[string]struct{}{})
		require.False(t, ok)
		require.Nil(t, blankFieldFields)
	})

	t.Run("graphqlAnonymousRequestAllowed", func(t *testing.T) {
		authCtx := &apptheory.Context{}
		authCtx.Set("is_authenticated", true)
		require.True(t, graphqlAnonymousRequestAllowed(authCtx))

		require.False(t, graphqlAnonymousRequestAllowed(&apptheory.Context{}))

		invalidCtx := &apptheory.Context{
			Request: apptheory.Request{Body: []byte(`{"query":"query { instance( }"}`)},
		}
		require.False(t, graphqlAnonymousRequestAllowed(invalidCtx))

		multipleOpsCtx := &apptheory.Context{
			Request: apptheory.Request{Body: []byte(`{"query":"query One { instance { domain } } query Two { announcements { id } }"}`)},
		}
		require.False(t, graphqlAnonymousRequestAllowed(multipleOpsCtx))

		allowedCtx := &apptheory.Context{
			Request: apptheory.Request{
				Query: map[string][]string{
					"query":         {`query PublicOp { actor(username: "alice") { id } } query PrivateOp { viewer { id } }`},
					"operationName": {"PublicOp"},
				},
			},
		}
		require.True(t, graphqlAnonymousRequestAllowed(allowedCtx))

		disallowedCtx := &apptheory.Context{
			Request: apptheory.Request{
				Body: []byte(`{"query":"query { instance { domain } viewer { id } }"}`),
			},
		}
		require.False(t, graphqlAnonymousRequestAllowed(disallowedCtx))
	})
}

func TestExtractStandardizedServices_AndResolveStreamQueue_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalCostTracker := costTracker
	originalCostSvc := costTrackingService
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		costTracker = originalCostTracker
		costTrackingService = originalCostSvc
	})

	mockStorage := &testingmocks.MockRepositoryStorage{}
	lambdaCtx = &common.LambdaContext{
		Config: &config.Config{JWTSecret: "secret"},
		Logger: zap.NewNop(),
		Repos:  mockStorage,
		AWSServices: &awsinit.AWSServices{
			Config: aws.Config{Region: "us-east-1"},
		},
		StreamQueue: streaming.StreamQueueService(&fakeStreamQueue{}),
	}

	extractStandardizedServices()
	require.Same(t, mockStorage, repos.(*testingmocks.MockRepositoryStorage))
	require.NotNil(t, costTracker)
	require.NotNil(t, logger)

	got := resolveStreamQueue()
	require.NotNil(t, got)
}

func TestExtractStandardizedServices_InitializesCentralizedCostTracking_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalLogger := logger
	originalRepos := repos
	originalCostSvc := costTrackingService
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		logger = originalLogger
		repos = originalRepos
		costTrackingService = originalCostSvc
	})

	logger = zap.NewNop()
	mockStorage := &testingmocks.MockRepositoryStorage{}
	cw := cloudwatch.NewFromConfig(aws.Config{Region: "us-east-1"})
	lambdaCtx = &common.LambdaContext{
		Config: &config.Config{JWTSecret: "secret"},
		Logger: logger,
		Repos:  mockStorage,
		AWSServices: &awsinit.AWSServices{
			CloudWatch: cw,
		},
	}

	extractStandardizedServices()
	require.NotNil(t, costTrackingService)
	if svc, ok := costTrackingService.(*cost.TrackingService); ok {
		_ = svc.Close(context.Background())
	}
}

func TestInitializeManualServices_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	originalNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
		newRepositoryFactoryFn = originalNewFactory
	})

	lambdaCtx = &common.LambdaContext{
		Logger: zap.NewNop(),
		Config: &config.Config{
			Region:          "us-east-1",
			DynamoTableName: "",
		},
	}

	var gotTable string
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &tabletheory.LambdaDB{}, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, tableName string, _ *zap.Logger) (storagecore.RepositoryStorage, error) {
		gotTable = tableName
		return &testingmocks.MockRepositoryStorage{}, nil
	}

	initializeManualServices()
	require.NotNil(t, repos)
	require.NotNil(t, lambdaCtx.Repos)
	require.Equal(t, "lesser-main", gotTable)
}

func TestInitializeManualServices_UsesConfiguredTableName_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	originalNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
		newRepositoryFactoryFn = originalNewFactory
	})

	lambdaCtx = &common.LambdaContext{
		Logger: zap.NewNop(),
		Config: &config.Config{
			Region:          "us-east-1",
			DynamoTableName: "custom-table",
		},
	}

	var gotTable string
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &tabletheory.LambdaDB{}, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, tableName string, _ *zap.Logger) (storagecore.RepositoryStorage, error) {
		gotTable = tableName
		return &testingmocks.MockRepositoryStorage{}, nil
	}

	initializeManualServices()
	require.NotNil(t, repos)
	require.Equal(t, "custom-table", gotTable)
}

func TestResolveStreamQueue_FallbackError_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
	})

	logger = zap.NewNop()
	cfg = &config.Config{Region: "us-east-1"}
	repos = nil
	lambdaCtx = &common.LambdaContext{
		StreamQueue: "not-a-queue",
	}

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return nil, errors.New("boom") }
	require.Nil(t, resolveStreamQueue())
}

func TestResolveStreamQueue_UsesLambdaContextDynamoDB_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
	})

	logger = zap.NewNop()
	cfg = &config.Config{Region: "us-east-1", DynamoTableName: "tbl"}
	repos = nil
	lambdaCtx = &common.LambdaContext{DynamoDB: &tabletheory.LambdaDB{}}
	require.NotNil(t, resolveStreamQueue())
}

func TestResolveStreamQueue_UsesReposGetDB_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
	})

	logger = zap.NewNop()
	cfg = &config.Config{Region: "us-east-1", DynamoTableName: "tbl"}

	mockStorage := &testingmocks.MockRepositoryStorage{}
	mockStorage.On("GetDB").Return(&tabletheory.LambdaDB{})
	repos = mockStorage
	lambdaCtx = &common.LambdaContext{}

	require.NotNil(t, resolveStreamQueue())
	mockStorage.AssertExpectations(t)
}

func TestResolveStreamQueue_CreatesClientWhenNoDB_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
	})

	logger = zap.NewNop()
	cfg = &config.Config{Region: "us-east-1", DynamoTableName: "tbl"}
	repos = nil
	lambdaCtx = &common.LambdaContext{}
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &tabletheory.LambdaDB{}, nil }

	require.NotNil(t, resolveStreamQueue())
}

func TestInitializeGraphQLSpecificServices_Round12(t *testing.T) {
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalLambdaCtx := lambdaCtx
	originalHandler := graphQLHandler
	originalCostTracker := costTracker
	t.Cleanup(func() {
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		lambdaCtx = originalLambdaCtx
		graphQLHandler = originalHandler
		costTracker = originalCostTracker
	})

	logger = zap.NewNop()
	cfg = &config.Config{
		Domain:          "example.com",
		JWTSecret:       "secret",
		DisableAI:       true,
		DynamoTableName: "tbl",
		S3BucketName:    "bucket",
		MaxUploadSize:   1024,
	}
	repos = &testingmocks.MockRepositoryStorage{}
	costTracker = cost.New()
	lambdaCtx = &common.LambdaContext{
		Config: cfg,
		Logger: logger,
		AWSServices: &awsinit.AWSServices{
			Config: aws.Config{Region: "us-east-1"},
		},
		Repos:       repos,
		StreamQueue: streaming.StreamQueueService(&fakeStreamQueue{}),
	}

	initializeGraphQLSpecificServices()
	require.NotNil(t, graphQLHandler)

	// Cover AI-enabled and debug tracing branches
	graphQLHandler = nil
	cfg.DisableAI = false
	cfg.DebugMode = true
	initializeGraphQLSpecificServices()
	require.NotNil(t, graphQLHandler)
}

func TestMain_RegistersAndStartsLambda_Round12(t *testing.T) {
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalLambdaCtx := lambdaCtx
	originalStart := lambdaStartFn
	originalGraphQLHandler := graphQLHandler
	t.Cleanup(func() {
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		lambdaCtx = originalLambdaCtx
		lambdaStartFn = originalStart
		graphQLHandler = originalGraphQLHandler
	})

	cfg = &config.Config{
		Domain:          "example.com",
		JWTSecret:       "secret",
		DisableAI:       true,
		DynamoTableName: "tbl",
		S3BucketName:    "bucket",
		MaxUploadSize:   1024,
		DebugMode:       false,
	}
	logger = zap.NewNop()
	repos = &testingmocks.MockRepositoryStorage{}
	costTracker = cost.New()
	lambdaCtx = &common.LambdaContext{
		Config:    cfg,
		Logger:    logger,
		StartTime: time.Now().Add(-time.Hour),
	}

	var started any
	lambdaStartFn = func(h any) { started = h }
	main()
	require.NotNil(t, started)

	h, ok := started.(func(context.Context, json.RawMessage) (any, error))
	require.True(t, ok)
	event := events.APIGatewayV2HTTPRequest{
		Version:  "2.0",
		RouteKey: "GET /ready",
		RawPath:  "/ready",
		Headers:  map[string]string{"accept": "application/json"},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-request-id",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/ready",
			},
			Stage: "$default",
		},
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	result, err := h(context.Background(), raw)
	require.NoError(t, err)
	resp, ok := result.(events.APIGatewayV2HTTPResponse)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)

	raw, err = json.Marshal(events.APIGatewayV2HTTPRequest{
		Version:  "2.0",
		RouteKey: "GET /health",
		RawPath:  "/health",
		Headers:  map[string]string{"accept": "application/json"},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-request-id",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/health",
			},
			Stage: "$default",
		},
	})
	require.NoError(t, err)
	result, err = h(context.Background(), raw)
	require.NoError(t, err)
	resp, ok = result.(events.APIGatewayV2HTTPResponse)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)

	raw, err = json.Marshal(events.APIGatewayV2HTTPRequest{
		Version:  "2.0",
		RouteKey: "OPTIONS /graphql",
		RawPath:  "/graphql",
		Headers:  map[string]string{"origin": "https://example.com"},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-request-id",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "OPTIONS",
				Path:   "/graphql",
			},
			Stage: "$default",
		},
	})
	require.NoError(t, err)
	result, err = h(context.Background(), raw)
	require.NoError(t, err)
	resp, ok = result.(events.APIGatewayV2HTTPResponse)
	require.True(t, ok)
	require.Equal(t, 204, resp.StatusCode)

	graphQLHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		Username: "alice",
		Scopes:   []string{"read"},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString([]byte(cfg.JWTSecret))
	require.NoError(t, err)
	raw, err = json.Marshal(events.APIGatewayV2HTTPRequest{
		Version:  "2.0",
		RouteKey: "GET /graphql",
		RawPath:  "/graphql",
		Headers: map[string]string{
			"accept":        "application/json",
			"authorization": "Bearer " + token,
		},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-request-id",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/graphql",
			},
			Stage: "$default",
		},
	})
	require.NoError(t, err)
	result, err = h(context.Background(), raw)
	require.NoError(t, err)
	resp, ok = result.(events.APIGatewayV2HTTPResponse)
	require.True(t, ok)
	require.Equal(t, 500, resp.StatusCode)

	started = nil
	cfg.DebugMode = true
	main()
	require.NotNil(t, started)
}
