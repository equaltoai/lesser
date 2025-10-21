package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock processor for testing
type mockProcessor struct {
	processedEvents []LiftEvent
	shouldError     bool
}

func (m *mockProcessor) ProcessEvent(_ context.Context, event LiftEvent) error {
	if m.shouldError {
		return assert.AnError
	}
	m.processedEvents = append(m.processedEvents, event)
	return nil
}

func TestEventAdapter_HandleSQSEvent(t *testing.T) {
	// Given
	processor := &mockProcessor{}
	adapter := NewEventAdapter(processor)

	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId:     "msg-123",
				Body:          `{"test": "data"}`,
				ReceiptHandle: "receipt-123",
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"correlationId": {
						StringValue: stringPtr("corr-123"),
					},
				},
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
		},
	}

	// When
	err := adapter.HandleSQSEvent(context.Background(), sqsEvent)

	// Then
	require.NoError(t, err)
	assert.Len(t, processor.processedEvents, 1)

	liftEvent, ok := processor.processedEvents[0].(*SQSLiftEvent)
	require.True(t, ok)
	assert.Equal(t, "sqs", liftEvent.GetSource())
	assert.Equal(t, "corr-123", liftEvent.ProcessingMetadata.CorrelationID)
	assert.Equal(t, 1, len(liftEvent.Records))
}

func TestEventAdapter_HandleEventBridgeEvent(t *testing.T) {
	// Given
	processor := &mockProcessor{}
	adapter := NewEventAdapter(processor).WithMetadata(ProcessingMetadata{
		TenantID: "tenant-123",
	})

	detail := map[string]interface{}{
		"orderId":       "order-123",
		"correlationId": "corr-456",
	}
	detailBytes, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Failed to marshal detail: %v", err)
	}

	ebEvent := events.CloudWatchEvent{
		ID:         "evt-123",
		Source:     "order.service",
		DetailType: "Order Created",
		Detail:     json.RawMessage(detailBytes),
		Time:       time.Now(),
		Region:     "us-east-1",
	}

	// When
	err = adapter.HandleEventBridgeEvent(context.Background(), ebEvent)

	// Then
	require.NoError(t, err)
	assert.Len(t, processor.processedEvents, 1)

	liftEvent, ok := processor.processedEvents[0].(*EventBridgeLiftEvent)
	require.True(t, ok)
	assert.Equal(t, "order.service", liftEvent.GetSource())
	assert.Equal(t, "evt-123", liftEvent.GetEventID())
	assert.Equal(t, "corr-456", liftEvent.ProcessingMetadata.CorrelationID)
	assert.Equal(t, "tenant-123", liftEvent.ProcessingMetadata.TenantID)
}

func TestEventAdapter_HandleS3Event(t *testing.T) {
	// Given
	processor := &mockProcessor{}
	adapter := NewEventAdapter(processor)

	s3Event := events.S3Event{
		Records: []events.S3EventRecord{
			{
				EventName: "ObjectCreated:Put",
				S3: events.S3Entity{
					Bucket: events.S3Bucket{
						Name: "test-bucket",
						Arn:  "arn:aws:s3:::test-bucket",
					},
					Object: events.S3Object{
						Key:  "path/to/file.txt",
						Size: 1024,
						ETag: "abc123",
					},
				},
				AWSRegion: "us-east-1",
			},
		},
	}

	// When
	err := adapter.HandleS3Event(context.Background(), s3Event)

	// Then
	require.NoError(t, err)
	assert.Len(t, processor.processedEvents, 1)

	liftEvent, ok := processor.processedEvents[0].(*S3LiftEvent)
	require.True(t, ok)
	assert.Equal(t, "s3", liftEvent.GetSource())
	assert.Equal(t, 1, len(liftEvent.Records))
}

func TestEventAdapter_HandleDynamoDBEvent(t *testing.T) {
	// Given
	processor := &mockProcessor{}
	adapter := NewEventAdapter(processor)

	dynamoEvent := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventID:   "evt-123",
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					Keys: map[string]events.DynamoDBAttributeValue{
						"id": events.NewStringAttribute("123"),
					},
					NewImage: map[string]events.DynamoDBAttributeValue{
						"id":   events.NewStringAttribute("123"),
						"name": events.NewStringAttribute("Test"),
					},
				},
			},
		},
	}

	// When
	err := adapter.HandleDynamoDBEvent(context.Background(), dynamoEvent)

	// Then
	require.NoError(t, err)
	assert.Len(t, processor.processedEvents, 1)

	liftEvent, ok := processor.processedEvents[0].(*DynamoDBLiftEvent)
	require.True(t, ok)
	assert.Equal(t, "dynamodb-streams", liftEvent.GetSource())
	assert.Equal(t, 1, len(liftEvent.Records))
}

func TestEventAdapter_HandleSNSEvent(t *testing.T) {
	// Given
	processor := &mockProcessor{}
	adapter := NewEventAdapter(processor)

	snsEvent := events.SNSEvent{
		Records: []events.SNSEventRecord{
			{
				SNS: events.SNSEntity{
					TopicArn:  "arn:aws:sns:us-east-1:123456789012:test-topic",
					Subject:   "Test Subject",
					Message:   "Test message",
					MessageID: "msg-123",
				},
			},
		},
	}

	// When
	err := adapter.HandleSNSEvent(context.Background(), snsEvent)

	// Then
	require.NoError(t, err)
	assert.Len(t, processor.processedEvents, 1)

	liftEvent, ok := processor.processedEvents[0].(*SNSLiftEvent)
	require.True(t, ok)
	assert.Equal(t, "sns", liftEvent.GetSource())
	assert.Equal(t, 1, len(liftEvent.Records))
}

func TestEventAdapter_HandleKinesisEvent(t *testing.T) {
	// Given
	processor := &mockProcessor{}
	adapter := NewEventAdapter(processor)

	kinesisEvent := events.KinesisEvent{
		Records: []events.KinesisEventRecord{
			{
				Kinesis: events.KinesisRecord{
					Data:           []byte("test-data"),
					SequenceNumber: "12345",
					PartitionKey:   "partition-1",
				},
				EventSourceArn: "arn:aws:kinesis:us-east-1:123456789012:stream/test-stream",
			},
		},
	}

	// When
	err := adapter.HandleKinesisEvent(context.Background(), kinesisEvent)

	// Then
	require.NoError(t, err)
	assert.Len(t, processor.processedEvents, 1)

	liftEvent, ok := processor.processedEvents[0].(*KinesisLiftEvent)
	require.True(t, ok)
	assert.Equal(t, "kinesis", liftEvent.GetSource())
	assert.Equal(t, 1, len(liftEvent.Records))
}

func TestEventEnvelope(t *testing.T) {
	// Given
	data := map[string]string{
		"key": "value",
	}

	// When
	envelope, err := NewEventEnvelope("test.source", "TestEvent", data)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "1.0", envelope.Version)
	assert.Equal(t, "test.source", envelope.Source)
	assert.Equal(t, "TestEvent", envelope.Type)
	assert.NotEmpty(t, envelope.ID)

	// Test unmarshal
	var unmarshaled map[string]string
	err = envelope.UnmarshalData(&unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, data, unmarshaled)
}

func TestLiftContextAdapter_AdaptSQSToHTTP(t *testing.T) {
	// Given
	adapter := NewLiftContextAdapter(nil)
	record := events.SQSMessage{
		MessageId: "msg-123",
		Body:      `{"action": "process", "data": "test"}`,
		MessageAttributes: map[string]events.SQSMessageAttribute{
			"userId": {
				StringValue: stringPtr("user-123"),
			},
		},
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
		ReceiptHandle:  "receipt-123",
	}

	// When
	ctx, err := adapter.AdaptSQSToHTTP(record)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "POST", ctx.Request.Method)
	assert.Equal(t, "/sqs/test-queue", ctx.Request.Path)
	assert.Equal(t, "user-123", ctx.Request.Headers["userId"])

	var body map[string]interface{}
	err = json.Unmarshal(ctx.Request.Body, &body)
	require.NoError(t, err)
	assert.Equal(t, "process", body["action"])

	assert.Equal(t, "msg-123", ctx.Get("sqsMessageId"))
	assert.Equal(t, "receipt-123", ctx.Get("sqsReceiptHandle"))
}

func TestLiftContextAdapter_AdaptEventBridgeToHTTP(t *testing.T) {
	// Given
	adapter := NewLiftContextAdapter(nil)
	detail := map[string]interface{}{
		"orderId": "order-123",
		"amount":  99.99,
	}
	detailBytes, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Failed to marshal detail: %v", err)
	}

	event := events.CloudWatchEvent{
		ID:         "evt-123",
		Source:     "order.service",
		DetailType: "Order Created",
		Detail:     json.RawMessage(detailBytes),
		Time:       time.Now(),
		Region:     "us-east-1",
	}

	// When
	ctx, err := adapter.AdaptEventBridgeToHTTP(event)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "POST", ctx.Request.Method)
	assert.Equal(t, "/events/order.service/Order Created", ctx.Request.Path)
	assert.Equal(t, "order.service", ctx.Request.Headers["X-Event-Source"])
	assert.Equal(t, "Order Created", ctx.Request.Headers["X-Event-Type"])

	var body map[string]interface{}
	err = json.Unmarshal(ctx.Request.Body, &body)
	require.NoError(t, err)
	assert.Equal(t, "order-123", body["orderId"])
}

func TestLiftContextAdapter_AdaptS3ToHTTP(t *testing.T) {
	// Given
	adapter := NewLiftContextAdapter(nil)
	record := events.S3EventRecord{
		EventName: "ObjectCreated:Put",
		S3: events.S3Entity{
			Bucket: events.S3Bucket{
				Name: "test-bucket",
			},
			Object: events.S3Object{
				Key:  "path/to/file.txt",
				Size: 1024,
				ETag: "abc123",
			},
		},
		AWSRegion: "us-east-1",
		ResponseElements: map[string]string{
			"x-amz-request-id": "req-123",
		},
	}

	// When
	ctx, err := adapter.AdaptS3ToHTTP(record)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "POST", ctx.Request.Method)
	assert.Equal(t, "/s3/test-bucket", ctx.Request.Path)
	assert.Equal(t, "test-bucket", ctx.Request.Headers["X-S3-Bucket"])
	assert.Equal(t, "path/to/file.txt", ctx.Request.Headers["X-S3-Key"])

	var body map[string]interface{}
	err = json.Unmarshal(ctx.Request.Body, &body)
	require.NoError(t, err)
	assert.Equal(t, "test-bucket", body["bucket"])
	assert.Equal(t, "path/to/file.txt", body["key"])
}

func TestBatchEventProcessor(t *testing.T) {
	// Given
	var processedBatches [][]LiftEvent
	processor := NewBatchEventProcessor(2, func(events []LiftEvent) error {
		processedBatches = append(processedBatches, events)
		return nil
	})

	events := []LiftEvent{
		&SQSLiftEvent{BaseEvent: BaseEvent{EventID: "1"}},
		&SQSLiftEvent{BaseEvent: BaseEvent{EventID: "2"}},
		&SQSLiftEvent{BaseEvent: BaseEvent{EventID: "3"}},
		&SQSLiftEvent{BaseEvent: BaseEvent{EventID: "4"}},
		&SQSLiftEvent{BaseEvent: BaseEvent{EventID: "5"}},
	}

	// When
	err := processor.ProcessBatch(events)

	// Then
	require.NoError(t, err)
	assert.Len(t, processedBatches, 3) // 5 events / batch size 2 = 3 batches
	assert.Len(t, processedBatches[0], 2)
	assert.Len(t, processedBatches[1], 2)
	assert.Len(t, processedBatches[2], 1)
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
