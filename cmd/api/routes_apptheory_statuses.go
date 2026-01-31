package main

import (
	"time"

	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/pay-theory/lift/pkg/lift"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureStatusesRoutesAppTheory(app *apptheory.App) {
	baseMiddleware := standardLiftMiddlewaresForAppTheory()

	// Community Notes.
	app.Post("/api/v1/notes", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreateNoteLift),
			20, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/notes/{object_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetNotesLift),
		baseMiddleware,
	))
	app.Post("/api/v1/notes/{id}/vote", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleVoteNoteLift),
			100, time.Hour, logger,
		),
		baseMiddleware,
	))

	// Status endpoints (Mastodon parity).
	app.Post("/api/v1/statuses", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreateStatusLift),
			300, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Put("/api/v1/statuses/{id}", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUpdateStatusLift),
			300, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Delete("/api/v1/statuses/{id}", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleDeleteStatusLift),
			300, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/statuses/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetStatusLift),
		baseMiddleware,
	))
	app.Get("/api/v1/statuses/{id}/context", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetStatusContextLift),
		baseMiddleware,
	))
	app.Get("/api/v1/statuses/{id}/history", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetStatusHistoryLift),
		baseMiddleware,
	))
	app.Get("/api/v1/statuses/{id}/source", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetStatusSourceLift),
		baseMiddleware,
	))
	app.Get("/api/v1/statuses/{id}/favourited_by", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetStatusFavouritedByLift),
		baseMiddleware,
	))
	app.Get("/api/v1/statuses/{id}/reblogged_by", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetStatusRebloggedByLift),
		baseMiddleware,
	))

	// Timelines.
	app.Get("/api/v1/timelines/home", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetHomeTimelineLift),
		baseMiddleware,
	))
	app.Get("/api/v1/timelines/public", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetPublicTimelineLift),
		baseMiddleware,
	))
	app.Get("/api/v1/timelines/tag/{hashtag}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTagTimelineLift),
		baseMiddleware,
	))
	app.Get("/api/v1/timelines/list/{list_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetListTimelineLift),
		baseMiddleware,
	))
	app.Get("/api/v1/timelines/direct", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetDirectTimelineLift),
		baseMiddleware,
	))
	app.Get("/api/v1/timelines/link", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetLinkTimelineLift),
		baseMiddleware,
	))

	// Trends (Mastodon v1).
	app.Get("/api/v1/trends", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrendsLift),
		baseMiddleware,
	))
	app.Get("/api/v1/trends/tags", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrendingTagsLift),
		baseMiddleware,
	))
	app.Get("/api/v1/trends/statuses", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrendingStatusesLift),
		baseMiddleware,
	))
	app.Get("/api/v1/trends/links", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrendingLinksLift),
		baseMiddleware,
	))

	// Status interactions.
	app.Post("/api/v1/statuses/{id}/favourite", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleFavoriteLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/unfavourite", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUnfavoriteLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/reblog", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleReblogLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/unreblog", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUnreblogLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/bookmark", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleBookmarkLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/unbookmark", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUnbookmarkLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/pin", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandlePinStatusLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/unpin", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUnpinStatusLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/mute", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleMuteConversationLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/unmute", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUnmuteConversationLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/statuses/{id}/translate", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleTranslateStatusLift),
			20, time.Hour, logger,
		),
		baseMiddleware,
	))

	// Bookmarks + favourites.
	app.Get("/api/v1/bookmarks", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetBookmarksLift),
		baseMiddleware,
	))
	app.Get("/api/v1/favourites", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFavouritesLift),
		baseMiddleware,
	))

	// Lists.
	app.Get("/api/v1/lists", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetListsLift),
		baseMiddleware,
	))
	app.Post("/api/v1/lists", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCreateListLift),
		baseMiddleware,
	))
	app.Get("/api/v1/lists/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetListLift),
		baseMiddleware,
	))
	app.Put("/api/v1/lists/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateListLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/lists/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteListLift),
		baseMiddleware,
	))
	app.Get("/api/v1/lists/{id}/accounts", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetListAccountsLift),
		baseMiddleware,
	))
	app.Post("/api/v1/lists/{id}/accounts", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAddAccountsToListLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/lists/{id}/accounts", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleRemoveAccountsFromListLift),
		baseMiddleware,
	))

	// Notifications.
	app.Get("/api/v1/notifications", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetNotificationsLift),
		baseMiddleware,
	))
	app.Get("/api/v1/notifications/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetNotificationLift),
		baseMiddleware,
	))
	app.Post("/api/v1/notifications/clear", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleClearNotificationsLift),
		baseMiddleware,
	))
	app.Post("/api/v1/notifications/{id}/dismiss", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDismissNotificationLift),
		baseMiddleware,
	))

	// Preferences + markers.
	app.Get("/api/v1/preferences", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetPreferencesLift),
		baseMiddleware,
	))
	app.Patch("/api/v1/preferences", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdatePreferencesLift),
		baseMiddleware,
	))
	app.Get("/api/v1/markers", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetMarkersLift),
		baseMiddleware,
	))
	app.Post("/api/v1/markers", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleSaveMarkersLift),
		baseMiddleware,
	))

	// Push subscriptions.
	app.Get("/api/v1/push/subscription", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetPushSubscriptionLift),
		baseMiddleware,
	))
	app.Post("/api/v1/push/subscription", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreatePushSubscriptionLift),
			20, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Put("/api/v1/push/subscription", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUpdatePushSubscriptionLift),
			20, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Delete("/api/v1/push/subscription", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeletePushSubscriptionLift),
		baseMiddleware,
	))

	// Scheduled statuses.
	app.Get("/api/v1/scheduled_statuses", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetScheduledStatusesLift),
		baseMiddleware,
	))
	app.Get("/api/v1/scheduled_statuses/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetScheduledStatusLift),
		baseMiddleware,
	))
	app.Put("/api/v1/scheduled_statuses/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateScheduledStatusLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/scheduled_statuses/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteScheduledStatusLift),
		baseMiddleware,
	))

	// Discovery.
	app.Get("/api/v1/directory", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetDirectoryLift),
		baseMiddleware,
	))
	app.Get("/api/v1/suggestions", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetSuggestionsV1Lift),
		baseMiddleware,
	))
	app.Delete("/api/v1/suggestions/{account_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleRemoveSuggestionLift),
		baseMiddleware,
	))
	app.Get("/api/v1/endorsements", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetEndorsementsLift),
		baseMiddleware,
	))

	// Announcements.
	app.Get("/api/v1/announcements", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetAnnouncementsLift),
		baseMiddleware,
	))
	app.Post("/api/v1/announcements/{id}/dismiss", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDismissAnnouncementLift),
		baseMiddleware,
	))
	app.Put("/api/v1/announcements/{id}/reactions/{name}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAddAnnouncementReactionLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/announcements/{id}/reactions/{name}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleRemoveAnnouncementReactionLift),
		baseMiddleware,
	))

	// Custom emojis.
	app.Get("/api/v1/custom_emojis", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetCustomEmojisLift),
		baseMiddleware,
	))

	// Status search.
	app.Get("/api/v1/search/statuses", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleStatusSearchLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/search/statuses", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleStatusSearchLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))

	// Conversations.
	app.Get("/api/v1/conversations", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetConversationsLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/conversations/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteConversationLift),
		baseMiddleware,
	))
	app.Post("/api/v1/conversations/{id}/read", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleMarkConversationReadLift),
		baseMiddleware,
	))

	// Instance endpoints.
	app.Get("/api/v1/instance", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetInstanceV1Lift),
		baseMiddleware,
	))
	app.Get("/api/v1/instance/peers", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetInstancePeersLift),
		baseMiddleware,
	))
	app.Get("/api/v1/instance/activity", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetInstanceActivityLift),
		baseMiddleware,
	))
	app.Get("/api/v1/instance/domain_blocks", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetInstanceDomainBlocksLift),
		baseMiddleware,
	))
	app.Get("/api/v1/instance/translation_languages", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTranslationLanguagesLift),
		baseMiddleware,
	))

	// API v2 endpoints.
	app.Get("/api/v2/instance", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetInstanceV2Lift),
		baseMiddleware,
	))
	app.Get("/api/v2/search", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleSearchV2Lift),
		baseMiddleware,
	))
	app.Get("/api/v2/suggestions", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetSuggestionsV2Lift),
		baseMiddleware,
	))

	// API v2 filters.
	app.Get("/api/v2/filters", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFiltersLift),
		baseMiddleware,
	))
	app.Get("/api/v2/filters/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFilterLift),
		baseMiddleware,
	))
	app.Post("/api/v2/filters", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCreateFilterLift),
		baseMiddleware,
	))
	app.Put("/api/v2/filters/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateFilterLift),
		baseMiddleware,
	))
	app.Delete("/api/v2/filters/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteFilterLift),
		baseMiddleware,
	))

	// API v2 filter keywords and statuses.
	app.Get("/api/v2/filters/{filter_id}/keywords", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFilterKeywordsLift),
		baseMiddleware,
	))
	app.Post("/api/v2/filters/{filter_id}/keywords", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAddFilterKeywordLift),
		baseMiddleware,
	))
	app.Delete("/api/v2/filters/{filter_id}/keywords/{keyword_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteFilterKeywordLift),
		baseMiddleware,
	))
	app.Get("/api/v2/filters/{filter_id}/statuses", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFilterStatusesLift),
		baseMiddleware,
	))
	app.Post("/api/v2/filters/{filter_id}/statuses", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAddFilterStatusLift),
		baseMiddleware,
	))
	app.Delete("/api/v2/filters/{filter_id}/statuses/{status_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteFilterStatusLift),
		baseMiddleware,
	))

	// API v2 trends endpoints.
	app.Get("/api/v2/trends", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrendsV2Lift),
		baseMiddleware,
	))
	app.Get("/api/v2/trends/tags", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrendingTagsV2Lift),
		baseMiddleware,
	))
	app.Get("/api/v2/trends/statuses", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrendingStatusesV2Lift),
		baseMiddleware,
	))
	app.Get("/api/v2/trends/links", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrendingLinksV2Lift),
		baseMiddleware,
	))

	// API v2 filter testing endpoint.
	app.Post("/api/v2/filters/test", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleTestFilterLift),
		baseMiddleware,
	))

	// API v2 grouped notifications endpoints.
	app.Get("/api/v2/notifications/grouped", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetGroupedNotificationsLift),
		baseMiddleware,
	))
	app.Post("/api/v2/notifications/groups/{group_id}/read", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleMarkGroupAsReadLift),
		baseMiddleware,
	))

	// Quote posts API endpoints.
	app.Post("/api/v1/statuses/{id}/quote", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreateQuotePostLift),
			30, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/statuses/{id}/quotes", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetQuotesOfStatusLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/statuses/{id}/quote/{quote_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteQuotePostLift),
		baseMiddleware,
	))
}

