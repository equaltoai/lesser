package streaming

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestEventBus_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := NewEventBus(DefaultEventBusConfig(), logger)

	// Test starting the event bus
	ctx := context.Background()
	err := eventBus.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start event bus: %v", err)
	}

	if !eventBus.IsRunning() {
		t.Error("event bus should be running after start")
	}

	// Test stopping the event bus
	err = eventBus.Stop()
	if err != nil {
		t.Fatalf("failed to stop event bus: %v", err)
	}

	if eventBus.IsRunning() {
		t.Error("event bus should not be running after stop")
	}
}

func TestEventBus_PublishSubscribe(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := NewEventBus(DefaultEventBusConfig(), logger)

	ctx := context.Background()
	err := eventBus.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start event bus: %v", err)
	}
	defer eventBus.Stop()

	// Create a subscriber
	filter := &EventFilter{
		Types: []EventType{EventTypeStatus},
	}
	subscriber, err := eventBus.Subscribe("test-subscriber", filter, 10)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Create a test event
	event := CreateEvent(EventTypeStatus, ActionCreate, map[string]string{
		"test": "data",
	})

	// Publish the event
	err = eventBus.Publish(event)
	if err != nil {
		t.Fatalf("failed to publish event: %v", err)
	}

	// Wait for the event to be delivered
	select {
	case receivedEvent := <-subscriber.Channel:
		if receivedEvent.ID != event.ID {
			t.Errorf("expected event ID %s, got %s", event.ID, receivedEvent.ID)
		}
		if receivedEvent.Type != EventTypeStatus {
			t.Errorf("expected event type %s, got %s", EventTypeStatus, receivedEvent.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestEventBus_EventFiltering(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := NewEventBus(DefaultEventBusConfig(), logger)

	ctx := context.Background()
	err := eventBus.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start event bus: %v", err)
	}
	defer eventBus.Stop()

	// Create subscribers with different filters
	statusFilter := &EventFilter{
		Types: []EventType{EventTypeStatus},
	}
	statusSubscriber, err := eventBus.Subscribe("status-subscriber", statusFilter, 10)
	if err != nil {
		t.Fatalf("failed to subscribe to status events: %v", err)
	}

	notificationFilter := &EventFilter{
		Types: []EventType{EventTypeNotification},
	}
	notificationSubscriber, err := eventBus.Subscribe("notification-subscriber", notificationFilter, 10)
	if err != nil {
		t.Fatalf("failed to subscribe to notification events: %v", err)
	}

	// Publish a status event
	statusEvent := CreateEvent(EventTypeStatus, ActionCreate, map[string]string{
		"content": "test status",
	})
	err = eventBus.Publish(statusEvent)
	if err != nil {
		t.Fatalf("failed to publish status event: %v", err)
	}

	// Publish a notification event
	notificationEvent := CreateEvent(EventTypeNotification, ActionCreate, map[string]string{
		"type": "mention",
	})
	err = eventBus.Publish(notificationEvent)
	if err != nil {
		t.Fatalf("failed to publish notification event: %v", err)
	}

	// Status subscriber should only receive status event
	select {
	case receivedEvent := <-statusSubscriber.Channel:
		if receivedEvent.Type != EventTypeStatus {
			t.Errorf("status subscriber received wrong event type: %s", receivedEvent.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("status subscriber timeout waiting for event")
	}

	// Check that status subscriber doesn't receive notification event
	select {
	case <-statusSubscriber.Channel:
		t.Error("status subscriber should not receive notification events")
	case <-time.After(100 * time.Millisecond):
		// Expected - no event should be received
	}

	// Notification subscriber should only receive notification event
	select {
	case receivedEvent := <-notificationSubscriber.Channel:
		if receivedEvent.Type != EventTypeNotification {
			t.Errorf("notification subscriber received wrong event type: %s", receivedEvent.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("notification subscriber timeout waiting for event")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := NewEventBus(DefaultEventBusConfig(), logger)

	ctx := context.Background()
	err := eventBus.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start event bus: %v", err)
	}
	defer eventBus.Stop()

	// Create multiple subscribers for the same event type
	filter := &EventFilter{
		Types: []EventType{EventTypeStatus},
	}

	subscriber1, err := eventBus.Subscribe("subscriber-1", filter, 10)
	if err != nil {
		t.Fatalf("failed to subscribe subscriber 1: %v", err)
	}

	subscriber2, err := eventBus.Subscribe("subscriber-2", filter, 10)
	if err != nil {
		t.Fatalf("failed to subscribe subscriber 2: %v", err)
	}

	// Publish an event
	event := CreateEvent(EventTypeStatus, ActionCreate, map[string]string{
		"test": "multiple subscribers",
	})
	err = eventBus.Publish(event)
	if err != nil {
		t.Fatalf("failed to publish event: %v", err)
	}

	// Both subscribers should receive the event
	receivedCount := 0
	timeout := time.After(1 * time.Second)

	for receivedCount < 2 {
		select {
		case <-subscriber1.Channel:
			receivedCount++
		case <-subscriber2.Channel:
			receivedCount++
		case <-timeout:
			t.Fatalf("timeout waiting for events, received %d out of 2", receivedCount)
		}
	}

	if receivedCount != 2 {
		t.Errorf("expected 2 events to be received, got %d", receivedCount)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	logger := zaptest.NewLogger(t)
	eventBus := NewEventBus(DefaultEventBusConfig(), logger)

	ctx := context.Background()
	err := eventBus.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start event bus: %v", err)
	}
	defer eventBus.Stop()

	// Create a subscriber
	filter := &EventFilter{
		Types: []EventType{EventTypeStatus},
	}
	subscriber, err := eventBus.Subscribe("test-subscriber", filter, 10)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Verify subscriber exists
	subscribers := eventBus.GetSubscribers()
	if len(subscribers) != 1 {
		t.Errorf("expected 1 subscriber, got %d", len(subscribers))
	}

	// Unsubscribe
	err = eventBus.Unsubscribe("test-subscriber")
	if err != nil {
		t.Fatalf("failed to unsubscribe: %v", err)
	}

	// Verify subscriber was removed
	subscribers = eventBus.GetSubscribers()
	if len(subscribers) != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe, got %d", len(subscribers))
	}

	// Verify subscriber is inactive
	if subscriber.Active {
		t.Error("subscriber should be inactive after unsubscribe")
	}
}

func TestEventFilter_Matches(t *testing.T) {
	tests := []struct {
		name     string
		filter   *EventFilter
		event    *InternalEvent
		expected bool
	}{
		{
			name: "matches event type",
			filter: &EventFilter{
				Types: []EventType{EventTypeStatus},
			},
			event: &InternalEvent{
				Type: EventTypeStatus,
			},
			expected: true,
		},
		{
			name: "doesn't match event type",
			filter: &EventFilter{
				Types: []EventType{EventTypeNotification},
			},
			event: &InternalEvent{
				Type: EventTypeStatus,
			},
			expected: false,
		},
		{
			name: "matches action",
			filter: &EventFilter{
				Actions: []EventAction{ActionCreate},
			},
			event: &InternalEvent{
				Action: ActionCreate,
			},
			expected: true,
		},
		{
			name: "matches user ID",
			filter: &EventFilter{
				UserID: "user123",
			},
			event: &InternalEvent{
				UserID: "user123",
			},
			expected: true,
		},
		{
			name: "doesn't match user ID",
			filter: &EventFilter{
				UserID: "user123",
			},
			event: &InternalEvent{
				UserID: "user456",
			},
			expected: false,
		},
		{
			name: "matches metadata",
			filter: &EventFilter{
				Metadata: map[string]string{
					"visibility": "public",
				},
			},
			event: &InternalEvent{
				Metadata: map[string]string{
					"visibility": "public",
					"language":   "en",
				},
			},
			expected: true,
		},
		{
			name: "doesn't match metadata",
			filter: &EventFilter{
				Metadata: map[string]string{
					"visibility": "public",
				},
			},
			event: &InternalEvent{
				Metadata: map[string]string{
					"visibility": "private",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.Matches(tt.event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEventBusMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultEventBusConfig()
	config.MetricsEnabled = true
	eventBus := NewEventBus(config, logger)

	ctx := context.Background()
	err := eventBus.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start event bus: %v", err)
	}
	defer eventBus.Stop()

	// Create a subscriber
	filter := &EventFilter{
		Types: []EventType{EventTypeStatus},
	}
	_, err = eventBus.Subscribe("test-subscriber", filter, 10)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Publish some events
	for i := 0; i < 3; i++ {
		event := CreateEvent(EventTypeStatus, ActionCreate, map[string]string{
			"index": string(rune(i)),
		})
		err = eventBus.Publish(event)
		if err != nil {
			t.Fatalf("failed to publish event %d: %v", i, err)
		}
	}

	// Wait a bit for events to be processed
	time.Sleep(100 * time.Millisecond)

	// Check metrics
	metrics := eventBus.GetMetrics()
	if metrics == nil {
		t.Fatal("metrics should not be nil")
	}

	if metrics.EventsPublished != 3 {
		t.Errorf("expected 3 events published, got %d", metrics.EventsPublished)
	}

	if metrics.SubscribersActive != 1 {
		t.Errorf("expected 1 active subscriber, got %d", metrics.SubscribersActive)
	}

	if metrics.SubscribersTotal != 1 {
		t.Errorf("expected 1 total subscriber, got %d", metrics.SubscribersTotal)
	}

	// Give time for goroutines to finish before test ends
	time.Sleep(50 * time.Millisecond)
}