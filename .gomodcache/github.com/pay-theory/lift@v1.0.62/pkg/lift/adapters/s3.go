package adapters

import (
	"encoding/json"
	"fmt"
)

// S3Adapter adapts Amazon S3 events into the normalized Request structure used
// by Lift.
type S3Adapter struct {
	BaseAdapter
}

// NewS3Adapter creates a new S3 adapter.
func NewS3Adapter() *S3Adapter {
	return &S3Adapter{
		BaseAdapter: BaseAdapter{triggerType: TriggerS3},
	}
}

// CanHandle reports whether the adapter recognizes the given raw event as an
// S3 event.
func (a *S3Adapter) CanHandle(event any) bool {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return false
	}

	// Check for S3 specific fields
	records, hasRecords := eventMap["Records"]
	if !hasRecords {
		return false
	}

	// Check if records is a slice
	recordsSlice, ok := records.([]any)
	if !ok || len(recordsSlice) == 0 {
		return false
	}

	// Check first record for S3 specific fields
	firstRecord, ok := recordsSlice[0].(map[string]any)
	if !ok {
		return false
	}

	// S3 records have eventSource "aws:s3"
	eventSource := extractStringField(firstRecord, "eventSource")
	return eventSource == "aws:s3"
}

// Validate checks that the raw event has the required S3 record structure
// before adapting it.
func (a *S3Adapter) Validate(event any) error {
	requiredFields := []string{"eventSource", "eventName", "s3"}
	return validateRecordsEvent(event, "aws:s3", requiredFields)
}

// Adapt converts an S3 event into a normalized Request.
func (a *S3Adapter) Adapt(rawEvent any) (*Request, error) {
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
		return nil, fmt.Errorf("failed to encode S3 records: %w", err)
	}

	// Extract metadata from first record for event-level info
	var eventID, timestamp, eventName string
	if len(records) > 0 {
		if firstRecord, ok := records[0].(map[string]any); ok {
			eventID = extractStringField(firstRecord, "responseElements.x-amz-request-id")
			timestamp = extractStringField(firstRecord, "eventTime")
			eventName = extractStringField(firstRecord, "eventName")
		}
	}

	return &Request{
		TriggerType: TriggerS3,
		RawEvent:    rawEvent,
		EventID:     eventID,
		Timestamp:   timestamp,
		Records:     records,
		Body:        body,
		Source:      "aws:s3",
		DetailType:  eventName,
	}, nil
}
