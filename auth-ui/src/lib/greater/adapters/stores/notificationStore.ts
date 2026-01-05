/**
 * Notification Store - Reactive state management for notifications with filtering and streaming
 * Built for Svelte 5 runes compatibility with fallback support
 */

import type {
	NotificationStore,
	NotificationState,
	Notification,
	NotificationConfig,
	NotificationFilter,
	NotificationPageInfo,
} from './types';
import { unifiedNotificationToStoreNotification } from './unifiedToNotification.js';
import { mapLesserNotification } from '../mappers/lesser/mappers.js';
import {
	convertGraphQLActorToLesserAccount,
	convertGraphQLObjectToLesser,
} from '../mappers/lesser/graphqlConverters.js';
import type { LesserNotificationFragment } from '../mappers/lesser/types.js';
import type { UnifiedNotification } from '../models/unified.js';
import type {
	NotificationStreamSubscription,
	TrustUpdatesSubscription,
	CostAlertsSubscription,
	ModerationEventsSubscription,
	ModerationAlertsSubscription,
	ModerationQueueUpdateSubscription,
} from '../graphql/generated/types.js';
import type { WebSocketEvent } from '../types.js';
import { fetchNotificationPage, normalizeNotificationType } from './fetchHelpers.js';

type NotificationStreamPayload = NotificationStreamSubscription['notificationStream'];
type TrustUpdatePayload = TrustUpdatesSubscription['trustUpdates'];
type CostAlertPayload = CostAlertsSubscription['costAlerts'];
type ModerationEventPayload = ModerationEventsSubscription['moderationEvents'];
type ModerationAlertPayload = ModerationAlertsSubscription['moderationAlerts'];
type ModerationQueuePayload = ModerationQueueUpdateSubscription['moderationQueueUpdate'];

function convertNotificationStreamPayload(
	payload: NotificationStreamPayload | null
): LesserNotificationFragment | null {
	if (!payload || typeof payload.id !== 'string') {
		return null;
	}

	const typeValue =
		(payload as Record<string, unknown>)['type'] ??
		(payload as Record<string, unknown>)['notificationType'];

	const createdAtValue =
		(payload as Record<string, unknown>)['createdAt'] ??
		(payload as Record<string, unknown>)['timestamp'];

	const accountValue =
		(payload as Record<string, unknown>)['account'] ??
		(payload as Record<string, unknown>)['triggerAccount'] ??
		(payload as Record<string, unknown>)['actor'];

	const statusValue =
		(payload as Record<string, unknown>)['status'] ??
		(payload as Record<string, unknown>)['object'] ??
		(payload as Record<string, unknown>)['post'];

	const readValue =
		(payload as Record<string, unknown>)['read'] ?? (payload as Record<string, unknown>)['isRead'];

	const triggerAccount = convertGraphQLActorToLesserAccount(accountValue);
	if (!triggerAccount) {
		return null;
	}

	const statusFragment = statusValue ? convertGraphQLObjectToLesser(statusValue) : null;

	const createdAt = (() => {
		if (typeof createdAtValue === 'string') {
			return createdAtValue;
		}
		if (typeof createdAtValue === 'number' && Number.isFinite(createdAtValue)) {
			return new Date(createdAtValue).toISOString();
		}
		return new Date(0).toISOString();
	})();

	return {
		id: payload.id,
		notificationType: normalizeNotificationType(typeValue),
		createdAt,
		triggerAccount,
		status: statusFragment ?? undefined,
		adminReport: undefined,
		isRead: typeof readValue === 'boolean' ? readValue : undefined,
	};
}

// Simple reactive state implementation that works everywhere
class ReactiveState<T> {
	private _value: T;
	private _subscribers = new Set<(value: T) => void>();

	constructor(initialValue: T) {
		this._value = initialValue;
	}

	get value(): T {
		return this._value;
	}

	set value(newValue: T) {
		this._value = newValue;
		this._subscribers.forEach((callback) => {
			try {
				callback(newValue);
			} catch (error) {
				console.error('Error in reactive state subscriber:', error);
			}
		});
	}

	subscribe(callback: (value: T) => void): () => void {
		this._subscribers.add(callback);
		// Call immediately with current value
		callback(this._value);

		return () => {
			this._subscribers.delete(callback);
		};
	}

	update(updater: (current: T) => T): void {
		this.value = updater(this._value);
	}
}

export function createNotificationStore(config: NotificationConfig): NotificationStore {
	// Create reactive state
	const state = new ReactiveState<NotificationState>({
		notifications: config.initialNotifications || [],
		filteredNotifications: [],
		unreadCounts: {},
		totalUnread: 0,
		filter: {
			types: undefined,
			readStatus: 'all',
			priority: undefined,
			dateRange: undefined,
			query: undefined,
		},
		isLoading: false,
		error: null,
		isStreaming: false,
		pageInfo: {
			endCursor: config.initialPageInfo?.endCursor ?? null,
			hasNextPage: config.initialPageInfo?.hasNextPage ?? true,
		},
		lastSync: null,
	});

	// Initialize computed values
	updateDerivedValues();

	// Auto-dismiss notification timers
	const dismissalTimers = new Map<string, ReturnType<typeof setTimeout>>();

	// Transport event handlers
	let streamingUnsubscribers: (() => void)[] = [];
	let updateDebounceTimer: ReturnType<typeof setTimeout> | null = null;
	let pendingNotifications: Notification[] = [];
	const pageSize = config.pageSize ?? 20;

	function enforceNotificationLimit(notifications: Notification[]): {
		trimmed: Notification[];
		removed: Notification[];
	} {
		if (!config.maxNotifications || notifications.length <= config.maxNotifications) {
			return { trimmed: notifications, removed: [] };
		}

		let toRemove = notifications.length - config.maxNotifications;
		const removalQueue: Notification[] = [];

		const sortedRead = notifications
			.filter((notification) => notification.isRead)
			.sort((a, b) => a.timestamp - b.timestamp);

		for (const notification of sortedRead) {
			if (toRemove === 0) break;
			removalQueue.push(notification);
			toRemove--;
		}

		if (toRemove > 0) {
			const remaining = notifications
				.filter((notification) => !removalQueue.includes(notification))
				.sort((a, b) => a.timestamp - b.timestamp);

			for (const notification of remaining) {
				if (toRemove === 0) break;
				removalQueue.push(notification);
				toRemove--;
			}
		}

		const removalIds = new Set(removalQueue.map((notification) => notification.id));
		const trimmed = notifications.filter((notification) => !removalIds.has(notification.id));

		return {
			trimmed,
			removed: removalQueue,
		};
	}

	function updateDerivedValues(notifications?: Notification[], filter?: NotificationFilter): void {
		const currentState = state.value;
		const notificationsToProcess = notifications || currentState.notifications;
		const filterToUse = filter || currentState.filter;
		const { types, readStatus, priority, dateRange, query } = filterToUse;

		// Filter notifications
		let filtered = [...notificationsToProcess];

		// Filter by types
		if (types && types.length > 0) {
			filtered = filtered.filter((n) => types.includes(n.type));
		}

		// Filter by read status
		if (readStatus === 'read') {
			filtered = filtered.filter((n) => n.isRead);
		} else if (readStatus === 'unread') {
			filtered = filtered.filter((n) => !n.isRead);
		}

		// Filter by priority
		if (priority && priority.length > 0) {
			filtered = filtered.filter((n) => priority.includes(n.priority));
		}

		// Filter by date range
		if (dateRange) {
			const start = dateRange.start.getTime();
			const end = dateRange.end.getTime();
			filtered = filtered.filter((n) => n.timestamp >= start && n.timestamp <= end);
		}

		// Filter by search query
		const normalizedQuery = query?.trim().toLowerCase();
		if (normalizedQuery) {
			filtered = filtered.filter(
				(n) =>
					n.title.toLowerCase().includes(normalizedQuery) ||
					n.message.toLowerCase().includes(normalizedQuery)
			);
		}

		// Sort by timestamp (newest first)
		filtered = filtered.sort((a, b) => b.timestamp - a.timestamp);

		// Calculate unread counts by type
		const unreadCounts: Record<string, number> = {};
		notificationsToProcess.forEach((notification) => {
			if (!notification.isRead) {
				unreadCounts[notification.type] = (unreadCounts[notification.type] || 0) + 1;
				unreadCounts['all'] = (unreadCounts['all'] || 0) + 1;
			}
		});

		// Update state with computed values only if they've changed
		const needsUpdate =
			JSON.stringify(currentState.filteredNotifications) !== JSON.stringify(filtered) ||
			JSON.stringify(currentState.unreadCounts) !== JSON.stringify(unreadCounts) ||
			currentState.totalUnread !== (unreadCounts['all'] || 0);

		if (needsUpdate) {
			state.update((current) => ({
				...current,
				filteredNotifications: filtered,
				unreadCounts,
				totalUnread: unreadCounts['all'] || 0,
			}));
		}
	}

	function normalizePageInfo(info?: Partial<NotificationPageInfo> | null): NotificationPageInfo {
		return {
			endCursor: info?.endCursor ?? null,
			hasNextPage: info?.hasNextPage ?? false,
		};
	}

	function scheduleAutoDismiss(notification: Notification): void {
		if (notification.dismissAfter && notification.dismissAfter > 0) {
			const timer = setTimeout(() => {
				removeNotification(notification.id);
				dismissalTimers.delete(notification.id);
			}, notification.dismissAfter);

			dismissalTimers.set(notification.id, timer);
		}
	}

	function clearAutoDismiss(notificationId: string): void {
		const timer = dismissalTimers.get(notificationId);
		if (timer) {
			clearTimeout(timer);
			dismissalTimers.delete(notificationId);
		}
	}

	function processPendingNotifications(): void {
		if (pendingNotifications.length === 0) return;

		const notifications = [...pendingNotifications];
		pendingNotifications = [];

		// Deduplicate notifications by ID and ignore already stored items
		const notificationMap = new Map<string, Notification>();
		notifications.forEach((notification) => notificationMap.set(notification.id, notification));

		const newNotifications = Array.from(notificationMap.values()).filter(
			(notification) =>
				!state.value.notifications.some((existing) => existing.id === notification.id)
		);

		if (newNotifications.length === 0) {
			return;
		}

		const newNotificationIds = new Set(newNotifications.map((notification) => notification.id));
		const mergedNotifications = [...state.value.notifications, ...newNotifications];
		const { trimmed, removed } = enforceNotificationLimit(mergedNotifications);

		state.update((current) => ({
			...current,
			notifications: trimmed,
		}));

		trimmed
			.filter((notification) => newNotificationIds.has(notification.id))
			.forEach(scheduleAutoDismiss);

		removed.forEach((notification) => clearAutoDismiss(notification.id));

		updateDerivedValues(trimmed);
	}

	function scheduleNotificationUpdate(notification: Notification): void {
		pendingNotifications.push(notification);

		if (updateDebounceTimer) {
			clearTimeout(updateDebounceTimer);
		}

		updateDebounceTimer = setTimeout(() => {
			processPendingNotifications();
			updateDebounceTimer = null;
		}, config.updateDebounceMs || 100);
	}

	function resolveTimestamp(value?: string | number): number {
		if (typeof value === 'number' && Number.isFinite(value)) {
			return value;
		}

		if (typeof value === 'string') {
			const parsed = Date.parse(value);
			if (!Number.isNaN(parsed)) {
				return parsed;
			}
		}

		return Date.now();
	}

	function convertUnifiedNotificationToStore(unified: UnifiedNotification): Notification {
		const base = unifiedNotificationToStoreNotification(unified);
		return {
			...base,
			id: unified.id,
			timestamp: resolveTimestamp(unified.createdAt),
			isRead: base.isRead ?? false,
		};
	}

	function upsertNotification(notification: Notification): void {
		const existing = state.value.notifications.find((item) => item.id === notification.id);
		if (existing) {
			updateNotificationFromStream(notification.id, {
				title: notification.title,
				message: notification.message,
				priority: notification.priority,
				isRead: notification.isRead,
				metadata: notification.metadata,
				dismissAfter: notification.dismissAfter,
				actions: notification.actions,
				timestamp: notification.timestamp,
			});
			return;
		}

		scheduleNotificationUpdate(notification);
	}

	function handleNotificationStreamEvent(payload: NotificationStreamPayload | null): void {
		if (!payload) return;

		const fragment = convertNotificationStreamPayload(payload);
		if (!fragment) {
			console.warn('[NotificationStore] Unable to convert notification stream payload');
			return;
		}

		const result = mapLesserNotification(fragment);
		if (!result.success || !result.data) {
			if (result.error) {
				console.warn('[NotificationStore] Failed to map notification stream payload', result.error);
			}
			return;
		}

		const notification = convertUnifiedNotificationToStore(result.data);
		scheduleNotificationUpdate(notification);
	}

	function handleTrustUpdateEvent(payload: TrustUpdatePayload | null): void {
		if (!payload) return;

		const trustNotifications = state.value.notifications.filter(
			(notification) => notification.type === 'trust_update'
		);
		const reason = `Category: ${payload.category}`;
		const timestamp = resolveTimestamp(payload.updatedAt);

		if (trustNotifications.length > 0) {
			trustNotifications.forEach((notification) => {
				const existingLesser = notification.metadata?.lesser ?? {};
				const previousScore =
					existingLesser.trustUpdate?.newScore ?? existingLesser.trustUpdate?.previousScore;

				const metadata = {
					...(notification.metadata ?? {}),
					lesser: {
						...existingLesser,
						trustUpdate: {
							newScore: payload.score,
							previousScore,
							reason,
						},
					},
				};

				updateNotificationFromStream(notification.id, {
					message: `Trust score is now ${payload.score}`,
					metadata,
					isRead: false,
					timestamp,
				});
			});
			return;
		}

		const notification: Notification = {
			id: `trust_${payload.updatedAt}_${payload.from?.id ?? 'global'}`,
			type: 'trust_update',
			title: 'Trust Score Updated',
			message: `Trust score is now ${payload.score}`,
			timestamp,
			isRead: false,
			priority: 'high',
			metadata: {
				lesser: {
					trustUpdate: {
						newScore: payload.score,
						previousScore: undefined,
						reason,
					},
				},
			},
		};

		scheduleNotificationUpdate(notification);
	}

	function handleCostAlertEvent(payload: CostAlertPayload | null): void {
		if (!payload) return;

		const notification: Notification = {
			id: payload.id,
			type: 'cost_alert',
			title: 'Cost Alert',
			message: payload.message ?? `Cost threshold exceeded: ${payload.amount}/${payload.threshold}`,
			timestamp: resolveTimestamp(payload.timestamp),
			isRead: false,
			priority: 'urgent',
			metadata: {
				lesser: {
					costAlert: {
						amount: payload.amount,
						threshold: payload.threshold,
					},
				},
			},
		};

		upsertNotification(notification);
	}

	function handleModerationEvent(payload: ModerationEventPayload | null): void {
		if (!payload) return;

		const evidence = payload.evidence?.join(', ') || 'Automatic moderation decision';

		const notification: Notification = {
			id: payload.id,
			type: 'moderation_action',
			title: 'Moderation Decision',
			message: `Decision: ${payload.decision}`,
			timestamp: resolveTimestamp(payload.timestamp),
			isRead: false,
			priority: 'high',
			metadata: {
				lesser: {
					moderationAction: {
						action: payload.decision,
						reason: evidence,
						statusId: payload.object?.id,
					},
				},
			},
		};

		upsertNotification(notification);
	}

	function handleModerationAlertEvent(payload: ModerationAlertPayload | null): void {
		if (!payload) return;

		const notification: Notification = {
			id: payload.id,
			type: 'moderation_action',
			title: 'Moderation Alert',
			message: `${payload.severity.toLowerCase()} severity alert: ${payload.matchedText}`,
			timestamp: resolveTimestamp(payload.timestamp),
			isRead: false,
			priority: 'high',
			metadata: {
				lesser: {
					moderationAction: {
						action: payload.suggestedAction,
						reason: payload.matchedText,
						statusId: payload.content?.id,
					},
				},
			},
		};

		upsertNotification(notification);
	}

	function handleModerationQueueEvent(payload: ModerationQueuePayload | null): void {
		if (!payload) return;

		const notification: Notification = {
			id: payload.id,
			type: 'moderation_action',
			title: 'Moderation Queue Update',
			message: `${payload.priority} priority item with ${payload.reportCount} reports`,
			timestamp: resolveTimestamp(payload.deadline),
			isRead: false,
			priority: 'high',
			metadata: {
				lesser: {
					moderationAction: {
						action: `Queue severity ${payload.severity}`,
						reason: `Reports: ${payload.reportCount}`,
						statusId: payload.content?.id,
					},
				},
			},
		};

		upsertNotification(notification);
	}

	// Store methods
	function addNotification(notificationData: Omit<Notification, 'id' | 'timestamp'>): string {
		const id = generateId();
		const timestamp = Date.now();

		const notification: Notification = {
			...notificationData,
			id,
			timestamp,
			isRead: false,
			dismissAfter: notificationData.dismissAfter || config.defaultDismissAfter,
		};

		const mergedNotifications = [...state.value.notifications, notification];
		const { trimmed, removed } = enforceNotificationLimit(mergedNotifications);
		const isNotificationKept = trimmed.some((item) => item.id === id);

		state.update((current) => ({
			...current,
			notifications: trimmed,
		}));

		if (isNotificationKept) {
			scheduleAutoDismiss(notification);
		}

		removed.forEach((item) => clearAutoDismiss(item.id));
		updateDerivedValues(trimmed);

		if (isNotificationKept && state.value.isStreaming && config.transportManager) {
			try {
				config.transportManager.send({
					type: 'notification_add',
					data: notification,
				});
			} catch (error) {
				state.update((current) => ({
					...current,
					error: error as Error,
				}));
				console.error('Failed to send notification to server:', error);
			}
		}

		return id;
	}

	function markAsRead(id: string): boolean {
		const currentState = state.value;
		const notificationIndex = currentState.notifications.findIndex((n) => n.id === id);
		if (notificationIndex === -1) return false;

		const notification = currentState.notifications[notificationIndex];
		if (!notification) return false;
		if (notification.isRead) return true; // Already read

		const updatedNotification = { ...notification, isRead: true };
		const newNotifications = currentState.notifications.map((n, index) =>
			index === notificationIndex ? updatedNotification : n
		);

		state.update((current) => ({
			...current,
			notifications: newNotifications,
		}));

		// Clear auto-dismiss timer since it's now read
		clearAutoDismiss(id);
		updateDerivedValues();

		// Send to server if streaming is active
		if (currentState.isStreaming && config.transportManager) {
			try {
				config.transportManager.send({
					type: 'notification_read',
					data: { id, isRead: true },
				});
			} catch (error) {
				state.update((current) => ({
					...current,
					error: error as Error,
				}));
				console.error('Failed to mark notification as read on server:', error);
			}
		}

		return true;
	}

	function markAllAsRead(): void {
		const unreadNotifications = state.value.notifications.filter((n) => !n.isRead);

		if (unreadNotifications.length === 0) return;

		state.update((current) => ({
			...current,
			notifications: current.notifications.map((n) => ({ ...n, isRead: true })),
		}));

		// Clear all auto-dismiss timers
		unreadNotifications.forEach((n) => clearAutoDismiss(n.id));
		updateDerivedValues();

		// Send to server if streaming is active
		if (state.value.isStreaming && config.transportManager) {
			try {
				config.transportManager.send({
					type: 'notifications_read_all',
					data: { notificationIds: unreadNotifications.map((n) => n.id) },
				});
			} catch (error) {
				state.update((current) => ({
					...current,
					error: error as Error,
				}));
				console.error('Failed to mark all notifications as read on server:', error);
			}
		}
	}

	function removeNotificationInternal(id: string): boolean {
		const notificationExists = state.value.notifications.some((n) => n.id === id);
		if (!notificationExists) return false;

		state.update((current) => ({
			...current,
			notifications: current.notifications.filter((n) => n.id !== id),
		}));

		clearAutoDismiss(id);
		return true;
	}

	function removeNotification(id: string): boolean {
		const success = removeNotificationInternal(id);

		if (success) {
			updateDerivedValues();

			// Send to server if streaming is active
			if (state.value.isStreaming && config.transportManager) {
				try {
					config.transportManager.send({
						type: 'notification_remove',
						data: { id },
					});
				} catch (error) {
					state.update((current) => ({
						...current,
						error: error as Error,
					}));
					console.error('Failed to remove notification on server:', error);
				}
			}
		}

		return success;
	}

	function clearAll(): void {
		// Clear all auto-dismiss timers
		state.value.notifications.forEach((n) => clearAutoDismiss(n.id));

		const notificationIds = state.value.notifications.map((n) => n.id);
		state.update((current) => ({
			...current,
			notifications: [],
		}));

		updateDerivedValues();

		// Send to server if streaming is active
		if (state.value.isStreaming && config.transportManager) {
			try {
				config.transportManager.send({
					type: 'notifications_clear_all',
					data: { notificationIds },
				});
			} catch (error) {
				state.update((current) => ({
					...current,
					error: error as Error,
				}));
				console.error('Failed to clear all notifications on server:', error);
			}
		}
	}

	function updateFilter(newFilter: Partial<NotificationFilter>): void {
		const currentState = state.value;
		const updatedFilter = { ...currentState.filter, ...newFilter };

		state.update((current) => ({
			...current,
			filter: updatedFilter,
		}));

		updateDerivedValues(currentState.notifications, updatedFilter);
	}

	async function fetchPage(after: string | null, replaceExisting = false): Promise<void> {
		if (!config.adapter) {
			const error = new Error('Notification adapter is required to fetch pages');
			state.update((current) => ({
				...current,
				isLoading: false,
				error,
			}));
			throw error;
		}

		const result = await fetchNotificationPage({
			adapter: config.adapter,
			pageSize,
			after,
		});

		if (replaceExisting) {
			state.update((current) => ({
				...current,
				notifications: [],
				filteredNotifications: [],
				unreadCounts: {},
				totalUnread: 0,
			}));
		}

		result.items.map(convertUnifiedNotificationToStore).forEach(scheduleNotificationUpdate);

		processPendingNotifications();

		state.update((current) => ({
			...current,
			isLoading: false,
			error: null,
			lastSync: Date.now(),
			pageInfo: normalizePageInfo(result.pageInfo),
		}));
	}

	async function loadMore(): Promise<void> {
		if (state.value.isLoading) return;
		const { pageInfo } = state.value;
		if (!pageInfo.hasNextPage && pageInfo.endCursor) {
			return;
		}

		state.update((current) => ({
			...current,
			isLoading: true,
			error: null,
		}));

		try {
			await fetchPage(state.value.pageInfo.endCursor ?? null, false);
		} catch (error) {
			state.update((current) => ({
				...current,
				error: error as Error,
				isLoading: false,
			}));
			throw error;
		}
	}

	async function refresh(): Promise<void> {
		if (state.value.isLoading) return;

		state.update((current) => ({
			...current,
			isLoading: true,
			error: null,
			pageInfo: {
				...normalizePageInfo(state.value.pageInfo),
				endCursor: null,
			},
			notifications: [],
		}));

		try {
			await fetchPage(null, true);
		} catch (error) {
			state.update((current) => ({
				...current,
				error: error as Error,
				isLoading: false,
			}));
			throw error;
		}
	}

	function startStreaming(): void {
		if (state.value.isStreaming || !config.transportManager) return;

		state.update((current) => ({
			...current,
			isStreaming: true,
			error: null,
		}));

		const notificationStreamHandler = config.transportManager.on(
			'notificationStream',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as NotificationStreamPayload | null;
				handleNotificationStreamEvent(payload);
			}
		);

		const trustUpdatesHandler = config.transportManager.on(
			'trustUpdates',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as TrustUpdatePayload | null;
				handleTrustUpdateEvent(payload);
			}
		);

		const costAlertsHandler = config.transportManager.on('costAlerts', (event: WebSocketEvent) => {
			const payload = (event?.data ?? null) as CostAlertPayload | null;
			handleCostAlertEvent(payload);
		});

		const moderationEventsHandler = config.transportManager.on(
			'moderationEvents',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as ModerationEventPayload | null;
				handleModerationEvent(payload);
			}
		);

		const moderationAlertsHandler = config.transportManager.on(
			'moderationAlerts',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as ModerationAlertPayload | null;
				handleModerationAlertEvent(payload);
			}
		);

		const moderationQueueHandler = config.transportManager.on(
			'moderationQueueUpdate',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as ModerationQueuePayload | null;
				handleModerationQueueEvent(payload);
			}
		);

		const errorHandler = config.transportManager.on('error', (event) => {
			state.update((current) => ({
				...current,
				error: event.error || new Error('Streaming connection error'),
			}));
		});

		const closeHandler = config.transportManager.on('close', () => {
			state.update((current) => ({
				...current,
				isStreaming: false,
			}));
		});

		streamingUnsubscribers.push(
			notificationStreamHandler,
			trustUpdatesHandler,
			costAlertsHandler,
			moderationEventsHandler,
			moderationAlertsHandler,
			moderationQueueHandler,
			errorHandler,
			closeHandler
		);

		try {
			config.transportManager.connect();
		} catch (error) {
			state.update((current) => ({
				...current,
				error: error as Error,
				isStreaming: false,
			}));
		}
	}

	function stopStreaming(): void {
		state.update((current) => ({
			...current,
			isStreaming: false,
		}));

		// Clean up event subscriptions
		streamingUnsubscribers.forEach((unsubscribe) => unsubscribe());
		streamingUnsubscribers = [];

		// Clear pending updates
		if (updateDebounceTimer) {
			clearTimeout(updateDebounceTimer);
			updateDebounceTimer = null;
		}
		pendingNotifications = [];
	}

	function updateNotificationFromStream(id: string, updates: Partial<Notification>): void {
		const currentState = state.value;
		const notificationIndex = currentState.notifications.findIndex((n) => n.id === id);
		if (notificationIndex === -1) return;

		const notification = currentState.notifications[notificationIndex];
		if (!notification) return;

		const updatedNotification = { ...notification, ...updates };
		const newNotifications = currentState.notifications.map((n, index) =>
			index === notificationIndex ? updatedNotification : n
		);

		state.update((current) => ({
			...current,
			notifications: newNotifications,
		}));

		// Handle read status change
		if ('isRead' in updates && updates.isRead && !notification.isRead) {
			clearAutoDismiss(id);
		}

		// Handle dismiss timer change
		if ('dismissAfter' in updates) {
			clearAutoDismiss(id);
			if (updates.dismissAfter) {
				scheduleAutoDismiss(updatedNotification);
			}
		}

		updateDerivedValues();
	}

	function subscribe(callback: (value: NotificationState) => void): () => void {
		return state.subscribe(callback);
	}

	function get(): NotificationState {
		return state.value;
	}

	function destroy(): void {
		stopStreaming();

		// Clear all auto-dismiss timers
		dismissalTimers.forEach((timer) => clearTimeout(timer));
		dismissalTimers.clear();

		// Clear debounce timer
		if (updateDebounceTimer) {
			clearTimeout(updateDebounceTimer);
			updateDebounceTimer = null;
		}

		// Clear state
		state.update(() => ({
			notifications: [],
			filteredNotifications: [],
			unreadCounts: {},
			totalUnread: 0,
			filter: {
				types: undefined,
				readStatus: 'all',
				priority: undefined,
				dateRange: undefined,
				query: undefined,
			},
			isLoading: false,
			error: null,
			isStreaming: false,
			pageInfo: {
				endCursor: null,
				hasNextPage: true,
			},
			lastSync: null,
		}));
	}

	function generateId(): string {
		return `notification_${globalThis.crypto.randomUUID()}`;
	}

	// Lesser-specific notification helpers
	function getQuoteNotifications(): Notification[] {
		return state.value.notifications.filter((n) => n.type === 'quote');
	}

	function getCommunityNoteNotifications(): Notification[] {
		return state.value.notifications.filter((n) => n.type === 'community_note');
	}

	function getTrustUpdateNotifications(): Notification[] {
		return state.value.notifications.filter((n) => n.type === 'trust_update');
	}

	function getCostAlertNotifications(): Notification[] {
		return state.value.notifications.filter((n) => n.type === 'cost_alert');
	}

	function getModerationActionNotifications(): Notification[] {
		return state.value.notifications.filter((n) => n.type === 'moderation_action');
	}

	function getUnreadLesserNotifications(): Notification[] {
		const lesserTypes = [
			'quote',
			'community_note',
			'trust_update',
			'cost_alert',
			'moderation_action',
		];
		return state.value.notifications.filter((n) => lesserTypes.includes(n.type) && !n.isRead);
	}

	return {
		subscribe,
		destroy,
		get,
		addNotification,
		markAsRead,
		markAllAsRead,
		removeNotification,
		clearAll,
		updateFilter,
		loadMore,
		refresh,
		startStreaming,
		stopStreaming,
		// Lesser-specific methods
		getQuoteNotifications,
		getCommunityNoteNotifications,
		getTrustUpdateNotifications,
		getCostAlertNotifications,
		getModerationActionNotifications,
		getUnreadLesserNotifications,
	};
}
