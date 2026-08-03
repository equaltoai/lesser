package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydb "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func setupMockAgentConfigRepoDB(db *dynamormmocks.MockDB, q *dynamormmocks.MockQuery) {
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
}

type testInstanceTransactionalDB struct {
	*dynamormmocks.MockDB
	err    error
	called bool
}

func (db *testInstanceTransactionalDB) TransactWrite(ctx context.Context, fn func(theorydb.TransactionBuilder) error) error {
	db.called = true
	if db.err != nil {
		return db.err
	}
	return fn(nil)
}

func TestNewInstanceTransactWriteFn_RequiresTransactionalDB(t *testing.T) {
	fn := newInstanceTransactWriteFn(new(dynamormmocks.MockDB))
	err := fn(context.Background(), func(theorydb.TransactionBuilder) error {
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database does not support transact write operations")
}

func TestNewInstanceTransactWriteFn_DelegatesAndPropagatesErrors(t *testing.T) {
	ctx := context.Background()
	db := &testInstanceTransactionalDB{MockDB: new(dynamormmocks.MockDB)}
	fnCalled := false

	err := newInstanceTransactWriteFn(db)(ctx, func(theorydb.TransactionBuilder) error {
		fnCalled = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, db.called)
	assert.True(t, fnCalled)

	expectedErr := errors.New("transact failed")
	failingDB := &testInstanceTransactionalDB{MockDB: new(dynamormmocks.MockDB), err: expectedErr}
	err = newInstanceTransactWriteFn(failingDB)(ctx, func(theorydb.TransactionBuilder) error {
		t.Fatal("transaction callback should not run when DB fails first")
		return nil
	})
	require.ErrorIs(t, err, expectedErr)
	assert.True(t, failingDB.called)
}

func TestInstanceRepository_AgentConfigExists_ReturnsFalseOnNotFound(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	ok, err := repo.AgentConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestInstanceRepository_AgentConfigExists_ReturnsTrueAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AgentInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = "AGENT_CONFIG"
		out.AllowAgentRegistration = true
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	ok, err := repo.AgentConfigExists(ctx)
	require.NoError(t, err)
	assert.True(t, ok)

	cached, cacheOK := repo.getCachedAgentConfig()
	require.True(t, cacheOK)
	assert.Equal(t, "AGENT_CONFIG", cached.SK)
	assert.True(t, cached.AllowAgentRegistration)
}

func TestInstanceRepository_AgentConfigExists_ReturnsFalseWhenKeysBlank(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	ok, err := repo.AgentConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)

	_, cacheOK := repo.getCachedAgentConfig()
	assert.False(t, cacheOK)
}

func TestInstanceRepository_AgentConfigExists_ReturnsError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(errors.New("boom")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	ok, err := repo.AgentConfigExists(ctx)
	require.Error(t, err)
	assert.False(t, ok)
}

func TestInstanceRepository_InvalidateAgentConfigCacheClearsCachedValue(t *testing.T) {
	repo := &InstanceRepository{}
	cfg := models.NewAgentInstanceConfig()
	repo.setCachedAgentConfig(cfg)

	cached, ok := repo.getCachedAgentConfig()
	require.True(t, ok)
	assert.Same(t, cfg, cached)

	repo.invalidateAgentConfigCache()
	_, ok = repo.getCachedAgentConfig()
	assert.False(t, ok)
}

func TestInstanceRepository_GetAgentInstanceConfig_DefaultWhenMissingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetAgentInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.False(t, cfg.AllowAgents)
	assert.False(t, cfg.AllowAgentRegistration)
	assert.NotEmpty(t, cfg.PK)
	assert.Equal(t, "AGENT_CONFIG", cfg.SK)

	// Cached path should not hit DB again.
	cfg2, err := repo.GetAgentInstanceConfig(ctx)
	require.NoError(t, err)
	assert.Same(t, cfg, cfg2)
}

func TestInstanceRepository_GetAgentInstanceConfig_ReturnsExistingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AgentInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = "AGENT_CONFIG"
		out.AllowAgents = true
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetAgentInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.AllowAgents)

	cfg2, err := repo.GetAgentInstanceConfig(ctx)
	require.NoError(t, err)
	assert.Same(t, cfg, cfg2)
}

func TestInstanceRepository_GetAgentInstanceConfig_ReturnsError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(errors.New("boom")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetAgentInstanceConfig(ctx)
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestInstanceRepository_EnsureAgentInstanceConfig_ReturnsExistingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AgentInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = "AGENT_CONFIG"
		out.AllowAgentRegistration = true
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureAgentInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.AllowAgentRegistration)

	cached, ok := repo.getCachedAgentConfig()
	require.True(t, ok)
	assert.Same(t, cfg, cached)
}

func TestInstanceRepository_EnsureAgentInstanceConfig_CreateWhenMissing(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)

	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureAgentInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "AGENT_CONFIG", cfg.SK)
	assert.False(t, cfg.AllowAgents)
	assert.False(t, cfg.AllowAgentRegistration)

	cached, ok := repo.getCachedAgentConfig()
	require.True(t, ok)
	assert.Same(t, cfg, cached)
}

func TestInstanceRepository_EnsureAgentInstanceConfig_ReturnsGetError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(errors.New("boom")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureAgentInstanceConfig(ctx)
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestInstanceRepository_EnsureAgentInstanceConfig_ReturnsCreateError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(errors.New("create failed")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureAgentInstanceConfig(ctx)
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestInstanceRepository_EnsureAgentInstanceConfig_ReturnsReadbackError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(errors.New("readback failed")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureAgentInstanceConfig(ctx)
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestInstanceRepository_EnsureAgentInstanceConfig_ConcurrentCreateReadsBack(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)

	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AgentInstanceConfig)
		*out = *models.NewAgentInstanceConfig()
		out.AllowAgents = true
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureAgentInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.AllowAgents)

	cached, ok := repo.getCachedAgentConfig()
	require.True(t, ok)
	assert.Same(t, cfg, cached)
}

func TestInstanceRepository_SetAgentInstanceConfig_UpdateSucceedsAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg := models.NewAgentInstanceConfig()
	cfg.AllowAgentRegistration = true

	require.NoError(t, repo.SetAgentInstanceConfig(ctx, cfg))

	cached, ok := repo.getCachedAgentConfig()
	require.True(t, ok)
	assert.Same(t, cfg, cached)
	assert.True(t, cached.AllowAgentRegistration)
}

func TestInstanceRepository_SetAgentInstanceConfig_UpsertsAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)

	q.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg := models.NewAgentInstanceConfig()
	cfg.AllowAgents = true

	require.NoError(t, repo.SetAgentInstanceConfig(ctx, cfg))

	cached, ok := repo.getCachedAgentConfig()
	require.True(t, ok)
	assert.Same(t, cfg, cached)
	assert.True(t, cached.AllowAgents)
}

func TestInstanceRepository_SetAgentInstanceConfig_ReturnsUpdateError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("Update", mock.Anything).Return(errors.New("update failed")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg := models.NewAgentInstanceConfig()

	require.Error(t, repo.SetAgentInstanceConfig(ctx, cfg))
}

func TestInstanceRepository_SetAgentInstanceConfig_ReturnsCreateError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockAgentConfigRepoDB(db, q)
	q.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(errors.New("create failed")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg := models.NewAgentInstanceConfig()

	require.Error(t, repo.SetAgentInstanceConfig(ctx, cfg))
}

func TestInstanceRepository_SetAgentInstanceConfig_NilRejected(t *testing.T) {
	repo := &InstanceRepository{}
	assert.Error(t, repo.SetAgentInstanceConfig(context.Background(), nil))
}
