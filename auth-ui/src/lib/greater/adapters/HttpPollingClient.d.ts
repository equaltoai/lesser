import type {
	HttpPollingClientConfig,
	HttpPollingClientState,
	HttpPollingMessage,
	WebSocketEventHandler,
	TransportAdapter,
} from './types';
/**
 * HTTP Polling client with automatic reconnection and latency tracking
 */
export declare class HttpPollingClient implements TransportAdapter<HttpPollingClientState> {
	private config;
	private state;
	private eventHandlers;
	private pollingTimer;
	private reconnectTimer;
	private latencySamples;
	private isDestroyed;
	private isExplicitDisconnect;
	private abortController;
	private consecutiveErrors;
	private maxConsecutiveErrors;
	private readonly logger;
	constructor(config: HttpPollingClientConfig);
	/**
	 * Start polling
	 */
	connect(): void;
	/**
	 * Stop polling
	 */
	disconnect(): void;
	/**
	 * Destroy the client and cleanup all resources
	 */
	destroy(): void;
	/**
	 * Send a message (HTTP POST)
	 */
	send(message: HttpPollingMessage): Promise<void>;
	/**
	 * Subscribe to events
	 */
	on(event: string, handler: WebSocketEventHandler): () => void;
	/**
	 * Get the current state
	 */
	getState(): Readonly<HttpPollingClientState>;
	/**
	 * Get average latency from recent samples
	 */
	getAverageLatency(): number | null;
	private startPolling;
	private poll;
	private combineSignals;
	private scheduleNextPoll;
	private handleMessage;
	private handleError;
	private resolveError;
	private isAbortError;
	private handleDisconnection;
	private scheduleReconnect;
	private addLatencySample;
	private cleanup;
	private setState;
	private emit;
	private loadLastEventId;
	private saveLastEventId;
}
//# sourceMappingURL=HttpPollingClient.d.ts.map
