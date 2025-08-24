package notifications

import "github.com/equaltoai/lesser/pkg/errors"

// Error constants for push notification operations
var (
	ErrLoadAWSConfig         = errors.ConnectionFailed("AWS config", nil)
	ErrMarshalPushMessage    = errors.MarshalingFailed("push message", nil)
	ErrQueuePushNotification = errors.ProcessingFailed("push notification queuing", nil)
)
