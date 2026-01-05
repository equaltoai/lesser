// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/google/uuid"
)

// PushSubscriptionRepository is a thread-safe in-memory implementation of interfaces.PushSubscriptionRepository.
type PushSubscriptionRepository struct {
	mu sync.RWMutex

	// Subscriptions: key = "username:subscriptionID"
	subscriptions map[string]*storage.PushSubscription

	// Index by user: username -> []subscriptionID
	byUser map[string][]string

	// VAPID keys
	vapidKeys *storage.VAPIDKeys
}

// NewPushSubscriptionRepository creates a new in-memory push subscription repository
func NewPushSubscriptionRepository() *PushSubscriptionRepository {
	return &PushSubscriptionRepository{
		subscriptions: make(map[string]*storage.PushSubscription),
		byUser:        make(map[string][]string),
	}
}

// subscriptionKey generates a unique key for a subscription
func subscriptionKey(username, subscriptionID string) string {
	return fmt.Sprintf("%s:%s", username, subscriptionID)
}

// CreatePushSubscription creates a new push subscription
func (r *PushSubscriptionRepository) CreatePushSubscription(_ context.Context, username string, subscription *storage.PushSubscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if subscription == nil {
		return fmt.Errorf("subscription cannot be nil")
	}

	if subscription.ID == "" {
		subscription.ID = uuid.New().String()
	}

	key := subscriptionKey(username, subscription.ID)
	if _, exists := r.subscriptions[key]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	subscription.Username = username
	subscription.CreatedAt = now
	subscription.UpdatedAt = now

	r.subscriptions[key] = subscription
	r.byUser[username] = append(r.byUser[username], subscription.ID)

	return nil
}

// GetPushSubscription retrieves a push subscription
func (r *PushSubscriptionRepository) GetPushSubscription(_ context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := subscriptionKey(username, subscriptionID)
	subscription, exists := r.subscriptions[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return subscription, nil
}

// GetUserPushSubscriptions retrieves all push subscriptions for a user
func (r *PushSubscriptionRepository) GetUserPushSubscriptions(_ context.Context, username string) ([]*storage.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subIDs := r.byUser[username]
	result := make([]*storage.PushSubscription, 0, len(subIDs))

	for _, id := range subIDs {
		key := subscriptionKey(username, id)
		if sub, exists := r.subscriptions[key]; exists {
			result = append(result, sub)
		}
	}

	return result, nil
}

// UpdatePushSubscription updates a push subscription's alerts
func (r *PushSubscriptionRepository) UpdatePushSubscription(_ context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := subscriptionKey(username, subscriptionID)
	subscription, exists := r.subscriptions[key]
	if !exists {
		return storage.ErrNotFound
	}

	subscription.Alerts = alerts
	subscription.UpdatedAt = time.Now()

	return nil
}

// DeletePushSubscription removes a push subscription
func (r *PushSubscriptionRepository) DeletePushSubscription(_ context.Context, username, subscriptionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := subscriptionKey(username, subscriptionID)
	if _, exists := r.subscriptions[key]; !exists {
		return storage.ErrNotFound
	}

	delete(r.subscriptions, key)
	r.byUser[username] = removePushKeyFromSlice(r.byUser[username], subscriptionID)

	return nil
}

// DeleteAllPushSubscriptions removes all push subscriptions for a user
func (r *PushSubscriptionRepository) DeleteAllPushSubscriptions(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	subIDs := r.byUser[username]
	for _, id := range subIDs {
		key := subscriptionKey(username, id)
		delete(r.subscriptions, key)
	}
	delete(r.byUser, username)

	return nil
}

// GetVAPIDKeys retrieves the VAPID keys
func (r *PushSubscriptionRepository) GetVAPIDKeys(_ context.Context) (*storage.VAPIDKeys, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.vapidKeys == nil {
		return nil, storage.ErrNotFound
	}

	return r.vapidKeys, nil
}

// SetVAPIDKeys sets the VAPID keys
func (r *PushSubscriptionRepository) SetVAPIDKeys(_ context.Context, keys *storage.VAPIDKeys) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if keys == nil {
		return fmt.Errorf("keys cannot be nil")
	}

	now := time.Now()
	if r.vapidKeys == nil {
		keys.CreatedAt = now
	}
	keys.UpdatedAt = now

	r.vapidKeys = keys

	return nil
}

// Helper functions

func removePushKeyFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// Test helper methods

// Clear clears all data (test helper)
func (r *PushSubscriptionRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.subscriptions = make(map[string]*storage.PushSubscription)
	r.byUser = make(map[string][]string)
	r.vapidKeys = nil
}

// GetSubscriptionCount returns the number of subscriptions (test helper)
func (r *PushSubscriptionRepository) GetSubscriptionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscriptions)
}

// HasVAPIDKeys returns whether VAPID keys are set (test helper)
func (r *PushSubscriptionRepository) HasVAPIDKeys() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.vapidKeys != nil
}

// Ensure PushSubscriptionRepository implements interfaces.PushSubscriptionRepository
var _ interfaces.PushSubscriptionRepository = (*PushSubscriptionRepository)(nil)
