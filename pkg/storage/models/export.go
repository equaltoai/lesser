package models

import (
	"fmt"
	"time"
)

// Export represents a data export request
type Export struct {
	// Primary keys - export records use EXPORT#{export_id} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`
	
	// Export metadata
	ID           string            `json:"id"`
	Username     string            `json:"username"`
	Type         string            `json:"type"`         // archive, followers, following, etc.
	Format       string            `json:"format"`       // activitypub, mastodon, csv
	Status       string            `json:"status"`       // pending, processing, completed, failed
	Options      map[string]any    `json:"options"`
	IncludeMedia bool             `json:"include_media"`
	DateRange    *ExportDateRange `json:"date_range"`
	
	// Status tracking
	DownloadURL  string     `json:"download_url,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
	RecordCount  int64      `json:"record_count,omitempty"`
	S3Key        string     `json:"s3_key,omitempty"`
	Error        string     `json:"error,omitempty"`
	
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

// UpdateKeys sets the primary keys for the Export model
func (e *Export) UpdateKeys() {
	e.PK = fmt.Sprintf("EXPORT#%s", e.ID)
	e.SK = fmt.Sprintf("EXPORT#%s", e.ID)
}

// TableName returns the DynamoDB table name
func (e *Export) TableName() string {
	return "" // Will be set by the repository
}