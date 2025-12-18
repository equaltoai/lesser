package models

import (
	"fmt"
	"time"
)

// Category represents a content category (hierarchical)
type Category struct {
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

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing Category.
func (Category) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the Category model
func (c *Category) UpdateKeys() error {
	if c.ID == "" {
		return fmt.Errorf("ID is required")
	}

	c.PK = "INSTANCE#CATEGORY"
	c.SK = fmt.Sprintf("ID#%s", c.ID)

	if c.ParentID != nil && *c.ParentID != "" {
		c.GSI1PK = fmt.Sprintf("CATEGORY#%s", *c.ParentID)
	} else {
		c.GSI1PK = "CATEGORY#ROOT"
	}
	c.GSI1SK = fmt.Sprintf("ID#%s", c.ID)

	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now

	return nil
}

// GetPK returns the partition key
func (c *Category) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *Category) GetSK() string {
	return c.SK
}
