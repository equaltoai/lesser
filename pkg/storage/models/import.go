package models

import (
	"fmt"
	"time"
)

// Import represents a data import request
type Import struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - import records use IMPORT#{import_id} pattern
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI1 for user queries - USER#{username}, CREATED#{timestamp}
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"`

	// Import metadata
	ID       string `dynamorm:"attr:id" json:"id"`
	Username string `dynamorm:"attr:username" json:"username"`
	Type     string `dynamorm:"attr:type" json:"type"`     // followers, following, blocks, mutes, lists, bookmarks, archive
	Mode     string `dynamorm:"attr:mode" json:"mode"`     // merge, overwrite
	Status   string `dynamorm:"attr:status" json:"status"` // pending, processing, completed, failed
	S3Key    string `dynamorm:"attr:s3Key" json:"s3_key"`  // Location of import file

	// Progress tracking
	Total        int      `dynamorm:"attr:total" json:"total"`
	Progress     int      `dynamorm:"attr:progress" json:"progress"`
	SuccessCount int      `dynamorm:"attr:successCount" json:"success_count"`
	SkipCount    int      `dynamorm:"attr:skipCount" json:"skip_count"`
	ErrorCount   int      `dynamorm:"attr:errorCount" json:"error_count"`
	Errors       []string `dynamorm:"attr:errors" json:"errors,omitempty"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt   time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`
	CompletedAt *time.Time `dynamorm:"attr:completedAt" json:"completed_at,omitempty"`

	// Error information
	Error string `dynamorm:"attr:error" json:"error,omitempty"`
}

// UpdateKeys sets the primary keys for the Import model
func (i *Import) UpdateKeys() {
	i.PK = fmt.Sprintf("IMPORT#%s", i.ID)
	i.SK = fmt.Sprintf("IMPORT#%s", i.ID)
	i.GSI1PK = fmt.Sprintf(KeyPatternUser, i.Username)
	i.GSI1SK = fmt.Sprintf("CREATED#%s", i.CreatedAt.Format(time.RFC3339))
}

// GetStatus returns the status of the import
func (i *Import) GetStatus() string {
	return i.Status
}

// GetCreatedAt returns the creation timestamp of the import
func (i *Import) GetCreatedAt() time.Time {
	return i.CreatedAt
}

// TableName returns the DynamoDB table backing Import.
func (i *Import) TableName() string {
	return MainTableName
}
