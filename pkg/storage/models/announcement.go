package models

import (
	"fmt"
	"time"

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

// CustomEmoji represents a custom emoji used in announcements
type CustomEmoji struct {
	Shortcode           string    `json:"shortcode"`
	URL                 string    `json:"url"`
	StaticURL           string    `json:"static_url"`
	VisibleInPicker     bool      `json:"visible_in_picker"`
	Category            string    `json:"category,omitempty"`
}

// Mention represents a mention in an announcement
type Mention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	URL      string `json:"url"`
	Acct     string `json:"acct"`
}

// Announcement represents an announcement in DynamoDB
type Announcement struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Announcement fields
	ID          string `json:"id"`
	Content     string `json:"content"`             // HTML content
	Text        string `json:"text"`                // Plain text version
	PublishedAt time.Time `json:"published_at"`     // When it was published
	UpdatedAt   time.Time `json:"updated_at"`       // When it was last updated
	AllDay      bool `json:"all_day"`              // Whether it's an all-day announcement
	StartsAt    *time.Time `json:"starts_at,omitempty"` // When the announcement starts
	EndsAt      *time.Time `json:"ends_at,omitempty"`   // When the announcement ends
	Reactions   []Reaction `json:"reactions,omitempty"` // Available reactions
	Tags        []string `json:"tags,omitempty"`         // Hashtags
	Emojis      []CustomEmoji `json:"emojis,omitempty"` // Custom emojis
	Mentions    []Mention `json:"mentions,omitempty"`   // Mentions
	CreatedBy   string `json:"created_by"`                      // Admin who created it
	CreatedAt   time.Time `json:"created_at"`
}

// UpdateKeys updates the PK and SK based on the announcement data
func (a *Announcement) UpdateKeys() error {
	a.PK = fmt.Sprintf("ANNOUNCEMENT#%s", a.ID)
	a.SK = "ANNOUNCEMENT"
	return nil
}

// BeforeCreate prepares the announcement for creation
func (a *Announcement) BeforeCreate() error {
	if a.ID == "" {
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
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Dismissal fields
	Username       string    `json:"username"`
	AnnouncementID string    `json:"announcement_id"`
	DismissedAt    time.Time `json:"dismissed_at"`
}

// UpdateKeys updates the PK and SK based on the dismissal data
func (d *AnnouncementDismissal) UpdateKeys() error {
	d.PK = fmt.Sprintf("USER#%s", d.Username)
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
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Reaction fields
	Username       string    `json:"username"`
	AnnouncementID string    `json:"announcement_id"`
	EmojiName      string    `json:"emoji_name"`
	ReactedAt      time.Time `json:"reacted_at"`
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
	r.UpdateKeys()
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