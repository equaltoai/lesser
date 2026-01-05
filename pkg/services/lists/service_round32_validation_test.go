package lists

import (
	"context"
	"testing"

	serviceerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestService_ValidationHelpers(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	ctx := context.Background()

	t.Run("validateUpdateListCommand covers required fields", func(t *testing.T) {
		require.ErrorIs(t, svc.validateUpdateListCommand(ctx, &UpdateListCommand{UpdaterID: "u"}), serviceerrors.ErrListIDRequired)
		require.ErrorIs(t, svc.validateUpdateListCommand(ctx, &UpdateListCommand{ListID: "l"}), serviceerrors.ErrListUpdaterIDRequired)
		require.ErrorIs(t, svc.validateUpdateListCommand(ctx, &UpdateListCommand{ListID: "l", UpdaterID: "u", Title: "   "}), serviceerrors.ErrListTitleEmpty)
	})

	t.Run("validateDeleteListCommand covers required fields", func(t *testing.T) {
		require.ErrorIs(t, svc.validateDeleteListCommand(ctx, &DeleteListCommand{DeleterID: "u"}), serviceerrors.ErrListIDRequired)
		require.ErrorIs(t, svc.validateDeleteListCommand(ctx, &DeleteListCommand{ListID: "l"}), serviceerrors.ErrListDeleterIDRequired)
	})

	t.Run("validateAddToListCommand covers required fields", func(t *testing.T) {
		require.ErrorIs(t, svc.validateAddToListCommand(ctx, &AddToListCommand{MemberUsername: "m", AdderID: "u"}), serviceerrors.ErrListIDRequired)
		require.ErrorIs(t, svc.validateAddToListCommand(ctx, &AddToListCommand{ListID: "l", AdderID: "u"}), serviceerrors.ErrListMemberUsernameRequired)
		require.ErrorIs(t, svc.validateAddToListCommand(ctx, &AddToListCommand{ListID: "l", MemberUsername: "m"}), serviceerrors.ErrListAdderIDRequired)
	})

	t.Run("validateRemoveFromListCommand covers required fields", func(t *testing.T) {
		require.ErrorIs(t, svc.validateRemoveFromListCommand(ctx, &RemoveFromListCommand{MemberUsername: "m", RemoverID: "u"}), serviceerrors.ErrListIDRequired)
		require.ErrorIs(t, svc.validateRemoveFromListCommand(ctx, &RemoveFromListCommand{ListID: "l", RemoverID: "u"}), serviceerrors.ErrListMemberUsernameRequired)
		require.ErrorIs(t, svc.validateRemoveFromListCommand(ctx, &RemoveFromListCommand{ListID: "l", MemberUsername: "m"}), serviceerrors.ErrListRemoverIDRequired)
	})
}
