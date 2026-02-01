package observability

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormtesting "github.com/theory-cloud/tabletheory/pkg/testing"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestWebhookDeliveryService_GetMatchingWebhooksAndFilters(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.Get()
	orig := cfg.AlertWebhookURL
	cfg.AlertWebhookURL = "https://example.com/webhook"
	t.Cleanup(func() { cfg.AlertWebhookURL = orig })

	svc := &WebhookDeliveryService{
		logger:               logger,
		enabled:              true,
		defaultTimeout:       time.Second,
		defaultMaxAttempts:   3,
		defaultRetryInterval: time.Second,
	}

	alert := &models.Alert{AlertID: "a1", Type: "latency", Severity: "critical", Service: "api"}
	webhooks, err := svc.getMatchingWebhooks(context.Background(), alert)
	require.NoError(t, err)
	require.Len(t, webhooks, 1)

	assert.True(t, svc.matchesWebhookFilters(alert, &WebhookConfig{
		AlertTypes:     []string{"latency"},
		SeverityLevels: []string{"critical"},
		Services:       []string{"api"},
	}))
	assert.False(t, svc.matchesWebhookFilters(alert, &WebhookConfig{
		AlertTypes: []string{"error_rate"},
	}))
}

func TestWebhookDeliveryService_DeliverAlert_NoWebhookConfigured(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.Get()
	orig := cfg.AlertWebhookURL
	cfg.AlertWebhookURL = ""
	t.Cleanup(func() { cfg.AlertWebhookURL = orig })

	svc := &WebhookDeliveryService{logger: logger, enabled: true}
	err := svc.DeliverAlert(context.Background(), &models.Alert{AlertID: "a1", Type: "t", Severity: "info"})
	require.NoError(t, err)
}

func TestWebhookDeliveryService_DeliverAlert_SuccessAndFailurePaths(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	cfg := config.Get()
	orig := cfg.AlertWebhookURL
	cfg.AlertWebhookURL = "https://example.com/webhook"
	t.Cleanup(func() { cfg.AlertWebhookURL = orig })

	alertDB := dynamormtesting.NewTestDB()
	webhookDB := dynamormtesting.NewTestDB()

	alertRepo := NewStandaloneAlertRepository(alertDB.MockDB, "table", logger, nil)
	webhookRepo := NewStandaloneWebhookRepository(webhookDB.MockDB, "table", logger)

	svc := NewWebhookDeliveryService(&WebhookDeliveryConfig{
		Logger:            logger,
		WebhookRepository: webhookRepo,
		AlertRepository:   alertRepo,
		Enabled:           true,
	})

	t.Run("success_updates_alert", func(t *testing.T) {
		alertDB.MockQuery.On("Update", []string(nil)).Return(nil).Once()

		svc.httpClient.Transport = webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`ok`)),
				Header:     http.Header{},
				Request:    req,
			}, nil
		})

		alert := &models.Alert{
			AlertID:    "a1",
			Type:       "t",
			Severity:   "info",
			Priority:   "P3",
			Status:     "firing",
			Title:      "t",
			Message:    "m",
			Service:    "svc",
			Region:     "us-east-1",
			Source:     "src",
			FiredAt:    time.Now(),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Dimensions: map[string]string{},
		}

		err := svc.DeliverAlert(ctx, alert)
		require.NoError(t, err)
		assert.Equal(t, 1, alert.DeliveryAttempts)
		assert.Nil(t, alert.NextRetryAt)
		alertDB.AssertExpectations(t)
	})

	t.Run("failure_stores_delivery_and_sets_retry", func(t *testing.T) {
		alertDB.MockQuery.On("Update", []string(nil)).Return(nil).Once()
		webhookDB.MockQuery.On("Create").Return(nil).Once()

		svc.httpClient.Transport = webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 500,
				Status:     "500 Internal Server Error",
				Body:       io.NopCloser(strings.NewReader(`nope`)),
				Header:     http.Header{},
				Request:    req,
			}, nil
		})

		alert := &models.Alert{
			AlertID:    "a2",
			Type:       "t",
			Severity:   "info",
			Priority:   "P3",
			Status:     "firing",
			Title:      "t",
			Message:    "m",
			Service:    "svc",
			Region:     "us-east-1",
			Source:     "src",
			FiredAt:    time.Now(),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Dimensions: map[string]string{},
		}

		err := svc.DeliverAlert(ctx, alert)
		require.Error(t, err)
		assert.Equal(t, 1, alert.DeliveryAttempts)
		assert.NotNil(t, alert.NextRetryAt)
		alertDB.AssertExpectations(t)
		webhookDB.AssertExpectations(t)
	})
}

func TestWebhookDeliveryService_RetryFailedDeliveries_SendsToDLQ(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	webhookDB := dynamormtesting.NewTestDB()
	alertDB := dynamormtesting.NewTestDB()
	dlqDB := dynamormtesting.NewTestDB()

	webhookRepo := NewStandaloneWebhookRepository(webhookDB.MockDB, "table", logger)
	alertRepo := NewStandaloneAlertRepository(alertDB.MockDB, "table", logger, nil)
	dlqRepo := NewStandaloneDeadLetterRepository(dlqDB.MockDB, "table", logger)

	svc := NewWebhookDeliveryService(&WebhookDeliveryConfig{
		Logger:               logger,
		WebhookRepository:    webhookRepo,
		AlertRepository:      alertRepo,
		DeadLetterRepository: dlqRepo,
		Enabled:              true,
	})

	// One pending retry delivery.
	webhookDB.ExpectIndex(models.IndexGSI2).
		ExpectWhere("gsi2PK", "=", "STATUS#retrying")
	webhookDB.MockQuery.On("Filter", "NextRetryAt", "<=", mock.Anything).Return(webhookDB.MockQuery).Once()
	webhookDB.ExpectOrderBy("gsi2SK", "ASC").
		ExpectLimit(100)

	// AttemptNumber increments before delivery; after that it will equal MaxAttempts and be sent to DLQ.
	past := time.Now().Add(-time.Minute)
	raw := []*models.WebhookDelivery{
		{
			DeliveryID:    "d1",
			AlertID:       "a1",
			WebhookID:     "w1",
			URL:           "https://example.com/webhook",
			Status:        models.DeliveryStatusFailed,
			AttemptNumber: 1,
			MaxAttempts:   2,
			NextRetryAt:   &past,
		},
	}
	webhookDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.WebhookDelivery)
		*dest = raw
	}).Return(nil).Once()

	// Get alert for retry.
	alertDB.ExpectWhere("PK", "=", "ALERT#a1").
		ExpectWhere("SK", "=", "METADATA")
	alertDB.MockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Alert)
		*dest = models.Alert{AlertID: "a1", FiredAt: time.Now()}
	}).Return(nil).Once()

	// Delivery fails -> DLQ write.
	dlqDB.MockQuery.On("Create").Return(nil).Once()
	webhookDB.MockQuery.On("Update", []string(nil)).Return(nil).Once()

	svc.httpClient.Transport = webhookRTFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Status:     "500 Internal Server Error",
			Body:       io.NopCloser(strings.NewReader(`nope`)),
			Header:     http.Header{},
			Request:    req,
		}, nil
	})

	require.NoError(t, svc.RetryFailedDeliveries(ctx))
	webhookDB.AssertExpectations(t)
	alertDB.AssertExpectations(t)
	dlqDB.AssertExpectations(t)
}

func TestWebhookDeliveryService_SendToDeadLetterQueue_NoRepoNoop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := &WebhookDeliveryService{logger: logger, deadLetterRepo: nil}
	require.NoError(t, svc.sendToDeadLetterQueue(context.Background(), &models.WebhookDelivery{DeliveryID: "d"}, &models.Alert{AlertID: "a"}, errors.New("boom")))
}
