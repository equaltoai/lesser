# COMPLETE Unbounded Query Inventory
**Date**: 2025-10-22  
**Total**: 165 unbounded .All() queries across 42 repository files

NO SUMMARIZATION. EVERY. SINGLE. QUERY.

---

## account_repository_auth.go (1 query)
- Line 1175: UpdateWebAuthnCredential

## account_repository_oauth.go (1 query)
- Line 460: ListOAuthClients

## account_repository_social.go (3 queries)
- Line 134: GetFollowers
- Line 181: GetFollowing  
- Line 495: GetBookmarks

## account_repository_timeline.go (6 queries)
- Line 39: GetHomeTimeline
- Line 75: GetLocalTimeline
- Line 117: GetPublicTimeline
- Line 153: GetHashtagTimeline
- Line 198: GetListTimeline
- Line 297: GetConversations

## account_repository.go (1 query)
- Line 1302: SearchAccounts

## activity_repository.go (3 queries)
- Line 149: GetInboxActivities
- Line 229: GetOutboxActivities (first occurrence)
- Line 238: GetOutboxActivities (second occurrence)

## announcement_repository.go (2 queries)
- Line 282: GetAnnouncementsPaginated
- Line 359: GetAnnouncementsByAdmin

## audit_repository.go (1 query)
- Line 152: GetSecurityEvents

## auth_repository.go (1 query)
- Line 966: queryWalletCredentials

## base_repository.go (10 queries)
- Line 467: Query
- Line 520: QueryWithSKPrefix
- Line 547: QueryGSI
- Line 586: BatchGet
- Line 779: QueryCollectionWithConversion (first)
- Line 793: QueryCollectionWithConversion (second)
- Line 1081: ListAggregatedByPeriod
- Line 1227: FindWithPagination
- Line 1422: QueryWithFilter
- Line 1473: QueryBetween

## base_repository_helpers.go (1 query)
- Line 62: QueryGSIWithTimeRangeHelper

## community_note_repository.go (1 query)
- Line 319: GetCommunityNotesByAuthor

## cost_tracking_repository.go (4 queries)
- Line 90: ListByTable
- Line 1029: GetRelayCostsByURL
- Line 1053: GetRelayCostsByDateRange
- Line 1157: GetRelayMetricsHistory

## dlq_repository.go (4 queries)
- Line 164: GetDLQMessagesByErrorType
- Line 189: GetDLQMessagesForReprocessing
- Line 219: GetDLQMessagesByStatus
- Line 492: SearchDLQMessages

## domain_block_repository.go (1 query)
- Line 139: GetUserDomainBlocks

## domain_pagination_helpers.go (2 queries)
- Line 65: getPaginatedInstanceDomainBlocks
- Line 164: getPaginatedDomainItems

## emoji_repository.go (1 query)
- Line 506: queryEmojiGSI

## enhanced_pattern_repository.go (5 queries)
- Line 144: GetActivePatterns
- Line 164: GetPatternsByType
- Line 184: GetPatternsByCategory
- Line 675: GetTestResults
- Line 711: GetPerformanceMetrics

## export.go (1 query)
- Line 193: GetExportCostTracking

## federation_activity_repository.go (1 query)
- Line 96: ListByDomain

## federation_cost_repository.go (3 queries)
- Line 94: GetFederationCosts
- Line 123: GetFederationCostsByActivityType
- Line 289: GetActiveBudgets

## federation_instance_repository.go (4 queries)
- Line 143: ListInstancesByStatusWithCursor
- Line 214: GetInstancesByTierWithCursor
- Line 409: SearchInstancesWithCursor
- Line 802: ListAllInstancesWithCursor

## hashtag_repository.go (3 queries)
- Line 559: getHashtagTimelineFromIndex
- Line 604: getHashtagTimelineByVisibility
- Line 903: GetFollowedHashtags

## import_export_simple_helpers.go (3 queries)
- Line 108: getImportExportItemsByStatus
- Line 184: getCostsByDateRange
- Line 234: getUserCosts

## import.go (1 query)
- Line 216: GetImportCostTracking

## instance_health_repository.go (2 queries)
- Line 183: GetHealthHistory
- Line 246: GetDomainsForHealthCheck

## list_repository.go (3 queries)
- Line 222: GetUserLists
- Line 261: GetListsByMember
- Line 735: GetListTimeline

## media_repository.go (5 queries)
- Line 768: GetTranscodingJobsByStatus
- Line 990: GetUnusedMedia
- Line 1213: GetModerationPendingMedia
- Line 1273: DeleteExpiredMedia
- Line 1321: GetTotalStorageUsage

## metrics_repository.go (3 queries)
- Line 125: ListByType
- Line 149: ListByService
- Line 653: GetMetricsByDate

## moderation_repository.go (13 queries)
- Line 161: GetModerationQueue
- Line 243: GetModerationQueuePaginated
- Line 334: GetModerationEventsByObject
- Line 387: GetModerationEventsByActor
- Line 1040: executeGSI2Query
- Line 1112: scanAllModerationEvents
- Line 2112: GetFlagsByObject
- Line 2159: GetFlagsByActor
- Line 2206: GetPendingFlags
- Line 2465: GetUserReports
- Line 2591: GetReportsByTarget
- Line 2656: GetReportsByStatus
- Line 3358: GetModerationDecisionsByModerator

## notification_cost_repository.go (4 queries)
- Line 75: GetCostTrackingByNotification
- Line 107: GetDailyCostTracking
- Line 187: ListAggregationsByPeriod
- Line 261: GetUserBudgets

## notification_helpers.go (2 queries)
- Line 67: executePaginatedNotificationQuery
- Line 153: GetCostTrackingByTimeRange

## notification_repository.go (7 queries)
- Line 142: GetUserNotifications
- Line 195: GetUnreadNotifications
- Line 249: GetNotificationsByType
- Line 413: GetPendingPushNotifications
- Line 464: GetNotificationGroups
- Line 617: GetNotificationCountsByType
- Line 923: GetNotificationsAdvanced

## oauth_helpers.go (1 query)
- Line 325: ListOAuthClientsGeneric

## oauth_session_repository.go (1 query)
- Line 144: GetUserOAuthSessions

## object_repository.go (7 queries)
- Line 381: GetObjectsByActor
- Line 418: CountObjectReplies
- Line 608: GetUpdateHistory
- Line 683: GetCollectionItems
- Line 1038: GetReplies
- Line 1322: GetQuotesForNote
- Line 1630: GetQuotesOfStatus

## pattern_repository.go (1 query)
- Line 163: GetPatterns

## push_subscription_repository.go (1 query)
- Line 137: GetUserPushSubscriptions

## query_utils.go (7 queries)
- Line 100: UserRelationshipQuery
- Line 141: TimeRangeQuery
- Line 173: GSIStatusQuery
- Line 363: QueryByGSI
- Line 397: QueryWithPrefix
- Line 463: GenericList
- Line 590: QueryBuilder.Execute

## quote_repository.go (1 query)
- Line 142: getQuotesByGSI

## relationship_helpers.go (2 queries)
- Line 125: GetRelatedUsers
- Line 187: GetUsersWhoRelated

## relationship_pagination_helpers.go (2 queries)
- Line 100: executeBlockQuery
- Line 126: executeMuteQuery

## relationship_repository.go (6 queries)
- Line 242: getRelationshipsByState
- Line 311: GetFollowing
- Line 693: GetAccountMoves
- Line 770: GetPendingMoves
- Line 797: GetMoveByTarget
- Line 1099: GetCollectionItems

## relay_repository.go (1 query)
- Line 192: GetAllRelays

## route_optimizer_repository.go (5 queries)
- Line 78: GetRouteResults
- Line 107: GetRecentResults
- Line 155: GetOptimizationDecisions
- Line 335: GetMetricsInRange (first)
- Line 357: GetMetricsInRange (second)

## scheduled_job_cost_repository.go (3 queries)
- Line 148: ListByJob
- Line 172: ListByStatus
- Line 196: ListByDateRange

## scheduled_status_repository.go (2 queries)
- Line 177: GetScheduledStatuses
- Line 261: GetDueScheduledStatuses

## search_repository.go (1 query)
- Line 1972: processBatch

## severance_repository.go (4 queries)
- Line 132: ListSeveredRelationships (first)
- Line 148: ListSeveredRelationships (second)
- Line 163: ListSeveredRelationships (third)
- Line 288: GetAffectedRelationships

## shared_helpers_simple.go (1 query)
- Line 57: AuditLogQueryHelper

## social_repository.go (5 queries)
- Line 213: GetBlockedUsers
- Line 265: GetBlockedByUsers
- Line 405: GetMutedUsers
- Line 546: GetStatusAnnounces
- Line 610: GetActorAnnounces

## status_repository.go (1 query)
- Line 796: queryStatusesByGSI

## timeline_repository.go (3 queries)
- Line 97: GetPublicTimeline
- Line 165: getTimelineEntriesByGSI
- Line 389: GetConversations

## user_repository.go (4 queries)
- Line 250: ListUsers
- Line 811: GetReputationHistory
- Line 2582: GetBookmarks
- Line 2779: getTimelineEntries

## websocket_cost_repository.go (3 queries)
- Line 163: queryByGSIWithTimeRange
- Line 214: queryBudgetsByGSI
- Line 268: queryAggregationsByGSI

---

## SUMMARY

**Total Files with Unbounded Queries**: 42  
**Total Unbounded Queries**: 165

**Distribution**:
- 10+ queries: base_repository.go (10), moderation_repository.go (13)
- 5-9 queries: 9 files
- 2-4 queries: 19 files
- 1 query: 12 files

**NO FILES HIDDEN. NO "PLUS MORE". THIS IS COMPLETE.**

---

## FIX APPROACH

For EACH of the 165 queries:
1. Add `.Limit(appropriate_value)` before `.All()`
2. If method has limit parameter, use it
3. If not, add limit parameter to method signature
4. Default limits: 100 for lists, 1000 for admin queries

**Estimated time**: 6-8 hours for all 165 queries (mechanical work)

**No shortcuts. Every single one must be fixed.**

