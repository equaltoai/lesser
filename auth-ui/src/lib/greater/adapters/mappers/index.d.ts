/**
 * API mappers for Fediverse protocols
 * Transform API-specific payloads to unified models
 */
import type { MappingError, SourceMetadata } from '../models/unified.js';
type UnknownRecord = Record<string, unknown>;
export type {
	UnifiedAccount,
	UnifiedStatus,
	UnifiedNotification,
	SourceMetadata,
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
	StreamingUpdate,
	StreamingDelete,
	StreamingEdit,
	MappingError,
	MapperResult,
	BatchMapperResult,
	ValidationRule,
	FieldValidator,
} from '../models/unified.js';
export type {
	MastodonAccount,
	MastodonField,
	MastodonEmoji,
	MastodonRelationship,
	MastodonStatus,
	MastodonMediaAttachment,
	MastodonMediaMeta,
	MastodonMention,
	MastodonTag,
	MastodonTagHistory,
	MastodonPreviewCard,
	MastodonPoll,
	MastodonPollOption,
	MastodonApplication,
	MastodonFilterResult,
	MastodonFilter,
	MastodonFilterKeyword,
	MastodonFilterStatus,
	MastodonNotification,
	MastodonReport,
	MastodonStreamingEvent,
	MastodonInstance,
	MastodonRule,
	MastodonContext,
	MastodonSearchResults,
	MastodonError,
	MastodonPaginationParams,
	MastodonPaginatedResponse,
} from './mastodon/types.js';
export {
	mapMastodonAccount,
	mapMastodonStatus,
	mapMastodonNotification,
	mapMastodonStreamingEvent,
	batchMapMastodonAccounts,
	batchMapMastodonStatuses,
	batchMapMastodonNotifications,
} from './mastodon/mappers.js';
export {
	isMastodonAccount,
	isMastodonStatus,
	isMastodonNotification,
	isMastodonStreamingEvent,
	MASTODON_VISIBILITY_VALUES,
	MASTODON_MEDIA_TYPES,
	MASTODON_NOTIFICATION_TYPES,
} from './mastodon/types.js';
export type {
	LesserGraphQLResponse,
	LesserGraphQLError,
	LesserAccountFragment,
	LesserProfileField,
	LesserEmojiFragment,
	LesserRelationshipFragment,
	LesserReputationFragment,
	LesserVouchFragment,
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
	LesserNotificationFragment,
	LesserAdminReportFragment,
	LesserCommunityNoteFragment,
	LesserQuoteContextFragment,
	LesserAIAnalysisFragment,
	LesserTimelineConnection,
	LesserTimelineEdge,
	LesserPageInfo,
	LesserStreamingUpdate,
	LesserStreamingData,
	LesserPostStreamingData,
	LesserNotificationStreamingData,
	LesserAccountStreamingData,
	LesserDeleteStreamingData,
	LesserGetTimelineQuery,
	LesserGetTimelineVariables,
	LesserGetNotificationsQuery,
	LesserGetNotificationsVariables,
	LesserCreatePostMutation,
	LesserCreatePostVariables,
	LesserInstanceInfo,
	LesserValidationError,
	LesserMutationError,
	LesserTimelineSubscription,
	LesserNotificationSubscription,
} from './lesser/types.js';
export {
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
} from './lesser/mappers.js';
export { convertGraphQLObjectToLesser } from './lesser/graphqlConverters.js';
export {
	isLesserGraphQLResponse,
	isLesserAccountFragment,
	isLesserPostFragment,
	isLesserNotificationFragment,
	isLesserStreamingUpdate,
	LESSER_VISIBILITY_VALUES,
	LESSER_MEDIA_TYPES,
	LESSER_NOTIFICATION_TYPES,
	LESSER_ACCOUNT_FRAGMENT,
	LESSER_POST_FRAGMENT,
	LESSER_NOTIFICATION_FRAGMENT,
} from './lesser/types.js';
export declare const MapperUtils: {
	/**
	 * Create a mapping error with consistent format
	 */
	readonly createError: (
		type: 'validation' | 'transformation' | 'missing_field' | 'unknown_format',
		message: string,
		payload?: unknown,
		fieldPath?: string,
		source?: {
			api: string;
			version: string;
			endpoint?: string;
		}
	) => MappingError;
	/**
	 * Safely extract and validate field values
	 */
	readonly safeExtract: {
		readonly string: (value: unknown, fallback?: string) => string;
		readonly number: (value: unknown, fallback?: number) => number;
		readonly boolean: (value: unknown, fallback?: boolean) => boolean;
		readonly array: <T>(value: unknown, fallback?: T[]) => T[];
	};
	/**
	 * Validate required fields in payload
	 */
	readonly validateRequired: (
		payload: UnknownRecord,
		requiredFields: string[]
	) => {
		valid: boolean;
		missing: string[];
	};
	/**
	 * Create source metadata
	 */
	readonly createMetadata: (
		source: 'mastodon' | 'lesser' | 'unknown',
		apiVersion: string,
		rawPayload?: unknown
	) => SourceMetadata;
};
//# sourceMappingURL=index.d.ts.map
