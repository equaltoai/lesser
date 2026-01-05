import type { LesserGraphQLAdapter } from '../graphql';
import {
	convertGraphQLActorToLesserAccount,
	convertGraphQLObjectToLesser,
} from '../mappers/lesser/graphqlConverters.js';
import { mapLesserNotification, mapLesserObject } from '../mappers/lesser/mappers.js';
import type { LesserNotificationFragment } from '../mappers/lesser/types.js';
import type { UnifiedNotification, UnifiedStatus } from '../models/unified.js';
import type { NotificationsQuery, TimelineQuery } from '../graphql/generated/types.js';
import type { NotificationPageInfo, TimelinePageInfo, TimelineSource } from './types.js';

const DEFAULT_PAGE_INFO: TimelinePageInfo = {
	endCursor: null,
	hasNextPage: false,
};

const DEFAULT_NOTIFICATION_PAGE_INFO: NotificationPageInfo = {
	endCursor: null,
	hasNextPage: false,
};

export interface TimelinePageResult {
	items: UnifiedStatus[];
	pageInfo: TimelinePageInfo;
}

export interface NotificationPageResult {
	items: UnifiedNotification[];
	pageInfo: NotificationPageInfo;
}

export function normalizeNotificationType(
	value: unknown
): LesserNotificationFragment['notificationType'] {
	if (typeof value === 'string') {
		const normalized = value.replace(/\./g, '_').toUpperCase();
		switch (normalized) {
			case 'MENTION':
			case 'FOLLOW':
			case 'FOLLOW_REQUEST':
			case 'SHARE':
			case 'FAVORITE':
			case 'POST':
			case 'POLL_ENDED':
			case 'STATUS_UPDATE':
			case 'ADMIN_SIGNUP':
			case 'ADMIN_REPORT':
			case 'QUOTE':
			case 'COMMUNITY_NOTE':
			case 'TRUST_UPDATE':
			case 'COST_ALERT':
			case 'MODERATION_ACTION':
				return normalized;
		}
	}
	return 'MENTION';
}

export async function fetchTimelinePage(options: {
	adapter: LesserGraphQLAdapter;
	source: TimelineSource;
	pageSize?: number;
	after?: string | null;
}): Promise<TimelinePageResult> {
	const { adapter, source, pageSize = 20, after } = options;
	const pagination = { first: pageSize, after: after ?? undefined };

	let connection: TimelineQuery['timeline'] | undefined;

	switch (source.type) {
		case 'home':
			connection = await adapter.fetchHomeTimeline(pagination);
			break;
		case 'local':
			connection = await adapter.fetchPublicTimeline(pagination, 'LOCAL');
			break;
		case 'federated':
			connection = await adapter.fetchPublicTimeline(pagination, 'PUBLIC');
			break;
		case 'direct':
			connection = await adapter.fetchDirectTimeline(pagination);
			break;
		case 'list': {
			if (!source.id) throw new Error('List timeline requires an id');
			connection = await adapter.fetchListTimeline(source.id, pagination);
			break;
		}
		case 'hashtag': {
			const hashtag = source.hashtag ?? source.id;
			if (!hashtag) throw new Error('Hashtag timeline requires a hashtag or id');
			connection = await adapter.fetchHashtagTimeline(hashtag, pagination);
			break;
		}
		case 'actor': {
			if (!source.id) throw new Error('Actor timeline requires an id');
			connection = await adapter.fetchActorTimeline(source.id, pagination);
			break;
		}
		default:
			throw new Error(`Unsupported timeline type: ${source.type}`);
	}

	const edges = connection?.edges ?? [];
	const items: UnifiedStatus[] = [];

	for (const edge of edges) {
		const lesserObject = convertGraphQLObjectToLesser(edge?.node);
		if (!lesserObject) continue;

		const mapped = mapLesserObject(lesserObject);
		if (mapped.success && mapped.data) {
			items.push(mapped.data);
		}
	}

	const pageInfo: TimelinePageInfo = {
		endCursor: connection?.pageInfo?.endCursor ?? DEFAULT_PAGE_INFO.endCursor,
		hasNextPage: connection?.pageInfo?.hasNextPage ?? DEFAULT_PAGE_INFO.hasNextPage,
	};

	return { items, pageInfo };
}

export async function fetchNotificationPage(options: {
	adapter: LesserGraphQLAdapter;
	pageSize?: number;
	after?: string | null;
}): Promise<NotificationPageResult> {
	const { adapter, pageSize = 20, after } = options;
	const connection = await adapter.fetchNotifications({
		first: pageSize,
		after: after ?? undefined,
	});

	const edges: NotificationsQuery['notifications']['edges'] = connection?.edges ?? [];
	const items: UnifiedNotification[] = [];

	for (const edge of edges) {
		const node = edge?.node;
		if (!node || typeof node !== 'object') continue;

		const triggerAccount = convertGraphQLActorToLesserAccount(
			(node as Record<string, unknown>)['account'] ?? (node as Record<string, unknown>)['actor']
		);
		if (!triggerAccount) continue;

		const status = convertGraphQLObjectToLesser(
			(node as Record<string, unknown>)['status'] ??
				(node as Record<string, unknown>)['object'] ??
				(node as Record<string, unknown>)['post']
		);

		const createdAtValue = (node as Record<string, unknown>)['createdAt'];
		const notification: LesserNotificationFragment = {
			id: (node as Record<string, unknown>)['id'] as string,
			notificationType: normalizeNotificationType(
				(node as Record<string, unknown>)['type'] ??
					(node as Record<string, unknown>)['notificationType']
			),
			createdAt: typeof createdAtValue === 'string' ? createdAtValue : new Date().toISOString(),
			triggerAccount,
			status: status ?? undefined,
			isRead: Boolean((node as Record<string, unknown>)['read']),
		};

		const mapped = mapLesserNotification(notification);
		if (mapped.success && mapped.data) {
			items.push(mapped.data);
		}
	}

	const pageInfo: NotificationPageInfo = {
		endCursor: connection?.pageInfo?.endCursor ?? DEFAULT_NOTIFICATION_PAGE_INFO.endCursor,
		hasNextPage: connection?.pageInfo?.hasNextPage ?? DEFAULT_NOTIFICATION_PAGE_INFO.hasNextPage,
	};

	return { items, pageInfo };
}
