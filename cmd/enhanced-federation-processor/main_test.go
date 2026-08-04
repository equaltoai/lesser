package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/equaltoai/lesser/pkg/activitypub"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

type fakeEnhancedRetryProcessor struct {
	calls int
	last  *federation.EnhancedRetryMessage
	err   error
}

func (f *fakeEnhancedRetryProcessor) ProcessEnhancedRetry(_ context.Context, msg *federation.EnhancedRetryMessage) error {
	f.calls++
	f.last = msg
	return f.err
}

func TestHandler_processMessage(t *testing.T) {
	processor := &fakeEnhancedRetryProcessor{}
	h := &Handler{
		retryProcessor: processor,
		logger:         zap.NewNop(),
	}

	t.Run("skips unknown delivery type", func(t *testing.T) {
		record := events.SQSMessage{
			MessageId: "m1",
			MessageAttributes: map[string]events.SQSMessageAttribute{
				"delivery_type": {StringValue: ptr("other")},
			},
			Body: "{}",
		}
		require.NoError(t, h.processMessage(context.Background(), record))
		require.Equal(t, 0, processor.calls)
	})

	t.Run("bad json returns error", func(t *testing.T) {
		record := events.SQSMessage{
			MessageId: "m2",
			MessageAttributes: map[string]events.SQSMessageAttribute{
				"delivery_type": {StringValue: ptr("enhanced_retry")},
			},
			Body: `{"delivery_id":`,
		}
		require.Error(t, h.processMessage(context.Background(), record))
	})

	t.Run("processor error is wrapped", func(t *testing.T) {
		processor.err = errors.New("boom")
		msg := federation.EnhancedRetryMessage{
			DeliveryID:   "d1",
			Activity:     &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1"}},
			ActivityType: "Create",
		}
		body, err := json.Marshal(msg)
		require.NoError(t, err)

		record := events.SQSMessage{
			MessageId: "m3",
			MessageAttributes: map[string]events.SQSMessageAttribute{
				"delivery_type": {StringValue: ptr("enhanced_retry")},
			},
			Body: string(body),
		}
		require.Error(t, h.processMessage(context.Background(), record))
		require.Equal(t, 1, processor.calls)
		processor.err = nil
	})

	t.Run("success calls processor", func(t *testing.T) {
		msg := federation.EnhancedRetryMessage{
			DeliveryID:   "d2",
			Activity:     &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a2"}},
			ActivityType: "Update",
			RetryCount:   2,
		}
		body, err := json.Marshal(msg)
		require.NoError(t, err)

		record := events.SQSMessage{
			MessageId: "m4",
			MessageAttributes: map[string]events.SQSMessageAttribute{
				"delivery_type": {StringValue: ptr("enhanced_retry")},
			},
			Body: string(body),
		}

		require.NoError(t, h.processMessage(context.Background(), record))
		require.Equal(t, 2, processor.calls)
		require.NotNil(t, processor.last)
		require.Equal(t, "d2", processor.last.DeliveryID)
		require.Equal(t, "a2", processor.last.Activity.ID)
	})
}

func TestHandler_HandleSQSEvent_ContinuesOnError(t *testing.T) {
	processor := &fakeEnhancedRetryProcessor{}
	h := &Handler{
		retryProcessor: processor,
		logger:         zap.NewNop(),
	}

	okMsg := federation.EnhancedRetryMessage{
		DeliveryID:   "d1",
		Activity:     &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1"}},
		ActivityType: "Create",
	}
	okBody, err := json.Marshal(okMsg)
	require.NoError(t, err)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "bad",
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"delivery_type": {StringValue: ptr("enhanced_retry")},
				},
				Body: `{"delivery_id":`,
			},
			{
				MessageId: "good",
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"delivery_type": {StringValue: ptr("enhanced_retry")},
				},
				Body: string(okBody),
			},
		},
	}

	require.NoError(t, h.HandleSQSEvent(context.Background(), event))
	require.Equal(t, 1, processor.calls)
}

func ptr(s string) *string { return &s }

func TestNewHandler_Round12(t *testing.T) {
	origDB := newLambdaOptimizedClientFn
	origSQS := newSQSClientFn
	origStorage := newFederationStorageFn
	origDelivery := newDeliveryServiceFn
	origRetry := newEnhancedRetryProcessorFn
	t.Cleanup(func() {
		newLambdaOptimizedClientFn = origDB
		newSQSClientFn = origSQS
		newFederationStorageFn = origStorage
		newDeliveryServiceFn = origDelivery
		newEnhancedRetryProcessorFn = origRetry
	})

	processor := &fakeEnhancedRetryProcessor{}
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return nil, nil }
	newSQSClientFn = func(aws.Config, ...func(*sqs.Options)) *sqs.Client { return &sqs.Client{} }
	newFederationStorageFn = func(dynamormCore.DB, string, string, *zap.Logger) *federation.DynamORMFederationStorage { return nil }
	newDeliveryServiceFn = func(federation.FederationStorage, *config.Config) *federation.DeliveryService {
		return &federation.DeliveryService{}
	}
	newEnhancedRetryProcessorFn = func(*federation.DeliveryService, *sqs.Client, string) enhancedRetryProcessor { return processor }

	lambdaCtx := &common.LambdaContext{
		Config: &config.Config{
			Region:                "us-east-1",
			DynamoTableName:       "table",
			Domain:                "example.com",
			EnhancedRetryQueueURL: "https://example.com/queue",
		},
		Logger:      zap.NewNop(),
		AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
	}

	h, err := NewHandler(lambdaCtx)
	require.NoError(t, err)
	require.NotNil(t, h)
	require.Equal(t, processor, h.retryProcessor)

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return nil, errors.New("boom") }
	_, err = NewHandler(lambdaCtx)
	require.Error(t, err)
}

func TestInitializeAndMainAndPanicRecovery_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origStart := lambdaStartFn
	origDB := newLambdaOptimizedClientFn
	origSQS := newSQSClientFn
	origStorage := newFederationStorageFn
	origDelivery := newDeliveryServiceFn
	origRetry := newEnhancedRetryProcessorFn
	origLambdaCtx := lambdaCtx
	origHandler := handler
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		lambdaStartFn = origStart
		newLambdaOptimizedClientFn = origDB
		newSQSClientFn = origSQS
		newFederationStorageFn = origStorage
		newDeliveryServiceFn = origDelivery
		newEnhancedRetryProcessorFn = origRetry
		lambdaCtx = origLambdaCtx
		handler = origHandler
	})

	processor := &fakeEnhancedRetryProcessor{}
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return nil, nil }
	newSQSClientFn = func(aws.Config, ...func(*sqs.Options)) *sqs.Client { return &sqs.Client{} }
	newFederationStorageFn = func(dynamormCore.DB, string, string, *zap.Logger) *federation.DynamORMFederationStorage { return nil }
	newDeliveryServiceFn = func(federation.FederationStorage, *config.Config) *federation.DeliveryService {
		return &federation.DeliveryService{}
	}
	newEnhancedRetryProcessorFn = func(*federation.DeliveryService, *sqs.Client, string) enhancedRetryProcessor { return processor }

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Region:                "us-east-1",
				DynamoTableName:       "table",
				Domain:                "example.com",
				EnhancedRetryQueueURL: "https://example.com/queue",
			},
			Logger:      zap.NewNop(),
			AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		}
	}

	require.NoError(t, initializeEnhancedFederationProcessor())
	require.NotNil(t, lambdaCtx)
	require.NotNil(t, handler)

	called := false
	lambdaStartFn = func(any) { called = true }
	main()
	require.True(t, called)

	okMsg := federation.EnhancedRetryMessage{
		DeliveryID:   "d1",
		Activity:     &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1"}},
		ActivityType: "Create",
	}
	okBody, err := json.Marshal(okMsg)
	require.NoError(t, err)

	require.NoError(t, handleEnhancedFederationSQSEvent(context.Background(), events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "good",
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"delivery_type": {StringValue: ptr("enhanced_retry")},
				},
				Body: string(okBody),
			},
		},
	}))

	panicProcessor := &fakeEnhancedRetryProcessor{}
	panicProcessor.err = nil
	handler = &Handler{
		retryProcessor: enhancedRetryProcessorFunc(func(context.Context, *federation.EnhancedRetryMessage) error {
			panic("boom")
		}),
		logger: zap.NewNop(),
	}
	lambdaCtx = &common.LambdaContext{Logger: zap.NewNop()}
	require.Error(t, handleEnhancedFederationSQSEvent(context.Background(), events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "panic",
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"delivery_type": {StringValue: ptr("enhanced_retry")},
				},
				Body: string(okBody),
			},
		},
	}))
}

type enhancedRetryProcessorFunc func(ctx context.Context, msg *federation.EnhancedRetryMessage) error

func (f enhancedRetryProcessorFunc) ProcessEnhancedRetry(ctx context.Context, msg *federation.EnhancedRetryMessage) error {
	return f(ctx, msg)
}
