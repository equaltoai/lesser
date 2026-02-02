package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormtesting "github.com/theory-cloud/tabletheory/pkg/testing"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

type costTrackerStub struct {
	ops []cost.DynamoOperation
	err error
}

func (s *costTrackerStub) TrackDynamoOperation(_ context.Context, operation cost.DynamoOperation) error {
	s.ops = append(s.ops, operation)
	return s.err
}

func TestStandaloneAlertRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	t.Run("missing_alert_id_is_error", func(t *testing.T) {
		repo := NewStandaloneAlertRepository(dynamormtesting.NewTestDB().MockDB, "table", logger, nil)
		err := repo.CreateAlert(ctx, &models.Alert{})
		require.Error(t, err)
	})

	t.Run("create_success_tracks_cost_and_sets_keys", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		alert := &models.Alert{
			AlertID:  "a1",
			Type:     "latency",
			Status:   "firing",
			Severity: "critical",
			Priority: "P0",
			Service:  "api",
		}
		testDB.ExpectCreate()

		costStub := &costTrackerStub{err: errors.New("track_failed")}
		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, costStub)

		err := repo.CreateAlert(ctx, alert)
		require.NoError(t, err)
		require.NotEmpty(t, alert.PK)
		require.NotEmpty(t, alert.SK)
		require.NotEmpty(t, costStub.ops)
		testDB.AssertExpectations(t)
	})

	t.Run("get_not_found_returns_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		pk := "ALERT#a1"
		sk := "METADATA"
		testDB.ExpectWhere("PK", "=", pk).
			ExpectWhere("SK", "=", sk).
			ExpectNotFound()

		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
		_, err := repo.GetByID(ctx, "a1")
		require.Error(t, err)
		testDB.AssertExpectations(t)
	})

	t.Run("get_success", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		pk := "ALERT#a1"
		sk := "METADATA"
		stored := &models.Alert{AlertID: "a1", PK: pk, SK: sk}

		testDB.ExpectWhere("PK", "=", pk).
			ExpectWhere("SK", "=", sk)
		testDB.MockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Alert)
			*dest = *stored
		}).Return(nil).Once()

		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
		got, err := repo.GetByID(ctx, "a1")
		require.NoError(t, err)
		require.Equal(t, "a1", got.AlertID)
		testDB.AssertExpectations(t)
	})
}

func TestStandaloneAlertRepository_UpdateAndQueries(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	t.Run("update_success", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		alert := &models.Alert{AlertID: "a1"}
		testDB.ExpectUpdate()

		costStub := &costTrackerStub{}
		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, costStub)

		err := repo.Update(ctx, alert)
		require.NoError(t, err)
		require.NotEmpty(t, costStub.ops)
		testDB.AssertExpectations(t)
	})

	t.Run("get_active_alerts", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		expected := []*models.Alert{{AlertID: "a1"}, {AlertID: "a2"}}

		testDB.ExpectIndex(models.IndexGSI3).
			ExpectWhere("gsi3PK", "=", "STATUS#firing").
			ExpectOrderBy("gsi3SK", "DESC").
			ExpectLimit(10)
		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Alert)
			*dest = expected
		}).Return(nil).Once()

		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
		alerts, err := repo.GetActiveAlerts(ctx, 10)
		require.NoError(t, err)
		require.Len(t, alerts, 2)
		testDB.AssertExpectations(t)
	})

	t.Run("alerts_needing_retry_filters_should_retry", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		nowPast := time.Now().Add(-time.Minute)
		raw := []*models.Alert{
			{AlertID: "a1", DeliveryAttempts: 0, NextRetryAt: &nowPast},
			{AlertID: "a2", DeliveryAttempts: 0, NextRetryAt: nil},
		}

		testDB.ExpectIndex(models.IndexGSI3).
			ExpectWhere("gsi3PK", "=", "STATUS#firing")
		testDB.MockQuery.On("Filter", "DeliveryAttempts", "<", 5).Return(testDB.MockQuery).Once()
		testDB.MockQuery.On("Filter", "NextRetryAt", "<=", mock.Anything).Return(testDB.MockQuery).Once()
		testDB.ExpectOrderBy("gsi3SK", "ASC").
			ExpectLimit(50)
		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Alert)
			*dest = raw
		}).Return(nil).Once()

		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
		alerts, err := repo.GetAlertsNeedingRetry(ctx, 50)
		require.NoError(t, err)
		require.Len(t, alerts, 1)
		require.Equal(t, "a1", alerts[0].AlertID)
		testDB.AssertExpectations(t)
	})
}

func TestStandaloneAlertRepository_ResolveAndCleanup(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	t.Run("resolve_alert_calls_update", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()

		testDB.ExpectWhere("PK", "=", "ALERT#a1").
			ExpectWhere("SK", "=", "METADATA")
		testDB.MockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Alert)
			*dest = models.Alert{AlertID: "a1", PK: "ALERT#a1", SK: "METADATA", Status: "firing"}
		}).Return(nil).Once()

		testDB.MockQuery.On("Update", []string(nil)).Return(nil).Once()

		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
		err := repo.ResolveAlert(ctx, "a1")
		require.NoError(t, err)
		testDB.AssertExpectations(t)
	})

	t.Run("cleanup_old_alerts_deletes_found_items", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()

		// Loosely allow the query chain for multiple alert types; return one alert on first All call.
		testDB.MockQuery.On("Index", mock.Anything).Return(testDB.MockQuery).Maybe()
		testDB.MockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(testDB.MockQuery).Maybe()
		testDB.MockQuery.On("Limit", mock.Anything).Return(testDB.MockQuery).Maybe()

		allCalls := 0
		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Alert)
			if allCalls == 0 {
				*dest = []*models.Alert{{AlertID: "a1", PK: "ALERT#a1", SK: "METADATA"}}
			} else {
				*dest = nil
			}
			allCalls++
		}).Return(nil).Maybe()

		testDB.MockQuery.On("Delete").Return(nil).Maybe()

		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
		deleted, err := repo.CleanupOldAlerts(ctx, time.Hour)
		require.NoError(t, err)
		require.Equal(t, 1, deleted)
	})

	t.Run("get_by_id_propagates_non_not_found_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectWhere("PK", "=", "ALERT#a1").
			ExpectWhere("SK", "=", "METADATA")
		testDB.MockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()

		repo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
		_, err := repo.GetByID(ctx, "a1")
		require.Error(t, err)
		testDB.AssertExpectations(t)
	})
}
