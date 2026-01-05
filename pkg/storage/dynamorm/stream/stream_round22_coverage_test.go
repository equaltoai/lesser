package stream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round22Nested struct {
	Name string `dynamorm:"attr:name"`
}

type round22Model struct {
	PK       string `dynamorm:"pk,attr:PK"`
	SK       string `dynamorm:"sk,attr:SK"`
	ID       string `dynamorm:"attr:id"`
	Age      int    `dynamorm:"attr:age"`
	Ratio    float64
	Active   bool             `dynamorm:"attr:active"`
	When     time.Time        `dynamorm:"attr:when"`
	Optional *string          `dynamorm:"attr:optional"`
	Nested   *round22Nested   `dynamorm:"attr:nested"`
	Tags     []string         `dynamorm:"attr:tags"`
	Meta     map[string]any   `dynamorm:"attr:meta"`
	AnyField any              `dynamorm:"attr:anyField"`
	Int8     int8             `dynamorm:"attr:int8"`
	BadBool  bool             `dynamorm:"attr:badBool"`
	Ignored  string           `json:"-"`
	Struct   round22PlainNest `dynamorm:"attr:struct"`
}

type round22PlainNest struct {
	Value string `json:"value"`
}

func round22Record(eventName string, image map[string]events.DynamoDBAttributeValue) events.DynamoDBEventRecord {
	record := events.DynamoDBEventRecord{
		EventID:   "evt",
		EventName: eventName,
	}
	switch eventName {
	case eventNameRemove:
		record.Change = events.DynamoDBStreamRecord{OldImage: image}
	default:
		record.Change = events.DynamoDBStreamRecord{NewImage: image}
	}
	return record
}

func TestUnmarshalItem_Round22(t *testing.T) {
	whenStr := "2024-06-15T14:30:00Z"
	when, err := time.Parse(time.RFC3339, whenStr)
	require.NoError(t, err)

	image := map[string]events.DynamoDBAttributeValue{
		"PK":       events.NewStringAttribute("AI#obj-1"),
		"SK":       events.NewStringAttribute("ANALYSIS#analysis-1"),
		"id":       events.NewStringAttribute("analysis-1"),
		"age":      events.NewNumberAttribute("42"),
		"Ratio":    events.NewNumberAttribute("0.75"), // exercises field-name fallback
		"active":   events.NewBooleanAttribute(true),
		"when":     events.NewStringAttribute(whenStr),
		"optional": events.NewStringAttribute("opt"),
		"nested": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"name": events.NewStringAttribute("nested"),
		}),
		"tags": events.NewListAttribute([]events.DynamoDBAttributeValue{
			events.NewStringAttribute("a"),
			events.NewStringAttribute("b"),
		}),
		"meta": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"k": events.NewStringAttribute("v"),
			"n": events.NewNumberAttribute("123"),
		}),
		"anyField": events.NewStringAttribute("anything"),
		"int8":     events.NewNumberAttribute("120"),
		"badBool":  events.NewStringAttribute("not-bool"),
		"struct": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"value": events.NewStringAttribute("plain"),
		}),
	}

	var out round22Model
	require.NoError(t, UnmarshalItem(round22Record(eventNameInsert, image), &out))

	require.Equal(t, "AI#obj-1", out.PK)
	require.Equal(t, "ANALYSIS#analysis-1", out.SK)
	require.Equal(t, "analysis-1", out.ID)
	require.Equal(t, 42, out.Age)
	require.InDelta(t, 0.75, out.Ratio, 0.0001)
	require.True(t, out.Active)
	require.True(t, out.When.Equal(when))
	require.NotNil(t, out.Optional)
	require.Equal(t, "opt", *out.Optional)
	require.NotNil(t, out.Nested)
	require.Equal(t, "nested", out.Nested.Name)
	require.Equal(t, []string{"a", "b"}, out.Tags)
	require.Equal(t, map[string]any{"k": "v", "n": "123"}, out.Meta)
	require.Equal(t, "anything", out.AnyField)
	require.Equal(t, int8(120), out.Int8)
	require.Equal(t, "plain", out.Struct.Value)
}

func TestUnmarshalItem_UsesOldImageForRemove_Round22(t *testing.T) {
	image := map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("AI#obj-2"),
		"SK": events.NewStringAttribute("ANALYSIS#analysis-2"),
		"id": events.NewStringAttribute("analysis-2"),
	}

	var out round22Model
	require.NoError(t, UnmarshalItem(round22Record(eventNameRemove, image), &out))
	require.Equal(t, "analysis-2", out.ID)
}

func TestUnmarshalItem_UnknownEvent_Round22(t *testing.T) {
	var out round22Model
	err := UnmarshalItem(events.DynamoDBEventRecord{EventName: "BOGUS"}, &out)
	require.Error(t, err)
}

func TestUnmarshalItems_SkipsBadRecords_Round22(t *testing.T) {
	okRecord := round22Record(eventNameInsert, map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("AI#obj-1"),
		"SK": events.NewStringAttribute("ANALYSIS#analysis-1"),
		"id": events.NewStringAttribute("analysis-1"),
	})
	badRecord := events.DynamoDBEventRecord{EventName: "BOGUS"}

	res, err := UnmarshalItems([]events.DynamoDBEventRecord{badRecord, okRecord}, round22Model{})
	require.NoError(t, err)
	typed, ok := res.([]round22Model)
	require.True(t, ok)
	require.Len(t, typed, 1)
	require.Equal(t, "analysis-1", typed[0].ID)
}

func TestProcessStreamRecords_Round22(t *testing.T) {
	calls := make(map[string]int)
	handler := func(_ context.Context, record events.DynamoDBEventRecord) error {
		calls[record.EventID]++
		if record.EventID == "fail" {
			return errors.New("boom")
		}
		return nil
	}

	records := []events.DynamoDBEventRecord{
		{EventID: "ok", EventName: eventNameInsert},
		{EventID: "skip", EventName: "BOGUS"},
		{EventID: "fail", EventName: eventNameModify},
		{EventID: "ok2", EventName: eventNameRemove},
	}

	require.NoError(t, ProcessStreamRecords(context.Background(), records, handler))
	require.Equal(t, 1, calls["ok"])
	require.Equal(t, 0, calls["skip"])
	require.Equal(t, 1, calls["fail"])
	require.Equal(t, 1, calls["ok2"])
}

func TestProcessor_ProcessStreamRecordsWithRetry_Round22_SequentialContinues(t *testing.T) {
	p := NewProcessor(&ProcessingConfig{
		EnableMetrics:        false,
		EnableErrorRecovery:  true,
		MaxRetryAttempts:     0,
		RetryBackoffInitial:  0,
		RetryBackoffMax:      0,
		EnableDLQ:            false,
		ParallelProcessing:   false,
		MaxConcurrentRecords: 1,
	}, zap.NewNop())

	records := []events.DynamoDBEventRecord{
		{EventID: "fail", EventName: eventNameInsert},
		{EventID: "ok", EventName: eventNameInsert},
	}

	calls := make(map[string]int)
	handler := func(_ context.Context, record events.DynamoDBEventRecord) error {
		calls[record.EventID]++
		if record.EventID == "fail" {
			return errors.New("not retryable")
		}
		return nil
	}

	// Sequential mode never returns record-level failures (it logs and continues).
	require.NoError(t, p.ProcessStreamRecordsWithRetry(context.Background(), records, handler))
	require.Equal(t, 1, calls["fail"])
	require.Equal(t, 1, calls["ok"])
}

func TestProcessor_ProcessStreamRecordsWithRetry_Round22_ParallelReturnsError(t *testing.T) {
	p := NewProcessor(&ProcessingConfig{
		EnableMetrics:        false,
		EnableErrorRecovery:  false,
		MaxRetryAttempts:     0,
		RetryBackoffInitial:  0,
		RetryBackoffMax:      0,
		EnableDLQ:            false,
		ParallelProcessing:   true,
		MaxConcurrentRecords: 1,
	}, zap.NewNop())

	records := []events.DynamoDBEventRecord{
		{EventID: "fail", EventName: eventNameInsert},
		{EventID: "ok", EventName: eventNameInsert},
	}

	handler := func(_ context.Context, record events.DynamoDBEventRecord) error {
		if record.EventID == "fail" {
			return errors.New("boom")
		}
		return nil
	}

	require.Error(t, p.ProcessStreamRecordsWithRetry(context.Background(), records, handler))
}

func TestProcessor_processRecordWithRetry_Round22_Retries(t *testing.T) {
	p := NewProcessor(&ProcessingConfig{
		EnableMetrics:        false,
		EnableErrorRecovery:  true,
		MaxRetryAttempts:     1,
		RetryBackoffInitial:  0,
		RetryBackoffMax:      0,
		EnableDLQ:            false,
		ParallelProcessing:   false,
		MaxConcurrentRecords: 1,
	}, zap.NewNop())

	record := events.DynamoDBEventRecord{EventID: "evt", EventName: eventNameInsert}

	attempts := 0
	handler := func(_ context.Context, _ events.DynamoDBEventRecord) error {
		attempts++
		if attempts == 1 {
			return errors.New("timeout")
		}
		return nil
	}

	require.NoError(t, p.processRecordWithRetry(context.Background(), record, handler))
	require.Equal(t, 2, attempts)
}

func Test_unmarshalWithDynamORMCompat_RequiresPtrToStruct_Round22(t *testing.T) {
	err := unmarshalWithDynamORMCompat(map[string]any{"x": "y"}, round22Model{})
	require.Error(t, err)
}
