package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Domain constants
const (
	domainLocal = "local"
)

// EmojiModel represents a custom emoji with DynamORM tags
type EmojiModel struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK" json:"-"` // EMOJI#shortcode
	SK string `dynamorm:"sk,attr:SK" json:"-"` // EMOJI

	// GSI keys - for querying all emojis
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"-"` // ALL_EMOJIS
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"-"` // EMOJI#shortcode

	// GSI keys - for querying by category
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"-"` // CATEGORY#category
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"-"` // EMOJI#shortcode

	// GSI keys - for search and domain queries
	GSI3PK string `dynamorm:"index:gsi3,pk,attr:gsi3PK" json:"-"` // SEARCH#prefix or DOMAIN#domain
	GSI3SK string `dynamorm:"index:gsi3,sk,attr:gsi3SK" json:"-"` // EMOJI#shortcode

	// GSI keys - for usage statistics and popularity
	GSI4PK string `dynamorm:"index:gsi4,pk,attr:gsi4PK" json:"-"` // USAGE#domain
	GSI4SK string `dynamorm:"index:gsi4,sk,attr:gsi4SK" json:"-"` // SCORE#usage_count#shortcode

	// Business fields
	Shortcode           string    `dynamorm:"attr:shortcode" json:"shortcode"`
	URL                 string    `dynamorm:"attr:url" json:"url"`
	StaticURL           string    `dynamorm:"attr:staticURL" json:"static_url"`
	VisibleInPicker     bool      `dynamorm:"attr:visibleInPicker" json:"visible_in_picker"`
	Category            string    `dynamorm:"attr:category" json:"category,omitempty"`
	CreatedAt           time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt           time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
	Disabled            bool      `dynamorm:"attr:disabled" json:"disabled"`
	Domain              string    `dynamorm:"attr:domain" json:"domain,omitempty"` // Empty for local emojis
	ImageRemoteURL      string    `dynamorm:"attr:imageRemoteURL" json:"image_remote_url,omitempty"`
	ImageStorageVersion int       `dynamorm:"attr:imageStorageVersion" json:"image_storage_version"`
	ImageFileSize       int64     `dynamorm:"attr:imageFileSize" json:"image_file_size"`
	ImageContentType    string    `dynamorm:"attr:imageContentType" json:"image_content_type"`
	ImageWidth          int       `dynamorm:"attr:imageWidth" json:"image_width"`
	ImageHeight         int       `dynamorm:"attr:imageHeight" json:"image_height"`
	ImageUpdatedAt      time.Time `dynamorm:"attr:imageUpdatedAt" json:"image_updated_at"`

	// Enhanced fields for sophisticated queries
	UsageCount      int64     `dynamorm:"attr:usageCount" json:"usage_count"`           // How many times this emoji has been used
	LastUsedAt      time.Time `dynamorm:"attr:lastUsedAt" json:"last_used_at"`          // When this emoji was last used
	PopularityScore float64   `dynamorm:"attr:popularityScore" json:"popularity_score"` // Calculated popularity score
	SearchKeywords  []string  `dynamorm:"attr:searchKeywords" json:"search_keywords"`   // Additional search terms
	AltText         string    `dynamorm:"attr:altText" json:"alt_text,omitempty"`       // Alternative text for accessibility
}

// GetPK returns the partition key
func (e *EmojiModel) GetPK() string {
	return e.PK
}

// GetSK returns the sort key
func (e *EmojiModel) GetSK() string {
	return e.SK
}

// UpdateKeys updates the composite keys based on the business fields
func (e *EmojiModel) UpdateKeys() error {
	// Primary keys
	if e.Domain != "" {
		// Remote emoji: include domain in primary key
		e.PK = fmt.Sprintf("EMOJI#%s@%s", e.Shortcode, e.Domain)
	} else {
		// Local emoji
		e.PK = fmt.Sprintf("EMOJI#%s", e.Shortcode)
	}
	e.SK = "EMOJI"

	// GSI1 - for querying all emojis
	e.GSI1PK = "ALL_EMOJIS"
	e.GSI1SK = fmt.Sprintf("EMOJI#%s", e.Shortcode)

	// GSI2 - for querying by category
	if e.Category != "" {
		e.GSI2PK = fmt.Sprintf("CATEGORY#%s", e.Category)
		e.GSI2SK = fmt.Sprintf("EMOJI#%s", e.Shortcode)
	}

	// GSI3 - for search and domain queries
	if e.Domain != "" {
		// For remote emojis, enable domain-based queries
		e.GSI3PK = fmt.Sprintf("DOMAIN#%s", e.Domain)
		e.GSI3SK = fmt.Sprintf("EMOJI#%s", e.Shortcode)
	} else {
		// For local emojis, enable prefix-based search
		prefix := e.generateSearchPrefix()
		e.GSI3PK = fmt.Sprintf("SEARCH#%s", prefix)
		e.GSI3SK = fmt.Sprintf("EMOJI#%s", e.Shortcode)
	}

	// GSI4 - for usage statistics and popularity
	domainKey := e.Domain
	if err := common.ValidateRequiredParam("domainKey", domainKey); err != nil {
		domainKey = domainLocal
	}
	e.GSI4PK = fmt.Sprintf("USAGE#%s", domainKey)
	// Sort by usage count (padded for lexicographic sorting) + shortcode
	e.GSI4SK = fmt.Sprintf("SCORE#%010d#%s", e.UsageCount, e.Shortcode)

	return nil
}

// generateSearchPrefix creates a search prefix from the shortcode for efficient prefix queries
func (e *EmojiModel) generateSearchPrefix() string {
	if len(e.Shortcode) >= 3 {
		return strings.ToLower(e.Shortcode[:3])
	}
	return strings.ToLower(e.Shortcode)
}

// IncrementUsage increments the usage count and updates last used time
func (e *EmojiModel) IncrementUsage() {
	e.UsageCount++
	e.LastUsedAt = time.Now()
	// Recalculate popularity score (simple algorithm)
	e.PopularityScore = e.calculatePopularityScore()
	// Update keys to reflect new usage count
	_ = e.UpdateKeys() // Ignore error for backwards compatibility
}

// calculatePopularityScore calculates a popularity score based on usage and recency
func (e *EmojiModel) calculatePopularityScore() float64 {
	if e.UsageCount == 0 {
		return 0.0
	}

	// Base score from usage count (logarithmic to prevent outliers)
	baseScore := float64(e.UsageCount)
	if baseScore > 1 {
		baseScore = 1.0 + (baseScore-1.0)*0.1 // Diminishing returns
	}

	// Recency bonus (emojis used recently get higher scores)
	if !e.LastUsedAt.IsZero() {
		daysSinceLastUse := time.Since(e.LastUsedAt).Hours() / 24
		if daysSinceLastUse < 7 {
			recencyBonus := (7 - daysSinceLastUse) / 7 * 0.5
			baseScore += recencyBonus
		}
	}

	return baseScore
}

// TableName returns the DynamoDB table backing EmojiModel.
func (EmojiModel) TableName() string {
	return MainTableName
}
