package models

import (
	"fmt"
	"time"
)

// CollectionItem represents an item in an ActivityPub collection
type CollectionItem struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk,attr:PK" json:"PK"` // COLLECTION#{collection}
	SK string `dynamorm:"sk,attr:SK" json:"SK"` // ITEM#{itemID}

	// GSI1 for reverse lookups (what collections is an item in)
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1PK"` // ITEM#{itemID}
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1SK"` // COLLECTION#{collection}

	// Collection item data
	Collection string    `dynamorm:"attr:collection" json:"Collection"` // The collection ID (e.g., featured, likes, etc.)
	ItemID     string    `dynamorm:"attr:itemID" json:"ItemID"`         // The item being added/removed
	ItemType   string    `dynamorm:"attr:itemType" json:"ItemType"`     // Type of the item (Note, Article, etc.)
	AddedBy    string    `dynamorm:"attr:addedBy" json:"AddedBy"`       // Who added the item
	AddedAt    time.Time `dynamorm:"attr:addedAt" json:"AddedAt"`       // When it was added
	Position   int       `dynamorm:"attr:position" json:"Position"`     // Optional position in ordered collections
	CreatedAt  time.Time `dynamorm:"attr:createdAt" json:"CreatedAt"`   // Database timestamp

	// Optional TTL for cleanup
	TTL *int64 `dynamorm:"ttl,attr:ttl" json:"TTL,omitempty"`
}

// Common collection types
const (
	CollectionFollowers = "followers"
	CollectionFollowing = "following"
	CollectionFeatured  = "featured"
	CollectionLikes     = "likes"
	CollectionShares    = "shares"
)

// TableName returns the DynamoDB table name
func (CollectionItem) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the record before creation
func (c *CollectionItem) BeforeCreate() error {
	c.CreatedAt = time.Now()
	if c.AddedAt.IsZero() {
		c.AddedAt = c.CreatedAt
	}
	c.UpdateKeys()
	return nil
}

// UpdateKeys updates GSI keys based on collection and item
func (c *CollectionItem) UpdateKeys() {
	c.PK = fmt.Sprintf("COLLECTION#%s", c.Collection)
	c.SK = fmt.Sprintf("ITEM#%s", c.ItemID)
	c.GSI1PK = fmt.Sprintf("ITEM#%s", c.ItemID)
	c.GSI1SK = fmt.Sprintf("COLLECTION#%s", c.Collection)
}

// NewCollectionItem creates a new collection item
func NewCollectionItem(collection, itemID, itemType, addedBy string) *CollectionItem {
	now := time.Now()
	item := &CollectionItem{
		Collection: collection,
		ItemID:     itemID,
		ItemType:   itemType,
		AddedBy:    addedBy,
		AddedAt:    now,
		CreatedAt:  now,
	}
	item.UpdateKeys()
	return item
}

// SetPosition sets the position for ordered collections
func (c *CollectionItem) SetPosition(position int) {
	c.Position = position
}

// SetTTL sets the TTL for the collection item (in Unix epoch seconds)
func (c *CollectionItem) SetTTL(ttl time.Time) {
	ttlUnix := ttl.Unix()
	c.TTL = &ttlUnix
}

// ExtractCollection extracts the collection name from PK
func (c *CollectionItem) ExtractCollection() string {
	prefix := "COLLECTION#"
	if len(c.PK) > len(prefix) {
		return c.PK[len(prefix):]
	}
	return ""
}

// ExtractItemID extracts the item ID from SK
func (c *CollectionItem) ExtractItemID() string {
	prefix := "ITEM#"
	if len(c.SK) > len(prefix) {
		return c.SK[len(prefix):]
	}
	return ""
}
