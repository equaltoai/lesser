package main

import (
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/crawler"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureRoutes(app *apptheory.App) {
	app.Get("/", func(*apptheory.Context) (*apptheory.Response, error) {
		return redirectResponse("/l/", false), nil
	})
	app.Handle("HEAD", "/", func(*apptheory.Context) (*apptheory.Response, error) {
		return redirectResponse("/l/", false), nil
	})
	app.Get("/robots.txt", crawler.RobotsHandler)

	// OAuth app registration (public, no auth required)
	app.Post("/api/v1/apps", apiHandler.HandleAppRegistrationLift)

	// Wallet authentication endpoints (public, for passwordless login)
	app.Post("/auth/wallet/challenge", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateChallengeLift,
		20, 5*time.Minute, logger))
	app.Post("/auth/wallet/verify", ratelimit.ApplyRateLimit(
		apiHandler.HandleVerifySignatureLift,
		20, 5*time.Minute, logger))
	app.Post("/auth/wallet/login", ratelimit.ApplyRateLimit(
		apiHandler.HandleLoginWalletLift,
		20, 5*time.Minute, logger))
	app.Post("/auth/wallet/link", apiHandler.HandleLinkWalletLift)
	app.Delete("/auth/wallet/unlink/{address}", apiHandler.HandleUnlinkWalletLift)
	app.Get("/auth/wallet/list", apiHandler.HandleGetWalletsLift)

	// OPTIONS handlers for CORS preflight (CORS headers set by middleware)
	optionsHandler := func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.JSON(200, map[string]string{"message": "OK"})
	}
	app.Handle("OPTIONS", "/auth/wallet/challenge", optionsHandler)
	app.Handle("OPTIONS", "/auth/wallet/verify", optionsHandler)
	app.Handle("OPTIONS", "/auth/wallet/login", optionsHandler)
	app.Handle("OPTIONS", "/auth/wallet/link", optionsHandler)
	app.Handle("OPTIONS", "/auth/wallet/unlink/{address}", optionsHandler)
	app.Handle("OPTIONS", "/auth/wallet/list", optionsHandler)

	// WebAuthn (passkey) endpoints
	// Registration begin/finish requires auth (binds a passkey to the logged-in user).
	app.Post("/api/v1/auth/webauthn/register/begin", ratelimit.ApplyRateLimit(
		apiHandler.HandleBeginWebAuthnRegistrationLift,
		20, 5*time.Minute, logger))
	app.Post("/api/v1/auth/webauthn/register/finish", ratelimit.ApplyRateLimit(
		apiHandler.HandleFinishWebAuthnRegistrationLift,
		20, 5*time.Minute, logger))
	// Login begin/finish is public (username provided), but rate limited.
	app.Post("/api/v1/auth/webauthn/login/begin", ratelimit.ApplyRateLimit(
		apiHandler.HandleBeginWebAuthnLoginLift,
		20, 5*time.Minute, logger))
	app.Post("/api/v1/auth/webauthn/login/finish", ratelimit.ApplyRateLimit(
		apiHandler.HandleFinishWebAuthnLoginLift,
		20, 5*time.Minute, logger))

	app.Get("/api/v1/auth/webauthn/credentials", apiHandler.HandleListWebAuthnCredentialsLift)
	app.Delete("/api/v1/auth/webauthn/credentials/{credentialId}", apiHandler.HandleDeleteWebAuthnCredentialLift)
	app.Put("/api/v1/auth/webauthn/credentials/{credentialId}", apiHandler.HandleUpdateWebAuthnCredentialNameLift)

	app.Handle("OPTIONS", "/api/v1/auth/webauthn/register/begin", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/auth/webauthn/register/finish", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/auth/webauthn/login/begin", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/auth/webauthn/login/finish", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/auth/webauthn/credentials", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/auth/webauthn/credentials/{credentialId}", optionsHandler)

	// OAuth endpoints with native Lift implementation + rate limiting
	app.Get("/oauth/authorize", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthAuthorizeLift,
		20, 5*time.Minute, logger))
	app.Post("/oauth/consent", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthConsentLift,
		20, 5*time.Minute, logger))
	app.Post("/oauth/device/code", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthDeviceCodeLift,
		10, time.Minute, logger))
	app.Post("/oauth/device/verify", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthDeviceVerifyLift,
		20, 5*time.Minute, logger))
	app.Post("/oauth/device/consent", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthDeviceConsentLift,
		20, 5*time.Minute, logger))
	app.Post("/oauth/token", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthTokenLift,
		10, time.Minute, logger))
	app.Post("/oauth/revoke", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthRevokeLift,
		10, time.Minute, logger))

	// OPTIONS handlers for OAuth endpoints (CORS preflight)
	app.Handle("OPTIONS", "/oauth/authorize", optionsHandler)
	app.Handle("OPTIONS", "/oauth/consent", optionsHandler)
	app.Handle("OPTIONS", "/oauth/device/code", optionsHandler)
	app.Handle("OPTIONS", "/oauth/device/verify", optionsHandler)
	app.Handle("OPTIONS", "/oauth/device/consent", optionsHandler)
	app.Handle("OPTIONS", "/oauth/token", optionsHandler)
	app.Handle("OPTIONS", "/oauth/revoke", optionsHandler)

	// NodeInfo endpoints with native Lift implementation
	app.Get("/.well-known/nodeinfo", apiHandler.HandleNodeInfoWellKnownLift)
	app.Get("/nodeinfo/2.0", apiHandler.HandleNodeInfoLift)

	// Reputation keys (used by the portable reputation system)
	app.Get("/.well-known/reputation-keys", apiHandler.HandleGetReputationKeysLift)

	// oEmbed + embed endpoints
	app.Get("/api/oembed", apiHandler.HandleOEmbedLift)
	app.Get("/embed/{id}", apiHandler.HandleEmbedPageLift)

	// Instance setup endpoints (locked-by-default bootstrapping)
	app.Get("/setup/status", apiHandler.HandleSetupStatusLift)
	app.Post("/setup/bootstrap/challenge", ratelimit.ApplyRateLimit(
		apiHandler.HandleSetupBootstrapChallengeLift,
		20, 5*time.Minute, logger))
	app.Post("/setup/bootstrap/verify", ratelimit.ApplyRateLimit(
		apiHandler.HandleSetupBootstrapVerifyLift,
		20, 5*time.Minute, logger))
	app.Post("/setup/admin", apiHandler.HandleSetupCreateAdminLift)
	app.Post("/setup/finalize", apiHandler.HandleSetupFinalizeLift)

	app.Handle("OPTIONS", "/setup/status", optionsHandler)
	app.Handle("OPTIONS", "/setup/bootstrap/challenge", optionsHandler)
	app.Handle("OPTIONS", "/setup/bootstrap/verify", optionsHandler)
	app.Handle("OPTIONS", "/setup/admin", optionsHandler)
	app.Handle("OPTIONS", "/setup/finalize", optionsHandler)

	// Account verification/update endpoints with native Lift implementation
	// verify_credentials is NOT rate limited (read-only)
	app.Get("/api/v1/accounts/verify_credentials", apiHandler.HandleVerifyCredentialsLift)
	// update_credentials IS rate limited
	app.Patch("/api/v1/accounts/update_credentials", ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdateCredentialsLift,
		10, time.Hour, logger))
	// account registration
	app.Post("/api/v1/accounts", ratelimit.ApplyRateLimit(
		apiHandler.HandleRegistrationLift,
		10, time.Hour, logger))

	// OPTIONS handlers for account endpoints (CORS preflight)
	app.Handle("OPTIONS", "/api/v1/accounts", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/accounts/verify_credentials", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/accounts/update_credentials", optionsHandler)

	// Agent endpoints (LLM agent support)
	app.Get("/api/v1/agents", apiHandler.HandleListAgentsLift)
	app.Post("/api/v1/agents/delegate", apiHandler.HandleDelegateAgentLift)
	app.Post("/api/v1/agents/register/challenge", apiHandler.HandleAgentRegisterChallengeLift)
	app.Post("/api/v1/agents/register", apiHandler.HandleAgentRegisterLift)
	app.Post("/api/v1/agents/auth/challenge", apiHandler.HandleAgentAuthChallengeLift)
	app.Post("/api/v1/agents/auth/token", apiHandler.HandleAgentAuthTokenLift)
	app.Get("/api/v1/agents/{username}", apiHandler.HandleGetAgentLift)
	app.Patch("/api/v1/agents/{username}", apiHandler.HandleUpdateAgentLift)
	app.Delete("/api/v1/agents/{username}", apiHandler.HandleDeleteAgentLift)
	app.Get("/api/v1/agents/{username}/activity", apiHandler.HandleGetAgentActivityLift)
	app.Post("/api/v1/agents/{username}/rotate-key/challenge", apiHandler.HandleAgentRotateKeyChallengeLift)
	app.Post("/api/v1/agents/{username}/rotate-key", apiHandler.HandleAgentRotateKeyLift)
	app.Get("/api/v1/agents/memory/search", apiHandler.HandleAgentMemorySearchLift)
	app.Post("/api/v1/agents/memory/search", apiHandler.HandleAgentMemorySearchLift)
	app.Post("/api/v1/agents/{username}/suspend", apiHandler.HandleSuspendAgentLift)

	app.Handle("OPTIONS", "/api/v1/agents", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/delegate", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/register/challenge", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/register", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/auth/challenge", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/auth/token", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/{username}", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/{username}/activity", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/{username}/rotate-key/challenge", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/{username}/rotate-key", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/memory/search", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/agents/{username}/suspend", optionsHandler)

	// Relationships endpoint with native Lift implementation
	app.Get("/api/v1/accounts/relationships", apiHandler.HandleGetRelationshipsLift)

	// Data exports with native Lift implementation
	// POST is rate limited (expensive operation)
	app.Post("/api/v1/exports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateExportLift,
		5, 24*time.Hour, logger))
	// GETs are NOT rate limited (read-only)
	app.Get("/api/v1/exports/{id}", apiHandler.HandleGetExportStatusLift)
	app.Get("/api/v1/exports/{id}/download", apiHandler.HandleDownloadExportLift)
	app.Get("/api/v1/exports", apiHandler.HandleListExportsLift)

	// Data imports with native Lift implementation
	// POST is rate limited (expensive operation)
	app.Post("/api/v1/imports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateImportLift,
		5, 24*time.Hour, logger))
	// GETs and DELETE are NOT rate limited
	app.Get("/api/v1/imports/{id}", apiHandler.HandleGetImportStatusLift)
	app.Delete("/api/v1/imports/{id}", apiHandler.HandleCancelImportLift)
	app.Get("/api/v1/imports", apiHandler.HandleListImportsLift)

	// Community Notes endpoints with native Lift implementation
	// POST create note is rate limited
	app.Post("/api/v1/notes", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateNoteLift,
		20, time.Hour, logger))
	// GETs are NOT rate limited
	app.Get("/api/v1/notes/{object_id}", apiHandler.HandleGetNotesLift)
	// POST vote is rate limited
	app.Post("/api/v1/notes/{id}/vote", ratelimit.ApplyRateLimit(
		apiHandler.HandleVoteNoteLift,
		100, time.Hour, logger))
	app.Get("/api/v1/accounts/{id}/notes", apiHandler.HandleGetUserNotesLift)

	// Status endpoints (Mastodon parity)
	app.Post("/api/v1/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateStatusLift,
		300, time.Hour, logger))
	app.Put("/api/v1/statuses/{id}", ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdateStatusLift,
		300, time.Hour, logger))
	app.Delete("/api/v1/statuses/{id}", ratelimit.ApplyRateLimit(
		apiHandler.HandleDeleteStatusLift,
		300, time.Hour, logger))
	app.Get("/api/v1/statuses/{id}", apiHandler.HandleGetStatusLift)
	app.Get("/api/v1/statuses/{id}/context", apiHandler.HandleGetStatusContextLift)
	app.Get("/api/v1/statuses/{id}/history", apiHandler.HandleGetStatusHistoryLift)
	app.Get("/api/v1/statuses/{id}/source", apiHandler.HandleGetStatusSourceLift)
	app.Get("/api/v1/statuses/{id}/favourited_by", apiHandler.HandleGetStatusFavouritedByLift)
	app.Get("/api/v1/statuses/{id}/reblogged_by", apiHandler.HandleGetStatusRebloggedByLift)
	app.Get("/api/v1/accounts/{id}/statuses", apiHandler.HandleGetAccountStatusesLift)

	// Additional Mastodon parity endpoints

	// Timelines
	app.Get("/api/v1/timelines/home", apiHandler.HandleGetHomeTimelineLift)
	app.Get("/api/v1/timelines/public", apiHandler.HandleGetPublicTimelineLift)
	app.Get("/api/v1/timelines/tag/{hashtag}", apiHandler.HandleGetTagTimelineLift)
	app.Get("/api/v1/timelines/list/{list_id}", apiHandler.HandleGetListTimelineLift)
	app.Get("/api/v1/timelines/direct", apiHandler.HandleGetDirectTimelineLift)
	app.Get("/api/v1/timelines/link", apiHandler.HandleGetLinkTimelineLift)

	// Trends (Mastodon v1)
	app.Get("/api/v1/trends", apiHandler.HandleGetTrendsLift)
	app.Get("/api/v1/trends/tags", apiHandler.HandleGetTrendingTagsLift)
	app.Get("/api/v1/trends/statuses", apiHandler.HandleGetTrendingStatusesLift)
	app.Get("/api/v1/trends/links", apiHandler.HandleGetTrendingLinksLift)

	// Status interactions
	app.Post("/api/v1/statuses/{id}/favourite", ratelimit.ApplyRateLimit(
		apiHandler.HandleFavoriteLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/unfavourite", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnfavoriteLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/reblog", ratelimit.ApplyRateLimit(
		apiHandler.HandleReblogLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/unreblog", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnreblogLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/bookmark", ratelimit.ApplyRateLimit(
		apiHandler.HandleBookmarkLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/unbookmark", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnbookmarkLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/pin", ratelimit.ApplyRateLimit(
		apiHandler.HandlePinStatusLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/unpin", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnpinStatusLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/mute", ratelimit.ApplyRateLimit(
		apiHandler.HandleMuteConversationLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/unmute", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnmuteConversationLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/statuses/{id}/translate", ratelimit.ApplyRateLimit(
		apiHandler.HandleTranslateStatusLift,
		20, time.Hour, logger))

	// Bookmarks + favourites
	app.Get("/api/v1/bookmarks", apiHandler.HandleGetBookmarksLift)
	app.Get("/api/v1/favourites", apiHandler.HandleGetFavouritesLift)

	// Lists
	app.Get("/api/v1/lists", apiHandler.HandleGetListsLift)
	app.Post("/api/v1/lists", apiHandler.HandleCreateListLift)
	app.Get("/api/v1/lists/{id}", apiHandler.HandleGetListLift)
	app.Put("/api/v1/lists/{id}", apiHandler.HandleUpdateListLift)
	app.Delete("/api/v1/lists/{id}", apiHandler.HandleDeleteListLift)
	app.Get("/api/v1/lists/{id}/accounts", apiHandler.HandleGetListAccountsLift)
	app.Post("/api/v1/lists/{id}/accounts", apiHandler.HandleAddAccountsToListLift)
	app.Delete("/api/v1/lists/{id}/accounts", apiHandler.HandleRemoveAccountsFromListLift)

	// Notifications
	app.Get("/api/v1/notifications", apiHandler.HandleGetNotificationsLift)
	app.Get("/api/v1/notifications/{id}", apiHandler.HandleGetNotificationLift)
	app.Post("/api/v1/notifications/clear", apiHandler.HandleClearNotificationsLift)
	app.Post("/api/v1/notifications/{id}/dismiss", apiHandler.HandleDismissNotificationLift)

	// Preferences + markers
	app.Get("/api/v1/preferences", apiHandler.HandleGetPreferencesLift)
	app.Patch("/api/v1/preferences", apiHandler.HandleUpdatePreferencesLift)
	app.Get("/api/v1/markers", apiHandler.HandleGetMarkersLift)
	app.Post("/api/v1/markers", apiHandler.HandleSaveMarkersLift)

	// Push subscriptions
	app.Get("/api/v1/push/subscription", apiHandler.HandleGetPushSubscriptionLift)
	app.Post("/api/v1/push/subscription", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreatePushSubscriptionLift,
		20, time.Hour, logger))
	app.Put("/api/v1/push/subscription", ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdatePushSubscriptionLift,
		20, time.Hour, logger))
	app.Delete("/api/v1/push/subscription", apiHandler.HandleDeletePushSubscriptionLift)

	// Scheduled statuses
	app.Get("/api/v1/scheduled_statuses", apiHandler.HandleGetScheduledStatusesLift)
	app.Get("/api/v1/scheduled_statuses/{id}", apiHandler.HandleGetScheduledStatusLift)
	app.Put("/api/v1/scheduled_statuses/{id}", apiHandler.HandleUpdateScheduledStatusLift)
	app.Delete("/api/v1/scheduled_statuses/{id}", apiHandler.HandleDeleteScheduledStatusLift)

	// Follow requests
	app.Get("/api/v1/follow_requests", apiHandler.HandleGetFollowRequestsLift)
	app.Post("/api/v1/follow_requests/{account_id}/authorize", apiHandler.HandleAuthorizeFollowRequestLift)
	app.Post("/api/v1/follow_requests/{account_id}/reject", apiHandler.HandleRejectFollowRequestLift)

	// Domain blocks (user-level)
	app.Get("/api/v1/domain_blocks", apiHandler.HandleGetDomainBlocksLift)
	app.Post("/api/v1/domain_blocks", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateDomainBlockLift,
		20, time.Hour, logger))
	app.Delete("/api/v1/domain_blocks", ratelimit.ApplyRateLimit(
		apiHandler.HandleDeleteDomainBlockLift,
		20, time.Hour, logger))

	// Moderation + reports
	app.Post("/api/v1/moderation/flag", ratelimit.ApplyRateLimit(
		apiHandler.HandleModerationFlagLift,
		30, time.Hour, logger))
	app.Get("/api/v1/moderation/queue", apiHandler.HandleModerationQueueLift)
	app.Post("/api/v1/moderation/review", ratelimit.ApplyRateLimit(
		apiHandler.HandleModerationReviewLift,
		60, time.Hour, logger))
	app.Get("/api/v1/moderation/history/{object_id}", apiHandler.HandleModerationHistoryLift)
	app.Get("/api/v1/moderation/consensus/{event_id}", apiHandler.HandleGetConsensusLift)
	app.Get("/api/v1/moderation/trust", apiHandler.HandleGetTrustRelationshipsLift)
	app.Put("/api/v1/moderation/trust", apiHandler.HandleUpdateTrustLift)
	app.Get("/api/v1/moderation/trust/{actor_id}/score", apiHandler.HandleGetTrustScoreLift)
	app.Post("/api/v1/reports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateReportLift,
		30, time.Hour, logger))

	// Discovery
	app.Get("/api/v1/directory", apiHandler.HandleGetDirectoryLift)
	app.Get("/api/v1/suggestions", apiHandler.HandleGetSuggestionsV1Lift)
	app.Delete("/api/v1/suggestions/{account_id}", apiHandler.HandleRemoveSuggestionLift)
	app.Get("/api/v1/endorsements", apiHandler.HandleGetEndorsementsLift)

	// Announcements
	app.Get("/api/v1/announcements", apiHandler.HandleGetAnnouncementsLift)
	app.Post("/api/v1/announcements/{id}/dismiss", apiHandler.HandleDismissAnnouncementLift)
	app.Put("/api/v1/announcements/{id}/reactions/{name}", apiHandler.HandleAddAnnouncementReactionLift)
	app.Delete("/api/v1/announcements/{id}/reactions/{name}", apiHandler.HandleRemoveAnnouncementReactionLift)

	// Custom emojis
	app.Get("/api/v1/custom_emojis", apiHandler.HandleGetCustomEmojisLift)

	// Reputation + vouches
	app.Get("/api/v1/reputation/{actor_id}", apiHandler.HandleGetReputationLift)
	app.Post("/api/v1/reputation/export", apiHandler.HandleExportReputationLift)
	app.Post("/api/v1/reputation/import", apiHandler.HandleImportReputationLift)
	app.Post("/api/v1/reputation/verify", apiHandler.HandleVerifyReputationLift)
	app.Post("/api/v1/vouches", apiHandler.HandleCreateVouchLift)
	app.Get("/api/v1/vouches/{actor_id}", apiHandler.HandleGetVouchesLift)
	app.Delete("/api/v1/vouches/{vouch_id}", apiHandler.HandleRevokeVouchLift)

	// lesser-host trust proxy (managed instances)
	// Requires user auth for all JSON endpoints; public media endpoints do not require auth.
	app.Post("/api/v1/trust/previews", apiHandler.HandleTrustCreateLinkPreviewLift)
	app.Get("/api/v1/trust/previews/{id}", apiHandler.HandleTrustGetLinkPreviewLift)
	app.Get("/api/v1/trust/previews/images/{imageId}", apiHandler.HandleTrustGetLinkPreviewImageLift)

	app.Post("/api/v1/trust/publish/jobs", apiHandler.HandleTrustCreatePublishJobLift)
	app.Get("/api/v1/trust/publish/jobs/{jobId}", apiHandler.HandleTrustGetPublishJobLift)

	app.Post("/api/v1/trust/renders", apiHandler.HandleTrustCreateRenderLift)
	app.Get("/api/v1/trust/renders/{renderId}", apiHandler.HandleTrustGetRenderLift)
	app.Get("/api/v1/trust/renders/{renderId}/thumbnail", apiHandler.HandleTrustGetRenderThumbnailLift)
	app.Get("/api/v1/trust/renders/{renderId}/snapshot", apiHandler.HandleTrustGetRenderSnapshotLift)

	app.Post("/api/v1/trust/ai/claims/verify", apiHandler.HandleTrustAIClaimVerifyLift)
	app.Get("/api/v1/trust/ai/jobs/{jobId}", apiHandler.HandleTrustGetAIJobLift)

	app.Get("/api/v1/trust/jwks.json", apiHandler.HandleTrustJWKSJSONLift)
	app.Get("/api/v1/trust/attestations", apiHandler.HandleTrustLookupAttestationLift)
	app.Get("/api/v1/trust/attestations/{id}", apiHandler.HandleTrustGetAttestationLift)

	app.Handle("OPTIONS", "/api/v1/trust/previews", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/previews/{id}", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/previews/images/{imageId}", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/publish/jobs", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/publish/jobs/{jobId}", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/renders", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/renders/{renderId}", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/renders/{renderId}/thumbnail", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/renders/{renderId}/snapshot", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/ai/claims/verify", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/ai/jobs/{jobId}", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/jwks.json", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/attestations", optionsHandler)
	app.Handle("OPTIONS", "/api/v1/trust/attestations/{id}", optionsHandler)

	// Admin endpoints (always enabled for administration)
	// Note: RBAC is handled within each handler's requireAdminLift() method
	// Account management (Admin only)
	app.Get("/api/v1/admin/accounts", apiHandler.HandleAdminGetAccountsLift)
	app.Post("/api/v1/admin/accounts", apiHandler.HandleAdminCreateUserLift)
	app.Get("/api/v1/admin/accounts/{id}", apiHandler.HandleAdminGetAccountLift)
	app.Post("/api/v1/admin/accounts/{id}/action", apiHandler.HandleAdminAccountActionLift)
	app.Post("/api/v1/admin/accounts/{id}/approve", apiHandler.HandleAdminApproveAccountLift)
	app.Post("/api/v1/admin/accounts/{id}/reject", apiHandler.HandleAdminRejectAccountLift)
	app.Post("/api/v1/admin/accounts/{id}/enable", apiHandler.HandleAdminEnableAccountLift)
	app.Post("/api/v1/admin/accounts/{id}/unsilence", apiHandler.HandleAdminUnsilenceAccountLift)
	app.Post("/api/v1/admin/accounts/{id}/unsuspend", apiHandler.HandleAdminUnsuspendAccountLift)
	app.Post("/api/v1/admin/accounts/{id}/unsensitive", apiHandler.HandleAdminUnsensitiveAccountLift)

	// Report management (Admin/Moderator)
	app.Get("/api/v1/admin/reports", apiHandler.HandleAdminGetReportsLift)
	app.Get("/api/v1/admin/reports/{id}", apiHandler.HandleAdminGetReportLift)
	app.Post("/api/v1/admin/reports/{id}/resolve", apiHandler.HandleAdminResolveReportLift)
	app.Post("/api/v1/admin/reports/{id}/reopen", apiHandler.HandleAdminReopenReportLift)
	app.Post("/api/v1/admin/reports/{id}/assign_to_self", apiHandler.HandleAdminAssignReportLift)
	app.Post("/api/v1/admin/reports/{id}/unassign", apiHandler.HandleAdminUnassignReportLift)

	// Status moderation (Admin only for deletion, Admin/Moderator for sensitivity)
	app.Get("/api/v1/admin/statuses", apiHandler.HandleAdminGetStatusesLift)
	app.Get("/api/v1/admin/statuses/{id}", apiHandler.HandleAdminGetStatusLift)
	app.Delete("/api/v1/admin/statuses/{id}", apiHandler.HandleAdminDeleteStatusLift)
	app.Post("/api/v1/admin/statuses/{id}/sensitive", apiHandler.HandleAdminMarkStatusSensitiveLift)
	app.Post("/api/v1/admin/statuses/{id}/unsensitive", apiHandler.HandleAdminUnmarkStatusSensitiveLift)

	// Agent governance (Admin only)
	app.Get("/api/v1/admin/agents/policy", apiHandler.HandleAdminGetAgentPolicyLift)
	app.Put("/api/v1/admin/agents/policy", apiHandler.HandleAdminUpdateAgentPolicyLift)
	app.Post("/api/v1/admin/agents/{username}/verify", apiHandler.HandleAdminVerifyAgentLift)
	app.Post("/api/v1/admin/agents/{username}/unverify", apiHandler.HandleAdminUnverifyAgentLift)

	// Domain blocks (Admin only)
	app.Get("/api/v1/admin/domain_blocks", apiHandler.HandleGetAdminDomainBlocksLift)
	app.Get("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleGetAdminDomainBlockLift)
	app.Post("/api/v1/admin/domain_blocks", apiHandler.HandleCreateAdminDomainBlockLift)
	app.Put("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleUpdateAdminDomainBlockLift)
	app.Delete("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleDeleteAdminDomainBlockLift)

	// Domain allows (Admin only)
	app.Get("/api/v1/admin/domain_allows", apiHandler.HandleGetAdminDomainAllowsLift)
	app.Post("/api/v1/admin/domain_allows", apiHandler.HandleCreateAdminDomainAllowLift)
	app.Delete("/api/v1/admin/domain_allows/{id}", apiHandler.HandleDeleteAdminDomainAllowLift)

	// Email domain blocks (Admin only)
	app.Get("/api/v1/admin/email_domain_blocks", apiHandler.HandleGetEmailDomainBlocksLift)
	app.Post("/api/v1/admin/email_domain_blocks", apiHandler.HandleCreateEmailDomainBlockLift)
	app.Delete("/api/v1/admin/email_domain_blocks/{id}", apiHandler.HandleDeleteEmailDomainBlockLift)

	// Federation (Admin only)
	app.Get("/api/v1/admin/federation/instances", apiHandler.HandleGetFederationInstancesLift)
	app.Get("/api/v1/admin/federation/instance/{domain}", apiHandler.HandleGetFederationInstanceLift)
	app.Get("/api/v1/admin/federation/statistics", apiHandler.HandleGetFederationStatisticsLift)

	// Announcements (Admin only)
	app.Post("/api/v1/admin/announcements", apiHandler.HandleCreateAnnouncementLift)

	// Custom emojis (Admin only)
	app.Post("/api/v1/admin/custom_emojis", apiHandler.HandleCreateCustomEmojiLift)
	app.Put("/api/v1/admin/custom_emojis/{shortcode}", apiHandler.HandleUpdateCustomEmojiLift)
	app.Delete("/api/v1/admin/custom_emojis/{shortcode}", apiHandler.HandleDeleteCustomEmojiLift)

	// Moderation overview and events (Admin/Moderator)
	app.Get("/api/v1/admin/moderation/overview", apiHandler.HandleAdminModerationOverviewLift)
	app.Get("/api/v1/admin/moderation/events", apiHandler.HandleAdminGetModerationEventsLift)
	app.Post("/api/v1/admin/moderation/events/{id}/override", apiHandler.HandleAdminOverrideModerationEventLift)

	// Trust graph management (Admin only)
	app.Get("/api/v1/admin/moderation/trust/graph", apiHandler.HandleAdminGetTrustGraphLift)
	app.Put("/api/v1/admin/moderation/trust/{from}/{to}", apiHandler.HandleAdminUpdateTrustLift)

	// Search endpoints with privacy enforcement (always enabled)
	// Account search is rate limited (scraping prevention)
	app.Get("/api/v1/accounts/search", ratelimit.ApplyRateLimit(
		apiHandler.HandleAccountSearchLift,
		30, 5*time.Minute, logger))
	// Suggestions are NOT rate limited
	app.Get("/api/v1/accounts/search/suggestions", apiHandler.HandleGetSearchSuggestionsLift)
	// Status search is rate limited (GET and POST)
	app.Get("/api/v1/search/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleStatusSearchLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/search/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleStatusSearchLift,
		30, 5*time.Minute, logger))

	// Relationship interactions
	app.Post("/api/v1/accounts/{id}/follow", ratelimit.ApplyRateLimit(
		apiHandler.HandleFollowLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/accounts/{id}/unfollow", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnfollowLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/accounts/{id}/block", ratelimit.ApplyRateLimit(
		apiHandler.HandleBlockLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/accounts/{id}/unblock", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnblockLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/accounts/{id}/mute", ratelimit.ApplyRateLimit(
		apiHandler.HandleMuteAccountLift,
		30, 5*time.Minute, logger))
	app.Post("/api/v1/accounts/{id}/unmute", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnmuteAccountLift,
		30, 5*time.Minute, logger))
	app.Get("/api/v1/blocks", ratelimit.ApplyRateLimit(
		apiHandler.HandleGetBlocksLift,
		60, time.Hour, logger))
	app.Get("/api/v1/mutes", ratelimit.ApplyRateLimit(
		apiHandler.HandleGetMutedAccountsLift,
		60, time.Hour, logger))

	// Reviewer management (Admin only)
	app.Get("/api/v1/admin/moderation/reviewers", apiHandler.HandleAdminGetReviewersLift)
	app.Post("/api/v1/admin/moderation/reviewers/{id}/promote", apiHandler.HandleAdminPromoteModeratorLift)
	app.Post("/api/v1/admin/moderation/reviewers/{id}/demote", apiHandler.HandleAdminDemoteModeratorLift)

	// Media endpoints - V1 (synchronous) and V2 (asynchronous)
	// V1 Media endpoints (backwards compatibility)
	// POST is rate limited (storage abuse prevention)
	app.Post("/api/v1/media", ratelimit.ApplyRateLimit(
		apiHandler.HandleUploadMediaLift,
		20, time.Hour, logger))
	// GET and PUT are NOT rate limited
	app.Get("/api/v1/media/{id}", apiHandler.HandleGetMediaLift)
	app.Put("/api/v1/media/{id}", apiHandler.HandleUpdateMediaLift)

	// Note: V2 media endpoints have been consolidated into main media handlers

	// Conversation endpoints (Direct Messages) - always enabled for 100% Mastodon API compatibility
	app.Get("/api/v1/conversations", apiHandler.HandleGetConversationsLift)
	app.Delete("/api/v1/conversations/{id}", apiHandler.HandleDeleteConversationLift)
	app.Post("/api/v1/conversations/{id}/read", apiHandler.HandleMarkConversationReadLift)

	// Instance endpoints
	app.Get("/api/v1/instance", apiHandler.HandleGetInstanceV1Lift)
	app.Get("/api/v1/instance/peers", apiHandler.HandleGetInstancePeersLift)
	app.Get("/api/v1/instance/activity", apiHandler.HandleGetInstanceActivityLift)
	app.Get("/api/v1/instance/domain_blocks", apiHandler.HandleGetInstanceDomainBlocksLift)
	app.Get("/api/v1/instance/translation_languages", apiHandler.HandleGetTranslationLanguagesLift)

	// API v2 endpoints - Enhanced Mastodon compatibility
	app.Get("/api/v2/instance", apiHandler.HandleGetInstanceV2Lift)
	app.Get("/api/v2/search", apiHandler.HandleSearchV2Lift)
	app.Get("/api/v2/suggestions", apiHandler.HandleGetSuggestionsV2Lift)

	// API v2 filters (advanced filtering) - existing implementations
	app.Get("/api/v2/filters", apiHandler.HandleGetFiltersLift)
	app.Get("/api/v2/filters/{id}", apiHandler.HandleGetFilterLift)
	app.Post("/api/v2/filters", apiHandler.HandleCreateFilterLift)
	app.Put("/api/v2/filters/{id}", apiHandler.HandleUpdateFilterLift)
	app.Delete("/api/v2/filters/{id}", apiHandler.HandleDeleteFilterLift)

	// API v2 filter keywords and statuses
	app.Get("/api/v2/filters/{filter_id}/keywords", apiHandler.HandleGetFilterKeywordsLift)
	app.Post("/api/v2/filters/{filter_id}/keywords", apiHandler.HandleAddFilterKeywordLift)
	app.Delete("/api/v2/filters/{filter_id}/keywords/{keyword_id}", apiHandler.HandleDeleteFilterKeywordLift)
	app.Get("/api/v2/filters/{filter_id}/statuses", apiHandler.HandleGetFilterStatusesLift)
	app.Post("/api/v2/filters/{filter_id}/statuses", apiHandler.HandleAddFilterStatusLift)
	app.Delete("/api/v2/filters/{filter_id}/statuses/{status_id}", apiHandler.HandleDeleteFilterStatusLift)

	// API v2 trends endpoints - Enhanced trending with metadata
	app.Get("/api/v2/trends", apiHandler.HandleGetTrendsV2Lift)
	app.Get("/api/v2/trends/tags", apiHandler.HandleGetTrendingTagsV2Lift)
	app.Get("/api/v2/trends/statuses", apiHandler.HandleGetTrendingStatusesV2Lift)
	app.Get("/api/v2/trends/links", apiHandler.HandleGetTrendingLinksV2Lift)

	// API v2 filter testing endpoint
	app.Post("/api/v2/filters/test", apiHandler.HandleTestFilterLift)

	// API v2 grouped notifications endpoints
	app.Get("/api/v2/notifications/grouped", apiHandler.HandleGetGroupedNotificationsLift)
	app.Post("/api/v2/notifications/groups/{group_id}/read", apiHandler.HandleMarkGroupAsReadLift)

	// Quote posts API endpoints
	// POST create quote is rate limited (spam prevention)
	app.Post("/api/v1/statuses/{id}/quote", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateQuotePostLift,
		30, time.Hour, logger))
	// GETs, DELETE, and PUT are NOT rate limited
	app.Get("/api/v1/statuses/{id}/quotes", apiHandler.HandleGetQuotesOfStatusLift)
	app.Delete("/api/v1/statuses/{id}/quote/{quote_id}", apiHandler.HandleDeleteQuotePostLift)
	app.Get("/api/v1/accounts/{id}/quote_permissions", apiHandler.HandleGetQuotePermissionsLift)
	app.Put("/api/v1/accounts/quote_permissions", apiHandler.HandleUpdateQuotePermissionsLift)
}

func redirectResponse(url string, permanent bool) *apptheory.Response {
	status := http.StatusFound
	if permanent {
		status = http.StatusMovedPermanently
	}
	return &apptheory.Response{
		Status: status,
		Headers: map[string][]string{
			"location": {url},
		},
	}
}
