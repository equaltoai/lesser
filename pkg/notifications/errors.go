package notifications

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Error constants for push notification operations
var (
	ErrLoadAWSConfig         = errors.ConnectionFailed("AWS config", stdErrors.New("AWS config connection failed"))
	ErrMarshalPushMessage    = errors.MarshalingFailed("push message", stdErrors.New("push message marshaling failed"))
	ErrQueuePushNotification = errors.ProcessingFailed("push notification queuing", stdErrors.New("failed to queue push notification"))
)
