package model

import "github.com/equaltoai/lesser/pkg/errors"

// GraphQL scalar validation errors
var (
	// ErrTimeNotString is returned when Time scalar receives non-string input
	ErrTimeNotString = errors.NewValidationError("time", "must be a string")

	// ErrCursorNotString is returned when Cursor scalar receives non-string input
	ErrCursorNotString = errors.NewValidationError("cursor", "must be a string")
)

// GraphQL enum validation errors
var (
	// ErrEnumNotString is returned when an enum receives non-string input
	ErrEnumNotString = errors.NewValidationError("enum", "must be strings")

	// ErrInvalidEnumValue is returned when an enum value is not valid
	ErrInvalidEnumValue = errors.NewValidationError("enum", "invalid value")
)

// GraphQL duration validation errors
var (
	// ErrInvalidDurationType is returned when Duration scalar receives unsupported type
	ErrInvalidDurationType = errors.NewValidationError("duration", "must be an integer (seconds) or a duration string")
)
