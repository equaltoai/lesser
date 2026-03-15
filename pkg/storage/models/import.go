package models

import (
	"fmt"
	"time"
)

// Import represents a data import request
type Import struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - import records use IMPORT#{import_id} pattern
	PK string `theorydb:"pk,attr:PK" json:"pk"`
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	// GSI1 for user queries - USER#{username}, CREATED#{timestamp}
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk"`

	// Import metadata
	ID       string `theorydb:"attr:id" json:"id"`
	Username string `theorydb:"attr:username" json:"username"`
	Type     string `theorydb:"attr:type" json:"type"`     // followers, following, blocks, mutes, lists, bookmarks, archive
	Mode     string `theorydb:"attr:mode" json:"mode"`     // merge, overwrite
	Status   string `theorydb:"attr:status" json:"status"` // pending, processing, completed, failed
	S3Key    string `theorydb:"attr:s3Key" json:"s3_key"`  // Location of import file

	// Progress tracking
	Total        int      `theorydb:"attr:total" json:"total"`
	Progress     int      `theorydb:"attr:progress" json:"progress"`
	SuccessCount int      `theorydb:"attr:successCount" json:"success_count"`
	SkipCount    int      `theorydb:"attr:skipCount" json:"skip_count"`
	ErrorCount   int      `theorydb:"attr:errorCount" json:"error_count"`
	Errors       []string `theorydb:"attr:errors" json:"errors,omitempty"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt   time.Time  `theorydb:"attr:updatedAt" json:"updated_at"`
	CompletedAt *time.Time `theorydb:"attr:completedAt" json:"completed_at,omitempty"`

	// Error information
	Error string `theorydb:"attr:error" json:"error,omitempty"`
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
