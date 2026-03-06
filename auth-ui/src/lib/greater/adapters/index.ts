/**
 * @equaltoai/greater-components-adapters - Transport adapters and state management for Fediverse protocols
 */

import type { AiAnalysisQuery } from './graphql';

export type AIAnalysis = NonNullable<AiAnalysisQuery['aiAnalysis']>;

// Transport Clients
export { WebSocketClient } from './WebSocketClient';
export { SseClient } from './SseClient';
export { HttpPollingClient } from './HttpPollingClient';
export { TransportFallback } from './TransportFallback';
export { TransportManager } from './TransportManager';

// WebSocket Connection Pool
export { WebSocketPool, getGlobalWebSocketPool, resetGlobalWebSocketPool } from './WebSocketPool';

export type { WebSocketPoolConfig, PooledWebSocket } from './WebSocketPool';

// GraphQL Client & Adapter
export {
	createGraphQLClient,
	getGraphQLClient,
	closeGraphQLClient,
	LesserGraphQLAdapter,
	createLesserGraphQLAdapter,
	typePolicies,
	cacheConfig,
	evictStaleCache,
	limitCacheSize,
	optimistic,
} from './graphql';

export type {
	GraphQLClientConfig,
	GraphQLClientInstance,
	LesserGraphQLAdapterConfig,
	TimelineVariables,
	CreateNoteVariables,
	CreateQuoteNoteMutationVariables,
	SearchVariables,
	Visibility,
	NotificationLevel,
	ObjectFieldsFragment,
	Actor,
	ViewerQuery,
	QuoteType,
	UploadMediaInput,
	MediaCategory,
	MediaType,
	UploadMediaMutationVariables,
	UploadMediaMutation,
	UploadMediaPayload,
} from './graphql';

// Reactive Stores (Svelte 5 Runes)
export { createTimelineStore, createNotificationStore, createPresenceStore } from './stores';

// Admin Streaming Store
export { AdminStreamingStore, createAdminStreamingStore } from './stores/adminStreamingStore';

export type {
	MetricsUpdate,
	ModerationAlert,
	CostAlert,
	BudgetAlert,
	FederationHealthUpdate,
	ModerationQueueItem,
	ThreatIntelligence,
	PerformanceAlert,
	InfrastructureEvent,
	AdminStreamingStoreState,
	AdminStreamingStoreConfig,
} from './stores/adminStreamingStore';

// Unified Model Converters
export {
	unifiedStatusToTimelineItem,
	unifiedStatusesToTimelineItems,
} from './stores/unifiedToTimeline';

export {
	unifiedNotificationToStoreNotification,
	unifiedNotificationsToStoreNotifications,
} from './stores/unifiedToNotification';

// Streaming Operations
export {
	StreamingOperationsManager,
	TimelineStreamingHandler,
	NotificationStreamingHandler,
	AccountStreamingHandler,
	DeleteStreamingHandler,
	EditStreamingHandler,
	StreamingStateManager,
} from './streaming/operations';

export type {
	StreamingOperation,
	StreamingOperationHandler,
	StreamingState,
	StreamingConfig,
} from './streaming/operations';

// API Mappers for Fediverse Protocols
export {
	// Mastodon REST v1 mappers
	mapMastodonAccount,
	mapMastodonStatus,
	mapMastodonNotification,
	mapMastodonStreamingEvent,
	batchMapMastodonAccounts,
	batchMapMastodonStatuses,
	batchMapMastodonNotifications,

	// Lesser GraphQL mappers
	mapLesserAccount,
	mapLesserPost,
	mapLesserNotification,
	mapLesserStreamingUpdate,
	handleLesserGraphQLResponse,
	mapLesserTimelineConnection,
	batchMapLesserAccounts,
	batchMapLesserPosts,
	batchMapLesserNotifications,
	mapLesserObject,
	convertGraphQLObjectToLesser,

	// Mapper utilities
	MapperUtils,
} from './mappers';

// Transport Types
export type {
	// Base types
	BaseTransportConfig,
	TransportAdapter,
	TransportFallbackConfig,

	// Transport Manager types
	TransportType,
	TransportManagerConfig,
	TransportManagerState,
	TransportSwitchEvent,
	TransportEventMap,
	TransportEventName,

	// WebSocket types
	WebSocketClientConfig,
	WebSocketClientState,
	WebSocketMessage,
	WebSocketEvent,
	WebSocketEventType,
	WebSocketEventHandler,

	// SSE types
	SseClientConfig,
	SseClientState,
	SseMessage,

	// HTTP Polling types
	HttpPollingClientConfig,
	HttpPollingClientState,
	HttpPollingMessage,

	// Shared types
	HeartbeatMessage,
	LatencySample,
} from './types';

// Store Types
export type {
	// Store interfaces
	TimelineStore,
	NotificationStore,
	PresenceStore,
	BaseStore,
	StoreFactory,

	// Timeline types
	TimelineItem,
	TimelineState,
	TimelineConfig,

	// Notification types
	Notification,
	NotificationState,
	NotificationConfig,
	NotificationFilter,
	NotificationAction,

	// Lesser metadata types
	LesserTimelineMetadata,
	LesserNotificationMetadata,

	// Presence types
	UserPresence,
	SessionInfo,
	PresenceState,
	PresenceConfig,

	// Legacy streaming types (for backward compatibility)
	StreamingEdit,
	JsonPatch,
} from './stores/types';

// Unified Model Types
export type {
	// Core unified types
	UnifiedAccount,
	UnifiedStatus,
	UnifiedNotification,
	SourceMetadata,

	// Media and content types
	MediaAttachment,
	MediaMeta,
	Mention,
	Tag,
	TagHistory,
	CustomEmoji,
	Poll,
	PollOption,
	AccountField,
	AccountRelationship,
	AdminReport,

	// Streaming types
	StreamingUpdate,
	StreamingDelete,
	StreamingEdit as UnifiedStreamingEdit,

	// Error handling
	MappingError,
	MapperResult,
	BatchMapperResult,

	// Validation types
	ValidationRule,
	FieldValidator,
} from './mappers';

// Mastodon API Types
export type {
	// Account types
	MastodonAccount,
	MastodonField,
	MastodonEmoji,
	MastodonRelationship,

	// Status types
	MastodonStatus,
	MastodonMediaAttachment,
	MastodonMediaMeta,
	MastodonMention,
	MastodonTag,
	MastodonTagHistory,
	MastodonPreviewCard,
	MastodonPoll,
	MastodonPollOption,

	// Notification types
	MastodonNotification,
	MastodonReport,

	// Streaming types
	MastodonStreamingEvent,

	// Instance types
	MastodonInstance,
	MastodonRule,

	// Error types
	MastodonError,

	// Pagination types
	MastodonPaginationParams,
	MastodonPaginatedResponse,
} from './mappers';

// Lesser GraphQL API Types
export type {
	// Base GraphQL types
	LesserGraphQLResponse,
	LesserGraphQLError,

	// Account types
	LesserAccountFragment,
	LesserProfileField,
	LesserEmojiFragment,
	LesserRelationshipFragment,

	// Post types
	LesserPostFragment,
	LesserInteractionCounts,
	LesserUserInteractions,
	LesserMediaFragment,
	LesserMediaMetadata,
	LesserMentionFragment,
	LesserHashtagFragment,
	LesserTrendingData,
	LesserPollFragment,
	LesserPollOption,

	// Notification types
	LesserNotificationFragment,
	LesserAdminReportFragment,

	// Timeline types
	LesserTimelineConnection,
	LesserTimelineEdge,
	LesserPageInfo,

	// Streaming types
	LesserStreamingUpdate,
	LesserStreamingData,
	LesserPostStreamingData,
	LesserNotificationStreamingData,
	LesserAccountStreamingData,
	LesserDeleteStreamingData,

	// Query types
	LesserGetTimelineQuery,
	LesserGetTimelineVariables,
	LesserGetNotificationsQuery,
	LesserGetNotificationsVariables,

	// Mutation types
	LesserCreatePostMutation,
	LesserCreatePostVariables,

	// Instance types
	LesserInstanceInfo,

	// Error types
	LesserValidationError,
	LesserMutationError,

	// Subscription types
	LesserTimelineSubscription,
	LesserNotificationSubscription,
} from './mappers';

// Lesser-specific fragment types
export type {
	LesserReputationFragment,
	LesserVouchFragment,
	LesserCommunityNoteFragment,
	LesserQuoteContextFragment,
	LesserAIAnalysisFragment,
} from './mappers/lesser/types.js';
