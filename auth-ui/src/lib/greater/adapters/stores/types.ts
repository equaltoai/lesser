/**
 * Store types for reactive state management with Svelte 5 runes
 */

import type { TransportManager } from '../TransportManager';
import type { LesserGraphQLAdapter } from '../graphql';
import type { WebSocketPool } from '../WebSocketPool';

// Base store types
export interface BaseStore<T = unknown> {
	/** Subscribe to store updates */
	subscribe(callback: (value: T) => void): () => void;
	/** Cleanup store resources */
	destroy(): void;
	/** Get current store value */
	get(): T;
}

// Timeline store types

/**
 * Lesser-specific metadata for timeline items
 */
export interface LesserTimelineMetadata {
	/** Estimated cost in microcents */
	estimatedCost?: number;
	/** Moderation score (0-1) */
	moderationScore?: number;
	/** Tombstone flag (Lesser soft delete) */
	isDeleted?: boolean;
	/** When the item was deleted */
	deletedAt?: string;
	/** Original type before tombstone */
	formerType?: string;
	/** Has community notes attached */
	hasCommunityNotes?: boolean;
	/** Community notes count */
	communityNotesCount?: number;
	/** Is a quote post */
	isQuote?: boolean;
	/** Quote count */
	quoteCount?: number;
	/** Hashtags associated with the timeline item */
	hashtags?: string[];
	/** Is quoteable */
	quoteable?: boolean;
	/** Quote permission level */
	quotePermission?: 'EVERYONE' | 'FOLLOWERS' | 'NONE';
	/** Trust score of author */
	authorTrustScore?: number;
	/** AI analysis result */
	aiModerationAction?: 'NONE' | 'FLAG' | 'HIDE' | 'REMOVE' | 'SHADOW_BAN' | 'REVIEW';
	/** AI confidence score */
	aiConfidence?: number;
	/** Identifiers of lists containing this item */
	listMemberships?: string[];
	/** Human-readable titles for lists containing this item */
	listTitles?: Record<string, string>;
	/** Latest relationship change affecting the author */
	relationshipStatus?: 'followed' | 'unfollowed' | 'blocked' | 'unblocked' | 'muted' | 'unmuted';
	/** Timestamp of the last relationship update */
	relationshipUpdatedAt?: number;
}

export interface TimelineItem {
	/** Unique identifier */
	id: string;
	/** Item type/category */
	type: 'status' | 'tombstone' | string;
	/** Creation timestamp */
	timestamp: number;
	/** Item content/data */
	content: unknown;
	/** Item metadata */
	metadata?: Record<string, unknown> & {
		/** Typed Lesser-specific metadata */
		lesser?: LesserTimelineMetadata;
	};
	/** Version for optimistic updates */
	version?: number;
	/** Temporary/optimistic flag */
	isOptimistic?: boolean;
}

export interface TimelineState {
	/** All timeline items sorted by timestamp */
	items: TimelineItem[];
	/** Items currently visible in viewport */
	visibleItems: TimelineItem[];
	/** Total item count */
	totalCount: number;
	/** Loading state */
	isLoading: boolean;
	/** Error state */
	error: Error | null;
	/** Streaming connection status */
	isStreaming: boolean;
	/** Last sync timestamp */
	lastSync: number | null;
	/** Pagination info from GraphQL */
	pageInfo: TimelinePageInfo;
	/** Virtualization window */
	virtualWindow: {
		startIndex: number;
		endIndex: number;
		itemHeight: number;
		containerHeight: number;
	};
}

export type TimelineSourceType =
	| 'home'
	| 'local'
	| 'federated'
	| 'direct'
	| 'list'
	| 'hashtag'
	| 'actor';

export interface TimelineSource {
	type: TimelineSourceType;
	id?: string;
	hashtag?: string;
	mediaOnly?: boolean;
}

export interface TimelinePageInfo {
	endCursor: string | null;
	hasNextPage: boolean;
}

export interface TimelineConfig {
	/** Transport manager for streaming updates */
	transportManager?: TransportManager;
	/** Optional WebSocket pool for streaming */
	webSocketPool?: WebSocketPool;
	/** Lesser GraphQL adapter for fetching timelines */
	adapter?: LesserGraphQLAdapter;
	/** Timeline source descriptor */
	timeline?: TimelineSource;
	/** Page size for GraphQL queries */
	pageSize?: number;
	/** Auth context (refreshed before operations that may require it) */
	authContext?: () => Promise<unknown>;
	/** Initial items to load */
	initialItems?: TimelineItem[];
	/** Initial page info to seed pagination */
	initialPageInfo?: Partial<TimelinePageInfo>;
	/** Item height for virtualization */
	itemHeight?: number;
	/** Container height for virtualization */
	containerHeight?: number;
	/** Number of items to render outside visible area */
	overscan?: number;
	/** Debounce time for batch updates */
	updateDebounceMs?: number;
	/** How deletions should be reflected in the timeline */
	deletionMode?: 'remove' | 'tombstone';
}

export interface StreamingEdit {
	/** Edit operation type */
	type: 'add' | 'replace' | 'delete' | 'patch';
	/** Target item ID */
	itemId: string;
	/** New/updated data */
	data?: unknown;
	/** Patch operations for partial updates */
	patches?: JsonPatch[];
	/** Operation timestamp */
	timestamp: number;
	/** User who made the edit */
	userId?: string;
}

export interface JsonPatch {
	op: 'add' | 'remove' | 'replace' | 'move' | 'copy' | 'test';
	path: string;
	value?: unknown;
	from?: string;
}

// Notification store types

/**
 * Lesser-specific notification metadata
 */
export interface LesserNotificationMetadata {
	/** For quote notifications */
	quoteStatus?: {
		id: string;
		content: string;
		author: string;
	};
	/** For community note notifications */
	communityNote?: {
		id: string;
		content: string;
		helpful: number;
		notHelpful: number;
	};
	/** For trust update notifications */
	trustUpdate?: {
		newScore: number;
		previousScore?: number;
		reason?: string;
	};
	/** For cost alert notifications */
	costAlert?: {
		amount: number;
		threshold: number;
	};
	/** For moderation action notifications */
	moderationAction?: {
		action: string;
		reason: string;
		statusId?: string;
	};
}

export interface Notification {
	/** Unique identifier */
	id: string;
	/** Notification type */
	type:
		| 'info'
		| 'success'
		| 'warning'
		| 'error'
		| 'system'
		| 'quote'
		| 'community_note'
		| 'trust_update'
		| 'cost_alert'
		| 'moderation_action';
	/** Notification title */
	title: string;
	/** Notification message */
	message: string;
	/** Creation timestamp */
	timestamp: number;
	/** Read status */
	isRead: boolean;
	/** Priority level */
	priority: 'low' | 'normal' | 'high' | 'urgent';
	/** Notification metadata */
	metadata?: Record<string, unknown> & {
		/** Typed Lesser-specific metadata */
		lesser?: LesserNotificationMetadata;
	};
	/** Auto-dismiss timeout in ms */
	dismissAfter?: number;
	/** Action buttons */
	actions?: NotificationAction[];
}

export interface NotificationAction {
	/** Action identifier */
	id: string;
	/** Action label */
	label: string;
	/** Action variant/style */
	variant: 'primary' | 'secondary' | 'danger';
	/** Action handler */
	handler: (notification: Notification) => void | Promise<void>;
}

export interface NotificationState {
	/** All notifications */
	notifications: Notification[];
	/** Filtered notifications based on current filter */
	filteredNotifications: Notification[];
	/** Unread count by type */
	unreadCounts: Record<string, number>;
	/** Total unread count */
	totalUnread: number;
	/** Current filter */
	filter: NotificationFilter;
	/** Loading state */
	isLoading: boolean;
	/** Error state */
	error: Error | null;
	/** Streaming connection status */
	isStreaming: boolean;
	/** Pagination info from GraphQL */
	pageInfo: NotificationPageInfo;
	/** Last sync timestamp */
	lastSync: number | null;
}

export interface NotificationFilter {
	/** Filter by notification types */
	types?: string[];
	/** Filter by read status */
	readStatus?: 'all' | 'read' | 'unread';
	/** Filter by priority */
	priority?: string[];
	/** Filter by date range */
	dateRange?: {
		start: Date;
		end: Date;
	};
	/** Search query */
	query?: string;
}

export interface NotificationConfig {
	/** Transport manager for streaming updates */
	transportManager: TransportManager;
	/** Lesser GraphQL adapter for fetching notifications */
	adapter?: LesserGraphQLAdapter;
	/** Page size for GraphQL queries */
	pageSize?: number;
	/** Auth context (refreshed before operations that may require it) */
	authContext?: () => Promise<unknown>;
	/** Initial notifications to load */
	initialNotifications?: Notification[];
	/** Initial page info to seed pagination */
	initialPageInfo?: Partial<NotificationPageInfo>;
	/** Auto-dismiss timeout for notifications */
	defaultDismissAfter?: number;
	/** Maximum number of notifications to keep */
	maxNotifications?: number;
	/** Debounce time for batch updates */
	updateDebounceMs?: number;
}

export interface NotificationPageInfo {
	endCursor: string | null;
	hasNextPage: boolean;
}

// Presence store types
export interface UserPresence {
	/** User identifier */
	userId: string;
	/** User display name */
	displayName: string;
	/** User avatar URL */
	avatar?: string;
	/** Online status */
	isOnline: boolean;
	/** Last seen timestamp */
	lastSeen: number;
	/** Current activity status */
	status: 'active' | 'idle' | 'busy' | 'away' | 'offline';
	/** Custom status message */
	statusMessage?: string;
	/** User location/context */
	location?: {
		page?: string;
		section?: string;
		coordinates?: { x: number; y: number };
	};
	/** Connection metadata */
	connection?: {
		sessionId: string;
		transportType: string;
		latency?: number;
	};
}

export interface SessionInfo {
	/** Session identifier */
	sessionId: string;
	/** User associated with session */
	userId: string;
	/** Session start time */
	startTime: number;
	/** Last activity time */
	lastActivity: number;
	/** Connection status */
	connectionStatus: 'connected' | 'reconnecting' | 'disconnected';
	/** Transport type used */
	transportType?: string;
	/** Connection quality metrics */
	metrics?: {
		latency: number;
		packetLoss: number;
		connectionStability: number;
	};
}

export interface PresenceState {
	/** Current user's presence */
	currentUser: UserPresence | null;
	/** All connected users */
	users: Map<string, UserPresence>;
	/** Active sessions */
	sessions: Map<string, SessionInfo>;
	/** Connection health status */
	connectionHealth: {
		status: 'healthy' | 'degraded' | 'poor' | 'disconnected';
		latency: number | null;
		lastHeartbeat: number | null;
		reconnectAttempts: number;
	};
	/** Presence statistics */
	stats: {
		totalUsers: number;
		onlineUsers: number;
		activeUsers: number;
		idleUsers: number;
	};
	/** Loading state */
	isLoading: boolean;
	/** Error state */
	error: Error | null;
	/** Streaming connection status */
	isStreaming: boolean;
}

export interface PresenceConfig {
	/** Transport manager for real-time updates */
	transportManager: TransportManager;
	/** Current user information */
	currentUser: {
		userId: string;
		displayName: string;
		avatar?: string;
	};
	/** Heartbeat interval in ms */
	heartbeatInterval?: number;
	/** User inactivity threshold in ms */
	inactivityThreshold?: number;
	/** Presence update debounce time in ms */
	updateDebounceMs?: number;
	/** Enable location tracking */
	enableLocationTracking?: boolean;
}

// Store factory types
export interface StoreFactory {
	/** Create timeline store instance */
	createTimelineStore(config: TimelineConfig): TimelineStore;
	/** Create notification store instance */
	createNotificationStore(config: NotificationConfig): NotificationStore;
	/** Create presence store instance */
	createPresenceStore(config: PresenceConfig): PresenceStore;
}

// Store interfaces
export interface TimelineStore extends BaseStore<TimelineState> {
	/** Add new timeline item */
	addItem(item: Omit<TimelineItem, 'id' | 'timestamp'>): string;
	/** Replace existing item */
	replaceItem(id: string, item: Partial<TimelineItem>): boolean;
	/** Delete item */
	deleteItem(id: string): boolean;
	/** Delete a status via adapter + apply deletion semantics */
	deleteStatus(id: string): Promise<void>;
	/** Apply streaming edit */
	applyStreamingEdit(edit: StreamingEdit): void;
	/** Update virtualization window */
	updateVirtualWindow(startIndex: number, endIndex: number): void;
	/** Load more items */
	loadMore(): Promise<void>;
	/** Refresh timeline */
	refresh(): Promise<void>;
	/** Start streaming updates */
	startStreaming(): void;
	/** Stop streaming updates */
	stopStreaming(): void;

	// Lesser-specific selectors
	/** Get items with cost below threshold */
	getItemsWithCost(maxCost?: number): TimelineItem[];
	/** Get items with minimum trust score */
	getItemsWithTrustScore(minScore: number): TimelineItem[];
	/** Get items with community notes */
	getItemsWithCommunityNotes(): TimelineItem[];
	/** Get quote posts */
	getQuotePosts(): TimelineItem[];
	/** Get items flagged by moderation */
	getModeratedItems(action?: string): TimelineItem[];
}

export interface NotificationStore extends BaseStore<NotificationState> {
	/** Add new notification */
	addNotification(notification: Omit<Notification, 'id' | 'timestamp'>): string;
	/** Mark notification as read */
	markAsRead(id: string): boolean;
	/** Mark all notifications as read */
	markAllAsRead(): void;
	/** Remove notification */
	removeNotification(id: string): boolean;
	/** Clear all notifications */
	clearAll(): void;
	/** Update notification filter */
	updateFilter(filter: Partial<NotificationFilter>): void;
	/** Load next page of notifications */
	loadMore(): Promise<void>;
	/** Refresh notifications from the beginning */
	refresh(): Promise<void>;
	/** Start streaming updates */
	startStreaming(): void;
	/** Stop streaming updates */
	stopStreaming(): void;

	// Lesser-specific selectors
	/** Get quote notifications */
	getQuoteNotifications(): Notification[];
	/** Get community note notifications */
	getCommunityNoteNotifications(): Notification[];
	/** Get trust update notifications */
	getTrustUpdateNotifications(): Notification[];
	/** Get cost alert notifications */
	getCostAlertNotifications(): Notification[];
	/** Get moderation action notifications */
	getModerationActionNotifications(): Notification[];
	/** Get all unread Lesser notifications */
	getUnreadLesserNotifications(): Notification[];
}

export interface PresenceStore extends BaseStore<PresenceState> {
	/** Update current user presence */
	updatePresence(presence: Partial<Omit<UserPresence, 'userId'>>): void;
	/** Update current user location */
	updateLocation(location: UserPresence['location']): void;
	/** Set user status */
	setStatus(status: UserPresence['status'], message?: string): void;
	/** Get user presence by ID */
	getUserPresence(userId: string): UserPresence | null;
	/** Get active sessions */
	getActiveSessions(): SessionInfo[];
	/** Start presence monitoring */
	startMonitoring(): void;
	/** Stop presence monitoring */
	stopMonitoring(): void;
}
