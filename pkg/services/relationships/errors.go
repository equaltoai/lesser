package relationships

import "errors"

// Common service operation errors
var (
	// ErrValidationFailed is returned when input validation fails
	ErrValidationFailed = errors.New("validation failed")

	// ErrStorageNotAvailable is returned when storage backend is not available
	ErrStorageNotAvailable = errors.New("storage not available")

	// ErrRepositoryNotAvailable is returned when a specific repository is not available
	ErrRepositoryNotAvailable = errors.New("repository not available")
)

// Relationship service errors
var (
	// ErrCannotFollowSelf is returned when a user tries to follow themselves
	ErrCannotFollowSelf = errors.New("users cannot follow themselves")

	// ErrCannotBlockSelf is returned when a user tries to block themselves
	ErrCannotBlockSelf = errors.New("users cannot block themselves")

	// ErrCannotMuteSelf is returned when a user tries to mute themselves
	ErrCannotMuteSelf = errors.New("users cannot mute themselves")

	// ErrFollowWhileBlocked is returned when trying to follow a user who has blocked you
	ErrFollowWhileBlocked = errors.New("cannot follow user: you are blocked")

	// ErrCannotAcceptOwnFollowRequest is returned when trying to accept your own follow request
	ErrCannotAcceptOwnFollowRequest = errors.New("cannot accept follow request from self")

	// ErrCannotRejectOwnFollowRequest is returned when trying to reject your own follow request
	ErrCannotRejectOwnFollowRequest = errors.New("cannot reject follow request from self")

	// ErrTargetIDsEmpty is returned when target IDs list is empty
	ErrTargetIDsEmpty = errors.New("target_ids cannot be empty")

	// ErrTooManyTargetIDs is returned when too many target IDs are provided
	ErrTooManyTargetIDs = errors.New("too many target_ids (max 40)")

	// ErrUnsupportedRelationType is returned when an unsupported relation type is requested
	ErrUnsupportedRelationType = errors.New("unsupported relation type")
)

// Account/User operation errors
var (
	// ErrFailedToGetAccount is returned when account retrieval fails
	ErrFailedToGetAccount = errors.New("failed to get account")

	// ErrFailedToGetActor is returned when actor retrieval fails
	ErrFailedToGetActor = errors.New("failed to get actor")

	// ErrNoRepositoryOrStorage is returned when neither repository nor storage is available
	ErrNoRepositoryOrStorage = errors.New("no repository or storage available")
)

// Relationship operation errors
var (
	// ErrFailedToCheckFollowStatus is returned when checking follow status fails
	ErrFailedToCheckFollowStatus = errors.New("failed to check follow status")

	// ErrFailedToCheckBlockStatus is returned when checking block status fails
	ErrFailedToCheckBlockStatus = errors.New("failed to check block status")

	// ErrFailedToCheckMuteStatus is returned when checking mute status fails
	ErrFailedToCheckMuteStatus = errors.New("failed to check mute status")

	// ErrFailedToGetExistingRelationship is returned when getting existing relationship fails
	ErrFailedToGetExistingRelationship = errors.New("failed to get existing relationship")

	// ErrFailedToCreateFollowRequest is returned when creating follow request fails
	ErrFailedToCreateFollowRequest = errors.New("failed to create follow request")

	// ErrFailedToAcceptFollowRequest is returned when accepting follow request fails
	ErrFailedToAcceptFollowRequest = errors.New("failed to accept follow request")

	// ErrFailedToRejectFollowRequest is returned when rejecting follow request fails
	ErrFailedToRejectFollowRequest = errors.New("failed to reject follow request")

	// ErrFailedToGetUpdatedRelationship is returned when getting updated relationship fails
	ErrFailedToGetUpdatedRelationship = errors.New("failed to get updated relationship")

	// ErrFailedToUnfollow is returned when unfollow operation fails
	ErrFailedToUnfollow = errors.New("failed to unfollow")

	// ErrFailedToBlockUser is returned when blocking user fails
	ErrFailedToBlockUser = errors.New("failed to block user")

	// ErrFailedToUnblockUser is returned when unblocking user fails
	ErrFailedToUnblockUser = errors.New("failed to unblock user")

	// ErrFailedToMuteUser is returned when muting user fails
	ErrFailedToMuteUser = errors.New("failed to mute user")

	// ErrFailedToUnmuteUser is returned when unmuting user fails
	ErrFailedToUnmuteUser = errors.New("failed to unmute user")

	// ErrFailedToBuildRelationshipData is returned when building relationship data fails
	ErrFailedToBuildRelationshipData = errors.New("failed to build relationship data")

	// ErrFailedToCountFollowers is returned when counting followers fails
	ErrFailedToCountFollowers = errors.New("failed to count followers")

	// ErrFailedToCountFollowing is returned when counting following fails
	ErrFailedToCountFollowing = errors.New("failed to count following")

	// ErrFollowRequestNotFound is returned when a follow request is not found
	ErrFollowRequestNotFound = errors.New("follow request not found")
)

// Domain block operation errors
var (
	// ErrDomainBlockRepositoryNotAvailable is returned when domain block repository is not available
	ErrDomainBlockRepositoryNotAvailable = errors.New("domain block repository not available")

	// ErrFailedToAddDomainBlock is returned when adding domain block fails
	ErrFailedToAddDomainBlock = errors.New("failed to add domain block")

	// ErrFailedToRemoveDomainBlock is returned when removing domain block fails
	ErrFailedToRemoveDomainBlock = errors.New("failed to remove domain block")

	// ErrFailedToGetDomainBlocks is returned when getting domain blocks fails
	ErrFailedToGetDomainBlocks = errors.New("failed to get domain blocks")
)

// Social operation errors
var (
	// ErrSocialRepositoryNotAvailable is returned when social repository is not available
	ErrSocialRepositoryNotAvailable = errors.New("social repository not available")

	// ErrFailedToGetMutedUsers is returned when getting muted users fails
	ErrFailedToGetMutedUsers = errors.New("failed to get muted users")

	// ErrFailedToGetBlockedUsers is returned when getting blocked users fails
	ErrFailedToGetBlockedUsers = errors.New("failed to get blocked users")

	// ErrFailedToGetRelationshipUsers is returned when getting relationship users fails
	ErrFailedToGetRelationshipUsers = errors.New("failed to get relationship users")

	// ErrFailedToGetPendingFollowRequests is returned when getting pending follow requests fails
	ErrFailedToGetPendingFollowRequests = errors.New("failed to get pending follow requests")
)

// General service errors (common to all services)
var (
	// ErrValidationFailed is returned when input validation fails
	ErrValidationFailedGeneral = errors.New("validation failed")

	// ErrGetAccount is returned when account retrieval fails
	ErrGetAccount = errors.New("failed to get account")

	// ErrGetActor is returned when actor retrieval fails
	ErrGetActor = errors.New("failed to get actor")

	// ErrCheckFollowingRelationship is returned when checking following relationship fails
	ErrCheckFollowingRelationship = errors.New("failed to check following relationship")

	// ErrFailedToCreateFollowRequest is returned when creating follow request fails
	ErrFailedToCreateFollowRequestLocal = errors.New("failed to create follow request")

	// ErrFailedToAcceptFollowRequest is returned when accepting follow request fails
	ErrFailedToAcceptFollowRequestLocal = errors.New("failed to accept follow request")

	// ErrFailedToRejectFollowRequest is returned when rejecting follow request fails
	ErrFailedToRejectFollowRequestLocal = errors.New("failed to reject follow request")

	// ErrFailedToGetUpdatedRelationship is returned when getting updated relationship fails
	ErrFailedToGetUpdatedRelationshipLocal = errors.New("failed to get updated relationship")

	// ErrFailedToCheckBlockStatus is returned when checking block status fails
	ErrFailedToCheckBlockStatusLocal = errors.New("failed to check block status")

	// ErrFailedToCheckMuteStatus is returned when checking mute status fails
	ErrFailedToCheckMuteStatusLocal = errors.New("failed to check mute status")

	// ErrFailedToUnfollow is returned when unfollow operation fails
	ErrFailedToUnfollowLocal = errors.New("failed to unfollow")

	// ErrFailedToBlockUser is returned when blocking user fails
	ErrFailedToBlockUserLocal = errors.New("failed to block user")

	// ErrFailedToUnblockUser is returned when unblocking user fails
	ErrFailedToUnblockUserLocal = errors.New("failed to unblock user")

	// ErrFailedToMuteUser is returned when muting user fails
	ErrFailedToMuteUserLocal = errors.New("failed to mute user")

	// ErrFailedToUnmuteUser is returned when unmuting user fails
	ErrFailedToUnmuteUserLocal = errors.New("failed to unmute user")

	// ErrFailedToBuildRelationshipData is returned when building relationship data fails
	ErrFailedToBuildRelationshipDataLocal = errors.New("failed to build relationship data")

	// ErrFailedToCountFollowers is returned when counting followers fails
	ErrFailedToCountFollowersLocal = errors.New("failed to count followers")

	// ErrFailedToCountFollowing is returned when counting following fails
	ErrFailedToCountFollowingLocal = errors.New("failed to count following")

	// ErrFollowRequestNotFound is returned when a follow request is not found
	ErrFollowRequestNotFoundLocal = errors.New("follow request not found")

	// ErrFailedToAddDomainBlock is returned when adding domain block fails
	ErrFailedToAddDomainBlockLocal = errors.New("failed to add domain block")

	// ErrFailedToRemoveDomainBlock is returned when removing domain block fails
	ErrFailedToRemoveDomainBlockLocal = errors.New("failed to remove domain block")

	// ErrFailedToGetDomainBlocks is returned when getting domain blocks fails
	ErrFailedToGetDomainBlocksLocal = errors.New("failed to get domain blocks")

	// ErrFailedToGetMutedUsers is returned when getting muted users fails
	ErrFailedToGetMutedUsersLocal = errors.New("failed to get muted users")

	// ErrFailedToGetBlockedUsers is returned when getting blocked users fails
	ErrFailedToGetBlockedUsersLocal = errors.New("failed to get blocked users")

	// ErrFailedToGetRelationshipUsers is returned when getting relationship users fails
	ErrFailedToGetRelationshipUsersLocal = errors.New("failed to get relationship users")

	// ErrFailedToGetPendingFollowRequests is returned when getting pending follow requests fails
	ErrFailedToGetPendingFollowRequestsLocal = errors.New("failed to get pending follow requests")

	// ErrUnsupportedRelationType is returned when an unsupported relation type is requested
	ErrUnsupportedRelationTypeLocal = errors.New("unsupported relation type")

	// ErrStorageNotAvailableLocal is returned when storage backend is not available
	ErrStorageNotAvailableLocal = errors.New("storage not available")

	// ErrRepositoryNotAvailableLocal is returned when a specific repository is not available
	ErrRepositoryNotAvailableLocal = errors.New("repository not available")

	// ErrDomainBlockRepositoryNotAvailableLocal is returned when domain block repository is not available
	ErrDomainBlockRepositoryNotAvailableLocal = errors.New("domain block repository not available")

	// ErrSocialRepositoryNotAvailableLocal is returned when social repository is not available
	ErrSocialRepositoryNotAvailableLocal = errors.New("social repository not available")

	// ErrNoRepositoryOrStorageLocal is returned when neither repository nor storage is available
	ErrNoRepositoryOrStorageLocal = errors.New("no repository or storage available")

	// ErrCannotFollowSelfLocal is returned when a user tries to follow themselves
	ErrCannotFollowSelfLocal = errors.New("users cannot follow themselves")

	// ErrCannotBlockSelfLocal is returned when a user tries to block themselves
	ErrCannotBlockSelfLocal = errors.New("users cannot block themselves")

	// ErrCannotMuteSelfLocal is returned when a user tries to mute themselves
	ErrCannotMuteSelfLocal = errors.New("users cannot mute themselves")

	// ErrFollowWhileBlockedLocal is returned when trying to follow a user who has blocked you
	ErrFollowWhileBlockedLocal = errors.New("cannot follow user: you are blocked")

	// ErrCannotAcceptOwnFollowRequestLocal is returned when trying to accept your own follow request
	ErrCannotAcceptOwnFollowRequestLocal = errors.New("cannot accept follow request from self")

	// ErrCannotRejectOwnFollowRequestLocal is returned when trying to reject your own follow request
	ErrCannotRejectOwnFollowRequestLocal = errors.New("cannot reject follow request from self")

	// ErrTargetIDsEmptyLocal is returned when target IDs list is empty
	ErrTargetIDsEmptyLocal = errors.New("target_ids cannot be empty")

	// ErrTooManyTargetIDsLocal is returned when too many target IDs are provided
	ErrTooManyTargetIDsLocal = errors.New("too many target_ids (max 40)")
)