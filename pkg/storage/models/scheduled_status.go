package models

import (
	"fmt"
	"time"
)

// ScheduledStatus represents a scheduled status post in DynamoDB
type ScheduledStatus struct {
	// Primary keys
	PK string `dynamorm:"pk"` // USER#{username}#SCHEDULED
	SK string `dynamorm:"sk"` // ID#{id}

	// GSI1 keys for time-based queries (due status queries)
	GSI1PK string `dynamorm:"index:gsi1,pk"` // SCHEDULED#DUE
	GSI1SK string `dynamorm:"index:gsi1,sk"` // TIME#{scheduled_at_RFC3339Nano}#ID#{id}

	// Business fields - embedded from storage.ScheduledStatus
	ID            string         `json:"id"`
	Username      string         `json:"username"` // Who scheduled the status
	Status        string         `json:"status"`   // The status content
	MediaIDs      []string       `json:"media_ids,omitempty"`
	Sensitive     bool           `json:"sensitive"`
	SpoilerText   string         `json:"spoiler_text,omitempty"`
	Visibility    string         `json:"visibility"` // public, unlisted, private, direct
	Language      string         `json:"language,omitempty"`
	InReplyToID   string         `json:"in_reply_to_id,omitempty"`
	Poll          map[string]any `json:"poll,omitempty"` // Poll data if any
	ScheduledAt   time.Time      `json:"scheduled_at"`   // When to publish
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Published     bool           `json:"published"` // Whether it has been published
	PublishedAt   *time.Time     `json:"published_at,omitempty"`
	ApplicationID string         `json:"application_id,omitempty"` // OAuth app that created it
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
