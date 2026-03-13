import type { TransportLogger } from './types.js';
/**
 * WebSocket Connection Pool
 *
 * Manages a pool of WebSocket connections for efficient resource usage.
 * Implements connection reuse, automatic reconnection, and health monitoring.
 */
export interface WebSocketPoolConfig {
	/**
	 * Maximum number of concurrent connections
	 * @default 5
	 */
	maxConnections?: number;
	/**
	 * Connection timeout in milliseconds
	 * @default 30000
	 */
	connectionTimeout?: number;
	/**
	 * Heartbeat interval in milliseconds
	 * @default 30000
	 */
	heartbeatInterval?: number;
	/**
	 * Maximum reconnection attempts
	 * @default 3
	 */
	maxReconnectAttempts?: number;
	/**
	 * Reconnection delay in milliseconds
	 * @default 1000
	 */
	reconnectDelay?: number;
	/**
	 * Idle connection timeout in milliseconds
	 * @default 300000 (5 minutes)
	 */
	idleTimeout?: number;
	/**
	 * Optional logger implementation for diagnostics
	 */
	logger?: TransportLogger;
}
export interface PooledWebSocket {
	/**
	 * The WebSocket connection
	 */
	socket: WebSocket;
	/**
	 * Connection URL
	 */
	url: string;
	/**
	 * Connection state
	 */
	state: 'connecting' | 'connected' | 'disconnecting' | 'disconnected';
	/**
	 * Number of active subscriptions using this connection
	 */
	refCount: number;
	/**
	 * Last activity timestamp
	 */
	lastActivity: number;
	/**
	 * Reconnection attempts
	 */
	reconnectAttempts: number;
	/**
	 * Heartbeat interval ID
	 */
	heartbeatId?: ReturnType<typeof setInterval>;
}
/**
 * WebSocket Connection Pool
 */
export declare class WebSocketPool {
	private config;
	private connections;
	private eventHandlers;
	private readonly logger;
	constructor(config?: WebSocketPoolConfig);
	/**
	 * Acquire a WebSocket connection for the given URL
	 */
	acquire(url: string): Promise<PooledWebSocket>;
	/**
	 * Release a connection (decrement reference count)
	 */
	release(url: string): void;
	/**
	 * Subscribe to messages on a WebSocket
	 */
	subscribe(url: string, handler: (event: MessageEvent) => void): () => void;
	/**
	 * Unsubscribe from messages
	 */
	private unsubscribe;
	/**
	 * Send message through a pooled WebSocket
	 */
	send(url: string, data: string | ArrayBuffer | Blob): void;
	/**
	 * Close a specific connection
	 */
	close(url: string): void;
	/**
	 * Close all connections
	 */
	closeAll(): void;
	/**
	 * Get pool statistics
	 */
	getStats(): {
		totalConnections: number;
		activeConnections: number;
		idleConnections: number;
		totalRefCount: number;
	};
	/**
	 * Create a new WebSocket connection
	 */
	private createConnection;
	/**
	 * Wait for WebSocket to connect
	 */
	private waitForConnection;
	/**
	 * Setup heartbeat for connection health monitoring
	 */
	private setupHeartbeat;
	/**
	 * Clear heartbeat interval
	 */
	private clearHeartbeat;
	/**
	 * Reconnect a connection
	 */
	private reconnect;
	/**
	 * Close a connection
	 */
	private closeConnection;
	/**
	 * Cleanup idle connections
	 */
	private cleanupIdleConnections;
	/**
	 * Evict least recently used connection
	 */
	private evictLRU;
}
/**
 * Get the global WebSocket pool instance
 */
export declare function getGlobalWebSocketPool(config?: WebSocketPoolConfig): WebSocketPool;
/**
 * Reset the global WebSocket pool (useful for testing)
 */
export declare function resetGlobalWebSocketPool(): void;
//# sourceMappingURL=WebSocketPool.d.ts.map
