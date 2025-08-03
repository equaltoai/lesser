package graph

import (

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// EventConverter converts streaming.InternalEvent to GraphQL model types
type EventConverter struct {
	logger *zap.Logger
}

// NewEventConverter creates a new event converter
func NewEventConverter(logger *zap.Logger) *EventConverter {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &EventConverter{
		logger: logger,
	}
}

// ConvertToObject converts a streaming event to a GraphQL Object
func (ec *EventConverter) ConvertToObject(event *streaming.InternalEvent) *model.Object {
	if event == nil {
		return nil
	}

	switch event.Type {
	case streaming.EventTypeStatus, streaming.EventTypeStatusUpdate:
		return ec.convertStatusToObject(event)
	default:
		ec.logger.Debug("cannot convert event type to Object",
			zap.String("event_type", string(event.Type)),
			zap.String("event_id", event.ID))
		return nil
	}
}

// ConvertToNotification converts a streaming event to a GraphQL Notification
func (ec *EventConverter) ConvertToNotification(event *streaming.InternalEvent) *model.Notification {
	if event == nil || event.Type != streaming.EventTypeNotification {
		return nil
	}

	payload, ok := event.Data.(*streaming.NotificationEventPayload)
	if !ok {
		ec.logger.Warn("invalid notification event payload",
			zap.String("event_id", event.ID),
			zap.String("event_type", string(event.Type)))
		return nil
	}

	return &model.Notification{
		ID:        payload.NotificationID,
		Type:      payload.Type,
		CreatedAt: model.Time(payload.CreatedAt),
		Account: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   payload.ActorID,
				Type: "Person",
			},
			PreferredUsername: extractUsernameFromID(payload.ActorID),
		},
		Status: func() *model.Object {
			if payload.StatusID == "" {
				return nil
			}
			return &model.Object{
				ID: payload.StatusID,
			}
		}(),
	}
}

// ConvertToCostUpdate converts a streaming event to a GraphQL CostUpdate
func (ec *EventConverter) ConvertToCostUpdate(event *streaming.InternalEvent) *model.CostUpdate {
	if event == nil {
		return nil
	}

	switch event.Type {
	case streaming.EventTypeCostUpdate, streaming.EventTypeCostAlert:
		return ec.convertCostEvent(event)
	default:
		return nil
	}
}

// ConvertToModerationDecision converts a streaming event to a moderation.ModerationDecision
func (ec *EventConverter) ConvertToModerationDecision(event *streaming.InternalEvent) *moderation.ModerationDecision {
	if event == nil {
		return nil
	}

	switch event.Type {
	case streaming.EventTypeModeration, streaming.EventTypeModerationFlag, streaming.EventTypeModerationReview:
		return ec.convertModerationEvent(event)
	default:
		return nil
	}
}

// ConvertToTrustEdge converts a streaming event to a trust.TrustEdge
func (ec *EventConverter) ConvertToTrustEdge(event *streaming.InternalEvent) *trust.TrustEdge {
	if event == nil {
		return nil
	}

	switch event.Type {
	case streaming.EventTypeTrustUpdate, streaming.EventTypeReputationUpdate, streaming.EventTypeVouchUpdate:
		return ec.convertTrustEvent(event)
	default:
		return nil
	}
}

// ConvertToAIAnalysis converts a streaming event to a model.AIAnalysis
func (ec *EventConverter) ConvertToAIAnalysis(event *streaming.InternalEvent) *model.AIAnalysis {
	if event == nil {
		return nil
	}

	switch event.Type {
	case streaming.EventTypeAIAnalysis, streaming.EventTypeAIClassification, streaming.EventTypeAIModeration:
		return ec.convertAIEvent(event)
	default:
		return nil
	}
}

// ConvertToHashtagActivity converts a streaming event to a model.HashtagActivityUpdate
func (ec *EventConverter) ConvertToHashtagActivity(event *streaming.InternalEvent) *model.HashtagActivityUpdate {
	if event == nil {
		return nil
	}

	switch event.Type {
	case streaming.EventTypeHashtagUpdate, streaming.EventTypeHashtagTrend:
		return ec.convertHashtagEvent(event)
	case streaming.EventTypeStatus:
		// Status events can also create hashtag activity
		return ec.convertStatusToHashtagActivity(event)
	default:
		return nil
	}
}

// ConvertToQuoteActivity converts a streaming event to a model.QuoteActivityUpdate
func (ec *EventConverter) ConvertToQuoteActivity(event *streaming.InternalEvent) *model.QuoteActivityUpdate {
	if event == nil {
		return nil
	}

	switch event.Type {
	case streaming.EventTypeStatus, streaming.EventTypeStatusUpdate, streaming.EventTypeStatusDelete:
		return ec.convertStatusToQuoteActivity(event)
	default:
		return nil
	}
}

// convertStatusToObject converts a status event to a GraphQL Object
func (ec *EventConverter) convertStatusToObject(event *streaming.InternalEvent) *model.Object {
	payload, ok := event.Data.(*streaming.StatusEventPayload)
	if !ok {
		ec.logger.Warn("invalid status event payload",
			zap.String("event_id", event.ID))
		return nil
	}

	// Convert status to Object model
	obj := &model.Object{
		ID:        payload.StatusID,
		Type:      model.ObjectTypeNote,
		Content:   payload.Content,
		CreatedAt: model.Time(payload.CreatedAt),
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   payload.AuthorID,
				Type: "Person",
			},
			PreferredUsername: payload.AuthorUsername,
		},
	}

	// Add reply context if present
	if payload.InReplyToID != "" {
		inReplyTo := &model.Object{ID: payload.InReplyToID}
		obj.InReplyTo = inReplyTo
	}

	return obj
}

// convertCostEvent converts a cost event to a GraphQL CostUpdate
func (ec *EventConverter) convertCostEvent(event *streaming.InternalEvent) *model.CostUpdate {
	payload, ok := event.Data.(*streaming.CostEventPayload)
	if !ok {
		ec.logger.Warn("invalid cost event payload",
			zap.String("event_id", event.ID))
		return nil
	}

	// Convert cost units to operation cost in micros
	operationCost := int(payload.CostUSD * 1000000) // Convert dollars to micros

	return &model.CostUpdate{
		OperationCost:     operationCost,
		DailyTotal:        payload.CostUSD,
		MonthlyProjection: payload.CostUSD * 30, // Simple projection
	}
}

// convertModerationEvent converts a moderation event to a ModerationDecision
func (ec *EventConverter) convertModerationEvent(event *streaming.InternalEvent) *moderation.ModerationDecision {
	payload, ok := event.Data.(*streaming.ModerationEventPayload)
	if !ok {
		ec.logger.Warn("invalid moderation event payload",
			zap.String("event_id", event.ID))
		return nil
	}

	return &moderation.ModerationDecision{
		ID:       payload.ItemID,
		ObjectID: payload.ItemID,
		Action:   moderation.ActionType(payload.Action),
		Reason:   payload.Reason,
		Decided:  payload.CreatedAt,
	}
}

// convertTrustEvent converts a trust event to a TrustEdge
func (ec *EventConverter) convertTrustEvent(event *streaming.InternalEvent) *trust.TrustEdge {
	payload, ok := event.Data.(*streaming.TrustEventPayload)
	if !ok {
		ec.logger.Warn("invalid trust event payload",
			zap.String("event_id", event.ID))
		return nil
	}

	return &trust.TrustEdge{
		From:       payload.UpdatedBy,
		To:         payload.SubjectID,
		Score:      payload.Score,
		Confidence: 0.8, // Default confidence
		Weight:     payload.Score * 0.8,
	}
}

// convertAIEvent converts an AI event to an AIAnalysis
func (ec *EventConverter) convertAIEvent(event *streaming.InternalEvent) *model.AIAnalysis {
	payload, ok := event.Data.(*streaming.AIEventPayload)
	if !ok {
		ec.logger.Warn("invalid AI event payload",
			zap.String("event_id", event.ID))
		return nil
	}

	return &model.AIAnalysis{
		ID:         payload.AnalysisID,
		ObjectID:   payload.ContentID,
		ObjectType: payload.ContentType,
		Confidence: payload.Confidence,
		AnalyzedAt: model.Time(payload.ProcessedAt),
	}
}

// convertHashtagEvent converts a hashtag event to a HashtagActivityUpdate
func (ec *EventConverter) convertHashtagEvent(event *streaming.InternalEvent) *model.HashtagActivityUpdate {
	payload, ok := event.Data.(*streaming.HashtagEventPayload)
	if !ok {
		ec.logger.Warn("invalid hashtag event payload",
			zap.String("event_id", event.ID))
		return nil
	}

	return &model.HashtagActivityUpdate{
		Hashtag:   payload.Hashtag,
		Timestamp: model.Time(payload.UpdatedAt),
	}
}

// convertStatusToHashtagActivity converts a status event to hashtag activity
func (ec *EventConverter) convertStatusToHashtagActivity(event *streaming.InternalEvent) *model.HashtagActivityUpdate {
	payload, ok := event.Data.(*streaming.StatusEventPayload)
	if !ok || len(payload.Hashtags) == 0 {
		return nil
	}

	// For now, just use the first hashtag
	// In a full implementation, we'd create separate events for each hashtag
	hashtag := payload.Hashtags[0]

	return &model.HashtagActivityUpdate{
		Hashtag: hashtag,
		Post: &model.Object{
			ID:        payload.StatusID,
			Type:      model.ObjectTypeNote,
			Content:   payload.Content,
			CreatedAt: model.Time(payload.CreatedAt),
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   payload.AuthorID,
					Type: "Person",
				},
				PreferredUsername: payload.AuthorUsername,
			},
		},
		Author: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   payload.AuthorID,
				Type: "Person",
			},
			PreferredUsername: payload.AuthorUsername,
		},
		Timestamp: model.Time(payload.CreatedAt),
	}
}

// convertStatusToQuoteActivity converts a status event to quote activity
func (ec *EventConverter) convertStatusToQuoteActivity(event *streaming.InternalEvent) *model.QuoteActivityUpdate {
	payload, ok := event.Data.(*streaming.StatusEventPayload)
	if !ok {
		return nil
	}

	// Determine the type of quote activity based on the event action
	var activityType string
	switch event.Action {
	case streaming.ActionCreate:
		activityType = "quote_created"
	case streaming.ActionUpdate:
		activityType = "quote_updated"
	case streaming.ActionDelete:
		activityType = "quote_removed"
	default:
		activityType = "quote_created"
	}

	return &model.QuoteActivityUpdate{
		Type: activityType,
		Quote: &model.Object{
			ID:        payload.StatusID,
			Type:      model.ObjectTypeNote,
			Content:   payload.Content,
			CreatedAt: model.Time(payload.CreatedAt),
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   payload.AuthorID,
					Type: "Person",
				},
				PreferredUsername: payload.AuthorUsername,
			},
		},
		Quoter: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   payload.AuthorID,
				Type: "Person",
			},
			PreferredUsername: payload.AuthorUsername,
		},
		Timestamp: model.Time(event.Timestamp),
	}
}

// extractUsernameFromID extracts username from an actor ID URL
func extractUsernameFromID(actorID string) string {
	// Simple extraction - in a full implementation this would be more robust
	if actorID == "" {
		return ""
	}
	
	// For now, just use the actor ID as username
	// In practice, this would parse URLs like "https://example.com/users/alice" -> "alice"
	return actorID
}