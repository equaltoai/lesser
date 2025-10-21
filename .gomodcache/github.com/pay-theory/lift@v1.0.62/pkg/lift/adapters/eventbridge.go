package adapters

import (
	"fmt"
)

// EventBridgeAdapter adapts Amazon EventBridge events into the normalized
// Request structure used by Lift.
type EventBridgeAdapter struct {
	BaseAdapter
}

// NewEventBridgeAdapter creates a new EventBridge adapter.
func NewEventBridgeAdapter() *EventBridgeAdapter {
	return &EventBridgeAdapter{
		BaseAdapter: BaseAdapter{triggerType: TriggerEventBridge},
	}
}

// CanHandle reports whether the adapter recognizes the given raw event as an
// EventBridge event.
func (a *EventBridgeAdapter) CanHandle(event any) bool {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return false
	}

	// Check for EventBridge specific fields
	_, hasSource := eventMap["source"]
	_, hasDetailType := eventMap["detail-type"]
	_, hasDetail := eventMap["detail"]
	_, hasTime := eventMap["time"]

	// EventBridge events have source, detail-type, detail, and time fields
	return hasSource && hasDetailType && hasDetail && hasTime
}

// Validate checks that the raw event has the required EventBridge fields before
// adapting it.
func (a *EventBridgeAdapter) Validate(event any) error {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return fmt.Errorf("event must be a map[string]any")
	}

	// Check required fields
	requiredFields := []string{"source", "detail-type", "detail", "time"}
	for _, field := range requiredFields {
		if _, exists := eventMap[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	return nil
}

// Adapt converts an EventBridge event into a normalized Request.
func (a *EventBridgeAdapter) Adapt(rawEvent any) (*Request, error) {
	if err := a.Validate(rawEvent); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	eventMap, ok := rawEvent.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event must be a map[string]any, got %T", rawEvent)
	}

	// Extract EventBridge specific fields
	source := extractStringField(eventMap, "source")
	detailType := extractStringField(eventMap, "detail-type")
	detail := extractMapField(eventMap, "detail")
	timestamp := extractStringField(eventMap, "time")
	eventID := extractStringField(eventMap, "id")

	// Extract resources (for scheduled events)
	resources := extractSliceField(eventMap, "resources")

	// Determine the actual trigger type based on source
	triggerType := TriggerEventBridge
	switch source {
	case "aws.s3":
		triggerType = TriggerS3
	case "aws.sqs":
		triggerType = TriggerSQS
		// aws.events remains as EventBridge for scheduled events
	}

	return &Request{
		TriggerType: triggerType,
		RawEvent:    rawEvent,
		EventID:     eventID,
		Timestamp:   timestamp,
		Source:      source,
		DetailType:  detailType,
		Detail:      detail,
		Records:     resources,
	}, nil
}
