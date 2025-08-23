package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Export represents a data export request
type Export struct {
	// Primary keys - export records use EXPORT#{export_id} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1 for user queries - USER#{username}, CREATED#{timestamp}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`

	// Export metadata
	ID           string           `json:"id"`
	Username     string           `json:"username"`
	Type         string           `json:"type"`   // archive, followers, following, etc.
	Format       string           `json:"format"` // activitypub, mastodon, csv
	Status       string           `json:"status"` // pending, processing, completed, failed
	Options      map[string]any   `json:"options"`
	IncludeMedia bool             `json:"include_media"`
	DateRange    *ExportDateRange `json:"date_range"`

	// Status tracking
	DownloadURL string     `json:"download_url,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	FileSize    int64      `json:"file_size,omitempty"`
	RecordCount int64      `json:"record_count,omitempty"`
	S3Key       string     `json:"s3_key,omitempty"`
	Error       string     `json:"error,omitempty"`

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ExportDateRange for filtering exports
type ExportDateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// NewExportDateRangeFromStrings creates a date range from ISO date strings
func NewExportDateRangeFromStrings(start, end string) (*ExportDateRange, error) {
	if start == "" || end == "" {
		return nil, nil
	}

	startTime, err := time.Parse(common.DateFormat, start)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrExportInvalidStartDate, err)
	}

	endTime, err := time.Parse(common.DateFormat, end)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrExportInvalidEndDate, err)
	}

	return &ExportDateRange{
		Start: startTime,
		End:   endTime,
	}, nil
}

// UpdateKeys sets the primary keys for the Export model
func (e *Export) UpdateKeys() {
	e.PK = fmt.Sprintf("EXPORT#%s", e.ID)
	e.SK = fmt.Sprintf("EXPORT#%s", e.ID)
	e.GSI1PK = fmt.Sprintf(KeyPatternUser, e.Username)
	e.GSI1SK = fmt.Sprintf("CREATED#%s", e.CreatedAt.Format(time.RFC3339))
}

// GetStatus returns the status of the export
func (e *Export) GetStatus() string {
	return e.Status
}

// GetCreatedAt returns the creation timestamp of the export
func (e *Export) GetCreatedAt() time.Time {
	return e.CreatedAt
}

// TableName returns the DynamoDB table name
func (e *Export) TableName() string {
	return "" // Will be set by the repository
}
