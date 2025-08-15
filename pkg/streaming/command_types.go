// Package streaming defines WebSocket command types and constants
package streaming

// Command type constants for WebSocket commands
const (
	// Status/Note Commands
	CmdCreateStatus     = "create_status"
	CmdDeleteStatus     = "delete_status"
	CmdFavoriteStatus   = "favorite_status"
	CmdUnfavoriteStatus = "unfavorite_status"
	CmdReblogStatus     = "reblog_status"
	CmdUnreblogStatus   = "unreblog_status"
	CmdBookmarkStatus   = "bookmark_status"
	CmdUnbookmarkStatus = "unbookmark_status"
	CmdMuteStatus       = "mute_status"
	CmdUnmuteStatus     = "unmute_status"
	CmdPinStatus        = "pin_status"
	CmdUnpinStatus      = "unpin_status"

	// Account/User Commands
	CmdFollowUser        = "follow_user"
	CmdUnfollowUser      = "unfollow_user"
	CmdBlockUser         = "block_user"
	CmdUnblockUser       = "unblock_user"
	CmdMuteUser          = "mute_user"
	CmdUnmuteUser        = "unmute_user"
	CmdUpdateProfile     = "update_profile"
	CmdUpdatePreferences = "update_preferences"

	// Relationship Commands
	CmdAcceptFollowRequest = "accept_follow_request"
	CmdRejectFollowRequest = "reject_follow_request"
	CmdRemoveFollower      = "remove_follower"

	// List Commands
	CmdCreateList     = "create_list"
	CmdUpdateList     = "update_list"
	CmdDeleteList     = "delete_list"
	CmdAddToList      = "add_to_list"
	CmdRemoveFromList = "remove_from_list"

	// Media Commands
	CmdUploadMedia = "upload_media"
	CmdUpdateMedia = "update_media"
	CmdDeleteMedia = "delete_media"

	// Conversation Commands
	CmdMarkConversationRead = "mark_conversation_read"
	CmdDeleteConversation   = "delete_conversation"

	// Notification Commands
	CmdMarkNotificationRead     = "mark_notification_read"
	CmdMarkAllNotificationsRead = "mark_all_notifications_read"
	CmdDismissNotification      = "dismiss_notification"

	// Scheduled Status Commands
	CmdCreateScheduledStatus = "create_scheduled_status"
	CmdUpdateScheduledStatus = "update_scheduled_status"
	CmdDeleteScheduledStatus = "delete_scheduled_status"

	// Poll Commands
	CmdVoteInPoll = "vote_in_poll"

	// Search Commands
	CmdSearchAccounts = "search_accounts"
	CmdSearchStatuses = "search_statuses"
	CmdSearchHashtags = "search_hashtags"

	// Admin Commands (require admin privileges)
	CmdAdminSuspendUser   = "admin_suspend_user"
	CmdAdminUnsuspendUser = "admin_unsuspend_user"
	CmdAdminSilenceUser   = "admin_silence_user"
	CmdAdminUnsilenceUser = "admin_unsilence_user"

	// System Commands
	CmdGetServerInfo       = "get_server_info"
	CmdGetTimeline         = "get_timeline"
	CmdGetNotifications    = "get_notifications"
	CmdSubscribeTimeline   = "subscribe_timeline"
	CmdUnsubscribeTimeline = "unsubscribe_timeline"

	// Bulk Operations Commands
	CmdBulkFollow         = "bulk_follow"
	CmdBulkUnfollow       = "bulk_unfollow"
	CmdBulkMute           = "bulk_mute"
	CmdBulkUnmute         = "bulk_unmute"
	CmdBulkBlock          = "bulk_block"
	CmdBulkUnblock        = "bulk_unblock"
	CmdBulkDeleteStatuses = "bulk_delete_statuses"
	CmdBulkListMembers    = "bulk_list_members"
	CmdGetBulkOperation   = "get_bulk_operation"
	
	// Bulk Content Management Commands
	CmdBulkDelete  = "bulk_delete"
	CmdBulkArchive = "bulk_archive"
	CmdBulkRestore = "bulk_restore"
	CmdBulkExport  = "bulk_export"

	// Import/Export Commands
	CmdCreateExport = "create_export"
	CmdGetExport    = "get_export"
	CmdListExports  = "list_exports"
	CmdCancelExport = "cancel_export"
	CmdCreateImport = "create_import"
	CmdGetImport    = "get_import"
	CmdListImports  = "list_imports"
)

// Command categories for organization and access control
const (
	CategoryStatus       = "status"
	CategoryAccount      = "account"
	CategoryRelationship = "relationship"
	CategoryList         = "list"
	CategoryMedia        = "media"
	CategoryConversation = "conversation"
	CategoryNotification = "notification"
	CategoryScheduled    = "scheduled"
	CategoryPoll         = "poll"
	CategorySearch       = "search"
	CategoryAdmin        = "admin"
	CategorySystem       = "system"
	CategoryBulk         = "bulk"
	CategoryExport       = "export"
)

// CommandInfo contains metadata about a command
type CommandInfo struct {
	Type           string   `json:"type"`
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	RequiresAuth   bool     `json:"requires_auth"`
	AdminOnly      bool     `json:"admin_only"`
	RequiredFields []string `json:"required_fields,omitempty"`
	OptionalFields []string `json:"optional_fields,omitempty"`
}

// GetCommandInfo returns metadata for all supported commands
func GetCommandInfo() map[string]CommandInfo {
	return map[string]CommandInfo{
		// Status Commands
		CmdCreateStatus: {
			Type:           CmdCreateStatus,
			Category:       CategoryStatus,
			Description:    "Create a new status/post",
			RequiresAuth:   true,
			RequiredFields: []string{"status"},
			OptionalFields: []string{"in_reply_to_id", "media_ids", "poll", "sensitive", "spoiler_text", "visibility", "language", "scheduled_at"},
		},
		CmdDeleteStatus: {
			Type:           CmdDeleteStatus,
			Category:       CategoryStatus,
			Description:    "Delete an existing status",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdFavoriteStatus: {
			Type:           CmdFavoriteStatus,
			Category:       CategoryStatus,
			Description:    "Favorite/like a status",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdUnfavoriteStatus: {
			Type:           CmdUnfavoriteStatus,
			Category:       CategoryStatus,
			Description:    "Remove favorite/like from a status",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdReblogStatus: {
			Type:           CmdReblogStatus,
			Category:       CategoryStatus,
			Description:    "Reblog/boost a status",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
			OptionalFields: []string{"visibility"},
		},
		CmdUnreblogStatus: {
			Type:           CmdUnreblogStatus,
			Category:       CategoryStatus,
			Description:    "Remove reblog/boost from a status",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},

		// Account Commands
		CmdFollowUser: {
			Type:           CmdFollowUser,
			Category:       CategoryAccount,
			Description:    "Follow a user account",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
			OptionalFields: []string{"reblogs", "notify"},
		},
		CmdUnfollowUser: {
			Type:           CmdUnfollowUser,
			Category:       CategoryAccount,
			Description:    "Unfollow a user account",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdBlockUser: {
			Type:           CmdBlockUser,
			Category:       CategoryAccount,
			Description:    "Block a user account",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdUnblockUser: {
			Type:           CmdUnblockUser,
			Category:       CategoryAccount,
			Description:    "Unblock a user account",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdMuteUser: {
			Type:           CmdMuteUser,
			Category:       CategoryAccount,
			Description:    "Mute a user account",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
			OptionalFields: []string{"notifications", "duration"},
		},
		CmdUnmuteUser: {
			Type:           CmdUnmuteUser,
			Category:       CategoryAccount,
			Description:    "Unmute a user account",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},

		// Relationship Commands
		CmdAcceptFollowRequest: {
			Type:           CmdAcceptFollowRequest,
			Category:       CategoryRelationship,
			Description:    "Accept a follow request",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdRejectFollowRequest: {
			Type:           CmdRejectFollowRequest,
			Category:       CategoryRelationship,
			Description:    "Reject a follow request",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},

		// List Commands
		CmdCreateList: {
			Type:           CmdCreateList,
			Category:       CategoryList,
			Description:    "Create a new list",
			RequiresAuth:   true,
			RequiredFields: []string{"title"},
			OptionalFields: []string{"replies_policy"},
		},
		CmdUpdateList: {
			Type:           CmdUpdateList,
			Category:       CategoryList,
			Description:    "Update an existing list",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
			OptionalFields: []string{"title", "replies_policy"},
		},
		CmdDeleteList: {
			Type:           CmdDeleteList,
			Category:       CategoryList,
			Description:    "Delete a list",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdAddToList: {
			Type:           CmdAddToList,
			Category:       CategoryList,
			Description:    "Add accounts to a list",
			RequiresAuth:   true,
			RequiredFields: []string{"id", "account_ids"},
		},
		CmdRemoveFromList: {
			Type:           CmdRemoveFromList,
			Category:       CategoryList,
			Description:    "Remove accounts from a list",
			RequiresAuth:   true,
			RequiredFields: []string{"id", "account_ids"},
		},

		// Media Commands
		CmdUploadMedia: {
			Type:           CmdUploadMedia,
			Category:       CategoryMedia,
			Description:    "Upload media file",
			RequiresAuth:   true,
			RequiredFields: []string{"file_data", "file_name"},
			OptionalFields: []string{"description", "focus"},
		},

		// Notification Commands
		CmdMarkNotificationRead: {
			Type:           CmdMarkNotificationRead,
			Category:       CategoryNotification,
			Description:    "Mark a notification as read",
			RequiresAuth:   true,
			RequiredFields: []string{"id"},
		},
		CmdMarkAllNotificationsRead: {
			Type:         CmdMarkAllNotificationsRead,
			Category:     CategoryNotification,
			Description:  "Mark all notifications as read",
			RequiresAuth: true,
		},

		// Search Commands
		CmdSearchAccounts: {
			Type:           CmdSearchAccounts,
			Category:       CategorySearch,
			Description:    "Search for user accounts",
			RequiresAuth:   false,
			RequiredFields: []string{"q"},
			OptionalFields: []string{"limit", "offset", "resolve", "following"},
		},

		// System Commands
		CmdGetServerInfo: {
			Type:         CmdGetServerInfo,
			Category:     CategorySystem,
			Description:  "Get server instance information",
			RequiresAuth: false,
		},
		CmdGetTimeline: {
			Type:           CmdGetTimeline,
			Category:       CategorySystem,
			Description:    "Get timeline posts",
			RequiresAuth:   false,
			RequiredFields: []string{"timeline"},
			OptionalFields: []string{"max_id", "since_id", "limit", "local", "remote"},
		},
		CmdGetNotifications: {
			Type:           CmdGetNotifications,
			Category:       CategorySystem,
			Description:    "Get user notifications",
			RequiresAuth:   true,
			OptionalFields: []string{"max_id", "since_id", "limit", "types", "exclude_types"},
		},

		// Admin Commands
		CmdAdminSuspendUser: {
			Type:           CmdAdminSuspendUser,
			Category:       CategoryAdmin,
			Description:    "Suspend a user account (admin only)",
			RequiresAuth:   true,
			AdminOnly:      true,
			RequiredFields: []string{"id"},
			OptionalFields: []string{"reason"},
		},
		CmdAdminUnsuspendUser: {
			Type:           CmdAdminUnsuspendUser,
			Category:       CategoryAdmin,
			Description:    "Unsuspend a user account (admin only)",
			RequiresAuth:   true,
			AdminOnly:      true,
			RequiredFields: []string{"id"},
		},

		// Bulk Operations Commands
		CmdBulkFollow: {
			Type:           CmdBulkFollow,
			Category:       CategoryBulk,
			Description:    "Follow multiple accounts in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"account_ids"},
			OptionalFields: []string{"reblogs", "notify", "languages"},
		},
		CmdBulkUnfollow: {
			Type:           CmdBulkUnfollow,
			Category:       CategoryBulk,
			Description:    "Unfollow multiple accounts in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"account_ids"},
		},
		CmdBulkMute: {
			Type:           CmdBulkMute,
			Category:       CategoryBulk,
			Description:    "Mute multiple accounts in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"account_ids"},
			OptionalFields: []string{"notifications", "duration"},
		},
		CmdBulkUnmute: {
			Type:           CmdBulkUnmute,
			Category:       CategoryBulk,
			Description:    "Unmute multiple accounts in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"account_ids"},
		},
		CmdBulkBlock: {
			Type:           CmdBulkBlock,
			Category:       CategoryBulk,
			Description:    "Block multiple accounts in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"account_ids"},
		},
		CmdBulkUnblock: {
			Type:           CmdBulkUnblock,
			Category:       CategoryBulk,
			Description:    "Unblock multiple accounts in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"account_ids"},
		},
		CmdBulkDeleteStatuses: {
			Type:           CmdBulkDeleteStatuses,
			Category:       CategoryBulk,
			Description:    "Delete multiple statuses in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"status_ids"},
			OptionalFields: []string{"date_range", "keep_pinned"},
		},
		CmdBulkListMembers: {
			Type:           CmdBulkListMembers,
			Category:       CategoryBulk,
			Description:    "Add or remove multiple accounts from a list",
			RequiresAuth:   true,
			RequiredFields: []string{"list_id", "account_ids", "operation"},
		},
		CmdGetBulkOperation: {
			Type:           CmdGetBulkOperation,
			Category:       CategoryBulk,
			Description:    "Get the status of a bulk operation",
			RequiresAuth:   true,
			RequiredFields: []string{"operation_id"},
		},
		CmdBulkDelete: {
			Type:           CmdBulkDelete,
			Category:       CategoryBulk,
			Description:    "Delete multiple posts/content in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"content_ids"},
			OptionalFields: []string{"content_type", "date_range", "permanent"},
		},
		CmdBulkArchive: {
			Type:           CmdBulkArchive,
			Category:       CategoryBulk,
			Description:    "Archive multiple posts in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"content_ids"},
			OptionalFields: []string{"content_type", "date_range"},
		},
		CmdBulkRestore: {
			Type:           CmdBulkRestore,
			Category:       CategoryBulk,
			Description:    "Restore multiple archived posts in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"content_ids"},
			OptionalFields: []string{"content_type", "date_range"},
		},
		CmdBulkExport: {
			Type:           CmdBulkExport,
			Category:       CategoryBulk,
			Description:    "Export multiple pieces of content in bulk",
			RequiresAuth:   true,
			RequiredFields: []string{"content_ids", "format"},
			OptionalFields: []string{"content_type", "include_media", "date_range"},
		},

		// Import/Export Commands
		CmdCreateExport: {
			Type:           CmdCreateExport,
			Category:       CategoryExport,
			Description:    "Create a new data export request",
			RequiresAuth:   true,
			RequiredFields: []string{"type", "format"},
			OptionalFields: []string{"include_media", "date_range", "options"},
		},
		CmdGetExport: {
			Type:           CmdGetExport,
			Category:       CategoryExport,
			Description:    "Get the status and details of an export",
			RequiresAuth:   true,
			RequiredFields: []string{"export_id"},
		},
		CmdListExports: {
			Type:           CmdListExports,
			Category:       CategoryExport,
			Description:    "List all exports for the current user",
			RequiresAuth:   true,
			OptionalFields: []string{"status", "limit", "cursor"},
		},
		CmdCancelExport: {
			Type:           CmdCancelExport,
			Category:       CategoryExport,
			Description:    "Cancel a pending or processing export",
			RequiresAuth:   true,
			RequiredFields: []string{"export_id"},
		},
		CmdCreateImport: {
			Type:           CmdCreateImport,
			Category:       CategoryExport,
			Description:    "Create a new data import request",
			RequiresAuth:   true,
			RequiredFields: []string{"type", "format", "file_url", "merge_strategy"},
			OptionalFields: []string{"options"},
		},
		CmdGetImport: {
			Type:           CmdGetImport,
			Category:       CategoryExport,
			Description:    "Get the status and details of an import",
			RequiresAuth:   true,
			RequiredFields: []string{"import_id"},
		},
		CmdListImports: {
			Type:           CmdListImports,
			Category:       CategoryExport,
			Description:    "List all imports for the current user",
			RequiresAuth:   true,
			OptionalFields: []string{"status", "limit", "cursor"},
		},
	}
}

// GetCommandsByCategory returns commands grouped by category
func GetCommandsByCategory() map[string][]string {
	categories := make(map[string][]string)
	commandInfo := GetCommandInfo()

	for cmdType, info := range commandInfo {
		categories[info.Category] = append(categories[info.Category], cmdType)
	}

	return categories
}

// GetRequiredAuth returns whether a command requires authentication
func GetRequiredAuth(commandType string) bool {
	if info, exists := GetCommandInfo()[commandType]; exists {
		return info.RequiresAuth
	}
	return true // Default to requiring auth for unknown commands
}

// IsAdminOnly returns whether a command is admin-only
func IsAdminOnly(commandType string) bool {
	if info, exists := GetCommandInfo()[commandType]; exists {
		return info.AdminOnly
	}
	return false
}
