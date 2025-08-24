package notifications

import "errors"

// Error constants for push notification operations
var (
	ErrLoadAWSConfig         = errors.New("failed to load AWS config")
	ErrMarshalPushMessage    = errors.New("failed to marshal push message")
	ErrQueuePushNotification = errors.New("failed to queue push notification")
)
