// Package health defines event types for federation health monitoring and EventBridge integration.
package health

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// HealthCheckEvent represents an EventBridge event for triggering health checks
//
//nolint:revive // HealthCheck prefix clarifies this is for health monitoring
type HealthCheckEvent struct {
	// Event metadata
	Source     string    `json:"source"`      // "lesser.federation.health"
	DetailType string    `json:"detail-type"` // "Health Check Request"
	Time       time.Time `json:"time"`
	Region     string    `json:"region"`
	Account    string    `json:"account"`

	// Event detail
	Detail HealthCheckDetail `json:"detail"`
}

// HealthCheckDetail contains the specifics of what to check
//
//nolint:revive // HealthCheck prefix clarifies this is for health monitoring
type HealthCheckDetail struct {
	// Action type
	Action string `json:"action"` // "check_health", "aggregate_summary", "cleanup"

	// Instance configuration
	InstanceIDs []string `json:"instance_ids,omitempty"` // Specific instances to check
	Domains     []string `json:"domains,omitempty"`      // Domains to check
	BatchSize   int      `json:"batch_size,omitempty"`   // Number of instances per batch

	// Time window configuration
	WindowHours int `json:"window_hours,omitempty"` // Hours to look back for aggregation

	// Check configuration
	Timeout         int    `json:"timeout,omitempty"`          // Request timeout in seconds
	FollowRedirects bool   `json:"follow_redirects,omitempty"` // Whether to follow HTTP redirects
	UserAgent       string `json:"user_agent,omitempty"`       // Custom user agent

	// Aggregation configuration
	SummaryWindows []string `json:"summary_windows,omitempty"` // ["1h", "24h", "7d"]

	// Cleanup configuration
	RetentionDays int `json:"retention_days,omitempty"` // Days to keep health data
}

// HealthCheckResult represents the result of health checking operation
//
//nolint:revive // HealthCheck prefix clarifies this is for health monitoring
type HealthCheckResult struct {
	// Event metadata
	EventID   string    `json:"event_id"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`

	// Results
	CheckedDomains   []string                  `json:"checked_domains"`
	SuccessfulChecks int                       `json:"successful_checks"`
	FailedChecks     int                       `json:"failed_checks"`
	Results          []DomainHealthCheckResult `json:"results"`

	// Performance metrics
	TotalDuration time.Duration `json:"total_duration"`
	AvgDuration   time.Duration `json:"avg_duration"`

	// Errors
	Errors []HealthCheckError `json:"errors,omitempty"`
}

// DomainHealthCheckResult represents health check result for a single domain
type DomainHealthCheckResult struct {
	Domain       string        `json:"domain"`
	Success      bool          `json:"success"`
	Reachable    bool          `json:"reachable"`
	StatusCode   int           `json:"status_code"`
	ResponseTime time.Duration `json:"response_time"`
	ErrorMessage string        `json:"error_message,omitempty"`
	HealthScore  float64       `json:"health_score"`
	CheckedAt    time.Time     `json:"checked_at"`

	// Federation-specific metrics
	InboxBacklog    int           `json:"inbox_backlog,omitempty"`
	ProcessingDelay time.Duration `json:"processing_delay,omitempty"`
}

// HealthCheckError represents an error during health checking
//
//nolint:revive // HealthCheck prefix clarifies this is for health monitoring
type HealthCheckError struct {
	Domain       string    `json:"domain,omitempty"`
	ErrorType    string    `json:"error_type"` // "network", "timeout", "invalid_response", "database"
	ErrorMessage string    `json:"error_message"`
	Timestamp    time.Time `json:"timestamp"`
}

// AggregationEvent represents an EventBridge event for summary aggregation
type AggregationEvent struct {
	Source     string            `json:"source"`
	DetailType string            `json:"detail-type"`
	Time       time.Time         `json:"time"`
	Detail     AggregationDetail `json:"detail"`
}

// AggregationDetail contains aggregation configuration
type AggregationDetail struct {
	Action           string   `json:"action"`                      // "aggregate_summaries"
	Domains          []string `json:"domains"`                     // Domains to aggregate
	Windows          []string `json:"windows"`                     // Time windows: ["1h", "24h", "7d"]
	ForceRecalculate bool     `json:"force_recalculate,omitempty"` // Recalculate even if recent summary exists
}

// ScheduledHealthCheckEvent represents a scheduled health check trigger
type ScheduledHealthCheckEvent struct {
	// Standard EventBridge scheduled event fields
	Source     string    `json:"source"`      // "aws.events"
	DetailType string    `json:"detail-type"` // "Scheduled Event"
	Time       time.Time `json:"time"`
	Account    string    `json:"account"`
	Region     string    `json:"region"`

	// Custom detail for health checking
	Detail ScheduledEventDetail `json:"detail"`
}

// ScheduledEventDetail contains configuration for scheduled health checks
type ScheduledEventDetail struct {
	// Schedule information
	ScheduleName string `json:"schedule_name"` // Name of the EventBridge rule
	ScheduleType string `json:"schedule_type"` // "health_check", "aggregation", "cleanup"

	// Configuration
	BatchSize    int      `json:"batch_size,omitempty"`    // Instances per invocation
	MaxInstances int      `json:"max_instances,omitempty"` // Total instances to check
	Windows      []string `json:"windows,omitempty"`       // Time windows for aggregation
}

// NewHealthCheckEvent creates a new health check event
func NewHealthCheckEvent(domains []string, batchSize int) *HealthCheckEvent {
	return &HealthCheckEvent{
		Source:     "lesser.federation.health",
		DetailType: "Health Check Request",
		Time:       time.Now().UTC(),
		Detail: HealthCheckDetail{
			Action:    "check_health",
			Domains:   domains,
			BatchSize: batchSize,
			Timeout:   30, // 30 second default timeout
			UserAgent: "Lesser/1.0 (Federation Health Check)",
		},
	}
}

// NewAggregationEvent creates a new aggregation event
func NewAggregationEvent(domains []string, windows []string) *AggregationEvent {
	return &AggregationEvent{
		Source:     "lesser.federation.health",
		DetailType: "Health Summary Aggregation",
		Time:       time.Now().UTC(),
		Detail: AggregationDetail{
			Action:  "aggregate_summaries",
			Domains: domains,
			Windows: windows,
		},
	}
}

// ToJSON converts the event to JSON bytes
func (e *HealthCheckEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FromJSON parses JSON bytes into a HealthCheckEvent
func (e *HealthCheckEvent) FromJSON(data []byte) error {
	return json.Unmarshal(data, e)
}

// ToJSON converts the aggregation event to JSON bytes
func (e *AggregationEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FromJSON parses JSON bytes into an AggregationEvent
func (e *AggregationEvent) FromJSON(data []byte) error {
	return json.Unmarshal(data, e)
}

// Validate checks if the health check event is valid
func (e *HealthCheckEvent) Validate() error {
	if err := common.ValidateRequiredParam("e.Detail.Action", e.Detail.Action); err != nil {
		return ErrActionRequired
	}

	if e.Detail.Action == "check_health" && len(e.Detail.Domains) == 0 && len(e.Detail.InstanceIDs) == 0 {
		return ErrDomainsOrInstanceIDsRequired
	}

	if e.Detail.BatchSize < 0 {
		return ErrBatchSizeMustBePositive
	}

	if e.Detail.Timeout < 0 {
		return ErrTimeoutMustBePositive
	}

	return nil
}

// Validate checks if the aggregation event is valid
func (e *AggregationEvent) Validate() error {
	if err := common.ValidateRequiredParam("e.Detail.Action", e.Detail.Action); err != nil {
		return ErrActionRequired
	}

	if err := common.ValidateSliceNotEmpty("e.Detail.Domains", e.Detail.Domains); err != nil {
		return ErrDomainsRequiredForAggregation
	}

	if err := common.ValidateSliceNotEmpty("e.Detail.Windows", e.Detail.Windows); err != nil {
		return ErrWindowsRequiredForAggregation
	}

	// Validate window formats
	validWindows := map[string]bool{
		"1h": true, "24h": true, "7d": true,
	}

	for _, window := range e.Detail.Windows {
		if !validWindows[window] {
			return errors.Join(ErrInvalidWindowFormat,
				errors.New(window+" (must be 1h, 24h, or 7d)"))
		}
	}

	return nil
}

// GetBatchedDomains splits domains into batches based on batch size
func (e *HealthCheckEvent) GetBatchedDomains() [][]string {
	domains := e.Detail.Domains
	batchSize := e.Detail.BatchSize

	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}

	if err := common.ValidateSliceNotEmpty("domains", domains); err != nil {
		return [][]string{}
	}

	var batches [][]string
	for i := 0; i < len(domains); i += batchSize {
		end := i + batchSize
		if end > len(domains) {
			end = len(domains)
		}
		batches = append(batches, domains[i:end])
	}

	return batches
}
