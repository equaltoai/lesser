package model

import (
	"github.com/equaltoai/lesser/pkg/activitypub"
)

// CMS enums

// ContentFormat describes how CMS content is encoded.
type ContentFormat string

const (
	// ContentFormatHTML indicates the content is stored as HTML.
	ContentFormatHTML ContentFormat = "HTML"
	// ContentFormatMarkdown indicates the content is stored as Markdown.
	ContentFormatMarkdown ContentFormat = "MARKDOWN"
)

// DraftStatus represents the lifecycle state of a draft.
type DraftStatus string

const (
	// DraftStatusDraft indicates a draft is not scheduled for publishing.
	DraftStatusDraft DraftStatus = "DRAFT"
	// DraftStatusScheduled indicates a draft is scheduled for publishing.
	DraftStatusScheduled DraftStatus = "SCHEDULED"
	// DraftStatusPublishing indicates a draft publish is in progress.
	DraftStatusPublishing DraftStatus = "PUBLISHING"
	// DraftStatusPublished indicates a draft has been published.
	DraftStatusPublished DraftStatus = "PUBLISHED"
	// DraftStatusFailed indicates the last publish attempt failed.
	DraftStatusFailed DraftStatus = "FAILED"
)

// ChangeType describes the kind of change recorded in a revision.
type ChangeType string

const (
	// ChangeTypeCreate indicates a revision created a new object.
	ChangeTypeCreate ChangeType = "CREATE"
	// ChangeTypeUpdate indicates a revision updated an object.
	ChangeTypeUpdate ChangeType = "UPDATE"
	// ChangeTypeRestore indicates a revision restored content from a previous revision.
	ChangeTypeRestore ChangeType = "RESTORE"
)

// PublicationRole defines the role granted to a publication member.
type PublicationRole string

const (
	// PublicationRoleOwner can manage settings and members.
	PublicationRoleOwner PublicationRole = "OWNER"
	// PublicationRoleEditor can edit and publish content.
	PublicationRoleEditor PublicationRole = "EDITOR"
	// PublicationRoleWriter can create and edit their own content.
	PublicationRoleWriter PublicationRole = "WRITER"
	// PublicationRoleContributor can contribute content with limited privileges.
	PublicationRoleContributor PublicationRole = "CONTRIBUTOR"
)

// CMS output types

// TOCEntry is a table-of-contents entry extracted from article content.
type TOCEntry struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// Article is a published piece of CMS content.
type Article struct {
	ID        string             `json:"id"`
	DeletedAt *Time              `json:"deletedAt"`
	Slug      string             `json:"slug"`
	AuthorID  string             `json:"authorId"`
	Author    *activitypub.Actor `json:"author"`

	Title    string  `json:"title"`
	Subtitle *string `json:"subtitle"`
	Excerpt  *string `json:"excerpt"`

	Content       string        `json:"content"`
	ContentFormat ContentFormat `json:"contentFormat"`
	// RawContentFormat preserves the storage declaration for the canonical renderer.
	// It is internal resolver state and is never serialized as a GraphQL field.
	RawContentFormat string `json:"-"`

	FeaturedImage   *Media      `json:"featuredImage"`
	TableOfContents []*TOCEntry `json:"tableOfContents"`

	ReadingTimeMinutes int `json:"readingTimeMinutes"`
	WordCount          int `json:"wordCount"`

	Series      *Series     `json:"series"`
	SeriesOrder *int        `json:"seriesOrder"`
	Categories  []*Category `json:"categories"`

	SEOTitle       *string `json:"seoTitle"`
	SEODescription *string `json:"seoDescription"`
	CanonicalURL   *string `json:"canonicalUrl"`
	OGImage        *string `json:"ogImage"`

	EditorNotes  *string `json:"editorNotes"`
	ReviewStatus *string `json:"reviewStatus"`

	GeneratedBy *activitypub.Actor `json:"generatedBy"`
	ReviewedBy  *activitypub.Actor `json:"reviewedBy"`
	PublishedBy *activitypub.Actor `json:"publishedBy"`

	PublishedAt Time `json:"publishedAt"`
	CreatedAt   Time `json:"createdAt"`
	UpdatedAt   Time `json:"updatedAt"`
}

// ArticleEdge is an edge in an ArticleConnection.
type ArticleEdge struct {
	Node   *Article `json:"node"`
	Cursor Cursor   `json:"cursor"`
}

// ArticleConnection is a paginated list of articles.
type ArticleConnection struct {
	Edges      []*ArticleEdge `json:"edges"`
	PageInfo   *PageInfo      `json:"pageInfo"`
	TotalCount int            `json:"totalCount"`
}

// Draft represents an editable piece of content prior to publication.
type Draft struct {
	ID       string             `json:"id"`
	AuthorID string             `json:"authorId"`
	Author   *activitypub.Actor `json:"author"`

	ContentType   ObjectType    `json:"contentType"`
	Title         *string       `json:"title"`
	Slug          *string       `json:"slug"`
	Content       string        `json:"content"`
	ContentFormat ContentFormat `json:"contentFormat"`

	Status      DraftStatus `json:"status"`
	ScheduledAt *Time       `json:"scheduledAt"`
	ObjectID    *string     `json:"objectId"`

	GeneratedBy   *activitypub.Actor  `json:"generatedBy"`
	ReviewedBy    *activitypub.Actor  `json:"reviewedBy"`
	ReviewVerdict *DraftReviewVerdict `json:"reviewVerdict"`

	AutosaveVersion int  `json:"autosaveVersion"`
	LastSavedAt     Time `json:"lastSavedAt"`

	CreatedAt Time `json:"createdAt"`
	UpdatedAt Time `json:"updatedAt"`
}

// DraftEdge is an edge in a DraftConnection.
type DraftEdge struct {
	Node   *Draft `json:"node"`
	Cursor Cursor `json:"cursor"`
}

// DraftConnection is a paginated list of drafts.
type DraftConnection struct {
	Edges      []*DraftEdge `json:"edges"`
	PageInfo   *PageInfo    `json:"pageInfo"`
	TotalCount int          `json:"totalCount"`
}

// Revision is an immutable snapshot of content at a specific version.
type Revision struct {
	ID            string             `json:"id"`
	ObjectID      string             `json:"objectId"`
	Version       int                `json:"version"`
	Content       string             `json:"content"`
	MetadataJSON  *string            `json:"metadataJson"`
	ChangedBy     *activitypub.Actor `json:"changedBy"`
	ChangeSummary *string            `json:"changeSummary"`
	ChangeType    ChangeType         `json:"changeType"`
	GeneratedBy   *activitypub.Actor `json:"generatedBy"`
	ReviewedBy    *activitypub.Actor `json:"reviewedBy"`
	PublishedBy   *activitypub.Actor `json:"publishedBy"`
	CreatedAt     Time               `json:"createdAt"`
}

// RevisionEdge is an edge in a RevisionConnection.
type RevisionEdge struct {
	Node   *Revision `json:"node"`
	Cursor Cursor    `json:"cursor"`
}

// RevisionConnection is a paginated list of revisions.
type RevisionConnection struct {
	Edges      []*RevisionEdge `json:"edges"`
	PageInfo   *PageInfo       `json:"pageInfo"`
	TotalCount int             `json:"totalCount"`
}

// Series groups related articles under a shared title and description.
type Series struct {
	ID     string             `json:"id"`
	Author *activitypub.Actor `json:"author"`

	Title         string  `json:"title"`
	Description   *string `json:"description"`
	Slug          string  `json:"slug"`
	CoverImageURL *string `json:"coverImageUrl"`

	IsComplete   bool `json:"isComplete"`
	ArticleCount int  `json:"articleCount"`

	CreatedAt Time `json:"createdAt"`
	UpdatedAt Time `json:"updatedAt"`
}

// SeriesEdge is an edge in a SeriesConnection.
type SeriesEdge struct {
	Node   *Series `json:"node"`
	Cursor Cursor  `json:"cursor"`
}

// SeriesConnection is a paginated list of series.
type SeriesConnection struct {
	Edges      []*SeriesEdge `json:"edges"`
	PageInfo   *PageInfo     `json:"pageInfo"`
	TotalCount int           `json:"totalCount"`
}

// Category is a taxonomy label that can be applied to articles.
type Category struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	Description *string     `json:"description"`
	Parent      *Category   `json:"parent"`
	Children    []*Category `json:"children"`

	ArticleCount int     `json:"articleCount"`
	Order        int     `json:"order"`
	Color        *string `json:"color"`

	CreatedAt Time `json:"createdAt"`
	UpdatedAt Time `json:"updatedAt"`
}

// Publication is a group/brand that can own articles and members.
type Publication struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Tagline     *string `json:"tagline"`
	Description *string `json:"description"`
	Slug        string  `json:"slug"`

	LogoURL      *string `json:"logoUrl"`
	BannerURL    *string `json:"bannerUrl"`
	CustomDomain *string `json:"customDomain"`

	Actor   *activitypub.Actor   `json:"actor"`
	Members []*PublicationMember `json:"members"`

	CreatedAt Time `json:"createdAt"`
	UpdatedAt Time `json:"updatedAt"`
}

// PublicationMember associates a user with a role in a publication.
type PublicationMember struct {
	User        *activitypub.Actor `json:"user"`
	Role        PublicationRole    `json:"role"`
	DisplayName *string            `json:"displayName"`
	Bio         *string            `json:"bio"`
	JoinedAt    Time               `json:"joinedAt"`
}

// CMS input types

// CreateDraftInput is the input payload for creating a draft.
type CreateDraftInput struct {
	ContentType   ObjectType    `json:"contentType"`
	Title         *string       `json:"title"`
	Slug          *string       `json:"slug"`
	Content       string        `json:"content"`
	ContentFormat ContentFormat `json:"contentFormat"`
	ObjectID      *string       `json:"objectId"`
}

// UpdateDraftInput is the input payload for updating a draft.
type UpdateDraftInput struct {
	Title         *string        `json:"title"`
	Slug          *string        `json:"slug"`
	Content       *string        `json:"content"`
	ContentFormat *ContentFormat `json:"contentFormat"`
}

// CreateArticleInput is the input payload for creating an article.
type CreateArticleInput struct {
	Slug          *string       `json:"slug"`
	Title         string        `json:"title"`
	Content       string        `json:"content"`
	ContentFormat ContentFormat `json:"contentFormat"`

	Subtitle        *string `json:"subtitle"`
	Excerpt         *string `json:"excerpt"`
	FeaturedImageID *string `json:"featuredImageId"`

	SeriesID    *string  `json:"seriesId"`
	SeriesOrder *int     `json:"seriesOrder"`
	CategoryIDs []string `json:"categoryIds"`

	SEOTitle       *string `json:"seoTitle"`
	SEODescription *string `json:"seoDescription"`
	CanonicalURL   *string `json:"canonicalUrl"`
	OGImage        *string `json:"ogImage"`

	EditorNotes  *string `json:"editorNotes"`
	ReviewStatus *string `json:"reviewStatus"`
}

// UpdateArticleInput is the input payload for updating an article.
type UpdateArticleInput struct {
	Slug          *string        `json:"slug"`
	Title         *string        `json:"title"`
	Content       *string        `json:"content"`
	ContentFormat *ContentFormat `json:"contentFormat"`

	Subtitle        *string `json:"subtitle"`
	Excerpt         *string `json:"excerpt"`
	FeaturedImageID *string `json:"featuredImageId"`

	SeriesID    *string  `json:"seriesId"`
	SeriesOrder *int     `json:"seriesOrder"`
	CategoryIDs []string `json:"categoryIds"`

	SEOTitle       *string `json:"seoTitle"`
	SEODescription *string `json:"seoDescription"`
	CanonicalURL   *string `json:"canonicalUrl"`
	OGImage        *string `json:"ogImage"`

	EditorNotes  *string `json:"editorNotes"`
	ReviewStatus *string `json:"reviewStatus"`
}

// CreateSeriesInput is the input payload for creating a series.
type CreateSeriesInput struct {
	Slug          *string `json:"slug"`
	Title         string  `json:"title"`
	Description   *string `json:"description"`
	CoverImageURL *string `json:"coverImageUrl"`
	IsComplete    *bool   `json:"isComplete"`
}

// UpdateSeriesInput is the input payload for updating a series.
type UpdateSeriesInput struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	CoverImageURL *string `json:"coverImageUrl"`
	IsComplete    *bool   `json:"isComplete"`
}

// CreateCategoryInput is the input payload for creating a category.
type CreateCategoryInput struct {
	Slug        *string `json:"slug"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	ParentID    *string `json:"parentId"`
	Color       *string `json:"color"`
	Order       *int    `json:"order"`
}

// UpdateCategoryInput is the input payload for updating a category.
type UpdateCategoryInput struct {
	Slug        *string `json:"slug"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	ParentID    *string `json:"parentId"`
	Color       *string `json:"color"`
	Order       *int    `json:"order"`
}

// CreatePublicationInput is the input payload for creating a publication.
type CreatePublicationInput struct {
	Slug         *string `json:"slug"`
	Name         string  `json:"name"`
	Tagline      *string `json:"tagline"`
	Description  *string `json:"description"`
	LogoID       *string `json:"logoId"`
	BannerID     *string `json:"bannerId"`
	CustomDomain *string `json:"customDomain"`
}

// UpdatePublicationInput is the input payload for updating a publication.
type UpdatePublicationInput struct {
	Slug         *string `json:"slug"`
	Name         *string `json:"name"`
	Tagline      *string `json:"tagline"`
	Description  *string `json:"description"`
	LogoID       *string `json:"logoId"`
	BannerID     *string `json:"bannerId"`
	CustomDomain *string `json:"customDomain"`
}
