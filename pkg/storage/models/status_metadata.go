package models

import (
	"fmt"
	"time"
)

// StatusMetadata represents additional metadata for a status/object
// This handles features like quote permissions, withdrawal status, etc.
type StatusMetadata struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "STATUS_META#{status_id}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "METADATA"

	// Core identification
	StatusID string `theorydb:"attr:statusID" json:"status_id"`

	// Quote-related metadata
	QuoteType           string `theorydb:"attr:quoteType" json:"quote_type"`                      // "public", "followers", "mentioned", "disabled"
	WithdrawnFromQuotes bool   `theorydb:"attr:withdrawnFromQuotes" json:"withdrawn_from_quotes"` // Whether status is withdrawn from quotes
	AllowQuotes         bool   `theorydb:"attr:allowQuotes" json:"allow_quotes"`                  // Whether quotes are allowed at all
	QuotePermissions    string `theorydb:"attr:quotePermissions" json:"quote_permissions"`        // JSON serialized quote permissions

	// Reply-related metadata
	AllowReplies     bool   `theorydb:"attr:allowReplies" json:"allow_replies"`         // Whether replies are allowed
	ReplyPermissions string `theorydb:"attr:replyPermissions" json:"reply_permissions"` // JSON serialized reply permissions
	ReplyCount       int    `theorydb:"attr:replyCount" json:"reply_count"`             // Cache of reply count

	// Moderation metadata
	ContentWarning  string   `theorydb:"attr:contentWarning" json:"content_warning"`   // Content warning text
	ModerationFlags []string `theorydb:"attr:moderationFlags" json:"moderation_flags"` // Applied moderation flags
	ModerationNotes string   `theorydb:"attr:moderationNotes" json:"moderation_notes"` // Internal moderation notes

	// Engagement settings
	DisableLikes   bool `theorydb:"attr:disableLikes" json:"disable_likes"`     // Whether likes are disabled
	DisableReblogs bool `theorydb:"attr:disableReblogs" json:"disable_reblogs"` // Whether reblogs are disabled

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// Version for optimistic locking
	Version int `theorydb:"version,attr:version" json:"version"`
}

// NewStatusMetadata creates a new status metadata record
func NewStatusMetadata(statusID string) *StatusMetadata {
	now := time.Now()
	metadata := &StatusMetadata{
		StatusID:            statusID,
		QuoteType:           "public", // Default to public quotes
		WithdrawnFromQuotes: false,    // Default to not withdrawn
		AllowQuotes:         true,     // Default to allow quotes
		AllowReplies:        true,     // Default to allow replies
		DisableLikes:        false,    // Default to allow likes
		DisableReblogs:      false,    // Default to allow reblogs
		ModerationFlags:     []string{},
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	metadata.UpdateKeys()
	return metadata
}

// UpdateKeys updates the DynamoDB keys
func (sm *StatusMetadata) UpdateKeys() {
	sm.PK = fmt.Sprintf("STATUS_META#%s", sm.StatusID)
	sm.SK = SKMetadata
}

// BeforeCreate is called before creating the record
func (sm *StatusMetadata) BeforeCreate() error {
	now := time.Now()
	sm.CreatedAt = now
	sm.UpdatedAt = now
	sm.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (sm *StatusMetadata) BeforeUpdate() error {
	sm.UpdatedAt = time.Now()
	sm.UpdateKeys()
	return nil
}

// TableName returns the DynamoDB table name
func (StatusMetadata) TableName() string {
	return MainTableName // Use the main table
}

// WithdrawFromQuotes marks the status as withdrawn from quotes
func (sm *StatusMetadata) WithdrawFromQuotes() {
	sm.WithdrawnFromQuotes = true
	sm.AllowQuotes = false
	sm.QuoteType = StatusDisabled
}

// RestoreToQuotes restores the status to allow quotes
func (sm *StatusMetadata) RestoreToQuotes() {
	sm.WithdrawnFromQuotes = false
	sm.AllowQuotes = true
	if sm.QuoteType == StatusDisabled {
		sm.QuoteType = VisibilityPublic // Reset to default
	}
}

// SetQuoteType sets the quote type and updates permissions accordingly
func (sm *StatusMetadata) SetQuoteType(quoteType string) {
	sm.QuoteType = quoteType
	switch quoteType {
	case StatusDisabled:
		sm.AllowQuotes = false
		sm.WithdrawnFromQuotes = true
	case VisibilityPublic, "followers", "mentioned":
		sm.AllowQuotes = true
		sm.WithdrawnFromQuotes = false
	default:
		// Unknown type, default to public
		sm.QuoteType = VisibilityPublic
		sm.AllowQuotes = true
		sm.WithdrawnFromQuotes = false
	}
}

// AddModerationFlag adds a moderation flag
func (sm *StatusMetadata) AddModerationFlag(flag string) {
	// Check if flag already exists
	for _, existing := range sm.ModerationFlags {
		if existing == flag {
			return
		}
	}
	sm.ModerationFlags = append(sm.ModerationFlags, flag)
}

// RemoveModerationFlag removes a moderation flag
func (sm *StatusMetadata) RemoveModerationFlag(flag string) {
	newFlags := make([]string, 0, len(sm.ModerationFlags))
	for _, existing := range sm.ModerationFlags {
		if existing != flag {
			newFlags = append(newFlags, existing)
		}
	}
	sm.ModerationFlags = newFlags
}

// HasModerationFlag checks if a moderation flag is present
func (sm *StatusMetadata) HasModerationFlag(flag string) bool {
	for _, existing := range sm.ModerationFlags {
		if existing == flag {
			return true
		}
	}
	return false
}

// IsQuotable returns whether the status can be quoted
func (sm *StatusMetadata) IsQuotable() bool {
	return sm.AllowQuotes && !sm.WithdrawnFromQuotes && sm.QuoteType != StatusDisabled
}

// IsPubliclyQuotable returns whether the status can be quoted by anyone
func (sm *StatusMetadata) IsPubliclyQuotable() bool {
	return sm.IsQuotable() && sm.QuoteType == VisibilityPublic
}

// IncrementReplyCount increments the reply count
func (sm *StatusMetadata) IncrementReplyCount() {
	sm.ReplyCount++
}

// DecrementReplyCount decrements the reply count (with minimum of 0)
func (sm *StatusMetadata) DecrementReplyCount() {
	if sm.ReplyCount > 0 {
		sm.ReplyCount--
	}
}

// SetReplyCount sets the reply count to a specific value
func (sm *StatusMetadata) SetReplyCount(count int) {
	if count < 0 {
		sm.ReplyCount = 0
	} else {
		sm.ReplyCount = count
	}
}
