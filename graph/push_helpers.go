package graph

import (
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
)

func convertPushSubscriptionToModel(subscription *storage.PushSubscription, serverKey string) *model.PushSubscription {
	if subscription == nil {
		return nil
	}

	var serverKeyPtr *string
	if serverKey != "" {
		serverKeyPtr = &serverKey
	}

	var createdAt *model.Time
	if !subscription.CreatedAt.IsZero() {
		t := model.Time(subscription.CreatedAt)
		createdAt = &t
	}

	var updatedAt *model.Time
	if !subscription.UpdatedAt.IsZero() {
		t := model.Time(subscription.UpdatedAt)
		updatedAt = &t
	}

	return &model.PushSubscription{
		ID:       subscription.ID,
		Endpoint: subscription.Endpoint,
		Keys: &model.PushSubscriptionKeys{
			Auth:   subscription.Auth,
			P256dh: subscription.P256dh,
		},
		Alerts: &model.PushSubscriptionAlerts{
			Follow:        subscription.Alerts.Follow,
			Favourite:     subscription.Alerts.Favourite,
			Reblog:        subscription.Alerts.Reblog,
			Mention:       subscription.Alerts.Mention,
			Poll:          subscription.Alerts.Poll,
			FollowRequest: subscription.Alerts.FollowRequest,
			Status:        subscription.Alerts.Status,
			Update:        subscription.Alerts.Update,
			AdminSignUp:   subscription.Alerts.AdminSignUp,
			AdminReport:   subscription.Alerts.AdminReport,
		},
		Policy:    subscription.Policy,
		ServerKey: serverKeyPtr,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}
