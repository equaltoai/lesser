package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// UserPreferences resolves the current viewer's preference state.
func (r *queryResolver) UserPreferences(ctx context.Context) (*model.UserPreferences, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	state := r.loadPreferenceState(ctx, username)
	return r.convertPreferenceStateToModel(state, username), nil
}

// PushSubscription resolves the viewer's push subscription, if one exists.
func (r *queryResolver) PushSubscription(ctx context.Context) (*model.PushSubscription, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	storage := r.Registry.GetStorage()
	if storage == nil || storage.PushSubscription() == nil {
		return nil, errors.New("push subscription repository is not available")
	}

	repo := storage.PushSubscription()
	subscriptions, err := repo.GetUserPushSubscriptions(ctx, username)
	if err != nil {
		r.Logger.Error("Failed to load push subscriptions",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to load push subscriptions"), err)
	}

	if len(subscriptions) == 0 {
		return nil, nil
	}

	var serverKey string
	if keys, err := repo.GetVAPIDKeys(ctx); err == nil && keys != nil {
		serverKey = keys.PublicKey
	} else if err != nil {
		r.Logger.Warn("Failed to load VAPID keys",
			zap.String("username", username),
			zap.Error(err))
	}

	return convertPushSubscriptionToModel(subscriptions[0], serverKey), nil
}
