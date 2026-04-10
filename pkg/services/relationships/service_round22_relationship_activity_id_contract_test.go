package relationships

import (
	"context"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestService_Follow_StoresCanonicalActivityIDOnPendingRelationshipRow(t *testing.T) {
	ctx := context.Background()
	service, relationshipRepo, _, _, _ := buildRemoteFollowPersistenceService(t, inmemory.NewActivityRepository())

	result, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.RequestID)
	require.True(t, strings.HasPrefix(result.RequestID, "https://example.com/activities/"))

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, result.RequestID, record.ActivityID)
}

func TestService_Follow_ReusesCanonicalRelationshipActivityIDOnIdempotentRetry(t *testing.T) {
	ctx := context.Background()
	service, relationshipRepo, _, _, _ := buildRemoteFollowPersistenceService(t, inmemory.NewActivityRepository())

	first, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, second)

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, first.RequestID, record.ActivityID)
	require.Equal(t, second.RequestID, record.ActivityID)
}

func TestService_Follow_ReopensRejectedRelationshipWithFreshActivityID(t *testing.T) {
	ctx := context.Background()
	service, relationshipRepo, _, _, _ := buildRemoteFollowPersistenceService(t, inmemory.NewActivityRepository())

	first, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, first)

	require.NoError(t, relationshipRepo.RejectFollowRequest(ctx, "alice", "bob@remote.social"))

	second, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, second)
	require.NotEmpty(t, second.RequestID)
	require.NotEqual(t, first.RequestID, second.RequestID)
	require.NotNil(t, second.Relationship)
	require.True(t, second.Relationship.Requested)
	require.False(t, second.Relationship.Following)

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.Equal(t, models.RelationshipPending, record.State)
	require.Equal(t, second.RequestID, record.ActivityID)
}
