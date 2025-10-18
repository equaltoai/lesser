// Package main implements error handlers for the outbox Lambda function.
package main

import "github.com/equaltoai/lesser/pkg/errors"

// Initialization errors - using Lambda domain functions
// repositoryStorageFromContextFailed creates an error when repository storage cannot be retrieved from Lambda context.
func repositoryStorageFromContextFailed() *errors.AppError {
	return errors.ServiceInitializationFailed("repository storage", nil)
}

// federationServiceFromContextFailed creates an error when federation delivery service cannot be retrieved from Lambda context.
func federationServiceFromContextFailed() *errors.AppError {
	return errors.ServiceInitializationFailed("federation delivery service", nil)
}

// costCalculatorFromContextFailed creates an error when cost calculator cannot be retrieved from Lambda context.
func costCalculatorFromContextFailed() *errors.AppError {
	return errors.ServiceInitializationFailed("cost calculator", nil)
}

// outboxProcessorInitializationFailed creates an error when outbox processor initialization fails.
func outboxProcessorInitializationFailed() *errors.AppError {
	return errors.ServiceInitializationFailed("outbox processor", nil)
}

// Message validation errors - using Lambda domain functions
// missingActivityInMessage creates an error when a message is missing the activity field.
func missingActivityInMessage() *errors.AppError {
	return errors.EventMissingField("message", "activity")
}

// missingActorInMessage creates an error when a message is missing the actor field.
func missingActorInMessage() *errors.AppError {
	return errors.EventMissingField("message", "actor")
}

// missingTargetInbox creates an error when a message is missing the target inbox field.
func missingTargetInbox() *errors.AppError {
	return errors.EventMissingField("message", "target_inbox")
}

// Authentication errors - using Auth domain functions
// invalidToken creates an error for invalid authentication tokens.
func invalidToken() *errors.AppError {
	return errors.TokenInvalid("invalid token format")
}

// unexpectedJWTSigningMethod creates an error for unexpected JWT signing methods.
func unexpectedJWTSigningMethod() *errors.AppError {
	return errors.TokenInvalid("unexpected JWT signing method")
}

// Federation errors - using Federation domain functions
// deliveryBudgetLimitExceeded creates an error when delivery is blocked by budget limits.
func deliveryBudgetLimitExceeded() *errors.AppError {
	return errors.QuotaExceeded("delivery budget", 0)
}

// deliveryRetryableFailure creates an error for retryable delivery failures.
func deliveryRetryableFailure(err error) *errors.AppError {
	return errors.DeliveryFailed("retryable delivery failure", err)
}

// Processing errors - using Lambda domain functions
// lambdaServicesInitializationFailed creates an error when Lambda services initialization fails.
func lambdaServicesInitializationFailed() *errors.AppError {
	return errors.ServiceInitializationFailed("Lambda services", nil)
}

// invalidMessageFormat creates an error for invalid message formats.
func invalidMessageFormat() *errors.AppError {
	return errors.EventInvalid("message", "invalid message format")
}

// federationDeliveryStatusRecordFailed creates an error when recording federation delivery status fails.
func federationDeliveryStatusRecordFailed() *errors.AppError {
	return errors.WorkflowStepFailed("federation delivery status record", nil)
}

// jwtTokenParsingFailed creates an error when JWT token parsing fails.
func jwtTokenParsingFailed(_ error) *errors.AppError {
	return errors.TokenInvalid("failed to parse JWT token")
}
