package model

import "errors"

// GraphQL scalar validation errors
var (
	// ErrTimeNotString is returned when Time scalar receives non-string input
	ErrTimeNotString = errors.New("time must be a string")
	
	// ErrCursorNotString is returned when Cursor scalar receives non-string input
	ErrCursorNotString = errors.New("cursor must be a string")
)

// GraphQL enum validation errors
var (
	// ErrEnumNotString is returned when an enum receives non-string input
	ErrEnumNotString = errors.New("enums must be strings")
	
	// ErrInvalidEnumValue is returned when an enum value is not valid
	ErrInvalidEnumValue = errors.New("invalid enum value")
)

// GraphQL duration validation errors
var (
	// ErrInvalidDurationType is returned when Duration scalar receives unsupported type
	ErrInvalidDurationType = errors.New("duration must be an integer (seconds) or a duration string")
)