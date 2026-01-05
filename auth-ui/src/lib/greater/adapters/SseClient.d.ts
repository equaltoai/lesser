import type {
	SseClientConfig,
	SseClientState,
	WebSocketEventHandler,
	TransportAdapter,
} from './types';
/**
 * Server-Sent Events client with automatic reconnection, heartbeat, and latency sampling
 */
export declare class SseClient implements TransportAdapter<SseClientState> {
	private config;
	private eventSource;
	private state;
	private eventHandlers;
	private reconnectTimer;
	private heartbeatTimer;
	private heartbeatTimeoutTimer;
	private latencySamplingTimer;
	private latencySamples;
	private pendingPings;
	private isDestroyed;
	private isExplicitDisconnect;
	private abortController;
	private readonly logger;
	constructor(config: SseClientConfig);
	/**
	 * Check if EventSource is supported
	 */
	static isSupported(): boolean;
	/**
	 * Connect to the SSE endpoint
	 */
	connect(): void;
	/**
	 * Connect using fetch for custom headers support
	 */
	private connectWithFetch;
	/**
	 * Process a single SSE line
	 */
	private processLine;
	private pendingEventType?;
	private serverSuggestedRetry?;
	/**
	 * Disconnect from the SSE endpoint
	 */
	disconnect(): void;
	/**
	 * Destroy the client and cleanup all resources
	 */
	destroy(): void;
	/**
	 * Subscribe to SSE events
	 */
	on(event: string, handler: WebSocketEventHandler): () => void;
	/**
	 * Get the current state
	 */
	getState(): Readonly<SseClientState>;
	/**
	 * Get average latency from recent samples
	 */
	getAverageLatency(): number | null;
	private setupEventListeners;
	private handleEventSourceData;
	private handleOpen;
	private handleClose;
	private handleError;
	private handleMessage;
	private parseMessage;
	private isTransportMessage;
	private toHeartbeatMessage;
	private isAbortError;
	private startHeartbeat;
	private sendPing;
	private stopHeartbeat;
	private handlePong;
	private startLatencySampling;
	private stopLatencySampling;
	private addLatencySample;
	private scheduleReconnect;
	private cleanup;
	private setState;
	private emit;
	private loadLastEventId;
	private saveLastEventId;
}
//# sourceMappingURL=SseClient.d.ts.map
