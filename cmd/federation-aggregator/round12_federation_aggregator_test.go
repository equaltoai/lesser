package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type fakeFederationActivityRepo struct {
	recentCalls int
	listCalls   int
	infoCalls   int
	recentSince time.Time

	recent []*models.FederationActivity
	list   []*models.FederationActivity
	info   *models.InstanceInfo

	recentErr error
	listErr   error
	infoErr   error
}

func (f *fakeFederationActivityRepo) GetRecentActivities(_ context.Context, since time.Time, _ int) ([]*models.FederationActivity, error) {
	f.recentCalls++
	f.recentSince = since
	if f.recentErr != nil {
		return nil, f.recentErr
	}
	return f.recent, nil
}

func (f *fakeFederationActivityRepo) ListByDomain(context.Context, string, time.Time, time.Time, int) ([]*models.FederationActivity, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakeFederationActivityRepo) GetInstanceInfo(context.Context, string) (*models.InstanceInfo, error) {
	f.infoCalls++
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return f.info, nil
}

type fakeSQSClient struct {
	sendCalls    int
	err          error
	lastQueueURL string
}

func (f *fakeSQSClient) SendMessage(_ context.Context, input *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.sendCalls++
	if input != nil && input.QueueUrl != nil {
		f.lastQueueURL = *input.QueueUrl
	}
	if f.err != nil {
		return nil, f.err
	}
	return &sqs.SendMessageOutput{MessageId: aws.String("mid")}, nil
}

type fakeLambdaClient struct {
	invokeCalls int
	result      *awslambda.InvokeOutput
	err         error
}

func (f *fakeLambdaClient) Invoke(context.Context, *awslambda.InvokeInput, ...func(*awslambda.Options)) (*awslambda.InvokeOutput, error) {
	f.invokeCalls++
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &awslambda.InvokeOutput{StatusCode: 202}, nil
}

func TestInitializeFederationAggregator_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origNewClient := newLambdaOptimizedClientFn
	origNewProcessor := newProcessorFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		newLambdaOptimizedClientFn = origNewClient
		newProcessorFn = origNewProcessor
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{Region: "us-east-1", DynamoTableName: "table"},
			Logger: zap.NewNop(),
			AWSServices: &awsInit.AWSServices{
				Config: aws.Config{Region: "us-east-1"},
			},
		}
	}
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return dynamormmocks.NewMockExtendedDB(), nil }
	newProcessorFn = func(db dynamormCore.DB, tableName string, ctx *common.LambdaContext) *FederationAggregatorProcessor {
		return &FederationAggregatorProcessor{
			db:                           db,
			tableName:                    tableName,
			logger:                       zap.NewNop(),
			federationActivityRepository: &fakeFederationActivityRepo{},
			lambdaCtx:                    ctx,
		}
	}

	require.NoError(t, initializeFederationAggregator())
	require.NotNil(t, processor)
}

func TestInitializeFederationAggregator_Error_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		newLambdaOptimizedClientFn = origNewClient
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:      &config.Config{Region: "us-east-1", DynamoTableName: "table"},
			Logger:      zap.NewNop(),
			AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		}
	}
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return nil, errors.New("boom") }

	require.Error(t, initializeFederationAggregator())
}

func TestMain_WiresAppTheoryStart_Round12(t *testing.T) {
	origStart := lambdaStartFn
	origProcessor := processor
	t.Cleanup(func() {
		lambdaStartFn = origStart
		processor = origProcessor
	})

	startCalls := 0
	lambdaStartFn = func(handler any) {
		startCalls++
		h, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		body, err := json.Marshal(AggregationEvent{
			Type:      "daily",
			StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)

		event := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId:      "m1",
					Body:           string(body),
					EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-dev-federation-aggregator-queue",
					EventSource:    "aws:sqs",
				},
			},
		}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := h(context.Background(), raw)
		require.NoError(t, err)

		resp, ok := respAny.(events.SQSEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	db.On("Model", mock.Anything).Return(query)
	query.On("Create").Return(nil)

	processor = &FederationAggregatorProcessor{
		db:                           db,
		logger:                       zap.NewNop(),
		federationActivityRepository: &fakeFederationActivityRepo{},
		lambdaCtx: &common.LambdaContext{
			Config:      &config.Config{Region: "us-east-1", AWSAccountID: "123"},
			Logger:      zap.NewNop(),
			AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		},
	}

	main()
	require.Equal(t, 1, startCalls)
}

func TestNewFederationAggregatorProcessor_Round12(t *testing.T) {
	db := dynamormmocks.NewMockExtendedDB()
	lctx := &common.LambdaContext{Logger: zap.NewNop(), Config: &config.Config{}}
	p := NewFederationAggregatorProcessor(db, "table", lctx)
	require.NotNil(t, p)
	require.NotNil(t, p.federationActivityRepository)
}

func TestProcessSQSMessage_Round12(t *testing.T) {
	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	db.On("Model", mock.Anything).Return(query)
	query.On("Create").Return(nil)

	p := &FederationAggregatorProcessor{
		db:                           db,
		logger:                       zap.NewNop(),
		federationActivityRepository: &fakeFederationActivityRepo{},
	}

	require.Error(t, p.processSQSMessage(context.Background(), events.SQSMessage{Body: `{"bad"`}))

	body, err := json.Marshal(AggregationEvent{Type: "daily", StartTime: time.Now().Add(-time.Hour), EndTime: time.Now()})
	require.NoError(t, err)
	require.NoError(t, p.processSQSMessage(context.Background(), events.SQSMessage{Body: string(body)}))
}

func TestHandleSQS_AndHandleEvent_Round12(t *testing.T) {
	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	db.On("Model", mock.Anything).Return(query)
	query.On("Create").Return(nil)

	repo := &fakeFederationActivityRepo{}
	p := &FederationAggregatorProcessor{
		db:                           db,
		logger:                       zap.NewNop(),
		federationActivityRepository: repo,
		lambdaCtx: &common.LambdaContext{
			Config:      &config.Config{Region: "us-east-1", AWSAccountID: "123"},
			Logger:      zap.NewNop(),
			AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		},
	}

	require.Error(t, p.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1", Body: `{"bad"`}))

	validDetail, err := json.Marshal(AggregationEvent{
		Type:      "daily",
		StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	_, err = p.HandleScheduledEvent(nil, events.EventBridgeEvent{Detail: validDetail})
	require.NoError(t, err)
}

func TestHandleEventBridgeEvent_DefaultsHourly_Round12(t *testing.T) {
	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	db.On("Model", mock.Anything).Return(query)
	query.On("Create").Return(nil)

	repo := &fakeFederationActivityRepo{}
	p := &FederationAggregatorProcessor{
		db:                           db,
		logger:                       zap.NewNop(),
		federationActivityRepository: repo,
		lambdaCtx: &common.LambdaContext{
			Config:      &config.Config{Region: "us-east-1", AWSAccountID: "123"},
			Logger:      zap.NewNop(),
			AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		},
	}

	event := events.EventBridgeEvent{
		Time:   time.Date(2024, 1, 1, 12, 34, 0, 0, time.UTC),
		Detail: []byte("{not-json"),
	}
	require.NoError(t, p.handleEventBridgeEvent(context.Background(), event))
	require.Equal(t, time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC), repo.recentSince)
}

func TestHandleEventBridgeEvent_DefaultsEmptyScheduledDetail_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	p := &FederationAggregatorProcessor{
		logger:                       zap.NewNop(),
		federationActivityRepository: repo,
		lambdaCtx: &common.LambdaContext{
			Config:      &config.Config{Region: "us-east-1", AWSAccountID: "123"},
			Logger:      zap.NewNop(),
			AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		},
	}

	event := events.EventBridgeEvent{
		Time:   time.Date(2024, 1, 1, 12, 34, 0, 0, time.UTC),
		Detail: []byte("{}"),
	}
	require.NoError(t, p.handleEventBridgeEvent(context.Background(), event))
	require.Equal(t, 1, repo.recentCalls)
	require.Equal(t, time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC), repo.recentSince)
}

func TestHandleAggregationEvent_FullPath_Round12(t *testing.T) {
	now := time.Now().UTC()

	repo := &fakeFederationActivityRepo{
		recent: []*models.FederationActivity{
			{Domain: "a", Success: true, ResponseTime: 10, InboundSize: 1, OutboundSize: 2, ActivityType: "Create", Timestamp: now.Add(-30 * time.Minute), InstanceInfo: &models.InstanceInfo{Software: "mastodon"}},
			{Domain: "b", Success: false, ResponseTime: 0, InboundSize: 3, OutboundSize: 4, ActivityType: "Update", Timestamp: now.Add(-20 * time.Minute)},
		},
		list: []*models.FederationActivity{
			{Success: true, ResponseTime: 10},
			{Success: true, ResponseTime: 20},
		},
		info: &models.InstanceInfo{Software: "pleroma"},
	}

	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	db.On("Model", mock.Anything).Return(query)
	query.On("Create").Return(nil).Once()

	p := &FederationAggregatorProcessor{
		db:                           db,
		logger:                       zap.NewNop(),
		federationActivityRepository: repo,
		lambdaCtx:                    &common.LambdaContext{Config: &config.Config{Region: "us-east-1", AWSAccountID: "123"}},
	}

	err := p.handleAggregationEvent(context.Background(), AggregationEvent{
		Type:      "daily",
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
		Domains:   []string{"a", "b"},
	})
	require.NoError(t, err)
}

func TestIsDomainIncluded_AndTriggerNextLevelAggregation_Round12(t *testing.T) {
	t.Setenv("FEDERATION_AGGREGATOR_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/123/federation-aggregator-queue")
	p := &FederationAggregatorProcessor{
		logger:       zap.NewNop(),
		lambdaCtx:    &common.LambdaContext{Config: &config.Config{Region: "us-east-1"}},
		sqsClient:    &fakeSQSClient{},
		lambdaClient: &fakeLambdaClient{},
	}

	require.True(t, p.isDomainIncluded("a", []string{"a", "b"}))
	require.False(t, p.isDomainIncluded("c", []string{"a", "b"}))

	end := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	p.triggerNextLevelAggregation(context.Background(), AggregationEvent{Type: "hourly", EndTime: end})
	require.Equal(t, 1, p.sqsClient.(*fakeSQSClient).sendCalls)

	p.triggerNextLevelAggregation(context.Background(), AggregationEvent{Type: "daily", EndTime: end})
	require.Equal(t, 1, p.sqsClient.(*fakeSQSClient).sendCalls)
}

func TestHooksAndTableName_Round12(t *testing.T) {
	originalTableName := federationAggregationTableName
	t.Cleanup(func() { federationAggregationTableName = originalTableName })
	federationAggregationTableName = "configured-table"

	agg := &FederationAggregation{}
	require.Equal(t, "configured-table", agg.TableName())
	require.NoError(t, agg.BeforeCreate())
	before := agg.UpdatedAt
	require.NoError(t, agg.BeforeUpdate())
	require.True(t, agg.UpdatedAt.After(before) || agg.UpdatedAt.Equal(before))
}

func TestFederationAggregator_InternalMethods_Round12(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeFederationActivityRepo{
		recent: []*models.FederationActivity{
			{Domain: "a", Timestamp: now.Add(-2 * time.Hour)},
			{Domain: "b", Timestamp: now.Add(2 * time.Hour)},
		},
		list: []*models.FederationActivity{
			{Success: true, ResponseTime: 10},
			{Success: true, ResponseTime: 20},
			{Success: false, ResponseTime: 99},
		},
		info: &models.InstanceInfo{Software: "mastodon"},
	}

	p := &FederationAggregatorProcessor{
		logger:                       zap.NewNop(),
		federationActivityRepository: repo,
	}

	event := AggregationEvent{StartTime: now.Add(-3 * time.Hour), EndTime: now.Add(-time.Hour)}
	filtered, err := p.getFilteredActivities(context.Background(), event)
	require.NoError(t, err)
	require.Len(t, filtered, 1)

	agg := p.createAggregation(AggregationEvent{Type: "daily", StartTime: now.Add(-24 * time.Hour), EndTime: now})
	domainSoftware := p.processActivities([]*models.FederationActivity{
		{Domain: "a", Success: true, ResponseTime: 10, InboundSize: 1, OutboundSize: 2, ActivityType: "Create", Timestamp: now, InstanceInfo: &models.InstanceInfo{Software: "mastodon"}},
		{Domain: "b", Success: false, ResponseTime: 0, InboundSize: 3, OutboundSize: 4, ActivityType: "Update", Timestamp: now},
	}, nil, agg)
	require.Equal(t, 2, agg.TotalActivities)
	require.Equal(t, 1, agg.SuccessfulActivities)
	require.Equal(t, 1, agg.FailedActivities)
	require.Equal(t, int64(4), agg.TotalInboundBytes)
	require.Equal(t, int64(6), agg.TotalOutboundBytes)
	require.Equal(t, 2, len(agg.DomainStats))
	require.Equal(t, "mastodon", domainSoftware["a"])

	require.NoError(t, p.calculateDomainMetrics(context.Background(), AggregationEvent{StartTime: now.Add(-time.Hour), EndTime: now}, agg))
	require.Equal(t, 2, agg.ActiveDomains)

	p.buildSoftwareDistribution(context.Background(), agg, domainSoftware)
	require.GreaterOrEqual(t, repo.infoCalls, 1)
}

func TestFederationAggregator_StoreAggregation_Round12(t *testing.T) {
	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	db.On("Model", mock.Anything).Return(query)

	p := &FederationAggregatorProcessor{
		db:     db,
		logger: zap.NewNop(),
	}

	agg := &FederationAggregation{}
	query.On("Create").Return(nil).Once()
	require.NoError(t, p.storeAggregation(context.Background(), agg))

	query = new(dynamormmocks.MockQuery)
	db = new(dynamormmocks.MockDB)
	db.On("Model", mock.Anything).Return(query)
	p.db = db
	query.On("Create").Return(errors.New("boom")).Once()
	query.On("Update", mock.Anything).Return(nil).Once()
	require.NoError(t, p.storeAggregation(context.Background(), agg))

	query = new(dynamormmocks.MockQuery)
	db = new(dynamormmocks.MockDB)
	db.On("Model", mock.Anything).Return(query)
	p.db = db
	query.On("Create").Return(errors.New("boom")).Once()
	query.On("Update", mock.Anything).Return(errors.New("nope")).Once()
	require.Error(t, p.storeAggregation(context.Background(), agg))
}

func TestFederationAggregator_TriggerAggregation_Round12(t *testing.T) {
	t.Setenv("FEDERATION_AGGREGATOR_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/123/federation-aggregator-queue")
	p := &FederationAggregatorProcessor{
		logger:       zap.NewNop(),
		lambdaCtx:    &common.LambdaContext{Config: &config.Config{Region: "us-east-1"}},
		sqsClient:    &fakeSQSClient{},
		lambdaClient: &fakeLambdaClient{},
	}

	err := p.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()})
	require.NoError(t, err)
	require.Equal(t, "https://sqs.us-east-1.amazonaws.com/123/federation-aggregator-queue", p.sqsClient.(*fakeSQSClient).lastQueueURL)

	p.sqsClient = &fakeSQSClient{err: errors.New("boom")}
	p.lambdaClient = &fakeLambdaClient{result: &awslambda.InvokeOutput{StatusCode: 202}}
	require.NoError(t, p.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()}))

	p.lambdaClient = nil
	err = p.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()})
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))

	p.lambdaClient = &fakeLambdaClient{result: &awslambda.InvokeOutput{StatusCode: 200, FunctionError: aws.String("Unhandled")}}
	require.Error(t, p.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()}))

	p.lambdaClient = &fakeLambdaClient{err: errors.New("boom")}
	require.Error(t, p.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()}))
}

func TestFederationAggregator_UsesDeployedResourceNames_Round12(t *testing.T) {
	t.Setenv("APP_NAME", "simulacrum")
	t.Setenv("FEDERATION_AGGREGATOR_QUEUE_URL", "https://sqs.us-west-2.amazonaws.com/123456789012/simulacrum-live-federation-aggregator-queue")
	p := &FederationAggregatorProcessor{
		lambdaCtx: &common.LambdaContext{Config: &config.Config{
			Region: "us-west-2",
			Stage:  "live",
		}},
	}

	queueURL, err := p.federationAggregatorQueueURL()
	require.NoError(t, err)
	require.Equal(t, "https://sqs.us-west-2.amazonaws.com/123456789012/simulacrum-live-federation-aggregator-queue", queueURL)
	require.Equal(t, "simulacrum-live-federation-aggregator", p.federationAggregatorFunctionName())
}

func TestFederationAggregator_ResourceNameFallbacks_Round12(t *testing.T) {
	t.Setenv("APP_NAME", "")
	t.Setenv("STAGE", "")
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("FEDERATION_AGGREGATOR_QUEUE_URL", "")

	var nilProcessor *FederationAggregatorProcessor
	_, err := nilProcessor.federationAggregatorQueueURL()
	require.Error(t, err)
	require.Equal(t, "lesser-dev-federation-aggregator", nilProcessor.federationAggregatorFunctionName())

	p := &FederationAggregatorProcessor{lambdaCtx: &common.LambdaContext{Config: &config.Config{
		Region:      "us-east-2",
		Environment: "production",
	}}}
	require.Equal(t, "lesser-live-federation-aggregator", p.federationAggregatorFunctionName())

	t.Setenv("FEDERATION_AGGREGATOR_QUEUE_URL", "https://sqs.us-east-2.amazonaws.com/111122223333/lesser-live-federation-aggregator-queue")
	queueURL, err := p.federationAggregatorQueueURL()
	require.NoError(t, err)
	require.Equal(t, "https://sqs.us-east-2.amazonaws.com/111122223333/lesser-live-federation-aggregator-queue", queueURL)
}

func TestFederationAggregator_TriggerAggregationFallsBackWithoutQueueURL_Round12(t *testing.T) {
	t.Setenv("FEDERATION_AGGREGATOR_QUEUE_URL", "")
	t.Setenv("APP_NAME", "simulacrum")
	t.Setenv("STAGE", "live")

	lambdaClient := &fakeLambdaClient{result: &awslambda.InvokeOutput{StatusCode: 202}}
	p := &FederationAggregatorProcessor{
		logger:       zap.NewNop(),
		lambdaCtx:    &common.LambdaContext{Config: &config.Config{Region: "us-east-1", Stage: "live"}},
		sqsClient:    &fakeSQSClient{},
		lambdaClient: lambdaClient,
	}

	err := p.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()})
	require.NoError(t, err)
	require.Equal(t, 0, p.sqsClient.(*fakeSQSClient).sendCalls)
	require.Equal(t, 1, lambdaClient.invokeCalls)
}

func TestFederationAggregator_QueueURLDoesNotRequireAWSAccountID_Round12(t *testing.T) {
	t.Setenv("FEDERATION_AGGREGATOR_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/111122223333/simulacrum-live-federation-aggregator-queue")
	t.Setenv("AWS_ACCOUNT_ID", "")

	p := &FederationAggregatorProcessor{
		lambdaCtx: &common.LambdaContext{Config: &config.Config{
			Region: "us-east-1",
			Stage:  "live",
		}},
	}
	queueURL, err := p.federationAggregatorQueueURL()
	require.NoError(t, err)
	require.Equal(t, "https://sqs.us-east-1.amazonaws.com/111122223333/simulacrum-live-federation-aggregator-queue", queueURL)

	t.Setenv("FEDERATION_AGGREGATOR_QUEUE_URL", "")
	_, err = p.federationAggregatorQueueURL()
	require.Error(t, err)
}
