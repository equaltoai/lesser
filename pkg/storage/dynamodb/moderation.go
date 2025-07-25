package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/moderation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
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

	av, err := s.MarshalItem(record)
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
	err = s.UnmarshalItem(result.Items[0], &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal moderation event: %w", err)
	}

	return record.Event, nil
}

// GetModerationQueue retrieves pending moderation events
func (s *dynamoDBStorage) GetModerationQueue(ctx context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error) {
	limit := 50 // Default limit
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)},
		},
		ScanIndexForward: aws.Bool(false), // Newest first
		Limit:            safeInt32(limit),
	}

	// Apply time filters
	if !filter.StartTime.IsZero() || !filter.EndTime.IsZero() {
		var conditionExpr string
		if !filter.StartTime.IsZero() && !filter.EndTime.IsZero() {
			conditionExpr = "GSI2SK BETWEEN :start_time AND :end_time"
			input.ExpressionAttributeValues[":start_time"] = &types.AttributeValueMemberS{Value: filter.StartTime.Format(time.RFC3339)}
			input.ExpressionAttributeValues[":end_time"] = &types.AttributeValueMemberS{Value: filter.EndTime.Format(time.RFC3339)}
		} else if !filter.StartTime.IsZero() {
			conditionExpr = "GSI2SK >= :start_time"
			input.ExpressionAttributeValues[":start_time"] = &types.AttributeValueMemberS{Value: filter.StartTime.Format(time.RFC3339)}
		} else if !filter.EndTime.IsZero() {
			conditionExpr = "GSI2SK <= :end_time"
			input.ExpressionAttributeValues[":end_time"] = &types.AttributeValueMemberS{Value: filter.EndTime.Format(time.RFC3339)}
		}

		if input.FilterExpression == nil {
			input.FilterExpression = aws.String(conditionExpr)
		} else {
			input.FilterExpression = aws.String(*input.FilterExpression + " AND " + conditionExpr)
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query moderation queue: %w", err)
	}

	items := make([]*storage.ModerationQueueItem, 0, len(result.Items))
	for _, item := range result.Items {
		var record ModerationRecord
		err = s.UnmarshalItem(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal moderation record", zap.Error(err))
			continue
		}

		if record.Event != nil {
			// Apply score filters
			if filter.MinScore > 0 && record.Event.ConfidenceScore < filter.MinScore {
				continue
			}
			if filter.MaxScore > 0 && record.Event.ConfidenceScore > filter.MaxScore {
				continue
			}

			// Apply content type filter
			if filter.ContentType != "" && record.Event.ObjectType != filter.ContentType {
				continue
			}

			// Apply action filter
			if filter.Action != "" && string(record.Event.EventType) != filter.Action {
				continue
			}

			// Get review count for this event
			reviewCount, _ := s.countReviews(ctx, record.Event.ID)

			queueItem := &storage.ModerationQueueItem{
				Event:       record.Event,
				Priority:    float64(record.Event.Severity) * record.Event.ConfidenceScore,
				ReviewCount: reviewCount,
			}
			items = append(items, queueItem)
		}
	}

	return items, nil
}

// GetModerationQueuePaginated retrieves pending moderation events with pagination
func (s *dynamoDBStorage) GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)},
		},
		ScanIndexForward: aws.Bool(false), // Newest first
		Limit:            safeInt32(limit),
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

	items := make([]*storage.ModerationQueueItem, 0, len(result.Items))
	for _, item := range result.Items {
		var record ModerationRecord
		err = s.UnmarshalItem(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal moderation record", zap.Error(err))
			continue
		}

		if record.Event != nil {
			// Get review count for this event
			reviewCount, _ := s.countReviews(ctx, record.Event.ID)

			queueItem := &storage.ModerationQueueItem{
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
		Limit:            safeInt32(limit),
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
		err = s.UnmarshalItem(item, &record)
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
		Limit:            safeInt32(limit),
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
		err = s.UnmarshalItem(item, &record)
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

	av, err := s.MarshalItem(record)
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
		err = s.UnmarshalItem(item, &record)
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

	av, err := s.MarshalItem(record)
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
	err = s.UnmarshalItem(result.Items[0], &record)
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
			err = s.UnmarshalItem(item, &record)
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
			Metadata: map[string]any{
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
			Metadata: map[string]any{
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

// GetModerationEvents retrieves all moderation events with optional filters
func (s *dynamoDBStorage) GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*moderation.ModerationEvent, string, error) {
	// If no filter, use a scan to get all events
	if filter == nil || (filter.EventType == nil && filter.Category == nil && filter.ActorID == "" && filter.ObjectID == "") {
		return s.scanAllModerationEvents(ctx, filter, limit, cursor)
	}

	// Use query based on the most selective filter
	if filter.ObjectID != "" {
		return s.GetModerationEventsByObject(ctx, filter.ObjectID, limit, cursor)
	}

	if filter.ActorID != "" {
		return s.GetModerationEventsByActor(ctx, filter.ActorID, limit, cursor)
	}

	// Query by event type and category using GSI2
	if filter.EventType != nil || filter.Category != nil {
		eventType := moderation.EventTypeFlagged
		if filter.EventType != nil {
			eventType = *filter.EventType
		}

		category := ""
		if filter.Category != nil {
			category = string(*filter.Category)
		}

		gsi2pk := fmt.Sprintf("TYPE#%s", eventType)
		if category != "" {
			gsi2pk = fmt.Sprintf("TYPE#%s#%s", eventType, category)
		}

		input := &dynamodb.QueryInput{
			TableName:              s.getTableName(),
			IndexName:              aws.String("GSI2"),
			KeyConditionExpression: aws.String("GSI2PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: gsi2pk},
			},
			ScanIndexForward: aws.Bool(false), // Newest first
			Limit:            safeInt32(limit),
		}

		if cursor != "" {
			input.ExclusiveStartKey = map[string]types.AttributeValue{
				"GSI2PK": &types.AttributeValueMemberS{Value: gsi2pk},
				"GSI2SK": &types.AttributeValueMemberS{Value: cursor},
			}
		}

		result, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, "", fmt.Errorf("failed to query moderation events: %w", err)
		}

		events := make([]*moderation.ModerationEvent, 0, len(result.Items))
		for _, item := range result.Items {
			var record ModerationRecord
			err = s.UnmarshalItem(item, &record)
			if err != nil {
				common.Logger().Error("Failed to unmarshal moderation record", zap.Error(err))
				continue
			}

			if record.Type == "EVENT" && record.Event != nil {
				// Apply additional filters
				if s.matchesEventFilter(record.Event, filter) {
					events = append(events, record.Event)
				}
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

		return events, nextCursor, nil
	}

	// Fallback to scan
	return s.scanAllModerationEvents(ctx, filter, limit, cursor)
}

// scanAllModerationEvents performs a scan operation to get all events
func (s *dynamoDBStorage) scanAllModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*moderation.ModerationEvent, string, error) {
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("#type = :type"),
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{Value: "EVENT"},
		},
		Limit: safeInt32(limit),
	}

	if cursor != "" {
		// Decode cursor to exclusive start key
		// In production, implement proper cursor encoding/decoding
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: cursor},
			"SK": &types.AttributeValueMemberS{Value: ""},
		}
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan moderation events: %w", err)
	}

	events := make([]*moderation.ModerationEvent, 0, len(result.Items))
	for _, item := range result.Items {
		var record ModerationRecord
		err = s.UnmarshalItem(item, &record)
		if err != nil {
			common.Logger().Error("Failed to unmarshal moderation record", zap.Error(err))
			continue
		}

		if record.Type == "EVENT" && record.Event != nil {
			// Apply filters
			if s.matchesEventFilter(record.Event, filter) {
				events = append(events, record.Event)
			}
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if pk, ok := result.LastEvaluatedKey["PK"]; ok {
			if pkStr, ok := pk.(*types.AttributeValueMemberS); ok {
				nextCursor = pkStr.Value
			}
		}
	}

	return events, nextCursor, nil
}

// matchesEventFilter checks if an event matches the given filter
func (s *dynamoDBStorage) matchesEventFilter(event *moderation.ModerationEvent, filter *storage.ModerationEventFilter) bool {
	if filter == nil {
		return true
	}

	if filter.EventType != nil && event.EventType != *filter.EventType {
		return false
	}

	if filter.Category != nil && event.Category != *filter.Category {
		return false
	}

	if filter.MinSeverity != nil && event.Severity < *filter.MinSeverity {
		return false
	}

	if filter.StartTime != nil && event.Created.Before(*filter.StartTime) {
		return false
	}

	if filter.EndTime != nil && event.Created.After(*filter.EndTime) {
		return false
	}

	return true
}

// CreateAdminReview creates an admin review that overrides consensus
func (s *dynamoDBStorage) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	// Create a special review with maximum weight
	review := &moderation.Review{
		ID:         fmt.Sprintf("admin_rev_%s", generateRandomString(12)),
		EventID:    eventID,
		ReviewerID: adminID,
		Action:     action,
		Category:   moderation.CategoryOther,    // Admin override doesn't need specific category
		Severity:   moderation.SeverityCritical, // Max severity for admin actions
		Confidence: 1.0,                         // Full confidence
		Notes:      fmt.Sprintf("Admin override: %s", reason),
		Weight:     1000.0, // Very high weight to override consensus
		Created:    time.Now(),
	}

	// Add the review
	if err := s.AddModerationReview(ctx, review); err != nil {
		return fmt.Errorf("failed to add admin review: %w", err)
	}

	// Immediately create a decision based on the admin action
	decision := &moderation.ModerationDecision{
		ID:               fmt.Sprintf("admin_dec_%s", generateRandomString(12)),
		EventID:          eventID,
		ObjectID:         "", // Will be filled from event
		Action:           action,
		ConsensusScore:   1.0, // Admin override has full consensus
		ReviewerCount:    1,
		TrustWeightTotal: 1000.0,
		Reviews:          []*moderation.Review{review},
		Decided:          time.Now(),
	}

	// Get the event to fill in the object ID
	event, err := s.GetModerationEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to get moderation event: %w", err)
	}
	decision.ObjectID = event.ObjectID

	// Create the decision
	if err := s.CreateModerationDecision(ctx, decision); err != nil {
		return fmt.Errorf("failed to create admin decision: %w", err)
	}

	common.Logger().Info("Admin override created",
		zap.String("admin", adminID),
		zap.String("event_id", eventID),
		zap.String("action", string(action)),
		zap.String("reason", reason),
	)

	return nil
}

// GetReviewerStats retrieves statistics for a reviewer
func (s *dynamoDBStorage) GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error) {
	stats := &storage.ReviewerStats{
		ReviewerID:        reviewerID,
		TotalReviews:      0,
		AccurateReviews:   0,
		AccuracyRate:      0.0,
		ReviewsByCategory: make(map[string]int),
		JoinedAt:          time.Now(), // Default, will be updated if we find reviews
	}

	// Get user to find when they joined
	user, err := s.GetUser(ctx, reviewerID)
	if err == nil {
		stats.JoinedAt = user.CreatedAt
	}

	// Get trust score for moderation category
	trustScore, err := s.GetTrustScore(ctx, reviewerID, "moderation")
	if err == nil && trustScore != nil {
		stats.TrustScore = trustScore.Score
	}

	// Query all reviews by this reviewer
	// We'll need to scan since we don't have a GSI for reviewer lookups
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("#type = :type AND Review.ReviewerID = :reviewer"),
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type":     &types.AttributeValueMemberS{Value: "REVIEW"},
			":reviewer": &types.AttributeValueMemberS{Value: reviewerID},
		},
	}

	var lastReviewTime time.Time
	paginator := dynamodb.NewScanPaginator(s.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reviews: %w", err)
		}

		for _, item := range page.Items {
			var record ModerationRecord
			err = s.UnmarshalItem(item, &record)
			if err != nil {
				continue
			}

			if record.Type == "REVIEW" && record.Review != nil && record.Review.ReviewerID == reviewerID {
				stats.TotalReviews++

				// Track by category
				category := string(record.Review.Category)
				stats.ReviewsByCategory[category]++

				// Update last review time
				if record.Review.Created.After(lastReviewTime) {
					lastReviewTime = record.Review.Created
				}

				// Check if this review was part of a consensus that matched the final decision
				// This is simplified - in production you'd check against the actual decision
				if record.Review.Weight > 0.5 { // Simplified accuracy check
					stats.AccurateReviews++
				}
			}
		}
	}

	stats.LastReviewAt = lastReviewTime

	// Calculate accuracy rate
	if stats.TotalReviews > 0 {
		stats.AccuracyRate = float64(stats.AccurateReviews) / float64(stats.TotalReviews)
	}

	return stats, nil
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

// StoreModerationDecision stores a moderation decision
func (s *dynamoDBStorage) StoreModerationDecision(ctx context.Context, decision *moderation.ModerationDecision) error {
	if decision.ID == "" {
		decision.ID = fmt.Sprintf("dec_%s", generateRandomString(12))
	}
	decision.Decided = time.Now()

	record := &ModerationRecord{
		PK:        fmt.Sprintf("DECISION#%s", decision.EventID),
		SK:        fmt.Sprintf("TIME#%s", decision.Decided.Format(time.RFC3339)),
		Type:      "DECISION",
		Decision:  decision,
		CreatedAt: decision.Decided,
		TTL:       time.Now().Add(90 * 24 * time.Hour).Unix(), // Keep decisions longer
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal moderation decision: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to store moderation decision: %w", err)
	}

	return nil
}

// UpdateModerationDecision updates a moderation decision based on a review
func (s *dynamoDBStorage) UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error {
	// Get the current decision for the content
	currentDecision, err := s.GetModerationDecision(ctx, contentID)
	if err != nil {
		return fmt.Errorf("failed to get current moderation decision: %w", err)
	}

	// If no decision exists, we cannot update it
	if currentDecision == nil {
		return fmt.Errorf("no moderation decision exists for content ID: %s", contentID)
	}

	// Create a new moderation decision based on the review
	newDecision := &moderation.ModerationDecision{
		ID:               fmt.Sprintf("dec_%s", generateRandomString(12)),
		EventID:          currentDecision.EventID,
		ObjectID:         contentID,
		Action:           review.Action,
		ConsensusScore:   review.Confidence,
		ReviewerCount:    1,
		TrustWeightTotal: review.Weight,
		Reviews: []*moderation.Review{
			{
				ID:         fmt.Sprintf("rev_%s", generateRandomString(12)),
				EventID:    currentDecision.EventID,
				ReviewerID: review.ReviewerID,
				Action:     review.Action,
				Category:   review.Category,
				Severity:   review.Severity,
				Confidence: review.Confidence,
				Notes:      review.Notes,
				Weight:     review.Weight,
				Created:    time.Now(),
			},
		},
		Decided: time.Now(),
	}

	// Create the updated decision
	if err := s.CreateModerationDecision(ctx, newDecision); err != nil {
		return fmt.Errorf("failed to create updated moderation decision: %w", err)
	}

	common.Logger().Info("Updated moderation decision",
		zap.String("content_id", contentID),
		zap.String("reviewer", review.ReviewerID),
		zap.String("action", string(review.Action)),
		zap.Float64("confidence", review.Confidence),
	)

	return nil
}
