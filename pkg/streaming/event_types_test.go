package streaming

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventFilter_Matches_EmptyFilterMatchesAll(t *testing.T) {
	filter := &EventFilter{}
	event := &InternalEvent{
		Type:      EventTypeStatus,
		Action:    ActionCreate,
		ActorID:   "actor1",
		UserID:    "user1",
		TenantID:  "tenant1",
		Streams:   []string{"public"},
		Metadata:  map[string]string{"k": "v"},
		Priority:  PriorityNormal,
		Timestamp: time.Now(),
	}

	assert.True(t, filter.Matches(event))
}

func TestEventFilter_Matches_AllCriteria(t *testing.T) {
	event := &InternalEvent{
		Type:     EventTypeStatusUpdate,
		Action:   ActionUpdate,
		ActorID:  "actor1",
		UserID:   "user1",
		TenantID: "tenant1",
		Streams:  []string{"public", "user"},
		Metadata: map[string]string{"source": "api"},
		Priority: PriorityHigh,
	}

	filter := &EventFilter{
		Types:       []EventType{EventTypeStatusUpdate},
		Actions:     []EventAction{ActionUpdate},
		ActorID:     "actor1",
		UserID:      "user1",
		TenantID:    "tenant1",
		Streams:     []string{"public"},
		Metadata:    map[string]string{"source": "api"},
		MinPriority: PriorityNormal,
	}

	assert.True(t, filter.Matches(event))
}

func TestEventFilter_Matches_Failures(t *testing.T) {
	baseEvent := &InternalEvent{
		Type:     EventTypeStatusUpdate,
		Action:   ActionUpdate,
		ActorID:  "actor1",
		UserID:   "user1",
		TenantID: "tenant1",
		Streams:  []string{"public"},
		Metadata: map[string]string{"source": "api"},
		Priority: PriorityNormal,
	}

	assert.False(t, (&EventFilter{Types: []EventType{EventTypeStatus}}).Matches(baseEvent))
	assert.False(t, (&EventFilter{Actions: []EventAction{ActionDelete}}).Matches(baseEvent))
	assert.False(t, (&EventFilter{ActorID: "other"}).Matches(baseEvent))
	assert.False(t, (&EventFilter{UserID: "other"}).Matches(baseEvent))
	assert.False(t, (&EventFilter{TenantID: "other"}).Matches(baseEvent))
	assert.False(t, (&EventFilter{Streams: []string{"other"}}).Matches(baseEvent))
	assert.False(t, (&EventFilter{Metadata: map[string]string{"source": "other"}}).Matches(baseEvent))
	assert.False(t, (&EventFilter{MinPriority: PriorityHigh}).Matches(baseEvent))
}

func TestInternalEvent_BuilderAndJSON(t *testing.T) {
	ev := CreateEvent(EventTypeStatus, ActionCreate, map[string]interface{}{"a": 1}).
		WithActor("actor1").
		WithTarget("t1").
		WithUser("u1").
		WithTenant("tenant1").
		WithStreams("public", "user").
		WithPriority(PriorityUrgent).
		WithMetadata("k", "v")

	require.NotNil(t, ev.Metadata)
	assert.Equal(t, "v", ev.Metadata["k"])
	assert.Equal(t, PriorityUrgent, ev.Priority)

	raw, err := ev.ToJSON()
	require.NoError(t, err)

	parsed, err := FromJSON(raw)
	require.NoError(t, err)
	assert.Equal(t, ev.Type, parsed.Type)
	assert.Equal(t, ev.Action, parsed.Action)
	assert.Equal(t, ev.ActorID, parsed.ActorID)
	assert.Equal(t, ev.TargetID, parsed.TargetID)
	assert.Equal(t, ev.UserID, parsed.UserID)
	assert.Equal(t, ev.TenantID, parsed.TenantID)
	assert.Equal(t, ev.Priority, parsed.Priority)
}

func TestRandomStringAndEventID(t *testing.T) {
	id := generateEventID()
	assert.Contains(t, id, "evt_")

	s := randomString(16)
	require.Len(t, s, 16)

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for _, r := range s {
		assert.Contains(t, charset, string(r))
	}
}
