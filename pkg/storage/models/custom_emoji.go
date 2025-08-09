package models

import (
	"fmt"
	"time"
)

// EmojiModel represents a custom emoji with DynamORM tags
type EmojiModel struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // EMOJI#shortcode
	SK string `dynamorm:"sk" json:"-"` // EMOJI

	// GSI keys - for querying all emojis
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"-"` // ALL_EMOJIS
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"-"` // EMOJI#shortcode

	// GSI keys - for querying by category
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"-"` // CATEGORY#category
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"-"` // EMOJI#shortcode

	// Business fields
	Shortcode           string    `json:"shortcode"`
	URL                 string    `json:"url"`
	StaticURL           string    `json:"static_url"`
	VisibleInPicker     bool      `json:"visible_in_picker"`
	Category            string    `json:"category,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	Disabled            bool      `json:"disabled"`
	Domain              string    `json:"domain,omitempty"` // Empty for local emojis
	ImageRemoteURL      string    `json:"image_remote_url,omitempty"`
	ImageStorageVersion int       `json:"image_storage_version"`
	ImageFileSize       int64     `json:"image_file_size"`
	ImageContentType    string    `json:"image_content_type"`
	ImageWidth          int       `json:"image_width"`
	ImageHeight         int       `json:"image_height"`
	ImageUpdatedAt      time.Time `json:"image_updated_at"`
}

// UpdateKeys updates the composite keys based on the business fields
func (e *EmojiModel) UpdateKeys() {
	// Primary keys
	e.PK = fmt.Sprintf("EMOJI#%s", e.Shortcode)
	e.SK = "EMOJI"

	// GSI1 - for querying all emojis
	e.GSI1PK = "ALL_EMOJIS"
	e.GSI1SK = fmt.Sprintf("EMOJI#%s", e.Shortcode)

	// GSI2 - for querying by category
	if e.Category != "" {
		e.GSI2PK = fmt.Sprintf("CATEGORY#%s", e.Category)
		e.GSI2SK = fmt.Sprintf("EMOJI#%s", e.Shortcode)
	}
}
