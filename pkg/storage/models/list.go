package models

import (
	"fmt"
	"time"
)

// List represents a user-created list for organizing followed accounts
type List struct {
	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"PK"` // LIST#listID
	SK string `dynamorm:"sk" json:"SK"` // METADATA

	// GSI1 for user's lists index
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK,omitempty"` // USER_LISTS#username
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK,omitempty"` // listID

	// Core fields from legacy
	ID            string    `json:"id"`
	Username      string    `json:"username"` // Owner of the list
	Title         string    `json:"title"`
	RepliesPolicy string    `json:"replies_policy"` // list, followed, none
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (List) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating
func (l *List) BeforeCreate() error {
	l.PK = fmt.Sprintf(KeyPatternList, l.ID)
	l.SK = SKMetadata
	l.CreatedAt = time.Now()
	l.UpdatedAt = time.Now()
	return l.UpdateKeys()
}

// BeforeUpdate sets the updated timestamp
func (l *List) BeforeUpdate() error {
	l.UpdatedAt = time.Now()
	return l.UpdateKeys()
}

// UpdateKeys updates the GSI keys based on current field values
func (l *List) UpdateKeys() error {
	// Set up GSI1 keys for user's lists index
	l.GSI1PK = fmt.Sprintf("USER_LISTS#%s", l.Username)
	l.GSI1SK = l.ID
	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (l *List) GetPK() string {
	return l.PK
}

// GetSK returns the sort key (required by BaseModel)
func (l *List) GetSK() string {
	return l.SK
}
