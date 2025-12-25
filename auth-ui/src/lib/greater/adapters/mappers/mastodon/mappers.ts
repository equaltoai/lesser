/**
 * Mastodon REST API v1 payload mappers
 * Transform Mastodon API responses to unified models
 */

import type {
	MastodonAccount,
	MastodonStatus,
	MastodonNotification,
	MastodonMediaAttachment,
	MastodonMention,
	MastodonTag,
	MastodonEmoji,
	MastodonPoll,
	MastodonRelationship,
	MastodonStreamingEvent,
} from './types.js';

import type {
	UnifiedAccount,
	UnifiedStatus,
	UnifiedNotification,
	AdminReport,
	MediaAttachment,
	Mention,
	Tag,
	CustomEmoji,
	Poll,
	AccountRelationship,
	StreamingUpdate,
	StreamingDelete,
	StreamingEdit,
	MapperResult,
	BatchMapperResult,
	MappingError,
	SourceMetadata,
} from '../../models/unified.js';

const mapStreamingEventType = (
	eventType: MastodonStreamingEvent['event']
): StreamingUpdate['type'] => {
	switch (eventType) {
		case 'notification':
			return 'notification';
		case 'conversation':
		case 'announcement':
		case 'filters_changed':
			return 'status';
		case 'update':
			return 'status';
		default:
			return 'status';
	}
};

/**
 * Create source metadata for Mastodon payloads
 */
function createMastodonMetadata(rawPayload?: unknown): SourceMetadata {
	return {
		source: 'mastodon',
		apiVersion: 'v1',
		lastUpdated: Date.now(),
		rawPayload,
	};
}

/**
 * Create a mapping error with context
 */
function createMappingError(
	type: MappingError['type'],
	message: string,
	payload?: unknown,
	fieldPath?: string
): MappingError {
	const error = new Error(message) as MappingError;
	error.type = type;
	error.payload = payload;
	error.fieldPath = fieldPath;
	error.source = {
		api: 'mastodon',
		version: 'v1',
	};
	return error;
}

/**
 * Safely extract string field with fallback
 */
function safeString(value: unknown, fallback = ''): string {
	return typeof value === 'string' ? value : fallback;
}

/**
 * Safely extract number field with fallback
 */
function safeNumber(value: unknown, fallback = 0): number {
	return typeof value === 'number' ? value : fallback;
}

/**
 * Safely extract boolean field with fallback
 */
function safeBoolean(value: unknown, fallback = false): boolean {
	return typeof value === 'boolean' ? value : fallback;
}

/**
 * Map Mastodon account to unified account model
 */
export function mapMastodonAccount(
	account: MastodonAccount,
	relationship?: MastodonRelationship
): MapperResult<UnifiedAccount> {
	const startTime = performance.now();

	try {
		if (!account || typeof account.id !== 'string') {
			return {
				success: false,
				error: createMappingError('validation', 'Invalid account: missing or invalid id', account),
			};
		}

		const unified: UnifiedAccount = {
			id: account.id,
			username: safeString(account.username),
			acct: safeString(account.acct),
			displayName: safeString(account.display_name),
			note: safeString(account.note),
			avatar: safeString(account.avatar),
			header: safeString(account.header),
			createdAt: safeString(account.created_at),
			followersCount: safeNumber(account.followers_count),
			followingCount: safeNumber(account.following_count),
			statusesCount: safeNumber(account.statuses_count),
			locked: safeBoolean(account.locked),
			verified: safeBoolean(account.verified),
			bot: safeBoolean(account.bot),
			fields: (account.fields || []).map((field) => ({
				name: safeString(field.name),
				value: safeString(field.value),
				verifiedAt: field.verified_at || undefined,
			})),
			relationship: relationship ? mapMastodonRelationship(relationship) : undefined,
			metadata: createMastodonMetadata(account),
		};

		const endTime = performance.now();
		return {
			success: true,
			data: unified,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(account).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map account: ${error instanceof Error ? error.message : 'Unknown error'}`,
				account
			),
		};
	}
}

/**
 * Map Mastodon relationship to unified relationship model
 */
function mapMastodonRelationship(relationship: MastodonRelationship): AccountRelationship {
	return {
		following: safeBoolean(relationship.following),
		followedBy: safeBoolean(relationship.followed_by),
		requested: safeBoolean(relationship.requested),
		blocking: safeBoolean(relationship.blocking),
		muting: safeBoolean(relationship.muting),
		mutingNotifications: safeBoolean(relationship.muting_notifications),
		domainBlocking: safeBoolean(relationship.domain_blocking),
		endorsed: safeBoolean(relationship.endorsed),
		note: safeString(relationship.note) || undefined,
	};
}

/**
 * Map Mastodon status to unified status model
 */
export function mapMastodonStatus(status: MastodonStatus): MapperResult<UnifiedStatus> {
	const startTime = performance.now();

	try {
		if (!status || typeof status.id !== 'string') {
			return {
				success: false,
				error: createMappingError('validation', 'Invalid status: missing or invalid id', status),
			};
		}

		if (!status.account) {
			return {
				success: false,
				error: createMappingError('validation', 'Invalid status: missing account', status),
			};
		}

		const accountResult = mapMastodonAccount(status.account);
		if (!accountResult.success || !accountResult.data) {
			return {
				success: false,
				error:
					accountResult.error ??
					createMappingError('transformation', 'Failed to map status account', status, 'account'),
			};
		}

		const reblogResult = status.reblog ? mapMastodonStatus(status.reblog) : undefined;
		const reblogStatus = reblogResult && reblogResult.success ? reblogResult.data : undefined;

		const unified: UnifiedStatus = {
			id: status.id,
			createdAt: safeString(status.created_at),
			content: safeString(status.content),
			spoilerText: safeString(status.spoiler_text) || undefined,
			visibility: status.visibility || 'public',
			sensitive: safeBoolean(status.sensitive),
			language: status.language || undefined,
			account: accountResult.data,
			mediaAttachments: (status.media_attachments || []).map(mapMastodonMediaAttachment),
			mentions: (status.mentions || []).map(mapMastodonMention),
			tags: (status.tags || []).map(mapMastodonTag),
			emojis: (status.emojis || []).map(mapMastodonEmoji),
			inReplyTo: status.in_reply_to_id
				? {
						id: status.in_reply_to_id,
						accountId: safeString(status.in_reply_to_account_id),
					}
				: undefined,
			reblog: reblogStatus,
			repliesCount: safeNumber(status.replies_count),
			reblogsCount: safeNumber(status.reblogs_count),
			favouritesCount: safeNumber(status.favourites_count),
			favourited: safeBoolean(status.favourited),
			reblogged: safeBoolean(status.reblogged),
			bookmarked: safeBoolean(status.bookmarked),
			pinned: safeBoolean(status.pinned),
			editedAt: status.edited_at || undefined,
			poll: status.poll ? mapMastodonPoll(status.poll) : undefined,
			metadata: createMastodonMetadata(status),
		};

		const endTime = performance.now();
		return {
			success: true,
			data: unified,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(status).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map status: ${error instanceof Error ? error.message : 'Unknown error'}`,
				status
			),
		};
	}
}

/**
 * Map Mastodon media attachment to unified model
 */
function mapMastodonMediaAttachment(attachment: MastodonMediaAttachment): MediaAttachment {
	return {
		id: safeString(attachment.id),
		type: attachment.type || 'unknown',
		url: safeString(attachment.url),
		previewUrl: safeString(attachment.preview_url) || undefined,
		remoteUrl: attachment.remote_url || undefined,
		description: attachment.description || undefined,
		blurhash: attachment.blurhash || undefined,
		meta: attachment.meta
			? {
					original: attachment.meta.original
						? {
								width: safeNumber(attachment.meta.original.width),
								height: safeNumber(attachment.meta.original.height),
								size: attachment.meta.original.size,
								aspect: safeNumber(attachment.meta.original.aspect),
								duration: safeNumber(attachment.meta.original.duration) || undefined,
								fps: attachment.meta.original.frame_rate
									? parseFloat(attachment.meta.original.frame_rate)
									: undefined,
								bitrate: safeNumber(attachment.meta.original.bitrate) || undefined,
							}
						: undefined,
					small: attachment.meta.small
						? {
								width: safeNumber(attachment.meta.small.width),
								height: safeNumber(attachment.meta.small.height),
								size: attachment.meta.small.size,
								aspect: safeNumber(attachment.meta.small.aspect),
							}
						: undefined,
				}
			: undefined,
	};
}

/**
 * Map Mastodon mention to unified model
 */
function mapMastodonMention(mention: MastodonMention): Mention {
	return {
		id: safeString(mention.id),
		username: safeString(mention.username),
		acct: safeString(mention.acct),
		url: safeString(mention.url),
	};
}

/**
 * Map Mastodon tag to unified model
 */
function mapMastodonTag(tag: MastodonTag): Tag {
	return {
		name: safeString(tag.name),
		url: safeString(tag.url),
		history: tag.history?.map((history) => ({
			day: safeString(history.day),
			uses: safeString(history.uses),
			accounts: safeString(history.accounts),
		})),
	};
}

/**
 * Map Mastodon emoji to unified model
 */
function mapMastodonEmoji(emoji: MastodonEmoji): CustomEmoji {
	return {
		shortcode: safeString(emoji.shortcode),
		staticUrl: safeString(emoji.static_url),
		url: safeString(emoji.url),
		visibleInPicker: safeBoolean(emoji.visible_in_picker),
		category: emoji.category || undefined,
	};
}

/**
 * Map Mastodon poll to unified model
 */
function mapMastodonPoll(poll: MastodonPoll): Poll {
	return {
		id: safeString(poll.id),
		expiresAt: poll.expires_at || undefined,
		expired: safeBoolean(poll.expired),
		multiple: safeBoolean(poll.multiple),
		votesCount: safeNumber(poll.votes_count),
		votersCount: poll.voters_count !== undefined ? safeNumber(poll.voters_count) : undefined,
		options: (poll.options || []).map((option) => ({
			title: safeString(option.title),
			votesCount: option.votes_count !== undefined ? safeNumber(option.votes_count) : undefined,
		})),
		voted: poll.voted !== undefined ? safeBoolean(poll.voted) : undefined,
		ownVotes: poll.own_votes || undefined,
	};
}

/**
 * Map Mastodon notification to unified notification model
 */
export function mapMastodonNotification(
	notification: MastodonNotification
): MapperResult<UnifiedNotification> {
	const startTime = performance.now();

	try {
		if (!notification || typeof notification.id !== 'string') {
			return {
				success: false,
				error: createMappingError(
					'validation',
					'Invalid notification: missing or invalid id',
					notification
				),
			};
		}

		if (!notification.account) {
			return {
				success: false,
				error: createMappingError(
					'validation',
					'Invalid notification: missing account',
					notification
				),
			};
		}

		const accountResult = mapMastodonAccount(notification.account);
		if (!accountResult.success || !accountResult.data) {
			return {
				success: false,
				error:
					accountResult.error ??
					createMappingError(
						'transformation',
						'Failed to map notification account',
						notification,
						'account'
					),
			};
		}

		let status: UnifiedStatus | undefined;
		if (notification.status) {
			const statusResult = mapMastodonStatus(notification.status);
			if (statusResult.success && statusResult.data) {
				status = statusResult.data;
			}
		}

		let report: AdminReport | undefined;
		if (notification.report) {
			const targetAccountResult = mapMastodonAccount(notification.report.target_account);
			if (targetAccountResult.success && targetAccountResult.data) {
				report = {
					id: safeString(notification.report.id),
					targetAccount: targetAccountResult.data,
					account: accountResult.data,
					comment: safeString(notification.report.comment),
					actionTaken: safeBoolean(notification.report.action_taken),
					createdAt: safeString(notification.report.created_at),
				};
			}
		}

		const unified: UnifiedNotification = {
			id: notification.id,
			type: notification.type,
			createdAt: safeString(notification.created_at),
			account: accountResult.data,
			status,
			report,
			metadata: createMastodonMetadata(notification),
		};

		const endTime = performance.now();
		return {
			success: true,
			data: unified,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(notification).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map notification: ${error instanceof Error ? error.message : 'Unknown error'}`,
				notification
			),
		};
	}
}

/**
 * Map Mastodon streaming event to unified streaming update
 */
export function mapMastodonStreamingEvent(
	event: MastodonStreamingEvent
): MapperResult<StreamingUpdate | StreamingDelete | StreamingEdit> {
	const startTime = performance.now();

	try {
		if (!event || typeof event.event !== 'string') {
			return {
				success: false,
				error: createMappingError(
					'validation',
					'Invalid streaming event: missing or invalid event type',
					event
				),
			};
		}

		const timestamp = Date.now();
		const metadata = createMastodonMetadata(event);

		// Handle delete events
		if (event.event === 'delete') {
			const deleteUpdate: StreamingDelete = {
				id: safeString(event.payload),
				itemType: 'status',
				timestamp,
			};

			const endTime = performance.now();
			return {
				success: true,
				data: deleteUpdate,
				metrics: {
					mappingTimeMs: endTime - startTime,
					fieldsProcessed: 2,
					fallbacksUsed: 0,
				},
			};
		}

		// Parse payload JSON for other events
		let payload: unknown;
		try {
			payload = JSON.parse(event.payload);
		} catch {
			return {
				success: false,
				error: createMappingError(
					'validation',
					'Invalid streaming event: payload is not valid JSON',
					event,
					'payload'
				),
			};
		}

		// Handle update events (new/edited status)
		if (event.event === 'update') {
			if (typeof payload === 'object' && payload && 'id' in payload) {
				const statusResult = mapMastodonStatus(payload as MastodonStatus);
				if (statusResult.success && statusResult.data) {
					const editUpdate: StreamingEdit = {
						id: statusResult.data.id,
						data: statusResult.data,
						timestamp,
						editType: 'content',
					};

					const endTime = performance.now();
					return {
						success: true,
						data: editUpdate,
						metrics: {
							mappingTimeMs: endTime - startTime,
							fieldsProcessed: Object.keys(payload as object).length,
							fallbacksUsed: 0,
						},
					};
				}
			}
		}

		// Handle generic streaming update
		const streamingUpdate: StreamingUpdate = {
			type: mapStreamingEventType(event.event),
			payload,
			stream: event.stream.join(','),
			timestamp,
			metadata,
		};

		const endTime = performance.now();
		return {
			success: true,
			data: streamingUpdate,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(event).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map streaming event: ${error instanceof Error ? error.message : 'Unknown error'}`,
				event
			),
		};
	}
}

/**
 * Batch map multiple Mastodon accounts
 */
export function batchMapMastodonAccounts(
	accounts: MastodonAccount[],
	relationships?: Map<string, MastodonRelationship>
): BatchMapperResult<UnifiedAccount> {
	const startTime = performance.now();
	const successful: UnifiedAccount[] = [];
	const failed: { payload: unknown; error: MappingError }[] = [];

	for (const account of accounts) {
		const relationship = relationships?.get(account.id);
		const result = mapMastodonAccount(account, relationship);

		if (result.success && result.data) {
			successful.push(result.data);
		} else if (result.error) {
			failed.push({ payload: account, error: result.error });
		}
	}

	const endTime = performance.now();
	return {
		successful,
		failed,
		totalProcessed: accounts.length,
		processingTimeMs: endTime - startTime,
	};
}

/**
 * Batch map multiple Mastodon statuses
 */
export function batchMapMastodonStatuses(
	statuses: MastodonStatus[]
): BatchMapperResult<UnifiedStatus> {
	const startTime = performance.now();
	const successful: UnifiedStatus[] = [];
	const failed: { payload: unknown; error: MappingError }[] = [];

	for (const status of statuses) {
		const result = mapMastodonStatus(status);

		if (result.success && result.data) {
			successful.push(result.data);
		} else if (result.error) {
			failed.push({ payload: status, error: result.error });
		}
	}

	const endTime = performance.now();
	return {
		successful,
		failed,
		totalProcessed: statuses.length,
		processingTimeMs: endTime - startTime,
	};
}

/**
 * Batch map multiple Mastodon notifications
 */
export function batchMapMastodonNotifications(
	notifications: MastodonNotification[]
): BatchMapperResult<UnifiedNotification> {
	const startTime = performance.now();
	const successful: UnifiedNotification[] = [];
	const failed: { payload: unknown; error: MappingError }[] = [];

	for (const notification of notifications) {
		const result = mapMastodonNotification(notification);

		if (result.success && result.data) {
			successful.push(result.data);
		} else if (result.error) {
			failed.push({ payload: notification, error: result.error });
		}
	}

	const endTime = performance.now();
	return {
		successful,
		failed,
		totalProcessed: notifications.length,
		processingTimeMs: endTime - startTime,
	};
}
