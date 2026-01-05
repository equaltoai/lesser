package lists

import (
	"context"
	"errors"
	"testing"

	serviceerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestService_GetListMembers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success returns members", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		list := &models.List{ID: "list-1", Username: "owner"}
		listRepo.On("GetList", ctx, "list-1").Return(list, nil).Once()

		members := []*storage.Account{{User: &storage.User{Username: "alice"}}, {User: &storage.User{Username: "bob"}}}
		paged := &interfaces.PaginatedResult[*storage.Account]{Items: members}
		listRepo.On("GetListMembers", ctx, "list-1", interfaces.PaginationOptions{Limit: 10}).Return(paged, nil).Once()

		out, err := service.GetListMembers(ctx, &GetListMembersQuery{
			ListID:   "list-1",
			ViewerID: "owner",
			Pagination: interfaces.PaginationOptions{
				Limit: 10,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, out)
		require.Len(t, out.Members, 2)
		require.Empty(t, out.Events)
	})

	t.Run("missing list returns not found error", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "missing").Return(&models.List{}, errors.New("boom")).Once()

		_, err := service.GetListMembers(ctx, &GetListMembersQuery{ListID: "missing", ViewerID: "owner"})
		require.ErrorIs(t, err, serviceerrors.ErrListNotFound)
	})

	t.Run("viewer must be owner", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "list-1").Return(&models.List{ID: "list-1", Username: "owner"}, nil).Once()

		_, err := service.GetListMembers(ctx, &GetListMembersQuery{ListID: "list-1", ViewerID: "someone-else"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Access denied")
	})

	t.Run("repository error returns get members error", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "list-1").Return(&models.List{ID: "list-1", Username: "owner"}, nil).Once()
		listRepo.On("GetListMembers", ctx, "list-1", interfaces.PaginationOptions{}).Return((*interfaces.PaginatedResult[*storage.Account])(nil), errors.New("boom")).Once()

		_, err := service.GetListMembers(ctx, &GetListMembersQuery{ListID: "list-1", ViewerID: "owner"})
		require.ErrorIs(t, err, serviceerrors.ErrGetListMembers)
	})
}
