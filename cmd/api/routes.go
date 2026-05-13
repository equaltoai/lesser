package main

import (
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/crawler"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureRoutes(app *apptheory.App) {
	optionalAuth := apptheory.OptionalAuth()
	requireAuth := apptheory.RequireAuth()
	requireRead := apptheory.RequireScope(auth.ScopeRead)
	requireWrite := apptheory.RequireScope(auth.ScopeWrite)
	requireAdmin := apptheory.RequireScope(auth.ScopeAdmin)
	requireAdminRead := apptheory.RequireAnyScope(auth.ScopeAdmin, "admin:read")
	requireAdminWrite := apptheory.RequireAnyScope(auth.ScopeAdmin, "admin:write")
	requireReadOrWrite := apptheory.RequireAnyScope(auth.ScopeRead, auth.ScopeWrite)
	requireWriteOrAdmin := apptheory.RequireAnyScope(auth.ScopeWrite, auth.ScopeAdmin)
	requireAccountRead := apptheory.RequireAnyScope("read:accounts", auth.ScopeRead)
	requireAccountWrite := apptheory.RequireAnyScope("write:accounts", auth.ScopeWrite)
	requireStatusWrite := apptheory.RequireAnyScope("write:statuses", auth.ScopeWrite)
	requireFollowRead := apptheory.RequireAnyScope(auth.ReadFollows, auth.ScopeRead)
	requireFollowWrite := apptheory.RequireAnyScope(auth.ScopeFollow, auth.WriteFollows, auth.ScopeWrite)
	requireBlockRead := apptheory.RequireAnyScope(auth.ReadBlocks, auth.ScopeRead)
	requireBlockWrite := apptheory.RequireAnyScope(auth.WriteBlocks, auth.ScopeWrite)
	requireNotificationRead := apptheory.RequireAnyScope(auth.ReadNotifications, auth.ScopeRead)
	requireNotificationWrite := apptheory.RequireAnyScope(auth.WriteNotifications, auth.ScopeWrite)
	requireFilterRead := apptheory.RequireAnyScope(auth.ReadFilters, auth.ScopeRead)
	requireFilterWrite := apptheory.RequireAnyScope(auth.WriteFilters, auth.ScopeWrite)
	requirePushRead := apptheory.RequireAnyScope(auth.ScopePush, auth.ScopeRead)
	requirePushWrite := apptheory.RequireAnyScope(auth.ScopePush, auth.ScopeWrite)
	requireManageAgents := apptheory.RequireAnyScope("write:accounts", auth.ScopeWrite)

	app.Get("/", func(*apptheory.Context) (*apptheory.Response, error) {
		return redirectResponse("/l/", false), nil
	})
	app.Handle("HEAD", "/", func(*apptheory.Context) (*apptheory.Response, error) {
		return redirectResponse("/l/", false), nil
	})
	app.Get("/robots.txt", crawler.RobotsHandler)

	// OAuth app registration (public, no auth required)
	app.Post("/api/v1/apps", apiHandler.HandleAppRegistrationLift, optionalAuth)
	app.Post("/api/v1/apps/{id}/rotate_secret", apiHandler.HandleAppRotateSecretLift, requireWriteOrAdmin)

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
	app.Post("/auth/wallet/link", apiHandler.HandleLinkWalletLift, optionalAuth)
	app.Delete("/auth/wallet/unlink/{address}", apiHandler.HandleUnlinkWalletLift, requireAuth)
	app.Get("/auth/wallet/list", apiHandler.HandleGetWalletsLift, requireAuth)
	app.Get("/auth/device", apiHandler.HandleOAuthDevicePageLift)

	// Real CORS preflights are handled centrally by AppTheory when WithCORS is enabled.

	// WebAuthn (passkey) endpoints
	// Registration begin/finish requires auth (binds a passkey to the logged-in user).
	app.Post("/api/v1/auth/webauthn/register/begin", ratelimit.ApplyRateLimit(
		apiHandler.HandleBeginWebAuthnRegistrationLift,
		20, 5*time.Minute, logger), requireAuth)
	app.Post("/api/v1/auth/webauthn/register/finish", ratelimit.ApplyRateLimit(
		apiHandler.HandleFinishWebAuthnRegistrationLift,
		20, 5*time.Minute, logger), requireAuth)
	// Login begin/finish is public (username provided), but rate limited.
	app.Post("/api/v1/auth/webauthn/login/begin", ratelimit.ApplyRateLimit(
		apiHandler.HandleBeginWebAuthnLoginLift,
		20, 5*time.Minute, logger))
	app.Post("/api/v1/auth/webauthn/login/finish", ratelimit.ApplyRateLimit(
		apiHandler.HandleFinishWebAuthnLoginLift,
		20, 5*time.Minute, logger))

	app.Get("/api/v1/auth/webauthn/credentials", apiHandler.HandleListWebAuthnCredentialsLift, requireAuth)
	app.Delete("/api/v1/auth/webauthn/credentials/{credentialId}", apiHandler.HandleDeleteWebAuthnCredentialLift, requireAuth)
	app.Put("/api/v1/auth/webauthn/credentials/{credentialId}", apiHandler.HandleUpdateWebAuthnCredentialNameLift, requireAuth)

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
	app.Post("/oauth/register", ratelimit.ApplyOAuthRegistrationRateLimit(
		apiHandler.HandleOAuthDynamicClientRegistrationLift,
		20, time.Minute, logger), optionalAuth)
	app.Post("/oauth/token", ratelimit.ApplyOAuthTokenRateLimit(
		apiHandler.HandleOAuthTokenLift,
		10, time.Minute, logger))
	app.Post("/oauth/revoke", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthRevokeLift,
		10, time.Minute, logger))
	app.Get("/.well-known/oauth-authorization-server", apiHandler.HandleOAuthAuthorizationServerMetadataLift)

	// NodeInfo endpoints with native Lift implementation
	app.Get("/.well-known/nodeinfo", apiHandler.HandleNodeInfoWellKnownLift)
	// lesser-soul HTTPS proof
	app.Get("/.well-known/lesser-soul-agent", apiHandler.HandleWellKnownLesserSoulAgentLift)
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

	// Account verification/update endpoints with native Lift implementation
	// verify_credentials is NOT rate limited (read-only)
	app.Get("/api/v1/accounts/verify_credentials", apiHandler.HandleVerifyCredentialsLift, requireRead)
	// update_credentials IS rate limited
	app.Patch("/api/v1/accounts/update_credentials", ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdateCredentialsLift,
		10, time.Hour, logger), requireWrite)
	// account registration
	app.Post("/api/v1/accounts", ratelimit.ApplyRateLimit(
		apiHandler.HandleRegistrationLift,
		10, time.Hour, logger))

	// Agent endpoints (LLM agent support)
	app.Get("/api/v1/agents", apiHandler.HandleListAgentsLift)
	app.Post("/api/v1/agents/delegate", apiHandler.HandleDelegateAgentLift, requireManageAgents)
	app.Post("/api/v1/agents/{username}/access-leases/challenge/principal", apiHandler.HandleCreateAgentAccessLeasePrincipalChallengeLift, requireManageAgents)
	app.Post("/api/v1/agents/{username}/access-leases/challenge/agent", apiHandler.HandleCreateAgentAccessLeaseAgentChallengeLift, requireManageAgents)
	app.Post("/api/v1/agents/{username}/access-leases", apiHandler.HandleCreateAgentAccessLeaseLift, requireManageAgents)
	app.Get("/api/v1/agents/{username}/access-leases", apiHandler.HandleListAgentAccessLeasesLift, requireManageAgents)
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/revoke", apiHandler.HandleRevokeAgentAccessLeaseLift, requireManageAgents)
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/session-key/challenge", apiHandler.HandleCreateAgentAccessLeaseSessionKeyChallengeLift)
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/session-key", apiHandler.HandleAuthorizeAgentAccessLeaseSessionKeyLift)
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/renew/challenge", apiHandler.HandleCreateAgentAccessLeaseRenewChallengeLift)
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/token", apiHandler.HandleExchangeAgentAccessLeaseTokenLift)
	app.Get("/api/v1/agents/{username}/runtime-sessions", apiHandler.HandleListAgentRuntimeSessionsLift, requireManageAgents)
	app.Post("/api/v1/agents/{username}/runtime-sessions/{sessionID}/revoke", apiHandler.HandleRevokeAgentRuntimeSessionLift, requireManageAgents)
	app.Post("/api/v1/agents/register/challenge", apiHandler.HandleAgentRegisterChallengeLift)
	app.Post("/api/v1/agents/register", apiHandler.HandleAgentRegisterLift)
	app.Post("/api/v1/agents/auth/challenge", apiHandler.HandleAgentAuthChallengeLift)
	app.Post("/api/v1/agents/auth/token", apiHandler.HandleAgentAuthTokenLift)
	app.Get("/api/v1/agents/{username}", apiHandler.HandleGetAgentLift)
	app.Patch("/api/v1/agents/{username}", apiHandler.HandleUpdateAgentLift, requireManageAgents)
	app.Delete("/api/v1/agents/{username}", apiHandler.HandleDeleteAgentLift, requireManageAgents)
	app.Get("/api/v1/agents/{username}/activity", apiHandler.HandleGetAgentActivityLift, requireRead)
	app.Post("/api/v1/agents/{username}/rotate-key/challenge", apiHandler.HandleAgentRotateKeyChallengeLift, requireRead)
	app.Post("/api/v1/agents/{username}/rotate-key", apiHandler.HandleAgentRotateKeyLift, requireWrite)
	app.Get("/api/v1/agents/memory/search", apiHandler.HandleAgentMemorySearchLift, requireRead)
	app.Post("/api/v1/agents/memory/search", apiHandler.HandleAgentMemorySearchLift, requireRead)
	app.Post("/api/v1/agents/{username}/suspend", apiHandler.HandleSuspendAgentLift, requireAdmin)

	app.Get("/api/v1/accounts/{id}/followers", apiHandler.HandleGetAccountFollowersLift, requireRead)
	app.Get("/api/v1/accounts/{id}/following", apiHandler.HandleGetAccountFollowingLift, requireRead)

	// Relationships endpoint with native Lift implementation
	app.Get("/api/v1/accounts/relationships", apiHandler.HandleGetRelationshipsLift, requireFollowRead)

	// Data exports with native Lift implementation
	// POST is rate limited (expensive operation)
	app.Post("/api/v1/exports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateExportLift,
		5, 24*time.Hour, logger), requireRead)
	// GETs are NOT rate limited (read-only)
	app.Get("/api/v1/exports/{id}", apiHandler.HandleGetExportStatusLift, requireRead)
	app.Get("/api/v1/exports/{id}/download", apiHandler.HandleDownloadExportLift, requireRead)
	app.Get("/api/v1/exports", apiHandler.HandleListExportsLift, requireRead)

	// Data imports with native Lift implementation
	// POST is rate limited (expensive operation)
	app.Post("/api/v1/imports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateImportLift,
		5, 24*time.Hour, logger), requireWrite)
	// GETs and DELETE are NOT rate limited
	app.Get("/api/v1/imports/{id}", apiHandler.HandleGetImportStatusLift, requireReadOrWrite)
	app.Delete("/api/v1/imports/{id}", apiHandler.HandleCancelImportLift, requireWrite)
	app.Get("/api/v1/imports", apiHandler.HandleListImportsLift, requireReadOrWrite)

	// Community Notes endpoints with native Lift implementation
	// POST create note is rate limited
	app.Post("/api/v1/notes", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateNoteLift,
		20, time.Hour, logger), requireAuth)
	// GETs are NOT rate limited
	app.Get("/api/v1/notes/{object_id}", apiHandler.HandleGetNotesLift, optionalAuth)
	// POST vote is rate limited
	app.Post("/api/v1/notes/{id}/vote", ratelimit.ApplyRateLimit(
		apiHandler.HandleVoteNoteLift,
		100, time.Hour, logger), requireAuth)
	app.Get("/api/v1/accounts/{id}/notes", apiHandler.HandleGetUserNotesLift)

	// Status endpoints (Mastodon parity)
	app.Post("/api/v1/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateStatusLift,
		300, time.Hour, logger), requireWrite)
	app.Put("/api/v1/statuses/{id}", ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdateStatusLift,
		300, time.Hour, logger), requireWrite)
	app.Delete("/api/v1/statuses/{id}", ratelimit.ApplyRateLimit(
		apiHandler.HandleDeleteStatusLift,
		300, time.Hour, logger), requireWrite)
	app.Get("/api/v1/statuses/{id}", apiHandler.HandleGetStatusLift, optionalAuth)
	app.Get("/api/v1/statuses/{id}/context", apiHandler.HandleGetStatusContextLift, optionalAuth)
	app.Get("/api/v1/statuses/{id}/history", apiHandler.HandleGetStatusHistoryLift, optionalAuth)
	app.Get("/api/v1/statuses/{id}/source", apiHandler.HandleGetStatusSourceLift, requireRead)
	app.Get("/api/v1/statuses/{id}/favourited_by", apiHandler.HandleGetStatusFavouritedByLift, requireRead)
	app.Get("/api/v1/statuses/{id}/reblogged_by", apiHandler.HandleGetStatusRebloggedByLift, requireRead)
	app.Get("/api/v1/accounts/{id}/statuses", apiHandler.HandleGetAccountStatusesLift, optionalAuth)

	// Additional Mastodon parity endpoints

	// Timelines
	app.Get("/api/v1/timelines/home", apiHandler.HandleGetHomeTimelineLift, requireRead)
	app.Get("/api/v1/timelines/public", apiHandler.HandleGetPublicTimelineLift, optionalAuth)
	app.Get("/api/v1/timelines/tag/{hashtag}", apiHandler.HandleGetTagTimelineLift, optionalAuth)
	app.Get("/api/v1/timelines/list/{list_id}", apiHandler.HandleGetListTimelineLift, requireRead)
	app.Get("/api/v1/timelines/direct", apiHandler.HandleGetDirectTimelineLift, requireRead)
	app.Get("/api/v1/timelines/link", apiHandler.HandleGetLinkTimelineLift)

	// Trends (Mastodon v1)
	app.Get("/api/v1/trends", apiHandler.HandleGetTrendsLift)
	app.Get("/api/v1/trends/tags", apiHandler.HandleGetTrendingTagsLift)
	app.Get("/api/v1/trends/statuses", apiHandler.HandleGetTrendingStatusesLift)
	app.Get("/api/v1/trends/links", apiHandler.HandleGetTrendingLinksLift)

	// Status interactions
	app.Post("/api/v1/statuses/{id}/favourite", ratelimit.ApplyRateLimit(
		apiHandler.HandleFavoriteLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/unfavourite", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnfavoriteLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/reblog", ratelimit.ApplyRateLimit(
		apiHandler.HandleReblogLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/unreblog", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnreblogLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/bookmark", ratelimit.ApplyRateLimit(
		apiHandler.HandleBookmarkLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/unbookmark", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnbookmarkLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/pin", ratelimit.ApplyRateLimit(
		apiHandler.HandlePinStatusLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/unpin", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnpinStatusLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/mute", ratelimit.ApplyRateLimit(
		apiHandler.HandleMuteConversationLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/unmute", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnmuteConversationLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/statuses/{id}/translate", ratelimit.ApplyRateLimit(
		apiHandler.HandleTranslateStatusLift,
		20, time.Hour, logger), requireAuth)

	// Bookmarks + favourites
	app.Get("/api/v1/bookmarks", apiHandler.HandleGetBookmarksLift, requireRead)
	app.Get("/api/v1/favourites", apiHandler.HandleGetFavouritesLift, requireRead)

	// Lists
	app.Get("/api/v1/lists", apiHandler.HandleGetListsLift, requireRead)
	app.Post("/api/v1/lists", apiHandler.HandleCreateListLift, requireWrite)
	app.Get("/api/v1/lists/{id}", apiHandler.HandleGetListLift, requireRead)
	app.Put("/api/v1/lists/{id}", apiHandler.HandleUpdateListLift, requireWrite)
	app.Delete("/api/v1/lists/{id}", apiHandler.HandleDeleteListLift, requireWrite)
	app.Get("/api/v1/lists/{id}/accounts", apiHandler.HandleGetListAccountsLift, requireRead)
	app.Post("/api/v1/lists/{id}/accounts", apiHandler.HandleAddAccountsToListLift, requireWrite)
	app.Delete("/api/v1/lists/{id}/accounts", apiHandler.HandleRemoveAccountsFromListLift, requireWrite)

	// Notifications
	app.Get("/api/v1/notifications", apiHandler.HandleGetNotificationsLift, requireNotificationRead)
	app.Get("/api/v1/notifications/{id}", apiHandler.HandleGetNotificationLift, requireNotificationRead)
	app.Post("/api/v1/notifications/deliver", apiHandler.HandleDeliverNotificationLift)
	app.Post("/api/v1/notifications/clear", apiHandler.HandleClearNotificationsLift, requireNotificationWrite)
	app.Post("/api/v1/notifications/{id}/dismiss", apiHandler.HandleDismissNotificationLift, requireNotificationWrite)

	// Preferences + markers
	app.Get("/api/v1/preferences", apiHandler.HandleGetPreferencesLift, requireRead)
	app.Patch("/api/v1/preferences", apiHandler.HandleUpdatePreferencesLift, requireWrite)
	app.Get("/api/v1/markers", apiHandler.HandleGetMarkersLift, requireRead)
	app.Post("/api/v1/markers", apiHandler.HandleSaveMarkersLift, requireWrite)

	// Push subscriptions
	app.Get("/api/v1/push/subscription", apiHandler.HandleGetPushSubscriptionLift, requirePushRead)
	app.Post("/api/v1/push/subscription", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreatePushSubscriptionLift,
		20, time.Hour, logger), requirePushWrite)
	app.Put("/api/v1/push/subscription", ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdatePushSubscriptionLift,
		20, time.Hour, logger), requirePushWrite)
	app.Delete("/api/v1/push/subscription", apiHandler.HandleDeletePushSubscriptionLift, requirePushWrite)

	// Scheduled statuses
	app.Get("/api/v1/scheduled_statuses", apiHandler.HandleGetScheduledStatusesLift, requireRead)
	app.Get("/api/v1/scheduled_statuses/{id}", apiHandler.HandleGetScheduledStatusLift, requireRead)
	app.Put("/api/v1/scheduled_statuses/{id}", apiHandler.HandleUpdateScheduledStatusLift, requireWrite)
	app.Delete("/api/v1/scheduled_statuses/{id}", apiHandler.HandleDeleteScheduledStatusLift, requireWrite)

	// Follow requests
	app.Get("/api/v1/follow_requests", apiHandler.HandleGetFollowRequestsLift, requireFollowRead)
	app.Post("/api/v1/follow_requests/{account_id}/authorize", apiHandler.HandleAuthorizeFollowRequestLift, requireFollowWrite)
	app.Post("/api/v1/follow_requests/{account_id}/reject", apiHandler.HandleRejectFollowRequestLift, requireFollowWrite)

	// Domain blocks (user-level)
	app.Get("/api/v1/domain_blocks", apiHandler.HandleGetDomainBlocksLift, requireBlockRead)
	app.Post("/api/v1/domain_blocks", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateDomainBlockLift,
		20, time.Hour, logger), requireBlockWrite)
	app.Delete("/api/v1/domain_blocks", ratelimit.ApplyRateLimit(
		apiHandler.HandleDeleteDomainBlockLift,
		20, time.Hour, logger), requireBlockWrite)

	// Moderation + reports
	app.Post("/api/v1/moderation/flag", ratelimit.ApplyRateLimit(
		apiHandler.HandleModerationFlagLift,
		30, time.Hour, logger), requireAuth)
	app.Get("/api/v1/moderation/queue", apiHandler.HandleModerationQueueLift, requireAuth)
	app.Post("/api/v1/moderation/review", ratelimit.ApplyRateLimit(
		apiHandler.HandleModerationReviewLift,
		60, time.Hour, logger), requireAuth)
	app.Get("/api/v1/moderation/history/{object_id}", apiHandler.HandleModerationHistoryLift, requireAuth)
	app.Get("/api/v1/moderation/consensus/{event_id}", apiHandler.HandleGetConsensusLift, requireAuth)
	app.Get("/api/v1/moderation/trust", apiHandler.HandleGetTrustRelationshipsLift, requireAuth)
	app.Put("/api/v1/moderation/trust", apiHandler.HandleUpdateTrustLift, requireAuth)
	app.Get("/api/v1/moderation/trust/{actor_id}/score", apiHandler.HandleGetTrustScoreLift, requireAuth)
	app.Post("/api/v1/reports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateReportLift,
		30, time.Hour, logger), requireWrite)

	// Discovery
	app.Get("/api/v1/directory", apiHandler.HandleGetDirectoryLift)
	app.Get("/api/v1/suggestions", apiHandler.HandleGetSuggestionsV1Lift, requireAuth)
	app.Delete("/api/v1/suggestions/{account_id}", apiHandler.HandleRemoveSuggestionLift, requireAuth)
	app.Get("/api/v1/endorsements", apiHandler.HandleGetEndorsementsLift, requireAccountRead)

	// Announcements
	app.Get("/api/v1/announcements", apiHandler.HandleGetAnnouncementsLift)
	app.Post("/api/v1/announcements/{id}/dismiss", apiHandler.HandleDismissAnnouncementLift, requireAuth)
	app.Put("/api/v1/announcements/{id}/reactions/{name}", apiHandler.HandleAddAnnouncementReactionLift, requireAuth)
	app.Delete("/api/v1/announcements/{id}/reactions/{name}", apiHandler.HandleRemoveAnnouncementReactionLift, requireAuth)

	// Custom emojis
	app.Get("/api/v1/custom_emojis", apiHandler.HandleGetCustomEmojisLift)

	// Reputation + vouches
	app.Get("/api/v1/reputation/{actor_id}", apiHandler.HandleGetReputationLift, requireAuth)
	app.Post("/api/v1/reputation/export", apiHandler.HandleExportReputationLift, requireAuth)
	app.Post("/api/v1/reputation/import", apiHandler.HandleImportReputationLift, requireAuth)
	app.Post("/api/v1/reputation/verify", apiHandler.HandleVerifyReputationLift, requireAuth)
	app.Post("/api/v1/vouches", apiHandler.HandleCreateVouchLift, requireAuth)
	app.Get("/api/v1/vouches/{actor_id}", apiHandler.HandleGetVouchesLift)
	app.Delete("/api/v1/vouches/{vouch_id}", apiHandler.HandleRevokeVouchLift, requireAuth)

	// Canonical skill authority (Lesser-exclusive additive API)
	app.Get("/api/v1/skills", apiHandler.HandleListSkillsLift, optionalAuth)
	app.Get("/api/v1/skills/resolve", apiHandler.HandleResolveEffectiveSkillsLift, requireRead)
	app.Get("/api/v1/skills/{skillId}", apiHandler.HandleGetSkillLift, optionalAuth)
	app.Get("/api/v1/skills/{skillId}/revisions", apiHandler.HandleListSkillRevisionsLift, optionalAuth)
	app.Get("/api/v1/skills/{skillId}/revisions/{revisionNumber}", apiHandler.HandleGetSkillRevisionLift, optionalAuth)

	// Souls
	app.Get("/api/v1/souls/bound/me", apiHandler.HandleGetBoundSoulMeLift, requireReadOrWrite)
	app.Get("/api/v1/souls/bound/me/mint-conversations", apiHandler.HandleListBoundSoulMintConversationsLift, requireRead)
	app.Get("/api/v1/souls/bound/me/mint-conversations/{conversationId}", apiHandler.HandleGetBoundSoulMintConversationLift, requireRead)
	app.Get("/api/v1/souls/mine", apiHandler.HandleGetMySoulsLift, requireReadOrWrite)
	app.Post("/api/v1/souls/{agentId}/incorporate", apiHandler.HandleIncorporateSoulLift, requireWrite)

	// lesser-host trust proxy (managed instances)
	// Requires user auth for all JSON endpoints; public media endpoints do not require auth.
	app.Post("/api/v1/trust/previews", apiHandler.HandleTrustCreateLinkPreviewLift, requireAuth)
	app.Get("/api/v1/trust/previews/{id}", apiHandler.HandleTrustGetLinkPreviewLift, requireAuth)
	app.Get("/api/v1/trust/previews/images/{imageId}", apiHandler.HandleTrustGetLinkPreviewImageLift)

	app.Post("/api/v1/trust/publish/jobs", apiHandler.HandleTrustCreatePublishJobLift, requireAuth)
	app.Get("/api/v1/trust/publish/jobs/{jobId}", apiHandler.HandleTrustGetPublishJobLift, requireAuth)

	app.Post("/api/v1/trust/renders", apiHandler.HandleTrustCreateRenderLift, requireAuth)
	app.Get("/api/v1/trust/renders/{renderId}", apiHandler.HandleTrustGetRenderLift, requireAuth)
	app.Get("/api/v1/trust/renders/{renderId}/thumbnail", apiHandler.HandleTrustGetRenderThumbnailLift)
	app.Get("/api/v1/trust/renders/{renderId}/snapshot", apiHandler.HandleTrustGetRenderSnapshotLift)

	app.Post("/api/v1/trust/ai/claims/verify", apiHandler.HandleTrustAIClaimVerifyLift, requireAuth)
	app.Get("/api/v1/trust/ai/jobs/{jobId}", apiHandler.HandleTrustGetAIJobLift, requireAuth)

	app.Get("/api/v1/trust/jwks.json", apiHandler.HandleTrustJWKSJSONLift)
	app.Get("/api/v1/trust/attestations", apiHandler.HandleTrustLookupAttestationLift)
	app.Get("/api/v1/trust/attestations/{id}", apiHandler.HandleTrustGetAttestationLift)

	// Admin endpoints (always enabled for administration)
	// Note: RBAC is handled within each handler's requireAdminLift() method
	// Account management (Admin only)
	app.Get("/api/v1/admin/accounts", apiHandler.HandleAdminGetAccountsLift, requireAuth)
	app.Post("/api/v1/admin/accounts", apiHandler.HandleAdminCreateUserLift, requireAuth)
	app.Get("/api/v1/admin/accounts/{id}", apiHandler.HandleAdminGetAccountLift, requireAuth)
	app.Post("/api/v1/admin/accounts/{id}/action", apiHandler.HandleAdminAccountActionLift, requireAuth)
	app.Post("/api/v1/admin/accounts/{id}/approve", apiHandler.HandleAdminApproveAccountLift, requireAuth)
	app.Post("/api/v1/admin/accounts/{id}/reject", apiHandler.HandleAdminRejectAccountLift, requireAuth)
	app.Post("/api/v1/admin/accounts/{id}/enable", apiHandler.HandleAdminEnableAccountLift, requireAuth)
	app.Post("/api/v1/admin/accounts/{id}/unsilence", apiHandler.HandleAdminUnsilenceAccountLift, requireAuth)
	app.Post("/api/v1/admin/accounts/{id}/unsuspend", apiHandler.HandleAdminUnsuspendAccountLift, requireAuth)
	app.Post("/api/v1/admin/accounts/{id}/unsensitive", apiHandler.HandleAdminUnsensitiveAccountLift, requireAuth)

	// Report management (Admin/Moderator)
	app.Get("/api/v1/admin/reports", apiHandler.HandleAdminGetReportsLift, requireAuth)
	app.Get("/api/v1/admin/reports/{id}", apiHandler.HandleAdminGetReportLift, requireAuth)
	app.Post("/api/v1/admin/reports/{id}/resolve", apiHandler.HandleAdminResolveReportLift, requireAuth)
	app.Post("/api/v1/admin/reports/{id}/reopen", apiHandler.HandleAdminReopenReportLift, requireAuth)
	app.Post("/api/v1/admin/reports/{id}/assign_to_self", apiHandler.HandleAdminAssignReportLift, requireAuth)
	app.Post("/api/v1/admin/reports/{id}/unassign", apiHandler.HandleAdminUnassignReportLift, requireAuth)

	// Status moderation (Admin only for deletion, Admin/Moderator for sensitivity)
	app.Get("/api/v1/admin/statuses", apiHandler.HandleAdminGetStatusesLift, requireAuth)
	app.Get("/api/v1/admin/statuses/{id}", apiHandler.HandleAdminGetStatusLift, requireAuth)
	app.Delete("/api/v1/admin/statuses/{id}", apiHandler.HandleAdminDeleteStatusLift, requireAuth)
	app.Post("/api/v1/admin/statuses/{id}/sensitive", apiHandler.HandleAdminMarkStatusSensitiveLift, requireAuth)
	app.Post("/api/v1/admin/statuses/{id}/unsensitive", apiHandler.HandleAdminUnmarkStatusSensitiveLift, requireAuth)

	// Agent governance (Admin only)
	app.Get("/api/v1/admin/agents/policy", apiHandler.HandleAdminGetAgentPolicyLift, requireAdmin)
	app.Put("/api/v1/admin/agents/policy", apiHandler.HandleAdminUpdateAgentPolicyLift, requireAdmin)
	app.Post("/api/v1/admin/agents/{username}/unlock", apiHandler.HandleAdminUnlockAgentLift, requireAdmin)
	app.Post("/api/v1/admin/agents/{username}/verify", apiHandler.HandleAdminVerifyAgentLift, requireAdmin)
	app.Post("/api/v1/admin/agents/{username}/unverify", apiHandler.HandleAdminUnverifyAgentLift, requireAdmin)

	// Soul governance (Admin only)
	app.Put("/api/v1/admin/soul/well-known", apiHandler.HandleAdminSetSoulWellKnownProofLift, requireAdmin)

	// Canonical skill authority administration (admin scope plus local admin role)
	app.Get("/api/v1/admin/skills/proposals", apiHandler.HandleAdminListSkillProposalsLift, requireAdminRead)
	app.Get("/api/v1/admin/skills/proposals/{proposalId}", apiHandler.HandleAdminGetSkillProposalLift, requireAdminRead)
	app.Get("/api/v1/admin/skills/{skillId}/assignments", apiHandler.HandleAdminListSkillAssignmentsLift, requireAdminRead)
	app.Post("/api/v1/admin/skills/{skillId}/revisions/{revisionNumber}/approve", apiHandler.HandleAdminApproveSkillRevisionLift, requireAdminWrite)
	app.Post("/api/v1/admin/skills/{skillId}/revisions/{revisionNumber}/revoke", apiHandler.HandleAdminRevokeSkillRevisionLift, requireAdminWrite)
	app.Post("/api/v1/admin/skills/{skillId}/assignments", apiHandler.HandleAdminCreateSkillAssignmentLift, requireAdminWrite)
	app.Post("/api/v1/admin/skills/{skillId}/assignments/{assignmentId}/revoke", apiHandler.HandleAdminRevokeSkillAssignmentLift, requireAdminWrite)

	// Domain blocks (Admin only)
	app.Get("/api/v1/admin/domain_blocks", apiHandler.HandleGetAdminDomainBlocksLift, requireAuth)
	app.Get("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleGetAdminDomainBlockLift, requireAuth)
	app.Post("/api/v1/admin/domain_blocks", apiHandler.HandleCreateAdminDomainBlockLift, requireAuth)
	app.Put("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleUpdateAdminDomainBlockLift, requireAuth)
	app.Delete("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleDeleteAdminDomainBlockLift, requireAuth)

	// Domain allows (Admin only)
	app.Get("/api/v1/admin/domain_allows", apiHandler.HandleGetAdminDomainAllowsLift, requireAuth)
	app.Post("/api/v1/admin/domain_allows", apiHandler.HandleCreateAdminDomainAllowLift, requireAuth)
	app.Delete("/api/v1/admin/domain_allows/{id}", apiHandler.HandleDeleteAdminDomainAllowLift, requireAuth)

	// Email domain blocks (Admin only)
	app.Get("/api/v1/admin/email_domain_blocks", apiHandler.HandleGetEmailDomainBlocksLift, requireAuth)
	app.Post("/api/v1/admin/email_domain_blocks", apiHandler.HandleCreateEmailDomainBlockLift, requireAuth)
	app.Delete("/api/v1/admin/email_domain_blocks/{id}", apiHandler.HandleDeleteEmailDomainBlockLift, requireAuth)

	// Federation (Admin only)
	app.Get("/api/v1/admin/federation/instances", apiHandler.HandleGetFederationInstancesLift, requireAuth)
	app.Get("/api/v1/admin/federation/instance/{domain}", apiHandler.HandleGetFederationInstanceLift, requireAuth)
	app.Get("/api/v1/admin/federation/statistics", apiHandler.HandleGetFederationStatisticsLift, requireAuth)

	// Announcements (Admin only)
	app.Post("/api/v1/admin/announcements", apiHandler.HandleCreateAnnouncementLift, requireAuth)

	// Custom emojis (Admin only)
	app.Post("/api/v1/admin/custom_emojis", apiHandler.HandleCreateCustomEmojiLift, requireAuth)
	app.Put("/api/v1/admin/custom_emojis/{shortcode}", apiHandler.HandleUpdateCustomEmojiLift, requireAuth)
	app.Delete("/api/v1/admin/custom_emojis/{shortcode}", apiHandler.HandleDeleteCustomEmojiLift, requireAuth)

	// Moderation overview and events (Admin/Moderator)
	app.Get("/api/v1/admin/moderation/overview", apiHandler.HandleAdminModerationOverviewLift, requireAuth)
	app.Get("/api/v1/admin/moderation/events", apiHandler.HandleAdminGetModerationEventsLift, requireAuth)
	app.Post("/api/v1/admin/moderation/events/{id}/override", apiHandler.HandleAdminOverrideModerationEventLift, requireAuth)

	// Trust graph management (Admin only)
	app.Get("/api/v1/admin/moderation/trust/graph", apiHandler.HandleAdminGetTrustGraphLift, requireAuth)
	app.Put("/api/v1/admin/moderation/trust/{from}/{to}", apiHandler.HandleAdminUpdateTrustLift, requireAuth)

	// Search endpoints with privacy enforcement (always enabled)
	// Account search is rate limited (scraping prevention)
	app.Get("/api/v1/accounts/search", ratelimit.ApplyRateLimit(
		apiHandler.HandleAccountSearchLift,
		30, 5*time.Minute, logger), optionalAuth)
	// Suggestions are NOT rate limited
	app.Get("/api/v1/accounts/search/suggestions", apiHandler.HandleGetSearchSuggestionsLift)
	// Status search is rate limited (GET and POST)
	app.Get("/api/v1/search/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleStatusSearchLift,
		30, 5*time.Minute, logger), requireRead)
	app.Post("/api/v1/search/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleStatusSearchLift,
		30, 5*time.Minute, logger), requireRead)

	// Relationship interactions
	app.Post("/api/v1/accounts/{id}/follow", ratelimit.ApplyRateLimit(
		apiHandler.HandleFollowLift,
		30, 5*time.Minute, logger), requireFollowWrite)
	app.Post("/api/v1/accounts/{id}/unfollow", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnfollowLift,
		30, 5*time.Minute, logger), requireFollowWrite)
	app.Post("/api/v1/accounts/{id}/block", ratelimit.ApplyRateLimit(
		apiHandler.HandleBlockLift,
		30, 5*time.Minute, logger), requireBlockWrite)
	app.Post("/api/v1/accounts/{id}/unblock", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnblockLift,
		30, 5*time.Minute, logger), requireBlockWrite)
	app.Post("/api/v1/accounts/{id}/mute", ratelimit.ApplyRateLimit(
		apiHandler.HandleMuteAccountLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Post("/api/v1/accounts/{id}/unmute", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnmuteAccountLift,
		30, 5*time.Minute, logger), requireWrite)
	app.Get("/api/v1/blocks", ratelimit.ApplyRateLimit(
		apiHandler.HandleGetBlocksLift,
		60, time.Hour, logger), requireBlockRead)
	app.Get("/api/v1/mutes", ratelimit.ApplyRateLimit(
		apiHandler.HandleGetMutedAccountsLift,
		60, time.Hour, logger), requireRead)

	// Reviewer management (Admin only)
	app.Get("/api/v1/admin/moderation/reviewers", apiHandler.HandleAdminGetReviewersLift, requireAuth)
	app.Post("/api/v1/admin/moderation/reviewers/{id}/promote", apiHandler.HandleAdminPromoteModeratorLift, requireAuth)
	app.Post("/api/v1/admin/moderation/reviewers/{id}/demote", apiHandler.HandleAdminDemoteModeratorLift, requireAuth)

	// Media endpoints - V1 (synchronous) and V2 (asynchronous)
	// V1 Media endpoints (backwards compatibility)
	// POST is rate limited (storage abuse prevention)
	app.Post("/api/v1/media", ratelimit.ApplyRateLimit(
		apiHandler.HandleUploadMediaLift,
		20, time.Hour, logger), requireWrite)
	// GET and PUT are NOT rate limited
	app.Get("/api/v1/media/{id}", apiHandler.HandleGetMediaLift, requireRead)
	app.Put("/api/v1/media/{id}", apiHandler.HandleUpdateMediaLift, requireWrite)

	// Note: V2 media endpoints have been consolidated into main media handlers

	// Conversation endpoints (Direct Messages) - always enabled for 100% Mastodon API compatibility
	app.Get("/api/v1/conversations", apiHandler.HandleGetConversationsLift, requireRead)
	app.Delete("/api/v1/conversations/{id}", apiHandler.HandleDeleteConversationLift, requireWrite)
	app.Post("/api/v1/conversations/{id}/read", apiHandler.HandleMarkConversationReadLift, requireWrite)

	// Instance endpoints
	app.Get("/api/v1/instance", apiHandler.HandleGetInstanceV1Lift)
	app.Get("/api/v1/instance/peers", apiHandler.HandleGetInstancePeersLift)
	app.Get("/api/v1/instance/activity", apiHandler.HandleGetInstanceActivityLift)
	app.Get("/api/v1/instance/domain_blocks", apiHandler.HandleGetInstanceDomainBlocksLift)
	app.Get("/api/v1/instance/translation_languages", apiHandler.HandleGetTranslationLanguagesLift)

	// API v2 endpoints - Enhanced Mastodon compatibility
	app.Get("/api/v2/instance", apiHandler.HandleGetInstanceV2Lift)
	app.Get("/api/v2/search", apiHandler.HandleSearchV2Lift, optionalAuth)
	app.Get("/api/v2/suggestions", apiHandler.HandleGetSuggestionsV2Lift, requireAuth)

	// API v2 filters (advanced filtering) - existing implementations
	app.Get("/api/v2/filters", apiHandler.HandleGetFiltersLift, requireFilterRead)
	app.Get("/api/v2/filters/{id}", apiHandler.HandleGetFilterLift, requireFilterRead)
	app.Post("/api/v2/filters", apiHandler.HandleCreateFilterLift, requireFilterWrite)
	app.Put("/api/v2/filters/{id}", apiHandler.HandleUpdateFilterLift, requireFilterWrite)
	app.Delete("/api/v2/filters/{id}", apiHandler.HandleDeleteFilterLift, requireFilterWrite)

	// API v2 filter keywords and statuses
	app.Get("/api/v2/filters/{filter_id}/keywords", apiHandler.HandleGetFilterKeywordsLift, requireFilterRead)
	app.Post("/api/v2/filters/{filter_id}/keywords", apiHandler.HandleAddFilterKeywordLift, requireFilterWrite)
	app.Delete("/api/v2/filters/{filter_id}/keywords/{keyword_id}", apiHandler.HandleDeleteFilterKeywordLift, requireFilterWrite)
	app.Get("/api/v2/filters/{filter_id}/statuses", apiHandler.HandleGetFilterStatusesLift, requireFilterRead)
	app.Post("/api/v2/filters/{filter_id}/statuses", apiHandler.HandleAddFilterStatusLift, requireFilterWrite)
	app.Delete("/api/v2/filters/{filter_id}/statuses/{status_id}", apiHandler.HandleDeleteFilterStatusLift, requireFilterWrite)

	// API v2 trends endpoints - Enhanced trending with metadata
	app.Get("/api/v2/trends", apiHandler.HandleGetTrendsV2Lift)
	app.Get("/api/v2/trends/tags", apiHandler.HandleGetTrendingTagsV2Lift)
	app.Get("/api/v2/trends/statuses", apiHandler.HandleGetTrendingStatusesV2Lift)
	app.Get("/api/v2/trends/links", apiHandler.HandleGetTrendingLinksV2Lift)

	// API v2 filter testing endpoint
	app.Post("/api/v2/filters/test", apiHandler.HandleTestFilterLift, requireFilterRead)

	// API v2 grouped notifications endpoints
	app.Get("/api/v2/notifications/grouped", apiHandler.HandleGetGroupedNotificationsLift, requireNotificationRead)
	app.Post("/api/v2/notifications/groups/{group_id}/read", apiHandler.HandleMarkGroupAsReadLift, requireNotificationWrite)

	// Quote posts API endpoints
	// POST create quote is rate limited (spam prevention)
	app.Post("/api/v1/statuses/{id}/quote", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateQuotePostLift,
		30, time.Hour, logger), requireStatusWrite)
	// GETs, DELETE, and PUT are NOT rate limited
	app.Get("/api/v1/statuses/{id}/quotes", apiHandler.HandleGetQuotesOfStatusLift)
	app.Delete("/api/v1/statuses/{id}/quote/{quote_id}", apiHandler.HandleDeleteQuotePostLift, requireStatusWrite)
	app.Get("/api/v1/accounts/{id}/quote_permissions", apiHandler.HandleGetQuotePermissionsLift)
	app.Put("/api/v1/accounts/quote_permissions", apiHandler.HandleUpdateQuotePermissionsLift, requireAccountWrite)
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
