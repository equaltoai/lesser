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
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeFederationActivityRepo struct {
	recentCalls int
	listCalls   int
	infoCalls   int

	recent []*models.FederationActivity
	list   []*models.FederationActivity
	info   *models.InstanceInfo

	recentErr error
	listErr   error
	infoErr   error
}

func (f *fakeFederationActivityRepo) GetRecentActivities(context.Context, time.Time, int) ([]*models.FederationActivity, error) {
	f.recentCalls++
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
	sendCalls int
	err       error
}

func (f *fakeSQSClient) SendMessage(context.Context, *sqs.SendMessageInput, ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.sendCalls++
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
	origInit := initializeWithDefaultsFn
	origNewClient := newLambdaOptimizedClientFn
	origNewProcessor := newProcessorFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origInit
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
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
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
	origInit := initializeWithDefaultsFn
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origInit
		newLambdaOptimizedClientFn = origNewClient
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:      &config.Config{Region: "us-east-1", DynamoTableName: "table"},
			Logger:      zap.NewNop(),
			AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return nil }
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return nil, errors.New("boom") }

	require.Error(t, initializeFederationAggregator())
}

func TestMain_WiresLambdaStart_Round12(t *testing.T) {
	origStart := lambdaStartFn
	origLambdaCtx := lambdaCtx
	origLogger := logger
	t.Cleanup(func() {
		lambdaStartFn = origStart
		lambdaCtx = origLambdaCtx
		logger = origLogger
	})

	lambdaCtx = &common.LambdaContext{Logger: zap.NewNop()}
	logger = zap.NewNop()

	called := false
	lambdaStartFn = func(any) { called = true }
	main()
	require.True(t, called)
}

func TestHandleFederationAggregatorSQS_Branches_Round12(t *testing.T) {
	origProcessor := processor
	t.Cleanup(func() { processor = origProcessor })

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))

	processor = nil
	require.Error(t, handleFederationAggregatorSQS(ctx))

	processor = &FederationAggregatorProcessor{logger: zap.NewNop(), lambdaCtx: &common.LambdaContext{AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}}}}
	ctx.Request.RawEvent = nil
	require.Error(t, handleFederationAggregatorSQS(ctx))

	ctx.Request.RawEvent = make(chan int)
	require.Error(t, handleFederationAggregatorSQS(ctx))

	ctx.Request.RawEvent = "not-an-object"
	require.Error(t, handleFederationAggregatorSQS(ctx))

	ctx.Request.RawEvent = events.SQSEvent{}
	require.NoError(t, handleFederationAggregatorSQS(ctx))
}

func TestHandleFederationAggregatorEventBridge_Branches_Round12(t *testing.T) {
	origProcessor := processor
	t.Cleanup(func() { processor = origProcessor })

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	processor = nil
	require.Error(t, handleFederationAggregatorEventBridge(ctx))

	processor = &FederationAggregatorProcessor{
		logger:                       zap.NewNop(),
		lambdaCtx:                    &common.LambdaContext{AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}}},
		federationActivityRepository: &fakeFederationActivityRepo{},
	}

	ctx.Request.RawEvent = nil
	require.Error(t, handleFederationAggregatorEventBridge(ctx))

	ctx.Request.RawEvent = make(chan int)
	require.Error(t, handleFederationAggregatorEventBridge(ctx))

	ctx.Request.RawEvent = "not-an-object"
	require.Error(t, handleFederationAggregatorEventBridge(ctx))

	detail, err := json.Marshal(AggregationEvent{
		Type:      "daily",
		StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	ctx.Request.RawEvent = events.CloudWatchEvent{Detail: detail}
	require.NoError(t, handleFederationAggregatorEventBridge(ctx))
}

func TestNewFederationAggregatorProcessor_Round12(t *testing.T) {
	db := dynamormmocks.NewMockExtendedDB()
	lctx := &common.LambdaContext{Logger: zap.NewNop(), Config: &config.Config{}}
	p := NewFederationAggregatorProcessor(db, "table", lctx)
	require.NotNil(t, p)
	require.NotNil(t, p.federationActivityRepository)
}

func TestProcessSQSMessage_Round12(t *testing.T) {
	p := &FederationAggregatorProcessor{
		logger:                       zap.NewNop(),
		federationActivityRepository: &fakeFederationActivityRepo{},
	}

	require.Error(t, p.processSQSMessage(context.Background(), events.SQSMessage{Body: `{"bad"`}))

	body, err := json.Marshal(AggregationEvent{Type: "daily", StartTime: time.Now().Add(-time.Hour), EndTime: time.Now()})
	require.NoError(t, err)
	require.NoError(t, p.processSQSMessage(context.Background(), events.SQSMessage{Body: string(body)}))
}

func TestHandleSQS_AndHandleEvent_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	p := &FederationAggregatorProcessor{
		logger:                       zap.NewNop(),
		federationActivityRepository: repo,
		lambdaCtx:                    &common.LambdaContext{AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}}},
	}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	require.Error(t, p.HandleSQS(ctx, events.SQSEvent{Records: []events.SQSMessage{{Body: `{"bad"`}}}))

	validDetail, err := json.Marshal(AggregationEvent{
		Type:      "daily",
		StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NoError(t, p.HandleEvent(ctx, events.CloudWatchEvent{Detail: validDetail}))
}

func TestHandleCloudWatchEvent_DefaultsHourly_Round12(t *testing.T) {
	p := &FederationAggregatorProcessor{
		logger:                       zap.NewNop(),
		federationActivityRepository: &fakeFederationActivityRepo{},
		lambdaCtx:                    &common.LambdaContext{AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}}},
	}

	event := events.CloudWatchEvent{
		Time:   time.Date(2024, 1, 1, 12, 34, 0, 0, time.UTC),
		Detail: []byte("{not-json"),
	}
	require.NoError(t, p.handleCloudWatchEvent(context.Background(), event))
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
	p := &FederationAggregatorProcessor{
		logger:       zap.NewNop(),
		lambdaCtx:    &common.LambdaContext{Config: &config.Config{Region: "us-east-1", AWSAccountID: "123"}},
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
	agg := &FederationAggregation{}
	require.Equal(t, "lesser-main", agg.TableName())
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
	p := &FederationAggregatorProcessor{
		logger:       zap.NewNop(),
		lambdaCtx:    &common.LambdaContext{Config: &config.Config{Region: "us-east-1", AWSAccountID: "123"}},
		sqsClient:    &fakeSQSClient{},
		lambdaClient: &fakeLambdaClient{},
	}

	err := p.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()})
	require.NoError(t, err)

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
