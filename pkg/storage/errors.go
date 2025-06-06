package storage

import "errors"

// Common storage errors
var (
	// ErrNotFound is returned when a requested item doesn't exist
	ErrNotFound = errors.New("item not found")

	// ErrAlreadyExists is returned when trying to create an item that already exists
	ErrAlreadyExists = errors.New("item already exists")

	// ErrInvalidInput is returned when input validation fails
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthorized is returned when an operation is not authorized
	ErrUnauthorized = errors.New("unauthorized")
)
