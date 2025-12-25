/**
 * Lesser GraphQL API payload mappers
 * Transform Lesser GraphQL API responses to unified models
 */
import type {
	LesserAccountFragment,
	LesserPostFragment,
	LesserNotificationFragment,
	LesserRelationshipFragment,
	LesserStreamingUpdate,
	LesserObjectFragment,
	LesserGraphQLResponse,
	LesserTimelineConnection,
} from './types.js';
import type {
	UnifiedAccount,
	UnifiedStatus,
	UnifiedNotification,
	StreamingUpdate,
	StreamingDelete,
	StreamingEdit,
	MapperResult,
	BatchMapperResult,
} from '../../models/unified.js';
/**
 * Map Lesser account to unified account model
 */
export declare function mapLesserAccount(
	account: LesserAccountFragment,
	relationship?: LesserRelationshipFragment
): MapperResult<UnifiedAccount>;
/**
 * Map a basic Lesser Object to UnifiedStatus
 * Used for notifications and other contexts where we have a basic Object, not a full LesserPostFragment
 */
export declare function mapLesserObject(obj: LesserObjectFragment): MapperResult<UnifiedStatus>;
/**
 * Map Lesser post to unified status model
 */
export declare function mapLesserPost(post: LesserPostFragment): MapperResult<UnifiedStatus>;
/**
 * Map Lesser notification to unified notification model
 */
export declare function mapLesserNotification(
	notification: LesserNotificationFragment
): MapperResult<UnifiedNotification>;
/**
 * Map Lesser streaming update to unified streaming models
 */
export declare function mapLesserStreamingUpdate(
	update: LesserStreamingUpdate
): MapperResult<StreamingUpdate | StreamingDelete | StreamingEdit>;
/**
 * Handle GraphQL response wrapper and extract data
 */
export declare function handleLesserGraphQLResponse<T>(
	response: LesserGraphQLResponse<T>
): MapperResult<T>;
/**
 * Map Lesser timeline connection to posts
 */
export declare function mapLesserTimelineConnection(
	connection: LesserTimelineConnection
): BatchMapperResult<UnifiedStatus>;
/**
 * Batch map multiple Lesser accounts
 */
export declare function batchMapLesserAccounts(
	accounts: LesserAccountFragment[],
	relationships?: Map<string, LesserRelationshipFragment>
): BatchMapperResult<UnifiedAccount>;
/**
 * Batch map multiple Lesser posts
 */
export declare function batchMapLesserPosts(
	posts: LesserPostFragment[]
): BatchMapperResult<UnifiedStatus>;
/**
 * Batch map multiple Lesser notifications
 */
export declare function batchMapLesserNotifications(
	notifications: LesserNotificationFragment[]
): BatchMapperResult<UnifiedNotification>;
//# sourceMappingURL=mappers.d.ts.map
