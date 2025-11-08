package main

import (
	"time"

	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/pay-theory/lift/pkg/lift"
)

// configureLiftRoutes sets up routes that use native Lift handlers
// This allows gradual migration from Lambda handlers to Lift handlers
func configureLiftRoutes(app *lift.App) {
	// OAuth app registration (public, no auth required)
	_ = app.POST("/api/v1/apps", lift.HandlerFunc(liftHandler.HandleAppRegistrationLift))

	// Wallet authentication endpoints (public, for passwordless login)
	_ = app.POST("/auth/wallet/challenge", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleCreateChallengeLift),
		20, 5*time.Minute, logger))
	_ = app.POST("/auth/wallet/verify", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleVerifySignatureLift),
		20, 5*time.Minute, logger))
	_ = app.POST("/auth/wallet/login", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleLoginWalletLift),
		20, 5*time.Minute, logger))
	_ = app.POST("/auth/wallet/link", lift.HandlerFunc(liftHandler.HandleLinkWalletLift))
	_ = app.DELETE("/auth/wallet/unlink/{address}", lift.HandlerFunc(liftHandler.HandleUnlinkWalletLift))
	_ = app.GET("/auth/wallet/list", lift.HandlerFunc(liftHandler.HandleGetWalletsLift))
	
	// OPTIONS handlers for CORS preflight (CORS headers set by middleware)
	optionsHandler := func(ctx *lift.Context) error {
		// CORS headers are set by middleware, just return 200
		return ctx.Status(200).JSON(map[string]string{"message": "OK"})
	}
	_ = app.Handle("OPTIONS", "/auth/wallet/challenge", optionsHandler)
	_ = app.Handle("OPTIONS", "/auth/wallet/verify", optionsHandler)
	_ = app.Handle("OPTIONS", "/auth/wallet/login", optionsHandler)
	_ = app.Handle("OPTIONS", "/auth/wallet/link", optionsHandler)
	_ = app.Handle("OPTIONS", "/auth/wallet/unlink/{address}", optionsHandler)
	_ = app.Handle("OPTIONS", "/auth/wallet/list", optionsHandler)

	// OAuth endpoints with native Lift implementation + rate limiting
	_ = app.GET("/oauth/authorize", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleOAuthAuthorizeLift),
		20, 5*time.Minute, logger))
	_ = app.POST("/oauth/consent", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleOAuthConsentLift),
		20, 5*time.Minute, logger))
	_ = app.POST("/oauth/token", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleOAuthTokenLift),
		10, time.Minute, logger))
	
	// OPTIONS handlers for OAuth endpoints (CORS preflight)
	_ = app.Handle("OPTIONS", "/oauth/authorize", optionsHandler)
	_ = app.Handle("OPTIONS", "/oauth/consent", optionsHandler)
	_ = app.Handle("OPTIONS", "/oauth/token", optionsHandler)

	// NodeInfo endpoints with native Lift implementation
	_ = app.GET("/.well-known/nodeinfo", lift.HandlerFunc(liftHandler.HandleNodeInfoWellKnownLift))
	_ = app.GET("/nodeinfo/2.0", lift.HandlerFunc(liftHandler.HandleNodeInfoLift))

	// Account verification/update endpoints with native Lift implementation
	// verify_credentials is NOT rate limited (read-only)
	_ = app.GET("/api/v1/accounts/verify_credentials", lift.HandlerFunc(liftHandler.HandleVerifyCredentialsLift))
	// update_credentials IS rate limited
	_ = app.PATCH("/api/v1/accounts/update_credentials", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleUpdateCredentialsLift),
		10, time.Hour, logger))
	// account registration
	_ = app.POST("/api/v1/accounts", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleRegistrationLift),
		10, time.Hour, logger))
	
	// OPTIONS handlers for account endpoints (CORS preflight)
	_ = app.Handle("OPTIONS", "/api/v1/accounts", optionsHandler)
	_ = app.Handle("OPTIONS", "/api/v1/accounts/verify_credentials", optionsHandler)
	_ = app.Handle("OPTIONS", "/api/v1/accounts/update_credentials", optionsHandler)

	// Relationships endpoint with native Lift implementation
	_ = app.GET("/api/v1/accounts/relationships", lift.HandlerFunc(liftHandler.HandleGetRelationshipsLift))

	// Data exports with native Lift implementation
	// POST is rate limited (expensive operation)
	_ = app.POST("/api/v1/exports", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleCreateExportLift),
		5, 24*time.Hour, logger))
	// GETs are NOT rate limited (read-only)
	_ = app.GET("/api/v1/exports/{id}", lift.HandlerFunc(liftHandler.HandleGetExportStatusLift))
	_ = app.GET("/api/v1/exports/{id}/download", lift.HandlerFunc(liftHandler.HandleDownloadExportLift))
	_ = app.GET("/api/v1/exports", lift.HandlerFunc(liftHandler.HandleListExportsLift))

	// Data imports with native Lift implementation
	// POST is rate limited (expensive operation)
	_ = app.POST("/api/v1/imports", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleCreateImportLift),
		5, 24*time.Hour, logger))
	// GETs and DELETE are NOT rate limited
	_ = app.GET("/api/v1/imports/{id}", lift.HandlerFunc(liftHandler.HandleGetImportStatusLift))
	_ = app.DELETE("/api/v1/imports/{id}", lift.HandlerFunc(liftHandler.HandleCancelImportLift))
	_ = app.GET("/api/v1/imports", lift.HandlerFunc(liftHandler.HandleListImportsLift))

	// Community Notes endpoints with native Lift implementation
	// POST create note is rate limited
	_ = app.POST("/api/v1/notes", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleCreateNoteLift),
		20, time.Hour, logger))
	// GETs are NOT rate limited
	_ = app.GET("/api/v1/notes/{object_id}", lift.HandlerFunc(liftHandler.HandleGetNotesLift))
	// POST vote is rate limited
	_ = app.POST("/api/v1/notes/{id}/vote", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleVoteNoteLift),
		100, time.Hour, logger))
	_ = app.GET("/api/v1/accounts/{id}/notes", lift.HandlerFunc(liftHandler.HandleGetUserNotesLift))

	// Status endpoints (Mastodon parity)
	_ = app.POST("/api/v1/statuses", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleCreateStatusLift),
		300, time.Hour, logger))
	_ = app.PUT("/api/v1/statuses/{id}", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleUpdateStatusLift),
		300, time.Hour, logger))
	_ = app.DELETE("/api/v1/statuses/{id}", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleDeleteStatusLift),
		300, time.Hour, logger))
	_ = app.GET("/api/v1/statuses/{id}", lift.HandlerFunc(liftHandler.HandleGetStatusLift))
	_ = app.GET("/api/v1/statuses/{id}/context", lift.HandlerFunc(liftHandler.HandleGetStatusContextLift))
	_ = app.GET("/api/v1/statuses/{id}/history", lift.HandlerFunc(liftHandler.HandleGetStatusHistoryLift))
	_ = app.GET("/api/v1/statuses/{id}/source", lift.HandlerFunc(liftHandler.HandleGetStatusSourceLift))
	_ = app.GET("/api/v1/statuses/{id}/favourited_by", lift.HandlerFunc(liftHandler.HandleGetStatusFavouritedByLift))
	_ = app.GET("/api/v1/statuses/{id}/reblogged_by", lift.HandlerFunc(liftHandler.HandleGetStatusRebloggedByLift))
	_ = app.GET("/api/v1/accounts/{id}/statuses", lift.HandlerFunc(liftHandler.HandleGetAccountStatusesLift))

	// Admin endpoints (always enabled for administration)
	// Note: RBAC is handled within each handler's requireAdminLift() method
	// Account management (Admin only)
	_ = app.GET("/api/v1/admin/accounts", lift.HandlerFunc(liftHandler.HandleAdminGetAccountsLift))
	_ = app.POST("/api/v1/admin/accounts", lift.HandlerFunc(liftHandler.HandleAdminCreateUserLift))
	_ = app.GET("/api/v1/admin/accounts/{id}", lift.HandlerFunc(liftHandler.HandleAdminGetAccountLift))
	_ = app.POST("/api/v1/admin/accounts/{id}/action", lift.HandlerFunc(liftHandler.HandleAdminAccountActionLift))
	_ = app.POST("/api/v1/admin/accounts/{id}/approve", lift.HandlerFunc(liftHandler.HandleAdminApproveAccountLift))
	_ = app.POST("/api/v1/admin/accounts/{id}/reject", lift.HandlerFunc(liftHandler.HandleAdminRejectAccountLift))
	_ = app.POST("/api/v1/admin/accounts/{id}/enable", lift.HandlerFunc(liftHandler.HandleAdminEnableAccountLift))
	_ = app.POST("/api/v1/admin/accounts/{id}/unsilence", lift.HandlerFunc(liftHandler.HandleAdminUnsilenceAccountLift))
	_ = app.POST("/api/v1/admin/accounts/{id}/unsuspend", lift.HandlerFunc(liftHandler.HandleAdminUnsuspendAccountLift))
	_ = app.POST("/api/v1/admin/accounts/{id}/unsensitive", lift.HandlerFunc(liftHandler.HandleAdminUnsensitiveAccountLift))

	// Report management (Admin/Moderator)
	_ = app.GET("/api/v1/admin/reports", lift.HandlerFunc(liftHandler.HandleAdminGetReportsLift))
	_ = app.GET("/api/v1/admin/reports/{id}", lift.HandlerFunc(liftHandler.HandleAdminGetReportLift))
	_ = app.POST("/api/v1/admin/reports/{id}/resolve", lift.HandlerFunc(liftHandler.HandleAdminResolveReportLift))
	_ = app.POST("/api/v1/admin/reports/{id}/reopen", lift.HandlerFunc(liftHandler.HandleAdminReopenReportLift))
	_ = app.POST("/api/v1/admin/reports/{id}/assign_to_self", lift.HandlerFunc(liftHandler.HandleAdminAssignReportLift))
	_ = app.POST("/api/v1/admin/reports/{id}/unassign", lift.HandlerFunc(liftHandler.HandleAdminUnassignReportLift))

	// Status moderation (Admin only for deletion, Admin/Moderator for sensitivity)
	_ = app.GET("/api/v1/admin/statuses", lift.HandlerFunc(liftHandler.HandleAdminGetStatusesLift))
	_ = app.GET("/api/v1/admin/statuses/{id}", lift.HandlerFunc(liftHandler.HandleAdminGetStatusLift))
	_ = app.DELETE("/api/v1/admin/statuses/{id}", lift.HandlerFunc(liftHandler.HandleAdminDeleteStatusLift))
	_ = app.POST("/api/v1/admin/statuses/{id}/sensitive", lift.HandlerFunc(liftHandler.HandleAdminMarkStatusSensitiveLift))
	_ = app.POST("/api/v1/admin/statuses/{id}/unsensitive", lift.HandlerFunc(liftHandler.HandleAdminUnmarkStatusSensitiveLift))

	// Domain blocks (Admin only)
	_ = app.GET("/api/v1/admin/domain_blocks", lift.HandlerFunc(liftHandler.HandleGetAdminDomainBlocksLift))
	_ = app.GET("/api/v1/admin/domain_blocks/{id}", lift.HandlerFunc(liftHandler.HandleGetAdminDomainBlockLift))
	_ = app.POST("/api/v1/admin/domain_blocks", lift.HandlerFunc(liftHandler.HandleCreateAdminDomainBlockLift))
	_ = app.PUT("/api/v1/admin/domain_blocks/{id}", lift.HandlerFunc(liftHandler.HandleUpdateAdminDomainBlockLift))
	_ = app.DELETE("/api/v1/admin/domain_blocks/{id}", lift.HandlerFunc(liftHandler.HandleDeleteAdminDomainBlockLift))

	// Domain allows (Admin only)
	_ = app.GET("/api/v1/admin/domain_allows", lift.HandlerFunc(liftHandler.HandleGetAdminDomainAllowsLift))
	_ = app.POST("/api/v1/admin/domain_allows", lift.HandlerFunc(liftHandler.HandleCreateAdminDomainAllowLift))
	_ = app.DELETE("/api/v1/admin/domain_allows/{id}", lift.HandlerFunc(liftHandler.HandleDeleteAdminDomainAllowLift))

	// Moderation overview and events (Admin/Moderator)
	_ = app.GET("/api/v1/admin/moderation/overview", lift.HandlerFunc(liftHandler.HandleAdminModerationOverviewLift))
	_ = app.GET("/api/v1/admin/moderation/events", lift.HandlerFunc(liftHandler.HandleAdminGetModerationEventsLift))
	_ = app.POST("/api/v1/admin/moderation/events/{id}/override", lift.HandlerFunc(liftHandler.HandleAdminOverrideModerationEventLift))

	// Trust graph management (Admin only)
	_ = app.GET("/api/v1/admin/moderation/trust/graph", lift.HandlerFunc(liftHandler.HandleAdminGetTrustGraphLift))
	_ = app.PUT("/api/v1/admin/moderation/trust/{from}/{to}", lift.HandlerFunc(liftHandler.HandleAdminUpdateTrustLift))

	// Search endpoints with privacy enforcement (always enabled)
	// Account search is rate limited (scraping prevention)
	_ = app.GET("/api/v1/accounts/search", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleAccountSearchLift),
		30, 5*time.Minute, logger))
	// Suggestions are NOT rate limited
	_ = app.GET("/api/v1/accounts/search/suggestions", lift.HandlerFunc(liftHandler.HandleGetSearchSuggestionsLift))
	// Status search is rate limited (GET and POST)
	_ = app.GET("/api/v1/search/statuses", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleStatusSearchLift),
		30, 5*time.Minute, logger))
	_ = app.POST("/api/v1/search/statuses", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleStatusSearchLift),
		30, 5*time.Minute, logger))

	// Relationship interactions
	_ = app.POST("/api/v1/accounts/{id}/follow", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleFollowLift),
		30, 5*time.Minute, logger))
	_ = app.POST("/api/v1/accounts/{id}/unfollow", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleUnfollowLift),
		30, 5*time.Minute, logger))
	_ = app.POST("/api/v1/accounts/{id}/block", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleBlockLift),
		30, 5*time.Minute, logger))
	_ = app.POST("/api/v1/accounts/{id}/unblock", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleUnblockLift),
		30, 5*time.Minute, logger))
	_ = app.GET("/api/v1/blocks", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleGetBlocksLift),
		60, time.Hour, logger))

	// Reviewer management (Admin only)
	_ = app.GET("/api/v1/admin/moderation/reviewers", lift.HandlerFunc(liftHandler.HandleAdminGetReviewersLift))
	_ = app.POST("/api/v1/admin/moderation/reviewers/{id}/promote", lift.HandlerFunc(liftHandler.HandleAdminPromoteModeratorLift))
	_ = app.POST("/api/v1/admin/moderation/reviewers/{id}/demote", lift.HandlerFunc(liftHandler.HandleAdminDemoteModeratorLift))

	// Media endpoints - V1 (synchronous) and V2 (asynchronous)
	// V1 Media endpoints (backwards compatibility)
	// POST is rate limited (storage abuse prevention)
	_ = app.POST("/api/v1/media", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleUploadMediaLift),
		20, time.Hour, logger))
	// GET and PUT are NOT rate limited
	_ = app.GET("/api/v1/media/{id}", lift.HandlerFunc(liftHandler.HandleGetMediaLift))
	_ = app.PUT("/api/v1/media/{id}", lift.HandlerFunc(liftHandler.HandleUpdateMediaLift))

	// Note: V2 media endpoints have been consolidated into main media handlers

	// Conversation endpoints (Direct Messages) - always enabled for 100% Mastodon API compatibility
	_ = app.GET("/api/v1/conversations", lift.HandlerFunc(liftHandler.HandleGetConversationsLift))
	_ = app.DELETE("/api/v1/conversations/{id}", lift.HandlerFunc(liftHandler.HandleDeleteConversationLift))
	_ = app.POST("/api/v1/conversations/{id}/read", lift.HandlerFunc(liftHandler.HandleMarkConversationReadLift))

	// Instance endpoints
	_ = app.GET("/api/v1/instance", lift.HandlerFunc(liftHandler.HandleGetInstanceV1Lift))
	_ = app.GET("/api/v1/instance/peers", lift.HandlerFunc(liftHandler.HandleGetInstancePeersLift))
	_ = app.GET("/api/v1/instance/activity", lift.HandlerFunc(liftHandler.HandleGetInstanceActivityLift))
	_ = app.GET("/api/v1/instance/domain_blocks", lift.HandlerFunc(liftHandler.HandleGetInstanceDomainBlocksLift))

	// API v2 endpoints - Enhanced Mastodon compatibility
	_ = app.GET("/api/v2/instance", lift.HandlerFunc(liftHandler.HandleGetInstanceV2Lift))
	_ = app.GET("/api/v2/search", lift.HandlerFunc(liftHandler.HandleSearchV2Lift))
	_ = app.GET("/api/v2/suggestions", lift.HandlerFunc(liftHandler.HandleGetSuggestionsV2Lift))

	// API v2 filters (advanced filtering) - existing implementations
	_ = app.GET("/api/v2/filters", lift.HandlerFunc(liftHandler.HandleGetFiltersLift))
	_ = app.GET("/api/v2/filters/{id}", lift.HandlerFunc(liftHandler.HandleGetFilterLift))
	_ = app.POST("/api/v2/filters", lift.HandlerFunc(liftHandler.HandleCreateFilterLift))
	_ = app.PUT("/api/v2/filters/{id}", lift.HandlerFunc(liftHandler.HandleUpdateFilterLift))
	_ = app.DELETE("/api/v2/filters/{id}", lift.HandlerFunc(liftHandler.HandleDeleteFilterLift))

	// API v2 filter keywords and statuses
	_ = app.GET("/api/v2/filters/{filter_id}/keywords", lift.HandlerFunc(liftHandler.HandleGetFilterKeywordsLift))
	_ = app.POST("/api/v2/filters/{filter_id}/keywords", lift.HandlerFunc(liftHandler.HandleAddFilterKeywordLift))
	_ = app.DELETE("/api/v2/filters/{filter_id}/keywords/{keyword_id}", lift.HandlerFunc(liftHandler.HandleDeleteFilterKeywordLift))
	_ = app.GET("/api/v2/filters/{filter_id}/statuses", lift.HandlerFunc(liftHandler.HandleGetFilterStatusesLift))
	_ = app.POST("/api/v2/filters/{filter_id}/statuses", lift.HandlerFunc(liftHandler.HandleAddFilterStatusLift))
	_ = app.DELETE("/api/v2/filters/{filter_id}/statuses/{status_id}", lift.HandlerFunc(liftHandler.HandleDeleteFilterStatusLift))

	// API v2 trends endpoints - Enhanced trending with metadata
	_ = app.GET("/api/v2/trends", lift.HandlerFunc(liftHandler.HandleGetTrendsV2Lift))
	_ = app.GET("/api/v2/trends/tags", lift.HandlerFunc(liftHandler.HandleGetTrendingTagsV2Lift))
	_ = app.GET("/api/v2/trends/statuses", lift.HandlerFunc(liftHandler.HandleGetTrendingStatusesV2Lift))
	_ = app.GET("/api/v2/trends/links", lift.HandlerFunc(liftHandler.HandleGetTrendingLinksV2Lift))

	// API v2 filter testing endpoint
	_ = app.POST("/api/v2/filters/test", lift.HandlerFunc(liftHandler.HandleTestFilterLift))

	// API v2 grouped notifications endpoints
	_ = app.GET("/api/v2/notifications/grouped", lift.HandlerFunc(liftHandler.HandleGetGroupedNotificationsLift))
	_ = app.POST("/api/v2/notifications/groups/{group_id}/read", lift.HandlerFunc(liftHandler.HandleMarkGroupAsReadLift))

	// Quote posts API endpoints
	// POST create quote is rate limited (spam prevention)
	_ = app.POST("/api/v1/statuses/{id}/quote", ratelimit.ApplyRateLimit(
		lift.HandlerFunc(liftHandler.HandleCreateQuotePostLift),
		30, time.Hour, logger))
	// GETs, DELETE, and PUT are NOT rate limited
	_ = app.GET("/api/v1/statuses/{id}/quotes", lift.HandlerFunc(liftHandler.HandleGetQuotesOfStatusLift))
	_ = app.DELETE("/api/v1/statuses/{id}/quote/{quote_id}", lift.HandlerFunc(liftHandler.HandleDeleteQuotePostLift))
	_ = app.GET("/api/v1/accounts/{id}/quote_permissions", lift.HandlerFunc(liftHandler.HandleGetQuotePermissionsLift))
	_ = app.PUT("/api/v1/accounts/quote_permissions", lift.HandlerFunc(liftHandler.HandleUpdateQuotePermissionsLift))

	// ActivityPub collection endpoints (always enabled for federation compatibility)
	_ = app.GET("/users/{username}/followers", lift.HandlerFunc(liftHandler.HandleActivityPubFollowersLift))
	_ = app.GET("/users/{username}/following", lift.HandlerFunc(liftHandler.HandleActivityPubFollowingLift))
}
