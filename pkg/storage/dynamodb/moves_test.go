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

func TestMoveOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	client := testutil.GetTestDynamoDBClient(t)

	t.Run("CreateMove", func(t *testing.T) {
		move := &storage.Move{
			ID:        "https://example.com/activities/move-1",
			Actor:     "https://oldserver.com/users/alice",
			Target:    "https://newserver.com/users/alice",
			Published: time.Now(),
		}

		err := client.CreateMove(ctx, move)
		require.NoError(t, err)

		// Verify move was created
		assert.NotZero(t, move.CreatedAt)
	})

	t.Run("CreateMove_DuplicateActor", func(t *testing.T) {
		// Create first move
		move1 := &storage.Move{
			ID:        "https://example.com/activities/move-2",
			Actor:     "https://oldserver.com/users/bob",
			Target:    "https://newserver.com/users/bob",
			Published: time.Now(),
		}
		err := client.CreateMove(ctx, move1)
		require.NoError(t, err)

		// Try to create another move from same actor
		move2 := &storage.Move{
			ID:        "https://example.com/activities/move-3",
			Actor:     "https://oldserver.com/users/bob",
			Target:    "https://anotherserver.com/users/bob",
			Published: time.Now(),
		}
		err = client.CreateMove(ctx, move2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "has already moved")
	})

	t.Run("GetMove", func(t *testing.T) {
		// Create a move
		originalMove := &storage.Move{
			ID:        "https://example.com/activities/move-4",
			Actor:     "https://oldserver.com/users/charlie",
			Target:    "https://newserver.com/users/charlie",
			Published: time.Now(),
		}
		err := client.CreateMove(ctx, originalMove)
		require.NoError(t, err)

		// Retrieve the move
		retrievedMove, err := client.GetMove(ctx, originalMove.Actor)
		require.NoError(t, err)
		assert.Equal(t, originalMove.ID, retrievedMove.ID)
		assert.Equal(t, originalMove.Actor, retrievedMove.Actor)
		assert.Equal(t, originalMove.Target, retrievedMove.Target)
	})

	t.Run("GetMove_NotFound", func(t *testing.T) {
		_, err := client.GetMove(ctx, "https://example.com/users/nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no move found")
	})

	t.Run("GetMoveByTarget", func(t *testing.T) {
		targetAccount := "https://newserver.com/users/popular"

		// Create multiple moves to the same target
		for i := 0; i < 3; i++ {
			move := &storage.Move{
				ID:        fmt.Sprintf("https://example.com/activities/move-target-%d", i),
				Actor:     fmt.Sprintf("https://oldserver.com/users/user%d", i),
				Target:    targetAccount,
				Published: time.Now().Add(time.Duration(i) * time.Hour),
			}
			err := client.CreateMove(ctx, move)
			require.NoError(t, err)
		}

		// Get all moves to this target
		moves, err := client.GetMoveByTarget(ctx, targetAccount)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(moves), 3)

		// Verify all moves point to the correct target
		for _, move := range moves {
			assert.Equal(t, targetAccount, move.Target)
		}
	})

	t.Run("HasMovedFrom", func(t *testing.T) {
		oldActor := "https://oldserver.com/users/dave"
		newActor := "https://newserver.com/users/dave"

		// Create a move
		move := &storage.Move{
			ID:        "https://example.com/activities/move-hasmoved",
			Actor:     oldActor,
			Target:    newActor,
			Published: time.Now(),
		}
		err := client.CreateMove(ctx, move)
		require.NoError(t, err)

		// Check if the move relationship exists
		hasMoved, err := client.HasMovedFrom(ctx, oldActor, newActor)
		require.NoError(t, err)
		assert.True(t, hasMoved)

		// Check non-existent relationship
		hasMoved, err = client.HasMovedFrom(ctx, oldActor, "https://wrongserver.com/users/dave")
		require.NoError(t, err)
		assert.False(t, hasMoved)
	})

	t.Run("MultipleMovesFromDifferentActors", func(t *testing.T) {
		// Create moves from different actors
		actors := []string{
			"https://server1.com/users/eve",
			"https://server2.com/users/frank",
			"https://server3.com/users/grace",
		}

		for i, actor := range actors {
			move := &storage.Move{
				ID:        fmt.Sprintf("https://example.com/activities/move-multi-%d", i),
				Actor:     actor,
				Target:    fmt.Sprintf("https://newserver.com/users/user%d", i),
				Published: time.Now(),
			}
			err := client.CreateMove(ctx, move)
			require.NoError(t, err)
		}

		// Verify each actor can only have one move
		for _, actor := range actors {
			move, err := client.GetMove(ctx, actor)
			require.NoError(t, err)
			assert.Equal(t, actor, move.Actor)
		}
	})
}
