package relationships

import (
	"context"
	"strings"
	"testing"

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
