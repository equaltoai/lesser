package main

import "errors"

// Error constants for stream-router
var (
	// Connection and subscription errors
	ErrConnectionNotFound      = errors.New("connection not found")
	ErrWebSocketEndpointNotSet = errors.New("WEBSOCKET_ENDPOINT environment variable not set")
	ErrHandlerNotInitialized   = errors.New("handler not initialized")

	// Processing errors
	ErrAllRecordsFailedProcessing = errors.New("all records failed processing")
	ErrBroadcastToAllFollowers    = errors.New("failed to broadcast to all followers")
	ErrSendToAllConnections       = errors.New("failed to send to all connections")

	// Data validation errors
	ErrNotificationMissingUsername = errors.New("notification missing recipient username")
	ErrAccountMissingID            = errors.New("account missing ID")
	ErrUsernameCannotBeEmpty       = errors.New("username cannot be empty")
	ErrCouldNotExtractUsername     = errors.New("could not extract username from account ID")

	// Event processing errors
	ErrUnknownEventName = errors.New("unknown event name")

	// Stream subscription errors
	ErrFailedToGetSubscriptionsForStream = errors.New("failed to get subscriptions for stream")
	ErrFailedToQueryConnection           = errors.New("failed to query connection")

	// Event bus errors
	ErrFailedToStartInternalEventBus = errors.New("failed to start internal event bus")

	// Marshaling errors
	ErrFailedToMarshalStatus       = errors.New("failed to marshal status")
	ErrFailedToMarshalNotification = errors.New("failed to marshal notification")
	ErrFailedToMarshalAccount      = errors.New("failed to marshal account")
	ErrFailedToMarshalMessage      = errors.New("failed to marshal message")

	// Account payload errors
	ErrFailedToCreateAccountPayload = errors.New("failed to create account payload")

	// Follower retrieval errors
	ErrFailedToGetFollowers = errors.New("failed to get followers")

	// Internal event bus publishing errors
	ErrFailedToPublishToInternalEventBus = errors.New("failed to publish to internal event bus")

	// Repository errors
	ErrFailedToGetSubscriptions = errors.New("failed to get subscriptions")

	// Status extraction errors
	ErrFailedToGetStatusForHashtagExtraction = errors.New("failed to get status for hashtag extraction")

	// Hashtag processing errors
	ErrHashtagProcessingFailed = errors.New("hashtag processing failed")
)
