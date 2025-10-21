package adapters

import (
	"fmt"
	"reflect"
)

// TriggerType represents the type of Lambda trigger
type TriggerType string

const (
	TriggerAPIGateway   TriggerType = "api_gateway"
	TriggerAPIGatewayV2 TriggerType = "api_gateway_v2"
	TriggerSQS          TriggerType = "sqs"
	TriggerS3           TriggerType = "s3"
	TriggerEventBridge  TriggerType = "eventbridge"
	TriggerWebSocket    TriggerType = "websocket"
	TriggerUnknown      TriggerType = "unknown"
)

// Request represents a normalized request from any event source
type Request struct {
	RawEvent    any               `json:"raw_event,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	PathParams  map[string]string `json:"path_params,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	Detail      map[string]any    `json:"detail,omitempty"`
	EventID     string            `json:"event_id,omitempty"`
	Timestamp   string            `json:"timestamp,omitempty"`
	Method      string            `json:"method,omitempty"`
	Path        string            `json:"path,omitempty"`
	Source      string            `json:"source,omitempty"`
	DetailType  string            `json:"detail_type,omitempty"`
	TriggerType TriggerType       `json:"trigger_type"`
	Body        []byte            `json:"body,omitempty"`
	Records     []any             `json:"records,omitempty"`
}

// EventAdapter defines the interface for converting Lambda events to normalized requests
type EventAdapter interface {
	// Adapt converts a raw Lambda event to a normalized Request
	Adapt(rawEvent any) (*Request, error)

	// GetTriggerType returns the trigger type this adapter handles
	GetTriggerType() TriggerType

	// Validate checks if the raw event matches this adapter's expected format
	Validate(event any) error

	// CanHandle returns true if this adapter can handle the given event
	CanHandle(event any) bool
}

// AdapterRegistry manages event adapters and provides automatic event type detection
type AdapterRegistry struct {
	adapters map[TriggerType]EventAdapter
}

// NewAdapterRegistry creates a new adapter registry with default adapters
func NewAdapterRegistry() *AdapterRegistry {
	registry := &AdapterRegistry{
		adapters: make(map[TriggerType]EventAdapter),
	}

	// Register default adapters
	registry.Register(NewAPIGatewayAdapter())
	registry.Register(NewAPIGatewayV2Adapter())
	registry.Register(NewSQSAdapter())
	registry.Register(NewS3Adapter())
	registry.Register(NewEventBridgeAdapter())
	registry.Register(NewWebSocketAdapter())

	return registry
}

// Register adds an adapter to the registry
func (r *AdapterRegistry) Register(adapter EventAdapter) {
	r.adapters[adapter.GetTriggerType()] = adapter
}

// GetAdapter returns the adapter for a specific trigger type
func (r *AdapterRegistry) GetAdapter(triggerType TriggerType) (EventAdapter, bool) {
	adapter, exists := r.adapters[triggerType]
	return adapter, exists
}

// DetectAndAdapt automatically detects the event type and adapts it
func (r *AdapterRegistry) DetectAndAdapt(rawEvent any) (*Request, error) {
	// Track which adapters were tried and why they failed
	attemptedAdapters := make([]string, 0, len(r.adapters))
	var detectedFields []string

	// Extract fields from the event for debugging
	if eventMap, ok := rawEvent.(map[string]any); ok {
		detectedFields = make([]string, 0, len(eventMap))
		for key := range eventMap {
			detectedFields = append(detectedFields, key)
		}
	}

	// Try each adapter to see which one can handle the event
	for triggerType, adapter := range r.adapters {
		attemptedAdapters = append(attemptedAdapters, string(triggerType))
		if adapter.CanHandle(rawEvent) {
			return adapter.Adapt(rawEvent)
		}
	}

	// If no adapter can handle it, return a detailed error
	eventType := reflect.TypeOf(rawEvent)
	return nil, fmt.Errorf("no adapter found for event type: %v\nDetected fields: %v\nTried adapters: %v\nHint: Check if your API Gateway integration is configured correctly",
		eventType, detectedFields, attemptedAdapters)
}

// AdaptWithType adapts an event using a specific adapter type
func (r *AdapterRegistry) AdaptWithType(rawEvent any, triggerType TriggerType) (*Request, error) {
	adapter, exists := r.adapters[triggerType]
	if !exists {
		return nil, fmt.Errorf("no adapter registered for trigger type: %s", triggerType)
	}

	return adapter.Adapt(rawEvent)
}

// ListSupportedTriggers returns all supported trigger types
func (r *AdapterRegistry) ListSupportedTriggers() []TriggerType {
	triggers := make([]TriggerType, 0, len(r.adapters))
	for triggerType := range r.adapters {
		triggers = append(triggers, triggerType)
	}
	return triggers
}

// BaseAdapter provides common functionality for all adapters
type BaseAdapter struct {
	triggerType TriggerType
}

// GetTriggerType returns the trigger type for this adapter
func (b *BaseAdapter) GetTriggerType() TriggerType {
	return b.triggerType
}

// validateRecordsEvent validates events that have a Records structure (S3, SQS, etc.)
func validateRecordsEvent(event any, eventSource string, requiredFields []string) error {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return fmt.Errorf("event must be a map[string]any")
	}

	// Check for Records field
	records, exists := eventMap["Records"]
	if !exists {
		return fmt.Errorf("missing required field: records")
	}

	// Validate records structure
	recordsSlice, ok := records.([]any)
	if !ok {
		return fmt.Errorf("records must be a slice")
	}

	if len(recordsSlice) == 0 {
		return fmt.Errorf("records slice cannot be empty")
	}

	// Validate first record
	firstRecord, ok := recordsSlice[0].(map[string]any)
	if !ok {
		return fmt.Errorf("records must contain map objects")
	}

	// Check eventSource if specified
	if eventSource != "" {
		actualSource := extractStringField(firstRecord, "eventSource")
		if actualSource != eventSource {
			return fmt.Errorf("expected eventSource %s, got %s", eventSource, actualSource)
		}
	}

	// Check required fields in record
	for _, field := range requiredFields {
		if _, exists := firstRecord[field]; !exists {
			return fmt.Errorf("missing required field in record: %s", field)
		}
	}

	return nil
}

// extractStringField safely extracts a string field from a map
func extractStringField(data map[string]any, key string) string {
	if value, exists := data[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// extractMapField safely extracts a map field from a map
func extractMapField(data map[string]any, key string) map[string]any {
	if value, exists := data[key]; exists {
		if mapValue, ok := value.(map[string]any); ok {
			return mapValue
		}
	}
	return make(map[string]any)
}

// extractStringMapField safely extracts a string map field from a map
// Handles both map[string]string and map[string]any input types
func extractStringMapField(data map[string]any, key string) map[string]string {
	result := make(map[string]string)
	if value, exists := data[key]; exists {
		// Handle map[string]string directly
		if stringMap, ok := value.(map[string]string); ok {
			for k, v := range stringMap {
				result[k] = v
			}
		} else if mapValue, ok := value.(map[string]any); ok {
			// Handle map[string]any by converting values to strings
			for k, v := range mapValue {
				if str, ok := v.(string); ok {
					result[k] = str
				}
			}
		}
	}
	return result
}

// extractSliceField safely extracts a slice field from a map
func extractSliceField(data map[string]any, key string) []any {
	if value, exists := data[key]; exists {
		if slice, ok := value.([]any); ok {
			return slice
		}
	}
	return make([]any, 0)
}
