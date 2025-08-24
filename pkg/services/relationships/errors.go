// Package relationships provides error handling utilities for relationship operations.
package relationships

import (
	"fmt"

	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
)

// ValidationFailed returns an error when validation fails.
func ValidationFailed() *pkgerrors.AppError {
	return pkgerrors.NewAppError(pkgerrors.CodeValidationFailed, pkgerrors.CategoryValidation, "validation failed")
}

// StorageNotAvailable returns an error when storage is not available.
func StorageNotAvailable() *pkgerrors.AppError {
	return pkgerrors.ServiceUnavailable("storage")
}

// RepositoryNotAvailable returns an error when a repository is not available.
func RepositoryNotAvailable(repoType string) *pkgerrors.AppError {
	return pkgerrors.ServiceUnavailable(fmt.Sprintf("%s repository", repoType))
}

// CannotFollowSelf returns an error when trying to follow oneself.
func CannotFollowSelf() *pkgerrors.AppError {
	return pkgerrors.OperationNotAllowedOnSelf("follow")
}

// CannotBlockSelf returns an error when trying to block oneself.
func CannotBlockSelf() *pkgerrors.AppError {
	return pkgerrors.OperationNotAllowedOnSelf("block")
}

// CannotMuteSelf returns an error when trying to mute oneself.
func CannotMuteSelf() *pkgerrors.AppError {
	return pkgerrors.OperationNotAllowedOnSelf("mute")
}

// ErrFollowWhileBlocked is a unique business rule - keep as local error
var ErrFollowWhileBlocked = pkgerrors.BusinessRuleViolated("follow_while_blocked", map[string]interface{}{"reason": "user is blocked"})

// CannotAcceptOwnFollowRequest returns an error when trying to accept one's own follow request.
func CannotAcceptOwnFollowRequest() *pkgerrors.AppError {
	return pkgerrors.OperationNotAllowedOnSelf("accept follow request")
}

// CannotRejectOwnFollowRequest returns an error when trying to reject one's own follow request.
func CannotRejectOwnFollowRequest() *pkgerrors.AppError {
	return pkgerrors.OperationNotAllowedOnSelf("reject follow request")
}

// TargetIDsEmpty returns an error when target IDs are empty.
func TargetIDsEmpty() *pkgerrors.AppError {
	return pkgerrors.RequiredFieldMissing("target_ids")
}

// TooManyTargetIDs returns an error when too many target IDs are provided.
func TooManyTargetIDs(count int) *pkgerrors.AppError {
	return pkgerrors.ValueOutOfRange("target_ids", 1, 40, count)
}

// ErrUnsupportedRelationType is specific to relationships - keep as local error
var ErrUnsupportedRelationType = pkgerrors.NewValidationError("relation_type", "unsupported")

// FailedToGetAccount returns an error when failing to get account.
func FailedToGetAccount(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToGet("account", err)
}

// FailedToGetActor returns an error when failing to get actor.
func FailedToGetActor(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToGet("actor", err)
}

// NoRepositoryOrStorage returns an error when neither repository nor storage is available.
func NoRepositoryOrStorage() *pkgerrors.AppError {
	return pkgerrors.ServiceUnavailable("repository or storage")
}

// FailedToCheckFollowStatus returns an error when failing to check follow status.
func FailedToCheckFollowStatus(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToQuery("follow status", err)
}

// FailedToCheckBlockStatus returns an error when failing to check block status.
func FailedToCheckBlockStatus(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToQuery("block status", err)
}

// FailedToCheckMuteStatus returns an error when failing to check mute status.
func FailedToCheckMuteStatus(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToQuery("mute status", err)
}

// FailedToGetExistingRelationship returns an error when failing to get existing relationship.
func FailedToGetExistingRelationship(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToGet("relationship", err)
}

// FailedToCreateFollowRequest returns an error when failing to create follow request.
func FailedToCreateFollowRequest(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToCreate("follow request", err)
}

// FailedToAcceptFollowRequest returns an error when failing to accept follow request.
func FailedToAcceptFollowRequest(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToUpdate("follow request", err)
}

// FailedToRejectFollowRequest returns an error when failing to reject follow request.
func FailedToRejectFollowRequest(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToDelete("follow request", err)
}

// FailedToGetUpdatedRelationship returns an error when failing to get updated relationship.
func FailedToGetUpdatedRelationship(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToGet("updated relationship", err)
}

// FailedToUnfollow returns an error when failing to unfollow.
func FailedToUnfollow(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToDelete("follow", err)
}

// FailedToBlockUser returns an error when failing to block user.
func FailedToBlockUser(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToCreate("block", err)
}

// FailedToUnblockUser returns an error when failing to unblock user.
func FailedToUnblockUser(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToDelete("block", err)
}

// FailedToMuteUser returns an error when failing to mute user.
func FailedToMuteUser(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToCreate("mute", err)
}

// FailedToUnmuteUser returns an error when failing to unmute user.
func FailedToUnmuteUser(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToDelete("mute", err)
}

// FailedToBuildRelationshipData returns an error when failing to build relationship data.
func FailedToBuildRelationshipData(err error) *pkgerrors.AppError {
	return pkgerrors.ProcessingFailed("relationship data building", err)
}

// FailedToCountFollowers returns an error when failing to count followers.
func FailedToCountFollowers(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToQuery("followers count", err)
}

// FailedToCountFollowing returns an error when failing to count following.
func FailedToCountFollowing(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToQuery("following count", err)
}

// ErrFollowRequestNotFound is specific to relationships - keep as local error  
var ErrFollowRequestNotFound = pkgerrors.NewAppError(pkgerrors.CodeNotFound, pkgerrors.CategoryBusiness, "follow request not found")

// DomainBlockRepositoryNotAvailable returns an error when domain block repository is not available.
func DomainBlockRepositoryNotAvailable() *pkgerrors.AppError {
	return pkgerrors.ServiceUnavailable("domain block repository")
}

// FailedToAddDomainBlock returns an error when failing to add domain block.
func FailedToAddDomainBlock(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToCreate("domain block", err)
}

// FailedToRemoveDomainBlock returns an error when failing to remove domain block.
func FailedToRemoveDomainBlock(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToDelete("domain block", err)
}

// FailedToGetDomainBlocks returns an error when failing to get domain blocks.
func FailedToGetDomainBlocks(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToList("domain blocks", err)
}

// SocialRepositoryNotAvailable returns an error when social repository is not available.
func SocialRepositoryNotAvailable() *pkgerrors.AppError {
	return pkgerrors.ServiceUnavailable("social repository")
}

// FailedToGetMutedUsers returns an error when failing to get muted users.
func FailedToGetMutedUsers(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToList("muted users", err)
}

// FailedToGetBlockedUsers returns an error when failing to get blocked users.
func FailedToGetBlockedUsers(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToList("blocked users", err)
}

// FailedToGetRelationshipUsers returns an error when failing to get relationship users.
func FailedToGetRelationshipUsers(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToList("relationship users", err)
}

// FailedToGetPendingFollowRequests returns an error when failing to get pending follow requests.
func FailedToGetPendingFollowRequests(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToList("pending follow requests", err)
}

// ALL "Local" DUPLICATES ELIMINATED
// Removed 50+ exact duplicate error variables that ended with "Local"
// These duplicates included:
// - All ErrFailedTo*Local variables (identical to their non-Local counterparts)
// - All ErrCannot*SelfLocal variables (identical to their non-Local counterparts) 
// - All storage/repository availability errors
// Use the centralized error functions above instead

// CheckFollowingRelationship returns an error when failing to check following relationship.
func CheckFollowingRelationship(err error) *pkgerrors.AppError {
	return pkgerrors.FailedToQuery("following relationship", err)
}