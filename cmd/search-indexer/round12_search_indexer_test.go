package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

type fakeSearchCostRepo struct {
	calls int
	err   error
}

func (f *fakeSearchCostRepo) RecordSearchCost(context.Context, *models.SearchCostTracking) error {
	f.calls++
	return f.err
}

type fakeUnifiedTracker struct {
	calls int
	err   error
}

func (f *fakeUnifiedTracker) TrackDynamoWrite(context.Context, string, int64) error {
	f.calls++
	return f.err
}

func setField(out any, name string, value any) {
	v := reflect.ValueOf(out)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	f := elem.FieldByName(name)
	if !f.IsValid() || !f.CanSet() {
		return
	}
	switch f.Kind() {
	case reflect.String:
		if s, ok := value.(string); ok {
			f.SetString(s)
		}
	case reflect.Slice:
		f.Set(reflect.ValueOf(value))
	}
}

func TestInitializeSearchIndexer_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origNewClient := newLambdaOptimizedClientFn
	origNewRepo := unmarshalItemFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		newLambdaOptimizedClientFn = origNewClient
		unmarshalItemFn = origNewRepo
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Region:          "us-east-1",
				DynamoTableName: "test-table",
			},
			Logger: zap.NewNop(),
		}
	}

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return new(dynamormmocks.MockDB), nil
	}

	unmarshalItemFn = func(events.DynamoDBEventRecord, any) error { return nil }

	initializeSearchIndexer()

	require.NotNil(t, lambdaCtx)
	require.NotNil(t, processor)
	require.NotNil(t, db)
}

func TestSearchIndexer_isIndexableRecord_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error {
		setField(out, "PK", "OBJECT#1")
		setField(out, "Type", "Note")
		setField(out, "Content", "hello")
		return nil
	}

	si := &SearchIndexer{logger: zap.NewNop()}
	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#1")}},
	}
	require.True(t, si.isIndexableRecord(record))

	record.EventName = "REMOVE"
	require.False(t, si.isIndexableRecord(record))

	record.EventName = "INSERT"
	record.Change.NewImage = nil
	require.False(t, si.isIndexableRecord(record))

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error {
		setField(out, "PK", "OBJECT#1")
		setField(out, "Type", "Note")
		setField(out, "Content", "")
		return nil
	}
	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#1")}
	require.False(t, si.isIndexableRecord(record))

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error {
		setField(out, "PK", "ACTOR#1")
		setField(out, "Type", "Person")
		setField(out, "Content", "")
		return nil
	}
	require.True(t, si.isIndexableRecord(record))

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error {
		setField(out, "PK", "STATUS#1")
		setField(out, "Type", "Other")
		setField(out, "Content", "")
		return nil
	}
	require.True(t, si.isIndexableRecord(record))

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error { return errors.New("boom") }
	require.False(t, si.isIndexableRecord(record))
}

func TestSearchIndexer_extractIndexableContent_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	unmarshalItemFn = func(record events.DynamoDBEventRecord, out any) error {
		switch record.EventID {
		case "object":
			setField(out, "PK", "OBJECT#o1")
		case "status":
			setField(out, "PK", "STATUS#s1")
		case "actor":
			setField(out, "PK", "ACTOR#a1")
		default:
			setField(out, "PK", "other")
		}
		setField(out, "Type", "Note")
		setField(out, "Content", "content")
		setField(out, "Summary", "summary")
		setField(out, "Name", "name")
		setField(out, "ActorID", "")
		setField(out, "Language", "en")
		setField(out, "CreatedAt", "2024-01-01T00:00:00Z")
		return nil
	}

	si := &SearchIndexer{logger: zap.NewNop()}
	content, err := si.extractIndexableContent(events.DynamoDBEventRecord{EventID: "object"})
	require.NoError(t, err)
	require.Equal(t, "o1", content.ID)
	require.Contains(t, content.Text, "content")
	require.Contains(t, content.Text, "summary")
	require.Contains(t, content.Text, "name")
	require.False(t, content.CreatedAt.IsZero())

	content, err = si.extractIndexableContent(events.DynamoDBEventRecord{EventID: "status"})
	require.NoError(t, err)
	require.Equal(t, "s1", content.ID)

	content, err = si.extractIndexableContent(events.DynamoDBEventRecord{EventID: "actor"})
	require.NoError(t, err)
	require.Equal(t, "a1", content.ID)

	content, err = si.extractIndexableContent(events.DynamoDBEventRecord{EventID: "other"})
	require.NoError(t, err)
	require.Equal(t, "other", content.ID)

	unmarshalItemFn = func(events.DynamoDBEventRecord, any) error { return errors.New("boom") }
	_, err = si.extractIndexableContent(events.DynamoDBEventRecord{})
	require.Error(t, err)
}

func TestSearchIndexer_createSearchIndex_AndAdditionalIndexes_Round12(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Create").Return(nil).Once()                // main index
	mockQuery.On("Create").Return(errors.New("boom")).Once() // actor index fails (warned, non-fatal)
	mockQuery.On("Create").Return(nil).Maybe()               // remaining writes

	si := &SearchIndexer{
		db:        mockDB,
		tableName: "test-table",
		logger:    zap.NewNop(),
	}

	content := &IndexableContent{
		ID:        "c1",
		Type:      "Note",
		Text:      "hello world",
		ActorID:   "u1",
		Tags:      []string{},
		Language:  "en",
		CreatedAt: time.Now(),
	}

	require.NoError(t, si.createSearchIndex(context.Background(), content))
}

func TestSearchIndexer_createAdditionalIndexes_TagCreateError_Round12(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(errors.New("boom")).Maybe()

	si := &SearchIndexer{
		db:        mockDB,
		tableName: "test-table",
		logger:    zap.NewNop(),
	}

	content := &IndexableContent{
		ID:        "c1",
		Type:      "Note",
		Text:      "hello",
		ActorID:   "",
		Tags:      []string{"Go", "", "Tag"},
		Language:  "en",
		CreatedAt: time.Now(),
	}

	require.NoError(t, si.createAdditionalIndexes(context.Background(), content))
}

func TestSearchIndexer_HandleDynamoDBRecord_ErrorAndSuccess_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	origAsync := runAsyncFn
	t.Cleanup(func() {
		unmarshalItemFn = origUnmarshal
		runAsyncFn = origAsync
	})

	runAsyncFn = func(fn func()) { fn() }

	unmarshalItemFn = func(record events.DynamoDBEventRecord, out any) error {
		setField(out, "PK", "OBJECT#"+record.EventID)
		setField(out, "SK", "METADATA")
		setField(out, "Type", "Note")
		setField(out, "Content", "hello")
		setField(out, "Summary", "")
		setField(out, "Name", "")
		setField(out, "ActorID", "")
		setField(out, "Tags", []string{})
		setField(out, "CreatedAt", "2024-01-01T00:00:00Z")
		return nil
	}

	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	mockQuery.On("Create").Return(nil).Maybe()

	costRepo := &fakeSearchCostRepo{err: errors.New("boom")}
	tracker := &fakeUnifiedTracker{err: errors.New("boom")}
	si := &SearchIndexer{
		db:             mockDB,
		tableName:      "test-table",
		logger:         zap.NewNop(),
		costRepo:       costRepo,
		unifiedTracker: tracker,
	}

	ctx := &apptheory.EventContext{RequestID: "req"}

	err := si.HandleDynamoDBRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "1",
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#1")}},
	})
	require.Error(t, err)
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))

	require.NoError(t, si.HandleDynamoDBRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "2",
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#2")}},
	}))
	require.GreaterOrEqual(t, costRepo.calls, 2)
	require.GreaterOrEqual(t, tracker.calls, 2)
}

func TestSearchIndexer_calculateIndexingCost_Round12(t *testing.T) {
	si := &SearchIndexer{}
	require.Equal(t, int64(0), si.calculateIndexingCost(0))
	require.Equal(t, int64(0), si.calculateIndexingCost(1))
	require.Greater(t, si.calculateIndexingCost(1_000_000), int64(0))
}

func TestRunSearchIndexer_Round12(t *testing.T) {
	origStart := lambdaStartFn
	origAsync := runAsyncFn
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() {
		lambdaStartFn = origStart
		runAsyncFn = origAsync
		unmarshalItemFn = origUnmarshal
	})

	runAsyncFn = func(fn func()) { fn() }
	unmarshalItemFn = func(record events.DynamoDBEventRecord, out any) error {
		setField(out, "PK", "OBJECT#"+record.EventID)
		setField(out, "SK", "METADATA")
		setField(out, "Type", "Note")
		setField(out, "Content", "hello")
		setField(out, "Summary", "")
		setField(out, "Name", "")
		setField(out, "ActorID", "")
		setField(out, "Tags", []string{})
		setField(out, "CreatedAt", "2024-01-01T00:00:00Z")
		return nil
	}

	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()

	processor = &SearchIndexer{
		db:             mockDB,
		tableName:      "test-table",
		logger:         zap.NewNop(),
		costRepo:       &fakeSearchCostRepo{},
		unifiedTracker: &fakeUnifiedTracker{},
	}

	called := false
	lambdaStartFn = func(handler any) {
		called = true
		fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
			{
				EventID:        "1",
				EventName:      "INSERT",
				EventSource:    "aws:dynamodb",
				EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/lesser-dev-main-table/stream/2024-01-01T00:00:00.000",
				Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("OBJECT#1"),
				}},
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
