package main

import "errors"

// Metrics processor-specific error constants following Phase 2.3 error standardization pattern
var (
	ErrStoreMetricRecord = errors.New("failed to store metric record")
)
