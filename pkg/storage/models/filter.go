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
	ID            string     `json:"id"`             // Unique filter ID
	Username      string     `json:"username"`       // Owner of the filter
	Title         string     `json:"title"`          // Human-readable title
	Context       []string   `json:"context"`        // Where to apply: home, notifications, public, thread, account
	FilterAction  string     `json:"filter_action"`  // Action to take: warn, hide, blur, silence, limit_reach
	Severity      string     `json:"severity"`       // Filter severity: low, medium, high
	MatchMode     string     `json:"match_mode"`     // Matching mode: keyword, regex, semantic, exact
	CaseSensitive bool       `json:"case_sensitive"` // Case-sensitive matching
	ExpiresAt     *time.Time `json:"expires_at"`     // Optional expiration
	CreatedAt     time.Time  `json:"created_at"`     // Creation timestamp
	UpdatedAt     time.Time  `json:"updated_at"`     // Last update timestamp
}

// TableName returns the DynamoDB table name
func (Filter) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamORM keys based on filter data
func (f *Filter) UpdateKeys() error {
	f.PK = fmt.Sprintf(KeyPatternUser, f.Username)
	f.SK = fmt.Sprintf("FILTER#%s", f.ID)
	return nil
}

// GetPK returns the partition key
func (f *Filter) GetPK() string {
	return f.PK
}

// GetSK returns the sort key
func (f *Filter) GetSK() string {
	return f.SK
}

// BeforeCreate hook to set timestamps and update keys
func (f *Filter) BeforeCreate() error {
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	f.UpdatedAt = f.CreatedAt
	return f.UpdateKeys()
}

// BeforeSave hook to update timestamps and keys
func (f *Filter) BeforeSave() error {
	f.UpdatedAt = time.Now()
	return f.UpdateKeys()
}

// FilterKeyword represents a keyword in a filter with DynamORM tags
type FilterKeyword struct {
	// Keys
	PK string `dynamorm:"pk"` // FILTER#filterID
	SK string `dynamorm:"sk"` // KEYWORD#keywordID

	// Keyword fields
	ID           string    `json:"id"`            // Unique keyword ID
	FilterID     string    `json:"filter_id"`     // Parent filter ID
	Keyword      string    `json:"keyword"`       // The keyword to filter
	WholeWord    bool      `json:"whole_word"`    // Match whole word only
	IsRegex      bool      `json:"is_regex"`      // Whether keyword is a regex pattern
	MatchWeight  float64   `json:"match_weight"`  // Weight for scoring matches (0.0-1.0)
	ContextTypes []string  `json:"context_types"` // Specific contexts where this keyword applies
	CreatedAt    time.Time `json:"created_at"`    // Creation timestamp
}

// TableName returns the DynamoDB table name
func (FilterKeyword) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamORM keys based on keyword data
func (fk *FilterKeyword) UpdateKeys() error {
	fk.PK = fmt.Sprintf("FILTER#%s", fk.FilterID)
	fk.SK = fmt.Sprintf("KEYWORD#%s", fk.ID)
	return nil
}

// GetPK returns the partition key
func (fk *FilterKeyword) GetPK() string {
	return fk.PK
}

// GetSK returns the sort key
func (fk *FilterKeyword) GetSK() string {
	return fk.SK
}

// BeforeCreate hook to set timestamps and update keys
func (fk *FilterKeyword) BeforeCreate() error {
	if fk.CreatedAt.IsZero() {
		fk.CreatedAt = time.Now()
	}
	return fk.UpdateKeys()
}

// BeforeSave hook to update keys
func (fk *FilterKeyword) BeforeSave() error {
	return fk.UpdateKeys()
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
func (fs *FilterStatus) UpdateKeys() error {
	fs.PK = fmt.Sprintf("FILTER#%s", fs.FilterID)
	fs.SK = fmt.Sprintf("STATUS#%s", fs.StatusID)
	return nil
}

// GetPK returns the partition key
func (fs *FilterStatus) GetPK() string {
	return fs.PK
}

// GetSK returns the sort key
func (fs *FilterStatus) GetSK() string {
	return fs.SK
}

// BeforeCreate hook to set timestamps and update keys
func (fs *FilterStatus) BeforeCreate() error {
	if fs.CreatedAt.IsZero() {
		fs.CreatedAt = time.Now()
	}
	return fs.UpdateKeys()
}

// BeforeSave hook to update keys
func (fs *FilterStatus) BeforeSave() error {
	return fs.UpdateKeys()
}
