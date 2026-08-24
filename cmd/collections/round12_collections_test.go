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
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	dynamormSchema "github.com/theory-cloud/tabletheory/v3/pkg/schema"
	pkgtypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
	"go.uber.org/zap"
)

type fakeActorRepo struct {
	actor     *activitypub.Actor
	err       error
	cached    map[string]*activitypub.Actor
	cachedErr error
}

func (f *fakeActorRepo) GetActor(context.Context, string) (*activitypub.Actor, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.actor, nil
}

func (f *fakeActorRepo) GetCachedRemoteActor(_ context.Context, handle string) (*activitypub.Actor, error) {
	if f.cachedErr != nil {
		return nil, f.cachedErr
	}
	if actor, ok := f.cached[handle]; ok {
		return actor, nil
	}
	return nil, common.ActorNotFoundError{Username: handle}
}

type fakeRelationshipRepo struct {
	followers       []string
	following       []string
	nextCursor      string
	followersCursor string
	followingCursor string
	countFollowers  int
	countFollowing  int
	err             error
}

func (f *fakeRelationshipRepo) GetFollowers(_ context.Context, _ string, _ int, cursor string) ([]string, string, error) {
	f.followersCursor = cursor
	if f.err != nil {
		return nil, "", f.err
	}
	return append([]string{}, f.followers...), f.nextCursor, nil
}

func (f *fakeRelationshipRepo) GetFollowing(_ context.Context, _ string, _ int, cursor string) ([]string, string, error) {
	f.followingCursor = cursor
	if f.err != nil {
		return nil, "", f.err
	}
	return append([]string{}, f.following...), f.nextCursor, nil
}

func (f *fakeRelationshipRepo) CountFollowers(context.Context, string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.countFollowers, nil
}

func (f *fakeRelationshipRepo) CountFollowing(context.Context, string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.countFollowing, nil
}

type fakeLikeRepo struct {
	likes      []*storageModels.Like
	nextCursor string
	count      int64
	err        error
}

func (f *fakeLikeRepo) GetActorLikes(context.Context, string, int, string) ([]*storageModels.Like, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	return append([]*storageModels.Like{}, f.likes...), f.nextCursor, nil
}

func (f *fakeLikeRepo) CountActorLikes(context.Context, string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.count, nil
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

type fakeRemoteResolver struct {
	resolution *federation.ExactActorResolution
	err        error
	calls      int
}

func (f *fakeRemoteResolver) ResolveExactActor(context.Context, string, string) (*federation.ExactActorResolution, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.resolution, nil
}

type extendedMockDB struct {
	inner *dynamormMocks.MockDB
}

var _ dynamormCore.ExtendedDB = (*extendedMockDB)(nil)

func (db *extendedMockDB) Model(model any) dynamormCore.Query { return db.inner.Model(model) }

func (db *extendedMockDB) Migrate() error { return nil }

func (db *extendedMockDB) AutoMigrate(models ...any) error { return nil }

func (db *extendedMockDB) Close() error { return nil }

func (db *extendedMockDB) WithContext(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) AutoMigrateWithOptions(_ any, _ ...dynamormSchema.AutoMigrateOption) error {
	return nil
}

func (db *extendedMockDB) RegisterTypeConverter(_ reflect.Type, _ pkgtypes.CustomConverter) error {
	return nil
}

func (db *extendedMockDB) CreateTable(_ any, _ ...dynamormSchema.TableOption) error { return nil }

func (db *extendedMockDB) EnsureTable(_ any) error { return nil }

func (db *extendedMockDB) DeleteTable(_ any) error { return nil }

func (db *extendedMockDB) DescribeTable(_ any) (any, error) { return nil, nil }

func (db *extendedMockDB) WithLambdaTimeout(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) WithLambdaTimeoutBuffer(_ time.Duration) dynamormCore.DB { return db }

func (db *extendedMockDB) Transact() dynamormCore.TransactionBuilder { return nil }

func (db *extendedMockDB) TransactWrite(_ context.Context, fn func(dynamormCore.TransactionBuilder) error) error {
	return fn(nil)
}

func mustUnmarshalBody[T any](t *testing.T, resp *apptheory.Response) T {
	t.Helper()
	require.NotNil(t, resp)
	var out T
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	return out
}

func TestCollectionsHelpers_Round12(t *testing.T) {
	ch := &CollectionsHandler{}

	require.Equal(t, "", ch.generatePreviousCursor("", collectionTypeFollowers, nil, nil))
	require.Equal(t, "before_alice", ch.generatePreviousCursor("", collectionTypeFollowers, []string{"alice", "bob"}, nil))
	require.Equal(t, "before_1", ch.generatePreviousCursor("", collectionTypeLiked, nil, []*storage.Like{{ID: "1"}}))

	names := []string{"a", "b", "c"}
	ch.reverseStringSlice(names)
	require.Equal(t, []string{"c", "b", "a"}, names)

	likes := []*storage.Like{{ID: "1"}, {ID: "2"}}
	ch.reverseLikeSlice(likes)
	require.Equal(t, "2", likes[0].ID)

	require.Equal(t, "after_c", ch.generateNextCursorForReverse(collectionTypeFollowers, []string{"a", "b", "c"}, nil))
	require.Equal(t, "after_2", ch.generateNextCursorForReverse(collectionTypeLiked, nil, []*storage.Like{{ID: "1"}, {ID: "2"}}))
	require.Equal(t, "", ch.generateNextCursorForReverse(collectionTypeFollowers, nil, nil))
}

func TestHandleLikedCollection_LockedEmpty_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	handler := &CollectionsHandler{
		actorRepo:    &fakeActorRepo{actor: actor},
		instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: true}},
		likeRepo:     &fakeLikeRepo{},
	}

	resp, err := handler.handleLikedCollection(&apptheory.Context{
		Request: apptheory.Request{Method: "GET", Path: "/users/alice/liked"},
		Params:  map[string]string{"username": "alice"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{contentTypeActivityJSON}, resp.Headers["content-type"])
	require.Equal(t, []string{cacheControlMaxAge300}, resp.Headers["cache-control"])
	collection := mustUnmarshalBody[activitypub.OrderedCollection](t, resp)
	require.Equal(t, 0, collection.TotalItems)

	resp, err = handler.handleLikedCollection(&apptheory.Context{
		Request: apptheory.Request{
			Method: "GET",
			Path:   "/users/alice/liked",
			Query:  map[string][]string{"page": {"1"}},
		},
		Params: map[string]string{"username": "alice"},
	})
	require.NoError(t, err)
	page := mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	items, ok := page.OrderedItems.([]any)
	require.True(t, ok)
	require.Len(t, items, 0)
}

func TestReturnCollectionAndPages_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	rel := &fakeRelationshipRepo{countFollowers: 2}
	ch := &CollectionsHandler{relationshipRepo: rel, likeRepo: &fakeLikeRepo{}, actorRepo: &fakeActorRepo{actor: actor}}

	resp, err := ch.returnCollection(nil, actor, collectionTypeFollowers)
	require.NoError(t, err)
	collection := mustUnmarshalBody[activitypub.OrderedCollection](t, resp)
	require.Equal(t, 2, collection.TotalItems)
	require.NotEmpty(t, collection.First)
	require.Equal(t, []string{contentTypeActivityJSON}, resp.Headers["content-type"])

	resp, err = ch.returnCollectionPage(nil, actor, collectionTypeFollowers, []string{"bob"}, nil, "", "next", 20)
	require.NoError(t, err)
	page := mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	items, ok := page.OrderedItems.([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Contains(t, page.Next, "cursor=next")
	require.Equal(t, "https://example.com/users/bob", items[0])

	cfg.Domain = "https://example.com"
	resp, err = ch.returnCollectionPage(nil, actor, collectionTypeFollowers, []string{"bob"}, nil, "cur", "", 10)
	require.NoError(t, err)
	page = mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	require.Contains(t, page.Prev, "dir=prev")
	items, ok = page.OrderedItems.([]any)
	require.True(t, ok)
	require.Equal(t, "https://example.com/users/bob", items[0])

	resp, err = ch.returnCollectionPage(nil, actor, collectionTypeLiked, nil, []*storage.Like{{ID: "1", Object: "obj-1"}}, "", "", 10)
	require.NoError(t, err)
	page = mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	items, ok = page.OrderedItems.([]any)
	require.True(t, ok)
	require.Equal(t, "obj-1", items[0])

	cfg.Domain = "example.com"
	ch.actorRepo = &fakeActorRepo{
		actor: actor,
		cached: map[string]*activitypub.Actor{
			"bob@remote.example": {
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob"},
				PreferredUsername: "bob",
			},
		},
	}
	resp, err = ch.returnCollectionPage(nil, actor, collectionTypeFollowers, []string{"bob@remote.example", "carol"}, nil, "", "", 10)
	require.NoError(t, err)
	page = mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	items, ok = page.OrderedItems.([]any)
	require.True(t, ok)
	require.Equal(t, "https://remote.example/users/bob", items[0])
	require.Equal(t, "https://example.com/users/carol", items[1])

	resolver := &fakeRemoteResolver{
		resolution: &federation.ExactActorResolution{
			Actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/@erin"},
				PreferredUsername: "erin",
			},
		},
	}
	ch.actorRepo = &fakeActorRepo{actor: actor}
	ch.remoteResolver = resolver
	resp, err = ch.returnCollectionPage(nil, actor, collectionTypeFollowers, []string{"erin@remote.example"}, nil, "", "", 10)
	require.NoError(t, err)
	page = mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	items, ok = page.OrderedItems.([]any)
	require.True(t, ok)
	require.Equal(t, "https://remote.example/@erin", items[0])
	require.Equal(t, 1, resolver.calls)

	ch.remoteResolver = &fakeRemoteResolver{err: errors.New("lookup failed")}
	resp, err = ch.returnCollectionPage(nil, actor, collectionTypeFollowers, []string{"frank@remote.example"}, nil, "", "", 10)
	require.NoError(t, err)
	page = mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	items, ok = page.OrderedItems.([]any)
	require.True(t, ok)
	require.Equal(t, "https://remote.example/users/frank", items[0])
}

func TestHandleReverseDirection_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	rel := &fakeRelationshipRepo{
		followers:  []string{"a", "b"},
		nextCursor: "next",
	}
	ch := &CollectionsHandler{
		actorRepo:        &fakeActorRepo{actor: actor},
		relationshipRepo: rel,
		likeRepo:         &fakeLikeRepo{},
		instanceRepo:     &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
	}

	resp, err := ch.handleFollowersCollection(&apptheory.Context{
		Request: apptheory.Request{
			Method: "GET",
			Path:   "/users/alice/followers",
			Query: map[string][]string{
				"page":   {"1"},
				"dir":    {"prev"},
				"cursor": {"before_x"},
				"limit":  {"2"},
			},
		},
		Params: map[string]string{"username": "alice"},
	})
	require.NoError(t, err)
	page := mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	require.Contains(t, page.ID, "dir=prev")
	require.Equal(t, "x", rel.followersCursor)
}

func TestCollectionsMain_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	origLambdaCtx := lambdaCtx
	origStart := lambdaStartFn
	origNewHandler := newCollectionsHandlerFn
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
		lambdaCtx = origLambdaCtx
		lambdaStartFn = origStart
		newCollectionsHandlerFn = origNewHandler
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()
	lambdaCtx = &common.LambdaContext{StartTime: time.Now(), Logger: zap.NewNop()}

	newCollectionsHandlerFn = func() *CollectionsHandler {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
			PreferredUsername: "alice",
		}
		return &CollectionsHandler{
			actorRepo:        &fakeActorRepo{actor: actor},
			relationshipRepo: &fakeRelationshipRepo{followers: []string{"bob"}},
			likeRepo:         &fakeLikeRepo{},
			instanceRepo:     &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		}
	}

	called := false
	lambdaStartFn = func(handler any) {
		called = true
		fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.APIGatewayV2HTTPRequest{
			Version:               "2.0",
			RouteKey:              "GET /users/alice/followers",
			RawPath:               "/users/alice/followers",
			RawQueryString:        "page=1",
			QueryStringParameters: map[string]string{"page": "1"},
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				RequestID: "req",
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: "GET",
					Path:   "/users/alice/followers",
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
		require.Equal(t, contentTypeActivityJSON, lambdaResp.Headers["content-type"])
	}

	main()
	require.True(t, called)
}

func TestHandleCollection_MoreBranches_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	t.Run("actor not found", func(t *testing.T) {
		ch := &CollectionsHandler{
			actorRepo:    &fakeActorRepo{err: common.ActorNotFoundError{Username: "alice"}},
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		}
		resp, err := ch.handleFollowersCollection(&apptheory.Context{
			Request: apptheory.Request{Method: "GET", Path: "/users/alice/followers"},
			Params:  map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.Equal(t, 404, resp.Status)
	})

	t.Run("relationship page error", func(t *testing.T) {
		ch := &CollectionsHandler{
			actorRepo:        &fakeActorRepo{actor: actor},
			relationshipRepo: &fakeRelationshipRepo{err: errors.New("boom")},
			instanceRepo:     &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			likeRepo:         &fakeLikeRepo{},
		}
		resp, err := ch.handleFollowersCollection(&apptheory.Context{
			Request: apptheory.Request{
				Method: "GET",
				Path:   "/users/alice/followers",
				Query:  map[string][]string{"page": {"1"}},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.Equal(t, 500, resp.Status)
	})

	t.Run("liked page converts model likes", func(t *testing.T) {
		ch := &CollectionsHandler{
			actorRepo:    &fakeActorRepo{actor: actor},
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			likeRepo: &fakeLikeRepo{likes: []*storageModels.Like{{
				ID:     "l1",
				Actor:  actor.ID,
				Object: "obj-1",
			}}},
			relationshipRepo: &fakeRelationshipRepo{},
		}
		resp, err := ch.handleLikedCollection(&apptheory.Context{
			Request: apptheory.Request{
				Method: "GET",
				Path:   "/users/alice/liked",
				Query:  map[string][]string{"page": {"1"}},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		page := mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
		items, ok := page.OrderedItems.([]any)
		require.True(t, ok)
		require.Equal(t, "obj-1", items[0])
	})

	t.Run("following collection handler", func(t *testing.T) {
		ch := &CollectionsHandler{
			actorRepo:        &fakeActorRepo{actor: actor},
			relationshipRepo: &fakeRelationshipRepo{following: []string{"bob"}},
			instanceRepo:     &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			likeRepo:         &fakeLikeRepo{},
		}
		resp, err := ch.handleFollowingCollection(&apptheory.Context{
			Request: apptheory.Request{
				Method: "GET",
				Path:   "/users/alice/following",
				Query:  map[string][]string{"page": {"1"}},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.Equal(t, 200, resp.Status)
	})

	t.Run("collection metadata when page not requested", func(t *testing.T) {
		rel := &fakeRelationshipRepo{countFollowers: 1}
		ch := &CollectionsHandler{
			actorRepo:        &fakeActorRepo{actor: actor},
			relationshipRepo: rel,
			instanceRepo:     &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			likeRepo:         &fakeLikeRepo{},
		}
		resp, err := ch.handleFollowersCollection(&apptheory.Context{
			Request: apptheory.Request{Method: "GET", Path: "/users/alice/followers"},
			Params:  map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		collection := mustUnmarshalBody[activitypub.OrderedCollection](t, resp)
		require.Equal(t, 1, collection.TotalItems)
	})
}

func TestInitializeCollectionsAndNewHandler_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	origLambdaCtx := lambdaCtx
	origRepos := repos
	origMust := mustInitializeLambdaFn
	origDefaults := initializeWithDefaultsFn
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
		lambdaCtx = origLambdaCtx
		repos = origRepos
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origDefaults
	})

	cfg = &config.Config{Domain: "example.com", DynamoTableName: "test-table"}
	logger = zap.NewNop()

	innerDB := new(dynamormMocks.MockDB)
	db := &extendedMockDB{inner: innerDB}
	repoFactory, err := factory.NewRepositoryFactory(db, "test-table", zap.NewNop())
	require.NoError(t, err)
	repos = repoFactory

	collectionsHandler := NewCollectionsHandler()
	require.NotNil(t, collectionsHandler.actorRepo)
	require.NotNil(t, collectionsHandler.relationshipRepo)
	require.NotNil(t, collectionsHandler.likeRepo)
	require.NotNil(t, collectionsHandler.instanceRepo)

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:    cfg,
			Logger:    nil,
			Repos:     repos,
			StartTime: time.Now(),
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
	initializeCollections()
	require.NotNil(t, lambdaCtx)
}

func TestReturnCollection_MoreBranches_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	ch := &CollectionsHandler{
		relationshipRepo: &fakeRelationshipRepo{countFollowing: 1},
		likeRepo:         &fakeLikeRepo{count: 2},
	}

	resp, err := ch.returnCollection(nil, actor, collectionTypeFollowing)
	require.NoError(t, err)
	collection := mustUnmarshalBody[activitypub.OrderedCollection](t, resp)
	require.Equal(t, 1, collection.TotalItems)

	resp, err = ch.returnCollection(nil, actor, collectionTypeLiked)
	require.NoError(t, err)
	collection = mustUnmarshalBody[activitypub.OrderedCollection](t, resp)
	require.Equal(t, 2, collection.TotalItems)

	ch.relationshipRepo = &fakeRelationshipRepo{countFollowers: 0}
	resp, err = ch.returnCollection(nil, actor, collectionTypeFollowers)
	require.NoError(t, err)
	collection = mustUnmarshalBody[activitypub.OrderedCollection](t, resp)
	require.Empty(t, collection.First)

	ch.relationshipRepo = &fakeRelationshipRepo{err: errors.New("boom")}
	resp, err = ch.returnCollection(nil, actor, collectionTypeFollowers)
	require.NoError(t, err)
	require.Equal(t, 500, resp.Status)

	ch.relationshipRepo = &fakeRelationshipRepo{err: errors.New("boom")}
	resp, err = ch.returnCollection(nil, actor, collectionTypeFollowing)
	require.NoError(t, err)
	require.Equal(t, 500, resp.Status)

	ch.relationshipRepo = &fakeRelationshipRepo{countFollowers: 1}
	ch.likeRepo = &fakeLikeRepo{err: errors.New("boom")}
	resp, err = ch.returnCollection(nil, actor, collectionTypeLiked)
	require.NoError(t, err)
	require.Equal(t, 500, resp.Status)
}

func TestHandleReverseDirection_MoreBranches_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	t.Run("following reverse", func(t *testing.T) {
		rel := &fakeRelationshipRepo{following: []string{"a", "b"}, nextCursor: "next"}
		ch := &CollectionsHandler{
			actorRepo:        &fakeActorRepo{actor: actor},
			relationshipRepo: rel,
			likeRepo:         &fakeLikeRepo{},
			instanceRepo:     &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		}

		resp, err := ch.handleFollowingCollection(&apptheory.Context{
			Request: apptheory.Request{
				Method: "GET",
				Path:   "/users/alice/following",
				Query: map[string][]string{
					"page":   {"1"},
					"dir":    {"prev"},
					"cursor": {"before_y"},
				},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		page := mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
		require.Contains(t, page.ID, "dir=prev")
		require.Equal(t, "y", rel.followingCursor)
	})

	t.Run("liked reverse", func(t *testing.T) {
		ch := &CollectionsHandler{
			actorRepo:    &fakeActorRepo{actor: actor},
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			likeRepo: &fakeLikeRepo{likes: []*storageModels.Like{
				{ID: "l1", Actor: actor.ID, Object: "o1"},
				{ID: "l2", Actor: actor.ID, Object: "o2"},
			}},
			relationshipRepo: &fakeRelationshipRepo{},
		}

		resp, err := ch.handleLikedCollection(&apptheory.Context{
			Request: apptheory.Request{
				Method: "GET",
				Path:   "/users/alice/liked",
				Query: map[string][]string{
					"page":   {"1"},
					"dir":    {"prev"},
					"cursor": {"before_l1"},
				},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		page := mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
		items, ok := page.OrderedItems.([]any)
		require.True(t, ok)
		require.Equal(t, []any{"o2", "o1"}, items)
	})
}

func TestReturnCollectionPageReverse_Links_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	ch := &CollectionsHandler{}
	resp, err := ch.returnCollectionPageReverse(nil, actor, collectionTypeFollowers, []string{"bob"}, nil, "before_bob", "prev", "next", 10)
	require.NoError(t, err)
	page := mustUnmarshalBody[activitypub.OrderedCollectionPage](t, resp)
	require.NotEmpty(t, page.Next)
	require.NotEmpty(t, page.Prev)
}

func TestCollectionsCoverageMarginHelpers_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	logger = zap.NewNop()

	cfg = nil
	require.Equal(t, "", collectionLocalDomain())
	require.Equal(t, "alice", (&CollectionsHandler{}).localCollectionActorID("alice"))

	cfg = &config.Config{Domain: "https://example.com:8443/path"}
	require.Equal(t, "example.com", collectionLocalDomain())

	username, domain, ok := parseCollectionHandle("@alice@remote.example ")
	require.True(t, ok)
	require.Equal(t, "alice", username)
	require.Equal(t, "remote.example", domain)

	_, _, ok = parseCollectionHandle("alice")
	require.False(t, ok)
	_, _, ok = parseCollectionHandle("@alice@")
	require.False(t, ok)
	_, _, ok = parseCollectionHandle("@@remote.example")
	require.False(t, ok)

	ch := &CollectionsHandler{}
	cfg = &config.Config{Domain: "example.com"}
	require.Equal(t, "https://example.com/users/alice", ch.localCollectionActorID("alice"))
	require.Equal(t, "not-a-handle", ch.remoteCollectionActorURL("not-a-handle"))
	require.Equal(t, "https://example.com/users/alice", ch.remoteCollectionActorURL("alice@example.com"))
	require.Equal(t, "https://remote.example/users/bob", ch.remoteCollectionActorURL("@bob@remote.example"))

	require.Equal(t, "", collectionsActorURL(nil))
	require.Equal(t, "https://example.com/users/alice", collectionsActorURL(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"},
		URL:        "https://example.com/@alice",
	}))
	require.Equal(t, "https://example.com/@alice", collectionsActorURL(&activitypub.Actor{
		URL: "https://example.com/@alice",
	}))

	queryCtx := &apptheory.Context{Request: apptheory.Request{Query: map[string][]string{"limit": {"5"}}}}
	require.Equal(t, "", collectionsQueryValue(nil, "limit"))
	require.Equal(t, "", collectionsQueryValue(queryCtx, " "))
	require.Equal(t, "", collectionsQueryValue(queryCtx, "missing"))
	require.Equal(t, "5", collectionsQueryValue(queryCtx, " limit "))

	require.Equal(t, "", collectionsContextRequestID(nil))
	require.Equal(t, " explicit ", collectionsRequestID(&apptheory.Context{RequestID: " explicit "}, "ignored"))
	generated := collectionsRequestID(&apptheory.Context{}, " ")
	require.Contains(t, generated, "collections-")
	ctxWithStoredID := &apptheory.Context{}
	ctxWithStoredID.Set("requestID", " stored ")
	require.Equal(t, "stored", collectionsContextRequestID(ctxWithStoredID))
	require.Equal(t, "request-id", collectionsContextRequestID(&apptheory.Context{RequestID: "request-id"}))

	resp := collectionsJSONError(http.StatusBadRequest, " ")
	require.Equal(t, http.StatusBadRequest, resp.Status)
	activityResp, err := collectionsActivityJSON(http.StatusAccepted, map[string]string{"ok": "yes"})
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, activityResp.Status)
	require.Equal(t, []string{contentTypeActivityJSON}, activityResp.Headers["content-type"])
}

func TestCollectionsMiddlewareCoverageMargin_Round12(t *testing.T) {
	recovered, err := collectionsPanicRecovery(nil)(func(*apptheory.Context) (*apptheory.Response, error) {
		panic("boom")
	})(&apptheory.Context{})
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, recovered.Status)

	okResp, err := collectionsPanicRecovery(zap.NewNop())(func(*apptheory.Context) (*apptheory.Response, error) {
		return collectionsJSONError(http.StatusTeapot, "short and stout"), nil
	})(&apptheory.Context{})
	require.NoError(t, err)
	require.Equal(t, http.StatusTeapot, okResp.Status)

	nilResp, err := collectionsActivityPubSecurityHeaders()(func(*apptheory.Context) (*apptheory.Response, error) {
		return nil, nil
	})(&apptheory.Context{})
	require.NoError(t, err)
	require.Nil(t, nilResp)

	secured, err := collectionsActivityPubSecurityHeaders()(func(*apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{Status: http.StatusOK}, nil
	})(&apptheory.Context{})
	require.NoError(t, err)
	require.Equal(t, []string{"nosniff"}, secured.Headers["x-content-type-options"])
	require.Equal(t, []string{"cross-origin"}, secured.Headers["cross-origin-resource-policy"])
}
