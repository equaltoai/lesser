package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/apptheory/pkg/streamer"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type fakeNotificationRepo struct {
	notifications     map[string]*models.Notification
	getErr            error
	updateErr         error
	markPushSentErr   error
	updateCalls       int
	markPushSentCalls int
}

func (f *fakeNotificationRepo) GetNotification(_ context.Context, notificationID string) (*models.Notification, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if n, ok := f.notifications[notificationID]; ok {
		return n, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeNotificationRepo) UpdateNotification(_ context.Context, notification *models.Notification) error {
	f.updateCalls++
	if f.notifications != nil && notification != nil {
		f.notifications[notification.ID] = notification
	}
	return f.updateErr
}

func (f *fakeNotificationRepo) MarkNotificationPushSent(_ context.Context, _ string) error {
	f.markPushSentCalls++
	return f.markPushSentErr
}

type fakeUserRepo struct {
	prefs *storage.UserPreferences
	err   error
}

func (f *fakeUserRepo) GetUserPreferences(_ context.Context, _ string) (*storage.UserPreferences, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.prefs, nil
}

type fakeTrackingRepo struct {
	err     error
	records []*models.DynamoDBCostRecord
}

func (f *fakeTrackingRepo) Create(_ context.Context, tracking *models.DynamoDBCostRecord) error {
	f.records = append(f.records, tracking)
	return f.err
}

type fakeNotificationCostRepo struct {
	createErr     error
	budget        *models.NotificationBudget
	budgetErr     error
	dailySpending int64
	dailyErr      error
}

func (f *fakeNotificationCostRepo) CreateCostTracking(_ context.Context, _ *models.NotificationCostTracking) error {
	return f.createErr
}

func (f *fakeNotificationCostRepo) GetBudget(_ context.Context, _, _ string) (*models.NotificationBudget, error) {
	if f.budgetErr != nil {
		return nil, f.budgetErr
	}
	return f.budget, nil
}

func (f *fakeNotificationCostRepo) GetDailySpending(_ context.Context, _ string) (int64, error) {
	if f.dailyErr != nil {
		return 0, f.dailyErr
	}
	return f.dailySpending, nil
}

type fakeWebSocketSubRepo struct {
	connections []string
	err         error
}

func (f *fakeWebSocketSubRepo) GetUserConnections(_ context.Context, _ string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.connections, nil
}

type fakeSQSSender struct {
	err       error
	sendCalls int
	last      *sqs.SendMessageInput
}

func (f *fakeSQSSender) SendMessage(_ context.Context, params *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.sendCalls++
	f.last = params
	if f.err != nil {
		return nil, f.err
	}
	return &sqs.SendMessageOutput{MessageId: aws.String("m1")}, nil
}

type fakeSNSPublisher struct {
	err          error
	publishCalls int
	last         *sns.PublishInput
}

func (f *fakeSNSPublisher) Publish(_ context.Context, params *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	f.publishCalls++
	f.last = params
	if f.err != nil {
		return nil, f.err
	}
	return &sns.PublishOutput{MessageId: aws.String("msg1")}, nil
}

type fakeWSClient struct {
	failIDs map[string]error
	posts   int
}

func (f *fakeWSClient) PostToConnection(_ context.Context, connectionID string, _ []byte) error {
	f.posts++
	if err, ok := f.failIDs[connectionID]; ok {
		return err
	}
	return nil
}

func (f *fakeWSClient) DeleteConnection(context.Context, string) error { return nil }
func (f *fakeWSClient) GetConnection(_ context.Context, _ string) (streamer.Connection, error) {
	return streamer.Connection{}, nil
}

func TestInitializeNotificationProcessor_AndMain(t *testing.T) {
	origMustInitialize := mustInitializeLambdaFn
	origInitializeDefaults := initializeWithDefaultsFn
	origNewProcessor := newNotificationProcessorFn
	origStart := lambdaStartFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInitialize
		initializeWithDefaultsFn = origInitializeDefaults
		newNotificationProcessorFn = origNewProcessor
		lambdaStartFn = origStart
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Logger: zap.NewNop(),
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("defaults") }
	newNotificationProcessorFn = func(*common.LambdaContext) *NotificationProcessor {
		return &NotificationProcessor{logger: zap.NewNop()}
	}

	var startHandler any
	lambdaStartFn = func(h any) { startHandler = h }

	require.NoError(t, initializeNotificationProcessor())
	require.NotNil(t, lambdaCtx)
	require.NotNil(t, processor)

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()
	require.NotNil(t, startHandler)

	handlerFn, ok := startHandler.(func(context.Context, json.RawMessage) (any, error))
	require.True(t, ok)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId:      "m1",
				Body:           "{",
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-dev-notification-processor-queue",
				EventSource:    "aws:sqs",
			},
		},
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)

	respAny, err := handlerFn(context.Background(), raw)
	require.NoError(t, err)
	resp, ok := respAny.(events.SQSEventResponse)
	require.True(t, ok)
	require.Len(t, resp.BatchItemFailures, 1)
	require.Equal(t, "m1", resp.BatchItemFailures[0].ItemIdentifier)
}

func TestRetryableError_Error(t *testing.T) {
	require.Equal(t, "retryable error", (&RetryableError{}).Error())
	require.Equal(t, "retryable error: boom", (&RetryableError{OriginalError: errors.New("boom")}).Error())
}

func TestNewNotificationProcessor_Branches(t *testing.T) {
	origDynamo := dynamormGetClientFn
	origStreamer := streamerNewClientFn
	t.Cleanup(func() {
		dynamormGetClientFn = origDynamo
		streamerNewClientFn = origStreamer
	})

	mockDB := new(dynamormmocks.MockDB)
	dynamormGetClientFn = func(context.Context) (dynamormCore.DB, error) { return mockDB, nil }

	lambdaCfg := &config.Config{
		Domain:          "example.com",
		Region:          "us-east-1",
		DynamoTableName: "test-table",
		JWTSecret:       "secret",
	}
	ctx := &common.LambdaContext{
		Config: lambdaCfg,
		Logger: zap.NewNop(),
		Repos:  nil,
	}

	config.ResetForTests()
	t.Setenv("WEBSOCKET_ENDPOINT", "")
	config.ResetForTests()

	p := NewNotificationProcessor(ctx)
	require.NotNil(t, p)
	require.Nil(t, p.wsClient)

	ctx.AWSServices = &awsinit.AWSServices{Config: aws.Config{Region: "us-east-1"}}
	t.Setenv("WEBSOCKET_ENDPOINT", "https://ws.example.com")
	config.ResetForTests()

	streamerNewClientFn = func(context.Context, string, ...streamer.Option) (streamer.Client, error) {
		return nil, errors.New("boom")
	}
	p = NewNotificationProcessor(ctx)
	require.Nil(t, p.wsClient)

	streamerNewClientFn = func(context.Context, string, ...streamer.Option) (streamer.Client, error) {
		return &fakeWSClient{}, nil
	}
	p = NewNotificationProcessor(ctx)
	require.NotNil(t, p.wsClient)
}

func TestNotificationProcessor_HandleSQSMessage_ReturnsErrorOnInvalidMessage(t *testing.T) {
	p := &NotificationProcessor{logger: zap.NewNop()}
	require.Error(t, p.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1", Body: "not-json"}))
}

func TestNotificationProcessor_HandleSQSMessage_RecoversPanic(t *testing.T) {
	p := &NotificationProcessor{logger: zap.NewNop()}
	body, err := json.Marshal(NotificationDeliveryRequest{NotificationID: "n1", UserID: "u1"})
	require.NoError(t, err)

	require.Error(t, p.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1", Body: string(body)}))
}

func TestNotificationProcessor_processMessage_Branches(t *testing.T) {
	now := time.Now().UTC()
	notif := &models.Notification{
		ID:        "n1",
		UserID:    "u1",
		Type:      "mention",
		Title:     "t",
		Body:      "b",
		ActorID:   "a1",
		TargetID:  "t1",
		CreatedAt: now,
		Data:      map[string]any{"retry_count": float64(1)},
	}

	nRepo := &fakeNotificationRepo{notifications: map[string]*models.Notification{"n1": notif}}
	trackingRepo := &fakeTrackingRepo{}
	costRepo := &fakeNotificationCostRepo{budget: &models.NotificationBudget{Enabled: true, LimitMicroCents: 1_000_000, SpentMicroCents: 0}}
	subRepo := &fakeWebSocketSubRepo{connections: []string{"c1", "c2"}}
	wsClient := &fakeWSClient{failIDs: map[string]error{"c1": errors.New("gone")}}

	sqsSender := &fakeSQSSender{}
	snsPublisher := &fakeSNSPublisher{}

	p := &NotificationProcessor{
		tableName:                 "test-table",
		logger:                    zap.NewNop(),
		notificationRepo:          nRepo,
		userRepo:                  &fakeUserRepo{prefs: &storage.UserPreferences{Preferences: map[string]string{"push_enabled": "true"}}},
		costTrackingRepo:          trackingRepo,
		notificationCostRepo:      costRepo,
		webSocketSubscriptionRepo: subRepo,
		snsClient:                 snsPublisher,
		wsClient:                  wsClient,
		sqsClient:                 sqsSender,
		domain:                    "example.com",
		retryQueueURL:             "https://example.com/retry",
		deadLetterQueueURL:        "https://example.com/dlq",
	}

	t.Run("unmarshal error", func(t *testing.T) {
		require.Error(t, p.processMessage(context.Background(), events.SQSMessage{MessageId: "m", Body: "{"}))
	})

	t.Run("scheduled future is requeued (delay capped)", func(t *testing.T) {
		future := time.Now().Add(24 * time.Hour)
		body, err := json.Marshal(NotificationDeliveryRequest{
			NotificationID: "n1",
			UserID:         "u1",
			Channels:       []string{"push"},
			ScheduledAt:    &future,
		})
		require.NoError(t, err)

		require.NoError(t, p.processMessage(context.Background(), events.SQSMessage{MessageId: "m", Body: string(body)}))
		require.NotNil(t, sqsSender.last)
		require.Equal(t, int32(900), sqsSender.last.DelaySeconds)
	})

	t.Run("get notification error", func(t *testing.T) {
		p.notificationRepo = &fakeNotificationRepo{getErr: errors.New("boom")}
		body, err := json.Marshal(NotificationDeliveryRequest{NotificationID: "n1", UserID: "u1"})
		require.NoError(t, err)

		require.Error(t, p.processMessage(context.Background(), events.SQSMessage{Body: string(body)}))
		p.notificationRepo = nRepo
	})

	t.Run("budget blocked", func(t *testing.T) {
		p.notificationCostRepo = &fakeNotificationCostRepo{
			budget: &models.NotificationBudget{
				Enabled:         true,
				BudgetExceeded:  true,
				LimitMicroCents: 100,
				SpentMicroCents: 100,
			},
		}
		body, err := json.Marshal(NotificationDeliveryRequest{
			NotificationID: "n1",
			UserID:         "u1",
			Channels:       []string{"push"},
		})
		require.NoError(t, err)
		require.ErrorContains(t, p.processMessage(context.Background(), events.SQSMessage{Body: string(body)}), "budget exceeded")
		p.notificationCostRepo = costRepo
	})

	t.Run("deliver push+websocket success", func(t *testing.T) {
		config.ResetForTests()
		t.Setenv("PUSH_NOTIFICATION_TOPIC_ARN", "arn:aws:sns:us-east-1:123456789012:push")
		config.ResetForTests()

		p.notificationRepo = nRepo
		p.notificationCostRepo = costRepo
		p.userRepo = &fakeUserRepo{prefs: &storage.UserPreferences{Preferences: map[string]string{"push_enabled": "true", "websocket_enabled": "true"}}}
		p.webSocketSubscriptionRepo = subRepo
		p.wsClient = wsClient
		p.snsClient = snsPublisher
		nRepo.markPushSentErr = errors.New("mark failed")

		body, err := json.Marshal(NotificationDeliveryRequest{
			NotificationID: "n1",
			UserID:         "u1",
			Channels:       []string{"push", "websocket"},
		})
		require.NoError(t, err)

		require.NoError(t, p.processMessage(context.Background(), events.SQSMessage{Body: string(body)}))
		require.GreaterOrEqual(t, snsPublisher.publishCalls, 1)
	})

	t.Run("schedule retry when delivery fails and under max retries", func(t *testing.T) {
		origRandRead := randReadFn
		t.Cleanup(func() { randReadFn = origRandRead })
		randReadFn = func(p []byte) (int, error) {
			for i := range p {
				p[i] = 0
			}
			return len(p), nil
		}

		p.userRepo = &fakeUserRepo{prefs: &storage.UserPreferences{Preferences: map[string]string{"push_enabled": "false"}}}

		body, err := json.Marshal(NotificationDeliveryRequest{
			NotificationID: "n1",
			UserID:         "u1",
			Channels:       []string{"push"},
			RetryCount:     0,
		})
		require.NoError(t, err)

		require.NoError(t, p.processMessage(context.Background(), events.SQSMessage{Body: string(body)}))
		require.GreaterOrEqual(t, sqsSender.sendCalls, 1)
	})

	t.Run("max retries exceeded sends to DLQ", func(t *testing.T) {
		p.userRepo = &fakeUserRepo{prefs: &storage.UserPreferences{Preferences: map[string]string{"push_enabled": "false"}}}

		body, err := json.Marshal(NotificationDeliveryRequest{
			NotificationID: "n1",
			UserID:         "u1",
			Channels:       []string{"push"},
			RetryCount:     DefaultRetryPolicy().MaxRetries,
		})
		require.NoError(t, err)

		require.NoError(t, p.processMessage(context.Background(), events.SQSMessage{Body: string(body)}))
		require.GreaterOrEqual(t, sqsSender.sendCalls, 1)
	})
}

func TestNotificationProcessor_Misc_Branches(t *testing.T) {
	p := &NotificationProcessor{
		logger: zap.NewNop(),
	}

	t.Run("deliverPush requires sns client", func(t *testing.T) {
		require.ErrorContains(t, p.deliverPush(context.Background(), &models.Notification{}, nil), "SNS client not initialized")
	})

	t.Run("sendWebSocketMessage requires websocket client", func(t *testing.T) {
		require.ErrorContains(t, p.sendWebSocketMessage(context.Background(), "c1", WebSocketMessage{}), "API Gateway client not initialized")
	})

	t.Run("sendWebSocketMessage marshal error", func(t *testing.T) {
		p.wsClient = &fakeWSClient{}
		msg := WebSocketMessage{Type: "t", Event: "e", Payload: map[string]any{"bad": make(chan int)}}
		require.Error(t, p.sendWebSocketMessage(context.Background(), "c1", msg))
	})

	t.Run("getActiveWebSocketConnections swallows repo errors", func(t *testing.T) {
		p.webSocketSubscriptionRepo = &fakeWebSocketSubRepo{err: errors.New("boom")}
		conns, err := p.getActiveWebSocketConnections(context.Background(), "u1")
		require.NoError(t, err)
		require.Empty(t, conns)
	})

	t.Run("deliverToChannel unsupported", func(t *testing.T) {
		p.notificationCostRepo = &fakeNotificationCostRepo{}
		p.costTrackingRepo = &fakeTrackingRepo{}
		result := p.deliverToChannel(context.Background(), &models.Notification{ID: "n1", UserID: "u1", Type: "t"}, &UserPreferences{}, "fax")
		require.False(t, result.Success)
		require.Contains(t, result.Error, "unsupported delivery channel")
	})

	t.Run("storeCostTracking continues on repo errors", func(t *testing.T) {
		p.tableName = "test-table"
		p.notificationCostRepo = &fakeNotificationCostRepo{createErr: errors.New("boom")}
		p.costTrackingRepo = &fakeTrackingRepo{err: errors.New("boom")}

		require.NoError(t, p.storeCostTracking(context.Background(), &models.NotificationCostTracking{
			NotificationID:      "n1",
			UserID:              "u1",
			Username:            "u1",
			DeliveryMethod:      "push",
			NotificationType:    "mention",
			Timestamp:           time.Now(),
			TotalCostMicroCents: 10,
			Tags:                map[string]string{"k": "v"},
			Success:             true,
			RetryCount:          0,
		}))
	})

	t.Run("sendPushNotification config and publish errors", func(t *testing.T) {
		config.ResetForTests()
		t.Setenv("PUSH_NOTIFICATION_TOPIC_ARN", "")
		config.ResetForTests()

		require.ErrorContains(t, p.sendPushNotification(context.Background(), "u1", []byte(`{}`)), "PUSH_NOTIFICATION_TOPIC_ARN not configured")

		config.ResetForTests()
		t.Setenv("PUSH_NOTIFICATION_TOPIC_ARN", "arn:aws:sns:us-east-1:123456789012:push")
		config.ResetForTests()

		p.snsClient = &fakeSNSPublisher{err: errors.New("boom")}
		require.ErrorContains(t, p.sendPushNotification(context.Background(), "u1", []byte(`{}`)), "failed to publish push notification to SNS")
	})

	t.Run("deliverWebSocket branches", func(t *testing.T) {
		p.webSocketSubscriptionRepo = &fakeWebSocketSubRepo{connections: []string{}}
		require.NoError(t, p.deliverWebSocket(context.Background(), &models.Notification{ID: "n1", UserID: "u1", CreatedAt: time.Now()}))

		p.webSocketSubscriptionRepo = &fakeWebSocketSubRepo{connections: []string{"c1"}}
		p.wsClient = &fakeWSClient{failIDs: map[string]error{"c1": errors.New("boom")}}
		require.ErrorContains(t, p.deliverWebSocket(context.Background(), &models.Notification{ID: "n1", UserID: "u1", CreatedAt: time.Now()}), "failed to deliver to any websocket connections")
	})

	t.Run("scheduleRetry and DLQ branches", func(t *testing.T) {
		p.sqsClient = nil
		p.retryQueueURL = ""
		require.ErrorContains(t, p.scheduleRetry(context.Background(), NotificationDeliveryRequest{NotificationID: "n1"}, errors.New("boom")), "SQS client or retry queue URL not configured")

		p.sqsClient = &fakeSQSSender{err: errors.New("boom")}
		p.retryQueueURL = "https://example.com/retry"
		require.ErrorContains(t, p.scheduleRetry(context.Background(), NotificationDeliveryRequest{NotificationID: "n1"}, errors.New("boom")), "failed to schedule retry")

		finalErr := errors.New("final")
		p.sqsClient = nil
		p.deadLetterQueueURL = "https://example.com/dlq"
		require.Equal(t, finalErr, p.sendToDeadLetterQueue(context.Background(), NotificationDeliveryRequest{NotificationID: "n1"}, finalErr))

		p.sqsClient = &fakeSQSSender{err: errors.New("boom")}
		require.Equal(t, finalErr, p.sendToDeadLetterQueue(context.Background(), NotificationDeliveryRequest{NotificationID: "n1"}, finalErr))
	})

	t.Run("checkNotificationBudget branches", func(t *testing.T) {
		p.notificationCostRepo = &fakeNotificationCostRepo{budgetErr: errors.New("boom")}
		ok, err := p.checkNotificationBudget(context.Background(), "u1", 10)
		require.NoError(t, err)
		require.True(t, ok)

		p.notificationCostRepo = &fakeNotificationCostRepo{budget: nil, dailyErr: errors.New("boom")}
		ok, err = p.checkNotificationBudget(context.Background(), "u1", 10)
		require.NoError(t, err)
		require.True(t, ok)

		p.notificationCostRepo = &fakeNotificationCostRepo{budget: nil, dailySpending: 990}
		ok, err = p.checkNotificationBudget(context.Background(), "u1", 20)
		require.NoError(t, err)
		require.False(t, ok)

		p.notificationCostRepo = &fakeNotificationCostRepo{budget: &models.NotificationBudget{Enabled: false}}
		ok, err = p.checkNotificationBudget(context.Background(), "u1", 10)
		require.NoError(t, err)
		require.True(t, ok)

		p.notificationCostRepo = &fakeNotificationCostRepo{budget: &models.NotificationBudget{Enabled: true, BudgetExceeded: true, LimitMicroCents: 100, SpentMicroCents: 100}}
		ok, err = p.checkNotificationBudget(context.Background(), "u1", 10)
		require.NoError(t, err)
		require.False(t, ok)

		p.notificationCostRepo = &fakeNotificationCostRepo{budget: &models.NotificationBudget{Enabled: true, LimitMicroCents: 100, SpentMicroCents: 95}}
		ok, err = p.checkNotificationBudget(context.Background(), "u1", 10)
		require.NoError(t, err)
		require.False(t, ok)

		p.notificationCostRepo = &fakeNotificationCostRepo{budget: &models.NotificationBudget{Enabled: true, LimitMicroCents: 100, SpentMicroCents: 10}}
		ok, err = p.checkNotificationBudget(context.Background(), "u1", 10)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("requeueScheduledNotification guards", func(t *testing.T) {
		p.sqsClient = nil
		require.ErrorContains(t, p.requeueScheduledNotification(context.Background(), NotificationDeliveryRequest{}), "SQS client not initialized")

		p.sqsClient = &fakeSQSSender{}
		p.retryQueueURL = ""
		require.ErrorContains(t, p.requeueScheduledNotification(context.Background(), NotificationDeliveryRequest{}), "retry queue URL not configured")

		p.retryQueueURL = "https://example.com/retry"
		past := time.Now().Add(-1 * time.Second)
		require.NoError(t, p.requeueScheduledNotification(context.Background(), NotificationDeliveryRequest{ScheduledAt: &past}))
	})

	t.Run("calculateRetryDelay jitter branches", func(t *testing.T) {
		origRandRead := randReadFn
		t.Cleanup(func() { randReadFn = origRandRead })

		// No jitter + cap at max delay.
		policy := &RetryPolicy{InitialDelay: time.Second, MaxDelay: 5 * time.Second, BackoffFactor: 2.0, JitterPercent: 0}
		require.Equal(t, 5*time.Second, p.calculateRetryDelay(10, policy))

		// Jitter read failure returns base delay.
		randReadFn = func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
		policy.JitterPercent = 0.1
		require.Equal(t, time.Second, p.calculateRetryDelay(0, policy))

		// Jitter can push delay negative when percent > 1.
		randReadFn = func(buf []byte) (int, error) {
			for i := range buf {
				buf[i] = 0
			}
			return len(buf), nil
		}
		policy.JitterPercent = 2.0
		require.Equal(t, policy.InitialDelay, p.calculateRetryDelay(0, policy))
	})

	t.Run("isRetryableError branches", func(t *testing.T) {
		require.False(t, p.isRetryableError(nil))
		require.False(t, p.isRetryableError(errors.New("budget exceeded")))
		require.True(t, p.isRetryableError(errors.New("timeout")))
		require.True(t, p.isRetryableError(errors.New("unknown")))
	})
}
