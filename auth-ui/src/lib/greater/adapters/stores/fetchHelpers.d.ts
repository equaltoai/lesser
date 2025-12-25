import type { LesserGraphQLAdapter } from '../graphql';
import type { LesserNotificationFragment } from '../mappers/lesser/types.js';
import type { UnifiedNotification, UnifiedStatus } from '../models/unified.js';
import type { NotificationPageInfo, TimelinePageInfo, TimelineSource } from './types.js';
export interface TimelinePageResult {
	items: UnifiedStatus[];
	pageInfo: TimelinePageInfo;
}
export interface NotificationPageResult {
	items: UnifiedNotification[];
	pageInfo: NotificationPageInfo;
}
export declare function normalizeNotificationType(
	value: unknown
): LesserNotificationFragment['notificationType'];
export declare function fetchTimelinePage(options: {
	adapter: LesserGraphQLAdapter;
	source: TimelineSource;
	pageSize?: number;
	after?: string | null;
}): Promise<TimelinePageResult>;
export declare function fetchNotificationPage(options: {
	adapter: LesserGraphQLAdapter;
	pageSize?: number;
	after?: string | null;
}): Promise<NotificationPageResult>;
//# sourceMappingURL=fetchHelpers.d.ts.map
