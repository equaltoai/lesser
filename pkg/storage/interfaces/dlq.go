// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// DLQTimeRange represents a time range for DLQ analytics queries.
type DLQTimeRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// DLQAnalytics represents analytics data for DLQ messages.
type DLQAnalytics struct {
	Service               string                         `json:"service"`
	TimeRange             DLQTimeRange                   `json:"time_range"`
	TotalMessages         int                            `json:"total_messages"`
	NewMessages           int                            `json:"new_messages"`
	ReprocessingMessages  int                            `json:"reprocessing_messages"`
	ResolvedMessages      int                            `json:"resolved_messages"`
	FailedMessages        int                            `json:"failed_messages"`
	AbandonedMessages     int                            `json:"abandoned_messages"`
	ResolutionRate        float64                        `json:"resolution_rate"`
	AbandonmentRate       float64                        `json:"abandonment_rate"`
	TotalCostMicroCents   int64                          `json:"total_cost_micro_cents"`
	TotalCostDollars      float64                        `json:"total_cost_dollars"`
	AverageCostPerMessage float64                        `json:"average_cost_per_message"`
	ErrorTypeStats        map[string]*DLQErrorTypeStats  `json:"error_type_stats"`
	ServiceStats          map[string]*DLQServiceStats    `json:"service_stats"`
	SimilarityGroups      map[string]*DLQSimilarityGroup `json:"similarity_groups"`
}

// DLQErrorTypeStats represents statistics for a specific error type.
type DLQErrorTypeStats struct {
	ErrorType           string  `json:"error_type"`
	Count               int     `json:"count"`
	ResolvedCount       int     `json:"resolved_count"`
	ResolutionRate      float64 `json:"resolution_rate"`
	TotalCostMicroCents int64   `json:"total_cost_micro_cents"`
}

// DLQServiceStats represents statistics for a specific service.
type DLQServiceStats struct {
	Service             string  `json:"service"`
	MessageCount        int     `json:"message_count"`
	ErrorTypes          int     `json:"error_types"`
	ResolutionRate      float64 `json:"resolution_rate"`
	TotalCostMicroCents int64   `json:"total_cost_micro_cents"`
}

// DLQSimilarityGroup represents a group of similar error messages.
type DLQSimilarityGroup struct {
	SimilarityHash string    `json:"similarity_hash"`
	ErrorType      string    `json:"error_type"`
	Service        string    `json:"service"`
	MessageCount   int       `json:"message_count"`
	MessageIDs     []string  `json:"message_ids"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
	SampleError    string    `json:"sample_error"`
}

// DLQTrends represents trend data over time.
type DLQTrends struct {
	Service    string                    `json:"service"`
	Days       int                       `json:"days"`
	DailyStats map[string]*DLQDailyStats `json:"daily_stats"`
}

// DLQDailyStats represents statistics for a single day.
type DLQDailyStats struct {
	Date                time.Time      `json:"date"`
	MessageCount        int            `json:"message_count"`
	TotalCostMicroCents int64          `json:"total_cost_micro_cents"`
	TotalCostDollars    float64        `json:"total_cost_dollars"`
	ErrorTypes          map[string]int `json:"error_types"`
	StatusCounts        map[string]int `json:"status_counts"`
}

// DLQSearchFilter represents search criteria for DLQ messages.
type DLQSearchFilter struct {
	Service     string    `json:"service"`
	ErrorType   string    `json:"error_type,omitempty"`
	Status      string    `json:"status,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	IsPermanent *bool     `json:"is_permanent,omitempty"`
	StartTime   time.Time `json:"start_time,omitempty"`
	EndTime     time.Time `json:"end_time,omitempty"`
	SearchText  string    `json:"search_text,omitempty"`
	Limit       int       `json:"limit,omitempty"`
	Cursor      string    `json:"cursor,omitempty"`
}

// DLQHealthStatus represents health metrics for DLQ monitoring.
type DLQHealthStatus struct {
	Service           string         `json:"service"`
	CheckTime         time.Time      `json:"check_time"`
	TotalMessages     int            `json:"total_messages"`
	NewMessages       int            `json:"new_messages"`
	ReprocessingCount int            `json:"reprocessing_count"`
	AbandonedCount    int            `json:"abandoned_count"`
	ErrorRates        map[string]int `json:"error_rates"`
	AverageRetryCount float64        `json:"average_retry_count"`
	IsHealthy         bool           `json:"is_healthy"`
	Alerts            []string       `json:"alerts"`
}

// DLQRepository defines the interface for dead letter queue operations.
// This handles failed message storage, retry logic, analytics, and health monitoring.
type DLQRepository interface {
	// ===== Core DLQ Operations =====

	// CreateDLQMessage creates a new DLQ message
	CreateDLQMessage(ctx context.Context, message *models.DLQMessage) error

	// GetDLQMessage retrieves a DLQ message by ID
	GetDLQMessage(ctx context.Context, id string) (*models.DLQMessage, error)

	// UpdateDLQMessage updates an existing DLQ message
	UpdateDLQMessage(ctx context.Context, message *models.DLQMessage) error

	// DeleteDLQMessage deletes a DLQ message
	DeleteDLQMessage(ctx context.Context, message *models.DLQMessage) error

	// BatchUpdateDLQMessages updates multiple DLQ messages
	BatchUpdateDLQMessages(ctx context.Context, messages []*models.DLQMessage) error

	// ===== Query Operations =====

	// GetDLQMessagesByService retrieves DLQ messages for a specific service with pagination
	GetDLQMessagesByService(ctx context.Context, service string, date time.Time, limit int, cursor string) ([]*models.DLQMessage, string, error)

	// GetDLQMessagesByServiceDateRange retrieves DLQ messages for a service across multiple dates
	GetDLQMessagesByServiceDateRange(ctx context.Context, service string, startDate, endDate time.Time, limit int) ([]*models.DLQMessage, error)

	// GetDLQMessagesByErrorType retrieves DLQ messages by error type with pagination
	GetDLQMessagesByErrorType(ctx context.Context, errorType string, limit int, cursor string) ([]*models.DLQMessage, string, error)

	// GetDLQMessagesForReprocessing retrieves messages that can be reprocessed
	GetDLQMessagesForReprocessing(ctx context.Context, service string, status string, limit int, cursor string) ([]*models.DLQMessage, string, error)

	// GetDLQMessagesByStatus retrieves messages by status
	GetDLQMessagesByStatus(ctx context.Context, service, status string, limit int, cursor string) ([]*models.DLQMessage, string, error)

	// SearchDLQMessages searches DLQ messages with various filters
	SearchDLQMessages(ctx context.Context, filter *DLQSearchFilter) ([]*models.DLQMessage, string, error)

	// GetSimilarMessages finds messages with the same similarity hash
	GetSimilarMessages(ctx context.Context, similarityHash string, limit int) ([]*models.DLQMessage, error)

	// ===== Analytics Operations =====

	// GetDLQAnalytics returns analytics data for DLQ messages
	GetDLQAnalytics(ctx context.Context, service string, timeRange DLQTimeRange) (*DLQAnalytics, error)

	// GetDLQTrends returns trend data for DLQ messages over time
	GetDLQTrends(ctx context.Context, service string, days int) (*DLQTrends, error)

	// AnalyzeFailurePatterns analyzes DLQ messages to identify common failure patterns
	AnalyzeFailurePatterns(ctx context.Context, service string, days int) (map[string]*DLQSimilarityGroup, error)

	// ===== Retry Operations =====

	// SendToDeadLetterQueue creates and stores a DLQ message with proper error categorization
	SendToDeadLetterQueue(ctx context.Context, service, messageID, messageBody, errorType, errorMessage string, isPermanent bool) error

	// RetryFailedMessage attempts to reprocess a DLQ message with exponential backoff
	RetryFailedMessage(ctx context.Context, messageID string) error

	// GetRetryableMessages returns messages that are ready for retry based on backoff schedule
	GetRetryableMessages(ctx context.Context, service string, limit int) ([]*models.DLQMessage, error)

	// ===== Cleanup Operations =====

	// CleanupExpiredMessages deletes expired DLQ messages
	CleanupExpiredMessages(ctx context.Context, before time.Time) (int, error)

	// ===== Health Monitoring =====

	// MonitorDLQHealth provides health metrics for DLQ monitoring and alerting
	MonitorDLQHealth(ctx context.Context, service string) (*DLQHealthStatus, error)
}
