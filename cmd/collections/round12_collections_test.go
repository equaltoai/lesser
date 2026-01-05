package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
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

type fakeActorRepo struct {
	actor *activitypub.Actor
	err   error
}

func (f *fakeActorRepo) GetActor(context.Context, string) (*activitypub.Actor, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.actor, nil
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

type extendedMockDB struct {
	inner *dynamormMocks.MockDB
}

var _ dynamormCore.ExtendedDB = (*extendedMockDB)(nil)

func (db *extendedMockDB) Model(model any) dynamormCore.Query { return db.inner.Model(model) }

func (db *extendedMockDB) Transaction(fn func(tx *dynamormCore.Tx) error) error { return fn(nil) }

func (db *extendedMockDB) Migrate() error { return nil }

func (db *extendedMockDB) AutoMigrate(models ...any) error { return nil }

func (db *extendedMockDB) Close() error { return nil }

func (db *extendedMockDB) WithContext(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) AutoMigrateWithOptions(_ any, _ ...any) error { return nil }

func (db *extendedMockDB) RegisterTypeConverter(_ reflect.Type, _ pkgtypes.CustomConverter) error { return nil }

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
		BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	handler := &CollectionsHandler{
		actorRepo:    &fakeActorRepo{actor: actor},
		instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: true}},
		likeRepo:     &fakeLikeRepo{},
	}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/users/alice/liked"}))
	ctx.SetParam("username", "alice")
	require.NoError(t, handler.handleLikedCollection(ctx))
	require.Equal(t, contentTypeActivityJSON, ctx.Response.Headers["Content-Type"])
	require.Equal(t, cacheControlMaxAge300, ctx.Response.Headers["Cache-Control"])
	collection, ok := ctx.Response.Body.(*activitypub.OrderedCollection)
	require.True(t, ok)
	require.Equal(t, 0, collection.TotalItems)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      "GET",
		Path:        "/users/alice/liked",
		QueryParams: map[string]string{"page": "1"},
	}))
	ctx.SetParam("username", "alice")
	require.NoError(t, handler.handleLikedCollection(ctx))
	page, ok := ctx.Response.Body.(*activitypub.OrderedCollectionPage)
	require.True(t, ok)
	require.Len(t, page.OrderedItems, 0)
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
		BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	rel := &fakeRelationshipRepo{countFollowers: 2}
	ch := &CollectionsHandler{relationshipRepo: rel, likeRepo: &fakeLikeRepo{}, actorRepo: &fakeActorRepo{actor: actor}}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	require.NoError(t, ch.returnCollection(ctx, actor, collectionTypeFollowers))
	collection, ok := ctx.Response.Body.(*activitypub.OrderedCollection)
	require.True(t, ok)
	require.Equal(t, 2, collection.TotalItems)
	require.NotEmpty(t, collection.First)
	require.Equal(t, contentTypeActivityJSON, ctx.Response.Headers["Content-Type"])

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	require.NoError(t, ch.returnCollectionPage(ctx, actor, collectionTypeFollowers, []string{"bob"}, nil, "", "next", 20))
	page, ok := ctx.Response.Body.(*activitypub.OrderedCollectionPage)
	require.True(t, ok)
	items, ok := page.OrderedItems.([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Contains(t, page.Next, "cursor=next")
	require.Equal(t, "https://example.com/users/bob", items[0])

	cfg.Domain = "https://example.com"
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	require.NoError(t, ch.returnCollectionPage(ctx, actor, collectionTypeFollowers, []string{"bob"}, nil, "cur", "", 10))
	page, ok = ctx.Response.Body.(*activitypub.OrderedCollectionPage)
	require.True(t, ok)
	require.Contains(t, page.Prev, "dir=prev")
	items, ok = page.OrderedItems.([]any)
	require.True(t, ok)
	require.Equal(t, "https://example.com/users/bob", items[0])

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	require.NoError(t, ch.returnCollectionPage(ctx, actor, collectionTypeLiked, nil, []*storage.Like{{ID: "1", Object: "obj-1"}}, "", "", 10))
	page, ok = ctx.Response.Body.(*activitypub.OrderedCollectionPage)
	require.True(t, ok)
	items, ok = page.OrderedItems.([]any)
	require.True(t, ok)
	require.Equal(t, "obj-1", items[0])
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
		BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
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

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      "GET",
		Path:        "/users/alice/followers",
		QueryParams: map[string]string{"page": "1", "dir": "prev", "cursor": "before_x", "limit": "2"},
	}))
	ctx.SetParam("username", "alice")
	require.NoError(t, ch.handleFollowersCollection(ctx))
	page, ok := ctx.Response.Body.(*activitypub.OrderedCollectionPage)
	require.True(t, ok)
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
			BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
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
		fn, ok := handler.(func(context.Context, interface{}) (interface{}, error))
		require.True(t, ok)

		event := map[string]any{
			"version":         "2.0",
			"routeKey":        "GET /users/alice/followers",
			"rawPath":         "/users/alice/followers",
			"rawQueryString":  "page=1",
			"queryStringParameters": map[string]any{
				"page": "1",
			},
			"requestContext": map[string]any{
				"requestId": "req",
				"http": map[string]any{
					"method": "GET",
					"path":   "/users/alice/followers",
				},
			},
		}

		resp, err := fn(context.Background(), event)
		require.NoError(t, err)
		liftResp, ok := resp.(*lift.Response)
		require.True(t, ok)
		require.Equal(t, 200, liftResp.StatusCode)
		require.Equal(t, contentTypeActivityJSON, liftResp.Headers["Content-Type"])
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
		BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	t.Run("actor not found", func(t *testing.T) {
		ch := &CollectionsHandler{
			actorRepo:    &fakeActorRepo{err: common.ActorNotFoundError{Username: "alice"}},
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/users/alice/followers"}))
		ctx.SetParam("username", "alice")
		err := ch.handleFollowersCollection(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 404, liftErr.StatusCode)
	})

	t.Run("relationship page error", func(t *testing.T) {
		ch := &CollectionsHandler{
			actorRepo:        &fakeActorRepo{actor: actor},
			relationshipRepo: &fakeRelationshipRepo{err: errors.New("boom")},
			instanceRepo:     &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			likeRepo:         &fakeLikeRepo{},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:      "GET",
			Path:        "/users/alice/followers",
			QueryParams: map[string]string{"page": "1"},
		}))
		ctx.SetParam("username", "alice")
		err := ch.handleFollowersCollection(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 500, liftErr.StatusCode)
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
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:      "GET",
			Path:        "/users/alice/liked",
			QueryParams: map[string]string{"page": "1"},
		}))
		ctx.SetParam("username", "alice")
		require.NoError(t, ch.handleLikedCollection(ctx))
		page, ok := ctx.Response.Body.(*activitypub.OrderedCollectionPage)
		require.True(t, ok)
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
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:      "GET",
			Path:        "/users/alice/following",
			QueryParams: map[string]string{"page": "1"},
		}))
		ctx.SetParam("username", "alice")
		require.NoError(t, ch.handleFollowingCollection(ctx))
	})

	t.Run("collection metadata when page not requested", func(t *testing.T) {
		rel := &fakeRelationshipRepo{countFollowers: 1}
		ch := &CollectionsHandler{
			actorRepo:        &fakeActorRepo{actor: actor},
			relationshipRepo: rel,
			instanceRepo:     &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			likeRepo:         &fakeLikeRepo{},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/users/alice/followers"}))
		ctx.SetParam("username", "alice")
		require.NoError(t, ch.handleFollowersCollection(ctx))
		collection, ok := ctx.Response.Body.(*activitypub.OrderedCollection)
		require.True(t, ok)
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
			Config:   cfg,
			Logger:   nil,
			Repos:    repos,
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
		BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	ch := &CollectionsHandler{
		relationshipRepo: &fakeRelationshipRepo{countFollowing: 1},
		likeRepo:         &fakeLikeRepo{count: 2},
	}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	require.NoError(t, ch.returnCollection(ctx, actor, collectionTypeFollowing))
	collection, ok := ctx.Response.Body.(*activitypub.OrderedCollection)
	require.True(t, ok)
	require.Equal(t, 1, collection.TotalItems)

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	require.NoError(t, ch.returnCollection(ctx, actor, collectionTypeLiked))
	collection, ok = ctx.Response.Body.(*activitypub.OrderedCollection)
	require.True(t, ok)
	require.Equal(t, 2, collection.TotalItems)

	ch.relationshipRepo = &fakeRelationshipRepo{countFollowers: 0}
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	require.NoError(t, ch.returnCollection(ctx, actor, collectionTypeFollowers))
	collection, ok = ctx.Response.Body.(*activitypub.OrderedCollection)
	require.True(t, ok)
	require.Empty(t, collection.First)

	ch.relationshipRepo = &fakeRelationshipRepo{err: errors.New("boom")}
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	err := ch.returnCollection(ctx, actor, collectionTypeFollowers)
	require.Error(t, err)

	ch.relationshipRepo = &fakeRelationshipRepo{err: errors.New("boom")}
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	err = ch.returnCollection(ctx, actor, collectionTypeFollowing)
	require.Error(t, err)

	ch.relationshipRepo = &fakeRelationshipRepo{countFollowers: 1}
	ch.likeRepo = &fakeLikeRepo{err: errors.New("boom")}
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	err = ch.returnCollection(ctx, actor, collectionTypeLiked)
	require.Error(t, err)
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
		BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
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

		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:      "GET",
			Path:        "/users/alice/following",
			QueryParams: map[string]string{"page": "1", "dir": "prev", "cursor": "before_y"},
		}))
		ctx.SetParam("username", "alice")
		require.NoError(t, ch.handleFollowingCollection(ctx))
		page, ok := ctx.Response.Body.(*activitypub.OrderedCollectionPage)
		require.True(t, ok)
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

		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:      "GET",
			Path:        "/users/alice/liked",
			QueryParams: map[string]string{"page": "1", "dir": "prev", "cursor": "before_l1"},
		}))
		ctx.SetParam("username", "alice")
		require.NoError(t, ch.handleLikedCollection(ctx))
		page, ok := ctx.Response.Body.(*activitypub.OrderedCollectionPage)
		require.True(t, ok)
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
		BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
		PreferredUsername: "alice",
	}

	ch := &CollectionsHandler{}
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: "GET", Path: "/"}))
	require.NoError(t, ch.returnCollectionPageReverse(ctx, actor, collectionTypeFollowers, []string{"bob"}, nil, "before_bob", "prev", "next", 10))
	page, ok := ctx.Response.Body.(*activitypub.OrderedCollectionPage)
	require.True(t, ok)
	require.NotEmpty(t, page.Next)
	require.NotEmpty(t, page.Prev)
}
