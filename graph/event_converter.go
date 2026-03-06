package graph

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage/models"
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
		ID:            payload.NotificationID,
		Type:          payload.Type,
		Communication: communicationNotificationFromData(payload.Type, payload.CreatedAt, payload.Data),
		Read:          payload.Read,
		CreatedAt:     model.Time(payload.CreatedAt),
		Account: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   payload.ActorID,
				Type: "Person",
			},
			PreferredUsername: extractUsernameFromID(payload.ActorID),
		},
		Status: func() *model.Object {
			if err := common.ValidateRequiredParam("status_id", payload.StatusID); err != nil {
				return nil
			}
			return &model.Object{
				ID:          payload.StatusID,
				ContentHash: contentHashForObjectID(payload.StatusID),
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
		ID:          payload.StatusID,
		Type:        model.ObjectTypeNote,
		Content:     payload.Content,
		CreatedAt:   model.Time(payload.CreatedAt),
		ContentHash: contentHashForObjectID(payload.StatusID),
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
		inReplyTo := &model.Object{
			ID:          payload.InReplyToID,
			ContentHash: contentHashForObjectID(payload.InReplyToID),
		}
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
			ID:          payload.StatusID,
			Type:        model.ObjectTypeNote,
			Content:     payload.Content,
			CreatedAt:   model.Time(payload.CreatedAt),
			ContentHash: contentHashForObjectID(payload.StatusID),
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
			ID:          payload.StatusID,
			Type:        model.ObjectTypeNote,
			Content:     payload.Content,
			CreatedAt:   model.Time(payload.CreatedAt),
			ContentHash: contentHashForObjectID(payload.StatusID),
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
	if err := common.ValidateRequiredParam("actor_id", actorID); err != nil {
		return ""
	}

	// For now, just use the actor ID as username
	// In practice, this would parse URLs like "https://example.com/users/alice" -> "alice"
	return actorID
}

// ConvertToMetricsUpdate converts a streaming event to a GraphQL MetricsUpdate
func (ec *EventConverter) ConvertToMetricsUpdate(event *streaming.InternalEvent) *model.MetricsUpdate {
	if event == nil || event.Type != streaming.EventTypeMetricsUpdate {
		return nil
	}

	// Try to extract MetricsRecord from event data
	if record, ok := event.Data.(*models.MetricRecord); ok {
		return ec.convertMetricRecordToUpdate(record, event)
	}

	// Try to extract MetricsEventPayload from event data
	if payload, ok := event.Data.(*streaming.MetricsEventPayload); ok {
		return ec.convertMetricsPayloadToUpdate(payload, event)
	}

	// Try to extract directly from event metadata
	return ec.convertEventMetadataToUpdate(event)
}

// convertMetricRecordToUpdate converts a MetricRecord to MetricsUpdate
func (ec *EventConverter) convertMetricRecordToUpdate(record *models.MetricRecord, event *streaming.InternalEvent) *model.MetricsUpdate {
	countValue, ok := safeIntFromInt64(record.Count)
	if !ok {
		ec.logger.Warn("metric record count exceeds supported range",
			zap.Int64("count", record.Count),
			zap.String("metric_id", record.MetricID))
		countValue = 0
	}

	update := &model.MetricsUpdate{
		MetricID:         record.MetricID,
		ServiceName:      record.ServiceName,
		MetricType:       record.MetricType,
		AggregationLevel: record.AggregationLevel,
		Timestamp:        model.Time(record.Timestamp),
		Count:            countValue,
		Sum:              record.Sum,
		Min:              record.Min,
		Max:              record.Max,
	}

	// Calculate average if not already set
	if record.Count > 0 {
		update.Average = record.Sum / float64(record.Count)
	}

	// Add percentiles if available
	if record.P50 > 0 {
		p50 := record.P50
		update.P50 = &p50
	}
	if record.P95 > 0 {
		p95 := record.P95
		update.P95 = &p95
	}
	if record.P99 > 0 {
		p99 := record.P99
		update.P99 = &p99
	}

	// Add unit if available
	if record.Unit != "" {
		update.Unit = &record.Unit
	}

	// Extract subscription category from event metadata
	if event.Metadata != nil {
		if category, exists := event.Metadata["subscription_category"]; exists {
			update.SubscriptionCategory = category
		}

		// Extract cost information
		if userCost, exists := event.Metadata["user_cost_microcents"]; exists {
			if cost, err := parseIntFromString(userCost); err == nil {
				update.UserCostMicrocents = &cost
			}
		}
		if totalCost, exists := event.Metadata["total_cost_microcents"]; exists {
			if cost, err := parseIntFromString(totalCost); err == nil {
				update.TotalCostMicrocents = &cost
			}
		}

		// Extract user/tenant information
		if userID, exists := event.Metadata["user_id"]; exists {
			update.UserID = &userID
		}
		if tenantID, exists := event.Metadata["tenant_id"]; exists {
			update.TenantID = &tenantID
		}
		if domain, exists := event.Metadata["instance_domain"]; exists {
			update.InstanceDomain = &domain
		}
	}

	// Convert dimensions to GraphQL format
	if record.Dimensions != nil {
		dimensions := make([]*model.MetricsDimension, 0, len(record.Dimensions))
		for key, value := range record.Dimensions {
			dimensions = append(dimensions, &model.MetricsDimension{
				Key:   key,
				Value: value,
			})
		}
		update.Dimensions = dimensions
	}

	return update
}

// convertMetricsPayloadToUpdate converts a MetricsEventPayload to MetricsUpdate
func (ec *EventConverter) convertMetricsPayloadToUpdate(payload *streaming.MetricsEventPayload, _ *streaming.InternalEvent) *model.MetricsUpdate {
	countValue, ok := safeIntFromInt64(payload.Count)
	if !ok {
		ec.logger.Warn("metrics count exceeds supported range",
			zap.Int64("count", payload.Count))
		countValue = 0
	}

	update := &model.MetricsUpdate{
		MetricID:             payload.MetricID,
		ServiceName:          payload.ServiceName,
		MetricType:           payload.MetricType,
		SubscriptionCategory: payload.SubscriptionCategory,
		AggregationLevel:     payload.AggregationLevel,
		Timestamp:            model.Time(payload.Timestamp),
		Count:                countValue,
		Sum:                  payload.Sum,
		Min:                  payload.Min,
		Max:                  payload.Max,
		Average:              payload.Average,
	}

	// Add optional fields if available
	if payload.P50 > 0 {
		update.P50 = &payload.P50
	}
	if payload.P95 > 0 {
		update.P95 = &payload.P95
	}
	if payload.P99 > 0 {
		update.P99 = &payload.P99
	}
	if payload.Unit != "" {
		update.Unit = &payload.Unit
	}
	if payload.UserCostMicrocents > 0 {
		if userCost, ok := safeIntFromInt64(payload.UserCostMicrocents); ok {
			update.UserCostMicrocents = &userCost
		} else {
			ec.logger.Warn("user cost exceeds supported range",
				zap.Int64("user_cost_microcents", payload.UserCostMicrocents))
		}
	}
	if payload.TotalCostMicrocents > 0 {
		if totalCost, ok := safeIntFromInt64(payload.TotalCostMicrocents); ok {
			update.TotalCostMicrocents = &totalCost
		} else {
			ec.logger.Warn("total cost exceeds supported range",
				zap.Int64("total_cost_microcents", payload.TotalCostMicrocents))
		}
	}
	if payload.UserID != "" {
		update.UserID = &payload.UserID
	}
	if payload.TenantID != "" {
		update.TenantID = &payload.TenantID
	}
	if payload.InstanceDomain != "" {
		update.InstanceDomain = &payload.InstanceDomain
	}

	// Convert dimensions to GraphQL format
	if payload.Dimensions != nil {
		dimensions := make([]*model.MetricsDimension, 0, len(payload.Dimensions))
		for key, value := range payload.Dimensions {
			dimensions = append(dimensions, &model.MetricsDimension{
				Key:   key,
				Value: value,
			})
		}
		update.Dimensions = dimensions
	}

	return update
}

// convertEventMetadataToUpdate creates a MetricsUpdate from event metadata
func (ec *EventConverter) convertEventMetadataToUpdate(event *streaming.InternalEvent) *model.MetricsUpdate {
	if event.Metadata == nil {
		ec.logger.Debug("cannot convert metrics event without metadata",
			zap.String("event_id", event.ID))
		return nil
	}

	update := &model.MetricsUpdate{
		MetricID:  event.ID,
		Timestamp: model.Time(event.Timestamp),
	}

	ec.extractBasicFields(event.Metadata, update)
	ec.extractNumericFields(event.Metadata, update)
	ec.extractPercentileFields(event.Metadata, update)
	ec.extractOptionalFields(event.Metadata, update)
	ec.extractDimensions(event.Metadata, update)

	return update
}

// extractBasicFields extracts basic string fields from metadata
func (ec *EventConverter) extractBasicFields(metadata map[string]string, update *model.MetricsUpdate) {
	if serviceName, exists := metadata["service_name"]; exists {
		update.ServiceName = serviceName
	}
	if metricType, exists := metadata["metric_type"]; exists {
		update.MetricType = metricType
	}
	if category, exists := metadata["subscription_category"]; exists {
		update.SubscriptionCategory = category
	}
	if level, exists := metadata["aggregation_level"]; exists {
		update.AggregationLevel = level
	}
}

// extractNumericFields extracts numeric fields from metadata
func (ec *EventConverter) extractNumericFields(metadata map[string]string, update *model.MetricsUpdate) {
	if count, exists := metadata["count"]; exists {
		if c, err := parseIntFromString(count); err == nil {
			update.Count = c
		}
	}
	if sum, exists := metadata["sum"]; exists {
		if s, err := parseFloatFromString(sum); err == nil {
			update.Sum = s
		}
	}
	if minVal, exists := metadata["min"]; exists {
		if m, err := parseFloatFromString(minVal); err == nil {
			update.Min = m
		}
	}
	if maxVal, exists := metadata["max"]; exists {
		if m, err := parseFloatFromString(maxVal); err == nil {
			update.Max = m
		}
	}
	if avg, exists := metadata["average"]; exists {
		if a, err := parseFloatFromString(avg); err == nil {
			update.Average = a
		}
	}
}

// extractPercentileFields extracts percentile fields from metadata
func (ec *EventConverter) extractPercentileFields(metadata map[string]string, update *model.MetricsUpdate) {
	if p50, exists := metadata["p50"]; exists {
		if p, err := parseFloatFromString(p50); err == nil {
			update.P50 = &p
		}
	}
	if p95, exists := metadata["p95"]; exists {
		if p, err := parseFloatFromString(p95); err == nil {
			update.P95 = &p
		}
	}
	if p99, exists := metadata["p99"]; exists {
		if p, err := parseFloatFromString(p99); err == nil {
			update.P99 = &p
		}
	}
}

// extractOptionalFields extracts optional fields from metadata
func (ec *EventConverter) extractOptionalFields(metadata map[string]string, update *model.MetricsUpdate) {
	if unit, exists := metadata["unit"]; exists {
		update.Unit = &unit
	}
	ec.extractCostField(metadata, "user_cost_microcents", &update.UserCostMicrocents)
	ec.extractCostField(metadata, "total_cost_microcents", &update.TotalCostMicrocents)
	if userID, exists := metadata["user_id"]; exists {
		update.UserID = &userID
	}
	if tenantID, exists := metadata["tenant_id"]; exists {
		update.TenantID = &tenantID
	}
	if domain, exists := metadata["instance_domain"]; exists {
		update.InstanceDomain = &domain
	}
}

// extractCostField extracts and converts cost fields from metadata
func (ec *EventConverter) extractCostField(metadata map[string]string, key string, target **int) {
	if costStr, exists := metadata[key]; exists {
		if cost, err := parseIntFromString(costStr); err == nil {
			*target = &cost
		}
	}
}

// extractDimensions extracts dimension fields from metadata
func (ec *EventConverter) extractDimensions(metadata map[string]string, update *model.MetricsUpdate) {
	dimensions := make([]*model.MetricsDimension, 0)
	for key, value := range metadata {
		if strings.HasPrefix(key, "dim_") {
			dimensionKey := strings.TrimPrefix(key, "dim_")
			dimensions = append(dimensions, &model.MetricsDimension{
				Key:   dimensionKey,
				Value: value,
			})
		}
	}
	update.Dimensions = dimensions
}

// Helper functions for parsing metadata strings
func parseIntFromString(s string) (int, error) {
	parsed, err := strconv.ParseInt(s, 10, strconv.IntSize)
	if err != nil {
		return 0, err
	}
	return int(parsed), nil
}

func parseFloatFromString(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func safeIntFromInt64(value int64) (int, bool) {
	if value > math.MaxInt32 || value < math.MinInt32 {
		return 0, false
	}
	return int(value), true
}

// ConvertToListUpdate converts a streaming event to a GraphQL ListUpdate
func (ec *EventConverter) ConvertToListUpdate(event *streaming.InternalEvent) *model.ListUpdate {
	if event == nil {
		return nil
	}

	// For now, return a basic list update
	// This would be enhanced based on the actual event payload structure
	const defaultUpdateType = "updated"
	updateType := extractStringFromData(event.Data, "update_type")
	if updateType == "" {
		updateType = defaultUpdateType
	}

	return &model.ListUpdate{
		List: &model.List{
			ID:    extractStringFromData(event.Data, "list_id"),
			Title: extractStringFromData(event.Data, "list_title"),
		},
		Type:      updateType,
		Timestamp: model.Time(event.Timestamp),
	}
}

// ConvertToConversation converts a streaming event to a GraphQL Conversation
func (ec *EventConverter) ConvertToConversation(event *streaming.InternalEvent) *model.Conversation {
	if event == nil {
		return nil
	}

	// For now, return a basic conversation
	// This would be enhanced based on the actual event payload structure
	lastStatusID := extractStringFromData(event.Data, "last_status_id")
	return &model.Conversation{
		ID:     extractStringFromData(event.Data, "conversation_id"),
		Unread: extractBoolFromData(event.Data, "unread"),
		LastStatus: &model.Object{
			ID:          lastStatusID,
			ContentHash: contentHashForObjectID(lastStatusID),
		},
		Accounts:  []*activitypub.Actor{},
		CreatedAt: model.Time(event.Timestamp),
	}
}

// ConvertToFederationHealthUpdate converts a streaming event to a GraphQL FederationHealthUpdate
func (ec *EventConverter) ConvertToFederationHealthUpdate(event *streaming.InternalEvent) *model.FederationHealthUpdate {
	if event == nil {
		return nil
	}

	// For now, return a basic health update
	// This would be enhanced based on the actual event payload structure
	domain := extractStringFromData(event.Data, "domain")
	return &model.FederationHealthUpdate{
		Domain:    domain,
		Timestamp: model.Time(event.Timestamp),
	}
}

// ConvertToRelationshipUpdate converts a streaming event to a GraphQL RelationshipUpdate
func (ec *EventConverter) ConvertToRelationshipUpdate(event *streaming.InternalEvent) *model.RelationshipUpdate {
	if event == nil {
		return nil
	}

	// For now, return a basic relationship update
	// This would be enhanced based on the actual event payload structure
	const defaultUpdateType = "updated"
	actorID := extractStringFromData(event.Data, "actor_id")
	updateType := extractStringFromData(event.Data, "update_type")
	if updateType == "" {
		updateType = defaultUpdateType
	}

	return &model.RelationshipUpdate{
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   actorID,
				Type: "Person",
			},
			PreferredUsername: extractUsernameFromID(actorID),
		},
		Relationship: &model.Relationship{
			ID:        extractStringFromData(event.Data, "relationship_id"),
			Following: extractBoolFromData(event.Data, "following"),
		},
		Type:      updateType,
		Timestamp: model.Time(event.Timestamp),
	}
}

// Helper functions for extracting data from event payloads

// extractStringFromData extracts a string value from event data
func extractStringFromData(data interface{}, key string) string {
	if dataMap, ok := data.(map[string]interface{}); ok {
		if val, exists := dataMap[key]; exists {
			if strVal, ok := val.(string); ok {
				return strVal
			}
		}
	}
	return ""
}

// extractBoolFromData extracts a boolean value from event data
func extractBoolFromData(data interface{}, key string) bool {
	if dataMap, ok := data.(map[string]interface{}); ok {
		if val, exists := dataMap[key]; exists {
			if boolVal, ok := val.(bool); ok {
				return boolVal
			}
		}
	}
	return false
}

// ConvertToBudgetAlert converts a streaming event to a GraphQL BudgetAlert
func (ec *EventConverter) ConvertToBudgetAlert(event *streaming.InternalEvent) *model.BudgetAlert {
	if event == nil {
		return nil
	}

	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract budget alert data from event payload
	alert := &model.BudgetAlert{
		ID:         event.ID,
		Domain:     extractStringFromData(data, "domain"),
		SpentUsd:   extractFloatFromData(data, "spent_usd"),
		BudgetUsd:  extractFloatFromData(data, "budget_usd"),
		AlertLevel: model.AlertLevel(extractStringFromData(data, "alert_level")),
	}

	// Calculate percentage if not provided
	if alert.BudgetUsd > 0 {
		alert.PercentUsed = (alert.SpentUsd / alert.BudgetUsd) * 100
	}

	// Set timestamp
	alert.Timestamp = model.Time(event.Timestamp)

	return alert
}

// ConvertToCostAlert converts a streaming event to a GraphQL CostAlert
func (ec *EventConverter) ConvertToCostAlert(event *streaming.InternalEvent, thresholdUSD float64) *model.CostAlert {
	if event == nil {
		return nil
	}

	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract cost from event payload
	costUSD := extractFloatFromData(data, "cost_usd")

	// Only create alert if cost exceeds threshold
	if costUSD <= thresholdUSD {
		return nil
	}

	// Extract service and tenant info
	service := extractStringFromData(data, "service")
	tenantID := extractStringFromData(data, "tenant_id")

	alert := &model.CostAlert{
		ID:        event.ID,
		Type:      "service_threshold",
		Amount:    costUSD,
		Threshold: thresholdUSD,
		Message: fmt.Sprintf("Cost alert for %s: $%.2f exceeded threshold $%.2f",
			service, costUSD, thresholdUSD),
		Timestamp: model.Time(event.Timestamp),
	}

	if tenantID != "" {
		domain := tenantID
		alert.Domain = &domain
	}

	return alert
}

// Helper to extract float from event data
func extractFloatFromData(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0.0
}

// ConvertToPerformanceAlert converts a streaming event to a GraphQL PerformanceAlert
func (ec *EventConverter) ConvertToPerformanceAlert(event *streaming.InternalEvent) *model.PerformanceAlert {
	if event == nil {
		return nil
	}

	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract performance alert data from event payload
	alert := &model.PerformanceAlert{
		ID:          event.ID,
		Service:     model.ServiceCategory(extractStringFromData(data, "service")),
		Metric:      extractStringFromData(data, "metric"),
		Threshold:   extractFloatFromData(data, "threshold"),
		ActualValue: extractFloatFromData(data, "actual_value"),
		Severity:    model.AlertSeverity(extractStringFromData(data, "severity")),
		Timestamp:   model.Time(event.Timestamp),
	}

	return alert
}

// ConvertToModerationAlert converts a streaming event to a GraphQL ModerationAlert
func (ec *EventConverter) ConvertToModerationAlert(event *streaming.InternalEvent, severity *model.ModerationSeverity) *model.ModerationAlert {
	if event == nil {
		return nil
	}

	// Create moderation alert
	alert := &model.ModerationAlert{
		ID:        fmt.Sprintf("alert_%d", event.Timestamp.UnixNano()),
		Timestamp: model.Time(event.Timestamp),
	}

	// Extract alert data from event
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}

	if alertSeverity, ok := data["severity"].(string); ok {
		// Convert string to ModerationSeverity
		switch strings.ToUpper(alertSeverity) {
		case "LOW":
			alert.Severity = model.ModerationSeverityLow
		case "MEDIUM":
			alert.Severity = model.ModerationSeverityMedium
		case "HIGH":
			alert.Severity = model.ModerationSeverityHigh
		case "CRITICAL":
			alert.Severity = model.ModerationSeverityCritical
		default:
			alert.Severity = model.ModerationSeverityInfo
		}
	}

	// Filter by severity if specified
	if severity != nil && alert.Severity != *severity {
		return nil
	}

	if matchedText, ok := data["matched_text"].(string); ok {
		alert.MatchedText = matchedText
	}
	if confidence, ok := data["confidence"].(float64); ok {
		alert.Confidence = confidence
	}
	if handled, ok := data["handled"].(bool); ok {
		alert.Handled = handled
	}

	return alert
}

// ConvertToThreatAlert converts a streaming event to a GraphQL ThreatAlert
func (ec *EventConverter) ConvertToThreatAlert(event *streaming.InternalEvent) *model.ThreatAlert {
	if event == nil {
		return nil
	}

	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract threat alert data from event payload
	alert := &model.ThreatAlert{
		ID:                event.ID,
		Type:              extractStringFromData(data, "type"),
		Severity:          model.ModerationSeverity(extractStringFromData(data, "severity")),
		Source:            extractStringFromData(data, "source"),
		Description:       extractStringFromData(data, "description"),
		AffectedInstances: []string{},
		MitigationSteps:   []string{},
		Timestamp:         model.Time(event.Timestamp),
	}

	return alert
}

// ConvertToInfrastructureEvent converts a streaming event to a GraphQL InfrastructureEvent
func (ec *EventConverter) ConvertToInfrastructureEvent(event *streaming.InternalEvent) *model.InfrastructureEvent {
	if event == nil {
		return nil
	}

	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract infrastructure event data from event payload
	infraEvent := &model.InfrastructureEvent{
		ID:          event.ID,
		Type:        model.InfrastructureEventType(extractStringFromData(data, "event_type")),
		Service:     extractStringFromData(data, "service"),
		Description: extractStringFromData(data, "description"),
		Impact:      extractStringFromData(data, "impact"),
		Timestamp:   model.Time(event.Timestamp),
	}

	return infraEvent
}
