package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type fakeCostRepo struct {
	createAggregatedCalls int
	createCalls           int
	aggregateCalls        int
	getAggregatedCalls    int
	highCostCalls         int

	lastAggregated *models.DynamoDBCostAggregation
	lastCreated    *models.DynamoDBCostRecord

	createAggregatedErr error
	createErr           error
	aggregateErr        error

	getAggregatedFn func(period, opType string) (*models.DynamoDBCostAggregation, error)
	highCostOps     []*models.DynamoDBCostRecord
}

func (f *fakeCostRepo) CreateAggregated(_ context.Context, aggregated *models.DynamoDBCostAggregation) error {
	f.createAggregatedCalls++
	f.lastAggregated = aggregated
	return f.createAggregatedErr
}

func (f *fakeCostRepo) Create(_ context.Context, tracking *models.DynamoDBCostRecord) error {
	f.createCalls++
	f.lastCreated = tracking
	return f.createErr
}

func (f *fakeCostRepo) Aggregate(_ context.Context, _, _ string, _ time.Time, _ time.Time) error {
	f.aggregateCalls++
	return f.aggregateErr
}

func (f *fakeCostRepo) GetAggregated(_ context.Context, period, operationType string, _ time.Time) (*models.DynamoDBCostAggregation, error) {
	f.getAggregatedCalls++
	if f.getAggregatedFn != nil {
		return f.getAggregatedFn(period, operationType)
	}
	return &models.DynamoDBCostAggregation{TotalCostDollars: 0, TotalOperations: 1, AverageCostPerOperation: 0}, nil
}

func (f *fakeCostRepo) GetHighCostOperations(context.Context, float64, time.Time, time.Time, int) ([]*models.DynamoDBCostRecord, error) {
	f.highCostCalls++
	return f.highCostOps, nil
}

type fakeSNSClient struct {
	publishCalls int
	lastInput    *sns.PublishInput
	err          error
}

func (f *fakeSNSClient) Publish(_ context.Context, input *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	f.publishCalls++
	f.lastInput = input
	if f.err != nil {
		return nil, f.err
	}
	return &sns.PublishOutput{MessageId: aws.String("mid")}, nil
}

type fakeCloudWatchClient struct {
	putCalls int
	err      error
}

func (f *fakeCloudWatchClient) PutMetricData(_ context.Context, _ *cloudwatch.PutMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	f.putCalls++
	if f.err != nil {
		return nil, f.err
	}
	return &cloudwatch.PutMetricDataOutput{}, nil
}

type fakeSQSClient struct {
	sendCalls int
	err       error
}

func (f *fakeSQSClient) SendMessage(_ context.Context, _ *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.sendCalls++
	if f.err != nil {
		return nil, f.err
	}
	return &sqs.SendMessageOutput{MessageId: aws.String("qid")}, nil
}

type fakeLambdaClient struct {
	invokeCalls int
	result      *awslambda.InvokeOutput
	err         error
}

func (f *fakeLambdaClient) Invoke(_ context.Context, _ *awslambda.InvokeInput, _ ...func(*awslambda.Options)) (*awslambda.InvokeOutput, error) {
	f.invokeCalls++
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &awslambda.InvokeOutput{StatusCode: 202}, nil
}

func TestCostAggregator_Helpers_Round12(t *testing.T) {
	require.Equal(t, "warning", (&CostAggregator{}).determineSeverity(2, 10))
	require.Equal(t, "critical", (&CostAggregator{}).determineSeverity(20, 10))
	require.Equal(t, "emergency", (&CostAggregator{}).determineSeverity(50, 10))

	image := map[string]events.DynamoDBAttributeValue{
		"A": events.NewStringAttribute("v"),
		"N": events.NewNumberAttribute("1.5"),
		"S": events.NewStringAttribute("2.5"),
	}
	val, ok := getAttribute(image, "A")
	require.True(t, ok)
	require.Equal(t, "v", val)

	_, ok = getAttribute(image, "missing")
	require.False(t, ok)

	num, ok := getNumberAttribute(image, "N")
	require.True(t, ok)
	require.Equal(t, 1.5, num)

	num, ok = getNumberFromMap(image, "S")
	require.True(t, ok)
	require.Equal(t, 2.5, num)

	require.True(t, (&CostAggregator{}).isCostRecord("cost#PutItem"))
	require.False(t, (&CostAggregator{}).isCostRecord("nope"))
}

func TestCostAggregator_extractCostFromStreamRecord_Round12(t *testing.T) {
	ca := &CostAggregator{}

	// Nil image should be ignored.
	cost, err := ca.extractCostFromStreamRecord(events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: nil},
	})
	require.NoError(t, err)
	require.Nil(t, cost)

	// Plain record is ignored.
	cost, err = ca.extractCostFromStreamRecord(events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OTHER#1")}},
	})
	require.NoError(t, err)
	require.Nil(t, cost)

	// Cost tracking record.
	cost, err = ca.extractCostFromStreamRecord(events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"PK":                   events.NewStringAttribute("cost#PutItem"),
			"operation_type":       events.NewStringAttribute("PutItem"),
			"table_name":           events.NewStringAttribute("tbl"),
			"read_capacity_units":  events.NewNumberAttribute("0"),
			"write_capacity_units": events.NewNumberAttribute("10"),
			"item_count":           events.NewNumberAttribute("1"),
			"service_name":         events.NewStringAttribute("api"),
			"timestamp":            events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, cost)
	require.Equal(t, "PutItem", cost.OperationType)
	require.Equal(t, "tbl", cost.Table)
	require.Greater(t, cost.TotalCostMicroCents, int64(0))

	// Embedded consumed capacity record.
	cost, err = ca.extractCostFromStreamRecord(events.DynamoDBEventRecord{
		EventName: "MODIFY",
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"ConsumedCapacity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
				"ReadCapacityUnits":  events.NewNumberAttribute("1"),
				"WriteCapacityUnits": events.NewStringAttribute("2"),
			}),
			"operation_type": events.NewStringAttribute("Query"),
			"table_name":     events.NewStringAttribute("tbl"),
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, cost)
	require.Equal(t, "Query", cost.OperationType)
	require.Equal(t, "tbl", cost.Table)
}

func TestCostAggregator_processRealtimeCosts_Round12(t *testing.T) {
	repo := &fakeCostRepo{}
	ca := &CostAggregator{
		costTrackingRepository: repo,
		logger:                 zap.NewNop(),
	}

	costs := []*models.DynamoDBCostRecord{
		{OperationType: "PutItem", Table: "tbl", ReadCapacityUnits: 0, WriteCapacityUnits: 1, TotalCostMicroCents: 10, ItemCount: 1, RequestDuration: 5, ServiceName: "api"},
		{OperationType: "PutItem", Table: "tbl2", ReadCapacityUnits: 0, WriteCapacityUnits: 2, TotalCostMicroCents: 20, ItemCount: 2, RequestDuration: 15, ServiceName: "api"},
	}

	require.NoError(t, ca.processRealtimeCosts(context.Background(), "req", costs))
	require.Equal(t, 1, repo.createAggregatedCalls)
	require.Equal(t, 2, repo.createCalls)
	require.NotNil(t, repo.lastAggregated)
	require.Equal(t, int64(2), repo.lastAggregated.TotalOperations)
	require.Equal(t, float64(10), repo.lastAggregated.AverageDuration)
}

func TestCostAggregator_handleAggregationEvent_TriggersDaily_Round12(t *testing.T) {
	repo := &fakeCostRepo{}
	sqsClient := &fakeSQSClient{}
	ca := &CostAggregator{
		costTrackingRepository: repo,
		logger:                 zap.NewNop(),
		cfg:                    &config.Config{Region: "us-east-1", AWSAccountID: "123"},
		sqsClient:              sqsClient,
	}

	end := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	start := end.Add(-time.Hour)

	require.NoError(t, ca.handleAggregationEvent(context.Background(), AggregationEvent{
		Type:      "hourly",
		StartTime: start,
		EndTime:   end,
	}))
	require.Greater(t, repo.aggregateCalls, 0)
	require.Equal(t, 1, sqsClient.sendCalls)
}

func TestCostAggregator_checkCostAlerts_SendsAlerts_Round12(t *testing.T) {
	repo := &fakeCostRepo{
		getAggregatedFn: func(period, opType string) (*models.DynamoDBCostAggregation, error) {
			if period == "hourly" && opType == "GetItem" {
				return &models.DynamoDBCostAggregation{TotalCostDollars: 1.0, TotalOperations: 10}, nil
			}
			return &models.DynamoDBCostAggregation{TotalCostDollars: 0.0, TotalOperations: 1}, nil
		},
		highCostOps: []*models.DynamoDBCostRecord{
			{OperationType: "GetItem", Table: "tbl", EstimatedCostDollars: 0.02, ItemCount: 1, ServiceName: "api"},
		},
	}

	snsClient := &fakeSNSClient{}
	cwClient := &fakeCloudWatchClient{}
	ca := &CostAggregator{
		costTrackingRepository: repo,
		logger:                 zap.NewNop(),
		cfg:                    &config.Config{Region: "us-east-1", AWSAccountID: "123", DynamoTableName: "tbl"},
		snsClient:              snsClient,
		cloudWatchClient:       cwClient,
	}

	now := time.Now().UTC()
	ca.checkCostAlerts(context.Background(), "hourly", now.Add(-time.Hour), now)

	require.GreaterOrEqual(t, snsClient.publishCalls, 1)
	require.GreaterOrEqual(t, cwClient.putCalls, 1)
	require.GreaterOrEqual(t, repo.highCostCalls, 1)
}

func TestCostAggregator_checkCostAlerts_UnknownPeriod_Round12(t *testing.T) {
	repo := &fakeCostRepo{}
	ca := &CostAggregator{
		costTrackingRepository: repo,
		logger:                 zap.NewNop(),
		cfg:                    &config.Config{Region: "us-east-1", AWSAccountID: "123", DynamoTableName: "tbl"},
	}

	now := time.Now().UTC()
	ca.checkCostAlerts(context.Background(), "minute", now.Add(-time.Hour), now)
	require.Equal(t, 0, repo.getAggregatedCalls)
}

func TestCostAggregator_sendCostAlert_NoClients_Round12(t *testing.T) {
	ca := &CostAggregator{
		logger: zap.NewNop(),
		cfg:    &config.Config{Region: "us-east-1", AWSAccountID: "123", DynamoTableName: "tbl"},
	}

	ca.sendCostAlert(context.Background(), CostAlert{
		Type:          "threshold_exceeded",
		OperationType: "GetItem",
		Period:        "hourly",
		Cost:          1.0,
		Threshold:     0.10,
		Operations:    10,
		Timestamp:     time.Now().UTC(),
		Severity:      "warning",
	})
}

func TestCostAggregator_sendSNSAlert_ErrorBranches_Round12(t *testing.T) {
	ca := &CostAggregator{
		logger: zap.NewNop(),
		cfg:    &config.Config{Region: "us-east-1", AWSAccountID: "123"},
	}

	err := ca.sendSNSAlert(context.Background(), CostAlert{OperationType: "GetItem", Severity: "warning", Timestamp: time.Now().UTC()})
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))

	ca.snsClient = &fakeSNSClient{}
	err = ca.sendSNSAlert(context.Background(), CostAlert{
		OperationType: "GetItem",
		Severity:      "warning",
		Timestamp:     time.Now().UTC(),
		Details:       map[string]interface{}{"bad": make(chan int)},
	})
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))

	ca.snsClient = &fakeSNSClient{err: errors.New("boom")}
	err = ca.sendSNSAlert(context.Background(), CostAlert{OperationType: "GetItem", Severity: "warning", Timestamp: time.Now().UTC()})
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))
}

func TestCostAggregator_putCloudWatchMetric_ErrorBranches_Round12(t *testing.T) {
	ca := &CostAggregator{
		logger: zap.NewNop(),
		cfg:    &config.Config{Region: "us-east-1", AWSAccountID: "123"},
	}

	err := ca.putCloudWatchMetric(context.Background(), CostAlert{OperationType: "GetItem", Period: "hourly", Timestamp: time.Now().UTC()})
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))

	ca.cloudWatchClient = &fakeCloudWatchClient{err: errors.New("boom")}
	err = ca.putCloudWatchMetric(context.Background(), CostAlert{OperationType: "GetItem", Period: "hourly", Timestamp: time.Now().UTC()})
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))
}

func TestCostAggregator_aggregateCosts_ErrorBranches_Round12(t *testing.T) {
	repo := &fakeCostRepo{aggregateErr: errors.New("boom")}
	ca := &CostAggregator{
		costTrackingRepository: repo,
		logger:                 zap.NewNop(),
	}

	err := ca.aggregateCosts(context.Background(), "GetItem", "hourly", time.Now().Add(-time.Hour), time.Now())
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))

	repo.aggregateErr = nil
	repo.getAggregatedFn = func(string, string) (*models.DynamoDBCostAggregation, error) {
		return nil, errors.New("nope")
	}
	require.NoError(t, ca.aggregateCosts(context.Background(), "GetItem", "hourly", time.Now().Add(-time.Hour), time.Now()))
}

func TestCostAggregator_triggerAggregation_FallbackToLambda_Round12(t *testing.T) {
	ca := &CostAggregator{
		logger:       zap.NewNop(),
		cfg:          &config.Config{Region: "us-east-1", AWSAccountID: "123"},
		sqsClient:    &fakeSQSClient{err: errors.New("boom")},
		lambdaClient: &fakeLambdaClient{result: &awslambda.InvokeOutput{StatusCode: 202}},
	}

	err := ca.triggerAggregation(context.Background(), AggregationEvent{
		Type:      "daily",
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
	})
	require.NoError(t, err)

	fail := &fakeLambdaClient{result: &awslambda.InvokeOutput{StatusCode: 200, FunctionError: aws.String("Unhandled")}}
	ca.lambdaClient = fail
	require.Error(t, ca.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()}))

	ca.lambdaClient = nil
	require.Error(t, ca.triggerAggregation(context.Background(), AggregationEvent{Type: "daily", StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()}))
}

func TestHandleCostAggregatorStreamRecord_Round12(t *testing.T) {
	origProcessor := processor
	t.Cleanup(func() { processor = origProcessor })

	processor = nil
	require.Error(t, handleCostAggregatorStreamRecord(nil, events.DynamoDBEventRecord{EventID: "e1", EventName: "INSERT"}))

	processor = &CostAggregator{logger: zap.NewNop(), costTrackingRepository: &fakeCostRepo{}}
	require.NoError(t, handleCostAggregatorStreamRecord(nil, events.DynamoDBEventRecord{EventID: "e1", EventName: "REMOVE"}))
}

func TestCostAggregator_HandleCloudWatchEvent_Round12(t *testing.T) {
	repo := &fakeCostRepo{}
	ca := &CostAggregator{
		costTrackingRepository: repo,
		logger:                 zap.NewNop(),
		cfg:                    &config.Config{Region: "us-east-1", AWSAccountID: "123", DynamoTableName: "tbl"},
	}

	validDetail := []byte(`{
		"type": "daily",
		"startTime": "2024-01-01T00:00:00Z",
		"endTime": "2024-01-02T00:00:00Z",
		"operationTypes": ["GetItem"]
	}`)
	require.NoError(t, ca.HandleCloudWatchEvent(context.Background(), events.CloudWatchEvent{Detail: validDetail}))
	require.Equal(t, 1, repo.aggregateCalls)

	repo.aggregateCalls = 0
	require.NoError(t, ca.HandleCloudWatchEvent(context.Background(), events.CloudWatchEvent{Detail: []byte("not-json")}))
	require.Equal(t, 10, repo.aggregateCalls)
}

func TestCostAggregator_HandleDynamoDBRecord_Round12(t *testing.T) {
	repo := &fakeCostRepo{createAggregatedErr: errors.New("agg"), createErr: errors.New("create")}
	ca := &CostAggregator{
		costTrackingRepository: repo,
		logger:                 zap.NewNop(),
	}

	require.NoError(t, ca.HandleDynamoDBRecord(nil, events.DynamoDBEventRecord{EventName: "REMOVE"}))
	require.NoError(t, ca.HandleDynamoDBRecord(nil, events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"PK":         events.NewStringAttribute("OTHER#1"),
			"table_name": events.NewStringAttribute("tbl"),
		}},
	}))

	require.NoError(t, ca.HandleDynamoDBRecord(nil, events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"PK":                   events.NewStringAttribute("cost#PutItem"),
			"operation_type":       events.NewStringAttribute("PutItem"),
			"table_name":           events.NewStringAttribute("tbl"),
			"read_capacity_units":  events.NewNumberAttribute("0"),
			"write_capacity_units": events.NewNumberAttribute("1"),
			"item_count":           events.NewNumberAttribute("1"),
			"timestamp":            events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
		}},
	}))
	require.Equal(t, 1, repo.createAggregatedCalls)
	require.Equal(t, 1, repo.createCalls)
}

func TestGetNumberAttribute_ParseFailure_Round12(t *testing.T) {
	image := map[string]events.DynamoDBAttributeValue{
		"N": events.NewNumberAttribute("nope"),
	}
	_, ok := getNumberAttribute(image, "N")
	require.False(t, ok)

	_, ok = getNumberAttribute(image, "missing")
	require.False(t, ok)
}

func TestInitializeCostAggregator_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origInitDefaults := initializeWithDefaultsFn
	origNewClient := newLambdaOptimizedClientFn
	origNewRepo := newCostTrackingRepoFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origInitDefaults
		newLambdaOptimizedClientFn = origNewClient
		newCostTrackingRepoFn = origNewRepo
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		awsCfg := aws.Config{Region: "us-east-1"}
		return &common.LambdaContext{
			Config: &config.Config{
				Region:          "us-east-1",
				DynamoTableName: "test-table",
				AWSAccountID:    "123",
			},
			Logger: zap.NewNop(),
			AWSServices: &awsInit.AWSServices{
				Config:     awsCfg,
				SNS:        sns.NewFromConfig(awsCfg),
				CloudWatch: cloudwatch.NewFromConfig(awsCfg),
				SQS:        sqs.NewFromConfig(awsCfg),
				Lambda:     awslambda.NewFromConfig(awsCfg),
			},
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return new(dynamormmocks.MockDB), nil
	}
	newCostTrackingRepoFn = func(dynamormCore.DB, string, *zap.Logger) costTrackingStore {
		return &fakeCostRepo{}
	}

	initializeCostAggregator()
	require.NotNil(t, processor)
	require.NotNil(t, processor.snsClient)
	require.NotNil(t, processor.cloudWatchClient)
	require.NotNil(t, processor.sqsClient)
	require.NotNil(t, processor.lambdaClient)
}

func TestMain_WiresLambdaStart_Round12(t *testing.T) {
	origStart := lambdaStartFn
	t.Cleanup(func() { lambdaStartFn = origStart })

	called := false
	lambdaStartFn = func(any) { called = true }
	main()
	require.True(t, called)

	// Ensure lambda invoke error mapping is reachable without panics.
	ca := &CostAggregator{
		logger:       zap.NewNop(),
		cfg:          &config.Config{Region: "us-east-1", AWSAccountID: "123"},
		lambdaClient: &fakeLambdaClient{err: errors.New("boom")},
	}
	err := ca.triggerAggregation(context.Background(), AggregationEvent{
		Type:      "daily",
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
	})
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))
}
