package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
)

// Article represents a long-form content object
// It extends the standard Object model with CMS-specific metadata
type Article struct {
	Object // Embeds standard ActivityPub Object fields

	// CMS-specific metadata
	Slug            string     `theorydb:"attr:slug" json:"slug,omitempty"`
	Subtitle        string     `theorydb:"attr:subtitle" json:"subtitle,omitempty"`
	Excerpt         string     `theorydb:"attr:excerpt" json:"excerpt,omitempty"`
	FeaturedImage   *Media     `theorydb:"attr:featuredImage" json:"featured_image,omitempty"`
	TableOfContents []TOCEntry `theorydb:"attr:tableOfContents" json:"table_of_contents,omitempty"`

	// EditorialMedia durably carries the published editorial bindings minted at
	// the publish transition. The draft is deleted after publish, so the article
	// record is the surviving source for composed inline media; every article
	// read path composes from this persisted list.
	EditorialMedia []ArticleEditorialMedia `theorydb:"attr:editorialMedia,omitempty" json:"editorial_media,omitempty"`

	// Publishing metadata
	ReadingTimeMinutes int    `theorydb:"attr:readingTimeMinutes" json:"reading_time_minutes"`
	WordCount          int    `theorydb:"attr:wordCount" json:"word_count"`
	ContentFormat      string `theorydb:"attr:contentFormat" json:"content_format"` // html, markdown

	// Organization
	SeriesID    *string  `theorydb:"attr:seriesID" json:"series_id,omitempty"`
	SeriesOrder *int     `theorydb:"attr:seriesOrder" json:"series_order,omitempty"`
	CategoryIDs []string `theorydb:"attr:categoryIDs" json:"category_ids,omitempty"`

	// SEO
	SEOTitle       string `theorydb:"attr:seoTitle" json:"seo_title,omitempty"`
	SEODescription string `theorydb:"attr:seoDescription" json:"seo_description,omitempty"`
	CanonicalURL   string `theorydb:"attr:canonicalURL" json:"canonical_url,omitempty"`
	OGImage        string `theorydb:"attr:ogImage" json:"og_image,omitempty"`

	// Editorial
	EditorNotes  string `theorydb:"attr:editorNotes" json:"editor_notes,omitempty"`
	ReviewStatus string `theorydb:"attr:reviewStatus" json:"review_status,omitempty"`

	// Authoring attribution
	GeneratedBy string `theorydb:"attr:generatedBy,omitempty" json:"generated_by,omitempty"`
	ReviewedBy  string `theorydb:"attr:reviewedBy,omitempty" json:"reviewed_by,omitempty"`
	PublishedBy string `theorydb:"attr:publishedBy,omitempty" json:"published_by,omitempty"`

	// ActedBy records the local actor URI of the caller who acted on the
	// attributed author's behalf under an active share grant (empty when
	// the author acted as themselves).
	ActedBy string `theorydb:"attr:actedBy,omitempty" json:"acted_by,omitempty"`

	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TOCEntry represents a table of contents entry
type TOCEntry struct {
	ID    string `json:"id"`
	Level int    `json:"level"` // 1-6 for h1-h6
	Text  string `json:"text"`
}

// APAttachments renders the article's bound editorial media as ActivityPub
// Document attachments from their minted public serving URLs. Social-card
// media is promotional and never attaches to the article object; inline and
// hero media attach in canonical binding order.
func (a *Article) APAttachments() []activitypub.Attachment {
	if a == nil {
		return nil
	}
	var out []activitypub.Attachment
	for _, m := range a.EditorialMedia {
		if m.Role == EditorialMediaRoleSocialCard {
			continue
		}
		url := strings.TrimSpace(m.URL)
		if url == "" {
			continue
		}
		out = append(out, activitypub.Attachment{
			Type:      "Document",
			MediaType: strings.TrimSpace(m.ContentType),
			URL:       url,
			Name:      strings.TrimSpace(m.AltText),
			Width:     m.Width,
			Height:    m.Height,
		})
	}
	return out
}

// RenderMediaList maps the article's persisted inline editorial bindings onto
// the canonical renderer's media descriptors so every published article read
// path composes the minted serving. Only inline media composes into published
// article HTML: the hero is the article's leading image in draft previews only
// and otherwise lives on Article.featuredImage, and social-card media never
// composes into the body. A binding without a minted URL is skipped: the
// publish gate only mints digest-verified assets, so an empty URL here is a
// fail-closed skip, never a placeholder.
func (a *Article) RenderMediaList() []cmsrender.ArticleMedia {
	if a == nil {
		return nil
	}
	var out []cmsrender.ArticleMedia
	for _, m := range a.EditorialMedia {
		if m.Role != EditorialMediaRoleInline {
			continue
		}
		render := m.RenderMedia()
		if strings.TrimSpace(render.URL) == "" {
			continue
		}
		out = append(out, render)
	}
	return out
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
