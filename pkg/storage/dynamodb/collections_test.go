package dynamodb_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectionOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	client := testutil.GetTestDynamoDBClient(t)

	t.Run("AddToCollection", func(t *testing.T) {
		item := &storage.CollectionItem{
			ItemID:   "https://example.com/posts/1",
			ItemType: "Note",
			AddedBy:  "https://example.com/users/alice",
		}

		err := client.AddToCollection(ctx, "featured", item)
		require.NoError(t, err)

		// Verify item was added
		assert.Equal(t, "featured", item.Collection)
		assert.NotZero(t, item.AddedAt)
	})

	t.Run("AddToCollection_Duplicate", func(t *testing.T) {
		// Add an item
		item1 := &storage.CollectionItem{
			ItemID:   "https://example.com/posts/2",
			ItemType: "Article",
			AddedBy:  "https://example.com/users/bob",
		}
		err := client.AddToCollection(ctx, "bookmarks", item1)
		require.NoError(t, err)

		// Try to add the same item again
		item2 := &storage.CollectionItem{
			ItemID:   "https://example.com/posts/2",
			ItemType: "Article",
			AddedBy:  "https://example.com/users/bob",
		}
		err = client.AddToCollection(ctx, "bookmarks", item2)
		assert.NoError(t, err) // Should succeed silently
	})

	t.Run("RemoveFromCollection", func(t *testing.T) {
		// Add an item first
		item := &storage.CollectionItem{
			ItemID:   "https://example.com/posts/3",
			ItemType: "Note",
			AddedBy:  "https://example.com/users/charlie",
		}
		err := client.AddToCollection(ctx, "likes", item)
		require.NoError(t, err)

		// Remove the item
		err = client.RemoveFromCollection(ctx, "likes", item.ItemID)
		require.NoError(t, err)

		// Verify it's gone
		exists, err := client.IsInCollection(ctx, "likes", item.ItemID)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("GetCollectionItems", func(t *testing.T) {
		collection := "test-collection"

		// Add multiple items
		for i := 0; i < 5; i++ {
			item := &storage.CollectionItem{
				ItemID:   fmt.Sprintf("https://example.com/posts/coll-%d", i),
				ItemType: "Note",
				AddedBy:  "https://example.com/users/dave",
			}
			// Add small delay to ensure different timestamps
			time.Sleep(10 * time.Millisecond)
			err := client.AddToCollection(ctx, collection, item)
			require.NoError(t, err)
		}

		// Get items
		items, cursor, err := client.GetCollectionItems(ctx, collection, 10, "")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(items), 5)
		assert.Empty(t, cursor) // Should not need pagination for 5 items

		// Verify items are in reverse chronological order
		for i := 1; i < len(items); i++ {
			assert.True(t, items[i-1].AddedAt.After(items[i].AddedAt) || items[i-1].AddedAt.Equal(items[i].AddedAt))
		}
	})

	t.Run("GetCollectionItems_Pagination", func(t *testing.T) {
		collection := "paginated-collection"

		// Add multiple items
		for i := 0; i < 7; i++ {
			item := &storage.CollectionItem{
				ItemID:   fmt.Sprintf("https://example.com/posts/page-%d", i),
				ItemType: "Article",
				AddedBy:  "https://example.com/users/eve",
			}
			err := client.AddToCollection(ctx, collection, item)
			require.NoError(t, err)
		}

		// Get first page
		items1, cursor1, err := client.GetCollectionItems(ctx, collection, 3, "")
		require.NoError(t, err)
		assert.Len(t, items1, 3)
		assert.NotEmpty(t, cursor1)

		// Get second page
		items2, cursor2, err := client.GetCollectionItems(ctx, collection, 3, cursor1)
		require.NoError(t, err)
		assert.Len(t, items2, 3)
		assert.NotEmpty(t, cursor2)

		// Get third page
		items3, cursor3, err := client.GetCollectionItems(ctx, collection, 3, cursor2)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(items3), 1)

		// Should have no more items if we collected all
		if len(items3) < 3 {
			assert.Empty(t, cursor3)
		}

		// Ensure no duplicate items
		allItemIDs := make(map[string]bool)
		for _, item := range append(append(items1, items2...), items3...) {
			assert.False(t, allItemIDs[item.ItemID], "Duplicate item found: %s", item.ItemID)
			allItemIDs[item.ItemID] = true
		}
	})

	t.Run("IsInCollection", func(t *testing.T) {
		// Add an item
		item := &storage.CollectionItem{
			ItemID:   "https://example.com/posts/check-1",
			ItemType: "Note",
			AddedBy:  "https://example.com/users/frank",
		}
		err := client.AddToCollection(ctx, "favorites", item)
		require.NoError(t, err)

		// Check if it's in the collection
		exists, err := client.IsInCollection(ctx, "favorites", item.ItemID)
		require.NoError(t, err)
		assert.True(t, exists)

		// Check non-existent item
		exists, err = client.IsInCollection(ctx, "favorites", "https://example.com/posts/nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)

		// Check wrong collection
		exists, err = client.IsInCollection(ctx, "wrong-collection", item.ItemID)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("CountCollectionItems", func(t *testing.T) {
		collection := "count-collection"

		// Add known number of items
		numItems := 8
		for i := 0; i < numItems; i++ {
			item := &storage.CollectionItem{
				ItemID:   fmt.Sprintf("https://example.com/posts/count-%d", i),
				ItemType: "Note",
				AddedBy:  "https://example.com/users/grace",
			}
			err := client.AddToCollection(ctx, collection, item)
			require.NoError(t, err)
		}

		// Count items
		count, err := client.CountCollectionItems(ctx, collection)
		require.NoError(t, err)
		assert.Equal(t, numItems, count)

		// Count empty collection
		count, err = client.CountCollectionItems(ctx, "empty-collection")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("DifferentCollectionTypes", func(t *testing.T) {
		// Test different collection types
		collections := []struct {
			name     string
			itemType string
		}{
			{"featured", "Note"},
			{"pinned", "Article"},
			{"highlights", "Image"},
			{"portfolio", "Page"},
		}

		for _, coll := range collections {
			item := &storage.CollectionItem{
				ItemID:   fmt.Sprintf("https://example.com/items/%s-1", coll.name),
				ItemType: coll.itemType,
				AddedBy:  "https://example.com/users/helen",
			}
			err := client.AddToCollection(ctx, coll.name, item)
			require.NoError(t, err)
		}

		// Verify each collection has its item
		for _, coll := range collections {
			items, _, err := client.GetCollectionItems(ctx, coll.name, 10, "")
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(items), 1)

			found := false
			for _, item := range items {
				if item.ItemType == coll.itemType {
					found = true
					break
				}
			}
			assert.True(t, found, "Item type %s not found in collection %s", coll.itemType, coll.name)
		}
	})
}
