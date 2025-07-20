package main

import (
	"github.com/pay-theory/lift/pkg/lift"
)

// configurePublicRoutes configures routes that don't require authentication
func configurePublicRoutes(app *lift.Application) {
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

	// Streaming endpoints (SSE/WebSocket)
	app.GET("/streaming/{stream}", wrapHandlerWithParam(handler.HandleSSEStream, "stream"))
	app.GET("/streaming", wrapHandler(handler.HandleSSEStream))
}

// configureAuthenticatedReadRoutes configures routes that require authentication for read operations
func configureAuthenticatedReadRoutes(app *lift.Application) {
	// Create a group with auth middleware
	authGroup := app.Group()
	authGroup.Use(createAuthMiddleware())

	// Account information
	authGroup.GET("/accounts/verify_credentials", wrapHandler(handler.HandleVerifyCredentials))
	authGroup.GET("/accounts/relationships", wrapHandler(handler.HandleGetRelationships))
	authGroup.GET("/accounts/search", wrapHandler(handler.HandleAccountSearch))
	authGroup.GET("/accounts/lookup", wrapHandler(handler.HandleAccountLookup))
	authGroup.GET("/accounts/{id}", wrapHandlerWithParam(handler.HandleGetAccount, "id"))
	authGroup.GET("/accounts/{id}/statuses", wrapHandlerWithParam(handler.HandleGetAccountStatuses, "id"))
	authGroup.GET("/accounts/{id}/followers", wrapHandlerWithParam(handler.HandleGetAccountFollowers, "id"))
	authGroup.GET("/accounts/{id}/following", wrapHandlerWithParam(handler.HandleGetAccountFollowing, "id"))

	// Timelines
	authGroup.GET("/timelines/home", wrapHandler(handler.HandleHomeTimeline))
	authGroup.GET("/timelines/list/{id}", wrapHandlerWithParam(handler.HandleListTimeline, "id"))
	authGroup.GET("/timelines/direct", wrapHandler(handler.HandleDirectTimeline))

	// Statuses
	authGroup.GET("/statuses/{id}", wrapHandlerWithParam(handler.HandleGetStatus, "id"))
	authGroup.GET("/statuses/{id}/context", wrapHandlerWithParam(handler.HandleGetStatusContext, "id"))
	authGroup.GET("/statuses/{id}/favourited_by", wrapHandlerWithParam(handler.HandleGetStatusFavouritedBy, "id"))
	authGroup.GET("/statuses/{id}/reblogged_by", wrapHandlerWithParam(handler.HandleGetStatusRebloggedBy, "id"))

	// User collections
	authGroup.GET("/bookmarks", wrapHandler(handler.HandleGetBookmarks))
	authGroup.GET("/favourites", wrapHandler(handler.HandleGetFavourites))
	authGroup.GET("/blocks", wrapHandler(handler.HandleGetBlocks))
	authGroup.GET("/mutes", wrapHandler(handler.HandleGetMutedAccounts))
	authGroup.GET("/domain_blocks", wrapHandler(handler.HandleGetDomainBlocks))

	// Lists
	authGroup.GET("/lists", wrapHandler(handler.HandleGetLists))
	authGroup.GET("/lists/{id}", wrapHandlerWithParam(handler.HandleGetList, "id"))
	authGroup.GET("/lists/{id}/accounts", wrapHandlerWithParam(handler.HandleGetListAccounts, "id"))

	// Notifications
	authGroup.GET("/notifications", wrapHandler(handler.HandleGetNotifications))
	authGroup.GET("/notifications/{id}", wrapHandlerWithParam(handler.HandleGetNotification, "id"))

	// Search
	authGroup.GET("/search", wrapHandler(handler.HandleSearch))

	// Trends
	authGroup.GET("/trends", wrapHandler(handler.HandleGetTrends))
	authGroup.GET("/trends/statuses", wrapHandler(handler.HandleGetTrendingStatuses))
	authGroup.GET("/trends/tags", wrapHandler(handler.HandleGetTrendingTags))
	authGroup.GET("/trends/links", wrapHandler(handler.HandleGetTrendingLinks))
}

// configureAuthenticatedWriteRoutes configures routes that require authentication for write operations
func configureAuthenticatedWriteRoutes(app *lift.Application) {
	// Create a group with auth middleware
	authGroup := app.Group()
	authGroup.Use(createAuthMiddleware())

	// Account updates
	authGroup.PATCH("/accounts/update_credentials", wrapHandler(handler.HandleUpdateCredentials))

	// Status management
	authGroup.POST("/statuses", wrapHandler(handler.HandleCreateStatus))
	authGroup.DELETE("/statuses/{id}", wrapHandlerWithParam(handler.HandleDeleteStatus, "id"))
	authGroup.PUT("/statuses/{id}", wrapHandlerWithParam(handler.HandleUpdateStatus, "id"))

	// Status interactions
	authGroup.POST("/statuses/{id}/favourite", wrapHandlerWithParam(handler.HandleFavourite, "id"))
	authGroup.POST("/statuses/{id}/unfavourite", wrapHandlerWithParam(handler.HandleUnfavourite, "id"))
	authGroup.POST("/statuses/{id}/reblog", wrapHandlerWithParam(handler.HandleUnifiedReblog, "id"))
	authGroup.POST("/statuses/{id}/unreblog", wrapHandlerWithParam(handler.HandleUnreblog, "id"))
	authGroup.POST("/statuses/{id}/bookmark", wrapHandlerWithParam(handler.HandleBookmark, "id"))
	authGroup.POST("/statuses/{id}/unbookmark", wrapHandlerWithParam(handler.HandleUnbookmark, "id"))
	authGroup.POST("/statuses/{id}/mute", wrapHandlerWithParam(handler.HandleMuteConversation, "id"))
	authGroup.POST("/statuses/{id}/unmute", wrapHandlerWithParam(handler.HandleUnmuteConversation, "id"))
	authGroup.POST("/statuses/{id}/pin", wrapHandlerWithParam(handler.HandlePinStatus, "id"))
	authGroup.POST("/statuses/{id}/unpin", wrapHandlerWithParam(handler.HandleUnpinStatus, "id"))

	// Account interactions
	authGroup.POST("/accounts/{id}/follow", wrapHandlerWithParam(handler.HandleFollow, "id"))
	authGroup.POST("/accounts/{id}/unfollow", wrapHandlerWithParam(handler.HandleUnfollow, "id"))
	authGroup.POST("/accounts/{id}/block", wrapHandlerWithParam(handler.HandleBlock, "id"))
	authGroup.POST("/accounts/{id}/unblock", wrapHandlerWithParam(handler.HandleUnblock, "id"))
	authGroup.POST("/accounts/{id}/mute", wrapHandlerWithParam(handler.HandleMuteAccount, "id"))
	authGroup.POST("/accounts/{id}/unmute", wrapHandlerWithParam(handler.HandleUnmuteAccount, "id"))
	authGroup.POST("/accounts/{id}/pin", wrapHandlerWithParam(handler.HandlePinAccount, "id"))
	authGroup.POST("/accounts/{id}/unpin", wrapHandlerWithParam(handler.HandleUnpinAccount, "id"))
	authGroup.POST("/accounts/{id}/note", wrapHandlerWithParam(handler.HandleSetAccountNote, "id"))
	authGroup.POST("/accounts/{id}/remove_from_followers", wrapHandlerWithParam(handler.HandleRemoveFromFollowers, "id"))

	// List management
	authGroup.POST("/lists", wrapHandler(handler.HandleCreateList))
	authGroup.PUT("/lists/{id}", wrapHandlerWithParam(handler.HandleUpdateList, "id"))
	authGroup.DELETE("/lists/{id}", wrapHandlerWithParam(handler.HandleDeleteList, "id"))
	authGroup.POST("/lists/{id}/accounts", wrapHandlerWithParam(handler.HandleAddAccountsToList, "id"))
	authGroup.DELETE("/lists/{id}/accounts", wrapHandlerWithParam(handler.HandleRemoveAccountsFromList, "id"))

	// Media uploads
	authGroup.POST("/media", wrapHandler(handler.HandleMediaUpload))
	authGroup.PUT("/media/{id}", wrapHandler(handler.HandleUpdateMedia))

	// Push subscriptions
	authGroup.POST("/push/subscription", wrapHandler(handler.HandleCreatePushSubscription))
	authGroup.GET("/push/subscription", wrapHandler(handler.HandleGetPushSubscription))
	authGroup.PUT("/push/subscription", wrapHandler(handler.HandleUpdatePushSubscription))
	authGroup.DELETE("/push/subscription", wrapHandler(handler.HandleDeletePushSubscription))

	// Domain blocks
	authGroup.POST("/domain_blocks", wrapHandler(handler.HandleCreateDomainBlock))
	authGroup.DELETE("/domain_blocks", wrapHandler(handler.HandleDeleteDomainBlock))

	// Notification management
	authGroup.POST("/notifications/clear", wrapHandler(handler.HandleClearNotifications))
	authGroup.POST("/notifications/{id}/dismiss", wrapHandlerWithParam(handler.HandleDismissNotification, "id"))

	// Featured tags
	authGroup.GET("/featured_tags", wrapHandler(handler.HandleGetFeaturedTags))
	authGroup.POST("/featured_tags", wrapHandler(handler.HandleCreateFeaturedTag))
	authGroup.DELETE("/featured_tags/{id}", wrapHandlerWithParam(handler.HandleDeleteFeaturedTag, "id"))
}

// configureAdminRoutes configures routes that require admin role
func configureAdminRoutes(app *lift.Application) {
	// Create a group with auth and admin middleware
	adminGroup := app.Group()
	adminGroup.Use(createAuthMiddleware())
	adminGroup.Use(createAdminMiddleware())

	// Admin routes
	adminGroup.GET("/admin", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "admin area"})
	})
}
