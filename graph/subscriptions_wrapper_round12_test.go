package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound12SubscriptionManagerWrapper_StartStopAndDelegates(t *testing.T) {
	connRepo := inmemory.NewStreamingConnectionRepository()
	pub := streaming.NewMockPublisher()
	sm := NewSubscriptionManager(connRepo, pub, nil)

	require.False(t, sm.IsRunning())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, sm.Start(ctx))
	require.True(t, sm.IsRunning())

	ctx = WithConnectionID(ctx, "conn-1")

	_, err := sm.SubscribeToTimelineUpdates(ctx, "alice", model.TimelineTypeHome)
	require.NoError(t, err)

	threshold := 10
	_, err = sm.SubscribeToCostUpdates(ctx, "alice", &threshold)
	require.NoError(t, err)

	_, err = sm.SubscribeToModerationEvents(ctx, nil)
	require.NoError(t, err)

	_, err = sm.SubscribeToTrustUpdates(ctx, "actor-1")
	require.NoError(t, err)

	_, err = sm.SubscribeToNotifications(ctx, "alice")
	require.NoError(t, err)

	_, err = sm.SubscribeToAIAnalysisUpdates(ctx, nil)
	require.NoError(t, err)

	_, err = sm.SubscribeToHashtagActivity(ctx, "alice", []string{"go"})
	require.NoError(t, err)

	metricsThreshold := 0.5
	_, err = sm.SubscribeToMetricsUpdates(ctx, "alice", []string{"cost"}, []string{"timeline"}, &metricsThreshold)
	require.NoError(t, err)

	_, err = sm.SubscribeToQuoteActivity(ctx, "alice", "note-1")
	require.NoError(t, err)

	_, err = sm.SubscribeToListActivity(ctx, "alice", "list-1")
	require.NoError(t, err)

	_, err = sm.SubscribeToConversation(ctx, "alice")
	require.NoError(t, err)

	_, err = sm.SubscribeToFederationHealth(ctx, "alice", nil)
	require.NoError(t, err)

	_, err = sm.SubscribeToRelationshipUpdates(ctx, "alice", nil)
	require.NoError(t, err)

	domain := "example.com"
	_, err = sm.SubscribeToBudgetAlerts(ctx, "alice", &domain)
	require.NoError(t, err)

	sev := model.ModerationSeverityHigh
	_, err = sm.SubscribeToModerationAlerts(ctx, "alice", &sev)
	require.NoError(t, err)

	_, err = sm.SubscribeToCostAlerts(ctx, "alice", 1.23)
	require.NoError(t, err)

	_, err = sm.SubscribeToPerformanceAlerts(ctx, "alice", model.AlertSeverityCritical)
	require.NoError(t, err)

	_, err = sm.SubscribeToThreatIntelligence(ctx, "alice")
	require.NoError(t, err)

	_, err = sm.SubscribeToInfrastructureEvents(ctx, "alice")
	require.NoError(t, err)

	priority := model.PriorityHigh
	_, err = sm.SubscribeToModerationQueueUpdate(ctx, "alice", &priority)
	require.NoError(t, err)

	require.NotEmpty(t, sm.GetStats())

	require.NoError(t, sm.Stop())
	require.False(t, sm.IsRunning())
}

func TestRound12SubscriptionManagerWrapper_ActivityStream_ConvertsObjectsToActivities(t *testing.T) {
	connRepo := inmemory.NewStreamingConnectionRepository()
	pub := streaming.NewMockPublisher()
	sm := NewSubscriptionManager(connRepo, pub, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, sm.Start(ctx))
	t.Cleanup(func() { _ = sm.Stop() })

	ctx = WithConnectionID(ctx, "conn-1")

	activityCh, err := sm.SubscribeToActivityStream(ctx, "alice", []model.ActivityType{model.ActivityType(activitypub.CreateType)})
	require.NoError(t, err)

	// Feed a timeline object directly into the underlying subscription channel.
	sm.manager.subscriptionsMux.RLock()
	var timelineSub *GraphQLSubscription
	for _, sub := range sm.manager.subscriptions {
		timelineSub = sub
		break
	}
	sm.manager.subscriptionsMux.RUnlock()
	require.NotNil(t, timelineSub)

	objCh, ok := timelineSub.OutputChannel.(chan *model.Object)
	require.True(t, ok)

	objCh <- &model.Object{
		ID:    "obj-1",
		Type:  model.ObjectTypeNote,
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://localhost/users/alice"}},
	}

	select {
	case act := <-activityCh:
		require.NotNil(t, act)
		require.Equal(t, activitypub.CreateType, act.Type)
		require.Equal(t, "https://localhost/users/alice", act.Actor)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for activity")
	}

	cancel()
}

func TestRound12SubscriptionManagerWrapper_ActivityStream_FiltersActivityTypes(t *testing.T) {
	connRepo := inmemory.NewStreamingConnectionRepository()
	pub := streaming.NewMockPublisher()
	sm := NewSubscriptionManager(connRepo, pub, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, sm.Start(ctx))
	t.Cleanup(func() { _ = sm.Stop() })

	ctx = WithConnectionID(ctx, "conn-1")

	activityCh, err := sm.SubscribeToActivityStream(ctx, "alice", []model.ActivityType{model.ActivityType("Like")})
	require.NoError(t, err)

	sm.manager.subscriptionsMux.RLock()
	var timelineSub *GraphQLSubscription
	for _, sub := range sm.manager.subscriptions {
		timelineSub = sub
		break
	}
	sm.manager.subscriptionsMux.RUnlock()
	require.NotNil(t, timelineSub)

	objCh, ok := timelineSub.OutputChannel.(chan *model.Object)
	require.True(t, ok)

	objCh <- &model.Object{
		ID:    "obj-2",
		Type:  model.ObjectTypeNote,
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://localhost/users/alice"}},
	}

	select {
	case act := <-activityCh:
		t.Fatalf("unexpected activity delivered: %v", act)
	case <-time.After(150 * time.Millisecond):
		// Expected: filter drops non-matching type.
	}

	cancel()
}
