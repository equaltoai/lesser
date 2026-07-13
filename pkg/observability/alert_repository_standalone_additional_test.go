package observability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormtesting "github.com/theory-cloud/tabletheory/v2/pkg/testing"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestStandaloneWebhookRepository_CRUDAndQueries(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	t.Run("create_and_update_delivery", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectCreate()
		testDB.ExpectUpdate()

		repo := NewStandaloneWebhookRepository(testDB.MockDB, "table", logger)

		delivery := &models.WebhookDelivery{
			DeliveryID:  "d1",
			AlertID:     "a1",
			WebhookID:   "w1",
			URL:         "https://example.com",
			Status:      "retrying",
			MaxAttempts: 3,
		}

		require.NoError(t, repo.CreateDelivery(ctx, delivery))
		require.NotEmpty(t, delivery.PK)
		require.NotEmpty(t, delivery.SK)

		require.NoError(t, repo.UpdateDelivery(ctx, delivery))
		testDB.AssertExpectations(t)
	})

	t.Run("get_pending_retries_filters_should_retry", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()

		testDB.ExpectIndex(models.IndexGSI2).
			ExpectWhere("gsi2PK", "=", "STATUS#retrying")
		testDB.MockQuery.On("Filter", "NextRetryAt", "<=", mock.Anything).Return(testDB.MockQuery).Once()
		testDB.ExpectOrderBy("gsi2SK", "ASC").
			ExpectLimit(10)

		past := time.Now().Add(-time.Minute)
		raw := []*models.WebhookDelivery{
			{DeliveryID: "d1", AlertID: "a1", WebhookID: "w1", Status: models.DeliveryStatusFailed, AttemptNumber: 1, MaxAttempts: 3, NextRetryAt: &past},
			{DeliveryID: "d2", AlertID: "a1", WebhookID: "w1", Status: "success", AttemptNumber: 1, MaxAttempts: 3},
		}

		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.WebhookDelivery)
			*dest = raw
		}).Return(nil).Once()

		repo := NewStandaloneWebhookRepository(testDB.MockDB, "table", logger)
		got, err := repo.GetPendingRetries(ctx, 10)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "d1", got[0].DeliveryID)
		testDB.AssertExpectations(t)
	})

	t.Run("get_deliveries_by_alert", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectIndex(models.IndexGSI1).
			ExpectWhere("gsi1PK", "=", "ALERT#a1").
			ExpectOrderBy("gsi1SK", "DESC").
			ExpectLimit(5)

		raw := []*models.WebhookDelivery{{DeliveryID: "d1"}, {DeliveryID: "d2"}}
		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.WebhookDelivery)
			*dest = raw
		}).Return(nil).Once()

		repo := NewStandaloneWebhookRepository(testDB.MockDB, "table", logger)
		got, err := repo.GetDeliveriesByAlert(ctx, "a1", 5)
		require.NoError(t, err)
		require.Len(t, got, 2)
		testDB.AssertExpectations(t)
	})
}

func TestStandaloneDeadLetterRepository_CreateAndGetByType(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	testDB := dynamormtesting.NewTestDB()
	testDB.ExpectCreate()
	testDB.ExpectWhere("PK", "=", "DLQ#type").
		ExpectOrderBy("SK", "DESC").
		ExpectLimit(10)

	raw := []*models.DeadLetterMessage{{MessageID: "m1"}}
	testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DeadLetterMessage)
		*dest = raw
	}).Return(nil).Once()

	repo := NewStandaloneDeadLetterRepository(testDB.MockDB, "table", logger)
	msg := &models.DeadLetterMessage{
		MessageID:    "m1",
		OriginalType: "type",
		Payload:      map[string]interface{}{"k": "v"},
	}
	require.NoError(t, repo.Create(ctx, msg))

	got, err := repo.GetByType(ctx, "type", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	testDB.AssertExpectations(t)
}
