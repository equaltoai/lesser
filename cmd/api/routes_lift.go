package main

import (
	"os"

	"github.com/pay-theory/lift/pkg/lift"
)

const (
	// Environment variable values
	envTrue = "true"
)

// configureLiftRoutes sets up routes that use native Lift handlers
// This allows gradual migration from Lambda handlers to Lift handlers
func configureLiftRoutes(app *lift.App) {
	// Check if we should use Lift handlers (can be controlled via environment variable)
	useLiftHandlers := os.Getenv("USE_LIFT_HANDLERS") == envTrue

	if useLiftHandlers {
		// OAuth endpoints with native Lift implementation
		_ = app.GET("/oauth/authorize", lift.HandlerFunc(liftHandler.HandleOAuthAuthorizeLift))

		// NodeInfo endpoints with native Lift implementation
		_ = app.GET("/.well-known/nodeinfo", lift.HandlerFunc(liftHandler.HandleNodeInfoWellKnownLift))
		_ = app.GET("/nodeinfo/2.0", lift.HandlerFunc(liftHandler.HandleNodeInfoLift))

		// Relationships endpoint with native Lift implementation
		_ = app.GET("/accounts/relationships", lift.HandlerFunc(liftHandler.HandleGetRelationshipsLift))

		// Data exports with native Lift implementation
		_ = app.POST("/exports", lift.HandlerFunc(liftHandler.HandleCreateExportLift))
		_ = app.GET("/exports/{id}", lift.HandlerFunc(liftHandler.HandleGetExportStatusLift))
		_ = app.GET("/exports/{id}/download", lift.HandlerFunc(liftHandler.HandleDownloadExportLift))
		_ = app.GET("/exports", lift.HandlerFunc(liftHandler.HandleListExportsLift))

		// Data imports with native Lift implementation  
		_ = app.POST("/imports", lift.HandlerFunc(liftHandler.HandleCreateImportLift))
		_ = app.GET("/imports/{id}", lift.HandlerFunc(liftHandler.HandleGetImportStatusLift))
		_ = app.DELETE("/imports/{id}", lift.HandlerFunc(liftHandler.HandleCancelImportLift))
		_ = app.GET("/imports", lift.HandlerFunc(liftHandler.HandleListImportsLift))

		// Community Notes endpoints with native Lift implementation
		_ = app.POST("/notes", lift.HandlerFunc(liftHandler.HandleCreateNoteLift))
		_ = app.GET("/notes/{object_id}", lift.HandlerFunc(liftHandler.HandleGetNotesLift))
		_ = app.POST("/notes/{id}/vote", lift.HandlerFunc(liftHandler.HandleVoteNoteLift))
		_ = app.GET("/accounts/{id}/notes", lift.HandlerFunc(liftHandler.HandleGetUserNotesLift))

	}

	// Admin endpoints (always enabled for administration)
	// Note: RBAC is handled within each handler's requireAdminLift() method
	// Account management (Admin only)
	_ = app.GET("/admin/accounts", lift.HandlerFunc(liftHandler.HandleAdminGetAccountsLift))
	_ = app.GET("/admin/accounts/{id}", lift.HandlerFunc(liftHandler.HandleAdminGetAccountLift))
	_ = app.POST("/admin/accounts/{id}/action", lift.HandlerFunc(liftHandler.HandleAdminAccountActionLift))
	_ = app.POST("/admin/accounts/{id}/approve", lift.HandlerFunc(liftHandler.HandleAdminApproveAccountLift))
	_ = app.POST("/admin/accounts/{id}/reject", lift.HandlerFunc(liftHandler.HandleAdminRejectAccountLift))
	_ = app.POST("/admin/accounts/{id}/enable", lift.HandlerFunc(liftHandler.HandleAdminEnableAccountLift))
	_ = app.POST("/admin/accounts/{id}/unsilence", lift.HandlerFunc(liftHandler.HandleAdminUnsilenceAccountLift))
	_ = app.POST("/admin/accounts/{id}/unsuspend", lift.HandlerFunc(liftHandler.HandleAdminUnsuspendAccountLift))
	_ = app.POST("/admin/accounts/{id}/unsensitive", lift.HandlerFunc(liftHandler.HandleAdminUnsensitiveAccountLift))

	// Report management (Admin/Moderator)
	_ = app.GET("/admin/reports", lift.HandlerFunc(liftHandler.HandleAdminGetReportsLift))
	_ = app.GET("/admin/reports/{id}", lift.HandlerFunc(liftHandler.HandleAdminGetReportLift))
	_ = app.POST("/admin/reports/{id}/resolve", lift.HandlerFunc(liftHandler.HandleAdminResolveReportLift))
	_ = app.POST("/admin/reports/{id}/reopen", lift.HandlerFunc(liftHandler.HandleAdminReopenReportLift))
	_ = app.POST("/admin/reports/{id}/assign_to_self", lift.HandlerFunc(liftHandler.HandleAdminAssignReportLift))
	_ = app.POST("/admin/reports/{id}/unassign", lift.HandlerFunc(liftHandler.HandleAdminUnassignReportLift))

	// Status moderation (Admin only for deletion, Admin/Moderator for sensitivity)
	_ = app.GET("/admin/statuses", lift.HandlerFunc(liftHandler.HandleAdminGetStatusesLift))
	_ = app.GET("/admin/statuses/{id}", lift.HandlerFunc(liftHandler.HandleAdminGetStatusLift))
	_ = app.DELETE("/admin/statuses/{id}", lift.HandlerFunc(liftHandler.HandleAdminDeleteStatusLift))
	_ = app.POST("/admin/statuses/{id}/sensitive", lift.HandlerFunc(liftHandler.HandleAdminMarkStatusSensitiveLift))
	_ = app.POST("/admin/statuses/{id}/unsensitive", lift.HandlerFunc(liftHandler.HandleAdminUnmarkStatusSensitiveLift))

	// Domain blocks (Admin only)
	_ = app.GET("/admin/domain_blocks", lift.HandlerFunc(liftHandler.HandleGetAdminDomainBlocksLift))
	_ = app.GET("/admin/domain_blocks/{id}", lift.HandlerFunc(liftHandler.HandleGetAdminDomainBlockLift))
	_ = app.POST("/admin/domain_blocks", lift.HandlerFunc(liftHandler.HandleCreateAdminDomainBlockLift))
	_ = app.PUT("/admin/domain_blocks/{id}", lift.HandlerFunc(liftHandler.HandleUpdateAdminDomainBlockLift))
	_ = app.DELETE("/admin/domain_blocks/{id}", lift.HandlerFunc(liftHandler.HandleDeleteAdminDomainBlockLift))

	// Domain allows (Admin only)
	_ = app.GET("/admin/domain_allows", lift.HandlerFunc(liftHandler.HandleGetAdminDomainAllowsLift))
	_ = app.POST("/admin/domain_allows", lift.HandlerFunc(liftHandler.HandleCreateAdminDomainAllowLift))
	_ = app.DELETE("/admin/domain_allows/{id}", lift.HandlerFunc(liftHandler.HandleDeleteAdminDomainAllowLift))

	// Moderation overview and events (Admin/Moderator)
	_ = app.GET("/admin/moderation/overview", lift.HandlerFunc(liftHandler.HandleAdminModerationOverviewLift))
	_ = app.GET("/admin/moderation/events", lift.HandlerFunc(liftHandler.HandleAdminGetModerationEventsLift))
	_ = app.POST("/admin/moderation/events/{id}/override", lift.HandlerFunc(liftHandler.HandleAdminOverrideModerationEventLift))

	// Trust graph management (Admin only)
	_ = app.GET("/admin/moderation/trust/graph", lift.HandlerFunc(liftHandler.HandleAdminGetTrustGraphLift))
	_ = app.PUT("/admin/moderation/trust/{from}/{to}", lift.HandlerFunc(liftHandler.HandleAdminUpdateTrustLift))

	// Search endpoints with privacy enforcement (always enabled)
	_ = app.GET("/api/v1/accounts/search", lift.HandlerFunc(liftHandler.HandleAccountSearchLift))
	_ = app.GET("/api/v1/accounts/search/suggestions", lift.HandlerFunc(liftHandler.HandleGetSearchSuggestionsLift))
	_ = app.GET("/api/v1/search/statuses", lift.HandlerFunc(liftHandler.HandleStatusSearchLift))
	_ = app.POST("/api/v1/search/statuses", lift.HandlerFunc(liftHandler.HandleStatusSearchLift))

	// Reviewer management (Admin only)
	_ = app.GET("/admin/moderation/reviewers", lift.HandlerFunc(liftHandler.HandleAdminGetReviewersLift))
	_ = app.POST("/admin/moderation/reviewers/{id}/promote", lift.HandlerFunc(liftHandler.HandleAdminPromoteModeratorLift))
	_ = app.POST("/admin/moderation/reviewers/{id}/demote", lift.HandlerFunc(liftHandler.HandleAdminDemoteModeratorLift))

	// Media endpoints - V1 (synchronous) and V2 (asynchronous) 
	// V1 Media endpoints (backwards compatibility)
	_ = app.POST("/media", lift.HandlerFunc(liftHandler.HandleUploadMediaLift))
	_ = app.GET("/media/{id}", lift.HandlerFunc(liftHandler.HandleGetMediaLift))
	_ = app.PUT("/media/{id}", lift.HandlerFunc(liftHandler.HandleUpdateMediaLift))

	// Note: V2 media endpoints have been consolidated into main media handlers

	// Conversation endpoints (Direct Messages) - always enabled for 100% Mastodon API compatibility
	_ = app.GET("/api/v1/conversations", lift.HandlerFunc(liftHandler.HandleGetConversationsLift))
	_ = app.DELETE("/api/v1/conversations/{id}", lift.HandlerFunc(liftHandler.HandleDeleteConversationLift))
	_ = app.POST("/api/v1/conversations/{id}/read", lift.HandlerFunc(liftHandler.HandleMarkConversationReadLift))

	// ActivityPub collection endpoints (always enabled for federation compatibility)
	_ = app.GET("/users/{username}/followers", lift.HandlerFunc(liftHandler.HandleActivityPubFollowersLift))
	_ = app.GET("/users/{username}/following", lift.HandlerFunc(liftHandler.HandleActivityPubFollowingLift))
}
