package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestInstanceRepository_GetAgentInstanceConfig_DefaultWhenMissingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetAgentInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.False(t, cfg.AllowAgents)
	assert.NotEmpty(t, cfg.PK)
	assert.Equal(t, "AGENT_CONFIG", cfg.SK)

	// Cached path should not hit DB again.
	cfg2, err := repo.GetAgentInstanceConfig(ctx)
	require.NoError(t, err)
	assert.Same(t, cfg, cfg2)
}

func TestInstanceRepository_EnsureAgentInstanceConfig_CreateWhenMissing(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureAgentInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "AGENT_CONFIG", cfg.SK)

	cached, ok := repo.getCachedAgentConfig()
	require.True(t, ok)
	assert.Same(t, cfg, cached)
}

func TestInstanceRepository_EnsureAgentInstanceConfig_ConcurrentCreateReadsBack(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

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

func TestInstanceRepository_SetAgentInstanceConfig_UpsertsAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()

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

func TestInstanceRepository_SetAgentInstanceConfig_NilRejected(t *testing.T) {
	repo := &InstanceRepository{}
	assert.Error(t, repo.SetAgentInstanceConfig(context.Background(), nil))
}
