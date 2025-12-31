package lists

import (
	"context"
	"errors"
	"testing"

	serviceerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestService_MembershipAndDelete_ErrorPaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("AddToList returns get failed when list lookup fails", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "list-1").Return(&models.List{}, errors.New("boom")).Once()

		_, err := service.AddToList(ctx, &AddToListCommand{ListID: "list-1", MemberUsername: "member", AdderID: "owner"})
		require.ErrorIs(t, err, serviceerrors.ErrListGetFailed)
	})

	t.Run("AddToList returns forbidden when not owner", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "list-1").Return(&models.List{ID: "list-1", Username: "owner"}, nil).Once()

		_, err := service.AddToList(ctx, &AddToListCommand{ListID: "list-1", MemberUsername: "member", AdderID: "not-owner"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Access denied")
	})

	t.Run("AddToList returns membership check error", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "list-1").Return(&models.List{ID: "list-1", Username: "owner"}, nil).Once()
		listRepo.On("IsListMember", ctx, "list-1", "member").Return(false, errors.New("boom")).Once()

		_, err := service.AddToList(ctx, &AddToListCommand{ListID: "list-1", MemberUsername: "member", AdderID: "owner"})
		require.ErrorIs(t, err, serviceerrors.ErrListMembershipCheckFailed)
	})

	t.Run("RemoveFromList returns membership check error", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "list-1").Return(&models.List{ID: "list-1", Username: "owner"}, nil).Once()
		listRepo.On("IsListMember", ctx, "list-1", "member").Return(false, errors.New("boom")).Once()

		_, err := service.RemoveFromList(ctx, &RemoveFromListCommand{ListID: "list-1", MemberUsername: "member", RemoverID: "owner"})
		require.ErrorIs(t, err, serviceerrors.ErrListMembershipCheckFailed)
	})

	t.Run("RemoveFromList returns remove failed on repository error", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "list-1").Return(&models.List{ID: "list-1", Username: "owner"}, nil).Once()
		listRepo.On("IsListMember", ctx, "list-1", "member").Return(true, nil).Once()
		listRepo.On("RemoveListMember", ctx, "list-1", "member").Return(errors.New("boom")).Once()

		_, err := service.RemoveFromList(ctx, &RemoveFromListCommand{ListID: "list-1", MemberUsername: "member", RemoverID: "owner"})
		require.ErrorIs(t, err, serviceerrors.ErrListMemberRemoveFailed)
	})

	t.Run("DeleteList returns delete failed on repository error", func(t *testing.T) {
		service, listRepo, _, _ := setupTestService()

		listRepo.On("GetList", ctx, "list-1").Return(&models.List{ID: "list-1", Username: "owner"}, nil).Once()
		listRepo.On("DeleteList", ctx, "list-1").Return(errors.New("boom")).Once()

		err := service.DeleteList(ctx, &DeleteListCommand{ListID: "list-1", DeleterID: "owner"})
		require.ErrorIs(t, err, serviceerrors.ErrListDeleteFailed)
	})
}
