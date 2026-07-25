package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfigv2 "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	storagefactory "github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestAssessTargetHealth_Branches_Round12(t *testing.T) {
	originalGetStats := getInstanceStatsFn
	t.Cleanup(func() { getInstanceStatsFn = originalGetStats })

	p := &FederationDeliveryProcessor{logger: zaptest.NewLogger(t), repos: nil}

	now := time.Now()
	tests := []struct {
		name         string
		retryCount   int
		stats        *storage.InstanceStats
		statsErr     error
		expectAllow  bool
		expectReason string
	}{
		{
			name:         "no_stats",
			statsErr:     errors.New("no stats"),
			expectAllow:  true,
			expectReason: "no_stats",
		},
		{
			name:         "high_error_rate",
			stats:        &storage.InstanceStats{ErrorRate: 0.6, AvgResponseTime: 1000, LastSeen: now},
			expectAllow:  false,
			expectReason: "high_error_rate",
		},
		{
			name:         "slow_response_time",
			stats:        &storage.InstanceStats{ErrorRate: 0.1, AvgResponseTime: 35000, LastSeen: now},
			expectAllow:  false,
			expectReason: "slow_response_time",
		},
		{
			name:         "retry_error_rate",
			retryCount:   1,
			stats:        &storage.InstanceStats{ErrorRate: 0.25, AvgResponseTime: 2000, LastSeen: now},
			expectAllow:  false,
			expectReason: "retry_error_rate",
		},
		{
			name:         "retry_slow_response",
			retryCount:   1,
			stats:        &storage.InstanceStats{ErrorRate: 0.1, AvgResponseTime: 18000, LastSeen: now},
			expectAllow:  false,
			expectReason: "retry_slow_response",
		},
		{
			name:         "stale_last_seen",
			stats:        &storage.InstanceStats{ErrorRate: 0.1, AvgResponseTime: 2000, LastSeen: now.Add(-48 * time.Hour)},
			expectAllow:  false,
			expectReason: "stale_last_seen",
		},
		{
			name:         "healthy",
			stats:        &storage.InstanceStats{ErrorRate: 0.05, AvgResponseTime: 1000, LastSeen: now.Add(-time.Hour)},
			expectAllow:  true,
			expectReason: "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getInstanceStatsFn = func(context.Context, core.RepositoryStorage, string) (*storage.InstanceStats, error) {
				return tt.stats, tt.statsErr
			}

			allow, reason := p.assessTargetHealth(context.Background(), "example.com", tt.retryCount)
			require.Equal(t, tt.expectAllow, allow)
			if strings.Contains(tt.expectReason, "_") || strings.Contains(tt.expectReason, "rate") || strings.Contains(tt.expectReason, "slow") {
				require.Contains(t, reason, tt.expectReason)
			} else {
				require.Equal(t, tt.expectReason, reason)
			}
		})
	}
}

func TestProcessDeliveryMessage_Branches_Round12(t *testing.T) {
	originalGetActor := getSigningActorFn
	originalGetStats := getInstanceStatsFn
	originalRecordActivity := recordFederationActivityFn
	originalCreateObject := createObjectFn
	originalDeliver := deliverActivityFn
	originalSend := sendSQSMessageFn
	t.Cleanup(func() {
		getSigningActorFn = originalGetActor
		getInstanceStatsFn = originalGetStats
		recordFederationActivityFn = originalRecordActivity
		createObjectFn = originalCreateObject
		deliverActivityFn = originalDeliver
		sendSQSMessageFn = originalSend
	})

	p := &FederationDeliveryProcessor{
		repos:     nil,
		cfg:       &config.Config{},
		queueURL:  "https://queue.example.com/url",
		sqsClient: nil,
		logger:    zaptest.NewLogger(t),
	}

	baseMsg := FederationDeliveryMessage{
		DeliveryID:     "d1",
		Activity:       &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1", Type: "Create"}},
		TargetInbox:    "https://remote.example.com/inbox",
		SigningActorID: "alice",
		RetryCount:     0,
		MaxRetries:     3,
		CreatedAt:      time.Now().Add(-time.Minute),
	}

	t.Run("skip_not_time_to_retry", func(t *testing.T) {
		msg := baseMsg
		next := time.Now().Add(time.Hour)
		msg.NextRetryAfter = &next
		require.NoError(t, p.processDeliveryMessage(context.Background(), p.logger, msg))
	})

	t.Run("signing_actor_missing", func(t *testing.T) {
		getSigningActorFn = func(context.Context, core.RepositoryStorage, string) (*activitypub.Actor, error) {
			return nil, errors.New("not found")
		}
		msg := baseMsg
		err := p.processDeliveryMessage(context.Background(), p.logger, msg)
		require.Error(t, err)
		appErr, ok := pkgErrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, pkgErrors.CodeNotFound, appErr.Code)
	})

	t.Run("health_skip_with_delay_after_multiple_retries", func(t *testing.T) {
		getSigningActorFn = func(context.Context, core.RepositoryStorage, string) (*activitypub.Actor, error) {
			return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "alice"}}, nil
		}
		getInstanceStatsFn = func(context.Context, core.RepositoryStorage, string) (*storage.InstanceStats, error) {
			return &storage.InstanceStats{ErrorRate: 0.6, AvgResponseTime: 1000, LastSeen: time.Now()}, nil
		}

		var gotInput *sqs.SendMessageInput
		sendSQSMessageFn = func(_ context.Context, _ *sqs.Client, input *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
			gotInput = input
			return &sqs.SendMessageOutput{}, nil
		}

		msg := baseMsg
		msg.RetryCount = 2
		require.NoError(t, p.processDeliveryMessage(context.Background(), p.logger, msg))
		require.NotNil(t, gotInput)
	})

	t.Run("permanent_error_stores_status_and_returns_max_attempts", func(t *testing.T) {
		getSigningActorFn = func(context.Context, core.RepositoryStorage, string) (*activitypub.Actor, error) {
			return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "alice"}}, nil
		}
		getInstanceStatsFn = func(context.Context, core.RepositoryStorage, string) (*storage.InstanceStats, error) {
			return nil, errors.New("no stats")
		}
		deliverErr := errors.New("status 404 not found")
		deliverActivityFn = func(context.Context, *federation.DeliveryService, *activitypub.Activity, string, *activitypub.Actor) error {
			return deliverErr
		}

		var gotStatus *models.DeliveryStatus
		createObjectFn = func(_ context.Context, _ core.RepositoryStorage, obj any) error {
			status, ok := obj.(*models.DeliveryStatus)
			require.True(t, ok)
			gotStatus = status
			return nil
		}
		recordFederationActivityFn = func(context.Context, core.RepositoryStorage, *storage.FederationActivity) error {
			return nil
		}

		msg := baseMsg
		err := p.processDeliveryMessage(context.Background(), p.logger, msg)
		require.Error(t, err)
		appErr, ok := pkgErrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, pkgErrors.CodeDeliveryFailed, appErr.Code)
		require.NotNil(t, gotStatus)
		require.Equal(t, "permanently_failed", gotStatus.Status)
		require.Contains(t, gotStatus.Error, "status 404")
	})

	t.Run("temporary_error_requeues_and_returns_nil", func(t *testing.T) {
		getSigningActorFn = func(context.Context, core.RepositoryStorage, string) (*activitypub.Actor, error) {
			return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "alice"}}, nil
		}
		getInstanceStatsFn = func(context.Context, core.RepositoryStorage, string) (*storage.InstanceStats, error) {
			return nil, errors.New("no stats")
		}
		deliverActivityFn = func(context.Context, *federation.DeliveryService, *activitypub.Activity, string, *activitypub.Actor) error {
			return errors.New("status 500 internal server error")
		}

		var gotInput *sqs.SendMessageInput
		sendSQSMessageFn = func(_ context.Context, _ *sqs.Client, input *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
			gotInput = input
			return &sqs.SendMessageOutput{}, nil
		}

		createObjectFn = func(context.Context, core.RepositoryStorage, any) error { return nil }
		recordFederationActivityFn = func(context.Context, core.RepositoryStorage, *storage.FederationActivity) error { return nil }

		msg := baseMsg
		require.NoError(t, p.processDeliveryMessage(context.Background(), p.logger, msg))
		require.NotNil(t, gotInput)
		require.Equal(t, "1", aws.ToString(gotInput.MessageAttributes["retry_count"].StringValue))
	})

	t.Run("temporary_error_requeue_failure_returns_original_error", func(t *testing.T) {
		getSigningActorFn = func(context.Context, core.RepositoryStorage, string) (*activitypub.Actor, error) {
			return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "alice"}}, nil
		}
		getInstanceStatsFn = func(context.Context, core.RepositoryStorage, string) (*storage.InstanceStats, error) {
			return nil, errors.New("no stats")
		}

		deliverErr := errors.New("timeout")
		deliverActivityFn = func(context.Context, *federation.DeliveryService, *activitypub.Activity, string, *activitypub.Actor) error {
			return deliverErr
		}
		sendSQSMessageFn = func(context.Context, *sqs.Client, *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
			return nil, errors.New("send failed")
		}
		createObjectFn = func(context.Context, core.RepositoryStorage, any) error { return nil }

		msg := baseMsg
		err := p.processDeliveryMessage(context.Background(), p.logger, msg)
		require.Error(t, err)
		appErr, ok := pkgErrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, pkgErrors.CodeInternal, appErr.Code)
	})

	t.Run("success_stores_status", func(t *testing.T) {
		getSigningActorFn = func(context.Context, core.RepositoryStorage, string) (*activitypub.Actor, error) {
			return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "alice"}}, nil
		}
		getInstanceStatsFn = func(context.Context, core.RepositoryStorage, string) (*storage.InstanceStats, error) {
			return nil, errors.New("no stats")
		}
		deliverActivityFn = func(context.Context, *federation.DeliveryService, *activitypub.Activity, string, *activitypub.Actor) error {
			return nil
		}
		recordFederationActivityFn = func(context.Context, core.RepositoryStorage, *storage.FederationActivity) error { return nil }

		var gotStatus *models.DeliveryStatus
		createObjectFn = func(_ context.Context, _ core.RepositoryStorage, obj any) error {
			status, ok := obj.(*models.DeliveryStatus)
			require.True(t, ok)
			gotStatus = status
			return nil
		}

		msg := baseMsg
		require.NoError(t, p.processDeliveryMessage(context.Background(), p.logger, msg))
		require.NotNil(t, gotStatus)
		require.Equal(t, "delivered", gotStatus.Status)
	})
}

func TestHandleDeliveryMessage_ParseError_Round12(t *testing.T) {
	p := &FederationDeliveryProcessor{logger: zap.NewNop()}
	ctx := &apptheory.EventContext{RequestID: "req"}

	err := p.HandleSQSMessage(ctx, events.SQSMessage{
		MessageId: "m1",
		Body:      "not-json",
	})
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeBadRequest, appErr.Code)
}

func TestHandleDeliveryMessage_Success_Round12(t *testing.T) {
	p := &FederationDeliveryProcessor{logger: zap.NewNop()}
	ctx := &apptheory.EventContext{RequestID: "req"}

	next := time.Now().Add(time.Hour)
	msg := FederationDeliveryMessage{
		DeliveryID:     "d1",
		RetryCount:     0,
		MaxRetries:     3,
		CreatedAt:      time.Now(),
		NextRetryAfter: &next,
	}
	body, err := json.Marshal(msg)
	require.NoError(t, err)

	require.NoError(t, p.HandleSQSMessage(ctx, events.SQSMessage{
		MessageId: "m1",
		Body:      string(body),
	}))
}

func TestCalculateRetryBackoff_Cases_Round12(t *testing.T) {
	p := &FederationDeliveryProcessor{}

	base := calculateBackoff(2) // 4
	require.Equal(t, base*3, p.calculateRetryBackoff(2, "rate_limit", ""))
	require.Equal(t, base*2, p.calculateRetryBackoff(2, "server_error", ""))
	require.Equal(t, base, p.calculateRetryBackoff(2, "network", ""))
	require.Equal(t, int(float64(base)*1.5), p.calculateRetryBackoff(2, "timeout", ""))
	require.Equal(t, base, p.calculateRetryBackoff(2, "temporary", ""))
}

func TestExtractDomainFromURL_Round12(t *testing.T) {
	require.Equal(t, "example.com", extractDomainFromURL("https://example.com/inbox"))
	require.Equal(t, "example.com", extractDomainFromURL("https://example.com"))
	require.Equal(t, "unknown", extractDomainFromURL("http://example.com/inbox"))
	require.Equal(t, "unknown", extractDomainFromURL(""))
}

func TestRequeueDelivery_DelayAndMarshalBranches_Round12(t *testing.T) {
	originalSend := sendSQSMessageFn
	t.Cleanup(func() { sendSQSMessageFn = originalSend })

	p := &FederationDeliveryProcessor{
		logger:    zap.NewNop(),
		queueURL:  "https://queue.example.com/url",
		sqsClient: nil,
	}

	var gotDelay int32
	sendSQSMessageFn = func(_ context.Context, _ *sqs.Client, input *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
		gotDelay = input.DelaySeconds
		return &sqs.SendMessageOutput{}, nil
	}

	msg := &FederationDeliveryMessage{DeliveryID: "d1"}
	require.NoError(t, p.requeueDelivery(context.Background(), msg, -10))
	require.Equal(t, int32(0), gotDelay)

	require.NoError(t, p.requeueDelivery(context.Background(), msg, 1000))
	require.Equal(t, int32(900), gotDelay)

	msg.Activity = &activitypub.Activity{Object: make(chan int)}
	err := p.requeueDelivery(context.Background(), msg, 1)
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeInternal, appErr.Code)
}

func TestDeliverWithRoutingOptimization_AnalyticsFailureBranches_Round12(t *testing.T) {
	originalDeliver := deliverActivityFn
	originalRecord := recordFederationActivityFn
	t.Cleanup(func() {
		deliverActivityFn = originalDeliver
		recordFederationActivityFn = originalRecord
	})

	p := &FederationDeliveryProcessor{logger: zap.NewNop()}

	recordFederationActivityFn = func(context.Context, core.RepositoryStorage, *storage.FederationActivity) error {
		return errors.New("analytics failed")
	}

	deliverActivityFn = func(context.Context, *federation.DeliveryService, *activitypub.Activity, string, *activitypub.Actor) error {
		return errors.New("status 500")
	}
	require.Error(t, p.deliverWithRoutingOptimization(context.Background(), &activitypub.Activity{}, "https://x/inbox", &activitypub.Actor{}, "x"))

	deliverActivityFn = func(context.Context, *federation.DeliveryService, *activitypub.Activity, string, *activitypub.Actor) error {
		return nil
	}
	require.NoError(t, p.deliverWithRoutingOptimization(context.Background(), &activitypub.Activity{}, "https://x/inbox", &activitypub.Actor{}, "x"))
}

func TestClassifyDeliveryError_NilAndPatterns_Round12(t *testing.T) {
	p := &FederationDeliveryProcessor{}
	require.Equal(t, "unknown", p.classifyDeliveryError(nil))
	require.Equal(t, errorTypePermanent, p.classifyDeliveryError(errors.New("status 404")))
	require.Equal(t, errorTypeTemporary, p.classifyDeliveryError(errors.New("timeout")))
}

func TestHandleSQSMessage_ReturnsErrorOnFailure_Round12(t *testing.T) {
	ctx := &apptheory.EventContext{RequestID: "req"}

	p := &FederationDeliveryProcessor{logger: zap.NewNop()}

	next := time.Now().Add(time.Hour)
	msg := FederationDeliveryMessage{
		DeliveryID:     "d1",
		RetryCount:     0,
		MaxRetries:     3,
		CreatedAt:      time.Now(),
		NextRetryAfter: &next,
	}
	body, err := json.Marshal(msg)
	require.NoError(t, err)

	require.NoError(t, p.HandleSQSMessage(ctx, events.SQSMessage{MessageId: "m1", Body: string(body)}))

	err = p.HandleSQSMessage(ctx, events.SQSMessage{MessageId: "m2", Body: "not-json"})
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeBadRequest, appErr.Code)
}

func TestProcessDeliveryMessage_AdditionalBranches_Round12(t *testing.T) {
	originalGetActor := getSigningActorFn
	originalGetStats := getInstanceStatsFn
	originalRecordActivity := recordFederationActivityFn
	originalCreateObject := createObjectFn
	originalDeliver := deliverActivityFn
	t.Cleanup(func() {
		getSigningActorFn = originalGetActor
		getInstanceStatsFn = originalGetStats
		recordFederationActivityFn = originalRecordActivity
		createObjectFn = originalCreateObject
		deliverActivityFn = originalDeliver
	})

	p := &FederationDeliveryProcessor{
		repos:  nil,
		logger: zap.NewNop(),
	}

	getSigningActorFn = func(context.Context, core.RepositoryStorage, string) (*activitypub.Actor, error) {
		return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "alice"}}, nil
	}
	recordFederationActivityFn = func(context.Context, core.RepositoryStorage, *storage.FederationActivity) error { return nil }
	createObjectFn = func(context.Context, core.RepositoryStorage, any) error { return nil }

	t.Run("health_unhealthy_but_retryCount_lt_2_continues", func(t *testing.T) {
		getInstanceStatsFn = func(context.Context, core.RepositoryStorage, string) (*storage.InstanceStats, error) {
			return &storage.InstanceStats{ErrorRate: 0.6, AvgResponseTime: 1000, LastSeen: time.Now()}, nil
		}
		deliverActivityFn = func(context.Context, *federation.DeliveryService, *activitypub.Activity, string, *activitypub.Actor) error {
			return nil
		}

		msg := FederationDeliveryMessage{
			DeliveryID:     "d1",
			Activity:       &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1", Type: "Create"}},
			TargetInbox:    "https://remote.example.com/inbox",
			SigningActorID: "alice",
			RetryCount:     1,
			MaxRetries:     3,
			CreatedAt:      time.Now().Add(-time.Minute),
		}
		require.NoError(t, p.processDeliveryMessage(context.Background(), zap.NewNop(), msg))
	})

	t.Run("retryCount_at_max_retries_returns_max_attempts", func(t *testing.T) {
		getInstanceStatsFn = func(context.Context, core.RepositoryStorage, string) (*storage.InstanceStats, error) {
			return nil, errors.New("no stats")
		}
		deliverActivityFn = func(context.Context, *federation.DeliveryService, *activitypub.Activity, string, *activitypub.Actor) error {
			return errors.New("timeout")
		}

		msg := FederationDeliveryMessage{
			DeliveryID:     "d1",
			Activity:       &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1", Type: "Create"}},
			TargetInbox:    "https://remote.example.com/inbox",
			SigningActorID: "alice",
			RetryCount:     3,
			MaxRetries:     3,
			CreatedAt:      time.Now().Add(-time.Minute),
		}
		err := p.processDeliveryMessage(context.Background(), zap.NewNop(), msg)
		require.Error(t, err)
		appErr, ok := pkgErrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, pkgErrors.CodeDeliveryFailed, appErr.Code)
	})
}

func TestInitializeFederationDelivery_SuccessAndErrors_Round12(t *testing.T) {
	originalMustInit := mustInitializeLambdaFn
	originalInitDefaults := initializeWithDefaultsFn
	originalLoadAWS := loadAWSConfigFn
	originalNewSQS := newSQSClientFn
	originalNewDelivery := newDeliveryServiceFn

	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalProcessor := processor

	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInit
		initializeWithDefaultsFn = originalInitDefaults
		loadAWSConfigFn = originalLoadAWS
		newSQSClientFn = originalNewSQS
		newDeliveryServiceFn = originalNewDelivery

		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		processor = originalProcessor
	})

	fakeCtx := &common.LambdaContext{
		Config: &config.Config{
			Region:             "us-east-1",
			FederationQueueURL: "https://queue.example.com/url",
		},
		Logger: zap.NewNop(),
		Repos:  &storagefactory.RepositoryFactory{},
	}

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext { return fakeCtx }
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("defaults failed") }
	loadAWSConfigFn = func(context.Context, ...func(*awsconfigv2.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	newSQSClientFn = func(aws.Config, ...func(*sqs.Options)) *sqs.Client { return &sqs.Client{} }
	newDeliveryServiceFn = func(federation.FederationStorage, *config.Config) *federation.DeliveryService {
		return &federation.DeliveryService{}
	}

	require.NoError(t, initializeFederationDelivery())
	require.NotNil(t, processor)
	require.Equal(t, fakeCtx.Config.FederationQueueURL, processor.queueURL)

	loadAWSConfigFn = func(context.Context, ...func(*awsconfigv2.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("aws cfg failed")
	}
	require.Error(t, initializeFederationDelivery())

	fakeCtx.Config.FederationQueueURL = ""
	loadAWSConfigFn = func(context.Context, ...func(*awsconfigv2.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	require.Error(t, initializeFederationDelivery())
}

func TestMain_WiresHandler_Round12(t *testing.T) {
	originalLambdaStart := lambdaStartFn
	originalProcessor := processor
	t.Cleanup(func() {
		lambdaStartFn = originalLambdaStart
		processor = originalProcessor
	})

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	var gotHandler any
	lambdaStartFn = func(h any) { gotHandler = h }
	main()
	require.NotNil(t, gotHandler)

	handlerFn, ok := gotHandler.(func(context.Context, json.RawMessage) (any, error))
	require.True(t, ok)

	next := time.Now().Add(time.Hour)
	msg := FederationDeliveryMessage{
		DeliveryID:     "d1",
		RetryCount:     0,
		MaxRetries:     3,
		CreatedAt:      time.Now(),
		NextRetryAfter: &next,
	}
	body, err := json.Marshal(msg)
	require.NoError(t, err)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId:      "m1",
				ReceiptHandle:  "rh",
				Body:           string(body),
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-dev-federation-delivery-queue",
				EventSource:    "aws:sqs",
			},
		},
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)

	processor = nil
	respAny, err := handlerFn(context.Background(), raw)
	require.NoError(t, err)

	resp, ok := respAny.(events.SQSEventResponse)
	require.True(t, ok)
	require.Len(t, resp.BatchItemFailures, 1)
	require.Equal(t, "m1", resp.BatchItemFailures[0].ItemIdentifier)

	processor = &FederationDeliveryProcessor{logger: zap.NewNop()}
	respAny, err = handlerFn(context.Background(), raw)
	require.NoError(t, err)

	resp, ok = respAny.(events.SQSEventResponse)
	require.True(t, ok)
	require.Empty(t, resp.BatchItemFailures)
}
