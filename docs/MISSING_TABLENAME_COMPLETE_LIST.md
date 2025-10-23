# COMPLETE TableName() Missing Methods Inventory
**Date**: 2025-10-22  
**Total Models**: 345  
**Models WITH TableName()**: 141  
**Models MISSING TableName()**: 237

Use this tracker to drive TableName() remediation. Update `Status` (`TODO`, `IN_PROGRESS`, `DONE`) and `Assignee` as work progresses. Do not remove rows until the method is implemented and verified.


---

### activity.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Activity |

---

### actor.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ActorField |
| DONE | Codex | ActorMetadata |

---

### ai_analysis.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | AIAnalysis |
| TODO |  | AIAnalysisQueue |

---

### ai_cost.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | AIAggregatedCost |
| DONE | Codex | AICost |

---

### alert.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | Alert |
| TODO |  | DeadLetterMessage |
| TODO |  | WebhookDelivery |

---

### analytics.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | EngagementMetrics |
| DONE | Codex | LinkShare |

---

### announcement.go (6 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Announcement |
| DONE | Codex | AnnouncementDismissal |
| DONE | Codex | AnnouncementReaction |
| DONE | Codex | CustomEmoji |
| DONE | Codex | Mention |
| DONE | Codex | Reaction |

---

### audit_log.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | AuthAuditLog |

---

### bookmark.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Bookmark |

---

### circuit_breaker.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | CircuitBreakerConfig |
| TODO |  | CircuitBreakerEvent |
| TODO |  | CircuitBreakerState |

---

### cloudwatch_metrics.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | CloudWatchMetrics |

---

### community_note.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | CommunityNote |
| DONE | Codex | CommunityNoteVote |

---

### cost_driver.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Driver |

---

### cost_projection.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | CostProjection |

---

### cost_tracking.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | DynamoDBCostRecordBuilder |
| DONE | Codex | DynamoDBServiceCostStats |
| DONE | Codex | DynamoDBTableCostStats |

---

### custom_emoji.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | EmojiModel |

---

### delivery_status.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | DeliveryStatus |

---

### device.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | Device |

---

### dlq_message.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | DLQMessageBuilder |

---

### dns_cache.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | DNSCache |

---

### domain_block.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | DomainAllow |
| DONE | Codex | EmailDomainBlock |
| DONE | Codex | InstanceDomainBlock |
| DONE | Codex | UserDomainBlock |

---

### export.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Export |
| DONE | Codex | ExportDateRange |

---

### export_cost_tracking.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ExportCostSummary |
| DONE | Codex | ExportCostTracking |
| DONE | Codex | ExportTypeCostStats |

---

### featured_tag.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | FeaturedTag |

---

### federation_activity.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationActivityBuilder |
| DONE | Codex | InstanceInfo |

---

### federation.go (6 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationCost |
| DONE | Codex | FederationCostActivity |
| DONE | Codex | FederationEdge |
| DONE | Codex | FederationHealthReport |
| DONE | Codex | FederationInstance |
| DONE | Codex | FederationNode |

---

### federation.go (continued - 3 more models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | InstanceCluster |
| DONE | Codex | InstanceConnection |
| DONE | Codex | InstanceMetadata |

---

### federation_cost_tracking.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationBudget |
| DONE | Codex | FederationCostTracking |

---

### federation_instance.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationInstanceRegistry |
| DONE | Codex | FederationInstanceRegistryHealthHistory |

---

### federation_instance_config.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationInstanceConfigTracking |
| DONE | Codex | RetryPolicy |

---

### federation_instance_health.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationInstanceHealthTracking |

---

### federation_metrics.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationAlert |
| DONE | Codex | FederationAnalyticsTimeSeries |

---

### federation_relationship.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationRelationship |
| DONE | Codex | FederationRelationshipAggregate |
| DONE | Codex | FederationRelationshipIndex |
| DONE | Codex | MetricsCompression |

---

### federation_route_metrics.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ErrorFrequency |
| DONE | Codex | FederationRouteAggregation |
| DONE | Codex | FederationRouteMetrics |
| DONE | Codex | RouteRecommendation |

---

### federation_severance.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationIssue |
| DONE | Codex | FederationSeverance |
| DONE | Codex | FederationTimeSeries |
| DONE | Codex | ReconnectionAttempt |

---

### federation_stats.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | FederationStats |

---

### follow.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | Follow |

---

### hashtag.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | Hashtag |

---

### hashtag_follow.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | HashtagFollow |

---

### hashtag_history_entry.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | HashtagHistoryEntry |

---

### hashtag_mute.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | HashtagMute |

---

### hashtag_notification_settings.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | HashtagNotificationSettings |
| DONE | Codex | NotificationFilter |

---

### hashtag_search_result.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | HashtagSearchResult |

---

### hashtag_stats.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | HashtagStats |

---

### health_check_result.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ComponentHealthHistory |
| DONE | Codex | HealthCheckResult |
| DONE | Codex | HealthCheckSummaryResult |

---

### import.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Import |

---

### import_budget.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ImportBudget |
| DONE | Codex | ImportCostSummary |
| DONE | Codex | ImportTypeCostStats |

---

### import_cost_tracking.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ImportCostTracking |

---

### inbox_item.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | InboxItem |

---

### instance_config.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | AIInstanceConfig |
| DONE | Codex | InstanceConfig |

---

### instance_health.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | InstanceHealth |
| DONE | Codex | InstanceHealthSummary |

---

### instance_health_report.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | InstanceHealthReport |

---

### instance_history.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | InstanceHistory |

---

### instance_metrics.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | InstanceMetrics |

---

### instance_rule.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | InstanceRule |

---

### like.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Like |

---

### link_metadata.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | LinkMetadata |

---

### media.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | MediaVariant |

---

### media_analytics.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | MediaAnalytics |
| DONE | Codex | MediaVariantCost |

---

### media_metadata.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | QualityCodecInfo |

---

### media_popularity.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | MediaPopularity |

---

### media_session.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | MediaSession |
| TODO |  | QualityChange |

---

### metrics.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | DimensionStats |
| DONE | Codex | MetricRecordBuilder |
| DONE | Codex | MetricsBuilder |

---

### missing_reply.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | MissingReply |

---

### moderation.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Moderation |
| DONE | Codex | ModerationEvidence |
| DONE | Codex | ModerationHistoryEntry |

---

### moderation_analytics.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ModerationAnalytics |

---

### moderation_metrics.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ModerationMetricsStats |
| DONE | Codex | ModerationMetricsTimeRange |
| DONE | Codex | PatternStats |
| DONE | Codex | RealtimeStats |

---

### moderation_ml.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | TrainingMetrics |

---

### notification.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | NotificationBuilder |

---

### notification_cost_tracking.go (7 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | NotificationBudget |
| DONE | Codex | NotificationChannelCostStats |
| DONE | Codex | NotificationCostAggregation |
| DONE | Codex | NotificationCostTracking |
| DONE | Codex | NotificationCostTrackingBuilder |
| DONE | Codex | NotificationTypeCostStats |
| DONE | Codex | NotificationUserCostStats |

---

### notification_preferences.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | NotificationPreferences |

---

### oauth_app.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | OAuthApp |

---

### oauth_state.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | OAuthState |

---

### object.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Object |

---

### outbox_item.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | OutboxItem |

---

### pattern_feedback.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | PatternFeedback |

---

### public_key_cache.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | PublicKeyCache |

---

### push_subscription.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | PushSubscriptionAlerts |

---

### query_cache.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | BatchGetKeys |
| TODO |  | QueryCacheEntry |

---

### quote_permissions.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | QuotePermissions |

---

### quote_relationship.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | QuoteRelationship |

---

### recovery.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | RecoveryCode |
| DONE | Codex | RecoveryRequest |
| DONE | Codex | RecoveryToken |
| DONE | Codex | Trustee |

---

### relay.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | Relay |

---

### relay_cost.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | RelayBudget |
| DONE | Codex | RelayCost |
| DONE | Codex | RelayMetrics |
| DONE | Codex | RelayOperationStats |

---

### relay_info.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | RelayInfo |

---

### remote_actor.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | RemoteActor |

---

### report.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | Report |
| TODO |  | ReportStats |

---

### reputation.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | Reputation |

---

### reviewer_stats.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | ReviewerStats |

---

### route_optimizer.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | OptimizationDecision |
| TODO |  | RouteDeliveryResult |

---

### routing_metrics.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | GlobalMetricsWindow |
| DONE | Codex | InstanceMetricsWindow |
| DONE | Codex | RouteMetricsWindow |

---

### scheduled_job_cost_tracking.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ScheduledJobCategoryStats |
| DONE | Codex | ScheduledJobCostRecordBuilder |
| DONE | Codex | ScheduledJobEnvironmentStats |
| DONE | Codex | ScheduledJobScheduleStats |

---

### scheduled_status.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | ScheduledStatus |

---

### search_analytics.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SearchAnalytics |

---

### search_cache.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SearchCache |

---

### search_click_rate.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SearchClickRate |

---

### search_cost_tracking.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SearchBudget |
| DONE | Codex | SearchCostAggregation |
| DONE | Codex | SearchCostTracking |
| DONE | Codex | SearchQueryStats |

---

### search_embedding.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SearchEmbedding |

---

### search_history_entry.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SearchHistoryEntry |

---

### search_results.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SearchResults |

---

### search_suggestion.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SearchSuggestion |

---

### severed_relationship.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | AffectedRelationship |
| DONE | Codex | SeveranceReconnectionAttempt |
| DONE | Codex | SeveredRelationship |

---

### social_recovery_request.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | SocialRecoveryRequest |

---

### status.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | StatusAttachment |
| DONE | Codex | StatusHashtagIndex |
| DONE | Codex | StatusTag |

---

### status_search_options.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | StatusSearchOptions |
| DONE | Codex | TimeRange |

---

### status_search_result.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | StatusSearchResult |

---

### status_test.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | StatusModelTestSuite |

---

### streaming.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | StreamingPreferences |

---

### streaming_cloudwatch_metrics.go (5 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | ConcurrentViewerMetrics |
| DONE | Codex | GeographicMetric |
| DONE | Codex | QualityMetric |
| DONE | Codex | StreamingCloudWatchMetrics |
| DONE | Codex | StreamingPerformanceMetrics |

---

### streaming_event.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | StreamingEvent |

---

### thread_context.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | ThreadContext |

---

### thread_node.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | ThreadNode |

---

### thread_sync.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | ThreadSync |

---

### threat_intel.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | ThreatIndicator |
| TODO |  | ThreatIntel |

---

### timeline_entry.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | TimelineEntry |

---

### trending_hashtag.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | TrendingHashtag |

---

### trending_link.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | TrendingLink |

---

### trending_status.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | TrendingStatus |

---

### trends.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | HashtagTrend |
| TODO |  | LinkTrend |
| TODO |  | PopularQueryCounter |
| TODO |  | SearchQuery |
| TODO |  | StatusTrend |

---

### trust.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | TrustEvidence |
| DONE | Codex | TrustRelationship |
| DONE | Codex | TrustScore |
| DONE | Codex | TrustUpdate |

---

### trustee_config.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | TrusteeConfig |

---

### update_history.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | UpdateHistory |

---

### user_app_consent.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | UserAppConsent |

---

### user_preferences.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | UserPreferencesStorage |

---

### user_test.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | UserModelTestSuite |

---

### vapid.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | VAPIDKeyRecord |

---

### vouch.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | Vouch |

---

### wallet.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | WalletIndex |

---

### websocket_connection.go (4 models)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | ~~WebSocketConnection~~ ✅ FIXED |
| TODO |  | ~~WebSocketSubscription~~ ✅ FIXED   |
| DONE | Codex | ConnectionInfo |
| DONE | Codex | ConnectionMetrics |

---

### websocket_cost_tracking.go (3 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | WebSocketCostBreakdown |
| DONE | Codex | WebSocketCostRecordBuilder |
| DONE | Codex | WebSocketTierCostStats |

---

### websocket_subscription_manager.go (2 models)

| Status | Assignee | Model |
| --- | --- | --- |
| DONE | Codex | WebSocketEventConnection |
| DONE | Codex | WebSocketEventSubscription |

---

### weekly_activity.go (1 model)

| Status | Assignee | Model |
| --- | --- | --- |
| TODO |  | WeeklyActivity |
