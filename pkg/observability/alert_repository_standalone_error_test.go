package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	dynamormtesting "github.com/pay-theory/dynamorm/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestStandaloneWebhookRepository_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	t.Run("create_delivery_key_error", func(t *testing.T) {
		repo := NewStandaloneWebhookRepository(dynamormtesting.NewTestDB().MockDB, "table", logger)
		err := repo.CreateDelivery(ctx, &models.WebhookDelivery{})
		require.Error(t, err)
	})

	t.Run("create_delivery_db_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.MockQuery.On("Create").Return(errors.New("boom")).Once()

		repo := NewStandaloneWebhookRepository(testDB.MockDB, "table", logger)
		err := repo.CreateDelivery(ctx, &models.WebhookDelivery{
			DeliveryID: "d1",
			WebhookID:  "w1",
			AlertID:    "a1",
			Status:     "failed",
		})
		require.Error(t, err)
		testDB.AssertExpectations(t)
	})

	t.Run("update_delivery_db_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.MockQuery.On("Update", []string(nil)).Return(errors.New("boom")).Once()

		repo := NewStandaloneWebhookRepository(testDB.MockDB, "table", logger)
		err := repo.UpdateDelivery(ctx, &models.WebhookDelivery{
			DeliveryID: "d1",
			WebhookID:  "w1",
		})
		require.Error(t, err)
		testDB.AssertExpectations(t)
	})

	t.Run("get_pending_retries_db_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectIndex(models.IndexGSI2).
			ExpectWhere("gsi2PK", "=", "STATUS#retrying")
		testDB.MockQuery.On("Filter", "NextRetryAt", "<=", mock.Anything).Return(testDB.MockQuery).Once()
		testDB.ExpectOrderBy("gsi2SK", "ASC").
			ExpectLimit(1)
		testDB.MockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()

		repo := NewStandaloneWebhookRepository(testDB.MockDB, "table", logger)
		_, err := repo.GetPendingRetries(ctx, 1)
		require.Error(t, err)
		testDB.AssertExpectations(t)
	})

	t.Run("get_deliveries_by_alert_db_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectIndex(models.IndexGSI1).
			ExpectWhere("gsi1PK", "=", "ALERT#a1").
			ExpectOrderBy("gsi1SK", "DESC").
			ExpectLimit(1)
		testDB.MockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()

		repo := NewStandaloneWebhookRepository(testDB.MockDB, "table", logger)
		_, err := repo.GetDeliveriesByAlert(ctx, "a1", 1)
		require.Error(t, err)
		testDB.AssertExpectations(t)
	})
}

func TestStandaloneDeadLetterRepository_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	t.Run("create_key_error", func(t *testing.T) {
		repo := NewStandaloneDeadLetterRepository(dynamormtesting.NewTestDB().MockDB, "table", logger)
		err := repo.Create(ctx, &models.DeadLetterMessage{})
		require.Error(t, err)
	})

	t.Run("create_db_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.MockQuery.On("Create").Return(errors.New("boom")).Once()

		repo := NewStandaloneDeadLetterRepository(testDB.MockDB, "table", logger)
		err := repo.Create(ctx, &models.DeadLetterMessage{
			MessageID:     "m1",
			OriginalType:  "t",
			OriginalID:    "id",
			ErrorMessage:  "err",
			ErrorType:     "type",
			AttemptCount:  1,
			LastAttemptAt: time.Now(),
			Payload:       map[string]interface{}{"k": "v"},
			CreatedAt:     time.Now(),
		})
		require.Error(t, err)
		testDB.AssertExpectations(t)
	})

	t.Run("get_by_type_db_error", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectWhere("PK", "=", "DLQ#type").
			ExpectOrderBy("SK", "DESC").
			ExpectLimit(1)
		testDB.MockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()

		repo := NewStandaloneDeadLetterRepository(testDB.MockDB, "table", logger)
		_, err := repo.GetByType(ctx, "type", 1)
		require.Error(t, err)
		testDB.AssertExpectations(t)
	})
}
