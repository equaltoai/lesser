package main

import "errors"

// Error constants for the federation-delivery Lambda function
var (
	// Message processing errors
	ErrInvalidMessageBody    = errors.New("invalid message body format")
	ErrMessageMarshalFailure = errors.New("failed to marshal message for requeue")
	ErrMessageRequeueFailure = errors.New("failed to requeue message")

	// Actor and authentication errors
	ErrSigningActorMissing = errors.New("signing actor not found")

	// Delivery processing errors
	ErrDeliveryMaxAttemptsExceeded = errors.New("delivery failed after maximum attempts")
)
