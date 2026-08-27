package models

import (
	"fmt"
	"time"
)

// ScheduledStatus represents a scheduled status post in DynamoDB
type ScheduledStatus struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK"` // USER#{username}#SCHEDULED
	SK string `theorydb:"sk,attr:SK"` // ID#{id}

	// GSI1 keys for time-based queries (due status queries)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"` // SCHEDULED#DUE
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"` // TIME#{scheduled_at_RFC3339Nano}#ID#{id}

	// GSI2 keys for ID lookups without knowing the owning user
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"` // SCHEDULED_ID#{id}
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"` // USER#{username}#SCHEDULED

	// Business fields - embedded from storage.ScheduledStatus
	ID            string         `theorydb:"attr:id" json:"id"`
	Username      string         `theorydb:"attr:username" json:"username"` // Who scheduled the status
	Status        string         `theorydb:"attr:status" json:"status"`     // The status content
	MediaIDs      []string       `theorydb:"attr:mediaIDs" json:"media_ids,omitempty"`
	Sensitive     bool           `theorydb:"attr:sensitive" json:"sensitive"`
	SpoilerText   string         `theorydb:"attr:spoilerText" json:"spoiler_text,omitempty"`
	Visibility    string         `theorydb:"attr:visibility" json:"visibility"` // public, unlisted, private, direct
	Language      string         `theorydb:"attr:language" json:"language,omitempty"`
	InReplyToID   string         `theorydb:"attr:inReplyToID" json:"in_reply_to_id,omitempty"`
	Poll          map[string]any `theorydb:"attr:poll" json:"poll,omitempty"`      // Poll data if any
	ScheduledAt   time.Time      `theorydb:"attr:scheduledAt" json:"scheduled_at"` // When to publish
	CreatedAt     time.Time      `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt     time.Time      `theorydb:"attr:updatedAt" json:"updated_at"`
	Published     bool           `theorydb:"attr:published" json:"published"` // Whether it has been published
	PublishedAt   *time.Time     `theorydb:"attr:publishedAt" json:"published_at,omitempty"`
	ApplicationID string         `theorydb:"attr:applicationID" json:"application_id,omitempty"` // OAuth app that created it
}

// UpdateKeys updates the DynamoDB keys based on the current field values
func (s *ScheduledStatus) UpdateKeys() error {
	s.PK = fmt.Sprintf("USER#%s#SCHEDULED", s.Username)
	s.SK = fmt.Sprintf("ID#%s", s.ID)
	s.GSI1PK = "SCHEDULED#DUE"
	s.GSI1SK = fmt.Sprintf("TIME#%s#ID#%s", s.ScheduledAt.Format(time.RFC3339Nano), s.ID)
	s.GSI2PK = fmt.Sprintf("SCHEDULED_ID#%s", s.ID)
	s.GSI2SK = fmt.Sprintf("USER#%s#SCHEDULED", s.Username)
	return nil
}

// GetPK returns the partition key
func (s *ScheduledStatus) GetPK() string {
	return s.PK
}

// GetSK returns the sort key
func (s *ScheduledStatus) GetSK() string {
	return s.SK
}

// TableName returns the DynamoDB table backing ScheduledStatus.
func (ScheduledStatus) TableName() string {
	return MainTableName
}
