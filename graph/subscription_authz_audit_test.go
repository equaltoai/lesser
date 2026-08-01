package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionResolvers_AnonymousAuthorizationMatrix(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	ctx := context.Background()
	listID := "list-1"

	gated := map[string]func() error{
		"actor timeline": func() error {
			_, err := resolver.Subscription().TimelineUpdates(ctx, model.TimelineTypeActor, nil)
			return err
		},
		"activity stream": func() error {
			_, err := resolver.Subscription().ActivityStream(ctx, nil)
			return err
		},
		"home timeline": func() error {
			_, err := resolver.Subscription().TimelineUpdates(ctx, model.TimelineTypeHome, nil)
			return err
		},
		"hashtag timeline": func() error {
			_, err := resolver.Subscription().TimelineUpdates(ctx, model.TimelineTypeHashtag, nil)
			return err
		},
		"list timeline": func() error {
			_, err := resolver.Subscription().TimelineUpdates(ctx, model.TimelineTypeList, &listID)
			return err
		},
		"direct timeline": func() error {
			_, err := resolver.Subscription().TimelineUpdates(ctx, model.TimelineTypeDirect, nil)
			return err
		},
		"notification stream": func() error {
			_, err := resolver.Subscription().NotificationStream(ctx, nil)
			return err
		},
		"conversation updates": func() error {
			_, err := resolver.Subscription().ConversationUpdates(ctx)
			return err
		},
		"list updates": func() error {
			_, err := resolver.Subscription().ListUpdates(ctx, listID)
			return err
		},
		"relationship updates": func() error {
			_, err := resolver.Subscription().RelationshipUpdates(ctx, nil)
			return err
		},
		"cost updates": func() error {
			_, err := resolver.Subscription().CostUpdates(ctx, nil)
			return err
		},
		"moderation events": func() error {
			_, err := resolver.Subscription().ModerationEvents(ctx, nil)
			return err
		},
		"trust updates": func() error {
			_, err := resolver.Subscription().TrustUpdates(ctx, "actor-1")
			return err
		},
		"AI analysis updates": func() error {
			_, err := resolver.Subscription().AiAnalysisUpdates(ctx, nil)
			return err
		},
		"quote activity": func() error {
			_, err := resolver.Subscription().QuoteActivity(ctx, "note-1")
			return err
		},
		"hashtag activity": func() error {
			_, err := resolver.Subscription().HashtagActivity(ctx, []string{"go"})
			return err
		},
		"metrics updates": func() error {
			_, err := resolver.Subscription().MetricsUpdates(ctx, nil, nil, nil)
			return err
		},
		"moderation alerts": func() error {
			_, err := resolver.Subscription().ModerationAlerts(ctx, nil)
			return err
		},
		"cost alerts": func() error {
			_, err := resolver.Subscription().CostAlerts(ctx, 1)
			return err
		},
		"budget alerts": func() error {
			_, err := resolver.Subscription().BudgetAlerts(ctx, nil)
			return err
		},
		"federation health updates": func() error {
			_, err := resolver.Subscription().FederationHealthUpdates(ctx, nil)
			return err
		},
		"moderation queue updates": func() error {
			_, err := resolver.Subscription().ModerationQueueUpdate(ctx, nil)
			return err
		},
		"threat intelligence": func() error {
			_, err := resolver.Subscription().ThreatIntelligence(ctx)
			return err
		},
		"performance alerts": func() error {
			_, err := resolver.Subscription().PerformanceAlert(ctx, model.AlertSeverityCritical)
			return err
		},
		"infrastructure events": func() error {
			_, err := resolver.Subscription().InfrastructureEvent(ctx)
			return err
		},
		"agent activity": func() error {
			_, err := resolver.Subscription().AgentActivity(ctx, "agent-1")
			return err
		},
	}

	for name, invoke := range gated {
		t.Run(name, func(t *testing.T) {
			err := invoke()
			require.Error(t, err)
			require.True(t, errors.HasCode(err, errors.CodeUnauthorized), "unexpected error: %v", err)
		})
	}
}

func TestTimelineUpdates_AnonymousSafeTypesStream(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	connRepo := inmemory.NewStreamingConnectionRepository()
	manager := NewSubscriptionManager(connRepo, streaming.NewMockPublisher(), nil)
	resolver.SubscriptionManager = manager

	ctx, cancel := context.WithCancel(WithConnectionID(context.Background(), "anonymous-conn"))
	t.Cleanup(cancel)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { _ = manager.Stop() })

	for _, timelineType := range []model.TimelineType{
		model.TimelineTypePublic,
		model.TimelineTypeLocal,
	} {
		t.Run(string(timelineType), func(t *testing.T) {
			manager.manager.subscriptionsMux.RLock()
			before := make(map[string]struct{}, len(manager.manager.subscriptions))
			for id := range manager.manager.subscriptions {
				before[id] = struct{}{}
			}
			manager.manager.subscriptionsMux.RUnlock()

			updates, err := resolver.Subscription().TimelineUpdates(ctx, timelineType, nil)
			require.NoError(t, err)
			require.NotNil(t, updates)

			manager.manager.subscriptionsMux.RLock()
			var subscription *GraphQLSubscription
			for id, candidate := range manager.manager.subscriptions {
				if _, existed := before[id]; !existed {
					subscription = candidate
					break
				}
			}
			manager.manager.subscriptionsMux.RUnlock()
			require.NotNil(t, subscription)

			objectChannel, ok := subscription.OutputChannel.(chan *model.Object)
			require.True(t, ok)
			want := &model.Object{ID: "object-" + string(timelineType), Type: model.ObjectTypeNote}
			objectChannel <- want

			select {
			case got := <-updates:
				require.Equal(t, want, got)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for anonymous timeline event")
			}
		})
	}
}

func TestSubscribeToTimeline_RoutesOnlyImplementedStreams(t *testing.T) {
	connRepo := inmemory.NewStreamingConnectionRepository()
	manager := NewGraphQLSubscriptionManager(connRepo, streaming.NewMockPublisher(), nil)
	ctx, cancel := context.WithCancel(WithConnectionID(context.Background(), "routing-conn"))
	t.Cleanup(cancel)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { _ = manager.Stop() })

	_, err := manager.SubscribeToTimeline(ctx, "", model.TimelineTypePublic)
	require.NoError(t, err)
	publicSubscriptions, err := connRepo.GetSubscriptionsForStream(ctx, StreamNamePublic)
	require.NoError(t, err)
	require.Len(t, publicSubscriptions, 1)

	_, err = manager.SubscribeToTimeline(ctx, "", model.TimelineTypeLocal)
	require.NoError(t, err)
	localSubscriptions, err := connRepo.GetSubscriptionsForStream(ctx, "public:local")
	require.NoError(t, err)
	require.Len(t, localSubscriptions, 1)

	_, err = manager.SubscribeToTimeline(ctx, "", model.TimelineTypeActor)
	require.Error(t, err)
	emptyUserSubscriptions, err := connRepo.GetSubscriptionsForStream(ctx, "user:")
	require.NoError(t, err)
	require.Empty(t, emptyUserSubscriptions)

	_, err = manager.SubscribeToTimeline(ctx, "alice", model.TimelineTypeHome)
	require.NoError(t, err)
	homeSubscriptions, err := connRepo.GetSubscriptionsForStream(ctx, "user:alice")
	require.NoError(t, err)
	require.Len(t, homeSubscriptions, 1)

	for _, tc := range []struct {
		name       string
		username   string
		typeValue  model.TimelineType
		streamName string
	}{
		{name: "home spaces", username: "   ", typeValue: model.TimelineTypeHome, streamName: "user:   "},
		{name: "home tab", username: "\t", typeValue: model.TimelineTypeHome, streamName: "user:\t"},
		{name: "direct spaces", username: "   ", typeValue: model.TimelineTypeDirect, streamName: "direct:   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, subscribeErr := manager.SubscribeToTimeline(ctx, tc.username, tc.typeValue)
			require.ErrorIs(t, subscribeErr, ErrUsernameCannotBeEmpty)
			subscriptions, lookupErr := connRepo.GetSubscriptionsForStream(ctx, tc.streamName)
			require.NoError(t, lookupErr)
			require.Empty(t, subscriptions)
		})
	}
}
