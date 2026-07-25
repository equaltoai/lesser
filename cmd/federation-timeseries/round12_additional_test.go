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
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

type fakeQuery struct {
	db *fakeDynamoDB
}

type fakeWhereCall struct {
	field string
	op    string
	value any
}

func (f *fakeQuery) Where(field string, op string, value any) dynamormCore.Query {
	f.db.wheres = append(f.db.wheres, fakeWhereCall{field: field, op: op, value: value})
	return f
}
func (f *fakeQuery) Index(string) dynamormCore.Query                                   { return f }
func (f *fakeQuery) Filter(string, string, any) dynamormCore.Query                     { return f }
func (f *fakeQuery) OrFilter(string, string, any) dynamormCore.Query                   { return f }
func (f *fakeQuery) FilterGroup(func(dynamormCore.Query)) dynamormCore.Query           { return f }
func (f *fakeQuery) OrFilterGroup(func(dynamormCore.Query)) dynamormCore.Query         { return f }
func (f *fakeQuery) IfNotExists() dynamormCore.Query                                   { return f }
func (f *fakeQuery) IfExists() dynamormCore.Query                                      { return f }
func (f *fakeQuery) WithCondition(string, string, any) dynamormCore.Query              { return f }
func (f *fakeQuery) WithConditionExpression(string, map[string]any) dynamormCore.Query { return f }
func (f *fakeQuery) OrderBy(string, string) dynamormCore.Query                         { return f }
func (f *fakeQuery) Limit(int) dynamormCore.Query                                      { return f }
func (f *fakeQuery) Offset(int) dynamormCore.Query                                     { return f }
func (f *fakeQuery) Select(...string) dynamormCore.Query                               { return f }
func (f *fakeQuery) ConsistentRead() dynamormCore.Query                                { return f }
func (f *fakeQuery) WithRetry(int, time.Duration) dynamormCore.Query                   { return f }
func (f *fakeQuery) First(any) error                                                   { return nil }
func (f *fakeQuery) All(any) error                                                     { return nil }
func (f *fakeQuery) AllPaginated(any) (*dynamormCore.PaginatedResult, error) {
	return &dynamormCore.PaginatedResult{}, nil
}
func (f *fakeQuery) Count() (int64, error) { return 0, nil }

func (f *fakeQuery) Create() error {
	f.db.createCalls++
	if f.db.failCreatesAfterFirst && f.db.createCalls > 1 {
		return errors.New("create failed")
	}
	return f.db.createErr
}

func (f *fakeQuery) CreateOrUpdate() error                        { return nil }
func (f *fakeQuery) Update(...string) error                       { return nil }
func (f *fakeQuery) UpdateBuilder() dynamormCore.UpdateBuilder    { return &fakeUpdateBuilder{db: f.db} }
func (f *fakeQuery) Delete() error                                { return nil }
func (f *fakeQuery) Scan(any) error                               { return nil }
func (f *fakeQuery) ParallelScan(int32, int32) dynamormCore.Query { return f }
func (f *fakeQuery) ScanAllSegments(any, int32) error             { return nil }
func (f *fakeQuery) BatchGet([]any, any) error                    { return nil }
func (f *fakeQuery) BatchGetWithOptions([]any, any, *dynamormCore.BatchGetOptions) error {
	return nil
}
func (f *fakeQuery) BatchGetBuilder() dynamormCore.BatchGetBuilder        { return nil }
func (f *fakeQuery) BatchCreate(any) error                                { return nil }
func (f *fakeQuery) BatchDelete([]any) error                              { return nil }
func (f *fakeQuery) BatchWrite([]any, []any) error                        { return nil }
func (f *fakeQuery) BatchUpdateWithOptions([]any, []string, ...any) error { return nil }
func (f *fakeQuery) Cursor(string) dynamormCore.Query                     { return f }
func (f *fakeQuery) SetCursor(string) error                               { return nil }
func (f *fakeQuery) WithContext(context.Context) dynamormCore.Query       { return f }

type fakeDynamoDB struct {
	createCalls           int
	createErr             error
	failCreatesAfterFirst bool
	updateCalls           int
	updateErr             error
	failUpdatesAfterFirst bool
	adds                  map[string]int64
	wheres                []fakeWhereCall
}

func (f *fakeDynamoDB) Model(any) dynamormCore.Query                { return &fakeQuery{db: f} }
func (f *fakeDynamoDB) Migrate() error                              { return nil }
func (f *fakeDynamoDB) AutoMigrate(...any) error                    { return nil }
func (f *fakeDynamoDB) Close() error                                { return nil }
func (f *fakeDynamoDB) WithContext(context.Context) dynamormCore.DB { return f }

type fakeUpdateBuilder struct {
	db *fakeDynamoDB
}

func (f *fakeUpdateBuilder) Set(string, any) dynamormCore.UpdateBuilder { return f }
func (f *fakeUpdateBuilder) SetIfNotExists(string, any, any) dynamormCore.UpdateBuilder {
	return f
}
func (f *fakeUpdateBuilder) Add(field string, value any) dynamormCore.UpdateBuilder {
	if f.db.adds == nil {
		f.db.adds = make(map[string]int64)
	}
	switch v := value.(type) {
	case int:
		f.db.adds[field] += int64(v)
	case int64:
		f.db.adds[field] += v
	}
	return f
}
func (f *fakeUpdateBuilder) AddAll(string, any) dynamormCore.UpdateBuilder              { return f }
func (f *fakeUpdateBuilder) Increment(string) dynamormCore.UpdateBuilder                { return f }
func (f *fakeUpdateBuilder) Decrement(string) dynamormCore.UpdateBuilder                { return f }
func (f *fakeUpdateBuilder) Remove(string) dynamormCore.UpdateBuilder                   { return f }
func (f *fakeUpdateBuilder) Delete(string, any) dynamormCore.UpdateBuilder              { return f }
func (f *fakeUpdateBuilder) AppendToList(string, any) dynamormCore.UpdateBuilder        { return f }
func (f *fakeUpdateBuilder) PrependToList(string, any) dynamormCore.UpdateBuilder       { return f }
func (f *fakeUpdateBuilder) RemoveFromListAt(string, int) dynamormCore.UpdateBuilder    { return f }
func (f *fakeUpdateBuilder) SetListElement(string, int, any) dynamormCore.UpdateBuilder { return f }
func (f *fakeUpdateBuilder) Condition(string, string, any) dynamormCore.UpdateBuilder   { return f }
func (f *fakeUpdateBuilder) OrCondition(string, string, any) dynamormCore.UpdateBuilder { return f }
func (f *fakeUpdateBuilder) ConditionExists(string) dynamormCore.UpdateBuilder          { return f }
func (f *fakeUpdateBuilder) ConditionNotExists(string) dynamormCore.UpdateBuilder       { return f }
func (f *fakeUpdateBuilder) ConditionVersion(int64) dynamormCore.UpdateBuilder          { return f }
func (f *fakeUpdateBuilder) ReturnValues(string) dynamormCore.UpdateBuilder             { return f }
func (f *fakeUpdateBuilder) Execute() error {
	f.db.updateCalls++
	if f.db.failUpdatesAfterFirst && f.db.updateCalls > 1 {
		return errors.New("update failed")
	}
	return f.db.updateErr
}
func (f *fakeUpdateBuilder) ExecuteWithResult(any) error { return f.Execute() }

func TestInitializeFederationTimeseries_WiresGlobals(t *testing.T) {
	origMustInit := mustInitializeLambdaFn
	origInitDefaults := initializeWithDefaultsFn
	origGetClient := dynamormGetClientFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInit
		initializeWithDefaultsFn = origInitDefaults
		dynamormGetClientFn = origGetClient
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:     &config.Config{DynamoTableName: "table"},
			Logger:     zap.NewNop(),
			LambdaType: common.LambdaTypeProcessor,
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("defaults failed") }

	db := &fakeDynamoDB{}
	dynamormGetClientFn = func(context.Context) (dynamormCore.DB, error) { return db, nil }

	initializeFederationTimeseries()

	require.NotNil(t, lambdaCtx)
	require.NotNil(t, processor)
	require.Equal(t, "table", processor.tableName)
}

func TestInitializeFederationTimeseries_UsesNopLoggerWhenNil(t *testing.T) {
	origMustInit := mustInitializeLambdaFn
	origInitDefaults := initializeWithDefaultsFn
	origGetClient := dynamormGetClientFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInit
		initializeWithDefaultsFn = origInitDefaults
		dynamormGetClientFn = origGetClient
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:     &config.Config{DynamoTableName: "table"},
			Logger:     nil,
			LambdaType: common.LambdaTypeProcessor,
			Repos:      "not a repository storage",
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return nil }
	dynamormGetClientFn = func(context.Context) (dynamormCore.DB, error) { return &fakeDynamoDB{}, nil }

	initializeFederationTimeseries()
	require.NotNil(t, logger)
	require.NotNil(t, processor)
}

func TestTimeseriesProcessor_StoreMetrics(t *testing.T) {
	ctx := context.Background()
	requestID := "req"

	t.Run("returns error when base record store fails", func(t *testing.T) {
		db := &fakeDynamoDB{updateErr: errors.New("nope")}
		tp := &TimeseriesProcessor{db: db, tableName: "table", logger: zap.NewNop()}
		err := tp.storeMetrics(ctx, requestID, time.Unix(0, 0).UTC(), &FederationMetrics{
			UniqueActors:    map[string]bool{},
			UniqueInstances: map[string]bool{},
		})
		require.Error(t, err)
	})

	t.Run("logs and continues on per-instance store failures", func(t *testing.T) {
		db := &fakeDynamoDB{failUpdatesAfterFirst: true}
		tp := &TimeseriesProcessor{db: db, tableName: "table", logger: zap.NewNop()}

		metrics := &FederationMetrics{
			FollowCount:     1,
			LikeCount:       1,
			AnnounceCount:   0,
			ActivityCount:   0,
			UniqueActors:    map[string]bool{"actor": true},
			UniqueInstances: map[string]bool{"example.com": true, "remote.example": true},
		}

		require.NoError(t, tp.storeMetrics(ctx, requestID, time.Unix(0, 0).UTC(), metrics))
		require.GreaterOrEqual(t, db.updateCalls, 1)
	})

	t.Run("adds counts into the current window instead of overwriting", func(t *testing.T) {
		db := &fakeDynamoDB{}
		tp := &TimeseriesProcessor{db: db, tableName: "table", logger: zap.NewNop()}

		metrics := &FederationMetrics{
			FollowCount:     2,
			LikeCount:       3,
			AnnounceCount:   4,
			ActivityCount:   5,
			UniqueActors:    map[string]bool{"actor-1": true, "actor-2": true},
			UniqueInstances: map[string]bool{"example.com": true},
		}

		require.NoError(t, tp.storeMetrics(ctx, requestID, time.Unix(0, 0).UTC(), metrics))
		require.Equal(t, int64(2), db.adds["FollowCount"])
		require.Equal(t, int64(3), db.adds["LikeCount"])
		require.Equal(t, int64(4), db.adds["AnnounceCount"])
		require.Equal(t, int64(5), db.adds["ActivityCount"])
		require.Equal(t, int64(2), db.adds["UniqueActorCount"])
		require.Equal(t, int64(1), db.adds["UniqueInstanceCount"])
		require.Len(t, db.wheres, 4)
		require.Equal(t, fakeWhereCall{field: "PK", op: "=", value: "TIMESERIES#FEDERATION"}, db.wheres[0])
		require.Equal(t, fakeWhereCall{field: "SK", op: "=", value: "WINDOW#1970-01-01T00:00:00Z"}, db.wheres[1])
		require.Equal(t, fakeWhereCall{field: "PK", op: "=", value: "TIMESERIES#INSTANCE#example.com"}, db.wheres[2])
		require.Equal(t, fakeWhereCall{field: "SK", op: "=", value: "WINDOW#1970-01-01T00:00:00Z"}, db.wheres[3])
	})
}

func TestTimeseriesProcessor_GroupingAndAggregation_Branches(t *testing.T) {
	tp := &TimeseriesProcessor{logger: zap.NewNop()}

	mkRecord := func(pk, typ, createdAt, timestamp, updatedAt string) events.DynamoDBEventRecord {
		image := map[string]events.DynamoDBAttributeValue{
			"PK":   events.NewStringAttribute(pk),
			"type": events.NewStringAttribute(typ),
		}
		if createdAt != "" {
			image["created_at"] = events.NewStringAttribute(createdAt)
		}
		if timestamp != "" {
			image["timestamp"] = events.NewStringAttribute(timestamp)
		}
		if updatedAt != "" {
			image["updated_at"] = events.NewStringAttribute(updatedAt)
		}
		if strings.Contains(pk, "ACTIVITY#") {
			image["actor_id"] = events.NewStringAttribute("https://remote.example/users/alice")
		}
		return events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change:    events.DynamoDBStreamRecord{NewImage: image},
		}
	}

	require.False(t, tp.isFederationRecord(events.DynamoDBEventRecord{EventName: "INSERT"}))
	require.False(t, tp.isFederationRecord(events.DynamoDBEventRecord{
		EventName: "UNKNOWN",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("ACTOR#1")}},
	}))
	require.True(t, tp.isFederationRecord(mkRecord("ACTOR#1", "Person", "2024-01-01T00:01:00Z", "", "")))
	require.True(t, tp.isFederationRecord(mkRecord("OTHER#1", "Like", "2024-01-01T00:01:00Z", "", "")))

	require.True(t, tp.extractTimestamp(mkRecord("ACTOR#1", "Person", "", "", "")).IsZero())
	require.Equal(t,
		time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC),
		tp.extractTimestamp(mkRecord("ACTOR#1", "Person", "bad", "2024-01-01T00:01:00Z", "")),
	)

	records := []events.DynamoDBEventRecord{
		mkRecord("OBJECT#1", "Note", "2024-01-01T00:01:00Z", "", ""), // non-federation
		mkRecord("OTHER#2", "Follow", "", "", ""),                    // federation, but no timestamp
		mkRecord("ACTIVITY#1", "Follow", "2024-01-01T00:01:00Z", "", ""),
		mkRecord("ACTIVITY#2", "Announce", "2024-01-01T00:06:00Z", "", ""),
	}

	metrics := tp.aggregateMetrics(records)
	require.Equal(t, 2, metrics.FollowCount)
	require.Equal(t, 0, metrics.LikeCount)
	require.Equal(t, 1, metrics.AnnounceCount)
	require.Equal(t, 1, metrics.ActivityCount)
	require.GreaterOrEqual(t, len(metrics.UniqueInstances), 1)

	require.Equal(t, "remote.example", tp.extractInstance("https://remote.example/users/alice"))
	require.Equal(t, "", tp.extractInstance("http://remote.example/users/alice"))
}

func TestTimeseriesProcessor_HandleDynamoDBRecord_LogsAndContinuesOnWindowErrors(t *testing.T) {
	db := &fakeDynamoDB{createErr: errors.New("nope")}
	tp := &TimeseriesProcessor{db: db, tableName: "table", logger: zap.NewNop()}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":         events.NewStringAttribute("ACTIVITY#1"),
				"type":       events.NewStringAttribute("Follow"),
				"actor_id":   events.NewStringAttribute("https://remote.example/users/alice"),
				"created_at": events.NewStringAttribute("2024-01-01T00:01:00Z"),
			},
		},
	}

	require.NoError(t, tp.HandleDynamoDBRecord(&apptheory.EventContext{RequestID: "req"}, record))
}

func TestHandleFederationTimeseriesStreamRecord_DecodeAndDispatch(t *testing.T) {
	origProcessor := processor
	t.Cleanup(func() { processor = origProcessor })

	db := &fakeDynamoDB{}
	processor = &TimeseriesProcessor{db: db, tableName: "table", logger: zap.NewNop()}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":         events.NewStringAttribute("ACTIVITY#1"),
				"type":       events.NewStringAttribute("Follow"),
				"actor_id":   events.NewStringAttribute("https://remote.example/users/alice"),
				"created_at": events.NewStringAttribute("2024-01-01T00:01:00Z"),
			},
		},
	}

	require.NoError(t, handleFederationTimeseriesStreamRecord(&apptheory.EventContext{RequestID: "req"}, record))
	require.GreaterOrEqual(t, db.updateCalls, 1)
}

func TestMain_InvokesLambdaStart(t *testing.T) {
	origStart := lambdaStartFn
	origProcessor := processor
	t.Cleanup(func() { lambdaStartFn = origStart })
	t.Cleanup(func() { processor = origProcessor })

	processor = &TimeseriesProcessor{logger: zap.NewNop()}

	var called bool
	lambdaStartFn = func(h any) {
		called = true
		fn, ok := h.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
			{
				EventID:        "evt1",
				EventName:      "INSERT",
				EventSource:    "aws:dynamodb",
				EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/lesser-dev-main-table/stream/2024-01-01T00:00:00.000",
				Change:         events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}},
			},
		}}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := fn(context.Background(), raw)
		require.NoError(t, err)
		resp, ok := respAny.(events.DynamoDBEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()
	require.True(t, called)
}

func TestTimeseriesProcessor_HandleDynamoDBRecord_NilAndNoopBranches(t *testing.T) {
	var nilProcessor *TimeseriesProcessor
	require.ErrorContains(t, nilProcessor.HandleDynamoDBRecord(nil, events.DynamoDBEventRecord{}), "timeseries processor is nil")

	tp := &TimeseriesProcessor{db: &fakeDynamoDB{}, tableName: "table"}
	require.NoError(t, tp.HandleDynamoDBRecord(nil, events.DynamoDBEventRecord{}))
	require.NotNil(t, tp.logger)
}
