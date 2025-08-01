# Handler Migration Status

## Migration Overview
This document tracks the migration of all AWS Lambda handlers to the Lift framework.

## Legend
- ✅ **MIGRATED**: Handler has been successfully migrated to Lift
- ❌ **NOT MIGRATED**: Handler still uses AWS Lambda events

---

## Summary Stats
- **Total Files with Handlers**: 54
- **Migrated Files**: 51
- **Remaining Files**: 2
- **Skipped Files**: 1 (recovery.go - email-based)
- **Migration Progress**: 96.3%

---

## File-by-File Status

### ✅ accounts.go (MIGRATED)
- **Lines**: 1482
- **Handlers**: 12
- HandleRegistration, HandleVerifyCredentials, HandleUpdateCredentials, HandleGetAccount, HandleAccountLookup, HandleGetAccountFollowers, HandleGetAccountFollowing, HandleGetFamiliarFollowers, HandlePinAccount, HandleUnpinAccount, HandleSetAccountNote, HandleRemoveFromFollowers

### ✅ admin_federation.go (MIGRATED)
- **Lines**: 874
- **Handlers**: 14
- HandleGetAdminDomainBlocksLift, HandleGetAdminDomainBlockLift, HandleCreateAdminDomainBlockLift, HandleUpdateAdminDomainBlockLift, HandleDeleteAdminDomainBlockLift, HandleGetAdminDomainAllowsLift, HandleCreateAdminDomainAllowLift, HandleDeleteAdminDomainAllowLift, HandleGetFederationInstancesLift, HandleGetFederationInstanceLift, HandleGetFederationStatisticsLift, HandleGetEmailDomainBlocksLift, HandleCreateEmailDomainBlockLift, HandleDeleteEmailDomainBlockLift

### ❌ admin.go (NOT MIGRATED)
- **Lines**: 1774
- **Handlers**: 21
- HandleAdminGetAccounts, HandleAdminGetAccount, HandleAdminAccountAction, HandleAdminApproveAccount, HandleAdminRejectAccount, HandleAdminEnableAccount, HandleAdminUnsilenceAccount, HandleAdminUnsuspendAccount, HandleAdminUnsensitiveAccount, HandleAdminGetReports, HandleAdminGetReport, HandleAdminResolveReport, HandleAdminReopenReport, HandleAdminAssignReport, HandleAdminUnassignReport, HandleAdminModerationOverview, HandleAdminGetModerationEvents, HandleAdminOverrideModerationEvent, HandleAdminGetTrustGraph, HandleAdminUpdateTrust, HandleAdminGetReviewers

### ✅ ai.go (MIGRATED)
- **Lines**: 285
- **Handlers**: 4
- HandleGetAIAnalysis, HandleRequestAIAnalysis, HandleGetAIStats, HandleGetAISummary

### ✅ announcements.go (MIGRATED)
- **Lines**: 449
- **Handlers**: 5
- HandleGetAnnouncements, HandleGetAnnouncement, HandleDismissAnnouncement, HandleCreateAnnouncement, HandleUpdateAnnouncement

### ✅ apps.go (MIGRATED)
- **Lines**: 313
- **Handlers**: 3
- HandleCreateApp, HandleGetApp, HandleVerifyAppCredentials

### auth_cookies.go (NO HANDLERS - CONFIG ONLY)
- **Lines**: 34
- **Handlers**: 0

### ✅ bookmarks.go (MIGRATED)
- **Lines**: 268
- **Handlers**: 1
- HandleGetBookmarks

### ✅ conversations.go (MIGRATED)
- **Lines**: 294
- **Handlers**: 3
- HandleGetConversations, HandleGetConversation, HandleMarkConversationAsRead

### ✅ custom_emojis.go (MIGRATED)
- **Lines**: 240
- **Handlers**: 1
- HandleGetCustomEmojis

### ✅ debug.go (MIGRATED)
- **Lines**: 531
- **Handlers**: 7
- HandleDebugFeatureToggle, HandleDebugConfig, HandleDebugStorage, HandleDebugCache, HandleDebugQueue, HandleDebugMetrics, HandleDebugHealth

### ✅ discovery.go (MIGRATED)
- **Lines**: 320
- **Handlers**: 3
- HandleGetDirectory, HandleGetSuggestions, HandleDeleteSuggestion

### ✅ domain_blocks.go (MIGRATED)
- **Lines**: 167
- **Handlers**: 3
- HandleGetDomainBlocks, HandleBlockDomain, HandleUnblockDomain

### ✅ endorsements.go (MIGRATED)
- **Lines**: 71
- **Handlers**: 1
- HandleGetEndorsements

### ✅ exports.go (MIGRATED)
- **Lines**: 488
- **Handlers**: 3
- HandleCreateExport, HandleGetExportStatus, HandleListExports

### ✅ favorites.go (MIGRATED)
- **Lines**: 134
- **Handlers**: 1
- HandleGetFavorites

### ✅ filters.go (MIGRATED)
- **Lines**: 604
- **Handlers**: 7
- HandleGetFilters, HandleGetFilter, HandleCreateFilter, HandleUpdateFilter, HandleDeleteFilter, HandleGetFilterKeywords, HandleGetFilterStatuses

### ✅ follow_requests.go (MIGRATED)
- **Lines**: 236
- **Handlers**: 3
- HandleGetFollowRequests, HandleAcceptFollowRequest, HandleRejectFollowRequest

### follow_request_helpers.go (NO HANDLERS - HELPER FUNCTIONS ONLY)
- **Lines**: 243
- **Handlers**: 0

### ✅ imports.go (MIGRATED)
- **Lines**: 467
- **Handlers**: 5
- HandleImportFollows, HandleImportBlocks, HandleImportMutes, HandleImportDomainBlocks, HandleImportBookmarks

### ✅ instance.go (MIGRATED)
- **Lines**: 411
- **Handlers**: 6
- HandleGetInstance, HandleGetInstancePeers, HandleGetInstanceActivity, HandleGetInstanceRules, HandleGetInstanceDomainBlocks, HandleGetExtendedDescription

### ✅ interactions.go (MIGRATED)
- **Lines**: 576
- **Handlers**: 6
- HandleFavoriteStatus, HandleUnfavoriteStatus, HandleReblogStatus, HandleUnreblogStatus, HandleBookmarkStatus, HandleUnbookmarkStatus

### ✅ lists.go (MIGRATED)
- **Lines**: 509
- **Handlers**: 8
- HandleGetLists, HandleGetList, HandleCreateList, HandleUpdateList, HandleDeleteList, HandleGetListAccounts, HandleAddAccountsToList, HandleRemoveAccountsFromList

### ✅ markers.go (MIGRATED)
- **Lines**: 174
- **Handlers**: 2
- HandleGetMarkers, HandleSetMarkers

### ✅ media.go (MIGRATED)
- **Lines**: 874
- **Handlers**: 3
- HandleUploadMedia, HandleGetMedia, HandleUpdateMedia

### ✅ media_v2.go (MIGRATED)
- **Lines**: 371
- **Handlers**: 3
- HandleUploadMediaV2, HandleGetMediaV2, HandleUpdateMediaV2

### ✅ metrics.go (MIGRATED)
- **Lines**: 453
- **Handlers**: 4
- HandleGetMetrics, HandleGetPrometheusMetrics, HandleGetHealthCheck, HandleGetReadinessCheck

### ✅ misc.go (MIGRATED)
- **Lines**: 989
- **Handlers**: 14
- HandleGetPreferences, HandleUpdatePreferences, HandleGetSuggestions, HandleDeleteSuggestion, HandleGetTrendingTags, HandleGetTrendingStatuses, HandleGetTrendingLinks, HandleGetFeaturedTags, HandleCreateFeaturedTag, HandleDeleteFeaturedTag, HandleGetTagSuggestions, HandleGetDirectory, HandleGetProofOfWork, HandleSubmitProofOfWork

### ✅ moderation.go (MIGRATED)
- **Lines**: 704
- **Handlers**: 8
- HandleModerationFlag, HandleModerationQueue, HandleModerationReview, HandleModerationHistory, HandleGetConsensus, HandleGetTrustRelationships, HandleUpdateTrust, HandleGetTrustScore

### ✅ mutes.go (MIGRATED)
- **Lines**: 243
- **Handlers**: 2
- HandleGetMutes, HandleMuteAccount, HandleUnmuteAccount

### ✅ nodeinfo.go (MIGRATED)
- **Lines**: 167
- **Handlers**: 2
- HandleNodeInfoWellKnown, HandleNodeInfo

### ✅ notes.go (MIGRATED)
- **Lines**: 455
- **Handlers**: 4
- HandleCreateNote, HandleGetNote, HandleUpdateNote, HandleDeleteNote

### ✅ oauth.go (MIGRATED)
- **Lines**: 665
- **Handlers**: 6
- HandleAuthorize, HandleToken, HandleRevoke, HandleIntrospect, HandleGetAuthorizedApps, HandleRevokeAuthorizedApp

### ✅ oembed.go (MIGRATED)
- **Lines**: 548
- **Handlers**: 1
- HandleOEmbed

### ✅ polls.go (MIGRATED)
- **Lines**: 334
- **Handlers**: 2
- HandleGetPoll, HandleVoteOnPoll

### ✅ preferences.go (MIGRATED)
- **Lines**: 179
- **Handlers**: 2
- HandleGetPreferences, HandleUpdatePreferences

### ✅ push_subscriptions.go (MIGRATED)
- **Lines**: 327
- **Handlers**: 4
- HandleCreatePushSubscription, HandleGetPushSubscription, HandleUpdatePushSubscription, HandleDeletePushSubscription

### ❌ recovery.go (NOT MIGRATED)
- **Lines**: 357
- **Handlers**: 4
- HandleInitiatePasswordReset, HandleResetPassword, HandleVerifyEmail, HandleResendConfirmation

### ✅ recovery_emailfree.go (MIGRATED)
- **Lines**: 325
- **Handlers**: 9
- HandleGetRecoveryOptions, HandleInitiateSocialRecovery, HandleConfirmSocialRecovery, HandleGenerateRecoveryCodes, HandleUseRecoveryCode, HandleAddTrustee, HandleListTrustees, HandleRemoveTrustee, HandleDeviceRecovery

### ✅ relationships.go (MIGRATED)
- **Lines**: 210
- **Handlers**: 4
- HandleGetRelationships, HandleFollowAccount, HandleUnfollowAccount, HandleGetFollowSuggestions

### ✅ reputation.go (MIGRATED)
- **Lines**: 419
- **Handlers**: 8
- HandleGetReputation, HandleExportReputation, HandleImportReputation, HandleCreateVouch, HandleGetVouches, HandleRevokeVouch, HandleVerifyReputation, HandleGetReputationKeys

### ✅ reports.go (MIGRATED)
- **Lines**: 196
- **Handlers**: 2
- HandleCreateReport, HandleGetReports

### ✅ scheduled_statuses.go (MIGRATED)
- **Lines**: 467
- **Handlers**: 5
- HandleGetScheduledStatuses, HandleGetScheduledStatus, HandleUpdateScheduledStatus, HandleDeleteScheduledStatus, HandleCreateScheduledStatus

### ✅ search.go (MIGRATED)
- **Lines**: 192
- **Handlers**: 2
- HandleSearch, HandleAccountSearch

### ✅ status_info.go (MIGRATED)
- **Lines**: 238
- **Handlers**: 3
- HandleGetStatus, HandleGetStatusContext, HandleGetStatusRebloggedBy

### ✅ status_interactions.go (MIGRATED)
- **Lines**: 149
- **Handlers**: 6
- HandleFavoriteStatus, HandleUnfavoriteStatus, HandleReblogStatus, HandleUnreblogStatus, HandleBookmarkStatus, HandleUnbookmarkStatus

### ✅ status_pins.go (MIGRATED)
- **Lines**: 304
- **Handlers**: 2
- HandlePinStatus, HandleUnpinStatus

### ❌ statuses.go (NOT MIGRATED)
- **Lines**: 1710
- **Handlers**: 18
- HandleCreateStatus, HandleGetStatus, HandleDeleteStatus, HandleGetStatusContext, HandleGetStatusCard, HandleGetStatusRebloggedBy, HandleGetStatusFavouritedBy, HandlePinStatus, HandleUnpinStatus, HandleGetStatusHistory, HandleGetStatusEdits, HandlePutStatusEditStatus, HandleGetStatusSource, HandleTranslateStatus, HandleMuteStatusConversation, HandleUnmuteStatusConversation, HandleReblogStatus, HandleUnreblogStatus

### ✅ statuses_unified_boost.go (MIGRATED)
- **Lines**: 364
- **Handlers**: 2
- HandleUnifiedBoost, HandleUndoUnifiedBoost

### ✅ tags.go (MIGRATED)
- **Lines**: 529
- **Handlers**: 9
- HandleGetTag, HandleFollowTag, HandleUnfollowTag, HandleGetFollowedTags, HandleGetFeaturedTags, HandleCreateFeaturedTag, HandleDeleteFeaturedTag, HandleGetFeaturedTagSuggestions, HandleGetAccountFeaturedTags

### ✅ timelines.go (MIGRATED)
- **Lines**: 711
- **Handlers**: 5
- HandleHomeTimeline, HandlePublicTimeline, HandleHashtagTimeline, HandleListTimeline, HandleDirectTimeline

### ✅ translation.go (MIGRATED)
- **Lines**: 226
- **Handlers**: 1
- HandleTranslateStatus

### ✅ trends.go (MIGRATED)
- **Lines**: 229
- **Handlers**: 3
- HandleGetTrendingTags, HandleGetTrendingStatuses, HandleGetTrendingLinks

### ✅ wallet.go (MIGRATED)
- **Lines**: 274
- **Handlers**: 6
- HandleWalletChallenge, HandleWalletAuth, HandleWalletLink, HandleWalletUnlink, HandleGetWalletCredentials, HandleGetWalletNonce

### ✅ webauthn.go (MIGRATED)
- **Lines**: 271
- **Handlers**: 6
- HandleWebAuthnBeginRegistration, HandleWebAuthnFinishRegistration, HandleWebAuthnBeginLogin, HandleWebAuthnFinishLogin, HandleGetWebAuthnCredentials, HandleDeleteWebAuthnCredential

### ✅ webfinger.go (MIGRATED)
- **Lines**: 134
- **Handlers**: 1
- HandleWebfinger

---

## Next Files to Migrate (Prioritized by Size)

### Remaining Files (2 total)
1. **❌ statuses.go** - 1710 lines, 18 handlers (Large)
2. **❌ admin.go** - 1774 lines, 21 handlers (Large)

### Skipped Files (1 total)
- **⚠️ recovery.go** - 357 lines, 5 handlers (Email-based recovery not supported in Lesser)

---

## Progress Tracking

### Completed Migrations ✅ (51 files)
- accounts.go, admin_federation.go, ai.go, announcements.go, apps.go, bookmarks.go, conversations.go, custom_emojis.go, debug.go, discovery.go, domain_blocks.go, endorsements.go, exports.go, favorites.go, filters.go, follow_requests.go, imports.go, instance.go, interactions.go, lists.go, markers.go, media.go, media_v2.go, metrics.go, misc.go, moderation.go, mutes.go, nodeinfo.go, notes.go, oauth.go, oembed.go, polls.go, preferences.go, push_subscriptions.go, recovery_emailfree.go, relationships.go, reputation.go, reports.go, scheduled_statuses.go, search.go, status_info.go, status_interactions.go, status_pins.go, statuses_unified_boost.go, tags.go, timelines.go, translation.go, trends.go, wallet.go, webauthn.go, webfinger.go

### Remaining Migrations ❌ (2 files)
- admin.go, statuses.go

### Files with No Handlers (3 files)
- auth_cookies.go (config only), follow_request_helpers.go (helper functions), common.go (shared utilities)

**NEXT TARGET: statuses.go (1710 lines, 18 handlers)** - Status management operations