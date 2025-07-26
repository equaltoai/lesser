package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ActivityDirection represents the direction of an activity (inbox or outbox)
type ActivityDirection string

const (
	InboxDirection  ActivityDirection = "inbox"
	OutboxDirection ActivityDirection = "outbox"
)

// ActivityHandler processes DynamoDB stream events for activities
type ActivityHandler struct {
	DB        core.DB
	TableName string
	Logger    *zap.Logger
}

// NewActivityHandler creates a new ActivityHandler
func NewActivityHandler(db core.DB, tableName string) *ActivityHandler {
	return &ActivityHandler{
		DB:        db,
		TableName: tableName,
		Logger:    zap.L(),
	}
}

// processRecord overrides the BaseHandler's processRecord method
func (h *ActivityHandler) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Only process INSERT events for activities
	if record.EventName != "INSERT" {
		return nil
	}

	// Extract entity type from PK
	entityType, err := stream.GetEventType(record)
	if err != nil {
		return fmt.Errorf("failed to get entity type: %w", err)
	}

	// Only process activity records
	if entityType != "activity" {
		return nil
	}

	// Unmarshal the activity record
	var activityRecord models.Activity
	if err := stream.UnmarshalItem(record, &activityRecord); err != nil {
		return fmt.Errorf("failed to unmarshal activity record: %w", err)
	}

	// Parse the activity
	activity, err := activitypub.ParseActivity([]byte(activityRecord.Activity))
	if err != nil {
		return fmt.Errorf("failed to parse activity: %w", err)
	}

	// Determine direction (inbox or outbox)
	direction := InboxDirection
	if strings.Contains(activityRecord.SK, "outbox") {
		direction = OutboxDirection
	}

	// Process the activity based on direction
	switch direction {
	case InboxDirection:
		return h.processInboxActivity(ctx, activity, activityRecord.Username)
	case OutboxDirection:
		return h.processOutboxActivity(ctx, activity, activityRecord.Username)
	default:
		return fmt.Errorf("unknown activity direction: %s", direction)
	}
}

// processInboxActivity processes an incoming activity
func (h *ActivityHandler) processInboxActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing inbox activity",
		zap.String("type", activity.Type),
		zap.String("username", username),
		zap.String("id", activity.ID),
	)

	// Process based on activity type
	switch activity.Type {
	case "Follow":
		return h.processFollowActivity(ctx, activity, username)
	case "Accept":
		return h.processAcceptActivity(ctx, activity, username)
	case "Create":
		return h.processCreateActivity(ctx, activity, username)
	default:
		h.Logger.Info("Ignoring unsupported activity type",
			zap.String("type", activity.Type),
		)
		return nil
	}
}

// processOutboxActivity processes an outgoing activity
func (h *ActivityHandler) processOutboxActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing outbox activity",
		zap.String("type", activity.Type),
		zap.String("username", username),
		zap.String("id", activity.ID),
	)

	// Process based on activity type
	switch activity.Type {
	case "Create":
		return h.deliverActivity(ctx, activity, username)
	case "Follow":
		return h.deliverActivity(ctx, activity, username)
	case "Accept":
		return h.deliverActivity(ctx, activity, username)
	default:
		h.Logger.Info("Ignoring unsupported activity type",
			zap.String("type", activity.Type),
		)
		return nil
	}
}

// processFollowActivity processes a Follow activity
func (h *ActivityHandler) processFollowActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	// Implementation would go here
	h.Logger.Info("Processing Follow activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
	)
	return nil
}

// processAcceptActivity processes an Accept activity
func (h *ActivityHandler) processAcceptActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	// Implementation would go here
	h.Logger.Info("Processing Accept activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
	)
	return nil
}

// processCreateActivity processes a Create activity
func (h *ActivityHandler) processCreateActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	// Implementation would go here
	h.Logger.Info("Processing Create activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
	)
	return nil
}

// deliverActivity delivers an activity to remote servers
func (h *ActivityHandler) deliverActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	// Implementation would go here
	h.Logger.Info("Delivering activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
		zap.String("type", activity.Type),
	)
	return nil
}
