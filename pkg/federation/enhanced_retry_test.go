package federation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestEnhancedRetryProcessor_QueueForEnhancedRetry_SendsSQSMessage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	deliverySvc := &DeliveryService{logger: logger}
	sqsStub := &sqsClientStub{}
	processor := NewEnhancedRetryProcessor(deliverySvc, sqsStub, "https://sqs.example/queue")

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "act-1",
			Type: activitypub.CreateType,
		},
	}
	actor := &activitypub.Actor{PreferredUsername: "alice"}

	require.NoError(t, processor.QueueForEnhancedRetry(ctx, activity, actor, []string{"bob@remote.example"}, activitypub.CreateType))
	require.Len(t, sqsStub.calls, 1)

	call := sqsStub.calls[0]
	assert.GreaterOrEqual(t, call.DelaySeconds, int32(16))
	assert.LessOrEqual(t, call.DelaySeconds, int32(20))

	require.NotNil(t, call.MessageAttributes["delivery_type"].StringValue)
	assert.Equal(t, "enhanced_retry", *call.MessageAttributes["delivery_type"].StringValue)
	require.NotNil(t, call.MessageAttributes["activity_type"].StringValue)
	assert.Equal(t, activitypub.CreateType, *call.MessageAttributes["activity_type"].StringValue)
	require.NotNil(t, call.MessageAttributes["retry_count"].StringValue)
	assert.Equal(t, "1", *call.MessageAttributes["retry_count"].StringValue)

	require.NotNil(t, call.MessageBody)
	var msg EnhancedRetryMessage
	require.NoError(t, json.Unmarshal([]byte(*call.MessageBody), &msg))
	assert.Equal(t, "alice", msg.SigningActorID)
	assert.Equal(t, 0, msg.RetryCount)
	assert.Equal(t, 25, msg.MaxRetries)
	assert.Equal(t, "polynomial", msg.RetryPolicy)
	assert.NotEmpty(t, msg.DeliveryID)
}

func TestEnhancedRetryProcessor_QueueForEnhancedRetry_MarshalError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	processor := NewEnhancedRetryProcessor(&DeliveryService{logger: logger}, nil, "")

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType},
		Object:     make(chan int),
	}

	err := processor.QueueForEnhancedRetry(ctx, activity, &activitypub.Actor{PreferredUsername: "alice"}, nil, activitypub.CreateType)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRetryMessageMarshalFailed)
}

func TestEnhancedRetryProcessor_ProcessEnhancedRetry_AllSuccess_RecordsFinalSuccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	privateKeyPEM, err := EncodePrivateKeyPEM(privateKey)
	require.NoError(t, err)

	signingActor := &activitypub.Actor{
		BaseObject:         activitypub.BaseObject{ID: "https://local.example/users/alice"},
		PreferredUsername:  "alice",
		PublicKey:          &activitypub.PublicKey{ID: "https://local.example/users/alice#main-key"},
		ManuallyApprovesFollowers: false,
	}

	final := make(chan *storage.FederationActivity, 1)
	store := &federationStoreStub{
		getActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) {
			return signingActor, nil
		},
		getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) {
			return string(privateKeyPEM), nil
		},
		recordActivityFn: func(_ context.Context, activity *storage.FederationActivity) error {
			if activity.Status == "delivered_with_retry" {
				final <- activity
			}
			return nil
		},
	}

	httpStub := &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	}}

	deliverySvc := &DeliveryService{
		store:      store,
		httpClient: httpStub,
		logger:     logger,
	}

	processor := NewEnhancedRetryProcessor(deliverySvc, nil, "")

	msg := &EnhancedRetryMessage{
		DeliveryID:        "deliv-1",
		Activity:          &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType}},
		SigningActorID:    "alice",
		ActivityType:      activitypub.CreateType,
		RetryCount:        1,
		MaxRetries:        25,
		RetryPolicy:       "polynomial",
		MaxRetryDuration:  20 * 24 * time.Hour,
		CreatedAt:         time.Now().Add(-1 * time.Hour),
		TargetInboxes:     []string{"https://remote.example/inbox"},
		FailedInboxes:     map[string]string{},
		SuccessfulInboxes: []string{},
	}

	require.NoError(t, processor.ProcessEnhancedRetry(ctx, msg))

	select {
	case activity := <-final:
		assert.True(t, activity.Success)
		assert.Equal(t, "delivered_with_retry", activity.Status)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for final success record")
	}
}

func TestEnhancedRetryProcessor_ProcessEnhancedRetry_PartialFailure_Requeues(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	privateKeyPEM, err := EncodePrivateKeyPEM(privateKey)
	require.NoError(t, err)

	signingActor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://local.example/users/alice"},
		PreferredUsername: "alice",
		PublicKey:         &activitypub.PublicKey{ID: "https://local.example/users/alice#main-key"},
	}

	store := &federationStoreStub{
		getActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) {
			return signingActor, nil
		},
		getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) {
			return string(privateKeyPEM), nil
		},
	}

	httpStub := &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.String(), "fail") {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("nope")),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	}}

	deliverySvc := &DeliveryService{
		store:      store,
		httpClient: httpStub,
		logger:     logger,
	}

	sqsStub := &sqsClientStub{}
	processor := NewEnhancedRetryProcessor(deliverySvc, sqsStub, "https://sqs.example/queue")

	msg := &EnhancedRetryMessage{
		DeliveryID:        "deliv-1",
		Activity:          &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType}},
		SigningActorID:    "alice",
		ActivityType:      activitypub.CreateType,
		RetryCount:        1,
		MaxRetries:        25,
		MaxRetryDuration:  20 * 24 * time.Hour,
		CreatedAt:         time.Now().Add(-1 * time.Hour),
		TargetInboxes:     []string{"https://remote.example/inbox", "https://remote.example/fail"},
		FailedInboxes:     map[string]string{},
		SuccessfulInboxes: []string{"https://remote.example/inbox"},
	}

	require.NoError(t, processor.ProcessEnhancedRetry(ctx, msg))

	assert.Equal(t, 2, msg.RetryCount)
	require.Len(t, sqsStub.calls, 1)
}

func TestEnhancedRetryProcessor_ProcessEnhancedRetry_MaxRetries_RecordsFinalFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	final := make(chan *storage.FederationActivity, 1)
	store := &federationStoreStub{
		recordActivityFn: func(_ context.Context, activity *storage.FederationActivity) error {
			if activity.Status == "failed_permanently" {
				final <- activity
			}
			return nil
		},
	}

	deliverySvc := &DeliveryService{
		store:  store,
		logger: logger,
	}
	processor := NewEnhancedRetryProcessor(deliverySvc, nil, "")

	msg := &EnhancedRetryMessage{
		DeliveryID:       "deliv-1",
		Activity:         &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType}},
		SigningActorID:   "alice",
		ActivityType:     activitypub.CreateType,
		RetryCount:       25,
		MaxRetries:       25,
		MaxRetryDuration: 20 * 24 * time.Hour,
		CreatedAt:        time.Now().Add(-1 * time.Hour),
		FailedInboxes:    map[string]string{"https://remote.example/inbox": "boom"},
	}

	require.NoError(t, processor.ProcessEnhancedRetry(ctx, msg))

	select {
	case activity := <-final:
		assert.False(t, activity.Success)
		assert.Contains(t, activity.ErrorMessage, "max_retries_exceeded")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for final failure record")
	}
}

func TestEnhancedRetryProcessor_requeueForRetry_MarshalError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	processor := NewEnhancedRetryProcessor(&DeliveryService{logger: logger}, nil, "")

	msg := &EnhancedRetryMessage{
		DeliveryID:        "deliv-1",
		Activity:          &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType}, Object: make(chan int)},
		SigningActorID:    "alice",
		ActivityType:      activitypub.CreateType,
		RetryCount:        1,
		MaxRetries:        25,
		MaxRetryDuration:  20 * 24 * time.Hour,
		CreatedAt:         time.Now().Add(-1 * time.Hour),
		FailedInboxes:     map[string]string{},
		SuccessfulInboxes: []string{},
	}

	err := processor.requeueForRetry(ctx, msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMessageMarshalFailed)
}

func TestEnhancedRetryProcessor_ProcessEnhancedRetry_SigningActorLookupFails_Requeues(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	store := &federationStoreStub{
		getActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) {
			return nil, errors.New("no actor")
		},
	}

	sqsStub := &sqsClientStub{}
	deliverySvc := &DeliveryService{store: store, logger: logger}
	processor := NewEnhancedRetryProcessor(deliverySvc, sqsStub, "https://sqs.example/queue")

	msg := &EnhancedRetryMessage{
		DeliveryID:       "deliv-1",
		Activity:         &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType}},
		SigningActorID:   "alice",
		ActivityType:     activitypub.CreateType,
		RetryCount:       1,
		MaxRetries:       25,
		MaxRetryDuration: 20 * 24 * time.Hour,
		CreatedAt:        time.Now().Add(-1 * time.Hour),
		FailedInboxes:    map[string]string{},
	}

	require.NoError(t, processor.ProcessEnhancedRetry(ctx, msg))
	require.Len(t, sqsStub.calls, 1)
}
