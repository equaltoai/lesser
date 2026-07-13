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

func TestRound10_RevisionRepository_CRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewRevisionRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	revision := &models.Revision{
		ID:       "rev-1",
		ObjectID: "object-1",
		Version:  1,
		Content:  "content",
	}
	require.NoError(t, repo.CreateRevision(ctx, revision))

	got, err := repo.GetRevision(ctx, "object-1", 1)
	require.NoError(t, err)
	require.NotNil(t, got)

	_, _, err = repo.ListRevisionsPaginated(ctx, "   ", 1, "")
	require.Error(t, err)

	revisions, next, err := repo.ListRevisionsPaginated(ctx, "object-1", 1, "00000002")
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	require.NotEmpty(t, next)
}

func TestRound10_RevisionRepository_MoreBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewRevisionRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	list, err := repo.ListRevisions(ctx, "object-1", 0)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	revisions, next, err := repo.ListRevisionsPaginated(ctx, "object-1", 10, "VERSION#00000002")
	require.NoError(t, err)
	require.NotEmpty(t, revisions)
	require.Empty(t, next)
}
