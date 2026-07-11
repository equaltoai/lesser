package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type testEBRModel struct {
	PK string
	SK string
	ID string
}

func (m *testEBRModel) UpdateKeys() error {
	m.PK = "PK#" + m.ID
	m.SK = "SK#" + m.ID
	return nil
}

func (m *testEBRModel) GetPK() string { return m.PK }
func (m *testEBRModel) GetSK() string { return m.SK }

type testEBRValidator struct {
	validateModelErr      error
	requiredFieldsErr     error
	businessRulesErr      error
	requiredFieldsCalls   int
	businessRulesCalls    int
	validateModelCalls    int
	validateBusinessCalls int
}

func (v *testEBRValidator) ValidateModel(context.Context, BaseModel) error {
	v.validateModelCalls++
	return v.validateModelErr
}

func (v *testEBRValidator) ValidateBusinessRules(context.Context, BaseModel, string) error {
	v.validateBusinessCalls++
	return v.businessRulesErr
}

func (v *testEBRValidator) ValidateRequiredFields(context.Context, BaseModel) error {
	v.requiredFieldsCalls++
	return v.requiredFieldsErr
}

type testEBRPermissions struct {
	calls int
	err   error
}

func (p *testEBRPermissions) CheckPermissions(context.Context, string, string, BaseModel) error {
	p.calls++
	return p.err
}

func (p *testEBRPermissions) HasPermission(context.Context, string, string) bool { return false }

type testEBREvents struct {
	seen []Event
	err  error
}

func (e *testEBREvents) Emit(_ context.Context, event Event) error {
	e.seen = append(e.seen, event)
	return e.err
}

type testEBRCache struct {
	getErr    error
	getCalls  int
	setCalls  int
	delCalls  int
	lastKey   string
	lastValue interface{}
}

func (c *testEBRCache) Get(_ context.Context, key string, dest interface{}) error {
	c.getCalls++
	c.lastKey = key
	if c.getErr != nil {
		return c.getErr
	}
	if m, ok := dest.(*testEBRModel); ok {
		*m = testEBRModel{ID: "cached", PK: "pk", SK: "sk"}
		return nil
	}
	return fmt.Errorf("unexpected dest type")
}

func (c *testEBRCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	c.setCalls++
	c.lastKey = key
	c.lastValue = value
	return nil
}

func (c *testEBRCache) Delete(_ context.Context, key string) error {
	c.delCalls++
	c.lastKey = key
	return nil
}

func (c *testEBRCache) InvalidatePattern(context.Context, string) error { return nil }

func TestEnhancedBaseRepository_round09_core_paths(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("validate_and_create_success_and_validation_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", logger, nil, "Repo", "entity")
		repo.SetValidationService(&testEBRValidator{requiredFieldsErr: fmt.Errorf("missing")})
		repo.SetPermissionService(nil)

		err := repo.ValidateAndCreate(ctx, &testEBRModel{ID: "1"})
		assert.Error(t, err)

		validator := &testEBRValidator{}
		repo.SetValidationService(validator)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil).Once()

		ev := &testEBREvents{}
		repo.SetEventService(ev)

		ctxWithActor := context.WithValue(ctx, common.ContextKeyClaims, testClaims{username: "u1"})
		require.NoError(t, repo.ValidateAndCreate(ctxWithActor, &testEBRModel{ID: "2"}))
		assert.Equal(t, 1, validator.requiredFieldsCalls)
		assert.Equal(t, 1, validator.validateBusinessCalls)
		assert.Len(t, ev.seen, 1)
	})

	t.Run("validate_and_update_cache_invalidation_and_permissions", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", logger, nil, "Repo", "entity")
		repo.SetValidationService(&testEBRValidator{validateModelErr: fmt.Errorf("bad model")})

		err := repo.ValidateAndUpdate(ctx, &testEBRModel{ID: "1"})
		assert.Error(t, err)

		validator := &testEBRValidator{}
		perms := &testEBRPermissions{err: fmt.Errorf("nope")}
		repo.SetValidationService(validator)
		repo.SetPermissionService(perms)

		ctxWithActor := context.WithValue(ctx, common.ContextKeyClaims, testClaims{username: "u1"})
		err = repo.ValidateAndUpdate(ctxWithActor, &testEBRModel{ID: "2"})
		assert.Error(t, err)
		assert.Equal(t, 1, perms.calls)

		// success path: update + cache invalidate
		cache := NewInMemoryCachingService()
		repo.SetCachingService(cache)
		repo.SetPermissionService(nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		err = repo.ValidateAndUpdate(ctxWithActor, &testEBRModel{ID: "3"})
		require.NoError(t, err)
	})

	t.Run("validate_and_delete_not_found_skip_permission_then_delete", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", logger, nil, "Repo", "entity")
		repo.SetPermissionService(&testEBRPermissions{})

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "sk").Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()

		mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "sk").Return(mockQuery)
		mockQuery.On("Delete").Return(nil).Once()

		err := repo.ValidateAndDelete(ctx, "pk", "sk")
		require.NoError(t, err)
	})

	t.Run("find_with_cache_get_with_cache_and_count_where", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", logger, nil, "Repo", "entity")
		cache := NewInMemoryCachingService()
		repo.SetCachingService(cache)

		// Seed cache for FindWithCache
		cacheKey := repo.generateCacheKey("find", "PK#c", "")
		require.NoError(t, cache.Set(ctx, cacheKey, []*testEBRModel{{PK: "PK#c", SK: "SK#c", ID: "c"}}, time.Hour))

		out, err := repo.FindWithCache(ctx, "PK#c", time.Hour)
		require.NoError(t, err)
		assert.Len(t, out, 1)

		// cache miss: FindByPK hits DB
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "PK#db").Return(mockQuery)
		mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
		mockQuery.On("Limit", 100).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*testEBRModel)
			*dest = []*testEBRModel{{PK: "PK#db", SK: "SK#1", ID: "1"}}
		}).Return(nil).Once()

		out, err = repo.FindWithCache(ctx, "PK#db", 0)
		require.NoError(t, err)
		assert.Len(t, out, 1)

		// GetWithCache: cache miss then populate
		var result testEBRModel
		mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "sk").Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*testEBRModel)
			*dest = testEBRModel{PK: "pk", SK: "sk", ID: "x"}
		}).Return(nil).Once()

		require.NoError(t, repo.GetWithCache(ctx, "pk", "sk", &result, 0))
		assert.Equal(t, "x", result.ID)

		// CountWhere: valid PK and missing PK
		mockQuery.On("Where", "PK", "=", "PK#count").Return(mockQuery)
		mockQuery.On("Count").Return(int64(3), nil).Once()
		n, err := repo.CountWhere(ctx, map[string]interface{}{"PK": "PK#count"})
		require.NoError(t, err)
		assert.EqualValues(t, 3, n)

		_, err = repo.CountWhere(ctx, map[string]interface{}{"other": "x"})
		assert.Error(t, err)
	})
}

func TestEnhancedBaseRepository_round09_batch_and_flags(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", zap.NewNop(), nil, "Repo", "entity")
	ev := &testEBREvents{}
	repo.SetEventService(ev)
	repo.SetCachingService(NewInMemoryCachingService())

	items := []*testEBRModel{{ID: "1"}, {ID: "2"}}

	mockQuery.On("Create").Return(nil).Twice()
	require.NoError(t, repo.ValidateAndBatchCreate(ctx, items))
	assert.GreaterOrEqual(t, len(ev.seen), 2)

	assert.Equal(t, "entity", repo.GetEntityName())
	assert.False(t, repo.HasPermissions())
	assert.True(t, repo.HasCaching())
	assert.True(t, repo.HasEvents())
}

func TestEnhancedBaseRepository_round09_event_helpers_and_more_branches(t *testing.T) {
	ctx := context.Background()

	m := &testEBRModel{ID: "1"}
	_ = m.UpdateKeys()
	created := NewCreatedEvent(m)
	updated := NewUpdatedEvent(m)
	deleted := NewDeletedEvent("pk", "sk")
	assert.Equal(t, "entity.created", created.Type)
	assert.Equal(t, "entity.updated", updated.Type)
	assert.Equal(t, "entity.deleted", deleted.Type)

	// FindByMultipleFields executes through QueryWithFilter; keep it simple with mocks.
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
	mockQuery.On("Limit", 51).Return(mockQuery).Once()
	mockQuery.On("Filter", "Field", "=", "v").Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", zap.NewNop(), nil, "Repo", "entity")
	_, err := repo.FindByMultipleFields(ctx, map[string]interface{}{"Field": "v"})
	require.NoError(t, err)
}

func TestEnhancedBaseRepository_round09_delete_permission_paths(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", zap.NewNop(), nil, "Repo", "entity")
	repo.SetCachingService(NewInMemoryCachingService())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	// Get fails with non-notfound -> returned
	repo.SetPermissionService(&testEBRPermissions{})
	mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
	err := repo.ValidateAndDelete(context.WithValue(ctx, common.ContextKeyClaims, testClaims{username: "u1"}), "pk", "sk")
	assert.Error(t, err)

	// Get succeeds but permission denies -> returned
	deny := &testEBRPermissions{err: fmt.Errorf("deny")}
	repo.SetPermissionService(deny)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*testEBRModel)
		*dest = testEBRModel{PK: "pk", SK: "sk", ID: "1"}
	}).Return(nil).Once()
	err = repo.ValidateAndDelete(context.WithValue(ctx, common.ContextKeyClaims, testClaims{username: "u1"}), "pk", "sk")
	assert.Error(t, err)
}

func TestEnhancedBaseRepository_round09_more_crud_branches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", zap.NewNop(), nil, "Repo", "entity")

	// ValidateAndCreate create error branch
	mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
	assert.Error(t, repo.ValidateAndCreate(ctx, &testEBRModel{ID: "1"}))

	// ValidateAndUpdate update error branch + caching delete
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	cache := &testEBRCache{}
	repo.SetCachingService(cache)
	mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
	assert.Error(t, repo.ValidateAndUpdate(ctx, &testEBRModel{ID: "2"}))
	assert.GreaterOrEqual(t, cache.delCalls, 0)

	// ValidateAndDelete success path with permissions + events + cache invalidation
	perms := &testEBRPermissions{}
	repo.SetPermissionService(perms)
	events := &testEBREvents{}
	repo.SetEventService(events)

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*testEBRModel)
		*dest = testEBRModel{ID: "3", PK: "pk", SK: "sk"}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Once()

	ctxWithActor := context.WithValue(ctx, common.ContextKeyClaims, testClaims{username: "u1"})
	require.NoError(t, repo.ValidateAndDelete(ctxWithActor, "pk", "sk"))
	assert.NotEmpty(t, events.seen)

	// GetWithCache cache hit branch
	repo.SetCachingService(&testEBRCache{})
	var cached testEBRModel
	require.NoError(t, repo.GetWithCache(ctx, "pk", "sk", &cached, time.Minute))
	assert.Equal(t, "cached", cached.ID)
}

func TestEnhancedBaseRepository_round09_batch_create_error_branch(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", zap.NewNop(), nil, "Repo", "entity")
	items := []*testEBRModel{{ID: "1"}, {ID: "2"}}

	mockQuery.On("Create").Return(nil).Once()
	mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
	assert.Error(t, repo.ValidateAndBatchCreate(ctx, items))
}

func TestEnhancedBaseRepository_round09_permission_checks_skip_without_actor(t *testing.T) {
	repo := NewEnhancedBaseRepository[*testEBRModel](new(mocks.MockDB), "tbl", zap.NewNop(), nil, "Repo", "entity")
	perms := &testEBRPermissions{}
	repo.SetPermissionService(perms)

	require.NoError(t, repo.checkCreatePermissions(context.Background(), &testEBRModel{ID: "1"}))
	require.NoError(t, repo.checkUpdatePermissions(context.Background(), &testEBRModel{ID: "1"}))
	require.NoError(t, repo.checkDeletePermissions(context.Background(), &testEBRModel{ID: "1"}))
	assert.Equal(t, 0, perms.calls)
}

func TestEnhancedBaseRepository_round09_batch_validation_and_permissions(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	repo := NewEnhancedBaseRepository[*testEBRModel](mockDB, "tbl", zap.NewNop(), nil, "Repo", "entity")

	require.NoError(t, repo.ValidateAndBatchCreate(ctx, nil))

	repo.SetValidationService(&testEBRValidator{requiredFieldsErr: fmt.Errorf("missing")})
	err := repo.ValidateAndBatchCreate(ctx, []*testEBRModel{{ID: "1"}})
	assert.Error(t, err)

	repo.SetValidationService(nil)
	repo.SetPermissionService(&testEBRPermissions{err: fmt.Errorf("deny")})
	err = repo.ValidateAndBatchCreate(context.WithValue(ctx, common.ContextKeyClaims, testClaims{username: "u1"}), []*testEBRModel{{ID: "1"}})
	assert.Error(t, err)
}
