package notes

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Notes service specific errors - consolidated to use centralized error system
var (
	// Notes validation errors
	// ErrNotesValidationFailed is returned when notes validation fails
	ErrNotesValidationFailed = errors.NewValidationError("notes", "validation failed")

	// ErrGetAuthorAccount is returned when getting author account fails
	ErrGetAuthorAccount = errors.FailedToGet("author account", stdErrors.New("failed to get author account"))

	// ErrCreateStatus is returned when status creation fails
	ErrCreateStatus = errors.FailedToCreate("status", stdErrors.New("failed to create status"))

	// ErrGetStatus is returned when status retrieval fails
	ErrGetStatus = errors.FailedToGet("status", stdErrors.New("failed to get status"))

	// ErrUpdateStatus is returned when status update fails
	ErrUpdateStatus = errors.FailedToUpdate("status", stdErrors.New("failed to update status"))

	// ErrDeleteStatus is returned when status deletion fails
	ErrDeleteStatus = errors.FailedToDelete("status", stdErrors.New("failed to delete status"))

	// ErrStatusNotFound is returned when status is not found
	ErrStatusNotFound = errors.StatusNotFound("")

	// ErrStatusIDRequired is returned when status ID is required but missing
	ErrStatusIDRequired = errors.RequiredFieldMissing("status_id")

	// ErrCheckViewPermissions is returned when checking view permissions fails
	ErrCheckViewPermissions = errors.ProcessingFailed("view permissions check", stdErrors.New("view permissions check failed"))

	// ErrCheckFollowingRelationship is returned when checking following relationship fails
	ErrCheckFollowingRelationship = errors.ProcessingFailed("following relationship check", stdErrors.New("following relationship check failed"))

	// Timeline errors
	// ErrHomeTimelineRequiresViewerID is returned when home timeline requires viewer_id
	ErrHomeTimelineRequiresViewerID = errors.RequiredFieldMissing("viewer_id")

	// ErrUserTimelineRequiresAuthorID is returned when user timeline requires author_id
	ErrUserTimelineRequiresAuthorID = errors.RequiredFieldMissing("author_id")

	// ErrConversationsTimelineRequiresConversationID is returned when conversations timeline requires conversation_id
	ErrConversationsTimelineRequiresConversationID = errors.RequiredFieldMissing("conversation_id")

	// ErrDirectTimelineRequiresViewerID is returned when direct timeline requires viewer_id
	ErrDirectTimelineRequiresViewerID = errors.RequiredFieldMissing("viewer_id")

	// ErrHashtagTimelineRequiresHashtag is returned when hashtag timeline requires hashtag
	ErrHashtagTimelineRequiresHashtag = errors.RequiredFieldMissing("hashtag")

	// ErrListTimelineRequiresListID is returned when list timeline requires list_id
	ErrListTimelineRequiresListID = errors.RequiredFieldMissing("list_id")

	// ErrUnsupportedTimelineType is returned when timeline type is unsupported
	ErrUnsupportedTimelineType = errors.InvalidValue("timeline_type", []string{"home", "user", "conversation", "direct", "hashtag", "list"}, "")

	// ErrGetTimeline is returned when timeline retrieval fails
	ErrGetTimeline = errors.FailedToGet("timeline", stdErrors.New("failed to get timeline"))

	// Content validation errors
	// ErrStatusContentValidationFailed is returned when status content validation fails
	ErrStatusContentValidationFailed = errors.NewValidationError("status_content", "status content validation failed")

	// ErrVisibilityValidationFailed is returned when visibility validation fails
	ErrVisibilityValidationFailed = errors.StatusInvalidVisibility("")

	// ErrSpoilerTextValidationFailed is returned when spoiler text validation fails
	ErrSpoilerTextValidationFailed = errors.StatusSpoilerTextTooLong(0)

	// ErrInvalidInReplyToID is returned when in_reply_to_id is invalid
	ErrInvalidInReplyToID = errors.IDInvalidFormat("in_reply_to_id")

	// ErrContentCannotBeEmpty is returned when content cannot be empty
	ErrContentCannotBeEmpty = errors.ContentEmpty("status")

	// ErrContentTooLong is returned when content is too long
	ErrContentTooLong = errors.ContentTooLong("status", 5000)

	// ErrContentTooLongShort is returned when content is too long for short form
	ErrContentTooLongShort = errors.ContentTooLong("status", 500)

	// Bookmark errors
	// ErrBookmarkStatus is returned when bookmarking status fails
	ErrBookmarkStatus = errors.FailedToCreate("bookmark", stdErrors.New("failed to create bookmark"))

	// ErrUnbookmarkStatus is returned when unbookmarking status fails
	ErrUnbookmarkStatus = errors.FailedToDelete("bookmark", stdErrors.New("failed to delete bookmark"))

	// ErrGetBookmarks is returned when getting bookmarks fails
	ErrGetBookmarks = errors.FailedToGet("bookmarks", stdErrors.New("failed to get bookmarks"))

	// Likes and reblog errors
	// ErrGetLikers is returned when getting likers fails
	ErrGetLikers = errors.FailedToGet("likers", stdErrors.New("failed to get likers"))

	// ErrGetRebloggerAccount is returned when getting reblogger account fails
	ErrGetRebloggerAccount = errors.FailedToGet("reblogger account", stdErrors.New("failed to get reblogger account"))

	// ErrReblogStatus is returned when reblogging status fails
	ErrReblogStatus = errors.FailedToCreate("reblog", stdErrors.New("failed to create reblog"))

	// ErrGetRebloggers is returned when getting rebloggers fails
	ErrGetRebloggers = errors.FailedToGet("rebloggers", stdErrors.New("failed to get rebloggers"))

	// ErrMuteStatus is returned when muting status fails
	ErrMuteStatus = errors.FailedToUpdate("status mute", stdErrors.New("failed to update status mute"))

	// ErrUnmuteStatus is returned when unmuting status fails
	ErrUnmuteStatus = errors.FailedToUpdate("status unmute", stdErrors.New("failed to update status unmute"))

	// ErrCreateLike is returned when creating like fails
	ErrCreateLike = errors.FailedToCreate("like", stdErrors.New("failed to create like"))

	// ErrDeleteLike is returned when deleting like fails
	ErrDeleteLike = errors.FailedToDelete("like", stdErrors.New("failed to delete like"))

	// ErrGetLikes is returned when getting likes fails
	ErrGetLikes = errors.FailedToGet("likes", stdErrors.New("failed to get likes"))

	// ErrCreateReblog is returned when creating reblog fails
	ErrCreateReblog = errors.FailedToCreate("reblog", stdErrors.New("failed to create reblog"))

	// ErrDeleteReblog is returned when deleting reblog fails
	ErrDeleteReblog = errors.FailedToDelete("reblog", stdErrors.New("failed to delete reblog"))

	// ErrGetAnnounces is returned when getting announces fails
	ErrGetAnnounces = errors.FailedToGet("announces", stdErrors.New("failed to get announces"))

	// Pin errors
	// ErrPinStatus is returned when pinning status fails
	ErrPinStatus = errors.FailedToUpdate("status pin", stdErrors.New("failed to update status pin"))

	// ErrUnpinStatus is returned when unpinning status fails
	ErrUnpinStatus = errors.FailedToUpdate("status unpin", stdErrors.New("failed to update status unpin"))

	// Conversation errors
	// ErrConversationServiceNotAvailable is returned when conversation service is not available
	ErrConversationServiceNotAvailable = errors.ServiceNotAvailable("conversation")

	// ErrMuteConversation is returned when muting conversation fails
	ErrMuteConversation = errors.FailedToUpdate("conversation mute", stdErrors.New("failed to update conversation mute"))

	// ErrUnmuteConversation is returned when unmuting conversation fails
	ErrUnmuteConversation = errors.FailedToUpdate("conversation unmute", stdErrors.New("failed to update conversation unmute"))

	// ErrGetConversations is returned when getting conversations fails
	ErrGetConversations = errors.FailedToGet("conversations", stdErrors.New("failed to get conversations"))

	// Favorited timeline errors
	// ErrViewerIDRequiredForFavoritedTimeline is returned when viewer_id is required for favorited timeline
	ErrViewerIDRequiredForFavoritedTimeline = errors.RequiredFieldMissing("viewer_id")

	// ErrGetViewerAccount is returned when getting viewer account fails
	ErrGetViewerAccount = errors.FailedToGet("viewer account", stdErrors.New("failed to get viewer account"))

	// ErrLikeRepositoryNotAvailable is returned when like repository is not available
	ErrLikeRepositoryNotAvailable = errors.ServiceNotAvailable("like repository")

	// ErrGetLikedObjects is returned when getting liked objects fails
	ErrGetLikedObjects = errors.FailedToGet("liked objects", stdErrors.New("failed to get liked objects"))

	// ErrGetStatuses is returned when getting statuses fails
	ErrGetStatuses = errors.FailedToGet("statuses", stdErrors.New("failed to get statuses"))

	// Scheduled status errors
	// ErrCreateScheduledStatus is returned when creating scheduled status fails
	ErrCreateScheduledStatus = errors.FailedToCreate("scheduled status", stdErrors.New("failed to create scheduled status"))

	// ErrScheduledTimeInPast is returned when scheduled time must be in the future
	ErrScheduledTimeInPast = errors.TimestampInFuture()

	// Search errors
	// ErrGetSearchSuggestions is returned when getting search suggestions fails
	ErrGetSearchSuggestions = errors.FailedToGet("search suggestions", stdErrors.New("failed to get search suggestions"))

	// Community notes errors
	// ErrCreateCommunityNote is returned when creating community note fails
	ErrCreateCommunityNote = errors.FailedToCreate("community note", stdErrors.New("failed to create community note"))

	// ErrGetVisibleCommunityNotes is returned when getting visible community notes fails
	ErrGetVisibleCommunityNotes = errors.FailedToGet("visible community notes", stdErrors.New("failed to get visible community notes"))

	// ErrGetCommunityNote is returned when getting community note fails
	ErrGetCommunityNote = errors.FailedToGet("community note", stdErrors.New("failed to get community note"))

	// ErrCreateCommunityNoteVote is returned when creating community note vote fails
	ErrCreateCommunityNoteVote = errors.FailedToCreate("community note vote", stdErrors.New("failed to create community note vote"))

	// ErrGetCommunityNotesByAuthor is returned when getting community notes by author fails
	ErrGetCommunityNotesByAuthor = errors.FailedToGet("community notes by author", stdErrors.New("failed to get community notes by author"))

	// Stats errors
	// ErrCountStatusesByAuthor is returned when counting statuses by author fails
	ErrCountStatusesByAuthor = errors.ProcessingFailed("status count by author", stdErrors.New("failed to count statuses by author"))

	// ErrGetUserTimeline is returned when getting user timeline fails
	ErrGetUserTimeline = errors.FailedToGet("user timeline", stdErrors.New("failed to get user timeline"))

	// ErrCountReplies is returned when counting replies fails
	ErrCountReplies = errors.ProcessingFailed("replies count", stdErrors.New("failed to count replies"))

	// ErrGetBoostCount is returned when getting boost count fails
	ErrGetBoostCount = errors.ProcessingFailed("boost count", stdErrors.New("failed to get boost count"))

	// ErrGetLikeCount is returned when getting like count fails
	ErrGetLikeCount = errors.ProcessingFailed("like count", stdErrors.New("failed to get like count"))

	// ErrCheckUserHasLiked is returned when checking if user has liked fails
	ErrCheckUserHasLiked = errors.ProcessingFailed("user liked check", stdErrors.New("failed to verify if user liked"))

	// ErrCheckUserHasReblogged is returned when checking if user has reblogged fails
	ErrCheckUserHasReblogged = errors.ProcessingFailed("user reblogged check", stdErrors.New("failed to verify if user reblogged"))

	// ErrCheckUserHasBookmarked is returned when checking if user has bookmarked fails
	ErrCheckUserHasBookmarked = errors.ProcessingFailed("user bookmarked check", stdErrors.New("failed to verify if user bookmarked"))

	// Permission errors
	// ErrCannotUpdatePostOwnedByOther is returned when user cannot update post owned by another user
	ErrCannotUpdatePostOwnedByOther = errors.InsufficientPermissions("update post")

	// ErrCannotDeletePostOwnedByOther is returned when user cannot delete post owned by another user
	ErrCannotDeletePostOwnedByOther = errors.InsufficientPermissions("delete post")

	// ErrCannotPinPostOwnedByOther is returned when user cannot pin/unpin post owned by another user
	ErrCannotPinPostOwnedByOther = errors.InsufficientPermissions("pin/unpin post")

	// Action execution errors
	// ErrExecuteAction is returned when action execution fails
	ErrExecuteAction = errors.ProcessingFailed("action execution", stdErrors.New("action execution failed"))
)
