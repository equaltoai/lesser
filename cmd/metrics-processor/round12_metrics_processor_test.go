package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type stubMetricWriter struct {
	create func(ctx context.Context, record *models.MetricRecord) error
}

func (s stubMetricWriter) CreateMetricRecord(ctx context.Context, record *models.MetricRecord) error {
	if s.create != nil {
		return s.create(ctx, record)
	}
	return nil
}

type stubDLQWriter struct {
	create func(ctx context.Context, message *models.DLQMessage) error
}

func (s stubDLQWriter) CreateDLQMessage(ctx context.Context, message *models.DLQMessage) error {
	if s.create != nil {
		return s.create(ctx, message)
	}
	return nil
}

func newTestProcessor(metricWriter metricRecordWriter, dlqWriter dlqMessageWriter) *MetricsStreamProcessor {
	return &MetricsStreamProcessor{
		repos:         nil,
		reportingRepo: metricWriter,
		costCalculator: &CostCalculator{
			DynamoDBReadCostPerUnit:  25,
			DynamoDBWriteCostPerUnit: 125,
			LambdaCostPerInvocation:  20,
			S3CostPerOperation:       5,
			KMSCostPerOperation:      6,
		},
		dlqHandler: &DLQHandler{
			dlqRepo: dlqWriter,
			logger:  zap.NewNop(),
		},
		logger: zap.NewNop(),
	}
}

func TestGenerateValidatedUUID_FallbackAndSuccess(t *testing.T) {
	originalNewUUIDFn := newUUIDFn
	originalValidateUUIDFn := validateUUIDFn
	t.Cleanup(func() {
		newUUIDFn = originalNewUUIDFn
		validateUUIDFn = originalValidateUUIDFn
	})

	newUUIDFn = func() string { return "not-a-uuid" }
	validateUUIDFn = func(_ string, _ string) error { return errors.New("invalid") }

	fallback := generateValidatedUUID()
	require.True(t, strings.HasPrefix(fallback, "id_"))

	newUUIDFn = func() string { return "fixed-id" }
	validateUUIDFn = func(_ string, _ string) error { return nil }

	require.Equal(t, "fixed-id", generateValidatedUUID())
}

func TestMetricsStreamProcessor_DetermineMetricType_ServiceAndCosts(t *testing.T) {
	processor := newTestProcessor(stubMetricWriter{}, stubDLQWriter{})

	require.Equal(t, metricEventUser, processor.determineMetricType("USER#admin", ""))
	require.Equal(t, metricEventActor, processor.determineMetricType("ACTOR#admin", ""))
	require.Equal(t, metricEventObject, processor.determineMetricType("object#123", ""))
	require.Equal(t, metricEventNotification, processor.determineMetricType("NOTIFICATION#123", ""))
	require.Equal(t, metricEventEngagement, processor.determineMetricType("LIKE#123", ""))
	require.Equal(t, metricEventSocial, processor.determineMetricType("FOLLOW#123", ""))
	require.Equal(t, metricEventMedia, processor.determineMetricType("MEDIA#123", ""))
	require.Equal(t, metricEventModeration, processor.determineMetricType("MODERATION#123", ""))
	require.Equal(t, metricEventSecurity, processor.determineMetricType("TRUST#123", ""))
	require.Equal(t, metricEventSecurity, processor.determineMetricType("THREAT#123", ""))
	require.Equal(t, metricEventCostTracking, processor.determineMetricType("cost#123", ""))
	require.Equal(t, metricEventFederation, processor.determineMetricType("FEDERATION#123", ""))
	require.Equal(t, metricEventFederation, processor.determineMetricType("RELAY#123", ""))
	require.Equal(t, "content_event", processor.determineMetricType("HASHTAG#123", ""))
	require.Equal(t, "search_event", processor.determineMetricType("SEARCH#123", ""))
	require.Equal(t, "performance_event", processor.determineMetricType("RATE_LIMIT#123", ""))
	require.Equal(t, "", processor.determineMetricType("UNKNOWN#123", ""))

	require.Equal(t, "explicit", processor.determineService("USER#admin", "", map[string]events.DynamoDBAttributeValue{
		"ServiceName": events.NewStringAttribute("explicit"),
	}))
	require.Equal(t, "fn", processor.determineService("USER#admin", "", map[string]events.DynamoDBAttributeValue{
		"FunctionName": events.NewStringAttribute("fn"),
	}))
	require.Equal(t, "api", processor.determineService("USER#admin", "", map[string]events.DynamoDBAttributeValue{}))
	require.Equal(t, "objects", processor.determineService("object#123", "", map[string]events.DynamoDBAttributeValue{}))
	require.Equal(t, "notification-processor", processor.determineService("NOTIFICATION#123", "", map[string]events.DynamoDBAttributeValue{}))
	require.Equal(t, "media-processor", processor.determineService("MEDIA#123", "", map[string]events.DynamoDBAttributeValue{}))
	require.Equal(t, "federation-delivery", processor.determineService("FEDERATION#123", "", map[string]events.DynamoDBAttributeValue{}))
	require.Equal(t, "search-indexer", processor.determineService("SEARCH#123", "", map[string]events.DynamoDBAttributeValue{}))
	require.Equal(t, "moderation-processor", processor.determineService("MODERATION#123", "", map[string]events.DynamoDBAttributeValue{}))
	require.Equal(t, "unknown", processor.determineService("OTHER#admin", "", map[string]events.DynamoDBAttributeValue{}))

	federationInsert := &events.DynamoDBEventRecord{EventName: eventTypeInsert}
	federationCosts := processor.calculateOperationCosts(context.Background(), federationInsert, metricEventFederation, "federation-delivery")
	require.Greater(t, federationCosts.InstanceCost, int64(0))
	require.Greater(t, federationCosts.TotalCost, int64(0))

	authInsert := &events.DynamoDBEventRecord{EventName: eventTypeInsert}
	authCosts := processor.calculateOperationCosts(context.Background(), authInsert, metricEventUser, "api")
	require.Greater(t, authCosts.UserCost, int64(100))

	defaultCosts := processor.calculateOperationCosts(context.Background(), authInsert, "custom_event", "api")
	require.Greater(t, defaultCosts.TotalCost, int64(0))
}

func TestMetricsStreamProcessor_ProcessStreamRecordAndDeletion(t *testing.T) {
	var created []*models.MetricRecord
	metricWriter := stubMetricWriter{
		create: func(_ context.Context, record *models.MetricRecord) error {
			created = append(created, record)
			return nil
		},
	}
	processor := newTestProcessor(metricWriter, stubDLQWriter{})

	insertRecord := &events.DynamoDBEventRecord{
		EventName: eventTypeInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":       events.NewStringAttribute("USER#admin"),
				"SK":       events.NewStringAttribute("METADATA"),
				"UserID":   events.NewStringAttribute("admin"),
				"TenantID": events.NewStringAttribute("tenant"),
				"Domain":   events.NewStringAttribute("example.com"),
				"Count":    events.NewNumberAttribute("1"),
			},
		},
	}
	require.NoError(t, processor.ProcessStreamRecord(context.Background(), insertRecord))

	removeRecord := &events.DynamoDBEventRecord{
		EventName: eventTypeRemove,
		Change: events.DynamoDBStreamRecord{
			OldImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("USER#admin"),
			},
		},
	}
	require.NoError(t, processor.ProcessStreamRecord(context.Background(), removeRecord))

	unknownRecord := &events.DynamoDBEventRecord{EventName: "UNKNOWN"}
	require.NoError(t, processor.ProcessStreamRecord(context.Background(), unknownRecord))

	require.GreaterOrEqual(t, len(created), 2)
}

func TestMetricsStreamProcessor_ProcessDataChange_SkipsAndFailsToStore(t *testing.T) {
	processor := newTestProcessor(stubMetricWriter{}, stubDLQWriter{})

	require.NoError(t, processor.processDataChange(context.Background(), &events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{},
	}))
	require.NoError(t, processor.processDataChange(context.Background(), &events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("USER#admin"),
			},
		},
	}))

	processor.reportingRepo = stubMetricWriter{
		create: func(_ context.Context, _ *models.MetricRecord) error {
			return errors.New("write failed")
		},
	}
	require.Error(t, processor.processDataChange(context.Background(), &events.DynamoDBEventRecord{
		EventName: eventTypeInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("USER#admin"),
				"SK": events.NewStringAttribute("METADATA"),
			},
		},
	}))
}

func TestMetricsStreamProcessor_MetadataPriorityAndStreams(t *testing.T) {
	processor := newTestProcessor(stubMetricWriter{}, stubDLQWriter{})

	record := models.NewMetricRecordBuilder().
		ForService("api").
		OfType(metricEventUser).
		WithAggregationLevel("raw").
		WithTimestamp(time.Unix(0, 0).UTC()).
		WithStats(2, 10, 1, 9, 5, 0, 0).
		WithUnit(metricUnitCount).
		WithDimension("user_id", "admin").
		WithDimension("tenant_id", "tenant").
		WithDimension("actor_id", "actor").
		WithDimension("total_cost_microcents", "100001").
		Build()
	record.P95 = 2000
	record.Max = 6000

	metadata := processor.buildMetricsMetadata(record, "performance")
	require.Equal(t, "performance", metadata["subscription_category"])
	require.Equal(t, "api", metadata["service_name"])
	require.NotEmpty(t, metadata["timestamp"])
	require.Equal(t, "2", metadata["count"])
	require.Equal(t, "6000.00", metadata["max"])
	require.Equal(t, "admin", metadata["user_id"])

	priority := processor.determinePriority(record, "performance")
	require.Equal(t, streaming.PriorityHigh, priority)

	costPriority := processor.determinePriority(record, "cost")
	require.Equal(t, streaming.PriorityHigh, costPriority)

	streams := processor.getSubscriptionStreams("performance", "admin", "tenant", "api")
	require.Contains(t, streams, "metrics:global")
	require.Contains(t, streams, "metrics:performance")
	require.Contains(t, streams, "metrics:service:api")
	require.Contains(t, streams, "metrics:user:admin")
	require.Contains(t, streams, "metrics:tenant:tenant")
	require.Contains(t, streams, "metrics:performance:service:api")

	processor.publishEventAndLog(nil, "performance", record)
	processor.publishEventAndLog(&streaming.InternalEvent{
		ID:      "evt",
		Type:    streaming.EventTypeMetricsUpdate,
		Action:  streaming.ActionUpdate,
		Streams: []string{"metrics:global"},
	}, "performance", record)
}

func TestMetricsStreamProcessor_triggerRealTimeUpdates_CoversAllCases(t *testing.T) {
	processor := newTestProcessor(stubMetricWriter{}, stubDLQWriter{})

	record := models.NewMetricRecordBuilder().
		ForService("api").
		OfType(metricEventUser).
		WithAggregationLevel("raw").
		WithTimestamp(time.Unix(0, 0).UTC()).
		WithStats(1, 1, 1, 1, 0, 0, 0).
		WithUnit(metricUnitCount).
		Build()

	for _, metricType := range []string{
		metricEventModeration,
		metricEventSecurity,
		"performance_event",
		metricEventFederation,
		metricEventUser,
		metricEventActor,
		metricEventCostTracking,
		metricEventMedia,
		metricEventObject,
		metricEventEngagement,
		metricEventSocial,
		metricEventNotification,
		"custom_event",
	} {
		processor.triggerRealTimeUpdates(metricType, record)
	}
}

func TestMetricsStreamProcessor_IsHighCost_AndNilDimensionBranches(t *testing.T) {
	processor := newTestProcessor(stubMetricWriter{}, stubDLQWriter{})

	emptyDims := models.NewMetricRecordBuilder().
		ForService("api").
		OfType(metricEventUser).
		WithAggregationLevel("raw").
		WithTimestamp(time.Unix(0, 0).UTC()).
		WithStats(0, 0, 0, 0, 0, 0, 0).
		WithUnit(metricUnitCount).
		Build()
	emptyDims.Dimensions = nil
	require.False(t, processor.isHighCost(emptyDims))

	missingKey := models.NewMetricRecordBuilder().
		ForService("api").
		OfType(metricEventUser).
		WithAggregationLevel("raw").
		WithTimestamp(time.Unix(0, 0).UTC()).
		WithStats(0, 0, 0, 0, 0, 0, 0).
		WithUnit(metricUnitCount).
		WithDimension("user_cost_microcents", "10").
		Build()
	require.False(t, processor.isHighCost(missingKey))

	parseFail := models.NewMetricRecordBuilder().
		ForService("api").
		OfType(metricEventUser).
		WithAggregationLevel("raw").
		WithTimestamp(time.Unix(0, 0).UTC()).
		WithStats(0, 0, 0, 0, 0, 0, 0).
		WithUnit(metricUnitCount).
		WithDimension("total_cost_microcents", "not-a-number").
		Build()
	require.False(t, processor.isHighCost(parseFail))

	nilDims := models.NewMetricRecordBuilder().
		ForService("api").
		OfType(metricEventUser).
		WithAggregationLevel("raw").
		WithTimestamp(time.Unix(0, 0).UTC()).
		WithStats(1, 1, 1, 1, 0, 0, 0).
		WithUnit(metricUnitCount).
		Build()
	nilDims.Dimensions = nil

	metadata := processor.buildMetricsMetadata(nilDims, "general")
	require.NotEmpty(t, metadata)
}

func TestDLQHandler_HandleStreamFailure(t *testing.T) {
	var stored []*models.DLQMessage
	handler := &DLQHandler{
		dlqRepo: stubDLQWriter{
			create: func(_ context.Context, message *models.DLQMessage) error {
				stored = append(stored, message)
				return nil
			},
		},
		logger: zap.NewNop(),
	}

	record := &events.DynamoDBEventRecord{EventID: "evt"}
	require.NoError(t, handler.HandleStreamFailure(context.Background(), record, errors.New("boom")))
	require.Len(t, stored, 1)

	handler.dlqRepo = stubDLQWriter{create: func(_ context.Context, _ *models.DLQMessage) error {
		return errors.New("dlq write failed")
	}}
	require.Error(t, handler.HandleStreamFailure(context.Background(), record, errors.New("boom")))
}

func TestHandler_HandleDynamoDBRecord_FailureSendsToDLQ(t *testing.T) {
	var dlqStored int
	processor := newTestProcessor(stubMetricWriter{
		create: func(_ context.Context, _ *models.MetricRecord) error {
			return errors.New("store failed")
		},
	}, stubDLQWriter{
		create: func(_ context.Context, _ *models.DLQMessage) error {
			dlqStored++
			return nil
		},
	})

	h := &Handler{
		processor: processor,
		repos:     nil,
		logger:    zap.NewNop(),
	}

	record := events.DynamoDBEventRecord{
		EventName:   eventTypeInsert,
		EventID:     "evt1",
		EventSource: "aws:dynamodb",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("USER#admin"),
				"SK": events.NewStringAttribute("METADATA"),
			},
		},
	}

	require.NoError(t, h.HandleDynamoDBRecord(nil, record))
	require.Equal(t, 1, dlqStored)
}

func TestInitializeMetricsProcessor_AndMain(t *testing.T) {
	originalMustInitializeLambdaFn := mustInitializeLambdaFn
	originalLambdaStartFn := lambdaStartFn
	originalRunningUnitTestsFn := runningUnitTestsFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInitializeLambdaFn
		lambdaStartFn = originalLambdaStartFn
		runningUnitTestsFn = originalRunningUnitTestsFn
	})

	repoStorage := mocks.NewMockRepositoryStorage()
	repoStorage.On("GetDB").Return(nil)
	repoStorage.On("GetTableName").Return("test-table")

	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: config.Get(),
			Logger: zap.NewNop(),
			Repos:  repoStorage,
		}
	}
	runningUnitTestsFn = func() bool { return false }

	initializeMetricsProcessorOnStart()
	require.NotNil(t, handler)
	require.NotNil(t, logger)
	require.NotNil(t, repos)

	handler.processor.reportingRepo = stubMetricWriter{}
	handler.processor.dlqHandler.dlqRepo = stubDLQWriter{}

	var (
		started       bool
		startedHandle any
	)
	lambdaStartFn = func(h any) {
		started = true
		startedHandle = h
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()
	require.True(t, started)

	handlerFn, ok := startedHandle.(func(context.Context, json.RawMessage) (any, error))
	require.True(t, ok)

	event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
		{
			EventID:        "evt",
			EventName:      eventTypeInsert,
			EventSource:    "aws:dynamodb",
			EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/lesser-dev-main-table/stream/2024-01-01T00:00:00.000",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("USER#admin"),
					"SK": events.NewStringAttribute("METADATA"),
				},
			},
		},
	}}
	raw, err := json.Marshal(event)
	require.NoError(t, err)

	respAny, err := handlerFn(context.Background(), raw)
	require.NoError(t, err)
	resp, ok := respAny.(events.DynamoDBEventResponse)
	require.True(t, ok)
	require.Empty(t, resp.BatchItemFailures)

	_, ok = any(repos).(core.RepositoryStorage)
	require.True(t, ok)
}

func TestInitializeMetricsProcessor_FailsClosedOnStorageError(t *testing.T) {
	originalMustInitializeLambdaFn := mustInitializeLambdaFn
	originalNewLambdaOptimizedClientFn := newLambdaOptimizedClientFn
	originalNewRepositoryFactoryFn := newRepositoryFactoryFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInitializeLambdaFn
		newLambdaOptimizedClientFn = originalNewLambdaOptimizedClientFn
		newRepositoryFactoryFn = originalNewRepositoryFactoryFn
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{Region: "us-east-1", DynamoTableName: "test-table"},
			Logger: zap.NewNop(),
		}
	}
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return nil, errors.New("storage unavailable")
	}

	err := initializeMetricsProcessor()
	require.Error(t, err)
	require.Contains(t, err.Error(), "storage client initialization failed")
}

func TestInitializeMetricsStorage_CreatesReposAndStoresClient(t *testing.T) {
	originalNewLambdaOptimizedClientFn := newLambdaOptimizedClientFn
	originalNewRepositoryFactoryFn := newRepositoryFactoryFn
	t.Cleanup(func() {
		newLambdaOptimizedClientFn = originalNewLambdaOptimizedClientFn
		newRepositoryFactoryFn = originalNewRepositoryFactoryFn
	})

	db := dynamormmocks.NewMockExtendedDB()
	expectedRepos := mocks.NewMockRepositoryStorage()
	var gotRegion, gotTable string
	newLambdaOptimizedClientFn = func(_ context.Context, region string) (dynamormCore.DB, error) {
		gotRegion = region
		return db, nil
	}
	newRepositoryFactoryFn = func(gotDB dynamormCore.DB, tableName string, _ *zap.Logger) (core.RepositoryStorage, error) {
		require.Same(t, db, gotDB)
		gotTable = tableName
		return expectedRepos, nil
	}

	ctx := &common.LambdaContext{
		Config: &config.Config{Region: "us-east-1", DynamoTableName: "test-table"},
		Logger: zap.NewNop(),
	}
	gotRepos, err := initializeMetricsStorage(ctx)
	require.NoError(t, err)
	require.Same(t, expectedRepos, gotRepos)
	require.Same(t, db, ctx.DynamoDB)
	require.Same(t, expectedRepos, ctx.Repos)
	require.Equal(t, "us-east-1", gotRegion)
	require.Equal(t, "test-table", gotTable)
}
