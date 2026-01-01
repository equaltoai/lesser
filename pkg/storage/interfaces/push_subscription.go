// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// PushSubscriptionRepository defines the interface for push subscription operations.
// This handles web push notifications and VAPID key management.
type PushSubscriptionRepository interface {
	// Push subscription operations
	CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error
	GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error)
	GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error)
	UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error
	DeletePushSubscription(ctx context.Context, username, subscriptionID string) error
	DeleteAllPushSubscriptions(ctx context.Context, username string) error

	// VAPID key management
	GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error)
	SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error
}
