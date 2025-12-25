/**
 * Mastodon REST API v1 payload mappers
 * Transform Mastodon API responses to unified models
 */
import type {
	MastodonAccount,
	MastodonStatus,
	MastodonNotification,
	MastodonRelationship,
	MastodonStreamingEvent,
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
 * Map Mastodon account to unified account model
 */
export declare function mapMastodonAccount(
	account: MastodonAccount,
	relationship?: MastodonRelationship
): MapperResult<UnifiedAccount>;
/**
 * Map Mastodon status to unified status model
 */
export declare function mapMastodonStatus(status: MastodonStatus): MapperResult<UnifiedStatus>;
/**
 * Map Mastodon notification to unified notification model
 */
export declare function mapMastodonNotification(
	notification: MastodonNotification
): MapperResult<UnifiedNotification>;
/**
 * Map Mastodon streaming event to unified streaming update
 */
export declare function mapMastodonStreamingEvent(
	event: MastodonStreamingEvent
): MapperResult<StreamingUpdate | StreamingDelete | StreamingEdit>;
/**
 * Batch map multiple Mastodon accounts
 */
export declare function batchMapMastodonAccounts(
	accounts: MastodonAccount[],
	relationships?: Map<string, MastodonRelationship>
): BatchMapperResult<UnifiedAccount>;
/**
 * Batch map multiple Mastodon statuses
 */
export declare function batchMapMastodonStatuses(
	statuses: MastodonStatus[]
): BatchMapperResult<UnifiedStatus>;
/**
 * Batch map multiple Mastodon notifications
 */
export declare function batchMapMastodonNotifications(
	notifications: MastodonNotification[]
): BatchMapperResult<UnifiedNotification>;
//# sourceMappingURL=mappers.d.ts.map
