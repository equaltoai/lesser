package models

import (
	"time"
)

// TimeRange represents a time-based filter
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// StatusSearchOptions configures status search behavior
// This is a query parameter type, not stored in DynamoDB
type StatusSearchOptions struct {
	Limit         int       `json:"limit"`          // Maximum results to return
	Offset        int       `json:"offset"`         // For pagination
	AccountID     string    `json:"account_id"`     // Filter by specific account
	FollowingOnly bool      `json:"following_only"` // Only from accounts user follows
	LocalOnly     bool      `json:"local_only"`     // Only local statuses
	MediaOnly     bool      `json:"media_only"`     // Only statuses with media
	Language      string    `json:"language"`       // Filter by language
	MinEngagement int       `json:"min_engagement"` // Minimum likes/boosts
	TimeRange     TimeRange `json:"time_range"`     // Time-based filtering
}

// NewStatusSearchOptions creates new search options with defaults
func NewStatusSearchOptions() *StatusSearchOptions {
	return &StatusSearchOptions{
		Limit:         20,
		Offset:        0,
		FollowingOnly: false,
		LocalOnly:     false,
		MediaOnly:     false,
		MinEngagement: 0,
		TimeRange: TimeRange{
			Start: time.Time{},
			End:   time.Now(),
		},
	}
}

// WithLimit sets the maximum number of results
func (o *StatusSearchOptions) WithLimit(limit int) *StatusSearchOptions {
	if limit > 0 && limit <= 100 {
		o.Limit = limit
	}
	return o
}

// WithOffset sets the pagination offset
func (o *StatusSearchOptions) WithOffset(offset int) *StatusSearchOptions {
	if offset >= 0 {
		o.Offset = offset
	}
	return o
}

// WithAccountID filters results by account
func (o *StatusSearchOptions) WithAccountID(accountID string) *StatusSearchOptions {
	o.AccountID = accountID
	return o
}

// WithFollowingOnly restricts results to accounts the user follows
func (o *StatusSearchOptions) WithFollowingOnly() *StatusSearchOptions {
	o.FollowingOnly = true
	return o
}

// WithLocalOnly restricts results to local statuses
func (o *StatusSearchOptions) WithLocalOnly() *StatusSearchOptions {
	o.LocalOnly = true
	return o
}

// WithMediaOnly restricts results to statuses with media
func (o *StatusSearchOptions) WithMediaOnly() *StatusSearchOptions {
	o.MediaOnly = true
	return o
}

// WithLanguage filters results by language
func (o *StatusSearchOptions) WithLanguage(language string) *StatusSearchOptions {
	o.Language = language
	return o
}

// WithMinEngagement sets minimum engagement threshold
func (o *StatusSearchOptions) WithMinEngagement(minVal int) *StatusSearchOptions {
	if minVal >= 0 {
		o.MinEngagement = minVal
	}
	return o
}

// WithTimeRange sets the time range filter
func (o *StatusSearchOptions) WithTimeRange(start, end time.Time) *StatusSearchOptions {
	o.TimeRange.Start = start
	o.TimeRange.End = end
	return o
}

// Validate ensures options are valid
func (o *StatusSearchOptions) Validate() error {
	if o.Limit <= 0 {
		o.Limit = 20
	}
	if o.Limit > 100 {
		o.Limit = 100
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	if !o.TimeRange.End.IsZero() && !o.TimeRange.Start.IsZero() {
		if o.TimeRange.Start.After(o.TimeRange.End) {
			o.TimeRange.Start, o.TimeRange.End = o.TimeRange.End, o.TimeRange.Start
		}
	}
	return nil
}
