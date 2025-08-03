package monitoring

import (
	"fmt"
	"time"
)

// HealthCheckEvent represents an EventBridge event for triggering health checks
type HealthCheckEvent struct {
	Action     string                    `json:"action"`
	Components []ComponentCheckConfig    `json:"components"`
	Options    HealthCheckOptions        `json:"options,omitempty"`
}

// ComponentCheckConfig defines a component to check
type ComponentCheckConfig struct {
	Type       string `json:"type"`       // "dynamodb", "lambda", "sqs"
	Identifier string `json:"identifier"` // table name, function name, or queue URL
	Name       string `json:"name"`       // friendly name for the component
}

// HealthCheckOptions provides configuration for health checks
type HealthCheckOptions struct {
	StoreResults     bool   `json:"store_results"`      // whether to store results in DynamoDB
	PublishMetrics   bool   `json:"publish_metrics"`    // whether to publish CloudWatch metrics
	IncludeMetadata  bool   `json:"include_metadata"`   // whether to include detailed metadata
	TimeoutSeconds   int    `json:"timeout_seconds"`    // timeout for individual checks
	RetryAttempts    int    `json:"retry_attempts"`     // number of retry attempts for failed checks
}

// HealthCheckResponse represents the response from a health check Lambda
type HealthCheckResponse struct {
	RequestID        string                      `json:"request_id"`
	Timestamp        time.Time                   `json:"timestamp"`
	OverallStatus    HealthStatus                `json:"overall_status"`
	ComponentResults []ComponentHealthResult     `json:"component_results"`
	Summary          HealthCheckSummary          `json:"summary"`
	ExecutionTime    int64                       `json:"execution_time_ms"`
}

// ComponentHealthResult represents the result of checking a single component
type ComponentHealthResult struct {
	Component     string                 `json:"component"`
	Type          string                 `json:"type"`
	Status        HealthStatus           `json:"status"`
	CheckTime     time.Time              `json:"check_time"`
	LatencyMs     int64                  `json:"latency_ms"`
	Error         string                 `json:"error,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	RetryAttempts int                    `json:"retry_attempts"`
}

// HealthCheckSummary provides aggregated information about the health check
type HealthCheckSummary struct {
	TotalComponents   int `json:"total_components"`
	HealthyComponents int `json:"healthy_components"`
	WarningComponents int `json:"warning_components"`
	CriticalComponents int `json:"critical_components"`
	UnknownComponents int `json:"unknown_components"`
	FailedChecks      int `json:"failed_checks"`
}

// Predefined health check event configurations
var (
	// DefaultHealthCheckEvent provides a basic health check configuration
	DefaultHealthCheckEvent = HealthCheckEvent{
		Action: "check_health",
		Components: []ComponentCheckConfig{
			{Type: "dynamodb", Identifier: "lesser-main", Name: "Main Database"},
		},
		Options: HealthCheckOptions{
			StoreResults:    true,
			PublishMetrics:  true,
			IncludeMetadata: true,
			TimeoutSeconds:  30,
			RetryAttempts:   2,
		},
	}

	// ComprehensiveHealthCheckEvent checks all major components
	ComprehensiveHealthCheckEvent = HealthCheckEvent{
		Action: "check_health",
		Components: []ComponentCheckConfig{
			{Type: "dynamodb", Identifier: "lesser-main", Name: "Main Database"},
			{Type: "lambda", Identifier: "lesser-api", Name: "API Handler"},
			{Type: "lambda", Identifier: "lesser-processor-timeline", Name: "Timeline Processor"},
			{Type: "sqs", Identifier: "timeline-updates", Name: "Timeline Updates Queue"},
			{Type: "sqs", Identifier: "federation-delivery", Name: "Federation Delivery Queue"},
		},
		Options: HealthCheckOptions{
			StoreResults:    true,
			PublishMetrics:  true,
			IncludeMetadata: true,
			TimeoutSeconds:  45,
			RetryAttempts:   3,
		},
	}

	// QuickHealthCheckEvent provides minimal checks for frequent monitoring
	QuickHealthCheckEvent = HealthCheckEvent{
		Action: "check_health",
		Components: []ComponentCheckConfig{
			{Type: "dynamodb", Identifier: "lesser-main", Name: "Main Database"},
		},
		Options: HealthCheckOptions{
			StoreResults:    false,
			PublishMetrics:  true,
			IncludeMetadata: false,
			TimeoutSeconds:  15,
			RetryAttempts:   1,
		},
	}
)

// ValidateHealthCheckEvent validates that a health check event is properly formatted
func ValidateHealthCheckEvent(event HealthCheckEvent) error {
	if event.Action == "" {
		return ErrInvalidHealthCheckEvent("action is required")
	}

	if len(event.Components) == 0 {
		return ErrInvalidHealthCheckEvent("at least one component must be specified")
	}

	for i, component := range event.Components {
		if component.Type == "" {
			return ErrInvalidHealthCheckEvent(fmt.Sprintf("component type is required for component %d", i))
		}
		if component.Identifier == "" {
			return ErrInvalidHealthCheckEvent(fmt.Sprintf("component identifier is required for component %d", i))
		}
		if !isValidComponentType(component.Type) {
			return ErrInvalidHealthCheckEvent(fmt.Sprintf("invalid component type '%s' for component %d", component.Type, i))
		}
	}

	return nil
}

// isValidComponentType checks if a component type is supported
func isValidComponentType(componentType string) bool {
	validTypes := []string{"dynamodb", "lambda", "sqs"}
	for _, validType := range validTypes {
		if componentType == validType {
			return true
		}
	}
	return false
}

// ErrInvalidHealthCheckEvent represents an error with health check event validation
type ErrInvalidHealthCheckEvent string

func (e ErrInvalidHealthCheckEvent) Error() string {
	return string(e)
}