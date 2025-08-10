package lift

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
)

// TestActivityPubCollectionStructure tests the ActivityPub collection structures
func TestActivityPubCollectionStructure(t *testing.T) {
	// Test OrderedCollection structure
	collection := &activitypub.OrderedCollection{
		Collection: activitypub.Collection{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				ID:      "https://example.com/users/testuser/followers",
				Type:    activitypub.OrderedCollectionType,
			},
			TotalItems: 42,
		},
	}
	collection.First = "https://example.com/users/testuser/followers?page=1"

	// Verify structure
	assert.Equal(t, activitypub.OrderedCollectionType, collection.Type)
	assert.Equal(t, "https://example.com/users/testuser/followers", collection.ID)
	assert.Equal(t, 42, collection.TotalItems)
	assert.Equal(t, "https://example.com/users/testuser/followers?page=1", collection.First)
	assert.NotNil(t, collection.Context)
}

// TestActivityPubCollectionPageStructure tests the OrderedCollectionPage structure
func TestActivityPubCollectionPageStructure(t *testing.T) {
	// Test OrderedCollectionPage structure
	orderedItems := []any{
		"https://example.com/users/alice",
		"https://example.com/users/bob",
		"https://example.com/users/charlie",
	}

	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      "https://example.com/users/testuser/followers?page=1",
					Type:    activitypub.OrderedCollectionPageType,
				},
				OrderedItems: orderedItems,
			},
			PartOf: "https://example.com/users/testuser/followers",
		},
	}
	page.Next = "https://example.com/users/testuser/followers?page=1&cursor=xyz&limit=20"

	// Verify structure
	assert.Equal(t, activitypub.OrderedCollectionPageType, page.Type)
	assert.Equal(t, "https://example.com/users/testuser/followers?page=1", page.ID)
	assert.Equal(t, "https://example.com/users/testuser/followers", page.PartOf)
	assert.Equal(t, orderedItems, page.OrderedItems)
	assert.Equal(t, "https://example.com/users/testuser/followers?page=1&cursor=xyz&limit=20", page.Next)
	assert.NotNil(t, page.Context)
}

// TestActivityPubCollectionConstants verifies ActivityPub type constants
func TestActivityPubCollectionConstants(t *testing.T) {
	// Test that we're using the correct ActivityPub type constants
	assert.Equal(t, "OrderedCollection", activitypub.OrderedCollectionType)
	assert.Equal(t, "OrderedCollectionPage", activitypub.OrderedCollectionPageType)
}

// TestEmptyCollection tests the structure of empty collections for privacy
func TestEmptyCollection(t *testing.T) {
	// Test empty collection structure (for private accounts)
	emptyCollection := &activitypub.OrderedCollection{
		Collection: activitypub.Collection{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				ID:      "https://example.com/users/privateuser/followers",
				Type:    activitypub.OrderedCollectionType,
			},
			TotalItems: 0,
		},
	}

	// Verify empty collection structure
	assert.Equal(t, activitypub.OrderedCollectionType, emptyCollection.Type)
	assert.Equal(t, "https://example.com/users/privateuser/followers", emptyCollection.ID)
	assert.Equal(t, 0, emptyCollection.TotalItems)
	assert.Empty(t, emptyCollection.First) // No first page for empty collection
	assert.NotNil(t, emptyCollection.Context)
}