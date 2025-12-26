# Lesser Headless Publishing System Design Document

## Executive Summary

This document outlines the design for evolving Lesser from a Mastodon-compatible ActivityPub server into a **headless publishing system** that supports multiple content modes while maintaining full federation capabilities. The goal is to enable Lesser to power diverse use cases—from social micro-blogging to long-form CMS applications—through a unified, mode-aware architecture.

### Vision

Lesser becomes the **universal ActivityPub backend** for:
- **Social Mode**: Mastodon-compatible micro-blogging (current)
- **CMS Mode**: Long-form publishing with editorial workflows
- **Hybrid Mode**: Social + CMS features for publication/newsletter platforms
- **Custom Modes**: Extensible foundation for future use cases (podcasts, portfolios, etc.)

---

## Current State Analysis

### Existing Capabilities

| Capability | Status | Notes |
|------------|--------|-------|
| ActivityPub Federation | ✅ Full | Inbox/Outbox, HTTP signatures, WebFinger |
| Object Types | ✅ Partial | `Note` fully supported, `Article` type exists but unused |
| GraphQL API | ✅ 60+ ops | Comprehensive query/mutation/subscription support |
| Mastodon REST API | ✅ Full | v1/v2 compatibility |
| Scheduled Posts | ✅ Basic | `ScheduledStatus` model with future publish dates |
| Media Handling | ✅ Full | S3 + CloudFront CDN with processing pipeline |
| Multi-Tenant | ✅ Full | Tenant isolation via partition key prefixing |
| Cost Tracking | ✅ Full | Per-operation cost calculation and budgets |

### Gaps for CMS Use Cases

| Gap | Impact | Effort |
|-----|--------|--------|
| No Draft System | Authors cannot save unpublished work | Medium |
| No Revision History | No version control for edits | Medium |
| Article type underutilized | No specialized handling for long-form | Low |
| No Publication/Author abstractions | Missing CMS organizational concepts | Medium |
| No Category/Series support | Content organization limited to tags | Low |
| No SEO metadata storage | No structured SEO data | Low |
| No canonical URL support | Cross-posting not tracked | Low |

---

## Design Principles

### 1. Mode-Agnostic Core

The storage layer and ActivityPub implementation remain mode-agnostic. Content modes are a **presentation concern** implemented via:
- Instance-level configuration
- Per-object metadata
- Mode-aware GraphQL resolvers

### 2. Additive Extensions

CMS features extend the existing data model without breaking changes:
- New GSI patterns for content organization
- Optional metadata fields on `Object`
- New models for drafts and revisions (non-breaking)

### 3. Federation-First

All content types federate naturally via ActivityPub:
- Articles → `Article` type with proper `name` (title)
- Drafts → Private visibility until published
- Comments → Standard `Note` replies with `inReplyTo`

### 4. Headless API Design

Lesser provides data, not presentation:
- GraphQL as primary API for CMS operations
- REST for Mastodon compatibility
- Frontend-agnostic data shapes

---

## Content Mode Architecture

### Instance Configuration

```yaml
# infra/cdk/config/production.yaml
instance:
  mode: hybrid                    # social | cms | hybrid
  features:
    socialPosting: true           # Enable Note creation
    longFormPublishing: true      # Enable Article creation
    scheduledPublishing: true     # Enable scheduled posts
    draftSystem: true             # Enable draft storage
    revisionHistory: true         # Enable version tracking
    editorialWorkflow: false      # Future: approval chains

  cms:
    defaultContentType: article   # article | note
    requireFeaturedImage: false
    enableSeries: true
    enableCategories: true
    maxRevisions: 50
    
  social:
    maxStatusChars: 5000
    enablePolls: true
    enableQuotes: true
```

### Content Type Hierarchy

```
ActivityPub Objects
├── Note (Social Mode primary)
│   ├── Short-form content (tweets, toots)
│   ├── Replies/comments
│   └── Quote posts
│
├── Article (CMS Mode primary)
│   ├── Long-form content (blog posts, documentation)
│   ├── Structured metadata (title, subtitle, featured image)
│   └── Extended fields (reading time, SEO, canonical URL)
│
└── Future Types
    ├── Audio (podcasts)
    ├── Video (vlogs)
    └── Event (calendaring)
```

---

## Implementation Patterns

This section provides references to existing Lesser patterns that **MUST** be followed for consistency. All CMS implementations should use these patterns.

### Repository Pattern (Reference: `pkg/storage/repositories/status_repository.go`)

All new repositories **MUST** extend `EnhancedBaseRepository`:

```go
// pkg/storage/repositories/draft_repository.go
package repositories

type DraftRepository struct {
    *EnhancedBaseRepository[*models.Draft]
}

func NewDraftRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *DraftRepository {
    enhancedRepo := NewEnhancedBaseRepository[*models.Draft](db, tableName, logger, costService, "DraftRepository", "draft")
    enhancedRepo.SetValidationService(NewDefaultValidationService())
    enhancedRepo.SetPermissionService(NewDefaultPermissionService())
    enhancedRepo.SetCachingService(NewInMemoryCachingService())
    enhancedRepo.SetEventService(NewDefaultEventService())
    
    return &DraftRepository{EnhancedBaseRepository: enhancedRepo}
}
```

### CRUD Operations (Reference: `pkg/storage/repositories/status_repository.go:67-349`)

**Create** - Use `ValidateAndCreate` from EnhancedBaseRepository:
```go
func (r *DraftRepository) CreateDraft(ctx context.Context, draft *models.Draft) error {
    // ValidateAndCreate handles validation, key setup, and event emission
    return r.ValidateAndCreate(ctx, draft)
}
```

**Read** - Use `Get` from BaseRepository:
```go
func (r *DraftRepository) GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error) {
    var draft models.Draft
    pk := fmt.Sprintf("USER#%s#DRAFT", authorID)
    sk := fmt.Sprintf("ID#%s", draftID)
    
    err := r.Get(ctx, pk, sk, &draft)
    if err != nil {
        return nil, err
    }
    return &draft, nil
}
```

**Update** - Use DynamORM UpdateBuilder for partial updates:
```go
func (r *DraftRepository) UpdateDraft(ctx context.Context, draft *models.Draft) error {
    return r.db.WithContext(ctx).Model(&models.Draft{}).
        Where("PK", "=", draft.PK).
        Where("SK", "=", draft.SK).
        UpdateBuilder().
        Set("Content", draft.Content).
        Set("Title", draft.Title).
        Set("UpdatedAt", time.Now()).
        Execute()
}
```

**Delete** - Use soft delete pattern:
```go
func (r *DraftRepository) DeleteDraft(ctx context.Context, authorID, draftID string) error {
    pk := fmt.Sprintf("USER#%s#DRAFT", authorID)
    sk := fmt.Sprintf("ID#%s", draftID)
    return r.Delete(ctx, pk, sk)
}
```

### Query Patterns (Reference: `pkg/storage/repositories/status_repository.go:209-234`)

**GSI Query with ordering:**
```go
func (r *DraftRepository) ListDraftsByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Draft, error) {
    var drafts []models.Draft
    err := r.db.WithContext(ctx).Model(&models.Draft{}).
        Index("GSI1").
        Where("gsi1PK", "=", fmt.Sprintf("AUTHOR#%s", authorID)).
        OrderBy("gsi1SK", "DESC").
        Limit(limit).
        All(&drafts)
    if err != nil {
        return nil, err
    }
    
    result := make([]*models.Draft, len(drafts))
    for i := range drafts {
        result[i] = &drafts[i]
    }
    return result, nil
}
```

### Service Pattern (Reference: `pkg/services/business_logic.go`)

Services receive repositories through the Registry:

```go
// pkg/services/cms/draft_service.go
package cms

type DraftService struct {
    draftRepo      *repositories.DraftRepository
    articleService *ArticleService
    domain         string
    logger         *zap.Logger
}

func NewDraftService(draftRepo *repositories.DraftRepository, articleService *ArticleService, domain string, logger *zap.Logger) *DraftService {
    return &DraftService{
        draftRepo:      draftRepo,
        articleService: articleService,
        domain:         domain,
        logger:         logger,
    }
}

func (s *DraftService) PublishDraft(ctx context.Context, authorID, draftID string) (*models.Article, error) {
    // 1. Get draft
    draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
    if err != nil {
        return nil, err
    }
    
    // 2. Convert to Article
    article := s.convertDraftToArticle(draft)
    
    // 3. Save article
    if err := s.articleService.CreateArticle(ctx, article); err != nil {
        return nil, err
    }
    
    // 4. Delete draft
    if err := s.draftRepo.DeleteDraft(ctx, authorID, draftID); err != nil {
        s.logger.Warn("failed to delete draft after publishing", zap.Error(err))
    }
    
    return article, nil
}
```

### Registry Integration (Reference: `pkg/services/registry.go`)

New services are added to the Registry with lazy initialization:

```go
// In pkg/services/registry.go, add:
type Registry struct {
    // ... existing fields ...
    draftService *cms.DraftService  // Add new service field
}

func (r *Registry) DraftService() *cms.DraftService {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if r.draftService == nil && r.storage != nil {
        draftRepo := r.storage.Draft()
        if draftRepo != nil {
            r.draftService = cms.NewDraftService(draftRepo, r.ArticleService(), r.logger)
            r.initialized["DraftService"] = true
        }
    }
    return r.draftService
}
```

---

## Data Model Extensions


### 1. Extended Object Model

The existing `Object` model gains optional CMS fields:

```go
// pkg/storage/models/object.go - Extended fields
type Object struct {
    // ... existing fields ...
    
    // CMS-specific metadata (optional, stored as JSON)
    ArticleMetadata *ArticleMetadata `dynamorm:"attr:articleMetadata" json:"article_metadata,omitempty"`
}

// ArticleMetadata contains CMS-specific fields for Article type objects
type ArticleMetadata struct {
    // Content structure
    Subtitle        string   `json:"subtitle,omitempty"`
    Excerpt         string   `json:"excerpt,omitempty"`         // Auto-generated or manual
    FeaturedImage   *Media   `json:"featured_image,omitempty"`
    TableOfContents []TOCEntry `json:"table_of_contents,omitempty"`
    
    // Publishing metadata
    ReadingTimeMinutes int       `json:"reading_time_minutes"`
    WordCount          int       `json:"word_count"`
    ContentFormat      string    `json:"content_format"`  // html, markdown, prosemirror
    
    // Organization
    SeriesID    *string  `json:"series_id,omitempty"`
    SeriesOrder *int     `json:"series_order,omitempty"`
    CategoryIDs []string `json:"category_ids,omitempty"`
    
    // SEO
    SEOTitle       string `json:"seo_title,omitempty"`
    SEODescription string `json:"seo_description,omitempty"`
    CanonicalURL   string `json:"canonical_url,omitempty"`
    OGImage        string `json:"og_image,omitempty"`
    
    // Editorial
    EditorNotes  string `json:"editor_notes,omitempty"`   // Internal notes
    ReviewStatus string `json:"review_status,omitempty"`  // draft, pending_review, approved
}

type TOCEntry struct {
    ID    string `json:"id"`
    Level int    `json:"level"`  // 1-6 for h1-h6
    Text  string `json:"text"`
}
```

### 2. Draft Model

Drafts are stored separately from published objects to enable:
- Autosave without affecting published content
- Multiple concurrent drafts
- Clear draft vs published distinction

```go
// pkg/storage/models/draft.go
package models

import (
    "fmt"
    "time"
)

// Draft represents an unpublished content draft
type Draft struct {
    _ struct{} `dynamorm:"naming:camelCase"`
    
    // Primary keys: USER#{author_id}#DRAFT / ID#{draft_id}
    PK string `dynamorm:"pk,attr:PK"`
    SK string `dynamorm:"sk,attr:SK"`
    
    // GSI1: Object drafts - OBJECT#{object_id}#DRAFT / TIME#{updated_at}
    // Allows finding all drafts for a specific published object
    GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK"`
    GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK"`
    
    // Core fields
    ID           string    `dynamorm:"attr:id" json:"id"`
    AuthorID     string    `dynamorm:"attr:authorID" json:"author_id"`
    ObjectID     *string   `dynamorm:"attr:objectID" json:"object_id,omitempty"` // nil = new, set = editing existing
    
    // Content
    ContentType  string    `dynamorm:"attr:contentType" json:"content_type"` // Note, Article
    Title        string    `dynamorm:"attr:title" json:"title,omitempty"`    // For Article
    Content      string    `dynamorm:"attr:content" json:"content"`
    ContentFormat string   `dynamorm:"attr:contentFormat" json:"content_format"` // html, markdown, prosemirror
    
    // Draft state
    Status       string    `dynamorm:"attr:status" json:"status"` // draft, scheduled, publishing, failed
    ScheduledAt  *time.Time `dynamorm:"attr:scheduledAt" json:"scheduled_at,omitempty"`
    
    // Metadata snapshot (full object metadata for preview)
    MetadataJSON string    `dynamorm:"attr:metadataJSON" json:"metadata_json,omitempty"`
    
    // Autosave tracking
    AutosaveVersion int       `dynamorm:"attr:autosaveVersion" json:"autosave_version"`
    LastSavedAt     time.Time `dynamorm:"attr:lastSavedAt" json:"last_saved_at"`
    
    // Timestamps
    CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

func (d *Draft) UpdateKeys() error {
    d.PK = fmt.Sprintf("USER#%s#DRAFT", d.AuthorID)
    d.SK = fmt.Sprintf("ID#%s", d.ID)
    
    if d.ObjectID != nil && *d.ObjectID != "" {
        d.GSI1PK = fmt.Sprintf("OBJECT#%s#DRAFT", *d.ObjectID)
    } else {
        d.GSI1PK = fmt.Sprintf("USER#%s#NEWDRAFT", d.AuthorID)
    }
    d.GSI1SK = fmt.Sprintf("TIME#%s", d.UpdatedAt.Format(time.RFC3339Nano))
    
    return nil
}

func (Draft) TableName() string {
    return MainTableName
}
```

### 3. Revision Model

Revisions track the history of published objects:

```go
// pkg/storage/models/revision.go
package models

import (
    "fmt"
    "time"
)

// Revision represents a historical version of a published object
type Revision struct {
    _ struct{} `dynamorm:"naming:camelCase"`
    
    // Primary keys: OBJECT#{object_id}#REVISION / VERSION#{version}
    PK string `dynamorm:"pk,attr:PK"`
    SK string `dynamorm:"sk,attr:SK"`
    
    // Core fields
    ID        string `dynamorm:"attr:id" json:"id"`         // Unique revision ID
    ObjectID  string `dynamorm:"attr:objectID" json:"object_id"`
    Version   int    `dynamorm:"attr:version" json:"version"`
    
    // Snapshot of content at this version
    Content      string `dynamorm:"attr:content" json:"content"`
    ContentHash  string `dynamorm:"attr:contentHash" json:"content_hash"` // SHA256 for deduplication
    MetadataJSON string `dynamorm:"attr:metadataJSON" json:"metadata_json"`
    
    // Change tracking
    ChangeSummary string    `dynamorm:"attr:changeSummary" json:"change_summary,omitempty"` // Optional commit message
    ChangedBy     string    `dynamorm:"attr:changedBy" json:"changed_by"`
    ChangeType    string    `dynamorm:"attr:changeType" json:"change_type"` // create, update, restore
    
    // Timestamps
    CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
}

func (r *Revision) UpdateKeys() error {
    r.PK = fmt.Sprintf("OBJECT#%s#REVISION", r.ObjectID)
    r.SK = fmt.Sprintf("VERSION#%08d", r.Version) // Zero-padded for sort order
    return nil
}

func (Revision) TableName() string {
    return MainTableName
}
```

### 4. Series & Category Models

```go
// pkg/storage/models/series.go
package models

// Series represents a multi-part content series (e.g., "JavaScript Fundamentals Part 1-10")
type Series struct {
    _ struct{} `dynamorm:"naming:camelCase"`
    
    PK string `dynamorm:"pk,attr:PK"` // AUTHOR#{author_id}#SERIES
    SK string `dynamorm:"sk,attr:SK"` // ID#{series_id}
    
    ID          string `dynamorm:"attr:id" json:"id"`
    AuthorID    string `dynamorm:"attr:authorID" json:"author_id"`
    Title       string `dynamorm:"attr:title" json:"title"`
    Description string `dynamorm:"attr:description" json:"description,omitempty"`
    Slug        string `dynamorm:"attr:slug" json:"slug"`
    CoverImage  string `dynamorm:"attr:coverImage" json:"cover_image,omitempty"`
    
    // Status
    IsComplete  bool `dynamorm:"attr:isComplete" json:"is_complete"`
    ArticleCount int `dynamorm:"attr:articleCount" json:"article_count"`
    
    // Timestamps
    CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// Category represents a content category (hierarchical)
type Category struct {
    _ struct{} `dynamorm:"naming:camelCase"`
    
    PK string `dynamorm:"pk,attr:PK"` // INSTANCE#CATEGORY
    SK string `dynamorm:"sk,attr:SK"` // ID#{category_id}
    
    // GSI: Parent lookup - CATEGORY#{parent_id} / ID#{category_id}
    GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK"`
    GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK"`
    
    ID          string  `dynamorm:"attr:id" json:"id"`
    Name        string  `dynamorm:"attr:name" json:"name"`
    Slug        string  `dynamorm:"attr:slug" json:"slug"`
    Description string  `dynamorm:"attr:description" json:"description,omitempty"`
    ParentID    *string `dynamorm:"attr:parentID" json:"parent_id,omitempty"`
    
    // Counts
    ArticleCount int `dynamorm:"attr:articleCount" json:"article_count"`
    
    // Display
    Order int    `dynamorm:"attr:order" json:"order"`
    Color string `dynamorm:"attr:color" json:"color,omitempty"` // Hex color for UI
}
```

### 5. Publication Model

For multi-author blogs or newsletter platforms:

```go
// pkg/storage/models/publication.go
package models

// Publication represents a blog/newsletter publication with multiple contributors
type Publication struct {
    _ struct{} `dynamorm:"naming:camelCase"`
    
    PK string `dynamorm:"pk,attr:PK"` // PUBLICATION#{id}
    SK string `dynamorm:"sk,attr:SK"` // METADATA
    
    ID          string `dynamorm:"attr:id" json:"id"`
    Name        string `dynamorm:"attr:name" json:"name"`
    Tagline     string `dynamorm:"attr:tagline" json:"tagline,omitempty"`
    Description string `dynamorm:"attr:description" json:"description,omitempty"`
    Slug        string `dynamorm:"attr:slug" json:"slug"`
    
    // Branding
    LogoURL   string `dynamorm:"attr:logoURL" json:"logo_url,omitempty"`
    BannerURL string `dynamorm:"attr:bannerURL" json:"banner_url,omitempty"`
    Theme     string `dynamorm:"attr:theme" json:"theme,omitempty"` // JSON theme config
    
    // Configuration
    CustomDomain string `dynamorm:"attr:customDomain" json:"custom_domain,omitempty"`
    
    // ActivityPub
    ActorID string `dynamorm:"attr:actorID" json:"actor_id"` // The AP Actor for this publication
    
    // Timestamps
    CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// PublicationMember represents a contributor to a publication
type PublicationMember struct {
    _ struct{} `dynamorm:"naming:camelCase"`
    
    PK string `dynamorm:"pk,attr:PK"` // PUBLICATION#{pub_id}#MEMBER
    SK string `dynamorm:"sk,attr:SK"` // USER#{user_id}
    
    PublicationID string `dynamorm:"attr:publicationID" json:"publication_id"`
    UserID        string `dynamorm:"attr:userID" json:"user_id"`
    Role          string `dynamorm:"attr:role" json:"role"` // owner, editor, writer, contributor
    
    // Display
    DisplayName string `dynamorm:"attr:displayName" json:"display_name,omitempty"` // Override user's name
    Bio         string `dynamorm:"attr:bio" json:"bio,omitempty"`                  // Publication-specific bio
    
    // Timestamps
    JoinedAt time.Time `dynamorm:"attr:joinedAt" json:"joined_at"`
}
```

---

## GraphQL API Extensions

### New Types

```graphql
# graph/cms.graphql - CMS-specific types

# Article extends Object with CMS metadata
type Article implements Object {
  # Standard Object fields
  id: ID!
  type: ObjectType!
  actor: Actor!
  content: String!
  visibility: Visibility!
  sensitive: Boolean!
  createdAt: Time!
  updatedAt: Time!
  
  # Article-specific fields
  title: String!
  subtitle: String
  excerpt: String
  featuredImage: Media
  tableOfContents: [TOCEntry!]!
  
  # Reading metadata
  readingTimeMinutes: Int!
  wordCount: Int!
  contentFormat: ContentFormat!
  
  # Organization
  series: Series
  seriesOrder: Int
  categories: [Category!]!
  
  # SEO
  seoTitle: String
  seoDescription: String
  canonicalUrl: String
  ogImage: String
  
  # Engagement
  repliesCount: Int!
  likesCount: Int!
  sharesCount: Int!
  bookmarkCount: Int!
}

enum ContentFormat {
  HTML
  MARKDOWN
}

type TOCEntry {
  id: String!
  level: Int!
  text: String!
}

# Draft management
type Draft {
  id: ID!
  author: Actor!
  
  # Content
  contentType: ObjectType!
  title: String
  content: String!
  contentFormat: ContentFormat!
  
  # State
  status: DraftStatus!
  scheduledAt: Time
  
  # Linked object (if editing existing)
  object: Object
  
  # Autosave
  autosaveVersion: Int!
  lastSavedAt: Time!
  
  # Preview
  preview: ArticlePreview
  
  createdAt: Time!
  updatedAt: Time!
}

enum DraftStatus {
  DRAFT
  SCHEDULED
  PUBLISHING
  PUBLISHED
  FAILED
}

type ArticlePreview {
  title: String!
  excerpt: String
  featuredImage: Media
  readingTimeMinutes: Int!
  wordCount: Int!
  tableOfContents: [TOCEntry!]!
}

# Revision history
type Revision {
  id: ID!
  version: Int!
  
  content: String!
  metadata: ArticleMetadata
  
  changeSummary: String
  changedBy: Actor!
  changeType: ChangeType!
  
  createdAt: Time!
}

enum ChangeType {
  CREATE
  UPDATE
  RESTORE
}

type RevisionConnection {
  edges: [RevisionEdge!]!
  pageInfo: PageInfo!
  totalCount: Int!
}

type RevisionEdge {
  node: Revision!
  cursor: Cursor!
}

# Series for multi-part content
type Series {
  id: ID!
  author: Actor!
  
  title: String!
  description: String
  slug: String!
  coverImage: Media
  
  isComplete: Boolean!
  articles: [Article!]!
  articleCount: Int!
  
  createdAt: Time!
  updatedAt: Time!
}

# Categories for organization
type Category {
  id: ID!
  name: String!
  slug: String!
  description: String
  parent: Category
  children: [Category!]!
  
  articleCount: Int!
  order: Int!
  color: String
}

# Publication (multi-author blog)
type Publication {
  id: ID!
  name: String!
  tagline: String
  description: String
  slug: String!
  
  logoUrl: String
  bannerUrl: String
  customDomain: String
  
  # ActivityPub
  actor: Actor!
  
  # Members
  members: [PublicationMember!]!
  
  # Content
  articles(first: Int, after: Cursor): ArticleConnection!
  
  createdAt: Time!
  updatedAt: Time!
}

type PublicationMember {
  user: Actor!
  role: PublicationRole!
  displayName: String
  bio: String
  joinedAt: Time!
}

enum PublicationRole {
  OWNER
  EDITOR
  WRITER
  CONTRIBUTOR
}
```

### New Queries

```graphql
extend type Query {
  # Draft operations
  draft(id: ID!): Draft
  myDrafts(
    contentType: ObjectType
    status: DraftStatus
    first: Int
    after: Cursor
  ): DraftConnection!
  
  # Revision history
  revisions(objectId: ID!, first: Int, after: Cursor): RevisionConnection!
  revision(objectId: ID!, version: Int!): Revision
  
  # Article queries
  article(id: ID!): Article
  articleBySlug(slug: String!): Article
  articles(
    authorId: ID
    seriesId: ID
    categoryId: ID
    tag: String
    first: Int
    after: Cursor
  ): ArticleConnection!
  
  # Series
  series(id: ID!): Series
  seriesBySlug(slug: String!): Series
  allSeries(authorId: ID, first: Int, after: Cursor): SeriesConnection!
  
  # Categories
  category(id: ID!): Category
  categoryBySlug(slug: String!): Category
  categories(parentId: ID): [Category!]!
  rootCategories: [Category!]!
  
  # Publication
  publication(id: ID!): Publication
  publicationBySlug(slug: String!): Publication
  myPublications: [Publication!]!
}
```

### New Mutations

```graphql
extend type Mutation {
  # Draft lifecycle
  createDraft(input: CreateDraftInput!): Draft!
  updateDraft(id: ID!, input: UpdateDraftInput!): Draft!
  autosaveDraft(id: ID!, content: String!): Draft!
  deleteDraft(id: ID!): Boolean!
  
  # Publishing
  publishDraft(id: ID!): Article!
  scheduleDraft(id: ID!, scheduledAt: Time!): Draft!
  cancelScheduledDraft(id: ID!): Draft!
  
  # Article operations
  createArticle(input: CreateArticleInput!): CreateArticlePayload!
  updateArticle(id: ID!, input: UpdateArticleInput!): Article!
  
  # Revision operations
  restoreRevision(objectId: ID!, version: Int!): Article!
  compareRevisions(objectId: ID!, versionA: Int!, versionB: Int!): RevisionDiff!
  
  # Series management
  createSeries(input: CreateSeriesInput!): Series!
  updateSeries(id: ID!, input: UpdateSeriesInput!): Series!
  deleteSeries(id: ID!): Boolean!
  addArticleToSeries(seriesId: ID!, articleId: ID!, order: Int): Series!
  removeArticleFromSeries(seriesId: ID!, articleId: ID!): Series!
  reorderSeriesArticles(seriesId: ID!, articleIds: [ID!]!): Series!
  
  # Category management
  createCategory(input: CreateCategoryInput!): Category!
  updateCategory(id: ID!, input: UpdateCategoryInput!): Category!
  deleteCategory(id: ID!): Boolean!
  addArticleToCategory(categoryId: ID!, articleId: ID!): Article!
  removeArticleFromCategory(categoryId: ID!, articleId: ID!): Article!
  
  # Publication management
  createPublication(input: CreatePublicationInput!): Publication!
  updatePublication(id: ID!, input: UpdatePublicationInput!): Publication!
  invitePublicationMember(publicationId: ID!, userId: ID!, role: PublicationRole!): PublicationMember!
  removePublicationMember(publicationId: ID!, userId: ID!): Boolean!
  updatePublicationMemberRole(publicationId: ID!, userId: ID!, role: PublicationRole!): PublicationMember!
}

# Input types
input CreateDraftInput {
  contentType: ObjectType! = ARTICLE
  title: String
  content: String!
  contentFormat: ContentFormat! = MARKDOWN
  objectId: ID # Set if editing existing object
}

input UpdateDraftInput {
  title: String
  content: String
  contentFormat: ContentFormat
  metadata: ArticleMetadataInput
}

input CreateArticleInput {
  title: String!
  content: String!
  contentFormat: ContentFormat! = MARKDOWN
  
  subtitle: String
  excerpt: String
  featuredImageId: ID
  
  visibility: Visibility! = PUBLIC
  sensitive: Boolean = false
  spoilerText: String
  
  # Organization
  seriesId: ID
  seriesOrder: Int
  categoryIds: [ID!]
  tags: [String!]
  
  # SEO
  seoTitle: String
  seoDescription: String
  canonicalUrl: String
  
  # Scheduling
  scheduledAt: Time # If set, creates scheduled instead of immediate publish
}

input ArticleMetadataInput {
  subtitle: String
  excerpt: String
  featuredImageId: ID
  seriesId: ID
  seriesOrder: Int
  categoryIds: [ID!]
  seoTitle: String
  seoDescription: String
  canonicalUrl: String
  ogImage: String
}

input CreateSeriesInput {
  title: String!
  description: String
  slug: String
  coverImageId: ID
}

input CreateCategoryInput {
  name: String!
  slug: String
  description: String
  parentId: ID
  color: String
  order: Int
}

input CreatePublicationInput {
  name: String!
  tagline: String
  description: String
  slug: String
  logoId: ID
  bannerId: ID
}
```

### New Subscriptions

```graphql
extend type Subscription {
  # Draft autosave notifications
  draftUpdated(draftId: ID!): Draft!
  
  # Publication activity
  publicationArticlePublished(publicationId: ID!): Article!
}
```

---

## Service Layer

### New Services

```go
// pkg/services/cms/article_service.go
type ArticleService interface {
    // CRUD
    CreateArticle(ctx context.Context, input CreateArticleInput) (*Article, error)
    GetArticle(ctx context.Context, id string) (*Article, error)
    GetArticleBySlug(ctx context.Context, slug string) (*Article, error)
    UpdateArticle(ctx context.Context, id string, input UpdateArticleInput) (*Article, error)
    DeleteArticle(ctx context.Context, id string) error
    
    // Queries
    ListArticles(ctx context.Context, filter ArticleFilter, pagination Pagination) (*ArticleConnection, error)
    ListByAuthor(ctx context.Context, authorID string, pagination Pagination) (*ArticleConnection, error)
    ListBySeries(ctx context.Context, seriesID string) ([]*Article, error)
    ListByCategory(ctx context.Context, categoryID string, pagination Pagination) (*ArticleConnection, error)
    
    // Content processing
    ProcessContent(ctx context.Context, content string, format ContentFormat) (*ProcessedContent, error)
    GenerateExcerpt(ctx context.Context, content string, maxLength int) (string, error)
    GenerateTOC(ctx context.Context, content string) ([]TOCEntry, error)
    CalculateReadingTime(ctx context.Context, content string) (int, error)
}

// pkg/services/cms/draft_service.go
type DraftService interface {
    // CRUD
    CreateDraft(ctx context.Context, authorID string, input CreateDraftInput) (*Draft, error)
    GetDraft(ctx context.Context, id string) (*Draft, error)
    UpdateDraft(ctx context.Context, id string, input UpdateDraftInput) (*Draft, error)
    DeleteDraft(ctx context.Context, id string) error
    
    // Autosave
    Autosave(ctx context.Context, draftID string, content string) (*Draft, error)
    
    // Queries
    ListDrafts(ctx context.Context, authorID string, filter DraftFilter) ([]*Draft, error)
    ListDraftsForObject(ctx context.Context, objectID string) ([]*Draft, error)
    
    // Publishing
    PublishDraft(ctx context.Context, draftID string) (*Object, error)
    ScheduleDraft(ctx context.Context, draftID string, publishAt time.Time) (*Draft, error)
    CancelScheduledDraft(ctx context.Context, draftID string) (*Draft, error)
    
    // Preview
    GeneratePreview(ctx context.Context, draftID string) (*ArticlePreview, error)
}

// pkg/services/cms/revision_service.go
type RevisionService interface {
    // Create revision (called automatically on publish/update)
    CreateRevision(ctx context.Context, objectID string, content string, metadata map[string]any, changedBy string, changeType string) (*Revision, error)
    
    // Queries
    ListRevisions(ctx context.Context, objectID string, pagination Pagination) (*RevisionConnection, error)
    GetRevision(ctx context.Context, objectID string, version int) (*Revision, error)
    
    // Comparison
    CompareRevisions(ctx context.Context, objectID string, versionA, versionB int) (*RevisionDiff, error)
    
    // Restore
    RestoreRevision(ctx context.Context, objectID string, version int) (*Object, error)
    
    // Cleanup
    PruneOldRevisions(ctx context.Context, objectID string, keepCount int) error
}

// pkg/services/cms/series_service.go
type SeriesService interface {
    CreateSeries(ctx context.Context, authorID string, input CreateSeriesInput) (*Series, error)
    GetSeries(ctx context.Context, id string) (*Series, error)
    GetSeriesBySlug(ctx context.Context, slug string) (*Series, error)
    UpdateSeries(ctx context.Context, id string, input UpdateSeriesInput) (*Series, error)
    DeleteSeries(ctx context.Context, id string) error
    
    AddArticle(ctx context.Context, seriesID, articleID string, order int) error
    RemoveArticle(ctx context.Context, seriesID, articleID string) error
    ReorderArticles(ctx context.Context, seriesID string, articleIDs []string) error
    
    ListSeries(ctx context.Context, authorID string, pagination Pagination) ([]*Series, error)
}

// pkg/services/cms/publication_service.go
type PublicationService interface {
    CreatePublication(ctx context.Context, ownerID string, input CreatePublicationInput) (*Publication, error)
    GetPublication(ctx context.Context, id string) (*Publication, error)
    GetPublicationBySlug(ctx context.Context, slug string) (*Publication, error)
    UpdatePublication(ctx context.Context, id string, input UpdatePublicationInput) (*Publication, error)
    DeletePublication(ctx context.Context, id string) error
    
    // Members
    InviteMember(ctx context.Context, publicationID, userID string, role PublicationRole) error
    RemoveMember(ctx context.Context, publicationID, userID string) error
    UpdateMemberRole(ctx context.Context, publicationID, userID string, role PublicationRole) error
    ListMembers(ctx context.Context, publicationID string) ([]*PublicationMember, error)
    
    // Content
    ListArticles(ctx context.Context, publicationID string, pagination Pagination) (*ArticleConnection, error)
    PublishToPublication(ctx context.Context, publicationID, articleID string) error
}
```

---

## ActivityPub Federation

### Article Federation

Articles federate as ActivityPub `Article` objects:

```json
{
  "@context": [
    "https://www.w3.org/ns/activitystreams",
    {
      "lesser": "https://lesser.dev/ns#",
      "readingTime": "lesser:readingTime",
      "wordCount": "lesser:wordCount",
      "series": "lesser:series",
      "featuredImage": "lesser:featuredImage"
    }
  ],
  "id": "https://example.com/articles/hello-world",
  "type": "Article",
  "attributedTo": "https://example.com/users/alice",
  "name": "Hello World: My First Post",
  "summary": "An introduction to my new blog...",
  "content": "<article>...</article>",
  "published": "2024-01-15T10:00:00Z",
  "updated": "2024-01-15T14:30:00Z",
  "to": ["https://www.w3.org/ns/activitystreams#Public"],
  "cc": ["https://example.com/users/alice/followers"],
  "tag": [
    {"type": "Hashtag", "name": "#introduction", "href": "https://example.com/tags/introduction"}
  ],
  "image": {
    "type": "Image",
    "url": "https://cdn.example.com/images/hello-world-hero.jpg",
    "mediaType": "image/jpeg"
  },
  "lesser:readingTime": 5,
  "lesser:wordCount": 1200,
  "lesser:series": {
    "id": "https://example.com/series/getting-started",
    "name": "Getting Started",
    "order": 1
  }
}
```

### Cross-Platform Compatibility

| Platform | Article Support | Rendering |
|----------|----------------|-----------|
| Mastodon | ✅ Displays | Shows as card with title/excerpt |
| Pleroma | ✅ Displays | Full article rendering |
| Misskey | ✅ Displays | Shows as expandable content |
| WordPress ActivityPub | ✅ Full | Native article format |
| Write.as | ✅ Full | Native article format |
| Ghost ActivityPub | ✅ Full | Native article format |

---

## Frontend Integration (Greater-Components)

### Blog Face Adapter

The `greater-components` Blog Face uses an adapter pattern. Lesser provides a ready-made adapter:

```typescript
// packages/adapters/src/lesser/blog-adapter.ts
import { Client, cacheExchange, fetchExchange } from '@urql/svelte';
import type { BlogAdapter, ArticleData, DraftData, AuthorData } from '@equaltoai/greater-components-blog';

export class LesserBlogAdapter implements BlogAdapter {
  private client: Client;
  
  constructor(endpoint: string, token?: string) {
    this.client = new Client({
      url: endpoint,
      exchanges: [cacheExchange, fetchExchange],
      fetchOptions: token ? { headers: { Authorization: `Bearer ${token}` } } : undefined
    });
  }
  
  // Article operations
  async getArticle(id: string): Promise<ArticleData | null> {
    const result = await this.client.query(GET_ARTICLE, { id }).toPromise();
    return result.data?.article ? this.mapArticle(result.data.article) : null;
  }
  
  async getArticleBySlug(slug: string): Promise<ArticleData | null> {
    const result = await this.client.query(GET_ARTICLE_BY_SLUG, { slug }).toPromise();
    return result.data?.articleBySlug ? this.mapArticle(result.data.articleBySlug) : null;
  }
  
  async listArticles(options: ListOptions): Promise<ArticleConnection> {
    const result = await this.client.query(LIST_ARTICLES, options).toPromise();
    return this.mapConnection(result.data?.articles);
  }
  
  // Draft operations
  async createDraft(input: CreateDraftInput): Promise<DraftData> {
    const result = await this.client.mutation(CREATE_DRAFT, { input }).toPromise();
    return this.mapDraft(result.data?.createDraft);
  }
  
  async updateDraft(id: string, input: UpdateDraftInput): Promise<DraftData> {
    const result = await this.client.mutation(UPDATE_DRAFT, { id, input }).toPromise();
    return this.mapDraft(result.data?.updateDraft);
  }
  
  async autosaveDraft(id: string, content: string): Promise<DraftData> {
    const result = await this.client.mutation(AUTOSAVE_DRAFT, { id, content }).toPromise();
    return this.mapDraft(result.data?.autosaveDraft);
  }
  
  async publishDraft(id: string): Promise<ArticleData> {
    const result = await this.client.mutation(PUBLISH_DRAFT, { id }).toPromise();
    return this.mapArticle(result.data?.publishDraft);
  }
  
  // Private mapping methods
  private mapArticle(raw: any): ArticleData {
    return {
      id: raw.id,
      slug: this.extractSlug(raw.id),
      metadata: {
        title: raw.title,
        subtitle: raw.subtitle,
        description: raw.excerpt,
        publishedAt: new Date(raw.createdAt),
        updatedAt: raw.updatedAt ? new Date(raw.updatedAt) : undefined,
        readingTime: raw.readingTimeMinutes,
        tags: raw.tags?.map((t: any) => t.name) ?? [],
        featuredImage: raw.featuredImage?.url
      },
      content: raw.content,
      contentFormat: raw.contentFormat.toLowerCase() as 'html' | 'markdown',
      author: this.mapAuthor(raw.actor),
      isPublished: true,
      tableOfContents: raw.tableOfContents
    };
  }
  
  private mapAuthor(raw: any): AuthorData {
    return {
      id: raw.id,
      name: raw.name || raw.preferredUsername,
      username: raw.preferredUsername,
      avatar: raw.icon?.url,
      bio: raw.summary
    };
  }
  
  private mapDraft(raw: any): DraftData {
    return {
      id: raw.id,
      title: raw.title,
      content: raw.content,
      contentFormat: raw.contentFormat.toLowerCase(),
      status: raw.status.toLowerCase(),
      lastSavedAt: new Date(raw.lastSavedAt),
      createdAt: new Date(raw.createdAt)
    };
  }
}
```

### Usage in paicodes

```svelte
<!-- src/routes/blog/[slug]/+page.svelte -->
<script lang="ts">
  import { Article } from '@equaltoai/greater-components-blog';
  import { LesserBlogAdapter } from '$lib/adapters/lesser';
  
  const adapter = new LesserBlogAdapter(
    import.meta.env.VITE_LESSER_GRAPHQL_ENDPOINT
  );
  
  export let data;
  let article = data.article;
</script>

<Article.Root {article} config={{ showTableOfContents: true }}>
  <Article.ReadingProgress />
  <Article.Header />
  <div class="article-layout">
    <Article.TableOfContents />
    <Article.Content />
  </div>
  <Article.ShareBar platforms={['twitter', 'linkedin', 'mastodon']} />
  <Article.Footer />
</Article.Root>
```

---

## Backward Compatibility

### Mastodon API

All existing Mastodon REST API endpoints continue to work unchanged:
- `POST /api/v1/statuses` creates Notes (unchanged)
- Articles appear in timelines as rich cards
- Cross-posting from Mastodon clients works

### GraphQL API

New types are additive:
- Existing queries/mutations unchanged
- `Object` union type extended to include `Article`
- New `article*` queries are additions

### Federation

- `Note` type handling unchanged
- `Article` type uses standard ActivityPub semantics
- Remote servers see articles as rich content

---

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Draft autosave latency | < 200ms p99 | CloudWatch latency metrics |
| Article publish time | < 2s end-to-end | X-Ray traces |
| Revision storage cost | < $0.01/100 revisions | Cost tracking |
| Federation delivery | > 99% success | Federation metrics |
| API compatibility | 100% existing tests pass | CI suite |

---

## Design Decisions

The following questions have been resolved:

### 1. Content Format
**Decision**: HTML and Markdown only.

Lesser has no existing integration with ProseMirror and will maintain support for HTML and Markdown content formats only. Rich text editors on the frontend should convert to these formats before storage.

```go
// Simplified ContentFormat enum (remove PROSEMIRROR)
enum ContentFormat {
  HTML
  MARKDOWN
}
```

### 2. Collaboration Model  
**Decision**: Single-author content ownership.

Content is always associated with a single author. Real-time collaborative editing is out of scope. The data model does not need to support multi-cursor or operational transforms.

- Each `Draft` has one `AuthorID`
- Each `Article` has one `AttributedTo` (ActivityPub actor)
- Publications enable multi-author *blogs*, but each article has a single author

### 3. Comments
**Decision**: Comments are standard `Note` replies.

Article comments use the existing `inReplyTo` mechanism for consistency with social mode:

- Comments are `Note` objects with `inReplyTo` pointing to the `Article`
- Leverages existing GSI6 indexing (`REPLIES#{parent_id}`)
- Uses existing `threads` service for traversal
- Federates naturally (Mastodon users can reply to articles)
- No separate `Comment` type needed

### 4. Content Distribution
**Decision**: Federation only, no email.

Lesser does not use email. Content distribution is purely through ActivityPub federation:

- No newsletter email delivery
- No subscriber email lists
- Followers receive articles via federation
- External services can subscribe via ActivityPub for email forwarding if needed

The `hasNewsletter` field on `Publication` is **removed** from the design.

### 5. Analytics & Engagement
**Decision**: Built-in engagement metrics are required.

Engagement metrics are just as important for articles as for social content:

- `repliesCount`, `likesCount`, `sharesCount`, `bookmarkCount` on `Article`
- View tracking via existing cost/metrics infrastructure
- Reading progress analytics (optional future enhancement)
- All metrics exposed via GraphQL API

Lesser already tracks engagement on `Object` types, so this extends naturally to `Article`.

---

## Appendix: Related ActivityPub Implementations

### Write.as (writeas)
- Uses `Article` type for long-form
- Custom extensions for blog metadata
- Full federation support

### Ghost ActivityPub
- Newsletter-first approach
- `Article` with rich metadata
- Subscriber federation

### WordPress ActivityPub Plugin
- Maps posts to `Article`
- Categories as hashtags
- Featured image in `image` property

### Plume (Rust)
- Dedicated blogging platform
- Full `Article` support
- Multi-author blogs

---

*Document Version: 1.0*
*Last Updated: 2024-12-17*
*Author: Lesser Development Team*
