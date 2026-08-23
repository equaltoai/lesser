package main

import (
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/crawler"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

// requireAnySecureScope preserves the Mastodon-compatible any-of scope aliases
// that predate SecureApp. SecureApp's Authenticated posture intentionally models
// all-of scopes, so these routes use Authenticated() for the identity gate and
// retain their narrower any-of authorization at the handler boundary.
func requireAnySecureScope(handler apptheory.Handler, scopes ...string) apptheory.Handler {
	return func(ctx *apptheory.Context) (*apptheory.Response, error) {
		if ctx != nil && ctx.AuthPrincipal != nil {
			for _, required := range scopes {
				for _, granted := range ctx.AuthPrincipal.Scopes {
					if required != "" && granted == required {
						return handler(ctx)
					}
				}
			}
		}

		return nil, apptheory.NewAppTheoryError("app.forbidden", "forbidden").WithStatusCode(403)
	}
}

func configureRoutes(app *apptheory.SecureApp) {
	app.Get("/", func(*apptheory.Context) (*apptheory.Response, error) {
		return redirectResponse("/l/", false), nil
	}, apptheory.Public())
	app.Handle("HEAD", "/", func(*apptheory.Context) (*apptheory.Response, error) {
		return redirectResponse("/l/", false), nil
	}, apptheory.Public())
	app.Get("/robots.txt", crawler.RobotsHandler, apptheory.Public())

	// OAuth app registration (public, no auth required)
	app.Post("/api/v1/apps", apiHandler.HandleAppRegistrationLift, apptheory.Optional())
	app.Post("/api/v1/apps/{id}/rotate_secret", requireAnySecureScope(apiHandler.HandleAppRotateSecretLift, auth.ScopeWrite, auth.ScopeAdmin), apptheory.Authenticated())

	// Wallet authentication endpoints (public, for passwordless login)
	app.Post("/auth/wallet/challenge", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateChallengeLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/auth/wallet/verify", ratelimit.ApplyRateLimit(
		apiHandler.HandleVerifySignatureLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/auth/wallet/login", ratelimit.ApplyRateLimit(
		apiHandler.HandleLoginWalletLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/auth/wallet/link", apiHandler.HandleLinkWalletLift, apptheory.Optional())
	app.Delete("/auth/wallet/unlink/{address}", apiHandler.HandleUnlinkWalletLift, apptheory.Authenticated())
	app.Get("/auth/wallet/list", apiHandler.HandleGetWalletsLift, apptheory.Authenticated())
	app.Get("/auth/device", apiHandler.HandleOAuthDevicePageLift, apptheory.Public())

	// Real CORS preflights are handled centrally by AppTheory when WithCORS is enabled.

	// WebAuthn (passkey) endpoints
	// Registration begin/finish requires auth (binds a passkey to the logged-in user).
	app.Post("/api/v1/auth/webauthn/register/begin", ratelimit.ApplyRateLimit(
		apiHandler.HandleBeginWebAuthnRegistrationLift,
		20, 5*time.Minute, logger), apptheory.Authenticated())
	app.Post("/api/v1/auth/webauthn/register/finish", ratelimit.ApplyRateLimit(
		apiHandler.HandleFinishWebAuthnRegistrationLift,
		20, 5*time.Minute, logger), apptheory.Authenticated())
	// Login begin/finish is public (username provided), but rate limited.
	app.Post("/api/v1/auth/webauthn/login/begin", ratelimit.ApplyRateLimit(
		apiHandler.HandleBeginWebAuthnLoginLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/api/v1/auth/webauthn/login/finish", ratelimit.ApplyRateLimit(
		apiHandler.HandleFinishWebAuthnLoginLift,
		20, 5*time.Minute, logger), apptheory.Public())
	// Signup begin/finish is public (username provided), but rate limited.
	app.Post("/api/v1/auth/webauthn/signup/begin", ratelimit.ApplyRateLimit(
		apiHandler.HandleBeginWebAuthnSignupLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/api/v1/auth/webauthn/signup/finish", ratelimit.ApplyRateLimit(
		apiHandler.HandleFinishWebAuthnSignupLift,
		20, 5*time.Minute, logger), apptheory.Public())

	app.Get("/api/v1/auth/webauthn/credentials", apiHandler.HandleListWebAuthnCredentialsLift, apptheory.Authenticated())
	app.Delete("/api/v1/auth/webauthn/credentials/{credentialId}", apiHandler.HandleDeleteWebAuthnCredentialLift, apptheory.Authenticated())
	app.Put("/api/v1/auth/webauthn/credentials/{credentialId}", apiHandler.HandleUpdateWebAuthnCredentialNameLift, apptheory.Authenticated())

	// OAuth endpoints with native Lift implementation + rate limiting
	// AppTheory's RFC 8414 primitive advertises the conventional root paths.
	// Keep the historical /oauth/* paths below as compatibility aliases.
	app.Get("/authorize", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthAuthorizeLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/register", ratelimit.ApplyOAuthRegistrationRateLimit(
		apiHandler.HandleOAuthDynamicClientRegistrationLift,
		20, time.Minute, logger), apptheory.Optional())
	app.Post("/token", ratelimit.ApplyOAuthTokenRateLimit(
		apiHandler.HandleOAuthTokenLift,
		10, time.Minute, logger), apptheory.Public())
	app.Get("/oauth/authorize", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthAuthorizeLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/oauth/consent", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthConsentLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/oauth/device/code", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthDeviceCodeLift,
		10, time.Minute, logger), apptheory.Public())
	app.Post("/oauth/device/verify", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthDeviceVerifyLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/oauth/device/consent", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthDeviceConsentLift,
		20, 5*time.Minute, logger), apptheory.Authenticated())
	app.Post("/oauth/register", ratelimit.ApplyOAuthRegistrationRateLimit(
		apiHandler.HandleOAuthDynamicClientRegistrationLift,
		20, time.Minute, logger), apptheory.Optional())
	app.Post("/oauth/token", ratelimit.ApplyOAuthTokenRateLimit(
		apiHandler.HandleOAuthTokenLift,
		10, time.Minute, logger), apptheory.Public())
	app.Post("/oauth/revoke", ratelimit.ApplyRateLimit(
		apiHandler.HandleOAuthRevokeLift,
		10, time.Minute, logger), apptheory.Public())
	app.Get("/.well-known/oauth-authorization-server", apiHandler.HandleOAuthAuthorizationServerMetadataLift, apptheory.Public())
	app.Get("/.well-known/oauth-protected-resource/mcp/{username}", apiHandler.HandleOAuthProtectedResourceMetadataLift, apptheory.Public())

	// NodeInfo endpoints with native Lift implementation
	app.Get("/.well-known/nodeinfo", apiHandler.HandleNodeInfoWellKnownLift, apptheory.Public())
	// lesser-soul HTTPS proof
	app.Get("/.well-known/lesser-soul-agent", apiHandler.HandleWellKnownLesserSoulAgentLift, apptheory.Public())
	app.Get("/nodeinfo/2.0", apiHandler.HandleNodeInfoLift, apptheory.Public())

	// Reputation keys (used by the portable reputation system)
	app.Get("/.well-known/reputation-keys", apiHandler.HandleGetReputationKeysLift, apptheory.Public())

	// oEmbed + embed endpoints
	app.Get("/api/oembed", apiHandler.HandleOEmbedLift, apptheory.Public())
	app.Get("/embed/{id}", apiHandler.HandleEmbedPageLift, apptheory.Public())

	// Instance setup endpoints (locked-by-default bootstrapping)
	app.Get("/setup/status", apiHandler.HandleSetupStatusLift, apptheory.Public())
	app.Post("/setup/bootstrap/challenge", ratelimit.ApplyRateLimit(
		apiHandler.HandleSetupBootstrapChallengeLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/setup/bootstrap/verify", ratelimit.ApplyRateLimit(
		apiHandler.HandleSetupBootstrapVerifyLift,
		20, 5*time.Minute, logger), apptheory.Public())
	app.Post("/setup/admin", apiHandler.HandleSetupCreateAdminLift, apptheory.Public())
	app.Post("/setup/finalize", apiHandler.HandleSetupFinalizeLift, apptheory.Public())

	// Account verification/update endpoints with native Lift implementation
	// verify_credentials is NOT rate limited (read-only)
	app.Get("/api/v1/accounts/verify_credentials", apiHandler.HandleVerifyCredentialsLift, apptheory.Authenticated(auth.ScopeRead))
	// update_credentials IS rate limited
	app.Patch("/api/v1/accounts/update_credentials", ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdateCredentialsLift,
		10, time.Hour, logger), apptheory.Authenticated(auth.ScopeWrite))
	// account registration
	app.Post("/api/v1/accounts", ratelimit.ApplyRateLimit(
		apiHandler.HandleRegistrationLift,
		10, time.Hour, logger), apptheory.Public())

	// Agent endpoints (LLM agent support)
	app.Get("/api/v1/agents", apiHandler.HandleListAgentsLift, apptheory.Public())
	app.Post("/api/v1/agents/delegate", requireAnySecureScope(apiHandler.HandleDelegateAgentLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/agents/{username}/access-leases/challenge/principal", requireAnySecureScope(apiHandler.HandleCreateAgentAccessLeasePrincipalChallengeLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/agents/{username}/access-leases/challenge/agent", requireAnySecureScope(apiHandler.HandleCreateAgentAccessLeaseAgentChallengeLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/agents/{username}/access-leases", requireAnySecureScope(apiHandler.HandleCreateAgentAccessLeaseLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Get("/api/v1/agents/{username}/access-leases", requireAnySecureScope(apiHandler.HandleListAgentAccessLeasesLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/revoke", requireAnySecureScope(apiHandler.HandleRevokeAgentAccessLeaseLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/session-key/challenge", apiHandler.HandleCreateAgentAccessLeaseSessionKeyChallengeLift, apptheory.Public())
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/session-key", apiHandler.HandleAuthorizeAgentAccessLeaseSessionKeyLift, apptheory.Public())
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/renew/challenge", apiHandler.HandleCreateAgentAccessLeaseRenewChallengeLift, apptheory.Public())
	app.Post("/api/v1/agents/{username}/access-leases/{leaseID}/token", apiHandler.HandleExchangeAgentAccessLeaseTokenLift, apptheory.Public())
	app.Get("/api/v1/agents/{username}/runtime-sessions", requireAnySecureScope(apiHandler.HandleListAgentRuntimeSessionsLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/agents/{username}/runtime-sessions/{sessionID}/revoke", requireAnySecureScope(apiHandler.HandleRevokeAgentRuntimeSessionLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/agents/register/challenge", ratelimit.ApplyOAuthRegistrationRateLimit(
		apiHandler.HandleAgentRegisterChallengeLift,
		20, time.Minute, logger), apptheory.Public())
	app.Post("/api/v1/agents/register", ratelimit.ApplyOAuthRegistrationRateLimit(
		apiHandler.HandleAgentRegisterLift,
		20, time.Minute, logger), apptheory.Public())
	app.Post("/api/v1/agents/auth/challenge", apiHandler.HandleAgentAuthChallengeLift, apptheory.Public())
	app.Post("/api/v1/agents/auth/token", apiHandler.HandleAgentAuthTokenLift, apptheory.Public())
	app.Get("/api/v1/agents/shared-with-me", apiHandler.HandleListAgentsSharedWithMeLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/agents/{username}/share", requireAnySecureScope(apiHandler.HandleListAgentSharesLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Put("/api/v1/agents/{username}/share/{grantee}", requireAnySecureScope(apiHandler.HandleGrantAgentShareLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Delete("/api/v1/agents/{username}/share/{grantee}", requireAnySecureScope(apiHandler.HandleRevokeAgentShareLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Get("/api/v1/agents/{username}/access", apiHandler.HandleGetAgentAccessLift, apptheory.Authenticated())
	app.Get("/api/v1/agents/{username}", apiHandler.HandleGetAgentLift, apptheory.Public())
	app.Patch("/api/v1/agents/{username}", requireAnySecureScope(apiHandler.HandleUpdateAgentLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Delete("/api/v1/agents/{username}", requireAnySecureScope(apiHandler.HandleDeleteAgentLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())
	app.Get("/api/v1/agents/{username}/activity", apiHandler.HandleGetAgentActivityLift, apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/agents/{username}/rotate-key/challenge", apiHandler.HandleAgentRotateKeyChallengeLift, apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/agents/{username}/rotate-key", apiHandler.HandleAgentRotateKeyLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Get("/api/v1/agents/memory/search", apiHandler.HandleAgentMemorySearchLift, apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/agents/memory/search", apiHandler.HandleAgentMemorySearchLift, apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/agents/{username}/suspend", apiHandler.HandleSuspendAgentLift, apptheory.Authenticated(auth.ScopeAdmin))

	app.Get("/api/v1/accounts/{id}/followers", apiHandler.HandleGetAccountFollowersLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/accounts/{id}/following", apiHandler.HandleGetAccountFollowingLift, apptheory.Authenticated(auth.ScopeRead))

	// Relationships endpoint with native Lift implementation
	app.Get("/api/v1/accounts/relationships", requireAnySecureScope(apiHandler.HandleGetRelationshipsLift, auth.ReadFollows, auth.ScopeRead), apptheory.Authenticated())

	// Data exports with native Lift implementation
	// POST is rate limited (expensive operation)
	app.Post("/api/v1/exports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateExportLift,
		5, 24*time.Hour, logger), apptheory.Authenticated(auth.ScopeRead))
	// GETs are NOT rate limited (read-only)
	app.Get("/api/v1/exports/{id}", apiHandler.HandleGetExportStatusLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/exports/{id}/download", apiHandler.HandleDownloadExportLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/exports", apiHandler.HandleListExportsLift, apptheory.Authenticated(auth.ScopeRead))

	// Data imports with native Lift implementation
	// POST is rate limited (expensive operation)
	app.Post("/api/v1/imports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateImportLift,
		5, 24*time.Hour, logger), apptheory.Authenticated(auth.ScopeWrite))
	// GETs and DELETE are NOT rate limited
	app.Get("/api/v1/imports/{id}", requireAnySecureScope(apiHandler.HandleGetImportStatusLift, auth.ScopeRead, auth.ScopeWrite), apptheory.Authenticated())
	app.Delete("/api/v1/imports/{id}", apiHandler.HandleCancelImportLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Get("/api/v1/imports", requireAnySecureScope(apiHandler.HandleListImportsLift, auth.ScopeRead, auth.ScopeWrite), apptheory.Authenticated())

	// Community Notes endpoints with native Lift implementation
	// POST create note is rate limited
	app.Post("/api/v1/notes", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateNoteLift,
		20, time.Hour, logger), apptheory.Authenticated())
	// GETs are NOT rate limited
	app.Get("/api/v1/notes/{object_id}", apiHandler.HandleGetNotesLift, apptheory.Optional())
	// POST vote is rate limited
	app.Post("/api/v1/notes/{id}/vote", ratelimit.ApplyRateLimit(
		apiHandler.HandleVoteNoteLift,
		100, time.Hour, logger), apptheory.Authenticated())
	app.Get("/api/v1/accounts/{id}/notes", apiHandler.HandleGetUserNotesLift, apptheory.Public())

	// Status endpoints (Mastodon parity)
	app.Post("/api/v1/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateStatusLift,
		300, time.Hour, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Put("/api/v1/statuses/{id}", ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdateStatusLift,
		300, time.Hour, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Delete("/api/v1/statuses/{id}", ratelimit.ApplyRateLimit(
		apiHandler.HandleDeleteStatusLift,
		300, time.Hour, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Get("/api/v1/statuses/{id}", apiHandler.HandleGetStatusLift, apptheory.Optional())
	app.Get("/api/v1/statuses/{id}/context", apiHandler.HandleGetStatusContextLift, apptheory.Optional())
	app.Get("/api/v1/statuses/{id}/history", apiHandler.HandleGetStatusHistoryLift, apptheory.Optional())
	app.Get("/api/v1/statuses/{id}/source", apiHandler.HandleGetStatusSourceLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/statuses/{id}/favourited_by", apiHandler.HandleGetStatusFavouritedByLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/statuses/{id}/reblogged_by", apiHandler.HandleGetStatusRebloggedByLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/accounts/{id}/statuses", apiHandler.HandleGetAccountStatusesLift, apptheory.Optional())

	// Additional Mastodon parity endpoints

	// Timelines
	app.Get("/api/v1/timelines/home", apiHandler.HandleGetHomeTimelineLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/timelines/public", apiHandler.HandleGetPublicTimelineLift, apptheory.Optional())
	app.Get("/api/v1/timelines/tag/{hashtag}", apiHandler.HandleGetTagTimelineLift, apptheory.Optional())
	app.Get("/api/v1/timelines/list/{list_id}", apiHandler.HandleGetListTimelineLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/timelines/direct", apiHandler.HandleGetDirectTimelineLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/timelines/link", apiHandler.HandleGetLinkTimelineLift, apptheory.Public())

	// Trends (Mastodon v1)
	app.Get("/api/v1/trends", apiHandler.HandleGetTrendsLift, apptheory.Public())
	app.Get("/api/v1/trends/tags", apiHandler.HandleGetTrendingTagsLift, apptheory.Public())
	app.Get("/api/v1/trends/statuses", apiHandler.HandleGetTrendingStatusesLift, apptheory.Public())
	app.Get("/api/v1/trends/links", apiHandler.HandleGetTrendingLinksLift, apptheory.Public())

	// Status interactions
	app.Post("/api/v1/statuses/{id}/favourite", ratelimit.ApplyRateLimit(
		apiHandler.HandleFavoriteLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/unfavourite", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnfavoriteLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/reblog", ratelimit.ApplyRateLimit(
		apiHandler.HandleReblogLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/unreblog", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnreblogLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/bookmark", ratelimit.ApplyRateLimit(
		apiHandler.HandleBookmarkLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/unbookmark", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnbookmarkLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/pin", ratelimit.ApplyRateLimit(
		apiHandler.HandlePinStatusLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/unpin", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnpinStatusLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/mute", ratelimit.ApplyRateLimit(
		apiHandler.HandleMuteConversationLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/unmute", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnmuteConversationLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/statuses/{id}/translate", ratelimit.ApplyRateLimit(
		apiHandler.HandleTranslateStatusLift,
		20, time.Hour, logger), apptheory.Authenticated())

	// Bookmarks + favourites
	app.Get("/api/v1/bookmarks", apiHandler.HandleGetBookmarksLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/favourites", apiHandler.HandleGetFavouritesLift, apptheory.Authenticated(auth.ScopeRead))

	// Lists
	app.Get("/api/v1/lists", apiHandler.HandleGetListsLift, apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/lists", apiHandler.HandleCreateListLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Get("/api/v1/lists/{id}", apiHandler.HandleGetListLift, apptheory.Authenticated(auth.ScopeRead))
	app.Put("/api/v1/lists/{id}", apiHandler.HandleUpdateListLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Delete("/api/v1/lists/{id}", apiHandler.HandleDeleteListLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Get("/api/v1/lists/{id}/accounts", apiHandler.HandleGetListAccountsLift, apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/lists/{id}/accounts", apiHandler.HandleAddAccountsToListLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Delete("/api/v1/lists/{id}/accounts", apiHandler.HandleRemoveAccountsFromListLift, apptheory.Authenticated(auth.ScopeWrite))

	// Notifications
	app.Get("/api/v1/notifications", requireAnySecureScope(apiHandler.HandleGetNotificationsLift, auth.ReadNotifications, auth.ScopeRead), apptheory.Authenticated())
	app.Get("/api/v1/notifications/{id}", requireAnySecureScope(apiHandler.HandleGetNotificationLift, auth.ReadNotifications, auth.ScopeRead), apptheory.Authenticated())
	app.Post("/api/v1/notifications/deliver", apiHandler.HandleDeliverNotificationLift, apptheory.InternalOnly())
	app.Post("/api/v1/notifications/clear", requireAnySecureScope(apiHandler.HandleClearNotificationsLift, auth.WriteNotifications, auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/notifications/{id}/dismiss", requireAnySecureScope(apiHandler.HandleDismissNotificationLift, auth.WriteNotifications, auth.ScopeWrite), apptheory.Authenticated())

	// Preferences + markers
	app.Get("/api/v1/preferences", apiHandler.HandleGetPreferencesLift, apptheory.Authenticated(auth.ScopeRead))
	app.Patch("/api/v1/preferences", apiHandler.HandleUpdatePreferencesLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Get("/api/v1/markers", apiHandler.HandleGetMarkersLift, apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/markers", apiHandler.HandleSaveMarkersLift, apptheory.Authenticated(auth.ScopeWrite))

	// Push subscriptions
	app.Get("/api/v1/push/subscription", requireAnySecureScope(apiHandler.HandleGetPushSubscriptionLift, auth.ScopePush, auth.ScopeRead), apptheory.Authenticated())
	app.Post("/api/v1/push/subscription", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleCreatePushSubscriptionLift,
		20, time.Hour, logger), auth.ScopePush, auth.ScopeWrite), apptheory.Authenticated())
	app.Put("/api/v1/push/subscription", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleUpdatePushSubscriptionLift,
		20, time.Hour, logger), auth.ScopePush, auth.ScopeWrite), apptheory.Authenticated())
	app.Delete("/api/v1/push/subscription", requireAnySecureScope(apiHandler.HandleDeletePushSubscriptionLift, auth.ScopePush, auth.ScopeWrite), apptheory.Authenticated())

	// Scheduled statuses
	app.Get("/api/v1/scheduled_statuses", apiHandler.HandleGetScheduledStatusesLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/scheduled_statuses/{id}", apiHandler.HandleGetScheduledStatusLift, apptheory.Authenticated(auth.ScopeRead))
	app.Put("/api/v1/scheduled_statuses/{id}", apiHandler.HandleUpdateScheduledStatusLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Delete("/api/v1/scheduled_statuses/{id}", apiHandler.HandleDeleteScheduledStatusLift, apptheory.Authenticated(auth.ScopeWrite))

	// Follow requests
	app.Get("/api/v1/follow_requests", requireAnySecureScope(apiHandler.HandleGetFollowRequestsLift, auth.ReadFollows, auth.ScopeRead), apptheory.Authenticated())
	app.Post("/api/v1/follow_requests/{account_id}/authorize", requireAnySecureScope(apiHandler.HandleAuthorizeFollowRequestLift, auth.ScopeFollow, auth.WriteFollows, auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/follow_requests/{account_id}/reject", requireAnySecureScope(apiHandler.HandleRejectFollowRequestLift, auth.ScopeFollow, auth.WriteFollows, auth.ScopeWrite), apptheory.Authenticated())

	// Domain blocks (user-level)
	app.Get("/api/v1/domain_blocks", requireAnySecureScope(apiHandler.HandleGetDomainBlocksLift, auth.ReadBlocks, auth.ScopeRead), apptheory.Authenticated())
	app.Post("/api/v1/domain_blocks", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateDomainBlockLift,
		20, time.Hour, logger), auth.WriteBlocks, auth.ScopeWrite), apptheory.Authenticated())
	app.Delete("/api/v1/domain_blocks", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleDeleteDomainBlockLift,
		20, time.Hour, logger), auth.WriteBlocks, auth.ScopeWrite), apptheory.Authenticated())

	// Moderation + reports
	app.Post("/api/v1/moderation/flag", ratelimit.ApplyRateLimit(
		apiHandler.HandleModerationFlagLift,
		30, time.Hour, logger), apptheory.Authenticated())
	app.Get("/api/v1/moderation/queue", apiHandler.HandleModerationQueueLift, apptheory.Authenticated())
	app.Post("/api/v1/moderation/review", ratelimit.ApplyRateLimit(
		apiHandler.HandleModerationReviewLift,
		60, time.Hour, logger), apptheory.Authenticated())
	app.Get("/api/v1/moderation/history/{object_id}", apiHandler.HandleModerationHistoryLift, apptheory.Authenticated())
	app.Get("/api/v1/moderation/consensus/{event_id}", apiHandler.HandleGetConsensusLift, apptheory.Authenticated())
	app.Get("/api/v1/moderation/trust", apiHandler.HandleGetTrustRelationshipsLift, apptheory.Authenticated())
	app.Put("/api/v1/moderation/trust", apiHandler.HandleUpdateTrustLift, apptheory.Authenticated())
	app.Get("/api/v1/moderation/trust/{actor_id}/score", apiHandler.HandleGetTrustScoreLift, apptheory.Authenticated())
	app.Post("/api/v1/reports", ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateReportLift,
		30, time.Hour, logger), apptheory.Authenticated(auth.ScopeWrite))

	// Discovery
	app.Get("/api/v1/directory", apiHandler.HandleGetDirectoryLift, apptheory.Public())
	app.Get("/api/v1/suggestions", apiHandler.HandleGetSuggestionsV1Lift, apptheory.Authenticated())
	app.Delete("/api/v1/suggestions/{account_id}", apiHandler.HandleRemoveSuggestionLift, apptheory.Authenticated())
	app.Get("/api/v1/endorsements", requireAnySecureScope(apiHandler.HandleGetEndorsementsLift, "read:accounts", auth.ScopeRead), apptheory.Authenticated())

	// Announcements
	app.Get("/api/v1/announcements", apiHandler.HandleGetAnnouncementsLift, apptheory.Public())
	app.Post("/api/v1/announcements/{id}/dismiss", apiHandler.HandleDismissAnnouncementLift, apptheory.Authenticated())
	app.Put("/api/v1/announcements/{id}/reactions/{name}", apiHandler.HandleAddAnnouncementReactionLift, apptheory.Authenticated())
	app.Delete("/api/v1/announcements/{id}/reactions/{name}", apiHandler.HandleRemoveAnnouncementReactionLift, apptheory.Authenticated())

	// Custom emojis
	app.Get("/api/v1/custom_emojis", apiHandler.HandleGetCustomEmojisLift, apptheory.Public())

	// Reputation + vouches
	app.Get("/api/v1/reputation/{actor_id}", apiHandler.HandleGetReputationLift, apptheory.Authenticated())
	app.Post("/api/v1/reputation/export", apiHandler.HandleExportReputationLift, apptheory.Authenticated())
	app.Post("/api/v1/reputation/import", apiHandler.HandleImportReputationLift, apptheory.Authenticated())
	app.Post("/api/v1/reputation/verify", apiHandler.HandleVerifyReputationLift, apptheory.Authenticated())
	app.Post("/api/v1/vouches", apiHandler.HandleCreateVouchLift, apptheory.Authenticated())
	app.Get("/api/v1/vouches/{actor_id}", apiHandler.HandleGetVouchesLift, apptheory.Authenticated())
	app.Delete("/api/v1/vouches/{vouch_id}", apiHandler.HandleRevokeVouchLift, apptheory.Authenticated())

	// Canonical skill authority (Lesser-exclusive additive API)
	app.Get("/api/v1/skills", ratelimit.ApplyRateLimit(
		apiHandler.HandleListSkillsLift,
		30, 5*time.Minute, logger), apptheory.Optional())
	app.Get("/api/v1/skills/catalog", ratelimit.ApplyRateLimit(
		apiHandler.HandleListSkillCatalogLift,
		30, 5*time.Minute, logger), apptheory.Optional())
	app.Get("/api/v1/skills/resolve", apiHandler.HandleResolveEffectiveSkillsLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/skills/{skillId}", apiHandler.HandleGetSkillLift, apptheory.Optional())
	app.Get("/api/v1/skills/{skillId}/revisions", apiHandler.HandleListSkillRevisionsLift, apptheory.Optional())
	app.Get("/api/v1/skills/{skillId}/revisions/{revisionNumber}", apiHandler.HandleGetSkillRevisionLift, apptheory.Optional())
	app.Get("/api/v1/skills/{skillId}/revisions/{revisionNumber}/bundle", apiHandler.HandleGetSkillBundleLift, apptheory.Optional())

	// Souls
	app.Get("/api/v1/souls/bound/me", requireAnySecureScope(apiHandler.HandleGetBoundSoulMeLift, auth.ScopeRead, auth.ScopeWrite), apptheory.Authenticated())
	app.Get("/api/v1/souls/bound/me/mint-conversations", apiHandler.HandleListBoundSoulMintConversationsLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/souls/bound/me/mint-conversations/{conversationId}", apiHandler.HandleGetBoundSoulMintConversationLift, apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/souls/bindings", apiHandler.HandleCreateSoulBindingLift, apptheory.Public())
	app.Get("/api/v1/souls/bindings/{agentId}", apiHandler.HandleGetSoulBindingLift, apptheory.Public())
	app.Get("/api/v1/souls/mine", requireAnySecureScope(apiHandler.HandleGetMySoulsLift, auth.ScopeRead, auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/souls/{agentId}/incorporate", apiHandler.HandleIncorporateSoulLift, apptheory.Authenticated(auth.ScopeWrite))

	// lesser-host trust proxy (managed instances)
	// Requires user auth for JSON endpoints and trust media reads
	// (previews/images, renders/thumbnail, renders/snapshot).
	app.Post("/api/v1/trust/previews", apiHandler.HandleTrustCreateLinkPreviewLift, apptheory.Authenticated())
	app.Get("/api/v1/trust/previews/{id}", apiHandler.HandleTrustGetLinkPreviewLift, apptheory.Authenticated())
	app.Get("/api/v1/trust/previews/images/{imageId}", apiHandler.HandleTrustGetLinkPreviewImageLift, apptheory.Authenticated())

	app.Post("/api/v1/trust/publish/jobs", apiHandler.HandleTrustCreatePublishJobLift, apptheory.Authenticated())
	app.Get("/api/v1/trust/publish/jobs/{jobId}", apiHandler.HandleTrustGetPublishJobLift, apptheory.Authenticated())

	app.Post("/api/v1/trust/renders", apiHandler.HandleTrustCreateRenderLift, apptheory.Authenticated())
	app.Get("/api/v1/trust/renders/{renderId}", apiHandler.HandleTrustGetRenderLift, apptheory.Authenticated())
	app.Get("/api/v1/trust/renders/{renderId}/thumbnail", apiHandler.HandleTrustGetRenderThumbnailLift, apptheory.Authenticated())
	app.Get("/api/v1/trust/renders/{renderId}/snapshot", apiHandler.HandleTrustGetRenderSnapshotLift, apptheory.Authenticated())

	app.Post("/api/v1/trust/ai/claims/verify", apiHandler.HandleTrustAIClaimVerifyLift, apptheory.Authenticated())
	app.Get("/api/v1/trust/ai/jobs/{jobId}", apiHandler.HandleTrustGetAIJobLift, apptheory.Authenticated())

	app.Get("/api/v1/trust/jwks.json", apiHandler.HandleTrustJWKSJSONLift, apptheory.Public())
	app.Get("/api/v1/trust/attestations", apiHandler.HandleTrustLookupAttestationLift, apptheory.Public())
	app.Get("/api/v1/trust/attestations/{id}", apiHandler.HandleTrustGetAttestationLift, apptheory.Public())

	// Admin endpoints (always enabled for administration)
	// Note: RBAC is handled within each handler's requireAdminLift() method
	// Account management (Admin only)
	app.Get("/api/v1/admin/accounts", apiHandler.HandleAdminGetAccountsLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/accounts", apiHandler.HandleAdminCreateUserLift, apptheory.Authenticated())
	app.Get("/api/v1/admin/accounts/{id}", apiHandler.HandleAdminGetAccountLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/accounts/{id}/action", apiHandler.HandleAdminAccountActionLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/accounts/{id}/approve", apiHandler.HandleAdminApproveAccountLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/accounts/{id}/reject", apiHandler.HandleAdminRejectAccountLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/accounts/{id}/enable", apiHandler.HandleAdminEnableAccountLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/accounts/{id}/unsilence", apiHandler.HandleAdminUnsilenceAccountLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/accounts/{id}/unsuspend", apiHandler.HandleAdminUnsuspendAccountLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/accounts/{id}/unsensitive", apiHandler.HandleAdminUnsensitiveAccountLift, apptheory.Authenticated())

	// Report management (Admin/Moderator)
	app.Get("/api/v1/admin/reports", apiHandler.HandleAdminGetReportsLift, apptheory.Authenticated())
	app.Get("/api/v1/admin/reports/{id}", apiHandler.HandleAdminGetReportLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/reports/{id}/resolve", apiHandler.HandleAdminResolveReportLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/reports/{id}/reopen", apiHandler.HandleAdminReopenReportLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/reports/{id}/assign_to_self", apiHandler.HandleAdminAssignReportLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/reports/{id}/unassign", apiHandler.HandleAdminUnassignReportLift, apptheory.Authenticated())

	// Status moderation (Admin only for deletion, Admin/Moderator for sensitivity)
	app.Get("/api/v1/admin/statuses", apiHandler.HandleAdminGetStatusesLift, apptheory.Authenticated())
	app.Get("/api/v1/admin/statuses/{id}", apiHandler.HandleAdminGetStatusLift, apptheory.Authenticated())
	app.Delete("/api/v1/admin/statuses/{id}", apiHandler.HandleAdminDeleteStatusLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/statuses/{id}/sensitive", apiHandler.HandleAdminMarkStatusSensitiveLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/statuses/{id}/unsensitive", apiHandler.HandleAdminUnmarkStatusSensitiveLift, apptheory.Authenticated())

	// Agent governance (Admin only)
	app.Get("/api/v1/admin/agents/policy", apiHandler.HandleAdminGetAgentPolicyLift, apptheory.Authenticated(auth.ScopeAdmin))
	app.Put("/api/v1/admin/agents/policy", apiHandler.HandleAdminUpdateAgentPolicyLift, apptheory.Authenticated(auth.ScopeAdmin))
	app.Post("/api/v1/admin/agents/{username}/unlock", apiHandler.HandleAdminUnlockAgentLift, apptheory.Authenticated(auth.ScopeAdmin))
	app.Post("/api/v1/admin/agents/{username}/verify", apiHandler.HandleAdminVerifyAgentLift, apptheory.Authenticated(auth.ScopeAdmin))
	app.Post("/api/v1/admin/agents/{username}/unverify", apiHandler.HandleAdminUnverifyAgentLift, apptheory.Authenticated(auth.ScopeAdmin))

	// Soul governance (Admin only)
	app.Put("/api/v1/admin/soul/well-known", apiHandler.HandleAdminSetSoulWellKnownProofLift, apptheory.Authenticated(auth.ScopeAdmin))

	// Canonical skill authority administration (admin scope plus local admin role)
	app.Get("/api/v1/admin/skills/proposals", requireAnySecureScope(apiHandler.HandleAdminListSkillProposalsLift, auth.ScopeAdmin, "admin:read"), apptheory.Authenticated())
	app.Get("/api/v1/admin/skills/proposals/{proposalId}", requireAnySecureScope(apiHandler.HandleAdminGetSkillProposalLift, auth.ScopeAdmin, "admin:read"), apptheory.Authenticated())
	app.Get("/api/v1/admin/skills/{skillId}/assignments", requireAnySecureScope(apiHandler.HandleAdminListSkillAssignmentsLift, auth.ScopeAdmin, "admin:read"), apptheory.Authenticated())
	app.Post("/api/v1/admin/skills/{skillId}/revisions/{revisionNumber}/approve", requireAnySecureScope(apiHandler.HandleAdminApproveSkillRevisionLift, auth.ScopeAdmin, "admin:write"), apptheory.Authenticated())
	app.Post("/api/v1/admin/skills/{skillId}/revisions/{revisionNumber}/revoke", requireAnySecureScope(apiHandler.HandleAdminRevokeSkillRevisionLift, auth.ScopeAdmin, "admin:write"), apptheory.Authenticated())
	app.Post("/api/v1/admin/skills/{skillId}/proposals/{proposalId}/promote", requireAnySecureScope(apiHandler.HandleAdminPromoteSkillProposalLift, auth.ScopeAdmin, "admin:write"), apptheory.Authenticated())
	app.Post("/api/v1/admin/skills/{skillId}/assignments", requireAnySecureScope(apiHandler.HandleAdminCreateSkillAssignmentLift, auth.ScopeAdmin, "admin:write"), apptheory.Authenticated())
	app.Post("/api/v1/admin/skills/{skillId}/assignments/{assignmentId}/revoke", requireAnySecureScope(apiHandler.HandleAdminRevokeSkillAssignmentLift, auth.ScopeAdmin, "admin:write"), apptheory.Authenticated())

	// Domain blocks (Admin only)
	app.Get("/api/v1/admin/domain_blocks", apiHandler.HandleGetAdminDomainBlocksLift, apptheory.Authenticated())
	app.Get("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleGetAdminDomainBlockLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/domain_blocks", apiHandler.HandleCreateAdminDomainBlockLift, apptheory.Authenticated())
	app.Put("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleUpdateAdminDomainBlockLift, apptheory.Authenticated())
	app.Delete("/api/v1/admin/domain_blocks/{id}", apiHandler.HandleDeleteAdminDomainBlockLift, apptheory.Authenticated())

	// Domain allows (Admin only)
	app.Get("/api/v1/admin/domain_allows", apiHandler.HandleGetAdminDomainAllowsLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/domain_allows", apiHandler.HandleCreateAdminDomainAllowLift, apptheory.Authenticated())
	app.Delete("/api/v1/admin/domain_allows/{id}", apiHandler.HandleDeleteAdminDomainAllowLift, apptheory.Authenticated())

	// Email domain blocks (Admin only)
	app.Get("/api/v1/admin/email_domain_blocks", apiHandler.HandleGetEmailDomainBlocksLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/email_domain_blocks", apiHandler.HandleCreateEmailDomainBlockLift, apptheory.Authenticated())
	app.Delete("/api/v1/admin/email_domain_blocks/{id}", apiHandler.HandleDeleteEmailDomainBlockLift, apptheory.Authenticated())

	// Federation (Admin only)
	app.Get("/api/v1/admin/federation/instances", apiHandler.HandleGetFederationInstancesLift, apptheory.Authenticated())
	app.Get("/api/v1/admin/federation/instance/{domain}", apiHandler.HandleGetFederationInstanceLift, apptheory.Authenticated())
	app.Get("/api/v1/admin/federation/statistics", apiHandler.HandleGetFederationStatisticsLift, apptheory.Authenticated())

	// Announcements (Admin only)
	app.Post("/api/v1/admin/announcements", apiHandler.HandleCreateAnnouncementLift, apptheory.Authenticated())

	// Custom emojis (Admin only)
	app.Post("/api/v1/admin/custom_emojis", apiHandler.HandleCreateCustomEmojiLift, apptheory.Authenticated())
	app.Put("/api/v1/admin/custom_emojis/{shortcode}", apiHandler.HandleUpdateCustomEmojiLift, apptheory.Authenticated())
	app.Delete("/api/v1/admin/custom_emojis/{shortcode}", apiHandler.HandleDeleteCustomEmojiLift, apptheory.Authenticated())

	// Moderation overview and events (Admin/Moderator)
	app.Get("/api/v1/admin/moderation/overview", apiHandler.HandleAdminModerationOverviewLift, apptheory.Authenticated())
	app.Get("/api/v1/admin/moderation/events", apiHandler.HandleAdminGetModerationEventsLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/moderation/events/{id}/override", apiHandler.HandleAdminOverrideModerationEventLift, apptheory.Authenticated())

	// Trust graph management (Admin only)
	app.Get("/api/v1/admin/moderation/trust/graph", apiHandler.HandleAdminGetTrustGraphLift, apptheory.Authenticated())
	app.Put("/api/v1/admin/moderation/trust/{from}/{to}", apiHandler.HandleAdminUpdateTrustLift, apptheory.Authenticated())

	// Search endpoints with privacy enforcement (always enabled)
	// Account search is rate limited (scraping prevention)
	app.Get("/api/v1/accounts/search", ratelimit.ApplyRateLimit(
		apiHandler.HandleAccountSearchLift,
		30, 5*time.Minute, logger), apptheory.Optional())
	// Suggestions are NOT rate limited
	app.Get("/api/v1/accounts/search/suggestions", apiHandler.HandleGetSearchSuggestionsLift, apptheory.Public())
	// Status search is rate limited (GET and POST)
	app.Get("/api/v1/search/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleStatusSearchLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeRead))
	app.Post("/api/v1/search/statuses", ratelimit.ApplyRateLimit(
		apiHandler.HandleStatusSearchLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeRead))

	// Relationship interactions
	app.Post("/api/v1/accounts/{id}/follow", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleFollowLift,
		30, 5*time.Minute, logger), auth.ScopeFollow, auth.WriteFollows, auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/accounts/{id}/unfollow", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleUnfollowLift,
		30, 5*time.Minute, logger), auth.ScopeFollow, auth.WriteFollows, auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/accounts/{id}/block", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleBlockLift,
		30, 5*time.Minute, logger), auth.WriteBlocks, auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/accounts/{id}/unblock", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleUnblockLift,
		30, 5*time.Minute, logger), auth.WriteBlocks, auth.ScopeWrite), apptheory.Authenticated())
	app.Post("/api/v1/accounts/{id}/mute", ratelimit.ApplyRateLimit(
		apiHandler.HandleMuteAccountLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/accounts/{id}/unmute", ratelimit.ApplyRateLimit(
		apiHandler.HandleUnmuteAccountLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeWrite))
	app.Get("/api/v1/blocks", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleGetBlocksLift,
		60, time.Hour, logger), auth.ReadBlocks, auth.ScopeRead), apptheory.Authenticated())
	app.Get("/api/v1/mutes", ratelimit.ApplyRateLimit(
		apiHandler.HandleGetMutedAccountsLift,
		60, time.Hour, logger), apptheory.Authenticated(auth.ScopeRead))

	// Reviewer management (Admin only)
	app.Get("/api/v1/admin/moderation/reviewers", apiHandler.HandleAdminGetReviewersLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/moderation/reviewers/{id}/promote", apiHandler.HandleAdminPromoteModeratorLift, apptheory.Authenticated())
	app.Post("/api/v1/admin/moderation/reviewers/{id}/demote", apiHandler.HandleAdminDemoteModeratorLift, apptheory.Authenticated())

	// Media endpoints - V1 (synchronous) and V2 (asynchronous)
	// V1 Media endpoints (backwards compatibility)
	// POST is rate limited (storage abuse prevention)
	app.Post("/api/v1/media", ratelimit.ApplyRateLimit(
		apiHandler.HandleUploadMediaLift,
		20, time.Hour, logger), apptheory.Authenticated(auth.ScopeWrite))
	// GET and PUT are NOT rate limited
	app.Get("/api/v1/media/{id}", apiHandler.HandleGetMediaLift, apptheory.Authenticated(auth.ScopeRead))
	app.Put("/api/v1/media/{id}", apiHandler.HandleUpdateMediaLift, apptheory.Authenticated(auth.ScopeWrite))

	// Note: V2 media endpoints have been consolidated into main media handlers

	// Conversation endpoints (Direct Messages) - always enabled for 100% Mastodon API compatibility
	app.Get("/api/v1/conversations", apiHandler.HandleGetConversationsLift, apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/conversations/lookup", ratelimit.ApplyRateLimit(
		apiHandler.HandleLookupConversationByCounterpartLift,
		30, 5*time.Minute, logger), apptheory.Authenticated(auth.ScopeRead))
	app.Get("/api/v1/conversations/{id}", apiHandler.HandleGetConversationLift, apptheory.Authenticated(auth.ScopeRead))
	app.Delete("/api/v1/conversations/{id}", apiHandler.HandleDeleteConversationLift, apptheory.Authenticated(auth.ScopeWrite))
	app.Post("/api/v1/conversations/{id}/read", apiHandler.HandleMarkConversationReadLift, apptheory.Authenticated(auth.ScopeWrite))

	// Instance endpoints
	app.Get("/api/v1/instance", apiHandler.HandleGetInstanceV1Lift, apptheory.Public())
	app.Get("/api/v1/instance/metrics/daily", apiHandler.HandleGetInstanceMetricsDailyLift, apptheory.Public())
	app.Get("/api/v1/instance/peers", apiHandler.HandleGetInstancePeersLift, apptheory.Public())
	app.Get("/api/v1/instance/activity", apiHandler.HandleGetInstanceActivityLift, apptheory.Public())
	app.Get("/api/v1/instance/domain_blocks", apiHandler.HandleGetInstanceDomainBlocksLift, apptheory.Public())
	app.Get("/api/v1/instance/translation_languages", apiHandler.HandleGetTranslationLanguagesLift, apptheory.Public())

	// API v2 endpoints - Enhanced Mastodon compatibility
	app.Get("/api/v2/instance", apiHandler.HandleGetInstanceV2Lift, apptheory.Public())
	app.Get("/api/v2/search", apiHandler.HandleSearchV2Lift, apptheory.Optional())
	app.Get("/api/v2/suggestions", apiHandler.HandleGetSuggestionsV2Lift, apptheory.Authenticated())

	// API v2 filters (advanced filtering) - existing implementations
	app.Get("/api/v2/filters", requireAnySecureScope(apiHandler.HandleGetFiltersLift, auth.ReadFilters, auth.ScopeRead), apptheory.Authenticated())
	app.Get("/api/v2/filters/{id}", requireAnySecureScope(apiHandler.HandleGetFilterLift, auth.ReadFilters, auth.ScopeRead), apptheory.Authenticated())
	app.Post("/api/v2/filters", requireAnySecureScope(apiHandler.HandleCreateFilterLift, auth.WriteFilters, auth.ScopeWrite), apptheory.Authenticated())
	app.Put("/api/v2/filters/{id}", requireAnySecureScope(apiHandler.HandleUpdateFilterLift, auth.WriteFilters, auth.ScopeWrite), apptheory.Authenticated())
	app.Delete("/api/v2/filters/{id}", requireAnySecureScope(apiHandler.HandleDeleteFilterLift, auth.WriteFilters, auth.ScopeWrite), apptheory.Authenticated())

	// API v2 filter keywords and statuses
	app.Get("/api/v2/filters/{filter_id}/keywords", requireAnySecureScope(apiHandler.HandleGetFilterKeywordsLift, auth.ReadFilters, auth.ScopeRead), apptheory.Authenticated())
	app.Post("/api/v2/filters/{filter_id}/keywords", requireAnySecureScope(apiHandler.HandleAddFilterKeywordLift, auth.WriteFilters, auth.ScopeWrite), apptheory.Authenticated())
	app.Delete("/api/v2/filters/{filter_id}/keywords/{keyword_id}", requireAnySecureScope(apiHandler.HandleDeleteFilterKeywordLift, auth.WriteFilters, auth.ScopeWrite), apptheory.Authenticated())
	app.Get("/api/v2/filters/{filter_id}/statuses", requireAnySecureScope(apiHandler.HandleGetFilterStatusesLift, auth.ReadFilters, auth.ScopeRead), apptheory.Authenticated())
	app.Post("/api/v2/filters/{filter_id}/statuses", requireAnySecureScope(apiHandler.HandleAddFilterStatusLift, auth.WriteFilters, auth.ScopeWrite), apptheory.Authenticated())
	app.Delete("/api/v2/filters/{filter_id}/statuses/{status_id}", requireAnySecureScope(apiHandler.HandleDeleteFilterStatusLift, auth.WriteFilters, auth.ScopeWrite), apptheory.Authenticated())

	// API v2 trends endpoints - Enhanced trending with metadata
	app.Get("/api/v2/trends", apiHandler.HandleGetTrendsV2Lift, apptheory.Public())
	app.Get("/api/v2/trends/tags", apiHandler.HandleGetTrendingTagsV2Lift, apptheory.Public())
	app.Get("/api/v2/trends/statuses", apiHandler.HandleGetTrendingStatusesV2Lift, apptheory.Public())
	app.Get("/api/v2/trends/links", apiHandler.HandleGetTrendingLinksV2Lift, apptheory.Public())

	// API v2 filter testing endpoint
	app.Post("/api/v2/filters/test", requireAnySecureScope(apiHandler.HandleTestFilterLift, auth.ReadFilters, auth.ScopeRead), apptheory.Authenticated())

	// API v2 grouped notifications endpoints
	app.Get("/api/v2/notifications/grouped", requireAnySecureScope(apiHandler.HandleGetGroupedNotificationsLift, auth.ReadNotifications, auth.ScopeRead), apptheory.Authenticated())
	app.Post("/api/v2/notifications/groups/{group_id}/read", requireAnySecureScope(apiHandler.HandleMarkGroupAsReadLift, auth.WriteNotifications, auth.ScopeWrite), apptheory.Authenticated())

	// Quote posts API endpoints
	// POST create quote is rate limited (spam prevention)
	app.Post("/api/v1/statuses/{id}/quote", requireAnySecureScope(ratelimit.ApplyRateLimit(
		apiHandler.HandleCreateQuotePostLift,
		30, time.Hour, logger), "write:statuses", auth.ScopeWrite), apptheory.Authenticated())
	// GETs, DELETE, and PUT are NOT rate limited
	app.Get("/api/v1/statuses/{id}/quotes", apiHandler.HandleGetQuotesOfStatusLift, apptheory.Public())
	app.Delete("/api/v1/statuses/{id}/quote/{quote_id}", requireAnySecureScope(apiHandler.HandleDeleteQuotePostLift, "write:statuses", auth.ScopeWrite), apptheory.Authenticated())
	app.Get("/api/v1/accounts/{id}/quote_permissions", apiHandler.HandleGetQuotePermissionsLift, apptheory.Authenticated())
	app.Put("/api/v1/accounts/quote_permissions", requireAnySecureScope(apiHandler.HandleUpdateQuotePermissionsLift, "write:accounts", auth.ScopeWrite), apptheory.Authenticated())

	// Public Mastodon-compatible account lookup endpoints. Keep static routes
	// before the generic /accounts/{id} route so lookup/search-style paths do
	// not resolve as account IDs.
	app.Get("/api/v1/accounts/lookup", apiHandler.HandleAccountLookupLift, apptheory.Public())
	app.Get("/api/v1/accounts/{id}", apiHandler.HandleGetAccountLift, apptheory.Public())
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
