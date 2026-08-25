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

// Error-branch and semantic-branch coverage for the O(1) instance-count
// helpers (instance_counts.go). The maintenance helpers are best-effort: on
// repository failure they log and continue, so these tests pin "no panic, no
// error propagation" for each failure mode.

func TestInstanceCounts_Bump_ExecuteErrorIsBestEffort(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)
	mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate)
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
	mockUpdate.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate)
	mockUpdate.On("Execute").Return(errors.New("boom"))

	// Must not panic and must not return an error.
	bumpInstanceTotalUsers(ctx, mockDB, zap.NewNop(), 1)
	bumpInstanceTotalDomains(ctx, mockDB, zap.NewNop(), -1)
}

func TestInstanceCounts_ReadStatuses_ValueFallback(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.InstanceMetrics{})

	// Legacy writers may store the counter in Value only.
	metric := &models.InstanceMetrics{PK: models.InstanceMetricsPK, SK: models.TotalStatusesMetricSK, Value: 5, UpdatedAt: time.Now()}
	require.NoError(t, db.WithContext(ctx).Model(metric).Create())

	count, err := readTotalStatusesCount(ctx, db, zap.NewNop())
	require.NoError(t, err)
	require.EqualValues(t, 5, count)

	// TotalStatuses attribute wins when present.
	require.NoError(t, db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.TotalStatusesMetricSK).
		UpdateBuilder().
		Set("TotalStatuses", int64(7)).
		Set("UpdatedAt", time.Now()).
		Execute())
	count, err = readTotalStatusesCount(ctx, db, zap.NewNop())
	require.NoError(t, err)
	require.EqualValues(t, 7, count)
}

func TestInstanceCounts_ReadField_UnknownAndError(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.InstanceMetrics{})
	metric := &models.InstanceMetrics{PK: models.InstanceMetricsPK, SK: models.TotalUsersMetricSK, TotalUsers: 4, UpdatedAt: time.Now()}
	require.NoError(t, db.WithContext(ctx).Model(metric).Create())

	// Unknown field reads as zero.
	v, err := readInstanceMetricsField(ctx, db, zap.NewNop(), models.TotalUsersMetricSK, "BogusField")
	require.NoError(t, err)
	require.Zero(t, v)

	// Repository error propagates.
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = readInstanceMetricsField(ctx, mockDB, zap.NewNop(), models.TotalUsersMetricSK, "TotalUsers")
	require.Error(t, err)
}

func TestInstanceCounts_EnsureTotalUsersSeeded_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(resetInstanceSeedBackoffForTest)

	t.Run("seed check error propagates", func(t *testing.T) {
		resetInstanceSeedBackoffForTest()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		_, err := ensureTotalUsersSeeded(ctx, mockDB, zap.NewNop())
		require.Error(t, err)
	})

	t.Run("compute scan error propagates and backoffs", func(t *testing.T) {
		resetInstanceSeedBackoffForTest()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Times(2)
		mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("scan failed")).Once()
		_, err := ensureTotalUsersSeeded(ctx, mockDB, zap.NewNop())
		require.Error(t, err)
		// The failed scan must enter backoff so it is not re-armed next read.
		requireBackoffActive(t, models.TotalUsersMetricSK)
	})

	t.Run("persist failure serves last known value and backoffs", func(t *testing.T) {
		resetInstanceSeedBackoffForTest()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdate := new(mocks.MockUpdateBuilder)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Times(2)
		mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			users := args.Get(0).(*[]models.User)
			*users = []models.User{{Username: "a"}, {Username: "b"}}
		}).Return(nil).Once()
		mockQuery.On("UpdateBuilder").Return(mockUpdate)
		mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Execute").Return(errors.New("persist failed")).Once()
		value, err := ensureTotalUsersSeeded(ctx, mockDB, zap.NewNop())
		require.NoError(t, err)
		require.EqualValues(t, 2, value)
		requireBackoffActive(t, models.TotalUsersMetricSK)
	})
}

func TestInstanceCounts_EnsureTotalDomainsSeeded_ScanError(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(resetInstanceSeedBackoffForTest)
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Times(2)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("scan failed")).Once()
	_, err := ensureTotalDomainsSeeded(ctx, mockDB, zap.NewNop())
	require.Error(t, err)
	requireBackoffActive(t, models.TotalDomainsMetricSK)
}

func TestInstanceCounts_EnsureActiveMonthSeeded_ScanError(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(resetInstanceSeedBackoffForTest)
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Times(2)
	mockDB.On("Model", mock.AnythingOfType("*models.Activity")).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("scan failed")).Once()
	err := ensureActiveMonthSeeded(ctx, mockDB, zap.NewNop(), 30)
	require.Error(t, err)
	requireBackoffActive(t, models.ActiveMonthSeedMetricSK)
}

func TestInstanceCounts_RecordActivityActorDay_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("marker create error is best-effort", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ActivityActorDay")).Return(mockQuery)
		mockQuery.On("IfNotExists").Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		recordActivityActorDay(ctx, mockDB, zap.NewNop(), "https://example.com/users/a", "2026-08-25")
	})

	t.Run("day counter bump error is best-effort", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdate := new(mocks.MockUpdateBuilder)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ActivityActorDay")).Return(mockQuery)
		mockQuery.On("IfNotExists").Return(mockQuery)
		mockQuery.On("Create").Return(nil).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.ActivityDayCounter")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("UpdateBuilder").Return(mockUpdate)
		mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Execute").Return(errors.New("boom")).Once()
		recordActivityActorDay(ctx, mockDB, zap.NewNop(), "https://example.com/users/a", "2026-08-25")
	})
}

func TestInstanceCounts_RecordActorDomain_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("counter create error is best-effort", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.DomainCounter")).Return(mockQuery)
		mockQuery.On("IfNotExists").Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		recordActorDomain(ctx, mockDB, zap.NewNop(), "example.com")
	})

	t.Run("existing-domain tally error is best-effort", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdate := new(mocks.MockUpdateBuilder)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.DomainCounter")).Return(mockQuery)
		mockQuery.On("IfNotExists").Return(mockQuery)
		mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("UpdateBuilder").Return(mockUpdate)
		mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Execute").Return(errors.New("boom")).Once()
		recordActorDomain(ctx, mockDB, zap.NewNop(), "example.com")
	})
}

func TestInstanceCounts_ReleaseActorDomain_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("condition failed is a no-op", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdate := new(mocks.MockUpdateBuilder)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.DomainCounter")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("UpdateBuilder").Return(mockUpdate)
		mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("ExecuteWithResult", mock.Anything).Return(dynamormErrors.ErrConditionFailed).Once()
		releaseActorDomain(ctx, mockDB, zap.NewNop(), "example.com")
	})

	t.Run("decrement error is best-effort", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdate := new(mocks.MockUpdateBuilder)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.DomainCounter")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("UpdateBuilder").Return(mockUpdate)
		mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("ExecuteWithResult", mock.Anything).Return(errors.New("boom")).Once()
		releaseActorDomain(ctx, mockDB, zap.NewNop(), "example.com")
	})

	t.Run("empty-domain delete error is best-effort", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdate := new(mocks.MockUpdateBuilder)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.DomainCounter")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("UpdateBuilder").Return(mockUpdate)
		mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate)
		mockUpdate.On("ExecuteWithResult", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.DomainCounter)
			dest.Value = 0
		}).Return(nil).Once()
		mockQuery.On("Delete").Return(errors.New("boom")).Once()
		releaseActorDomain(ctx, mockDB, zap.NewNop(), "example.com")
	})
}

func TestInstanceCounts_ReadActiveMonth_DayCounterError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.InstanceMetrics)
		return ok
	})).Return(nil).Maybe() // seed marker present
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.ActivityDayCounter)
		return ok
	})).Return(errors.New("boom")).Once()

	_, err := readActiveMonthCount(ctx, mockDB, zap.NewNop(), 30)
	require.Error(t, err)
}

func TestInstanceCounts_DayBucketing_Semantics(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)

	// Published time wins for bucketing.
	published := now.Add(-48 * time.Hour)
	a := models.Activity{Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{Published: &published}}, CreatedAt: now}
	require.Equal(t, "2026-08-23", activityDayOf(a))
	require.True(t, activityInWindow(a, now.Add(-72*time.Hour)))
	require.False(t, activityInWindow(a, now.Add(-24*time.Hour)))

	// Creation time fallback when unpublished.
	b := models.Activity{CreatedAt: now}
	require.Equal(t, "2026-08-25", activityDayOf(b))
	require.True(t, activityInWindow(b, now.Add(-time.Hour)))
	require.False(t, activityInWindow(b, now.Add(time.Hour)))
}

func TestDomainFromActorID_Semantics(t *testing.T) {
	require.Equal(t, "", domainFromActorID(""))
	require.Equal(t, "", domainFromActorID("not-a-url"))
	require.Equal(t, "example.com", domainFromActorID("https://example.com/users/alice"))
	require.Equal(t, "example.com", domainFromActorID("https://example.com"))
}
