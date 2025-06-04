package common

import (
	"fmt"
)

// Domain-specific error types

// ActorNotFoundError indicates an actor was not found
type ActorNotFoundError struct {
	Username string
}

func (e ActorNotFoundError) Error() string {
	return fmt.Sprintf("actor not found: %s", e.Username)
}

// ActivityNotFoundError indicates an activity was not found
type ActivityNotFoundError struct {
	ID string
}

func (e ActivityNotFoundError) Error() string {
	return fmt.Sprintf("activity not found: %s", e.ID)
}

// ValidationError indicates input validation failed
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

// AuthenticationError indicates authentication failed
type AuthenticationError struct {
	Message string
}

func (e AuthenticationError) Error() string {
	return fmt.Sprintf("authentication failed: %s", e.Message)
}

// AuthorizationError indicates authorization failed
type AuthorizationError struct {
	Action   string
	Resource string
}

func (e AuthorizationError) Error() string {
	return fmt.Sprintf("not authorized to %s %s", e.Action, e.Resource)
}

// ConflictError indicates a resource conflict
type ConflictError struct {
	Resource string
	Message  string
}

func (e ConflictError) Error() string {
	return fmt.Sprintf("conflict with %s: %s", e.Resource, e.Message)
}

// FederationError indicates a federation operation failed
type FederationError struct {
	Operation string
	Remote    string
	Err       error
}

func (e FederationError) Error() string {
	return fmt.Sprintf("federation %s failed for %s: %v", e.Operation, e.Remote, e.Err)
}

func (e FederationError) Unwrap() error {
	return e.Err
}

// Error checking functions

// IsNotFound returns true if the error is a not found error
func IsNotFound(err error) bool {
	switch err.(type) {
	case ActorNotFoundError, ActivityNotFoundError:
		return true
	}
	return false
}

// IsValidation returns true if the error is a validation error
func IsValidation(err error) bool {
	_, ok := err.(ValidationError)
	return ok
}

// IsAuthentication returns true if the error is an authentication error
func IsAuthentication(err error) bool {
	_, ok := err.(AuthenticationError)
	return ok
}

// IsAuthorization returns true if the error is an authorization error
func IsAuthorization(err error) bool {
	_, ok := err.(AuthorizationError)
	return ok
}

// IsConflict returns true if the error is a conflict error
func IsConflict(err error) bool {
	_, ok := err.(ConflictError)
	return ok
}

// IsFederation returns true if the error is a federation error
func IsFederation(err error) bool {
	_, ok := err.(FederationError)
	return ok
}
