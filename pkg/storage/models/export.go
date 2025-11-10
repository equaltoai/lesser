package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Export represents a data export request
type Export struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - export records use EXPORT#{export_id} pattern
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI1 for user queries - USER#{username}, CREATED#{timestamp}
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK" json:"gsi1_sk"`

	// Export metadata
	ID           string           `dynamorm:"attr:id" json:"id"`
	Username     string           `dynamorm:"attr:username" json:"username"`
	Type         string           `dynamorm:"attr:type" json:"type"`     // archive, followers, following, etc.
	Format       string           `dynamorm:"attr:format" json:"format"` // activitypub, mastodon, csv
	Status       string           `dynamorm:"attr:status" json:"status"` // pending, processing, completed, failed
	Options      map[string]any   `dynamorm:"attr:options" json:"options"`
	IncludeMedia bool             `dynamorm:"attr:includeMedia" json:"include_media"`
	DateRange    *ExportDateRange `dynamorm:"attr:dateRange" json:"date_range"`

	// Status tracking
	DownloadURL string     `dynamorm:"attr:downloadURL" json:"download_url,omitempty"`
	ExpiresAt   *time.Time `dynamorm:"attr:expiresAt" json:"expires_at,omitempty"`
	FileSize    int64      `dynamorm:"attr:fileSize" json:"file_size,omitempty"`
	RecordCount int64      `dynamorm:"attr:recordCount" json:"record_count,omitempty"`
	S3Key       string     `dynamorm:"attr:s3Key" json:"s3_key,omitempty"`
	Error       string     `dynamorm:"attr:error" json:"error,omitempty"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt   time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`
	CompletedAt *time.Time `dynamorm:"attr:completedAt" json:"completed_at,omitempty"`
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

// TableName returns the DynamoDB table backing Export.
func (e *Export) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing ExportDateRange.
func (ExportDateRange) TableName() string {
	return MainTableName
}
