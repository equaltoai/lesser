package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Export represents a data export request
type Export struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - export records use EXPORT#{export_id} pattern
	PK string `theorydb:"pk,attr:PK" json:"pk"`
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	// GSI1 for user queries - USER#{username}, CREATED#{timestamp}
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk"`

	// Export metadata
	ID           string           `theorydb:"attr:id" json:"id"`
	Username     string           `theorydb:"attr:username" json:"username"`
	Type         string           `theorydb:"attr:type" json:"type"`     // archive, followers, following, etc.
	Format       string           `theorydb:"attr:format" json:"format"` // activitypub, mastodon, csv
	Status       string           `theorydb:"attr:status" json:"status"` // pending, processing, completed, failed
	Options      map[string]any   `theorydb:"attr:options" json:"options"`
	IncludeMedia bool             `theorydb:"attr:includeMedia" json:"include_media"`
	DateRange    *ExportDateRange `theorydb:"attr:dateRange" json:"date_range"`

	// Status tracking
	DownloadURL string     `theorydb:"attr:downloadURL" json:"download_url,omitempty"`
	ExpiresAt   *time.Time `theorydb:"attr:expiresAt" json:"expires_at,omitempty"`
	FileSize    int64      `theorydb:"attr:fileSize" json:"file_size,omitempty"`
	RecordCount int64      `theorydb:"attr:recordCount" json:"record_count,omitempty"`
	S3Key       string     `theorydb:"attr:s3Key" json:"s3_key,omitempty"`
	Error       string     `theorydb:"attr:error" json:"error,omitempty"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt   time.Time  `theorydb:"attr:updatedAt" json:"updated_at"`
	CompletedAt *time.Time `theorydb:"attr:completedAt" json:"completed_at,omitempty"`
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
