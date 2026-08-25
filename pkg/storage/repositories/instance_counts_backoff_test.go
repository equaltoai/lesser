package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// resetInstanceSeedBackoffForTest clears the package-level backoff registry so
// tests are hermetic (the registry is process-lifetime state).
func resetInstanceSeedBackoffForTest() {
	instanceSeedBackoff.mu.Lock()
	defer instanceSeedBackoff.mu.Unlock()
	instanceSeedBackoff.entries = make(map[string]instanceSeedBackoffEntry)
}

// setSeedBackoffTTLForTest overrides the jittered TTL for the duration of the
// test and resets the backoff registry.
func setSeedBackoffTTLForTest(t *testing.T, d time.Duration) {
	t.Helper()
	t.Cleanup(resetInstanceSeedBackoffForTest)
	old := seedBackoffTTL
	seedBackoffTTL = func() time.Duration { return d }
	t.Cleanup(func() { seedBackoffTTL = old })
}

// requireBackoffActive asserts an entry is inside its backoff window.
func requireBackoffActive(t *testing.T, metric string) {
	t.Helper()
	_, ok := instanceSeedBackoffValue(metric)
	require.True(t, ok, "expected seed backoff active for %q", metric)
}

// requireBackoffExpired asserts the backoff window for a metric has closed
// (entry lazily cleaned on read).
func requireBackoffExpired(t *testing.T, metric string) {
	t.Helper()
	_, ok := instanceSeedBackoffValue(metric)
	require.False(t, ok, "expected seed backoff expired for %q", metric)
}

// TestInstanceCounts_SeedBackoff_PersistFailureNoRescan pins F1: a persist
// failure must not re-arm the full-body scan at the next read. Within the
// backoff window the read serves the last known value and the probe read
// calls the counter, never the scan.
func TestInstanceCounts_SeedBackoff_PersistFailureNoRescan(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(resetInstanceSeedBackoffForTest)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		users := args.Get(0).(*[]models.User)
		*users = []models.User{{Username: "a"}, {Username: "b"}}
	}).Return(nil)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
	mockUpdate.On("Execute").Return(errors.New("persist failed"))

	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	// First read: compute succeeds, persist fails -> last known value served,
	// backoff recorded, no error.
	count, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	requireBackoffActive(t, models.TotalUsersMetricSK)

	// Second read inside the window: the scan must NOT run again; the last
	// known value is served.
	count, err = repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	mockQuery.AssertNumberOfCalls(t, "All", 1)
}

// TestInstanceCounts_SeedBackoff_ExpiryAllowsSingleRetry pins F1: after the
// backoff window expires exactly one retry is allowed, and a successful retry
// clears the backoff state.
func TestInstanceCounts_SeedBackoff_ExpiryAllowsSingleRetry(t *testing.T) {
	ctx := context.Background()
	setSeedBackoffTTLForTest(t, time.Millisecond)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		users := args.Get(0).(*[]models.User)
		*users = []models.User{{Username: "a"}, {Username: "b"}}
	}).Return(nil)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
	mockUpdate.On("Execute").Return(errors.New("persist failed")).Once()
	mockUpdate.On("Execute").Return(nil).Once()

	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	count, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	requireBackoffActive(t, models.TotalUsersMetricSK)

	// Within the window: no re-scan.
	_, err = repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, 1, numberOfCalls(&mockQuery.Mock, "All"))

	// After the window expires: exactly one retry is allowed and succeeds.
	time.Sleep(5 * time.Millisecond)
	requireBackoffExpired(t, models.TotalUsersMetricSK)
	count, err = repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	mockQuery.AssertNumberOfCalls(t, "All", 2)
}

// TestInstanceCounts_ActiveMonthSeed_MarkerPersistFailureNoRescan pins F1a/F1b:
// a seed-marker persist failure must not re-arm the activity body scan at the
// next read; the marker-write-failure path respects the in-memory backoff.
func TestInstanceCounts_ActiveMonthSeed_MarkerPersistFailureNoRescan(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(resetInstanceSeedBackoffForTest)

	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)

	mockDB := new(mocks.MockDB)
	mockMetrics := new(mocks.MockQuery)
	mockActivity := new(mocks.MockQuery)
	mockDay := new(mocks.MockQuery)
	mockUpdateDay := new(mocks.MockUpdateBuilder)
	mockUpdateMarker := new(mocks.MockUpdateBuilder)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockMetrics)
	mockMetrics.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockMetrics)
	mockMetrics.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)
	// The final seed-marker persist fails.
	mockMetrics.On("UpdateBuilder").Return(mockUpdateMarker)
	mockUpdateMarker.On("Set", mock.Anything, mock.Anything).Return(mockUpdateMarker)
	mockUpdateMarker.On("Execute").Return(errors.New("marker persist failed"))

	mockDB.On("Model", mock.AnythingOfType("*models.Activity")).Return(mockActivity)
	mockActivity.On("All", mock.Anything).Run(func(args mock.Arguments) {
		activities := args.Get(0).(*[]models.Activity)
		*activities = []models.Activity{
			{Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1", Published: &yesterday}}, CreatedAt: yesterday},
			{Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a2", Published: &yesterday}}, CreatedAt: yesterday},
		}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.ActivityDayCounter")).Return(mockDay)
	mockDay.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockDay)
	// The mock does not actually store the written counters, so the bounded
	// read-side sum sees no day counters (0) — the invariant pinned here is
	// that the scan never re-runs and the backoff is respected.
	mockDay.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)
	mockDay.On("UpdateBuilder").Return(mockUpdateDay)
	mockUpdateDay.On("Set", mock.Anything, mock.Anything).Return(mockUpdateDay)
	mockUpdateDay.On("Execute").Return(nil)

	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	count, err := repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Zero(t, count)
	requireBackoffActive(t, models.ActiveMonthSeedMetricSK)

	// Second read inside the window: the scan must NOT run again; the read
	// serves the day-counter rollup.
	count, err = repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Zero(t, count)
	mockActivity.AssertNumberOfCalls(t, "All", 1)
}

// numberOfCalls returns how many times a method has been invoked on a mock.
func numberOfCalls(m *mock.Mock, method string) int {
	calls := 0
	for _, call := range m.Calls {
		if call.Method == method {
			calls++
		}
	}
	return calls
}
