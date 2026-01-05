package dynamorm

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func Test_convertInterfaceToSlice(t *testing.T) {
	assert.Equal(t, []interface{}{}, convertInterfaceToSlice(nil))
	assert.Equal(t, []interface{}{}, convertInterfaceToSlice("not-a-slice"))

	out := convertInterfaceToSlice([]int{1, 2})
	if assert.Len(t, out, 2) {
		assert.Equal(t, 1, out[0])
		assert.Equal(t, 2, out[1])
	}
}

type dummyRepo struct{}

func (dummyRepo) GetUserTimeline(_ context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if username == "err" {
		return nil, cursor, errors.New("boom")
	}
	return []string{"a", "b"}, "next", nil
}

func Test_callRepositoryMethod(t *testing.T) {
	ctx := context.Background()

	items, next, handled, err := callRepositoryMethod(ctx, dummyRepo{}, "GetUserTimeline", "alice", 10, "cur")
	assert.True(t, handled)
	assert.NoError(t, err)
	assert.Equal(t, "next", next)
	assert.Equal(t, []string{"a", "b"}, items)

	_, _, handled, err = callRepositoryMethod(ctx, dummyRepo{}, "Missing", "alice", 10, "cur")
	assert.False(t, handled)
	assert.NoError(t, err)

	items, next, handled, err = callRepositoryMethod(ctx, dummyRepo{}, "GetUserTimeline", "err", 10, "cur")
	assert.True(t, handled)
	assert.Error(t, err)
	assert.Equal(t, "cur", next)
	assert.Nil(t, items)
}

func Test_createReflectionBasedMethodCallsMap(t *testing.T) {
	ctx := context.Background()
	methodMap := createReflectionBasedMethodCallsMap(ctx, "alice", 10, "cur", []RepositoryMethodCall{
		{MethodName: "GetTimeline", RepositoryMethod: "GetUserTimeline"},
	})

	fn, ok := methodMap["GetTimeline"]
	assert.True(t, ok)

	items, next, err, handled := fn(dummyRepo{})
	assert.True(t, handled)
	assert.NoError(t, err)
	assert.Equal(t, "next", next)
	assert.Equal(t, []interface{}{"a", "b"}, items)
}

func Test_executeRepositoryMethodWithFallback(t *testing.T) {
	primaryCalls := 0
	fallbackCalls := 0

	primary := func(_ interface{}) ([]interface{}, string, error, bool) {
		primaryCalls++
		return []interface{}{"p"}, "pc", nil, true
	}
	fallback := func(_ interface{}) ([]interface{}, string, error, bool) {
		fallbackCalls++
		return []interface{}{"f"}, "fc", nil, true
	}

	items, cursor, err := executeRepositoryMethodWithFallback(struct{}{}, struct{}{}, primary, fallback)
	assert.NoError(t, err)
	assert.Equal(t, []interface{}{"p"}, items)
	assert.Equal(t, "pc", cursor)
	assert.Equal(t, 1, primaryCalls)
	assert.Equal(t, 0, fallbackCalls)
}

type timelinePrimaryRepo struct{}

func (timelinePrimaryRepo) GetTimeline(_ context.Context, _ string, _ int, _ string) ([]interface{}, string, error) {
	return []interface{}{"x"}, "c", nil
}

func Test_executeGetMethodWithTypedFallback_PrimaryInterface(t *testing.T) {
	items, cursor, err := executeGetMethodWithTypedFallback[any](context.Background(), timelinePrimaryRepo{}, "GetTimeline", "alice", 10, "cur", nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "c", cursor)
	assert.Equal(t, []interface{}{"x"}, items)
}

func Test_executeGetMethodWithTypedFallback_ReflectionFallback(t *testing.T) {
	items, cursor, err := executeGetMethodWithTypedFallback[any](context.Background(), dummyRepo{}, "GetTimeline", "alice", 10, "cur", nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "next", cursor)
	assert.Equal(t, []interface{}{"a", "b"}, items)
}

type searchFallbackRepo struct{}

func (searchFallbackRepo) SearchAccounts(_ context.Context, query string, limit int, cursor string) ([]string, string, error) {
	_ = limit
	_ = cursor
	return []string{query}, "next", nil
}

func Test_executeSearchMethodWithTypedFallback_ReflectionFallback(t *testing.T) {
	items, cursor, err := executeSearchMethodWithTypedFallback[any](context.Background(), nil, searchFallbackRepo{}, "SearchUsers", "q", 10, "cur")
	assert.NoError(t, err)
	assert.Equal(t, "next", cursor)
	assert.Equal(t, []interface{}{"q"}, items)
}

func TestStorageAdapter_ErrorPaths_WithNilRepos(t *testing.T) {
	repoStorage := &SimpleRepositoryStorage{
		db:        nil,
		tableName: "test-table",
		logger:    zap.NewNop(),
	}
	adapter := NewStorageAdapter(repoStorage)
	ctx := context.Background()

	// Actor paths
	assert.Error(t, adapter.CreateActor(ctx, &activitypub.Actor{}, ""))
	_, err := adapter.GetActor(ctx, "alice")
	assert.Error(t, err)
	assert.Error(t, adapter.UpdateActor(ctx, &activitypub.Actor{}))
	assert.Error(t, adapter.DeleteActor(ctx, "alice"))
	_, err = adapter.GetActorByID(ctx, "actor-id")
	assert.Error(t, err)
	_, err = adapter.GetActorPrivateKey(ctx, "alice")
	assert.Error(t, err)
	assert.Error(t, adapter.UpdateActorKeys(ctx, "alice", "pub", "priv"))

	// User paths
	assert.Error(t, adapter.CreateUser(ctx, "bad-type"))
	assert.Error(t, adapter.UpdateUser(ctx, "bad-type"))

	err = adapter.CreateUser(ctx, &storage.User{Username: "alice"})
	assert.Error(t, err)

	_, err = adapter.GetUser(ctx, "alice")
	assert.Error(t, err)

	err = adapter.UpdateUser(ctx, &storage.User{Username: "alice"})
	assert.Error(t, err)

	err = adapter.DeleteUser(ctx, "alice")
	assert.Error(t, err)
}
