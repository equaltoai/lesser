package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEventConverter_ConvertToModerationAlert(t *testing.T) {
	logger := zap.NewNop()
	converter := NewEventConverter(logger)

	t.Run("converts moderation alert with severity", func(t *testing.T) {
		event := &streaming.InternalEvent{
			ID:        "alert_123",
			Type:      streaming.EventTypeModerationFlag,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"severity":     "HIGH",
				"matched_text": "inappropriate content",
				"confidence":   0.95,
				"handled":      false,
			},
		}

		alert := converter.ConvertToModerationAlert(event, nil)

		require.NotNil(t, alert)
		assert.NotEmpty(t, alert.ID)
		assert.Equal(t, model.ModerationSeverityHigh, alert.Severity)
		assert.Equal(t, "inappropriate content", alert.MatchedText)
		assert.Equal(t, 0.95, alert.Confidence)
		assert.False(t, alert.Handled)
	})

	t.Run("filters by severity when specified", func(t *testing.T) {
		event := &streaming.InternalEvent{
			ID:        "alert_456",
			Type:      streaming.EventTypeModerationFlag,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"severity": "LOW",
			},
		}

		// Request HIGH severity alerts only
		highSeverity := model.ModerationSeverityHigh
		alert := converter.ConvertToModerationAlert(event, &highSeverity)

		// Should be filtered out because event is LOW but we requested HIGH
		assert.Nil(t, alert)
	})

	t.Run("converts different severity levels", func(t *testing.T) {
		testCases := []struct {
			severityStr      string
			expectedSeverity model.ModerationSeverity
		}{
			{"LOW", model.ModerationSeverityLow},
			{"MEDIUM", model.ModerationSeverityMedium},
			{"HIGH", model.ModerationSeverityHigh},
			{"CRITICAL", model.ModerationSeverityCritical},
			{"UNKNOWN", model.ModerationSeverityInfo}, // default
		}

		for _, tc := range testCases {
			t.Run(tc.severityStr, func(t *testing.T) {
				event := &streaming.InternalEvent{
					ID:        "alert_test",
					Type:      streaming.EventTypeModerationFlag,
					Timestamp: time.Now(),
					Data: map[string]interface{}{
						"severity": tc.severityStr,
					},
				}

				alert := converter.ConvertToModerationAlert(event, nil)
				require.NotNil(t, alert)
				assert.Equal(t, tc.expectedSeverity, alert.Severity)
			})
		}
	})

	t.Run("handles nil event", func(t *testing.T) {
		alert := converter.ConvertToModerationAlert(nil, nil)
		assert.Nil(t, alert)
	})

	t.Run("handles invalid data format", func(t *testing.T) {
		event := &streaming.InternalEvent{
			ID:        "alert_invalid",
			Type:      streaming.EventTypeModerationFlag,
			Timestamp: time.Now(),
			Data:      "invalid data",
		}

		alert := converter.ConvertToModerationAlert(event, nil)
		assert.Nil(t, alert)
	})
}

func TestEventConverter_ConvertToCostAlert(t *testing.T) {
	logger := zap.NewNop()
	converter := NewEventConverter(logger)

	t.Run("converts cost alert with threshold", func(t *testing.T) {
		event := &streaming.InternalEvent{
			ID:        "cost_123",
			Type:      streaming.EventTypeCostAlert,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"service":     "federation",
				"tenant_id":   "tenant123",
				"cost_usd":    150.50,
				"instance_id": "inst_456",
			},
		}

		threshold := 100.0
		alert := converter.ConvertToCostAlert(event, threshold)

		require.NotNil(t, alert)
		assert.Equal(t, "cost_123", alert.ID)
		assert.Equal(t, "service_threshold", alert.Type) // hardcoded in implementation
		assert.Equal(t, 150.50, alert.Amount)
		assert.Equal(t, 100.0, alert.Threshold)
		assert.NotNil(t, alert.Domain)
		assert.Equal(t, "tenant123", *alert.Domain)
	})

	t.Run("handles nil event", func(t *testing.T) {
		alert := converter.ConvertToCostAlert(nil, 100.0)
		assert.Nil(t, alert)
	})
}

func TestEventConverter_ConvertToBudgetAlert(t *testing.T) {
	logger := zap.NewNop()
	converter := NewEventConverter(logger)

	t.Run("converts budget alert", func(t *testing.T) {
		event := &streaming.InternalEvent{
			ID:        "budget_123",
			Type:      streaming.EventTypeCostAlert,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"domain":      "example.com",
				"budget_usd":  1000.0,
				"spent_usd":   850.0,
				"alert_level": "WARNING",
			},
		}

		alert := converter.ConvertToBudgetAlert(event)

		require.NotNil(t, alert)
		assert.Equal(t, "budget_123", alert.ID)
		assert.Equal(t, "example.com", alert.Domain)
		assert.Equal(t, 1000.0, alert.BudgetUsd)
		assert.Equal(t, 850.0, alert.SpentUsd)
		// PercentUsed is calculated automatically
		assert.Equal(t, 85.0, alert.PercentUsed)
		assert.Equal(t, model.AlertLevelWarning, alert.AlertLevel)
		// ProjectedOverspend is not extracted in current implementation
	})

	t.Run("handles nil event", func(t *testing.T) {
		alert := converter.ConvertToBudgetAlert(nil)
		assert.Nil(t, alert)
	})
}

func TestEventConverter_ConvertToFederationHealthUpdate(t *testing.T) {
	logger := zap.NewNop()
	converter := NewEventConverter(logger)

	t.Run("converts federation health update", func(t *testing.T) {
		event := &streaming.InternalEvent{
			ID:        "health_123",
			Type:      streaming.EventTypeFederationHealthUpdate,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"domain": "example.com",
			},
		}

		update := converter.ConvertToFederationHealthUpdate(event)

		require.NotNil(t, update)
		assert.Equal(t, "example.com", update.Domain)
		// Note: Current implementation is basic and only extracts domain
		// Status and issues extraction would be added when needed
	})

	t.Run("handles nil event", func(t *testing.T) {
		update := converter.ConvertToFederationHealthUpdate(nil)
		assert.Nil(t, update)
	})
}
