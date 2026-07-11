package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_PublicationMemberRepository_CRUDAndLists(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewPublicationMemberRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	member := &models.PublicationMember{
		PublicationID: "pub-1",
		UserID:        "user-1",
		Role:          "writer",
	}
	require.NoError(t, repo.CreateMember(ctx, member))

	got, err := repo.GetMember(ctx, "pub-1", "user-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NoError(t, repo.DeleteMember(ctx, "pub-1", "user-1"))

	list, err := repo.ListMembers(ctx, "pub-1")
	require.NoError(t, err)
	require.NotEmpty(t, list)

	_, _, err = repo.ListMembershipsForUserPaginated(ctx, "   ", 1, "")
	require.Error(t, err)

	memberships, next, err := repo.ListMembershipsForUserPaginated(ctx, "user-1", 1, "PUBLICATION#pub-0")
	require.NoError(t, err)
	require.Len(t, memberships, 1)
	require.NotEmpty(t, next)
}
