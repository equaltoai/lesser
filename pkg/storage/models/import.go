package models

import (
	"fmt"
	"time"
)

// Import represents a data import request
type Import struct {
	// Primary keys - import records use IMPORT#{import_id} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`
	
	// Import metadata
	ID       string         `json:"id"`
	Username string         `json:"username"`
	Type     string         `json:"type"`   // followers, following, blocks, mutes, lists, bookmarks, archive
	Mode     string         `json:"mode"`   // merge, overwrite
	Status   string         `json:"status"` // pending, processing, completed, failed
	S3Key    string         `json:"s3_key"` // Location of import file
	
	// Progress tracking
	Total        int      `json:"total"`
	Progress     int      `json:"progress"`
	SuccessCount int      `json:"success_count"`
	SkipCount    int      `json:"skip_count"`
	ErrorCount   int      `json:"error_count"`
	Errors       []string `json:"errors,omitempty"`
	
	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	
	// Error information
	Error string `json:"error,omitempty"`
}

// UpdateKeys sets the primary keys for the Import model
func (i *Import) UpdateKeys() {
	i.PK = fmt.Sprintf("IMPORT#%s", i.ID)
	i.SK = fmt.Sprintf("IMPORT#%s", i.ID)
}

// TableName returns the DynamoDB table name
func (i *Import) TableName() string {
	return "" // Will be set by the repository
}