package models

import (
	"fmt"
	"strings"
	"time"
)

// Post represents a blog post
type Post struct {
	ID            string            `dynamorm:"pk" json:"id"`
	Slug          string            `dynamorm:"index:gsi-slug,pk" json:"slug"`
	AuthorID      string            `dynamorm:"index:gsi-author,pk" json:"author_id"`
	Title         string            `json:"title"`
	Content       string            `json:"content"`
	Excerpt       string            `json:"excerpt,omitempty"`
	Status        string            `dynamorm:"index:gsi-status-date,pk" json:"status"` // draft, published, archived
	PublishedAt   time.Time         `dynamorm:"index:gsi-status-date,sk" json:"published_at,omitempty"`
	Tags          []string          `dynamorm:"set" json:"tags,omitempty"`
	CategoryID    string            `dynamorm:"index:gsi-category,pk" json:"category_id,omitempty"`
	ViewCount     int               `json:"view_count"`
	CommentCount  int               `json:"comment_count"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	FeaturedImage string            `json:"featured_image,omitempty"`
	SEO           SEOMetadata       `json:"seo,omitempty"`
	CreatedAt     time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt     time.Time         `dynamorm:"updated_at" json:"updated_at"`
	Version       int               `dynamorm:"version" json:"version"`
}

// PostStatus constants
const (
	PostStatusDraft     = "draft"
	PostStatusPublished = "published"
	PostStatusArchived  = "archived"
)

// Comment represents a comment on a post
type Comment struct {
	ID          string    `dynamorm:"pk" json:"id"`
	PostID      string    `dynamorm:"index:gsi-post,pk" json:"post_id"`
	ParentID    string    `dynamorm:"index:gsi-post,sk,prefix:parent" json:"parent_id,omitempty"`
	AuthorID    string    `json:"author_id"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	Content     string    `json:"content"`
	Status      string    `json:"status"` // approved, pending, spam
	IPAddress   string    `json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt   time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// CommentStatus constants
const (
	CommentStatusApproved = "approved"
	CommentStatusPending  = "pending"
	CommentStatusSpam     = "spam"
)

// Author represents a blog author
type Author struct {
	ID          string    `dynamorm:"pk" json:"id"`
	Email       string    `dynamorm:"index:gsi-email,pk" json:"email"`
	Username    string    `dynamorm:"index:gsi-username,pk" json:"username"`
	Name        string    `json:"name"`
	Bio         string    `json:"bio,omitempty"`
	Avatar      string    `json:"avatar,omitempty"`
	Role        string    `json:"role"` // admin, editor, author
	Active      bool      `json:"active"`
	PostCount   int       `json:"post_count"`
	LastLoginAt time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt   time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// AuthorRole constants
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleAuthor = "author"
)

// Category represents a blog category
type Category struct {
	ID          string    `dynamorm:"pk" json:"id"`
	Slug        string    `dynamorm:"index:gsi-slug,pk" json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ParentID    string    `json:"parent_id,omitempty"`
	PostCount   int       `json:"post_count"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt   time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// Tag represents a blog tag
type Tag struct {
	ID        string    `dynamorm:"pk" json:"id"`
	Name      string    `dynamorm:"index:gsi-name,pk" json:"name"`
	Slug      string    `dynamorm:"index:gsi-slug,pk" json:"slug"`
	PostCount int       `json:"post_count"`
	CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
}

// Subscriber represents an email subscriber
type Subscriber struct {
	ID               string    `dynamorm:"pk" json:"id"`
	Email            string    `dynamorm:"index:gsi-email,pk" json:"email"`
	Name             string    `json:"name,omitempty"`
	Status           string    `json:"status"` // active, unsubscribed, bounced
	Categories       []string  `dynamorm:"set" json:"categories,omitempty"`
	Tags             []string  `dynamorm:"set" json:"tags,omitempty"`
	VerifiedAt       time.Time `json:"verified_at,omitempty"`
	UnsubscribeToken string    `json:"-"`
	CreatedAt        time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt        time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// SearchIndex represents a search index entry for full-text search
type SearchIndex struct {
	ID          string    `dynamorm:"pk" json:"id"`                            // post-{postID}
	ContentType string    `dynamorm:"index:gsi-search,pk" json:"content_type"` // post, page
	SearchTerms string    `dynamorm:"index:gsi-search,sk" json:"search_terms"` // lowercase, space-separated
	PostID      string    `json:"post_id"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	Tags        []string  `dynamorm:"set" json:"tags"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Analytics represents page view analytics
type Analytics struct {
	ID          string         `dynamorm:"pk" json:"id"` // Format: "post_id#date"
	PostID      string         `json:"post_id"`
	Date        string         `json:"date"` // YYYY-MM-DD
	Views       int            `json:"views"`
	UniqueViews int            `json:"unique_views"`
	Countries   map[string]int `json:"countries"`
	Referrers   map[string]int `json:"referrers"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Helper methods for Analytics composite key
func (a *Analytics) SetCompositeKey() {
	a.ID = fmt.Sprintf("%s#%s", a.PostID, a.Date)
}

func (a *Analytics) ParseCompositeKey() error {
	parts := strings.Split(a.ID, "#")
	if len(parts) != 2 {
		return fmt.Errorf("invalid composite key format: %s", a.ID)
	}
	a.PostID = parts[0]
	a.Date = parts[1]
	return nil
}

// Session represents a user session for tracking unique views
type Session struct {
	ID          string    `dynamorm:"pk" json:"id"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	PostsViewed []string  `dynamorm:"set" json:"posts_viewed"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `dynamorm:"ttl" json:"expires_at"`
}

// PostView tracks individual post views for analytics
type PostView struct {
	ID        string    `dynamorm:"pk" json:"id"` // Format: "post_id#timestamp"
	PostID    string    `json:"post_id"`
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
	Country   string    `json:"country,omitempty"`
	Referrer  string    `json:"referrer,omitempty"`
	TTL       time.Time `dynamorm:"ttl" json:"ttl"` // 90 days retention
}

// Helper methods for PostView composite key
func (p *PostView) SetCompositeKey() {
	p.ID = fmt.Sprintf("%s#%s", p.PostID, p.Timestamp.Format(time.RFC3339Nano))
}

func (p *PostView) ParseCompositeKey() error {
	parts := strings.Split(p.ID, "#")
	if len(parts) != 2 {
		return fmt.Errorf("invalid composite key format: %s", p.ID)
	}
	p.PostID = parts[0]
	timestamp, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}
	p.Timestamp = timestamp
	return nil
}

// RelatedPost represents a many-to-many relationship between posts
type RelatedPost struct {
	ID        string    `dynamorm:"pk,composite:post_id,related_id" json:"id"`
	PostID    string    `dynamorm:"extract:post_id" json:"post_id"`
	RelatedID string    `dynamorm:"extract:related_id" json:"related_id"`
	Score     float64   `json:"score"` // Relevance score
	CreatedAt time.Time `json:"created_at"`
}

// ContentBlock represents reusable content blocks
type ContentBlock struct {
	ID        string            `dynamorm:"pk" json:"id"`
	Name      string            `dynamorm:"index:gsi-name,unique" json:"name"`
	Type      string            `json:"type"` // html, markdown, component
	Content   string            `json:"content"`
	Variables map[string]string `dynamorm:"json" json:"variables,omitempty"`
	CreatedAt time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt time.Time         `dynamorm:"updated_at" json:"updated_at"`
	Version   int               `dynamorm:"version" json:"version"`
}

// Page represents a static page
type Page struct {
	ID          string            `dynamorm:"pk" json:"id"`
	Slug        string            `dynamorm:"index:gsi-slug,pk" json:"slug"`
	ParentID    string            `json:"parent_id,omitempty"`
	Title       string            `json:"title"`
	Content     string            `json:"content"`
	Template    string            `json:"template,omitempty"`
	Order       int               `json:"order"`
	Status      string            `json:"status"` // draft, published
	Metadata    map[string]string `json:"metadata,omitempty"`
	SEO         SEOMetadata       `json:"seo,omitempty"`
	CreatedAt   time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt   time.Time         `dynamorm:"updated_at" json:"updated_at"`
	PublishedAt time.Time         `json:"published_at,omitempty"`
	Version     int               `dynamorm:"version" json:"version"`
}

// EmailTemplate represents an email template
type EmailTemplate struct {
	ID        string            `dynamorm:"pk" json:"id"`
	Name      string            `dynamorm:"index:gsi-name,pk" json:"name"`
	Subject   string            `json:"subject"`
	Body      string            `json:"body"`
	Variables []string          `json:"variables"` // List of required variables
	Type      string            `json:"type"`      // welcome, notification, newsletter
	Active    bool              `json:"active"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt time.Time         `dynamorm:"updated_at" json:"updated_at"`
}

// SEOMetadata represents SEO information for posts and pages
type SEOMetadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Keywords    string `json:"keywords,omitempty"`
	OGImage     string `json:"og_image,omitempty"`
}
