package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
)

func TestRound12EventConverter_BasicConversions(t *testing.T) {
	ec := NewEventConverter(nil)

	require.Nil(t, ec.ConvertToObject(nil))
	require.Nil(t, ec.ConvertToNotification(nil))
	require.Nil(t, ec.ConvertToCostUpdate(nil))
	require.Nil(t, ec.ConvertToModerationDecision(nil))
	require.Nil(t, ec.ConvertToTrustEdge(nil))
	require.Nil(t, ec.ConvertToAIAnalysis(nil))
	require.Nil(t, ec.ConvertToHashtagActivity(nil))
	require.Nil(t, ec.ConvertToQuoteActivity(nil))
	require.Nil(t, ec.ConvertToMetricsUpdate(nil))

	now := time.Now().UTC()

	// Status -> Object (with in-reply-to)
	obj := ec.ConvertToObject(&streaming.InternalEvent{
		ID:        "evt-status",
		Type:      streaming.EventTypeStatus,
		Action:    streaming.ActionCreate,
		Timestamp: now,
		Data: &streaming.StatusEventPayload{
			StatusID:       "status-1",
			AuthorID:       "https://localhost/users/alice",
			AuthorUsername: "alice",
			Content:        "hi",
			InReplyToID:    "status-0",
			CreatedAt:      now.Add(-time.Minute),
		},
	})
	require.NotNil(t, obj)
	require.NotNil(t, obj.InReplyTo)

	// Invalid payload for status -> nil
	require.Nil(t, ec.ConvertToObject(&streaming.InternalEvent{
		ID:   "evt-status-bad",
		Type: streaming.EventTypeStatus,
		Data: "nope",
	}))

	// Notification with and without StatusID.
	notification := ec.ConvertToNotification(&streaming.InternalEvent{
		ID:        "evt-notif",
		Type:      streaming.EventTypeNotification,
		Timestamp: now,
		Data: &streaming.NotificationEventPayload{
			NotificationID: "n1",
			Type:           "mention",
			ActorID:        "https://localhost/users/bob",
			StatusID:       "status-1",
			CreatedAt:      now,
		},
	})
	require.NotNil(t, notification)
	require.NotNil(t, notification.Status)

	communication := ec.ConvertToNotification(&streaming.InternalEvent{
		ID:        "evt-notif-comm",
		Type:      streaming.EventTypeNotification,
		Timestamp: now,
		Data: &streaming.NotificationEventPayload{
			NotificationID: "n1-comm",
			Type:           "communication:inbound",
			ActorID:        "https://localhost/users/bob",
			Read:           true,
			Data: map[string]interface{}{
				"channel":   "email",
				"messageId": "msg-1",
				"from": map[string]interface{}{
					"address": "bob@example.com",
				},
			},
			CreatedAt: now,
		},
	})
	require.NotNil(t, communication)
	require.True(t, communication.Read)
	require.NotNil(t, communication.Communication)
	require.Equal(t, "msg-1", communication.Communication.MessageID)

	noStatus := ec.ConvertToNotification(&streaming.InternalEvent{
		ID:        "evt-notif-2",
		Type:      streaming.EventTypeNotification,
		Timestamp: now,
		Data: &streaming.NotificationEventPayload{
			NotificationID: "n2",
			Type:           "follow",
			ActorID:        "https://localhost/users/bob",
			StatusID:       "",
			CreatedAt:      now,
		},
	})
	require.NotNil(t, noStatus)
	require.Nil(t, noStatus.Status)

	require.Nil(t, ec.ConvertToNotification(&streaming.InternalEvent{
		ID:   "evt-notif-bad",
		Type: streaming.EventTypeNotification,
		Data: "nope",
	}))

	// Cost updates
	cost := ec.ConvertToCostUpdate(&streaming.InternalEvent{
		ID:   "evt-cost",
		Type: streaming.EventTypeCostUpdate,
		Data: &streaming.CostEventPayload{CostUSD: 1.25},
	})
	require.NotNil(t, cost)
	require.Greater(t, cost.OperationCost, 0)

	// Moderation decision
	decision := ec.ConvertToModerationDecision(&streaming.InternalEvent{
		ID:   "evt-mod",
		Type: streaming.EventTypeModerationReview,
		Data: &streaming.ModerationEventPayload{
			ItemID:    "item-1",
			Action:    "approve",
			Reason:    "ok",
			CreatedAt: now,
		},
	})
	require.NotNil(t, decision)

	// Trust edge
	edge := ec.ConvertToTrustEdge(&streaming.InternalEvent{
		ID:   "evt-trust",
		Type: streaming.EventTypeTrustUpdate,
		Data: &streaming.TrustEventPayload{
			SubjectID: "bob",
			Score:     0.5,
			UpdatedBy: "alice",
		},
	})
	require.NotNil(t, edge)

	// AI analysis
	analysis := ec.ConvertToAIAnalysis(&streaming.InternalEvent{
		ID:   "evt-ai",
		Type: streaming.EventTypeAIAnalysis,
		Data: &streaming.AIEventPayload{
			AnalysisID:  "a1",
			ContentID:   "status-1",
			ContentType: "status",
			Confidence:  0.9,
			ProcessedAt: now,
		},
	})
	require.NotNil(t, analysis)

	// Hashtag activity: status events generate hashtag activity when hashtags exist.
	hashtagActivity := ec.ConvertToHashtagActivity(&streaming.InternalEvent{
		ID:   "evt-hash-status",
		Type: streaming.EventTypeStatus,
		Data: &streaming.StatusEventPayload{
			StatusID:       "status-2",
			AuthorID:       "https://localhost/users/alice",
			AuthorUsername: "alice",
			Content:        "hello #tag",
			Hashtags:       []string{"tag"},
			CreatedAt:      now,
		},
	})
	require.NotNil(t, hashtagActivity)
	require.NotNil(t, hashtagActivity.Post)

	require.Nil(t, ec.ConvertToHashtagActivity(&streaming.InternalEvent{
		ID:   "evt-hash-status-empty",
		Type: streaming.EventTypeStatus,
		Data: &streaming.StatusEventPayload{
			StatusID:  "status-3",
			Hashtags:  nil,
			CreatedAt: now,
		},
	}))

	// Quote activity types cover action switch.
	for _, action := range []streaming.EventAction{streaming.ActionCreate, streaming.ActionUpdate, streaming.ActionDelete, streaming.ActionRead} {
		update := ec.ConvertToQuoteActivity(&streaming.InternalEvent{
			ID:        "evt-quote-" + string(action),
			Type:      streaming.EventTypeStatus,
			Action:    action,
			Timestamp: now,
			Data: &streaming.StatusEventPayload{
				StatusID:       "status-q",
				AuthorID:       "https://localhost/users/alice",
				AuthorUsername: "alice",
				Content:        "quote",
				CreatedAt:      now,
			},
		})
		require.NotNil(t, update)
	}
}

func TestRound12EventConverter_MetricsConversions(t *testing.T) {
	ec := NewEventConverter(nil)
	now := time.Now().UTC()

	// MetricRecord path (with metadata parsing).
	update := ec.ConvertToMetricsUpdate(&streaming.InternalEvent{
		ID:        "evt-metrics-record",
		Type:      streaming.EventTypeMetricsUpdate,
		Timestamp: now,
		Data: &models.MetricRecord{
			MetricID:         "m1",
			ServiceName:      "svc",
			MetricType:       "latency",
			AggregationLevel: "raw",
			Timestamp:        now,
			Count:            2,
			Sum:              10,
			Min:              1,
			Max:              9,
			P50:              4,
			P95:              8,
			P99:              9,
			Unit:             "ms",
			Dimensions:       map[string]string{"route": "/"},
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		Metadata: map[string]string{
			"subscription_category": "performance",
			"user_cost_microcents":  "123",
			"total_cost_microcents": "456",
			"user_id":               "alice",
			"tenant_id":             "t1",
			"instance_domain":       "localhost",
		},
	})
	require.NotNil(t, update)
	require.NotNil(t, update.Unit)
	require.NotNil(t, update.Dimensions)
	require.NotNil(t, update.UserCostMicrocents)
	require.NotNil(t, update.TotalCostMicrocents)

	// Payload path.
	payloadUpdate := ec.ConvertToMetricsUpdate(&streaming.InternalEvent{
		ID:        "evt-metrics-payload",
		Type:      streaming.EventTypeMetricsUpdate,
		Timestamp: now,
		Data: &streaming.MetricsEventPayload{
			MetricID:             "m2",
			ServiceName:          "svc",
			MetricType:           "count",
			SubscriptionCategory: "moderation",
			AggregationLevel:     "hourly",
			Timestamp:            now,
			Count:                1,
			Sum:                  2,
			Average:              2,
			P50:                  2,
			P95:                  2,
			P99:                  2,
			Unit:                 "count",
			UserCostMicrocents:   10,
			TotalCostMicrocents:  20,
			UserID:               "alice",
			TenantID:             "t1",
			InstanceDomain:       "localhost",
			Dimensions:           map[string]string{"k": "v"},
		},
	})
	require.NotNil(t, payloadUpdate)
	require.NotNil(t, payloadUpdate.Unit)
	require.NotNil(t, payloadUpdate.UserCostMicrocents)
	require.NotNil(t, payloadUpdate.TotalCostMicrocents)

	// Metadata-only path.
	metadataOnly := ec.ConvertToMetricsUpdate(&streaming.InternalEvent{
		ID:        "evt-metrics-metadata",
		Type:      streaming.EventTypeMetricsUpdate,
		Timestamp: now,
		Data:      map[string]interface{}{},
		Metadata: map[string]string{
			"service_name":          "svc",
			"metric_type":           "mtype",
			"subscription_category": "security",
			"aggregation_level":     "daily",
			"count":                 "4",
			"sum":                   "10.5",
			"min":                   "1",
			"max":                   "9",
			"average":               "2.5",
			"p50":                   "2.0",
			"p95":                   "8.0",
			"p99":                   "9.0",
			"unit":                  "ms",
			"user_cost_microcents":  "11",
			"total_cost_microcents": "22",
			"user_id":               "alice",
			"tenant_id":             "t1",
			"instance_domain":       "localhost",
			"dim_route":             "/",
		},
	})
	require.NotNil(t, metadataOnly)
	require.NotNil(t, metadataOnly.Unit)
	require.NotNil(t, metadataOnly.Dimensions)

	// No metadata -> nil for metrics conversion.
	require.Nil(t, ec.ConvertToMetricsUpdate(&streaming.InternalEvent{
		ID:        "evt-metrics-no-metadata",
		Type:      streaming.EventTypeMetricsUpdate,
		Timestamp: now,
		Data:      map[string]interface{}{},
	}))
}

func TestRound12EventConverter_AlertAndMiscConversions(t *testing.T) {
	ec := NewEventConverter(nil)
	now := time.Now().UTC()

	// List update: default update type path.
	listUpdate := ec.ConvertToListUpdate(&streaming.InternalEvent{
		ID:        "evt-list",
		Type:      streaming.EventTypeTimelineUpdate,
		Timestamp: now,
		Data: map[string]interface{}{
			"list_id":    "l1",
			"list_title": "title",
		},
	})
	require.NotNil(t, listUpdate)
	require.Equal(t, "updated", listUpdate.Type)

	// Conversation update.
	conversation := ec.ConvertToConversation(&streaming.InternalEvent{
		ID:        "evt-conv",
		Type:      streaming.EventTypeTimelineUpdate,
		Timestamp: now,
		Data: map[string]interface{}{
			"conversation_id": "c1",
			"unread":          true,
			"last_status_id":  "status-1",
		},
	})
	require.NotNil(t, conversation)
	require.True(t, conversation.Unread)

	// Federation health update.
	fed := ec.ConvertToFederationHealthUpdate(&streaming.InternalEvent{
		ID:        "evt-fed",
		Type:      streaming.EventTypeFederationHealthUpdate,
		Timestamp: now,
		Data: map[string]interface{}{
			"domain": "example.com",
		},
	})
	require.NotNil(t, fed)
	require.Equal(t, "example.com", fed.Domain)

	// Relationship update: default type and bool extraction.
	relUpdate := ec.ConvertToRelationshipUpdate(&streaming.InternalEvent{
		ID:        "evt-rel",
		Type:      streaming.EventTypeAccountFollow,
		Timestamp: now,
		Data: map[string]interface{}{
			"actor_id":         "https://localhost/users/alice",
			"relationship_id":  "r1",
			"following":        true,
			"update_type":      "",
			"unrelated_number": 1,
		},
	})
	require.NotNil(t, relUpdate)
	require.Equal(t, "updated", relUpdate.Type)

	// Budget alert calculates percent used.
	budget := ec.ConvertToBudgetAlert(&streaming.InternalEvent{
		ID:        "evt-budget",
		Timestamp: now,
		Data: map[string]interface{}{
			"domain":      "example.com",
			"spent_usd":   float32(5),
			"budget_usd":  float64(10),
			"alert_level": "HIGH",
		},
	})
	require.NotNil(t, budget)
	require.Greater(t, budget.PercentUsed, 0.0)

	// Cost alert threshold gating.
	require.Nil(t, ec.ConvertToCostAlert(&streaming.InternalEvent{
		ID:        "evt-cost-low",
		Timestamp: now,
		Data: map[string]interface{}{
			"cost_usd": 0.5,
		},
	}, 1.0))
	costAlert := ec.ConvertToCostAlert(&streaming.InternalEvent{
		ID:        "evt-cost-high",
		Timestamp: now,
		Data: map[string]interface{}{
			"cost_usd":  2,
			"service":   "svc",
			"tenant_id": "example.com",
		},
	}, 1.0)
	require.NotNil(t, costAlert)
	require.NotNil(t, costAlert.Domain)

	// Performance alert.
	perf := ec.ConvertToPerformanceAlert(&streaming.InternalEvent{
		ID:        "evt-perf",
		Timestamp: now,
		Data: map[string]interface{}{
			"service":      "DATABASE",
			"metric":       "latency",
			"threshold":    int64(100),
			"actual_value": int(200),
			"severity":     "HIGH",
		},
	})
	require.NotNil(t, perf)

	// Moderation alert with severity filter.
	sev := model.ModerationSeverityHigh
	alert := ec.ConvertToModerationAlert(&streaming.InternalEvent{
		ID:        "evt-mod-alert",
		Timestamp: now,
		Data: map[string]interface{}{
			"severity":     "HIGH",
			"matched_text": "x",
			"confidence":   float64(0.9),
			"handled":      true,
		},
	}, &sev)
	require.NotNil(t, alert)
	require.Equal(t, model.ModerationSeverityHigh, alert.Severity)

	otherSev := model.ModerationSeverityLow
	require.Nil(t, ec.ConvertToModerationAlert(&streaming.InternalEvent{
		ID:        "evt-mod-alert-filter",
		Timestamp: now,
		Data: map[string]interface{}{
			"severity": "HIGH",
		},
	}, &otherSev))

	// Threat and infrastructure events.
	threat := ec.ConvertToThreatAlert(&streaming.InternalEvent{
		ID:        "evt-threat",
		Timestamp: now,
		Data: map[string]interface{}{
			"type":        "spam",
			"severity":    "CRITICAL",
			"source":      "ai",
			"description": "desc",
		},
	})
	require.NotNil(t, threat)

	infra := ec.ConvertToInfrastructureEvent(&streaming.InternalEvent{
		ID:        "evt-infra",
		Timestamp: now,
		Data: map[string]interface{}{
			"event_type":  "DEPLOY",
			"service":     "api",
			"description": "desc",
			"impact":      "low",
		},
	})
	require.NotNil(t, infra)
}

func TestRound12EventConverter_ParseAndExtractHelpers(t *testing.T) {
	_, err := parseIntFromString("nope")
	require.Error(t, err)

	value, err := parseIntFromString("123")
	require.NoError(t, err)
	require.Equal(t, 123, value)

	f, err := parseFloatFromString("1.5")
	require.NoError(t, err)
	require.Equal(t, 1.5, f)

	require.Equal(t, "", extractStringFromData("not-a-map", "k"))
	require.False(t, extractBoolFromData("not-a-map", "k"))
	require.Equal(t, "", extractStringFromData(map[string]interface{}{"k": 1}, "k"))
	require.False(t, extractBoolFromData(map[string]interface{}{"k": "true"}, "k"))
	require.Equal(t, "v", extractStringFromData(map[string]interface{}{"k": "v"}, "k"))
	require.True(t, extractBoolFromData(map[string]interface{}{"k": true}, "k"))

	data := map[string]interface{}{
		"f64": float64(1.5),
		"f32": float32(2.5),
		"i":   int(3),
		"i64": int64(4),
	}
	require.Equal(t, 1.5, extractFloatFromData(data, "f64"))
	require.Equal(t, 2.5, extractFloatFromData(data, "f32"))
	require.Equal(t, 3.0, extractFloatFromData(data, "i"))
	require.Equal(t, 4.0, extractFloatFromData(data, "i64"))
	require.Equal(t, 0.0, extractFloatFromData(data, "missing"))
}
