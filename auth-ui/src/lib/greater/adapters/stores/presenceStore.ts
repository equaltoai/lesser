/**
 * Presence Store - Reactive state management for user presence and connection monitoring
 * Built for Svelte 5 runes compatibility with fallback support
 */

import type {
	PresenceStore,
	PresenceState,
	UserPresence,
	SessionInfo,
	PresenceConfig,
} from './types';
import { mapLesserAccount } from '../mappers/lesser/mappers.js';
import type { LesserAccountFragment } from '../mappers/lesser/types.js';
import type { UnifiedAccount } from '../models/unified.js';
import type {
	RelationshipUpdatesSubscription,
	TrustUpdatesSubscription,
} from '../graphql/generated/types.js';
import type { WebSocketEvent } from '../types.js';

type RelationshipUpdatePayload = RelationshipUpdatesSubscription['relationshipUpdates'];
type TrustUpdatePayload = TrustUpdatesSubscription['trustUpdates'];

const STATUS_PRIORITY: Record<UserPresence['status'], number> = {
	active: 1,
	busy: 2,
	away: 3,
	idle: 4,
	offline: 5,
};

let fallbackSessionCounter = 0;

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === 'object' && value !== null;

const isPresenceStatus = (value: unknown): value is UserPresence['status'] =>
	typeof value === 'string' && value in STATUS_PRIORITY;

const isConnectionStatus = (value: unknown): value is SessionInfo['connectionStatus'] =>
	value === 'connected' || value === 'reconnecting' || value === 'disconnected';

function parseUserPresencePayload(payload: unknown): UserPresence | null {
	if (!isRecord(payload)) return null;

	const userId = payload['userId'];
	const displayName = payload['displayName'];
	const isOnline = payload['isOnline'];
	const lastSeen = payload['lastSeen'];
	const status = payload['status'];

	if (
		typeof userId !== 'string' ||
		typeof displayName !== 'string' ||
		typeof isOnline !== 'boolean' ||
		typeof lastSeen !== 'number' ||
		!isPresenceStatus(status)
	) {
		return null;
	}

	const presence: UserPresence = {
		userId,
		displayName,
		isOnline,
		lastSeen,
		status,
	};

	const avatar = payload['avatar'];
	if (typeof avatar === 'string') {
		presence.avatar = avatar;
	} else if (typeof avatar !== 'undefined') {
		return null;
	}

	const statusMessage = payload['statusMessage'];
	if (typeof statusMessage === 'string') {
		presence.statusMessage = statusMessage;
	} else if (typeof statusMessage !== 'undefined') {
		return null;
	}

	const locationValue = payload['location'];
	if (typeof locationValue !== 'undefined') {
		if (!isRecord(locationValue)) {
			return null;
		}

		const location: Exclude<UserPresence['location'], undefined> = {};
		const page = locationValue['page'];
		if (typeof page === 'string') {
			location.page = page;
		} else if (typeof page !== 'undefined') {
			return null;
		}

		const section = locationValue['section'];
		if (typeof section === 'string') {
			location.section = section;
		} else if (typeof section !== 'undefined') {
			return null;
		}

		const coordinatesValue = locationValue['coordinates'];
		if (typeof coordinatesValue !== 'undefined') {
			if (!isRecord(coordinatesValue)) {
				return null;
			}

			const x = coordinatesValue['x'];
			const y = coordinatesValue['y'];
			if (typeof x !== 'number' || typeof y !== 'number') {
				return null;
			}

			location.coordinates = { x, y };
		}

		if (Object.keys(location).length > 0) {
			presence.location = location;
		}
	}

	const connectionValue = payload['connection'];
	if (typeof connectionValue !== 'undefined') {
		if (!isRecord(connectionValue)) {
			return null;
		}

		const sessionId = connectionValue['sessionId'];
		const transportType = connectionValue['transportType'];
		if (typeof sessionId !== 'string' || typeof transportType !== 'string') {
			return null;
		}

		const connection: UserPresence['connection'] = {
			sessionId,
			transportType,
		};

		const latency = connectionValue['latency'];
		if (typeof latency === 'number') {
			connection.latency = latency;
		} else if (typeof latency !== 'undefined') {
			return null;
		}

		presence.connection = connection;
	}

	return presence;
}

function parseSessionInfoPayload(payload: Record<string, unknown>): SessionInfo | null {
	const sessionId = payload['sessionId'];
	const userId = payload['userId'];
	const startTime = payload['startTime'];
	const lastActivity = payload['lastActivity'];
	const connectionStatus = payload['connectionStatus'];
	const transportType = payload['transportType'];
	const metricsValue = payload['metrics'];

	if (
		typeof sessionId !== 'string' ||
		typeof userId !== 'string' ||
		typeof startTime !== 'number' ||
		typeof lastActivity !== 'number' ||
		!isConnectionStatus(connectionStatus)
	) {
		return null;
	}

	const session: SessionInfo = {
		sessionId,
		userId,
		startTime,
		lastActivity,
		connectionStatus,
	};

	if (typeof transportType === 'string') {
		session.transportType = transportType;
	} else if (typeof transportType !== 'undefined') {
		return null;
	}

	if (typeof metricsValue !== 'undefined') {
		if (!isRecord(metricsValue)) {
			return null;
		}

		const latency = metricsValue['latency'];
		const packetLoss = metricsValue['packetLoss'];
		const connectionStability = metricsValue['connectionStability'];

		if (
			typeof latency !== 'number' ||
			typeof packetLoss !== 'number' ||
			typeof connectionStability !== 'number'
		) {
			return null;
		}

		session.metrics = {
			latency,
			packetLoss,
			connectionStability,
		};
	}

	return session;
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

function createReactiveMap<K, V>(
	source: Map<K, V> | Iterable<[K, V]> | undefined,
	onChange: () => void
): Map<K, V> {
	const base = source instanceof Map ? new Map(source) : source ? new Map(source) : new Map<K, V>();

	const proxy = new Proxy(base, {
		get(target, prop, receiver) {
			if (prop === 'size') {
				return target.size;
			}

			if (prop === 'set') {
				return (key: K, value: V) => {
					target.set(key, value);
					onChange();
					return receiver;
				};
			}

			if (prop === 'delete') {
				return (key: K) => {
					const removed = target.delete(key);
					if (removed) {
						onChange();
					}
					return removed;
				};
			}

			if (prop === 'clear') {
				return () => {
					if (target.size > 0) {
						target.clear();
						onChange();
					} else {
						target.clear();
					}
				};
			}

			const value = Reflect.get(target, prop, receiver);
			if (typeof value === 'function') {
				return value.bind(target);
			}
			return value;
		},
	});

	return proxy as Map<K, V>;
}

export function createPresenceStore(config: PresenceConfig): PresenceStore {
	// Create reactive state
	const state = new ReactiveState<PresenceState>({
		currentUser: null,
		users: createReactiveUsersMap(),
		sessions: createReactiveSessionsMap(),
		connectionHealth: {
			status: 'disconnected',
			latency: null,
			lastHeartbeat: null,
			reconnectAttempts: 0,
		},
		stats: {
			totalUsers: 0,
			onlineUsers: 0,
			activeUsers: 0,
			idleUsers: 0,
		},
		isLoading: false,
		error: null,
		isStreaming: false,
	});

	// Initialize current user
	if (config.currentUser) {
		const currentUserPresence: UserPresence = {
			...config.currentUser,
			isOnline: true,
			lastSeen: Date.now(),
			status: 'active',
			location:
				config.enableLocationTracking && typeof window !== 'undefined'
					? { page: window.location.pathname }
					: undefined,
			connection: {
				sessionId: generateSessionId(),
				transportType: 'unknown',
				latency: undefined,
			},
		};

		state.update((current) => {
			const users = cloneUsersMap(current.users);
			users.set(config.currentUser.userId, currentUserPresence);

			return {
				...current,
				currentUser: currentUserPresence,
				users,
			};
		});
	}

	// Initialize computed values
	updateDerivedValues();

	// Connection health monitoring
	let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
	let inactivityTimer: ReturnType<typeof setTimeout> | null = null;
	let updateDebounceTimer: ReturnType<typeof setTimeout> | null = null;
	let lastActivity = Date.now();

	// Transport event handlers
	let streamingUnsubscribers: (() => void)[] = [];
	const pendingPresenceUpdates: Map<string, UserPresence> = new Map();

	function createReactiveUsersMap(source?: Map<string, UserPresence>): Map<string, UserPresence> {
		return createReactiveMap(source, updateDerivedValues);
	}

	function createReactiveSessionsMap(source?: Map<string, SessionInfo>): Map<string, SessionInfo> {
		return createReactiveMap(source, () => {});
	}

	function cloneUsersMap(source: Map<string, UserPresence>): Map<string, UserPresence> {
		return createReactiveUsersMap(source);
	}

	function cloneSessionsMap(source: Map<string, SessionInfo>): Map<string, SessionInfo> {
		return createReactiveSessionsMap(source);
	}

	function mergePresence(existing: UserPresence | undefined, incoming: UserPresence): UserPresence {
		if (!existing) {
			return { ...incoming };
		}

		const existingPriority = STATUS_PRIORITY[existing.status] ?? 0;
		const incomingPriority = STATUS_PRIORITY[incoming.status] ?? 0;
		const nextStatus = incomingPriority >= existingPriority ? incoming.status : existing.status;

		const location = mergeLocation(existing.location, incoming.location);
		const connection = mergeConnection(existing.connection, incoming.connection);

		return {
			...existing,
			...incoming,
			status: nextStatus,
			lastSeen: Math.max(existing.lastSeen, incoming.lastSeen),
			location,
			connection,
		};
	}

	function mergeLocation(
		existing: UserPresence['location'],
		incoming: UserPresence['location']
	): UserPresence['location'] {
		if (!existing && !incoming) {
			return undefined;
		}

		return {
			...(existing || {}),
			...(incoming || {}),
		};
	}

	function mergeConnection(
		existing: UserPresence['connection'],
		incoming: UserPresence['connection']
	): UserPresence['connection'] {
		if (!existing && !incoming) {
			return undefined;
		}

		if (!existing) {
			return incoming;
		}

		if (!incoming) {
			return existing;
		}

		return {
			sessionId: incoming.sessionId || existing.sessionId,
			transportType: incoming.transportType || existing.transportType,
			latency: typeof incoming.latency === 'number' ? incoming.latency : existing.latency,
		};
	}

	function updateDerivedValues(): void {
		const currentState = state.value;
		const users = Array.from(currentState.users.values());

		const stats = {
			totalUsers: users.length,
			onlineUsers: users.filter((u) => u.isOnline).length,
			activeUsers: users.filter((u) => u.status === 'active').length,
			idleUsers: users.filter((u) => u.status === 'idle').length,
		};

		const existing = currentState.stats;
		if (
			existing.totalUsers === stats.totalUsers &&
			existing.onlineUsers === stats.onlineUsers &&
			existing.activeUsers === stats.activeUsers &&
			existing.idleUsers === stats.idleUsers
		) {
			return;
		}

		state.update((current) => ({
			...current,
			stats,
		}));
	}

	function processPendingUpdates(): void {
		if (pendingPresenceUpdates.size === 0) return;

		const updates = new Map(pendingPresenceUpdates);
		pendingPresenceUpdates.clear();

		state.update((current) => {
			const users = cloneUsersMap(current.users);

			updates.forEach((presence, userId) => {
				users.set(userId, presence);
			});

			const updatedCurrentUser = updates.get(config.currentUser.userId) ?? current.currentUser;

			return {
				...current,
				users,
				currentUser: updatedCurrentUser ?? current.currentUser,
			};
		});

		updateDerivedValues();
	}

	function schedulePresenceUpdate(presence: UserPresence): void {
		const existingPending = pendingPresenceUpdates.get(presence.userId);
		const currentPresence = state.value.users.get(presence.userId);
		const mergedPresence = mergePresence(existingPending ?? currentPresence, presence);

		pendingPresenceUpdates.set(presence.userId, mergedPresence);

		if (updateDebounceTimer) {
			clearTimeout(updateDebounceTimer);
		}

		updateDebounceTimer = setTimeout(() => {
			processPendingUpdates();
			updateDebounceTimer = null;
		}, config.updateDebounceMs || 250);
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

	function mapAccountToPresence(
		account: UnifiedAccount,
		overrides: Partial<UserPresence> = {}
	): UserPresence {
		const base: UserPresence = {
			userId: account.id,
			displayName: account.displayName || account.username,
			avatar: account.avatar || undefined,
			isOnline: true,
			lastSeen: Date.now(),
			status: 'active',
			statusMessage: undefined,
			location: undefined,
			connection: overrides.connection,
		};

		return {
			...base,
			...overrides,
			status: overrides.status ?? base.status,
			isOnline: overrides.isOnline ?? base.isOnline,
			lastSeen: overrides.lastSeen ?? base.lastSeen,
		};
	}

	function mapActorToPresence(
		actor: LesserAccountFragment,
		overrides: Partial<UserPresence> = {}
	): UserPresence | null {
		const result = mapLesserAccount(actor);
		if (!result.success || !result.data) {
			if (result.error) {
				console.warn('[PresenceStore] Failed to map Lesser actor to presence', result.error);
			}
			return null;
		}

		return mapAccountToPresence(result.data, overrides);
	}

	function handleRelationshipUpdate(payload: RelationshipUpdatePayload | null): void {
		if (!payload?.actor) return;

		const statusMap: Partial<Record<string, UserPresence['status']>> = {
			followed: 'active',
			unblocked: 'active',
			blocked: 'offline',
			unfollowed: 'offline',
			muted: 'away',
			unmuted: 'active',
		};

		const messageMap: Record<string, string> = {
			followed: 'Started following you',
			unfollowed: 'Stopped following you',
			blocked: 'Blocked you',
			unblocked: 'Unblocked you',
			muted: 'Muted you',
			unmuted: 'Unmuted you',
		};

		const offlineStatuses = new Set(['blocked', 'unfollowed']);
		const isOnline = !offlineStatuses.has(payload.type);
		const presence = mapActorToPresence(payload.actor as unknown as LesserAccountFragment, {
			isOnline,
			status: statusMap[payload.type] ?? 'active',
			statusMessage: messageMap[payload.type] ?? `Relationship ${payload.type}`,
			lastSeen: resolveTimestamp(payload.timestamp),
			connection: {
				sessionId: payload.relationship.id,
				transportType: 'relationship',
			},
		});

		if (presence) {
			schedulePresenceUpdate(presence);
		}
	}

	function handleTrustUpdate(payload: TrustUpdatePayload | null): void {
		if (!payload?.to) return;

		const presence = mapActorToPresence(payload.to as unknown as LesserAccountFragment, {
			status: 'active',
			isOnline: true,
			statusMessage: `Trust score: ${payload.score}`,
			lastSeen: resolveTimestamp(payload.updatedAt),
		});

		if (presence) {
			schedulePresenceUpdate(presence);
		}
	}

	function handleMetricsUpdate(data: unknown): void {
		if (!isRecord(data)) return;

		const latencyValue = data['latency'];
		if (typeof latencyValue === 'number') {
			state.update((current) => ({
				...current,
				connectionHealth: {
					...current.connectionHealth,
					latency: latencyValue,
					status: latencyValue > 250 ? 'poor' : current.connectionHealth.status,
				},
			}));
		}

		const usersValue = data['users'];
		if (Array.isArray(usersValue)) {
			usersValue.forEach((entry) => {
				const presence = parseUserPresencePayload(entry);
				if (presence) {
					schedulePresenceUpdate(presence);
				}
			});
		}

		const sessionInfo = parseSessionInfoPayload(data);
		if (sessionInfo) {
			state.update((current) => {
				const sessions = cloneSessionsMap(current.sessions);
				sessions.set(sessionInfo.sessionId, sessionInfo);
				return { ...current, sessions };
			});
		}
	}

	function startHeartbeat(): void {
		if (heartbeatTimer || !config.transportManager) return;

		const interval = config.heartbeatInterval || 30000; // 30 seconds

		heartbeatTimer = setInterval(() => {
			if (state.value.isStreaming) {
				sendHeartbeat();
			}
		}, interval);
	}

	function stopHeartbeat(): void {
		if (heartbeatTimer) {
			clearInterval(heartbeatTimer);
			heartbeatTimer = null;
		}
	}

	function sendHeartbeat(): void {
		if (!config.transportManager || !state.value.currentUser) return;

		const heartbeatData = {
			userId: state.value.currentUser.userId,
			timestamp: Date.now(),
			status: state.value.currentUser.status,
			location: state.value.currentUser.location,
		};

		try {
			config.transportManager.send({
				type: 'presence_heartbeat',
				data: heartbeatData,
			});

			// Update connection health based on transport state
			const transportState = config.transportManager.getState();
			state.update((current) => ({
				...current,
				connectionHealth: {
					...current.connectionHealth,
					status: transportState.status === 'connected' ? 'healthy' : 'poor',
					latency: transportState.latency,
					reconnectAttempts: transportState.reconnectAttempts,
					lastHeartbeat: Date.now(),
				},
			}));
		} catch (error) {
			state.update((current) => ({
				...current,
				connectionHealth: { ...current.connectionHealth, status: 'poor' },
				error: error as Error,
			}));
		}
	}

	function startInactivityMonitoring(): void {
		const threshold = config.inactivityThreshold || 300000; // 5 minutes

		function resetInactivityTimer(): void {
			const now = Date.now();
			lastActivity = now;

			if (inactivityTimer) {
				clearTimeout(inactivityTimer);
			}

			// Update last activity in presence
			if (state.value.currentUser) {
				updatePresence({ lastSeen: lastActivity });
			}

			// Set user to active if currently idle
			if (state.value.currentUser && state.value.currentUser.status === 'idle') {
				updatePresence({ status: 'active' });
			}

			inactivityTimer = setTimeout(() => {
				if (state.value.currentUser && state.value.currentUser.status === 'active') {
					updatePresence({ status: 'idle' });
				}
			}, threshold);
		}

		// Listen for user activity
		if (typeof window !== 'undefined') {
			const activityEvents = ['mousedown', 'mousemove', 'keypress', 'scroll', 'touchstart'];

			const handleActivity = () => resetInactivityTimer();

			activityEvents.forEach((event) => {
				window.addEventListener(event, handleActivity, { passive: true });
			});

			// Store cleanup function
			const cleanup = () => {
				activityEvents.forEach((event) => {
					window.removeEventListener(event, handleActivity);
				});
			};

			streamingUnsubscribers.push(cleanup);
		}

		// Initialize timer
		resetInactivityTimer();
	}

	function trackLocationChanges(): void {
		if (!config.enableLocationTracking || typeof window === 'undefined') return;

		// Listen for navigation changes
		const handleLocationChange = () => {
			if (state.value.currentUser) {
				updateLocation({
					page: window.location.pathname,
					section: window.location.hash ? window.location.hash.slice(1) : undefined,
				});
			}
		};

		window.addEventListener('popstate', handleLocationChange);
		window.addEventListener('hashchange', handleLocationChange);

		// Store cleanup function
		streamingUnsubscribers.push(() => {
			window.removeEventListener('popstate', handleLocationChange);
			window.removeEventListener('hashchange', handleLocationChange);
		});
	}

	// Store methods
	function updatePresence(presenceUpdates: Partial<Omit<UserPresence, 'userId'>>): void {
		const currentUserPresence = state.value.currentUser;
		if (!currentUserPresence) return;

		let mergedPresence = mergePresence(currentUserPresence, {
			...currentUserPresence,
			...presenceUpdates,
			userId: currentUserPresence.userId,
			lastSeen: Date.now(),
		});

		if (typeof presenceUpdates.status !== 'undefined') {
			mergedPresence = { ...mergedPresence, status: presenceUpdates.status };
		}

		state.update((current) => {
			const users = cloneUsersMap(current.users);
			users.set(mergedPresence.userId, mergedPresence);

			return {
				...current,
				currentUser: mergedPresence,
				users,
			};
		});

		updateDerivedValues();

		if (state.value.isStreaming && config.transportManager) {
			try {
				config.transportManager.send({
					type: 'presence_update',
					data: mergedPresence,
				});
			} catch (error) {
				state.update((current) => ({
					...current,
					error: error as Error,
				}));
				console.error('Failed to send presence update:', error);
			}
		}
	}

	function updateLocation(location: UserPresence['location']): void {
		if (!state.value.currentUser) return;

		updatePresence({ location });
	}

	function setStatus(status: UserPresence['status'], message?: string): void {
		if (!state.value.currentUser) return;

		updatePresence({
			status,
			statusMessage: message,
		});
	}

	function getUserPresence(userId: string): UserPresence | null {
		return state.value.users.get(userId) || null;
	}

	function getActiveSessions(): SessionInfo[] {
		return Array.from(state.value.sessions.values()).filter(
			(session) => session.connectionStatus === 'connected'
		);
	}

	function startMonitoring(): void {
		if (state.value.isStreaming || !config.transportManager) return;

		state.update((current) => ({
			...current,
			isStreaming: true,
			error: null,
		}));

		const relationshipHandler = config.transportManager.on(
			'relationshipUpdates',
			(event: WebSocketEvent) => {
				const payload = (event?.data ?? null) as RelationshipUpdatePayload | null;
				handleRelationshipUpdate(payload);
			}
		);

		const trustHandler = config.transportManager.on('trustUpdates', (event: WebSocketEvent) => {
			const payload = (event?.data ?? null) as TrustUpdatePayload | null;
			handleTrustUpdate(payload);
		});

		const metricsHandler = config.transportManager.on('metricsUpdates', (event: WebSocketEvent) => {
			handleMetricsUpdate(event?.data);
		});

		// Subscribe to connection events
		const openHandler = config.transportManager.on('open', () => {
			state.update((current) => ({
				...current,
				connectionHealth: {
					...current.connectionHealth,
					status: 'healthy',
					reconnectAttempts: 0,
				},
			}));

			// Send initial presence
			if (state.value.currentUser) {
				updatePresence({ isOnline: true });
			}
		});

		const errorHandler = config.transportManager.on('error', (event) => {
			state.update((current) => ({
				...current,
				connectionHealth: { ...current.connectionHealth, status: 'poor' },
				error: event.error || new Error('Connection error'),
			}));
		});

		const closeHandler = config.transportManager.on('close', () => {
			state.update((current) => {
				const currentUser = current.currentUser;
				const users = cloneUsersMap(current.users);

				if (currentUser) {
					const offlineUser = { ...currentUser, isOnline: false };
					users.set(offlineUser.userId, offlineUser);
				}

				return {
					...current,
					isStreaming: false,
					currentUser: currentUser ? { ...currentUser, isOnline: false } : null,
					users,
					connectionHealth: { ...current.connectionHealth, status: 'disconnected' },
				};
			});

			updateDerivedValues();
		});

		const reconnectingHandler = config.transportManager.on('reconnecting', () => {
			state.update((current) => ({
				...current,
				connectionHealth: {
					...current.connectionHealth,
					status: 'poor',
					reconnectAttempts: current.connectionHealth.reconnectAttempts + 1,
				},
			}));
		});

		streamingUnsubscribers.push(
			relationshipHandler,
			trustHandler,
			metricsHandler,
			openHandler,
			errorHandler,
			closeHandler,
			reconnectingHandler
		);

		// Start monitoring systems
		startHeartbeat();
		startInactivityMonitoring();
		trackLocationChanges();

		// Start the transport connection
		try {
			config.transportManager.connect();
		} catch (error) {
			state.update((current) => ({
				...current,
				error: error as Error,
				isStreaming: false,
				connectionHealth: { ...current.connectionHealth, status: 'disconnected' },
			}));
		}
	}

	function stopMonitoring(): void {
		state.update((current) => ({
			...current,
			isStreaming: false,
			connectionHealth: { ...current.connectionHealth, status: 'disconnected' },
		}));

		// Stop monitoring systems
		stopHeartbeat();

		if (inactivityTimer) {
			clearTimeout(inactivityTimer);
			inactivityTimer = null;
		}

		if (updateDebounceTimer) {
			clearTimeout(updateDebounceTimer);
			updateDebounceTimer = null;
		}

		// Clean up event subscriptions
		streamingUnsubscribers.forEach((unsubscribe) => unsubscribe());
		streamingUnsubscribers = [];

		// Clear pending updates
		pendingPresenceUpdates.clear();
	}

	function subscribe(callback: (value: PresenceState) => void): () => void {
		return state.subscribe(callback);
	}

	function get(): PresenceState {
		return state.value;
	}

	function destroy(): void {
		stopMonitoring();

		// Clear all state
		state.update(() => ({
			currentUser: null,
			users: createReactiveUsersMap(),
			sessions: createReactiveSessionsMap(),
			connectionHealth: {
				status: 'disconnected',
				latency: null,
				lastHeartbeat: null,
				reconnectAttempts: 0,
			},
			stats: {
				totalUsers: 0,
				onlineUsers: 0,
				activeUsers: 0,
				idleUsers: 0,
			},
			isLoading: false,
			error: null,
			isStreaming: false,
		}));
	}

	function generateSessionId(): string {
		const cryptoObj = typeof globalThis !== 'undefined' ? globalThis.crypto : undefined;
		if (cryptoObj && typeof cryptoObj.randomUUID === 'function') {
			return `session_${cryptoObj.randomUUID()}`;
		}

		if (cryptoObj && typeof cryptoObj.getRandomValues === 'function') {
			const buffer = new Uint8Array(16);
			cryptoObj.getRandomValues(buffer);
			const hex = Array.from(buffer, (value) => value.toString(16).padStart(2, '0')).join('');
			return `session_${hex}`;
		}

		fallbackSessionCounter = (fallbackSessionCounter + 1) % Number.MAX_SAFE_INTEGER;
		return `session_${Date.now()}_${fallbackSessionCounter}`;
	}

	return {
		subscribe,
		destroy,
		get,
		updatePresence,
		updateLocation,
		setStatus,
		getUserPresence,
		getActiveSessions,
		startMonitoring,
		stopMonitoring,
	};
}
