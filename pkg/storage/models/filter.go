package models

import (
	"fmt"
	"time"
)

// Filter represents a user's content filter with DynamORM tags
type Filter struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK"` // USER#username
	SK string `dynamorm:"sk,attr:SK"` // FILTER#filterID

	// Filter fields
	ID            string     `dynamorm:"attr:id" json:"id"`                              // Unique filter ID
	Username      string     `dynamorm:"attr:username" json:"username"`                  // Owner of the filter
	Title         string     `dynamorm:"attr:title" json:"title"`                        // Human-readable title
	Context       []string   `dynamorm:"attr:context" json:"context"`                    // Where to apply: home, notifications, public, thread, account
	FilterAction  string     `dynamorm:"attr:filterAction" json:"filter_action"`         // Action to take: warn, hide, blur, silence, limit_reach
	Severity      string     `dynamorm:"attr:severity" json:"severity"`                  // Filter severity: low, medium, high
	MatchMode     string     `dynamorm:"attr:matchMode" json:"match_mode"`               // Matching mode: keyword, regex, semantic, exact
	CaseSensitive bool       `dynamorm:"attr:caseSensitive" json:"case_sensitive"`       // Case-sensitive matching
	ExpiresAt     *time.Time `dynamorm:"attr:expiresAt" json:"expires_at"`               // Optional expiration
	CreatedAt     time.Time  `dynamorm:"attr:createdAt" json:"created_at"`               // Creation timestamp
	UpdatedAt     time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`               // Last update timestamp
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK"` // FILTER#filterID
	SK string `dynamorm:"sk,attr:SK"` // KEYWORD#keywordID

	// Keyword fields
	ID           string    `dynamorm:"attr:id" json:"id"`                        // Unique keyword ID
	FilterID     string    `dynamorm:"attr:filterID" json:"filter_id"`           // Parent filter ID
	Keyword      string    `dynamorm:"attr:keyword" json:"keyword"`             // The keyword to filter
	WholeWord    bool      `dynamorm:"attr:wholeWord" json:"whole_word"`        // Match whole word only
	IsRegex      bool      `dynamorm:"attr:isRegex" json:"is_regex"`            // Whether keyword is a regex pattern
	MatchWeight  float64   `dynamorm:"attr:matchWeight" json:"match_weight"`    // Weight for scoring matches (0.0-1.0)
	ContextTypes []string  `dynamorm:"attr:contextTypes" json:"context_types"`  // Specific contexts where this keyword applies
	CreatedAt    time.Time `dynamorm:"attr:createdAt" json:"created_at"`        // Creation timestamp
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK"` // FILTER#filterID
	SK string `dynamorm:"sk,attr:SK"` // STATUS#statusID

	// Status fields
	ID        string    `dynamorm:"attr:id" json:"id"`                 // Unique filter status ID
	FilterID  string    `dynamorm:"attr:filterID" json:"filter_id"`   // Parent filter ID
	StatusID  string    `dynamorm:"attr:statusID" json:"status_id"`   // The status ID to filter
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"` // Creation timestamp
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
