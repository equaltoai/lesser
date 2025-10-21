// Package observability defines interfaces and common types for structured
// logging and metrics used across Lift. Concrete implementations (e.g.,
// CloudWatch logging and metrics) live under subpackages and integrate with the
// Lift Context and middleware for consistent telemetry in Lambda.
package observability
