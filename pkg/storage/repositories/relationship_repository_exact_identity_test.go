package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"go.uber.org/zap"
)

func TestRelationshipRepository_IsFollowing_FallsBackToLegacyRemoteIdentity(t *testing.T) {
	ctx := context.Background()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", zap.NewNop())
	repo.localDomain = "localhost"

	mockQuery.On("First", mock.AnythingOfType("*models.RelationshipRecord")).
		Return(dynamormerrors.ErrItemNotFound).
		Once()
	mockQuery.On("First", mock.AnythingOfType("*models.RelationshipRecord")).
		Run(func(args mock.Arguments) {
			record := args.Get(0).(*models.RelationshipRecord)
			record.PK = "FOLLOW#alice"
			record.SK = "FOLLOWING#@bob@remote.example"
			record.GSI1SK = "FOLLOWER#alice"
			record.State = models.RelationshipAccepted
		}).
		Return(nil).
		Once()

	following, err := repo.IsFollowing(ctx, "alice", "https://remote.example/users/bob")
	require.NoError(t, err)
	require.True(t, following)
}

func TestRelationshipRepository_GetFollowers_NormalizesLegacyRemoteIdentity(t *testing.T) {
	ctx := context.Background()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", zap.NewNop())

	mockQuery.On("All", mock.AnythingOfType("*[]models.RelationshipRecord")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.RelationshipRecord)
			*out = []models.RelationshipRecord{
				{GSI1SK: "FOLLOWER#@carol@remote.example"},
				{GSI1SK: "FOLLOWER#dave@remote.example"},
			}
		}).
		Return(nil).
		Once()

	followers, next, err := repo.GetFollowers(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Empty(t, next)
	require.Equal(t, []string{"carol@remote.example", "dave@remote.example"}, followers)
}
