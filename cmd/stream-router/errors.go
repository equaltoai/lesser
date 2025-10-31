package main

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Stream router error functions using centralized error system

// Connection and subscription errors

// ConnectionNotFound creates an error indicating a WebSocket connection was not found.
func ConnectionNotFound() *errors.AppError {
	return errors.NewLambdaError(errors.CodeNotFound, "connection not found")
}

// WebSocketEndpointNotSet creates an error indicating the WEBSOCKET_ENDPOINT environment variable is not configured.
func WebSocketEndpointNotSet() *errors.AppError {
	return errors.EnvironmentVariableMissing("WEBSOCKET_ENDPOINT")
}

// HandlerNotInitialized creates an error indicating the stream router handler has not been initialized.
func HandlerNotInitialized() *errors.AppError {
	return errors.ServiceInitializationFailed("stream-router handler", nil)
}

// Processing errors

// AllRecordsFailedProcessing creates an error indicating all records in a batch failed processing.
func AllRecordsFailedProcessing() *errors.AppError {
	return errors.SQSBatchProcessingFailed(0, 0, stdErrors.New("all records in batch failed to process")).WithMetadata("reason", "all records failed")
}

// BroadcastToAllFollowersFailed creates an error indicating broadcasting to all followers failed.
func BroadcastToAllFollowersFailed() *errors.AppError {
	return errors.StreamingEventProcessingFailed("follower_broadcast", stdErrors.New("failed to broadcast to followers"))
}

// SendToAllConnectionsFailed creates an error indicating sending to all connections failed.
func SendToAllConnectionsFailed() *errors.AppError {
	return errors.StreamingEventProcessingFailed("connection_broadcast", stdErrors.New("failed to send to all connections"))
}

// Data validation errors

// NotificationMissingUsername creates an error indicating a notification is missing the recipient username.
func NotificationMissingUsername() *errors.AppError {
	return errors.RequiredFieldMissing("recipient_username")
}

// AccountMissingID creates an error indicating an account record is missing its ID field.
func AccountMissingID() *errors.AppError {
	return errors.RequiredFieldMissing("account_id")
}

// UsernameCannotBeEmpty creates an error indicating a username field cannot be empty.
func UsernameCannotBeEmpty() *errors.AppError {
	return errors.UsernameEmpty()
}

// CouldNotExtractUsername creates an error indicating username extraction from account ID failed.
func CouldNotExtractUsername() *errors.AppError {
	return errors.EventInvalid("account", "could not extract username from account ID")
}

// Event processing errors

// UnknownEventName creates an error indicating an unknown event name was encountered.
func UnknownEventName() *errors.AppError {
	return errors.EventInvalid("stream_event", "unknown event name")
}

// Stream subscription errors

// FailedToGetSubscriptionsForStream creates an error indicating retrieval of stream subscriptions failed.
func FailedToGetSubscriptionsForStream(err error) *errors.AppError {
	return errors.FailedToQuery("stream subscriptions", err)
}

// FailedToQueryConnection creates an error indicating querying connection details failed.
func FailedToQueryConnection(err error) *errors.AppError {
	return errors.FailedToQuery("connection", err)
}

// Marshaling errors

// FailedToMarshalStatus creates an error indicating status marshaling failed.
func FailedToMarshalStatus(err error) *errors.AppError {
	return errors.ObjectMarshalingFailed("status", err)
}

// FailedToMarshalNotification creates an error indicating notification marshaling failed.
func FailedToMarshalNotification(err error) *errors.AppError {
	return errors.ObjectMarshalingFailed("notification", err)
}

// FailedToMarshalAccount creates an error indicating account marshaling failed.
func FailedToMarshalAccount(err error) *errors.AppError {
	return errors.ObjectMarshalingFailed("account", err)
}

// FailedToMarshalMessage creates an error indicating message marshaling failed.
func FailedToMarshalMessage(err error) *errors.AppError {
	return errors.ObjectMarshalingFailed("message", err)
}

// Account payload errors

// FailedToCreateAccountPayload creates an error indicating account payload creation failed.
func FailedToCreateAccountPayload(err error) *errors.AppError {
	return errors.ProcessingFailed("account_payload_creation", err)
}

// Follower retrieval errors

// FailedToGetFollowers creates an error indicating follower retrieval failed.
func FailedToGetFollowers(err error) *errors.AppError {
	return errors.FailedToGet("followers", err)
}

// Repository errors

// FailedToGetSubscriptions creates an error indicating subscription retrieval failed.
func FailedToGetSubscriptions(err error) *errors.AppError {
	return errors.FailedToGet("subscriptions", err)
}

// Status extraction errors

// FailedToGetStatusForHashtagExtraction creates an error indicating status retrieval for hashtag extraction failed.
func FailedToGetStatusForHashtagExtraction(err error) *errors.AppError {
	return errors.FailedToGet("status for hashtag extraction", err)
}

// Hashtag processing errors

// HashtagProcessingFailed creates an error indicating hashtag processing failed.
func HashtagProcessingFailed(err error) *errors.AppError {
	return errors.ProcessingFailed("hashtag", err)
}
