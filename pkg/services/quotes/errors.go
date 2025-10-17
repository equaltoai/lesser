// Package quotes provides error handling utilities for quote operations.
package quotes

import (
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
)

// Quote validation errors
var (
	ErrInvalidQuoteRequest     = pkgerrors.ValidationFailedWithField("quote request")
	ErrTargetStatusNotFound    = pkgerrors.NotFound("target status")
	ErrTargetStatusNotQuotable = pkgerrors.BusinessRuleViolated("target status not quotable", nil)
	ErrNotAuthorizedToQuote    = pkgerrors.InsufficientPermissions("quote status")
	ErrQuoteContentTooLong     = pkgerrors.ContentValidationFailed("quote content", "too long")
)

// Quote operation errors
var (
	ErrCreateQuoteStatus          = pkgerrors.FailedToCreate("quote status", nil)
	ErrCreateQuoteRelationship    = pkgerrors.FailedToCreate("quote relationship", nil)
	ErrQuoteRelationshipNotFound  = pkgerrors.NotFound("quote relationship")
	ErrNotAuthorizedToDeleteQuote = pkgerrors.InsufficientPermissions("delete quote")
	ErrWithdrawQuoteRelationship  = pkgerrors.FailedToDelete("quote relationship", nil)
)

// Quote permissions errors
var (
	ErrSaveQuotePermissions = pkgerrors.FailedToSave("quote permissions", nil)
)

// ErrGetTargetStatus returns an error when failing to retrieve the target status for a quote
func ErrGetTargetStatus(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToGet("target status", err)
}

// ErrCheckQuotePermissions returns an error when failing to check quote permissions
func ErrCheckQuotePermissions(err error) *pkgerrors.AppError {
	return pkgerrors.ProcessingFailed("quote permissions check", err)
}

// ErrGetQuoteRelationships returns an error when failing to retrieve quote relationships
func ErrGetQuoteRelationships(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToGet("quote relationships", err)
}

// ErrGetQuoteRelationship returns an error when failing to retrieve a single quote relationship
func ErrGetQuoteRelationship(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToGet("quote relationship", err)
}

// ErrGetQuotePermissions returns an error when failing to retrieve quote permissions
func ErrGetQuotePermissions(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToGet("quote permissions", err)
}

// ErrCheckExistingPermissions returns an error when failing to check existing quote permissions
func ErrCheckExistingPermissions(err error) *pkgerrors.AppError {
	return pkgerrors.ProcessingFailed("existing permissions check", err)
}
