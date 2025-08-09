package models

import (
	"fmt"
	"time"
)

// Filter represents a user's content filter with DynamORM tags
type Filter struct {
	// Keys
	PK string `dynamorm:"pk"` // USER#username
	SK string `dynamorm:"sk"` // FILTER#filterID

	// Filter fields
	ID           string     `json:"id"`            // Unique filter ID
	Username     string     `json:"username"`      // Owner of the filter
	Title        string     `json:"title"`         // Human-readable title
	Context      []string   `json:"context"`       // Where to apply: home, notifications, public, thread, account
	FilterAction string     `json:"filter_action"` // Action to take: warn, hide, blur
	ExpiresAt    *time.Time `json:"expires_at"`    // Optional expiration
	CreatedAt    time.Time  `json:"created_at"`    // Creation timestamp
	UpdatedAt    time.Time  `json:"updated_at"`    // Last update timestamp
}

// TableName returns the DynamoDB table name
func (Filter) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamORM keys based on filter data
func (f *Filter) UpdateKeys() {
	f.PK = fmt.Sprintf(KeyPatternUser, f.Username)
	f.SK = fmt.Sprintf("FILTER#%s", f.ID)
}

// BeforeCreate hook to set timestamps and update keys
func (f *Filter) BeforeCreate() error {
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	f.UpdatedAt = f.CreatedAt
	f.UpdateKeys()
	return nil
}

// BeforeSave hook to update timestamps and keys
func (f *Filter) BeforeSave() error {
	f.UpdatedAt = time.Now()
	f.UpdateKeys()
	return nil
}

// FilterKeyword represents a keyword in a filter with DynamORM tags
type FilterKeyword struct {
	// Keys
	PK string `dynamorm:"pk"` // FILTER#filterID
	SK string `dynamorm:"sk"` // KEYWORD#keywordID

	// Keyword fields
	ID        string    `json:"id"`         // Unique keyword ID
	FilterID  string    `json:"filter_id"`  // Parent filter ID
	Keyword   string    `json:"keyword"`    // The keyword to filter
	WholeWord bool      `json:"whole_word"` // Match whole word only
	CreatedAt time.Time `json:"created_at"` // Creation timestamp
}

// TableName returns the DynamoDB table name
func (FilterKeyword) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamORM keys based on keyword data
func (fk *FilterKeyword) UpdateKeys() {
	fk.PK = fmt.Sprintf("FILTER#%s", fk.FilterID)
	fk.SK = fmt.Sprintf("KEYWORD#%s", fk.ID)
}

// BeforeCreate hook to set timestamps and update keys
func (fk *FilterKeyword) BeforeCreate() error {
	if fk.CreatedAt.IsZero() {
		fk.CreatedAt = time.Now()
	}
	fk.UpdateKeys()
	return nil
}

// BeforeSave hook to update keys
func (fk *FilterKeyword) BeforeSave() error {
	fk.UpdateKeys()
	return nil
}

// FilterStatus represents a status in a filter with DynamORM tags
type FilterStatus struct {
	// Keys
	PK string `dynamorm:"pk"` // FILTER#filterID
	SK string `dynamorm:"sk"` // STATUS#statusID

	// Status fields
	ID        string    `json:"id"`         // Unique filter status ID
	FilterID  string    `json:"filter_id"`  // Parent filter ID
	StatusID  string    `json:"status_id"`  // The status ID to filter
	CreatedAt time.Time `json:"created_at"` // Creation timestamp
}

// TableName returns the DynamoDB table name
func (FilterStatus) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamORM keys based on status data
func (fs *FilterStatus) UpdateKeys() {
	fs.PK = fmt.Sprintf("FILTER#%s", fs.FilterID)
	fs.SK = fmt.Sprintf(KeyPatternStatus, fs.StatusID)
}

// BeforeCreate hook to set timestamps and update keys
func (fs *FilterStatus) BeforeCreate() error {
	if fs.CreatedAt.IsZero() {
		fs.CreatedAt = time.Now()
	}
	fs.UpdateKeys()
	return nil
}

// BeforeSave hook to update keys
func (fs *FilterStatus) BeforeSave() error {
	fs.UpdateKeys()
	return nil
}
