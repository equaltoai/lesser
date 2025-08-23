package notes

import "errors"

// Notes service specific errors
var (
	// Notes validation errors
	// ErrNotesValidationFailed is returned when notes validation fails
	ErrNotesValidationFailed = errors.New("validation failed")

	// ErrGetAuthorAccount is returned when getting author account fails
	ErrGetAuthorAccount = errors.New("failed to get author account")

	// ErrCreateStatus is returned when status creation fails
	ErrCreateStatus = errors.New("failed to create status")

	// ErrGetStatus is returned when status retrieval fails
	ErrGetStatus = errors.New("failed to get status")

	// ErrUpdateStatus is returned when status update fails
	ErrUpdateStatus = errors.New("failed to update status")

	// ErrDeleteStatus is returned when status deletion fails
	ErrDeleteStatus = errors.New("failed to delete status")

	// ErrStatusNotFound is returned when status is not found
	ErrStatusNotFound = errors.New("status not found")

	// ErrStatusIDRequired is returned when status ID is required but missing
	ErrStatusIDRequired = errors.New("status_id is required")

	// ErrCheckViewPermissions is returned when checking view permissions fails
	ErrCheckViewPermissions = errors.New("failed to check view permissions")

	// ErrCheckFollowingRelationship is returned when checking following relationship fails
	ErrCheckFollowingRelationship = errors.New("failed to check following relationship")

	// Timeline errors
	// ErrHomeTimelineRequiresViewerID is returned when home timeline requires viewer_id
	ErrHomeTimelineRequiresViewerID = errors.New("home timeline requires viewer_id")

	// ErrUserTimelineRequiresAuthorID is returned when user timeline requires author_id
	ErrUserTimelineRequiresAuthorID = errors.New("user timeline requires author_id")

	// ErrConversationsTimelineRequiresConversationID is returned when conversations timeline requires conversation_id
	ErrConversationsTimelineRequiresConversationID = errors.New("conversations timeline requires conversation_id")

	// ErrDirectTimelineRequiresViewerID is returned when direct timeline requires viewer_id
	ErrDirectTimelineRequiresViewerID = errors.New("direct timeline requires viewer_id")

	// ErrHashtagTimelineRequiresHashtag is returned when hashtag timeline requires hashtag
	ErrHashtagTimelineRequiresHashtag = errors.New("hashtag timeline requires hashtag")

	// ErrListTimelineRequiresListID is returned when list timeline requires list_id
	ErrListTimelineRequiresListID = errors.New("list timeline requires list_id")

	// ErrUnsupportedTimelineType is returned when timeline type is unsupported
	ErrUnsupportedTimelineType = errors.New("unsupported timeline type")

	// ErrGetTimeline is returned when timeline retrieval fails
	ErrGetTimeline = errors.New("failed to get timeline")

	// Content validation errors
	// ErrStatusContentValidationFailed is returned when status content validation fails
	ErrStatusContentValidationFailed = errors.New("status content validation failed")

	// ErrVisibilityValidationFailed is returned when visibility validation fails
	ErrVisibilityValidationFailed = errors.New("visibility validation failed")

	// ErrSpoilerTextValidationFailed is returned when spoiler text validation fails
	ErrSpoilerTextValidationFailed = errors.New("spoiler text validation failed")

	// ErrInvalidInReplyToID is returned when in_reply_to_id is invalid
	ErrInvalidInReplyToID = errors.New("invalid in_reply_to_id")

	// ErrContentCannotBeEmpty is returned when content cannot be empty
	ErrContentCannotBeEmpty = errors.New("content cannot be empty")

	// ErrContentTooLong is returned when content is too long
	ErrContentTooLong = errors.New("content too long (max 5000 characters)")

	// ErrContentTooLongShort is returned when content is too long for short form
	ErrContentTooLongShort = errors.New("content too long (max 500 characters)")

	// Bookmark errors
	// ErrBookmarkStatus is returned when bookmarking status fails
	ErrBookmarkStatus = errors.New("failed to bookmark status")

	// ErrUnbookmarkStatus is returned when unbookmarking status fails
	ErrUnbookmarkStatus = errors.New("failed to unbookmark status")

	// ErrGetBookmarks is returned when getting bookmarks fails
	ErrGetBookmarks = errors.New("failed to get bookmarks")

	// Likes and reblog errors
	// ErrGetLikers is returned when getting likers fails
	ErrGetLikers = errors.New("failed to get likers")

	// ErrGetRebloggerAccount is returned when getting reblogger account fails
	ErrGetRebloggerAccount = errors.New("failed to get reblogger account")

	// ErrReblogStatus is returned when reblogging status fails
	ErrReblogStatus = errors.New("failed to reblog status")

	// ErrGetRebloggers is returned when getting rebloggers fails
	ErrGetRebloggers = errors.New("failed to get rebloggers")

	// ErrMuteStatus is returned when muting status fails
	ErrMuteStatus = errors.New("failed to mute status")

	// ErrUnmuteStatus is returned when unmuting status fails
	ErrUnmuteStatus = errors.New("failed to unmute status")

	// ErrCreateLike is returned when creating like fails
	ErrCreateLike = errors.New("failed to create like")

	// ErrDeleteLike is returned when deleting like fails
	ErrDeleteLike = errors.New("failed to delete like")

	// ErrGetLikes is returned when getting likes fails
	ErrGetLikes = errors.New("failed to get likes")

	// ErrCreateReblog is returned when creating reblog fails
	ErrCreateReblog = errors.New("failed to create reblog")

	// ErrDeleteReblog is returned when deleting reblog fails
	ErrDeleteReblog = errors.New("failed to delete reblog")

	// ErrGetAnnounces is returned when getting announces fails
	ErrGetAnnounces = errors.New("failed to get announces")

	// Pin errors
	// ErrPinStatus is returned when pinning status fails
	ErrPinStatus = errors.New("failed to pin status")

	// ErrUnpinStatus is returned when unpinning status fails
	ErrUnpinStatus = errors.New("failed to unpin status")

	// Conversation errors
	// ErrConversationServiceNotAvailable is returned when conversation service is not available
	ErrConversationServiceNotAvailable = errors.New("conversation service not available")

	// ErrMuteConversation is returned when muting conversation fails
	ErrMuteConversation = errors.New("failed to mute conversation")

	// ErrUnmuteConversation is returned when unmuting conversation fails
	ErrUnmuteConversation = errors.New("failed to unmute conversation")

	// ErrGetConversations is returned when getting conversations fails
	ErrGetConversations = errors.New("failed to get conversations")

	// Favorited timeline errors
	// ErrViewerIDRequiredForFavoritedTimeline is returned when viewer_id is required for favorited timeline
	ErrViewerIDRequiredForFavoritedTimeline = errors.New("viewer_id is required for favorited timeline")

	// ErrGetViewerAccount is returned when getting viewer account fails
	ErrGetViewerAccount = errors.New("failed to get viewer account")

	// ErrLikeRepositoryNotAvailable is returned when like repository is not available
	ErrLikeRepositoryNotAvailable = errors.New("like repository not available")

	// ErrGetLikedObjects is returned when getting liked objects fails
	ErrGetLikedObjects = errors.New("failed to get liked objects")

	// ErrGetStatuses is returned when getting statuses fails
	ErrGetStatuses = errors.New("failed to get statuses")

	// Scheduled status errors
	// ErrCreateScheduledStatus is returned when creating scheduled status fails
	ErrCreateScheduledStatus = errors.New("failed to create scheduled status")

	// ErrScheduledTimeInPast is returned when scheduled time must be in the future
	ErrScheduledTimeInPast = errors.New("scheduled time must be in the future")

	// Search errors
	// ErrGetSearchSuggestions is returned when getting search suggestions fails
	ErrGetSearchSuggestions = errors.New("failed to get search suggestions")

	// Community notes errors
	// ErrCreateCommunityNote is returned when creating community note fails
	ErrCreateCommunityNote = errors.New("failed to create community note")

	// ErrGetVisibleCommunityNotes is returned when getting visible community notes fails
	ErrGetVisibleCommunityNotes = errors.New("failed to get visible community notes")

	// ErrGetCommunityNote is returned when getting community note fails
	ErrGetCommunityNote = errors.New("failed to get community note")

	// ErrCreateCommunityNoteVote is returned when creating community note vote fails
	ErrCreateCommunityNoteVote = errors.New("failed to create community note vote")

	// ErrGetCommunityNotesByAuthor is returned when getting community notes by author fails
	ErrGetCommunityNotesByAuthor = errors.New("failed to get community notes by author")

	// Stats errors
	// ErrCountStatusesByAuthor is returned when counting statuses by author fails
	ErrCountStatusesByAuthor = errors.New("failed to count statuses by author")

	// ErrGetUserTimeline is returned when getting user timeline fails
	ErrGetUserTimeline = errors.New("failed to get user timeline")

	// ErrCountReplies is returned when counting replies fails
	ErrCountReplies = errors.New("failed to count replies")

	// ErrGetBoostCount is returned when getting boost count fails
	ErrGetBoostCount = errors.New("failed to get boost count")

	// ErrGetLikeCount is returned when getting like count fails
	ErrGetLikeCount = errors.New("failed to get like count")

	// ErrCheckUserHasLiked is returned when checking if user has liked fails
	ErrCheckUserHasLiked = errors.New("failed to check if user has liked")

	// ErrCheckUserHasReblogged is returned when checking if user has reblogged fails
	ErrCheckUserHasReblogged = errors.New("failed to check if user has reblogged")

	// ErrCheckUserHasBookmarked is returned when checking if user has bookmarked fails
	ErrCheckUserHasBookmarked = errors.New("failed to check if user has bookmarked")

	// Permission errors
	// ErrCannotUpdatePostOwnedByOther is returned when user cannot update post owned by another user
	ErrCannotUpdatePostOwnedByOther = errors.New("cannot update post owned by another user")

	// ErrCannotDeletePostOwnedByOther is returned when user cannot delete post owned by another user
	ErrCannotDeletePostOwnedByOther = errors.New("cannot delete post owned by another user")

	// ErrCannotPinPostOwnedByOther is returned when user cannot pin/unpin post owned by another user
	ErrCannotPinPostOwnedByOther = errors.New("cannot pin/unpin post owned by another user")

	// Action execution errors
	// ErrExecuteAction is returned when action execution fails
	ErrExecuteAction = errors.New("failed to execute action")
)