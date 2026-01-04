package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound12PushHelpers_ConvertPushSubscriptionToModel(t *testing.T) {
	t.Parallel()

	require.Nil(t, convertPushSubscriptionToModel(nil, ""))

	now := time.Now()
	sub := &storage.PushSubscription{
		ID:       "sub-1",
		Endpoint: "https://push.local/endpoint",
		Auth:     "auth",
		P256dh:   "p256dh",
		Alerts: storage.PushSubscriptionAlerts{
			Follow:  true,
			Mention: true,
		},
		Policy:    "all",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now.Add(-time.Minute),
	}

	converted := convertPushSubscriptionToModel(sub, "")
	require.NotNil(t, converted)
	require.Equal(t, sub.ID, converted.ID)
	require.Equal(t, sub.Endpoint, converted.Endpoint)
	require.NotNil(t, converted.Keys)
	require.Equal(t, sub.Auth, converted.Keys.Auth)
	require.Equal(t, sub.P256dh, converted.Keys.P256dh)
	require.NotNil(t, converted.Alerts)
	require.True(t, converted.Alerts.Follow)
	require.True(t, converted.Alerts.Mention)
	require.Nil(t, converted.ServerKey)
	require.NotNil(t, converted.CreatedAt)
	require.NotNil(t, converted.UpdatedAt)

	convertedWithKey := convertPushSubscriptionToModel(sub, "public-key")
	require.NotNil(t, convertedWithKey.ServerKey)
	require.Equal(t, "public-key", *convertedWithKey.ServerKey)
}

func TestRound12PushHelpers_BuildPushAlerts(t *testing.T) {
	t.Parallel()

	fallback := storage.PushSubscriptionAlerts{
		Follow:        true,
		Favourite:     true,
		Reblog:        true,
		Mention:       true,
		FollowRequest: true,
	}

	require.Equal(t, fallback, buildPushAlerts(nil, &fallback))

	input := &model.PushSubscriptionAlertsInput{
		Follow: ptrBool(false),
		Poll:   ptrBool(true),
	}

	out := buildPushAlerts(input, &fallback)
	require.False(t, out.Follow)
	require.True(t, out.Poll)
	require.True(t, out.Favourite)
	require.True(t, out.Reblog)
	require.True(t, out.Mention)
	require.True(t, out.FollowRequest)
}

func TestRound12MutationResolvers_PushSubscriptions_MainlineAndErrors(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	resolver.Logger = zap.NewNop()

	mutations := &mutationResolver{resolver}

	// Auth required.
	_, err := mutations.RegisterPushSubscription(context.Background(), model.RegisterPushSubscriptionInput{})
	require.Error(t, err)

	ctx := round12AuthContext("alice")

	// Validation required.
	_, err = mutations.RegisterPushSubscription(ctx, model.RegisterPushSubscriptionInput{
		Keys: &model.PushSubscriptionKeysInput{
			Auth:   "auth",
			P256dh: "p256dh",
		},
	})
	require.Error(t, err)

	// Register succeeds.
	registered, err := mutations.RegisterPushSubscription(ctx, model.RegisterPushSubscriptionInput{
		Endpoint: "https://push.local/endpoint",
		Keys: &model.PushSubscriptionKeysInput{
			Auth:   "auth",
			P256dh: "p256dh",
		},
		Alerts: &model.PushSubscriptionAlertsInput{
			Follow:  ptrBool(true),
			Reblog:  ptrBool(true),
			Mention: ptrBool(true),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, registered)
	require.Equal(t, "https://push.local/endpoint", registered.Endpoint)

	// Update errors when no subscriptions found.
	_, err = mutations.UpdatePushSubscription(ctx, model.UpdatePushSubscriptionInput{})
	require.Error(t, err)

	// Update succeeds when the repo returns a subscription.
	state.autoPopulateAll = true
	state.autoPopulateCount = 1

	updated, err := mutations.UpdatePushSubscription(ctx, model.UpdatePushSubscriptionInput{
		Alerts: &model.PushSubscriptionAlertsInput{
			Follow:      ptrBool(false),
			Mention:     ptrBool(true),
			AdminReport: ptrBool(true),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.Alerts)
	require.False(t, updated.Alerts.Follow)
	require.True(t, updated.Alerts.Mention)
	require.True(t, updated.Alerts.AdminReport)

	// Delete succeeds.
	ok, err := mutations.DeletePushSubscription(ctx)
	require.NoError(t, err)
	require.True(t, ok)
}
