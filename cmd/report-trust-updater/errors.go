package main

import "errors"

// Error constants for report-trust-updater service
var (
	// Stream processing errors
	ErrMissingKeys = errors.New("missing keys")
	
	// Report processing errors
	ErrReportRetrieval = errors.New("failed to get report")
)