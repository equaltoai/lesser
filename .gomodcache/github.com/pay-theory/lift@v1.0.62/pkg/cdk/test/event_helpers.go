package test

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// EventHelpers provides utilities for testing event-driven CDK constructs
type EventHelpers struct{}

// NewEventHelpers creates a new instance of EventHelpers
func NewEventHelpers() *EventHelpers {
	return &EventHelpers{}
}

// GenerateSQSEvent creates a mock SQS event for testing
func (e *EventHelpers) GenerateSQSEvent(messages []SQSMessage) events.SQSEvent {
	event := events.SQSEvent{
		Records: make([]events.SQSMessage, len(messages)),
	}

	for i, msg := range messages {
		event.Records[i] = events.SQSMessage{
			MessageId:     msg.ID,
			Body:          msg.Body,
			ReceiptHandle: "test-receipt-handle-" + msg.ID,
			Attributes: map[string]string{
				"ApproximateReceiveCount":          "1",
				"SentTimestamp":                    msg.Timestamp,
				"ApproximateFirstReceiveTimestamp": msg.Timestamp,
			},
			MessageAttributes: msg.Attributes,
			EventSourceARN:    msg.SourceARN,
		}
	}

	return event
}

// SQSMessage represents a simplified SQS message for testing
type SQSMessage struct {
	ID         string
	Body       string
	Attributes map[string]events.SQSMessageAttribute
	SourceARN  string
	Timestamp  string
}

// GenerateEventBridgeEvent creates a mock EventBridge event for testing
func (e *EventHelpers) GenerateEventBridgeEvent(source, detailType string, detail interface{}) events.CloudWatchEvent {
	detailBytes, err := json.Marshal(detail)
	if err != nil {
		// Fallback to empty JSON object for test
		detailBytes = []byte("{}")
	}

	return events.CloudWatchEvent{
		ID:         "test-event-" + time.Now().Format("20060102150405"),
		Source:     source,
		DetailType: detailType,
		Detail:     json.RawMessage(detailBytes),
		Time:       time.Now(),
		Region:     "us-east-1",
		Resources:  []string{},
	}
}

// GenerateS3Event creates a mock S3 event for testing
func (e *EventHelpers) GenerateS3Event(bucket, key string, eventName string) events.S3Event {
	return events.S3Event{
		Records: []events.S3EventRecord{
			{
				EventName: eventName,
				S3: events.S3Entity{
					Bucket: events.S3Bucket{
						Name: bucket,
						OwnerIdentity: events.S3UserIdentity{
							PrincipalID: "test-principal",
						},
						Arn: "arn:aws:s3:::" + bucket,
					},
					Object: events.S3Object{
						Key:       key,
						Size:      1024,
						ETag:      "test-etag",
						VersionID: "test-version",
					},
				},
				AWSRegion: "us-east-1",
				EventTime: time.Now(),
			},
		},
	}
}

// GenerateDynamoDBStreamEvent creates a mock DynamoDB stream event for testing
func (e *EventHelpers) GenerateDynamoDBStreamEvent(tableName string, records []DynamoDBRecord) events.DynamoDBEvent {
	event := events.DynamoDBEvent{
		Records: make([]events.DynamoDBEventRecord, len(records)),
	}

	for i, record := range records {
		event.Records[i] = events.DynamoDBEventRecord{
			EventID:        "test-event-" + record.ID,
			EventName:      record.EventName,
			EventSource:    "aws:dynamodb",
			EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/" + tableName + "/stream/test-stream",
			Change: events.DynamoDBStreamRecord{
				Keys:           record.Keys,
				NewImage:       record.NewImage,
				OldImage:       record.OldImage,
				SizeBytes:      256,
				StreamViewType: record.StreamViewType,
			},
			AWSRegion: "us-east-1",
		}
	}

	return event
}

// DynamoDBRecord represents a simplified DynamoDB stream record
type DynamoDBRecord struct {
	ID             string
	EventName      string
	Keys           map[string]events.DynamoDBAttributeValue
	NewImage       map[string]events.DynamoDBAttributeValue
	OldImage       map[string]events.DynamoDBAttributeValue
	StreamViewType string
}

// GenerateSNSEvent creates a mock SNS event for testing
func (e *EventHelpers) GenerateSNSEvent(topicArn, subject, message string) events.SNSEvent {
	return events.SNSEvent{
		Records: []events.SNSEventRecord{
			{
				EventSource: "aws:sns",
				SNS: events.SNSEntity{
					TopicArn:  topicArn,
					Subject:   subject,
					Message:   message,
					MessageID: "test-message-id",
					Timestamp: time.Now(),
					MessageAttributes: map[string]interface{}{
						"test": map[string]interface{}{
							"Type":  "String",
							"Value": "test-value",
						},
					},
				},
			},
		},
	}
}

// GenerateKinesisEvent creates a mock Kinesis event for testing
func (e *EventHelpers) GenerateKinesisEvent(streamArn string, records []KinesisRecord) events.KinesisEvent {
	event := events.KinesisEvent{
		Records: make([]events.KinesisEventRecord, len(records)),
	}

	for i, record := range records {
		event.Records[i] = events.KinesisEventRecord{
			EventSource:    "aws:kinesis",
			EventSourceArn: streamArn,
			Kinesis: events.KinesisRecord{
				Data:                        []byte(record.Data),
				SequenceNumber:              record.SequenceNumber,
				PartitionKey:                record.PartitionKey,
				ApproximateArrivalTimestamp: events.SecondsEpochTime{Time: time.Now()},
			},
			EventID: "test-event-" + record.SequenceNumber,
		}
	}

	return event
}

// KinesisRecord represents a simplified Kinesis record
type KinesisRecord struct {
	Data           string
	SequenceNumber string
	PartitionKey   string
}

// ValidateEventPattern validates an EventBridge event pattern against an event
func (e *EventHelpers) ValidateEventPattern(pattern map[string]interface{}, event events.CloudWatchEvent) bool {
	// Check source
	if sources, ok := pattern["source"].([]string); ok {
		found := false
		for _, s := range sources {
			if s == event.Source {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check detail-type
	if detailTypes, ok := pattern["detail-type"].([]string); ok {
		found := false
		for _, dt := range detailTypes {
			if dt == event.DetailType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Additional pattern matching would go here
	return true
}

// MockEventSource represents a mock event source for testing
type MockEventSource struct {
	Events []interface{}
	Delay  time.Duration
}

// NewMockEventSource creates a new mock event source
func NewMockEventSource() *MockEventSource {
	return &MockEventSource{
		Events: make([]interface{}, 0),
		Delay:  0,
	}
}

// AddEvent adds an event to the mock source
func (m *MockEventSource) AddEvent(event interface{}) {
	m.Events = append(m.Events, event)
}

// SetDelay sets a delay between events
func (m *MockEventSource) SetDelay(delay time.Duration) {
	m.Delay = delay
}

// Emit emits all events with the configured delay
func (m *MockEventSource) Emit() <-chan interface{} {
	ch := make(chan interface{})

	go func() {
		defer close(ch)
		for _, event := range m.Events {
			ch <- event
			if m.Delay > 0 {
				time.Sleep(m.Delay)
			}
		}
	}()

	return ch
}

// EventRecorder records events for testing
type EventRecorder struct {
	RecordedEvents []interface{}
	Errors         []error
}

// NewEventRecorder creates a new event recorder
func NewEventRecorder() *EventRecorder {
	return &EventRecorder{
		RecordedEvents: make([]interface{}, 0),
		Errors:         make([]error, 0),
	}
}

// Record records an event
func (r *EventRecorder) Record(event interface{}) {
	r.RecordedEvents = append(r.RecordedEvents, event)
}

// RecordError records an error
func (r *EventRecorder) RecordError(err error) {
	r.Errors = append(r.Errors, err)
}

// GetEventCount returns the number of recorded events
func (r *EventRecorder) GetEventCount() int {
	return len(r.RecordedEvents)
}

// GetErrorCount returns the number of recorded errors
func (r *EventRecorder) GetErrorCount() int {
	return len(r.Errors)
}

// Clear clears all recorded events and errors
func (r *EventRecorder) Clear() {
	r.RecordedEvents = make([]interface{}, 0)
	r.Errors = make([]error, 0)
}

// EventReplay allows replaying recorded events
type EventReplay struct {
	Events []interface{}
}

// NewEventReplay creates a new event replay instance
func NewEventReplay(events []interface{}) *EventReplay {
	return &EventReplay{
		Events: events,
	}
}

// Replay replays events to a handler function
func (r *EventReplay) Replay(handler func(interface{}) error) error {
	for _, event := range r.Events {
		if err := handler(event); err != nil {
			return err
		}
	}
	return nil
}

// ReplayWithDelay replays events with a delay between each
func (r *EventReplay) ReplayWithDelay(handler func(interface{}) error, delay time.Duration) error {
	for _, event := range r.Events {
		if err := handler(event); err != nil {
			return err
		}
		time.Sleep(delay)
	}
	return nil
}

// EventValidator provides event validation utilities
type EventValidator struct{}

// NewEventValidator creates a new event validator
func NewEventValidator() *EventValidator {
	return &EventValidator{}
}

// ValidateSQSMessage validates an SQS message
func (v *EventValidator) ValidateSQSMessage(msg events.SQSMessage) error {
	if msg.MessageId == "" {
		return errors.New("missing message ID")
	}
	if msg.Body == "" {
		return errors.New("missing message body")
	}
	return nil
}

// ValidateEventBridgeEvent validates an EventBridge event
func (v *EventValidator) ValidateEventBridgeEvent(event events.CloudWatchEvent) error {
	if event.Source == "" {
		return errors.New("missing event source")
	}
	if event.DetailType == "" {
		return errors.New("missing detail type")
	}
	return nil
}

// ValidateS3Event validates an S3 event
func (v *EventValidator) ValidateS3Event(event events.S3Event) error {
	if len(event.Records) == 0 {
		return errors.New("no S3 records found")
	}
	for _, record := range event.Records {
		if record.S3.Bucket.Name == "" {
			return errors.New("missing bucket name")
		}
		if record.S3.Object.Key == "" {
			return errors.New("missing object key")
		}
	}
	return nil
}
