package dynamodb_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/internal/testutil"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlagOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	client := testutil.GetTestDynamoDBClient(t)

	t.Run("CreateFlag", func(t *testing.T) {
		flag := &storage.Flag{
			ID:        "https://example.com/activities/flag-1",
			Actor:     "https://example.com/users/reporter",
			Object:    []string{"https://example.com/posts/spam-1", "https://example.com/users/spammer"},
			Content:   "This is spam content",
			Published: time.Now(),
		}

		err := client.CreateFlag(ctx, flag)
		require.NoError(t, err)

		// Verify the flag was created with pending status
		assert.Equal(t, storage.FlagStatusPending, flag.Status)
		assert.NotZero(t, flag.CreatedAt)
	})

	t.Run("GetFlag", func(t *testing.T) {
		// Create a flag first
		originalFlag := &storage.Flag{
			ID:        "https://example.com/activities/flag-2",
			Actor:     "https://example.com/users/reporter2",
			Object:    []string{"https://example.com/posts/offensive-1"},
			Content:   "Offensive content",
			Published: time.Now(),
		}
		err := client.CreateFlag(ctx, originalFlag)
		require.NoError(t, err)

		// Retrieve the flag
		retrievedFlag, err := client.GetFlag(ctx, originalFlag.ID)
		require.NoError(t, err)
		assert.Equal(t, originalFlag.ID, retrievedFlag.ID)
		assert.Equal(t, originalFlag.Actor, retrievedFlag.Actor)
		assert.Equal(t, originalFlag.Object, retrievedFlag.Object)
		assert.Equal(t, originalFlag.Content, retrievedFlag.Content)
		assert.Equal(t, storage.FlagStatusPending, retrievedFlag.Status)
	})

	t.Run("GetFlag_NotFound", func(t *testing.T) {
		_, err := client.GetFlag(ctx, "https://example.com/activities/nonexistent-flag")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("GetFlagsByObject", func(t *testing.T) {
		objectID := "https://example.com/posts/flagged-multiple"

		// Create multiple flags for the same object
		for i := 0; i < 3; i++ {
			flag := &storage.Flag{
				ID:        fmt.Sprintf("https://example.com/activities/flag-obj-%d", i),
				Actor:     fmt.Sprintf("https://example.com/users/reporter-%d", i),
				Object:    []string{objectID},
				Content:   fmt.Sprintf("Flag reason %d", i),
				Published: time.Now().Add(time.Duration(i) * time.Minute),
			}
			err := client.CreateFlag(ctx, flag)
			require.NoError(t, err)
		}

		// Retrieve flags for the object
		flags, cursor, err := client.GetFlagsByObject(ctx, objectID, 10, "")
		require.NoError(t, err)
		assert.Len(t, flags, 3)
		assert.Empty(t, cursor)

		// Verify flags are ordered by most recent first
		assert.True(t, flags[0].Published.After(flags[1].Published))
		assert.True(t, flags[1].Published.After(flags[2].Published))
	})

	t.Run("GetFlagsByObject_Pagination", func(t *testing.T) {
		objectID := "https://example.com/posts/flagged-paginated"

		// Create multiple flags
		for i := 0; i < 5; i++ {
			flag := &storage.Flag{
				ID:        fmt.Sprintf("https://example.com/activities/flag-page-%d", i),
				Actor:     fmt.Sprintf("https://example.com/users/reporter-page-%d", i),
				Object:    []string{objectID},
				Content:   "Spam",
				Published: time.Now().Add(time.Duration(i) * time.Minute),
			}
			err := client.CreateFlag(ctx, flag)
			require.NoError(t, err)
		}

		// Get first page
		flags1, cursor1, err := client.GetFlagsByObject(ctx, objectID, 2, "")
		require.NoError(t, err)
		assert.Len(t, flags1, 2)
		assert.NotEmpty(t, cursor1)

		// Get second page
		flags2, cursor2, err := client.GetFlagsByObject(ctx, objectID, 2, cursor1)
		require.NoError(t, err)
		assert.Len(t, flags2, 2)
		assert.NotEmpty(t, cursor2)

		// Get third page
		flags3, cursor3, err := client.GetFlagsByObject(ctx, objectID, 2, cursor2)
		require.NoError(t, err)
		assert.Len(t, flags3, 1)
		assert.Empty(t, cursor3)
	})

	t.Run("GetPendingFlags", func(t *testing.T) {
		// Create flags with different statuses
		pendingFlag1 := &storage.Flag{
			ID:        "https://example.com/activities/flag-pending-1",
			Actor:     "https://example.com/users/reporter-pending-1",
			Object:    []string{"https://example.com/posts/pending-1"},
			Content:   "Pending flag 1",
			Published: time.Now().Add(-2 * time.Hour),
		}
		err := client.CreateFlag(ctx, pendingFlag1)
		require.NoError(t, err)

		pendingFlag2 := &storage.Flag{
			ID:        "https://example.com/activities/flag-pending-2",
			Actor:     "https://example.com/users/reporter-pending-2",
			Object:    []string{"https://example.com/posts/pending-2"},
			Content:   "Pending flag 2",
			Published: time.Now().Add(-1 * time.Hour),
		}
		err = client.CreateFlag(ctx, pendingFlag2)
		require.NoError(t, err)

		// Get pending flags
		flags, _, err := client.GetPendingFlags(ctx, 10, "")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(flags), 2)

		// Verify all returned flags are pending
		for _, flag := range flags {
			assert.Equal(t, storage.FlagStatusPending, flag.Status)
		}
	})

	t.Run("UpdateFlagStatus", func(t *testing.T) {
		// Create a flag
		flag := &storage.Flag{
			ID:        "https://example.com/activities/flag-update-status",
			Actor:     "https://example.com/users/reporter-status",
			Object:    []string{"https://example.com/posts/status-test"},
			Content:   "Test flag for status update",
			Published: time.Now(),
		}
		err := client.CreateFlag(ctx, flag)
		require.NoError(t, err)

		// Update the flag status
		err = client.UpdateFlagStatus(ctx, flag.ID, storage.FlagStatusResolved, "https://example.com/users/moderator", "Content removed")
		require.NoError(t, err)

		// Verify the update
		updatedFlag, err := client.GetFlag(ctx, flag.ID)
		require.NoError(t, err)
		assert.Equal(t, storage.FlagStatusResolved, updatedFlag.Status)
		assert.Equal(t, "https://example.com/users/moderator", updatedFlag.ReviewedBy)
		assert.NotNil(t, updatedFlag.ReviewedAt)
		assert.Equal(t, "Content removed", updatedFlag.ReviewNote)
	})

	t.Run("CountPendingFlags", func(t *testing.T) {
		// Create some pending flags
		for i := 0; i < 3; i++ {
			flag := &storage.Flag{
				ID:        fmt.Sprintf("https://example.com/activities/flag-count-%d", i),
				Actor:     "https://example.com/users/reporter-count",
				Object:    []string{fmt.Sprintf("https://example.com/posts/count-%d", i)},
				Content:   "Count test",
				Published: time.Now(),
			}
			err := client.CreateFlag(ctx, flag)
			require.NoError(t, err)
		}

		// Count pending flags
		count, err := client.CountPendingFlags(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 3)
	})

	t.Run("GetFlagsByActor", func(t *testing.T) {
		actorID := "https://example.com/users/frequent-reporter"

		// Create multiple flags by the same actor
		for i := 0; i < 3; i++ {
			flag := &storage.Flag{
				ID:        fmt.Sprintf("https://example.com/activities/flag-actor-%d", i),
				Actor:     actorID,
				Object:    []string{fmt.Sprintf("https://example.com/posts/actor-flagged-%d", i)},
				Content:   "Actor test flag",
				Published: time.Now().Add(time.Duration(i) * time.Minute),
			}
			err := client.CreateFlag(ctx, flag)
			require.NoError(t, err)
		}

		// Get flags by actor
		flags, _, err := client.GetFlagsByActor(ctx, actorID, 10, "")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(flags), 3)

		// Verify all flags are from the correct actor
		for _, flag := range flags {
			assert.Equal(t, actorID, flag.Actor)
		}
	})

	t.Run("UpdateFlagStatus_DifferentStatuses", func(t *testing.T) {
		testCases := []struct {
			name   string
			status storage.FlagStatus
			note   string
		}{
			{"Reviewed", storage.FlagStatusReviewed, "Under investigation"},
			{"Dismissed", storage.FlagStatusDismissed, "False report"},
			{"Resolved", storage.FlagStatusResolved, "Action taken"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				flag := &storage.Flag{
					ID:        fmt.Sprintf("https://example.com/activities/flag-status-%s", tc.name),
					Actor:     "https://example.com/users/reporter",
					Object:    []string{"https://example.com/posts/test"},
					Content:   "Test content",
					Published: time.Now(),
				}
				err := client.CreateFlag(ctx, flag)
				require.NoError(t, err)

				err = client.UpdateFlagStatus(ctx, flag.ID, tc.status, "https://example.com/users/mod", tc.note)
				require.NoError(t, err)

				updated, err := client.GetFlag(ctx, flag.ID)
				require.NoError(t, err)
				assert.Equal(t, tc.status, updated.Status)
				assert.Equal(t, tc.note, updated.ReviewNote)
			})
		}
	})
}
