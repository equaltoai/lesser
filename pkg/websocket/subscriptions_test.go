package websocket

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConvertToMap(t *testing.T) {
	m, err := convertToMap(ModerationFilter{
		Severity: []string{"high"},
		Types:    []string{"spam"},
		UserID:   "u1",
	})
	require.NoError(t, err)

	require.Contains(t, m, "severity")
	require.Contains(t, m, "types")
	require.Equal(t, "u1", m["user_id"])
}

func TestSubscriptionManager_MatchesModerationFilter(t *testing.T) {
	sm := &subscriptionManager{}
	event := &ModerationEvent{
		Type:      "spam",
		Severity:  "high",
		ContentID: "c1",
		UserID:    "u1",
		Timestamp: time.Now(),
	}

	filter, err := convertToMap(ModerationFilter{
		Severity: []string{"high"},
		Types:    []string{"spam"},
		UserID:   "u1",
	})
	require.NoError(t, err)
	require.True(t, sm.matchesModerationFilter(event, filter))

	filter, err = convertToMap(ModerationFilter{
		Severity: []string{"low"},
		Types:    []string{"spam"},
	})
	require.NoError(t, err)
	require.False(t, sm.matchesModerationFilter(event, filter))
}

func TestSubscriptionManager_MatchesPerformanceFilter(t *testing.T) {
	sm := &subscriptionManager{}
	alert := &PerformanceAlert{
		Type:     "latency",
		Severity: "critical",
	}

	require.True(t, sm.matchesPerformanceFilter(alert, map[string]any{"severity": "critical"}))
	require.False(t, sm.matchesPerformanceFilter(alert, map[string]any{"severity": "warning"}))
}
