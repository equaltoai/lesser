package theorydb

import (
	"context"
	stdErrors "errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgtypes "github.com/theory-cloud/tabletheory/pkg/types"
)

func resetClientState() {
	client = nil
	lambdaDB = nil
	clientErr = nil
	clientOnce = sync.Once{}
}

func TestGetClient_UsesDefaultRegionAndLocalEndpointCredentials(t *testing.T) {
	resetClientState()

	origGetConfig := getAppConfig
	origNewClient := newDynamormStandardClient
	t.Cleanup(func() {
		getAppConfig = origGetConfig
		newDynamormStandardClient = origNewClient
		resetClientState()
	})

	getAppConfig = func() *config.Config {
		return &config.Config{
			Region:           "",
			DynamoDBEndpoint: "http://localhost:8000",
		}
	}

	var got session.Config
	calls := 0
	newDynamormStandardClient = func(cfg session.Config) (core.DB, error) {
		calls++
		got = cfg
		return fakeDB{}, nil
	}

	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	db, err := GetClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.Equal(t, 1, calls)
	assert.Equal(t, "us-east-1", got.Region)
	assert.Equal(t, "http://localhost:8000", got.Endpoint)
	assert.Equal(t, "fakeMyKeyId", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "fakeSecretAccessKey", os.Getenv("AWS_SECRET_ACCESS_KEY"))

	// Second call should reuse the singleton.
	db2, err := GetClient(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, db, db2)
}

func TestGetClient_UsesConfiguredRegion(t *testing.T) {
	resetClientState()

	origGetConfig := getAppConfig
	origNewClient := newDynamormStandardClient
	t.Cleanup(func() {
		getAppConfig = origGetConfig
		newDynamormStandardClient = origNewClient
		resetClientState()
	})

	getAppConfig = func() *config.Config {
		return &config.Config{Region: "eu-west-1"}
	}

	var got session.Config
	newDynamormStandardClient = func(cfg session.Config) (core.DB, error) {
		got = cfg
		return fakeDB{}, nil
	}

	_, err := GetClient(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", got.Region)
}

func TestWithTimeoutBuffer_NilAndNonLambdaDB(t *testing.T) {
	assert.Nil(t, WithTimeoutBuffer(nil, 0))

	db := fakeDB{}
	assert.Equal(t, db, WithTimeoutBuffer(db, 0))
}

func TestWithTimeoutBuffer_LambdaDB(t *testing.T) {
	resetClientState()
	t.Cleanup(resetClientState)

	lambdaClient, err := getLambdaOptimizedClient()
	require.NoError(t, err)
	require.NotNil(t, lambdaClient)

	db := WithTimeoutBuffer(lambdaClient, 123*time.Millisecond)
	require.NotNil(t, db)
	assert.Equal(t, 123*time.Millisecond, lambdaTimeoutBufferOf(t, db))
}

func TestRegisterDefaultTypeConverters_UsesRegistrarWithoutExtendedDB(t *testing.T) {
	db := &recordingRegistrarDB{}

	require.NoError(t, registerDefaultTypeConverters(db))

	assert.ElementsMatch(t, []reflect.Type{
		mapStringAnyType,
		sliceAnyType,
		activityPubNoteType,
		activityPubContextValueType,
		agentsCapabilitiesType,
	}, db.registered)
}

func TestGetLambdaClientPreservesDefaultTimeoutBuffer(t *testing.T) {
	resetClientState()
	t.Cleanup(resetClientState)

	deadline := time.Now().Add(30 * time.Second).Round(0)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	db, err := GetLambdaClient(ctx)
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.Equal(t, defaultTimeoutBuffer, lambdaTimeoutBufferOf(t, db))
	assert.Equal(t, deadline, lambdaDeadlineOf(t, db))
}

func TestGetLambdaClientNilContextReturnsBufferedClient(t *testing.T) {
	resetClientState()
	t.Cleanup(resetClientState)

	db, err := GetLambdaClient(nil)
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.Equal(t, defaultTimeoutBuffer, lambdaTimeoutBufferOf(t, db))
	assert.True(t, lambdaDeadlineOf(t, db).IsZero())
}

func TestNewLambdaOptimizedClient_RegistersDefaultTypeConverters(t *testing.T) {
	db, err := NewLambdaOptimizedClient(nil, "us-east-1")
	require.NoError(t, err)
	require.NotNil(t, db)

	assertDefaultTypeConvertersRegistered(t, db)
}

func TestNewLambdaOptimizedClient_TimeoutSurvivesOperationBoundary(t *testing.T) {
	deadline := time.Now().Add(30 * time.Second).Round(0)
	lambdaCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	db, err := NewLambdaOptimizedClient(lambdaCtx, "us-east-1")
	require.NoError(t, err)
	require.NotNil(t, db)

	operationCtx := context.WithValue(context.Background(), testContextKey{}, "repository-operation")
	query := db.WithContext(operationCtx).Model(&lambdaTimeoutProbeModel{
		PK: "PROBE#1",
		SK: "META",
	})

	assert.Equal(t, defaultTimeoutBuffer, queryExecutorLambdaTimeoutBufferOf(t, query))
	assert.Equal(t, deadline, queryExecutorLambdaDeadlineOf(t, query))
}

func TestWithLambdaOptimizedEnvironmentAppliesLocalEndpoint(t *testing.T) {
	errSentinel := stdErrors.New("stop after env inspection")
	t.Setenv("AWS_REGION", "original-region")
	t.Setenv("AWS_ENDPOINT_URL_DYNAMODB", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	_, err := withLambdaOptimizedEnvironment(
		lambdaOptimizedClientOptions{
			Region:   "us-west-2",
			Endpoint: "http://localhost:8000",
		},
		func() (*tabletheory.LambdaDB, error) {
			assert.Equal(t, "us-west-2", os.Getenv("AWS_REGION"))
			assert.Equal(t, "http://localhost:8000", os.Getenv("AWS_ENDPOINT_URL_DYNAMODB"))
			assert.Equal(t, "fakeMyKeyId", os.Getenv("AWS_ACCESS_KEY_ID"))
			assert.Equal(t, "fakeSecretAccessKey", os.Getenv("AWS_SECRET_ACCESS_KEY"))
			return nil, errSentinel
		},
	)

	require.ErrorIs(t, err, errSentinel)
	assert.Equal(t, "original-region", os.Getenv("AWS_REGION"))
	assert.Empty(t, os.Getenv("AWS_ENDPOINT_URL_DYNAMODB"))
	assert.Equal(t, "fakeMyKeyId", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "fakeSecretAccessKey", os.Getenv("AWS_SECRET_ACCESS_KEY"))
}

func lambdaTimeoutBufferOf(t *testing.T, db core.DB) time.Duration {
	t.Helper()

	field := tableTheoryDBField(t, db, "lambdaTimeoutBuffer")
	return time.Duration(field.Int())
}

func lambdaDeadlineOf(t *testing.T, db core.DB) time.Time {
	t.Helper()

	field := tableTheoryDBField(t, db, "lambdaDeadline")
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(time.Time)
}

func tableTheoryDBField(t *testing.T, db core.DB, name string) reflect.Value {
	t.Helper()

	value := reflect.ValueOf(db)
	require.Equal(t, reflect.Ptr, value.Kind())

	elem := value.Elem()
	field := elem.FieldByName(name)
	if field.IsValid() {
		return field
	}

	inner := elem.FieldByName("db")
	require.True(t, inner.IsValid(), "expected returned Lambda client to expose %s", name)
	require.Equal(t, reflect.Ptr, inner.Kind())
	require.False(t, inner.IsNil())

	innerValue := reflect.NewAt(inner.Type(), unsafe.Pointer(inner.UnsafeAddr())).Elem()
	field = innerValue.Elem().FieldByName(name)
	require.True(t, field.IsValid(), "expected returned Lambda client DB to expose %s", name)
	return field
}

func assertDefaultTypeConvertersRegistered(t *testing.T, db core.DB) {
	t.Helper()

	field := tableTheoryDBField(t, db, "converter")
	converter := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()

	hasCustomConverter, ok := converter.(interface{ HasCustomConverter(reflect.Type) bool })
	require.True(t, ok, "expected TableTheory converter to expose HasCustomConverter")

	for _, typ := range []reflect.Type{
		mapStringAnyType,
		sliceAnyType,
		activityPubNoteType,
		activityPubContextValueType,
		agentsCapabilitiesType,
	} {
		assert.True(t, hasCustomConverter.HasCustomConverter(typ), "expected custom converter for %s", typ)
	}
}

func queryExecutorLambdaTimeoutBufferOf(t *testing.T, query core.Query) time.Duration {
	t.Helper()

	field := queryExecutorDBField(t, query, "lambdaTimeoutBuffer")
	return time.Duration(field.Int())
}

func queryExecutorLambdaDeadlineOf(t *testing.T, query core.Query) time.Time {
	t.Helper()

	field := queryExecutorDBField(t, query, "lambdaDeadline")
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(time.Time)
}

func queryExecutorDBField(t *testing.T, query core.Query, name string) reflect.Value {
	t.Helper()

	value := reflect.ValueOf(query)
	require.Equal(t, reflect.Ptr, value.Kind())
	elem := value.Elem()

	executorField := elem.FieldByName("executor")
	require.True(t, executorField.IsValid(), "expected TableTheory query to expose executor")

	executorValue := reflect.NewAt(executorField.Type(), unsafe.Pointer(executorField.UnsafeAddr())).Elem()
	require.False(t, executorValue.IsNil(), "expected TableTheory query executor")

	executor := reflect.ValueOf(executorValue.Interface())
	require.Equal(t, reflect.Ptr, executor.Kind())
	executorElem := executor.Elem()

	dbField := executorElem.FieldByName("db")
	require.True(t, dbField.IsValid(), "expected TableTheory query executor DB")
	require.Equal(t, reflect.Ptr, dbField.Kind())
	require.False(t, dbField.IsNil())

	dbValue := reflect.NewAt(dbField.Type(), unsafe.Pointer(dbField.UnsafeAddr())).Elem()
	field := dbValue.Elem().FieldByName(name)
	require.True(t, field.IsValid(), "expected query executor DB to expose %s", name)
	return field
}

type recordingRegistrarDB struct {
	fakeDB
	registered []reflect.Type
}

func (db *recordingRegistrarDB) RegisterTypeConverter(typ reflect.Type, _ pkgtypes.CustomConverter) error {
	db.registered = append(db.registered, typ)
	return nil
}

type testContextKey struct{}

type lambdaTimeoutProbeModel struct {
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`
}

func (*lambdaTimeoutProbeModel) TableName() string { return "lambda_timeout_probe" }
