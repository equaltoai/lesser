package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/pay-theory/lift/pkg/lift"
)

// EventAdapter provides integration between AWS events and Lift handlers
type EventAdapter struct {
	processor EventProcessor
	metadata  ProcessingMetadata
}

// EventProcessor is the interface for processing Lift events
type EventProcessor interface {
	// ProcessEvent processes a Lift event.
	// Parameters:
	//   - ctx: The context for the request
	//   - event: The Lift event to process
	// Returns:
	//   - An error if the processing fails
	ProcessEvent(ctx context.Context, event LiftEvent) error
}

// NewEventAdapter creates a new event adapter
func NewEventAdapter(processor EventProcessor) *EventAdapter {
	return &EventAdapter{
		processor: processor,
	}
}

// WithMetadata adds metadata to be included with all events
func (a *EventAdapter) WithMetadata(metadata ProcessingMetadata) *EventAdapter {
	a.metadata = metadata
	return a
}

// HandleSQSEvent adapts SQS events to Lift events
func (a *EventAdapter) HandleSQSEvent(ctx context.Context, sqsEvent events.SQSEvent) error {
	liftEvent := &SQSLiftEvent{
		BaseEvent: BaseEvent{
			Source:    "sqs",
			Timestamp: timeNow(),
			EventID:   generateEventID(),
		},
		SQSEvent:           sqsEvent,
		ProcessingMetadata: a.metadata,
	}

	// Add correlation ID from message attributes if available
	if len(sqsEvent.Records) > 0 {
		if corrID, ok := sqsEvent.Records[0].MessageAttributes["correlationId"]; ok && corrID.StringValue != nil {
			liftEvent.ProcessingMetadata.CorrelationID = *corrID.StringValue
		}
	}

	return a.processor.ProcessEvent(ctx, liftEvent)
}

// HandleEventBridgeEvent adapts EventBridge events to Lift events
func (a *EventAdapter) HandleEventBridgeEvent(ctx context.Context, event events.CloudWatchEvent) error {
	liftEvent := &EventBridgeLiftEvent{
		BaseEvent: BaseEvent{
			Source:    event.Source,
			Timestamp: event.Time,
			EventID:   event.ID,
		},
		CloudWatchEvent:    event,
		ProcessingMetadata: a.metadata,
	}

	// Extract correlation ID from detail if available
	var detail map[string]interface{}
	if err := json.Unmarshal(event.Detail, &detail); err == nil {
		if corrID, ok := detail["correlationId"].(string); ok {
			liftEvent.ProcessingMetadata.CorrelationID = corrID
		}
	}

	return a.processor.ProcessEvent(ctx, liftEvent)
}

// HandleS3Event adapts S3 events to Lift events
func (a *EventAdapter) HandleS3Event(ctx context.Context, s3Event events.S3Event) error {
	liftEvent := &S3LiftEvent{
		BaseEvent: BaseEvent{
			Source:    "s3",
			Timestamp: timeNow(),
			EventID:   generateEventID(),
		},
		S3Event:            s3Event,
		ProcessingMetadata: a.metadata,
	}

	return a.processor.ProcessEvent(ctx, liftEvent)
}

// HandleDynamoDBEvent adapts DynamoDB stream events to Lift events
func (a *EventAdapter) HandleDynamoDBEvent(ctx context.Context, dynamoEvent events.DynamoDBEvent) error {
	liftEvent := &DynamoDBLiftEvent{
		BaseEvent: BaseEvent{
			Source:    "dynamodb-streams",
			Timestamp: timeNow(),
			EventID:   generateEventID(),
		},
		DynamoDBEvent:      dynamoEvent,
		ProcessingMetadata: a.metadata,
	}

	return a.processor.ProcessEvent(ctx, liftEvent)
}

// HandleSNSEvent adapts SNS events to Lift events
func (a *EventAdapter) HandleSNSEvent(ctx context.Context, snsEvent events.SNSEvent) error {
	liftEvent := &SNSLiftEvent{
		BaseEvent: BaseEvent{
			Source:    "sns",
			Timestamp: timeNow(),
			EventID:   generateEventID(),
		},
		SNSEvent:           snsEvent,
		ProcessingMetadata: a.metadata,
	}

	return a.processor.ProcessEvent(ctx, liftEvent)
}

// HandleKinesisEvent adapts Kinesis events to Lift events
func (a *EventAdapter) HandleKinesisEvent(ctx context.Context, kinesisEvent events.KinesisEvent) error {
	liftEvent := &KinesisLiftEvent{
		BaseEvent: BaseEvent{
			Source:    "kinesis",
			Timestamp: timeNow(),
			EventID:   generateEventID(),
		},
		KinesisEvent:       kinesisEvent,
		ProcessingMetadata: a.metadata,
	}

	return a.processor.ProcessEvent(ctx, liftEvent)
}

// LiftContextAdapter adapts events to work with Lift's Context
type LiftContextAdapter struct {
	app *lift.App
}

// NewLiftContextAdapter creates a new Lift context adapter
func NewLiftContextAdapter(app *lift.App) *LiftContextAdapter {
	return &LiftContextAdapter{app: app}
}

// AdaptSQSToHTTP converts SQS messages to HTTP-like requests for Lift
func (l *LiftContextAdapter) AdaptSQSToHTTP(record events.SQSMessage) (*lift.Context, error) {
	// Parse message body as JSON
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(record.Body), &body); err != nil {
		// If not JSON, wrap in object
		body = map[string]interface{}{
			"message": record.Body,
		}
	}

	// Extract headers from message attributes
	headers := make(map[string]string)
	for key, attr := range record.MessageAttributes {
		if attr.StringValue != nil {
			headers[key] = *attr.StringValue
		}
	}

	// Marshal body to JSON
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SQS body: %w", err)
	}

	// Create a synthetic HTTP request
	ctx := &lift.Context{
		Request: &lift.Request{
			Method:  "POST",
			Path:    "/sqs/" + extractQueueName(record.EventSourceARN),
			Headers: headers,
			Body:    bodyBytes,
		},
	}

	// Add SQS-specific data
	ctx.Set("sqsMessageId", record.MessageId)
	ctx.Set("sqsReceiptHandle", record.ReceiptHandle)
	ctx.Set("sqsEventSourceARN", record.EventSourceARN)

	return ctx, nil
}

// AdaptEventBridgeToHTTP converts EventBridge events to HTTP-like requests
func (l *LiftContextAdapter) AdaptEventBridgeToHTTP(event events.CloudWatchEvent) (*lift.Context, error) {
	// Parse detail as body
	var body map[string]interface{}
	if err := json.Unmarshal(event.Detail, &body); err != nil {
		return nil, fmt.Errorf("failed to parse event detail: %w", err)
	}

	// Create headers from event metadata
	headers := map[string]string{
		"X-Event-Source": event.Source,
		"X-Event-Type":   event.DetailType,
		"X-Event-ID":     event.ID,
		"X-Event-Time":   event.Time.Format(timeRFC3339),
		"X-Event-Region": event.Region,
	}

	// Marshal body to JSON
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EventBridge body: %w", err)
	}

	// Create synthetic HTTP request
	ctx := &lift.Context{
		Request: &lift.Request{
			Method:  "POST",
			Path:    "/events/" + event.Source + "/" + event.DetailType,
			Headers: headers,
			Body:    bodyBytes,
		},
	}

	// Add EventBridge-specific data
	ctx.Set("eventBridgeSource", event.Source)
	ctx.Set("eventBridgeDetailType", event.DetailType)
	ctx.Set("eventBridgeResources", event.Resources)

	return ctx, nil
}

// AdaptS3ToHTTP converts S3 events to HTTP-like requests
func (l *LiftContextAdapter) AdaptS3ToHTTP(record events.S3EventRecord) (*lift.Context, error) {
	// Create headers from S3 metadata
	headers := map[string]string{
		"X-S3-Bucket":     record.S3.Bucket.Name,
		"X-S3-Key":        record.S3.Object.Key,
		"X-S3-Event":      record.EventName,
		"X-S3-Region":     record.AWSRegion,
		"X-S3-Request-ID": record.ResponseElements["x-amz-request-id"],
	}

	// Create body with S3 event details
	body := map[string]interface{}{
		"bucket":    record.S3.Bucket.Name,
		"key":       record.S3.Object.Key,
		"size":      record.S3.Object.Size,
		"etag":      record.S3.Object.ETag,
		"eventName": record.EventName,
	}

	// Marshal body to JSON
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal S3 body: %w", err)
	}

	// Create synthetic HTTP request
	ctx := &lift.Context{
		Request: &lift.Request{
			Method:  "POST",
			Path:    "/s3/" + record.S3.Bucket.Name,
			Headers: headers,
			Body:    bodyBytes,
		},
	}

	// Add S3-specific data
	ctx.Set("s3Bucket", record.S3.Bucket.Name)
	ctx.Set("s3Key", record.S3.Object.Key)
	ctx.Set("s3EventName", record.EventName)

	return ctx, nil
}

// BatchEventProcessor processes events in batches
// Memory optimized: 16 → 8 bytes (8 bytes saved)
type BatchEventProcessor struct {
	// Function pointer first (8 bytes)
	processor func([]LiftEvent) error
	// Int last (4 bytes)
	batchSize int
}

// NewBatchEventProcessor creates a new batch processor
func NewBatchEventProcessor(batchSize int, processor func([]LiftEvent) error) *BatchEventProcessor {
	return &BatchEventProcessor{
		batchSize: batchSize,
		processor: processor,
	}
}

// ProcessBatch accumulates events and processes them in batches
func (b *BatchEventProcessor) ProcessBatch(events []LiftEvent) error {
	for i := 0; i < len(events); i += b.batchSize {
		end := i + b.batchSize
		if end > len(events) {
			end = len(events)
		}

		batch := events[i:end]
		if err := b.processor(batch); err != nil {
			return fmt.Errorf("batch processing failed at index %d: %w", i, err)
		}
	}

	return nil
}

// Helper functions

func extractQueueName(arn string) string {
	// Extract queue name from ARN
	// arn:aws:sqs:region:account:queue-name
	parts := splitARN(arn)
	if len(parts) >= 6 {
		return parts[5]
	}
	return "unknown"
}

func splitARN(arn string) []string {
	// Simple ARN parser
	return splitString(arn, ":")
}

func splitString(s, sep string) []string {
	// Simple string split implementation
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

func timeNow() time.Time {
	return time.Now()
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"
