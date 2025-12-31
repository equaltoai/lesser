package relationships

import (
	"errors"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestRelationshipErrors_ConstructorsReturnAppErrors(t *testing.T) {
	t.Parallel()

	require.NotNil(t, ValidationFailed())
	require.Equal(t, apperrors.CodeValidationFailed, ValidationFailed().Code)
	require.NotNil(t, StorageNotAvailable())
	require.NotNil(t, RepositoryNotAvailable("test"))
	require.NotNil(t, CannotFollowSelf())
	require.NotNil(t, CannotBlockSelf())
	require.NotNil(t, CannotMuteSelf())
	require.NotNil(t, CannotAcceptOwnFollowRequest())
	require.NotNil(t, CannotRejectOwnFollowRequest())
	require.NotNil(t, TargetIDsEmpty())
	require.NotNil(t, TooManyTargetIDs(41))

	cause := errors.New("boom")
	require.NotNil(t, FailedToGetAccount(cause))
	require.NotNil(t, FailedToGetActor(cause))
	require.NotNil(t, FailedToCheckFollowStatus(cause))
	require.NotNil(t, FailedToCheckBlockStatus(cause))
	require.NotNil(t, FailedToCheckMuteStatus(cause))
	require.NotNil(t, FailedToGetExistingRelationship(cause))
	require.NotNil(t, FailedToCreateFollowRequest(cause))
	require.NotNil(t, FailedToAcceptFollowRequest(cause))
	require.NotNil(t, FailedToRejectFollowRequest(cause))
	require.NotNil(t, FailedToGetUpdatedRelationship(cause))
	require.NotNil(t, FailedToUnfollow(cause))
	require.NotNil(t, FailedToBlockUser(cause))
	require.NotNil(t, FailedToUnblockUser(cause))
	require.NotNil(t, FailedToMuteUser(cause))
	require.NotNil(t, FailedToUnmuteUser(cause))
	require.NotNil(t, FailedToBuildRelationshipData(cause))
	require.NotNil(t, FailedToCountFollowers(cause))
	require.NotNil(t, FailedToCountFollowing(cause))
	require.NotNil(t, DomainBlockRepositoryNotAvailable())
	require.NotNil(t, FailedToAddDomainBlock(cause))
	require.NotNil(t, FailedToRemoveDomainBlock(cause))
	require.NotNil(t, FailedToGetDomainBlocks(cause))
	require.NotNil(t, SocialRepositoryNotAvailable())
	require.NotNil(t, FailedToGetMutedUsers(cause))
	require.NotNil(t, FailedToGetBlockedUsers(cause))
	require.NotNil(t, FailedToGetRelationshipUsers(cause))
	require.NotNil(t, FailedToGetPendingFollowRequests(cause))
	require.NotNil(t, CheckFollowingRelationship(cause))
	require.NotNil(t, NoRepositoryOrStorage())

	require.NotNil(t, ErrFollowWhileBlocked)
	require.NotNil(t, ErrUnsupportedRelationType)
	require.NotNil(t, ErrFollowRequestNotFound)
}
