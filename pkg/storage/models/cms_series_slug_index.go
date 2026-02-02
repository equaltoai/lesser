package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	cmsSeriesSlugIndexPKPrefix = "CMS#SERIES#SLUG#"
	cmsSeriesSlugIndexSK       = "REF"
)

// CMSSeriesSlugIndex maps a series slug to its owning author + series ID for efficient lookup.
type CMSSeriesSlugIndex struct {
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Slug     string `theorydb:"attr:slug" json:"slug"`
	AuthorID string `theorydb:"attr:authorID" json:"author_id"`
	SeriesID string `theorydb:"attr:seriesID" json:"series_id"`

	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing CMSSeriesSlugIndex.
func (CMSSeriesSlugIndex) TableName() string {
	return MainTableName
}

// UpdateKeys sets PK/SK based on the slug and stamps timestamps.
func (i *CMSSeriesSlugIndex) UpdateKeys() error {
	i.Slug = strings.TrimSpace(i.Slug)
	i.AuthorID = strings.TrimSpace(i.AuthorID)
	i.SeriesID = strings.TrimSpace(i.SeriesID)

	if i.Slug == "" {
		return fmt.Errorf("slug is required")
	}
	if i.AuthorID == "" {
		return fmt.Errorf("authorID is required")
	}
	if i.SeriesID == "" {
		return fmt.Errorf("seriesID is required")
	}

	i.PK = CMSSeriesSlugIndexPK(i.Slug)
	i.SK = cmsSeriesSlugIndexSK

	now := time.Now()
	if i.CreatedAt.IsZero() {
		i.CreatedAt = now
	}
	i.UpdatedAt = now

	return nil
}

// GetPK returns the partition key.
func (i *CMSSeriesSlugIndex) GetPK() string { return i.PK }

// GetSK returns the sort key.
func (i *CMSSeriesSlugIndex) GetSK() string { return i.SK }

// CMSSeriesSlugIndexPK returns the PK for a given series slug.
func CMSSeriesSlugIndexPK(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	return cmsSeriesSlugIndexPKPrefix + slug
}

// CMSSeriesSlugIndexSK returns the fixed SK for slug index entries.
func CMSSeriesSlugIndexSK() string { return cmsSeriesSlugIndexSK }
