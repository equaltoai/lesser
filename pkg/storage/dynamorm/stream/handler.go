package stream

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
)

// Handler is a generic interface for DynamoDB stream handlers
type Handler interface {
	HandleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error
}

// BaseHandler provides common functionality for DynamoDB stream handlers
type BaseHandler struct {
	DB        *dynamorm.LambdaDB
	TableName string
	Logger    *zap.Logger
}

// NewBaseHandler creates a new BaseHandler
func NewBaseHandler(db *dynamorm.LambdaDB, tableName string) *BaseHandler {
	return &BaseHandler{
		DB:        db,
		TableName: tableName,
		Logger:    common.Logger(),
	}
}

// HandleDynamoDBStream processes DynamoDB stream events
func (h *BaseHandler) HandleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error {
	h.Logger.Info("Processing DynamoDB stream event",
		zap.Int("records", len(event.Records)),
	)

	for _, record := range event.Records {
		if err := h.processRecord(ctx, record); err != nil {
			h.Logger.Error("Failed to process record",
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

// processRecord processes a single DynamoDB stream record
// This is a base implementation that should be overridden by specific handlers
func (h *BaseHandler) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Extract event type for logging
	eventType, err := GetEventType(record)
	if err != nil {
		h.Logger.Warn("Could not determine event type", zap.Error(err))
		eventType = "unknown"
	}

	h.Logger.Info("BaseHandler processing record - concrete handler should override this method",
		zap.String("eventID", record.EventID),
		zap.String("eventName", record.EventName),
		zap.String("eventType", eventType),
	)

	// Base implementation does nothing but log - this is expected behavior
	// Concrete handlers should override this method to provide specific processing logic
	return nil
}

// GetEventType extracts the entity type from a DynamoDB stream record
func GetEventType(record events.DynamoDBEventRecord) (string, error) {
	// Extract PK to determine entity type
	var pk string

	switch record.EventName {
	case "INSERT", "MODIFY":
		if pkAttr, ok := record.Change.NewImage["PK"]; ok {
			pk = pkAttr.String()
		}
	case "REMOVE":
		if pkAttr, ok := record.Change.OldImage["PK"]; ok {
			pk = pkAttr.String()
		}
	default:
		return "", fmt.Errorf("unknown event type: %s", record.EventName)
	}

	if pk == "" {
		return "", fmt.Errorf("PK not found in record")
	}

	// Extract entity type from PK (format: entity_type#id)
	parts := common.SplitKey(pk)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid PK format: %s", pk)
	}

	return parts[0], nil
}
