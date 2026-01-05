import type {
	TransportManagerConfig,
	TransportManagerState,
	TransportType,
	TransportAdapter,
	WebSocketEventHandler,
	TransportEventName,
} from './types';
/**
 * Unified transport manager with automatic fallback through WebSocket → SSE → HTTP Polling
 *
 * Features:
 * - Feature detection for transport support
 * - Automatic fallback on persistent failures
 * - Unified event interface across all transports
 * - Transport upgrading after successful connections
 * - Comprehensive connection state management
 */
export declare class TransportManager implements TransportAdapter<TransportManagerState> {
	private config;
	private state;
	private currentTransport;
	private eventHandlers;
	private unsubscribers;
	private upgradeTimer;
	private isDestroyed;
	private isExplicitDisconnect;
	private consecutiveFailures;
	private readonly logger;
	constructor(config: TransportManagerConfig);
	/**
	 * Feature detection for transport support
	 */
	static getFeatureSupport(): Record<TransportType, boolean>;
	/**
	 * Connect using the optimal transport
	 */
	connect(): void;
	/**
	 * Disconnect the current transport
	 */
	disconnect(): void;
	/**
	 * Destroy the manager and cleanup all resources
	 */
	destroy(): void;
	/**
	 * Send a message through the current transport
	 */
	send(message: unknown): void;
	/**
	 * Subscribe to events
	 * Validates event names against the TransportEventMap to prevent undefined subscriptions
	 */
	on(event: TransportEventName | string, handler: WebSocketEventHandler): () => void;
	/**
	 * Get the current state
	 */
	getState(): Readonly<TransportManagerState>;
	/**
	 * Get the current active transport type
	 */
	getActiveTransport(): TransportType | null;
	/**
	 * Check if specific transport is supported
	 */
	isTransportSupported(transport: TransportType): boolean;
	/**
	 * Force switch to a specific transport
	 */
	switchTransport(transportType: TransportType, reason?: string): void;
	private detectTransportPriority;
	private selectOptimalTransport;
	private selectFallbackTransport;
	private connectWithTransport;
	private setupTransportEventHandlers;
	private subscribeAllHandlers;
	private subscribeHandlerToCurrentTransport;
	private addUnsubscribers;
	private cleanupHandlerUnsubscribers;
	private handleTransportError;
	private handleTransportDisconnection;
	private shouldAttemptFallback;
	private attemptFallback;
	private startUpgradeTimer;
	private stopUpgradeTimer;
	private attemptTransportUpgrade;
	private cleanupTransport;
	private setState;
	private emit;
	private loadLastEventId;
}
//# sourceMappingURL=TransportManager.d.ts.map
