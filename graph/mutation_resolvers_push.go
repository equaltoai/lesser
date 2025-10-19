package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// RegisterPushSubscription registers a browser/device for push notifications.
func (r *mutationResolver) RegisterPushSubscription(ctx context.Context, input model.RegisterPushSubscriptionInput) (*model.PushSubscription, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateRequiredParam("endpoint", input.Endpoint); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("keys.auth", input.Keys.Auth); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("keys.p256dh", input.Keys.P256dh); err != nil {
		return nil, err
	}

	store := r.Registry.GetStorage()
	if store == nil || store.PushSubscription() == nil {
		return nil, errors.New("push subscription repository is not available")
	}

	repo := store.PushSubscription()
	if err := repo.DeleteAllPushSubscriptions(ctx, username); err != nil {
		r.Logger.Warn("Failed to clear existing push subscriptions",
			zap.String("username", username),
			zap.Error(err))
	}

	subscription := &storage.PushSubscription{
		Endpoint: input.Endpoint,
		Auth:     input.Keys.Auth,
		P256dh:   input.Keys.P256dh,
		Alerts:   buildPushAlerts(input.Alerts, nil),
		Policy:   "all",
	}

	if err := repo.CreatePushSubscription(ctx, username, subscription); err != nil {
		r.Logger.Error("Failed to register push subscription",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to register push subscription"), err)
	}

	var serverKey string
	if keys, err := repo.GetVAPIDKeys(ctx); err == nil && keys != nil {
		serverKey = keys.PublicKey
	} else if err != nil {
		r.Logger.Warn("Failed to fetch VAPID keys",
			zap.String("username", username),
			zap.Error(err))
	}

	return convertPushSubscriptionToModel(subscription, serverKey), nil
}

// UpdatePushSubscription mutates the alert preferences for the viewer's subscription.
func (r *mutationResolver) UpdatePushSubscription(ctx context.Context, input model.UpdatePushSubscriptionInput) (*model.PushSubscription, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	store := r.Registry.GetStorage()
	if store == nil || store.PushSubscription() == nil {
		return nil, errors.New("push subscription repository is not available")
	}

	repo := store.PushSubscription()
	subscriptions, err := repo.GetUserPushSubscriptions(ctx, username)
	if err != nil {
		r.Logger.Error("Failed to load push subscriptions",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to load push subscriptions"), err)
	}
	if len(subscriptions) == 0 {
		return nil, errors.New("push subscription not found")
	}

	existing := subscriptions[0]
	alerts := buildPushAlerts(input.Alerts, &existing.Alerts)

	if err := repo.UpdatePushSubscription(ctx, username, existing.ID, alerts); err != nil {
		r.Logger.Error("Failed to update push subscription",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update push subscription"), err)
	}

	var serverKey string
	if keys, err := repo.GetVAPIDKeys(ctx); err == nil && keys != nil {
		serverKey = keys.PublicKey
	} else if err != nil {
		r.Logger.Warn("Failed to fetch VAPID keys",
			zap.String("username", username),
			zap.Error(err))
	}

	existing.Alerts = alerts
	return convertPushSubscriptionToModel(existing, serverKey), nil
}

// DeletePushSubscription removes all push subscriptions for the viewer.
func (r *mutationResolver) DeletePushSubscription(ctx context.Context) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	store := r.Registry.GetStorage()
	if store == nil || store.PushSubscription() == nil {
		return false, errors.New("push subscription repository is not available")
	}

	if err := store.PushSubscription().DeleteAllPushSubscriptions(ctx, username); err != nil {
		r.Logger.Error("Failed to delete push subscriptions",
			zap.String("username", username),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to delete push subscription"), err)
	}

	return true, nil
}

func buildPushAlerts(input *model.PushSubscriptionAlertsInput, fallback *storage.PushSubscriptionAlerts) storage.PushSubscriptionAlerts {
	base := storage.PushSubscriptionAlerts{}
	if fallback != nil {
		base = *fallback
	}
	if input == nil {
		return base
	}
	if input.Follow != nil {
		base.Follow = *input.Follow
	}
	if input.Favourite != nil {
		base.Favourite = *input.Favourite
	}
	if input.Reblog != nil {
		base.Reblog = *input.Reblog
	}
	if input.Mention != nil {
		base.Mention = *input.Mention
	}
	if input.Poll != nil {
		base.Poll = *input.Poll
	}
	if input.FollowRequest != nil {
		base.FollowRequest = *input.FollowRequest
	}
	if input.Status != nil {
		base.Status = *input.Status
	}
	if input.Update != nil {
		base.Update = *input.Update
	}
	if input.AdminSignUp != nil {
		base.AdminSignUp = *input.AdminSignUp
	}
	if input.AdminReport != nil {
		base.AdminReport = *input.AdminReport
	}
	return base
}
