/**
 * API mappers for Fediverse protocols
 * Transform API-specific payloads to unified models
 */

import type { MappingError, SourceMetadata } from '../models/unified.js';

type UnknownRecord = Record<string, unknown>;

// Unified Models
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
	StreamingEdit,

	// Error handling
	MappingError,
	MapperResult,
	BatchMapperResult,

	// Validation types
	ValidationRule,
	FieldValidator,
} from '../models/unified.js';

// Mastodon REST v1 API
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
	MastodonApplication,
	MastodonFilterResult,
	MastodonFilter,
	MastodonFilterKeyword,
	MastodonFilterStatus,

	// Notification types
	MastodonNotification,
	MastodonReport,

	// Streaming types
	MastodonStreamingEvent,

	// Instance types
	MastodonInstance,
	MastodonRule,

	// Context types
	MastodonContext,
	MastodonSearchResults,

	// Error types
	MastodonError,

	// Pagination types
	MastodonPaginationParams,
	MastodonPaginatedResponse,
} from './mastodon/types.js';

// Mastodon mappers
export {
	mapMastodonAccount,
	mapMastodonStatus,
	mapMastodonNotification,
	mapMastodonStreamingEvent,
	batchMapMastodonAccounts,
	batchMapMastodonStatuses,
	batchMapMastodonNotifications,
} from './mastodon/mappers.js';

// Mastodon type guards
export {
	isMastodonAccount,
	isMastodonStatus,
	isMastodonNotification,
	isMastodonStreamingEvent,
	MASTODON_VISIBILITY_VALUES,
	MASTODON_MEDIA_TYPES,
	MASTODON_NOTIFICATION_TYPES,
} from './mastodon/types.js';

// Lesser GraphQL API
export type {
	// Base GraphQL types
	LesserGraphQLResponse,
	LesserGraphQLError,

	// Account types
	LesserAccountFragment,
	LesserProfileField,
	LesserEmojiFragment,
	LesserRelationshipFragment,
	LesserReputationFragment,
	LesserVouchFragment,

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
	LesserCommunityNoteFragment,
	LesserQuoteContextFragment,
	LesserAIAnalysisFragment,

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
} from './lesser/types.js';

// Lesser mappers
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

// Lesser type guards and constants
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

// Unified mapper utilities
export const MapperUtils = {
	/**
	 * Create a mapping error with consistent format
	 */
	createError(
		type: 'validation' | 'transformation' | 'missing_field' | 'unknown_format',
		message: string,
		payload?: unknown,
		fieldPath?: string,
		source?: { api: string; version: string; endpoint?: string }
	): MappingError {
		const error = new Error(message) as MappingError;
		error.type = type;
		error.payload = payload;
		error.fieldPath = fieldPath;
		error.source = source;
		return error;
	},

	/**
	 * Safely extract and validate field values
	 */
	safeExtract: {
		string: (value: unknown, fallback = ''): string => {
			return typeof value === 'string' ? value : fallback;
		},
		number: (value: unknown, fallback = 0): number => {
			return typeof value === 'number' ? value : fallback;
		},
		boolean: (value: unknown, fallback = false): boolean => {
			return typeof value === 'boolean' ? value : fallback;
		},
		array: <T>(value: unknown, fallback: T[] = []): T[] => {
			return Array.isArray(value) ? value : fallback;
		},
	},

	/**
	 * Validate required fields in payload
	 */
	validateRequired(
		payload: UnknownRecord,
		requiredFields: string[]
	): { valid: boolean; missing: string[] } {
		const missing: string[] = [];

		for (const field of requiredFields) {
			if (!Object.prototype.hasOwnProperty.call(payload, field)) {
				missing.push(field);
				continue;
			}

			const value = payload[field];
			if (value === null || value === undefined) {
				missing.push(field);
			}
		}

		return {
			valid: missing.length === 0,
			missing,
		};
	},

	/**
	 * Create source metadata
	 */
	createMetadata(
		source: 'mastodon' | 'lesser' | 'unknown',
		apiVersion: string,
		rawPayload?: unknown
	): SourceMetadata {
		return {
			source,
			apiVersion,
			lastUpdated: Date.now(),
			rawPayload,
		};
	},
} as const;
