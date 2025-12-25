import type {
	TransportFallbackConfig,
	TransportAdapter,
	WebSocketEventHandler,
	TransportFallbackState,
} from './types';
/**
 * Transport client with automatic fallback from SSE to HTTP Polling
 */
export declare class TransportFallback implements TransportAdapter<TransportFallbackState> {
	private readonly config;
	private currentTransport;
	private transportType;
	private eventHandlers;
	private unsubscribers;
	private isDestroyed;
	private fallbackAttempted;
	private readonly logger;
	constructor(config: TransportFallbackConfig);
	/**
	 * Connect using the appropriate transport
	 */
	connect(): void;
	/**
	 * Disconnect the current transport
	 */
	disconnect(): void;
	/**
	 * Destroy the client and cleanup all resources
	 */
	destroy(): void;
	/**
	 * Send a message through the current transport
	 */
	send(message: unknown): void;
	/**
	 * Subscribe to events
	 */
	on(event: string, handler: WebSocketEventHandler): () => void;
	/**
	 * Get the current state
	 */
	getState(): TransportFallbackState;
	/**
	 * Get the current transport type
	 */
	getTransportType(): 'sse' | 'polling' | null;
	/**
	 * Check if SSE is supported
	 */
	static isSseSupported(): boolean;
	private selectTransport;
	private connectSse;
	private connectPolling;
	private addUnsubscribers;
	private fallbackToPolling;
	private shouldFallback;
	private subscribeHandlers;
	private cleanupTransport;
	private emitToHandlers;
}
//# sourceMappingURL=TransportFallback.d.ts.map
