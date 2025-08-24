// Package main implements error constants for the outbox Lambda function.
package main

import "errors"

// Error constants for outbox processing
var (
	// Initialization errors
	ErrRepositoryStorageFromContext  = errors.New("failed to get repository storage from Lambda context")
	ErrFederationServiceFromContext  = errors.New("failed to get federation delivery service from Lambda context")
	ErrCostCalculatorFromContext     = errors.New("failed to get cost calculator from Lambda context")
	ErrOutboxProcessorInitialization = errors.New("failed to initialize outbox processor")

	// Message validation errors
	ErrMissingActivityInMessage = errors.New("missing activity in message")
	ErrMissingActorInMessage    = errors.New("missing actor in message")
	ErrMissingTargetInbox       = errors.New("missing target inbox in message")

	// Authentication errors
	ErrInvalidToken               = errors.New("invalid token")
	ErrUnexpectedJWTSigningMethod = errors.New("unexpected JWT signing method")

	// Federation errors
	ErrDeliveryBudgetLimitExceeded = errors.New("delivery blocked by budget limits")
	ErrDeliveryRetryableFailure    = errors.New("delivery failed with retryable error")

	// Processing errors
	ErrLambdaServicesInitialization   = errors.New("failed to initialize Lambda services")
	ErrInvalidMessageFormat           = errors.New("invalid message format")
	ErrFederationDeliveryStatusRecord = errors.New("failed to record federation delivery status")
	ErrJWTTokenParsing                = errors.New("failed to parse token")
)
