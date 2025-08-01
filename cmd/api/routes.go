package main

import (
	"github.com/pay-theory/lift/pkg/lift"
)

// configurePublicRoutes configures routes that don't require authentication
func configurePublicRoutes(app *lift.App) {
	// OAuth endpoints
	app.POST("/apps", wrapHandler(handler.HandleAppRegistration))
	app.GET("/oauth/authorize", wrapHandler(handler.HandleOAuthAuthorize))
	app.POST("/oauth/token", wrapHandler(handler.HandleOAuthToken))

	// Account registration
	app.POST("/accounts", wrapHandler(handler.HandleRegistration))

	// Instance information
	app.GET("/instance", wrapHandler(handler.HandleGetInstanceV1))
	app.GET("/instance/activity", wrapHandler(handler.HandleGetInstanceActivity))
	app.GET("/instance/peers", wrapHandler(handler.HandleGetInstancePeers))
	// app.GET("/instance/rules", wrapHandler(handler.HandleGetInstanceRules))

	// Public timelines
	app.GET("/timelines/public", wrapHandler(handler.HandlePublicTimeline))
	app.GET("/timelines/tag/{hashtag}", wrapHandlerWithParam(handler.HandleHashtagTimeline, "hashtag"))

	// Webfinger and nodeinfo
	app.GET("/.well-known/webfinger", wrapHandler(handler.HandleWebFinger))
	app.GET("/.well-known/nodeinfo", wrapHandler(handler.HandleNodeInfoWellKnown))
	app.GET("/nodeinfo/2.0", wrapHandler(handler.HandleNodeInfo))

	// Custom emojis
	app.GET("/custom_emojis", wrapHandler(handler.HandleGetCustomEmojis))

	// Streaming endpoints are handled by WebSocket Lambda
}

// configureAuthenticatedReadRoutes configures routes that require authentication for read operations
func configureAuthenticatedReadRoutes(app *lift.App) {
	// Account information
	app.GET("/accounts/verify_credentials", wrapAuthHandler(wrapHandler(handler.HandleVerifyCredentials)))
	app.GET("/accounts/relationships", wrapAuthHandler(wrapHandler(handler.HandleGetRelationships)))
	app.GET("/accounts/search", wrapAuthHandler(wrapHandler(handler.HandleAccountSearch)))
	app.GET("/accounts/lookup", wrapAuthHandler(wrapHandler(handler.HandleAccountLookup)))
	app.GET("/accounts/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetAccount, "id")))
	app.GET("/accounts/{id}/statuses", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetAccountStatuses, "id")))
	app.GET("/accounts/{id}/followers", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetAccountFollowers, "id")))
	app.GET("/accounts/{id}/following", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetAccountFollowing, "id")))

	// Timelines
	app.GET("/timelines/home", wrapAuthHandler(wrapHandler(handler.HandleHomeTimeline)))
	app.GET("/timelines/list/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleListTimeline, "id")))
	app.GET("/timelines/direct", wrapAuthHandler(wrapHandler(handler.HandleDirectTimeline)))

	// Statuses
	app.GET("/statuses/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetStatus, "id")))
	app.GET("/statuses/{id}/context", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetStatusContext, "id")))
	app.GET("/statuses/{id}/favourited_by", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetStatusFavouritedBy, "id")))
	app.GET("/statuses/{id}/reblogged_by", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetStatusRebloggedBy, "id")))

	// User collections
	app.GET("/bookmarks", wrapAuthHandler(wrapHandler(handler.HandleGetBookmarks)))
	app.GET("/favourites", wrapAuthHandler(wrapHandler(handler.HandleGetFavourites)))
	app.GET("/blocks", wrapAuthHandler(wrapHandler(handler.HandleGetBlocks)))
	app.GET("/mutes", wrapAuthHandler(wrapHandler(handler.HandleGetMutedAccounts)))
	app.GET("/domain_blocks", wrapAuthHandler(wrapHandler(handler.HandleGetDomainBlocks)))

	// Data exports
	app.GET("/exports", wrapAuthHandler(wrapHandler(handler.HandleListExports)))
	app.GET("/exports/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetExportStatus, "id")))

	// Lists
	app.GET("/lists", wrapAuthHandler(wrapHandler(handler.HandleGetLists)))
	app.GET("/lists/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetList, "id")))
	app.GET("/lists/{id}/accounts", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetListAccounts, "id")))

	// Notifications
	app.GET("/notifications", wrapAuthHandler(wrapHandler(handler.HandleGetNotifications)))
	app.GET("/notifications/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleGetNotification, "id")))

	// Search
	app.GET("/search", wrapAuthHandler(wrapHandler(handler.HandleSearch)))

	// Trends
	app.GET("/trends", wrapAuthHandler(wrapHandler(handler.HandleGetTrends)))
	app.GET("/trends/statuses", wrapAuthHandler(wrapHandler(handler.HandleGetTrendingStatuses)))
	app.GET("/trends/tags", wrapAuthHandler(wrapHandler(handler.HandleGetTrendingTags)))
	app.GET("/trends/links", wrapAuthHandler(wrapHandler(handler.HandleGetTrendingLinks)))
}

// configureAuthenticatedWriteRoutes configures routes that require authentication for write operations
func configureAuthenticatedWriteRoutes(app *lift.App) {
	// Account updates
	app.PATCH("/accounts/update_credentials", wrapAuthHandler(wrapHandler(handler.HandleUpdateCredentials)))

	// Status creation and interactions
	app.POST("/statuses", wrapAuthHandler(wrapHandler(handler.HandleCreateStatus)))
	app.DELETE("/statuses/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleDeleteStatus, "id")))
	app.POST("/statuses/{id}/reblog", wrapAuthHandler(wrapHandlerWithParam(handler.HandleReblog, "id")))
	app.POST("/statuses/{id}/unreblog", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnreblog, "id")))
	app.POST("/statuses/{id}/favourite", wrapAuthHandler(wrapHandlerWithParam(handler.HandleFavourite, "id")))
	app.POST("/statuses/{id}/unfavourite", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnfavourite, "id")))
	app.POST("/statuses/{id}/bookmark", wrapAuthHandler(wrapHandlerWithParam(handler.HandleBookmark, "id")))
	app.POST("/statuses/{id}/unbookmark", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnbookmark, "id")))
	app.POST("/statuses/{id}/pin", wrapAuthHandler(wrapHandlerWithParam(handler.HandlePinStatus, "id")))
	app.POST("/statuses/{id}/unpin", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnpinStatus, "id")))
	app.POST("/statuses/{id}/mute", wrapAuthHandler(wrapHandlerWithParam(handler.HandleMuteConversation, "id")))
	app.POST("/statuses/{id}/unmute", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnmuteConversation, "id")))
	app.PUT("/statuses/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUpdateStatus, "id")))

	// Account relationships
	app.POST("/accounts/{id}/follow", wrapAuthHandler(wrapHandlerWithParam(handler.HandleFollow, "id")))
	app.POST("/accounts/{id}/unfollow", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnfollow, "id")))
	app.POST("/accounts/{id}/block", wrapAuthHandler(wrapHandlerWithParam(handler.HandleBlock, "id")))
	app.POST("/accounts/{id}/unblock", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnblock, "id")))
	app.POST("/accounts/{id}/mute", wrapAuthHandler(wrapHandlerWithParam(handler.HandleMuteAccount, "id")))
	app.POST("/accounts/{id}/unmute", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnmuteAccount, "id")))
	app.POST("/accounts/{id}/pin", wrapAuthHandler(wrapHandlerWithParam(handler.HandlePinAccount, "id")))
	app.POST("/accounts/{id}/unpin", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnpinAccount, "id")))
	app.POST("/accounts/{id}/note", wrapAuthHandler(wrapHandlerWithParam(handler.HandleSetAccountNote, "id")))

	// Domain blocks
	app.POST("/domain_blocks", wrapAuthHandler(wrapHandler(handler.HandleCreateDomainBlock)))
	app.DELETE("/domain_blocks", wrapAuthHandler(wrapHandler(handler.HandleDeleteDomainBlock)))

	// Lists management
	app.POST("/lists", wrapAuthHandler(wrapHandler(handler.HandleCreateList)))
	app.PUT("/lists/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUpdateList, "id")))
	app.DELETE("/lists/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleDeleteList, "id")))
	app.POST("/lists/{id}/accounts", wrapAuthHandler(wrapHandlerWithParam(handler.HandleAddAccountsToList, "id")))
	app.DELETE("/lists/{id}/accounts", wrapAuthHandler(wrapHandlerWithParam(handler.HandleRemoveAccountsFromList, "id")))

	// Follow requests
	app.POST("/follow_requests/{id}/authorize", wrapAuthHandler(wrapHandlerWithParam(handler.HandleAuthorizeFollowRequest, "id")))
	app.POST("/follow_requests/{id}/reject", wrapAuthHandler(wrapHandlerWithParam(handler.HandleRejectFollowRequest, "id")))

	// Notifications
	app.POST("/notifications/clear", wrapAuthHandler(wrapHandler(handler.HandleClearNotifications)))
	app.POST("/notifications/{id}/dismiss", wrapAuthHandler(wrapHandlerWithParam(handler.HandleDismissNotification, "id")))

	// Push notifications
	app.POST("/push/subscription", wrapAuthHandler(wrapHandler(handler.HandleCreatePushSubscription)))
	app.GET("/push/subscription", wrapAuthHandler(wrapHandler(handler.HandleGetPushSubscription)))
	app.PUT("/push/subscription", wrapAuthHandler(wrapHandler(handler.HandleUpdatePushSubscription)))
	app.DELETE("/push/subscription", wrapAuthHandler(wrapHandler(handler.HandleDeletePushSubscription)))

	// Filters
	app.POST("/filters", wrapAuthHandler(wrapHandler(handler.HandleCreateFilter)))
	app.PUT("/filters/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUpdateFilter, "id")))
	app.DELETE("/filters/{id}", wrapAuthHandler(wrapHandlerWithParam(handler.HandleDeleteFilter, "id")))

	// Reports
	app.POST("/reports", wrapAuthHandler(wrapHandler(handler.HandleCreateReport)))

	// Endorsements
	app.POST("/accounts/{id}/endorse", wrapAuthHandler(wrapHandlerWithParam(handler.HandlePinAccount, "id")))
	app.POST("/accounts/{id}/unendorse", wrapAuthHandler(wrapHandlerWithParam(handler.HandleUnpinAccount, "id")))

	// Markers
	app.POST("/markers", wrapAuthHandler(wrapHandler(handler.HandleSaveMarkers)))

	// Media
	app.POST("/media", wrapAuthHandler(wrapHandler(handler.HandleMediaUpload)))
	app.PUT("/media/{id}", wrapAuthHandler(wrapHandler(handler.HandleUpdateMedia)))

	// Data exports
	app.POST("/exports", wrapAuthHandler(wrapHandler(handler.HandleCreateExport)))
}

// configureAdminRoutes configures routes that require admin role
func configureAdminRoutes(app *lift.App) {
	// Admin routes
	app.GET("/admin", wrapAdminHandler(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "admin area"})
	})))
}