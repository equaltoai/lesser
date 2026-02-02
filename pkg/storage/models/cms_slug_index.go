package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	cmsSlugIndexSK = "REF"

	cmsArticleSlugIndexPKPrefix     = "CMS#ARTICLE#SLUG#"
	cmsCategorySlugIndexPKPrefix    = "CMS#CATEGORY#SLUG#"
	cmsPublicationSlugIndexPKPrefix = "CMS#PUBLICATION#SLUG#"
)

// CMSSlugIndex maps a slug to a canonical object ID.
//
// PK determines the index namespace (article/category/publication) and includes the slug.
type CMSSlugIndex struct {
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Slug     string `theorydb:"attr:slug" json:"slug"`
	TargetID string `theorydb:"attr:targetID" json:"target_id"`

	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing CMSSlugIndex.
func (CMSSlugIndex) TableName() string {
	return MainTableName
}

// UpdateKeys stamps timestamps and validates required fields.
// Callers must set PK before persisting the record.
func (i *CMSSlugIndex) UpdateKeys() error {
	i.PK = strings.TrimSpace(i.PK)
	i.Slug = strings.TrimSpace(i.Slug)
	i.TargetID = strings.TrimSpace(i.TargetID)

	if i.PK == "" {
		return fmt.Errorf("PK is required")
	}
	if i.Slug == "" {
		return fmt.Errorf("slug is required")
	}
	if i.TargetID == "" {
		return fmt.Errorf("targetID is required")
	}

	i.SK = cmsSlugIndexSK

	now := time.Now()
	if i.CreatedAt.IsZero() {
		i.CreatedAt = now
	}
	i.UpdatedAt = now

	return nil
}

// GetPK returns the partition key.
func (i *CMSSlugIndex) GetPK() string { return i.PK }

// GetSK returns the sort key.
func (i *CMSSlugIndex) GetSK() string { return i.SK }

// CMSSlugIndexSK returns the fixed SK for slug index entries.
func CMSSlugIndexSK() string { return cmsSlugIndexSK }

// CMSArticleSlugIndexPK returns the PK for a given article slug.
func CMSArticleSlugIndexPK(slug string) string {
	return cmsSlugIndexPK(cmsArticleSlugIndexPKPrefix, slug)
}

// CMSCategorySlugIndexPK returns the PK for a given category slug.
func CMSCategorySlugIndexPK(slug string) string {
	return cmsSlugIndexPK(cmsCategorySlugIndexPKPrefix, slug)
}

// CMSPublicationSlugIndexPK returns the PK for a given publication slug.
func CMSPublicationSlugIndexPK(slug string) string {
	return cmsSlugIndexPK(cmsPublicationSlugIndexPKPrefix, slug)
}

// CMSArticleSlugIndexSK returns the fixed SK for article slug index entries.
func CMSArticleSlugIndexSK() string { return cmsSlugIndexSK }

// CMSCategorySlugIndexSK returns the fixed SK for category slug index entries.
func CMSCategorySlugIndexSK() string { return cmsSlugIndexSK }

// CMSPublicationSlugIndexSK returns the fixed SK for publication slug index entries.
func CMSPublicationSlugIndexSK() string { return cmsSlugIndexSK }

func cmsSlugIndexPK(prefix string, slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	return prefix + slug
}
