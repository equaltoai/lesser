import type {
	WebSocketClientConfig,
	WebSocketClientState,
	WebSocketMessage,
	WebSocketEventHandler,
	TransportAdapter,
} from './types';
/**
 * WebSocketClient with automatic reconnection, heartbeat, and latency sampling
 */
export declare class WebSocketClient implements TransportAdapter<WebSocketClientState> {
	private config;
	private socket;
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
	private readonly logger;
	constructor(config: WebSocketClientConfig);
	/**
	 * Connect to the WebSocket server
	 */
	connect(): void;
	/**
	 * Disconnect from the WebSocket server
	 */
	disconnect(): void;
	/**
	 * Destroy the client and cleanup all resources
	 */
	destroy(): void;
	/**
	 * Send a message through the WebSocket
	 */
	send(message: WebSocketMessage): void;
	/**
	 * Subscribe to WebSocket events
	 */
	on(event: string, handler: WebSocketEventHandler): () => void;
	/**
	 * Get the current state
	 */
	getState(): Readonly<WebSocketClientState>;
	/**
	 * Get average latency from recent samples
	 */
	getAverageLatency(): number | null;
	private setupEventListeners;
	private handleOpen;
	private handleClose;
	private handleError;
	private handleMessage;
	private startHeartbeat;
	private stopHeartbeat;
	private handlePong;
	private startLatencySampling;
	private stopLatencySampling;
	private addLatencySample;
	private scheduleReconnect;
	private cleanup;
	private setState;
	private resolveError;
	private emit;
	private loadLastEventId;
	private saveLastEventId;
}
//# sourceMappingURL=WebSocketClient.d.ts.map
