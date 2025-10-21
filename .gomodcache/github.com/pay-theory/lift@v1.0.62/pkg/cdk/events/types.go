package events

import (
	"encoding/json"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// LiftEvent is the base interface for all Lift events
type LiftEvent interface {
	GetSource() string
	GetTimestamp() time.Time
	GetEventID() string
}

// BaseEvent provides common fields for all events
type BaseEvent struct {
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	EventID   string    `json:"eventId"`
}

func (e BaseEvent) GetSource() string       { return e.Source }
func (e BaseEvent) GetTimestamp() time.Time { return e.Timestamp }
func (e BaseEvent) GetEventID() string      { return e.EventID }

// SQSLiftEvent wraps an SQS event with Lift functionality
type SQSLiftEvent struct {
	ProcessingMetadata ProcessingMetadata `json:"processingMetadata,omitempty"`
	BaseEvent
	events.SQSEvent
}

// EventBridgeLiftEvent wraps an EventBridge event with Lift functionality
type EventBridgeLiftEvent struct {
	ProcessingMetadata ProcessingMetadata `json:"processingMetadata,omitempty"`
	BaseEvent
	CloudWatchEvent events.CloudWatchEvent `json:"cloudWatchEvent"`
}

// S3LiftEvent wraps an S3 event with Lift functionality
type S3LiftEvent struct {
	ProcessingMetadata ProcessingMetadata `json:"processingMetadata,omitempty"`
	BaseEvent
	events.S3Event
}

// DynamoDBLiftEvent wraps a DynamoDB stream event with Lift functionality
type DynamoDBLiftEvent struct {
	ProcessingMetadata ProcessingMetadata `json:"processingMetadata,omitempty"`
	BaseEvent
	events.DynamoDBEvent
}

// SNSLiftEvent wraps an SNS event with Lift functionality
type SNSLiftEvent struct {
	ProcessingMetadata ProcessingMetadata `json:"processingMetadata,omitempty"`
	BaseEvent
	events.SNSEvent
}

// KinesisLiftEvent wraps a Kinesis event with Lift functionality
type KinesisLiftEvent struct {
	ProcessingMetadata ProcessingMetadata `json:"processingMetadata,omitempty"`
	BaseEvent
	events.KinesisEvent
}

// ProcessingMetadata contains metadata about event processing
type ProcessingMetadata struct {
	Tags          map[string]string `json:"tags,omitempty"`
	CorrelationID string            `json:"correlationId,omitempty"`
	UserID        string            `json:"userId,omitempty"`
	TenantID      string            `json:"tenantId,omitempty"`
	TraceID       string            `json:"traceId,omitempty"`
	SpanID        string            `json:"spanId,omitempty"`
	RetryCount    int               `json:"retryCount,omitempty"`
}

// EventEnvelope wraps any event with metadata
type EventEnvelope struct {
	Metadata ProcessingMetadata `json:"metadata"`
	Time     time.Time          `json:"time"`
	Version  string             `json:"version"`
	ID       string             `json:"id"`
	Source   string             `json:"source"`
	Type     string             `json:"type"`
	Data     json.RawMessage    `json:"data"`
}

// NewEventEnvelope creates a new event envelope
func NewEventEnvelope(source, eventType string, data interface{}) (*EventEnvelope, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &EventEnvelope{
		Version: "1.0",
		ID:      generateEventID(),
		Source:  source,
		Type:    eventType,
		Time:    time.Now(),
		Data:    dataBytes,
	}, nil
}

// UnmarshalData unmarshals the event data into the provided interface
func (e *EventEnvelope) UnmarshalData(v interface{}) error {
	return json.Unmarshal(e.Data, v)
}

// EventResponse represents a standard response from event processing
type EventResponse struct {
	Errors            []EventError                 `json:"errors,omitempty"`
	BatchItemFailures []events.SQSBatchItemFailure `json:"batchItemFailures,omitempty"`
	ProcessedCount    int                          `json:"processedCount"`
	FailedCount       int                          `json:"failedCount"`
	Success           bool                         `json:"success"`
}

// EventError represents an error during event processing
type EventError struct {
	Timestamp time.Time `json:"timestamp"`
	EventID   string    `json:"eventId"`
	Error     string    `json:"error"`
	ErrorType string    `json:"errorType"`
	Retryable bool      `json:"retryable"`
}

// Multi-event types for orchestration

// OrchestratedEvent represents an event in an orchestration flow
// Memory optimized: 240 → 232 bytes (8 bytes saved)
type OrchestratedEvent struct {
	Context       map[string]interface{} `json:"context"`
	CorrelationID string                 `json:"correlationId"`
	Status        OrchestrationStatus    `json:"status"`
	EventEnvelope
	SequenceID int `json:"sequenceId"`
	TotalSteps int `json:"totalSteps"`
}

// OrchestrationStatus represents the status of an orchestrated flow
type OrchestrationStatus string

const (
	OrchestrationPending      OrchestrationStatus = "PENDING"
	OrchestrationInProgress   OrchestrationStatus = "IN_PROGRESS"
	OrchestrationCompleted    OrchestrationStatus = "COMPLETED"
	OrchestrationFailed       OrchestrationStatus = "FAILED"
	OrchestrationCompensating OrchestrationStatus = "COMPENSATING"
)

// SagaEvent represents an event in a saga pattern
type SagaEvent struct {
	Error         *SagaError             `json:"error,omitempty"`
	Context       map[string]interface{} `json:"context"`
	SagaID        string                 `json:"sagaId"`
	TransactionID string                 `json:"transactionId"`
	Step          string                 `json:"step"`
	Action        SagaAction             `json:"action"`
	Status        SagaStatus             `json:"status"`
	Payload       json.RawMessage        `json:"payload"`
}

// SagaAction represents the action type in a saga
type SagaAction string

const (
	SagaExecute    SagaAction = "EXECUTE"
	SagaCompensate SagaAction = "COMPENSATE"
)

// SagaStatus represents the status of a saga step
type SagaStatus string

const (
	SagaPending     SagaStatus = "PENDING"
	SagaExecuting   SagaStatus = "EXECUTING"
	SagaCompleted   SagaStatus = "COMPLETED"
	SagaFailed      SagaStatus = "FAILED"
	SagaCompensated SagaStatus = "COMPENSATED"
)

// SagaError represents an error in saga processing
type SagaError struct {
	Details map[string]interface{} `json:"details,omitempty"`
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
}

// AsyncAPIRequest represents a request in the event-driven API pattern
type AsyncAPIRequest struct {
	Headers     map[string]string      `json:"headers"`
	Metadata    map[string]interface{} `json:"metadata"`
	RequestID   string                 `json:"requestId"`
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	CallbackURL string                 `json:"callbackUrl,omitempty"`
	WebhookURL  string                 `json:"webhookUrl,omitempty"`
	Body        json.RawMessage        `json:"body"`
}

// AsyncAPIResponse represents a response in the event-driven API pattern
type AsyncAPIResponse struct {
	Headers    map[string]string      `json:"headers,omitempty"`
	Error      *AsyncAPIError         `json:"error,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	RequestID  string                 `json:"requestId"`
	Status     AsyncAPIStatus         `json:"status"`
	Body       json.RawMessage        `json:"body,omitempty"`
	StatusCode int                    `json:"statusCode"`
}

// AsyncAPIStatus represents the status of an async API request
type AsyncAPIStatus string

const (
	AsyncAPIAccepted   AsyncAPIStatus = "ACCEPTED"
	AsyncAPIProcessing AsyncAPIStatus = "PROCESSING"
	AsyncAPICompleted  AsyncAPIStatus = "COMPLETED"
	AsyncAPIFailed     AsyncAPIStatus = "FAILED"
)

// AsyncAPIError represents an error in async API processing
type AsyncAPIError struct {
	Details map[string]interface{} `json:"details,omitempty"`
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
}

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Metadata     ProcessingMetadata `json:"metadata"`
	ConnectionID string             `json:"connectionId"`
	Action       string             `json:"action"`
	Data         json.RawMessage    `json:"data"`
}

// WebSocketResponse represents a response to send via WebSocket
type WebSocketResponse struct {
	ConnectionID string          `json:"connectionId"`
	Data         json.RawMessage `json:"data"`
	StatusCode   int             `json:"statusCode"`
}

// Helper functions

func generateEventID() string {
	// In production, use a proper UUID library
	return "evt_" + time.Now().Format("20060102150405") + "_" + generateRandomString(8)
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// Simplified for example - use crypto/rand in production
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[i%len(charset)]
	}
	return string(result)
}
