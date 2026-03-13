import { resolveLogger } from './logger.js';
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
export class WebSocketPool {
	private config: Required<Omit<WebSocketPoolConfig, 'logger'>>;
	private connections: Map<string, PooledWebSocket>;
	private eventHandlers: Map<string, Set<(event: MessageEvent) => void>>;
	private readonly logger: Required<TransportLogger>;

	constructor(config: WebSocketPoolConfig = {}) {
		this.logger = resolveLogger(config.logger);
		this.config = {
			maxConnections: config.maxConnections ?? 5,
			connectionTimeout: config.connectionTimeout ?? 30000,
			heartbeatInterval: config.heartbeatInterval ?? 30000,
			maxReconnectAttempts: config.maxReconnectAttempts ?? 3,
			reconnectDelay: config.reconnectDelay ?? 1000,
			idleTimeout: config.idleTimeout ?? 300000,
		};

		this.connections = new Map();
		this.eventHandlers = new Map();

		// Cleanup idle connections periodically
		if (typeof setInterval !== 'undefined') {
			setInterval(() => {
				this.cleanupIdleConnections();
			}, 60000); // Check every minute
		}
	}

	/**
	 * Acquire a WebSocket connection for the given URL
	 */
	async acquire(url: string): Promise<PooledWebSocket> {
		const existingConnection = this.connections.get(url);

		if (existingConnection) {
			if (existingConnection.state === 'connected') {
				existingConnection.refCount++;
				existingConnection.lastActivity = Date.now();
				return existingConnection;
			}

			if (existingConnection.state === 'disconnected') {
				this.connections.delete(url);
			}
		}

		// Check connection limit
		if (this.connections.size >= this.config.maxConnections) {
			// Try to evict least recently used connection
			const evicted = this.evictLRU();
			if (!evicted) {
				throw new Error('WebSocket pool exhausted');
			}
		}

		// Create new connection
		return this.createConnection(url);
	}

	/**
	 * Release a connection (decrement reference count)
	 */
	release(url: string): void {
		const connection = this.connections.get(url);
		if (!connection) return;

		connection.refCount = Math.max(0, connection.refCount - 1);
		connection.lastActivity = Date.now();

		// Don't close immediately, let idle timeout handle it
	}

	/**
	 * Subscribe to messages on a WebSocket
	 */
	subscribe(url: string, handler: (event: MessageEvent) => void): () => void {
		let handlers = this.eventHandlers.get(url);
		if (!handlers) {
			handlers = new Set();
			this.eventHandlers.set(url, handlers);
		}

		handlers.add(handler);

		// Return unsubscribe function
		return () => {
			this.unsubscribe(url, handler);
		};
	}

	/**
	 * Unsubscribe from messages
	 */
	private unsubscribe(url: string, handler: (event: MessageEvent) => void): void {
		const handlers = this.eventHandlers.get(url);
		if (handlers) {
			handlers.delete(handler);
			if (handlers.size === 0) {
				this.eventHandlers.delete(url);
			}
		}
	}

	/**
	 * Send message through a pooled WebSocket
	 */
	send(url: string, data: string | ArrayBuffer | Blob): void {
		const connection = this.connections.get(url);
		if (!connection) {
			throw new Error(`No connection found for ${url}`);
		}

		if (connection.state !== 'connected') {
			throw new Error(`Connection not ready: ${connection.state}`);
		}

		connection.socket.send(data);
		connection.lastActivity = Date.now();
	}

	/**
	 * Close a specific connection
	 */
	close(url: string): void {
		const connection = this.connections.get(url);
		if (!connection) return;

		this.closeConnection(connection);
		this.connections.delete(url);
		this.eventHandlers.delete(url);
	}

	/**
	 * Close all connections
	 */
	closeAll(): void {
		for (const [url] of this.connections) {
			this.close(url);
		}
	}

	/**
	 * Get pool statistics
	 */
	getStats(): {
		totalConnections: number;
		activeConnections: number;
		idleConnections: number;
		totalRefCount: number;
	} {
		let activeCount = 0;
		let idleCount = 0;
		let totalRefCount = 0;

		for (const connection of this.connections.values()) {
			totalRefCount += connection.refCount;

			if (connection.refCount > 0) {
				activeCount++;
			} else {
				idleCount++;
			}
		}

		return {
			totalConnections: this.connections.size,
			activeConnections: activeCount,
			idleConnections: idleCount,
			totalRefCount,
		};
	}

	/**
	 * Create a new WebSocket connection
	 */
	private async createConnection(
		url: string,
		initialReconnectAttempts = 0
	): Promise<PooledWebSocket> {
		const socket = new WebSocket(url);

		const connection: PooledWebSocket = {
			socket,
			url,
			state: 'connecting',
			refCount: 1,
			lastActivity: Date.now(),
			reconnectAttempts: initialReconnectAttempts,
		};

		this.connections.set(url, connection);

		// Setup event handlers
		socket.onopen = () => {
			connection.state = 'connected';
			connection.reconnectAttempts = 0;
			this.setupHeartbeat(connection);
		};

		socket.onclose = () => {
			connection.state = 'disconnected';
			this.clearHeartbeat(connection);

			// Attempt reconnection if needed
			if (
				connection.refCount > 0 &&
				connection.reconnectAttempts < this.config.maxReconnectAttempts
			) {
				this.reconnect(connection);
			}
		};

		socket.onerror = (error) => {
			this.logger.error(`WebSocket error on ${url}`, { error });
			connection.state = 'disconnected';
		};

		socket.onmessage = (event) => {
			connection.lastActivity = Date.now();

			// Notify all subscribers
			const handlers = this.eventHandlers.get(url);
			if (handlers) {
				for (const handler of handlers) {
					try {
						handler(event);
					} catch (error) {
						this.logger.error('Error in WebSocket message handler', { error });
					}
				}
			}
		};

		// Wait for connection with timeout
		await this.waitForConnection(socket, this.config.connectionTimeout);

		return connection;
	}

	/**
	 * Wait for WebSocket to connect
	 */
	private waitForConnection(socket: WebSocket, timeout: number): Promise<void> {
		return new Promise((resolve, reject) => {
			const timer = setTimeout(() => {
				reject(new Error('WebSocket connection timeout'));
			}, timeout);

			const checkConnection = () => {
				if (socket.readyState === WebSocket.OPEN) {
					clearTimeout(timer);
					resolve();
				} else if (
					socket.readyState === WebSocket.CLOSED ||
					socket.readyState === WebSocket.CLOSING
				) {
					clearTimeout(timer);
					reject(new Error('WebSocket connection failed'));
				} else {
					setTimeout(checkConnection, 100);
				}
			};

			checkConnection();
		});
	}

	/**
	 * Setup heartbeat for connection health monitoring
	 */
	private setupHeartbeat(connection: PooledWebSocket): void {
		if (typeof setInterval === 'undefined') return;

		connection.heartbeatId = setInterval(() => {
			if (connection.socket.readyState === WebSocket.OPEN) {
				try {
					connection.socket.send(JSON.stringify({ type: 'ping' }));
				} catch (error) {
					this.logger.error('Heartbeat failed', { error });
					this.reconnect(connection);
				}
			}
		}, this.config.heartbeatInterval);
	}

	/**
	 * Clear heartbeat interval
	 */
	private clearHeartbeat(connection: PooledWebSocket): void {
		if (connection.heartbeatId) {
			clearInterval(connection.heartbeatId);
			connection.heartbeatId = undefined;
		}
	}

	/**
	 * Reconnect a connection
	 */
	private async reconnect(connection: PooledWebSocket): Promise<void> {
		connection.reconnectAttempts++;

		if (connection.reconnectAttempts > this.config.maxReconnectAttempts) {
			this.logger.error(`Max reconnection attempts reached for ${connection.url}`);
			this.connections.delete(connection.url);
			return;
		}

		// Wait before reconnecting
		await new Promise((resolve) => setTimeout(resolve, this.config.reconnectDelay));

		try {
			await this.createConnection(connection.url, connection.reconnectAttempts);
		} catch (error) {
			this.logger.error(`Reconnection failed for ${connection.url}`, { error });
		}
	}

	/**
	 * Close a connection
	 */
	private closeConnection(connection: PooledWebSocket): void {
		this.clearHeartbeat(connection);

		connection.state = 'disconnecting';

		try {
			if (
				connection.socket.readyState === WebSocket.OPEN ||
				connection.socket.readyState === WebSocket.CONNECTING
			) {
				connection.socket.close();
			}
		} catch (error) {
			this.logger.error('Error closing WebSocket', { error });
		}

		connection.state = 'disconnected';
	}

	/**
	 * Cleanup idle connections
	 */
	private cleanupIdleConnections(): void {
		const now = Date.now();

		for (const [url, connection] of this.connections) {
			// Close connections with no active subscriptions and idle for too long
			if (connection.refCount === 0 && now - connection.lastActivity > this.config.idleTimeout) {
				this.logger.info('Closing idle connection', { url });
				this.close(url);
			}
		}
	}

	/**
	 * Evict least recently used connection
	 */
	private evictLRU(): boolean {
		let lruConnection: PooledWebSocket | null = null;
		let lruUrl: string | null = null;
		let oldestActivity = Date.now();

		// Find connection with ref count 0 and oldest activity
		for (const [url, connection] of this.connections) {
			if (connection.refCount === 0 && connection.lastActivity < oldestActivity) {
				lruConnection = connection;
				lruUrl = url;
				oldestActivity = connection.lastActivity;
			}
		}

		if (lruConnection && lruUrl) {
			this.logger.info('Evicting least recently used connection', { url: lruUrl });
			this.close(lruUrl);
			return true;
		}

		return false;
	}
}

/**
 * Create a global WebSocket pool instance
 */
let globalPool: WebSocketPool | null = null;

/**
 * Get the global WebSocket pool instance
 */
export function getGlobalWebSocketPool(config?: WebSocketPoolConfig): WebSocketPool {
	if (!globalPool) {
		globalPool = new WebSocketPool(config);
	}
	return globalPool;
}

/**
 * Reset the global WebSocket pool (useful for testing)
 */
export function resetGlobalWebSocketPool(): void {
	if (globalPool) {
		globalPool.closeAll();
		globalPool = null;
	}
}
