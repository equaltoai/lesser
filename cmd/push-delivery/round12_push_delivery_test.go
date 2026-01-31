package main

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/pay-theory/dynamorm"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (rt roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

type fakePushSubscriptionRepo struct {
	subscriptions    []*storage.PushSubscription
	subscriptionsErr error

	vapidKeys *storage.VAPIDKeys
	vapidErr  error

	deleteCalls []struct {
		username string
		id       string
	}
	deleteErr error
}

func (f *fakePushSubscriptionRepo) GetUserPushSubscriptions(_ context.Context, _ string) ([]*storage.PushSubscription, error) {
	if f.subscriptionsErr != nil {
		return nil, f.subscriptionsErr
	}
	return f.subscriptions, nil
}

func (f *fakePushSubscriptionRepo) GetVAPIDKeys(_ context.Context) (*storage.VAPIDKeys, error) {
	if f.vapidErr != nil {
		return nil, f.vapidErr
	}
	return f.vapidKeys, nil
}

func (f *fakePushSubscriptionRepo) DeletePushSubscription(_ context.Context, username, subscriptionID string) error {
	f.deleteCalls = append(f.deleteCalls, struct {
		username string
		id       string
	}{username: username, id: subscriptionID})
	return f.deleteErr
}

type fakeActivityRepo struct {
	recordErr error
	calls     []string
}

func (f *fakeActivityRepo) RecordActivity(_ context.Context, activityType string, _ string, _ time.Time) error {
	f.calls = append(f.calls, activityType)
	return f.recordErr
}

type fakeRepos struct {
	pushRepo     *fakePushSubscriptionRepo
	activityRepo *fakeActivityRepo
}

func (r *fakeRepos) PushSubscription() pushSubscriptionRepository {
	return r.pushRepo
}

func (r *fakeRepos) Activity() activityRepository {
	return r.activityRepo
}

func mustGenerateClientKeys(t *testing.T) (string, string) {
	t.Helper()

	key, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)

	auth := make([]byte, 16)
	_, err = rand.Read(auth)
	require.NoError(t, err)

	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(auth)
}

func mustGenerateVAPIDKeys(t *testing.T) *storage.VAPIDKeys {
	t.Helper()

	priv := make([]byte, 32)
	priv[31] = 1

	return &storage.VAPIDKeys{
		Subject:    "https://example.com",
		PublicKey:  "public-key",
		PrivateKey: base64.RawURLEncoding.EncodeToString(priv),
	}
}

func TestPushDeliveryErrors_Wrappers(t *testing.T) {
	base := errors.New("boom")
	require.Error(t, ErrProcessorInitialization(base))
	require.Error(t, ErrDynamoDBInit(base))
	require.Error(t, ErrRepositoryFactory(base))
	require.Error(t, ErrInvalidMessageFormat(base))
	require.Error(t, ErrGetPushSubscriptions(base))
	require.Error(t, ErrGetVAPIDKeys(base))
	require.Error(t, ErrMarshalPayload(base))
	require.Error(t, ErrEncryptPayload(base))
	require.Error(t, ErrCreateVAPIDJWT(base))
	require.Error(t, ErrCreateRequest(base))
	require.Error(t, ErrSendRequest(base))
	require.Error(t, ErrPushServiceError())
	require.Error(t, ErrParseEndpoint(base))
	require.Error(t, ErrDecodePrivateKey(base))
	require.Error(t, ErrSign(base))
	require.Error(t, ErrDecodeP256dh(base))
	require.Error(t, ErrDecodeAuth(base))
	require.Error(t, ErrGenerateServerKey(base))
	require.Error(t, ErrConvertToECDH(base))
	require.Error(t, ErrParseClientPublicKey(base))
	require.Error(t, ErrPerformECDH(base))
	require.Error(t, ErrGenerateSalt(base))
	require.Error(t, ErrGenerateNonce(base))
	require.Error(t, ErrCreateCipher(base))
	require.Error(t, ErrCreateGCM(base))
}

func TestRateLimiter_AllowAndReset(t *testing.T) {
	rl := &RateLimiter{limits: make(map[string]*userLimit)}
	require.True(t, rl.Allow("user"))

	rl.limits["user"] = &userLimit{count: 99, resetTime: time.Now().Add(time.Hour)}
	require.True(t, rl.Allow("user"))
	require.False(t, rl.Allow("user"))

	rl.limits["user"] = &userLimit{count: 100, resetTime: time.Now().Add(time.Hour)}
	require.False(t, rl.Allow("user"))

	rl.limits["user"] = &userLimit{count: 100, resetTime: time.Now().Add(-time.Hour)}
	require.True(t, rl.Allow("user"))
	require.Equal(t, 1, rl.limits["user"].count)
}

func TestNotificationHelpers(t *testing.T) {
	alerts := storage.PushSubscriptionAlerts{
		Follow:        true,
		Favourite:     true,
		Reblog:        true,
		Mention:       true,
		Poll:          true,
		FollowRequest: true,
		Status:        true,
		Update:        true,
		AdminSignUp:   true,
		AdminReport:   true,
	}

	require.True(t, shouldSendNotification(alerts, "follow"))
	require.True(t, shouldSendNotification(alerts, "favourite"))
	require.True(t, shouldSendNotification(alerts, "reblog"))
	require.True(t, shouldSendNotification(alerts, "mention"))
	require.True(t, shouldSendNotification(alerts, "poll"))
	require.True(t, shouldSendNotification(alerts, "follow_request"))
	require.True(t, shouldSendNotification(alerts, "status"))
	require.True(t, shouldSendNotification(alerts, "update"))
	require.True(t, shouldSendNotification(alerts, "admin.sign_up"))
	require.True(t, shouldSendNotification(alerts, "admin.report"))
	require.False(t, shouldSendNotification(alerts, "unknown"))

	require.False(t, hasDeliveryFailures([]DeliveryResult{{Status: PushStatusDelivered}}))
	require.True(t, hasDeliveryFailures([]DeliveryResult{{Status: PushStatusFailed}}))
}

func TestCreateVAPIDJWT_ErrorsAndSuccess(t *testing.T) {
	pdp := &PushDeliveryProcessor{}

	_, err := pdp.createVAPIDJWT("http://[::1", "https://example.com", "key")
	require.Error(t, err)

	_, err = pdp.createVAPIDJWT("https://example.com/push", "https://example.com", "!!!")
	require.Error(t, err)

	priv := make([]byte, 32)
	priv[31] = 1
	token, err := pdp.createVAPIDJWT("https://example.com/push", "https://example.com", base64.RawURLEncoding.EncodeToString(priv))
	require.NoError(t, err)
	require.Len(t, strings.Split(token, "."), 3)
}

func TestEncryptPayload_ErrorsAndSuccess(t *testing.T) {
	pdp := &PushDeliveryProcessor{}
	payload := []byte("hello")

	_, _, _, err := pdp.encryptPayload(payload, "%%%INVALID", "auth")
	require.Error(t, err)

	p256dh, _ := mustGenerateClientKeys(t)
	_, _, _, err = pdp.encryptPayload(payload, p256dh, "%%%INVALID")
	require.Error(t, err)

	auth := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 16))
	_, _, _, err = pdp.encryptPayload(payload, base64.RawURLEncoding.EncodeToString([]byte("short")), auth)
	require.Error(t, err)

	p256dh, auth = mustGenerateClientKeys(t)
	encrypted, salt, pub, err := pdp.encryptPayload(payload, p256dh, auth)
	require.NoError(t, err)
	require.NotEmpty(t, encrypted)
	require.NotEmpty(t, salt)
	require.NotEmpty(t, pub)
}

func TestSendWebPush_ResponsesAndErrors(t *testing.T) {
	p256dh, auth := mustGenerateClientKeys(t)
	vapid := mustGenerateVAPIDKeys(t)

	pushRepo := &fakePushSubscriptionRepo{vapidKeys: vapid}
	repos := &fakeRepos{pushRepo: pushRepo, activityRepo: &fakeActivityRepo{}}

	pdp := &PushDeliveryProcessor{
		repos:  repos,
		logger: zap.NewNop(),
		cfg:    &config.Config{Domain: "example.com"},
		httpClient: &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return nil, errors.New("network")
			}),
		},
	}

	msg := &PushMessage{
		Username:         "admin",
		NotificationType: "mention",
		Title:            "hello",
		Body:             "world",
		NotificationID:   "n1",
		AccessToken:      "token",
	}

	subscription := &storage.PushSubscription{
		Username: "admin",
		ID:       "sub1",
		Endpoint: "https://push.example.com/endpoint",
		P256dh:   "bad",
		Auth:     "bad",
	}
	result := pdp.sendWebPush(context.Background(), subscription, msg, vapid)
	require.Equal(t, PushStatusFailed, result.Status)
	require.Error(t, result.Error)

	subscription.P256dh = p256dh
	subscription.Auth = auth
	badVapid := &storage.VAPIDKeys{Subject: "https://example.com", PublicKey: "pub", PrivateKey: "!!!"}
	result = pdp.sendWebPush(context.Background(), subscription, msg, badVapid)
	require.Equal(t, PushStatusFailed, result.Status)
	require.Error(t, result.Error)

	pdp.httpClient.Transport = roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 410,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})
	result = pdp.sendWebPush(context.Background(), subscription, msg, vapid)
	require.Equal(t, PushStatusSubscriptionInvalid, result.Status)
	require.Equal(t, 410, result.StatusCode)
	require.Error(t, result.Error)
	require.Len(t, pushRepo.deleteCalls, 1)

	pushRepo.deleteErr = errors.New("delete failed")
	pdp.httpClient.Transport = roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})
	result = pdp.sendWebPush(context.Background(), subscription, msg, vapid)
	require.Equal(t, PushStatusSubscriptionInvalid, result.Status)
	require.Len(t, pushRepo.deleteCalls, 2)

	pdp.httpClient.Transport = roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})
	result = pdp.sendWebPush(context.Background(), subscription, msg, vapid)
	require.Equal(t, PushStatusFailed, result.Status)
	require.Equal(t, 500, result.StatusCode)
	require.Error(t, result.Error)

	pdp.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "aes128gcm", req.Header.Get("Content-Encoding"))
		require.NotEmpty(t, req.Header.Get("Authorization"))
		require.NotEmpty(t, req.Header.Get("Crypto-Key"))
		require.NotEmpty(t, req.Header.Get("Encryption"))
		return &http.Response{
			StatusCode: 201,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})
	result = pdp.sendWebPush(context.Background(), subscription, msg, vapid)
	require.Equal(t, PushStatusDelivered, result.Status)
	require.Equal(t, 201, result.StatusCode)
	require.NoError(t, result.Error)
}

func TestProcessMessage_Branches(t *testing.T) {
	p256dh, auth := mustGenerateClientKeys(t)
	vapid := mustGenerateVAPIDKeys(t)

	pushRepo := &fakePushSubscriptionRepo{
		vapidKeys: vapid,
		subscriptions: []*storage.PushSubscription{
			{
				Username: "admin",
				ID:       "sub1",
				Endpoint: "https://push.example.com/endpoint",
				P256dh:   p256dh,
				Auth:     auth,
				Alerts: storage.PushSubscriptionAlerts{
					Mention: false,
				},
			},
		},
	}
	activityRepo := &fakeActivityRepo{recordErr: errors.New("write failed")}

	pdp := &PushDeliveryProcessor{
		repos:       &fakeRepos{pushRepo: pushRepo, activityRepo: activityRepo},
		logger:      zap.NewNop(),
		cfg:         &config.Config{Domain: "example.com"},
		rateLimiter: &RateLimiter{limits: make(map[string]*userLimit)},
		httpClient:  &http.Client{Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) { return nil, errors.New("network") })},
	}

	ctx := context.Background()

	require.Error(t, pdp.processMessage(ctx, events.SQSMessage{Body: "not-json"}))

	body := `{"username":"admin","notification_type":"mention","title":"t","body":"b","notification_id":"n1","access_token":"tok"}`
	require.NoError(t, pdp.processMessage(ctx, events.SQSMessage{Body: body, MessageId: "m1"}))

	originalSubs := pushRepo.subscriptions

	pushRepo.subscriptions = []*storage.PushSubscription{}
	require.NoError(t, pdp.processMessage(ctx, events.SQSMessage{Body: body, MessageId: "m2"}))
	pushRepo.subscriptions = originalSubs

	pushRepo.subscriptionsErr = errors.New("query failed")
	require.Error(t, pdp.processMessage(ctx, events.SQSMessage{Body: body, MessageId: "m3"}))
	pushRepo.subscriptionsErr = nil

	pushRepo.vapidErr = errors.New("vapid failed")
	require.Error(t, pdp.processMessage(ctx, events.SQSMessage{Body: body, MessageId: "m4"}))
	pushRepo.vapidErr = nil

	pushRepo.subscriptions = []*storage.PushSubscription{
		{
			Username: "admin",
			ID:       "sub2",
			Endpoint: "https://push.example.com/endpoint",
			P256dh:   "bad",
			Auth:     "bad",
			Alerts: storage.PushSubscriptionAlerts{
				Mention: true,
			},
		},
	}
	require.NoError(t, pdp.processMessage(ctx, events.SQSMessage{Body: body, MessageId: "m4b"}))

	pdp.rateLimiter.limits["admin"] = &userLimit{count: 100, resetTime: time.Now().Add(time.Hour)}
	require.NoError(t, pdp.processMessage(ctx, events.SQSMessage{Body: body, MessageId: "m5"}))
}

func TestHandleSQSMessage_ReturnsErrorOnInvalidMessage(t *testing.T) {
	pdp := &PushDeliveryProcessor{logger: zap.NewNop()}
	require.Error(t, pdp.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1", Body: "not-json"}))
}

func TestHandleSQSMessage_RecoversPanic(t *testing.T) {
	pdp := &PushDeliveryProcessor{logger: zap.NewNop()}
	// nil rateLimiter forces a panic in processMessage after JSON is parsed.
	body := `{"username":"admin","notification_type":"mention","title":"t","body":"b","notification_id":"n1","access_token":"tok"}`
	require.Error(t, pdp.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1", Body: body}))
}

func TestNewPushDeliveryProcessor_MainAndRecover(t *testing.T) {
	originalMustInitializeLambdaFn := mustInitializeLambdaFn
	originalGetDynamoClientFn := getDynamoClientFn
	originalNewRepositoryFactoryFn := newRepositoryFactoryFn
	originalLambdaStartFn := lambdaStartFn
	originalNewProcessorFn := newPushDeliveryProcessorFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInitializeLambdaFn
		getDynamoClientFn = originalGetDynamoClientFn
		newRepositoryFactoryFn = originalNewRepositoryFactoryFn
		lambdaStartFn = originalLambdaStartFn
		newPushDeliveryProcessorFn = originalNewProcessorFn
	})

	ctx := &common.LambdaContext{
		Config: &config.Config{Domain: "example.com"},
		Logger: zap.NewNop(),
	}
	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext { return ctx }
	getDynamoClientFn = func(context.Context) (dynamormCore.DB, error) { return &dynamorm.DB{}, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, tableName string, _ *zap.Logger) (storagecore.RepositoryStorage, error) {
		require.Equal(t, "lesser-main", tableName)
		return mocks.NewMockRepositoryStorage(), nil
	}

	processor, err := NewPushDeliveryProcessor()
	require.NoError(t, err)
	require.NotNil(t, processor.repos.PushSubscription())
	require.NotNil(t, processor.repos.Activity())
	require.NotNil(t, ctx.DynamoDB)
	require.NotNil(t, ctx.Repos)

	getDynamoClientFn = func(context.Context) (dynamormCore.DB, error) { return nil, errors.New("boom") }
	_, err = NewPushDeliveryProcessor()
	require.Error(t, err)

	getDynamoClientFn = func(context.Context) (dynamormCore.DB, error) { return &dynamorm.DB{}, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, _ string, _ *zap.Logger) (storagecore.RepositoryStorage, error) {
		return nil, errors.New("boom")
	}
	_, err = NewPushDeliveryProcessor()
	require.Error(t, err)

	pushRepo := &fakePushSubscriptionRepo{}
	repos := &fakeRepos{pushRepo: pushRepo, activityRepo: &fakeActivityRepo{}}

	testProcessor := &PushDeliveryProcessor{
		repos:       repos,
		logger:      zap.NewNop(),
		rateLimiter: &RateLimiter{limits: make(map[string]*userLimit)},
	}
	newPushDeliveryProcessorFn = func() (*PushDeliveryProcessor, error) { return testProcessor, nil }

	var startHandler any
	lambdaStartFn = func(h any) { startHandler = h }

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
				Body:           `{"username":"admin","notification_type":"mention","title":"t","body":"b","notification_id":"n1","access_token":"tok"}`,
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-dev-push-delivery-queue",
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
	require.Empty(t, resp.BatchItemFailures)

	testProcessor.rateLimiter = nil
	respAny, err = handlerFn(context.Background(), raw)
	require.NoError(t, err)
	resp, ok = respAny.(events.SQSEventResponse)
	require.True(t, ok)
	require.Len(t, resp.BatchItemFailures, 1)
	require.Equal(t, "m1", resp.BatchItemFailures[0].ItemIdentifier)

	newPushDeliveryProcessorFn = func() (*PushDeliveryProcessor, error) { return nil, errors.New("boom") }
	require.Panics(t, func() { main() })
}
