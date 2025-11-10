package models

import (
	"fmt"
	"time"
)

// ScheduledStatus represents a scheduled status post in DynamoDB
type ScheduledStatus struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys
	PK string `dynamorm:"pk,attr:PK"` // USER#{username}#SCHEDULED
	SK string `dynamorm:"sk,attr:SK"` // ID#{id}

	// GSI1 keys for time-based queries (due status queries)
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK"` // SCHEDULED#DUE
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK"` // TIME#{scheduled_at_RFC3339Nano}#ID#{id}

	// Business fields - embedded from storage.ScheduledStatus
	ID            string         `dynamorm:"attr:id" json:"id"`
	Username      string         `dynamorm:"attr:username" json:"username"` // Who scheduled the status
	Status        string         `dynamorm:"attr:status" json:"status"`     // The status content
	MediaIDs      []string       `dynamorm:"attr:mediaIDs" json:"media_ids,omitempty"`
	Sensitive     bool           `dynamorm:"attr:sensitive" json:"sensitive"`
	SpoilerText   string         `dynamorm:"attr:spoilerText" json:"spoiler_text,omitempty"`
	Visibility    string         `dynamorm:"attr:visibility" json:"visibility"` // public, unlisted, private, direct
	Language      string         `dynamorm:"attr:language" json:"language,omitempty"`
	InReplyToID   string         `dynamorm:"attr:inReplyToID" json:"in_reply_to_id,omitempty"`
	Poll          map[string]any `dynamorm:"attr:poll" json:"poll,omitempty"`      // Poll data if any
	ScheduledAt   time.Time      `dynamorm:"attr:scheduledAt" json:"scheduled_at"` // When to publish
	CreatedAt     time.Time      `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt     time.Time      `dynamorm:"attr:updatedAt" json:"updated_at"`
	Published     bool           `dynamorm:"attr:published" json:"published"` // Whether it has been published
	PublishedAt   *time.Time     `dynamorm:"attr:publishedAt" json:"published_at,omitempty"`
	ApplicationID string         `dynamorm:"attr:applicationID" json:"application_id,omitempty"` // OAuth app that created it
}

// UpdateKeys updates the DynamoDB keys based on the current field values
func (s *ScheduledStatus) UpdateKeys() error {
	s.PK = fmt.Sprintf("USER#%s#SCHEDULED", s.Username)
	s.SK = fmt.Sprintf("ID#%s", s.ID)
	s.GSI1PK = "SCHEDULED#DUE"
	s.GSI1SK = fmt.Sprintf("TIME#%s#ID#%s", s.ScheduledAt.Format(time.RFC3339Nano), s.ID)
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
