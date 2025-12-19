package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

// Reaction represents an available reaction for announcements
type Reaction struct {
	Name      string `json:"name"`                 // Emoji name or custom emoji shortcode
	Count     int    `json:"count"`                // Number of users who reacted
	Me        bool   `json:"me"`                   // Whether the current user reacted
	URL       string `json:"url,omitempty"`        // URL for custom emoji
	StaticURL string `json:"static_url,omitempty"` // Static URL for custom emoji
}

// TableName returns the DynamoDB table backing Reaction.
func (Reaction) TableName() string {
	return MainTableName
}

// CustomEmoji represents a custom emoji used in announcements
type CustomEmoji struct {
	Shortcode       string `json:"shortcode"`
	URL             string `json:"url"`
	StaticURL       string `json:"static_url"`
	VisibleInPicker bool   `json:"visible_in_picker"`
	Category        string `json:"category,omitempty"`
}

// TableName returns the DynamoDB table backing CustomEmoji.
func (CustomEmoji) TableName() string {
	return MainTableName
}

// Mention represents a mention in an announcement
type Mention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	URL      string `json:"url"`
	Acct     string `json:"acct"`
}

// TableName returns the DynamoDB table backing Mention.
func (Mention) TableName() string {
	return MainTableName
}

// Announcement represents an announcement in DynamoDB
type Announcement struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// GSI1 - Status-based queries (active/inactive)
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "ANNOUNCEMENT#active" or "ANNOUNCEMENT#inactive"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "reverse_timestamp" for chronological order

	// GSI2 - Created by queries (admin management)
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "ADMIN#{admin_username}"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{published_at}#{id}"

	// Announcement fields
	ID          string        `dynamorm:"attr:id" json:"id"`
	Content     string        `dynamorm:"attr:content" json:"content"`               // HTML content
	Text        string        `dynamorm:"attr:text" json:"text"`                     // Plain text version
	PublishedAt time.Time     `dynamorm:"attr:publishedAt" json:"published_at"`      // When it was published
	UpdatedAt   time.Time     `dynamorm:"attr:updatedAt" json:"updated_at"`          // When it was last updated
	AllDay      bool          `dynamorm:"attr:allDay" json:"all_day"`                // Whether it's an all-day announcement
	StartsAt    *time.Time    `dynamorm:"attr:startsAt" json:"starts_at,omitempty"`  // When the announcement starts
	EndsAt      *time.Time    `dynamorm:"attr:endsAt" json:"ends_at,omitempty"`      // When the announcement ends
	Reactions   []Reaction    `dynamorm:"attr:reactions" json:"reactions,omitempty"` // Available reactions
	Tags        []string      `dynamorm:"attr:tags" json:"tags,omitempty"`           // Hashtags
	Emojis      []CustomEmoji `dynamorm:"attr:emojis" json:"emojis,omitempty"`       // Custom emojis
	Mentions    []Mention     `dynamorm:"attr:mentions" json:"mentions,omitempty"`   // Mentions
	CreatedBy   string        `dynamorm:"attr:createdBy" json:"created_by"`          // Admin who created it
	CreatedAt   time.Time     `dynamorm:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table backing Announcement.
func (Announcement) TableName() string {
	return MainTableName
}

// UpdateKeys updates the PK and SK based on the announcement data
func (a *Announcement) UpdateKeys() error {
	a.PK = fmt.Sprintf("ANNOUNCEMENT#%s", a.ID)
	a.SK = "ANNOUNCEMENT"

	// Set up GSI keys
	a.setupGSIKeys()

	return nil
}

// setupGSIKeys configures GSI partition and sort keys
func (a *Announcement) setupGSIKeys() {
	now := time.Now()

	// GSI1 - Status-based queries with date ordering
	status := a.getStatusString(now)
	a.GSI1PK = fmt.Sprintf("ANNOUNCEMENT#%s", status)
	// Use reverse timestamp for newest-first ordering
	reverseTimestamp := 9999999999 - a.PublishedAt.Unix()
	a.GSI1SK = fmt.Sprintf("%010d", reverseTimestamp)

	// GSI2 - Admin queries
	if a.CreatedBy != "" {
		a.GSI2PK = "ADMIN#" + a.CreatedBy
		a.GSI2SK = fmt.Sprintf("%s#%s", a.PublishedAt.Format(time.RFC3339), a.ID)
	} else {
		a.GSI2PK = ""
		a.GSI2SK = ""
	}
}

// getStatusString determines if announcement is active or inactive
func (a *Announcement) getStatusString(now time.Time) string {
	// Check if announcement has started (if StartsAt is set)
	if a.StartsAt != nil && a.StartsAt.After(now) {
		return "inactive" // Not yet started
	}

	// Check if announcement has ended (if EndsAt is set)
	if a.EndsAt != nil && a.EndsAt.Before(now) {
		return "inactive" // Already ended
	}

	return StatusActive
}

// IsActive determines if the announcement is currently active
func (a *Announcement) IsActive() bool {
	now := time.Now()
	return a.getStatusString(now) == StatusActive
}

// BeforeCreate prepares the announcement for creation
func (a *Announcement) BeforeCreate() error {
	if err := common.ValidateRequiredParam("a.ID", a.ID); err != nil {
		a.ID = uuid.New().String()
	}
	now := time.Now()
	a.PublishedAt = now
	a.UpdatedAt = now
	a.CreatedAt = now
	return a.UpdateKeys()
}

// AnnouncementDismissal represents a user dismissing an announcement
type AnnouncementDismissal struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// Dismissal fields
	Username       string    `dynamorm:"attr:username" json:"username"`
	AnnouncementID string    `dynamorm:"attr:announcementID" json:"announcement_id"`
	DismissedAt    time.Time `dynamorm:"attr:dismissedAt" json:"dismissed_at"`
}

// TableName returns the DynamoDB table backing AnnouncementDismissal.
func (AnnouncementDismissal) TableName() string {
	return MainTableName
}

// UpdateKeys updates the PK and SK based on the dismissal data
func (d *AnnouncementDismissal) UpdateKeys() error {
	d.PK = fmt.Sprintf(KeyPatternUser, d.Username)
	d.SK = fmt.Sprintf("ANNOUNCEMENT_DISMISSED#%s", d.AnnouncementID)
	return nil
}

// BeforeCreate prepares the dismissal for creation
func (d *AnnouncementDismissal) BeforeCreate() error {
	d.DismissedAt = time.Now()
	return d.UpdateKeys()
}

// AnnouncementReaction represents a user's reaction to an announcement
type AnnouncementReaction struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// Reaction fields
	Username       string    `dynamorm:"attr:username" json:"username"`
	AnnouncementID string    `dynamorm:"attr:announcementID" json:"announcement_id"`
	EmojiName      string    `dynamorm:"attr:emojiName" json:"emoji_name"`
	ReactedAt      time.Time `dynamorm:"attr:reactedAt" json:"reacted_at"`
}

// TableName returns the DynamoDB table backing AnnouncementReaction.
func (AnnouncementReaction) TableName() string {
	return MainTableName
}

// UpdateKeys updates the PK and SK based on the reaction data
func (r *AnnouncementReaction) UpdateKeys() error {
	r.PK = fmt.Sprintf("ANNOUNCEMENT_REACTION#%s", r.AnnouncementID)
	r.SK = fmt.Sprintf("USER#%s#%s", r.Username, r.EmojiName)
	return nil
}

// BeforeCreate prepares the reaction for creation
func (r *AnnouncementReaction) BeforeCreate() error {
	r.ReactedAt = time.Now()
	if err := r.UpdateKeys(); err != nil {
		return err
	}
	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (a *Announcement) GetPK() string {
	return a.PK
}

// GetSK returns the sort key (required by BaseModel)
func (a *Announcement) GetSK() string {
	return a.SK
}

// GetPK returns the partition key for dismissal
func (d *AnnouncementDismissal) GetPK() string {
	return d.PK
}

// GetSK returns the sort key for dismissal
func (d *AnnouncementDismissal) GetSK() string {
	return d.SK
}

// GetPK returns the partition key for reaction
func (r *AnnouncementReaction) GetPK() string {
	return r.PK
}

// GetSK returns the sort key for reaction
func (r *AnnouncementReaction) GetSK() string {
	return r.SK
}
