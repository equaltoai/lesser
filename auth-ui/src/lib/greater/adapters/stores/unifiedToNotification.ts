/**
 * Unified Model to Notification Converter
 *
 * Converts UnifiedNotification to store Notification with Lesser metadata
 */

import type { UnifiedNotification } from '../models/unified.js';
import type { Notification as StoreNotification, LesserNotificationMetadata } from './types.js';

/**
 * Map unified notification priority based on type
 */
function getNotificationPriority(type: UnifiedNotification['type']): StoreNotification['priority'] {
	switch (type) {
		case 'cost_alert':
			return 'urgent';
		case 'moderation_action':
		case 'trust_update':
			return 'high';
		case 'admin.report':
		case 'admin.sign_up':
			return 'high';
		default:
			return 'normal';
	}
}

/**
 * Get notification display title
 */
function getNotificationTitle(notification: UnifiedNotification): string {
	switch (notification.type) {
		case 'quote':
			return 'New Quote';
		case 'community_note':
			return 'Community Note Added';
		case 'trust_update':
			return 'Trust Score Updated';
		case 'cost_alert':
			return 'Cost Alert';
		case 'moderation_action':
			return 'Moderation Action';
		case 'mention':
			return 'New Mention';
		case 'reblog':
			return 'Boosted Your Post';
		case 'favourite':
			return 'Favorited Your Post';
		case 'follow':
			return 'New Follower';
		case 'follow_request':
			return 'Follow Request';
		default:
			return 'Notification';
	}
}

/**
 * Get notification display message
 */
function getNotificationMessage(notification: UnifiedNotification): string {
	const displayName = notification.account.displayName || notification.account.username;

	switch (notification.type) {
		case 'quote':
			return `${displayName} quoted your post`;
		case 'community_note':
			return `${displayName} added a community note to your post`;
		case 'trust_update':
			return notification.trustUpdate?.reason || 'Your trust score was updated';
		case 'cost_alert':
			return notification.costAlert?.message || 'Cost threshold exceeded';
		case 'moderation_action':
			return `Moderation action: ${notification.moderationAction?.action}`;
		case 'mention':
			return `${displayName} mentioned you`;
		case 'reblog':
			return `${displayName} boosted your post`;
		case 'favourite':
			return `${displayName} favorited your post`;
		case 'follow':
			return `${displayName} followed you`;
		case 'follow_request':
			return `${displayName} requested to follow you`;
		default:
			return 'You have a new notification';
	}
}

/**
 * Convert UnifiedNotification to store Notification
 */
export function unifiedNotificationToStoreNotification(
	notification: UnifiedNotification
): Omit<StoreNotification, 'id' | 'timestamp'> {
	const lesserMetadata: LesserNotificationMetadata = {};

	// Populate Lesser-specific metadata
	if (notification.quoteStatus) {
		lesserMetadata.quoteStatus = {
			id: notification.quoteStatus.id,
			content: notification.quoteStatus.content,
			author:
				notification.quoteStatus.account.displayName || notification.quoteStatus.account.username,
		};
	}

	if (notification.communityNote) {
		lesserMetadata.communityNote = notification.communityNote;
	}

	if (notification.trustUpdate) {
		lesserMetadata.trustUpdate = notification.trustUpdate;
	}

	if (notification.costAlert) {
		lesserMetadata.costAlert = notification.costAlert;
	}

	if (notification.moderationAction) {
		lesserMetadata.moderationAction = notification.moderationAction;
	}

	return {
		type: notification.type as StoreNotification['type'],
		title: getNotificationTitle(notification),
		message: getNotificationMessage(notification),
		isRead: notification.read || false,
		priority: getNotificationPriority(notification.type),
		metadata:
			Object.keys(lesserMetadata).length > 0
				? {
						lesser: lesserMetadata,
						// Also store the full notification for reference
						unifiedNotification: notification,
					}
				: {
						unifiedNotification: notification,
					},
	};
}

/**
 * Batch convert multiple UnifiedNotifications
 */
export function unifiedNotificationsToStoreNotifications(
	notifications: UnifiedNotification[]
): Array<Omit<StoreNotification, 'id' | 'timestamp'>> {
	return notifications.map(unifiedNotificationToStoreNotification);
}
