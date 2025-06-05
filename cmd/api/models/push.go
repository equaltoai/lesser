package models

import "time"

// PushSubscription represents a push notification subscription
type PushSubscription struct {
	ID        string                 `json:"id"`
	Endpoint  string                 `json:"endpoint"`
	Keys      PushSubscriptionKeys   `json:"keys"`
	Alerts    PushSubscriptionAlerts `json:"alerts"`
	Policy    string                 `json:"policy,omitempty"`
	ServerKey string                 `json:"server_key"`
}

// PushSubscriptionKeys represents the keys for push encryption
type PushSubscriptionKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// PushSubscriptionAlerts represents which events trigger push notifications
type PushSubscriptionAlerts struct {
	Follow        bool `json:"follow"`
	Favourite     bool `json:"favourite"`
	Reblog        bool `json:"reblog"`
	Mention       bool `json:"mention"`
	Poll          bool `json:"poll"`
	FollowRequest bool `json:"follow_request"`
	Status        bool `json:"status"`
	Update        bool `json:"update"`
	AdminSignUp   bool `json:"admin.sign_up"`
	AdminReport   bool `json:"admin.report"`
}

// PushSubscriptionRequest represents a request to create/update push subscription
type PushSubscriptionRequest struct {
	Subscription PushSubscriptionData   `json:"subscription"`
	Data         PushSubscriptionAlerts `json:"data"`
}

// PushSubscriptionData contains the subscription endpoint and keys
type PushSubscriptionData struct {
	Endpoint string               `json:"endpoint"`
	Keys     PushSubscriptionKeys `json:"keys"`
}

// PushNotification represents a notification to be sent via push
type PushNotification struct {
	ID               string    `json:"-"`
	Username         string    `json:"-"`
	SubscriptionID   string    `json:"-"`
	NotificationType string    `json:"notification_type"`
	Title            string    `json:"title"`
	Body             string    `json:"body"`
	Icon             string    `json:"icon,omitempty"`
	PreferredLocale  string    `json:"preferred_locale"`
	AccessToken      string    `json:"access_token"`
	NotificationID   string    `json:"notification_id"`
	CreatedAt        time.Time `json:"-"`
}
