package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound12SubscriptionResolvers_ManagerGuardsAndTimelineValidation(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")

	// Manager missing.
	ch, err := resolver.Subscription().ActivityStream(ctx, nil)
	require.Error(t, err)
	require.NotNil(t, ch)
	_, ok := <-ch
	require.False(t, ok)

	// Manager present but not running.
	connRepo := inmemory.NewStreamingConnectionRepository()
	pub := streaming.NewMockPublisher()
	resolver.SubscriptionManager = NewSubscriptionManager(connRepo, pub, nil)

	ch, err = resolver.Subscription().ActivityStream(WithConnectionID(ctx, "conn-1"), nil)
	require.Error(t, err)
	require.NotNil(t, ch)
	_, ok = <-ch
	require.False(t, ok)

	// Timeline validations.
	_, err = resolver.Subscription().TimelineUpdates(ctx, model.TimelineTypeHome, nil)
	require.Error(t, err)
	_, err = resolver.Subscription().TimelineUpdates(context.Background(), model.TimelineTypeList, nil)
	require.Error(t, err)
}

func TestRound12SubscriptionResolvers_SuccessPaths(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	connRepo := inmemory.NewStreamingConnectionRepository()
	pub := streaming.NewMockPublisher()
	sm := NewSubscriptionManager(connRepo, pub, nil)
	resolver.SubscriptionManager = sm

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, sm.Start(ctx))
	t.Cleanup(func() { _ = sm.Stop() })

	authCtx := WithConnectionID(round12AuthContext("alice"), "conn-1")

	_, err := resolver.Subscription().ActivityStream(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().TimelineUpdates(authCtx, model.TimelineTypeHome, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().RelationshipUpdates(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().TrustUpdates(authCtx, "actor-1")
	require.NoError(t, err)

	_, err = resolver.Subscription().NotificationStream(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().AiAnalysisUpdates(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().ThreatIntelligence(authCtx)
	require.NoError(t, err)

	_, err = resolver.Subscription().ListUpdates(authCtx, "list-1")
	require.NoError(t, err)

	_, err = resolver.Subscription().ConversationUpdates(authCtx)
	require.NoError(t, err)

	_, err = resolver.Subscription().QuoteActivity(authCtx, "note-1")
	require.NoError(t, err)

	_, err = resolver.Subscription().ModerationEvents(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().ModerationAlerts(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().ModerationQueueUpdate(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().FederationHealthUpdates(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().InfrastructureEvent(authCtx)
	require.NoError(t, err)

	_, err = resolver.Subscription().BudgetAlerts(authCtx, nil)
	require.NoError(t, err)

	_, err = resolver.Subscription().CostAlerts(authCtx, 1.23)
	require.NoError(t, err)

	_, err = resolver.Subscription().CostUpdates(authCtx, nil)
	require.NoError(t, err)

	metricsThreshold := 0.5
	_, err = resolver.Subscription().MetricsUpdates(authCtx, []string{"cost"}, []string{"timeline"}, &metricsThreshold)
	require.NoError(t, err)

	_, err = resolver.Subscription().PerformanceAlert(authCtx, model.AlertSeverityCritical)
	require.NoError(t, err)

	_, err = resolver.Subscription().HashtagActivity(authCtx, []string{"go"})
	require.NoError(t, err)
}

func TestRound12SubscriptionResolvers_NotificationStream_TypeFiltering(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	connRepo := inmemory.NewStreamingConnectionRepository()
	pub := streaming.NewMockPublisher()
	sm := NewSubscriptionManager(connRepo, pub, nil)
	resolver.SubscriptionManager = sm

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, sm.Start(ctx))
	t.Cleanup(func() { _ = sm.Stop() })

	authCtx := WithConnectionID(round12AuthContext("alice"), "conn-1")

	filtered, err := resolver.Subscription().NotificationStream(authCtx, []string{"follow"})
	require.NoError(t, err)
	require.NotNil(t, filtered)

	// Feed notifications into the underlying subscription channel.
	sm.manager.subscriptionsMux.RLock()
	var notifSub *GraphQLSubscription
	for _, sub := range sm.manager.subscriptions {
		if sub.Type == "notification" {
			notifSub = sub
			break
		}
	}
	sm.manager.subscriptionsMux.RUnlock()
	require.NotNil(t, notifSub)

	inputCh, ok := notifSub.OutputChannel.(chan *model.Notification)
	require.True(t, ok)

	inputCh <- &model.Notification{ID: "n1", Type: "follow"}
	inputCh <- &model.Notification{ID: "n2", Type: "mention"}

	select {
	case got := <-filtered:
		require.NotNil(t, got)
		require.Equal(t, "follow", got.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for filtered notification")
	}

	cancel()
}

func TestRound12SubscriptionResolvers_GetConnectionID(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	sr := &subscriptionResolver{resolver}

	require.Equal(t, "", sr.getConnectionID(context.Background()))
	require.Equal(t, "conn-1", sr.getConnectionID(WithConnectionID(context.Background(), "conn-1")))
}
