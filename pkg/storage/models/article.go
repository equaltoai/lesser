package models

import (
	"fmt"
	"time"
)

// Article represents a long-form content object
// It extends the standard Object model with CMS-specific metadata
type Article struct {
	Object // Embeds standard ActivityPub Object fields

	// CMS-specific metadata
	Subtitle        string     `dynamorm:"attr:subtitle" json:"subtitle,omitempty"`
	Excerpt         string     `dynamorm:"attr:excerpt" json:"excerpt,omitempty"`
	FeaturedImage   *Media     `dynamorm:"attr:featuredImage" json:"featured_image,omitempty"`
	TableOfContents []TOCEntry `dynamorm:"attr:tableOfContents" json:"table_of_contents,omitempty"`

	// Publishing metadata
	ReadingTimeMinutes int    `dynamorm:"attr:readingTimeMinutes" json:"reading_time_minutes"`
	WordCount          int    `dynamorm:"attr:wordCount" json:"word_count"`
	ContentFormat      string `dynamorm:"attr:contentFormat" json:"content_format"` // html, markdown

	// Organization
	SeriesID    *string  `dynamorm:"attr:seriesID" json:"series_id,omitempty"`
	SeriesOrder *int     `dynamorm:"attr:seriesOrder" json:"series_order,omitempty"`
	CategoryIDs []string `dynamorm:"attr:categoryIDs" json:"category_ids,omitempty"`

	// SEO
	SEOTitle       string `dynamorm:"attr:seoTitle" json:"seo_title,omitempty"`
	SEODescription string `dynamorm:"attr:seoDescription" json:"seo_description,omitempty"`
	CanonicalURL   string `dynamorm:"attr:canonicalURL" json:"canonical_url,omitempty"`
	OGImage        string `dynamorm:"attr:ogImage" json:"og_image,omitempty"`

	// Editorial
	EditorNotes  string `dynamorm:"attr:editorNotes" json:"editor_notes,omitempty"`
	ReviewStatus string `dynamorm:"attr:reviewStatus" json:"review_status,omitempty"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TOCEntry represents a table of contents entry
type TOCEntry struct {
	ID    string `json:"id"`
	Level int    `json:"level"` // 1-6 for h1-h6
	Text  string `json:"text"`
}

// TableName returns the DynamoDB table backing Article.
func (Article) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the Article model
// This overrides Object.UpdateKeys to ensure correct key generation for Articles
func (a *Article) UpdateKeys() error {
	// Ensure ID is set
	if a.ID == "" {
		return fmt.Errorf("ID is required")
	}

	// Set primary keys (same as Object)
	a.PK = fmt.Sprintf("object#%s", a.ID)
	a.SK = fmt.Sprintf("object#%s", a.ID)

	// Update GSI keys using Object's logic
	a.UpdateGSIKeys()

	// Ensure timestamps are set
	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	a.Updated = now // Sync Object.Updated

	return nil
}
