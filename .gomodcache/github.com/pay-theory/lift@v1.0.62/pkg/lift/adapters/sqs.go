package adapters

import (
	"encoding/json"
	"fmt"
)

// SQSAdapter adapts Amazon SQS events into the normalized Request structure
// used by Lift.
type SQSAdapter struct {
	BaseAdapter
}

// NewSQSAdapter creates a new SQS adapter.
func NewSQSAdapter() *SQSAdapter {
	return &SQSAdapter{
		BaseAdapter: BaseAdapter{triggerType: TriggerSQS},
	}
}

// CanHandle reports whether the adapter recognizes the given raw event as an
// SQS event.
func (a *SQSAdapter) CanHandle(event any) bool {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return false
	}

	// Check for SQS specific fields
	records, hasRecords := eventMap["Records"]
	if !hasRecords {
		return false
	}

	// Check if records is a slice
	recordsSlice, ok := records.([]any)
	if !ok || len(recordsSlice) == 0 {
		return false
	}

	// Check first record for SQS specific fields
	firstRecord, ok := recordsSlice[0].(map[string]any)
	if !ok {
		return false
	}

	// SQS records have eventSource "aws:sqs"
	eventSource := extractStringField(firstRecord, "eventSource")
	return eventSource == "aws:sqs"
}

// Validate checks that the raw event has the required SQS record structure
// before adapting it.
func (a *SQSAdapter) Validate(event any) error {
	requiredFields := []string{"eventSource", "body", "receiptHandle"}
	return validateRecordsEvent(event, "aws:sqs", requiredFields)
}

// Adapt converts an SQS event into a normalized Request.
func (a *SQSAdapter) Adapt(rawEvent any) (*Request, error) {
	if err := a.Validate(rawEvent); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	eventMap, ok := rawEvent.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event must be a map[string]any, got %T", rawEvent)
	}
	records := extractSliceField(eventMap, "Records")
	body, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("failed to encode SQS records: %w", err)
	}

	// Extract metadata from first record for event-level info
	var eventID, timestamp string
	if len(records) > 0 {
		if firstRecord, ok := records[0].(map[string]any); ok {
			eventID = extractStringField(firstRecord, "messageId")
			timestamp = extractStringField(firstRecord, "attributes.SentTimestamp")
		}
	}

	return &Request{
		TriggerType: TriggerSQS,
		RawEvent:    rawEvent,
		EventID:     eventID,
		Timestamp:   timestamp,
		Records:     records,
		Body:        body,
		Source:      "aws:sqs",
	}, nil
}
