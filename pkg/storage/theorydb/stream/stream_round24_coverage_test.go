package stream

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round24Stringer string

func (s round24Stringer) String() string { return string(s) }

type round24NeedsStringer interface {
	fmt.Stringer
}

func TestDefaultProcessingConfig_Round24(t *testing.T) {
	cfg := DefaultProcessingConfig()
	require.NotNil(t, cfg)
	require.True(t, cfg.EnableMetrics)
	require.True(t, cfg.EnableErrorRecovery)
	require.GreaterOrEqual(t, cfg.MaxRetryAttempts, 1)
	require.True(t, cfg.ParallelProcessing)
	require.Greater(t, cfg.MaxConcurrentRecords, 0)
}

func TestNewProcessor_DefaultsAndOverrides_Round24(t *testing.T) {
	p := NewProcessor(nil, nil)
	require.NotNil(t, p)
	require.NotNil(t, p.config)
	require.NotNil(t, p.logger)

	cfg := &ProcessingConfig{
		EnableMetrics:        false,
		EnableErrorRecovery:  false,
		MaxRetryAttempts:     0,
		RetryBackoffInitial:  0,
		RetryBackoffMax:      0,
		EnableDLQ:            false,
		ParallelProcessing:   false,
		MaxConcurrentRecords: 1,
	}
	logger := zap.NewNop()
	p2 := NewProcessor(cfg, logger)
	require.Same(t, cfg, p2.config)
	require.Same(t, logger, p2.logger)
}

func TestProcessor_ProcessStreamRecordsWithRetry_EmitsMetrics_Round24(t *testing.T) {
	p := NewProcessor(&ProcessingConfig{
		EnableMetrics:        true,
		EnableErrorRecovery:  false,
		MaxRetryAttempts:     0,
		RetryBackoffInitial:  0,
		RetryBackoffMax:      0,
		EnableDLQ:            false,
		ParallelProcessing:   false,
		MaxConcurrentRecords: 1,
	}, zap.NewNop())

	records := []events.DynamoDBEventRecord{
		{EventID: "ok", EventName: eventNameInsert},
	}

	require.NoError(t, p.ProcessStreamRecordsWithRetry(context.Background(), records, func(context.Context, events.DynamoDBEventRecord) error {
		return nil
	}))
}

func TestProcessor_processRecordsParallel_UsesContextCancellation_Round24(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := NewProcessor(&ProcessingConfig{
		EnableMetrics:        false,
		EnableErrorRecovery:  false,
		MaxRetryAttempts:     0,
		RetryBackoffInitial:  0,
		RetryBackoffMax:      0,
		EnableDLQ:            false,
		ParallelProcessing:   true,
		MaxConcurrentRecords: 0, // exercises maxConcurrent <= 0 branch
	}, zap.NewNop())

	records := []events.DynamoDBEventRecord{
		{EventID: "a", EventName: eventNameInsert},
		{EventID: "b", EventName: eventNameInsert},
	}

	handlerEntered := make(chan struct{})
	handler := func(ctx context.Context, _ events.DynamoDBEventRecord) error {
		select {
		case <-handlerEntered:
		default:
			close(handlerEntered)
		}
		<-ctx.Done()
		return ctx.Err()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.processRecordsParallel(ctx, records, handler, new(int), new(int))
	}()

	<-handlerEntered
	cancel()

	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestProcessor_processRecordWithRetry_SkipsUnknownEvents_Round24(t *testing.T) {
	p := NewProcessor(&ProcessingConfig{
		EnableMetrics:        false,
		EnableErrorRecovery:  false,
		MaxRetryAttempts:     3,
		RetryBackoffInitial:  0,
		RetryBackoffMax:      0,
		EnableDLQ:            false,
		ParallelProcessing:   false,
		MaxConcurrentRecords: 1,
	}, zap.NewNop())

	called := 0
	handler := func(context.Context, events.DynamoDBEventRecord) error {
		called++
		return nil
	}

	require.NoError(t, p.processRecordWithRetry(context.Background(), events.DynamoDBEventRecord{EventName: "BOGUS"}, handler))
	require.Equal(t, 0, called)
}

func TestProcessor_applyBackoff_Round24(t *testing.T) {
	p := NewProcessor(&ProcessingConfig{
		EnableMetrics:        false,
		EnableErrorRecovery:  false,
		MaxRetryAttempts:     0,
		RetryBackoffInitial:  0,
		RetryBackoffMax:      0,
		EnableDLQ:            false,
		ParallelProcessing:   false,
		MaxConcurrentRecords: 1,
	}, zap.NewNop())

	p.applyBackoff(1)
	p.applyBackoff(1000) // exercises safeAttempt clamp
}

func Test_getFieldName_Round24(t *testing.T) {
	type model struct {
		A string `theorydb:"pk"`
		B string `theorydb:"foo"`
		C string `theorydb:"index:gsi1"`
		D string `theorydb:"pk,sk"`
		E string `json:"name,omitempty"`
		F string `json:"-"`
	}

	typ := reflect.TypeOf(model{})
	require.Equal(t, "A", getFieldName(typ.Field(0)))
	require.Equal(t, "foo", getFieldName(typ.Field(1)))
	require.Equal(t, "C", getFieldName(typ.Field(2)))
	require.Equal(t, "D", getFieldName(typ.Field(3)))
	require.Equal(t, "name", getFieldName(typ.Field(4)))
	require.Equal(t, "F", getFieldName(typ.Field(5)))
}

func Test_tryDirectAssignmentEnhanced_NilValue_Round24(t *testing.T) {
	var s string
	require.False(t, tryDirectAssignmentEnhanced(reflect.ValueOf(&s).Elem(), nil))
}

func Test_setFieldValueEnhanced_StringConversions_Round24(t *testing.T) {
	var s string
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&s).Elem(), []byte("bytes"), reflect.StructField{}))
	require.Equal(t, "bytes", s)

	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&s).Elem(), round24Stringer("stringer"), reflect.StructField{}))
	require.Equal(t, "stringer", s)

	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&s).Elem(), 123, reflect.StructField{}))
}

func Test_setFieldValueEnhanced_InterfaceAssignment_Round24(t *testing.T) {
	var anyField any
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&anyField).Elem(), "ok", reflect.StructField{}))
	require.Equal(t, "ok", anyField)

	var needs round24NeedsStringer
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&needs).Elem(), 123, reflect.StructField{}))
}

func Test_setFieldValueEnhanced_NumericConversions_Round24(t *testing.T) {
	var i8 int8
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&i8).Elem(), float64(12), reflect.StructField{}))
	require.Equal(t, int8(12), i8)

	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&i8).Elem(), "9999", reflect.StructField{}))
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&i8).Elem(), "not-int", reflect.StructField{}))
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&i8).Elem(), true, reflect.StructField{}))

	var f32 float32
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&f32).Elem(), "not-float", reflect.StructField{}))
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&f32).Elem(), "1e40", reflect.StructField{}))
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&f32).Elem(), float32(1.25), reflect.StructField{}))
	require.InDelta(t, 1.25, f32, 0.0001)
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&f32).Elem(), false, reflect.StructField{}))
}

func Test_setFieldValueEnhanced_BoolConversions_Round24(t *testing.T) {
	var b bool
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&b).Elem(), "true", reflect.StructField{}))
	require.True(t, b)
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&b).Elem(), 1, reflect.StructField{}))
}

func Test_setFieldValueEnhanced_Collections_Round24(t *testing.T) {
	var list []int
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&list).Elem(), "not-slice", reflect.StructField{}))
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&list).Elem(), []any{"not-int"}, reflect.StructField{}))

	var m map[string]string
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&m).Elem(), map[string]any{"k": "v"}, reflect.StructField{}))
	require.Equal(t, map[string]string{"k": "v"}, m)

	var badMap map[string]int
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&badMap).Elem(), map[string]any{"k": "not-int"}, reflect.StructField{}))
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&badMap).Elem(), []any{}, reflect.StructField{}))
}

func Test_setFieldValueEnhanced_StructAndPtr_Round24(t *testing.T) {
	var ts time.Time
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&ts).Elem(), "2024-06-15", reflect.StructField{}))
	require.False(t, ts.IsZero())

	var ts2 time.Time
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&ts2).Elem(), "1710000000", reflect.StructField{}))
	require.False(t, ts2.IsZero())

	var ts3 time.Time
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&ts3).Elem(), float64(1710000000), reflect.StructField{}))
	require.False(t, ts3.IsZero())

	var ts4 time.Time
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&ts4).Elem(), int64(1710000000), reflect.StructField{}))
	require.False(t, ts4.IsZero())

	var ts5 time.Time
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&ts5).Elem(), "not-a-time", reflect.StructField{}))
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&ts5).Elem(), math.MaxInt64, reflect.StructField{}))

	type plain struct{ Value string }
	var p plain
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&p).Elem(), "not-map", reflect.StructField{}))

	var sp *string
	require.NoError(t, setFieldValueEnhanced(reflect.ValueOf(&sp).Elem(), nil, reflect.StructField{}))
	require.Nil(t, sp)
}

func Test_setFieldValueEnhanced_UnsupportedType_Round24(t *testing.T) {
	var c complex64
	require.Error(t, setFieldValueEnhanced(reflect.ValueOf(&c).Elem(), 1, reflect.StructField{}))
}
