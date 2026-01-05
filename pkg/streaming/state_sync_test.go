package streaming

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubConflictResolver struct {
	resolve func(ctx context.Context, local, remote *models.WebSocketConnection) (*models.WebSocketConnection, error)
}

func (r *stubConflictResolver) ResolveConflict(ctx context.Context, local, remote *models.WebSocketConnection) (*models.WebSocketConnection, error) {
	if r.resolve == nil {
		return local, nil
	}
	return r.resolve(ctx, local, remote)
}

func TestStateSynchronizer_IdentifyResolveAndCleanup(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	ss := NewStateSynchronizer(repo, zap.NewNop(), &StateSynchronizerConfig{
		InstanceID:       "i1",
		SyncInterval:     time.Hour,
		StaleThreshold:   time.Second,
		ConflictResolver: &LastWriteWinsResolver{},
	})

	now := time.Now()
	staleTime := now.Add(-10 * time.Second)

	connOldActivity := models.WebSocketConnection{
		ConnectionID:   "c1",
		State:          models.ConnectionStateConnected,
		LastActivity:   staleTime,
		StateChangedAt: now,
	}
	connConnecting := models.WebSocketConnection{
		ConnectionID:   "c2",
		State:          models.ConnectionStateConnecting,
		LastActivity:   now,
		StateChangedAt: staleTime,
	}
	connErrorMaxed := models.WebSocketConnection{
		ConnectionID:   "c3",
		State:          models.ConnectionStateError,
		LastActivity:   now,
		StateChangedAt: staleTime,
		RetryCount:     2,
		MaxRetries:     2,
	}
	connFresh := models.WebSocketConnection{
		ConnectionID:   "c4",
		State:          models.ConnectionStateConnected,
		LastActivity:   now,
		StateChangedAt: now,
	}

	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnecting).Return([]models.WebSocketConnection{connConnecting}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).Return([]models.WebSocketConnection{connOldActivity, connFresh}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateClosing).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateError).Return([]models.WebSocketConnection{connErrorMaxed}, nil).Once()

	stale, err := ss.identifyStaleConnections(context.Background())
	require.NoError(t, err)
	assert.Len(t, stale, 3)

	// Resolve conflicts
	ss.conflictResolver = &stubConflictResolver{
		resolve: func(_ context.Context, _ *models.WebSocketConnection, remote *models.WebSocketConnection) (*models.WebSocketConnection, error) {
			return remote, nil
		},
	}

	currentC1 := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateIdle, StateChangedAt: now}
	currentC2 := &models.WebSocketConnection{ConnectionID: "c2", State: models.ConnectionStateConnecting, StateChangedAt: connConnecting.StateChangedAt.Add(2 * time.Second)}

	repo.On("GetConnection", mock.Anything, "c1").Return(currentC1, nil).Once()
	repo.On("UpdateConnection", mock.Anything, mock.MatchedBy(func(c *models.WebSocketConnection) bool {
		return c.ConnectionID == "c1" && c.State == models.ConnectionStateIdle
	})).Return(nil).Once()

	repo.On("GetConnection", mock.Anything, "c2").Return(currentC2, nil).Once()
	repo.On("GetConnection", mock.Anything, "c3").Return(nil, errors.New("not found")).Once()

	resolved, err := ss.resolveStateConflicts(context.Background(), stale)
	require.NoError(t, err)
	assert.Equal(t, 1, resolved)
	assert.Equal(t, int64(1), ss.syncStats.ConflictsResolved)

	// Cleanup orphaned connections
	oldClosed := models.WebSocketConnection{ConnectionID: "x1", State: models.ConnectionStateClosed, StateChangedAt: now.Add(-2 * time.Hour)}
	recentClosed := models.WebSocketConnection{ConnectionID: "x2", State: models.ConnectionStateClosed, StateChangedAt: now.Add(-10 * time.Minute)}
	failClosed := models.WebSocketConnection{ConnectionID: "x3", State: models.ConnectionStateClosed, StateChangedAt: now.Add(-2 * time.Hour)}

	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateClosed).
		Return([]models.WebSocketConnection{oldClosed, recentClosed, failClosed}, nil).
		Once()
	repo.On("DeleteAllSubscriptions", mock.Anything, "x1").Return(nil).Once()
	repo.On("DeleteConnection", mock.Anything, "x1").Return(nil).Once()
	repo.On("DeleteAllSubscriptions", mock.Anything, "x3").Return(errors.New("subs down")).Once()
	repo.On("DeleteConnection", mock.Anything, "x3").Return(errors.New("delete down")).Once()

	cleaned, err := ss.cleanupOrphanedConnections(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, cleaned)
}

func TestStateSynchronizer_UpdateSyncStatsAndForceSync(t *testing.T) {
	ss := NewStateSynchronizer(testmocks.NewMockStreamingConnectionRepository(), zap.NewNop(), &StateSynchronizerConfig{
		InstanceID:       "i1",
		SyncInterval:     time.Hour,
		StaleThreshold:   time.Second,
		ConflictResolver: &LastWriteWinsResolver{},
	})

	// Running average branch.
	ss.syncStats.TotalSyncs = 3
	ss.syncStats.AverageSyncDuration = 10 * time.Second
	ss.updateSyncStats(20*time.Second, 0, 0, 0)
	assert.NotZero(t, ss.lastSyncTime)
	assert.Equal(t, 40*time.Second/3, ss.syncStats.AverageSyncDuration)

	// ForceSync requires running.
	require.Error(t, ss.ForceSync(context.Background()))
}

func TestStateSynchronizer_StartStop(t *testing.T) {
	ss := NewStateSynchronizer(testmocks.NewMockStreamingConnectionRepository(), zap.NewNop(), &StateSynchronizerConfig{
		InstanceID:       "i1",
		SyncInterval:     time.Hour,
		StaleThreshold:   time.Second,
		ConflictResolver: &LastWriteWinsResolver{},
	})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, ss.Start(ctx))
	require.Error(t, ss.Start(ctx))
	cancel()
	require.NoError(t, ss.Stop())
	require.False(t, ss.IsRunning())
	require.NoError(t, ss.Stop())
}

func TestConflictResolvers(t *testing.T) {
	now := time.Now()
	local := &models.WebSocketConnection{State: models.ConnectionStateConnected, StateChangedAt: now, Metrics: models.ConnectionMetrics{ConnectionQuality: 0.4}}
	remote := &models.WebSocketConnection{State: models.ConnectionStateIdle, StateChangedAt: now.Add(time.Second), Metrics: models.ConnectionMetrics{ConnectionQuality: 0.9}}

	r := &LastWriteWinsResolver{}
	res, err := r.ResolveConflict(context.Background(), local, remote)
	require.NoError(t, err)
	assert.Equal(t, remote, res)
	res, err = r.ResolveConflict(context.Background(), remote, local)
	require.NoError(t, err)
	assert.Equal(t, remote, res)

	r2 := &HighestPriorityResolver{}
	res, err = r2.ResolveConflict(context.Background(), local, remote)
	require.NoError(t, err)
	assert.Equal(t, local, res)
	res, err = r2.ResolveConflict(context.Background(), remote, local)
	require.NoError(t, err)
	assert.Equal(t, local, res)

	// Same priority falls back to last write wins.
	remote.State = local.State
	remote.StateChangedAt = local.StateChangedAt.Add(time.Second)
	res, err = r2.ResolveConflict(context.Background(), local, remote)
	require.NoError(t, err)
	assert.Equal(t, remote, res)

	local.State = models.ConnectionStateError
	local.Metrics.ErrorCount = 10
	remote.State = models.ConnectionStateConnected
	remote.Metrics.ErrorCount = 0

	r3 := &HealthBasedResolver{}
	res, err = r3.ResolveConflict(context.Background(), local, remote)
	require.NoError(t, err)
	assert.Equal(t, remote, res)

	// Local healthier wins.
	local.State = models.ConnectionStateConnected
	local.Metrics.ErrorCount = 0
	remote.State = models.ConnectionStateError
	remote.Metrics.ErrorCount = 10
	res, err = r3.ResolveConflict(context.Background(), local, remote)
	require.NoError(t, err)
	assert.Equal(t, local, res)

	// Quality and timestamp tiebreakers.
	local.State = models.ConnectionStateIdle
	remote.State = models.ConnectionStateIdle
	local.Metrics.ConnectionQuality = 0.1
	remote.Metrics.ConnectionQuality = 0.2
	res, err = r3.ResolveConflict(context.Background(), local, remote)
	require.NoError(t, err)
	assert.Equal(t, remote, res)

	remote.Metrics.ConnectionQuality = local.Metrics.ConnectionQuality
	remote.StateChangedAt = local.StateChangedAt.Add(time.Second)
	res, err = r3.ResolveConflict(context.Background(), local, remote)
	require.NoError(t, err)
	assert.Equal(t, remote, res)
}

func TestHostnameResolution(t *testing.T) {
	t.Setenv("HOSTNAME", "h1")
	t.Setenv("ECS_TASK_ID", "")
	t.Setenv("POD_NAME", "")
	assert.Equal(t, "h1", getHostname())

	t.Setenv("HOSTNAME", "")
	t.Setenv("ECS_TASK_ID", "task1")
	assert.Equal(t, "task1", getHostname())

	t.Setenv("HOSTNAME", "")
	t.Setenv("ECS_TASK_ID", "")
	t.Setenv("POD_NAME", "pod1")
	assert.Equal(t, "pod1", getHostname())

	t.Setenv("HOSTNAME", "")
	t.Setenv("ECS_TASK_ID", "")
	t.Setenv("POD_NAME", "")
	assert.Equal(t, "streaming-instance", getHostname())

	t.Setenv("HOSTNAME", "h1")
	assert.Contains(t, generateInstanceID(), "h1")
}

func TestStateSynchronizer_DefaultConfigAndInstanceInfo(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	ss := NewStateSynchronizer(repo, zap.NewNop(), nil)
	assert.NotEmpty(t, ss.instanceID)
	assert.True(t, ss.syncInterval > 0)
	assert.True(t, ss.staleThreshold > 0)
	require.NotNil(t, ss.conflictResolver)

	info := ss.GetInstanceInfo()
	assert.Equal(t, ss.instanceID, info["instance_id"])
	assert.Equal(t, false, info["is_running"])
	assert.NotNil(t, info["stats"])
}

func TestStateSynchronizer_ForceSync_CoversPerformSync(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	ss := NewStateSynchronizer(repo, zap.NewNop(), &StateSynchronizerConfig{
		InstanceID:       "i1",
		SyncInterval:     time.Hour,
		StaleThreshold:   time.Second,
		ConflictResolver: &LastWriteWinsResolver{},
	})

	// performSync internally calls identifyStaleConnections + cleanupOrphanedConnections.
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnecting).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateClosing).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateError).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateClosed).Return([]models.WebSocketConnection{}, nil).Once()

	ctx := context.Background()
	require.NoError(t, ss.Start(ctx))
	require.NoError(t, ss.ForceSync(ctx))
	require.NotZero(t, ss.GetSyncStats().LastSyncDuration)
	require.NoError(t, ss.Stop())
}

func TestStateSynchronizer_SyncRoutine_TickBranches(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := testmocks.NewMockStreamingConnectionRepository()
		repo.On("GetConnectionsByState", mock.Anything, mock.Anything).Return([]models.WebSocketConnection{}, nil).Maybe()

		ss := NewStateSynchronizer(repo, zap.NewNop(), &StateSynchronizerConfig{
			InstanceID:       "i1",
			SyncInterval:     5 * time.Millisecond,
			StaleThreshold:   time.Second,
			ConflictResolver: &LastWriteWinsResolver{},
		})

		require.NoError(t, ss.Start(context.Background()))
		time.Sleep(25 * time.Millisecond)
		require.NoError(t, ss.Stop())
		assert.True(t, ss.syncStats.TotalSyncs > 0)
		assert.True(t, ss.syncStats.SuccessfulSyncs > 0)
	})

	t.Run("failure", func(t *testing.T) {
		repo := testmocks.NewMockStreamingConnectionRepository()
		repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateClosed).Return(nil, errors.New("boom")).Maybe()
		repo.On("GetConnectionsByState", mock.Anything, mock.Anything).Return([]models.WebSocketConnection{}, nil).Maybe()

		ss := NewStateSynchronizer(repo, zap.NewNop(), &StateSynchronizerConfig{
			InstanceID:       "i1",
			SyncInterval:     5 * time.Millisecond,
			StaleThreshold:   time.Second,
			ConflictResolver: &LastWriteWinsResolver{},
		})

		require.NoError(t, ss.Start(context.Background()))
		time.Sleep(25 * time.Millisecond)
		require.NoError(t, ss.Stop())
		assert.True(t, ss.syncStats.TotalSyncs > 0)
		assert.True(t, ss.syncStats.FailedSyncs > 0)
	})
}
