package stream

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
)

// EventProcessor is a function type for processing DynamoDB stream events
type EventProcessor func(ctx context.Context, record events.DynamoDBEventRecord) error

// ProcessStreamEvent processes a DynamoDB stream event with the given processor function
func ProcessStreamEvent(ctx context.Context, event events.DynamoDBEvent, processor EventProcessor) error {
	logger := common.Logger()
	logger.Info("Processing DynamoDB stream event", zap.Int("records", len(event.Records)))

	for _, record := range event.Records {
		if err := processor(ctx, record); err != nil {
			logger.Error("Failed to process record",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Error(err),
			)
			// Continue processing other records
			continue
		}
	}

	return nil
}

// FilterRecordsByEntityType filters DynamoDB stream records by entity type
func FilterRecordsByEntityType(records []events.DynamoDBEventRecord, entityType string) []events.DynamoDBEventRecord {
	var filtered []events.DynamoDBEventRecord

	for _, record := range records {
		recordType, err := GetEventType(record)
		if err == nil && recordType == entityType {
			filtered = append(filtered, record)
		}
	}

	return filtered
}

// FilterRecordsByEventName filters DynamoDB stream records by event name
func FilterRecordsByEventName(records []events.DynamoDBEventRecord, eventNames ...string) []events.DynamoDBEventRecord {
	var filtered []events.DynamoDBEventRecord

	for _, record := range records {
		for _, name := range eventNames {
			if record.EventName == name {
				filtered = append(filtered, record)
				break
			}
		}
	}

	return filtered
}

// GetStringAttribute extracts a string attribute from a DynamoDB stream record
func GetStringAttribute(record events.DynamoDBEventRecord, key string) (string, error) {
	var attr events.DynamoDBAttributeValue
	var ok bool

	switch record.EventName {
	case eventNameInsert, eventNameModify:
		attr, ok = record.Change.NewImage[key]
	case eventNameRemove:
		attr, ok = record.Change.OldImage[key]
	default:
		return "", fmt.Errorf("unknown event type: %s", record.EventName)
	}

	if !ok {
		return "", fmt.Errorf("attribute %s not found", key)
	}

	return attr.String(), nil
}

// GetNumberAttribute extracts a number attribute from a DynamoDB stream record
func GetNumberAttribute(record events.DynamoDBEventRecord, key string) (string, error) {
	var attr events.DynamoDBAttributeValue
	var ok bool

	switch record.EventName {
	case eventNameInsert, eventNameModify:
		attr, ok = record.Change.NewImage[key]
	case eventNameRemove:
		attr, ok = record.Change.OldImage[key]
	default:
		return "", fmt.Errorf("unknown event type: %s", record.EventName)
	}

	if !ok {
		return "", fmt.Errorf("attribute %s not found", key)
	}

	return attr.Number(), nil
}

// GetBooleanAttribute extracts a boolean attribute from a DynamoDB stream record
func GetBooleanAttribute(record events.DynamoDBEventRecord, key string) (bool, error) {
	var attr events.DynamoDBAttributeValue
	var ok bool

	switch record.EventName {
	case eventNameInsert, eventNameModify:
		attr, ok = record.Change.NewImage[key]
	case eventNameRemove:
		attr, ok = record.Change.OldImage[key]
	default:
		return false, fmt.Errorf("unknown event type: %s", record.EventName)
	}

	if !ok {
		return false, fmt.Errorf("attribute %s not found", key)
	}

	return attr.Boolean(), nil
}

// ExtractEntityIDFromPK extracts the entity ID from a PK attribute
func ExtractEntityIDFromPK(pk string) (string, error) {
	parts := strings.Split(pk, "#")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid PK format: %s", pk)
	}
	return parts[1], nil
}

// CreateStreamHandler creates a new Lambda handler function for DynamoDB streams
func CreateStreamHandler(_ *dynamorm.LambdaDB, processor EventProcessor) func(ctx context.Context, event events.DynamoDBEvent) error {
	return func(ctx context.Context, event events.DynamoDBEvent) error {
		return ProcessStreamEvent(ctx, event, processor)
	}
}
