/**
 * Unified Model to Notification Converter
 *
 * Converts UnifiedNotification to store Notification with Lesser metadata
 */
import type { UnifiedNotification } from '../models/unified.js';
import type { Notification as StoreNotification } from './types.js';
/**
 * Convert UnifiedNotification to store Notification
 */
export declare function unifiedNotificationToStoreNotification(
	notification: UnifiedNotification
): Omit<StoreNotification, 'id' | 'timestamp'>;
/**
 * Batch convert multiple UnifiedNotifications
 */
export declare function unifiedNotificationsToStoreNotifications(
	notifications: UnifiedNotification[]
): Array<Omit<StoreNotification, 'id' | 'timestamp'>>;
//# sourceMappingURL=unifiedToNotification.d.ts.map
