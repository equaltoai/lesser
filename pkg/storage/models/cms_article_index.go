package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	// CMSArticleIndexSKPrefix is the prefix used for sortable published-time keys.
	CMSArticleIndexSKPrefix = "TIME#"

	cmsArticleIndexAuthorPKPrefix   = "CMS#ARTICLE#AUTHOR#"
	cmsArticleIndexSeriesPKPrefix   = "CMS#ARTICLE#SERIES#"
	cmsArticleIndexCategoryPKPrefix = "CMS#ARTICLE#CATEGORY#"

	cmsArticleIndexSKArticleMarker = "#ARTICLE#"
)

// CMSArticleIndex stores an indexed view of articles grouped by author/series/category.
// It enables efficient queries without scans (single-table design).
type CMSArticleIndex struct {
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	ArticleID   string    `theorydb:"attr:articleID" json:"article_id"`
	PublishedAt time.Time `theorydb:"attr:publishedAt" json:"published_at"`
	CreatedAt   time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table backing CMSArticleIndex.
func (CMSArticleIndex) TableName() string {
	return MainTableName
}

// UpdateKeys validates required keys and sets timestamps.
func (i *CMSArticleIndex) UpdateKeys() error {
	if strings.TrimSpace(i.PK) == "" {
		return fmt.Errorf("PK is required")
	}
	if strings.TrimSpace(i.SK) == "" {
		return fmt.Errorf("SK is required")
	}

	now := time.Now()
	if i.CreatedAt.IsZero() {
		i.CreatedAt = now
	}

	return nil
}

// GetPK returns the partition key.
func (i *CMSArticleIndex) GetPK() string { return i.PK }

// GetSK returns the sort key.
func (i *CMSArticleIndex) GetSK() string { return i.SK }

// CMSArticleIndexPKForAuthor returns the PK for author-indexed articles (grouped by actor ID).
func CMSArticleIndexPKForAuthor(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}
	return cmsArticleIndexAuthorPKPrefix + actorID
}

// CMSArticleIndexPKForSeries returns the PK for series-indexed articles (grouped by GraphQL series ID).
func CMSArticleIndexPKForSeries(seriesID string) string {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return ""
	}
	return cmsArticleIndexSeriesPKPrefix + seriesID
}

// CMSArticleIndexPKForCategory returns the PK for category-indexed articles (grouped by category ID).
func CMSArticleIndexPKForCategory(categoryID string) string {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return ""
	}
	return cmsArticleIndexCategoryPKPrefix + categoryID
}

// CMSArticleIndexSK returns the sortable SK for an article based on its published time and ID.
func CMSArticleIndexSK(published time.Time, articleID string) string {
	articleID = strings.TrimSpace(articleID)
	if articleID == "" {
		return ""
	}
	if published.IsZero() {
		published = time.Now()
	}
	return fmt.Sprintf("%s%s%s%s", CMSArticleIndexSKPrefix, published.UTC().Format(time.RFC3339Nano), cmsArticleIndexSKArticleMarker, articleID)
}

// CMSArticleIndexExtractArticleID extracts the article ID from an index SK.
func CMSArticleIndexExtractArticleID(sk string) string {
	sk = strings.TrimSpace(sk)
	if sk == "" {
		return ""
	}
	idx := strings.LastIndex(sk, cmsArticleIndexSKArticleMarker)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(sk[idx+len(cmsArticleIndexSKArticleMarker):])
}
