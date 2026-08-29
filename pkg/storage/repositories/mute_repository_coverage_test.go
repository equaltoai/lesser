package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestMuteRepository_CreateMute_SuccessAndDuplicatePaths(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMuteRepository(mockDB, "test-table", zap.NewNop(), nil)

	muter := "https://example.com/users/alice"
	muted := "https://example.com/users/bob"

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	// Success path.
	mockQuery.On("Create").Return(nil).Once()
	require.NoError(t, repo.CreateMute(ctx, muter, muted, "act-1", false, nil))

	// Duplicate/idempotent path (conditional create failed).
	mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
	require.NoError(t, repo.CreateMute(ctx, muter, muted, "act-2", true, nil))
}

func TestMuteRepository_CreateMute_CreateError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMuteRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Create").Return(ErrTestMockError).Once()

	err := repo.CreateMute(ctx, "https://example.com/users/alice", "https://example.com/users/bob", "act-err", false, nil)
	require.Error(t, err)
}

func TestMuteRepository_DeleteMute_SuccessAndError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMuteRepository(mockDB, "test-table", zap.NewNop(), nil)

	muter := "https://example.com/users/alice"
	muted := "https://example.com/users/bob"

	pk := "MUTE#alice"
	sk := "MUTED#bob"

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery).Maybe()
	mockQuery.On("Where", "SK", "=", sk).Return(mockQuery).Maybe()

	mockQuery.On("Delete").Return(nil).Once()
	require.NoError(t, repo.DeleteMute(ctx, muter, muted))

	mockQuery.On("Delete").Return(ErrTestMockError).Once()
	require.Error(t, repo.DeleteMute(ctx, muter, muted))
}

func TestMuteRepository_IsMuted_NotFoundAndErrorAndSuccess(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMuteRepository(mockDB, "test-table", zap.NewNop(), nil)

	muter := "https://example.com/users/alice"
	muted := "https://example.com/users/bob"

	pk := "MUTE#alice"
	sk := "MUTED#bob"

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery).Maybe()
	mockQuery.On("Where", "SK", "=", sk).Return(mockQuery).Maybe()

	// Not found (wrapped) should be treated as not muted.
	mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Return(dynamormErrors.ErrItemNotFound).Once()
	mutedResult, err := repo.IsMuted(ctx, muter, muted)
	require.NoError(t, err)
	require.False(t, mutedResult)

	// Real error path.
	mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Return(ErrTestMockError).Once()
	_, err = repo.IsMuted(ctx, muter, muted)
	require.Error(t, err)

	// Success path.
	mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Return(nil).Once()
	mutedResult, err = repo.IsMuted(ctx, muter, muted)
	require.NoError(t, err)
	require.True(t, mutedResult)
}

func TestMuteRepository_GetMutedUsers_And_GetUsersWhoMuted(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMuteRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	// First call returns limit+1 items to set next cursor.
	mockQuery.On("All", mock.AnythingOfType("*[]models.Mute")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Mute)
		*dest = []models.Mute{
			{SK: "MUTED#bob", Object: "https://example.com/users/bob", Actor: "https://example.com/users/alice"},
			{SK: "MUTED#carol", Object: "https://example.com/users/carol", Actor: "https://example.com/users/alice"},
			{SK: "MUTED#dave", Object: "https://example.com/users/dave", Actor: "https://example.com/users/alice"},
		}
	}).Return(nil).Once()

	users, nextCursor, err := repo.GetMutedUsers(ctx, "https://example.com/users/alice", 2, "")
	require.NoError(t, err)
	require.Len(t, users, 2)
	require.NotEmpty(t, nextCursor)

	// Second call: reverse lookup; error path.
	mockQuery.On("All", mock.AnythingOfType("*[]models.Mute")).Return(ErrTestMockError).Once()
	_, _, err = repo.GetUsersWhoMuted(ctx, "https://example.com/users/bob", 2, "cursor")
	require.Error(t, err)
}

func TestMuteRepository_GetMute_Counts(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMuteRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// GetMute success.
	mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.Mute)
		record.Actor = "https://example.com/users/alice"
		record.Object = "https://example.com/users/bob"
		record.ID = "act-1"
	}).Return(nil).Once()

	mute, err := repo.GetMute(ctx, "https://example.com/users/alice", "https://example.com/users/bob")
	require.NoError(t, err)
	require.Equal(t, "act-1", mute.ID)

	// GetMute error path (non-notfound).
	mockQuery.On("First", mock.AnythingOfType("*models.Mute")).Return(ErrTestMockError).Once()
	_, err = repo.GetMute(ctx, "https://example.com/users/alice", "https://example.com/users/bob")
	require.Error(t, err)

	// CountMutedUsers error path (r.Count walks the keyed partition).
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.Mute")).Return(nil, ErrTestMockError).Once()
	_, err = repo.CountMutedUsers(ctx, "https://example.com/users/alice")
	require.Error(t, err)

	// CountUsersWhoMuted error path (gsi1 walk).
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Mute")).Return(nil, ErrTestMockError).Once()
	_, err = repo.CountUsersWhoMuted(ctx, "https://example.com/users/bob")
	require.Error(t, err)
}

func TestMuteRepository_Counts_Success(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMuteRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.Mute")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Mute)
		*dest = []*models.Mute{{ID: "m1"}, {ID: "m2"}, {ID: "m3"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	count, err := repo.CountMutedUsers(ctx, "https://example.com/users/alice")
	require.NoError(t, err)
	require.Equal(t, 3, count)

	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "MUTED#bob").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Mute")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Mute)
		*dest = []models.Mute{{ID: "m1"}, {ID: "m2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	count, err = repo.CountUsersWhoMuted(ctx, "https://example.com/users/bob")
	require.NoError(t, err)
	require.Equal(t, 2, count)
}
