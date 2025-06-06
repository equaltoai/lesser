package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/moderation"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// ModerationRecord represents how moderation events are stored in DynamoDB
type ModerationRecord struct {
	PK        string                         `dynamodbav:"PK"`
	SK        string                         `dynamodbav:"SK"`
	GSI1PK    string                         `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK    string                         `dynamodbav:"GSI1SK,omitempty"`
	GSI2PK    string                         `dynamodbav:"GSI2PK,omitempty"`
	GSI2SK    string                         `dynamodbav:"GSI2SK,omitempty"`
	GSI3PK    string                         `dynamodbav:"GSI3PK,omitempty"`
	GSI3SK    string                         `dynamodbav:"GSI3SK,omitempty"`
	Type      string                         `dynamodbav:"Type"`
	Event     *moderation.ModerationEvent    `dynamodbav:"Event,omitempty"`
	Review    *moderation.Review             `dynamodbav:"Review,omitempty"`
	Decision  *moderation.ModerationDecision `dynamodbav:"Decision,omitempty"`
	TTL       int64                          `dynamodbav:"TTL,omitempty"`
	CreatedAt time.Time                      `dynamodbav:"CreatedAt"`
}

// CreateModerationEvent creates a new moderation event
func (s *dynamoDBStorage) CreateModerationEvent(ctx context.Context, event *moderation.ModerationEvent) error {
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt_%s", generateRandomString(12))
	}
	event.Created = time.Now()
	event.Updated = event.Created

	// Set TTL if not specified (30 days default)
	if event.TTL == 0 {
		event.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}

	record := &ModerationRecord{
		PK:        fmt.Sprintf("EVENT#%s", event.ObjectID),
		SK:        fmt.Sprintf("TIME#%s#%s", event.Created.Format(time.RFC3339), event.ID),
		GSI1PK:    fmt.Sprintf("ACTOR#%s", event.ActorID),
		GSI1SK:    fmt.Sprintf("TIME#%s", event.Created.Format(time.RFC3339)),
		GSI2PK:    fmt.Sprintf("TYPE#%s#%s", event.EventType, event.Category),
		GSI2SK:    fmt.Sprintf("SEVERITY#%d#%s", event.Severity, event.Created.Format(time.RFC3339)),
		GSI3PK:    fmt.Sprintf("EVENTID#%s", event.ID),
		GSI3SK:    fmt.Sprintf("EVENTID#%s", event.ID),
		Type:      "EVENT",
		Event:     event,
		TTL:       event.TTL,
		CreatedAt: event.Created,
	}

	av, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal moderation event: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	if err != nil {
		return fmt.Errorf("failed to create moderation event: %w", err)
	}

	common.Logger().Debug("Created moderation event",
		zap.String("event_id", event.ID),
		zap.String("object_id", event.ObjectID),
		zap.String("type", string(event.EventType)),
	)

	return nil
}

// GetModerationEvent retrieves a moderation event by ID
func (s *dynamoDBStorage) GetModerationEvent(ctx context.Context, eventID string) (*moderation.ModerationEvent, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("EVENTID#%s", eventID)},
		},
		Limit: aws.Int32(1),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get moderation event: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("moderation event not found")
	}

	var record ModerationRecord
	err = attributevalue.UnmarshalMap(result.Items[0], &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal moderation event: %w", err)
	}

	return record.Event, nil
}

// GetModerationQueue retrieves pending moderation events
func (s *dynamoDBStorage) GetModerationQueue(ctx context.Context, limit int, cursor string) ([]*moderation.QueueItem, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)},
		},
		ScanIndexForward: aws.Bool(false), // Newest first
		Limit:            aws.Int32(int32(limit)),
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI2PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)},
			"GSI2SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query moderation queue: %w", err)
	}

	items := make([]*moderation.QueueItem, 0, len(result.Items))
	for _, item := range result.Items {
		var record ModerationRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal moderation record", zap.Error(err))
			continue
		}

		if record.Event != nil {
			// Get review count for this event
			reviewCount, _ := s.countReviews(ctx, record.Event.ID)

			queueItem := &moderation.QueueItem{
				Event:       record.Event,
				Priority:    float64(record.Event.Severity) * record.Event.ConfidenceScore,
				ReviewCount: reviewCount,
			}
			items = append(items, queueItem)
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI2SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return items, nextCursor, nil
}

// GetModerationEventsByObject retrieves all moderation events for an object
func (s *dynamoDBStorage) GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*moderation.ModerationEvent, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("EVENT#%s", objectID)},
		},
		ScanIndexForward: aws.Bool(false), // Newest first
		Limit:            aws.Int32(int32(limit)),
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EVENT#%s", objectID)},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query moderation events: %w", err)
	}

	events := make([]*moderation.ModerationEvent, 0, len(result.Items))
	for _, item := range result.Items {
		var record ModerationRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal moderation record", zap.Error(err))
			continue
		}

		if record.Type == "EVENT" && record.Event != nil {
			events = append(events, record.Event)
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return events, nextCursor, nil
}

// GetModerationEventsByActor retrieves all moderation events created by an actor
func (s *dynamoDBStorage) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*moderation.ModerationEvent, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actorID)},
		},
		ScanIndexForward: aws.Bool(false), // Newest first
		Limit:            aws.Int32(int32(limit)),
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actorID)},
			"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query moderation events by actor: %w", err)
	}

	events := make([]*moderation.ModerationEvent, 0, len(result.Items))
	for _, item := range result.Items {
		var record ModerationRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal moderation record", zap.Error(err))
			continue
		}

		if record.Type == "EVENT" && record.Event != nil {
			events = append(events, record.Event)
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["GSI1SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return events, nextCursor, nil
}

// AddModerationReview adds a review to a moderation event
func (s *dynamoDBStorage) AddModerationReview(ctx context.Context, review *moderation.Review) error {
	if review.ID == "" {
		review.ID = fmt.Sprintf("rev_%s", generateRandomString(12))
	}
	review.Created = time.Now()

	record := &ModerationRecord{
		PK:        fmt.Sprintf("REVIEW#%s", review.EventID),
		SK:        fmt.Sprintf("REVIEWER#%s", review.ReviewerID),
		Type:      "REVIEW",
		Review:    review,
		CreatedAt: review.Created,
		TTL:       time.Now().Add(30 * 24 * time.Hour).Unix(),
	}

	av, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal review: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	if err != nil {
		return fmt.Errorf("failed to add review: %w", err)
	}

	common.Logger().Debug("Added moderation review",
		zap.String("review_id", review.ID),
		zap.String("event_id", review.EventID),
		zap.String("reviewer", review.ReviewerID),
	)

	return nil
}

// GetModerationReviews retrieves all reviews for a moderation event
func (s *dynamoDBStorage) GetModerationReviews(ctx context.Context, eventID string) ([]*moderation.Review, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("REVIEW#%s", eventID)},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query reviews: %w", err)
	}

	reviews := make([]*moderation.Review, 0, len(result.Items))
	for _, item := range result.Items {
		var record ModerationRecord
		err = attributevalue.UnmarshalMap(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal review record", zap.Error(err))
			continue
		}

		if record.Type == "REVIEW" && record.Review != nil {
			reviews = append(reviews, record.Review)
		}
	}

	return reviews, nil
}

// CreateModerationDecision creates a consensus decision
func (s *dynamoDBStorage) CreateModerationDecision(ctx context.Context, decision *moderation.ModerationDecision) error {
	if decision.ID == "" {
		decision.ID = fmt.Sprintf("dec_%s", generateRandomString(12))
	}
	decision.Decided = time.Now()

	record := &ModerationRecord{
		PK:        fmt.Sprintf("DECISION#%s", decision.ObjectID),
		SK:        fmt.Sprintf("TIME#%s", decision.Decided.Format(time.RFC3339)),
		GSI1PK:    "ACTIVE_DECISIONS",
		GSI1SK:    fmt.Sprintf("OBJECT#%s", decision.ObjectID),
		Type:      "DECISION",
		Decision:  decision,
		CreatedAt: decision.Decided,
		TTL:       time.Now().Add(90 * 24 * time.Hour).Unix(), // 90 days retention
	}

	av, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal decision: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	if err != nil {
		return fmt.Errorf("failed to create decision: %w", err)
	}

	common.Logger().Info("Created moderation decision",
		zap.String("decision_id", decision.ID),
		zap.String("object_id", decision.ObjectID),
		zap.String("action", string(decision.Action)),
		zap.Float64("consensus", decision.ConsensusScore),
	)

	return nil
}

// GetModerationDecision retrieves the current decision for an object
func (s *dynamoDBStorage) GetModerationDecision(ctx context.Context, objectID string) (*moderation.ModerationDecision, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "ACTIVE_DECISIONS"},
			":sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectID)},
		},
		Limit: aws.Int32(1),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get moderation decision: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil // No decision yet
	}

	var record ModerationRecord
	err = attributevalue.UnmarshalMap(result.Items[0], &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal decision: %w", err)
	}

	return record.Decision, nil
}

// GetModerationHistory retrieves the complete moderation history for an object
func (s *dynamoDBStorage) GetModerationHistory(ctx context.Context, objectID string) (*moderation.ModerationHistory, error) {
	history := &moderation.ModerationHistory{
		ObjectID:  objectID,
		Events:    []moderation.ModerationEvent{},
		Decisions: []moderation.ModerationDecision{},
		Timeline:  []moderation.TimelineEntry{},
	}

	// Get all events
	events, _, err := s.GetModerationEventsByObject(ctx, objectID, 100, "")
	if err != nil {
		return nil, err
	}
	history.Events = make([]moderation.ModerationEvent, len(events))
	for i, e := range events {
		history.Events[i] = *e
	}

	// Get all decisions
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("DECISION#%s", objectID)},
		},
	})

	if err == nil {
		for _, item := range result.Items {
			var record ModerationRecord
			err = attributevalue.UnmarshalMap(item, &record)
			if err == nil && record.Type == "DECISION" && record.Decision != nil {
				history.Decisions = append(history.Decisions, *record.Decision)
			}
		}
	}

	// Build timeline
	for _, event := range history.Events {
		history.Timeline = append(history.Timeline, moderation.TimelineEntry{
			Timestamp:   event.Created,
			Type:        "event",
			ActorID:     event.ActorID,
			Description: fmt.Sprintf("%s event: %s", event.EventType, event.Category),
			Metadata: map[string]interface{}{
				"event_id": event.ID,
				"severity": event.Severity,
			},
		})
	}

	for _, decision := range history.Decisions {
		history.Timeline = append(history.Timeline, moderation.TimelineEntry{
			Timestamp:   decision.Decided,
			Type:        "decision",
			ActorID:     "system",
			Description: fmt.Sprintf("Decision: %s (consensus: %.2f)", decision.Action, decision.ConsensusScore),
			Metadata: map[string]interface{}{
				"decision_id": decision.ID,
				"action":      decision.Action,
			},
		})
	}

	// Determine current status
	if len(history.Decisions) > 0 {
		lastDecision := history.Decisions[len(history.Decisions)-1]
		history.CurrentStatus = string(lastDecision.Action)
	} else {
		history.CurrentStatus = "pending"
	}

	return history, nil
}

// countReviews is a helper to count reviews for an event
func (s *dynamoDBStorage) countReviews(ctx context.Context, eventID string) (int, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("REVIEW#%s", eventID)},
		},
		Select: types.SelectCount,
	})

	if err != nil {
		return 0, err
	}

	return int(result.Count), nil
}
