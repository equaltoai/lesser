package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGraphQLSubscriptionManager_StartStopAndIsRunning(t *testing.T) {
	t.Parallel()

	connRepo := inmemory.NewStreamingConnectionRepository()
	pub := streaming.NewMockPublisher()
	sm := NewGraphQLSubscriptionManager(connRepo, pub, zap.NewNop())

	require.False(t, sm.IsRunning())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, sm.Start(ctx))
	require.True(t, sm.IsRunning())
	require.ErrorIs(t, sm.Start(ctx), ErrSubscriptionManagerAlreadyRunning)

	require.NoError(t, sm.Stop())
	require.False(t, sm.IsRunning())
	require.NoError(t, sm.Stop())
}

func TestGraphQLSubscriptionManager_WithConnectionID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	require.Equal(t, "", connectionIDFromContext(ctx))

	ctx = WithConnectionID(ctx, "")
	require.Equal(t, "", connectionIDFromContext(ctx))

	ctx = WithConnectionID(ctx, "conn-1")
	require.Equal(t, "conn-1", connectionIDFromContext(ctx))
}

func TestGraphQLSubscriptionManager_createSubscriptionRecord_RepoAndConnectionErrors(t *testing.T) {
	t.Parallel()

	pub := streaming.NewMockPublisher()

	smNoRepo := NewGraphQLSubscriptionManager(nil, pub, zap.NewNop())
	_, err := smNoRepo.createSubscriptionRecord(context.Background(), "sub-1", "user-1", []string{"stream-a"})
	require.Error(t, err)

	connRepo := inmemory.NewStreamingConnectionRepository()
	sm := NewGraphQLSubscriptionManager(connRepo, pub, zap.NewNop())

	_, err = sm.createSubscriptionRecord(context.Background(), "sub-2", "user-2", []string{"stream-a"})
	require.Error(t, err)
}

func TestGraphQLSubscriptionManager_SubscribeVariantsAndCleanup(t *testing.T) {
	connRepo := inmemory.NewStreamingConnectionRepository()
	pub := streaming.NewMockPublisher()
	sm := NewGraphQLSubscriptionManager(connRepo, pub, zap.NewNop())

	// Not running guards.
	_, err := sm.SubscribeToTimeline(context.Background(), "alice", model.TimelineTypeHome, nil, nil, nil)
	require.ErrorIs(t, err, ErrSubscriptionManagerNotRunning)
	_, err = sm.SubscribeToHashtagActivity(context.Background(), "alice", nil)
	require.ErrorIs(t, err, ErrSubscriptionManagerNotRunning)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, sm.Start(ctx))
	t.Cleanup(func() { _ = sm.Stop() })

	ctx = WithConnectionID(ctx, "conn-1")

	// Exercise stream selection branches.
	_, err = sm.SubscribeToTimeline(ctx, "alice", model.TimelineTypeHome, nil, nil, nil)
	require.NoError(t, err)
	_, err = sm.SubscribeToTimeline(ctx, "alice", model.TimelineTypePublic, nil, nil, nil)
	require.NoError(t, err)
	_, err = sm.SubscribeToTimeline(ctx, "alice", model.TimelineTypeLocal, nil, nil, nil)
	require.NoError(t, err)
	_, err = sm.SubscribeToTimeline(ctx, "alice", model.TimelineTypeDirect, nil, nil, nil)
	require.NoError(t, err)
	_, err = sm.SubscribeToTimeline(ctx, "alice", model.TimelineTypeActor, nil, nil, nil)
	require.Error(t, err)

	// Various subscription types.
	_, err = sm.SubscribeToNotifications(ctx, "alice")
	require.NoError(t, err)

	threshold := 10
	_, err = sm.SubscribeToCostUpdates(ctx, "alice", &threshold)
	require.NoError(t, err)

	_, err = sm.SubscribeToModerationEvents(ctx, nil)
	require.NoError(t, err)
	actorID := "actor-1"
	_, err = sm.SubscribeToModerationEvents(ctx, &actorID)
	require.NoError(t, err)

	_, err = sm.SubscribeToTrustUpdates(ctx, "actor-2")
	require.NoError(t, err)

	_, err = sm.SubscribeToAIAnalysis(ctx, nil)
	require.NoError(t, err)
	objID := "obj-1"
	_, err = sm.SubscribeToAIAnalysis(ctx, &objID)
	require.NoError(t, err)

	_, err = sm.SubscribeToHashtagActivity(ctx, "alice", []string{"go", "lesser"})
	require.NoError(t, err)
	_, err = sm.SubscribeToHashtagActivity(ctx, "alice", []string{})
	require.ErrorIs(t, err, ErrAtLeastOneHashtagRequired)

	_, err = sm.SubscribeToQuoteActivity(ctx, "alice", "note-1")
	require.NoError(t, err)
	_, err = sm.SubscribeToQuoteActivity(ctx, "", "note-1")
	require.ErrorIs(t, err, ErrUsernameCannotBeEmpty)
	_, err = sm.SubscribeToQuoteActivity(ctx, "alice", "")
	require.ErrorIs(t, err, ErrNoteIDCannotBeEmpty)

	metricsThreshold := 0.5
	_, err = sm.SubscribeToMetricsUpdates(ctx, "alice", []string{"cost"}, []string{"timeline"}, &metricsThreshold)
	require.NoError(t, err)

	_, err = sm.SubscribeToListActivity(ctx, "alice", "list-1")
	require.NoError(t, err)

	_, err = sm.SubscribeToConversation(ctx, "alice")
	require.NoError(t, err)

	domain := "example.com"
	_, err = sm.SubscribeToFederationHealth(ctx, "alice", &domain)
	require.NoError(t, err)
	_, err = sm.SubscribeToFederationHealth(ctx, "alice", nil)
	require.NoError(t, err)

	_, err = sm.SubscribeToRelationshipUpdates(ctx, "alice", nil)
	require.NoError(t, err)
	relActor := "bob"
	_, err = sm.SubscribeToRelationshipUpdates(ctx, "alice", &relActor)
	require.NoError(t, err)

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

	stats := sm.GetStats()
	require.NotEmpty(t, stats)

	// Force cleanup of stale subscription to cover cleanup logic.
	sm.subscriptionsMux.Lock()
	for _, sub := range sm.subscriptions {
		sub.LastActivity = time.Now().Add(-10 * time.Minute)
		break
	}
	sm.subscriptionsMux.Unlock()
	sm.cleanupInactiveSubscriptions()
}
