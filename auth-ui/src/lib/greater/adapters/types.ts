import type {
	TimelineUpdatesSubscription,
	NotificationStreamSubscription,
	QuoteActivitySubscription,
	HashtagActivitySubscription,
	ListUpdatesSubscription,
	RelationshipUpdatesSubscription,
	TrustUpdatesSubscription,
	ModerationEventsSubscription,
	ModerationAlertsSubscription,
	ModerationQueueUpdateSubscription,
	CostAlertsSubscription,
} from './graphql/generated/types.js';

/**
 * Transport types and interfaces for WebSocket, SSE, and HTTP Polling
 */

// Base configuration shared across all transports
export interface BaseTransportConfig {
	/** URL to connect to */
	url: string;

	/** Authentication token for connection */
	authToken?: string;

	/** Initial reconnect delay in milliseconds (default: 500) */
	initialReconnectDelay?: number;

	/** Maximum reconnect delay in milliseconds (default: 30000) */
	maxReconnectDelay?: number;

	/** Jitter factor for reconnect delay (0-1, default: 0.3) */
	jitterFactor?: number;

	/** Maximum number of reconnect attempts (default: Infinity) */
	maxReconnectAttempts?: number;

	/** Storage key for persisting lastEventId */
	lastEventIdStorageKey?: string;

	/** Custom storage adapter for lastEventId persistence */
	storage?: Storage;

	/** Optional logger implementation for transport diagnostics */
	logger?: TransportLogger;
}

export interface WebSocketClientConfig extends BaseTransportConfig {
	/** Heartbeat interval in milliseconds (default: 30000) */
	heartbeatInterval?: number;

	/** Heartbeat timeout in milliseconds (default: 60000) */
	heartbeatTimeout?: number;

	/** Enable latency sampling (default: true) */
	enableLatencySampling?: boolean;

	/** Latency sampling interval in milliseconds (default: 10000) */
	latencySamplingInterval?: number;
}

export interface WebSocketMessage {
	/** Message type/event name */
	type: string;

	/** Message payload */
	data?: unknown;

	/** Event ID for resumption */
	id?: string;

	/** Timestamp of the message */
	timestamp?: number;
}

export interface WebSocketClientState {
	/** Current connection state */
	status: 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

	/** Number of reconnection attempts */
	reconnectAttempts: number;

	/** Current latency in milliseconds */
	latency: number | null;

	/** Last error if any */
	error: Error | null;

	/** Last event ID for resumption */
	lastEventId: string | null;
}

export type WebSocketEventType =
	| 'open'
	| 'close'
	| 'error'
	| 'message'
	| 'reconnecting'
	| 'reconnected'
	| 'latency';

export interface WebSocketEvent {
	type: WebSocketEventType | TransportEventName | string;
	data?: unknown;
	error?: Error;
	latency?: number;
}

export type WebSocketEventHandler = (event: WebSocketEvent) => void;

export interface TransportLogger {
	debug?(message: string, context?: unknown): void;
	info?(message: string, context?: unknown): void;
	warn?(message: string, context?: unknown): void;
	error?(message: string, context?: unknown): void;
}

export interface HeartbeatMessage {
	type: 'ping' | 'pong';
	timestamp: number;
}

export interface LatencySample {
	timestamp: number;
	latency: number;
}

// SSE-specific types
export interface SseClientConfig extends BaseTransportConfig {
	/** Heartbeat interval in milliseconds (default: 30000) */
	heartbeatInterval?: number;

	/** Heartbeat timeout in milliseconds (default: 60000) */
	heartbeatTimeout?: number;

	/** Enable latency sampling (default: true) */
	enableLatencySampling?: boolean;

	/** Latency sampling interval in milliseconds (default: 10000) */
	latencySamplingInterval?: number;

	/** Whether to include credentials in the request (default: false) */
	withCredentials?: boolean;

	/** Custom headers to send with the SSE request */
	headers?: Record<string, string>;
}

export interface SseMessage {
	/** Event type */
	event?: string;

	/** Event data */
	data: string;

	/** Event ID for resumption */
	id?: string;

	/** Retry duration in milliseconds */
	retry?: number;
}

export interface SseClientState {
	/** Current connection state */
	status: 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

	/** Number of reconnection attempts */
	reconnectAttempts: number;

	/** Current latency in milliseconds */
	latency: number | null;

	/** Last error if any */
	error: Error | null;

	/** Last event ID for resumption */
	lastEventId: string | null;
}

// HTTP Polling-specific types
export interface HttpPollingClientConfig extends BaseTransportConfig {
	/** Polling interval in milliseconds (default: 5000) */
	pollingInterval?: number;

	/** Whether to include credentials in the request (default: false) */
	withCredentials?: boolean;

	/** Custom headers to send with requests */
	headers?: Record<string, string>;

	/** Request timeout in milliseconds (default: 30000) */
	requestTimeout?: number;

	/** Enable latency sampling (default: true) */
	enableLatencySampling?: boolean;
}

export interface HttpPollingClientState {
	/** Current connection state */
	status: 'disconnected' | 'polling' | 'waiting' | 'reconnecting';

	/** Number of reconnection attempts */
	reconnectAttempts: number;

	/** Current latency in milliseconds */
	latency: number | null;

	/** Last error if any */
	error: Error | null;

	/** Last event ID for resumption */
	lastEventId: string | null;
}

export interface HttpPollingMessage {
	/** Message type/event name */
	type: string;

	/** Message payload */
	data?: unknown;

	/** Event ID for resumption */
	id?: string;

	/** Timestamp of the message */
	timestamp?: number;
}

// Transport fallback types
export interface TransportAdapter<TState = Record<string, unknown>> {
	connect(): void;
	disconnect(): void;
	destroy(): void;
	send?(message: unknown): void;
	on(event: string, handler: WebSocketEventHandler): () => void;
	getState(): Readonly<TState>;
}

export interface TransportFallbackConfig {
	/** Primary transport config (SSE) */
	primary: SseClientConfig;

	/** Fallback transport config (HTTP Polling) */
	fallback: HttpPollingClientConfig;

	/** Whether to auto-detect and fallback (default: true) */
	autoFallback?: boolean;

	/** Force a specific transport ('sse' | 'polling' | 'auto') */
	forceTransport?: 'sse' | 'polling' | 'auto';

	/** Optional logger shared across transports */
	logger?: TransportLogger;
}

export type TransportFallbackState =
	| (SseClientState & { transport: 'sse' })
	| (HttpPollingClientState & { transport: 'polling' })
	| {
			status: 'disconnected';
			transport: null;
			error: Error | null;
	  };

// Transport Manager types for unified multi-transport support
export type TransportType = 'websocket' | 'sse' | 'polling';

export interface TransportManagerConfig {
	/** WebSocket transport configuration */
	websocket: WebSocketClientConfig;

	/** SSE transport configuration */
	sse: SseClientConfig;

	/** HTTP Polling transport configuration */
	polling: HttpPollingClientConfig;

	/** Whether to auto-detect and fallback through transport hierarchy (default: true) */
	autoFallback?: boolean;

	/** Force a specific transport ('websocket' | 'sse' | 'polling' | 'auto') */
	forceTransport?: TransportType | 'auto';

	/** Maximum number of persistent failures before switching transports (default: 3) */
	maxFailuresBeforeSwitch?: number;

	/** Whether to attempt to retry with higher-priority transports after successful connection (default: false) */
	enableUpgradeAttempts?: boolean;

	/** Interval in milliseconds for attempting transport upgrades (default: 300000 - 5 minutes) */
	upgradeAttemptInterval?: number;

	/** Optional logger for manager level diagnostics */
	logger?: TransportLogger;
}

export interface TransportManagerState {
	/** Current connection state */
	status: 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

	/** Currently active transport type */
	activeTransport: TransportType | null;

	/** Number of consecutive failures for current transport */
	failureCount: number;

	/** Whether transport switching is available */
	canFallback: boolean;

	/** Number of reconnection attempts */
	reconnectAttempts: number;

	/** Current latency in milliseconds */
	latency: number | null;

	/** Last error if any */
	error: Error | null;

	/** Last event ID for resumption */
	lastEventId: string | null;

	/** Transport priority order based on feature detection */
	transportPriority: TransportType[];
}

export interface TransportSwitchEvent {
	/** Previous transport type */
	from: TransportType | null;

	/** New transport type */
	to: TransportType;

	/** Reason for the switch */
	reason: 'fallback' | 'upgrade' | 'forced' | 'feature_detection';

	/** Error that triggered the switch (if applicable) */
	error?: Error;
}

/**
 * Transport Event Map defining all valid subscription events and their payload types
 * This ensures type safety and prevents subscription to undefined channels
 */
export interface TransportEventMap {
	// Core transport events
	open: Record<string, never>;
	close: { code?: number; reason?: string };
	error: { error?: Error };
	message: unknown;
	reconnecting: { attempt?: number; delay?: number };
	reconnected: Record<string, never>;
	latency: { latency: number };
	transport_switch: TransportSwitchEvent;

	// Lesser Subscription Events - Timeline & Social
	timelineUpdates: TimelineUpdatesSubscription['timelineUpdates'];
	notificationStream: NotificationStreamSubscription['notificationStream'];
	conversationUpdates: unknown; // Conversation from schema
	listUpdates: ListUpdatesSubscription['listUpdates'];
	activityStream: unknown; // Activity from schema
	relationshipUpdates: RelationshipUpdatesSubscription['relationshipUpdates'];

	// Lesser Subscription Events - Quote Posts
	quoteActivity: QuoteActivitySubscription['quoteActivity'];

	// Lesser Subscription Events - Hashtags
	hashtagActivity: HashtagActivitySubscription['hashtagActivity'];

	// Lesser Subscription Events - Trust & Moderation
	trustUpdates: TrustUpdatesSubscription['trustUpdates'];
	moderationEvents: ModerationEventsSubscription['moderationEvents'];
	moderationAlerts: ModerationAlertsSubscription['moderationAlerts'];
	moderationQueueUpdate: ModerationQueueUpdateSubscription['moderationQueueUpdate'];
	threatIntelligence: unknown; // ThreatAlert from schema

	// Lesser Subscription Events - AI Analysis
	aiAnalysisUpdates: unknown; // AIAnalysis from schema

	// Lesser Subscription Events - Cost & Budget
	costUpdates: unknown; // CostUpdate from schema
	costAlerts: CostAlertsSubscription['costAlerts'];
	budgetAlerts: unknown; // BudgetAlert from schema

	// Lesser Subscription Events - Metrics & Performance
	metricsUpdates: unknown; // MetricsUpdate from schema
	performanceAlert: unknown; // PerformanceAlert from schema

	// Lesser Subscription Events - Federation & Infrastructure
	federationHealthUpdates: unknown; // FederationHealthUpdate from schema
	infrastructureEvent: unknown; // InfrastructureEvent from schema
}

/**
 * Valid event names from the TransportEventMap
 */
export type TransportEventName = keyof TransportEventMap;
