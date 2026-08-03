package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestAlertRepository_CRUD_AndGetByIDBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateAlert validation error", func(t *testing.T) {
		repo := NewAlertRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		require.Error(t, repo.CreateAlert(ctx, &models.Alert{}))
	})

	t.Run("CreateAlert sets times and creates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)

		alert := &models.Alert{
			AlertID:  "a1",
			Type:     "latency",
			Severity: "critical",
			Priority: "P0",
			Status:   "firing",
			Service:  "api",
			Title:    "high latency",
		}

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.Alert")).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		err := repo.CreateAlert(ctx, alert)
		require.NoError(t, err)
		assert.False(t, alert.CreatedAt.IsZero())
		assert.False(t, alert.UpdatedAt.IsZero())
		assert.False(t, alert.FiredAt.IsZero())
	})

	t.Run("GetByID not found uses not found error branch", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.Alert")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "ALERT#a1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", models.SKMetadata).Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Alert")).Return(errors.New("not found")).Once()

		alert, err := repo.GetByID(ctx, "a1")
		require.Error(t, err)
		assert.Nil(t, alert)
	})

	t.Run("GetByID success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.Alert")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "ALERT#a1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", models.SKMetadata).Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Alert")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Alert)
			dest.AlertID = "a1"
			dest.Status = "firing"
		}).Return(nil).Once()

		alert, err := repo.GetByID(ctx, "a1")
		require.NoError(t, err)
		require.NotNil(t, alert)
		assert.Equal(t, "a1", alert.AlertID)
	})

	t.Run("Update and Delete", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)

		alert := &models.Alert{
			AlertID:   "a1",
			Type:      "latency",
			Severity:  "critical",
			Priority:  "P0",
			Status:    "firing",
			Service:   "api",
			FiredAt:   time.Now().Add(-time.Minute),
			CreatedAt: time.Now().Add(-time.Minute),
			UpdatedAt: time.Now().Add(-time.Minute),
		}

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.AnythingOfType("*models.Alert")).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		mockQuery.On("Delete").Return(nil).Once()

		require.NoError(t, repo.Update(ctx, alert))
		require.NoError(t, repo.Delete(ctx, "a1"))
	})
}

func TestAlertRepository_QueryMethods_AndHelpers(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	t.Run("GetActiveAlerts success and error", func(t *testing.T) {
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Alert)
			*dest = []*models.Alert{{AlertID: "a1"}}
		}).Return(nil).Once()

		alerts, err := repo.GetActiveAlerts(ctx, 10)
		require.NoError(t, err)
		require.Len(t, alerts, 1)

		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(ErrTestMockError).Once()
		_, err = repo.GetActiveAlerts(ctx, 10)
		require.Error(t, err)
	})

	t.Run("GetAlertsByType and GetAlertsByService", func(t *testing.T) {
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(nil).Twice()
		_, err := repo.GetAlertsByType(ctx, "latency", time.Now().Add(-time.Hour), 5)
		require.NoError(t, err)
		_, err = repo.GetAlertsByService(ctx, "api", 5)
		require.NoError(t, err)
	})

	t.Run("GetAlertsByType and GetAlertsByService errors", func(t *testing.T) {
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(ErrTestMockError).Twice()
		_, err := repo.GetAlertsByType(ctx, "latency", time.Now().Add(-time.Hour), 5)
		require.Error(t, err)
		_, err = repo.GetAlertsByService(ctx, "api", 5)
		require.Error(t, err)
	})

	t.Run("queryAlertsWithIndex covers gsi2 and gsi3 prefixes", func(t *testing.T) {
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(nil).Twice()
		_, err := repo.GetAlertsBySeverity(ctx, "api", "critical", time.Now().Add(-time.Hour), 5)
		require.NoError(t, err)
		_, err = repo.GetAlertsByPriority(ctx, "firing", "P0", time.Now().Add(-time.Hour), 5)
		require.NoError(t, err)
	})

	t.Run("GetAlertsNeedingRetry filters via ShouldRetry", func(t *testing.T) {
		past := time.Now().Add(-time.Minute)
		future := time.Now().Add(time.Hour)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Alert)
			*dest = []*models.Alert{
				{AlertID: "a1", NextRetryAt: &past, DeliveryAttempts: 0},
				{AlertID: "a2", NextRetryAt: &future, DeliveryAttempts: 0},
				{AlertID: "a3", NextRetryAt: &past, DeliveryAttempts: 5},
			}
		}).Return(nil).Once()

		alerts, err := repo.GetAlertsNeedingRetry(ctx, 10)
		require.NoError(t, err)
		require.Len(t, alerts, 1)
		assert.Equal(t, "a1", alerts[0].AlertID)
	})

	t.Run("GetAlertsNeedingRetry error", func(t *testing.T) {
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(ErrTestMockError).Once()
		_, err := repo.GetAlertsNeedingRetry(ctx, 10)
		require.Error(t, err)
	})
}

func TestAlertRepository_Stats_And_Cleanup(t *testing.T) {
	ctx := context.Background()

	t.Run("GetAlertStats mixes count paths", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		// countAlertsByStatus uses Count().
		mockQuery.On("Count").Return(int64(0), ErrTestMockError).Once()
		mockQuery.On("Count").Return(int64(2), nil).Maybe()

		// getAllAlertsSince uses All() for each alert type; include errors to hit continue.
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(ErrTestMockError).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Alert)
			*dest = []*models.Alert{
				{AlertID: "a1", Severity: "critical"},
				{AlertID: "a2", Severity: "warning"},
			}
		}).Return(nil).Maybe()

		stats, err := repo.GetAlertStats(ctx, time.Now().Add(-time.Hour))
		require.NoError(t, err)
		require.NotNil(t, stats)
		assert.GreaterOrEqual(t, stats.TotalCount, 0)
		assert.NotNil(t, stats.BySeverity)
		assert.NotNil(t, stats.ByType)
	})

	t.Run("CleanupOldAlerts queries types and deletes with per-item error continue", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Alert)
			*dest = []*models.Alert{{AlertID: "a1"}, {AlertID: "a2"}}
		}).Return(nil).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(ErrTestMockError).Maybe()

		mockQuery.On("First", mock.AnythingOfType("*models.Alert")).Return(nil).Twice()

		mockQuery.On("Delete").Return(ErrTestMockError).Once()
		mockQuery.On("Delete").Return(nil).Once()

		count, err := repo.CleanupOldAlerts(ctx, 24*time.Hour)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 0)
	})
}

func TestAlertRepository_ActionMethods_AndDeadLetterMessage(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.Alert")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Alert)
		dest.AlertID = "a1"
		dest.Type = "latency"
		dest.Status = "firing"
		dest.Title = "t"
		dest.FiredAt = time.Now().Add(-time.Minute)
		dest.CreatedAt = time.Now().Add(-time.Minute)
		dest.UpdatedAt = time.Now().Add(-time.Minute)
	}).Return(nil).Times(3)

	mockQuery.On("Update", mock.Anything).Return(nil).Times(3)

	require.NoError(t, repo.ResolveAlert(ctx, "a1"))
	require.NoError(t, repo.AcknowledgeAlert(ctx, "a1"))
	require.NoError(t, repo.SuppressAlert(ctx, "a1", time.Now().Add(time.Hour)))

	mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(nil).Once()
	_, err := repo.GetCriticalAlerts(ctx, 5)
	require.NoError(t, err)

	msg := &DeadLetterMessage{}
	require.Error(t, msg.UpdateKeys())

	msg = &DeadLetterMessage{MessageID: "m1", OriginalType: "alert"}
	require.NoError(t, msg.UpdateKeys())
	assert.NotEmpty(t, msg.GetPK())
	assert.NotEmpty(t, msg.GetSK())

	msg2 := &DeadLetterMessage{MessageID: "m2", OriginalType: "alert", TTL: 123}
	require.NoError(t, msg2.UpdateKeys())
	assert.Equal(t, int64(123), msg2.TTL)
}
