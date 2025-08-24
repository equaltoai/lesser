package main

import "errors"

// API-specific error constants following Phase 2.3 error standardization pattern
var (
	ErrRepositoriesNotInitialized = errors.New("repositories not initialized")
)
